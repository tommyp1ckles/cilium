// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of Cilium

//go:build !privileged_tests

package cmd

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stesting "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/cache"

	"github.com/cilium/cilium/pkg/endpoint"
	"github.com/cilium/cilium/pkg/k8s"
	ciliumv2 "github.com/cilium/cilium/pkg/k8s/apis/cilium.io/v2"
	cilium_v2a1 "github.com/cilium/cilium/pkg/k8s/apis/cilium.io/v2alpha1"
	"github.com/cilium/cilium/pkg/k8s/client/clientset/versioned/fake"
	slimmetav1 "github.com/cilium/cilium/pkg/k8s/slim/k8s/apis/meta/v1"
	"github.com/cilium/cilium/pkg/k8s/types"
	"github.com/cilium/cilium/pkg/k8s/watchers"
	"github.com/cilium/cilium/pkg/lock"
)

func TestCleanStaleCeps(t *testing.T) {
	assert := assert.New(t)
	tests := map[string]struct {
		ciliumEndpoints []types.CiliumEndpoint
		// should only be used if disableCEPCRD is true.
		ciliumEndpointSlices []cilium_v2a1.CiliumEndpointSlice
		// if true, simulates running CiliumEndpointSlice watcher instead of CEP.
		enableCES bool
		// endpoints in endpointManaged.
		managedEndpoints map[string]*endpoint.Endpoint
		// expectedDeletedSet contains CiliumEndpoints that are expected to be deleted
		// during test, in the form '<namespace>/<cilium_endpoint>'.
		expectedDeletedSet []string
	}{
		"CEPs with local pods without endpoints should be GCd": {
			ciliumEndpoints:    []types.CiliumEndpoint{cep("foo", "x", "<nil>"), cep("foo", "y", "<nil>")},
			managedEndpoints:   map[string]*endpoint.Endpoint{"y/foo": {}},
			expectedDeletedSet: []string{"x/foo"},
		},
		"CEPs with local pods with endpoints should not be GCd": {
			ciliumEndpoints:    []types.CiliumEndpoint{cep("foo", "x", "")},
			managedEndpoints:   map[string]*endpoint.Endpoint{"x/foo": {}},
			expectedDeletedSet: []string{},
		},
		"Non local CEPs should not be GCd": {
			ciliumEndpoints:    []types.CiliumEndpoint{cep("foo", "x", "1.2.3.4")},
			managedEndpoints:   map[string]*endpoint.Endpoint{},
			expectedDeletedSet: []string{},
		},
		"Nothing should be deleted if fields are missing": {
			ciliumEndpoints:    []types.CiliumEndpoint{cep("", "", "")},
			managedEndpoints:   map[string]*endpoint.Endpoint{},
			expectedDeletedSet: []string{},
		},
		"CES: local CEPs without endpoints should be GCd": {
			ciliumEndpointSlices: []cilium_v2a1.CiliumEndpointSlice{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "ciliumEndpointSlices-000",
					},
					Namespace: "x",
					Endpoints: []cilium_v2a1.CoreCiliumEndpoint{
						{
							Name: "foo",
							Networking: &ciliumv2.EndpointNetworking{
								Addressing: ciliumv2.AddressPairList{},
								NodeIP:     "<nil>",
							},
						},
					},
				},
			},
			ciliumEndpoints: []types.CiliumEndpoint{
				cep("bar", "x", "<nil>"),
				cep("foo", "x", "<nil>"),
				cep("notlocal", "x", "1.2.3.4"),
			},
			enableCES:          true,
			managedEndpoints:   map[string]*endpoint.Endpoint{"x/bar": {}},
			expectedDeletedSet: []string{"x/foo"},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			d := Daemon{
				k8sWatcher: &watchers.K8sWatcher{},
			}

			fakeClient := fake.NewSimpleClientset()
			fakeClient.PrependReactor("create", "ciliumendpoints", k8stesting.ReactionFunc(func(action k8stesting.Action) (bool, runtime.Object, error) {
				cep := action.(k8stesting.CreateAction).GetObject().(*ciliumv2.CiliumEndpoint)
				return false, cep, nil
			}))
			cepStore := cache.NewIndexer(cache.DeletionHandlingMetaNamespaceKeyFunc, cache.Indexers{
				"localNode": watchers.CreateCiliumEndpointLocalPodIndexFunc(), // empty nodeIP means this will index all nodes.
			})
			ciliumEndpointSlicesStore := cache.NewIndexer(cache.DeletionHandlingMetaNamespaceKeyFunc, cache.Indexers{
				"localNode": watchers.CreateCiliumEndpointSliceLocalPodIndexFunc(), // empty nodeIP means this will index all nodes.
			})

			for _, ces := range test.ciliumEndpointSlices {
				ciliumEndpointSlicesStore.Add(ces.DeepCopy())
			}
			for _, cep := range test.ciliumEndpoints {
				_, err := fakeClient.CiliumV2().CiliumEndpoints(cep.Namespace).Create(context.Background(), &ciliumv2.CiliumEndpoint{
					ObjectMeta: metav1.ObjectMeta{
						Name:      cep.Name,
						Namespace: cep.Namespace,
					},
				}, metav1.CreateOptions{})
				assert.NoError(err)
				cepStore.Add(cep.DeepCopy())
			}
			d.k8sWatcher.SetIndexer("ciliumendpoint", cepStore)
			d.k8sWatcher.SetIndexer("ciliumendpointslice", ciliumEndpointSlicesStore)
			l := &lock.Mutex{}
			var deletedSet []string
			fakeClient.PrependReactor("delete", "ciliumendpoints", k8stesting.ReactionFunc(func(action k8stesting.Action) (bool, runtime.Object, error) {
				l.Lock()
				defer l.Unlock()
				a := action.(k8stesting.DeleteAction)
				deletedSet = append(deletedSet, fmt.Sprintf("%s/%s", a.GetNamespace(), a.GetName()))
				return false, nil, nil
			}))

			epm := &fakeEPManager{test.managedEndpoints}

			err := d.cleanStaleCEPs(context.Background(), epm, k8s.K8sCiliumClient{
				Interface: fakeClient,
			}.CiliumV2(), test.enableCES)

			assert.NoError(err)
			assert.ElementsMatch(test.expectedDeletedSet, deletedSet)
		})
	}
}

type fakeEPManager struct {
	byPodName map[string]*endpoint.Endpoint
}

func (epm *fakeEPManager) LookupPodName(name string) *endpoint.Endpoint {
	ep, ok := epm.byPodName[name]
	if !ok {
		return nil
	}
	return ep
}

func cep(name, ns, nodeIP string) types.CiliumEndpoint {
	return types.CiliumEndpoint{
		ObjectMeta: slimmetav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Networking: &ciliumv2.EndpointNetworking{
			NodeIP: nodeIP,
		},
	}
}

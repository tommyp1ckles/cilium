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

	"github.com/cilium/cilium/pkg/endpoint"
	"github.com/cilium/cilium/pkg/k8s"
	ciliumv2 "github.com/cilium/cilium/pkg/k8s/apis/cilium.io/v2"
	cilium_v2a1 "github.com/cilium/cilium/pkg/k8s/apis/cilium.io/v2alpha1"
	"github.com/cilium/cilium/pkg/k8s/client/clientset/versioned/fake"
	slimcorev1 "github.com/cilium/cilium/pkg/k8s/slim/k8s/api/core/v1"
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
		pods      []slimcorev1.Pod
		// endpoints in endpointManaged.
		managedEndpoints map[string]*endpoint.Endpoint
		// expectedDeletedSet contains CiliumEndpoints that are expected to be deleted
		// during test, in the form '<namespace>/<cilium_endpoint>'.
		expectedDeletedSet []string
	}{
		"CEPs with local pods without endpoints should be GCd": {
			ciliumEndpoints:    []types.CiliumEndpoint{cep("foo", "x"), cep("foo", "y")},
			pods:               []slimcorev1.Pod{pod("foo", "x"), pod("foo", "y")},
			managedEndpoints:   map[string]*endpoint.Endpoint{"y/foo": {}},
			expectedDeletedSet: []string{"x/foo"},
		},
		"CEPs with local pods with endpoints should be GCd": {
			ciliumEndpoints:    []types.CiliumEndpoint{cep("foo", "x")},
			pods:               []slimcorev1.Pod{pod("foo", "x")},
			managedEndpoints:   map[string]*endpoint.Endpoint{"x/foo": {}},
			expectedDeletedSet: []string{},
		},
		"Nothing should be deleted if fields are missing": {
			ciliumEndpoints:    []types.CiliumEndpoint{cep("", "")},
			pods:               []slimcorev1.Pod{pod("", "")},
			managedEndpoints:   map[string]*endpoint.Endpoint{},
			expectedDeletedSet: []string{},
		},
		"Test without CEP enabled": {
			ciliumEndpointSlices: []cilium_v2a1.CiliumEndpointSlice{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "ciliumEndpointSlices-000",
						Namespace: "x",
					},
					Endpoints: []cilium_v2a1.CoreCiliumEndpoint{
						{
							Name: "foo",
							Networking: &ciliumv2.EndpointNetworking{
								Addressing: ciliumv2.AddressPairList{},
								NodeIP:     "10.0.1.2",
							},
						},
					},
				},
			},
			ciliumEndpoints:    []types.CiliumEndpoint{cep("bar", "x"), cep("foo", "x")},
			pods:               []slimcorev1.Pod{pod("bar", "x"), pod("foo", "x")},
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
			cepStore := &fakeStore{cache: map[interface{}]interface{}{}}
			ciliumEndpointSlicesStore := &fakeStore{cache: map[interface{}]interface{}{}}
			podStore := &fakeStore{cache: map[interface{}]interface{}{}}
			for _, ces := range test.ciliumEndpointSlices {
				ciliumEndpointSlicesStore.cache[fmt.Sprintf("%s/%s", ces.Namespace, ces.Name)] = ces.DeepCopy()
			}
			for _, cep := range test.ciliumEndpoints {
				_, err := fakeClient.CiliumV2().CiliumEndpoints(cep.Namespace).Create(context.Background(), &ciliumv2.CiliumEndpoint{
					ObjectMeta: metav1.ObjectMeta{
						Name:      cep.Name,
						Namespace: cep.Namespace,
					},
				}, metav1.CreateOptions{})
				assert.NoError(err)
				cepStore.cache[fmt.Sprintf("%s/%s", cep.Namespace, cep.Name)] = cep.DeepCopy()
			}
			for _, pod := range test.pods {
				podStore.cache[fmt.Sprintf("%s/%s", pod.Namespace, pod.Name)] = pod.DeepCopy()
			}
			d.k8sWatcher.SetStore("ciliumendpoint", cepStore)
			d.k8sWatcher.SetStore("pod", podStore)
			d.k8sWatcher.SetStore("ciliumendpointslice", ciliumEndpointSlicesStore)
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

type fakeStore struct {
	cache map[interface{}]interface{}
}

func (s *fakeStore) Add(obj interface{}) error    { return nil }
func (s *fakeStore) Update(obj interface{}) error { return nil }
func (s *fakeStore) Delete(obj interface{}) error { return nil }
func (s *fakeStore) List() []interface{} {
	arr := []interface{}{}
	for _, obj := range s.cache {
		arr = append(arr, obj)
	}
	return arr
}
func (s *fakeStore) ListKeys() []string { return nil }
func (s *fakeStore) Get(obj interface{}) (item interface{}, exists bool, err error) {
	return nil, false, nil
}
func (s *fakeStore) GetByKey(key string) (item interface{}, exists bool, err error) {
	obj, exists := s.cache[key]
	return obj, exists, nil
}
func (s *fakeStore) Replace([]interface{}, string) error { return nil }
func (s *fakeStore) Resync() error                       { return nil }

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

func pod(name, ns string) slimcorev1.Pod {
	return slimcorev1.Pod{
		ObjectMeta: slimmetav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
	}
}

func cep(name, ns string) types.CiliumEndpoint {
	return types.CiliumEndpoint{
		ObjectMeta: slimmetav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
	}
}

func ciliumEndpointSlices(name, ns string) cilium_v2a1.CiliumEndpointSlice {
	return cilium_v2a1.CiliumEndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
	}
}

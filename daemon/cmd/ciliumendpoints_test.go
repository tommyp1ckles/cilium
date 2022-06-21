package cmd

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/cilium/cilium/pkg/k8s"
	ciliumv2 "github.com/cilium/cilium/pkg/k8s/apis/cilium.io/v2"
	"github.com/cilium/cilium/pkg/k8s/client/clientset/versioned/fake"
	slimcorev1 "github.com/cilium/cilium/pkg/k8s/slim/k8s/api/core/v1"
	slimmetav1 "github.com/cilium/cilium/pkg/k8s/slim/k8s/apis/meta/v1"
	"github.com/cilium/cilium/pkg/k8s/types"
	"github.com/cilium/cilium/pkg/k8s/watchers"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stesting "k8s.io/client-go/testing"
)

type fakeStore struct {
	list []interface{}
}

func (s *fakeStore) Add(obj interface{}) error    { return nil }
func (s *fakeStore) Update(obj interface{}) error { return nil }
func (s *fakeStore) Delete(obj interface{}) error { return nil }
func (s *fakeStore) List() []interface{}          { return s.list }
func (s *fakeStore) ListKeys() []string           { return nil }
func (s *fakeStore) Get(obj interface{}) (item interface{}, exists bool, err error) {
	return nil, false, nil
}
func (s *fakeStore) GetByKey(key string) (item interface{}, exists bool, err error) {
	return nil, false, nil
}
func (s *fakeStore) Replace([]interface{}, string) error { return nil }
func (s *fakeStore) Resync() error                       { return nil }

func TestCleanStaleCeps(t *testing.T) {
	assert := assert.New(t)
	// In this test - for each test I want to create:
	// * A mock Pod store.
	// * A mock CEP store.
	// * Populate stores with slim Pods/CEPS.
	// * Check that desired CEPs get marked for deletion.
	//
	// Edge cases to consider:
	// * Empty names?
	// * Other reasons CEP might be stale?
	//
	// Other TODOS:
	// * Is there a way to check coverage?
	tests := map[string]struct {
		ceps               []types.CiliumEndpoint
		pods               []slimcorev1.Pod
		expectedDeletedSet []string
	}{

		"Ensure good CEPs are not GCd": {
			ceps: []types.CiliumEndpoint{
				{
					ObjectMeta: slimmetav1.ObjectMeta{
						Name:      "foo",
						Namespace: "x",
						OwnerReferences: []slimmetav1.OwnerReference{
							{Kind: "Pod", Name: "foo"},
						},
					},
					Networking: &ciliumv2.EndpointNetworking{
						Addressing: ciliumv2.AddressPairList{
							{IPV4: "10.20.0.1"},
						},
					},
				},
			},
			pods: []slimcorev1.Pod{
				slimcorev1.Pod{
					ObjectMeta: slimmetav1.ObjectMeta{
						Name:      "foo",
						Namespace: "x",
					},
					Spec: slimcorev1.PodSpec{HostNetwork: false},
					Status: slimcorev1.PodStatus{
						PodIPs: []slimcorev1.PodIP{
							{IP: "10.20.0.1"},
						}, // todo: why are there two fields.
						PodIP: "10.20.0.1",
					},
				},
			},
			expectedDeletedSet: []string{},
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
			cepStore := &fakeStore{}
			podStore := &fakeStore{}
			for _, cep := range test.ceps {
				_, err := fakeClient.CiliumV2().CiliumEndpoints(cep.Namespace).Create(context.Background(), &ciliumv2.CiliumEndpoint{
					ObjectMeta: metav1.ObjectMeta{
						Name:      cep.Name,
						Namespace: cep.Namespace,
					},
				}, metav1.CreateOptions{})
				assert.NoError(err)
				cepStore.list = append(cepStore.list, &cep)
			}
			for _, pod := range test.pods {
				podStore.list = append(podStore.list, &pod)
			}
			d.k8sWatcher.SetStore("ciliumendpoint", cepStore)
			d.k8sWatcher.SetStore("pod", podStore)
			l := &sync.Mutex{}
			var deletedSet []string
			fakeClient.PrependReactor("delete", "ciliumendpoints", k8stesting.ReactionFunc(func(action k8stesting.Action) (bool, runtime.Object, error) {
				l.Lock()
				defer l.Unlock()
				deletedSet = append(deletedSet, action.(k8stesting.DeleteAction).GetName())
				return false, nil, nil
			}))
			// _, err := fakeClient.CiliumV2().CiliumEndpoints("x").Create(context.Background(), &ciliumv2.CiliumEndpoint{
			// 	ObjectMeta: metav1.ObjectMeta{
			// 		Name: "foo",
			// 	},
			// }, v1.CreateOptions{})

			err := d.CleanStaleCEPs(context.Background(), k8s.K8sCiliumClient{
				Interface: fakeClient,
			}.CiliumV2())

			assert.NoError(err)
			fmt.Println("RESULT:", deletedSet)
			assert.ElementsMatch(test.expectedDeletedSet, deletedSet)
		})

	}
}

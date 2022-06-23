package cmd

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/cilium/cilium/pkg/endpoint"
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

func TestCleanStaleCeps(t *testing.T) {
	assert := assert.New(t)
	// Edge cases to consider:
	// * Empty names?
	// * Other reasons CEP might be stale?
	//
	// Other TODOS:
	// * Is there a way to check coverage?
	tests := map[string]struct {
		ceps               []types.CiliumEndpoint
		pods               []slimcorev1.Pod
		managedEndpoints   map[string]*endpoint.Endpoint
		expectedDeletedSet []string
	}{
		"CEPs with local pods without endpoints should be GCd": {
			ceps: []types.CiliumEndpoint{
				{
					ObjectMeta: slimmetav1.ObjectMeta{
						Name:      "foo",
						Namespace: "x",
					},
				},
				{
					ObjectMeta: slimmetav1.ObjectMeta{
						Name:      "foo",
						Namespace: "y",
					},
				},
			},
			pods: []slimcorev1.Pod{
				{
					ObjectMeta: slimmetav1.ObjectMeta{
						Name:      "foo",
						Namespace: "x",
					},
				},
				{
					ObjectMeta: slimmetav1.ObjectMeta{
						Name:      "foo",
						Namespace: "y",
					},
				},
			},
			managedEndpoints:   map[string]*endpoint.Endpoint{"y/foo": {}},
			expectedDeletedSet: []string{"x/foo"},
		},
		"CEPs with local pods with endpoints should be GCd": {
			ceps: []types.CiliumEndpoint{
				{
					ObjectMeta: slimmetav1.ObjectMeta{
						Name:      "foo",
						Namespace: "x",
					},
				},
			},
			pods: []slimcorev1.Pod{
				{
					ObjectMeta: slimmetav1.ObjectMeta{
						Name:      "foo",
						Namespace: "x",
					},
				},
			},
			managedEndpoints:   map[string]*endpoint.Endpoint{"x/foo": {}},
			expectedDeletedSet: []string{},
		},
		"empty test": {
			ceps: []types.CiliumEndpoint{
				{ObjectMeta: slimmetav1.ObjectMeta{}},
			},
			pods: []slimcorev1.Pod{
				{ObjectMeta: slimmetav1.ObjectMeta{}},
			},
			managedEndpoints:   map[string]*endpoint.Endpoint{},
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
			cepStore := &fakeStore{
				cache: map[interface{}]interface{}{},
			}
			podStore := &fakeStore{
				cache: map[interface{}]interface{}{},
			}
			for _, cep := range test.ceps {
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
			l := &sync.Mutex{}
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
			}.CiliumV2())

			assert.NoError(err)
			assert.ElementsMatch(test.expectedDeletedSet, deletedSet)
		})

	}
}

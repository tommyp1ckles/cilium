package cmd

import (
	"context"
	"testing"

	"github.com/cilium/cilium/pkg/k8s"
	"github.com/cilium/cilium/pkg/k8s/client/clientset/versioned/fake"
	"github.com/cilium/cilium/pkg/k8s/watchers"
	"k8s.io/client-go/rest"
	k8stesting "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/cache"
)

func TestCleanStaleCeps(t *testing.T) {
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
	d := Daemon{
		k8sWatcher: &watchers.K8sWatcher{},
	}
	d.k8sWatcher.SetStore("ciliumendpoint", cache.NewStore(cache.MetaNamespaceKeyFunc))
	d.k8sWatcher.SetStore("pod", cache.NewStore(cache.MetaNamespaceKeyFunc))
	fakeClient := fake.NewSimpleClientset()

	fakeClient.PrependProxyReactor("ciliumendpoint", func(action k8stesting.Action) (bool, rest.ResponseWrapper, error) {
		t.Log("Reacting!!!!!", action)
		return false, nil, nil
	})
	err := d.cleanStaleCEPs(context.Background(), k8s.K8sCiliumClient{
		Interface: fakeClient,
	}.CiliumV2())

	if err != nil {
		t.Fail()
		return
	}
}

// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of Cilium

//go:build !privileged_tests

package watchers

import (
	"context"
	"fmt"
	"time"

	. "gopkg.in/check.v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stesting "k8s.io/client-go/testing"

	ciliumv2 "github.com/cilium/cilium/pkg/k8s/apis/cilium.io/v2"
	"github.com/cilium/cilium/pkg/k8s/client/clientset/versioned/fake"
	slimmetav1 "github.com/cilium/cilium/pkg/k8s/slim/k8s/apis/meta/v1"
	"github.com/cilium/cilium/pkg/k8s/types"
	"github.com/cilium/cilium/pkg/option"
)

func cepModelToAPI(c *types.CiliumEndpoint) *ciliumv2.CiliumEndpoint {
	return &ciliumv2.CiliumEndpoint{ObjectMeta: metav1.ObjectMeta{Name: c.Name, Namespace: c.Namespace}}
}

func (s *K8sWatcherSuite) Test_ciliumEndpointManager(c *C) {
	fakeClient := fake.NewSimpleClientset()
	fakeClient.PrependReactor("create", "ciliumendpoints", k8stesting.ReactionFunc(func(action k8stesting.Action) (bool, runtime.Object, error) {
		cep := action.(k8stesting.CreateAction).GetObject().(*ciliumv2.CiliumEndpoint)
		return false, cep, nil
	}))
	deletedSet := []string{}
	fakeClient.PrependReactor("delete", "ciliumendpoints", k8stesting.ReactionFunc(func(action k8stesting.Action) (bool, runtime.Object, error) {
		a := action.(k8stesting.DeleteAction)
		deletedSet = append(deletedSet, fmt.Sprintf("%s/%s", a.GetNamespace(), a.GetName()))
		return false, nil, nil
	}))

	kw := &K8sWatcher{
		ciliumEndpointManager: newCiliumEndpointManager(fakeClient.CiliumV2()),
	}
	defer k8sCM.RemoveController("ciliumendpoint-local-gc")
	c.Assert(kw.ciliumEndpointManager.ciliumEndpointCleanupEnabled(), Equals, false)
	option.Config.LocalCiliumEndpointGCInterval = time.Millisecond
	ep0 := &types.CiliumEndpoint{
		ObjectMeta: slimmetav1.ObjectMeta{
			Name:      "foo",
			Namespace: "x",
		},
	}
	ep1 := &types.CiliumEndpoint{
		ObjectMeta: slimmetav1.ObjectMeta{
			Name:      "bar",
			Namespace: "y",
		},
	}
	kw.ciliumEndpointManager.markForDeletion(ep0)
	kw.ciliumEndpointManager.markForDeletion(ep1)
	fakeClient.CiliumV2().CiliumEndpoints(ep0.Namespace).Create(context.Background(), cepModelToAPI(ep0), metav1.CreateOptions{})
	fakeClient.CiliumV2().CiliumEndpoints(ep1.Namespace).Create(context.Background(), cepModelToAPI(ep1), metav1.CreateOptions{})
	c.Assert(len(kw.ciliumEndpointManager.markedForGC), Equals, 2)
	kw.EnableCiliumEndpointCleanup()
	c.Assert(kw.ciliumEndpointManager.ciliumEndpointCleanupEnabled(), Equals, true)
	time.Sleep(2 * time.Millisecond)
	c.Assert(len(kw.ciliumEndpointManager.markedForGC), Equals, 0)
	c.Assert(len(deletedSet), Equals, 2)
	c.Assert(deletedSet[0], Equals, "y/bar")
	c.Assert(deletedSet[1], Equals, "x/foo")
}

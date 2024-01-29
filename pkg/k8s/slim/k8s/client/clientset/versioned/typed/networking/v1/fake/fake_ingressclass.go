// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of Cilium

// Code generated by client-gen. DO NOT EDIT.

package fake

import (
	"context"

	v1 "github.com/cilium/cilium/pkg/k8s/slim/k8s/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	labels "k8s.io/apimachinery/pkg/labels"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	testing "k8s.io/client-go/testing"
)

// FakeIngressClasses implements IngressClassInterface
type FakeIngressClasses struct {
	Fake *FakeNetworkingV1
}

var ingressclassesResource = v1.SchemeGroupVersion.WithResource("ingressclasses")

var ingressclassesKind = v1.SchemeGroupVersion.WithKind("IngressClass")

// Get takes name of the ingressClass, and returns the corresponding ingressClass object, and an error if there is any.
func (c *FakeIngressClasses) Get(ctx context.Context, name string, options metav1.GetOptions) (result *v1.IngressClass, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootGetAction(ingressclassesResource, name), &v1.IngressClass{})
	if obj == nil {
		return nil, err
	}
	return obj.(*v1.IngressClass), err
}

// List takes label and field selectors, and returns the list of IngressClasses that match those selectors.
func (c *FakeIngressClasses) List(ctx context.Context, opts metav1.ListOptions) (result *v1.IngressClassList, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootListAction(ingressclassesResource, ingressclassesKind, opts), &v1.IngressClassList{})
	if obj == nil {
		return nil, err
	}

	label, _, _ := testing.ExtractFromListOptions(opts)
	if label == nil {
		label = labels.Everything()
	}
	list := &v1.IngressClassList{ListMeta: obj.(*v1.IngressClassList).ListMeta}
	for _, item := range obj.(*v1.IngressClassList).Items {
		if label.Matches(labels.Set(item.Labels)) {
			list.Items = append(list.Items, item)
		}
	}
	return list, err
}

// Watch returns a watch.Interface that watches the requested ingressClasses.
func (c *FakeIngressClasses) Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
	return c.Fake.
		InvokesWatch(testing.NewRootWatchAction(ingressclassesResource, opts))
}

// Create takes the representation of a ingressClass and creates it.  Returns the server's representation of the ingressClass, and an error, if there is any.
func (c *FakeIngressClasses) Create(ctx context.Context, ingressClass *v1.IngressClass, opts metav1.CreateOptions) (result *v1.IngressClass, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootCreateAction(ingressclassesResource, ingressClass), &v1.IngressClass{})
	if obj == nil {
		return nil, err
	}
	return obj.(*v1.IngressClass), err
}

// Update takes the representation of a ingressClass and updates it. Returns the server's representation of the ingressClass, and an error, if there is any.
func (c *FakeIngressClasses) Update(ctx context.Context, ingressClass *v1.IngressClass, opts metav1.UpdateOptions) (result *v1.IngressClass, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootUpdateAction(ingressclassesResource, ingressClass), &v1.IngressClass{})
	if obj == nil {
		return nil, err
	}
	return obj.(*v1.IngressClass), err
}

// Delete takes name of the ingressClass and deletes it. Returns an error if one occurs.
func (c *FakeIngressClasses) Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error {
	_, err := c.Fake.
		Invokes(testing.NewRootDeleteActionWithOptions(ingressclassesResource, name, opts), &v1.IngressClass{})
	return err
}

// DeleteCollection deletes a collection of objects.
func (c *FakeIngressClasses) DeleteCollection(ctx context.Context, opts metav1.DeleteOptions, listOpts metav1.ListOptions) error {
	action := testing.NewRootDeleteCollectionAction(ingressclassesResource, listOpts)

	_, err := c.Fake.Invokes(action, &v1.IngressClassList{})
	return err
}

// Patch applies the patch and returns the patched ingressClass.
func (c *FakeIngressClasses) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (result *v1.IngressClass, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootPatchSubresourceAction(ingressclassesResource, name, pt, data, subresources...), &v1.IngressClass{})
	if obj == nil {
		return nil, err
	}
	return obj.(*v1.IngressClass), err
}
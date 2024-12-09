/*
SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and dns-masquerading-operator contributors
SPDX-License-Identifier: Apache-2.0
*/

// Code generated by client-gen. DO NOT EDIT.

package fake

import (
	"context"

	v1alpha1 "github.com/sap/dns-masquerading-operator/api/v1alpha1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	labels "k8s.io/apimachinery/pkg/labels"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	testing "k8s.io/client-go/testing"
)

// FakeMasqueradingRules implements MasqueradingRuleInterface
type FakeMasqueradingRules struct {
	Fake *FakeDnsV1alpha1
	ns   string
}

var masqueradingrulesResource = v1alpha1.SchemeGroupVersion.WithResource("masqueradingrules")

var masqueradingrulesKind = v1alpha1.SchemeGroupVersion.WithKind("MasqueradingRule")

// Get takes name of the masqueradingRule, and returns the corresponding masqueradingRule object, and an error if there is any.
func (c *FakeMasqueradingRules) Get(ctx context.Context, name string, options v1.GetOptions) (result *v1alpha1.MasqueradingRule, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewGetAction(masqueradingrulesResource, c.ns, name), &v1alpha1.MasqueradingRule{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.MasqueradingRule), err
}

// List takes label and field selectors, and returns the list of MasqueradingRules that match those selectors.
func (c *FakeMasqueradingRules) List(ctx context.Context, opts v1.ListOptions) (result *v1alpha1.MasqueradingRuleList, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewListAction(masqueradingrulesResource, masqueradingrulesKind, c.ns, opts), &v1alpha1.MasqueradingRuleList{})

	if obj == nil {
		return nil, err
	}

	label, _, _ := testing.ExtractFromListOptions(opts)
	if label == nil {
		label = labels.Everything()
	}
	list := &v1alpha1.MasqueradingRuleList{ListMeta: obj.(*v1alpha1.MasqueradingRuleList).ListMeta}
	for _, item := range obj.(*v1alpha1.MasqueradingRuleList).Items {
		if label.Matches(labels.Set(item.Labels)) {
			list.Items = append(list.Items, item)
		}
	}
	return list, err
}

// Watch returns a watch.Interface that watches the requested masqueradingRules.
func (c *FakeMasqueradingRules) Watch(ctx context.Context, opts v1.ListOptions) (watch.Interface, error) {
	return c.Fake.
		InvokesWatch(testing.NewWatchAction(masqueradingrulesResource, c.ns, opts))

}

// Create takes the representation of a masqueradingRule and creates it.  Returns the server's representation of the masqueradingRule, and an error, if there is any.
func (c *FakeMasqueradingRules) Create(ctx context.Context, masqueradingRule *v1alpha1.MasqueradingRule, opts v1.CreateOptions) (result *v1alpha1.MasqueradingRule, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewCreateAction(masqueradingrulesResource, c.ns, masqueradingRule), &v1alpha1.MasqueradingRule{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.MasqueradingRule), err
}

// Update takes the representation of a masqueradingRule and updates it. Returns the server's representation of the masqueradingRule, and an error, if there is any.
func (c *FakeMasqueradingRules) Update(ctx context.Context, masqueradingRule *v1alpha1.MasqueradingRule, opts v1.UpdateOptions) (result *v1alpha1.MasqueradingRule, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateAction(masqueradingrulesResource, c.ns, masqueradingRule), &v1alpha1.MasqueradingRule{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.MasqueradingRule), err
}

// UpdateStatus was generated because the type contains a Status member.
// Add a +genclient:noStatus comment above the type to avoid generating UpdateStatus().
func (c *FakeMasqueradingRules) UpdateStatus(ctx context.Context, masqueradingRule *v1alpha1.MasqueradingRule, opts v1.UpdateOptions) (*v1alpha1.MasqueradingRule, error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateSubresourceAction(masqueradingrulesResource, "status", c.ns, masqueradingRule), &v1alpha1.MasqueradingRule{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.MasqueradingRule), err
}

// Delete takes name of the masqueradingRule and deletes it. Returns an error if one occurs.
func (c *FakeMasqueradingRules) Delete(ctx context.Context, name string, opts v1.DeleteOptions) error {
	_, err := c.Fake.
		Invokes(testing.NewDeleteActionWithOptions(masqueradingrulesResource, c.ns, name, opts), &v1alpha1.MasqueradingRule{})

	return err
}

// DeleteCollection deletes a collection of objects.
func (c *FakeMasqueradingRules) DeleteCollection(ctx context.Context, opts v1.DeleteOptions, listOpts v1.ListOptions) error {
	action := testing.NewDeleteCollectionAction(masqueradingrulesResource, c.ns, listOpts)

	_, err := c.Fake.Invokes(action, &v1alpha1.MasqueradingRuleList{})
	return err
}

// Patch applies the patch and returns the patched masqueradingRule.
func (c *FakeMasqueradingRules) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts v1.PatchOptions, subresources ...string) (result *v1alpha1.MasqueradingRule, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewPatchSubresourceAction(masqueradingrulesResource, c.ns, name, pt, data, subresources...), &v1alpha1.MasqueradingRule{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.MasqueradingRule), err
}

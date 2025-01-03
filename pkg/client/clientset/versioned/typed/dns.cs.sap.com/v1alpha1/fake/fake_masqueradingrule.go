/*
SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and dns-masquerading-operator contributors
SPDX-License-Identifier: Apache-2.0
*/

// Code generated by client-gen. DO NOT EDIT.

package fake

import (
	v1alpha1 "github.com/sap/dns-masquerading-operator/api/v1alpha1"
	dnscssapcomv1alpha1 "github.com/sap/dns-masquerading-operator/pkg/client/clientset/versioned/typed/dns.cs.sap.com/v1alpha1"
	gentype "k8s.io/client-go/gentype"
)

// fakeMasqueradingrules implements MasqueradingRuleInterface
type fakeMasqueradingrules struct {
	*gentype.FakeClientWithList[*v1alpha1.MasqueradingRule, *v1alpha1.MasqueradingRuleList]
	Fake *FakeDnsV1alpha1
}

func newFakeMasqueradingrules(fake *FakeDnsV1alpha1, namespace string) dnscssapcomv1alpha1.MasqueradingRuleInterface {
	return &fakeMasqueradingrules{
		gentype.NewFakeClientWithList[*v1alpha1.MasqueradingRule, *v1alpha1.MasqueradingRuleList](
			fake.Fake,
			namespace,
			v1alpha1.SchemeGroupVersion.WithResource("masqueradingrules"),
			v1alpha1.SchemeGroupVersion.WithKind("MasqueradingRule"),
			func() *v1alpha1.MasqueradingRule { return &v1alpha1.MasqueradingRule{} },
			func() *v1alpha1.MasqueradingRuleList { return &v1alpha1.MasqueradingRuleList{} },
			func(dst, src *v1alpha1.MasqueradingRuleList) { dst.ListMeta = src.ListMeta },
			func(list *v1alpha1.MasqueradingRuleList) []*v1alpha1.MasqueradingRule {
				return gentype.ToPointerSlice(list.Items)
			},
			func(list *v1alpha1.MasqueradingRuleList, items []*v1alpha1.MasqueradingRule) {
				list.Items = gentype.FromPointerSlice(items)
			},
		),
		fake,
	}
}

/*
SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and dns-masquerading-operator contributors
SPDX-License-Identifier: Apache-2.0
*/

// Code generated by client-gen. DO NOT EDIT.

package fake

import (
	v1alpha1 "github.com/sap/dns-masquerading-operator/pkg/client/clientset/versioned/typed/dns.cs.sap.com/v1alpha1"
	rest "k8s.io/client-go/rest"
	testing "k8s.io/client-go/testing"
)

type FakeDnsV1alpha1 struct {
	*testing.Fake
}

func (c *FakeDnsV1alpha1) Masqueradingrules(namespace string) v1alpha1.MasqueradingRuleInterface {
	return newFakeMasqueradingrules(c, namespace)
}

// RESTClient returns a RESTClient that is used to communicate
// with API server by this client implementation.
func (c *FakeDnsV1alpha1) RESTClient() rest.Interface {
	var ret *rest.RESTClient
	return ret
}

/*
SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and dns-masquerading-operator contributors
SPDX-License-Identifier: Apache-2.0
*/

// Code generated by lister-gen. DO NOT EDIT.

package v1alpha1

import (
	v1alpha1 "github.com/sap/dns-masquerading-operator/api/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/cache"
)

// MasqueradingRuleLister helps list MasqueradingRules.
// All objects returned here must be treated as read-only.
type MasqueradingRuleLister interface {
	// List lists all MasqueradingRules in the indexer.
	// Objects returned here must be treated as read-only.
	List(selector labels.Selector) (ret []*v1alpha1.MasqueradingRule, err error)
	// MasqueradingRules returns an object that can list and get MasqueradingRules.
	MasqueradingRules(namespace string) MasqueradingRuleNamespaceLister
	MasqueradingRuleListerExpansion
}

// masqueradingRuleLister implements the MasqueradingRuleLister interface.
type masqueradingRuleLister struct {
	indexer cache.Indexer
}

// NewMasqueradingRuleLister returns a new MasqueradingRuleLister.
func NewMasqueradingRuleLister(indexer cache.Indexer) MasqueradingRuleLister {
	return &masqueradingRuleLister{indexer: indexer}
}

// List lists all MasqueradingRules in the indexer.
func (s *masqueradingRuleLister) List(selector labels.Selector) (ret []*v1alpha1.MasqueradingRule, err error) {
	err = cache.ListAll(s.indexer, selector, func(m interface{}) {
		ret = append(ret, m.(*v1alpha1.MasqueradingRule))
	})
	return ret, err
}

// MasqueradingRules returns an object that can list and get MasqueradingRules.
func (s *masqueradingRuleLister) MasqueradingRules(namespace string) MasqueradingRuleNamespaceLister {
	return masqueradingRuleNamespaceLister{indexer: s.indexer, namespace: namespace}
}

// MasqueradingRuleNamespaceLister helps list and get MasqueradingRules.
// All objects returned here must be treated as read-only.
type MasqueradingRuleNamespaceLister interface {
	// List lists all MasqueradingRules in the indexer for a given namespace.
	// Objects returned here must be treated as read-only.
	List(selector labels.Selector) (ret []*v1alpha1.MasqueradingRule, err error)
	// Get retrieves the MasqueradingRule from the indexer for a given namespace and name.
	// Objects returned here must be treated as read-only.
	Get(name string) (*v1alpha1.MasqueradingRule, error)
	MasqueradingRuleNamespaceListerExpansion
}

// masqueradingRuleNamespaceLister implements the MasqueradingRuleNamespaceLister
// interface.
type masqueradingRuleNamespaceLister struct {
	indexer   cache.Indexer
	namespace string
}

// List lists all MasqueradingRules in the indexer for a given namespace.
func (s masqueradingRuleNamespaceLister) List(selector labels.Selector) (ret []*v1alpha1.MasqueradingRule, err error) {
	err = cache.ListAllByNamespace(s.indexer, s.namespace, selector, func(m interface{}) {
		ret = append(ret, m.(*v1alpha1.MasqueradingRule))
	})
	return ret, err
}

// Get retrieves the MasqueradingRule from the indexer for a given namespace and name.
func (s masqueradingRuleNamespaceLister) Get(name string) (*v1alpha1.MasqueradingRule, error) {
	obj, exists, err := s.indexer.GetByKey(s.namespace + "/" + name)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.NewNotFound(v1alpha1.Resource("masqueradingrule"), name)
	}
	return obj.(*v1alpha1.MasqueradingRule), nil
}

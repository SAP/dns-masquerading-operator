/*
SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and dns-masquerading-operator contributors
SPDX-License-Identifier: Apache-2.0
*/

package controllers

import (
	"context"

	"github.com/pkg/errors"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	dnsv1alpha1 "github.com/sap/dns-masquerading-operator/api/v1alpha1"
)

const (
	labelControllerGroup   = "dns.cs.sap.com/controller-group"
	labelControllerVersion = "dns.cs.sap.com/controller-version"
	labelControllerKind    = "dns.cs.sap.com/controller-kind"
	labelControllerName    = "dns.cs.sap.com/controller-name"
	labelControllerUid     = "dns.cs.sap.com/controller-uid"
)

const (
	annotationMasqueradeTo       = "dns.cs.sap.com/masquerade-to"
	annotationMasqueradeToLegacy = "masquerading-operator.dns.sap.com/masquerade-to"
)

const (
	finalizer = "dns.cs.sap.com/masquerading-operator"
)

// manage dependent masquerading rules of an arbitrary resource
func manageDependents(ctx context.Context, c client.Client, obj client.Object, hosts []string) error {
	log := ctrl.LoggerFrom(ctx)

	masqueradingRuleList := &dnsv1alpha1.MasqueradingRuleList{}
	if err := c.List(ctx, masqueradingRuleList, client.InNamespace(obj.GetNamespace()), client.MatchingLabels{labelControllerUid: string(obj.GetUID())}); err != nil {
		return errors.Wrap(err, "failed to list dependent masquerading rules")
	}
	numDependents := len(masqueradingRuleList.Items)

	if obj.GetDeletionTimestamp().IsZero() {
		var masqueradingRules []*dnsv1alpha1.MasqueradingRule
		to := obj.GetAnnotations()[annotationMasqueradeTo]
		// TODO: the following can be removed in the future
		if to == "" {
			to = obj.GetAnnotations()[annotationMasqueradeToLegacy]
		}

		if to != "" {
			if controllerutil.AddFinalizer(obj, finalizer) {
				if err := c.Update(ctx, obj); err != nil {
					return errors.Wrap(err, "failed to add finalizer")
				}
			}
			for _, from := range hosts {
				found := false
				for _, masqueradingRule := range masqueradingRuleList.Items {
					if masqueradingRule.Spec.From == from && masqueradingRule.Spec.To == to {
						masqueradingRules = append(masqueradingRules, &masqueradingRule)
						found = true
						break
					}
				}
				if !found {
					masqueradingRule := buildMasqueradingRule(obj.GetNamespace(), obj.GetName(), obj.GetObjectKind().GroupVersionKind(), obj.GetName(), obj.GetUID(), from, to)
					if err := c.Create(ctx, masqueradingRule); err != nil {
						return errors.Wrapf(err, "failed to create masquerading rule for host %s", from)
					}
					numDependents++
					log.Info("created masquerading rule %s/%s", masqueradingRule.Namespace, masqueradingRule.Name)
					masqueradingRules = append(masqueradingRules, masqueradingRule)
				}
			}
		}

		for _, masqueradingRule := range masqueradingRuleList.Items {
			found := false
			for _, mr := range masqueradingRules {
				if mr.UID == masqueradingRule.UID {
					found = true
					break
				}
			}
			if !found {
				if masqueradingRule.DeletionTimestamp.IsZero() {
					if err := c.Delete(ctx, &masqueradingRule, client.PropagationPolicy(metav1.DeletePropagationForeground)); err != nil {
						return errors.Wrapf(err, "failed to delete masquerading rule %s/%s", masqueradingRule.Namespace, masqueradingRule.Name)
					}
				}
				numDependents--
			}
		}
	} else {
		for _, masqueradingRule := range masqueradingRuleList.Items {
			if masqueradingRule.DeletionTimestamp.IsZero() {
				if err := c.Delete(ctx, &masqueradingRule, client.PropagationPolicy(metav1.DeletePropagationForeground)); err != nil {
					return errors.Wrapf(err, "failed to delete masquerading rule %s/%s", masqueradingRule.Namespace, masqueradingRule.Name)
				}
			}
			numDependents--
		}
	}

	if numDependents == 0 {
		if controllerutil.RemoveFinalizer(obj, finalizer) {
			if err := c.Update(ctx, obj); err != nil {
				return errors.Wrap(err, "failed to remove finalizer")
			}
		}
	}

	return nil
}

// build masquerading rule resource with owner
func buildMasqueradingRule(namespace string, namePrefix string, ownerGVK schema.GroupVersionKind, ownerName string, ownerUid types.UID, from string, to string) *dnsv1alpha1.MasqueradingRule {
	return &dnsv1alpha1.MasqueradingRule{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:    namespace,
			GenerateName: namePrefix + "-",
			Labels: map[string]string{
				labelControllerGroup:   ownerGVK.Group,
				labelControllerVersion: ownerGVK.Version,
				labelControllerKind:    ownerGVK.Kind,
				labelControllerName:    ownerName,
				labelControllerUid:     string(ownerUid),
			},
		},
		Spec: dnsv1alpha1.MasqueradingRuleSpec{
			From: from,
			To:   to,
		},
	}
}

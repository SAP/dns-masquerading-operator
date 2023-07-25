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

	dnsv1alpha1 "github.com/sap/dns-masquerading-operator/api/v1alpha1"
)

const (
	labelControllerGroup = "dns.cs.sap.com/controller-group"
	labelControllerKind  = "dns.cs.sap.com/controller-kind"
	labelControllerName  = "dns.cs.sap.com/controller-name"
	labelControllerUid   = "dns.cs.sap.com/controller-uid"
)

const (
	annotationMasqueradeTo       = "dns.cs.sap.com/masquerade-to"
	annotationMasqueradeToLegacy = "masquerading-operator.dns.sap.com/masquerade-to"
)

// manage dependent masquerading rules of an arbitrary resource
func manageDependents(ctx context.Context, c client.Client, obj client.Object, hosts []string) error {
	log := ctrl.LoggerFrom(ctx)

	if !obj.GetDeletionTimestamp().IsZero() {
		// No action needed, because the dependent masquerading rules have this object set as owner and will be purged by garbage collection anyway
		return nil
	}

	masqueradingRuleList := &dnsv1alpha1.MasqueradingRuleList{}
	if err := c.List(ctx, masqueradingRuleList, &client.ListOptions{Namespace: obj.GetNamespace()}, client.MatchingLabels{labelControllerUid: string(obj.GetUID())}); err != nil {
		return errors.Wrap(err, "failed to list dependent masquerading rules")
	}

	var masqueradingRules []*dnsv1alpha1.MasqueradingRule
	to := obj.GetAnnotations()[annotationMasqueradeTo]
	// TODO: this can be removed in the future
	if to == "" {
		to = obj.GetAnnotations()[annotationMasqueradeToLegacy]
	}

	if to != "" {
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
				if err := c.Create(ctx, masqueradingRule, &client.CreateOptions{}); err != nil {
					return errors.Wrapf(err, "failed to create masquerading rule for host %s", from)
				}
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
			if err := c.Delete(ctx, &masqueradingRule, &client.DeleteOptions{PropagationPolicy: &[]metav1.DeletionPropagation{metav1.DeletePropagationForeground}[0]}); err != nil {
				return errors.Wrapf(err, "failed to delete masquerading rule %s/%s", masqueradingRule.Namespace, masqueradingRule.Name)
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
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion:         ownerGVK.GroupVersion().String(),
					Kind:               ownerGVK.Kind,
					Name:               ownerName,
					UID:                ownerUid,
					BlockOwnerDeletion: &[]bool{true}[0],
				},
			},
			Labels: map[string]string{
				labelControllerGroup: ownerGVK.Group,
				labelControllerKind:  ownerGVK.Kind,
				labelControllerName:  ownerName,
				labelControllerUid:   string(ownerUid),
			},
		},
		Spec: dnsv1alpha1.MasqueradingRuleSpec{
			From: from,
			To:   to,
		},
	}
}

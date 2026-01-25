/*
SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and dns-masquerading-operator contributors
SPDX-License-Identifier: Apache-2.0
*/

package controllers

import (
	"context"

	"github.com/pkg/errors"
	"github.com/sap/go-generics/maps"

	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// IngressReconciler reconciles an Ingress object
type IngressReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// Reconcile an ingress resource
func (r *IngressReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, err error) {
	log := ctrl.LoggerFrom(ctx)
	log.V(1).Info("running reconcile")

	// Retrieve target ingress
	ingress := &networkingv1.Ingress{}
	if err := r.Get(ctx, req.NamespacedName, ingress); err != nil {
		if err := client.IgnoreNotFound(err); err != nil {
			return ctrl.Result{}, errors.Wrap(err, "unexpected get error")
		}
		log.Info("not found; ignoring")
		return ctrl.Result{}, nil
	}

	if err := manageDependents(ctx, r.Client, ingress, getHostsFromIngress(ingress)); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// getHostsFromIngress extracts hosts of an ingress resource
func getHostsFromIngress(ingress *networkingv1.Ingress) []string {
	// TODO: consider external-dns.alpha.kubernetes.io/hostname and dns.gardener.cloud/dnsnames annotation as well ?
	hosts := make(map[string]struct{})
	for _, rule := range ingress.Spec.Rules {
		if rule.Host != "" {
			hosts[rule.Host] = struct{}{}
		}
	}
	return maps.Keys(hosts)
}

// SetupWithManager sets up the controller with the Manager.
func (r *IngressReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1.Ingress{}).
		Complete(r)
}

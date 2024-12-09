/*
SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and dns-masquerading-operator contributors
SPDX-License-Identifier: Apache-2.0
*/

package controllers

import (
	"context"

	"github.com/pkg/errors"
	"github.com/sap/go-generics/maps"

	istionetworkingv1beta1 "istio.io/client-go/pkg/apis/networking/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// GatewayReconciler reconciles a Gateway object
type GatewayReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=networking.istio.io,resources=gateways,verbs=get;list;watch;update

// Reconcile a gateway resource
func (r *GatewayReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, err error) {
	log := ctrl.LoggerFrom(ctx)
	log.V(1).Info("running reconcile")

	// Retrieve target gateway
	gateway := &istionetworkingv1beta1.Gateway{}
	if err := r.Get(ctx, req.NamespacedName, gateway); err != nil {
		if err := client.IgnoreNotFound(err); err != nil {
			return ctrl.Result{}, errors.Wrap(err, "unexpected get error")
		}
		log.Info("not found; ignoring")
		return ctrl.Result{}, nil
	}

	if err := manageDependents(ctx, r.Client, gateway, getHostsFromGateway(gateway)); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// getHostsFromGateway extracts hosts of a gateway resource
func getHostsFromGateway(gateway *istionetworkingv1beta1.Gateway) []string {
	// TODO: consider external-dns.alpha.kubernetes.io/hostname annotation as well ?
	hosts := make(map[string]struct{})
	for _, server := range gateway.Spec.Servers {
		for _, host := range server.Hosts {
			hosts[host] = struct{}{}
		}
	}
	return maps.Keys(hosts)
}

// SetupWithManager sets up the controller with the Manager.
func (r *GatewayReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&istionetworkingv1beta1.Gateway{}).
		Complete(r)
}

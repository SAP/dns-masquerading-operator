/*
SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and dns-masquerading-operator contributors
SPDX-License-Identifier: Apache-2.0
*/

package controllers

import (
	"context"
	"fmt"
	"strings"

	"github.com/pkg/errors"
	"github.com/sap/go-generics/maps"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// ServiceReconciler reconciles a Service object
type ServiceReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;update

// Reconcile a service resource
func (r *ServiceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, err error) {
	log := ctrl.LoggerFrom(ctx)
	log.V(1).Info("running reconcile")

	// Retrieve target service
	service := &corev1.Service{}
	if err := r.Get(ctx, req.NamespacedName, service); err != nil {
		if err := client.IgnoreNotFound(err); err != nil {
			return ctrl.Result{}, errors.Wrap(err, "unexpected get error")
		}
		log.Info("not found; ignoring")
		return ctrl.Result{}, nil
	}

	if service.Annotations[annotationMasqueradeFrom] != "" && service.Annotations[annotationMasqueradeTo] == "" && service.Annotations[annotationMasqueradeToLegacy] == "" {
		// TODO: make cluster domain (cluster.local) configurable, or auto-detect it somehow
		service.Annotations[annotationMasqueradeTo] = fmt.Sprintf("%s.%s.svc.cluster.local", service.Name, service.Namespace)
	}

	if err := manageDependents(ctx, r.Client, service, getHostsFromService(service)); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// getHostsFromService extracts hosts of a service resource
func getHostsFromService(service *corev1.Service) []string {
	hosts := make(map[string]struct{})
	if v, ok := service.Annotations[annotationMasqueradeFrom]; ok {
		for _, host := range strings.Split(v, ",") {
			hosts[host] = struct{}{}
		}
	}
	// services do not have a canonical way to specify external hostnames, so we just can apply some heuristic guess here ...
	if v, ok := service.Annotations["external-dns.alpha.kubernetes.io/hostname"]; ok {
		for _, host := range strings.Split(v, ",") {
			hosts[host] = struct{}{}
		}
	}
	if v, ok := service.Annotations["dns.gardener.cloud/dnsnames"]; ok {
		for _, host := range strings.Split(v, ",") {
			hosts[host] = struct{}{}
		}
	}
	return maps.Keys(hosts)
}

// custom predicate to filter for service type
func serviceTypePredicate(serviceType corev1.ServiceType) predicate.Predicate {
	f := func(obj client.Object, serviceType corev1.ServiceType) bool {
		if service, ok := obj.(*corev1.Service); ok {
			return service.Spec.Type == serviceType
		}
		return true
	}
	return predicate.Funcs{
		CreateFunc:  func(e event.CreateEvent) bool { return f(e.Object, serviceType) },
		UpdateFunc:  func(e event.UpdateEvent) bool { return f(e.ObjectNew, serviceType) },
		DeleteFunc:  func(e event.DeleteEvent) bool { return f(e.Object, serviceType) },
		GenericFunc: func(e event.GenericEvent) bool { return f(e.Object, serviceType) },
	}
}

// custom predicate to filter for annotation being set
func annotationPredicate(key string) predicate.Predicate {
	f := func(obj client.Object, key string) bool {
		_, ok := obj.GetAnnotations()[key]
		return ok
	}
	return predicate.Funcs{
		CreateFunc:  func(e event.CreateEvent) bool { return f(e.Object, key) },
		UpdateFunc:  func(e event.UpdateEvent) bool { return f(e.ObjectNew, key) },
		DeleteFunc:  func(e event.DeleteEvent) bool { return f(e.Object, key) },
		GenericFunc: func(e event.GenericEvent) bool { return f(e.Object, key) },
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *ServiceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Service{}, builder.WithPredicates(predicate.Or(serviceTypePredicate(corev1.ServiceTypeLoadBalancer), annotationPredicate(annotationMasqueradeFrom)))).
		Complete(r)
}

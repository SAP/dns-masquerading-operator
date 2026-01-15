/*
SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and dns-masquerading-operator contributors
SPDX-License-Identifier: Apache-2.0
*/

package controllers

import (
	"context"
	"fmt"
	"regexp"
	"time"

	"github.com/pkg/errors"
	"github.com/sap/go-generics/slices"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	dnsv1alpha1 "github.com/sap/dns-masquerading-operator/api/v1alpha1"
	"github.com/sap/dns-masquerading-operator/internal/coredns"
)

const (
	annotationLastUpdatedAt = "dns.cs.sap.com/last-updated-at"
)

// MasqueradingRuleReconciler reconciles a MasqueradingRule object
type MasqueradingRuleReconciler struct {
	client.Client
	Scheme                      *runtime.Scheme
	Recorder                    record.EventRecorder
	CorednsConfigMapNamespace   string
	CorednsConfigMapName        string
	CorednsConfigMapKey         string
	CorednsConfigMapUpdateDelay time.Duration
	Resolver                    coredns.Resolver
}

//+kubebuilder:rbac:groups=dns.cs.sap.com,resources=masqueradingrules,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=dns.cs.sap.com,resources=masqueradingrules/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=dns.cs.sap.com,resources=masqueradingrules/finalizers,verbs=update
//+kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update
//+kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=endpoints,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=pods/portforward,verbs=create

// TODO: add status info about the duration of the reconciliation
// (in particular how long it took to become effective in DNS)

// TODO: add metrics (such as the duration of reconciliation, but also other figures)

// Reconcile a MasqueradingRule resource
func (r *MasqueradingRuleReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, err error) {
	log := ctrl.LoggerFrom(ctx)
	log.V(1).Info("running reconcile")

	// Retrieve target masquerading rule
	masqueradingRule := &dnsv1alpha1.MasqueradingRule{}
	if err := r.Get(ctx, req.NamespacedName, masqueradingRule); err != nil {
		if err := client.IgnoreNotFound(err); err != nil {
			return ctrl.Result{}, errors.Wrap(err, "unexpected get error")
		}
		log.Info("not found; ignoring")
		return ctrl.Result{}, nil
	}
	previousMasqueradingRuleStatus := masqueradingRule.Status.DeepCopy()

	// Call the defaulting webhook logic also here (because defaulting through the webhook might be incomplete in case of generateName usage)
	masqueradingRule.Default()

	// Acknowledge observed generation
	masqueradingRule.Status.ObservedGeneration = masqueradingRule.Generation

	// Always attempt to update the status
	skipStatusUpdate := false
	defer func() {
		if skipStatusUpdate {
			return
		}
		if err != nil {
			masqueradingRule.SetState(dnsv1alpha1.MasqueradingRuleStateError, err.Error())
			r.Recorder.Event(masqueradingRule, corev1.EventTypeWarning, "ReconciliationFailed", err.Error())
		}
		if updateErr := r.Status().Update(ctx, masqueradingRule, client.FieldOwner(fieldOwner)); updateErr != nil {
			err = utilerrors.NewAggregate([]error{err, updateErr})
			result = ctrl.Result{}
		}
	}()

	// Create events on owners (if any)
	defer func() {
		if _, ok := masqueradingRule.Labels[labelControllerUid]; !ok {
			return
		}
		gvk := schema.GroupVersionKind{
			Group:   masqueradingRule.Labels[labelControllerGroup],
			Version: masqueradingRule.Labels[labelControllerVersion],
			Kind:    masqueradingRule.Labels[labelControllerKind],
		}
		name := masqueradingRule.Labels[labelControllerName]
		if masqueradingRule.Status.State == dnsv1alpha1.MasqueradingRuleStateReady && previousMasqueradingRuleStatus.State != dnsv1alpha1.MasqueradingRuleStateReady {
			if err := r.createEventForObject(
				ctx,
				gvk,
				masqueradingRule.Namespace,
				name,
				corev1.EventTypeNormal,
				"ReconciliationSucceeded",
				"Masquerading rule %s/%s (host %s) successfully reconciled",
				masqueradingRule.Namespace,
				masqueradingRule.Name,
				masqueradingRule.Spec.From,
			); err != nil {
				log.Error(err, "failed to record event for owner", "group", gvk.Group, "version", gvk.Version, "kind", gvk.Kind, "name", name)
			}
		} else if masqueradingRule.Status.State == dnsv1alpha1.MasqueradingRuleStateError && previousMasqueradingRuleStatus.State != dnsv1alpha1.MasqueradingRuleStateError {
			if err := r.createEventForObject(
				ctx,
				gvk,
				masqueradingRule.Namespace,
				name,
				corev1.EventTypeWarning,
				"ReconciliationFailed",
				"Masquerading rule %s/%s (host %s) reconciliation failed",
				masqueradingRule.Namespace,
				masqueradingRule.Name,
				masqueradingRule.Spec.From,
			); err != nil {
				log.Error(err, "failed to record event for owner", "group", gvk.Group, "version", gvk.Version, "kind", gvk.Kind, "name", name)
			}
		}
	}()

	// Set a first status (and requeue, because the status update itself will not trigger another reconciliation because of the event filter set)
	if masqueradingRule.Status.State == "" {
		masqueradingRule.SetState(dnsv1alpha1.MasqueradingRuleStateNew, "First seen")
		return ctrl.Result{Requeue: true}, nil
	}

	// Retrieve coredns custom config map
	configMap := &corev1.ConfigMap{}
	if err := r.Get(ctx, types.NamespacedName{Namespace: r.CorednsConfigMapNamespace, Name: r.CorednsConfigMapName}, configMap); err != nil {
		if err := client.IgnoreNotFound(err); err != nil {
			return ctrl.Result{}, errors.Wrap(err, "unexpected get error")
		}
		log.Info("configmap not found", "namespace", r.CorednsConfigMapNamespace, "name", r.CorednsConfigMapName)
		configMap = nil
	}

	// Set owner identifier for later usage
	owner := fmt.Sprintf("%s (%s/%s)", masqueradingRule.UID, masqueradingRule.Namespace, masqueradingRule.Name)

	// Do the reconciliation
	// TODO: there is a race condition when worker counts > 1 are configured, while maintaining the coredns custom config map;
	// this in in principle harmless, but it will pollute the logs with 409 error messages;
	// to overcome this, we would need to introduce a mutex to synchronize the reconciliation (at least the relevant parts of the logic)
	// across the workers; but this is maybe not a good idea as well (so we leave it for now as it is) ...
	if masqueradingRule.DeletionTimestamp.IsZero() {
		// Create/update case
		if !slices.Contains(masqueradingRule.Finalizers, finalizer) {
			controllerutil.AddFinalizer(masqueradingRule, finalizer)
			if err := r.Update(ctx, masqueradingRule, client.FieldOwner(fieldOwner)); err != nil {
				return ctrl.Result{}, errors.Wrap(err, "error setting finalizer")
			}
		}

		rule, err := coredns.NewRewriteRule(owner, masqueradingRule.Spec.From, masqueradingRule.Spec.To)
		if err != nil {
			return ctrl.Result{}, errors.Wrap(err, "error adding rewrite rule")
		}

		if configMap == nil {
			ruleset := coredns.NewRewriteRuleSet()
			if _, err := ruleset.AddRule(rule); err != nil {
				return ctrl.Result{}, errors.Wrap(err, "error adding rewrite rule")
			}
			masqueradingRule.SetState(dnsv1alpha1.MasqueradingRuleStateProcessing, "waiting for masquerading rule to be reconciled")
			configMap = &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: r.CorednsConfigMapNamespace,
					Name:      r.CorednsConfigMapName,
				},
				Data: map[string]string{
					r.CorednsConfigMapKey: ruleset.String(),
				},
			}
			if err := r.Create(ctx, configMap, client.FieldOwner(fieldOwner)); err != nil {
				return ctrl.Result{}, errors.Wrapf(err, "error creating config map %s/%s", configMap.Namespace, configMap.Name)
			}
			log.V(1).Info("configmap successfully created", "namespace", r.CorednsConfigMapNamespace, "name", r.CorednsConfigMapName)
			return ctrl.Result{RequeueAfter: 1 * time.Second}, nil
		} else {
			ruleset, err := coredns.ParseRewriteRuleSet(configMap.Data[r.CorednsConfigMapKey])
			if err != nil {
				return ctrl.Result{}, errors.Wrapf(err, "error loading rewrite rules from config map %s/%s (key: %s)", configMap.Namespace, configMap.Name, r.CorednsConfigMapKey)
			}
			changed, err := ruleset.AddRule(rule)
			if err != nil {
				return ctrl.Result{}, errors.Wrap(err, "error adding rewrite rule")
			}
			if changed {
				masqueradingRule.SetState(dnsv1alpha1.MasqueradingRuleStateProcessing, "waiting for masquerading rule to be reconciled")
				if configMap.Data == nil {
					configMap.Data = make(map[string]string)
				}
				// TODO: the following is needed (for test execution) until we have https://github.com/coredns/coredns/issues/6243 or a similar fix;
				// note: delaying the update is probably not required in 'real' deployments, since high-frequency configmap updates are
				// anyway buffered there by kubelet's configmap/secret distribution logic.
				now := time.Now()
				if val, ok := configMap.Annotations[annotationLastUpdatedAt]; ok {
					lastUpdatedAt, err := time.Parse(time.RFC3339Nano, val)
					if err != nil {
						return ctrl.Result{}, errors.Wrapf(err, "found invalid timestamp in configmap annotation %s: %s", annotationLastUpdatedAt, val)
					}
					if now.Before(lastUpdatedAt.Add(r.CorednsConfigMapUpdateDelay)) {
						log.V(1).Info("delaying update of configmap", "namespace", r.CorednsConfigMapNamespace, "name", r.CorednsConfigMapName)
						return ctrl.Result{RequeueAfter: 1 * time.Second}, nil
					}
				}
				if configMap.Annotations == nil {
					configMap.Annotations = make(map[string]string)
				}
				configMap.Annotations[annotationLastUpdatedAt] = now.Format(time.RFC3339Nano)
				// end
				configMap.Data[r.CorednsConfigMapKey] = ruleset.String()
				if err := r.Update(ctx, configMap, client.FieldOwner(fieldOwner)); err != nil {
					return ctrl.Result{}, errors.Wrapf(err, "error updating config map %s/%s", configMap.Namespace, configMap.Name)
				}
				log.V(1).Info("configmap successfully updated", "namespace", r.CorednsConfigMapNamespace, "name", r.CorednsConfigMapName)
				return ctrl.Result{RequeueAfter: 1 * time.Second}, nil
			}
		}

		active, err := r.Resolver.CheckRecord(ctx, regexp.MustCompile(`^\*(.*)$`).ReplaceAllString(masqueradingRule.Spec.From, `wildcard$1`), masqueradingRule.Spec.To)
		if err != nil {
			return ctrl.Result{}, errors.Wrap(err, "error check DNS record")
		}

		if active {
			masqueradingRule.SetState(dnsv1alpha1.MasqueradingRuleStateReady, "masquerading rule completely reconciled")
			r.Recorder.Eventf(masqueradingRule, corev1.EventTypeNormal, "ReconcilationSucceeded", "masquerading rule completely reconciled")
			log.V(1).Info("dns record active")
			return ctrl.Result{RequeueAfter: 10 * time.Minute}, nil
		} else {
			masqueradingRule.SetState(dnsv1alpha1.MasqueradingRuleStateProcessing, "waiting for masquerading rule to be reconciled")
			r.Recorder.Eventf(masqueradingRule, corev1.EventTypeWarning, "ReconcilationProcessing", "waiting for masquerading rule to be reconciled")
			log.V(1).Info("dns record not (active); rechecking in 10s ...")
			return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
		}
	} else if len(slices.Remove(masqueradingRule.Finalizers, finalizer)) > 0 {
		masqueradingRule.SetState(dnsv1alpha1.MasqueradingRuleStateDeletionBlocked, "Deletion blocked due to foreign finalizers")
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	} else {
		// Deletion case
		if configMap != nil {
			ruleset, err := coredns.ParseRewriteRuleSet(configMap.Data[r.CorednsConfigMapKey])
			if err != nil {
				return ctrl.Result{}, errors.Wrapf(err, "error loading rewrite rules from config map %s/%s (key: %s)", configMap.Namespace, configMap.Name, r.CorednsConfigMapKey)
			}
			changed := ruleset.RemoveRule(owner)
			if changed {
				masqueradingRule.SetState(dnsv1alpha1.MasqueradingRuleStateDeleting, "waiting for masquerading rule to be deleted")
				if configMap.Data == nil {
					configMap.Data = make(map[string]string)
				}
				// TODO: the following is needed (for test execution) until we have https://github.com/coredns/coredns/issues/6243 or a similar fix;
				// note: delaying the update is probably not required in 'real' deployments, since high-frequency configmap updates are
				// anyway buffered there by kubelet's configmap/secret distribution logic.
				now := time.Now()
				if val, ok := configMap.Annotations[annotationLastUpdatedAt]; ok {
					lastUpdatedAt, err := time.Parse(time.RFC3339Nano, val)
					if err != nil {
						return ctrl.Result{}, errors.Wrapf(err, "found invalid timestamp in configmap annotation %s: %s", annotationLastUpdatedAt, val)
					}
					if now.Before(lastUpdatedAt.Add(r.CorednsConfigMapUpdateDelay)) {
						log.V(1).Info("delaying update of configmap", "namespace", r.CorednsConfigMapNamespace, "name", r.CorednsConfigMapName)
						return ctrl.Result{RequeueAfter: 1 * time.Second}, nil
					}
				}
				if configMap.Annotations == nil {
					configMap.Annotations = make(map[string]string)
				}
				configMap.Annotations[annotationLastUpdatedAt] = now.Format(time.RFC3339Nano)
				// end
				configMap.Data[r.CorednsConfigMapKey] = ruleset.String()
				if err := r.Update(ctx, configMap, client.FieldOwner(fieldOwner)); err != nil {
					return ctrl.Result{}, errors.Wrapf(err, "error updating config map %s/%s", configMap.Namespace, configMap.Name)
				}
				log.V(1).Info("configmap successfully updated", "namespace", r.CorednsConfigMapNamespace, "name", r.CorednsConfigMapName)
				return ctrl.Result{RequeueAfter: 1 * time.Second}, nil
			}
		}

		if slices.Contains(masqueradingRule.Finalizers, finalizer) {
			controllerutil.RemoveFinalizer(masqueradingRule, finalizer)
			if err := r.Update(ctx, masqueradingRule, client.FieldOwner(fieldOwner)); err != nil {
				return ctrl.Result{}, errors.Wrap(err, "error clearing finalizer")
			}
		}
		// skip status update, since the resource will anyway deleted timely by the API server
		// this will suppress unnecessary ugly 409'ish error messages in the logs
		// (occurring in the case that API server would delete the resource in the course of the subsequent reconciliation)
		skipStatusUpdate = true
		return ctrl.Result{}, nil
	}
}

// record an event for specified object
func (r *MasqueradingRuleReconciler) createEventForObject(ctx context.Context, gvk schema.GroupVersionKind, namespace string, name string, eventType string, reason string, message string, args ...interface{}) error {
	owner, err := r.Scheme.New(gvk)
	if err != nil {
		return err
	}
	if err := r.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, owner.(client.Object)); err != nil {
		return err
	}
	r.Recorder.Eventf(owner, eventType, reason, message, args...)
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *MasqueradingRuleReconciler) SetupWithManager(mgr ctrl.Manager) error {
	predicate := predicate.Or(predicate.GenerationChangedPredicate{}, predicate.AnnotationChangedPredicate{})
	return ctrl.NewControllerManagedBy(mgr).
		For(&dnsv1alpha1.MasqueradingRule{}, builder.WithPredicates(predicate)).
		WithOptions(controller.Options{MaxConcurrentReconciles: 3}).
		Complete(r)
}

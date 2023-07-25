/*
SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and dns-masquerading-operator contributors
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
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	dnsv1alpha1 "github.com/sap/dns-masquerading-operator/api/v1alpha1"
	"github.com/sap/dns-masquerading-operator/internal/coredns"
)

const (
	finalizer = "dns.cs.sap.com/masquerading-operator"
)

// MasqueradingRuleReconciler reconciles a MasqueradingRule object
type MasqueradingRuleReconciler struct {
	client.Client
	Scheme                    *runtime.Scheme
	Config                    *rest.Config
	Recorder                  record.EventRecorder
	CorednsConfigMapNamespace string
	CorednsConfigMapName      string
	CorednsConfigMapKey       string
	InCluster                 bool
}

//+kubebuilder:rbac:groups=dns.cs.sap.com,resources=masqueradingrules,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=dns.cs.sap.com,resources=masqueradingrules/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=dns.cs.sap.com,resources=masqueradingrules/finalizers,verbs=update
//+kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update
//+kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=endpoints,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=pods/portforward,verbs=create

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
		}
		if updateErr := r.Status().Update(ctx, masqueradingRule); updateErr != nil {
			err = utilerrors.NewAggregate([]error{err, updateErr})
			result = ctrl.Result{}
		}
	}()

	// Create events on owners (if any)
	defer func() {
		for _, ownerRef := range masqueradingRule.OwnerReferences {
			if masqueradingRule.Status.State == dnsv1alpha1.MasqueradingRuleStateReady && previousMasqueradingRuleStatus.State != dnsv1alpha1.MasqueradingRuleStateReady {
				if err := r.createEventForOwnerRef(
					ctx,
					masqueradingRule.Namespace,
					ownerRef,
					corev1.EventTypeNormal,
					"ReconciliationSucceeded",
					"Masquerading rule %s/%s (host %s) successfully reconciled",
					masqueradingRule.Namespace,
					masqueradingRule.Name,
					masqueradingRule.Spec.From,
				); err != nil {
					log.Error(err, "failed to record event for owner", "version", ownerRef.APIVersion, "kind", ownerRef.Kind, "name", ownerRef.Name)
				}
			} else if masqueradingRule.Status.State == dnsv1alpha1.MasqueradingRuleStateError && previousMasqueradingRuleStatus.State != dnsv1alpha1.MasqueradingRuleStateError {
				if err := r.createEventForOwnerRef(
					ctx,
					masqueradingRule.Namespace,
					ownerRef,
					corev1.EventTypeWarning,
					"ReconciliationFailed",
					"Masquerading rule %s/%s (host %s) reconciliation failed",
					masqueradingRule.Namespace,
					masqueradingRule.Name,
					masqueradingRule.Spec.From,
				); err != nil {
					log.Error(err, "failed to record event for owner", "version", ownerRef.APIVersion, "kind", ownerRef.Kind, "name", ownerRef.Name)
				}
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
	if masqueradingRule.DeletionTimestamp.IsZero() {
		// Create/update case
		if !slices.Contains(masqueradingRule.Finalizers, finalizer) {
			controllerutil.AddFinalizer(masqueradingRule, finalizer)
			if err := r.Update(ctx, masqueradingRule); err != nil {
				return ctrl.Result{}, errors.Wrap(err, "error setting finalizer")
			}
		}

		if configMap == nil {
			ruleset := coredns.NewRewriteRuleSet()
			if err := ruleset.AddRule(coredns.RewriteRule{Owner: owner, From: masqueradingRule.Spec.From, To: masqueradingRule.Spec.To}); err != nil {
				return ctrl.Result{}, errors.Wrap(err, "error adding rewrite rule")
			}
			configMap = &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: r.CorednsConfigMapNamespace,
					Name:      r.CorednsConfigMapName,
				},
				Data: map[string]string{
					r.CorednsConfigMapKey: ruleset.String(),
				},
			}
			if err := r.Create(ctx, configMap, &client.CreateOptions{}); err != nil {
				return ctrl.Result{}, errors.Wrapf(err, "error creating config map %s/%s", configMap.Namespace, configMap.Name)
			}
			log.V(1).Info("configmap successfully created", "namespace", r.CorednsConfigMapNamespace, "name", r.CorednsConfigMapName)
			masqueradingRule.SetState(dnsv1alpha1.MasqueradingRuleStateProcessing, "waiting for masquerading rule to be reconciled")
			return ctrl.Result{RequeueAfter: 1 * time.Second}, nil
		} else {
			ruleset, err := coredns.ParseRewriteRuleSet(configMap.Data[r.CorednsConfigMapKey])
			if err != nil {
				return ctrl.Result{}, errors.Wrapf(err, "error loading rewrite rules from config map %s/%s (key: %s)", configMap.Namespace, configMap.Name, r.CorednsConfigMapKey)
			}
			if rule := ruleset.GetRule(owner); rule == nil || rule.From != masqueradingRule.Spec.From || rule.To != masqueradingRule.Spec.To {
				if err := ruleset.AddRule(coredns.RewriteRule{Owner: owner, From: masqueradingRule.Spec.From, To: masqueradingRule.Spec.To}); err != nil {
					return ctrl.Result{}, errors.Wrap(err, "error adding rewrite rule")
				}
				if configMap.Data == nil {
					configMap.Data = make(map[string]string)
				}
				configMap.Data[r.CorednsConfigMapKey] = ruleset.String()
				if err := r.Update(ctx, configMap, &client.UpdateOptions{}); err != nil {
					return ctrl.Result{}, errors.Wrapf(err, "error updating config map %s/%s", configMap.Namespace, configMap.Name)
				}
				log.V(1).Info("configmap successfully updated", "namespace", r.CorednsConfigMapNamespace, "name", r.CorednsConfigMapName)
				masqueradingRule.SetState(dnsv1alpha1.MasqueradingRuleStateProcessing, "waiting for masquerading rule to be reconciled")
				return ctrl.Result{RequeueAfter: 1 * time.Second}, nil
			}
		}

		active, err := coredns.CheckRecord(ctx, r.Client, r.Config, regexp.MustCompile(`^\*\.(.+)$`).ReplaceAllString(masqueradingRule.Spec.From, `wildcard.$1`), masqueradingRule.Spec.To, r.InCluster)
		if err != nil {
			return ctrl.Result{}, errors.Wrap(err, "error check DNS record")
		}

		if active {
			log.V(1).Info("dns record active")
			masqueradingRule.SetState(dnsv1alpha1.MasqueradingRuleStateReady, "masquerading rule completely reconciled")
			return ctrl.Result{RequeueAfter: 10 * time.Minute}, nil
		} else {
			log.V(1).Info("dns record not (active); rechecking in 10s ...")
			masqueradingRule.SetState(dnsv1alpha1.MasqueradingRuleStateProcessing, "waiting for masquerading rule to be reconciled")
			return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
		}
	} else if len(slices.Remove(masqueradingRule.Finalizers, finalizer)) > 0 {
		masqueradingRule.SetState(dnsv1alpha1.MasqueradingRuleStateDeletionBlocked, "Deletion blocked due to foreign finalizers")
		// TODO: apply some increasing period, depending on the age of the last update
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	} else {
		// Deletion case
		if configMap != nil {
			ruleset, err := coredns.ParseRewriteRuleSet(configMap.Data[r.CorednsConfigMapKey])
			if err != nil {
				return ctrl.Result{}, errors.Wrapf(err, "error loading rewrite rules from config map %s/%s (key: %s)", configMap.Namespace, configMap.Name, r.CorednsConfigMapKey)
			}
			if rule := ruleset.GetRule(owner); rule != nil {
				if err := ruleset.RemoveRule(owner); err != nil {
					return ctrl.Result{}, errors.Wrap(err, "error removing rewrite rule")
				}
				if configMap.Data == nil {
					configMap.Data = make(map[string]string)
				}
				configMap.Data[r.CorednsConfigMapKey] = ruleset.String()
				if err := r.Update(ctx, configMap, &client.UpdateOptions{}); err != nil {
					return ctrl.Result{}, errors.Wrapf(err, "error updating config map %s/%s", configMap.Namespace, configMap.Name)
				}
				log.V(1).Info("configmap successfully updated", "namespace", r.CorednsConfigMapNamespace, "name", r.CorednsConfigMapName)
				masqueradingRule.SetState(dnsv1alpha1.MasqueradingRuleStateDeleting, "waiting for masquerading rule to be deleted")
				return ctrl.Result{RequeueAfter: 1 * time.Second}, nil
			}
		}

		if slices.Contains(masqueradingRule.Finalizers, finalizer) {
			controllerutil.RemoveFinalizer(masqueradingRule, finalizer)
			if err := r.Update(ctx, masqueradingRule); err != nil {
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

// Record an event for an owner reference
func (r *MasqueradingRuleReconciler) createEventForOwnerRef(ctx context.Context, namespace string, ownerRef metav1.OwnerReference, eventType string, reason string, message string, args ...interface{}) error {
	owner, err := r.Scheme.New(schema.FromAPIVersionAndKind(ownerRef.APIVersion, ownerRef.Kind))
	if err != nil {
		return err
	}
	if err := r.Get(ctx, types.NamespacedName{Namespace: namespace, Name: ownerRef.Name}, owner.(client.Object), &client.GetOptions{}); err != nil {
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
		Complete(r)
}

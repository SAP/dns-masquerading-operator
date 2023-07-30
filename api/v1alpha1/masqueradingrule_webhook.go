/*
SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and dns-masquerading-operator contributors
SPDX-License-Identifier: Apache-2.0
*/

package v1alpha1

import (
	"fmt"

	"github.com/sap/dns-masquerading-operator/internal/netutil"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// log is for logging in this package.
var masqueradingrulelog = logf.Log.WithName("masqueradingrule-resource")

func (r *MasqueradingRule) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

//+kubebuilder:webhook:path=/mutate-dns-cs-sap-com-v1alpha1-masqueradingrule,mutating=true,failurePolicy=fail,sideEffects=None,groups=dns.cs.sap.com,resources=masqueradingrules,verbs=create;update,versions=v1alpha1,name=mmasqueradingrule.kb.io,admissionReviewVersions=v1

var _ webhook.Defaulter = &MasqueradingRule{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *MasqueradingRule) Default() {
	masqueradingrulelog.Info("default", "name", r.Name)
}

//+kubebuilder:webhook:path=/validate-dns-cs-sap-com-v1alpha1-masqueradingrule,mutating=false,failurePolicy=fail,sideEffects=None,groups=dns.cs.sap.com,resources=masqueradingrules,verbs=create;update;delete,versions=v1alpha1,name=vmasqueradingrule.kb.io,admissionReviewVersions=v1

var _ webhook.Validator = &MasqueradingRule{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *MasqueradingRule) ValidateCreate() error {
	masqueradingrulelog.Info("validate create", "name", r.Name)

	return r.validate()
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *MasqueradingRule) ValidateUpdate(old runtime.Object) error {
	masqueradingrulelog.Info("validate update", "name", r.Name)

	return r.validate()
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *MasqueradingRule) ValidateDelete() error {
	masqueradingrulelog.Info("validate delete", "name", r.Name)

	return nil
}

func (r *MasqueradingRule) validate() error {
	if err := netutil.CheckDnsName(r.Spec.From, true); err != nil {
		return fmt.Errorf("invalid .spec.from: %s (%s)", r.Spec.From, err)
	}
	if netutil.IsIpAddress(r.Spec.To) {
		if netutil.IsWildcardDnsName(r.Spec.From) {
			return fmt.Errorf("invalid .spec.to: %s (.spec.from must not be a wildcard DNS name if .spec.to is an IP address)", r.Spec.To)
		}
	} else {
		if err := netutil.CheckDnsName(r.Spec.To, false); err != nil {
			return fmt.Errorf("invalid .spec.to: %s (%s)", r.Spec.To, err)
		}
	}
	return nil
}

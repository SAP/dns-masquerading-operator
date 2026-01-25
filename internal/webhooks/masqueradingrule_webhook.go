package webhooks

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/sap/dns-masquerading-operator/api/v1alpha1"
	"github.com/sap/dns-masquerading-operator/internal/coredns"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

type MasqueradingRuleWebhook struct {
	Log logr.Logger
}

func (w *MasqueradingRuleWebhook) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &v1alpha1.MasqueradingRule{}).
		WithValidator(w).
		WithDefaulter(w).
		Complete()
}

func (w *MasqueradingRuleWebhook) ValidateCreate(ctx context.Context, masqueradingRule *v1alpha1.MasqueradingRule) (admission.Warnings, error) {
	w.Log.Info("validate create", "name", masqueradingRule.Name)

	return nil, w.validate(masqueradingRule)
}

func (w *MasqueradingRuleWebhook) ValidateUpdate(ctx context.Context, oldmasqueradingRule *v1alpha1.MasqueradingRule, masqueradingRule *v1alpha1.MasqueradingRule) (admission.Warnings, error) {
	w.Log.Info("validate update", "name", masqueradingRule.Name)

	return nil, w.validate(masqueradingRule)
}

func (w *MasqueradingRuleWebhook) ValidateDelete(ctx context.Context, masqueradingRule *v1alpha1.MasqueradingRule) (admission.Warnings, error) {
	w.Log.Info("validate delete", "name", masqueradingRule.Name)

	return nil, nil
}

func (w *MasqueradingRuleWebhook) Default(ctx context.Context, masqueradingRule *v1alpha1.MasqueradingRule) error {
	w.Log.Info("default", "name", masqueradingRule.Name)

	return nil
}

func (w *MasqueradingRuleWebhook) validate(masqueradingRule *v1alpha1.MasqueradingRule) error {
	_, err := coredns.NewRewriteRule("", masqueradingRule.Spec.From, masqueradingRule.Spec.To)
	if err != nil {
		return fmt.Errorf("invalid rule specification: %s", err)
	}
	return nil
}

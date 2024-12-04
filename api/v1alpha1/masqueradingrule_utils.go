/*
SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and dns-masquerading-operator contributors
SPDX-License-Identifier: Apache-2.0
*/

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Set state (and the 'Ready' condition) of a MasqueradingRule
func (masqueradingRule *MasqueradingRule) SetState(state MasqueradingRuleState, message string) {
	conditionStatus := corev1.ConditionUnknown

	switch state {
	case MasqueradingRuleStateReady:
		conditionStatus = corev1.ConditionTrue
	case MasqueradingRuleStateError:
		conditionStatus = corev1.ConditionFalse
	}

	setCondition(&masqueradingRule.Status.Conditions, MasqueradingRuleConditionTypeReady, conditionStatus, string(state), message)
	masqueradingRule.Status.State = state
}

func getCondition(conditions []MasqueradingRuleCondition, conditionType MasqueradingRuleConditionType) *MasqueradingRuleCondition {
	for i := 0; i < len(conditions); i++ {
		if conditions[i].Type == conditionType {
			return &conditions[i]
		}
	}
	return nil
}

func setCondition(conditions *[]MasqueradingRuleCondition, conditionType MasqueradingRuleConditionType, conditionStatus corev1.ConditionStatus, conditionReason string, conditionMessage string) {
	now := metav1.Now()

	cond := getCondition(*conditions, conditionType)

	if cond == nil {
		*conditions = append(*conditions, MasqueradingRuleCondition{Type: conditionType, LastTransitionTime: &now})
		cond = &(*conditions)[len(*conditions)-1]
	} else if cond.Status != conditionStatus {
		cond.LastTransitionTime = &now
	}
	cond.LastUpdateTime = &now
	cond.Status = conditionStatus
	cond.Reason = conditionReason
	cond.Message = conditionMessage
}

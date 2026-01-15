/*
SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and dns-masquerading-operator contributors
SPDX-License-Identifier: Apache-2.0
*/

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="State",type=string,JSONPath=`.status.state`
//+kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
//+genclient

// MasqueradingRule is the Schema for the masqueradingrules API
type MasqueradingRule struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec MasqueradingRuleSpec `json:"spec,omitempty"`
	// +kubebuilder:default={"observedGeneration":-1}
	Status MasqueradingRuleStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// MasqueradingRuleList contains a list of MasqueradingRule
type MasqueradingRuleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MasqueradingRule `json:"items"`
}

// MasqueradingRuleSpec defines the desired state of MasqueradingRule
type MasqueradingRuleSpec struct {
	// +kubebuilder:validation:Pattern=^(\*|[a-z0-9]|[a-z0-9][a-z0-9-]*[a-z0-9])(\.([a-z0-9]|[a-z0-9][a-z0-9-]*[a-z0-9]))*$
	From string `json:"from"`
	// +kubebuilder:validation:Pattern=^([a-z0-9]|[a-z0-9][a-z0-9-]*[a-z0-9])(\.([a-z0-9]|[a-z0-9][a-z0-9-]*[a-z0-9]))*$
	To string `json:"to"`
}

// MasqueradingRuleStatus defines the observed state of MasqueradingRule
type MasqueradingRuleStatus struct {
	// Observed generation
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// List of status conditions to indicate the status of a MasqueradingRule.
	// Known condition types are `Ready`.
	// +optional
	Conditions []MasqueradingRuleCondition `json:"conditions,omitempty"`

	// Readable form of the state.
	// +optional
	State MasqueradingRuleState `json:"state,omitempty"`
}

// MasqueradingRuleCondition contains condition information for a MasqueradingRule.
type MasqueradingRuleCondition struct {
	// Type of the condition, known values are ('Ready').
	Type MasqueradingRuleConditionType `json:"type"`

	// Status of the condition, one of ('True', 'False', 'Unknown').
	Status corev1.ConditionStatus `json:"status"`

	// LastUpdateTime is the timestamp corresponding to the last status
	// update of this condition.
	// +optional
	LastUpdateTime *metav1.Time `json:"lastUpdateTime,omitempty"`

	// LastTransitionTime is the timestamp corresponding to the last status
	// change of this condition.
	// +optional
	LastTransitionTime *metav1.Time `json:"lastTransitionTime,omitempty"`

	// Reason is a brief machine readable explanation for the condition's last
	// transition.
	// +optional
	Reason string `json:"reason,omitempty"`

	// Message is a human readable description of the details of the last
	// transition, complementing reason.
	// +optional
	Message string `json:"message,omitempty"`
}

// MasqueradingRuleConditionType represents a MasqueradingRule condition value.
type MasqueradingRuleConditionType string

const (
	// MasqueradingRuleConditionReady represents the fact that a given MasqueradingRule is ready.
	MasqueradingRuleConditionTypeReady MasqueradingRuleConditionType = "Ready"
)

// MasqueradingRuleState represents a condition state in a readable form
// +kubebuilder:validation:Enum=New;Processing;DeletionBlocked;Deleting;Ready;Error
type MasqueradingRuleState string

// These are valid condition states
const (
	// Represents the fact that the MasqueradingRule was first seen.
	MasqueradingRuleStateNew MasqueradingRuleState = "New"

	// MasqueradingRuleStateProcessing represents the fact that the MasqueradingRule is reconciling
	MasqueradingRuleStateProcessing MasqueradingRuleState = "Processing"

	// Represents the fact that the MasqueradingRule should be deleted, but deletion is blocked.
	MasqueradingRuleStateDeletionBlocked MasqueradingRuleState = "DeletionBlocked"

	// MasqueradingRuleStateProcessing represents the fact that the MasqueradingRule is being deleted
	MasqueradingRuleStateDeleting MasqueradingRuleState = "Deleting"

	// MasqueradingRuleStateProcessing represents the fact that the MasqueradingRule is ready
	MasqueradingRuleStateReady MasqueradingRuleState = "Ready"

	// MasqueradingRuleStateProcessing represents the fact that the MasqueradingRule is not ready resp. has an error
	MasqueradingRuleStateError MasqueradingRuleState = "Error"
)

func init() {
	SchemeBuilder.Register(&MasqueradingRule{}, &MasqueradingRuleList{})
}

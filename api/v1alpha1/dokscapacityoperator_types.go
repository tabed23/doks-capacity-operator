/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// DoksCapacityOperatorSpec defines the desired state of DoksCapacityOperator
type DoksCapacityOperatorSpec struct {
	// ClusterID is the DigitalOcean cluster UUID.
	// Optional: leave empty and the operator discovers it by matching
	// PoolID/PoolName across the clusters visible to the API token.
	// +optional
	ClusterID string `json:"clusterID,omitempty"`

	// PoolID is the DigitalOcean node pool UUID. Either PoolID or PoolName must
	// be set. PoolID is preferred because it is globally unique.
	// +optional
	PoolID string `json:"poolID,omitempty"`

	// PoolName is the node pool name. Used only when PoolID is empty.
	// +optional
	PoolName string `json:"poolName,omitempty"`

	// TriggerFreeNodes: expand when (poolMax - currentCount) <= this value.
	// +kubebuilder:default=3
	// +kubebuilder:validation:Minimum=0
	TriggerFreeNodes int `json:"triggerFreeNodes"`

	// ExpandBy: how many nodes to add to the pool's autoscale max per expansion.
	// +kubebuilder:default=5
	// +kubebuilder:validation:Minimum=1
	ExpandBy int `json:"expandBy"`

	// MaxNodes is the HARD CEILING. The pool's autoscale max is raised up to,
	// but never above, this value. DigitalOcean caps a node pool at 100.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=100
	MaxNodes int `json:"maxNodes"`

	// CooldownMinutes: minimum minutes between two expansions.
	// +kubebuilder:default=15
	// +kubebuilder:validation:Minimum=0
	CooldownMinutes int `json:"cooldownMinutes"`
}

// DoksCapacityOperatorStatus defines the observed state of DoksCapacityOperator.
type DoksCapacityOperatorStatus struct {
	// +optional
	ResolvedClusterID string `json:"resolvedClusterID,omitempty"`
	// +optional
	ResolvedPoolID string `json:"resolvedPoolID,omitempty"`
	// +optional
	CurrentNodeCount int `json:"currentNodeCount,omitempty"`
	// +optional
	CurrentMaxNodes int `json:"currentMaxNodes,omitempty"`
	// +optional
	LastExpansionTime *metav1.Time `json:"lastExpansionTime,omitempty"`
	// Phase is a coarse human-friendly state: Stable, Cooldown, Expanded,
	// AtCeiling, Blocked, Error.
	// +optional
	Phase string `json:"phase,omitempty"`
	// +optional
	Message string `json:"message,omitempty"`
	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=oco
// +kubebuilder:printcolumn:name="Pool",type=string,JSONPath=`.status.resolvedPoolID`
// +kubebuilder:printcolumn:name="Count",type=integer,JSONPath=`.status.currentNodeCount`
// +kubebuilder:printcolumn:name="Max",type=integer,JSONPath=`.status.currentMaxNodes`
// +kubebuilder:printcolumn:name="Ceiling",type=integer,JSONPath=`.spec.maxNodes`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`

// DoksCapacityOperator is the Schema for the dokscapacityoperators API
type DoksCapacityOperator struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of DoksCapacityOperator
	// +required
	Spec DoksCapacityOperatorSpec `json:"spec"`

	// status defines the observed state of DoksCapacityOperator
	// +optional
	Status DoksCapacityOperatorStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// DoksCapacityOperatorList contains a list of DoksCapacityOperator
type DoksCapacityOperatorList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Spec            DoksCapacityOperator     `json:"spec,omitempty"`
	Status          DoksCapacityOperatorSpec `json:"status,omitempty"`
}

func init() {
	SchemeBuilder.Register(&DoksCapacityOperator{}, &DoksCapacityOperatorList{})
}

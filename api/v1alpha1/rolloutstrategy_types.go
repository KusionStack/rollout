/*
 * Copyright 2023 The KusionStack Authors.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// PauseModeType is the type of the pause mode
type PauseModeType string

const (
	// PauseModeTypeFirstBatch is the pause mode of first batch
	PauseModeTypeFirstBatch PauseModeType = "FirstBatch"

	// PauseModeTypeEachBatch is the pause mode of each batch
	PauseModeTypeEachBatch PauseModeType = "EachBatch"

	// PauseModeEnvFirstBatch is the pause mode that the first batch of each env would be paused.
	PauseModeEnvFirstBatch PauseModeType = "EnvFirstBatch"

	// PauseModeEnvLastBatch is the pause mode that the last finished batch of each env would be paused.
	PauseModeEnvLastBatch PauseModeType = "EnvLastBatch"

	// PauseModeEnvFirstLastBatch is the pause mode that the first and the last finished batches of each env would be paused.
	PauseModeEnvFirstLastBatch PauseModeType = "EnvFirstAndLastBatch"

	// PauseModeTypeNever is the pause mode of never
	PauseModeTypeNever PauseModeType = "Never"
)

// CheckPointType is the type of the check point
type CheckPointType string

const (
	// CheckPointTypePreBatch indicates that do check before batch
	CheckPointTypePreBatch CheckPointType = "PreBatch"

	// CheckPointTypePostBatch indicates that do check after batch
	CheckPointTypePostBatch CheckPointType = "PostBatch"
)

// TolerationPolicy defines the toleration policy
type TolerationPolicy struct {
	// FailureThreshold indicates how many failed pods can be tolerated before marking the rollout batch as success
	// If not set, the default value is 0, which means no failed pods can be tolerated
	// +optional
	FailureThreshold *intstr.IntOrString `json:"failureThreshold,omitempty"`

	// UnitFailureThreshold indicates how many failed resources can be tolerated in a unit.
	// The default value is 0, which means no failed resources can be tolerated.
	// +optional
	UnitFailureThreshold *intstr.IntOrString `json:"unitFailureThreshold,omitempty"`

	// WaitTimeSeconds indicates how long to wait before starting the toleration check
	WaitTimeSeconds int32 `json:"waitTimeSeconds"`
}

// AnalysisStrategy defines the analysis strategy
type AnalysisStrategy struct {
	// Rules is rules for call provider
	Rules []*AnalysisRule `json:"rules"`
}

type AnalysisRule struct {
	// CheckPoints is conditions for call provider
	CheckPoints []CheckPointType `json:"checkPoints,omitempty"`

	// Provider is config for provider
	Provider AnalysisProvider `json:"provider,omitempty"`

	// IntervalSeconds is the interval for call provider
	IntervalSeconds int32 `json:"intervalSeconds,omitempty"`

	// MaxFailureCount is the threshold for call provider
	MaxFailureCount int32 `json:"maxFailureCount,omitempty"`
}

// AnalysisProvider defines the analysis provider
type AnalysisProvider struct {
	// Name is the name of the provider
	Name string `json:"name"`

	// Address is the address of the provider
	Address string `json:"address"`
}

// BatchTemplateProvider defines the batch template provider
type BatchTemplateProvider struct {
}

// BetaBatch defines the rollout policy of beta(first) batch
type BetaBatch struct {
	// Replicas indicates the replicas of the beta(first) batch, which will be chosen evenly from workloads
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`

	// ReplicasPerUnit indicates the replicas per rollout unit
	// +optional
	ReplicasPerUnit *int32 `json:"replicasPerUnit,omitempty"`
}

type FixedBatchPolicy struct {
	// BatchCount indicates the number of batches, workloads across different clusters in the same batch will be
	// upgraded at the same time
	BatchCount int32 `json:"batchCount"`

	// Breakpoints is the breakpoints of the fixed batch policy, which indicates pause points before determined batch
	Breakpoints []int32 `json:"breakpoints,omitempty"`

	// BetaBatch defines the rollout policy of beta(first) batch
	BetaBatch *BetaBatch `json:"betaBatch,omitempty"`
}

// RolloutUnit defines the rollout unit, which represents rollout partition of a workload
type RolloutUnit struct {
	// Name is the name of the rollout unit
	Name string `json:"name"`

	// Replicas is the replicas of the rollout unit, which represents the number of pods to be upgraded
	Replicas intstr.IntOrString `json:"replicas"`

	// Selector is the selector of the rollout unit, which is used to select the workload to be upgraded
	Selector *metav1.LabelSelector `json:"selector,omitempty"`

	// Targets are the specific name of resources to operate.
	Targets []string `json:"targets,omitempty"`
}

// Batch defines the batch
type Batch struct {
	// Units defines the rollout units of the batch
	Units []RolloutUnit `json:"units"`
}

// CustomBatchPolicy defines the custom batch policy
type CustomBatchPolicy struct {
	// Batches defines the batches of the custom batch policy
	Batches []Batch `json:"batches"`
}

// BatchTemplate defines the batch template
type BatchTemplate struct {
	// Provider is the provider of the batch template
	// +optional
	Provider *BatchTemplateProvider `json:"provider,omitempty"`

	// FixedBatchPolicy is the fixed batch policy of the batch template
	// +optional
	FixedBatchPolicy *FixedBatchPolicy `json:"fixedBatchPolicy,omitempty"`

	// CustomizedBatchPolicy is the customized batch policy of the batch template
	// +optional
	CustomBatchPolicy *CustomBatchPolicy `json:"customBatchPolicy,omitempty"`

	// PartialAdaptiveFactor is the special config for partial targets
	// +optional
	PartialAdaptiveFactor *PartialAdaptiveFactor `json:"partialAdaptiveFactor,omitempty"`
}

// PartialAdaptiveFactor defines the auto batch policy
type PartialAdaptiveFactor struct {
	// Threshold indicates how many pods can use this PartialAdaptiveFactor, default 50%
	// +optional
	Threshold *intstr.IntOrString `json:"threshold,omitempty"`
	// MinBatchCount indicates the number of min batches, default 2
	// +optional
	MinBatchCount int32 `json:"minBatchCount,omitempty"`
}

// BatchStrategy defines the batch strategy
type BatchStrategy struct {
	// PauseMode is the pause mode of the canary strategy
	PauseMode PauseModeType `json:"pauseMode"`

	// TolerationPolicy is the toleration policy of the canary strategy
	// +optional
	TolerationPolicy *TolerationPolicy `json:"tolerationPolicy,omitempty"`

	// Analysis defines the analysis of the canary strategy
	// +optional
	Analysis *AnalysisStrategy `json:"analysis,omitempty"`

	// BatchTemplate is the batch template of the canary strategy
	// +optional
	BatchTemplate *BatchTemplate `json:"batchTemplate,omitempty"`
}

// RolloutStrategySpec defines the desired state of RolloutStrategy
type RolloutStrategySpec struct {
	// Batch is the batch strategy for upgrade and operation
	// +optional
	Batch *BatchStrategy `json:"batch,omitempty"`
}

//+kubebuilder:object:root=true

// RolloutStrategy is the Schema for the rolloutstrategies API
type RolloutStrategy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec RolloutStrategySpec `json:"spec,omitempty"`
}

//+kubebuilder:object:root=true

// RolloutStrategyList contains a list of RolloutStrategy
type RolloutStrategyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RolloutStrategy `json:"items"`
}

func init() {
	SchemeBuilder.Register(&RolloutStrategy{}, &RolloutStrategyList{})
}

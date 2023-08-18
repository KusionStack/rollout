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
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// WorkloadRef is a reference to a workload
type WorkloadRef struct {
	//// APIVersion of the referent
	//APIVersion string `json:"apiVersion"`
	//
	//// Kind of the referent
	//Kind string `json:"kind"`
	metav1.TypeMeta `json:",inline"`

	// Selector is a label query over a set of resources, in this case workload
	Selector metav1.LabelSelector `json:"selector"`
}

// ApplicationSpecTemplateProvider defines the provider of the application spec template
type ApplicationSpecTemplateProvider struct {
	// Name is the name of the provider
	Name string `json:"name"`

	// Address is the address of the provider
	Address string `json:"address"`
}

// ApplicationSpecTemplate defines the template of the application
type ApplicationSpecTemplate struct {
	// Provider defines spec template provider of application, from which the spec template is rendered
	// +optional
	Provider *ApplicationSpecTemplateProvider `json:"provider,omitempty"`
}

// Application defines the application and its related workload
type Application struct {
	// WorkloadRef is a reference to a workload
	WorkloadRef WorkloadRef `json:"workloadRef"`

	// Template is the template of the application
	// +optional
	Template *ApplicationSpecTemplate `json:"template,omitempty"`

	// ExtInfo is the extended information of the application
	// +optional
	ExtInfo map[string]string `json:"extInfo,omitempty"`
}

// AutoTriggerCondition defines the condition to trigger the rollout automatically
type AutoTriggerCondition struct {
	// BarrierMatcher is a barrier matcher to match the rollout trigger condition
	// +optional
	BarrierMatcher *metav1.LabelSelectorRequirement `json:"barrierMatcher,omitempty"`
}

// TriggerTarget defines the target of the rollout
type TriggerTarget struct {
	// Pods is the list of pod names
	// +optional
	Pods []string `json:"pods,omitempty"`
}

// ManualTriggerCondition defines the condition to trigger the rollout manually
type ManualTriggerCondition struct {
	// RolloutId is the id of the rollout, trigger the rollout if the id is not matched with status.rolloutId
	RolloutId string `json:"rolloutId"`

	// Target is the target of the rollout
	// +optional
	Target *TriggerTarget `json:"target,omitempty"`
}

// TriggerCondition defines the condition to trigger the rollout
type TriggerCondition struct {
	// AutoMode indicates how the rollout is triggered automatically
	// +optional
	AutoMode *AutoTriggerCondition `json:"autoMode,omitempty"`

	// ManualMode indicates how the rollout is triggered manually
	// +optional
	ManualMode *ManualTriggerCondition `json:"manualMode,omitempty"`
}

// RolloutSpec defines the desired state of Rollout
type RolloutSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// App is the application and its related workload
	App Application `json:"app"`

	// StrategyRef is the reference to the rollout strategy
	StrategyRef string `json:"strategyRef"`

	// TriggerCondition is the condition to trigger the rollout
	// +optional
	TriggerCondition TriggerCondition `json:"triggerCondition,omitempty"`
}

// StageType is used for indicates the type of Stage
type StageType string

const (
	// StageTypeBatch means the Stage is for Batch
	StageTypeBatch StageType = "Batch"

	// StageTypeSuspend means the Stage is for Suspend task
	StageTypeSuspend StageType = "Suspend"
)

// RolloutConditionType is a valid value for RolloutCondition.Type
type RolloutConditionType string

const (
	// RolloutConditionPaused means the rollout is paused
	RolloutConditionPaused RolloutConditionType = "Paused"

	// RolloutConditionProgressing means the rollout is progressing
	RolloutConditionProgressing RolloutConditionType = "Progressing"

	// RolloutConditionAborted means the rollout is aborted
	RolloutConditionAborted RolloutConditionType = "Aborted"

	// RolloutConditionCompleted means the rollout is completed
	RolloutConditionCompleted RolloutConditionType = "Completed"
)

type RolloutCondition struct {
	// Type of rollout condition.
	Type RolloutConditionType `json:"type"`
	// Status of the condition, one of True, False, Unknown.
	Status metav1.ConditionStatus `json:"status"`
	// Last time the condition transitioned from one status to another.
	// +optional
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty"`
	// The last time this condition was updated.
	// +optional
	ListUpdateTime metav1.Time `json:"listUpdateTime,omitempty"`
	// The reason for the condition's last transition.
	// +optional
	Reason string `json:"reason,omitempty"`
	// A human-readable message indicating details about the transition.
	// +optional
	Message string `json:"message,omitempty"`
}

// RolloutPhase is a valid value for Rollout.Status.Phase
type RolloutPhase string

const (
	// RolloutPhasePending means the rollout is pending
	RolloutPhasePending RolloutPhase = "Pending"

	// RolloutPhaseRunning means the rollout is running
	RolloutPhaseRunning RolloutPhase = "Running"

	// RolloutPhasePaused means the rollout is paused
	RolloutPhasePaused RolloutPhase = "Paused"

	// RolloutPhaseAborted means the rollout is aborted
	RolloutPhaseAborted RolloutPhase = "Aborted"

	// RolloutPhaseCompleted means the rollout is completed
	RolloutPhaseCompleted RolloutPhase = "Completed"
)

// RolloutStatus defines the observed state of Rollout
type RolloutStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// ObservedGeneration is the most recent generation observed for this Rollout. It corresponds to the
	// Rollout's generation, which is updated on mutation by the API Server.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// ObservedRolloutID is the id of the rollout
	ObservedRolloutID string `json:"observedRolloutId,omitempty"`

	// StartedAt is the time when the rollout started
	// +optional
	StartedAt *metav1.Time `json:"startedAt,omitempty"`

	// FinishedAt is the time when the rollout finished
	// +optional
	FinishedAt *metav1.Time `json:"finishedAt,omitempty"`

	// LastUpdatedAt is the time when the rollout was last updated
	// +optional
	LastUpdatedAt *metav1.Time `json:"lastUpdatedAt,omitempty"`

	// Conditions is the list of conditions
	Conditions Conditions `json:"conditions,omitempty"`

	// WorkflowInfo is the info of latest workflow
	WorkflowInfo WorkflowInfo `json:"workflowInfo,omitempty"`

	// Summary is the summary of the rollout
	// +optional
	Summary *RolloutSummary `json:"summary,omitempty"`

	// Stages is the stages of the rollout
	// +optional
	Stages []Stage `json:"stages,omitempty"`

	// CurrentStageIndex is the index of current rollout stage
	// +optional
	CurrentStageIndex int32 `json:"currentStageIndex,omitempty"`
}

// RolloutSummary defines the summary of the rollout
type RolloutSummary struct {
	Replicas                 int32 `json:"replicas"`
	UpdatedReplicas          int32 `json:"updatedReplicas"`
	UpdatedReadyReplicas     int32 `json:"updatedReadyReplicas"`
	UpdatedAvailableReplicas int32 `json:"updatedAvailableReplicas"`
}

// WorkflowInfo defines the details of related workflow
type WorkflowInfo struct {
	// Name of workflow
	Name string `json:"name"`

	// Conditions of workflow
	Conditions Conditions `json:"conditions,omitempty"`
}

func (rs *RolloutStatus) GetConditions() Conditions {
	return rs.Conditions
}

func (rs *RolloutStatus) SetConditions(conditions Conditions) {
	rs.Conditions = conditions
}

// SetCondition sets the condition with the given type, update LastTransitionTime if the status changes
func (rs *RolloutStatus) SetCondition(conditionType ConditionType, status metav1.ConditionStatus, reason, message string) {
	newCondition := Condition{
		Type:    conditionType,
		Status:  status,
		Reason:  reason,
		Message: message,
	}
	cm := NewConditionManager(rs)
	cm.SetCondition(newCondition)
}

// UpdateCondition sets the conditions of the task
func (rs *RolloutStatus) UpdateCondition() {
	if len(rs.Stages) == 0 {
		return
	}

	var succeed, cancelled, failed, skipped, running, pending, paused int
	var suspended bool
	for _, stage := range rs.Stages {
		if stage.Type == StageTypeSuspend && stage.IsPaused() {
			suspended = true
			break
		}
		if stage.Type == StageTypeBatch {
			switch {
			case stage.IsSucceeded():
				succeed++
			case stage.IsCancelled():
				cancelled++
			case stage.IsFailed():
				failed++
			case stage.IsSkipped():
				skipped++
			case stage.IsRunning():
				running++
			case stage.IsPaused():
				paused++
			default:
				pending++
			}
		}
	}

	status := metav1.ConditionUnknown
	reason := ConditionReasonRunning.String()
	message := fmt.Sprintf("Stages suspended: %v, succeed: %d cancelled %d failed %d skipped %d running %d pending %d",
		suspended, succeed, cancelled, failed, skipped, running, pending)

	if suspended {
		reason = ConditionReasonPaused.String()
	} else if running == 0 {
		if failed > 0 {
			status = metav1.ConditionFalse
			reason = ConditionReasonFailed.String()
		} else if paused > 0 {
			reason = ConditionReasonPaused.String()
		} else if succeed == len(rs.Stages) {
			status = metav1.ConditionTrue
			reason = ConditionReasonSucceeded.String()
		} else if pending == len(rs.Stages) {
			reason = ConditionReasonPending.String()
		}
	}

	rs.SetCondition(ConditionSucceeded, status, reason, message)
}

// Stage defines the info of a stage
type Stage struct {
	// Type defines different stage type, Valid values are:
	// - Batch: Executed batch stage.
	// - Suspend: Control stage between executed batches.
	Type StageType `json:"type,omitempty"`

	// Name is the name of the stage
	Name string `json:"name"`

	// TODO(hengzhuo): consider whether we do need prevStage and nextStage
	// PreviousStage is the name of previous stage
	PrevStage string `json:"prevStage,omitempty"`

	// NextStage is the name of next stage
	NextStage string `json:"nextStage,omitempty"`

	// SuccessCondition defines the condition of the stage
	SuccessCondition *Condition `json:"successCondition,omitempty"`

	// StartedAt is the time when the stage started
	// +optional
	StartedAt *metav1.Time `json:"startedAt,omitempty"`

	// FinishedAt is the time when the stage finished
	// +optional
	FinishedAt *metav1.Time `json:"finishedAt,omitempty"`

	// WorkloadDetails contains release details for each workload
	// +optional
	WorkloadDetails []*WorkloadReleaseTaskStatus `json:"workloadDetails,omitempty"`

	// Summary is the summary of the stage
	// +optional
	Summary *RolloutSummary `json:"summary,omitempty"`
}

func (ws *Stage) IsRunning() bool {
	return ws.SuccessCondition.IsUnknown()
}

// IsFailed returns true if the task is failed
func (ws *Stage) IsFailed() bool {
	return ws.SuccessCondition.IsFalse()
}

// IsSucceeded returns true if the task is successful
func (ws *Stage) IsSucceeded() bool {
	return ws.SuccessCondition.IsTrue()
}

// IsSkipped returns true if the task is skipped
func (ws *Stage) IsSkipped() bool {
	// todo: use condition instead of phase to determine the status of the task
	// whether to skip children tasks if the parent task is skipped
	// return ws.Phase == TaskPhaseSkipped
	return false
}

// IsCancelled returns true if the task is cancelled
func (ws *Stage) IsCancelled() bool {
	c := ws.SuccessCondition
	return c != nil && c.IsFalse() && c.Reason == ConditionReasonCancelled.String()
}

// IsPaused returns true if the task is suspended
func (ws *Stage) IsPaused() bool {
	c := ws.SuccessCondition
	return c != nil && c.IsUnknown() && c.Reason == ConditionReasonPaused.String()
}

// IsProcessing returns true if the task is not ended and not init
func (ws *Stage) IsProcessing() bool {
	return ws.IsRunning() || ws.IsPaused()
}

// UpdateBatchInfo sets the conditions and timestamp
func (ws *Stage) UpdateBatchInfo(tasks []*WorkflowTaskStatus) {
	if tasks == nil {
		return
	}

	needUpdateStart := false
	if ws.StartedAt == nil {
		needUpdateStart = true
	}

	ws.WorkloadDetails = []*WorkloadReleaseTaskStatus{}

	var succeed, cancelled, failed, skipped, running, pending, paused int
	for _, task := range tasks {
		if task.WorkloadRelease != nil {
			ws.WorkloadDetails = append(ws.WorkloadDetails, task.WorkloadRelease)
		}
		if needUpdateStart {
			if ws.StartedAt == nil {
				ws.StartedAt = task.StartedAt
			} else if task.StartedAt != nil && ws.StartedAt.Sub(task.StartedAt.Time) > 0 {
				ws.StartedAt = task.StartedAt
			}
		}

		if ws.FinishedAt == nil {
			ws.FinishedAt = task.FinishedAt
		} else if task.FinishedAt != nil && ws.FinishedAt.Sub(task.FinishedAt.Time) < 0 {
			ws.FinishedAt = task.FinishedAt
		}

		if task.SuccessCondition == nil || task.SuccessCondition.Type != ConditionSucceeded {
			pending++
		} else {
			switch {
			case task.IsSucceeded():
				succeed++
			case task.IsCancelled():
				cancelled++
			case task.IsFailed():
				failed++
			case task.IsSkipped():
				skipped++
			case task.IsRunning():
				running++
			case task.IsPaused():
				paused++
			default:
				pending++
			}
		}
	}

	status := metav1.ConditionUnknown
	reason := ConditionReasonRunning.String()

	completedTasks := succeed + cancelled
	incompleteTasks := running + pending + paused + failed
	message := fmt.Sprintf("Tasks Completed: %d (Succeed: %d, Cancelled: %d), Tasks Incomplete: %d (Cancelled: %d, Paused: %d, Running: %d, Pending: %d), Skipped: %d",
		completedTasks, succeed, failed, incompleteTasks, failed, paused, running, pending, skipped)

	if running == 0 {
		if failed > 0 {
			status = metav1.ConditionFalse
			reason = ConditionReasonFailed.String()
		} else if paused > 0 {
			reason = ConditionReasonPaused.String()
		} else if succeed == len(tasks) {
			status = metav1.ConditionTrue
			reason = ConditionReasonSucceeded.String()
		} else if pending == len(tasks) {
			reason = ConditionReasonPending.String()
		}
	}

	if ws.SuccessCondition == nil {
		ws.SuccessCondition = &Condition{Type: ConditionSucceeded}
	}
	ws.SuccessCondition.SetCondition(status, reason, message)
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// Rollout is the Schema for the rollouts API
type Rollout struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RolloutSpec   `json:"spec,omitempty"`
	Status RolloutStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// RolloutList contains a list of Rollout
type RolloutList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Rollout `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Rollout{}, &RolloutList{})
}

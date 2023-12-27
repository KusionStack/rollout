// Copyright 2023 The KusionStack Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package v1alpha1

import (
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// TaskSpecStatus defines the status of the task
type TaskSpecStatus string

const (
	// TaskSpecStatusCanceled indicates that the user wants to cancel the task
	TaskSpecStatusCanceled TaskSpecStatus = "Canceled"
	// TaskSpecStatusPaused indicates that the user wants to suspend the task
	TaskSpecStatusPaused TaskSpecStatus = "Paused"
	// TaskSpecStatusPending indicates that the task is pending
	TaskSpecStatusPending TaskSpecStatus = "Pending"
)

// TaskSpec defines the desired state of Task
type TaskSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// RetryStrategy defines the retry strategy for the task when it fails
	// +optional
	RetryStrategy *RetryStrategy `json:"retryStrategy,omitempty"`

	// Params is a list of input parameters required to run the task.
	// +optional
	Params Params `json:"params,omitempty"`

	// Status is used to control the status of the task
	// +optional
	Status TaskSpecStatus `json:"status,omitempty"`

	// Resource type task is used to manipulate k8s resources
	// +optional
	Resource *ResourceTask `json:"resource,omitempty"`

	// WorkloadRelease is used to release the workload
	// +optional
	WorkloadRelease *WorkloadReleaseTask `json:"workloadRelease,omitempty"`

	// Suspend is used to pause the task
	// +optional
	Suspend *SuspendTask `json:"suspend,omitempty"`

	// Webhook is used to run webhook
	// +optional
	Webhook *WebhookTask `json:"webhook,omitempty"`

	// Custom is used to run custom task defined by the user
	// +optional
	Custom *CustomTask `json:"custom,omitempty"`

	// Echo is used to print out the message, mainly for testing or demo purpose
	// +optional
	Echo *EchoTask `json:"echo,omitempty"`
}

// TaskType defines the type of the task
type TaskType string

const (
	// TaskTypeResource is the type of the task that manipulates k8s resources
	TaskTypeResource TaskType = "Resource"
	// TaskTypeSuspend is the type of the task that pauses the task
	TaskTypeSuspend TaskType = "Suspend"
	// TaskTypeWebhook is the type of the task that runs webhook
	TaskTypeWebhook TaskType = "Webhook"
	// TaskTypeCustom is the type of the task that runs custom task defined by the user
	TaskTypeCustom TaskType = "Custom"
	// TaskTypeEcho is the type of the task that prints out the message, mainly for testing or demo purpose
	TaskTypeEcho TaskType = "Echo"
	// TaskTypeWorkloadRelease is the type of the task that runs workflow
	TaskTypeWorkloadRelease TaskType = "WorkloadRelease"
	// TaskTypeUnknown is the type of the task that is unknown
	TaskTypeUnknown TaskType = "Unknown"
)

// GetType returns the type of the task
func (ts *TaskSpec) GetType() TaskType {
	if ts.Resource != nil {
		return TaskTypeResource
	}
	if ts.Suspend != nil {
		return TaskTypeSuspend
	}
	if ts.Webhook != nil {
		return TaskTypeWebhook
	}
	if ts.WorkloadRelease != nil {
		return TaskTypeWorkloadRelease
	}
	if ts.Custom != nil {
		return TaskTypeCustom
	}
	if ts.Echo != nil {
		return TaskTypeEcho
	}
	return TaskTypeUnknown
}

// TaskStatus defines the observed state of Task
type TaskStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// ObservedGeneration is the most recent generation observed for this Task. It corresponds to the
	// Task's generation, which is updated on mutation by the API Server.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Phase is the current phase of the task
	// +optional
	Phase TaskPhase `json:"phase,omitempty"`

	// StartedAt is the time when the task started
	// +optional
	StartedAt *metav1.Time `json:"startedAt,omitempty"`

	// FinishedAt is the time when the task finished
	// +optional
	FinishedAt *metav1.Time `json:"finishedAt,omitempty"`

	// LastUpdatedAt is the time when the task status was last updated
	// +optional
	LastUpdatedAt *metav1.Time `json:"lastUpdatedAt,omitempty"`

	// Message is the message of the task
	// +optional
	Message string `json:"message,omitempty"`

	// Conditions is a list of conditions of the task
	// +optional
	Conditions Conditions `json:"conditions,omitempty"`

	// WorkloadRelease is the status of the workload release task
	// +optional
	WorkloadRelease *WorkloadReleaseTaskStatus `json:"workloadRelease,omitempty"`

	// Webhook is the status of webhook task.
	Webhook *WebhookTaskStatus `json:"webhook,omitempty"`
}

// Succeed sets the task status to succeeded
func (ts *TaskStatus) Succeed(message string) {
	ts.SetCondition(WorkflowConditionSucceeded, metav1.ConditionTrue, WorkflowReasonSucceeded.String(), message)
	ts.Phase = TaskPhaseSucceeded
	ts.FinishedAt = &metav1.Time{Time: time.Now()}
}

// Fail sets the task status to failed
func (ts *TaskStatus) Fail(reason string, message string) {
	ts.SetCondition(WorkflowConditionSucceeded, metav1.ConditionFalse, reason, message)
	ts.Phase = TaskPhaseFailed
	ts.FinishedAt = &metav1.Time{Time: time.Now()}
}

// Running sets the task status to running
func (ts *TaskStatus) Running(reason, message string) {
	ts.SetCondition(WorkflowConditionSucceeded, metav1.ConditionUnknown, reason, message)
	ts.Phase = TaskPhaseRunning
}

// Pause sets the task status to paused
func (ts *TaskStatus) Pause(message string) {
	ts.SetCondition(WorkflowConditionSucceeded, metav1.ConditionUnknown, WorkflowReasonPaused.String(), message)
	ts.Phase = TaskPhasePaused
}

// Cancel sets the task status to canceled
func (ts *TaskStatus) Cancel(message string) {
	ts.SetCondition(WorkflowConditionSucceeded, metav1.ConditionFalse, WorkflowReasonCanceled.String(), message)
	ts.Phase = TaskPhaseCanceled
	ts.FinishedAt = &metav1.Time{Time: time.Now()}
}

// Pending sets the task status to pending
func (ts *TaskStatus) Pending(message string) {
	ts.SetCondition(WorkflowConditionSucceeded, metav1.ConditionUnknown, WorkflowReasonPending.String(), message)
	ts.Phase = TaskPhasePending
}

// IsCompleted returns true if the task is completed
func (ts *TaskStatus) IsCompleted() bool {
	return ts.IsSucceeded() || ts.IsCanceled() || ts.IsFailed() || ts.IsSkipped()
}

// IsRunning returns true if the task is running
func (ts *TaskStatus) IsRunning() bool {
	return ts.GetCondition(WorkflowConditionSucceeded).IsUnknown() && !ts.IsPaused() && !ts.IsPending()
}

// IsFailed returns true if the task is failed
func (ts *TaskStatus) IsFailed() bool {
	return ts.GetCondition(WorkflowConditionSucceeded).IsFalse()
}

// IsSucceeded returns true if the task is successful
func (ts *TaskStatus) IsSucceeded() bool {
	return ts.GetCondition(WorkflowConditionSucceeded).IsTrue()
}

// IsSkipped returns true if the task is skipped
func (ts *TaskStatus) IsSkipped() bool {
	// todo: use condition instead of phase to determine the status of the task
	// whether to skip children tasks if the parent task is skipped
	return ts.Phase == TaskPhaseSkipped
}

// IsCanceled returns true if the task is canceled
func (ts *TaskStatus) IsCanceled() bool {
	c := ts.GetCondition(WorkflowConditionSucceeded)
	return c != nil && c.IsFalse() && c.Reason == WorkflowReasonCanceled.String()
}

// IsPaused returns true if the task is suspended
func (ts *TaskStatus) IsPaused() bool {
	c := ts.GetCondition(WorkflowConditionSucceeded)
	return c != nil && c.IsUnknown() && c.Reason == WorkflowReasonPaused.String()
}

// IsPending returns true if the task is pending
func (ts *TaskStatus) IsPending() bool {
	c := ts.GetCondition(WorkflowConditionSucceeded)
	return c != nil && c.IsUnknown() && c.Reason == WorkflowReasonPending.String()
}

// GetCondition returns the condition of the task
func (ts *TaskStatus) GetCondition(conditionType ConditionType) *Condition {
	return NewConditionManager(ts).GetCondition(conditionType)
}

// SetCondition sets the condition of the task
func (ts *TaskStatus) SetCondition(conditionType ConditionType, status metav1.ConditionStatus, reason, message string) {
	newCondition := Condition{
		Type:    conditionType,
		Status:  status,
		Reason:  reason,
		Message: message,
	}
	if ts.Conditions == nil ||
		len(ts.Conditions) == 0 {
		newCondition.LastUpdateTime = metav1.Now()
		newCondition.LastTransitionTime = metav1.Now()
	}
	NewConditionManager(ts).SetCondition(newCondition)
}

// GetConditions returns the conditions of the task
func (ts *TaskStatus) GetConditions() Conditions {
	return ts.Conditions
}

// SetConditions sets the conditions of the task
func (ts *TaskStatus) SetConditions(conditions Conditions) {
	ts.Conditions = conditions
}

func (ts *TaskStatus) Initialize() {
	ts.StartedAt = &metav1.Time{Time: time.Now()}
	ts.Running(WorkflowReasonStarted.String(), "task started")
}

// TaskPhase defines the phase of the task
type TaskPhase string

const (
	// TaskPhasePending indicates that the task is pending
	TaskPhasePending TaskPhase = "Pending"
	// TaskPhaseRunning indicates that the task is running
	TaskPhaseRunning TaskPhase = "Running"
	// TaskPhaseSucceeded indicates that the task is succeeded
	TaskPhaseSucceeded TaskPhase = "Succeeded"
	// TaskPhaseFailed indicates that the task is failed
	TaskPhaseFailed TaskPhase = "Failed"
	// TaskPhaseSkipped indicates that the task is skipped
	TaskPhaseSkipped TaskPhase = "Skipped"
	// TaskPhaseCanceled indicates that the task is canceled
	TaskPhaseCanceled TaskPhase = "Canceled"
	// TaskPhasePaused indicates that the task is paused
	TaskPhasePaused TaskPhase = "Paused"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// Task is the Schema for the tasks API
type Task struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TaskSpec   `json:"spec,omitempty"`
	Status TaskStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
//+kubebuilder:object:root=true

// TaskList contains a list of Task
type TaskList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Task `json:"items"`
}

// IsPaused returns true if the task is paused
func (t *Task) IsPaused() bool {
	return t.Spec.Status == TaskSpecStatusPaused
}

// IsCanceled returns true if the task is canceled
func (t *Task) IsCanceled() bool {
	return t.Spec.Status == TaskSpecStatusCanceled
}

// HasStarted returns true if the task has started
func (t *Task) HasStarted() bool {
	return !t.Status.StartedAt.IsZero()
}

// IsDone returns true if the task is done
func (t *Task) IsDone() bool {
	return !t.Status.GetCondition(WorkflowConditionSucceeded).IsUnknown()
}

// IsPending returns true if the task needs to be pending
func (t *Task) IsPending() bool {
	return t.Spec.Status == TaskSpecStatusPending
}
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
	"sort"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"kusionstack.io/rollout/pkg/dag"
)

// +k8s:deepcopy-gen=false
type ConditionsAccessor interface {
	GetConditions() Conditions
	SetConditions(Conditions)
}

// +k8s:deepcopy-gen=false
type ConditionManager interface {
	GetCondition(t ConditionType) *Condition
	SetCondition(cond Condition)
}

func NewConditionManager(conditionsAccessor ConditionsAccessor) ConditionManager {
	return &conditionManager{
		conditionsAccessor: conditionsAccessor,
	}
}

// conditionManager is a helper to manage conditions.
type conditionManager struct {
	conditionsAccessor ConditionsAccessor
}

// GetCondition returns the condition with the provided type.
func (cm conditionManager) GetCondition(t ConditionType) *Condition {
	if cm.conditionsAccessor == nil {
		return nil
	}
	for _, cond := range cm.conditionsAccessor.GetConditions() {
		if cond.Type == t {
			return &cond
		}
	}
	return nil
}

// SetCondition sets or update the accessor's conditions with the provided condition.
func (cm conditionManager) SetCondition(cond Condition) {
	if cm.conditionsAccessor == nil {
		return
	}
	var conditions Conditions
	if cm.conditionsAccessor.GetConditions() == nil ||
		len(cm.conditionsAccessor.GetConditions()) == 0 {
		conditions = []Condition{cond}
	} else {
		for _, currentCondition := range cm.conditionsAccessor.GetConditions() {
			if currentCondition.Type != cond.Type {
				conditions = append(conditions, currentCondition)
			} else {
				if conditionEquals(currentCondition, cond) {
					// condition does not changed
					return
				}
				if currentCondition.Status == cond.Status {
					// inherite LastTransitionTime from current condition
					cond.LastTransitionTime = currentCondition.LastTransitionTime
				} else {
					cond.LastTransitionTime = metav1.Now()
				}
				cond.LastUpdateTime = metav1.Now()
				conditions = append(conditions, cond)
			}
		}
	}

	sort.Slice(conditions, func(i, j int) bool {
		return conditions[i].Type < conditions[j].Type
	})
	cm.conditionsAccessor.SetConditions(conditions)
}

func conditionEquals(a, b Condition) bool {
	if a.Type == b.Type &&
		a.Status == b.Status &&
		a.Reason == b.Reason &&
		a.Message == b.Message {
		return true
	}
	return false
}

const (
	// WorkflowConditionSucceeded specifies that the resource has finished.
	// For resource which run to completion.
	WorkflowConditionSucceeded ConditionType = "Succeeded"
)

// ConditionReason defines the reason for the condition's last transition
type ConditionReason string

const (
	// WorkflowReasonInitialized indicates that the workflow has just been initialized
	WorkflowReasonInitialized ConditionReason = "Initialized"
	// WorkflowReasonStarted indicates that the workflow has just started
	WorkflowReasonStarted ConditionReason = "Started"
	// WorkflowReasonSucceeded indicates that the workflow has succeeded
	WorkflowReasonSucceeded ConditionReason = "Succeeded"
	// WorkflowReasonFailed indicates that the workflow has failed
	WorkflowReasonFailed ConditionReason = "Failed"
	// WorkflowReasonCanceled indicates that the workflow has been canceled
	WorkflowReasonCanceled ConditionReason = "Canceled"
	// WorkflowReasonCanceling indicates that the workflow is being canceled
	WorkflowReasonCanceling ConditionReason = "Canceling"
	// WorkflowReasonPending indicates that the workflow is pending
	WorkflowReasonPending ConditionReason = "Pending"
	// WorkflowReasonRunning indicates that the workflow is running
	WorkflowReasonRunning ConditionReason = "Running"
	// WorkflowReasonPaused indicates that the workflow has been paused
	WorkflowReasonPaused ConditionReason = "Paused"
	// WorkflowReasonPausing indicates that the workflow is being paused
	WorkflowReasonPausing ConditionReason = "Pausing"
	// WorkflowReasonError indicates that the workflow has an error
	WorkflowReasonError ConditionReason = "Error"
)

func (r ConditionReason) String() string {
	return string(r)
}

// IsUnknown returns true if the condition is unknown
func (c *Condition) IsUnknown() bool {
	if c == nil {
		return false
	}
	return c.Status == metav1.ConditionUnknown
}

// IsTrue returns true if the condition is true
func (c *Condition) IsTrue() bool {
	if c == nil {
		return false
	}
	return c.Status == metav1.ConditionTrue
}

// IsFalse returns true if the condition is false
func (c *Condition) IsFalse() bool {
	if c == nil {
		return false
	}
	return c.Status == metav1.ConditionFalse
}

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// WorkflowSpecStatus defines the Workflow spec status the user can provide
type WorkflowSpecStatus string

const (
	// WorkflowSpecStatusCanceled indicates that user wants to cancel the rollout flow
	WorkflowSpecStatusCanceled WorkflowSpecStatus = "Canceled"

	// WorkflowSpecStatusPaused indicates that user wants to pause the rollout flow
	WorkflowSpecStatusPaused WorkflowSpecStatus = "Paused"

	// WorkflowSpecStatusPending indicates that user wants to postpone starting
	// the rollout flow until some condition is met
	WorkflowSpecStatusPending WorkflowSpecStatus = "Pending"
)

// WhenExpressions defines the condition to run the task
type WhenExpressions []WhenExpression

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// Workflow is the Schema for the workflows API
type Workflow struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   WorkflowSpec   `json:"spec,omitempty"`
	Status WorkflowStatus `json:"status,omitempty"`
}

// HasStarted returns true if the workflow has started
func (w *Workflow) HasStarted() bool {
	return !w.Status.StartedAt.IsZero()
}

// IsPending returns true if the workflow is pending
func (w *Workflow) IsPending() bool {
	return w.Spec.Status == WorkflowSpecStatusPending
}

// IsPaused returns true if the workflow is paused
func (w *Workflow) IsPaused() bool {
	return w.Spec.Status == WorkflowSpecStatusPaused
}

// IsCanceled returns true if the workflow is canceled
func (w *Workflow) IsCanceled() bool {
	return w.Spec.Status == WorkflowSpecStatusCanceled
}

// IsDone returns true if the workflow is done
func (w *Workflow) IsDone() bool {
	return !w.Status.GetCondition(WorkflowConditionSucceeded).IsUnknown()
}

// WorkflowSpec defines the desired state of Workflow
type WorkflowSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Status is used for pausing/canceling rollout flow
	// +optional
	Status WorkflowSpecStatus `json:"status,omitempty"`

	// Params is the list of parameters for the rollout flow
	Params Params `json:"params,omitempty"`

	// Tasks is the list of tasks in the rollout flow
	// +optional
	Tasks []WorkflowTask `json:"tasks,omitempty"`
}

// WorkflowTask defines the task in the rollout flow
type WorkflowTask struct {
	// Name is the name of the task
	// It is used to identify the task in the rollout flow, which will be used
	// with the runAfter field in the task to define the task dependency
	Name string `json:"name"`

	// DisplayName is the display name of the task, which will be used in the UI
	// +optional
	DisplayName string `json:"displayName,omitempty"`

	// Labels is the list of labels for the task
	// +optional
	Labels map[string]string `json:"labels,omitempty"`

	// When is the condition to run the task
	When WhenExpressions `json:"when,omitempty"`

	// RunAfter is the list of WorkflowTask names that the current task depends on
	RunAfter []string `json:"runAfter,omitempty"`

	// TaskSpec is the spec of the task
	TaskSpec TaskSpec `json:"taskSpec,omitempty"`
}

// RetryStrategy defines the retry strategy for the task when it fails
type RetryStrategy struct {
	// Limit is the maximum number of retries
	Limit int `json:"limit"`

	// Backoff is the backoff strategy
	// +optional
	Backoff *Backoff `json:"backoff,omitempty"`
}

// Backoff defines the backoff strategy
type Backoff struct {
	// todo: add more backoff strategies
}

// WorkflowStatus defines the observed state of Workflow
type WorkflowStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// ObservedGeneration is the most recent generation observed for this Workflow. It corresponds to the
	// Workflow's generation, which is updated on mutation by the API Server.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Phase is the rollout flow phase
	// +optional
	Phase WorkflowPhase `json:"phase,omitempty"`

	// StartedAt is the time when the rollout flow started
	// +optional
	StartedAt *metav1.Time `json:"startedAt,omitempty"`

	// FinishedAt is the time when the rollout flow finished
	// +optional
	FinishedAt *metav1.Time `json:"finishedAt,omitempty"`

	// LastUpdatedAt is the time when the rollout flow was last updated
	// +optional
	LastUpdatedAt *metav1.Time `json:"lastUpdatedAt,omitempty"`

	// Message indicates details about the current status
	// +optional
	Message string `json:"message,omitempty"`

	// Conditions is a list of conditions the rollout flow is in
	Conditions Conditions `json:"conditions,omitempty"`

	// SkippedTasks is the list of tasks that are skipped
	// +optional
	SkippedTasks []SkippedTask `json:"skippedTasks,omitempty"`

	// Tasks is the list of tasks in the rollout flow
	// +optional
	Tasks []WorkflowTaskStatus `json:"tasks,omitempty"`
}

// Initialize initializes the workflow status
func (wfs *WorkflowStatus) Initialize(taskStatuses []WorkflowTaskStatus) {
	wfs.StartedAt = &metav1.Time{Time: time.Now()}
	wfs.Running(WorkflowReasonInitialized.String(), "Workflow initialized")
	wfs.Tasks = taskStatuses
}

// Succeed sets the workflow status to succeeded
func (wfs *WorkflowStatus) Succeed(message string) {
	wfs.SetCondition(WorkflowConditionSucceeded, metav1.ConditionTrue, WorkflowReasonSucceeded.String(), message)
	wfs.Phase = WorkflowPhaseSucceeded
	wfs.FinishedAt = &metav1.Time{Time: time.Now()}
}

// Fail sets the workflow status to failed
func (wfs *WorkflowStatus) Fail(reason, message string) {
	wfs.SetCondition(WorkflowConditionSucceeded, metav1.ConditionFalse, reason, message)
	wfs.Phase = WorkflowPhaseFailed
	wfs.FinishedAt = &metav1.Time{Time: time.Now()}
}

// Cancel sets the workflow status to canceled
func (wfs *WorkflowStatus) Cancel(message string) {
	wfs.SetCondition(WorkflowConditionSucceeded, metav1.ConditionFalse, WorkflowReasonCanceled.String(), message)
	wfs.Phase = WorkflowPhaseCanceled
	wfs.FinishedAt = &metav1.Time{Time: time.Now()}
}

// Canceling sets the workflow status to canceling
func (wfs *WorkflowStatus) Canceling(message string) {
	wfs.SetCondition(WorkflowConditionSucceeded, metav1.ConditionUnknown, WorkflowReasonCanceling.String(), message)
	wfs.Phase = WorkflowPhaseCanceling
}

// Running sets the workflow status to running
func (wfs *WorkflowStatus) Running(reason, message string) {
	wfs.SetCondition(WorkflowConditionSucceeded, metav1.ConditionUnknown, reason, message)
}

// Pause sets the workflow status to paused
func (wfs *WorkflowStatus) Pause(message string) {
	wfs.SetCondition(WorkflowConditionSucceeded, metav1.ConditionUnknown, WorkflowReasonPaused.String(), message)
	wfs.Phase = WorkflowPhasePaused
}

// Pending sets the workflow status to pending
func (wfs *WorkflowStatus) Pending(message string) {
	wfs.SetCondition(WorkflowConditionSucceeded, metav1.ConditionUnknown, WorkflowReasonPending.String(), message)
	wfs.Phase = WorkflowPhasePending
}

// IsPending returns true if the workflow is pending
func (wfs *WorkflowStatus) IsPending() bool {
	c := wfs.GetCondition(WorkflowConditionSucceeded)
	return c != nil && c.IsUnknown() && c.Reason == WorkflowReasonPending.String()
}

// IsPaused returns true if the workflow is paused
func (wfs *WorkflowStatus) IsPaused() bool {
	c := wfs.GetCondition(WorkflowConditionSucceeded)
	return c != nil && c.IsUnknown() && c.Reason == WorkflowReasonPaused.String()
}

// IsPausing returns true if the workflow is pausing
func (wfs *WorkflowStatus) IsPausing() bool {
	c := wfs.GetCondition(WorkflowConditionSucceeded)
	return c != nil && c.IsUnknown() && c.Reason == WorkflowReasonPausing.String()
}

// IsCanceled returns true if the workflow is canceled
func (wfs *WorkflowStatus) IsCanceled() bool {
	c := wfs.GetCondition(WorkflowConditionSucceeded)
	return c != nil && c.IsFalse() && c.Reason == WorkflowReasonCanceled.String()
}

// IsCanceling returns true if the workflow is canceling
func (wfs *WorkflowStatus) IsCanceling() bool {
	c := wfs.GetCondition(WorkflowConditionSucceeded)
	return c != nil && c.IsUnknown() && c.Reason == WorkflowReasonCanceling.String()
}

// IsSucceeded returns true if the workflow is succeeded
func (wfs *WorkflowStatus) IsSucceeded() bool {
	return wfs.GetCondition(WorkflowConditionSucceeded).IsTrue()
}

// IsFailed returns true if the workflow is failed
func (wfs *WorkflowStatus) IsFailed() bool {
	return wfs.GetCondition(WorkflowConditionSucceeded).IsFalse()
}

// IsRunning returns true if the workflow is running
func (wfs *WorkflowStatus) IsRunning() bool {
	return wfs.GetCondition(WorkflowConditionSucceeded).IsUnknown() && !wfs.IsPaused() && !wfs.IsPending()
}

func (wfs *WorkflowStatus) GetConditions() Conditions {
	return wfs.Conditions
}

func (wfs *WorkflowStatus) SetConditions(conditions Conditions) {
	wfs.Conditions = conditions
}

// GetCondition returns the condition with the given type
func (wfs *WorkflowStatus) GetCondition(conditionType ConditionType) *Condition {
	return NewConditionManager(wfs).GetCondition(conditionType)
}

// SetCondition sets the condition with the given type, update LastTransitionTime if the status changes
func (wfs *WorkflowStatus) SetCondition(conditionType ConditionType, status metav1.ConditionStatus, reason, message string) {
	newCondition := Condition{
		Type:    conditionType,
		Status:  status,
		Reason:  reason,
		Message: message,
	}
	if wfs.Conditions == nil ||
		len(wfs.Conditions) == 0 {
		newCondition.LastUpdateTime = metav1.Now()
		newCondition.LastTransitionTime = metav1.Now()
	}
	cm := NewConditionManager(wfs)
	cm.SetCondition(newCondition)
}

// WorkflowTaskStatus defines the observed state of WorkflowTask
type WorkflowTaskStatus struct {
	// Name is the name of the task
	Name string `json:"name"`

	// DisplayName is the display name of the task, used in UI
	DisplayName string `json:"displayName,omitempty"`

	// Labels is the labels of the task
	Labels map[string]string `json:"labels,omitempty"`

	// SuccessCondition is the condition that indicates the task is succeeded
	SuccessCondition *Condition `json:"successCondition,omitempty"`

	// StartedAt is the time when the task is started
	// +optional
	StartedAt *metav1.Time `json:"startedAt,omitempty"`

	// FinishedAt is the time when the task is finished
	// +optional
	FinishedAt *metav1.Time `json:"finishedAt,omitempty"`

	// WorkloadRelease is the release status of the workload
	// +optional
	WorkloadRelease *WorkloadReleaseTaskStatus `json:"workloadRelease,omitempty"`

	// Webhook is the status of webhook task.
	Webhook *WebhookTaskStatus `json:"webhook,omitempty"`
}

func (ws *WorkflowTaskStatus) IsRunning() bool {
	return ws.SuccessCondition.IsUnknown()
}

// IsFailed returns true if the task is failed
func (ws *WorkflowTaskStatus) IsFailed() bool {
	return ws.SuccessCondition.IsFalse()
}

// IsSucceeded returns true if the task is successful
func (ws *WorkflowTaskStatus) IsSucceeded() bool {
	return ws.SuccessCondition.IsTrue()
}

// IsSkipped returns true if the task is skipped
func (ws *WorkflowTaskStatus) IsSkipped() bool {
	// todo: use condition instead of phase to determine the status of the task
	// whether to skip children tasks if the parent task is skipped
	// return ws.Phase == TaskPhaseSkipped
	return false
}

// IsCanceled returns true if the task is canceled
func (ws *WorkflowTaskStatus) IsCanceled() bool {
	c := ws.SuccessCondition
	return c != nil && c.IsFalse() && c.Reason == WorkflowReasonCanceled.String()
}

// IsPaused returns true if the task is suspended
func (ws *WorkflowTaskStatus) IsPaused() bool {
	c := ws.SuccessCondition
	return c != nil && c.IsFalse() && c.Reason == WorkflowReasonPaused.String()
}

// IsProcessing returns true if the task is not ended and not init
func (ws *WorkflowTaskStatus) IsProcessing() bool {
	return ws.IsRunning() || ws.IsPaused()
}

// WorkflowPhase defines the rollout flow phase
type WorkflowPhase string

const (
	// WorkflowPhasePending indicates that the rollout flow is pending
	WorkflowPhasePending WorkflowPhase = "Pending"

	// WorkflowPhaseRunning indicates that the rollout flow is running
	WorkflowPhaseRunning WorkflowPhase = "Running"

	// WorkflowPhaseSucceeded indicates that the rollout flow is succeeded
	WorkflowPhaseSucceeded WorkflowPhase = "Succeeded"

	// WorkflowPhaseFailed indicates that the rollout flow is failed
	WorkflowPhaseFailed WorkflowPhase = "Failed"

	// WorkflowPhaseCanceled indicates that the rollout flow is canceled
	WorkflowPhaseCanceled WorkflowPhase = "Canceled"

	// WorkflowPhaseCanceling indicates that the rollout flow is canceling
	WorkflowPhaseCanceling WorkflowPhase = "Canceling"

	// WorkflowPhasePaused indicates that the rollout flow is paused
	WorkflowPhasePaused WorkflowPhase = "Paused"

	// WorkflowPhaseError indicates that the rollout flow is in error
	WorkflowPhaseError WorkflowPhase = "Error"
)

// SkippedTask defines the skipped task
type SkippedTask struct {
	// Name is the name of the skipped task
	Name string `json:"name"`

	// Reason is the reason why the task is skipped
	// +optional
	Reason SkippedReason `json:"reason,omitempty"`
}

// SkippedReason defines the reason why the task is skipped
type SkippedReason string

const (
	// SkippedReasonWhenExpression indicates that the task is skipped because the when expression is not satisfied
	SkippedReasonWhenExpression SkippedReason = "WhenExpression"

	// SkippedReasonRunAfter indicates that the task is skipped because the task it depends on is skipped
	SkippedReasonRunAfter SkippedReason = "RunAfter"

	// SkippedReasonStopped indicates that the task is skipped because the rollout flow is stopped
	SkippedReasonStopped SkippedReason = "Stopped"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
//+kubebuilder:object:root=true

// WorkflowList contains a list of Workflow
type WorkflowList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Workflow `json:"items"`
}

// HashKey returns the hash key of the workflow task
func (task WorkflowTask) HashKey() string {
	return task.Name
}

// Deps returns the dependencies of the workflow task
func (task WorkflowTask) Deps() []string {
	return task.RunAfter
}

// WorkflowTaskList is a list of WorkflowTask
type WorkflowTaskList []WorkflowTask

// Deps returns the dependencies of the workflow task list
func (l WorkflowTaskList) Deps() map[string][]string {
	deps := make(map[string][]string)
	for _, t := range l {
		deps[t.Name] = t.RunAfter
	}
	return deps
}

// Items returns the items of the workflow task list
func (l WorkflowTaskList) Items() []dag.Task {
	items := make([]dag.Task, len(l))
	for i, t := range l {
		items[i] = t
	}
	return items
}

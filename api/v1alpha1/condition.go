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
	"reflect"
	"sort"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Conditions []Condition

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
	for _, c := range cm.conditionsAccessor.GetConditions() {
		if c.Type != cond.Type {
			conditions = append(conditions, c)
		} else {
			// set LastTransitionTime to the existing condition, if the condition changes, we will update it later
			cond.LastTransitionTime = c.LastTransitionTime
			if reflect.DeepEqual(c, cond) {
				return
			}
		}
	}
	cond.LastTransitionTime = metav1.Now()
	conditions = append(conditions, cond)
	sort.Slice(conditions, func(i, j int) bool {
		return conditions[i].Type < conditions[j].Type
	})
	cm.conditionsAccessor.SetConditions(conditions)
}

// Condition defines the condition of a resource
// See: https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#typical-status-properties
type Condition struct {
	// Type is the type of the condition
	Type ConditionType `json:"type"`
	// Status is the status of the condition
	Status metav1.ConditionStatus `json:"status"`
	// LastTransitionTime is the last time the condition transitioned from one status to another
	// +optional
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty"`
	// Reason is the reason for the condition's last transition
	// +optional
	Reason string `json:"reason,omitempty"`
	// Message is the human-readable message indicating details about last transition
	// +optional
	Message string `json:"message,omitempty"`
}

type ConditionType string

const (
	// ConditionSucceeded specifies that the resource has finished.
	// For resource which run to completion.
	ConditionSucceeded ConditionType = "Succeeded"
)

// ConditionReason defines the reason for the condition's last transition
type ConditionReason string

const (
	// ConditionReasonInitialized indicates that the workflow has just been initialized
	ConditionReasonInitialized ConditionReason = "Initialized"
	// ConditionReasonStarted indicates that the workflow has just started
	ConditionReasonStarted ConditionReason = "Started"
	// ConditionReasonSucceeded indicates that the workflow has succeeded
	ConditionReasonSucceeded ConditionReason = "Succeeded"
	// ConditionReasonFailed indicates that the workflow has failed
	ConditionReasonFailed ConditionReason = "Failed"
	// ConditionReasonCancelled indicates that the workflow has been cancelled
	ConditionReasonCancelled ConditionReason = "Cancelled"
	// ConditionReasonCancelling indicates that the workflow is being cancelled
	ConditionReasonCancelling ConditionReason = "Cancelling"
	// ConditionReasonPending indicates that the workflow is pending
	ConditionReasonPending ConditionReason = "Pending"
	// ConditionReasonRunning indicates that the workflow is running
	ConditionReasonRunning ConditionReason = "Running"
	// ConditionReasonPaused indicates that the workflow has been paused
	ConditionReasonPaused ConditionReason = "Paused"
	// ConditionReasonPausing indicates that the workflow is being paused
	ConditionReasonPausing ConditionReason = "Pausing"

	// ConditionReasonError indicates that the workflow has an error
	ConditionReasonError ConditionReason = "Error"
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

// SetCondition sets or update the condition
func (c *Condition) SetCondition(status metav1.ConditionStatus, reason, message string) {
	if c.Status == status && c.Reason == reason && c.Message == message {
		return
	}
	c.LastTransitionTime = metav1.Now()
	c.Status = status
	c.Message = message
	c.Reason = reason
}

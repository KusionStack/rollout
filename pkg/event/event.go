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

package event

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"

	"kusionstack.io/rollout/apis/workflow/v1alpha1"
)

const (
	// EventReasonInitailized is the reason used for events emitted when a condition is "unknown" and there was no condition before
	EventReasonInitailized = "Initialized"
	// EventReasonSucceeded is the reason used for events emitted when a condition is "true"
	EventReasonSucceeded = "Succeeded"
	// EventReasonFailed is the reason used for events emitted when a condition is "false"
	EventReasonFailed = "Failed"
	// EventReasonError is the reason used for events emitted when there is an error
	EventReasonError = "Error"
)

// Emit emits events for object
func Emit(recorder record.EventRecorder, beforeCondition *v1alpha1.Condition, afterCondition *v1alpha1.Condition, object runtime.Object) {
	// Events that are going to be sent
	//
	// Status "ConditionUnknown":
	//   beforeCondition == nil, emit EventReasonStarted
	//   beforeCondition != nil, emit afterCondition.Reason
	//
	//  Status "ConditionTrue": emit EventReasonSucceded
	//  Status "ConditionFalse": emit EventReasonFailed
	if !equality.Semantic.DeepEqual(beforeCondition, afterCondition) && afterCondition != nil {
		// If the condition changed, and the target condition is not empty, we send an event
		switch afterCondition.Status {
		case metav1.ConditionTrue:
			recorder.Event(object, corev1.EventTypeNormal, EventReasonSucceeded, afterCondition.Message)
		case metav1.ConditionFalse:
			recorder.Event(object, corev1.EventTypeWarning, EventReasonFailed, afterCondition.Message)
		case metav1.ConditionUnknown:
			if beforeCondition == nil {
				// If the condition changed, the status is "unknown", and there was no condition before,
				// we emit the "Initialized event". We ignore further updates of the "unknown" status.
				recorder.Event(object, corev1.EventTypeNormal, EventReasonInitailized, "")
			} else {
				// If the condition changed, the status is "unknown", and there was a condition before,
				// we emit an event that matches the reason and message of the condition.
				// This is used for instance to signal the transition from "started" to "running"
				recorder.Event(object, corev1.EventTypeNormal, afterCondition.Reason, afterCondition.Message)
			}
		}
	}
}

// EmitError emits an error event for object
func EmitError(recorder record.EventRecorder, err error, object runtime.Object) {
	recorder.Event(object, corev1.EventTypeWarning, EventReasonError, err.Error())
}

/**
 * Copyright 2024 The KusionStack Authors
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

package executor

import (
	"context"
	"sync"

	"github.com/elliotchance/pie/v2"
	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	rolloutv1alpha1 "kusionstack.io/rollout/apis/rollout/v1alpha1"
	"kusionstack.io/rollout/pkg/workload"
)

// ExecutorContext context of rolloutRun
type ExecutorContext struct {
	context.Context
	once sync.Once

	Rollout    *rolloutv1alpha1.Rollout
	RolloutRun *rolloutv1alpha1.RolloutRun
	NewStatus  *rolloutv1alpha1.RolloutRunStatus
	Workloads  *workload.Set
}

func (c *ExecutorContext) Initialize() {
	c.once.Do(func() {
		if c.NewStatus == nil {
			c.NewStatus = c.RolloutRun.Status.DeepCopy()
		}
		newStatus := c.NewStatus

		if len(newStatus.Phase) == 0 {
			newStatus.Phase = rolloutv1alpha1.RolloutRunPhaseInitial
		}

		if c.RolloutRun.Spec.Canary != nil && newStatus.CanaryStatus == nil {
			newStatus.CanaryStatus = &rolloutv1alpha1.RolloutRunBatchStatusRecord{
				State: rolloutv1alpha1.BatchStepStatePending,
			}
		}
		// init BatchStatus
		if c.RolloutRun.Spec.Batch != nil {
			if newStatus.BatchStatus == nil {
				newStatus.BatchStatus = &rolloutv1alpha1.RolloutRunBatchStatus{}
			}
			// resize records
			specBatches := len(c.RolloutRun.Spec.Batch.Batches)
			statusBatches := len(newStatus.BatchStatus.Records)
			if specBatches > statusBatches {
				for i := 0; i < specBatches-statusBatches; i++ {
					newStatus.BatchStatus.Records = append(
						newStatus.BatchStatus.Records,
						rolloutv1alpha1.RolloutRunBatchStatusRecord{
							Index: ptr.To(int32(statusBatches + i)),
							State: rolloutv1alpha1.BatchStepStatePending,
						},
					)
				}
			} else if specBatches < statusBatches {
				newStatus.BatchStatus.Records = newStatus.BatchStatus.Records[:specBatches]
			}
		}
	})
}

// filterWebhooks return webhooks met hookType
func filterWebhooks(hookType rolloutv1alpha1.HookType, rolloutRun *rolloutv1alpha1.RolloutRun) []rolloutv1alpha1.RolloutWebhook {
	return pie.Filter(rolloutRun.Spec.Webhooks, func(w rolloutv1alpha1.RolloutWebhook) bool {
		for _, item := range w.HookTypes {
			if item == hookType {
				return true
			}
		}
		return false
	})
}

func (c *ExecutorContext) GetWebhooksAndLatestStatusBy(hookType rolloutv1alpha1.HookType) ([]rolloutv1alpha1.RolloutWebhook, *rolloutv1alpha1.BatchWebhookStatus) {
	c.Initialize()

	run := c.RolloutRun
	newStatus := c.NewStatus
	webhooks := filterWebhooks(hookType, run)
	if len(webhooks) == 0 {
		// no webhooks
		return nil, nil
	}
	var webhookStatuses []rolloutv1alpha1.BatchWebhookStatus
	if c.inCanary() {
		webhookStatuses = newStatus.CanaryStatus.Webhooks
	} else {
		index := newStatus.BatchStatus.CurrentBatchIndex
		webhookStatuses = newStatus.BatchStatus.Records[index].Webhooks
	}
	var status *rolloutv1alpha1.BatchWebhookStatus
	if len(webhookStatuses) > 0 {
		latestStatus := &webhookStatuses[len(webhookStatuses)-1]
		if latestStatus.HookType == hookType {
			status = latestStatus
		}
	}
	return webhooks, status
}

func (c *ExecutorContext) SetWebhookStatus(status rolloutv1alpha1.BatchWebhookStatus) {
	c.Initialize()

	newStatus := c.NewStatus
	if c.inCanary() {
		newStatus.CanaryStatus.Webhooks = appendWebhookStatus(newStatus.CanaryStatus.Webhooks, status)
	} else {
		index := newStatus.BatchStatus.CurrentBatchIndex
		newStatus.BatchStatus.Records[index].Webhooks = appendWebhookStatus(newStatus.BatchStatus.Records[index].Webhooks, status)
	}
}

func isFinalStepState(state rolloutv1alpha1.RolloutBatchStepState) bool {
	return state == rolloutv1alpha1.BatchStepStateSucceeded || state == rolloutv1alpha1.BatchStepStateCanceled
}

func (c *ExecutorContext) MoveToNextState(nextState rolloutv1alpha1.RolloutBatchStepState) {
	c.Initialize()

	newStatus := c.NewStatus
	if c.inCanary() {
		newStatus.CanaryStatus.State = nextState
		if isFinalStepState(nextState) {
			newStatus.CanaryStatus.FinishTime = ptr.To(metav1.Now())
		}
	} else {
		index := newStatus.BatchStatus.CurrentBatchIndex
		newStatus.BatchStatus.CurrentBatchState = nextState
		newStatus.BatchStatus.Records[index].State = nextState
		if isFinalStepState(nextState) {
			newStatus.BatchStatus.Records[index].FinishTime = ptr.To(metav1.Now())
		}
	}
}

func appendWebhookStatus(origin []rolloutv1alpha1.BatchWebhookStatus, input rolloutv1alpha1.BatchWebhookStatus) []rolloutv1alpha1.BatchWebhookStatus {
	length := len(origin)
	if length == 0 {
		return []rolloutv1alpha1.BatchWebhookStatus{input}
	}

	if origin[length-1].HookType == input.HookType && origin[length-1].Name == input.Name {
		origin[length-1] = input
	} else {
		origin = append(origin, input)
	}
	return origin
}

func (r *ExecutorContext) inCanary() bool {
	r.Initialize()
	run := r.RolloutRun
	newStatus := r.NewStatus
	if run.Spec.Canary != nil {
		canaryStatus := newStatus.CanaryStatus
		if canaryStatus == nil {
			return true
		}
		if canaryStatus.State == rolloutv1alpha1.BatchStepStateCanceled ||
			canaryStatus.State == rolloutv1alpha1.BatchStepStateSucceeded {
			return false
		}
		return true
	}
	return false
}

func (r *ExecutorContext) makeRolloutWebhookReview(hookType rolloutv1alpha1.HookType, webhook rolloutv1alpha1.RolloutWebhook) rolloutv1alpha1.RolloutWebhookReview {
	r.Initialize()

	rollout := r.Rollout
	rolloutRun := r.RolloutRun
	newStatus := r.NewStatus

	review := rolloutv1alpha1.RolloutWebhookReview{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: rollout.Namespace,
			Name:      webhook.Name,
		},
		Spec: rolloutv1alpha1.RolloutWebhookReviewSpec{
			RolloutName: rollout.Name,
			RolloutID:   rolloutRun.Name,
			HookType:    hookType,
			Properties:  webhook.Properties,
			TargetType:  rolloutRun.Spec.TargetType,
		},
	}

	if r.inCanary() {
		review.Spec.Canary = &rolloutv1alpha1.RolloutWebhookReviewCanary{
			Targets:    rolloutRun.Spec.Canary.Targets,
			Properties: rolloutRun.Spec.Canary.Properties,
		}
	} else {
		review.Spec.Batch = &rolloutv1alpha1.RolloutWebhookReviewBatch{
			BatchIndex: newStatus.BatchStatus.CurrentBatchIndex,
			Targets:    rolloutRun.Spec.Batch.Batches[newStatus.BatchStatus.CurrentBatchIndex].Targets,
			Properties: rolloutRun.Spec.Batch.Batches[newStatus.BatchStatus.CurrentBatchIndex].Properties,
		}
	}

	return review
}

func (e *ExecutorContext) loggerWithContext(logger logr.Logger) logr.Logger {
	e.Initialize()
	return logger.WithValues(
		"namespace", e.Rollout.Namespace,
		"rollout", e.Rollout.Name,
		"rolloutRun", e.RolloutRun.Name,
	)
}

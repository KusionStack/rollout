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
	"slices"
	"sync"

	"github.com/go-logr/logr"
	"github.com/samber/lo"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	rolloutv1alpha1 "kusionstack.io/kube-api/rollout/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	rorexecutor "kusionstack.io/rollout/pkg/controllers/rolloutrun/executor"
	"kusionstack.io/rollout/pkg/workload"
)

// ExecutorContext context of scaleRun
type ExecutorContext struct {
	context.Context
	once     sync.Once
	Client   client.Client
	Recorder record.EventRecorder

	Accessor  workload.Accessor
	ScaleRun  *rolloutv1alpha1.ScaleRun
	NewStatus *rolloutv1alpha1.ScaleRunStatus
	Workloads *workload.Set
}

func (c *ExecutorContext) Initialize() {
	c.once.Do(func() {
		if c.NewStatus == nil {
			c.NewStatus = c.ScaleRun.Status.DeepCopy()
		}
		newStatus := c.NewStatus
		newStatus.ObservedGeneration = c.ScaleRun.Generation

		if len(newStatus.Phase) == 0 {
			newStatus.Phase = rolloutv1alpha1.RolloutRunPhaseInitial
		}

		// init BatchStatus
		if c.ScaleRun.Spec.Batch != nil {
			if newStatus.Batches == nil {
				newStatus.Batches = &rolloutv1alpha1.ScaleRunBatchStatus{}
			}
			// resize records
			specBatchSize := len(c.ScaleRun.Spec.Batch.Batches)
			statusBatchSize := len(newStatus.Batches.Records)
			if specBatchSize > statusBatchSize {
				for i := 0; i < specBatchSize-statusBatchSize; i++ {
					newStatus.Batches.Records = append(newStatus.Batches.Records,
						rolloutv1alpha1.ScaleRunStepStatus{
							Index: ptr.To(int32(statusBatchSize + i)),
							State: rorexecutor.StepNone,
						},
					)
				}
			} else if specBatchSize < statusBatchSize {
				newStatus.Batches.Records = newStatus.Batches.Records[:specBatchSize]
			}
		}
	})
}

// filterWebhooks return webhooks met hookType
func filterWebhooks(hookType rolloutv1alpha1.HookType, scaleRun *rolloutv1alpha1.ScaleRun) []rolloutv1alpha1.RolloutWebhook {
	return lo.Filter(scaleRun.Spec.Webhooks, func(w rolloutv1alpha1.RolloutWebhook, _ int) bool {
		return slices.Contains(w.HookTypes, hookType)
	})
}

func (c *ExecutorContext) GetWebhooksAndLatestStatusBy(hookType rolloutv1alpha1.HookType) ([]rolloutv1alpha1.RolloutWebhook, *rolloutv1alpha1.RolloutWebhookStatus) {
	c.Initialize()

	run := c.ScaleRun
	newStatus := c.NewStatus
	webhooks := filterWebhooks(hookType, run)
	if len(webhooks) == 0 {
		// no webhooks
		return nil, nil
	}
	var webhookStatuses []rolloutv1alpha1.RolloutWebhookStatus
	index := newStatus.Batches.CurrentBatchIndex
	webhookStatuses = newStatus.Batches.Records[index].Webhooks
	var status *rolloutv1alpha1.RolloutWebhookStatus
	if len(webhookStatuses) > 0 {
		latestStatus := &webhookStatuses[len(webhookStatuses)-1]
		if latestStatus.HookType == hookType {
			status = latestStatus
		}
	}
	return webhooks, status
}

func (c *ExecutorContext) SetWebhookStatus(status rolloutv1alpha1.RolloutWebhookStatus) {
	c.Initialize()

	newStatus := c.NewStatus
	index := newStatus.Batches.CurrentBatchIndex
	newStatus.Batches.Records[index].Webhooks = appendWebhookStatus(newStatus.Batches.Records[index].Webhooks, status)
}

func isFinalStepState(state rolloutv1alpha1.RolloutStepState) bool {
	return state == rorexecutor.StepSucceeded
}

func (c *ExecutorContext) GetCurrentState() (string, rolloutv1alpha1.RolloutStepState) {
	return "batch", c.NewStatus.Batches.CurrentBatchState
}

func (c *ExecutorContext) MoveToNextState(nextState rolloutv1alpha1.RolloutStepState) {
	c.Initialize()

	newStatus := c.NewStatus
	index := newStatus.Batches.CurrentBatchIndex
	newStatus.Batches.CurrentBatchState = nextState
	newStatus.Batches.Records[index].State = nextState
	if nextState == rorexecutor.StepPreBatchStepHook {
		newStatus.Batches.Records[index].StartTime = ptr.To(metav1.Now())
	} else if isFinalStepState(nextState) {
		newStatus.Batches.Records[index].FinishTime = ptr.To(metav1.Now())
	}
}

func (c *ExecutorContext) SkipCurrentRelease() {
	c.Initialize()

	newStatus := c.NewStatus
	newStatus.Batches.CurrentBatchIndex = int32(len(c.ScaleRun.Spec.Batch.Batches) - 1)
	newStatus.Batches.CurrentBatchState = rorexecutor.StepSucceeded
	for i := range newStatus.Batches.Records {
		if newStatus.Batches.Records[i].State == rorexecutor.StepNone ||
			newStatus.Batches.Records[i].State == rorexecutor.StepPending {
			newStatus.Batches.Records[i].State = rorexecutor.StepSucceeded
		}
		if newStatus.Batches.Records[i].StartTime == nil {
			newStatus.Batches.Records[i].StartTime = ptr.To(metav1.Now())
		}
		if newStatus.Batches.Records[i].FinishTime == nil {
			newStatus.Batches.Records[i].FinishTime = ptr.To(metav1.Now())
		}
	}
}

func (c *ExecutorContext) Pause() {
	c.Initialize()
	c.NewStatus.Phase = rolloutv1alpha1.RolloutRunPhasePaused
}

func (c *ExecutorContext) Fail(err error) {
	c.Initialize()
	//nolint:errorlint
	crm, ok := err.(*rolloutv1alpha1.CodeReasonMessage)
	if ok {
		c.NewStatus.Error = crm
	} else {
		c.NewStatus.Error = &rolloutv1alpha1.CodeReasonMessage{
			Code:    "Error",
			Reason:  "ExecutorFailed",
			Message: err.Error(),
		}
	}
}

func (c *ExecutorContext) MoveToNextStateIfMatch(curState, nextState rolloutv1alpha1.RolloutStepState) {
	c.Initialize()
	_, state := c.GetCurrentState()
	if state == curState {
		c.MoveToNextState(nextState)
	}
}

func appendWebhookStatus(origin []rolloutv1alpha1.RolloutWebhookStatus, input rolloutv1alpha1.RolloutWebhookStatus) []rolloutv1alpha1.RolloutWebhookStatus {
	length := len(origin)
	if length == 0 {
		return []rolloutv1alpha1.RolloutWebhookStatus{input}
	}

	if origin[length-1].HookType == input.HookType && origin[length-1].Name == input.Name {
		origin[length-1] = input
	} else {
		origin = append(origin, input)
	}
	return origin
}

func (r *ExecutorContext) makeRolloutWebhookReview(hookType rolloutv1alpha1.HookType, webhook rolloutv1alpha1.RolloutWebhook) rolloutv1alpha1.RolloutWebhookReview {
	r.Initialize()

	scaleRun := r.ScaleRun
	newStatus := r.NewStatus

	review := rolloutv1alpha1.RolloutWebhookReview{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: scaleRun.Namespace,
			Name:      webhook.Name,
		},
		Spec: rolloutv1alpha1.RolloutWebhookReviewSpec{
			RolloutID:   scaleRun.Name,
			HookType:    hookType,
			Properties:  webhook.Properties,
			TargetType:  scaleRun.Spec.TargetType,
		},
	}

	review.Spec.Scale = &rolloutv1alpha1.ScaleWebhookReviewBatch{
		BatchIndex: newStatus.Batches.CurrentBatchIndex,
		Targets:    scaleRun.Spec.Batch.Batches[newStatus.Batches.CurrentBatchIndex].Targets,
		Properties: scaleRun.Spec.Batch.Batches[newStatus.Batches.CurrentBatchIndex].Properties,
	}

	return review
}

func (e *ExecutorContext) WithLogger(logger logr.Logger) logr.Logger {
	l := logger.WithValues(
		"namespace", e.ScaleRun.Namespace,
		"scaleRun", e.ScaleRun.Name,
	)

	e.Context = logr.NewContext(e.Context, l)
	return l
}

func (e *ExecutorContext) GetLogger() logr.Logger {
	return logr.FromContextOrDiscard(e.Context)
}

func (e *ExecutorContext) GetBatchLogger() logr.Logger {
	e.Initialize()
	l := e.GetLogger().WithValues("step", "batch")
	if e.NewStatus != nil && e.NewStatus.Batches != nil {
		l = l.WithValues("batchIndex", e.NewStatus.Batches.CurrentBatchIndex)
	}
	return l
}

func (e *ExecutorContext) GetCanaryLogger() logr.Logger {
	return e.GetLogger().WithValues("step", "canary")
}

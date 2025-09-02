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

	trafficcontrol "kusionstack.io/rollout/pkg/trafficrouting/control"
	"kusionstack.io/rollout/pkg/workload"
)

// ExecutorContext context of rolloutRun
type ExecutorContext struct {
	context.Context
	once     sync.Once
	Client   client.Client
	Recorder record.EventRecorder

	Accessor       workload.Accessor
	OwnerKind      string
	OwnerName      string
	RolloutRun     *rolloutv1alpha1.RolloutRun
	NewStatus      *rolloutv1alpha1.RolloutRunStatus
	Workloads      *workload.Set
	TrafficManager *trafficcontrol.Manager
}

func (c *ExecutorContext) Initialize() {
	c.once.Do(func() {
		if c.NewStatus == nil {
			c.NewStatus = c.RolloutRun.Status.DeepCopy()
		}
		newStatus := c.NewStatus
		newStatus.ObservedGeneration = c.RolloutRun.Generation

		if len(newStatus.Phase) == 0 {
			newStatus.Phase = rolloutv1alpha1.RolloutRunPhaseInitial
		}

		// init canary status
		if c.RolloutRun.Spec.Canary != nil && newStatus.CanaryStatus == nil {
			newStatus.CanaryStatus = &rolloutv1alpha1.RolloutRunStepStatus{}
		}
		// init BatchStatus
		if c.RolloutRun.Spec.Batch != nil {
			if newStatus.BatchStatus == nil {
				newStatus.BatchStatus = &rolloutv1alpha1.RolloutRunBatchStatus{}
			}
			// resize records
			specBatchSize := len(c.RolloutRun.Spec.Batch.Batches)
			statusBatchSize := len(newStatus.BatchStatus.Records)
			if specBatchSize > statusBatchSize {
				for i := 0; i < specBatchSize-statusBatchSize; i++ {
					newStatus.BatchStatus.Records = append(newStatus.BatchStatus.Records,
						rolloutv1alpha1.RolloutRunStepStatus{
							Index: ptr.To(int32(statusBatchSize + i)),
							State: StepNone,
						},
					)
				}
			} else if specBatchSize < statusBatchSize {
				newStatus.BatchStatus.Records = newStatus.BatchStatus.Records[:specBatchSize]
			}
		}

		// init RollbackStatus
		if c.RolloutRun.Spec.Rollback != nil {
			if newStatus.RollbackStatus == nil {
				newStatus.RollbackStatus = &rolloutv1alpha1.RolloutRunBatchStatus{}
			}
			// resize records
			specBatchSize := len(c.RolloutRun.Spec.Rollback.Batches)
			statusBatchSize := len(newStatus.RollbackStatus.Records)
			if specBatchSize > statusBatchSize {
				for i := 0; i < specBatchSize-statusBatchSize; i++ {
					newStatus.RollbackStatus.Records = append(newStatus.RollbackStatus.Records,
						rolloutv1alpha1.RolloutRunStepStatus{
							Index: ptr.To(int32(statusBatchSize + i)),
							State: StepNone,
						},
					)
				}
			} else if specBatchSize < statusBatchSize {
				newStatus.RollbackStatus.Records = newStatus.RollbackStatus.Records[:specBatchSize]
			}
		}
	})
}

// filterWebhooks return webhooks met hookType
func filterWebhooks(hookType rolloutv1alpha1.HookType, rolloutRun *rolloutv1alpha1.RolloutRun) []rolloutv1alpha1.RolloutWebhook {
	return lo.Filter(rolloutRun.Spec.Webhooks, func(w rolloutv1alpha1.RolloutWebhook, _ int) bool {
		return slices.Contains(w.HookTypes, hookType)
	})
}

func (c *ExecutorContext) GetWebhooksAndLatestStatusBy(hookType rolloutv1alpha1.HookType) ([]rolloutv1alpha1.RolloutWebhook, *rolloutv1alpha1.RolloutWebhookStatus) {
	c.Initialize()

	run := c.RolloutRun
	newStatus := c.NewStatus
	webhooks := filterWebhooks(hookType, run)
	if len(webhooks) == 0 {
		// no webhooks
		return nil, nil
	}
	var webhookStatuses []rolloutv1alpha1.RolloutWebhookStatus
	if c.inCanary() {
		webhookStatuses = newStatus.CanaryStatus.Webhooks
	} else if c.inRollback() {
		index := newStatus.RollbackStatus.CurrentBatchIndex
		webhookStatuses = newStatus.RollbackStatus.Records[index].Webhooks
	} else {
		index := newStatus.BatchStatus.CurrentBatchIndex
		webhookStatuses = newStatus.BatchStatus.Records[index].Webhooks
	}
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
	if c.inCanary() {
		newStatus.CanaryStatus.Webhooks = appendWebhookStatus(newStatus.CanaryStatus.Webhooks, status)
	} else if c.inRollback() {
		index := newStatus.RollbackStatus.CurrentBatchIndex
		newStatus.RollbackStatus.Records[index].Webhooks = appendWebhookStatus(newStatus.RollbackStatus.Records[index].Webhooks, status)
	} else {
		index := newStatus.BatchStatus.CurrentBatchIndex
		newStatus.BatchStatus.Records[index].Webhooks = appendWebhookStatus(newStatus.BatchStatus.Records[index].Webhooks, status)
	}
}

func isFinalStepState(state rolloutv1alpha1.RolloutStepState) bool {
	return state == StepSucceeded
}

func (c *ExecutorContext) GetCurrentState() (string, rolloutv1alpha1.RolloutStepState) {
	if c.inCanary() {
		return "canary", c.NewStatus.CanaryStatus.State
	} else if c.inRollback() {
		return "rollback", c.NewStatus.RollbackStatus.CurrentBatchState
	} else {
		return "batch", c.NewStatus.BatchStatus.CurrentBatchState
	}
}

func (c *ExecutorContext) MoveToNextState(nextState rolloutv1alpha1.RolloutStepState) {
	c.Initialize()

	newStatus := c.NewStatus
	if c.inRollback() {
		index := newStatus.RollbackStatus.CurrentBatchIndex
		newStatus.RollbackStatus.CurrentBatchState = nextState
		newStatus.RollbackStatus.Records[index].State = nextState
		if nextState == StepPreRollbackStepHook {
			newStatus.RollbackStatus.Records[index].StartTime = ptr.To(metav1.Now())
		} else if isFinalStepState(nextState) {
			newStatus.RollbackStatus.Records[index].FinishTime = ptr.To(metav1.Now())
		}
	} else if c.inCanary() {
		newStatus.CanaryStatus.State = nextState
		if nextState == StepPreCanaryStepHook {
			newStatus.CanaryStatus.StartTime = ptr.To(metav1.Now())
		} else if isFinalStepState(nextState) {
			newStatus.CanaryStatus.FinishTime = ptr.To(metav1.Now())
		}
	} else {
		index := newStatus.BatchStatus.CurrentBatchIndex
		newStatus.BatchStatus.CurrentBatchState = nextState
		newStatus.BatchStatus.Records[index].State = nextState
		if nextState == StepPreBatchStepHook {
			newStatus.BatchStatus.Records[index].StartTime = ptr.To(metav1.Now())
		} else if isFinalStepState(nextState) {
			newStatus.BatchStatus.Records[index].FinishTime = ptr.To(metav1.Now())
		}
	}
}

func (c *ExecutorContext) SkipCurrentRelease() {
	c.Initialize()

	newStatus := c.NewStatus
	if c.inCanary() {
		newStatus.CanaryStatus.State = StepSucceeded
		if newStatus.CanaryStatus.StartTime == nil {
			newStatus.CanaryStatus.StartTime = ptr.To(metav1.Now())
		}
		newStatus.CanaryStatus.FinishTime = ptr.To(metav1.Now())
	} else if c.inRollback() {
		newStatus.RollbackStatus.CurrentBatchIndex = int32(len(c.RolloutRun.Spec.Rollback.Batches) - 1)
		newStatus.RollbackStatus.CurrentBatchState = StepSucceeded
		for i := range newStatus.RollbackStatus.Records {
			if newStatus.RollbackStatus.Records[i].State == StepNone ||
				newStatus.RollbackStatus.Records[i].State == StepPending {
				newStatus.RollbackStatus.Records[i].State = StepSucceeded
			}
			if newStatus.RollbackStatus.Records[i].StartTime == nil {
				newStatus.RollbackStatus.Records[i].StartTime = ptr.To(metav1.Now())
			}
			if newStatus.RollbackStatus.Records[i].FinishTime == nil {
				newStatus.RollbackStatus.Records[i].FinishTime = ptr.To(metav1.Now())
			}
		}
	} else {
		newStatus.BatchStatus.CurrentBatchIndex = int32(len(c.RolloutRun.Spec.Batch.Batches) - 1)
		newStatus.BatchStatus.CurrentBatchState = StepSucceeded
		for i := range newStatus.BatchStatus.Records {
			if newStatus.BatchStatus.Records[i].State == StepNone ||
				newStatus.BatchStatus.Records[i].State == StepPending {
				newStatus.BatchStatus.Records[i].State = StepSucceeded
			}
			if newStatus.BatchStatus.Records[i].StartTime == nil {
				newStatus.BatchStatus.Records[i].StartTime = ptr.To(metav1.Now())
			}
			if newStatus.BatchStatus.Records[i].FinishTime == nil {
				newStatus.BatchStatus.Records[i].FinishTime = ptr.To(metav1.Now())
			}
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

func (r *ExecutorContext) inCanary() bool {
	r.Initialize()
	run := r.RolloutRun
	newStatus := r.NewStatus
	if run.Spec.Canary != nil {
		canaryStatus := newStatus.CanaryStatus
		if canaryStatus == nil {
			return true
		}

		if isFinalStepState(canaryStatus.State) {
			return false
		}
		return true
	}
	return false
}

func (r *ExecutorContext) inBatchGray() bool {
	// todo: need to consider case of every batch gray
	r.Initialize()
	run := r.RolloutRun
	newStatus := r.NewStatus
	if newStatus.BatchStatus == nil {
		return false
	}

	currentBatchIndex := newStatus.BatchStatus.CurrentBatchIndex
	currentBatch := run.Spec.Batch.Batches[currentBatchIndex]
	if currentBatch.Traffic == nil || int(currentBatchIndex+1) == len(run.Spec.Batch.Batches) {
		return false
	}

	currentBatchState := newStatus.BatchStatus.CurrentBatchState
	if !isFinalStepState(currentBatchState) {
		return true
	}

	if int(currentBatchIndex+1) < len(run.Spec.Batch.Batches) {
		nextBatch := run.Spec.Batch.Batches[currentBatchIndex+1]
		if nextBatch.Traffic != nil {
			return true
		}
	}

	return false
}

func (r *ExecutorContext) inRollback() bool {
	r.Initialize()
	run := r.RolloutRun
	newStatus := r.NewStatus
	if newStatus.RollbackStatus == nil {
		return false
	}
	if run.Spec.Rollback != nil {
		if newStatus.Phase != rolloutv1alpha1.RolloutRunPhaseRollbacked {
			return true
		}
	}
	return false
}

func (r *ExecutorContext) makeRolloutWebhookReview(hookType rolloutv1alpha1.HookType, webhook rolloutv1alpha1.RolloutWebhook) rolloutv1alpha1.RolloutWebhookReview {
	r.Initialize()

	rolloutRun := r.RolloutRun
	newStatus := r.NewStatus

	review := rolloutv1alpha1.RolloutWebhookReview{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: rolloutRun.Namespace,
			Name:      webhook.Name,
		},
		Spec: rolloutv1alpha1.RolloutWebhookReviewSpec{
			Kind:        r.OwnerKind,
			RolloutName: r.OwnerName,
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
	} else if r.inRollback() {
		review.Spec.Rollback = &rolloutv1alpha1.RolloutWebhookReviewBatch{
			BatchIndex: newStatus.RollbackStatus.CurrentBatchIndex,
			Targets:    rolloutRun.Spec.Rollback.Batches[newStatus.RollbackStatus.CurrentBatchIndex].Targets,
			Properties: rolloutRun.Spec.Rollback.Batches[newStatus.RollbackStatus.CurrentBatchIndex].Properties,
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

func (e *ExecutorContext) WithLogger(logger logr.Logger) logr.Logger {
	l := logger.WithValues(
		"namespace", e.RolloutRun.Namespace,
		"rollout", e.OwnerName,
		"rolloutRun", e.RolloutRun.Name,
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
	if e.NewStatus != nil && e.NewStatus.BatchStatus != nil {
		l = l.WithValues("batchIndex", e.NewStatus.BatchStatus.CurrentBatchIndex)
	}
	return l
}

func (e *ExecutorContext) GetCanaryLogger() logr.Logger {
	return e.GetLogger().WithValues("step", "canary")
}

func (e *ExecutorContext) GetRollbackLogger() logr.Logger {
	e.Initialize()
	l := e.GetLogger().WithValues("step", "rollback")
	if e.NewStatus != nil && e.NewStatus.RollbackStatus != nil {
		l = l.WithValues("rollbackIndex", e.NewStatus.RollbackStatus.CurrentBatchIndex)
	}
	return l
}

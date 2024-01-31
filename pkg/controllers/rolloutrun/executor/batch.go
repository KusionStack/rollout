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
	"time"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"

	rolloutv1alpha1 "kusionstack.io/rollout/apis/rollout/v1alpha1"
)

type batchExecutor struct {
	logger  logr.Logger
	webhook webhookExecutor
}

func newBatchExecutor(logger logr.Logger, webhook webhookExecutor) *batchExecutor {
	return &batchExecutor{
		logger:  logger,
		webhook: webhook,
	}
}

func (e *batchExecutor) loggerWithContext(executorContext *ExecutorContext) logr.Logger {
	return executorContext.loggerWithContext(e.logger).WithValues(
		"batchIndex", executorContext.NewStatus.BatchStatus.CurrentBatchIndex,
	)
}

func (e *batchExecutor) Do(ctx *ExecutorContext) (done bool, result ctrl.Result, err error) {
	newStatus := ctx.NewStatus
	rolloutRun := ctx.RolloutRun

	result = ctrl.Result{Requeue: true}

	switch newStatus.BatchStatus.CurrentBatchState {
	case rolloutv1alpha1.RolloutStepPending:
		e.doBatchInitial(ctx)
	case rolloutv1alpha1.RolloutStepPaused:
		// do nothing
		result.Requeue = false
	case rolloutv1alpha1.RolloutStepPreBatchStepHook:
		var webhookDone bool
		webhookDone, result, err = e.webhook.Do(ctx, rolloutv1alpha1.PreBatchStepHook)
		if webhookDone {
			ctx.MoveToNextState(rolloutv1alpha1.RolloutStepRunning)
		}
	case rolloutv1alpha1.RolloutStepRunning:
		result, err = e.doBatchUpgrading(ctx)
	case rolloutv1alpha1.RolloutStepPostBatchStepHook:
		var webhookDone bool
		webhookDone, result, err = e.webhook.Do(ctx, rolloutv1alpha1.PostBatchStepHook)
		if webhookDone {
			ctx.MoveToNextState(rolloutv1alpha1.RolloutStepSucceeded)
		}
	case rolloutv1alpha1.RolloutStepSucceeded:
		e.doBatchSucceeded(ctx)
		if newStatus.BatchStatus.CurrentBatchState == rolloutv1alpha1.RolloutStepSucceeded &&
			int(newStatus.BatchStatus.CurrentBatchIndex) >= (len(rolloutRun.Spec.Batch.Batches)-1) {
			done = true
		}
	case rolloutv1alpha1.RolloutStepCanceled:
		done = true
	}
	return done, result, err
}

// doBatchInitial process Initialized
func (e *batchExecutor) doBatchInitial(ctx *ExecutorContext) {
	newBatchStatus := ctx.NewStatus.BatchStatus
	currentBatchIndex := newBatchStatus.CurrentBatchIndex

	if ctx.RolloutRun.Spec.Batch.Batches[currentBatchIndex].Breakpoint {
		newBatchStatus.CurrentBatchState = rolloutv1alpha1.RolloutStepPaused
		newBatchStatus.Records[currentBatchIndex].State = newBatchStatus.CurrentBatchState
	} else {
		newBatchStatus.RolloutBatchStatus.CurrentBatchState = rolloutv1alpha1.RolloutStepPreBatchStepHook
		if newBatchStatus.Records[currentBatchIndex].StartTime == nil {
			newBatchStatus.Records[currentBatchIndex].StartTime = &metav1.Time{Time: time.Now()}
		}
		newBatchStatus.Records[currentBatchIndex].State = newBatchStatus.CurrentBatchState
	}
}

// doBatchSucceeded process succeeded state
func (e *batchExecutor) doBatchSucceeded(ctx *ExecutorContext) {
	newBatchStatus := ctx.NewStatus.BatchStatus
	currentBatchIndex := newBatchStatus.CurrentBatchIndex

	if int(currentBatchIndex+1) < len(ctx.RolloutRun.Spec.Batch.Batches) {
		newBatchStatus.CurrentBatchState = rolloutv1alpha1.RolloutStepPending
		newBatchStatus.CurrentBatchIndex = currentBatchIndex + 1
		if int(newBatchStatus.CurrentBatchIndex) < len(newBatchStatus.Records) {
			newBatchStatus.Records[newBatchStatus.CurrentBatchIndex] = rolloutv1alpha1.RolloutRunStepStatus{
				Index: ptr.To(newBatchStatus.CurrentBatchIndex),
				State: rolloutv1alpha1.RolloutStepPending,
			}
		} else {
			newBatchStatus.Records = append(newBatchStatus.Records, rolloutv1alpha1.RolloutRunStepStatus{
				Index: ptr.To(newBatchStatus.CurrentBatchIndex),
				State: rolloutv1alpha1.RolloutStepPending,
			})
		}
	}
}

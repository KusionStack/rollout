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
	"fmt"
	"time"

	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"

	rolloutv1alpha1 "kusionstack.io/rollout/apis/rollout/v1alpha1"
	"kusionstack.io/rollout/pkg/controllers/rolloutrun/control"
	"kusionstack.io/rollout/pkg/workload"
)

type batchExecutor struct {
	logger       logr.Logger
	webhook      webhookExecutor
	stateMachine *stepStateMachine
}

func newBatchExecutor(logger logr.Logger, webhook webhookExecutor) *batchExecutor {
	e := &batchExecutor{
		logger:       logger,
		webhook:      webhook,
		stateMachine: newStepStateMachine(),
	}

	e.stateMachine.add(StepNone, StepPending, e.doPausing)
	e.stateMachine.add(StepPending, StepPreBatchStepHook, skipStep)
	e.stateMachine.add(StepPreBatchStepHook, StepRunning, e.doPreStepHook)
	e.stateMachine.add(StepRunning, StepPostBatchStepHook, e.doBatchUpgrading)
	e.stateMachine.add(StepPostBatchStepHook, StepSucceeded, e.doPostStepHook)
	e.stateMachine.add(StepSucceeded, "", skipStep)
	return e
}

func (e *batchExecutor) loggerWithContext(executorContext *ExecutorContext) logr.Logger {
	return executorContext.loggerWithContext(e.logger).WithValues(
		"step", "batch",
		"batchIndex", executorContext.NewStatus.BatchStatus.CurrentBatchIndex,
	)
}

func (e *batchExecutor) Do(ctx *ExecutorContext) (done bool, result ctrl.Result, err error) {
	newStatus := ctx.NewStatus
	currentState := newStatus.BatchStatus.CurrentBatchState

	stepDone, result, err := e.stateMachine.do(ctx, currentState)

	if stepDone {
		// move to next batch
		currentBatchIndex := newStatus.BatchStatus.CurrentBatchIndex
		if int(currentBatchIndex+1) < len(ctx.RolloutRun.Spec.Batch.Batches) {
			newStatus.BatchStatus.CurrentBatchState = StepNone
			newStatus.BatchStatus.CurrentBatchIndex = currentBatchIndex + 1
		}
	}

	if isFinalStepState(currentState) &&
		isFinalStepState(newStatus.BatchStatus.CurrentBatchState) &&
		int(newStatus.BatchStatus.CurrentBatchIndex+1) == len(ctx.RolloutRun.Spec.Batch.Batches) {
		done = true
	}
	return done, result, err
}

func (e *batchExecutor) doPausing(ctx *ExecutorContext) (bool, time.Duration, error) {
	rolloutName := ctx.Rollout.Name
	rolloutRunName := ctx.RolloutRun.Name
	newStatus := ctx.NewStatus
	currentBatchIndex := newStatus.BatchStatus.CurrentBatchIndex
	currentBatch := ctx.RolloutRun.Spec.Batch.Batches[currentBatchIndex]

	batchControl := control.NewBatchReleaseControl(ctx.Accessor, ctx.Client)

	for _, item := range currentBatch.Targets {
		wi := ctx.Workloads.Get(item.Cluster, item.Name)
		if wi == nil {
			return false, retryStop, newWorkloadNotFoundError(item.CrossClusterObjectNameReference)
		}
		err := batchControl.Initialize(wi, rolloutName, rolloutRunName, currentBatchIndex)
		if err != nil {
			return false, retryStop, err
		}
	}

	if ctx.RolloutRun.Spec.Batch.Batches[currentBatchIndex].Breakpoint {
		ctx.Pause()
	}
	return true, retryImmediately, nil
}

func (e *batchExecutor) doPreStepHook(ctx *ExecutorContext) (bool, time.Duration, error) {
	return e.webhook.Do(ctx, rolloutv1alpha1.PreBatchStepHook)
}

func (e *batchExecutor) doPostStepHook(ctx *ExecutorContext) (bool, time.Duration, error) {
	return e.webhook.Do(ctx, rolloutv1alpha1.PostBatchStepHook)
}

func newWorkloadNotFoundError(ref rolloutv1alpha1.CrossClusterObjectNameReference) error {
	return control.TerminalError(&rolloutv1alpha1.CodeReasonMessage{
		Code:    "WorkloadNotFound",
		Reason:  "WorkloadNotFound",
		Message: fmt.Sprintf("workload (%s) not found ", ref.String()),
	})
}

// doBatchUpgrading process upgrading state
func (e *batchExecutor) doBatchUpgrading(ctx *ExecutorContext) (bool, time.Duration, error) {
	rolloutRun := ctx.RolloutRun
	newStatus := ctx.NewStatus
	currentBatchIndex := newStatus.BatchStatus.CurrentBatchIndex
	currentBatch := rolloutRun.Spec.Batch.Batches[currentBatchIndex]

	logger := e.loggerWithContext(ctx)
	logger.Info("do batch upgrading and check")

	batchControl := control.NewBatchReleaseControl(ctx.Accessor, ctx.Client)

	// upgrade partition
	batchTargetStatuses := make([]rolloutv1alpha1.RolloutWorkloadStatus, 0)
	workloadChanged := false
	for _, item := range currentBatch.Targets {
		wi := ctx.Workloads.Get(item.Cluster, item.Name)
		if wi == nil {
			return false, retryStop, newWorkloadNotFoundError(item.CrossClusterObjectNameReference)
		}

		// upgradePartition is an idempotent function
		changed, err := batchControl.UpdatePartition(wi, item.Replicas)
		if err != nil {
			return false, retryStop, err
		}

		batchTargetStatuses = append(batchTargetStatuses, wi.APIStatus())

		if changed {
			workloadChanged = true
		}
	}

	// update target status in batch
	newStatus.BatchStatus.Records[currentBatchIndex].Targets = batchTargetStatuses

	if workloadChanged {
		// check next time, give the controller a little time to react
		return false, retryDefault, nil
	}

	// all workloads are updated now, then check if they are ready
	for _, item := range currentBatch.Targets {
		// target will not be nil here
		info := ctx.Workloads.Get(item.Cluster, item.Name)
		status := info.APIStatus()
		partition, _ := workload.CalculatePartitionReplicas(&status.Replicas, item.Replicas)

		if !info.CheckUpdatedReady(partition) {
			// ready
			logger.Info("still waiting for target ready", "target", item.CrossClusterObjectNameReference)
			return false, retryDefault, nil
		}
	}
	return true, retryImmediately, nil
}

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

	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	ctrl "sigs.k8s.io/controller-runtime"

	rolloutv1alpha1 "kusionstack.io/rollout/apis/rollout/v1alpha1"
	"kusionstack.io/rollout/pkg/controllers/rolloutrun/control"
	"kusionstack.io/rollout/pkg/workload"
)

type batchExecutor struct {
	webhook      webhookExecutor
	stateMachine *stepStateMachine
}

func newBatchExecutor(webhook webhookExecutor) *batchExecutor {
	e := &batchExecutor{
		webhook:      webhook,
		stateMachine: newStepStateMachine(),
	}

	e.stateMachine.add(StepNone, StepPending, e.doPausing)
	e.stateMachine.add(StepPending, StepPreBatchStepHook, skipStep)
	e.stateMachine.add(StepPreBatchStepHook, StepRunning, e.doPreStepHook)
	e.stateMachine.add(StepRunning, StepPostBatchStepHook, e.doBatchUpgrading)
	e.stateMachine.add(StepPostBatchStepHook, StepResourceRecycling, e.doPostStepHook)
	e.stateMachine.add(StepResourceRecycling, StepSucceeded, e.doRecycle)
	e.stateMachine.add(StepSucceeded, "", skipStep)
	return e
}

func (e *batchExecutor) Do(ctx *ExecutorContext) (done bool, result ctrl.Result, err error) {
	newStatus := ctx.NewStatus
	currentBatchIndex := newStatus.BatchStatus.CurrentBatchIndex
	currentState := newStatus.BatchStatus.CurrentBatchState

	logger := ctx.GetBatchLogger()
	if !e.isSupported(ctx) {
		// skip batch release if workload accessor don't support it.
		logger.Info("workload accessor don't support batch release, skip it")
		ctx.SkipCurrentRelease()
		return true, ctrl.Result{Requeue: true}, nil
	}

	stepDone, result, err := e.stateMachine.do(ctx, currentState)
	if err != nil {
		return false, result, err
	}
	if !stepDone {
		return false, result, nil
	}

	if int(currentBatchIndex+1) < len(ctx.RolloutRun.Spec.Batch.Batches) {
		// move to next batch
		newStatus.BatchStatus.CurrentBatchState = StepNone
		newStatus.BatchStatus.CurrentBatchIndex = currentBatchIndex + 1
		return false, result, nil
	}

	return true, result, nil
}

func (e *batchExecutor) isSupported(ctx *ExecutorContext) bool {
	_, ok := ctx.Accessor.(workload.BatchReleaseControl)
	return ok
}

func (e *batchExecutor) doRecycle(ctx *ExecutorContext) (bool, time.Duration, error) {
	// recycling only work on last batch now
	if int(ctx.NewStatus.BatchStatus.CurrentBatchIndex+1) < len(ctx.RolloutRun.Spec.Batch.Batches) {
		return true, retryImmediately, nil
	}

	// last batch finished, try to finalize all workloads
	allTargets := map[rolloutv1alpha1.CrossClusterObjectNameReference]bool{}
	// finalize batch release
	batchControl := control.NewBatchReleaseControl(ctx.Accessor, ctx.Client)

	for _, item := range ctx.RolloutRun.Spec.Batch.Batches {
		for _, target := range item.Targets {
			allTargets[target.CrossClusterObjectNameReference] = true
		}
	}

	var finalizeErrs []error

	for target := range allTargets {
		wi := ctx.Workloads.Get(target.Cluster, target.Name)
		if wi == nil {
			// ignore not found workload
			continue
		}
		err := batchControl.Finalize(wi)
		if err != nil {
			// try our best to finalize all workloasd
			finalizeErrs = append(finalizeErrs, err)
			continue
		}
	}

	if len(finalizeErrs) > 0 {
		return false, retryDefault, utilerrors.NewAggregate(finalizeErrs)
	}

	return true, retryImmediately, nil
}

func (e *batchExecutor) doPausing(ctx *ExecutorContext) (bool, time.Duration, error) {
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
		err := batchControl.Initialize(wi, ctx.OwnerKind, ctx.OwnerName, rolloutRunName, currentBatchIndex)
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

	logger := ctx.GetBatchLogger()

	batchControl := control.NewBatchReleaseControl(ctx.Accessor, ctx.Client)

	batchTargetStatuses := make([]rolloutv1alpha1.RolloutWorkloadStatus, 0)

	allWorkloadReady := true
	for _, item := range currentBatch.Targets {
		info := ctx.Workloads.Get(item.Cluster, item.Name)
		if info == nil {
			// If the target workload does not exist, the retries will stop.
			return false, retryStop, newWorkloadNotFoundError(item.CrossClusterObjectNameReference)
		}

		status := info.APIStatus()
		batchTargetStatuses = append(batchTargetStatuses, info.APIStatus())

		expectedUpdatedReplicas, _ := workload.CalculateUpdatedReplicas(&status.Replicas, item.Replicas)

		if info.CheckUpdatedReady(expectedUpdatedReplicas) {
			// if the target is ready, we will not change partition
			continue
		}

		allWorkloadReady = false
		logger.V(3).Info("still waiting for target to be ready", "target", item.CrossClusterObjectNameReference)

		// ensure partition: upgradePartition is an idempotent function
		changed, err := batchControl.UpdatePartition(info, item.Replicas)
		if err != nil {
			return false, retryStop, err
		}
		if changed {
			logger.V(2).Info("upgrade target partition", "target", item.CrossClusterObjectNameReference, "partition", expectedUpdatedReplicas)
		}
	}

	// update target status in batch
	newStatus.BatchStatus.Records[currentBatchIndex].Targets = batchTargetStatuses

	if allWorkloadReady {
		return true, retryImmediately, nil
	}

	// wait for next reconcile
	return false, retryDefault, nil
}

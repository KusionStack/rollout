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
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"

	rolloutv1alpha1 "kusionstack.io/rollout/apis/rollout/v1alpha1"
	"kusionstack.io/rollout/pkg/controllers/rolloutrun/control"
	"kusionstack.io/rollout/pkg/workload"
)

type batchExecutor struct {
	webhook     webhookExecutor
	stateEngine *stepStateEngine
}

func newBatchExecutor(webhook webhookExecutor) *batchExecutor {
	e := &batchExecutor{
		webhook:     webhook,
		stateEngine: newStepStateEngine(),
	}

	e.stateEngine.add(StepNone, StepPending, e.doPausing, e.release)
	e.stateEngine.add(StepPending, StepPreBatchStepHook, skipStep, e.release)
	e.stateEngine.add(StepPreBatchStepHook, StepRunning, e.doPreStepHook, e.release)
	e.stateEngine.add(StepRunning, StepPostBatchStepHook, e.doBatchUpgrading, e.release)
	e.stateEngine.add(StepPostBatchStepHook, StepResourceRecycling, e.doPostStepHook, e.release)
	e.stateEngine.add(StepResourceRecycling, StepSucceeded, e.doRecycle, e.release)
	e.stateEngine.add(StepSucceeded, "", skipStep, skipStep)
	return e
}

func (e *batchExecutor) init(ctx *ExecutorContext) bool {
	logger := ctx.GetBatchLogger()
	if !e.isSupported(ctx) {
		// skip batch release if workload accessor don't support it.
		logger.Info("workload accessor don't support batch release, skip it")
		ctx.SkipCurrentRelease()
		return true
	}
	return false
}

func (e *batchExecutor) Do(ctx *ExecutorContext) (done bool, result ctrl.Result, err error) {
	if e.init(ctx) {
		return true, ctrl.Result{Requeue: true}, nil
	}
	newStatus := ctx.NewStatus
	currentBatchIndex := newStatus.BatchStatus.CurrentBatchIndex
	currentState := newStatus.BatchStatus.CurrentBatchState

	stepDone, result, err := e.stateEngine.do(ctx, currentState)
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

func (e *batchExecutor) Cancel(ctx *ExecutorContext) (done bool, result ctrl.Result, err error) {
	done = e.init(ctx)
	if done {
		return true, ctrl.Result{Requeue: true}, nil
	}
	return e.stateEngine.cancel(ctx, ctx.NewStatus.BatchStatus.CurrentBatchState)
}

func (e *batchExecutor) isSupported(ctx *ExecutorContext) bool {
	_, ok := ctx.Accessor.(workload.BatchReleaseControl)
	return ok
}

func (e *batchExecutor) release(ctx *ExecutorContext) (bool, time.Duration, error) {
	// frstly try to stop webhook
	e.webhook.Cancel(ctx)

	// try to finalize all workloads
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
		err := batchControl.Finalize(ctx, wi)
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

func (e *batchExecutor) doRecycle(ctx *ExecutorContext) (bool, time.Duration, error) {
	// recycling only work on last batch now
	if int(ctx.NewStatus.BatchStatus.CurrentBatchIndex+1) < len(ctx.RolloutRun.Spec.Batch.Batches) {
		return true, retryImmediately, nil
	}
	return e.release(ctx)
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

		currentBatchExpectedReplicas, _ := workload.CalculateUpdatedReplicas(&status.Replicas, item.Replicas)

		if info.CheckUpdatedReady(currentBatchExpectedReplicas) {
			// if the target is ready, we will not change partition
			continue
		}

		allWorkloadReady = false
		logger.V(3).Info("still waiting for target to be ready", "target", item.CrossClusterObjectNameReference)

		expectedReplicas, err := e.calculateExpectedReplicasBySlidingWindow(status, currentBatchExpectedReplicas, item.ReplicaSlidingWindow)
		if err != nil {
			return false, retryStop, err
		}

		// ensure partition: upgradePartition is an idempotent function
		changed, err := batchControl.UpdatePartition(info, expectedReplicas)
		if err != nil {
			return false, retryStop, err
		}
		if changed {
			logger.V(2).Info("upgrade target partition", "target", item.CrossClusterObjectNameReference, "partition", expectedReplicas)
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

// calculateExpectedReplicasBySlidingWindow calculate expected replicas by sliding window
// if window is nil, return currentBatchExpectedReplicas
// if window is not nil, return min(currentBatchExpectedReplicas, updatedAvailableReplicas + increment)
func (e *batchExecutor) calculateExpectedReplicasBySlidingWindow(status rolloutv1alpha1.RolloutWorkloadStatus, currentBatchExpectedReplicas int32, window *intstr.IntOrString) (int32, error) {
	if window == nil {
		return currentBatchExpectedReplicas, nil
	}
	increment, err := workload.CalculateUpdatedReplicas(&status.Replicas, *window)
	if err != nil {
		return currentBatchExpectedReplicas, err
	}
	expected := status.UpdatedAvailableReplicas + increment
	// limit expected replicas to currentBatchExpectedReplicas
	expected = min(currentBatchExpectedReplicas, expected)
	return expected, nil
}

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

	corev1 "k8s.io/api/core/v1"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/intstr"
	rolloutv1alpha1 "kusionstack.io/kube-api/rollout/v1alpha1"
	ctrl "sigs.k8s.io/controller-runtime"

	"kusionstack.io/rollout/pkg/controllers/rolloutrun/control"
	"kusionstack.io/rollout/pkg/utils"
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
		err := batchControl.Initialize(ctx, wi, ctx.OwnerKind, ctx.OwnerName, rolloutRunName, currentBatchIndex)
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
	return utils.TerminalError(&rolloutv1alpha1.CodeReasonMessage{
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
	totalBatches := len(rolloutRun.Spec.Batch.Batches)
	isLastBatch := int(currentBatchIndex+1) == totalBatches

	logger := ctx.GetBatchLogger()

	batchControl := control.NewBatchReleaseControl(ctx.Accessor, ctx.Client)

	batchTargetStatuses := make([]rolloutv1alpha1.RolloutWorkloadStatus, 0)

	allWorkloadReady := true
	allWorkloadsAutoSkippable := true

	for _, item := range currentBatch.Targets {
		info := ctx.Workloads.Get(item.Cluster, item.Name)
		if info == nil {
			// If the target workload does not exist, the retries will stop.
			return false, retryStop, newWorkloadNotFoundError(item.CrossClusterObjectNameReference)
		}

		status := info.APIStatus()
		batchTargetStatuses = append(batchTargetStatuses, info.APIStatus())

		currentBatchExpectedReplicas, _ := workload.CalculateUpdatedReplicas(&status.Replicas, item.Replicas)

		ready, reason := info.CheckUpdatedReady(currentBatchExpectedReplicas, isLastBatch)
		if ready {
			// if the target is ready, we will not change partition
			continue
		}
		ctx.Recorder.Eventf(ctx.RolloutRun, corev1.EventTypeNormal, "WaitingWorkloadUpdatedReady", "still waiting for target to be ready, target: %v, reason: %s", item.CrossClusterObjectNameReference, reason)

		allWorkloadReady = false
		logger.V(3).Info("still waiting for target to be ready", "target", item.CrossClusterObjectNameReference, "reason", reason)

		// Check auto-skip toleration for this workload
		if !e.canAutoSkipTarget(item, info, currentBatchExpectedReplicas, isLastBatch, newStatus) {
			allWorkloadsAutoSkippable = false
		}

		expectedReplicas, err := e.calculateExpectedReplicasBySlidingWindow(status, currentBatchExpectedReplicas, item.ReplicaSlidingWindow)
		if err != nil {
			return false, retryStop, err
		}

		// ensure partition: upgradePartition is an idempotent function
		changed, err := batchControl.UpdatePartition(ctx, info, expectedReplicas)
		if err != nil {
			logger.Error(err, "failed to update partition", "target", item.CrossClusterObjectNameReference)
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

	if allWorkloadsAutoSkippable {
		logger.Info("auto-skipping batch due to toleration")
		newStatus.BatchStatus.Records[currentBatchIndex].State = StepSkipped
		recordRolloutRunTolerations(&newStatus.BatchStatus.Tolerations, rolloutRun.Spec.Batch.Batches, ctx.Workloads, currentBatchIndex)

		// Mirror manual-skip semantics (see do_command.go:handleBatchStatusWhenSkipped):
		// bypass PostBatchStepHook (webhook) and ResourceRecycling, and advance to the
		// next batch or PostRollout phase directly. Returns done=false so the state
		// engine does NOT call MoveToNextState(StepPostBatchStepHook), which would
		// otherwise overwrite the StepSkipped state we just wrote above.
		if int(currentBatchIndex) >= len(rolloutRun.Spec.Batch.Batches)-1 {
			// Last batch: advance phase to PostRollout (will transition to Succeeded
			// on the next reconcile, mirroring manual skip behavior).
			newStatus.Phase = rolloutv1alpha1.RolloutRunPhasePostRollout
		} else {
			// Not the last batch: advance to the next batch from StepNone so the
			// state machine restarts on doPausing/Initialize for the new batch.
			newStatus.BatchStatus.CurrentBatchIndex = currentBatchIndex + 1
			newStatus.BatchStatus.CurrentBatchState = StepNone
		}
		return false, retryImmediately, nil
	}

	// wait for next reconcile
	return false, retryDefault, nil
}

// canAutoSkipTarget checks if the workload target meets the auto-skip toleration conditions.
// Returns true only when the workload is not ready due to a real deficit (gap > 0) within
// the toleration threshold and the initial delay has elapsed.
// Transient states (Generation mismatch, terminating replicas, last-batch overscaling)
// are NOT auto-skippable because the gap is unreliable until the workload stabilizes.
func (e *batchExecutor) canAutoSkipTarget(item rolloutv1alpha1.RolloutRunStepTarget, info *workload.Info, currentBatchExpectedReplicas int32, isLastBatch bool, newStatus *rolloutv1alpha1.RolloutRunStatus) bool {
	if item.Toleration == nil || item.Toleration.FailureThreshold == nil {
		return false
	}

	// Not skippable while workload has not been reconciled yet (Generation mismatch).
	// UpdatedAvailableReplicas may be stale from the previous generation.
	if info.Generation != info.Status.ObservedGeneration {
		return false
	}

	// On last batch, not skippable if observed replicas exceed desired or terminating replicas exist(strict check).
	if isLastBatch && info.Status.ObservedReplicas > info.Status.DesiredReplicas || info.Status.TerminatingReplicas != 0 {
		return false
	}

	// Only evaluate toleration on a real deficit.
	gap := currentBatchExpectedReplicas - info.Status.UpdatedAvailableReplicas
	if gap <= 0 {
		return false
	}

	if gap > *item.Toleration.FailureThreshold {
		return false
	}

	// gap is within threshold, check timeout
	if item.Toleration.InitialDelaySeconds != nil {
		currentBatchIndex := newStatus.BatchStatus.CurrentBatchIndex
		startTime := newStatus.BatchStatus.Records[currentBatchIndex].StartTime
		if startTime == nil {
			return false
		}
		elapsed := time.Since(startTime.Time)
		if elapsed < time.Duration(*item.Toleration.InitialDelaySeconds)*time.Second {
			return false
		}
	}

	return true
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

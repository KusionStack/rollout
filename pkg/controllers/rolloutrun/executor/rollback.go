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
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"kusionstack.io/kube-api/rollout"
	rolloutv1alpha1 "kusionstack.io/kube-api/rollout/v1alpha1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"kusionstack.io/rollout/pkg/controllers/rolloutrun/control"
	"kusionstack.io/rollout/pkg/workload"
)

func newDoRollbackError(reason, msg string) *rolloutv1alpha1.CodeReasonMessage {
	return &rolloutv1alpha1.CodeReasonMessage{
		Code:    "DoRollbackError",
		Reason:  reason,
		Message: msg,
	}
}

type rollbackExecutor struct {
	webhook     webhookExecutor
	stateEngine *stepStateEngine
}

func newRollbackExecutor(webhook webhookExecutor) *rollbackExecutor {
	e := &rollbackExecutor{
		webhook:     webhook,
		stateEngine: newStepStateEngine(),
	}

	e.stateEngine.add(StepNone, StepPending, e.doPausing, e.release)
	e.stateEngine.add(StepPending, StepPreRollbackStepHook, skipStep, e.release)
	e.stateEngine.add(StepPreRollbackStepHook, StepRunning, e.doPreStepHook, e.release)
	e.stateEngine.add(StepRunning, StepPostRollbackStepHook, e.doBatchUpgrading, e.release)
	e.stateEngine.add(StepPostRollbackStepHook, StepResourceRecycling, e.doPostStepHook, e.release)
	e.stateEngine.add(StepResourceRecycling, StepSucceeded, e.doFinalize, e.release)
	e.stateEngine.add(StepSucceeded, "", skipStep, skipStep)

	return e
}

func (e *rollbackExecutor) init(ctx *ExecutorContext) (done bool) {
	logger := ctx.GetRollbackLogger()
	if !e.isSupported(ctx) {
		// skip rollback release if workload accessor don't support it.
		logger.Info("workload accessor don't support rollback release, skip it")
		ctx.SkipCurrentRelease()
		return true
	}

	if ctx.inCanary() {
		ctx.TrafficManager.With(ctx.RolloutRun.Spec.Canary.Targets, ctx.RolloutRun.Spec.Canary.Traffic)
	} else if ctx.inBatchGray() {
		currentBatchIndex := ctx.NewStatus.BatchStatus.CurrentBatchIndex
		currentBatch := ctx.RolloutRun.Spec.Batch.Batches[currentBatchIndex]
		ctx.TrafficManager.With(currentBatch.Targets, currentBatch.Traffic)
	}
	return false
}

func (e *rollbackExecutor) Do(ctx *ExecutorContext) (done bool, result ctrl.Result, err error) {
	if e.init(ctx) {
		return true, ctrl.Result{Requeue: true}, nil
	}

	newStatus := ctx.NewStatus
	currentBatchIndex := newStatus.RollbackStatus.CurrentBatchIndex
	currentState := newStatus.RollbackStatus.CurrentBatchState

	// revert workload version and reset canary route before rollback stateEngine start
	if currentBatchIndex == 0 && currentState == StepNone {
		done, err := e.revert(ctx)
		if !done {
			return false, ctrl.Result{}, err
		}

		done, retry := e.deleteCanaryRoute(ctx)
		if !done {
			return false, ctrl.Result{RequeueAfter: retry}, nil
		}

		if ctx.inCanary() {
			done, retry, err = e.recycle(ctx)
			if !done {
				switch retry {
				case retryStop:
					result = ctrl.Result{}
				default:
					result = ctrl.Result{RequeueAfter: retry}
				}
				return false, result, err
			}
		}

		//mark rollout into rollback
		ctx.RolloutRun.Annotations[rollout.AnnoRolloutPhaseRollbacking] = "true"
	}

	stepDone, result, err := e.stateEngine.do(ctx, currentState)
	if err != nil {
		return false, result, err
	}
	if !stepDone {
		return false, result, nil
	}

	if int(currentBatchIndex+1) < len(ctx.RolloutRun.Spec.Rollback.Batches) {
		// move to next batch
		newStatus.RollbackStatus.CurrentBatchState = StepNone
		newStatus.RollbackStatus.CurrentBatchIndex = currentBatchIndex + 1
		return false, result, nil
	}

	recycleDone, retry := e.recyleCanaryResource(ctx)
	if !recycleDone {
		return false, ctrl.Result{RequeueAfter: retry}, nil
	}

	return true, result, nil
}

func (e *rollbackExecutor) Cancel(ctx *ExecutorContext) (done bool, result ctrl.Result, err error) {
	done = e.init(ctx)
	if done {
		return true, ctrl.Result{Requeue: true}, nil
	}

	return e.stateEngine.cancel(ctx, ctx.NewStatus.RollbackStatus.CurrentBatchState)
}

func (e *rollbackExecutor) isSupported(ctx *ExecutorContext) bool {
	_, ok := ctx.Accessor.(workload.RollbackReleaseControl)
	return ok
}

func (e *rollbackExecutor) doPausing(ctx *ExecutorContext) (bool, time.Duration, error) {
	rolloutRunName := ctx.RolloutRun.Name
	newStatus := ctx.NewStatus
	currentBatchIndex := newStatus.RollbackStatus.CurrentBatchIndex
	currentBatch := ctx.RolloutRun.Spec.Rollback.Batches[currentBatchIndex]

	rollbackControl := control.NewRollbackReleaseControl(ctx.Accessor, ctx.Client)

	for _, item := range currentBatch.Targets {
		wi := ctx.Workloads.Get(item.Cluster, item.Name)
		if wi == nil {
			return false, retryStop, newWorkloadNotFoundError(item.CrossClusterObjectNameReference)
		}
		err := rollbackControl.Initialize(ctx, wi, ctx.OwnerKind, ctx.OwnerName, rolloutRunName, currentBatchIndex)
		if err != nil {
			return false, retryStop, err
		}
	}

	if ctx.RolloutRun.Spec.Rollback.Batches[currentBatchIndex].Breakpoint {
		ctx.Pause()
	}
	return true, retryImmediately, nil
}

func (e *rollbackExecutor) doPreStepHook(ctx *ExecutorContext) (bool, time.Duration, error) {
	return e.webhook.Do(ctx, rolloutv1alpha1.PreRollbackStepHook)
}

func (e *rollbackExecutor) doPostStepHook(ctx *ExecutorContext) (bool, time.Duration, error) {
	return e.webhook.Do(ctx, rolloutv1alpha1.PostRollbackStepHook)
}

// doBatchUpgrading process upgrading state
func (e *rollbackExecutor) doBatchUpgrading(ctx *ExecutorContext) (bool, time.Duration, error) {
	rolloutRun := ctx.RolloutRun
	newStatus := ctx.NewStatus
	currentBatchIndex := newStatus.RollbackStatus.CurrentBatchIndex
	currentBatch := rolloutRun.Spec.Rollback.Batches[currentBatchIndex]

	logger := ctx.GetBatchLogger()

	rollbackControl := control.NewRollbackReleaseControl(ctx.Accessor, ctx.Client)

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

		expectedReplicas, err := calculateExpectedReplicasBySlidingWindow(status, currentBatchExpectedReplicas, item.ReplicaSlidingWindow)
		if err != nil {
			return false, retryStop, err
		}

		// ensure partition: upgradePartition is an idempotent function
		changed, err := rollbackControl.UpdatePartition(ctx, info, expectedReplicas)
		if err != nil {
			return false, retryStop, err
		}
		if changed {
			logger.V(2).Info("upgrade target partition", "target", item.CrossClusterObjectNameReference, "partition", expectedReplicas)
		}
	}

	// update target status in rollback
	newStatus.RollbackStatus.Records[currentBatchIndex].Targets = batchTargetStatuses

	if allWorkloadReady {
		return true, retryImmediately, nil
	}

	// wait for next reconcile
	return false, retryDefault, nil
}

func (e *rollbackExecutor) revert(ctx *ExecutorContext) (bool, error) {
	newStatus := ctx.NewStatus
	currentBatchIndex := newStatus.RollbackStatus.CurrentBatchIndex
	currentBatch := ctx.RolloutRun.Spec.Rollback.Batches[currentBatchIndex]

	rollbackControl := control.NewRollbackReleaseControl(ctx.Accessor, ctx.Client)

	for _, item := range currentBatch.Targets {
		wi := ctx.Workloads.Get(item.Cluster, item.Name)
		if wi == nil {
			return false, newWorkloadNotFoundError(item.CrossClusterObjectNameReference)
		}
		err := rollbackControl.Revert(ctx, wi)
		if err != nil {
			return false, err
		}
	}
	return true, nil
}

func (e *rollbackExecutor) deleteCanaryRoute(ctx *ExecutorContext) (bool, time.Duration) {
	if !ctx.inCanary() && !ctx.inBatchGray() {
		return true, retryDefault
	}

	done, retry := e.modifyTraffic(ctx, "deleteCanaryRoute")
	if !done {
		return false, retry
	}

	return true, retryDefault
}

func (e *rollbackExecutor) deleteCanaryWorkloads(ctx *ExecutorContext) (bool, time.Duration, error) {
	if !ctx.inCanary() {
		return true, retryDefault, nil
	}

	rolloutRun := ctx.RolloutRun
	releaseControl := control.NewCanaryReleaseControl(ctx.Accessor, ctx.Client)

	for _, item := range rolloutRun.Spec.Canary.Targets {
		wi := ctx.Workloads.Get(item.Cluster, item.Name)
		if wi == nil {
			return false, retryStop, newWorkloadNotFoundError(item.CrossClusterObjectNameReference)
		}

		if err := releaseControl.Finalize(ctx, wi); err != nil {
			return false, retryStop, newDoRollbackError(
				"FailedFinalize",
				fmt.Sprintf("failed to delete canary resource for workload(%s), err: %v", item.CrossClusterObjectNameReference, err),
			)
		}
	}

	return true, retryDefault, nil
}

func (e *rollbackExecutor) recyleCanaryResource(ctx *ExecutorContext) (bool, time.Duration) {
	if !ctx.inCanary() && !ctx.inBatchGray() {
		return true, retryDefault
	}

	done, retry := e.modifyTraffic(ctx, "resetRoute")
	if !done {
		return false, retry
	}

	done, retry = e.modifyTraffic(ctx, "deleteForkedBackends")
	if !done {
		return false, retry
	}

	return true, retryDefault
}

func (e *rollbackExecutor) recycle(ctx *ExecutorContext) (bool, time.Duration, error) {
	done, retry, err := e.deleteCanaryWorkloads(ctx)
	if !done {
		return false, retry, err
	}

	done, retry = e.recyleCanaryResource(ctx)
	if !done {
		return false, retry, nil
	}

	return true, retryDefault, nil
}

func (e *rollbackExecutor) doFinalize(ctx *ExecutorContext) (bool, time.Duration, error) {
	// recycling only work on last batch now
	if int(ctx.NewStatus.RollbackStatus.CurrentBatchIndex+1) < len(ctx.RolloutRun.Spec.Rollback.Batches) {
		return true, retryImmediately, nil
	}
	return e.release(ctx)
}

func (e *rollbackExecutor) release(ctx *ExecutorContext) (bool, time.Duration, error) {
	// frstly try to stop webhook
	e.webhook.Cancel(ctx)

	// try to finalize all workloads
	allTargets := map[rolloutv1alpha1.CrossClusterObjectNameReference]bool{}
	// finalize rollback release
	rollbackControl := control.NewRollbackReleaseControl(ctx.Accessor, ctx.Client)

	for _, item := range ctx.RolloutRun.Spec.Rollback.Batches {
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
		err := rollbackControl.Finalize(ctx, wi)
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

func (e *rollbackExecutor) modifyTraffic(ctx *ExecutorContext, op string) (bool, time.Duration) {
	logger := ctx.GetCanaryLogger()
	rolloutRun := ctx.RolloutRun
	currentBatchIndex := ctx.NewStatus.BatchStatus.CurrentBatchIndex
	opResult := controllerutil.OperationResultNone

	if rolloutRun.Spec.Canary.Traffic == nil && rolloutRun.Spec.Batch.Batches[currentBatchIndex].Traffic == nil {
		logger.V(3).Info("traffic is nil, skip modify traffic")
		return true, retryImmediately
	}

	goctx := logr.NewContext(ctx.Context, logger)

	var err error
	switch op {
	case "deleteCanaryRoute":
		opResult, err = ctx.TrafficManager.DeleteCanaryRoute(goctx)
	case "resetRoute":
		opResult, err = ctx.TrafficManager.ResetRoute(goctx)
	case "deleteForkedBackends":
		opResult, err = ctx.TrafficManager.DeleteForkedBackends(goctx)
	}
	if err != nil {
		logger.Error(err, "failed to modify traffic", "operation", op)
		return false, retryDefault
	}

	if opResult != controllerutil.OperationResultNone {
		logger.Info("modify traffic routing", "operation", op, "result", opResult)
		// check next time
		return false, retryDefault
	}

	// 1.b. waiting for traffic
	ready := ctx.TrafficManager.CheckReady(ctx)
	if !ready {
		return false, retryDefault
	}

	return true, retryImmediately
}

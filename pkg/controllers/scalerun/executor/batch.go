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
	rolloutv1alpha1 "kusionstack.io/kube-api/rollout/v1alpha1"
	ctrl "sigs.k8s.io/controller-runtime"

	rorexecutor "kusionstack.io/rollout/pkg/controllers/rolloutrun/executor"
	"kusionstack.io/rollout/pkg/controllers/scalerun/control"
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

	e.stateEngine.add(rorexecutor.StepNone, rorexecutor.StepPending, e.doPausing, e.release)
	e.stateEngine.add(rorexecutor.StepPending, rorexecutor.StepPreBatchStepHook, skipStep, e.release)
	e.stateEngine.add(rorexecutor.StepPreBatchStepHook, rorexecutor.StepRunning, e.doPreStepHook, e.release)
	e.stateEngine.add(rorexecutor.StepRunning, rorexecutor.StepPostBatchStepHook, e.doBatchUpgrading, e.release)
	e.stateEngine.add(rorexecutor.StepPostBatchStepHook, rorexecutor.StepResourceRecycling, e.doPostStepHook, e.release)
	e.stateEngine.add(rorexecutor.StepResourceRecycling, rorexecutor.StepSucceeded, e.doRecycle, e.release)
	e.stateEngine.add(rorexecutor.StepSucceeded, "", skipStep, skipStep)
	return e
}

func (e *batchExecutor) init(ctx *ExecutorContext) bool {
	logger := ctx.GetBatchLogger()
	if !e.isSupported(ctx) {
		// skip batch scale if workload accessor don't support it.
		logger.Info("workload accessor don't support batch scale, skip it")
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
	currentBatchIndex := newStatus.Batches.CurrentBatchIndex
	currentState := newStatus.Batches.CurrentBatchState

	stepDone, result, err := e.stateEngine.do(ctx, currentState)
	if err != nil {
		return false, result, err
	}
	if !stepDone {
		return false, result, nil
	}

	if int(currentBatchIndex+1) < len(ctx.ScaleRun.Spec.Batch.Batches) {
		// move to next batch
		newStatus.Batches.CurrentBatchState = rorexecutor.StepNone
		newStatus.Batches.CurrentBatchIndex = currentBatchIndex + 1
		return false, result, nil
	}

	return true, result, nil
}

func (e *batchExecutor) Cancel(ctx *ExecutorContext) (done bool, result ctrl.Result, err error) {
	done = e.init(ctx)
	if done {
		return true, ctrl.Result{Requeue: true}, nil
	}
	return e.stateEngine.cancel(ctx, ctx.NewStatus.Batches.CurrentBatchState)
}

func (e *batchExecutor) isSupported(ctx *ExecutorContext) bool {
	_, ok := ctx.Accessor.(workload.ScaleControl)
	return ok
}

func (e *batchExecutor) release(ctx *ExecutorContext) (bool, time.Duration, error) {
	// frstly try to stop webhook
	e.webhook.Cancel(ctx)

	// try to finalize all workloads
	allTargets := map[rolloutv1alpha1.CrossClusterObjectNameReference]bool{}
	// finalize batch release
	batchControl := control.NewBatchScaleControl(ctx.Accessor, ctx.Client)

	for _, item := range ctx.ScaleRun.Spec.Batch.Batches {
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
	if int(ctx.NewStatus.Batches.CurrentBatchIndex+1) < len(ctx.ScaleRun.Spec.Batch.Batches) {
		return true, retryImmediately, nil
	}
	return e.release(ctx)
}

func (e *batchExecutor) doPausing(ctx *ExecutorContext) (bool, time.Duration, error) {
	scaleRunName := ctx.ScaleRun.Name
	newStatus := ctx.NewStatus
	currentBatchIndex := newStatus.Batches.CurrentBatchIndex
	currentBatch := ctx.ScaleRun.Spec.Batch.Batches[currentBatchIndex]

	batchControl := control.NewBatchScaleControl(ctx.Accessor, ctx.Client)

	for _, item := range currentBatch.Targets {
		wi := ctx.Workloads.Get(item.Cluster, item.Name)
		if wi == nil {
			return false, retryStop, newWorkloadNotFoundError(item.CrossClusterObjectNameReference)
		}
		err := batchControl.Initialize(ctx, wi, scaleRunName, currentBatchIndex)
		if err != nil {
			return false, retryStop, err
		}
	}

	if ctx.ScaleRun.Spec.Batch.Batches[currentBatchIndex].Breakpoint {
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
	scaleRun := ctx.ScaleRun
	newStatus := ctx.NewStatus
	currentBatchIndex := newStatus.Batches.CurrentBatchIndex
	currentBatch := scaleRun.Spec.Batch.Batches[currentBatchIndex]
	currentBatchWorkloadStatus := newStatus.Batches.Records[currentBatchIndex].Targets

	logger := ctx.GetBatchLogger()

	batchControl := control.NewBatchScaleControl(ctx.Accessor, ctx.Client)

	batchTargetStatuses := make([]rolloutv1alpha1.ScaleWorkloadStatus, 0)

	allWorkloadReady := true
	for _, item := range currentBatch.Targets {
		info := ctx.Workloads.Get(item.Cluster, item.Name)
		if info == nil {
			// If the target workload does not exist, the retries will stop.
			return false, retryStop, newWorkloadNotFoundError(item.CrossClusterObjectNameReference)
		}

		needApplyReplicas := false

		workloadStatus := info.ScaleWorkloadStatus()
		currentWorkloadStatus := e.findCurrentWorkloadStatus(currentBatchWorkloadStatus, item.Cluster, item.Name)
		if currentWorkloadStatus == nil {
			workloadStatus.ScaleFrom = info.Status.Replicas
			workloadStatus.ScaleTo = item.Replicas
			needApplyReplicas = true
		} else {
			workloadStatus.ScaleFrom = currentWorkloadStatus.ScaleFrom
			workloadStatus.ScaleTo = currentWorkloadStatus.ScaleTo
		}
		batchTargetStatuses = append(batchTargetStatuses, workloadStatus)

		if e.checkScaledReady(info, workloadStatus.ScaleFrom, workloadStatus.ScaleTo) && !needApplyReplicas {
			// if the target is ready, we will not change replicas
			continue
		}

		allWorkloadReady = false
		if !needApplyReplicas {
			// if the target's replicas has been updated, we will not change replicas
			continue
		}

		logger.Info("need to apply target replicas", "target", item.CrossClusterObjectNameReference)
		changed, err := batchControl.Scale(ctx, info, item.Replicas)
		if err != nil {
			return false, retryStop, err
		}
		if changed {
			logger.V(2).Info("upgrade target replicas", "target", item.CrossClusterObjectNameReference, "replicas", item.Replicas)
		}
	}

	// update target status in batch
	newStatus.Batches.Records[currentBatchIndex].Targets = batchTargetStatuses

	if allWorkloadReady {
		return true, retryImmediately, nil
	}

	// wait for next reconcile
	return false, retryDefault, nil
}

func (e *batchExecutor) checkScaledReady(info *workload.Info, scaledFrom, scaledTo int32) bool {
	if info.Status.ObservedGeneration != info.Generation {
		return false
	}

	if scaledFrom >= scaledTo {
		return info.Status.CurrentReplicas <= info.Status.Replicas && info.Status.TerminatingReplicas == 0
	} else {
		return info.Status.AvailableReplicas >= info.Status.Replicas
	}
}

func (e *batchExecutor) findCurrentWorkloadStatus(status []rolloutv1alpha1.ScaleWorkloadStatus, cluster, name string) *rolloutv1alpha1.ScaleWorkloadStatus {
	for _, workloadStatus := range status {
		if workloadStatus.Cluster == cluster && workloadStatus.Name == name {
			return &workloadStatus
		}
	}
	return nil
}

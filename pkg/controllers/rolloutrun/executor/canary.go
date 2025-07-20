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

	rolloutapi "kusionstack.io/kube-api/rollout"
	rolloutv1alpha1 "kusionstack.io/kube-api/rollout/v1alpha1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"kusionstack.io/rollout/pkg/controllers/rolloutrun/control"
	"kusionstack.io/rollout/pkg/workload"
)

func newDoCanaryError(reason, msg string) *rolloutv1alpha1.CodeReasonMessage {
	return &rolloutv1alpha1.CodeReasonMessage{
		Code:    "DoCanaryError",
		Reason:  reason,
		Message: msg,
	}
}

type canaryExecutor struct {
	webhook     webhookExecutor
	stateEngine *stepStateEngine
}

func newCanaryExecutor(webhook webhookExecutor) *canaryExecutor {
	e := &canaryExecutor{
		webhook:     webhook,
		stateEngine: newStepStateEngine(),
	}

	e.stateEngine.add(StepNone, StepPending, skipStep, e.release)
	e.stateEngine.add(StepPending, StepPreCanaryStepHook, e.doInit, e.release)
	e.stateEngine.add(StepPreCanaryStepHook, StepRunning, e.doPreStepHook, e.release)
	e.stateEngine.add(StepRunning, StepPostCanaryStepHook, e.doCanary, e.release)
	e.stateEngine.add(StepPostCanaryStepHook, StepResourceRecycling, e.doPostStepHook, e.release)
	e.stateEngine.add(StepResourceRecycling, StepSucceeded, e.release, e.release)
	e.stateEngine.add(StepSucceeded, "", skipStep, skipStep)

	return e
}

func (e *canaryExecutor) init(ctx *ExecutorContext) (done bool) {
	if !ctx.inCanary() {
		return true
	}

	logger := ctx.GetCanaryLogger()
	if !e.isSupported(ctx) {
		// skip canary release if workload accessor don't support it.
		logger.Info("workload accessor don't support canary release, skip it")
		ctx.SkipCurrentRelease()
		return true
	}
	ctx.TrafficManager.With(logger, ctx.RolloutRun.Spec.Canary.Targets, ctx.RolloutRun.Spec.Canary.Traffic)
	return false
}

func (e *canaryExecutor) Do(ctx *ExecutorContext) (done bool, result ctrl.Result, err error) {
	done = e.init(ctx)
	if done {
		return true, ctrl.Result{Requeue: true}, nil
	}
	return e.stateEngine.do(ctx, ctx.NewStatus.CanaryStatus.State)
}

func (e *canaryExecutor) Cancel(ctx *ExecutorContext) (done bool, result ctrl.Result, err error) {
	done = e.init(ctx)
	if done {
		return true, ctrl.Result{Requeue: true}, nil
	}

	return e.stateEngine.cancel(ctx, ctx.NewStatus.CanaryStatus.State)
}

func (e *canaryExecutor) isSupported(ctx *ExecutorContext) bool {
	_, ok := ctx.Accessor.(workload.CanaryReleaseControl)
	return ok
}

func (e *canaryExecutor) doInit(ctx *ExecutorContext) (bool, time.Duration, error) {
	rolloutRun := ctx.RolloutRun
	releaseControl := control.NewCanaryReleaseControl(ctx.Accessor, ctx.Client)
	for _, item := range rolloutRun.Spec.Canary.Targets {
		wi := ctx.Workloads.Get(item.Cluster, item.Name)
		if wi == nil {
			return false, retryStop, newWorkloadNotFoundError(item.CrossClusterObjectNameReference)
		}

		err := releaseControl.Initialize(ctx, wi, ctx.OwnerKind, ctx.OwnerName, rolloutRun.Name)
		if err != nil {
			return false, retryStop, err
		}
	}

	return true, retryDefault, nil
}

func (e *canaryExecutor) doPreStepHook(ctx *ExecutorContext) (bool, time.Duration, error) {
	return e.webhook.Do(ctx, rolloutv1alpha1.PreCanaryStepHook)
}

func (e *canaryExecutor) doPostStepHook(ctx *ExecutorContext) (bool, time.Duration, error) {
	done, retry, err := e.webhook.Do(ctx, rolloutv1alpha1.PostCanaryStepHook)
	if done {
		ctx.Pause()
	}
	return done, retry, err
}

func (e *canaryExecutor) modifyTraffic(ctx *ExecutorContext, op string) (bool, time.Duration) {
	logger := ctx.GetCanaryLogger()
	rolloutRun := ctx.RolloutRun
	opResult := controllerutil.OperationResultNone

	if rolloutRun.Spec.Canary.Traffic == nil {
		logger.Info("traffic is nil, skip modify traffic")
		return true, retryImmediately
	}

	// 1.a. do traffic initialization
	var err error
	switch op {
	case "forkBackends":
		opResult, err = ctx.TrafficManager.ForkBackends()
	case "initializeRoute":
		opResult, err = ctx.TrafficManager.InitializeRoute()
	case "addCanaryRoute":
		opResult, err = ctx.TrafficManager.AddCanaryRoute()
	case "deleteCanaryRoute":
		opResult, err = ctx.TrafficManager.DeleteCanaryRoute()
	case "resetRoute":
		opResult, err = ctx.TrafficManager.ResetRoute()
	case "deleteForkedBackends":
		opResult, err = ctx.TrafficManager.DeleteForkedBackends()
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
	ready := ctx.TrafficManager.CheckReady()
	if !ready {
		logger.Info("waiting for BackendRouting ready")
		return false, retryDefault
	}

	return true, retryImmediately
}

func (e *canaryExecutor) doCanary(ctx *ExecutorContext) (bool, time.Duration, error) {
	logger := ctx.GetCanaryLogger()
	rolloutRun := ctx.RolloutRun

	// 1. fork backends
	prepareDone, retry := e.modifyTraffic(ctx, "forkBackends")
	if !prepareDone {
		return false, retry, nil
	}

	// 2. do traffic initialization
	prepareDone, retry = e.modifyTraffic(ctx, "initializeRoute")
	if !prepareDone {
		return false, retry, nil
	}

	// 2.a. do create canary resources
	logger.Info("about to create canary resources and check")
	canaryWorkloads := make([]*workload.Info, 0)

	patch := appendBuiltinPodTemplateMetadataPatch(rolloutRun.Spec.Canary.TemplateMetadataPatch)

	changed := false
	releaseControl := control.NewCanaryReleaseControl(ctx.Accessor, ctx.Client)

	for _, item := range rolloutRun.Spec.Canary.Targets {
		wi := ctx.Workloads.Get(item.Cluster, item.Name)
		if wi == nil {
			return false, retryStop, newWorkloadNotFoundError(item.CrossClusterObjectNameReference)
		}

		result, canaryInfo, err := releaseControl.CreateOrUpdate(ctx.Context, wi, item.Replicas, patch)
		if err != nil {
			return false, retryStop, err
		}

		if result != controllerutil.OperationResultNone {
			changed = true
		}

		canaryWorkloads = append(canaryWorkloads, canaryInfo)
	}

	if changed {
		return false, retryDefault, nil
	}

	// 2.b. waiting canary workload ready
	for _, info := range canaryWorkloads {
		if !info.CheckUpdatedReady(info.Status.Replicas) {
			// ready
			logger.Info("still waiting for canary target ready",
				"cluster", info.ClusterName,
				"name", info.Name,
				"replicas", info.Status.Replicas,
				"readyReplicas", info.Status.UpdatedAvailableReplicas,
			)
			return false, retryDefault, nil
		}
	}

	// 3. add canary route
	trafficCanaryDone, retry := e.modifyTraffic(ctx, "addCanaryRoute")
	if !trafficCanaryDone {
		return false, retry, nil
	}

	return true, retryImmediately, nil
}

func appendBuiltinPodTemplateMetadataPatch(patch *rolloutv1alpha1.MetadataPatch) *rolloutv1alpha1.MetadataPatch {
	if patch == nil {
		patch = &rolloutv1alpha1.MetadataPatch{}
	}

	if patch.Labels == nil {
		patch.Labels = map[string]string{}
	}

	patch.Labels[rolloutapi.LabelCanary] = "true"
	patch.Labels[rolloutapi.LabelTrafficLane] = rolloutapi.LabelValueTrafficLaneCanary
	return patch
}

func (e *canaryExecutor) release(ctx *ExecutorContext) (bool, time.Duration, error) {
	// firstly try to stop webhook
	e.webhook.Cancel(ctx)

	done, retry := e.modifyTraffic(ctx, "deleteCanaryRoute")
	if !done {
		return false, retry, nil
	}

	rolloutRun := ctx.RolloutRun
	releaseControl := control.NewCanaryReleaseControl(ctx.Accessor, ctx.Client)

	for _, item := range rolloutRun.Spec.Canary.Targets {
		wi := ctx.Workloads.Get(item.Cluster, item.Name)
		if wi == nil {
			return false, retryStop, newWorkloadNotFoundError(item.CrossClusterObjectNameReference)
		}

		if err := releaseControl.Finalize(ctx, wi); err != nil {
			return false, retryStop, newDoCanaryError(
				"FailedFinalize",
				fmt.Sprintf("failed to delete canary resource for workload(%s), err: %v", item.CrossClusterObjectNameReference, err),
			)
		}
	}

	done, retry = e.modifyTraffic(ctx, "resetRoute")
	if !done {
		return false, retry, nil
	}

	done, retry = e.modifyTraffic(ctx, "deleteForkedBackends")
	if !done {
		return false, retry, nil
	}

	return true, retryDefault, nil
}

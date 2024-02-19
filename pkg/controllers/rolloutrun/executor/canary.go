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
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	rolloutapi "kusionstack.io/rollout/apis/rollout"
	rolloutv1alpha1 "kusionstack.io/rollout/apis/rollout/v1alpha1"
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
	logger       logr.Logger
	webhook      webhookExecutor
	stateMachine *stepStateMachine
}

func newCanaryExecutor(logger logr.Logger, webhook webhookExecutor) *canaryExecutor {
	e := &canaryExecutor{
		logger:       logger,
		webhook:      webhook,
		stateMachine: newStepStateMachine(),
	}

	e.stateMachine.add(StepPending, StepPreCanaryStepHook, e.doInit)
	e.stateMachine.add(StepPreCanaryStepHook, StepRunning, e.doPreStepHook)
	e.stateMachine.add(StepRunning, StepPostCanaryStepHook, e.doCanary)
	e.stateMachine.add(StepPostCanaryStepHook, StepResourceRecycling, e.doPostStepHook)
	e.stateMachine.add(StepResourceRecycling, StepSucceeded, skipStep)
	e.stateMachine.add(StepSucceeded, "", skipStep)

	return e
}

func (e *canaryExecutor) loggerWithContext(ctx *ExecutorContext) logr.Logger {
	return ctx.loggerWithContext(e.logger).WithValues("step", "canary")
}

func (e *canaryExecutor) Do(ctx *ExecutorContext) (done bool, result ctrl.Result, err error) {
	if !ctx.inCanary() {
		return true, ctrl.Result{Requeue: true}, nil
	}

	currentState := ctx.NewStatus.CanaryStatus.State

	return e.stateMachine.do(ctx, currentState)
}

func (e *canaryExecutor) doInit(ctx *ExecutorContext) (bool, time.Duration, error) {
	rollout := ctx.Rollout
	rolloutRun := ctx.RolloutRun
	for _, item := range rolloutRun.Spec.Canary.Targets {
		wi := ctx.Workloads.Get(item.Cluster, item.Name)
		if wi == nil {
			return false, retryStop, newDoCanaryError(
				ReasonWorkloadInterfaceNotExist,
				fmt.Sprintf("the workload (%s) does not exists", item.CrossClusterObjectNameReference),
			)
		}

		canaryStrategy, err := wi.CanaryStrategy()
		if err != nil {
			return false, retryStop, newDoCanaryError(
				"InternalError",
				fmt.Sprintf("failed to get canary strategy, err: %v", err),
			)
		}

		err = canaryStrategy.Initialize(rollout.Name, rolloutRun.Name)
		if err != nil {
			return false, retryStop, newDoCanaryError(
				"InitializeFailed",
				fmt.Sprintf("failed to initialize canary strategy, err: %v", err),
			)
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

func (e *canaryExecutor) doCanary(ctx *ExecutorContext) (bool, time.Duration, error) {
	rolloutRun := ctx.RolloutRun
	// 1.a. do traffic initialization

	// 1.b. waiting for traffic

	// 2.a. do create canary resources
	logger := e.loggerWithContext(ctx)
	logger.Info("about to create canary resources and check")

	canaryWorkloads := make([]workload.CanaryStrategy, 0)

	patch := appendBuiltinPodTemplateMetadataPatch(rolloutRun.Spec.Canary.PodTemplateMetadataPatch)

	changed := false

	for _, item := range rolloutRun.Spec.Canary.Targets {
		wi := ctx.Workloads.Get(item.Cluster, item.Name)
		if wi == nil {
			return false, retryStop, newDoCanaryError(
				ReasonWorkloadInterfaceNotExist,
				fmt.Sprintf("the workload (%s) does not exists", item.CrossClusterObjectNameReference),
			)
		}

		canaryStrategy, err := wi.CanaryStrategy()
		if err != nil {
			return false, retryStop, newDoCanaryError(
				"InternalError",
				fmt.Sprintf("failed to get canary strategy, err: %v", err),
			)
		}

		result, err := canaryStrategy.CreateOrUpdate(item.Replicas, patch)
		if err != nil {
			return false, retryStop, newDoCanaryError(
				"CreateOrUpdateFailed",
				fmt.Sprintf("failed to ensure canary resource for workload(%s), err: %v", item.CrossClusterObjectNameReference, err),
			)
		}

		if result != controllerutil.OperationResultNone {
			changed = true
		}

		canaryWorkloads = append(canaryWorkloads, canaryStrategy)
	}

	if changed {
		return false, retryDefault, nil
	}

	// 2.b. waiting canary workload ready
	for _, cw := range canaryWorkloads {
		info := cw.GetCanaryInfo()
		if !info.CheckPartitionReady(info.Status.Replicas) {
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

	// 3.a. do canary traffic routing

	// 3.b. waiting for traffic ready

	// 4. move to next state
	// ctx.MoveToNextState(StepPostCanaryStepHook)
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
	patch.Labels[rolloutapi.LabelPodRevision] = "canary"
	return patch
}

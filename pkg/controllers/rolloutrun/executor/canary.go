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

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	rolloutapi "kusionstack.io/rollout/apis/rollout"
	rolloutv1alpha1 "kusionstack.io/rollout/apis/rollout/v1alpha1"
	"kusionstack.io/rollout/pkg/workload"
)

type canaryExecutor struct {
	logger  logr.Logger
	webhook webhookExecutor
}

func newCanaryExecutor(logger logr.Logger, webhook webhookExecutor) *canaryExecutor {
	return &canaryExecutor{
		logger:  logger,
		webhook: webhook,
	}
}

func (e *canaryExecutor) loggerWithContext(ctx *ExecutorContext) logr.Logger {
	return ctx.loggerWithContext(e.logger).WithValues("step", "canary")
}

func (e *canaryExecutor) Do(ctx *ExecutorContext) (done bool, result ctrl.Result, err error) {
	if !ctx.inCanary() {
		return true, ctrl.Result{Requeue: true}, nil
	}
	newStatus := ctx.NewStatus

	result = ctrl.Result{Requeue: true}

	switch newStatus.CanaryStatus.State {
	case rolloutv1alpha1.RolloutStepPending:
		ctx.MoveToNextState(rolloutv1alpha1.RolloutStepPreCanaryStepHook)
	case rolloutv1alpha1.RolloutStepPreCanaryStepHook:
		var webhookDone bool
		webhookDone, result, err = e.webhook.Do(ctx, rolloutv1alpha1.PreCanaryStepHook)
		if webhookDone {
			ctx.MoveToNextState(rolloutv1alpha1.RolloutStepRunning)
		}
	case rolloutv1alpha1.RolloutStepRunning:
		var canaryDone bool
		canaryDone, result, err = e.doCanary(ctx)
		if canaryDone {
			ctx.MoveToNextState(rolloutv1alpha1.RolloutStepPostCanaryStepHook)
		}
	case rolloutv1alpha1.RolloutStepPostCanaryStepHook:
		var webhookDone bool
		webhookDone, result, err = e.webhook.Do(ctx, rolloutv1alpha1.PostCanaryStepHook)
		if webhookDone {
			ctx.MoveToNextState(rolloutv1alpha1.RolloutStepPaused)
		}
	case rolloutv1alpha1.RolloutStepPaused:
		// waiting for user confirm, do nothing
		result.Requeue = false
	case rolloutv1alpha1.RolloutStepResourceRecycling:
		result.Requeue = true
	case rolloutv1alpha1.RolloutStepSucceeded, rolloutv1alpha1.RolloutStepCanceled:
		done = true
		result.Requeue = false
	}
	return done, result, err
}

func (e *canaryExecutor) doCanary(ctx *ExecutorContext) (bool, ctrl.Result, error) {
	rolloutRun := ctx.RolloutRun
	newStatus := ctx.NewStatus
	// 1.a. do traffic initialization

	// 1.b. waiting for traffic

	// 2.a. do create canary resources
	logger := e.loggerWithContext(ctx)
	logger.Info("about to create canary resources and check")

	canaryWorkloads := make([]workload.Interface, 0)

	patch := appendBuiltinPodTemplateMetadataPatch(rolloutRun.Spec.Canary.PodTemplateMetadataPatch)
	canaryMetadataPatch := &rolloutv1alpha1.MetadataPatch{
		Labels: map[string]string{
			rolloutapi.LabelCanary: "true",
		},
	}
	for _, item := range rolloutRun.Spec.Canary.Targets {
		wi := ctx.Workloads.Get(item.Cluster, item.Name)
		if wi == nil {
			newStatus.Error = newUpgradingError(
				ReasonWorkloadInterfaceNotExist,
				fmt.Sprintf("the workload (%s) does not exists", item.CrossClusterObjectNameReference),
			)
			return false, ctrl.Result{}, errors.New(newStatus.Error.Message)
		}

		canaryWI, err := wi.EnsureCanaryWorkload(item.Replicas, canaryMetadataPatch, patch)
		if err != nil {
			newStatus.Error = newUpgradingError(
				ReasonUpgradePartitionError,
				fmt.Sprintf("failed to ensure canary resource for workload(%s), err: %v", item.CrossClusterObjectNameReference, err),
			)
			return false, ctrl.Result{}, errors.New(newStatus.Error.Message)
		}

		canaryWorkloads = append(canaryWorkloads, canaryWI)
	}

	// 2.b. waiting canary workload ready
	for _, cw := range canaryWorkloads {
		info := cw.GetInfo()
		if !info.CheckPartitionReady(info.Status.Replicas) {
			// ready
			logger.Info("still waiting for canary target ready",
				"cluster", info.ClusterName,
				"name", info.Name,
				"replicas", info.Status.Replicas,
				"readyReplicas", info.Status.UpdatedAvailableReplicas,
			)
			return false, reconcile.Result{RequeueAfter: defaultRequeueAfter}, nil
		}
	}

	// 3.a. do canary traffic routing

	// 3.b. waiting for traffic ready

	// 4. move to next state
	// ctx.MoveToNextState(rolloutv1alpha1.RolloutStepPostCanaryStepHook)
	return true, ctrl.Result{Requeue: true}, nil
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

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
	"errors"
	"fmt"
	"time"

	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	rolloutv1alpha1 "kusionstack.io/kube-api/rollout/v1alpha1"
	ctrl "sigs.k8s.io/controller-runtime"

	"kusionstack.io/rollout/pkg/utils"
)

const (
	retryStop        = time.Duration(-1)
	retryImmediately = time.Duration(0)
	retryDefault     = 5 * time.Second
)

func newUnknownStepStateError(state rolloutv1alpha1.RolloutStepState) *rolloutv1alpha1.CodeReasonMessage {
	return &rolloutv1alpha1.CodeReasonMessage{
		Code:    "Error",
		Reason:  "UnknownStepState",
		Message: fmt.Sprintf("Unknown step state %s in state machine", state),
	}
}

type stateProcess func(*ExecutorContext) (done bool, retry time.Duration, err error)

func skipStep(*ExecutorContext) (bool, time.Duration, error) {
	return true, retryImmediately, nil
}

type stepLifecycle struct {
	current rolloutv1alpha1.RolloutStepState
	next    rolloutv1alpha1.RolloutStepState
	do      stateProcess
	cancel  stateProcess
}

type stepStateEngine struct {
	lifecycle []stepLifecycle
}

func newStepStateEngine() *stepStateEngine {
	return &stepStateEngine{
		lifecycle: make([]stepLifecycle, 0),
	}
}

func (e *stepStateEngine) add(state, nextState rolloutv1alpha1.RolloutStepState, do, cancel stateProcess) {
	if do == nil {
		do = skipStep
	}
	if cancel == nil {
		do = skipStep
	}
	e.lifecycle = append(e.lifecycle, stepLifecycle{
		current: state,
		next:    nextState,
		do:      do,
		cancel:  cancel,
	})
}

func (e *stepStateEngine) cancel(ctx *ExecutorContext, currentState rolloutv1alpha1.RolloutStepState) (done bool, result ctrl.Result, err error) {
	return e.process(ctx, currentState, true)
}

func (e *stepStateEngine) do(ctx *ExecutorContext, currentState rolloutv1alpha1.RolloutStepState) (done bool, result ctrl.Result, err error) {
	return e.process(ctx, currentState, false)
}

func (e *stepStateEngine) process(ctx *ExecutorContext, currentState rolloutv1alpha1.RolloutStepState, cancel bool) (done bool, result ctrl.Result, err error) {
	lifecycle, found := lo.Find(e.lifecycle, func(step stepLifecycle) bool {
		return step.current == currentState
	})

	if !found {
		ctx.Fail(newUnknownStepStateError(currentState))
		return false, ctrl.Result{}, nil
	}

	fn := lifecycle.do
	if cancel {
		fn = lifecycle.cancel
	}
	stateDone, retry, err := fn(ctx)
	if err != nil {
		ctx.Recorder.Eventf(ctx.ScaleRun, corev1.EventTypeWarning, "FailedRunStep", "step failed, currentState %s, err: %v", currentState, err)
		if errors.Is(err, utils.TerminalError(nil)) {
			// we will stop retry if err is CodeReasonMessage
			// TODO: change err to reconcile.TerminalError when controller-runtime supports it
			ctx.Fail(err)
		}
		return false, ctrl.Result{}, err
	}

	if stateDone {
		if cancel {
			// if cancel is true, we stop at this state
			ctx.Recorder.Eventf(ctx.ScaleRun, corev1.EventTypeNormal, "StepCanceled", "step canceled, currentState %s", currentState)
			done = true
		} else if len(lifecycle.next) == 0 {
			// final state
			done = true
		} else {
			done = false
			ctx.MoveToNextState(lifecycle.next)
		}
	}

	switch retry {
	case retryStop:
		result = ctrl.Result{}
	case retryImmediately:
		result = ctrl.Result{Requeue: true}
	default:
		result = ctrl.Result{RequeueAfter: retry}
	}

	return done, result, nil
}

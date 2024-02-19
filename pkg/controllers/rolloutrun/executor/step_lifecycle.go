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

	"github.com/elliotchance/pie/v2"
	ctrl "sigs.k8s.io/controller-runtime"

	rolloutv1alpha1 "kusionstack.io/rollout/apis/rollout/v1alpha1"
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
}

type stepStateMachine struct {
	lifecycle []stepLifecycle
}

func newStepStateMachine() *stepStateMachine {
	return &stepStateMachine{
		lifecycle: make([]stepLifecycle, 0),
	}
}

func (e *stepStateMachine) add(state, nextState rolloutv1alpha1.RolloutStepState, do stateProcess) {
	e.lifecycle = append(e.lifecycle, stepLifecycle{
		current: state,
		next:    nextState,
		do:      do,
	})
}

func (e *stepStateMachine) do(ctx *ExecutorContext, currentState rolloutv1alpha1.RolloutStepState) (done bool, result ctrl.Result, err error) {
	index := pie.FindFirstUsing(e.lifecycle, func(step stepLifecycle) bool {
		return step.current == currentState
	})

	if index == -1 {
		ctx.Fail(newUnknownStepStateError(currentState))
		return false, ctrl.Result{}, nil
	}

	lifecycle := e.lifecycle[index]

	stateDone, retry, err := lifecycle.do(ctx)
	if err != nil {
		ctx.Fail(err)
		return false, ctrl.Result{}, nil
	}

	if stateDone {
		if len(lifecycle.next) == 0 {
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

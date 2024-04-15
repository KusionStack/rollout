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

import rolloutv1alpha1 "kusionstack.io/rollout/apis/rollout/v1alpha1"

const (
	StepNone               = rolloutv1alpha1.RolloutStepNone
	StepPending            = rolloutv1alpha1.RolloutStepPending
	StepPreCanaryStepHook  = rolloutv1alpha1.RolloutStepPreCanaryStepHook
	StepPreBatchStepHook   = rolloutv1alpha1.RolloutStepPreBatchStepHook
	StepRunning            = rolloutv1alpha1.RolloutStepRunning
	StepPostCanaryStepHook = rolloutv1alpha1.RolloutStepPostCanaryStepHook
	StepPostBatchStepHook  = rolloutv1alpha1.RolloutStepPostBatchStepHook
	StepSucceeded          = rolloutv1alpha1.RolloutStepSucceeded
	StepResourceRecycling  = rolloutv1alpha1.RolloutStepResourceRecycling
)

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
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExecutorContext_SkipCurrentRelease(t *testing.T) {
	ror := testCanaryRolloutRun.DeepCopy()
	ctx := ExecutorContext{
		RolloutRun: ror,
	}
	// skip canary release
	ctx.SkipCurrentRelease()
	if assert.NotNil(t, ctx.NewStatus.CanaryStatus) {
		canaryStatus := ctx.NewStatus.CanaryStatus
		assert.Equal(t, StepSucceeded, canaryStatus.State)
		assert.NotNil(t, canaryStatus.StartTime)
		assert.NotNil(t, canaryStatus.FinishTime)
	}

	// skip batch release
	ctx.SkipCurrentRelease()

	if assert.NotNil(t, ctx.NewStatus.BatchStatus) {
		batchStatus := ctx.NewStatus.BatchStatus
		assert.Len(t, batchStatus.Records, len(ror.Spec.Batch.Batches))
		assert.Equal(t, StepSucceeded, batchStatus.CurrentBatchState)
		assert.EqualValues(t, len(batchStatus.Records)-1, batchStatus.CurrentBatchIndex)
		for i := range batchStatus.Records {
			assert.Equal(t, StepSucceeded, batchStatus.Records[i].State)
			assert.NotNil(t, batchStatus.Records[i].StartTime)
			assert.NotNil(t, batchStatus.Records[i].FinishTime)
		}
	}
}

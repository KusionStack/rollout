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
	"github.com/stretchr/testify/suite"
	rolloutv1alpha1 "kusionstack.io/kube-api/rollout/v1alpha1"
)

type executorContextTestSuite struct {
	suite.Suite

	rolloutRun *rolloutv1alpha1.RolloutRun
	context    *ExecutorContext
}

func (s *executorContextTestSuite) SetupSuite() {
	s.rolloutRun = testCanaryRolloutRun.DeepCopy()
}

func (s *executorContextTestSuite) SetupTest() {
	s.context = &ExecutorContext{
		RolloutRun: s.rolloutRun,
	}
}

func (s *executorContextTestSuite) TestExecutorContext_SkipCurrentRelease() {
	ctx := s.context
	// skip canary release
	ctx.SkipCurrentRelease()
	if s.NotNil(ctx.NewStatus.CanaryStatus) {
		canaryStatus := ctx.NewStatus.CanaryStatus
		s.Equal(StepSucceeded, canaryStatus.State)
		s.NotNil(canaryStatus.StartTime)
		s.NotNil(canaryStatus.FinishTime)
	}

	// skip batch release
	ctx.SkipCurrentRelease()

	if s.NotNil(ctx.NewStatus.BatchStatus) {
		batchStatus := ctx.NewStatus.BatchStatus
		s.Len(batchStatus.Records, len(s.rolloutRun.Spec.Batch.Batches))
		s.Equal(StepSucceeded, batchStatus.CurrentBatchState)
		s.EqualValues(len(batchStatus.Records)-1, batchStatus.CurrentBatchIndex)
		for i := range batchStatus.Records {
			s.Equal(StepSucceeded, batchStatus.Records[i].State)
			s.NotNil(batchStatus.Records[i].StartTime)
			s.NotNil(batchStatus.Records[i].FinishTime)
		}
	}
}

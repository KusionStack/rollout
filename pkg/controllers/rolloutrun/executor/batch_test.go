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
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	rolloutv1alpha1 "kusionstack.io/rollout/apis/rollout/v1alpha1"
	"kusionstack.io/rollout/pkg/workload"
	"kusionstack.io/rollout/pkg/workload/fake"
)

func newTestBatchExecutor(webhook webhookExecutor) *batchExecutor {
	return newBatchExecutor(newTestLogger(), webhook)
}

func Test_BatchExecutor_Do(t *testing.T) {
	tests := []struct {
		name         string
		getObjects   func() (*rolloutv1alpha1.Rollout, *rolloutv1alpha1.RolloutRun)
		getWorkloads func() *workload.Set
		assertResult func(assert *assert.Assertions, done bool, result reconcile.Result, err error)
		assertStatus func(assert *assert.Assertions, status *rolloutv1alpha1.RolloutRunStatus)
	}{
		{
			name: "Pending to PreBatchStepHook",
			getObjects: func() (*rolloutv1alpha1.Rollout, *rolloutv1alpha1.RolloutRun) {
				rollout := testRollout.DeepCopy()
				rolloutRun := testRolloutRun.DeepCopy()

				rolloutRun.Spec.Batch.Batches = []rolloutv1alpha1.RolloutRunStep{{
					Targets: unimportantTargets,
				}}
				rolloutRun.Status.BatchStatus = &rolloutv1alpha1.RolloutRunBatchStatus{
					RolloutBatchStatus: rolloutv1alpha1.RolloutBatchStatus{
						CurrentBatchState: rolloutv1alpha1.RolloutStepPending,
					},
					Records: []rolloutv1alpha1.RolloutRunStepStatus{
						{
							State: rolloutv1alpha1.RolloutStepPending,
						},
					},
				}
				return rollout, rolloutRun
			},
			assertResult: func(assert *assert.Assertions, done bool, result reconcile.Result, err error) {
				assert.False(done)
				assert.Nil(err)
				assert.Equal(reconcile.Result{Requeue: true}, result)
			},
			assertStatus: func(assert *assert.Assertions, status *rolloutv1alpha1.RolloutRunStatus) {
				assert.EqualValues(rolloutv1alpha1.RolloutStepPreBatchStepHook, status.BatchStatus.CurrentBatchState)
				assert.EqualValues(rolloutv1alpha1.RolloutStepPreBatchStepHook, status.BatchStatus.Records[0].State)
			},
		},
		{
			name: "Pending to Paused",
			getObjects: func() (*rolloutv1alpha1.Rollout, *rolloutv1alpha1.RolloutRun) {
				rollout := testRollout.DeepCopy()
				rolloutRun := testRolloutRun.DeepCopy()

				rolloutRun.Spec.Batch.Batches = []rolloutv1alpha1.RolloutRunStep{{
					Breakpoint: true,
					Targets:    unimportantTargets,
				}}
				rolloutRun.Status.BatchStatus = &rolloutv1alpha1.RolloutRunBatchStatus{
					RolloutBatchStatus: rolloutv1alpha1.RolloutBatchStatus{
						CurrentBatchState: rolloutv1alpha1.RolloutStepPending,
					},
					Records: []rolloutv1alpha1.RolloutRunStepStatus{
						{
							State: rolloutv1alpha1.RolloutStepPending,
						},
					},
				}
				return rollout, rolloutRun
			},
			assertResult: func(assert *assert.Assertions, done bool, result reconcile.Result, err error) {
				assert.False(done)
				assert.Nil(err)
				assert.Equal(reconcile.Result{Requeue: true}, result)
			},
			assertStatus: func(assert *assert.Assertions, status *rolloutv1alpha1.RolloutRunStatus) {
				assert.EqualValues(rolloutv1alpha1.RolloutStepPaused, status.BatchStatus.CurrentBatchState)
				assert.EqualValues(rolloutv1alpha1.RolloutStepPaused, status.BatchStatus.Records[0].State)
			},
		},
		{
			name: "Paused do nothing",
			getObjects: func() (*rolloutv1alpha1.Rollout, *rolloutv1alpha1.RolloutRun) {
				rollout := testRollout.DeepCopy()
				rolloutRun := testRolloutRun.DeepCopy()

				rolloutRun.Spec.Batch.Batches = []rolloutv1alpha1.RolloutRunStep{{
					Breakpoint: true,
					Targets:    unimportantTargets,
				}}
				rolloutRun.Status.BatchStatus = &rolloutv1alpha1.RolloutRunBatchStatus{
					RolloutBatchStatus: rolloutv1alpha1.RolloutBatchStatus{
						CurrentBatchState: rolloutv1alpha1.RolloutStepPaused,
					},
					Records: []rolloutv1alpha1.RolloutRunStepStatus{
						{
							State: rolloutv1alpha1.RolloutStepPaused,
						},
					},
				}
				return rollout, rolloutRun
			},
			assertResult: func(assert *assert.Assertions, done bool, result reconcile.Result, err error) {
				assert.False(done)
				assert.Nil(err)
				assert.Equal(reconcile.Result{}, result)
			},
			assertStatus: func(assert *assert.Assertions, status *rolloutv1alpha1.RolloutRunStatus) {
				assert.EqualValues(rolloutv1alpha1.RolloutStepPaused, status.BatchStatus.CurrentBatchState)
				assert.EqualValues(rolloutv1alpha1.RolloutStepPaused, status.BatchStatus.Records[0].State)
			},
		},
		{
			name: "PreBatchStepHook to Running",
			getObjects: func() (*rolloutv1alpha1.Rollout, *rolloutv1alpha1.RolloutRun) {
				rollout := testRollout.DeepCopy()
				rolloutRun := testRolloutRun.DeepCopy()

				rolloutRun.Spec.Batch.Batches = []rolloutv1alpha1.RolloutRunStep{{
					Breakpoint: true,
					Targets:    unimportantTargets,
				}}
				rolloutRun.Status.BatchStatus = &rolloutv1alpha1.RolloutRunBatchStatus{
					RolloutBatchStatus: rolloutv1alpha1.RolloutBatchStatus{
						CurrentBatchState: rolloutv1alpha1.RolloutStepPreBatchStepHook,
					},
					Records: []rolloutv1alpha1.RolloutRunStepStatus{
						{
							State: rolloutv1alpha1.RolloutStepPreBatchStepHook,
						},
					},
				}
				return rollout, rolloutRun
			},
			assertResult: func(assert *assert.Assertions, done bool, result reconcile.Result, err error) {
				assert.False(done)
				assert.Nil(err)
				assert.Equal(reconcile.Result{Requeue: true}, result)
			},
			assertStatus: func(assert *assert.Assertions, status *rolloutv1alpha1.RolloutRunStatus) {
				assert.EqualValues(rolloutv1alpha1.RolloutStepRunning, status.BatchStatus.CurrentBatchState)
				assert.EqualValues(rolloutv1alpha1.RolloutStepRunning, status.BatchStatus.Records[0].State)
			},
		},
		{
			name: "Running to PostBatchStepHook",
			getObjects: func() (*rolloutv1alpha1.Rollout, *rolloutv1alpha1.RolloutRun) {
				rollout := testRollout.DeepCopy()
				rolloutRun := testRolloutRun.DeepCopy()

				rolloutRun.Spec.Batch.Batches = []rolloutv1alpha1.RolloutRunStep{{
					Targets: []rolloutv1alpha1.RolloutRunStepTarget{
						newRunStepTarget("cluster-a", "test-0", intstr.FromInt(100)),
					},
				}}
				rolloutRun.Status.BatchStatus = &rolloutv1alpha1.RolloutRunBatchStatus{
					RolloutBatchStatus: rolloutv1alpha1.RolloutBatchStatus{
						CurrentBatchState: rolloutv1alpha1.RolloutStepRunning,
					},
					Records: []rolloutv1alpha1.RolloutRunStepStatus{
						{
							State: rolloutv1alpha1.RolloutStepRunning,
						},
					},
				}
				return rollout, rolloutRun
			},
			getWorkloads: func() *workload.Set {
				return workload.NewWorkloadSet(
					fake.New("cluster-a", "default", "test-0").ChangeStatus(100, 100, 100),
				)
			},
			assertResult: func(assert *assert.Assertions, done bool, result reconcile.Result, err error) {
				assert.False(done)
				assert.Nil(err)
				assert.Equal(reconcile.Result{Requeue: true}, result)
			},
			assertStatus: func(assert *assert.Assertions, status *rolloutv1alpha1.RolloutRunStatus) {
				assert.EqualValues(rolloutv1alpha1.PostBatchStepHook, status.BatchStatus.CurrentBatchState)
				assert.EqualValues(rolloutv1alpha1.PostBatchStepHook, status.BatchStatus.Records[0].State)
			},
		},
		{
			name: "PostBatchStepHook to Succeeded",
			getObjects: func() (*rolloutv1alpha1.Rollout, *rolloutv1alpha1.RolloutRun) {
				rollout := testRollout.DeepCopy()
				rolloutRun := testRolloutRun.DeepCopy()

				rolloutRun.Spec.Batch.Batches = []rolloutv1alpha1.RolloutRunStep{{
					Targets: unimportantTargets,
				}}
				rolloutRun.Status.BatchStatus = &rolloutv1alpha1.RolloutRunBatchStatus{
					RolloutBatchStatus: rolloutv1alpha1.RolloutBatchStatus{
						CurrentBatchState: rolloutv1alpha1.RolloutStepPostBatchStepHook,
					},
					Records: []rolloutv1alpha1.RolloutRunStepStatus{
						{
							State: rolloutv1alpha1.RolloutStepPostBatchStepHook,
						},
					},
				}
				return rollout, rolloutRun
			},
			assertResult: func(assert *assert.Assertions, done bool, result reconcile.Result, err error) {
				assert.False(done)
				assert.Nil(err)
				assert.Equal(reconcile.Result{Requeue: true}, result)
			},
			assertStatus: func(assert *assert.Assertions, status *rolloutv1alpha1.RolloutRunStatus) {
				assert.EqualValues(rolloutv1alpha1.RolloutStepSucceeded, status.BatchStatus.CurrentBatchState)
				assert.EqualValues(rolloutv1alpha1.RolloutStepSucceeded, status.BatchStatus.Records[0].State)
			},
		},
		{
			name: "Succeeded to next batch",
			getObjects: func() (*rolloutv1alpha1.Rollout, *rolloutv1alpha1.RolloutRun) {
				rollout := testRollout.DeepCopy()
				rolloutRun := testRolloutRun.DeepCopy()

				rolloutRun.Spec.Batch.Batches = []rolloutv1alpha1.RolloutRunStep{
					{
						Targets: unimportantTargets,
					},
					{
						Targets: unimportantTargets,
					},
				}
				rolloutRun.Status.BatchStatus = &rolloutv1alpha1.RolloutRunBatchStatus{
					RolloutBatchStatus: rolloutv1alpha1.RolloutBatchStatus{
						CurrentBatchState: rolloutv1alpha1.RolloutStepSucceeded,
					},
					Records: []rolloutv1alpha1.RolloutRunStepStatus{
						{
							State: rolloutv1alpha1.RolloutStepSucceeded,
						},
					},
				}
				return rollout, rolloutRun
			},
			assertResult: func(assert *assert.Assertions, done bool, result reconcile.Result, err error) {
				assert.False(done)
				assert.Nil(err)
				assert.Equal(reconcile.Result{Requeue: true}, result)
			},
			assertStatus: func(assert *assert.Assertions, status *rolloutv1alpha1.RolloutRunStatus) {
				assert.EqualValues(rolloutv1alpha1.RolloutStepPending, status.BatchStatus.CurrentBatchState)
				assert.EqualValues(1, status.BatchStatus.CurrentBatchIndex)
				assert.EqualValues(rolloutv1alpha1.RolloutStepPending, status.BatchStatus.Records[1].State)
			},
		},
		{
			name: "Succeeded to done",
			getObjects: func() (*rolloutv1alpha1.Rollout, *rolloutv1alpha1.RolloutRun) {
				rollout := testRollout.DeepCopy()
				rolloutRun := testRolloutRun.DeepCopy()

				rolloutRun.Spec.Batch.Batches = []rolloutv1alpha1.RolloutRunStep{
					{
						Targets: unimportantTargets,
					},
				}
				rolloutRun.Status.BatchStatus = &rolloutv1alpha1.RolloutRunBatchStatus{
					RolloutBatchStatus: rolloutv1alpha1.RolloutBatchStatus{
						CurrentBatchState: rolloutv1alpha1.RolloutStepSucceeded,
					},
					Records: []rolloutv1alpha1.RolloutRunStepStatus{
						{
							State: rolloutv1alpha1.RolloutStepSucceeded,
						},
					},
				}
				return rollout, rolloutRun
			},
			assertResult: func(assert *assert.Assertions, done bool, result reconcile.Result, err error) {
				assert.True(done)
				assert.Nil(err)
				assert.Equal(reconcile.Result{Requeue: true}, result)
			},
			assertStatus: func(assert *assert.Assertions, status *rolloutv1alpha1.RolloutRunStatus) {
				assert.EqualValues(rolloutv1alpha1.RolloutStepSucceeded, status.BatchStatus.CurrentBatchState)
				assert.EqualValues(rolloutv1alpha1.RolloutStepSucceeded, status.BatchStatus.Records[0].State)
			},
		},
	}

	executor := newTestBatchExecutor(newFakeWebhookExecutor())
	for i := range tests {
		tt := tests[i]
		t.Run(tt.name, func(t *testing.T) {
			rollout, rolloutRun := tt.getObjects()
			var workloads *workload.Set
			if tt.getWorkloads != nil {
				workloads = tt.getWorkloads()
			}
			ctx := createTestExecutorContext(rollout, rolloutRun, workloads)
			done, got, err := executor.Do(ctx)
			assert := assert.New(t)
			tt.assertResult(assert, done, got, err)
			tt.assertStatus(assert, ctx.NewStatus)
		})
	}
}
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
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	rolloutv1alpha1 "kusionstack.io/rollout/apis/rollout/v1alpha1"
	"kusionstack.io/rollout/pkg/workload"
)

var (
	unimportantTargets = []rolloutv1alpha1.RolloutRunStepTarget{
		newRunStepTarget("cluster-a", "test-0", intstr.FromInt(1)),
	}

	unimportantWorkloads = func() []client.Object {
		return []client.Object{
			newFakeObject("cluster-a", "default", "test-0", 1, 1, 1),
		}
	}
)

func newTestBatchExecutor(webhook webhookExecutor) *batchExecutor {
	return newBatchExecutor(webhook)
}

type batchExectorTestCase struct {
	name            string
	getObjects      func() (*rolloutv1alpha1.Rollout, *rolloutv1alpha1.RolloutRun)
	getWorkloads    func() []client.Object
	assertResult    func(assert *assert.Assertions, done bool, result reconcile.Result, err error)
	assertStatus    func(assert *assert.Assertions, status *rolloutv1alpha1.RolloutRunStatus)
	assertWorkloads func(assert *assert.Assertions, objects []client.Object)
}

func runBatchTestCases(t *testing.T, tests []batchExectorTestCase) {
	executor := newTestBatchExecutor(newFakeWebhookExecutor())
	for i := range tests {
		tt := tests[i]
		t.Run(tt.name, func(t *testing.T) {
			rollout, rolloutRun := tt.getObjects()
			var objs []client.Object
			if tt.getWorkloads != nil {
				objs = tt.getWorkloads()
			}
			ctx := createTestExecutorContext(rollout, rolloutRun, objs...)
			done, got, err := executor.Do(ctx)
			assert := assert.New(t)
			tt.assertResult(assert, done, got, err)

			if tt.assertStatus != nil {
				tt.assertStatus(assert, ctx.NewStatus)
			}
			if len(objs) > 0 && tt.assertWorkloads != nil {
				tt.assertWorkloads(assert, objs)
			}
		})
	}
}

func Test_BatchExecutor_Do(t *testing.T) {
	tests := []batchExectorTestCase{
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
						CurrentBatchState: StepPending,
					},
					Records: []rolloutv1alpha1.RolloutRunStepStatus{
						{
							State: StepPending,
						},
					},
				}
				return rollout, rolloutRun
			},
			getWorkloads: unimportantWorkloads,
			assertResult: func(assert *assert.Assertions, done bool, result reconcile.Result, err error) {
				assert.False(done)
				assert.Nil(err)
				assert.Equal(reconcile.Result{Requeue: true}, result)
			},
			assertStatus: func(assert *assert.Assertions, status *rolloutv1alpha1.RolloutRunStatus) {
				assert.EqualValues(StepPreBatchStepHook, status.BatchStatus.CurrentBatchState)
				assert.EqualValues(StepPreBatchStepHook, status.BatchStatus.Records[0].State)
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
						CurrentBatchState: StepPreBatchStepHook,
					},
					Records: []rolloutv1alpha1.RolloutRunStepStatus{
						{
							State: StepPreBatchStepHook,
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
				assert.EqualValues(StepRunning, status.BatchStatus.CurrentBatchState)
				assert.EqualValues(StepRunning, status.BatchStatus.Records[0].State)
			},
		},
		{
			name: "Running to PostBatchStepHook",
			getObjects: func() (*rolloutv1alpha1.Rollout, *rolloutv1alpha1.RolloutRun) {
				rollout := testRollout.DeepCopy()
				rolloutRun := testRolloutRun.DeepCopy()

				rolloutRun.Spec.Batch.Batches = []rolloutv1alpha1.RolloutRunStep{{
					Targets: []rolloutv1alpha1.RolloutRunStepTarget{
						newRunStepTarget("cluster-a", "test-0", intstr.FromInt(10)),
						newRunStepTarget("cluster-a", "test-1", intstr.FromInt(100)),
					},
				}}
				rolloutRun.Status.BatchStatus = &rolloutv1alpha1.RolloutRunBatchStatus{
					RolloutBatchStatus: rolloutv1alpha1.RolloutBatchStatus{
						CurrentBatchState: StepRunning,
					},
					Records: []rolloutv1alpha1.RolloutRunStepStatus{
						{
							State: StepRunning,
						},
					},
				}
				return rollout, rolloutRun
			},
			getWorkloads: func() []client.Object {
				return []client.Object{
					newFakeObject("cluster-a", "default", "test-0", 100, 10, 10),
					newFakeObject("cluster-a", "default", "test-1", 100, 100, 100),
				}
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
			name: "PostBatchStepHook to Recycling",
			getObjects: func() (*rolloutv1alpha1.Rollout, *rolloutv1alpha1.RolloutRun) {
				rollout := testRollout.DeepCopy()
				rolloutRun := testRolloutRun.DeepCopy()

				rolloutRun.Spec.Batch.Batches = []rolloutv1alpha1.RolloutRunStep{{
					Targets: unimportantTargets,
				}}
				rolloutRun.Status.BatchStatus = &rolloutv1alpha1.RolloutRunBatchStatus{
					RolloutBatchStatus: rolloutv1alpha1.RolloutBatchStatus{
						CurrentBatchState: StepPostBatchStepHook,
					},
					Records: []rolloutv1alpha1.RolloutRunStepStatus{
						{
							State: StepPostBatchStepHook,
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
				assert.EqualValues(StepResourceRecycling, status.BatchStatus.CurrentBatchState)
				assert.EqualValues(StepResourceRecycling, status.BatchStatus.Records[0].State)
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
						CurrentBatchState: StepSucceeded,
					},
					Records: []rolloutv1alpha1.RolloutRunStepStatus{
						{
							State: StepSucceeded,
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
				assert.EqualValues(StepNone, status.BatchStatus.CurrentBatchState)
				assert.EqualValues(1, status.BatchStatus.CurrentBatchIndex)
				assert.EqualValues(StepNone, status.BatchStatus.Records[1].State)
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
						CurrentBatchState: StepSucceeded,
					},
					Records: []rolloutv1alpha1.RolloutRunStepStatus{
						{
							State: StepSucceeded,
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
				assert.EqualValues(StepSucceeded, status.BatchStatus.CurrentBatchState)
				assert.EqualValues(StepSucceeded, status.BatchStatus.Records[0].State)
			},
		},
	}

	runBatchTestCases(t, tests)
}

func newRunStepTarget(cluster, name string, replicas intstr.IntOrString) rolloutv1alpha1.RolloutRunStepTarget {
	return rolloutv1alpha1.RolloutRunStepTarget{
		CrossClusterObjectNameReference: rolloutv1alpha1.CrossClusterObjectNameReference{
			Cluster: cluster,
			Name:    name,
		},
		Replicas: replicas,
	}
}

func Test_BatchExecutor_Do_Pending_Paused(t *testing.T) {
	tests := []batchExectorTestCase{
		{
			name: "None to Pending(Paused)",
			getObjects: func() (*rolloutv1alpha1.Rollout, *rolloutv1alpha1.RolloutRun) {
				rollout := testRollout.DeepCopy()
				rolloutRun := testRolloutRun.DeepCopy()

				rolloutRun.Spec.Batch.Batches = []rolloutv1alpha1.RolloutRunStep{
					{
						Breakpoint: true,
						Targets: []rolloutv1alpha1.RolloutRunStepTarget{
							newRunStepTarget("cluster-a", "test-0", intstr.FromInt(10)),
						},
					},
					{
						Targets: []rolloutv1alpha1.RolloutRunStepTarget{
							newRunStepTarget("cluster-a", "test-1", intstr.FromInt(10)),
						},
					},
				}
				rolloutRun.Status.BatchStatus = &rolloutv1alpha1.RolloutRunBatchStatus{
					RolloutBatchStatus: rolloutv1alpha1.RolloutBatchStatus{
						CurrentBatchState: StepNone,
					},
					Records: []rolloutv1alpha1.RolloutRunStepStatus{
						{
							State: StepNone,
						},
					},
				}
				return rollout, rolloutRun
			},
			getWorkloads: func() []client.Object {
				return []client.Object{
					newFakeObject("cluster-a", "default", "test-0", 10, 10, 10),
					newFakeObject("cluster-a", "default", "test-1", 10, 10, 10),
				}
			},
			assertResult: func(assert *assert.Assertions, done bool, result reconcile.Result, err error) {
				assert.False(done)
				assert.Nil(err)
				assert.Equal(reconcile.Result{Requeue: true}, result)
			},
			assertStatus: func(assert *assert.Assertions, status *rolloutv1alpha1.RolloutRunStatus) {
				assert.EqualValues(rolloutv1alpha1.RolloutRunPhasePaused, status.Phase)
				assert.EqualValues(StepPending, status.BatchStatus.CurrentBatchState)
				assert.EqualValues(StepPending, status.BatchStatus.Records[0].State)
			},
			assertWorkloads: func(assert *assert.Assertions, objects []client.Object) {
				assert.Len(objects, 2)
				for _, obj := range objects {
					if obj.GetName() == "test-0" {
						assert.True(workload.IsProgressing(obj))
					} else if obj.GetName() == "test-1" {
						assert.False(workload.IsProgressing(obj))
					}
				}
			},
		},
		{
			name: "None to Pending",
			getObjects: func() (*rolloutv1alpha1.Rollout, *rolloutv1alpha1.RolloutRun) {
				rollout := testRollout.DeepCopy()
				rolloutRun := testRolloutRun.DeepCopy()

				rolloutRun.Spec.Batch.Batches = []rolloutv1alpha1.RolloutRunStep{
					{
						Targets: []rolloutv1alpha1.RolloutRunStepTarget{
							newRunStepTarget("cluster-a", "test-0", intstr.FromInt(10)),
						},
					},
					{
						Targets: []rolloutv1alpha1.RolloutRunStepTarget{
							newRunStepTarget("cluster-a", "test-1", intstr.FromInt(10)),
						},
					},
				}
				rolloutRun.Status.Phase = rolloutv1alpha1.RolloutRunPhaseProgressing
				rolloutRun.Status.BatchStatus = &rolloutv1alpha1.RolloutRunBatchStatus{
					RolloutBatchStatus: rolloutv1alpha1.RolloutBatchStatus{
						CurrentBatchIndex: 0,
						CurrentBatchState: StepNone,
					},
					Records: []rolloutv1alpha1.RolloutRunStepStatus{
						{
							State: StepNone,
						},
					},
				}
				return rollout, rolloutRun
			},
			getWorkloads: func() []client.Object {
				return []client.Object{
					newFakeObject("cluster-a", "default", "test-0", 10, 10, 10),
					newFakeObject("cluster-a", "default", "test-1", 10, 10, 10),
				}
			},
			assertResult: func(assert *assert.Assertions, done bool, result reconcile.Result, err error) {
				assert.False(done)
				assert.Nil(err)
				assert.Equal(reconcile.Result{Requeue: true}, result)
			},
			assertStatus: func(assert *assert.Assertions, status *rolloutv1alpha1.RolloutRunStatus) {
				assert.EqualValues(rolloutv1alpha1.RolloutRunPhaseProgressing, status.Phase)
				assert.EqualValues(StepPending, status.BatchStatus.CurrentBatchState)
				assert.EqualValues(StepPending, status.BatchStatus.Records[0].State)
			},
			assertWorkloads: func(assert *assert.Assertions, objects []client.Object) {
				assert.Len(objects, 2)
				for _, obj := range objects {
					if obj.GetName() == "test-0" {
						assert.True(workload.IsProgressing(obj))
					} else if obj.GetName() == "test-1" {
						assert.False(workload.IsProgressing(obj))
					}
				}
			},
		},
	}

	runBatchTestCases(t, tests)
}

func Test_BatchExecutor_Do_Running(t *testing.T) {
	tests := []batchExectorTestCase{
		{
			name: "batch target is empty",
			getObjects: func() (*rolloutv1alpha1.Rollout, *rolloutv1alpha1.RolloutRun) {
				rollout := testRollout.DeepCopy()
				rolloutRun := testRolloutRun.DeepCopy()

				rolloutRun.Spec.Batch.Batches = []rolloutv1alpha1.RolloutRunStep{{
					Targets: nil,
				}}

				rolloutRun.Status.BatchStatus = &rolloutv1alpha1.RolloutRunBatchStatus{
					RolloutBatchStatus: rolloutv1alpha1.RolloutBatchStatus{
						CurrentBatchState: StepRunning,
					},
					Records: []rolloutv1alpha1.RolloutRunStepStatus{
						{
							State: StepRunning,
						},
					},
				}
				return rollout, rolloutRun
			},
			assertResult: func(assert *assert.Assertions, done bool, result reconcile.Result, err error) {
				assert.Nil(err)
				assert.Equal(reconcile.Result{Requeue: true}, result)
				assert.False(done)
			},
			assertStatus: func(assert *assert.Assertions, status *rolloutv1alpha1.RolloutRunStatus) {
				assert.Nil(status.Error)
			},
		},
		{
			name: "workflow instance not found",
			getObjects: func() (*rolloutv1alpha1.Rollout, *rolloutv1alpha1.RolloutRun) {
				rollout := testRollout.DeepCopy()
				rolloutRun := testRolloutRun.DeepCopy()

				rolloutRun.Spec.Batch.Batches = []rolloutv1alpha1.RolloutRunStep{{
					Targets: []rolloutv1alpha1.RolloutRunStepTarget{
						newRunStepTarget("non-exist", "non-exits", intstr.FromInt(1)),
					},
				}}

				rolloutRun.Status.Phase = rolloutv1alpha1.RolloutRunPhaseProgressing
				rolloutRun.Status.BatchStatus = &rolloutv1alpha1.RolloutRunBatchStatus{
					RolloutBatchStatus: rolloutv1alpha1.RolloutBatchStatus{
						CurrentBatchIndex: 0,
						CurrentBatchState: StepRunning,
					},
					Records: []rolloutv1alpha1.RolloutRunStepStatus{
						{
							Index:     ptr.To[int32](0),
							State:     StepRunning,
							StartTime: ptr.To(metav1.Now()),
						},
					},
				}
				return rollout, rolloutRun
			},
			assertResult: func(assert *assert.Assertions, done bool, result reconcile.Result, err error) {
				assert.Equal(reconcile.Result{}, result)
				assert.False(done)
			},
		},
		{
			name: "upgrade workload and then requeue after 5 seconds",
			getObjects: func() (*rolloutv1alpha1.Rollout, *rolloutv1alpha1.RolloutRun) {
				rollout := testRollout.DeepCopy()
				rolloutRun := testRolloutRun.DeepCopy()

				// setup rolloutRun
				rolloutRun.Spec.Batch.Batches = []rolloutv1alpha1.RolloutRunStep{{
					Targets: []rolloutv1alpha1.RolloutRunStepTarget{
						newRunStepTarget("cluster-a", "test-a", intstr.FromInt(10)),
						newRunStepTarget("cluster-a", "test-b", intstr.FromInt(10)),
					},
				}}
				rolloutRun.Status.Phase = rolloutv1alpha1.RolloutRunPhaseProgressing
				rolloutRun.Status.BatchStatus = &rolloutv1alpha1.RolloutRunBatchStatus{
					RolloutBatchStatus: rolloutv1alpha1.RolloutBatchStatus{
						CurrentBatchIndex: 0,
						CurrentBatchState: StepRunning,
					},
					Records: []rolloutv1alpha1.RolloutRunStepStatus{
						{
							Index:     ptr.To[int32](0),
							State:     StepRunning,
							StartTime: ptr.To(metav1.Now()),
						},
					},
				}
				return rollout, rolloutRun
			},
			getWorkloads: func() []client.Object {
				return []client.Object{
					newFakeObject("cluster-a", "default", "test-a", 100, 0, 0),
					newFakeObject("cluster-a", "default", "test-b", 100, 0, 0),
				}
			},
			assertResult: func(assert *assert.Assertions, done bool, result reconcile.Result, err error) {
				assert.Nil(err)
				assert.Equal(reconcile.Result{RequeueAfter: retryDefault}, result)
				assert.False(done)
			},
			assertStatus: func(assert *assert.Assertions, status *rolloutv1alpha1.RolloutRunStatus) {
				assert.Len(status.BatchStatus.Records, 1)
				assert.Len(status.BatchStatus.Records[0].Targets, 2)

				for _, target := range status.BatchStatus.Records[0].Targets {
					assert.EqualValues(100, target.Replicas)
					assert.EqualValues(0, target.UpdatedReplicas)
					assert.EqualValues(0, target.UpdatedReadyReplicas)
					assert.EqualValues(0, target.UpdatedAvailableReplicas)
				}
			},
			assertWorkloads: func(assert *assert.Assertions, objs []client.Object) {
				for _, obj := range objs {
					if assert.IsType(&appsv1.StatefulSet{}, obj) {
						sts := obj.(*appsv1.StatefulSet)
						assert.NotNil(sts.Spec.UpdateStrategy.RollingUpdate)
						assert.NotNil(sts.Spec.UpdateStrategy.RollingUpdate.Partition)
						assert.EqualValues(90, *sts.Spec.UpdateStrategy.RollingUpdate.Partition)
					}
				}
			},
		},
		{
			name: "waiting for workload ready",
			getObjects: func() (*rolloutv1alpha1.Rollout, *rolloutv1alpha1.RolloutRun) {
				rollout := testRollout.DeepCopy()
				rolloutRun := testRolloutRun.DeepCopy()

				// setup rolloutRun
				rolloutRun.Spec.Batch.Batches = []rolloutv1alpha1.RolloutRunStep{{
					Targets: []rolloutv1alpha1.RolloutRunStepTarget{
						newRunStepTarget("cluster-a", "test-a", intstr.FromInt(10)),
						newRunStepTarget("cluster-a", "test-b", intstr.FromInt(10)),
					},
				}}
				rolloutRun.Status.Phase = rolloutv1alpha1.RolloutRunPhaseProgressing
				rolloutRun.Status.BatchStatus = &rolloutv1alpha1.RolloutRunBatchStatus{
					RolloutBatchStatus: rolloutv1alpha1.RolloutBatchStatus{
						CurrentBatchIndex: 0,
						CurrentBatchState: StepRunning,
					},
					Records: []rolloutv1alpha1.RolloutRunStepStatus{
						{
							Index:     ptr.To[int32](0),
							State:     StepRunning,
							StartTime: ptr.To(metav1.Now()),
						},
					},
				}
				return rollout, rolloutRun
			},
			getWorkloads: func() []client.Object {
				return []client.Object{
					newFakeObject("cluster-a", "default", "test-a", 100, 10, 1),
					newFakeObject("cluster-a", "default", "test-b", 100, 20, 10),
				}
			},
			assertResult: func(assert *assert.Assertions, done bool, result reconcile.Result, err error) {
				assert.Nil(err)
				assert.Equal(reconcile.Result{RequeueAfter: retryDefault}, result)
				assert.False(done)
			},
			assertStatus: func(assert *assert.Assertions, status *rolloutv1alpha1.RolloutRunStatus) {
				assert.Len(status.BatchStatus.Records, 1)
				assert.Len(status.BatchStatus.Records[0].Targets, 2)
				assert.EqualValues(100, status.BatchStatus.Records[0].Targets[0].Replicas)
				assert.EqualValues(1, status.BatchStatus.Records[0].Targets[0].UpdatedAvailableReplicas)
			},
			assertWorkloads: func(assert *assert.Assertions, objs []client.Object) {
				for _, obj := range objs {
					if assert.IsType(&appsv1.StatefulSet{}, obj) {
						sts := obj.(*appsv1.StatefulSet)
						switch sts.Name {
						case "test-a":
							assert.NotNil(sts.Spec.UpdateStrategy.RollingUpdate)
							assert.NotNil(sts.Spec.UpdateStrategy.RollingUpdate.Partition)
							assert.EqualValues(90, *sts.Spec.UpdateStrategy.RollingUpdate.Partition)
						case "test-b":
							assert.NotNil(sts.Spec.UpdateStrategy.RollingUpdate)
							assert.NotNil(sts.Spec.UpdateStrategy.RollingUpdate.Partition)
							// partition will not be changed because this object is already achieved the desired state
							assert.EqualValues(80, *sts.Spec.UpdateStrategy.RollingUpdate.Partition)
						}
					}
				}
			},
		},
		{
			name: "all workload ready, move to next state",
			getObjects: func() (*rolloutv1alpha1.Rollout, *rolloutv1alpha1.RolloutRun) {
				rollout := testRollout.DeepCopy()
				rolloutRun := testRolloutRun.DeepCopy()

				// setup rolloutRun
				rolloutRun.Spec.Batch.Batches = []rolloutv1alpha1.RolloutRunStep{{
					Targets: []rolloutv1alpha1.RolloutRunStepTarget{
						newRunStepTarget("cluster-a", "test-a", intstr.FromInt(10)),
						newRunStepTarget("cluster-b", "test-b", intstr.FromInt(10)),
						newRunStepTarget("cluster-c", "test-c", intstr.FromInt(10)),
					},
				}}
				rolloutRun.Status.Phase = rolloutv1alpha1.RolloutRunPhaseProgressing
				rolloutRun.Status.BatchStatus = &rolloutv1alpha1.RolloutRunBatchStatus{
					RolloutBatchStatus: rolloutv1alpha1.RolloutBatchStatus{
						CurrentBatchIndex: 0,
						CurrentBatchState: StepRunning,
					},
					Records: []rolloutv1alpha1.RolloutRunStepStatus{
						{
							Index:     ptr.To[int32](0),
							State:     StepRunning,
							StartTime: ptr.To(metav1.Now()),
						},
					},
				}

				return rollout, rolloutRun
			},
			getWorkloads: func() []client.Object {
				return []client.Object{
					newFakeObject("cluster-a", "default", "test-a", 100, 10, 10),
					newFakeObject("cluster-b", "default", "test-b", 100, 20, 10),
					// this object's partition will be nil
					newFakeObject("cluster-c", "default", "test-c", 100, 100, 10),
				}
			},
			assertResult: func(assert *assert.Assertions, done bool, result reconcile.Result, err error) {
				assert.Nil(err)
				assert.Equal(reconcile.Result{Requeue: true}, result)
				assert.False(done)
			},
			assertStatus: func(assert *assert.Assertions, status *rolloutv1alpha1.RolloutRunStatus) {
				// check records
				assert.Len(status.BatchStatus.Records, 1)
				// check targets
				assert.Len(status.BatchStatus.Records[0].Targets, 3)

				for _, target := range status.BatchStatus.Records[0].Targets {
					assert.EqualValues(100, target.Replicas)
					assert.EqualValues(10, target.UpdatedAvailableReplicas)
				}
				// check state
				assert.Equal(StepPostBatchStepHook, status.BatchStatus.CurrentBatchState)
				assert.Equal(StepPostBatchStepHook, status.BatchStatus.Records[0].State)
			},
			assertWorkloads: func(assert *assert.Assertions, objs []client.Object) {
				for _, obj := range objs {
					if assert.IsType(&appsv1.StatefulSet{}, obj) {
						sts := obj.(*appsv1.StatefulSet)
						switch sts.Name {
						case "test-a":
							assert.NotNil(sts.Spec.UpdateStrategy.RollingUpdate)
							assert.NotNil(sts.Spec.UpdateStrategy.RollingUpdate.Partition)
							assert.EqualValues(90, *sts.Spec.UpdateStrategy.RollingUpdate.Partition)
						case "test-b":
							assert.NotNil(sts.Spec.UpdateStrategy.RollingUpdate)
							assert.NotNil(sts.Spec.UpdateStrategy.RollingUpdate.Partition)
							assert.EqualValues(80, *sts.Spec.UpdateStrategy.RollingUpdate.Partition)
						case "test-c":
							assert.Nil(sts.Spec.UpdateStrategy.RollingUpdate)
						}
					}
				}
			},
		},
	}

	runBatchTestCases(t, tests)
}

func Test_BatchExecutor_Do_Recycling(t *testing.T) {
	tests := []batchExectorTestCase{
		{
			name: "Recycling to Succeeded",
			getObjects: func() (*rolloutv1alpha1.Rollout, *rolloutv1alpha1.RolloutRun) {
				rollout := testRollout.DeepCopy()
				rolloutRun := testRolloutRun.DeepCopy()

				rolloutRun.Spec.Batch.Batches = []rolloutv1alpha1.RolloutRunStep{
					{
						Targets: []rolloutv1alpha1.RolloutRunStepTarget{
							newRunStepTarget("cluster-a", "test-0", intstr.FromInt(10)),
						},
					},
					{
						Targets: []rolloutv1alpha1.RolloutRunStepTarget{
							newRunStepTarget("cluster-a", "test-1", intstr.FromInt(10)),
						},
					},
				}
				rolloutRun.Status.BatchStatus = &rolloutv1alpha1.RolloutRunBatchStatus{
					RolloutBatchStatus: rolloutv1alpha1.RolloutBatchStatus{
						CurrentBatchState: StepResourceRecycling,
					},
					Records: []rolloutv1alpha1.RolloutRunStepStatus{
						{
							State: StepResourceRecycling,
						},
					},
				}
				return rollout, rolloutRun
			},
			getWorkloads: func() []client.Object {
				return []client.Object{
					withProgressingInfo(newFakeObject("cluster-a", "default", "test-0", 10, 10, 10)),
					withProgressingInfo(newFakeObject("cluster-a", "default", "test-1", 10, 0, 0)),
				}
			},
			assertResult: func(assert *assert.Assertions, done bool, result reconcile.Result, err error) {
				assert.False(done)
				assert.Nil(err)
				assert.Equal(reconcile.Result{Requeue: true}, result)
			},
			assertStatus: func(assert *assert.Assertions, status *rolloutv1alpha1.RolloutRunStatus) {
				assert.EqualValues(StepSucceeded, status.BatchStatus.CurrentBatchState)
				assert.EqualValues(StepSucceeded, status.BatchStatus.Records[0].State)
			},
			assertWorkloads: func(assert *assert.Assertions, objs []client.Object) {
				for _, obj := range objs {
					assert.True(workload.IsProgressing(obj))
				}
			},
		},
		{
			name: "Recycling to Succeeded and finalize all workload",
			getObjects: func() (*rolloutv1alpha1.Rollout, *rolloutv1alpha1.RolloutRun) {
				rollout := testRollout.DeepCopy()
				rolloutRun := testRolloutRun.DeepCopy()

				rolloutRun.Spec.Batch.Batches = []rolloutv1alpha1.RolloutRunStep{
					{
						Targets: []rolloutv1alpha1.RolloutRunStepTarget{
							newRunStepTarget("cluster-a", "test-0", intstr.FromInt(10)),
						},
					},
					{
						Targets: []rolloutv1alpha1.RolloutRunStepTarget{
							newRunStepTarget("cluster-a", "test-1", intstr.FromInt(10)),
						},
					},
				}
				rolloutRun.Status.BatchStatus = &rolloutv1alpha1.RolloutRunBatchStatus{
					RolloutBatchStatus: rolloutv1alpha1.RolloutBatchStatus{
						CurrentBatchIndex: 1,
						CurrentBatchState: StepResourceRecycling,
					},
					Records: []rolloutv1alpha1.RolloutRunStepStatus{
						{
							State: StepSucceeded,
						},
						{
							State: StepResourceRecycling,
						},
					},
				}
				return rollout, rolloutRun
			},
			getWorkloads: func() []client.Object {
				return []client.Object{
					withProgressingInfo(newFakeObject("cluster-a", "default", "test-0", 10, 10, 10)),
					withProgressingInfo(newFakeObject("cluster-a", "default", "test-1", 10, 10, 10)),
				}
			},
			assertResult: func(assert *assert.Assertions, done bool, result reconcile.Result, err error) {
				assert.False(done)
				assert.Nil(err)
				assert.Equal(reconcile.Result{Requeue: true}, result)
			},
			assertStatus: func(assert *assert.Assertions, status *rolloutv1alpha1.RolloutRunStatus) {
				assert.EqualValues(StepSucceeded, status.BatchStatus.CurrentBatchState)
				assert.EqualValues(StepSucceeded, status.BatchStatus.Records[0].State)
			},
			assertWorkloads: func(assert *assert.Assertions, objs []client.Object) {
				for _, obj := range objs {
					assert.False(workload.IsProgressing(obj))
				}
			},
		},
	}

	runBatchTestCases(t, tests)
}

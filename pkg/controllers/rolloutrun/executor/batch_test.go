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
	"time"

	"github.com/stretchr/testify/suite"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	rolloutv1alpha1 "kusionstack.io/kube-api/rollout/v1alpha1"
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

type fakeWebhookExecutor struct{}

func (e *fakeWebhookExecutor) Do(ctx *ExecutorContext, hookType rolloutv1alpha1.HookType) (bool, time.Duration, error) {
	return true, retryImmediately, nil
}

func (e *fakeWebhookExecutor) Cancel(ctx *ExecutorContext) {
}

type batchExecutorTestSuite struct {
	suite.Suite

	executor *batchExecutor

	rollout    *rolloutv1alpha1.Rollout
	rolloutRun *rolloutv1alpha1.RolloutRun
}

func (s *batchExecutorTestSuite) SetupSuite() {
	s.executor = newBatchExecutor(&fakeWebhookExecutor{})
}

func (s *batchExecutorTestSuite) SetupTest() {
	s.rollout = testRollout.DeepCopy()
	s.rolloutRun = testRolloutRun.DeepCopy()
}

func (s *batchExecutorTestSuite) runBatchTestCases(tests []batchExectorTestCase) {
	for i := range tests {
		tt := tests[i]
		s.Run(tt.name, func() {
			rollout, rolloutRun := tt.getObjects()
			var objs []client.Object
			if tt.getWorkloads != nil {
				objs = tt.getWorkloads()
			}
			ctx := createTestExecutorContext(rollout, rolloutRun, objs...)
			done, got, err := s.executor.Do(ctx)
			tt.assertResult(done, got, err)

			if tt.assertStatus != nil {
				tt.assertStatus(ctx.NewStatus)
			}
			if len(objs) > 0 && tt.assertWorkloads != nil {
				tt.assertWorkloads(objs)
			}
		})
	}
}

type batchExectorTestCase struct {
	name            string
	getObjects      func() (*rolloutv1alpha1.Rollout, *rolloutv1alpha1.RolloutRun)
	getWorkloads    func() []client.Object
	assertResult    func(done bool, result reconcile.Result, err error)
	assertStatus    func(status *rolloutv1alpha1.RolloutRunStatus)
	assertWorkloads func(objects []client.Object)
}

func (s *batchExecutorTestSuite) Test_BatchExecutor_Do() {
	tests := []batchExectorTestCase{
		{
			name: "Pending to PreBatchStepHook",
			getObjects: func() (*rolloutv1alpha1.Rollout, *rolloutv1alpha1.RolloutRun) {
				rollout := s.rollout.DeepCopy()
				rolloutRun := s.rolloutRun.DeepCopy()

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
			assertResult: func(done bool, result reconcile.Result, err error) {
				s.Require().NoError(err)
				s.False(done)
				s.Equal(reconcile.Result{Requeue: true}, result)
			},
			assertStatus: func(status *rolloutv1alpha1.RolloutRunStatus) {
				s.Equal(StepPreBatchStepHook, status.BatchStatus.CurrentBatchState)
				s.Equal(StepPreBatchStepHook, status.BatchStatus.Records[0].State)
			},
		},
		{
			name: "PreBatchStepHook to Running",
			getObjects: func() (*rolloutv1alpha1.Rollout, *rolloutv1alpha1.RolloutRun) {
				rollout := s.rollout.DeepCopy()
				rolloutRun := s.rolloutRun.DeepCopy()

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
			assertResult: func(done bool, result reconcile.Result, err error) {
				s.Require().NoError(err)
				s.False(done)
				s.Equal(reconcile.Result{Requeue: true}, result)
			},
			assertStatus: func(status *rolloutv1alpha1.RolloutRunStatus) {
				s.Equal(StepRunning, status.BatchStatus.CurrentBatchState)
				s.Equal(StepRunning, status.BatchStatus.Records[0].State)
			},
		},
		{
			name: "Running to PostBatchStepHook",
			getObjects: func() (*rolloutv1alpha1.Rollout, *rolloutv1alpha1.RolloutRun) {
				rollout := s.rollout.DeepCopy()
				rolloutRun := s.rolloutRun.DeepCopy()

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
			assertResult: func(done bool, result reconcile.Result, err error) {
				s.Require().NoError(err)
				s.False(done)
				s.Equal(reconcile.Result{Requeue: true}, result)
			},
			assertStatus: func(status *rolloutv1alpha1.RolloutRunStatus) {
				s.EqualValues(rolloutv1alpha1.PostBatchStepHook, status.BatchStatus.CurrentBatchState)
				s.EqualValues(rolloutv1alpha1.PostBatchStepHook, status.BatchStatus.Records[0].State)
			},
		},
		{
			name: "PostBatchStepHook to Recycling",
			getObjects: func() (*rolloutv1alpha1.Rollout, *rolloutv1alpha1.RolloutRun) {
				rollout := s.rollout.DeepCopy()
				rolloutRun := s.rolloutRun.DeepCopy()

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
			assertResult: func(done bool, result reconcile.Result, err error) {
				s.Require().NoError(err)
				s.False(done)
				s.Equal(reconcile.Result{Requeue: true}, result)
			},
			assertStatus: func(status *rolloutv1alpha1.RolloutRunStatus) {
				s.Equal(StepResourceRecycling, status.BatchStatus.CurrentBatchState)
				s.Equal(StepResourceRecycling, status.BatchStatus.Records[0].State)
			},
		},
		{
			name: "Succeeded to next batch",
			getObjects: func() (*rolloutv1alpha1.Rollout, *rolloutv1alpha1.RolloutRun) {
				rollout := s.rollout.DeepCopy()
				rolloutRun := s.rolloutRun.DeepCopy()

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
			assertResult: func(done bool, result reconcile.Result, err error) {
				s.Require().NoError(err)
				s.False(done)
				s.Equal(reconcile.Result{Requeue: true}, result)
			},
			assertStatus: func(status *rolloutv1alpha1.RolloutRunStatus) {
				s.Equal(StepNone, status.BatchStatus.CurrentBatchState)
				s.EqualValues(1, status.BatchStatus.CurrentBatchIndex)
				s.Equal(StepNone, status.BatchStatus.Records[1].State)
			},
		},
		{
			name: "Succeeded to done",
			getObjects: func() (*rolloutv1alpha1.Rollout, *rolloutv1alpha1.RolloutRun) {
				rollout := s.rollout.DeepCopy()
				rolloutRun := s.rolloutRun.DeepCopy()

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
			assertResult: func(done bool, result reconcile.Result, err error) {
				s.Require().NoError(err)
				s.True(done)
				s.Equal(reconcile.Result{Requeue: true}, result)
			},
			assertStatus: func(status *rolloutv1alpha1.RolloutRunStatus) {
				s.Equal(StepSucceeded, status.BatchStatus.CurrentBatchState)
				s.Equal(StepSucceeded, status.BatchStatus.Records[0].State)
			},
		},
	}

	s.runBatchTestCases(tests)
}

func newRunStepTarget(cluster, name string, replicas intstr.IntOrString) rolloutv1alpha1.RolloutRunStepTarget {
	return newRunStepTargetWithSlidingWindow(cluster, name, replicas, nil)
}

func newRunStepTargetWithSlidingWindow(cluster, name string, replicas intstr.IntOrString, window *intstr.IntOrString) rolloutv1alpha1.RolloutRunStepTarget {
	return rolloutv1alpha1.RolloutRunStepTarget{
		CrossClusterObjectNameReference: rolloutv1alpha1.CrossClusterObjectNameReference{
			Cluster: cluster,
			Name:    name,
		},
		Replicas:             replicas,
		ReplicaSlidingWindow: window,
	}
}

func (s *batchExecutorTestSuite) Test_BatchExecutor_Do_Pending_Paused() {
	tests := []batchExectorTestCase{
		{
			name: "None to Pending(Paused)",
			getObjects: func() (*rolloutv1alpha1.Rollout, *rolloutv1alpha1.RolloutRun) {
				rollout := s.rollout.DeepCopy()
				rolloutRun := s.rolloutRun.DeepCopy()

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
			assertResult: func(done bool, result reconcile.Result, err error) {
				s.Require().NoError(err)
				s.False(done)
				s.Equal(reconcile.Result{Requeue: true}, result)
			},
			assertStatus: func(status *rolloutv1alpha1.RolloutRunStatus) {
				s.Equal(rolloutv1alpha1.RolloutRunPhasePaused, status.Phase)
				s.Equal(StepPending, status.BatchStatus.CurrentBatchState)
				s.Equal(StepPending, status.BatchStatus.Records[0].State)
			},
			assertWorkloads: func(objects []client.Object) {
				s.Len(objects, 2)
				for _, obj := range objects {
					if obj.GetName() == "test-0" {
						s.True(workload.IsProgressing(obj))
					} else if obj.GetName() == "test-1" {
						s.False(workload.IsProgressing(obj))
					}
				}
			},
		},
		{
			name: "None to Pending",
			getObjects: func() (*rolloutv1alpha1.Rollout, *rolloutv1alpha1.RolloutRun) {
				rollout := s.rollout.DeepCopy()
				rolloutRun := s.rolloutRun.DeepCopy()

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
			assertResult: func(done bool, result reconcile.Result, err error) {
				s.Require().NoError(err)
				s.False(done)
				s.Equal(reconcile.Result{Requeue: true}, result)
			},
			assertStatus: func(status *rolloutv1alpha1.RolloutRunStatus) {
				s.Equal(rolloutv1alpha1.RolloutRunPhaseProgressing, status.Phase)
				s.Equal(StepPending, status.BatchStatus.CurrentBatchState)
				s.Equal(StepPending, status.BatchStatus.Records[0].State)
			},
			assertWorkloads: func(objects []client.Object) {
				s.Len(objects, 2)
				for _, obj := range objects {
					if obj.GetName() == "test-0" {
						s.True(workload.IsProgressing(obj))
					} else if obj.GetName() == "test-1" {
						s.False(workload.IsProgressing(obj))
					}
				}
			},
		},
	}

	s.runBatchTestCases(tests)
}

func (s *batchExecutorTestSuite) Test_BatchExecutor_Do_Running() {
	tests := []batchExectorTestCase{
		{
			name: "batch target is empty",
			getObjects: func() (*rolloutv1alpha1.Rollout, *rolloutv1alpha1.RolloutRun) {
				rollout := s.rollout.DeepCopy()
				rolloutRun := s.rolloutRun.DeepCopy()

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
			assertResult: func(done bool, result reconcile.Result, err error) {
				s.Require().NoError(err)
				s.Equal(reconcile.Result{Requeue: true}, result)
				s.False(done)
			},
			assertStatus: func(status *rolloutv1alpha1.RolloutRunStatus) {
				s.Nil(status.Error)
			},
		},
		{
			name: "workload instance not found",
			getObjects: func() (*rolloutv1alpha1.Rollout, *rolloutv1alpha1.RolloutRun) {
				rollout := s.rollout.DeepCopy()
				rolloutRun := s.rolloutRun.DeepCopy()

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
			assertResult: func(done bool, result reconcile.Result, err error) {
				s.Equal(reconcile.Result{}, result)
				s.False(done)
			},
		},
		{
			name: "upgrade workload and then requeue after 5 seconds",
			getObjects: func() (*rolloutv1alpha1.Rollout, *rolloutv1alpha1.RolloutRun) {
				rollout := s.rollout.DeepCopy()
				rolloutRun := s.rolloutRun.DeepCopy()

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
			assertResult: func(done bool, result reconcile.Result, err error) {
				s.Require().NoError(err)
				s.Equal(reconcile.Result{RequeueAfter: retryDefault}, result)
				s.False(done)
			},
			assertStatus: func(status *rolloutv1alpha1.RolloutRunStatus) {
				s.Len(status.BatchStatus.Records, 1)
				s.Len(status.BatchStatus.Records[0].Targets, 2)

				for _, target := range status.BatchStatus.Records[0].Targets {
					s.EqualValues(100, target.Replicas)
					s.EqualValues(0, target.UpdatedReplicas)
					s.EqualValues(0, target.UpdatedReadyReplicas)
					s.EqualValues(0, target.UpdatedAvailableReplicas)
				}
			},
			assertWorkloads: func(objs []client.Object) {
				for _, obj := range objs {
					if s.IsType(&appsv1.StatefulSet{}, obj) {
						sts := obj.(*appsv1.StatefulSet)
						s.Require().NotNil(sts.Spec.UpdateStrategy.RollingUpdate)
						s.Require().NotNil(sts.Spec.UpdateStrategy.RollingUpdate.Partition)
						s.EqualValues(90, *sts.Spec.UpdateStrategy.RollingUpdate.Partition)
					}
				}
			},
		},
		{
			name: "upgrade workload partition by sliding window",
			getObjects: func() (*rolloutv1alpha1.Rollout, *rolloutv1alpha1.RolloutRun) {
				rollout := s.rollout.DeepCopy()
				rolloutRun := s.rolloutRun.DeepCopy()

				// setup rolloutRun
				rolloutRun.Spec.Batch.Batches = []rolloutv1alpha1.RolloutRunStep{{
					Targets: []rolloutv1alpha1.RolloutRunStepTarget{
						// test-a with normal sliding window
						newRunStepTargetWithSlidingWindow("cluster-a", "test-a", intstr.FromInt(50), ptr.To(intstr.FromInt(10))),
						// test-b with a too big sliding window
						newRunStepTargetWithSlidingWindow("cluster-a", "test-b", intstr.FromInt(10), ptr.To(intstr.FromInt(50))),
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
					newFakeObject("cluster-a", "default", "test-a", 100, 20, 15),
					newFakeObject("cluster-a", "default", "test-b", 100, 0, 0),
				}
			},
			assertResult: func(done bool, result reconcile.Result, err error) {
				s.Require().NoError(err)
				s.Equal(reconcile.Result{RequeueAfter: retryDefault}, result)
				s.False(done)
			},
			assertStatus: func(status *rolloutv1alpha1.RolloutRunStatus) {
				s.Len(status.BatchStatus.Records, 1)
				s.Len(status.BatchStatus.Records[0].Targets, 2)

				for _, target := range status.BatchStatus.Records[0].Targets {
					s.EqualValues(100, target.Replicas)
					switch target.Name {
					case "test-a":
						s.EqualValues(15, target.UpdatedReplicas)
						s.EqualValues(15, target.UpdatedReadyReplicas)
						s.EqualValues(15, target.UpdatedAvailableReplicas)
					case "test-b":
						s.EqualValues(0, target.UpdatedReplicas)
						s.EqualValues(0, target.UpdatedReadyReplicas)
						s.EqualValues(0, target.UpdatedAvailableReplicas)
					}
				}
			},
			assertWorkloads: func(objs []client.Object) {
				for _, obj := range objs {
					if s.IsType(&appsv1.StatefulSet{}, obj) {
						sts := obj.(*appsv1.StatefulSet)
						s.Require().NotNil(sts.Spec.UpdateStrategy.RollingUpdate)
						s.Require().NotNil(sts.Spec.UpdateStrategy.RollingUpdate.Partition)
						switch sts.Name {
						case "test-a":
							// partition = total(100) - (updatedReplicas(15) + slidingWindow(10) ) = 75
							s.EqualValues(75, *sts.Spec.UpdateStrategy.RollingUpdate.Partition)
						case "test-b":
							s.EqualValues(90, *sts.Spec.UpdateStrategy.RollingUpdate.Partition)
						}
					}
				}
			},
		},
		{
			name: "waiting for workload ready",
			getObjects: func() (*rolloutv1alpha1.Rollout, *rolloutv1alpha1.RolloutRun) {
				rollout := s.rollout.DeepCopy()
				rolloutRun := s.rolloutRun.DeepCopy()

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
			assertResult: func(done bool, result reconcile.Result, err error) {
				s.Require().NoError(err)
				s.Equal(reconcile.Result{RequeueAfter: retryDefault}, result)
				s.False(done)
			},
			assertStatus: func(status *rolloutv1alpha1.RolloutRunStatus) {
				s.Len(status.BatchStatus.Records, 1)
				s.Len(status.BatchStatus.Records[0].Targets, 2)
				s.EqualValues(100, status.BatchStatus.Records[0].Targets[0].Replicas)
				s.EqualValues(1, status.BatchStatus.Records[0].Targets[0].UpdatedAvailableReplicas)
			},
			assertWorkloads: func(objs []client.Object) {
				for _, obj := range objs {
					if s.IsType(&appsv1.StatefulSet{}, obj) {
						sts := obj.(*appsv1.StatefulSet)
						switch sts.Name {
						case "test-a":
							s.NotNil(sts.Spec.UpdateStrategy.RollingUpdate)
							s.NotNil(sts.Spec.UpdateStrategy.RollingUpdate.Partition)
							s.EqualValues(90, *sts.Spec.UpdateStrategy.RollingUpdate.Partition)
						case "test-b":
							s.NotNil(sts.Spec.UpdateStrategy.RollingUpdate)
							s.NotNil(sts.Spec.UpdateStrategy.RollingUpdate.Partition)
							// partition will not be changed because this object is already achieved the desired state
							s.EqualValues(80, *sts.Spec.UpdateStrategy.RollingUpdate.Partition)
						}
					}
				}
			},
		},
		{
			name: "all workload ready, move to next state",
			getObjects: func() (*rolloutv1alpha1.Rollout, *rolloutv1alpha1.RolloutRun) {
				rollout := s.rollout.DeepCopy()
				rolloutRun := s.rolloutRun.DeepCopy()

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
			assertResult: func(done bool, result reconcile.Result, err error) {
				s.Require().NoError(err)
				s.Equal(reconcile.Result{Requeue: true}, result)
				s.False(done)
			},
			assertStatus: func(status *rolloutv1alpha1.RolloutRunStatus) {
				// check records
				s.Len(status.BatchStatus.Records, 1)
				// check targets
				s.Len(status.BatchStatus.Records[0].Targets, 3)

				for _, target := range status.BatchStatus.Records[0].Targets {
					s.EqualValues(100, target.Replicas)
					s.EqualValues(10, target.UpdatedAvailableReplicas)
				}
				// check state
				s.Equal(StepPostBatchStepHook, status.BatchStatus.CurrentBatchState)
				s.Equal(StepPostBatchStepHook, status.BatchStatus.Records[0].State)
			},
			assertWorkloads: func(objs []client.Object) {
				for _, obj := range objs {
					if s.IsType(&appsv1.StatefulSet{}, obj) {
						sts := obj.(*appsv1.StatefulSet)
						switch sts.Name {
						case "test-a":
							s.NotNil(sts.Spec.UpdateStrategy.RollingUpdate)
							s.NotNil(sts.Spec.UpdateStrategy.RollingUpdate.Partition)
							s.EqualValues(90, *sts.Spec.UpdateStrategy.RollingUpdate.Partition)
						case "test-b":
							s.NotNil(sts.Spec.UpdateStrategy.RollingUpdate)
							s.NotNil(sts.Spec.UpdateStrategy.RollingUpdate.Partition)
							s.EqualValues(80, *sts.Spec.UpdateStrategy.RollingUpdate.Partition)
						case "test-c":
							s.Nil(sts.Spec.UpdateStrategy.RollingUpdate)
						}
					}
				}
			},
		},
	}

	s.runBatchTestCases(tests)
}

func (s *batchExecutorTestSuite) Test_BatchExecutor_Do_Recycling() {
	tests := []batchExectorTestCase{
		{
			name: "Recycling to Succeeded",
			getObjects: func() (*rolloutv1alpha1.Rollout, *rolloutv1alpha1.RolloutRun) {
				rollout := s.rollout.DeepCopy()
				rolloutRun := s.rolloutRun.DeepCopy()

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
			assertResult: func(done bool, result reconcile.Result, err error) {
				s.Require().NoError(err)
				s.False(done)
				s.Equal(reconcile.Result{Requeue: true}, result)
			},
			assertStatus: func(status *rolloutv1alpha1.RolloutRunStatus) {
				s.Equal(StepSucceeded, status.BatchStatus.CurrentBatchState)
				s.Equal(StepSucceeded, status.BatchStatus.Records[0].State)
			},
			assertWorkloads: func(objs []client.Object) {
				for _, obj := range objs {
					s.True(workload.IsProgressing(obj))
				}
			},
		},
		{
			name: "Recycling to Succeeded and finalize all workload",
			getObjects: func() (*rolloutv1alpha1.Rollout, *rolloutv1alpha1.RolloutRun) {
				rollout := s.rollout.DeepCopy()
				rolloutRun := s.rolloutRun.DeepCopy()

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
			assertResult: func(done bool, result reconcile.Result, err error) {
				s.Require().NoError(err)
				s.False(done)
				s.Equal(reconcile.Result{Requeue: true}, result)
			},
			assertStatus: func(status *rolloutv1alpha1.RolloutRunStatus) {
				s.Equal(StepSucceeded, status.BatchStatus.CurrentBatchState)
				s.Equal(StepSucceeded, status.BatchStatus.Records[0].State)
			},
			assertWorkloads: func(objs []client.Object) {
				for _, obj := range objs {
					s.False(workload.IsProgressing(obj))
				}
			},
		},
	}

	s.runBatchTestCases(tests)
}

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
	"k8s.io/utils/ptr"
	rolloutv1alpha1 "kusionstack.io/kube-api/rollout/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	rorexecutor "kusionstack.io/rollout/pkg/controllers/rolloutrun/executor"
	"kusionstack.io/rollout/pkg/workload"
)

// fakeWebhookExecutor is a no-op webhook executor used by the batch test suite.
// It reports every hook as immediately completed so the state machine can
// advance through PreBatchStepHook / PostBatchStepHook transitions.
type fakeWebhookExecutor struct{}

func (e *fakeWebhookExecutor) Do(ctx *ExecutorContext, hookType rolloutv1alpha1.HookType) (bool, time.Duration, error) {
	return true, retryImmediately, nil
}

func (e *fakeWebhookExecutor) Cancel(ctx *ExecutorContext) {}

type batchExecutorTestSuite struct {
	suite.Suite

	executor *batchExecutor
	scaleRun *rolloutv1alpha1.ScaleRun
}

func (s *batchExecutorTestSuite) SetupSuite() {
	s.executor = newBatchExecutor(&fakeWebhookExecutor{})
}

func (s *batchExecutorTestSuite) SetupTest() {
	s.scaleRun = testScaleRun.DeepCopy()
}

// runBatchTestCases executes each table-driven batch executor testcase and
// invokes the provided assert hooks for done/result/err, NewStatus, and
// (optionally) the workload objects post-reconcile.
func (s *batchExecutorTestSuite) runBatchTestCases(tests []scaleBatchTestCase) {
	for i := range tests {
		tt := tests[i]
		s.Run(tt.name, func() {
			scaleRun := tt.getObjects()
			var objs []client.Object
			if tt.getWorkloads != nil {
				objs = tt.getWorkloads()
			}
			ctx := createTestScaleExecutorContext(scaleRun, objs...)
			done, got, err := s.executor.Do(ctx)
			tt.assertResult(done, got, err)

			if tt.assertStatus != nil {
				tt.assertStatus(ctx.NewStatus)
			}
			if len(objs) > 0 && tt.assertWorkloads != nil {
				newObjs := []client.Object{}
				for _, info := range ctx.Workloads.ToSlice() {
					newObjs = append(newObjs, info.Object)
				}
				tt.assertWorkloads(newObjs)
			}
		})
	}
}

type scaleBatchTestCase struct {
	name            string
	getObjects      func() *rolloutv1alpha1.ScaleRun
	getWorkloads    func() []client.Object
	assertResult    func(done bool, result reconcile.Result, err error)
	assertStatus    func(status *rolloutv1alpha1.ScaleRunStatus)
	assertWorkloads func(objects []client.Object)
}

// newScaleRunStepTarget builds a ScaleRunStepTarget with the given replicas and
// no toleration.
func newScaleRunStepTarget(cluster, name string, replicas int32) rolloutv1alpha1.ScaleRunStepTarget {
	return rolloutv1alpha1.ScaleRunStepTarget{
		CrossClusterObjectNameReference: rolloutv1alpha1.CrossClusterObjectNameReference{
			Cluster: cluster,
			Name:    name,
		},
		Replicas: replicas,
	}
}

// newScaleRunStepTargetWithToleration builds a ScaleRunStepTarget with a
// toleration. Toleration only applies for scale-up scenarios (ScaleFrom < ScaleTo).
func newScaleRunStepTargetWithToleration(cluster, name string, replicas int32, toleration *rolloutv1alpha1.RolloutStepTargetToleration) rolloutv1alpha1.ScaleRunStepTarget {
	target := newScaleRunStepTarget(cluster, name, replicas)
	target.Toleration = toleration
	return target
}

// Test_BatchExecutor_Do_SkipToleration covers the auto-skip toleration
// scenarios for ScaleRun (design scenarios 6 & 7), plus the negative cases that
// must NOT trigger auto-skip:
//   - scale-down: toleration does not apply (scenario 6: no skip)
//   - no toleration / nil FailureThreshold: behavior unchanged
//   - gap exceeds FailureThreshold: keep waiting (scenario 2)
//   - Generation mismatch: transient state, not skippable
//   - middle-batch auto-skip when gap within threshold & delay elapsed (scenario 7)
//   - last-batch auto-skip transitions toward success (scenario 7 on last batch)
func (s *batchExecutorTestSuite) Test_BatchExecutor_Do_SkipToleration() {
	tests := []scaleBatchTestCase{
		{
			name: "auto-skip applies in middle batch when gap within threshold and delay elapsed",
			getObjects: func() *rolloutv1alpha1.ScaleRun {
				scaleRun := s.scaleRun.DeepCopy()
				// 3 batches, currently on batch index 1 (middle, scale-up)
				scaleRun.Spec.Batch.Batches = []rolloutv1alpha1.ScaleRunStep{
					{Targets: []rolloutv1alpha1.ScaleRunStepTarget{
						newScaleRunStepTarget("cluster-a", "test-a", 20),
					}},
					{Targets: []rolloutv1alpha1.ScaleRunStepTarget{
						newScaleRunStepTargetWithToleration("cluster-a", "test-a", 20, &rolloutv1alpha1.RolloutStepTargetToleration{
							FailureThreshold:    ptr.To[int32](5),
							InitialDelaySeconds: ptr.To[int32](0),
						}),
					}},
					{Targets: []rolloutv1alpha1.ScaleRunStepTarget{
						newScaleRunStepTarget("cluster-a", "test-a", 10),
					}},
				}
				scaleRun.Status.Phase = rolloutv1alpha1.RolloutRunPhaseProgressing
				scaleRun.Status.Batches = &rolloutv1alpha1.ScaleRunBatchStatus{
					RolloutBatchStatus: rolloutv1alpha1.RolloutBatchStatus{
						CurrentBatchIndex: 1,
						CurrentBatchState: rorexecutor.StepRunning,
					},
					Records: []rolloutv1alpha1.ScaleRunStepStatus{
						{Index: ptr.To[int32](0), State: rorexecutor.StepSkipped},
						{Index: ptr.To[int32](1), State: rorexecutor.StepRunning, StartTime: ptr.To(metav1.Now())},
						{Index: ptr.To[int32](2), State: rorexecutor.StepNone},
					},
				}
				return scaleRun
			},
			getWorkloads: func() []client.Object {
				// ScaleFrom = Spec.Replicas(10), ScaleTo = item.Replicas(20)
				// gap = 20 - 15 = 5 <= FailureThreshold(5) -> auto-skippable
				// InitialDelaySeconds = 0 means no wait
				return []client.Object{
					newFakeScaleObject("cluster-a", "default", "test-a", 10, 15, 15),
				}
			},
			assertResult: func(done bool, result reconcile.Result, err error) {
				s.Require().NoError(err)
				s.False(done) // not done yet; auto-skip advanced to next batch
				s.Equal(reconcile.Result{Requeue: true}, result)
			},
			assertStatus: func(status *rolloutv1alpha1.ScaleRunStatus) {
				// auto-skip mirrors manual skip: bypass PostBatchStepHook/Recycling,
				// mark current batch as StepSkipped, advance to next batch (index 2)
				// from StepNone.
				s.Equal(rorexecutor.StepSkipped, status.Batches.Records[1].State)
				s.Equal(int32(2), status.Batches.CurrentBatchIndex)
				s.Equal(rorexecutor.StepNone, status.Batches.CurrentBatchState)
				s.Equal(rolloutv1alpha1.RolloutRunPhaseProgressing, status.Phase)
				// gap = 20 - 15 = 5
				s.Len(status.Batches.Tolerations, 1)
				s.Equal(int32(5), status.Batches.Tolerations[0].Toleration)
			},
		},
		{
			name: "auto-skip does not apply when gap exceeds threshold, batch stays running",
			getObjects: func() *rolloutv1alpha1.ScaleRun {
				scaleRun := s.scaleRun.DeepCopy()
				scaleRun.Spec.Batch.Batches = []rolloutv1alpha1.ScaleRunStep{
					{Targets: []rolloutv1alpha1.ScaleRunStepTarget{
						newScaleRunStepTarget("cluster-a", "test-a", 20),
					}},
					{Targets: []rolloutv1alpha1.ScaleRunStepTarget{
						newScaleRunStepTargetWithToleration("cluster-a", "test-a", 20, &rolloutv1alpha1.RolloutStepTargetToleration{
							FailureThreshold:    ptr.To[int32](5),
							InitialDelaySeconds: ptr.To[int32](0),
						}),
					}},
					{Targets: []rolloutv1alpha1.ScaleRunStepTarget{
						newScaleRunStepTarget("cluster-a", "test-a", 10),
					}},
				}
				scaleRun.Status.Phase = rolloutv1alpha1.RolloutRunPhaseProgressing
				scaleRun.Status.Batches = &rolloutv1alpha1.ScaleRunBatchStatus{
					RolloutBatchStatus: rolloutv1alpha1.RolloutBatchStatus{
						CurrentBatchIndex: 1,
						CurrentBatchState: rorexecutor.StepRunning,
					},
					Records: []rolloutv1alpha1.ScaleRunStepStatus{
						{Index: ptr.To[int32](0), State: rorexecutor.StepSkipped},
						{Index: ptr.To[int32](1), State: rorexecutor.StepRunning, StartTime: ptr.To(metav1.Now())},
						{Index: ptr.To[int32](2), State: rorexecutor.StepNone},
					},
				}
				return scaleRun
			},
			getWorkloads: func() []client.Object {
				// gap = 20 - 12 = 8 > FailureThreshold(5) -> not auto-skippable, keep waiting
				return []client.Object{
					newFakeScaleObject("cluster-a", "default", "test-a", 10, 12, 12),
				}
			},
			assertResult: func(done bool, result reconcile.Result, err error) {
				s.Require().NoError(err)
				s.False(done)
				s.Equal(reconcile.Result{RequeueAfter: retryDefault}, result)
			},
			assertStatus: func(status *rolloutv1alpha1.ScaleRunStatus) {
				s.Equal(rorexecutor.StepRunning, status.Batches.CurrentBatchState)
				s.Empty(status.Batches.Tolerations)
			},
		},
		{
			name: "auto-skip on last batch transitions toward success",
			getObjects: func() *rolloutv1alpha1.ScaleRun {
				scaleRun := s.scaleRun.DeepCopy()
				scaleRun.Spec.Batch.Batches = []rolloutv1alpha1.ScaleRunStep{
					{Targets: []rolloutv1alpha1.ScaleRunStepTarget{
						newScaleRunStepTarget("cluster-a", "test-a", 20),
					}},
					{Targets: []rolloutv1alpha1.ScaleRunStepTarget{
						newScaleRunStepTarget("cluster-a", "test-a", 30),
					}},
					{Targets: []rolloutv1alpha1.ScaleRunStepTarget{
						newScaleRunStepTargetWithToleration("cluster-a", "test-a", 20, &rolloutv1alpha1.RolloutStepTargetToleration{
							FailureThreshold:    ptr.To[int32](5),
							InitialDelaySeconds: ptr.To[int32](0),
						}),
					}},
				}
				scaleRun.Status.Phase = rolloutv1alpha1.RolloutRunPhaseProgressing
				scaleRun.Status.Batches = &rolloutv1alpha1.ScaleRunBatchStatus{
					RolloutBatchStatus: rolloutv1alpha1.RolloutBatchStatus{
						CurrentBatchIndex: 2,
						CurrentBatchState: rorexecutor.StepRunning,
					},
					Records: []rolloutv1alpha1.ScaleRunStepStatus{
						{Index: ptr.To[int32](0), State: rorexecutor.StepSucceeded},
						{Index: ptr.To[int32](1), State: rorexecutor.StepSucceeded},
						{Index: ptr.To[int32](2), State: rorexecutor.StepRunning, StartTime: ptr.To(metav1.Now())},
					},
				}
				return scaleRun
			},
			getWorkloads: func() []client.Object {
				// Last batch: scale-up 10 -> 20, gap = 20 - 16 = 4 <= 5
				return []client.Object{
					newFakeScaleObject("cluster-a", "default", "test-a", 10, 16, 16),
				}
			},
			assertResult: func(done bool, result reconcile.Result, err error) {
				s.Require().NoError(err)
				s.False(done) // auto-skip transitions Phase to PostRollout; not yet Succeeded
				s.Equal(reconcile.Result{Requeue: true}, result)
			},
			assertStatus: func(status *rolloutv1alpha1.ScaleRunStatus) {
				// auto-skip on last batch mirrors manual skip: mark StepSkipped,
				// bypass PostBatchStepHook/Recycling, advance Phase to PostRollout.
				s.Equal(rorexecutor.StepSkipped, status.Batches.Records[2].State)
				s.Equal(int32(2), status.Batches.CurrentBatchIndex)
				s.Equal(rolloutv1alpha1.RolloutRunPhasePostRollout, status.Phase)
				s.Len(status.Batches.Tolerations, 1)
				s.Equal(int32(4), status.Batches.Tolerations[0].Toleration)
			},
		},
		{
			name: "scale-down does not skip: toleration only applies for scale-up (scenario 6)",
			getObjects: func() *rolloutv1alpha1.ScaleRun {
				scaleRun := s.scaleRun.DeepCopy()
				// 1 batch: scale-down from 10 to 5
				scaleRun.Spec.Batch.Batches = []rolloutv1alpha1.ScaleRunStep{
					{Targets: []rolloutv1alpha1.ScaleRunStepTarget{
						newScaleRunStepTargetWithToleration("cluster-a", "test-a", 5, &rolloutv1alpha1.RolloutStepTargetToleration{
							FailureThreshold:    ptr.To[int32](5),
							InitialDelaySeconds: ptr.To[int32](0),
						}),
					}},
				}
				scaleRun.Status.Phase = rolloutv1alpha1.RolloutRunPhaseProgressing
				scaleRun.Status.Batches = &rolloutv1alpha1.ScaleRunBatchStatus{
					RolloutBatchStatus: rolloutv1alpha1.RolloutBatchStatus{
						CurrentBatchIndex: 0,
						CurrentBatchState: rorexecutor.StepRunning,
					},
					Records: []rolloutv1alpha1.ScaleRunStepStatus{
						{Index: ptr.To[int32](0), State: rorexecutor.StepRunning, StartTime: ptr.To(metav1.Now())},
					},
				}
				return scaleRun
			},
			getWorkloads: func() []client.Object {
				// Workload still has Spec.Replicas=10, ObservedReplicas=10 (not yet scaled down)
				return []client.Object{
					newFakeScaleObject("cluster-a", "default", "test-a", 10, 10, 10),
				}
			},
			assertResult: func(done bool, result reconcile.Result, err error) {
				s.Require().NoError(err)
				s.False(done)
				s.Equal(reconcile.Result{RequeueAfter: retryDefault}, result)
			},
			assertStatus: func(status *rolloutv1alpha1.ScaleRunStatus) {
				s.Equal(rorexecutor.StepRunning, status.Batches.CurrentBatchState)
				// Scale-down: toleration should not be recorded
				s.Empty(status.Batches.Tolerations)
			},
			assertWorkloads: func(objs []client.Object) {
				s.Require().Len(objs, 1)
				sts := objs[0].(*appsv1.StatefulSet)
				// Scale was applied: Spec.Replicas changed 10 -> 5
				s.NotNil(sts.Spec.Replicas)
				s.Equal(int32(5), *sts.Spec.Replicas)
			},
		},
		{
			name: "no skip toleration, behavior unchanged",
			getObjects: func() *rolloutv1alpha1.ScaleRun {
				scaleRun := s.scaleRun.DeepCopy()
				// 1 batch: scale-up with no Toleration configured
				scaleRun.Spec.Batch.Batches = []rolloutv1alpha1.ScaleRunStep{
					{Targets: []rolloutv1alpha1.ScaleRunStepTarget{
						newScaleRunStepTarget("cluster-a", "test-a", 20),
					}},
				}
				scaleRun.Status.Phase = rolloutv1alpha1.RolloutRunPhaseProgressing
				scaleRun.Status.Batches = &rolloutv1alpha1.ScaleRunBatchStatus{
					RolloutBatchStatus: rolloutv1alpha1.RolloutBatchStatus{
						CurrentBatchIndex: 0,
						CurrentBatchState: rorexecutor.StepRunning,
					},
					Records: []rolloutv1alpha1.ScaleRunStepStatus{
						{Index: ptr.To[int32](0), State: rorexecutor.StepRunning, StartTime: ptr.To(metav1.Now())},
					},
				}
				return scaleRun
			},
			getWorkloads: func() []client.Object {
				// AvailableReplicas=8 < ScaleTo=20 -> not ready; no toleration -> keep waiting
				return []client.Object{
					newFakeScaleObject("cluster-a", "default", "test-a", 10, 8, 8),
				}
			},
			assertResult: func(done bool, result reconcile.Result, err error) {
				s.Require().NoError(err)
				s.False(done)
				s.Equal(reconcile.Result{RequeueAfter: retryDefault}, result)
			},
			assertStatus: func(status *rolloutv1alpha1.ScaleRunStatus) {
				s.Equal(rorexecutor.StepRunning, status.Batches.CurrentBatchState)
				s.Empty(status.Batches.Tolerations)
			},
		},
		{
			name: "Generation mismatch blocks auto-skip (transient state, gap unreliable)",
			getObjects: func() *rolloutv1alpha1.ScaleRun {
				scaleRun := s.scaleRun.DeepCopy()
				scaleRun.Spec.Batch.Batches = []rolloutv1alpha1.ScaleRunStep{
					{Targets: []rolloutv1alpha1.ScaleRunStepTarget{
						newScaleRunStepTargetWithToleration("cluster-a", "test-a", 20, &rolloutv1alpha1.RolloutStepTargetToleration{
							FailureThreshold:    ptr.To[int32](5),
							InitialDelaySeconds: ptr.To[int32](0),
						}),
					}},
				}
				scaleRun.Status.Phase = rolloutv1alpha1.RolloutRunPhaseProgressing
				scaleRun.Status.Batches = &rolloutv1alpha1.ScaleRunBatchStatus{
					RolloutBatchStatus: rolloutv1alpha1.RolloutBatchStatus{
						CurrentBatchIndex: 0,
						CurrentBatchState: rorexecutor.StepRunning,
					},
					Records: []rolloutv1alpha1.ScaleRunStepStatus{
						{Index: ptr.To[int32](0), State: rorexecutor.StepRunning, StartTime: ptr.To(metav1.Now())},
					},
				}
				return scaleRun
			},
			getWorkloads: func() []client.Object {
				// Generation mismatch: spec updated but status hasn't caught up
				obj := newFakeScaleObject("cluster-a", "default", "test-a", 10, 15, 15)
				obj.Generation = 2
				obj.Status.ObservedGeneration = 1
				return []client.Object{obj}
			},
			assertResult: func(done bool, result reconcile.Result, err error) {
				s.Require().NoError(err)
				s.False(done)
				s.Equal(reconcile.Result{RequeueAfter: retryDefault}, result)
			},
			assertStatus: func(status *rolloutv1alpha1.ScaleRunStatus) {
				s.Equal(rorexecutor.StepRunning, status.Batches.CurrentBatchState)
				// Gap within threshold, but Generation mismatch blocks auto-skip
				s.Empty(status.Batches.Tolerations)
			},
		},
	}

	s.runBatchTestCases(tests)
}

// Test_BatchExecutor_Do_Running covers doBatchUpgrading behavior outside the
// auto-skip path: applying Scale on first reconcile (scale-up & scale-down) and
// the all-ready transition to PostBatchStepHook.
func (s *batchExecutorTestSuite) Test_BatchExecutor_Do_Running() {
	tests := []scaleBatchTestCase{
		{
			name: "scale-up not ready: apply replicas and requeue",
			getObjects: func() *rolloutv1alpha1.ScaleRun {
				scaleRun := s.scaleRun.DeepCopy()
				scaleRun.Spec.Batch.Batches = []rolloutv1alpha1.ScaleRunStep{
					{Targets: []rolloutv1alpha1.ScaleRunStepTarget{
						newScaleRunStepTarget("cluster-a", "test-a", 20),
					}},
				}
				scaleRun.Status.Phase = rolloutv1alpha1.RolloutRunPhaseProgressing
				scaleRun.Status.Batches = &rolloutv1alpha1.ScaleRunBatchStatus{
					RolloutBatchStatus: rolloutv1alpha1.RolloutBatchStatus{
						CurrentBatchIndex: 0,
						CurrentBatchState: rorexecutor.StepRunning,
					},
					Records: []rolloutv1alpha1.ScaleRunStepStatus{
						{Index: ptr.To[int32](0), State: rorexecutor.StepRunning, StartTime: ptr.To(metav1.Now())},
					},
				}
				return scaleRun
			},
			getWorkloads: func() []client.Object {
				// Spec.Replicas=10, Available=10 -> not yet at ScaleTo=20
				return []client.Object{
					newFakeScaleObject("cluster-a", "default", "test-a", 10, 10, 10),
				}
			},
			assertResult: func(done bool, result reconcile.Result, err error) {
				s.Require().NoError(err)
				s.False(done)
				s.Equal(reconcile.Result{RequeueAfter: retryDefault}, result)
			},
			assertStatus: func(status *rolloutv1alpha1.ScaleRunStatus) {
				s.Equal(rorexecutor.StepRunning, status.Batches.CurrentBatchState)
				s.Len(status.Batches.Records, 1)
				s.Len(status.Batches.Records[0].Targets, 1)
				// ScaleFrom/ScaleTo captured for the target
				s.Equal(int32(10), status.Batches.Records[0].Targets[0].ScaleFrom)
				s.Equal(int32(20), status.Batches.Records[0].Targets[0].ScaleTo)
			},
			assertWorkloads: func(objs []client.Object) {
				s.Require().Len(objs, 1)
				sts := objs[0].(*appsv1.StatefulSet)
				s.NotNil(sts.Spec.Replicas)
				s.Equal(int32(20), *sts.Spec.Replicas) // applied
			},
		},
		{
			// On the second reconcile after replicas were applied, Records[0].Targets
			// already carries ScaleFrom/ScaleTo, so findCurrentWorkloadStatus returns
			// a match and needApplyReplicas=false. With AvailableReplicas>=ScaleTo,
			// checkScaledReady returns true and the all-ready path advances the state
			// machine to PostBatchStepHook.
			name: "scale-up all ready (second reconcile): move to PostBatchStepHook",
			getObjects: func() *rolloutv1alpha1.ScaleRun {
				scaleRun := s.scaleRun.DeepCopy()
				scaleRun.Spec.Batch.Batches = []rolloutv1alpha1.ScaleRunStep{
					{Targets: []rolloutv1alpha1.ScaleRunStepTarget{
						newScaleRunStepTarget("cluster-a", "test-a", 20),
					}},
				}
				scaleRun.Status.Phase = rolloutv1alpha1.RolloutRunPhaseProgressing
				scaleRun.Status.Batches = &rolloutv1alpha1.ScaleRunBatchStatus{
					RolloutBatchStatus: rolloutv1alpha1.RolloutBatchStatus{
						CurrentBatchIndex: 0,
						CurrentBatchState: rorexecutor.StepRunning,
					},
					Records: []rolloutv1alpha1.ScaleRunStepStatus{
						{
							Index:     ptr.To[int32](0),
							State:     rorexecutor.StepRunning,
							StartTime: ptr.To(metav1.Now()),
							// Pre-populated to simulate a prior reconcile that
							// already recorded ScaleFrom/ScaleTo. Without this,
							// needApplyReplicas is forced true and the all-ready
							// branch is bypassed on the first reconcile.
							Targets: []rolloutv1alpha1.ScaleWorkloadStatus{
								{Cluster: "cluster-a", Name: "test-a", ScaleFrom: 10, ScaleTo: 20},
							},
						},
					},
				}
				return scaleRun
			},
			getWorkloads: func() []client.Object {
				// Workload already scaled to 20 and Available=20 >= ScaleTo=20 -> ready.
				return []client.Object{
					newFakeScaleObject("cluster-a", "default", "test-a", 20, 20, 20),
				}
			},
			assertResult: func(done bool, result reconcile.Result, err error) {
				s.Require().NoError(err)
				s.False(done)
				s.Equal(reconcile.Result{Requeue: true}, result)
			},
			assertStatus: func(status *rolloutv1alpha1.ScaleRunStatus) {
				s.Equal(rorexecutor.StepPostBatchStepHook, status.Batches.CurrentBatchState)
				s.Empty(status.Batches.Tolerations)
			},
		},
		{
			name: "scale-down not ready: apply replicas and requeue",
			getObjects: func() *rolloutv1alpha1.ScaleRun {
				scaleRun := s.scaleRun.DeepCopy()
				scaleRun.Spec.Batch.Batches = []rolloutv1alpha1.ScaleRunStep{
					{Targets: []rolloutv1alpha1.ScaleRunStepTarget{
						newScaleRunStepTarget("cluster-a", "test-a", 5),
					}},
				}
				scaleRun.Status.Phase = rolloutv1alpha1.RolloutRunPhaseProgressing
				scaleRun.Status.Batches = &rolloutv1alpha1.ScaleRunBatchStatus{
					RolloutBatchStatus: rolloutv1alpha1.RolloutBatchStatus{
						CurrentBatchIndex: 0,
						CurrentBatchState: rorexecutor.StepRunning,
					},
					Records: []rolloutv1alpha1.ScaleRunStepStatus{
						{Index: ptr.To[int32](0), State: rorexecutor.StepRunning, StartTime: ptr.To(metav1.Now())},
					},
				}
				return scaleRun
			},
			getWorkloads: func() []client.Object {
				// Workload still at Spec=10, Observed=10 (not yet scaled to 5)
				return []client.Object{
					newFakeScaleObject("cluster-a", "default", "test-a", 10, 10, 10),
				}
			},
			assertResult: func(done bool, result reconcile.Result, err error) {
				s.Require().NoError(err)
				s.False(done)
				s.Equal(reconcile.Result{RequeueAfter: retryDefault}, result)
			},
			assertStatus: func(status *rolloutv1alpha1.ScaleRunStatus) {
				s.Equal(rorexecutor.StepRunning, status.Batches.CurrentBatchState)
				s.Len(status.Batches.Records, 1)
				s.Len(status.Batches.Records[0].Targets, 1)
				s.Equal(int32(10), status.Batches.Records[0].Targets[0].ScaleFrom)
				s.Equal(int32(5), status.Batches.Records[0].Targets[0].ScaleTo)
			},
			assertWorkloads: func(objs []client.Object) {
				s.Require().Len(objs, 1)
				sts := objs[0].(*appsv1.StatefulSet)
				s.NotNil(sts.Spec.Replicas)
				s.Equal(int32(5), *sts.Spec.Replicas) // applied
			},
		},
	}

	s.runBatchTestCases(tests)
}

// Test_BatchExecutor_Do_Pending_Paused covers the StepNone -> StepPending
// transition. With Breakpoint=true, doPausing sets Phase=Paused and the state
// engine advances to StepPending.
func (s *batchExecutorTestSuite) Test_BatchExecutor_Do_Pending_Paused() {
	tests := []scaleBatchTestCase{
		{
			name: "None to Pending(Paused) with breakpoint batch",
			getObjects: func() *rolloutv1alpha1.ScaleRun {
				scaleRun := s.scaleRun.DeepCopy()
				scaleRun.Spec.Batch.Batches = []rolloutv1alpha1.ScaleRunStep{
					{
						Breakpoint: true,
						Targets: []rolloutv1alpha1.ScaleRunStepTarget{
							newScaleRunStepTarget("cluster-a", "test-0", 10),
						},
					},
					{
						Targets: []rolloutv1alpha1.ScaleRunStepTarget{
							newScaleRunStepTarget("cluster-a", "test-1", 10),
						},
					},
				}
				scaleRun.Status.Phase = rolloutv1alpha1.RolloutRunPhaseProgressing
				scaleRun.Status.Batches = &rolloutv1alpha1.ScaleRunBatchStatus{
					RolloutBatchStatus: rolloutv1alpha1.RolloutBatchStatus{
						CurrentBatchIndex: 0,
						CurrentBatchState: rorexecutor.StepNone,
					},
					Records: []rolloutv1alpha1.ScaleRunStepStatus{
						{Index: ptr.To[int32](0), State: rorexecutor.StepNone},
					},
				}
				return scaleRun
			},
			getWorkloads: func() []client.Object {
				return []client.Object{
					newFakeScaleObject("cluster-a", "default", "test-0", 10, 10, 10),
					newFakeScaleObject("cluster-a", "default", "test-1", 10, 10, 10),
				}
			},
			assertResult: func(done bool, result reconcile.Result, err error) {
				s.Require().NoError(err)
				s.False(done)
				s.Equal(reconcile.Result{Requeue: true}, result)
			},
			assertStatus: func(status *rolloutv1alpha1.ScaleRunStatus) {
				s.Equal(rolloutv1alpha1.RolloutRunPhasePaused, status.Phase)
				s.Equal(rorexecutor.StepPending, status.Batches.CurrentBatchState)
				s.Equal(rorexecutor.StepPending, status.Batches.Records[0].State)
			},
			assertWorkloads: func(objs []client.Object) {
				// Only the current batch's targets get the progressing
				// annotation from BatchScaleControl.Initialize. test-0 lives
				// in the current batch (index 0); test-1 is in the next batch
				// and is NOT initialized yet.
				s.Require().Len(objs, 2)
				progressingByName := map[string]bool{}
				for _, obj := range objs {
					progressingByName[obj.GetName()] = workload.IsProgressing(obj)
				}
				s.True(progressingByName["test-0"], "test-0 should be progressing (current batch target)")
				s.False(progressingByName["test-1"], "test-1 should NOT be progressing (next batch)")
			},
		},
	}

	s.runBatchTestCases(tests)
}

// Test_BatchExecutor_Do_Recycling covers the StepResourceRecycling state on the
// last batch, where release() finalizes all workloads (removes progressing
// annotation) and the state advances to StepSucceeded.
func (s *batchExecutorTestSuite) Test_BatchExecutor_Do_Recycling() {
	tests := []scaleBatchTestCase{
		{
			name: "Recycling on last batch finalizes workloads and moves to Succeeded",
			getObjects: func() *rolloutv1alpha1.ScaleRun {
				scaleRun := s.scaleRun.DeepCopy()
				scaleRun.Spec.Batch.Batches = []rolloutv1alpha1.ScaleRunStep{
					{Targets: []rolloutv1alpha1.ScaleRunStepTarget{
						newScaleRunStepTarget("cluster-a", "test-0", 10),
					}},
					{Targets: []rolloutv1alpha1.ScaleRunStepTarget{
						newScaleRunStepTarget("cluster-a", "test-1", 10),
					}},
				}
				scaleRun.Status.Batches = &rolloutv1alpha1.ScaleRunBatchStatus{
					RolloutBatchStatus: rolloutv1alpha1.RolloutBatchStatus{
						CurrentBatchIndex: 1, // last batch
						CurrentBatchState: rorexecutor.StepResourceRecycling,
					},
					Records: []rolloutv1alpha1.ScaleRunStepStatus{
						{Index: ptr.To[int32](0), State: rorexecutor.StepSucceeded},
						{Index: ptr.To[int32](1), State: rorexecutor.StepResourceRecycling},
					},
				}
				return scaleRun
			},
			getWorkloads: func() []client.Object {
				return []client.Object{
					withProgressingInfo(newFakeScaleObject("cluster-a", "default", "test-0", 10, 10, 10)),
					withProgressingInfo(newFakeScaleObject("cluster-a", "default", "test-1", 10, 10, 10)),
				}
			},
			assertResult: func(done bool, result reconcile.Result, err error) {
				s.Require().NoError(err)
				s.False(done) // not yet done at batch level (still moves to next state)
				// doRecycle -> release on last batch returns (true, retryImmediately, nil);
				// the state engine converts retryImmediately to reconcile.Result{Requeue: true}.
				s.Equal(reconcile.Result{Requeue: true}, result)
			},
			assertStatus: func(status *rolloutv1alpha1.ScaleRunStatus) {
				s.Equal(rorexecutor.StepSucceeded, status.Batches.CurrentBatchState)
				s.Equal(rorexecutor.StepSucceeded, status.Batches.Records[1].State)
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

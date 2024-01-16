package executor

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/klog/v2/klogr"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	rolloutv1alpha1 "kusionstack.io/rollout/apis/rollout/v1alpha1"
	"kusionstack.io/rollout/pkg/workload"
	"kusionstack.io/rollout/pkg/workload/fake"
)

func newRunStepTarget(cluster, name string, replicas intstr.IntOrString) rolloutv1alpha1.RolloutRunStepTarget {
	return rolloutv1alpha1.RolloutRunStepTarget{
		CrossClusterObjectNameReference: rolloutv1alpha1.CrossClusterObjectNameReference{
			Cluster: cluster,
			Name:    name,
		},
		Replicas: replicas,
	}
}

func TestExecutor_doBatchUpgrading(t *testing.T) {
	r := &Executor{
		logger: klogr.New(),
	}
	tests := []struct {
		name         string
		getObjects   func() (*rolloutv1alpha1.Rollout, *rolloutv1alpha1.RolloutRun)
		getWorkloads func() *workload.Set
		assertResult func(assert *assert.Assertions, result reconcile.Result, err error)
		assertStatus func(assert *assert.Assertions, status *rolloutv1alpha1.RolloutRunStatus)
	}{
		{
			name: "batch target is empty",
			getObjects: func() (*rolloutv1alpha1.Rollout, *rolloutv1alpha1.RolloutRun) {
				rollout := testRollout.DeepCopy()
				rolloutRun := testRolloutRun.DeepCopy()

				rolloutRun.Spec.Batch.Batches = []rolloutv1alpha1.RolloutRunStep{{Targets: nil}}
				return rollout, rolloutRun
			},
			assertResult: func(assert *assert.Assertions, result reconcile.Result, err error) {
				assert.Nil(err)
				assert.Equal(reconcile.Result{Requeue: true}, result)
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

				rolloutRun.Spec.Batch.Batches = []rolloutv1alpha1.RolloutRunStep{{Targets: nil}}
				rolloutRun.Spec.Batch.Batches = []rolloutv1alpha1.RolloutRunStep{{
					Targets: []rolloutv1alpha1.RolloutRunStepTarget{
						newRunStepTarget("non-exist", "non-exits", intstr.FromInt(1)),
					},
				}}

				rolloutRun.Status.Phase = rolloutv1alpha1.RolloutRunPhaseProgressing
				rolloutRun.Status.BatchStatus = &rolloutv1alpha1.RolloutRunBatchStatus{
					RolloutBatchStatus: rolloutv1alpha1.RolloutBatchStatus{
						CurrentBatchIndex: 0,
						CurrentBatchState: BatchStateUpgrading,
					},
					Records: []rolloutv1alpha1.RolloutRunBatchStatusRecord{
						{
							Index:     ptr.To[int32](0),
							State:     BatchStateUpgrading,
							StartTime: &metav1.Time{Time: time.Now()},
						},
					},
				}
				return rollout, rolloutRun
			},
			assertResult: func(assert *assert.Assertions, result reconcile.Result, err error) {
				assert.NotNil(err)
				assert.Equal(reconcile.Result{}, result)
			},
			assertStatus: func(assert *assert.Assertions, status *rolloutv1alpha1.RolloutRunStatus) {
				assert.NotNil(status.Error)
				assert.Equal(CodeUpgradingError, status.Error.Code)
				assert.Equal(ReasonWorkloadInterfaceNotExist, status.Error.Reason)
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
						newRunStepTarget("cluster-a", "test-0", intstr.FromInt(1)),
					},
				}}
				rolloutRun.Status.Phase = rolloutv1alpha1.RolloutRunPhaseProgressing
				rolloutRun.Status.BatchStatus = &rolloutv1alpha1.RolloutRunBatchStatus{
					RolloutBatchStatus: rolloutv1alpha1.RolloutBatchStatus{
						CurrentBatchIndex: 0,
						CurrentBatchState: BatchStateUpgrading,
					},
					Records: []rolloutv1alpha1.RolloutRunBatchStatusRecord{
						{
							Index:     ptr.To[int32](0),
							State:     BatchStateUpgrading,
							StartTime: &metav1.Time{Time: time.Now()},
						},
					},
				}
				return rollout, rolloutRun
			},
			getWorkloads: func() *workload.Set {
				return workload.NewWorkloadSet(fake.New("cluster-a", "default", "test-0"))
			},
			assertResult: func(assert *assert.Assertions, result reconcile.Result, err error) {
				assert.Nil(err)
				assert.Equal(reconcile.Result{RequeueAfter: defaultRequeueAfter}, result)
			},
			assertStatus: func(assert *assert.Assertions, status *rolloutv1alpha1.RolloutRunStatus) {
				assert.Nil(status.Error)
				assert.Len(status.BatchStatus.Records, 1)
				assert.Len(status.BatchStatus.Records[0].Targets, 1)
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
						newRunStepTarget("cluster-a", "test-0", intstr.FromInt(10)),
					},
				}}
				rolloutRun.Status.Phase = rolloutv1alpha1.RolloutRunPhaseProgressing
				rolloutRun.Status.BatchStatus = &rolloutv1alpha1.RolloutRunBatchStatus{
					RolloutBatchStatus: rolloutv1alpha1.RolloutBatchStatus{
						CurrentBatchIndex: 0,
						CurrentBatchState: BatchStateUpgrading,
					},
					Records: []rolloutv1alpha1.RolloutRunBatchStatusRecord{
						{
							Index:     ptr.To[int32](0),
							State:     BatchStateUpgrading,
							StartTime: &metav1.Time{Time: time.Now()},
						},
					},
				}
				return rollout, rolloutRun
			},
			getWorkloads: func() *workload.Set {
				return workload.NewWorkloadSet(fake.New("cluster-a", "default", "test-0").ChangeStatus(100, 10, 1))
			},
			assertResult: func(assert *assert.Assertions, result reconcile.Result, err error) {
				assert.Nil(err)
				assert.Equal(reconcile.Result{RequeueAfter: defaultRequeueAfter}, result)
			},
			assertStatus: func(assert *assert.Assertions, status *rolloutv1alpha1.RolloutRunStatus) {
				assert.Nil(status.Error)
				assert.Len(status.BatchStatus.Records, 1)
				assert.Len(status.BatchStatus.Records[0].Targets, 1)
				assert.EqualValues(100, status.BatchStatus.Records[0].Targets[0].Replicas)
				assert.EqualValues(1, status.BatchStatus.Records[0].Targets[0].UpdatedAvailableReplicas)
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
						newRunStepTarget("cluster-a", "test-0", intstr.FromInt(10)),
						newRunStepTarget("cluster-b", "test-0", intstr.FromInt(10)),
					},
				}}
				rolloutRun.Status.Phase = rolloutv1alpha1.RolloutRunPhaseProgressing
				rolloutRun.Status.BatchStatus = &rolloutv1alpha1.RolloutRunBatchStatus{
					RolloutBatchStatus: rolloutv1alpha1.RolloutBatchStatus{
						CurrentBatchIndex: 0,
						CurrentBatchState: BatchStateUpgrading,
					},
					Records: []rolloutv1alpha1.RolloutRunBatchStatusRecord{
						{
							Index:     ptr.To[int32](0),
							State:     BatchStateUpgrading,
							StartTime: &metav1.Time{Time: time.Now()},
						},
					},
				}

				return rollout, rolloutRun
			},

			getWorkloads: func() *workload.Set {
				return workload.NewWorkloadSet(
					fake.New("cluster-a", "default", "test-0").ChangeStatus(100, 10, 10),
					fake.New("cluster-b", "default", "test-0").ChangeStatus(100, 10, 10),
				)
			},
			assertResult: func(assert *assert.Assertions, result reconcile.Result, err error) {
				assert.Nil(err)
				assert.Equal(reconcile.Result{Requeue: true}, result)
			},
			assertStatus: func(assert *assert.Assertions, status *rolloutv1alpha1.RolloutRunStatus) {
				assert.Nil(status.Error)
				// check records
				assert.Len(status.BatchStatus.Records, 1)
				// check targets
				assert.Len(status.BatchStatus.Records[0].Targets, 2)
				assert.EqualValues(100, status.BatchStatus.Records[0].Targets[0].Replicas)
				assert.EqualValues(10, status.BatchStatus.Records[0].Targets[0].UpdatedAvailableReplicas)
				// check state
				assert.Equal(rolloutv1alpha1.BatchStepStatePostBatchStepHook, status.BatchStatus.CurrentBatchState)
				assert.Equal(rolloutv1alpha1.BatchStepStatePostBatchStepHook, status.BatchStatus.Records[0].State)
			},
		},
	}
	for i := range tests {
		tt := tests[i]
		t.Run(tt.name, func(t *testing.T) {
			rollout, rolloutRun := tt.getObjects()
			var workloads *workload.Set
			if tt.getWorkloads != nil {
				workloads = tt.getWorkloads()
			}
			ctx := createTestExecutorContext(rollout, rolloutRun, workloads)
			got, err := r.doBatchUpgrading(context.Background(), ctx)
			assert := assert.New(t)
			tt.assertResult(assert, got, err)
			tt.assertStatus(assert, ctx.NewStatus)
		})
	}
}

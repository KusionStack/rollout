package executor

import (
	"context"
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
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
	RegisterFailHandler(Fail)

	r := &Executor{
		logger: klogr.New(),
	}
	tests := []struct {
		name        string
		fulfill     func(ctx *ExecutorContext)
		checkResult func(ctx *ExecutorContext, result reconcile.Result, err error)
	}{
		{
			name: "batch target is empty",
			fulfill: func(ctx *ExecutorContext) {
				rolloutRun := ctx.RolloutRun
				rolloutRun.Spec.Batch.Batches = []rolloutv1alpha1.RolloutRunStep{{Targets: nil}}
			},
			checkResult: func(ctx *ExecutorContext, result reconcile.Result, err error) {
				Expect(err).To(HaveOccurred())
				Expect(result).To(Equal(reconcile.Result{Requeue: true}))
				Expect(ctx.NewStatus.Error).NotTo(BeNil())
			},
		},
		{
			name: "workflow instance not found",
			fulfill: func(ctx *ExecutorContext) {
				rolloutRun := ctx.RolloutRun
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
				ctx.NewStatus = rolloutRun.Status.DeepCopy()
			},
			checkResult: func(ctx *ExecutorContext, result reconcile.Result, err error) {
				Expect(err).To(HaveOccurred())
				Expect(result).To(Equal(reconcile.Result{}))
				Expect(ctx.NewStatus.Error).NotTo(BeNil())
			},
		},
		{
			name: "upgrade workload and then requeue after 5 seconds",
			fulfill: func(ctx *ExecutorContext) {
				rolloutRun := ctx.RolloutRun
				// setup workloads
				ctx.Workloads = workload.NewWorkloadSet(fake.New("cluster-a", rolloutRun.Namespace, "test-0"))
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
				// setup newStatus
				ctx.NewStatus = rolloutRun.Status.DeepCopy()
			},
			checkResult: func(ctx *ExecutorContext, result reconcile.Result, err error) {
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(reconcile.Result{RequeueAfter: defaultRequeueAfter}))
				Expect(ctx.NewStatus.Error).To(BeNil())
				Expect(ctx.NewStatus.BatchStatus.Records).To(HaveLen(1))
				Expect(ctx.NewStatus.BatchStatus.Records[0].Targets).To(HaveLen(1))
			},
		},
		{
			name: "waiting for workload ready",
			fulfill: func(ctx *ExecutorContext) {
				rolloutRun := ctx.RolloutRun
				// setup workloads
				ctx.Workloads = workload.NewWorkloadSet(fake.New("cluster-a", rolloutRun.Namespace, "test-0").ChangeStatus(100, 10, 1))
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
				// setup newStatus
				ctx.NewStatus = rolloutRun.Status.DeepCopy()
			},
			checkResult: func(ctx *ExecutorContext, result reconcile.Result, err error) {
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(reconcile.Result{RequeueAfter: defaultRequeueAfter}))
				Expect(ctx.NewStatus.Error).To(BeNil())
				Expect(ctx.NewStatus.BatchStatus.Records).To(HaveLen(1))
				Expect(ctx.NewStatus.BatchStatus.Records[0].Targets).To(HaveLen(1))
				Expect(ctx.NewStatus.BatchStatus.Records[0].Targets[0].Replicas).To(Equal(100))
				Expect(ctx.NewStatus.BatchStatus.Records[0].Targets[0].UpdatedAvailableReplicas).To(Equal(1))
			},
		},
		{
			name: "all workload ready, move to next state",
			fulfill: func(ctx *ExecutorContext) {
				rolloutRun := ctx.RolloutRun
				// setup workloads
				ctx.Workloads = workload.NewWorkloadSet(fake.New("cluster-a", rolloutRun.Namespace, "test-0").ChangeStatus(100, 10, 10))
				ctx.Workloads = workload.NewWorkloadSet(fake.New("cluster-b", rolloutRun.Namespace, "test-0").ChangeStatus(100, 10, 10))
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
				// setup newStatus
				ctx.NewStatus = rolloutRun.Status.DeepCopy()
			},
			checkResult: func(ctx *ExecutorContext, result reconcile.Result, err error) {
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(reconcile.Result{RequeueAfter: defaultRequeueAfter}))
				Expect(ctx.NewStatus.Error).To(BeNil())
				// check records
				Expect(ctx.NewStatus.BatchStatus.Records).To(HaveLen(1))
				// check targets
				Expect(ctx.NewStatus.BatchStatus.Records[0].Targets).To(HaveLen(2))
				Expect(ctx.NewStatus.BatchStatus.Records[0].Targets[0].Replicas).To(Equal(100))
				Expect(ctx.NewStatus.BatchStatus.Records[0].Targets[0].UpdatedAvailableReplicas).To(Equal(10))
				// check state
				Expect(ctx.NewStatus.BatchStatus.CurrentBatchState).To(Equal(rolloutv1alpha1.BatchStepStatePostBatchStepHook))
				Expect(ctx.NewStatus.BatchStatus.Records[0].State).To(Equal(rolloutv1alpha1.BatchStepStatePostBatchStepHook))
			},
		},
	}
	for i := range tests {
		tt := tests[i]
		t.Run(tt.name, func(t *testing.T) {
			defer GinkgoRecover()
			ctx := newTestExecutorContext()
			tt.fulfill(ctx)
			ctx.Initialize()
			got, err := r.doBatchUpgrading(context.Background(), ctx)
			tt.checkResult(ctx, got, err)
		})
	}
}

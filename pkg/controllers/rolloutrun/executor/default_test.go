package executor

import (
	"context"
	"github.com/go-logr/logr"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"os"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"testing"

	rolloutv1alpha1 "kusionstack.io/rollout/apis/rollout/v1alpha1"
	"kusionstack.io/rollout/pkg/workload/fake"
	ctrl "sigs.k8s.io/controller-runtime"
)

var (
	fakeWi     = fake.New("", "", "")
	apiVersion = schema.GroupVersion{
		Group: fakeWi.GetInfo().GVK.Group, Version: fakeWi.GetInfo().GVK.Version,
	}

	rollout = rolloutv1alpha1.Rollout{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "foo",
			Namespace:   metav1.NamespaceDefault,
			UID:         types.UID(uuid.New().String()),
			Labels:      make(map[string]string),
			Annotations: make(map[string]string),
		},
		Spec: rolloutv1alpha1.RolloutSpec{},
	}

	rolloutRun = rolloutv1alpha1.RolloutRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "foo",
			Namespace:   metav1.NamespaceDefault,
			UID:         types.UID(uuid.New().String()),
			Labels:      make(map[string]string),
			Annotations: make(map[string]string),
		},
		Spec: rolloutv1alpha1.RolloutRunSpec{
			TargetType: rolloutv1alpha1.ObjectTypeRef{
				APIVersion: apiVersion.String(), Kind: fakeWi.GetInfo().GVK.Kind,
			},
			Webhooks: []rolloutv1alpha1.RolloutWebhook{},
			Batch: rolloutv1alpha1.RolloutRunBatchStrategy{
				Toleration: &rolloutv1alpha1.TolerationStrategy{},
				Batches:    []rolloutv1alpha1.RolloutRunStep{},
			},
		},
		Status: rolloutv1alpha1.RolloutRunStatus{
			Conditions: []rolloutv1alpha1.Condition{},
		},
	}
)

type checkResult func(done bool, result ctrl.Result, error error, rolloutRun *rolloutv1alpha1.RolloutRun) (bool, error)
type makeExecutorContext func(rollout *rolloutv1alpha1.Rollout, rolloutRun *rolloutv1alpha1.RolloutRun) *ExecutorContext

func TestDoInitialized(t *testing.T) {
	RegisterFailHandler(Fail)

	testcases := []struct {
		name                string
		checkResult         checkResult
		makeExecutorContext makeExecutorContext
	}{
		{
			name: "empty batch",
			makeExecutorContext: func(rollout *rolloutv1alpha1.Rollout, rolloutRun *rolloutv1alpha1.RolloutRun) *ExecutorContext {
				rolloutRun.Status.Phase = rolloutv1alpha1.RolloutPhaseProgressing
				return &ExecutorContext{rollout: rollout, rolloutRun: rolloutRun, newStatus: &rolloutRun.Status}
			},
			checkResult: func(done bool, result ctrl.Result, error error, rolloutRun *rolloutv1alpha1.RolloutRun) (bool, error) {
				if !done || result.Requeue || error != nil {
					return false, nil
				}
				batchStatus := rolloutRun.Status.BatchStatus
				if batchStatus.CurrentBatchIndex != 0 ||
					batchStatus.CurrentBatchError != nil ||
					len(batchStatus.Records) != 0 {
					return false, nil
				}
				return true, nil
			},
		},
		{
			name: "doInitialized",
			makeExecutorContext: func(rollout *rolloutv1alpha1.Rollout, rolloutRun *rolloutv1alpha1.RolloutRun) *ExecutorContext {
				rolloutRun.Status.Phase = rolloutv1alpha1.RolloutPhaseProgressing
				rolloutRun.Spec.Batch.Batches = []rolloutv1alpha1.RolloutRunStep{{
					Targets: []rolloutv1alpha1.RolloutRunStepTarget{
						{CrossClusterObjectNameReference: rolloutv1alpha1.CrossClusterObjectNameReference{Cluster: "cluster-a", Name: "test-1"}, Replicas: intstr.FromInt(1)},
					},
				}}
				return &ExecutorContext{rollout: rollout, rolloutRun: rolloutRun, newStatus: &rolloutRun.Status}
			},
			checkResult: func(done bool, result ctrl.Result, error error, rolloutRun *rolloutv1alpha1.RolloutRun) (bool, error) {
				if done || !result.Requeue || error != nil {
					return false, nil
				}
				batchStatus := rolloutRun.Status.BatchStatus
				if batchStatus.CurrentBatchIndex != 0 ||
					batchStatus.CurrentBatchError != nil ||
					len(batchStatus.Records) != 1 ||
					batchStatus.Records[0].StartTime == nil ||
					batchStatus.Records[0].State != rolloutv1alpha1.BatchStepStatePreBatchStepHook ||
					batchStatus.CurrentBatchState != rolloutv1alpha1.BatchStepStatePreBatchStepHook {
					return false, nil
				}
				return true, nil
			},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := logr.NewContext(
				context.Background(),
				zap.New(
					zap.WriteTo(os.Stdout), zap.UseDevMode(true),
				),
			)

			executor := NewDefaultExecutor()

			newRollout := rollout.DeepCopy()
			newRolloutRun := rolloutRun.DeepCopy()
			done, result, err := executor.Do(
				ctx, tc.makeExecutorContext(newRollout, newRolloutRun),
			)

			expect, err := tc.checkResult(done, result, err, newRolloutRun)
			Expect(err).NotTo(HaveOccurred())
			Expect(expect).Should(BeTrue())
		})
	}
}

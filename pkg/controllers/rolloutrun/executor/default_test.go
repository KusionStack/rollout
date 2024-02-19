package executor

import (
	"github.com/go-logr/logr"
	"github.com/google/uuid"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	rolloutv1alpha1 "kusionstack.io/rollout/apis/rollout/v1alpha1"
	"kusionstack.io/rollout/pkg/workload"
	fakev1alpha1 "kusionstack.io/rollout/pkg/workload/fake"
)

var (
	apiVersion = schema.GroupVersion{
		Group: fakev1alpha1.GVK.Group, Version: fakev1alpha1.GVK.Version,
	}

	testRollout = rolloutv1alpha1.Rollout{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "test-rollout",
			Namespace:   metav1.NamespaceDefault,
			UID:         types.UID(uuid.New().String()),
			Labels:      make(map[string]string),
			Annotations: make(map[string]string),
		},
		Spec: rolloutv1alpha1.RolloutSpec{},
	}

	testRolloutRun = rolloutv1alpha1.RolloutRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "test-rolloutrun",
			Namespace:   metav1.NamespaceDefault,
			UID:         types.UID(uuid.New().String()),
			Labels:      make(map[string]string),
			Annotations: make(map[string]string),
		},
		Spec: rolloutv1alpha1.RolloutRunSpec{
			TargetType: rolloutv1alpha1.ObjectTypeRef{
				APIVersion: apiVersion.String(), Kind: fakev1alpha1.GVK.Kind,
			},
			Webhooks: []rolloutv1alpha1.RolloutWebhook{},
			Batch: &rolloutv1alpha1.RolloutRunBatchStrategy{
				Toleration: &rolloutv1alpha1.TolerationStrategy{},
				Batches:    []rolloutv1alpha1.RolloutRunStep{},
			},
		},
		Status: rolloutv1alpha1.RolloutRunStatus{
			Conditions: []rolloutv1alpha1.Condition{},
		},
	}
)

func newTestLogger() logr.Logger {
	return zap.New(zap.UseDevMode(true), zap.ConsoleEncoder())
}

func createTestExecutorContext(rollout *rolloutv1alpha1.Rollout, rolloutRun *rolloutv1alpha1.RolloutRun, workloads *workload.Set) *ExecutorContext {
	if workloads == nil {
		workloads = workload.NewWorkloadSet()
	}
	ctx := &ExecutorContext{
		Rollout:    rollout,
		RolloutRun: rolloutRun,
		Workloads:  workloads,
		NewStatus:  rolloutRun.Status.DeepCopy(),
	}
	ctx.Initialize()
	return ctx
}

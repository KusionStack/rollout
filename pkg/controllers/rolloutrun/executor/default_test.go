package executor

import (
	"context"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	fakeclientset "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/kubernetes/scheme"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	rolloutapi "kusionstack.io/rollout/apis/rollout"
	rolloutv1alpha1 "kusionstack.io/rollout/apis/rollout/v1alpha1"
	"kusionstack.io/rollout/pkg/workload"
	"kusionstack.io/rollout/pkg/workload/statefulset"
)

var (
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
				APIVersion: statefulset.GVK.GroupVersion().String(),
				Kind:       statefulset.GVK.Kind,
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
	testCanaryRolloutRun = rolloutv1alpha1.RolloutRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "ror-with-canary",
			Namespace:   metav1.NamespaceDefault,
			UID:         types.UID(uuid.New().String()),
			Labels:      make(map[string]string),
			Annotations: make(map[string]string),
		},
		Spec: rolloutv1alpha1.RolloutRunSpec{
			TargetType: rolloutv1alpha1.ObjectTypeRef{
				APIVersion: statefulset.GVK.GroupVersion().String(),
				Kind:       statefulset.GVK.Kind,
			},
			Webhooks: []rolloutv1alpha1.RolloutWebhook{},
			Canary: &rolloutv1alpha1.RolloutRunCanaryStrategy{
				Targets: []rolloutv1alpha1.RolloutRunStepTarget{},
			},
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

func createTestExecutorContext(rollout *rolloutv1alpha1.Rollout, rolloutRun *rolloutv1alpha1.RolloutRun, objs ...client.Object) *ExecutorContext {
	infos := []*workload.Info{}
	inter := newTestWorkloadInterface()
	rolloutv1alpha1.AddToScheme(scheme.Scheme)
	clientbuilder := fake.NewClientBuilder().WithScheme(scheme.Scheme)
	for i := range objs {
		obj := objs[i]
		clientbuilder.WithObjects(obj)
		w, _ := inter.GetInfo(obj.GetClusterName(), obj)
		infos = append(infos, w)
	}

	kubeClient := fakeclientset.NewSimpleClientset()
	broadcaster := record.NewBroadcaster()
	broadcaster.StartStructuredLogging(0)
	broadcaster.StartRecordingToSink(&corev1client.EventSinkImpl{Interface: kubeClient.CoreV1().Events("")})
	recorder := broadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: "test"})

	workloads := workload.NewSet(infos...)
	c := clientbuilder.Build()
	ctx := &ExecutorContext{
		Context:    context.TODO(),
		Client:     c,
		Recorder:   recorder,
		Accessor:   inter,
		OwnerName:  rollout.Name,
		RolloutRun: rolloutRun,
		Workloads:  workloads,
		NewStatus:  rolloutRun.Status.DeepCopy(),
	}
	ctx.Initialize()
	ctx.WithLogger(newTestLogger())
	return ctx
}

func newTestWorkloadInterface() workload.Accessor {
	return statefulset.New()
}

func newFakeObject(cluster, namespace, name string, replicas, partition, updated int32) *appsv1.StatefulSet {
	realPartition := replicas - partition
	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   namespace,
			ClusterName: cluster,
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: &replicas,
			UpdateStrategy: appsv1.StatefulSetUpdateStrategy{
				Type: appsv1.RollingUpdateStatefulSetStrategyType,
				RollingUpdate: &appsv1.RollingUpdateStatefulSetStrategy{
					Partition: &realPartition,
				},
			},
		},
		Status: appsv1.StatefulSetStatus{
			Replicas:          replicas,
			UpdatedReplicas:   updated,
			ReadyReplicas:     replicas,
			AvailableReplicas: replicas,
		},
	}
	if realPartition <= 0 {
		sts.Spec.UpdateStrategy.RollingUpdate = nil
	}

	if partition == 0 {
		// partition == 0 means all replicas are updated
		sts.Status.CurrentRevision = "v1"
		sts.Status.UpdateRevision = "v1"
		sts.Status.CurrentReplicas = updated
		sts.Status.UpdatedReplicas = updated
	} else {
		sts.Status.CurrentRevision = "v1"
		sts.Status.UpdateRevision = "v2"
		sts.Status.CurrentReplicas = replicas - updated
		sts.Status.UpdatedReplicas = updated
	}
	return sts
}

func withProgressingInfo(obj *appsv1.StatefulSet) *appsv1.StatefulSet {
	if obj.Annotations == nil {
		obj.Annotations = map[string]string{}
	}
	obj.Annotations[rolloutapi.AnnoRolloutProgressingInfo] = ""
	return obj
}

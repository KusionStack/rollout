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
	rolloutapi "kusionstack.io/kube-api/rollout"
	rolloutv1alpha1 "kusionstack.io/kube-api/rollout/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"kusionstack.io/rollout/pkg/workload"
	"kusionstack.io/rollout/pkg/workload/statefulset"
)

// testScaleRun is the template used as the base for each test case.
// Tests deep-copy and then mutate Spec/Status as needed.
var testScaleRun = rolloutv1alpha1.ScaleRun{
	ObjectMeta: metav1.ObjectMeta{
		Name:        "test-scalerun",
		Namespace:   metav1.NamespaceDefault,
		UID:         types.UID(uuid.New().String()),
		Labels:      make(map[string]string),
		Annotations: make(map[string]string),
	},
	Spec: rolloutv1alpha1.ScaleRunSpec{
		TargetType: rolloutv1alpha1.ObjectTypeRef{
			APIVersion: statefulset.GVK.GroupVersion().String(),
			Kind:       statefulset.GVK.Kind,
		},
		Batch: &rolloutv1alpha1.ScaleRunBatchStrategy{},
	},
	Status: rolloutv1alpha1.ScaleRunStatus{
		Conditions: []rolloutv1alpha1.Condition{},
	},
}

func newTestScaleLogger() logr.Logger {
	return zap.New(zap.UseDevMode(true), zap.ConsoleEncoder())
}

func createTestScaleExecutorContext(scaleRun *rolloutv1alpha1.ScaleRun, objs ...client.Object) *ExecutorContext {
	infos := []*workload.Info{}
	inter := newTestScaleWorkloadInterface()
	rolloutv1alpha1.AddToScheme(scheme.Scheme)
	clientbuilder := fake.NewClientBuilder().WithScheme(scheme.Scheme)
	for i := range objs {
		obj := objs[i]
		clientbuilder.WithObjects(obj)
		w, _ := inter.GetInfo(obj.GetLabels()["kusionstack.io/cluster"], obj)
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
		Context:   context.TODO(),
		Client:    c,
		Recorder:  recorder,
		Accessor:  inter,
		ScaleRun:  scaleRun,
		Workloads: workloads,
		NewStatus: scaleRun.Status.DeepCopy(),
	}
	ctx.Initialize()
	ctx.WithLogger(newTestScaleLogger())
	return ctx
}

func newTestScaleWorkloadInterface() workload.Accessor {
	return statefulset.New()
}

// newFakeScaleObject builds a StatefulSet suitable for scale-run tests.
//
// specReplicas:      the workload's current Spec.Replicas (maps to info.Status.DesiredReplicas).
// availableReplicas: the workload's Status.AvailableReplicas (governs scale-up readiness and gap calc).
// observedReplicas:  the workload's Status.Replicas (maps to info.Status.ObservedReplicas).
//
// Generation and ObservedGeneration are both 1 by default; tests can mutate
// the returned object to simulate a generation mismatch.
func newFakeScaleObject(cluster, namespace, name string, specReplicas, availableReplicas, observedReplicas int32) *appsv1.StatefulSet {
	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  namespace,
			Generation: 1,
			Labels: map[string]string{
				"kusionstack.io/cluster": cluster,
			},
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: &specReplicas,
			UpdateStrategy: appsv1.StatefulSetUpdateStrategy{
				Type: appsv1.RollingUpdateStatefulSetStrategyType,
			},
		},
		Status: appsv1.StatefulSetStatus{
			ObservedGeneration: 1,
			Replicas:           observedReplicas,
			ReadyReplicas:      availableReplicas,
			AvailableReplicas:  availableReplicas,
			CurrentReplicas:    observedReplicas,
			UpdatedReplicas:    observedReplicas,
			CurrentRevision:    "v1",
			UpdateRevision:     "v1",
		},
	}
}

// withProgressingInfo adds the rollout progressing annotation to a StatefulSet,
// mirroring what BatchScaleControl.Initialize would write. Used to simulate
// workloads that already have progressing info from a prior reconcile.
func withProgressingInfo(obj *appsv1.StatefulSet) *appsv1.StatefulSet {
	if obj.Annotations == nil {
		obj.Annotations = map[string]string{}
	}
	obj.Annotations[rolloutapi.AnnoRolloutProgressingInfo] = ""
	return obj
}

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

package registry

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	operatingv1alpha1 "kusionstack.io/kube-api/apps/v1alpha1"

	"kusionstack.io/rollout/pkg/workload"
	"kusionstack.io/rollout/pkg/workload/collaset"
	"kusionstack.io/rollout/pkg/workload/statefulset"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var _ workload.Accessor = &fakeWorkloadAccessor{}

type fakeWorkloadAccessor struct {
	gvk          schema.GroupVersionKind
	dependentGVK schema.GroupVersionKind
}

func newFakeWorkloadAccessor(gvk, dependentGVK schema.GroupVersionKind) workload.Accessor {
	return &fakeWorkloadAccessor{
		gvk:          gvk,
		dependentGVK: dependentGVK,
	}
}

// DependentWorkloadGVKs implements workload.Accessor.
func (f *fakeWorkloadAccessor) DependentWorkloadGVKs() []schema.GroupVersionKind {
	return []schema.GroupVersionKind{f.dependentGVK}
}

// GetInfo implements workload.Accessor.
func (f *fakeWorkloadAccessor) GetInfo(cluster string, obj client.Object) (*workload.Info, error) {
	panic("unimplemented")
}

// GroupVersionKind implements workload.Accessor.
func (f *fakeWorkloadAccessor) GroupVersionKind() schema.GroupVersionKind {
	return f.gvk
}

// NewObject implements workload.Accessor.
func (f *fakeWorkloadAccessor) NewObject() client.Object {
	panic("unimplemented")
}

// NewObjectList implements workload.Accessor.
func (f *fakeWorkloadAccessor) NewObjectList() client.ObjectList {
	panic("unimplemented")
}

// PodControl implements workload.Accessor.
func (f *fakeWorkloadAccessor) PodControl(client.Reader) workload.PodControl {
	panic("unimplemented")
}

// ReleaseControl implements workload.Accessor.
func (f *fakeWorkloadAccessor) ReleaseControl() workload.ReleaseControl {
	panic("unimplemented")
}

// Watchable implements workload.Accessor.
func (f *fakeWorkloadAccessor) Watchable() bool {
	panic("unimplemented")
}

func newTestWorkloadRegistry() WorkloadRegistry {
	r := NewWorkloadRegistry()
	r.Register(collaset.GVK, collaset.New())
	r.Register(statefulset.GVK, statefulset.New())
	// we add a fake workload accessor for testing
	// The workload resource owner chain is Deployment -> ReplicaSet -> Pod
	fake := newFakeWorkloadAccessor(appsv1.SchemeGroupVersion.WithKind("Deployment"), appsv1.SchemeGroupVersion.WithKind("ReplicaSet"))
	r.Register(fake.GroupVersionKind(), fake)
	return r
}

func newTestObject(gvk schema.GroupVersionKind, namespace, name string, owner client.Object) client.Object {
	obj, err := scheme.Scheme.New(gvk)
	if err != nil {
		panic(err)
	}
	metaObj, ok := obj.(client.Object)
	if !ok {
		panic("failed to convert object to client.Object")
	}

	metaObj.SetName(name)
	metaObj.SetNamespace(namespace)
	if owner != nil {
		ownerGVKs, _, err := scheme.Scheme.ObjectKinds(owner)
		if err != nil {
			panic(err)
		}
		owner := metav1.NewControllerRef(owner, ownerGVKs[0])
		metaObj.SetOwnerReferences([]metav1.OwnerReference{*owner})
	}
	return metaObj
}

func newFakeClient(objs ...client.Object) client.Client {
	clientbuilder := fake.NewClientBuilder().WithScheme(scheme.Scheme)
	for i := range objs {
		obj := objs[i]
		if obj == nil {
			continue
		}
		clientbuilder.WithObjects(obj)
	}
	return clientbuilder.Build()
}

func Test_registryImpl_GetPodOwnerWorkload(t *testing.T) {
	utilruntime.Must(operatingv1alpha1.AddToScheme(scheme.Scheme))
	podGVK := corev1.SchemeGroupVersion.WithKind("Pod")
	tests := []struct {
		name         string
		getObject    func() (*corev1.Pod, client.Client)
		assertResult func(assert assert.Assertions, obj client.Object, accessor workload.Accessor, err error)
	}{
		{
			name: "orphan pod",
			getObject: func() (*corev1.Pod, client.Client) {
				pod := newTestObject(podGVK, "default", "pod", nil)
				c := newFakeClient(pod)
				return pod.(*corev1.Pod), c
			},
			assertResult: func(assert assert.Assertions, obj client.Object, accessor workload.Accessor, err error) {
				assert.Nil(obj)
				assert.Nil(accessor)
				assert.NoError(err)
			},
		},
		{
			name: "statefulset -> pod",
			getObject: func() (*corev1.Pod, client.Client) {
				obj := newTestObject(statefulset.GVK, "default", "owner", nil)
				pod := newTestObject(podGVK, obj.GetNamespace(), "pod", obj)
				c := newFakeClient(obj, pod)
				return pod.(*corev1.Pod), c
			},
			assertResult: func(assert assert.Assertions, obj client.Object, accessor workload.Accessor, err error) {
				assert.NotNil(obj)
				if assert.NotNil(accessor) {
					assert.Equal(accessor.GroupVersionKind().Kind, "StatefulSet")
				}
				assert.NoError(err)
			},
		},
		{
			name: "collaset -> pod",
			getObject: func() (*corev1.Pod, client.Client) {
				obj := newTestObject(collaset.GVK, "default", "owner", nil)
				pod := newTestObject(podGVK, obj.GetNamespace(), "pod", obj)
				c := newFakeClient(obj, pod)
				return pod.(*corev1.Pod), c
			},
			assertResult: func(assert assert.Assertions, obj client.Object, accessor workload.Accessor, err error) {
				assert.NotNil(obj)
				if assert.NotNil(accessor) {
					assert.Equal(accessor.GroupVersionKind().Kind, "CollaSet")
				}
				assert.NoError(err)
			},
		},
		{
			name: "deployment -> replicaset -> pod",
			getObject: func() (*corev1.Pod, client.Client) {
				obj := newTestObject(appsv1.SchemeGroupVersion.WithKind("Deployment"), "default", "owner", nil)
				dependent := newTestObject(appsv1.SchemeGroupVersion.WithKind("ReplicaSet"), obj.GetNamespace(), "dependent", obj)
				pod := newTestObject(podGVK, obj.GetNamespace(), "pod", dependent)
				c := newFakeClient(obj, dependent, pod)
				return pod.(*corev1.Pod), c
			},
			assertResult: func(assert assert.Assertions, obj client.Object, accessor workload.Accessor, err error) {
				assert.NotNil(obj)
				if assert.NotNil(accessor) {
					assert.Equal(accessor.GroupVersionKind().Kind, "Deployment")
				}
				assert.NoError(err)
			},
		},
		{
			name: "daemonset -> statefulset -> pod",
			getObject: func() (*corev1.Pod, client.Client) {
				obj := newTestObject(appsv1.SchemeGroupVersion.WithKind("DaemonSet"), "default", "owner", nil)
				dependent := newTestObject(statefulset.GVK, obj.GetNamespace(), "dependent", obj)
				pod := newTestObject(podGVK, obj.GetNamespace(), "pod", dependent)
				c := newFakeClient(obj, dependent, pod)
				return pod.(*corev1.Pod), c
			},
			assertResult: func(assert assert.Assertions, obj client.Object, accessor workload.Accessor, err error) {
				assert.NotNil(obj)
				if assert.NotNil(accessor) {
					assert.Equal(accessor.GroupVersionKind().Kind, "StatefulSet")
				}
				assert.NoError(err)
			},
		},
	}

	for i := range tests {
		tt := tests[i]
		t.Run(tt.name, func(t *testing.T) {
			r := newTestWorkloadRegistry()
			pod, c := tt.getObject()
			got, accessor, err := r.GetPodOwnerWorkload(context.Background(), c, pod)
			tt.assertResult(*assert.New(t), got, accessor, err)
		})
	}
}

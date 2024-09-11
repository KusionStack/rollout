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
	"k8s.io/utils/ptr"
	operatingv1alpha1 "kusionstack.io/kube-api/apps/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"kusionstack.io/rollout/pkg/workload"
	"kusionstack.io/rollout/pkg/workload/collaset"
	"kusionstack.io/rollout/pkg/workload/statefulset"
)

var (
	podGVK        = corev1.SchemeGroupVersion.WithKind("Pod")
	daemonsetGVK  = appsv1.SchemeGroupVersion.WithKind("DaemonSet")
	deploymentGVK = appsv1.SchemeGroupVersion.WithKind("Deployment")
	replicasetGVK = appsv1.SchemeGroupVersion.WithKind("ReplicaSet")
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
	fake := newFakeWorkloadAccessor(deploymentGVK, replicasetGVK)
	r.Register(fake.GroupVersionKind(), fake)
	return r
}

func newTestObject(gvk schema.GroupVersionKind, namespace, name string, owners ...client.Object) client.Object {
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

	for i, owner := range owners {
		ownerGVKs, _, err := scheme.Scheme.ObjectKinds(owner)
		if err != nil {
			panic(err)
		}
		ownerRefs := metaObj.GetOwnerReferences()
		if i == 0 {
			ref := metav1.NewControllerRef(owner, ownerGVKs[0])
			ownerRefs = append(ownerRefs, *ref)
		} else {
			ref := metav1.NewControllerRef(owner, ownerGVKs[0])
			ref.Controller = ptr.To(false)
			ownerRefs = append(ownerRefs, *ref)
		}
		metaObj.SetOwnerReferences(ownerRefs)
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

func Test_registryImpl_GetControllerOf(t *testing.T) {
	utilruntime.Must(operatingv1alpha1.AddToScheme(scheme.Scheme))
	tests := []struct {
		name         string
		getObject    func() (*corev1.Pod, client.Client)
		assertResult func(assert assert.Assertions, in *WorkloadAccessor, err error)
	}{
		{
			name: "orphan pod",
			getObject: func() (*corev1.Pod, client.Client) {
				pod := newTestObject(podGVK, "default", "pod")
				c := newFakeClient(pod)
				return pod.(*corev1.Pod), c
			},
			assertResult: func(assert assert.Assertions, in *WorkloadAccessor, err error) {
				assert.Nil(in)
				assert.NoError(err)
			},
		},
		{
			name: "statefulset -> pod",
			getObject: func() (*corev1.Pod, client.Client) {
				obj := newTestObject(statefulset.GVK, "default", "owner")
				pod := newTestObject(podGVK, obj.GetNamespace(), "pod", obj)
				c := newFakeClient(obj, pod)
				return pod.(*corev1.Pod), c
			},
			assertResult: func(assert assert.Assertions, in *WorkloadAccessor, err error) {
				if assert.NotNil(in) {
					assert.NotNil(in.Object)
					if assert.NotNil(in.Accessor) {
						assert.Equal(in.Accessor.GroupVersionKind().Kind, "StatefulSet")
					}
				}
				assert.NoError(err)
			},
		},
		{
			name: "collaset -> pod",
			getObject: func() (*corev1.Pod, client.Client) {
				obj := newTestObject(collaset.GVK, "default", "owner")
				pod := newTestObject(podGVK, obj.GetNamespace(), "pod", obj)
				c := newFakeClient(obj, pod)
				return pod.(*corev1.Pod), c
			},
			assertResult: func(assert assert.Assertions, in *WorkloadAccessor, err error) {
				if assert.NotNil(in) {
					assert.NotNil(in.Object)
					if assert.NotNil(in.Accessor) {
						assert.Equal(in.Accessor.GroupVersionKind().Kind, "CollaSet")
					}
				}
				assert.NoError(err)
			},
		},
		{
			name: "statefulset -> collaset -> pod",
			getObject: func() (*corev1.Pod, client.Client) {
				obj := newTestObject(statefulset.GVK, "default", "owner")
				dependent := newTestObject(collaset.GVK, obj.GetNamespace(), "dependent", obj)
				pod := newTestObject(podGVK, obj.GetNamespace(), "pod", dependent)
				c := newFakeClient(obj, dependent, pod)
				return pod.(*corev1.Pod), c
			},
			assertResult: func(assert assert.Assertions, in *WorkloadAccessor, err error) {
				if assert.NotNil(in) {
					assert.NotNil(in.Object)
					if assert.NotNil(in.Accessor) {
						assert.Equal(in.Accessor.GroupVersionKind().Kind, "StatefulSet")
					}
				}
				assert.NoError(err)
			},
		},
		{
			name: "deployment -> replicaset -> pod",
			getObject: func() (*corev1.Pod, client.Client) {
				obj := newTestObject(deploymentGVK, "default", "owner")
				dependent := newTestObject(replicasetGVK, obj.GetNamespace(), "dependent", obj)
				pod := newTestObject(podGVK, obj.GetNamespace(), "pod", dependent)
				c := newFakeClient(obj, dependent, pod)
				return pod.(*corev1.Pod), c
			},
			assertResult: func(assert assert.Assertions, in *WorkloadAccessor, err error) {
				if assert.NotNil(in) {
					assert.NotNil(in.Object)
					if assert.NotNil(in.Accessor) {
						assert.Equal(in.Accessor.GroupVersionKind().Kind, "Deployment")
					}
				}
				assert.NoError(err)
			},
		},
		{
			name: "daemonset -> statefulset -> pod",
			getObject: func() (*corev1.Pod, client.Client) {
				obj := newTestObject(daemonsetGVK, "default", "owner")
				dependent := newTestObject(statefulset.GVK, obj.GetNamespace(), "dependent", obj)
				pod := newTestObject(podGVK, obj.GetNamespace(), "pod", dependent)
				c := newFakeClient(obj, dependent, pod)
				return pod.(*corev1.Pod), c
			},
			assertResult: func(assert assert.Assertions, in *WorkloadAccessor, err error) {
				if assert.NotNil(in) {
					assert.NotNil(in.Object)
					if assert.NotNil(in.Accessor) {
						assert.Equal(in.Accessor.GroupVersionKind().Kind, "StatefulSet")
					}
				}
				assert.NoError(err)
			},
		},
		{
			name: "statefulset -> daemonset -> pod",
			getObject: func() (*corev1.Pod, client.Client) {
				obj := newTestObject(statefulset.GVK, "default", "owner")
				dependent := newTestObject(daemonsetGVK, obj.GetNamespace(), "dependent", obj)
				pod := newTestObject(podGVK, obj.GetNamespace(), "pod", dependent)
				c := newFakeClient(obj, dependent, pod)
				return pod.(*corev1.Pod), c
			},
			assertResult: func(assert assert.Assertions, in *WorkloadAccessor, err error) {
				assert.Nil(in)
				assert.NoError(err)
			},
		},
	}

	for i := range tests {
		tt := tests[i]
		t.Run(tt.name, func(t *testing.T) {
			r := newTestWorkloadRegistry()
			pod, c := tt.getObject()
			got, err := r.GetControllerOf(context.Background(), c, pod)
			tt.assertResult(*assert.New(t), got, err)
		})
	}
}

func Test_registryImpl_GetOwnersOf(t *testing.T) {
	utilruntime.Must(operatingv1alpha1.AddToScheme(scheme.Scheme))
	tests := []struct {
		name         string
		getObject    func() (*corev1.Pod, client.Client)
		assertResult func(assert assert.Assertions, in []*WorkloadAccessor, err error)
	}{
		{
			name: "orphan pod",
			getObject: func() (*corev1.Pod, client.Client) {
				pod := newTestObject(podGVK, "default", "pod")
				c := newFakeClient(pod)
				return pod.(*corev1.Pod), c
			},
			assertResult: func(assert assert.Assertions, in []*WorkloadAccessor, err error) {
				assert.Len(in, 0)
				assert.NoError(err)
			},
		},
		{
			name: "statefulset -> pod, deployment -> pod",
			getObject: func() (*corev1.Pod, client.Client) {
				owner1 := newTestObject(statefulset.GVK, "default", "owner")
				owner2 := newTestObject(collaset.GVK, "default", "owner")
				pod := newTestObject(podGVK, "default", "pod", owner1, owner2)
				c := newFakeClient(owner1, owner2, pod)
				return pod.(*corev1.Pod), c
			},
			assertResult: func(assert assert.Assertions, in []*WorkloadAccessor, err error) {
				if assert.Len(in, 2) {
					assert.NotNil(in[0].Object)
					if assert.NotNil(in[0].Accessor) {
						assert.Equal(in[0].Accessor.GroupVersionKind().Kind, "StatefulSet")
					}
					assert.NotNil(in[1].Object)
					if assert.NotNil(in[1].Accessor) {
						assert.Equal(in[1].Accessor.GroupVersionKind().Kind, "CollaSet")
					}
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
			got, err := r.GetOwnersOf(context.Background(), c, pod)
			tt.assertResult(*assert.New(t), got, err)
		})
	}
}

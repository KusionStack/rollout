// Copyright 2023 The KusionStack Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package statefulset

import (
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"kusionstack.io/rollout/pkg/workload"
)

var GVK = appsv1.SchemeGroupVersion.WithKind("StatefulSet")

var ObjectTypeError = fmt.Errorf("object must be %s", GVK.GroupKind().String())

type accessorImpl struct{}

func New() workload.Accessor {
	return &accessorImpl{}
}

func (s *accessorImpl) GroupVersionKind() schema.GroupVersionKind {
	return GVK
}

func (c *accessorImpl) DependentWorkloadGVKs() []schema.GroupVersionKind {
	return nil
}

func (s *accessorImpl) Watchable() bool {
	return true
}

func (s *accessorImpl) NewObject() client.Object {
	return &appsv1.StatefulSet{}
}

func (s *accessorImpl) NewObjectList() client.ObjectList {
	return &appsv1.StatefulSetList{}
}

func (s *accessorImpl) GetInfo(cluster string, object client.Object) (*workload.Info, error) {
	obj, err := checkObj(object)
	if err != nil {
		return nil, err
	}

	return workload.NewInfo(cluster, GVK, obj, s.getStatus(obj)), nil
}

func (p *accessorImpl) getStatus(obj *appsv1.StatefulSet) workload.InfoStatus {
	info := workload.InfoStatus{
		ObservedGeneration: obj.Status.ObservedGeneration,
		StableRevision:     obj.Status.CurrentRevision,
		UpdatedRevision:    obj.Status.UpdateRevision,
		Replicas:           ptr.Deref(obj.Spec.Replicas, 0),
		UpdatedReplicas:    obj.Status.UpdatedReplicas,
	}

	if obj.Status.CurrentRevision == obj.Status.UpdateRevision {
		// if someone delete pod directly, the status.updatedReplicas will be reduced firstly
		// then Status.ReadyReplicas and Status.AvailableReplicas will be reduced.
		// So we neet to respect the min value.
		info.UpdatedReadyReplicas = min(obj.Status.ReadyReplicas, obj.Status.UpdatedReplicas)
		info.UpdatedAvailableReplicas = min(obj.Status.AvailableReplicas, obj.Status.UpdatedReplicas)
	} else {
		// status.ReadyReplicas contains all current and updated replicas
		info.UpdatedReadyReplicas = min(max(obj.Status.ReadyReplicas-obj.Status.CurrentReplicas, 0), obj.Status.UpdatedReplicas)
		info.UpdatedAvailableReplicas = min(max(obj.Status.AvailableReplicas-obj.Status.CurrentReplicas, 0), obj.Status.UpdatedReplicas)
	}

	return info
}

func checkObj(object client.Object) (*appsv1.StatefulSet, error) {
	obj, ok := object.(*appsv1.StatefulSet)
	if !ok {
		return nil, ObjectTypeError
	}
	return obj, nil
}

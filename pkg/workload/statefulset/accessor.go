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

func (s *accessorImpl) GetInfo(cluster string, obj client.Object) (*workload.Info, error) {
	_, ok := obj.(*appsv1.StatefulSet)
	if !ok {
		return nil, ObjectTypeError
	}

	return workload.NewInfo(cluster, GVK, obj, s.getStatus(obj)), nil
}

func (p *accessorImpl) getStatus(obj client.Object) workload.InfoStatus {
	sts := obj.(*appsv1.StatefulSet)
	return workload.InfoStatus{
		ObservedGeneration:       sts.Status.ObservedGeneration,
		StableRevision:           sts.Status.CurrentRevision,
		UpdatedRevision:          sts.Status.UpdateRevision,
		Replicas:                 ptr.Deref(sts.Spec.Replicas, 0),
		UpdatedReplicas:          sts.Status.UpdatedReplicas,
		UpdatedReadyReplicas:     sts.Status.UpdatedReplicas,
		UpdatedAvailableReplicas: sts.Status.UpdatedReplicas,
	}
}

func (s *accessorImpl) ReleaseControl() workload.ReleaseControl {
	return &releaseControl{}
}

func (s *accessorImpl) PodControl(client.Reader) workload.PodControl {
	return &podControl{}
}

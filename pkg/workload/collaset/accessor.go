/**
 * Copyright 2024 The KusionStack Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     https://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package collaset

import (
	"fmt"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"
	operatingv1alpha1 "kusionstack.io/kube-api/apps/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"kusionstack.io/rollout/pkg/workload"
)

// GVK is the GroupVersionKind of the CollaSet
var GVK = schema.GroupVersionKind{
	Group:   operatingv1alpha1.GroupVersion.Group,
	Version: operatingv1alpha1.GroupVersion.Version,
	Kind:    "CollaSet",
}

var ObjectTypeError = fmt.Errorf("object must be %s", GVK.GroupKind().String())

type accessorImpl struct{}

func New() workload.Accessor {
	return &accessorImpl{}
}

func (w *accessorImpl) GroupVersionKind() schema.GroupVersionKind {
	return GVK
}

func (c *accessorImpl) DependentWorkloadGVKs() []schema.GroupVersionKind {
	return nil
}

func (w *accessorImpl) Watchable() bool {
	return true
}

func (w *accessorImpl) NewObject() client.Object {
	return &operatingv1alpha1.CollaSet{}
}

func (w *accessorImpl) NewObjectList() client.ObjectList {
	return &operatingv1alpha1.CollaSetList{}
}

func (w *accessorImpl) GetInfo(cluster string, obj client.Object) (*workload.Info, error) {
	_, ok := obj.(*operatingv1alpha1.CollaSet)
	if !ok {
		return nil, ObjectTypeError
	}
	return workload.NewInfo(cluster, GVK, obj, w.getStatus(obj)), nil
}

func (w *accessorImpl) getStatus(obj client.Object) workload.InfoStatus {
	cs := obj.(*operatingv1alpha1.CollaSet)
	return workload.InfoStatus{
		StableRevision:           cs.Status.CurrentRevision,
		UpdatedRevision:          cs.Status.UpdatedRevision,
		ObservedGeneration:       cs.Status.ObservedGeneration,
		Replicas:                 ptr.Deref(cs.Spec.Replicas, 0),
		UpdatedReplicas:          cs.Status.UpdatedReplicas,
		UpdatedReadyReplicas:     cs.Status.UpdatedReadyReplicas,
		UpdatedAvailableReplicas: cs.Status.UpdatedAvailableReplicas,
	}
}

func (s *accessorImpl) ReleaseControl() workload.ReleaseControl {
	return &releaseControl{}
}

func (s *accessorImpl) PodControl(client.Reader) workload.PodControl {
	return &podControl{}
}

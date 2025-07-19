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

	"kusionstack.io/rollout/pkg/utils/accessor"
	"kusionstack.io/rollout/pkg/workload"
)

// GVK is the GroupVersionKind of the CollaSet
var GVK = schema.GroupVersionKind{
	Group:   operatingv1alpha1.GroupVersion.Group,
	Version: operatingv1alpha1.GroupVersion.Version,
	Kind:    "CollaSet",
}

var ObjectTypeError = fmt.Errorf("object must be %s", GVK.GroupKind().String())

var _ workload.Accessor = &accessorImpl{}

type accessorImpl struct {
	accessor.ObjectAccessor
}

func New() workload.Accessor {
	return &accessorImpl{
		ObjectAccessor: accessor.NewObjectAccessor(
			GVK,
			&operatingv1alpha1.CollaSet{},
			&operatingv1alpha1.CollaSetList{},
		),
	}
}

func (c *accessorImpl) DependentWorkloadGVKs() []schema.GroupVersionKind {
	return nil
}

func (w *accessorImpl) Watchable() bool {
	return true
}

func (w *accessorImpl) GetInfo(cluster string, object client.Object) (*workload.Info, error) {
	obj, err := checkObj(object)
	if err != nil {
		return nil, err
	}
	return workload.NewInfo(cluster, GVK, obj, w.getStatus(obj)), nil
}

func (w *accessorImpl) getStatus(obj *operatingv1alpha1.CollaSet) workload.InfoStatus {
	return workload.InfoStatus{
		StableRevision:           obj.Status.CurrentRevision,
		UpdatedRevision:          obj.Status.UpdatedRevision,
		ObservedGeneration:       obj.Status.ObservedGeneration,
		Replicas:                 ptr.Deref(obj.Spec.Replicas, 0),
		UpdatedReplicas:          obj.Status.UpdatedReplicas,
		UpdatedReadyReplicas:     obj.Status.UpdatedReadyReplicas,
		UpdatedAvailableReplicas: obj.Status.UpdatedAvailableReplicas,
	}
}

func checkObj(object client.Object) (*operatingv1alpha1.CollaSet, error) {
	obj, ok := object.(*operatingv1alpha1.CollaSet)
	if !ok {
		return nil, ObjectTypeError
	}
	return obj, nil
}

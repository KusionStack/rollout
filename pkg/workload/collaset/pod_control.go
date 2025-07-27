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
	"context"
	"fmt"

	"github.com/samber/lo"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"kusionstack.io/rollout/pkg/workload"
)

var _ workload.ReplicaObjectControl = &accessorImpl{}

func (c *accessorImpl) ReplicaType() schema.GroupVersionKind {
	return corev1.SchemeGroupVersion.WithKind("Pod")
}

func (c *accessorImpl) RecognizeRevision(_ context.Context, _ client.Reader, workload, obj client.Object) (isCurrent, isUpdated bool, err error) {
	cls, err := checkObj(workload)
	if err != nil {
		return false, false, err
	}
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		return false, false, fmt.Errorf("object must be Pod")
	}
	revision := pod.Labels[appsv1.ControllerRevisionHashLabelKey]
	if revision == cls.Status.CurrentRevision {
		isCurrent = true
	}
	if cls.Generation == cls.Status.ObservedGeneration &&
		revision == cls.Status.UpdatedRevision {
		isUpdated = true
	}
	return isCurrent, isUpdated, nil
}

func (c *accessorImpl) GetReplicObjects(ctx context.Context, reader client.Reader, workload client.Object) ([]client.Object, error) {
	obj, err := checkObj(workload)
	if err != nil {
		return nil, err
	}
	selector, err := metav1.LabelSelectorAsSelector(obj.Spec.Selector)
	if err != nil {
		return nil, err
	}

	podList := &corev1.PodList{}
	err = reader.List(ctx, podList, client.InNamespace(obj.Namespace), client.MatchingLabelsSelector{Selector: selector})
	if err != nil {
		return nil, err
	}

	return lo.Map(podList.Items, func(pod corev1.Pod, _ int) client.Object { return &pod }), nil
}

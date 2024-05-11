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

package statefulset

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"kusionstack.io/rollout/pkg/utils"
)

type podControl struct{}

func (c *podControl) IsUpdatedPod(obj client.Object, pod *corev1.Pod) (bool, error) {
	sts, ok := obj.(*appsv1.StatefulSet)
	if !ok {
		return false, ObjectTypeError
	}
	revision := utils.GetMapValueByDefault(pod.Labels, appsv1.ControllerRevisionHashLabelKey, sts.Status.CurrentRevision)
	if revision == sts.Status.UpdateRevision {
		return true, nil
	}
	return false, nil
}

func (c *podControl) GetPodSelector(obj client.Object) (labels.Selector, error) {
	sts, ok := obj.(*appsv1.StatefulSet)
	if !ok {
		return nil, ObjectTypeError
	}
	selector, err := metav1.LabelSelectorAsSelector(sts.Spec.Selector)
	if err != nil {
		return nil, err
	}
	return selector, nil
}

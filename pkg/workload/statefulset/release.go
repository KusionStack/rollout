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
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"kusionstack.io/rollout/apis/rollout/v1alpha1"
	rolloutv1alpha1 "kusionstack.io/rollout/apis/rollout/v1alpha1"
	"kusionstack.io/rollout/pkg/workload"
)

var (
	_ workload.CanaryReleaseControl = &accessorImpl{}
	_ workload.BatchReleaseControl  = &accessorImpl{}
)

func (c *accessorImpl) BatchPreCheck(object client.Object) error {
	obj, err := checkObj(object)
	if err != nil {
		return err
	}
	if obj.Spec.UpdateStrategy.Type != appsv1.RollingUpdateStatefulSetStrategyType {
		return fmt.Errorf("rollout can not upgrade partition in StatefulSet if the upgrade strategy type is not RollingUpdate")
	}
	return nil
}

func (c *accessorImpl) ApplyPartition(object client.Object, expectedUpdated intstr.IntOrString) error {
	obj, err := checkObj(object)
	if err != nil {
		return err
	}
	// get current partition number
	specPartition := int32(0)
	if obj.Spec.UpdateStrategy.RollingUpdate != nil {
		specPartition = ptr.Deref(obj.Spec.UpdateStrategy.RollingUpdate.Partition, 0)
	}

	expectedPartition, err := workload.CalculateExpectedPartition(obj.Spec.Replicas, expectedUpdated, specPartition)
	if err != nil {
		return err
	}

	if expectedPartition > 0 {
		obj.Spec.UpdateStrategy = appsv1.StatefulSetUpdateStrategy{
			Type: appsv1.RollingUpdateStatefulSetStrategyType,
			RollingUpdate: &appsv1.RollingUpdateStatefulSetStrategy{
				Partition: ptr.To(expectedPartition),
			},
		}
	} else if obj.Spec.UpdateStrategy.RollingUpdate != nil {
		// omit partition when it is zero, zero means update all
		obj.Spec.UpdateStrategy.RollingUpdate.Partition = nil
	}
	return nil
}

func (c *accessorImpl) CanaryPreCheck(object client.Object) error {
	return nil
}

func (c *accessorImpl) Scale(object client.Object, replicas int32) error {
	obj, err := checkObj(object)
	if err != nil {
		return err
	}
	obj.Spec.Replicas = &replicas
	return nil
}

func (c *accessorImpl) ApplyCanaryPatch(object client.Object, podTemplatePatch *v1alpha1.MetadataPatch) error {
	obj, err := checkObj(object)
	if err != nil {
		return err
	}
	applyPodTemplateMetadataPatch(obj, podTemplatePatch)
	return nil
}

func applyPodTemplateMetadataPatch(obj *appsv1.StatefulSet, patch *rolloutv1alpha1.MetadataPatch) {
	if patch == nil {
		return
	}
	if len(patch.Labels) > 0 {
		if obj.Spec.Selector.MatchLabels == nil {
			obj.Spec.Selector.MatchLabels = make(map[string]string)
		}
		for k, v := range patch.Labels {
			obj.Spec.Selector.MatchLabels[k] = v
		}

		if obj.Spec.Template.Labels == nil {
			obj.Spec.Template.Labels = make(map[string]string)
		}
		for k, v := range patch.Labels {
			obj.Spec.Template.Labels[k] = v
		}
	}
	if len(patch.Annotations) > 0 {
		if obj.Spec.Template.Annotations == nil {
			obj.Spec.Template.Annotations = make(map[string]string)
		}
		for k, v := range patch.Annotations {
			obj.Spec.Template.Annotations[k] = v
		}
	}
}

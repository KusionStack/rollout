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
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	operatingv1alpha1 "kusionstack.io/kube-api/apps/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"kusionstack.io/rollout/apis/rollout/v1alpha1"
	rolloutv1alpha1 "kusionstack.io/rollout/apis/rollout/v1alpha1"
	"kusionstack.io/rollout/pkg/workload"
)

type releaseControl struct{}

func (c *releaseControl) BatchPreCheck(object client.Object) error {
	return nil
}

func (c *releaseControl) ApplyPartition(object client.Object, partition intstr.IntOrString) error {
	obj, err := c.checkObj(object)
	if err != nil {
		return err
	}
	expectedPartition, err := workload.CalculatePartitionReplicas(obj.Spec.Replicas, partition)
	if err != nil {
		return err
	}

	currentPartition := int32(0)
	if obj.Spec.UpdateStrategy.RollingUpdate != nil && obj.Spec.UpdateStrategy.RollingUpdate.ByPartition != nil {
		currentPartition = ptr.Deref(obj.Spec.UpdateStrategy.RollingUpdate.ByPartition.Partition, 0)
	}

	if currentPartition >= expectedPartition {
		return nil
	}

	// update
	obj.Spec.UpdateStrategy.RollingUpdate = &operatingv1alpha1.RollingUpdateCollaSetStrategy{
		ByPartition: &operatingv1alpha1.ByPartition{
			Partition: ptr.To(expectedPartition),
		},
	}

	return nil
}

func (c *releaseControl) CanaryPreCheck(object client.Object) error {
	return nil
}

func (c *releaseControl) Scale(object client.Object, replicas int32) error {
	obj, err := c.checkObj(object)
	if err != nil {
		return err
	}
	obj.Spec.Replicas = &replicas
	return nil
}

func (c *releaseControl) ApplyCanaryPatch(object client.Object, podTemplatePatch *v1alpha1.MetadataPatch) error {
	obj, err := c.checkObj(object)
	if err != nil {
		return err
	}
	applyPodTemplateMetadataPatch(obj, podTemplatePatch)
	return nil
}

func (c *releaseControl) checkObj(object client.Object) (*operatingv1alpha1.CollaSet, error) {
	obj, ok := object.(*operatingv1alpha1.CollaSet)
	if !ok {
		return nil, ObjectTypeError
	}
	return obj, nil
}

func applyPodTemplateMetadataPatch(obj *operatingv1alpha1.CollaSet, patch *rolloutv1alpha1.MetadataPatch) {
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

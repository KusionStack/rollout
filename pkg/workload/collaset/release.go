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
	"maps"

	"k8s.io/utils/ptr"
	operatingv1alpha1 "kusionstack.io/kube-api/apps/v1alpha1"
	rolloutv1alpha1 "kusionstack.io/kube-api/rollout/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"

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

	if obj.Spec.UpdateStrategy.RollingUpdate != nil && obj.Spec.UpdateStrategy.RollingUpdate.ByLabel != nil {
		return fmt.Errorf("rollout can not upgrade partition in CollaSet if the 'spec.updateStrategy.rollingUpdate.byLabel' is not nil")
	}
	return nil
}

func (c *accessorImpl) ApplyPartition(object client.Object, expectedUpdated int32) error {
	obj, err := checkObj(object)
	if err != nil {
		return err
	}

	specPartition := int32(0)
	if obj.Spec.UpdateStrategy.RollingUpdate != nil && obj.Spec.UpdateStrategy.RollingUpdate.ByPartition != nil {
		specPartition = ptr.Deref(obj.Spec.UpdateStrategy.RollingUpdate.ByPartition.Partition, 0)
	}

	expectedPartition := workload.CalculateExpectedPartition(obj.Spec.Replicas, expectedUpdated, specPartition)

	if expectedPartition > 0 {
		obj.Spec.UpdateStrategy.RollingUpdate = &operatingv1alpha1.RollingUpdateCollaSetStrategy{
			ByPartition: &operatingv1alpha1.ByPartition{
				Partition: ptr.To(expectedPartition),
			},
		}
	} else if obj.Spec.UpdateStrategy.RollingUpdate != nil &&
		obj.Spec.UpdateStrategy.RollingUpdate.ByPartition != nil {
		// expectedPartition == 0
		obj.Spec.UpdateStrategy.RollingUpdate.ByPartition.Partition = nil
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

func (c *accessorImpl) ApplyCanaryPatch(object client.Object, podTemplatePatch *rolloutv1alpha1.MetadataPatch) error {
	obj, err := checkObj(object)
	if err != nil {
		return err
	}
	applyPodTemplateMetadataPatch(obj, podTemplatePatch)
	return nil
}

func applyPodTemplateMetadataPatch(obj *operatingv1alpha1.CollaSet, patch *rolloutv1alpha1.MetadataPatch) {
	if patch == nil {
		return
	}
	if len(patch.Labels) > 0 {
		if obj.Spec.Selector.MatchLabels == nil {
			obj.Spec.Selector.MatchLabels = make(map[string]string)
		}
		maps.Copy(obj.Spec.Selector.MatchLabels, patch.Labels)

		if obj.Spec.Template.Labels == nil {
			obj.Spec.Template.Labels = make(map[string]string)
		}
		maps.Copy(obj.Spec.Template.Labels, patch.Labels)
	}
	if len(patch.Annotations) > 0 {
		if obj.Spec.Template.Annotations == nil {
			obj.Spec.Template.Annotations = make(map[string]string)
		}
		maps.Copy(obj.Spec.Template.Annotations, patch.Annotations)
	}
}

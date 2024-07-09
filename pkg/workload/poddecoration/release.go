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

package poddecoration

import (
	"fmt"

	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	operatingv1alpha1 "kusionstack.io/kube-api/apps/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"kusionstack.io/rollout/pkg/workload"
)

var _ workload.BatchReleaseControl = &accessorImpl{}

func (c *accessorImpl) BatchPreCheck(object client.Object) error {
	obj, err := checkObj(object)
	if err != nil {
		return err
	}

	if obj.Spec.UpdateStrategy.RollingUpdate != nil && obj.Spec.UpdateStrategy.RollingUpdate.Selector != nil {
		return fmt.Errorf("rollout can not upgrade partition in PodDecoration if the 'spec.updateStrategy.rollingUpdate.selector' is not nil")
	}
	return nil
}

func (c *accessorImpl) ApplyPartition(object client.Object, partition intstr.IntOrString) error {
	// object must be *operatingv1alpha1.PodDecoration
	obj, err := checkObj(object)
	if err != nil {
		return err
	}

	// partition in PodDecoration means the number of pods with current revision.
	expectedUpdatedPartition, err := workload.CalculatePartitionReplicas(&obj.Status.MatchedPods, partition)
	if err != nil {
		return err
	}

	totalReplicas := obj.Status.MatchedPods

	var specPartition int32
	if obj.Spec.UpdateStrategy.RollingUpdate != nil {
		specPartition = ptr.Deref(obj.Spec.UpdateStrategy.RollingUpdate.Partition, 0)
	}
	currentUpdatedPartition := totalReplicas - specPartition

	if currentUpdatedPartition >= expectedUpdatedPartition {
		// already updated if the current updated partition is greater than or equal to the expected updated partition
		return nil
	}

	// calculate the expected partition
	realPartition := totalReplicas - expectedUpdatedPartition
	// update partition
	obj.Spec.UpdateStrategy.RollingUpdate = &operatingv1alpha1.PodDecorationRollingUpdate{
		Partition: ptr.To(realPartition),
	}
	if realPartition == 0 {
		obj.Spec.UpdateStrategy.RollingUpdate = nil
	}

	return nil
}
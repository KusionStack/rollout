/**
 * Copyright 2024 The KusionStack Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
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
	"encoding/json"
	"fmt"

	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	operatingv1alpha1 "kusionstack.io/kube-api/apps/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	rolloutapi "kusionstack.io/rollout/apis/rollout"
	rolloutv1alpha1 "kusionstack.io/rollout/apis/rollout/v1alpha1"
	"kusionstack.io/rollout/pkg/utils"
	"kusionstack.io/rollout/pkg/workload"
)

func (w *workloadImpl) BatchStrategy() workload.BatchStrategy {
	return &batchStrategy{
		workloadImpl: w,
	}
}

var _ workload.BatchStrategy = &batchStrategy{}

type batchStrategy struct {
	*workloadImpl
}

func (s *batchStrategy) Initialize(rollout, rolloutRun string, batchIndex int32) error {
	info := rolloutv1alpha1.ProgressingInfo{
		RolloutName: rollout,
		RolloutID:   rolloutRun,
		Batch: &rolloutv1alpha1.BatchProgressingInfo{
			CurrentBatchIndex: batchIndex,
		},
	}
	progress, _ := json.Marshal(info)

	// set progressing info
	return s.UpdateOnConflict(context.TODO(), func(obj client.Object) error {
		utils.MutateAnnotations(obj, func(annotations map[string]string) {
			annotations[rolloutapi.AnnoRolloutProgressingInfo] = string(progress)
		})
		return nil
	})
}

func (s *batchStrategy) UpgradePartition(partition intstr.IntOrString) (bool, error) {
	expectedPartition, err := workload.CalculatePartitionReplicas(s.obj.Spec.Replicas, partition)
	if err != nil {
		return false, err
	}

	currentPartition := int32(0)
	if s.obj.Spec.UpdateStrategy.RollingUpdate != nil && s.obj.Spec.UpdateStrategy.RollingUpdate.ByPartition != nil {
		currentPartition = ptr.Deref[int32](s.obj.Spec.UpdateStrategy.RollingUpdate.ByPartition.Partition, 0)
	}

	if currentPartition >= expectedPartition {
		return false, nil
	}

	// update
	err = s.UpdateOnConflict(context.TODO(), func(o client.Object) error {
		collaset, ok := o.(*operatingv1alpha1.CollaSet)
		if !ok {
			return fmt.Errorf("expect client.Object to be *operatingv1alpha1.CollaSet")
		}
		collaset.Spec.UpdateStrategy.RollingUpdate = &operatingv1alpha1.RollingUpdateCollaSetStrategy{
			ByPartition: &operatingv1alpha1.ByPartition{
				Partition: ptr.To[int32](expectedPartition),
			},
		}
		return nil
	})

	if err != nil {
		return false, err
	}
	return true, nil
}

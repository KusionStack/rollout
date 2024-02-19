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

package statefulset

import (
	"context"
	"encoding/json"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
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
	if s.obj.Spec.UpdateStrategy.Type != appsv1.RollingUpdateStatefulSetStrategyType {
		return fmt.Errorf("rollout can not upgrade partition in StatefulSet if the upgrade strategy type is not RollingUpdate")
	}

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
	obj := s.obj

	expectedReplicas, err := workload.CalculatePartitionReplicas(obj.Spec.Replicas, partition)
	if err != nil {
		return false, err
	}

	// get current partition number
	currentPartition := int32(0)
	if obj.Spec.UpdateStrategy.RollingUpdate != nil {
		currentPartition = ptr.Deref[int32](obj.Spec.UpdateStrategy.RollingUpdate.Partition, 0)
	}

	// get total replicas number
	totalReplicas := ptr.Deref[int32](obj.Spec.Replicas, 0)
	// if totalReplicas == 100, expectReplicas == 10, then expectedPartition is 90
	expectedPartition := totalReplicas - expectedReplicas

	if currentPartition <= expectedPartition {
		// already update
		return false, nil
	}

	// we need to update partiton here
	err = s.UpdateOnConflict(context.TODO(), func(obj client.Object) error {
		sts, ok := obj.(*appsv1.StatefulSet)
		if !ok {
			return fmt.Errorf("expect client.Object to be *appsv1.StatefulSet")
		}

		sts.Spec.UpdateStrategy = appsv1.StatefulSetUpdateStrategy{
			Type: appsv1.RollingUpdateStatefulSetStrategyType,
			RollingUpdate: &appsv1.RollingUpdateStatefulSetStrategy{
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

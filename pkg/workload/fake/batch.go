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

package fake

import (
	"encoding/json"

	"k8s.io/apimachinery/pkg/util/intstr"

	rolloutapi "kusionstack.io/rollout/apis/rollout"
	rolloutv1alpha1 "kusionstack.io/rollout/apis/rollout/v1alpha1"
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
	if s.Info.Annotations == nil {
		s.Info.Annotations = make(map[string]string)
	}

	s.Info.Annotations[rolloutapi.AnnoRolloutProgressingInfo] = string(progress)
	return nil
}

func (s *batchStrategy) UpgradePartition(partition intstr.IntOrString) (bool, error) {
	partitionInt, _ := workload.CalculatePartitionReplicas(&s.Status.Replicas, partition)
	if partitionInt <= s.Partition {
		// already updated
		return false, nil
	}
	s.ChangeStatus(s.Status.Replicas, partitionInt, partitionInt)
	return true, nil
}

// Copyright 2023 The KusionStack Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package workload

import (
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	"kusionstack.io/kube-utils/multicluster/clusterinfo"

	rolloutv1alpha1 "kusionstack.io/rollout/apis/rollout/v1alpha1"
)

func GetClusterFromLabel(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}
	return labels[clusterinfo.ClusterLabelKey]
}

func CalculatePartitionReplicas(totalReplicas *int32, partition intstr.IntOrString) (int, error) {
	replicas := ptr.Deref[int32](totalReplicas, 0)
	if replicas == 0 {
		return 0, nil
	}
	return intstr.GetScaledValueFromIntOrPercent(&partition, int(replicas), true)
}

func CheckPartitionReady(status rolloutv1alpha1.RolloutWorkloadStatus, partiton int32) bool {
	if status.Generation != status.ObservedGeneration {
		return false
	}

	return status.UpdatedAvailableReplicas >= partiton
}

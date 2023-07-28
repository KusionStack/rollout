/*
 * Copyright 2023 The KusionStack Authors.
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

package workload

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/KusionStack/rollout/api/v1alpha1"
)

// Interface is the interface for workload
type Interface interface {
	// GetObj returns the workload object
	GetObj() client.Object

	// GetTypeMeta returns the type meta of the workload
	GetTypeMeta() *metav1.TypeMeta

	// GetIdentity returns the identity of the workload
	GetIdentity() string

	// GetCluster returns the cluster of the workload
	GetCluster() string

	// UpgradePartition upgrades the workload to the specified partition
	UpgradePartition(partition *intstr.IntOrString) (bool, error)

	// CheckReady checks if the workload is ready
	CheckReady(expectUpdatedReplicas *int32) (bool, error)

	// CalculatePartitionReplicas calculates the replicas of the workload from the specified partition
	CalculatePartitionReplicas(partition *intstr.IntOrString) (int, error)

	// CalculateAtLeastUpdatedAvailableReplicas calculates the replicas of the workload from the specified failureThreshold
	CalculateAtLeastUpdatedAvailableReplicas(failureThreshold *intstr.IntOrString) (int, error)

	// GetReleaseStatus returns the release status of the workload
	GetReleaseStatus() *v1alpha1.WorkloadReleaseTaskStatus
}

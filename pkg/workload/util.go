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
	"context"
	"maps"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	rolloutapi "kusionstack.io/kube-api/rollout"
	rolloutv1alpha1 "kusionstack.io/kube-api/rollout/v1alpha1"
	"kusionstack.io/kube-utils/multicluster/clusterinfo"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func GetClusterFromLabel(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}
	return labels[clusterinfo.ClusterLabelKey]
}

func CalculateUpdatedReplicas(totalReplicas *int32, expectedReplicas intstr.IntOrString) (int32, error) {
	replicas := ptr.Deref(totalReplicas, 0)
	if replicas == 0 {
		return 0, nil
	}
	partitionInt, err := intstr.GetScaledValueFromIntOrPercent(&expectedReplicas, int(replicas), true)
	if err != nil {
		return 0, err
	}
	if partitionInt > int(replicas) {
		partitionInt = int(replicas)
	}
	return int32(partitionInt), nil
}

// CalculateExpectedPartition calculates the expected partition based on the total replicas, expected replicas, and the partition in the spec.
// In this function, partition means how many replicas are not updated.
func CalculateExpectedPartition(total *int32, expectedUpdatedReplicas, partitionInSpec int32) int32 {
	totalReplicas := ptr.Deref(total, 0)
	currentUpdatedReplicas := max(totalReplicas-partitionInSpec, 0)

	if currentUpdatedReplicas >= expectedUpdatedReplicas {
		// already updated if the current updated partition is greater than or equal to the expected updated partition
		return partitionInSpec
	}

	return max(totalReplicas-expectedUpdatedReplicas, 0)
}

// CalculateProgressingPartition calculates the progressing partition based on the total replicas, expected replicas, and the partition in the spec.
// In this function, partition means how many replicas need to be updated.
func CalculateProgressingPartition(total *int32, expectedUpdatedReplicas, partitionInSpec int32) int32 {
	totalReplicas := ptr.Deref(total, 0)

	if partitionInSpec >= expectedUpdatedReplicas {
		// already updated if the current updated partition is greater than or equal to the expected updated partition
		return partitionInSpec
	}

	return min(expectedUpdatedReplicas, totalReplicas)
}

// PatchMetadata patches metadata with the given patch
func PatchMetadata(meta *metav1.ObjectMeta, patch rolloutv1alpha1.MetadataPatch) {
	if len(patch.Labels) > 0 {
		if meta.Labels == nil {
			meta.Labels = make(map[string]string)
		}
		maps.Copy(meta.Labels, patch.Labels)
	}
	if len(patch.Annotations) > 0 {
		if meta.Annotations == nil {
			meta.Annotations = make(map[string]string)
		}
		maps.Copy(meta.Annotations, patch.Annotations)
	}
}

func IsControlledByRollout(workload client.Object) bool {
	_, ok := workload.GetLabels()[rolloutapi.LabelWorkload]
	return ok
}

func IsProgressing(workload client.Object) bool {
	_, ok := workload.GetAnnotations()[rolloutapi.AnnoRolloutProgressingInfo]
	return ok
}

func IsCanary(workload client.Object) bool {
	canaryValue := workload.GetLabels()[rolloutapi.CanaryResourceLabelKey]
	return canaryValue == "true"
}

type Owner struct {
	Ref *metav1.OwnerReference
	GVK schema.GroupVersionKind
}

func GetControllerOf(controllee client.Object) (*Owner, error) {
	owner := metav1.GetControllerOf(controllee)
	if owner == nil {
		// not found
		return nil, nil
	}

	gv, err := schema.ParseGroupVersion(owner.APIVersion)
	if err != nil {
		return nil, err
	}
	gvk := gv.WithKind(owner.Kind)
	return &Owner{Ref: owner, GVK: gvk}, nil
}

func GetOwnersOf(controllee client.Object) ([]*Owner, error) {
	refs := controllee.GetOwnerReferences()
	result := []*Owner{}
	for i := range refs {
		gv, err := schema.ParseGroupVersion(refs[i].APIVersion)
		if err != nil {
			return nil, err
		}
		gvk := gv.WithKind(refs[i].Kind)
		result = append(result, &Owner{
			Ref: &refs[i],
			GVK: gvk,
		})
	}
	return result, nil
}

func RecognizeTrafficLane(
	ctx context.Context,
	accessor Accessor,
	pc ReplicaObjectControl,
	reader client.Reader,
	workloadObj, replicaObj client.Object,
) string {
	if IsCanary(workloadObj) {
		// the workload canary resource, always set object revision to canary
		return rolloutapi.CanaryTrafficLane
	}
	if !IsProgressing(workloadObj) {
		// workload is not progressing, set the object revision to stable
		return rolloutapi.StableTrafficLane
	}

	// workload is progressing, check revision
	isCurrent, isUpdated, _ := pc.RecognizeRevision(ctx, reader, workloadObj, replicaObj)
	// sometimes the object can be both current and updated,
	// we must to set revision to stable if it is current
	if isCurrent {
		return rolloutapi.StableTrafficLane
	}
	// if workload and object is all updated, set revision to canary
	if isUpdated {
		return rolloutapi.CanaryTrafficLane
	}
	// if workload status is out of date, set revision to unknown
	info, _ := accessor.GetInfo("", workloadObj)
	if info.Generation != info.Status.ObservedGeneration {
		return rolloutapi.UnknownTrafficLane
	}

	// This is a special case that but the replicaObj is is neither current nor updated.
	// During the process of updating the workload from v2 to v2 version, another update to v3 occurred.
	// We need to treat the v2 replica object as stable.
	return rolloutapi.StableTrafficLane
}

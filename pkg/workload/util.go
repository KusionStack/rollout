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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	"kusionstack.io/kube-utils/multicluster/clusterinfo"
	"sigs.k8s.io/controller-runtime/pkg/client"

	rolloutapi "kusionstack.io/rollout/apis/rollout"
	rolloutv1alpha1 "kusionstack.io/rollout/apis/rollout/v1alpha1"
	"kusionstack.io/rollout/pkg/utils"
)

func GetClusterFromLabel(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}
	return labels[clusterinfo.ClusterLabelKey]
}

func CalculatePartitionReplicas(totalReplicas *int32, partition intstr.IntOrString) (int32, error) {
	replicas := ptr.Deref(totalReplicas, 0)
	if replicas == 0 {
		return 0, nil
	}
	partitionInt, err := intstr.GetScaledValueFromIntOrPercent(&partition, int(replicas), true)
	if err != nil {
		return 0, err
	}
	if partitionInt > int(replicas) {
		partitionInt = int(replicas)
	}
	return int32(partitionInt), nil
}

// PatchMetadata patches metadata with the given patch
func PatchMetadata(meta *metav1.ObjectMeta, patch rolloutv1alpha1.MetadataPatch) {
	if len(patch.Labels) > 0 {
		if meta.Labels == nil {
			meta.Labels = make(map[string]string)
		}
		for k, v := range patch.Labels {
			meta.Labels[k] = v
		}
	}
	if len(patch.Annotations) > 0 {
		if meta.Annotations == nil {
			meta.Annotations = make(map[string]string)
		}
		for k, v := range patch.Annotations {
			meta.Annotations[k] = v
		}
	}
}

func IsControlledByRollout(workload client.Object) bool {
	_, ok := utils.GetMapValue(workload.GetLabels(), rolloutapi.LabelWorkload)
	return ok
}

func IsProgressing(workload client.Object) bool {
	_, ok := utils.GetMapValue(workload.GetAnnotations(), rolloutapi.AnnoRolloutProgressingInfo)
	return ok
}

func IsCanary(workload client.Object) bool {
	_, ok := utils.GetMapValue(workload.GetLabels(), rolloutapi.LabelCanary)
	return ok
}

func GetOwnerAndGVK(obj client.Object) (*metav1.OwnerReference, schema.GroupVersionKind, error) {
	owner := metav1.GetControllerOf(obj)
	if owner == nil {
		// not found
		return nil, schema.GroupVersionKind{}, nil
	}

	gv, err := schema.ParseGroupVersion(owner.APIVersion)
	if err != nil {
		return nil, schema.GroupVersionKind{}, err
	}
	gvk := gv.WithKind(owner.Kind)
	return owner, gvk, nil
}

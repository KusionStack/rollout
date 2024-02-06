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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	rolloutv1alpha1 "kusionstack.io/rollout/apis/rollout/v1alpha1"
)

// Interface is the interface for workload
type Interface interface {
	// GetInfo returns basic workload informations.
	GetInfo() Info

	// IsWaitingRollout returns if the workload is waiting for rollout.
	IsWaitingRollout() bool

	// UpdateOnConflict try its best to updates the workload on conflict.
	UpdateOnConflict(ctx context.Context, modifyFunc func(obj client.Object) error) error

	// UpgradePartition upgrades the workload to the specified partition
	// It should return true if the workload changed.
	//
	// NOTE: This function must be idempotent.
	UpgradePartition(partition intstr.IntOrString, metadataPatch rolloutv1alpha1.MetadataPatch) (bool, error)

	// EnsureCanaryWorkload ensures the canary workload is created and updated.
	EnsureCanaryWorkload(canaryReplicas intstr.IntOrString, canaryMetadataPatch, podTemplatePatch *rolloutv1alpha1.MetadataPatch) (Interface, error)
}

// workload info
type Info struct {
	metav1.ObjectMeta
	// GVK is the GroupVersionKind of the workload.
	schema.GroupVersionKind
	// Status is the status of the workload.
	Status Status
}

// workload status
type Status struct {
	// ObservedGeneration is the most recent generation observed for this workload.
	ObservedGeneration int64
	// StableRevision is the old stable revision used to generate pods.
	StableRevision string
	// UpdatedRevision is the updated template revision used to generate pods.
	UpdatedRevision string
	// Replicas is the desired number of pods targeted by workload
	Replicas int32
	// UpdatedReplicas is the number of pods targeted by workload that have the updated template spec.
	UpdatedReplicas int32
	// UpdatedReadyReplicas is the number of ready pods targeted by workload that have the updated template spec.
	UpdatedReadyReplicas int32
	// UpdatedAvailableReplicas is the number of service available pods targeted by workload that have the updated template spec.
	UpdatedAvailableReplicas int32
}

func NewInfoFrom(cluster string, gvk schema.GroupVersionKind, obj client.Object, status Status) Info {
	return Info{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   obj.GetNamespace(),
			Name:        obj.GetName(),
			Labels:      obj.GetLabels(),
			ClusterName: cluster,
			Generation:  obj.GetGeneration(),
		},
		GroupVersionKind: gvk,
		Status:           status,
	}
}

func (o Info) NamespacedName() types.NamespacedName {
	return types.NamespacedName{
		Namespace: o.Namespace,
		Name:      o.Name,
	}
}

func (o Info) CheckPartitionReady(partiton int32) bool {
	if o.Generation != o.Status.ObservedGeneration {
		return false
	}
	return o.Status.UpdatedAvailableReplicas >= partiton
}

func (o Info) APIStatus() rolloutv1alpha1.RolloutWorkloadStatus {
	return rolloutv1alpha1.RolloutWorkloadStatus{
		RolloutReplicasSummary: rolloutv1alpha1.RolloutReplicasSummary{
			Replicas:                 o.Status.Replicas,
			UpdatedReplicas:          o.Status.UpdatedReplicas,
			UpdatedReadyReplicas:     o.Status.UpdatedReadyReplicas,
			UpdatedAvailableReplicas: o.Status.UpdatedAvailableReplicas,
		},
		Generation:         o.Generation,
		ObservedGeneration: o.Status.ObservedGeneration,
		StableRevision:     o.Status.StableRevision,
		UpdatedRevision:    o.Status.UpdatedRevision,
		Cluster:            o.ClusterName,
		Name:               o.Name,
	}
}

func (o Info) DeepCopy() Info {
	return Info{
		ObjectMeta:       *o.ObjectMeta.DeepCopy(),
		GroupVersionKind: o.GroupVersionKind,
		Status:           o.Status,
	}
}

type WorkloadMatcher interface {
	Matches(cluster, name string, label map[string]string) bool
}

type workloadMatcher struct {
	selector labels.Selector
	refs     []rolloutv1alpha1.CrossClusterObjectNameReference
}

func MatchAsMatcher(match rolloutv1alpha1.ResourceMatch) WorkloadMatcher {
	var selector labels.Selector
	if match.Selector != nil {
		selector, _ = metav1.LabelSelectorAsSelector(match.Selector)
	}

	return &workloadMatcher{
		selector: selector,
		refs:     match.Names,
	}
}

func (m *workloadMatcher) Matches(cluster, name string, label map[string]string) bool {
	if m.selector != nil {
		return m.selector.Matches(labels.Set(label))
	}
	for _, ref := range m.refs {
		if ref.Matches(cluster, name) {
			return true
		}
	}
	return false
}

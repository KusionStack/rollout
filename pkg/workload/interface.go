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
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	rolloutv1alpha1 "kusionstack.io/rollout/apis/rollout/v1alpha1"
)

type Info struct {
	Cluster   string
	Namespace string
	Name      string
	GVK       schema.GroupVersionKind
	Labels    map[string]string
}

func NewInfo(cluster string, gvk schema.GroupVersionKind, obj client.Object) Info {
	return Info{
		Cluster:   cluster,
		GVK:       gvk,
		Namespace: obj.GetNamespace(),
		Name:      obj.GetName(),
		Labels:    obj.GetLabels(),
	}
}

// Interface is the interface for workload
type Interface interface {
	// GetInfo returns basic workload informations.
	GetInfo() Info

	// GetStatus returns current workload status
	GetStatus() rolloutv1alpha1.RolloutWorkloadStatus

	// GetObj returns the workload object
	GetObj() client.Object

	IsWaitingRollout() bool

	// UpgradePartition upgrades the workload to the specified partition
	UpgradePartition(partition *intstr.IntOrString) (bool, error)

	// CheckReady checks if the workload is ready
	CheckReady(expectUpdatedReplicas *int32) (bool, error)

	// CalculateAtLeastUpdatedAvailableReplicas calculates the replicas of the workload from the specified failureThreshold
	CalculateAtLeastUpdatedAvailableReplicas(failureThreshold *intstr.IntOrString) (int, error)

	UpdateOnConflict(ctx context.Context, modifyFunc func(obj client.Object) error) error
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

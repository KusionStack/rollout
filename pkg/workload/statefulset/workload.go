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

package statefulset

import (
	"context"
	"fmt"
	"math"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/pointer"
	"kusionstack.io/kube-utils/multicluster/clusterinfo"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	rolloutv1alpha1 "kusionstack.io/rollout/apis/rollout/v1alpha1"
	"kusionstack.io/rollout/pkg/utils"
	"kusionstack.io/rollout/pkg/workload"
)

var GVK = appsv1.SchemeGroupVersion.WithKind("StatefulSet")

type realWorkload struct {
	info workload.Info

	obj *appsv1.StatefulSet

	client client.Client
}

// GetInfo implements workload.Interface.
func (w *realWorkload) GetInfo() workload.Info {
	return w.info
}

func (w *realWorkload) GetStatus() rolloutv1alpha1.RolloutWorkloadStatus {
	return rolloutv1alpha1.RolloutWorkloadStatus{
		Name:               w.info.Name,
		Cluster:            w.info.Cluster,
		Generation:         w.obj.Generation,
		ObservedGeneration: w.obj.Status.ObservedGeneration,
		StableRevision:     w.obj.Status.CurrentRevision,
		UpdatedRevision:    w.obj.Status.UpdateRevision,
		RolloutReplicasSummary: rolloutv1alpha1.RolloutReplicasSummary{
			Replicas:                 *w.obj.Spec.Replicas,
			UpdatedReplicas:          w.obj.Status.UpdatedReplicas,
			UpdatedReadyReplicas:     w.obj.Status.UpdatedReplicas,
			UpdatedAvailableReplicas: w.obj.Status.UpdatedReplicas,
		},
	}
}

func (w *realWorkload) GetObj() client.Object {
	return w.obj
}

func (w *realWorkload) UpdateObject(obj client.Object) {
	sts, ok := obj.(*appsv1.StatefulSet)
	if !ok {
		return
	}
	w.obj = sts
}

func (w *realWorkload) IsWaitingRollout() bool {
	sts := w.obj

	if len(sts.Status.CurrentRevision) != 0 &&
		sts.Status.CurrentRevision != sts.Status.UpdateRevision &&
		sts.Status.UpdatedReplicas == 0 {
		return true
	}

	return false
}

func (w *realWorkload) UpgradePartition(partition *intstr.IntOrString) (bool, error) {
	expectReplicas, err := w.CalculatePartitionReplicas(partition)
	if err != nil {
		return false, err
	}

	replicas := *w.obj.Spec.Replicas
	if int32(expectReplicas) > *w.obj.Spec.Replicas {
		return false, fmt.Errorf(fmt.Sprintf("expectReplicas %d must lte replicas %d", expectReplicas, replicas))
	}

	oldPartition := w.obj.Spec.UpdateStrategy.RollingUpdate.Partition
	w.obj.Spec.UpdateStrategy.RollingUpdate.Partition = pointer.Int32(*w.obj.Spec.Replicas - int32(expectReplicas))
	if oldPartition == nil ||
		*oldPartition != *w.obj.Spec.UpdateStrategy.RollingUpdate.Partition {
		return true, w.client.Update(clusterinfo.WithCluster(context.Background(), w.info.Cluster), w.obj)
	}

	return false, nil
}

func (w *realWorkload) CheckReady(expectUpdatedReplicas *int32) (bool, error) {
	if w.obj.GetGeneration() != w.obj.Status.ObservedGeneration {
		return false, nil
	}

	if (*w.obj.Spec.Replicas) == 0 {
		return false, nil
	}

	var expectUpdatedAvailableReplicas int32
	if expectUpdatedReplicas != nil {
		expectUpdatedAvailableReplicas = *expectUpdatedReplicas
	} else {
		replica := *w.obj.Spec.Replicas
		partition := *w.obj.Spec.UpdateStrategy.RollingUpdate.Partition
		if partition >= replica {
			expectUpdatedAvailableReplicas = math.MaxInt16
		} else {
			expectUpdatedAvailableReplicas = replica - partition
		}
	}
	if w.obj.Status.UpdatedReplicas >= expectUpdatedAvailableReplicas {
		return true, nil
	}

	return false, nil
}

func (w *realWorkload) CalculatePartitionReplicas(partition *intstr.IntOrString) (int, error) {
	if w.obj.Spec.Replicas == nil {
		return intstr.GetScaledValueFromIntOrPercent(partition, 0, true)
	}
	return intstr.GetScaledValueFromIntOrPercent(partition, int(*w.obj.Spec.Replicas), true)
}

func (w *realWorkload) CalculateAtLeastUpdatedAvailableReplicas(failureThreshold *intstr.IntOrString) (int, error) {
	failureReplicas, err := intstr.GetScaledValueFromIntOrPercent(failureThreshold, int(*w.obj.Spec.Replicas), true)
	if err != nil {
		return 0, err
	}

	return int(*w.obj.Spec.Replicas) - failureReplicas, nil
}

func (w *realWorkload) UpdateOnConflict(ctx context.Context, modifyFunc func(obj client.Object) error) error {
	obj := w.obj
	result, err := utils.UpdateOnConflict(clusterinfo.WithCluster(ctx, w.info.Cluster), w.client, w.client, obj, func() error {
		return modifyFunc(obj)
	})
	if err != nil {
		return err
	}
	if result == controllerutil.OperationResultUpdated {
		// update local reference
		w.obj = obj
	}
	return nil
}

func (w *realWorkload) Matches(match rolloutv1alpha1.ResourceMatch) bool {
	macher := workload.MatchAsMatcher(match)
	return macher.Matches(w.info.Cluster, w.obj.Name, w.obj.Labels)
}

func NewWorkload(client client.Client, key types.NamespacedName, cluster string) (workload.Interface, error) {
	var obj appsv1.StatefulSet
	if err := client.Get(clusterinfo.WithCluster(context.Background(), cluster), key, &obj); err != nil {
		return nil, err
	}
	return &realWorkload{
		info:   workload.NewInfo(cluster, GVK, &obj),
		client: client,
		obj:    &obj,
	}, nil
}

func NewWorkloadSet(client client.Client, namespace string, match rolloutv1alpha1.ResourceMatch) (*workload.Set, error) {
	list, err := NewWorkloadList(client, namespace, match)
	if err != nil {
		return nil, err
	}
	return workload.NewWorkloadSet(list...), nil
}

func NewWorkloadList(c client.Client, namespace string, match rolloutv1alpha1.ResourceMatch) ([]workload.Interface, error) {
	var list appsv1.StatefulSetList
	if err := c.List(clusterinfo.ContextClusters, &list, &client.ListOptions{Namespace: namespace}); err != nil {
		return nil, err
	}
	matcher := workload.MatchAsMatcher(match)
	workloads := make([]workload.Interface, 0, len(list.Items))
	for _, obj := range list.Items {
		cluster := workload.GetClusterFromLabel(obj.Labels)
		if !matcher.Matches(cluster, obj.GetName(), obj.Labels) {
			continue
		}
		workloads = append(workloads, &realWorkload{
			info:   workload.NewInfo(cluster, GVK, &obj),
			client: c,
			obj:    obj.DeepCopy(),
		})
	}
	return workloads, nil
}

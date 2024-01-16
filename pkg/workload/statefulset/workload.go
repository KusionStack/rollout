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

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
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

func (w *realWorkload) UpgradePartition(partition intstr.IntOrString) (bool, error) {
	if w.obj.Spec.UpdateStrategy.Type != appsv1.RollingUpdateStatefulSetStrategyType {
		return false, fmt.Errorf("rollout can not upgrade partition in StatefulSet if the upgrade strategy type is not RollingUpdate")
	}

	expectedReplicas, err := workload.CalculatePartitionReplicas(w.obj.Spec.Replicas, partition)
	if err != nil {
		return false, err
	}

	// get current partition number
	currentPartition := int32(0)
	if w.obj.Spec.UpdateStrategy.RollingUpdate != nil {
		currentPartition = ptr.Deref[int32](w.obj.Spec.UpdateStrategy.RollingUpdate.Partition, 0)
	}

	// get total replicas number
	totalReplicas := ptr.Deref[int32](w.obj.Spec.Replicas, 0)
	// if totalReplicas == 100, expectReplicas == 10, then expectedPartition is 90
	expectedPartition := totalReplicas - expectedReplicas

	if currentPartition <= expectedPartition {
		// already update
		return false, nil
	}

	// we need to update partiton here
	err = w.UpdateOnConflict(context.TODO(), func(obj client.Object) error {
		sts, ok := obj.(*appsv1.StatefulSet)
		if !ok {
			return fmt.Errorf("expect client.Object to be *appsv1.StatefulSet")
		}

		// TODO: add batch info in annotation
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

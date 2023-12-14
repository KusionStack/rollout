package collaset

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	operatingv1alpha1 "kusionstack.io/kube-api/apps/v1alpha1"
	"kusionstack.io/kube-utils/multicluster/clusterinfo"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	rolloutv1alpha1 "kusionstack.io/rollout/apis/rollout/v1alpha1"
	"kusionstack.io/rollout/pkg/utils"
	"kusionstack.io/rollout/pkg/workload"
)

// GVK is the GroupVersionKind of the CollaSet
var GVK = schema.GroupVersionKind{
	Group:   operatingv1alpha1.GroupVersion.Group,
	Version: operatingv1alpha1.GroupVersion.Version,
	Kind:    "CollaSet",
}

type realWorkload struct {
	info workload.Info
	obj  *operatingv1alpha1.CollaSet

	client client.Client
}

// GetInfo returns basic workload informations.
func (w *realWorkload) GetInfo() workload.Info {
	return w.info
}

func (w *realWorkload) GetStatus() rolloutv1alpha1.RolloutWorkloadStatus {
	status := rolloutv1alpha1.RolloutWorkloadStatus{
		Name:               w.info.Name,
		Cluster:            w.info.Cluster,
		StableRevision:     w.obj.Status.CurrentRevision,
		Generation:         w.obj.Generation,
		ObservedGeneration: w.obj.Status.ObservedGeneration,
		RolloutReplicasSummary: rolloutv1alpha1.RolloutReplicasSummary{
			Replicas:                 *w.obj.Spec.Replicas,
			UpdatedReplicas:          w.obj.Status.UpdatedReplicas,
			UpdatedReadyReplicas:     w.obj.Status.UpdatedReadyReplicas,
			UpdatedAvailableReplicas: w.obj.Status.UpdatedAvailableReplicas,
		},
	}

	if len(w.obj.Status.UpdatedRevision) > 0 {
		status.UpdatedRevision = w.obj.Status.UpdatedRevision
	}

	return status
}

// GetObj returns the underlying object implementing the workload interface
func (w *realWorkload) GetObj() client.Object {
	return w.obj
}

// IsWaitingRollout
func (w *realWorkload) IsWaitingRollout() bool {
	cd := w.obj
	if cd.Status.UpdatedReplicas >= *cd.Spec.Replicas ||
		cd.Status.UpdatedAvailableReplicas >= *cd.Spec.Replicas {
		return false
	}
	if cd.Status.UpdatedRevision == cd.Status.CurrentRevision {
		return false
	}
	return true
}

// CalculateAtLeastUpdatedAvailableReplicas calculates the number of replicas that should be available after the upgrade
func (w *realWorkload) CalculateAtLeastUpdatedAvailableReplicas(failureThreshold *intstr.IntOrString) (int, error) {
	if *w.obj.Spec.Replicas == 0 {
		return 0, nil
	}
	failureReplicas, err := intstr.GetScaledValueFromIntOrPercent(failureThreshold, int(*w.obj.Spec.Replicas), true)
	if err != nil {
		return 0, err
	}

	return int(*w.obj.Spec.Replicas) - failureReplicas, nil
}

// calculatePartitionReplicas calculates the number of replicas that should be upgraded to the specified partition
func (w *realWorkload) calculatePartitionReplicas(partition *intstr.IntOrString) (int, error) {
	return intstr.GetScaledValueFromIntOrPercent(partition, int(*w.obj.Spec.Replicas), true)
}

// UpgradePartition upgrades the workload to the specified partition
func (w *realWorkload) UpgradePartition(partition *intstr.IntOrString) (bool, error) {
	expectReplicas, err := w.calculatePartitionReplicas(partition)
	if err != nil {
		return false, err
	}

	currentPartition := w.obj.Spec.UpdateStrategy.RollingUpdate.ByPartition.Partition
	if int(*currentPartition) <= expectReplicas {
		w.obj.Spec.UpdateStrategy.RollingUpdate.ByPartition.Partition = ptr.To(int32(expectReplicas))
		return true, w.client.Update(clusterinfo.WithCluster(context.Background(), w.info.Cluster), w.obj)
	}

	return true, nil
}

// CheckReady checks if the workload is ready
// TODO: show the reason why the workload is not ready
func (w *realWorkload) CheckReady(expectUpdatedReplicas *int32) (bool, error) {
	if w.obj.GetGeneration() != w.obj.Status.ObservedGeneration {
		return false, nil
	}

	if *w.obj.Spec.Replicas == 0 {
		return false, nil
	}

	var expectUpdatedAvailableReplicas int32
	if expectUpdatedReplicas != nil {
		expectUpdatedAvailableReplicas = *expectUpdatedReplicas
	} else {
		expectUpdatedAvailableReplicas = *w.obj.Spec.Replicas
	}
	if w.obj.Status.UpdatedAvailableReplicas >= expectUpdatedAvailableReplicas {
		return true, nil
	}

	return false, nil
}

func (w *realWorkload) UpdateOnConflict(ctx context.Context, modifyFunc func(client.Object) error) error {
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

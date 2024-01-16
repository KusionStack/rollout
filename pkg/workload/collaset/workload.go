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

// UpgradePartition upgrades the workload to the specified partition
func (w *realWorkload) UpgradePartition(partition intstr.IntOrString) (bool, error) {
	expectReplicas, err := workload.CalculatePartitionReplicas(w.obj.Spec.Replicas, partition)
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

package collaset

import (
	"context"
	"fmt"

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

type workloadImpl struct {
	info   workload.Info
	obj    *operatingv1alpha1.CollaSet
	client client.Client
}

// GetInfo returns basic workload informations.
func (w *workloadImpl) GetInfo() workload.Info {
	return w.info
}

// IsWaitingRollout
func (w *workloadImpl) IsWaitingRollout() bool {
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
func (w *workloadImpl) UpgradePartition(partition intstr.IntOrString, metadataPatch rolloutv1alpha1.MetadataPatch) (bool, error) {
	expectedPartition, err := workload.CalculatePartitionReplicas(w.obj.Spec.Replicas, partition)
	if err != nil {
		return false, err
	}

	currentPartition := int32(0)
	if w.obj.Spec.UpdateStrategy.RollingUpdate != nil && w.obj.Spec.UpdateStrategy.RollingUpdate.ByPartition != nil {
		currentPartition = ptr.Deref[int32](w.obj.Spec.UpdateStrategy.RollingUpdate.ByPartition.Partition, 0)
	}

	if currentPartition >= expectedPartition {
		return false, nil
	}

	// update
	err = w.UpdateOnConflict(context.TODO(), func(o client.Object) error {
		collaset, ok := o.(*operatingv1alpha1.CollaSet)
		if !ok {
			return fmt.Errorf("expect client.Object to be *operatingv1alpha1.CollaSet")
		}
		workload.PatchMetadata(&collaset.ObjectMeta, metadataPatch)
		collaset.Spec.UpdateStrategy.RollingUpdate = &operatingv1alpha1.RollingUpdateCollaSetStrategy{
			ByPartition: &operatingv1alpha1.ByPartition{
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

func (w *workloadImpl) UpdateOnConflict(ctx context.Context, modifyFunc func(client.Object) error) error {
	obj := w.obj
	result, err := utils.UpdateOnConflict(clusterinfo.WithCluster(ctx, w.info.ClusterName), w.client, w.client, obj, func() error {
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

func (w *workloadImpl) EnsureCanaryWorkload(canaryReplicas intstr.IntOrString, canaryMetadataPatch, podMetadataPatch *rolloutv1alpha1.MetadataPatch) (workload.Interface, error) {
	panic("unimplemented")
}

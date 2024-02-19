package collaset

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime/schema"
	operatingv1alpha1 "kusionstack.io/kube-api/apps/v1alpha1"
	"kusionstack.io/kube-utils/multicluster/clusterinfo"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

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

func newFrom(cluster string, client client.Client, obj *operatingv1alpha1.CollaSet) *workloadImpl {
	return &workloadImpl{
		info:   workload.NewInfoFrom(cluster, GVK, obj, getStatus(obj)),
		client: client,
		obj:    obj.DeepCopy(),
	}
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

func (w *workloadImpl) SetObject(obj client.Object) {
	cs, ok := obj.(*operatingv1alpha1.CollaSet)
	if !ok {
		// ignore
		return
	}
	newInfo := workload.NewInfoFrom(w.info.ClusterName, GVK, cs, getStatus(cs))
	w.info = newInfo
	w.obj = cs
}

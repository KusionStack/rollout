package collaset

import (
	"fmt"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"
	operatingv1alpha1 "kusionstack.io/kube-api/apps/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"kusionstack.io/rollout/pkg/workload"
)

// GVK is the GroupVersionKind of the CollaSet
var GVK = schema.GroupVersionKind{
	Group:   operatingv1alpha1.GroupVersion.Group,
	Version: operatingv1alpha1.GroupVersion.Version,
	Kind:    "CollaSet",
}

type accessorImpl struct{}

func New() workload.Accessor {
	return &accessorImpl{}
}

func (w *accessorImpl) GroupVersionKind() schema.GroupVersionKind {
	return GVK
}

func (w *accessorImpl) Watchable() bool {
	return true
}

func (w *accessorImpl) NewObject() client.Object {
	return &operatingv1alpha1.CollaSet{}
}

func (w *accessorImpl) NewObjectList() client.ObjectList {
	return &operatingv1alpha1.CollaSetList{}
}

func (w *accessorImpl) GetInfo(cluster string, obj client.Object) (*workload.Info, error) {
	_, ok := obj.(*operatingv1alpha1.CollaSet)
	if !ok {
		return nil, fmt.Errorf("obj must be collaset")
	}
	return workload.NewInfo(cluster, GVK, obj, w.getStatus(obj)), nil
}

func (w *accessorImpl) getStatus(obj client.Object) workload.InfoStatus {
	cs := obj.(*operatingv1alpha1.CollaSet)
	return workload.InfoStatus{
		StableRevision:           cs.Status.CurrentRevision,
		UpdatedRevision:          cs.Status.UpdatedRevision,
		ObservedGeneration:       cs.Status.ObservedGeneration,
		Replicas:                 ptr.Deref(cs.Spec.Replicas, 0),
		UpdatedReplicas:          cs.Status.UpdatedReplicas,
		UpdatedReadyReplicas:     cs.Status.UpdatedReadyReplicas,
		UpdatedAvailableReplicas: cs.Status.UpdatedAvailableReplicas,
	}
}

func (s *accessorImpl) ReleaseControl() workload.ReleaseControl {
	return &releaseControl{}
}

package collaset

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	operatingv1alpha1 "kusionstack.io/kube-api/apps/v1alpha1"
	"kusionstack.io/kube-utils/multicluster/clusterinfo"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	rolloutv1alpha1 "kusionstack.io/rollout/apis/rollout/v1alpha1"
	"kusionstack.io/rollout/pkg/workload"
	"kusionstack.io/rollout/pkg/workload/registry"
)

type Storage struct {
	client client.Client
}

func NewStorage(mgr manager.Manager) registry.Store {
	return &Storage{
		client: mgr.GetClient(),
	}
}

func (p *Storage) GroupVersionKind() schema.GroupVersionKind {
	return GVK
}

func (p *Storage) Watchable() bool {
	return true
}

func (p *Storage) NewObject() client.Object {
	return &operatingv1alpha1.CollaSet{}
}

func (p *Storage) NewObjectList() client.ObjectList {
	return &operatingv1alpha1.CollaSetList{}
}

func (p *Storage) Wrap(cluster string, obj client.Object) (workload.Interface, error) {
	sts, ok := obj.(*operatingv1alpha1.CollaSet)
	if !ok {
		return nil, fmt.Errorf("obj must be CollaSet")
	}
	return &realWorkload{
		info:   workload.NewInfo(cluster, GVK, obj),
		client: p.client,
		obj:    sts.DeepCopy(),
	}, nil
}

func (p *Storage) Get(ctx context.Context, cluster, namespace, name string) (workload.Interface, error) {
	var obj operatingv1alpha1.CollaSet
	if err := p.client.Get(clusterinfo.WithCluster(ctx, cluster), types.NamespacedName{Namespace: namespace, Name: name}, &obj); err != nil {
		return nil, err
	}
	return p.Wrap(cluster, &obj)
}

func (p *Storage) List(ctx context.Context, namespace string, match rolloutv1alpha1.ResourceMatch) ([]workload.Interface, error) {
	return registry.GetWorkloadList(ctx, p.client, p, namespace, match)
}

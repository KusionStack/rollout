package fake

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"

	rolloutv1alpha1 "kusionstack.io/rollout/apis/rollout/v1alpha1"
	"kusionstack.io/rollout/pkg/workload"
)

type Storage struct {
	Client client.Client
}

func (p *Storage) NewObject() client.Object {
	panic("implement me")
}

func (p *Storage) NewObjectList() client.ObjectList {
	panic("implement me")
}

func (p *Storage) Watchable() bool {
	panic("implement me")
}

func (p *Storage) Wrap(cluster string, obj client.Object) (workload.Interface, error) {
	//TODO implement me
	panic("implement me")
}

func (p *Storage) Get(ctx context.Context, cluster, namespace, name string) (workload.Interface, error) {
	return New(cluster, namespace, name), nil
}

func (p *Storage) List(ctx context.Context, namespace string, match rolloutv1alpha1.ResourceMatch) ([]workload.Interface, error) {
	return []workload.Interface{}, nil
}

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
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
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

// GroupVersionKind implements registry.Store.
func (*Storage) GroupVersionKind() schema.GroupVersionKind {
	return GVK
}

func (p *Storage) Watchable() bool {
	return true
}

func (p *Storage) NewObject() client.Object {
	return &appsv1.StatefulSet{}
}

func (p *Storage) NewObjectList() client.ObjectList {
	return &appsv1.StatefulSetList{}
}

func (p *Storage) Wrap(cluster string, obj client.Object) (workload.Interface, error) {
	sts, ok := obj.(*appsv1.StatefulSet)
	if !ok {
		return nil, fmt.Errorf("obj must be statefulset")
	}
	return newFrom(cluster, p.client, sts), nil
}

func (p *Storage) Get(ctx context.Context, cluster, namespace, name string) (workload.Interface, error) {
	var obj appsv1.StatefulSet
	if err := p.client.Get(clusterinfo.WithCluster(ctx, cluster), types.NamespacedName{Namespace: namespace, Name: name}, &obj); err != nil {
		return nil, err
	}
	return p.Wrap(cluster, &obj)
}

func (p *Storage) List(ctx context.Context, namespace string, match rolloutv1alpha1.ResourceMatch) ([]workload.Interface, error) {
	return registry.GetWorkloadList(ctx, p.client, p, namespace, match)
}

func getStatus(obj *appsv1.StatefulSet) workload.Status {
	return workload.Status{
		ObservedGeneration:       obj.Status.ObservedGeneration,
		StableRevision:           obj.Status.CurrentRevision,
		UpdatedRevision:          obj.Status.UpdateRevision,
		Replicas:                 ptr.Deref(obj.Spec.Replicas, 0),
		UpdatedReplicas:          obj.Status.UpdatedReplicas,
		UpdatedReadyReplicas:     obj.Status.UpdatedReplicas,
		UpdatedAvailableReplicas: obj.Status.UpdatedReplicas,
	}
}

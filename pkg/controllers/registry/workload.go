/**
 * Copyright 2024 The KusionStack Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package registry

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"
	"kusionstack.io/kube-utils/multicluster/clusterinfo"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"kusionstack.io/rollout/pkg/genericregistry"
	"kusionstack.io/rollout/pkg/workload"
	"kusionstack.io/rollout/pkg/workload/collaset"
	"kusionstack.io/rollout/pkg/workload/poddecoration"
	"kusionstack.io/rollout/pkg/workload/statefulset"
)

const (
	WorkloadRegistryName = "workload-registry"
)

var Workloads = NewWorkloadRegistry()

type WorkloadRegistry interface {
	genericregistry.Registry[schema.GroupVersionKind, workload.Accessor]

	GetControllerOf(ctx context.Context, c client.Client, obj client.Object) (*WorkloadAccessor, error)
	GetOwnersOf(ctx context.Context, c client.Client, obj client.Object) ([]*WorkloadAccessor, error)
}

func NewWorkloadRegistry() WorkloadRegistry {
	return &registryImpl{
		Registry: genericregistry.New[schema.GroupVersionKind, workload.Accessor](),
	}
}

func InitWorkloadRegistry(mgr manager.Manager) (bool, error) {
	Workloads.Register(collaset.GVK, collaset.New())
	Workloads.Register(poddecoration.GVK, poddecoration.New())
	Workloads.Register(statefulset.GVK, statefulset.New())
	return true, nil
}

func IsSupportedWorkload(gvk schema.GroupVersionKind) bool {
	_, err := Workloads.Get(gvk)
	return err == nil
}

type registryImpl struct {
	genericregistry.Registry[schema.GroupVersionKind, workload.Accessor]
}

type WorkloadAccessor struct {
	IsController bool
	Object       client.Object
	Accessor     workload.Accessor
}

func (r *registryImpl) GetControllerOf(ctx context.Context, c client.Client, obj client.Object) (*WorkloadAccessor, error) {
	owner, err := workload.GetControllerOf(obj)
	if err != nil {
		return nil, err
	}
	if owner == nil {
		return nil, nil
	}
	return r.getOwnerWorkload(ctx, c, obj, owner)
}

func (r *registryImpl) GetOwnersOf(ctx context.Context, c client.Client, obj client.Object) ([]*WorkloadAccessor, error) {
	owners, err := workload.GetOwnersOf(obj)
	if err != nil {
		return nil, err
	}
	result := []*WorkloadAccessor{}

	for _, owner := range owners {
		workload, err := r.getOwnerWorkload(ctx, c, obj, owner)
		if err != nil {
			return nil, err
		}
		if workload != nil {
			result = append(result, workload)
		}
	}
	return result, nil
}

func (r *registryImpl) getOwnerWorkload(ctx context.Context, c client.Client, obj client.Object, owner *workload.Owner) (*WorkloadAccessor, error) {
	cluster := workload.GetClusterFromLabel(obj.GetLabels())
	ctx = clusterinfo.WithCluster(ctx, cluster)

	ownerRef := owner.Ref
	ownerGVK := owner.GVK

	var accessor workload.Accessor
	var object client.Object
	scheme := c.Scheme()

	for {
		if !r.isSupportedGVK(ownerGVK) {
			// not supported workload
			break
		}

		// supported, get object of owner
		tempObj, err := scheme.New(ownerGVK)
		if err != nil {
			return nil, err
		}
		ownerObj := tempObj.(client.Object)
		err = c.Get(ctx, client.ObjectKey{Namespace: obj.GetNamespace(), Name: ownerRef.Name}, ownerObj)
		if err != nil {
			return nil, client.IgnoreNotFound(err)
		}

		ac, err := r.Registry.Get(ownerGVK)
		if err == nil {
			// if owner is workload, then we should record it as result
			accessor = ac
			object = ownerObj
		}

		// check if ownerObj has owner too
		parent, err := workload.GetControllerOf(ownerObj)
		if err != nil {
			return nil, err
		}
		if parent == nil {
			// parent has no owner, so it is the root workload
			break
		}

		ownerRef = parent.Ref
		ownerGVK = parent.GVK
		// continue to find root workload
	}

	var result *WorkloadAccessor
	if object != nil && accessor != nil {
		result = &WorkloadAccessor{
			IsController: ptr.Deref(ownerRef.Controller, false),
			Object:       object,
			Accessor:     accessor,
		}
	}
	return result, nil
}

func (r *registryImpl) isSupportedGVK(gvk schema.GroupVersionKind) bool {
	_, err := r.Registry.Get(gvk)
	if err == nil {
		return true
	}
	var found bool
	r.Range(func(_ schema.GroupVersionKind, value workload.Accessor) bool {
		gvks := value.DependentWorkloadGVKs()
		for i := range gvks {
			if gvks[i] == gvk {
				found = true
				return false
			}
		}
		return true
	})
	return found
}

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

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
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

	GetPodOwnerWorkload(ctx context.Context, c client.Client, pod *corev1.Pod) (client.Object, workload.Accessor, error)
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

func (r *registryImpl) GetPodOwnerWorkload(ctx context.Context, c client.Client, pod *corev1.Pod) (client.Object, workload.Accessor, error) {
	cluster := workload.GetClusterFromLabel(pod.Labels)
	ctx = clusterinfo.WithCluster(ctx, cluster)

	// firstly, get owner from pod
	owner, ownerGVK, err := workload.GetOwnerAndGVK(pod)
	if err != nil || owner == nil {
		// no owner or get owner failed, return directly
		return nil, nil, err
	}

	var accessor workload.Accessor
	var result client.Object
	scheme := c.Scheme()

	for {
		if !r.isSupportedGVK(ownerGVK) {
			// not supported workload
			break
		}

		// supported, get object of owner
		tempObj, err := scheme.New(ownerGVK)
		if err != nil {
			return nil, nil, err
		}
		ownerObj := tempObj.(client.Object)
		err = c.Get(ctx, client.ObjectKey{Namespace: pod.Namespace, Name: owner.Name}, ownerObj)
		if err != nil {
			return nil, nil, client.IgnoreNotFound(err)
		}

		ac, err := r.Registry.Get(ownerGVK)
		if err == nil {
			// if owner is workload, then we should record it as result
			accessor = ac
			result = ownerObj
		}

		// check if ownerObj has owner too
		parentOwner, parentOwnerGVK, err := workload.GetOwnerAndGVK(ownerObj)
		if err != nil {
			return nil, nil, err
		}
		if parentOwner == nil {
			// parent has no owner, so it is the root workload
			break
		}

		owner = parentOwner
		ownerGVK = parentOwnerGVK
		// continue to find root workload
	}

	return result, accessor, nil
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

/**
 * Copyright 2024 The KusionStack Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     https://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package workload

import (
	"context"
	"fmt"
	"reflect"
	"sort"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/conversion"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	rolloutv1alpha1 "kusionstack.io/kube-api/rollout/v1alpha1"
	"kusionstack.io/kube-utils/multicluster/clusterinfo"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"kusionstack.io/rollout/pkg/utils"
)

// workload info
type Info struct {
	metav1.ObjectMeta
	// GVK is the GroupVersionKind of the workload.
	schema.GroupVersionKind
	// Status is the status of the workload.
	Status InfoStatus
	// Object is the object representing the workload.
	Object client.Object
}

// workload status
type InfoStatus struct {
	// ObservedGeneration is the most recent generation observed for this workload.
	ObservedGeneration int64
	// StableRevision is the old stable revision used to generate pods.
	StableRevision string
	// UpdatedRevision is the updated template revision used to generate pods.
	UpdatedRevision string
	// Replicas is the desired number of pods targeted by workload
	Replicas int32
	// UpdatedReplicas is the number of pods targeted by workload that have the updated template spec.
	UpdatedReplicas int32
	// UpdatedReadyReplicas is the number of ready pods targeted by workload that have the updated template spec.
	UpdatedReadyReplicas int32
	// UpdatedAvailableReplicas is the number of service available pods targeted by workload that have the updated template spec.
	UpdatedAvailableReplicas int32
}

func NewInfo(cluster string, gvk schema.GroupVersionKind, obj client.Object, status InfoStatus) *Info {
	return &Info{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   obj.GetNamespace(),
			Name:        obj.GetName(),
			Labels:      obj.GetLabels(),
			ClusterName: cluster,
			Generation:  obj.GetGeneration(),
		},
		GroupVersionKind: gvk,
		Status:           status,
		Object:           obj,
	}
}

func (o *Info) NamespacedName() types.NamespacedName {
	return types.NamespacedName{
		Namespace: o.Namespace,
		Name:      o.Name,
	}
}

func (o *Info) String() string {
	return rolloutv1alpha1.CrossClusterObjectNameReference{Cluster: o.ClusterName, Name: o.Name}.String()
}

func (o *Info) CheckUpdatedReady(replicas int32) bool {
	if o.Generation != o.Status.ObservedGeneration {
		return false
	}
	return o.Status.UpdatedAvailableReplicas >= replicas
}

func (o *Info) APIStatus() rolloutv1alpha1.RolloutWorkloadStatus {
	return rolloutv1alpha1.RolloutWorkloadStatus{
		RolloutReplicasSummary: rolloutv1alpha1.RolloutReplicasSummary{
			Replicas:                 o.Status.Replicas,
			UpdatedReplicas:          o.Status.UpdatedReplicas,
			UpdatedReadyReplicas:     o.Status.UpdatedReadyReplicas,
			UpdatedAvailableReplicas: o.Status.UpdatedAvailableReplicas,
		},
		Generation:         o.Generation,
		ObservedGeneration: o.Status.ObservedGeneration,
		StableRevision:     o.Status.StableRevision,
		UpdatedRevision:    o.Status.UpdatedRevision,
		Cluster:            o.ClusterName,
		Name:               o.Name,
	}
}

func (o *Info) UpdateOnConflict(ctx context.Context, c client.Client, mutateFn func(client.Object) error) (bool, error) {
	ctx = clusterinfo.WithCluster(ctx, o.ClusterName)
	obj := o.Object
	updated, err := utils.UpdateOnConflict(ctx, c, c, obj, func() error {
		return mutateFn(obj)
	})
	if err != nil {
		return false, err
	}
	if updated {
		o.Object = obj
	}
	return updated, nil
}

func (o *Info) UpdateForRevisionOnConflict(ctx context.Context, c client.Client, mutateFn func(client.Object, *appsv1.ControllerRevision) error) (bool, error) {
	ctx = clusterinfo.WithCluster(ctx, o.ClusterName)
	obj := o.Object
	lastRevision, err := o.GetPreviousRevision(ctx, c)
	if err != nil {
		return false, err
	}
	updated, err := utils.UpdateOnConflict(ctx, c, c, obj, func() error {
		return mutateFn(obj, lastRevision)
	})
	if err != nil {
		return false, err
	}
	if updated {
		o.Object = obj
	}
	return updated, nil
}

func (o *Info) GetPreviousRevision(ctx context.Context, c client.Client) (*appsv1.ControllerRevision, error) {
	crs := &appsv1.ControllerRevisionList{}
	err := c.List(clusterinfo.WithCluster(ctx, o.GetClusterName()), crs, &client.ListOptions{
		Namespace: o.GetNamespace(),
	})
	if err != nil {
		return nil, err
	}

	revisions := make([]*appsv1.ControllerRevision, 0)
	for _, revision := range crs.Items {
		for _, owner := range revision.OwnerReferences {
			if owner.Kind == o.Kind && owner.Name == o.Name {
				revisions = append(revisions, &revision)
				break
			}
		}
	}

	if len(revisions) < 2 {
		return nil, fmt.Errorf("no previous available controllerrevision found for workload %s/%s", o.Kind, o.Name)
	}

	sort.Slice(revisions, func(i, j int) bool {
		return revisions[i].Revision > revisions[j].Revision
	})

	return revisions[len(revisions)-2], nil
}

func IsWaitingRollout(info Info) bool {
	if len(info.Status.StableRevision) != 0 &&
		info.Status.StableRevision != info.Status.UpdatedRevision &&
		info.Status.UpdatedReplicas == 0 {
		return true
	}
	return false
}

func Get(ctx context.Context, c client.Client, inter Accessor, cluster, namespace, name string) (*Info, error) {
	obj := inter.NewObject()
	ctx = clusterinfo.WithCluster(ctx, cluster)
	if err := c.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, obj); err != nil {
		return nil, err
	}

	return inter.GetInfo(cluster, obj)
}

// List return a list of workloads that match the given namespace and match.
func List(ctx context.Context, c client.Client, inter Accessor, namespace string, match rolloutv1alpha1.ResourceMatch) (workloads, canaryWorkloads []*Info, err error) {
	listObj := inter.NewObjectList()
	if err := c.List(clusterinfo.WithCluster(ctx, clusterinfo.Clusters), listObj, &client.ListOptions{Namespace: namespace}); err != nil {
		return nil, nil, err
	}

	matcher := MatchAsMatcher(match)

	listPtr, err := meta.GetItemsPtr(listObj)
	if err != nil {
		return nil, nil, err
	}

	v, err := conversion.EnforcePtr(listPtr)
	if err != nil || v.Kind() != reflect.Slice {
		return nil, nil, fmt.Errorf("neet ptr to slice: %w", err)
	}

	length := v.Len()
	workloads = make([]*Info, 0)
	canaryObjects := map[string]client.Object{}

	for i := range length {
		elemPtr := v.Index(i).Addr().Interface()
		obj, ok := elemPtr.(client.Object)
		if !ok {
			return nil, nil, fmt.Errorf("can not convert element to client.Object")
		}

		if obj.GetDeletionTimestamp() != nil {
			// ignore deleting workload
			continue
		}

		if IsCanary(obj) {
			// ignore canary workload here
			canaryObjects[obj.GetName()] = obj
			continue
		}
		cluster := GetClusterFromLabel(obj.GetLabels())
		if !matcher.Matches(cluster, obj.GetName(), obj.GetLabels()) {
			continue
		}
		info, err := inter.GetInfo(cluster, obj)
		if err != nil {
			return nil, nil, err
		}
		workloads = append(workloads, info)
	}

	_, canCanary := inter.(CanaryReleaseControl)
	if canCanary {
		// find canary workload
		for _, w := range workloads {
			name := GetCanaryName(w.Name)
			canaryObject, ok := canaryObjects[name]
			if !ok {
				continue
			}
			cluster := GetClusterFromLabel(canaryObject.GetLabels())
			info, err := inter.GetInfo(cluster, canaryObject)
			if err != nil {
				return nil, nil, err
			}
			canaryWorkloads = append(canaryWorkloads, info)
		}
	}
	return workloads, canaryWorkloads, nil
}

func GetCanaryName(workloadName string) string {
	return workloadName + "-canary"
}

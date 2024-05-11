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

package podcanarylabel

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"kusionstack.io/kube-utils/controller/mixin"
	"kusionstack.io/kube-utils/multicluster"
	"kusionstack.io/kube-utils/multicluster/clusterinfo"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	rolloutapi "kusionstack.io/rollout/apis/rollout"
	rolloutcontroller "kusionstack.io/rollout/pkg/controllers/rollout"
	"kusionstack.io/rollout/pkg/utils"
	"kusionstack.io/rollout/pkg/workload"
)

const (
	ControllerName = "podcanarylabel"
)

type PodCanaryReconciler struct {
	*mixin.ReconcilerMixin

	workloadRegistry workload.Registry
}

func NewPodCanaryReconciler(mgr manager.Manager, workloadRegistry workload.Registry) *PodCanaryReconciler {
	return &PodCanaryReconciler{
		ReconcilerMixin:  mixin.NewReconcilerMixin(ControllerName, mgr),
		workloadRegistry: workloadRegistry,
	}
}

func (r *PodCanaryReconciler) SetupWithManager(mgr manager.Manager) error {
	b := builder.ControllerManagedBy(mgr).
		For(&corev1.Pod{}).
		Watches(
			multicluster.ClustersKind(&source.Kind{Type: &corev1.Pod{}}),
			&handler.EnqueueRequestForObject{},
		)

	allworkloads := rolloutcontroller.GetWatchableWorkloads(r.workloadRegistry, r.Logger, r.Client, r.Config)
	for _, accessor := range allworkloads {
		gvk := accessor.GroupVersionKind()
		r.Logger.Info("add watcher for workload", "gvk", gvk.String())
		b.Watches(
			multicluster.ClustersKind(&source.Kind{Type: accessor.NewObject()}),
			enqueueWorkloadPods(accessor, r.Client, r.Scheme, r.Logger),
			builder.WithPredicates(workloadLabelOrStatusChangedPredict{accessor: accessor}),
		)
	}

	return b.Complete(r)
}

func (r *PodCanaryReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	logger := r.Logger.WithValues("pod", req.String())
	logger.V(4).Info("started reconciling pod")
	defer logger.V(4).Info("finished reconciling pod")

	pod := &corev1.Pod{}
	err := r.Client.Get(ctx, req.NamespacedName, pod)
	if err != nil {
		return reconcile.Result{}, client.IgnoreNotFound(err)
	}

	if pod.DeletionTimestamp != nil {
		// ignore deleted pod
		return reconcile.Result{}, nil
	}

	// get workload of pod
	accessor, workloadObj, err := r.getSupportedWorkloadFromPod(ctx, pod)
	if err != nil {
		return reconcile.Result{}, err
	}
	if workloadObj == nil {
		return reconcile.Result{}, nil
	}

	if !workload.IsControlledByRollout(workloadObj) {
		// this workload is not controlled by rollout, we need to make sure pod revision label is not added
		updated, err := utils.UpdateOnConflict(ctx, r.Client, r.Client, pod, func() error {
			utils.MutateLabels(pod, func(labels map[string]string) {
				delete(labels, rolloutapi.LabelPodRevision)
			})
			return nil
		})
		if updated {
			logger.V(2).Info("delete pod revision label")
		}
		return reconcile.Result{}, err
	}

	podRevision := recognizePodRevision(accessor.PodControl(r.Client), workloadObj, pod)

	// patch pod label
	updated, err := utils.UpdateOnConflict(ctx, r.Client, r.Client, pod, func() error {
		utils.MutateLabels(pod, func(labels map[string]string) {
			labels[rolloutapi.LabelPodRevision] = podRevision
		})
		return nil
	})

	if updated {
		logger.V(2).Info("updated pod revision label value", "revision", podRevision)
	}

	return reconcile.Result{}, err
}

func (r *PodCanaryReconciler) getSupportedWorkloadFromPod(ctx context.Context, pod *corev1.Pod) (workload.Accessor, client.Object, error) {
	cluster := workload.GetClusterFromLabel(pod.Labels)
	ctx = clusterinfo.WithCluster(ctx, cluster)

	// firstly, get owner from pod
	owner, ownerGVK, err := getOwnerAndGVK(pod)
	if err != nil || owner == nil {
		// no owner or get owner failed, return directly
		return nil, nil, err
	}

	var accessor workload.Accessor
	var result client.Object

	for {
		if !r.isSupportedGVK(ownerGVK) {
			// not supported workload
			break
		}

		// supported, get object of owner
		tempObj, err := r.Scheme.New(ownerGVK)
		if err != nil {
			return nil, nil, err
		}
		ownerObj := tempObj.(client.Object)
		err = r.Client.Get(ctx, client.ObjectKey{Namespace: pod.Namespace, Name: owner.Name}, ownerObj)
		if err != nil {
			return nil, nil, client.IgnoreNotFound(err)
		}

		ac, err := r.workloadRegistry.Get(ownerGVK)
		if err == nil {
			// if owner is workload, then we should record it as result
			accessor = ac
			result = ownerObj
		}

		// check if ownerObj has owner too
		parentOwner, parentOwnerGVK, err := getOwnerAndGVK(ownerObj)
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

	return accessor, result, nil
}

func (r *PodCanaryReconciler) isSupportedGVK(gvk schema.GroupVersionKind) bool {
	_, err := r.workloadRegistry.Get(gvk)
	if err == nil {
		return true
	}

	var found bool
	r.workloadRegistry.Range(func(_ schema.GroupVersionKind, value workload.Accessor) bool {
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

func getOwnerAndGVK(obj client.Object) (*metav1.OwnerReference, schema.GroupVersionKind, error) {
	owner := metav1.GetControllerOf(obj)
	if owner == nil {
		// not found
		return nil, schema.GroupVersionKind{}, nil
	}

	gv, err := schema.ParseGroupVersion(owner.APIVersion)
	if err != nil {
		return nil, schema.GroupVersionKind{}, err
	}
	gvk := gv.WithKind(owner.Kind)
	return owner, gvk, nil
}

func recognizePodRevision(pc workload.PodControl, workloadObj client.Object, pod *corev1.Pod) string {
	if workload.IsCanary(workloadObj) {
		// canary workload, always set pod revision to canary
		return rolloutapi.LabelValuePodRevisionCanary
	}

	if !workload.IsProgressing(workloadObj) {
		// workload is not progressing, set pod revision to base
		return rolloutapi.LabelValuePodRevisionBase
	}

	// workload is progressing, set updated pod revision to canary
	if updated, _ := pc.IsUpdatedPod(workloadObj, pod); updated {
		return rolloutapi.LabelValuePodRevisionCanary
	}
	return rolloutapi.LabelValuePodRevisionBase
}

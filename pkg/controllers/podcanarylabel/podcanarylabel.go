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
	"kusionstack.io/kube-utils/controller/mixin"
	"kusionstack.io/kube-utils/multicluster"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	rolloutapi "kusionstack.io/rollout/apis/rollout"
	"kusionstack.io/rollout/pkg/controllers/registry"
	rolloutcontroller "kusionstack.io/rollout/pkg/controllers/rollout"
	"kusionstack.io/rollout/pkg/utils"
	"kusionstack.io/rollout/pkg/workload"
)

const (
	ControllerName = "podcanarylabel"
)

type PodCanaryReconciler struct {
	*mixin.ReconcilerMixin

	workloadRegistry registry.WorkloadRegistry
}

func NewPodCanaryReconciler(mgr manager.Manager, workloadRegistry registry.WorkloadRegistry) *PodCanaryReconciler {
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
	workloadObj, accessor, err := r.workloadRegistry.GetPodOwnerWorkload(ctx, r.Client, pod)
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

	pc, ok := accessor.(workload.PodControl)
	if !ok {
		logger.V(2).Info("accessor does not support pod control, skip reconciling pod")
		return reconcile.Result{}, nil
	}

	podRevision := recognizePodRevision(pc, workloadObj, pod)

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

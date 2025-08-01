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
	"time"

	corev1 "k8s.io/api/core/v1"
	rolloutapi "kusionstack.io/kube-api/rollout"
	"kusionstack.io/kube-utils/controller/mixin"
	"kusionstack.io/kube-utils/multicluster"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

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
		pc, ok := accessor.(workload.ReplicaObjectControl)
		if !ok {
			continue
		}
		if pc.ReplicaType().Kind != "Pod" {
			continue
		}
		gvk := accessor.GroupVersionKind()
		r.Logger.Info("add watcher for workload", "gvk", gvk.String())
		b.Watches(
			multicluster.ClustersKind(&source.Kind{Type: accessor.NewObject()}),
			enqueueWorkloadPods(gvk, pc, r.Client, r.Scheme, r.Logger),
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
	workloadObj, err := r.workloadRegistry.GetControllerOf(ctx, r.Client, pod)
	if err != nil {
		return reconcile.Result{}, err
	}
	if workloadObj == nil {
		return reconcile.Result{}, nil
	}

	if !workload.IsControlledByRollout(workloadObj.Object) {
		// this workload is not controlled by rollout, we need to make sure pod revision label is not added
		updated, err := utils.UpdateOnConflict(ctx, r.Client, r.Client, pod, func() error {
			utils.MutateLabels(pod, func(labels map[string]string) {
				delete(labels, rolloutapi.TrafficLaneLabelKey)
			})
			return nil
		})
		if updated {
			logger.V(2).Info("delete pod revision label")
		}
		return reconcile.Result{}, err
	}

	pc, ok := workloadObj.Accessor.(workload.ReplicaObjectControl)
	if !ok {
		logger.V(2).Info("accessor does not support pod control, skip reconciling pod")
		return reconcile.Result{}, nil
	}

	trafficLane := workload.RecognizeTrafficLane(ctx, workloadObj.Accessor, pc, r.Client, workloadObj.Object, pod)

	// patch pod label
	updated, err := utils.UpdateOnConflict(ctx, r.Client, r.Client, pod, func() error {
		utils.MutateLabels(pod, func(labels map[string]string) {
			labels[rolloutapi.TrafficLaneLabelKey] = trafficLane
		})
		return nil
	})
	if err != nil {
		return reconcile.Result{}, err
	}

	if updated {
		logger.V(2).Info("updated pod traffic lane label value", "traffic-lane", trafficLane)
	}

	if trafficLane == rolloutapi.UnknownTrafficLane {
		// unknown traffic lane, requeue after 5 seconds
		return reconcile.Result{RequeueAfter: 5 * time.Second}, nil
	}

	return reconcile.Result{}, err
}

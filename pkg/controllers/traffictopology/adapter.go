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

package traffictopology

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"k8s.io/utils/ptr"
	rolloutapi "kusionstack.io/kube-api/rollout"
	rolloutv1alpha1 "kusionstack.io/kube-api/rollout/v1alpha1"
	"kusionstack.io/kube-utils/multicluster/clusterinfo"
	rsFrameController "kusionstack.io/resourceconsist/pkg/frame/controller"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/ratelimiter"

	"kusionstack.io/rollout/pkg/controllers/registry"
	"kusionstack.io/rollout/pkg/features"
	"kusionstack.io/rollout/pkg/features/rolloutclasspredicate"
	"kusionstack.io/rollout/pkg/utils"
	"kusionstack.io/rollout/pkg/workload"
)

type TPControllerAdapter struct {
	client.Client
	workloadRegistry        registry.WorkloadRegistry
	maxConcurrentReconciles int
}

func NewTPControllerAdapter(mgr manager.Manager, workloadRegistry registry.WorkloadRegistry) *TPControllerAdapter {
	opts := mgr.GetControllerOptions()
	groupKind := rolloutv1alpha1.SchemeGroupVersion.WithKind("TrafficTopology").GroupKind().String()
	c := &TPControllerAdapter{
		Client:           mgr.GetClient(),
		workloadRegistry: workloadRegistry,
	}
	if concurrency, ok := opts.GroupKindConcurrency[groupKind]; ok && concurrency > 0 {
		c.maxConcurrentReconciles = concurrency
	}
	return c
}

var (
	_ rsFrameController.ReconcileAdapter      = &TPControllerAdapter{}
	_ rsFrameController.ReconcileWatchOptions = &TPControllerAdapter{}
	_ rsFrameController.StatusRecordOptions   = &TPControllerAdapter{}
	_ rsFrameController.MultiClusterOptions   = &TPControllerAdapter{}
	_ rsFrameController.ReconcileOptions      = &TPControllerAdapter{}
)

// GetMaxConcurrent implements controller.ReconcileOptions.
func (t *TPControllerAdapter) GetMaxConcurrent() int {
	return t.maxConcurrentReconciles
}

// GetRateLimiter implements controller.ReconcileOptions.
func (t *TPControllerAdapter) GetRateLimiter() ratelimiter.RateLimiter {
	return nil
}

func (t *TPControllerAdapter) GetControllerName() string {
	return ControllerName
}

func (t *TPControllerAdapter) GetSelectedEmployeeNames(ctx context.Context, employer client.Object) ([]string, error) {
	return nil, nil
}

func (t *TPControllerAdapter) GetExpectedEmployer(ctx context.Context, employer client.Object) ([]rsFrameController.IEmployer, error) {
	var expected []rsFrameController.IEmployer

	trafficTopology, ok := employer.(*rolloutv1alpha1.TrafficTopology)
	if !ok {
		return expected, fmt.Errorf("not type of TrafficTopology")
	}

	if trafficTopology.GetDeletionTimestamp() != nil {
		return expected, nil
	}

	inter, err := t.workloadRegistry.Get(schema.FromAPIVersionAndKind(
		trafficTopology.Spec.WorkloadRef.APIVersion, trafficTopology.Spec.WorkloadRef.Kind))
	if err != nil {
		return expected, err
	}

	workloads, _, err := workload.List(ctx, t.Client, inter, trafficTopology.Namespace, trafficTopology.Spec.WorkloadRef.Match)
	if err != nil {
		return expected, err
	}

	if len(workloads) == 0 {
		return expected, nil
	}

	backendApiVersion := ptr.Deref(trafficTopology.Spec.Backend.APIVersion, "v1")
	backendKind := ptr.Deref(trafficTopology.Spec.Backend.Kind, "Service")

	// caution:
	// for InClusterTrafficType, clusterName of brBackend will be changed
	brBackend := rolloutv1alpha1.CrossClusterObjectReference{
		ObjectTypeRef: rolloutv1alpha1.ObjectTypeRef{
			APIVersion: backendApiVersion,
			Kind:       backendKind,
		},
		CrossClusterObjectNameReference: rolloutv1alpha1.CrossClusterObjectNameReference{
			Name: trafficTopology.Spec.Backend.Name,
		},
	}

	brRoutes := make([]rolloutv1alpha1.CrossClusterObjectReference, len(trafficTopology.Spec.Routes))
	for i, route := range trafficTopology.Spec.Routes {
		routeApiVersion := ptr.Deref(route.APIVersion, "gateway.networking.k8s.io/v1")
		routeKind := ptr.Deref(route.Kind, "HTTPRoute")

		brRoutes[i] = rolloutv1alpha1.CrossClusterObjectReference{
			ObjectTypeRef: rolloutv1alpha1.ObjectTypeRef{
				APIVersion: routeApiVersion,
				Kind:       routeKind,
			},
			CrossClusterObjectNameReference: rolloutv1alpha1.CrossClusterObjectNameReference{
				Name: route.Name,
			},
		}
	}

	owenr := metav1.NewControllerRef(trafficTopology, rolloutv1alpha1.SchemeGroupVersion.WithKind("TrafficTopology"))

	labels := map[string]string{}
	if features.DefaultFeatureGate.Enabled(features.RolloutClassPredicate) {
		claas, labeled := trafficTopology.Labels[rolloutapi.LabelRolloutClass]
		if labeled {
			labels[rolloutapi.LabelRolloutClass] = claas
		}
	}

	switch trafficTopology.Spec.TrafficType {
	case rolloutv1alpha1.MultiClusterTrafficType:
		br := rolloutv1alpha1.BackendRouting{
			ObjectMeta: metav1.ObjectMeta{
				Name:            employer.GetName() + "-msc",
				Namespace:       employer.GetNamespace(),
				OwnerReferences: []metav1.OwnerReference{*owenr},
				Labels:          labels,
			},
			Spec: rolloutv1alpha1.BackendRoutingSpec{
				TrafficType: rolloutv1alpha1.MultiClusterTrafficType,
				Backend:     brBackend,
				Routes:      brRoutes,
			},
		}
		backendRoutingEmployer := TPEmployer{
			BackendRoutingName: employer.GetName() + "-msc",
			BackendRouting:     br,
		}

		workloadInfos := make([]rolloutv1alpha1.CrossClusterObjectNameReference, len(workloads))
		for idx, info := range workloads {
			workloadInfos[idx] = rolloutv1alpha1.CrossClusterObjectNameReference{
				Name:    info.Name,
				Cluster: info.ClusterName,
			}
		}

		backendRoutingEmployer.Workloads = workloadInfos
		expected = []rsFrameController.IEmployer{backendRoutingEmployer}

	case rolloutv1alpha1.InClusterTrafficType:
		brNameTPEmployerMap := make(map[string]TPEmployer)
		for _, info := range workloads {
			clusterName := info.ClusterName
			brName := employer.GetName() + "-ics"
			if clusterName != "" {
				brName = brName + "-" + clusterName
			}
			brBackend.Cluster = clusterName
			brRoutesCopy := make([]rolloutv1alpha1.CrossClusterObjectReference, len(brRoutes))
			for i, brRoute := range brRoutes {
				brRoute.Cluster = clusterName
				brRoutesCopy[i] = brRoute
			}
			br := rolloutv1alpha1.BackendRouting{
				ObjectMeta: metav1.ObjectMeta{
					Name:            brName,
					Namespace:       employer.GetNamespace(),
					OwnerReferences: []metav1.OwnerReference{*owenr},
					Labels:          labels,
				},
				Spec: rolloutv1alpha1.BackendRoutingSpec{
					TrafficType: rolloutv1alpha1.InClusterTrafficType,
					Backend:     brBackend,
					Routes:      brRoutesCopy,
				},
			}
			trEmployer, ok := brNameTPEmployerMap[brName]
			if !ok {
				brNameTPEmployerMap[brName] = TPEmployer{
					BackendRoutingName: brName,
					BackendRouting:     br,
					Workloads: []rolloutv1alpha1.CrossClusterObjectNameReference{
						{
							Cluster: clusterName,
							Name:    info.Name,
						},
					},
				}
			} else {
				trEmployer.Workloads = append(trEmployer.Workloads, rolloutv1alpha1.CrossClusterObjectNameReference{
					Cluster: clusterName,
					Name:    info.Name,
				})
				brNameTPEmployerMap[brName] = trEmployer
			}
		}

		for _, tpEmployer := range brNameTPEmployerMap {
			expected = append(expected, tpEmployer)
		}
	}
	return expected, nil
}

// GetCurrentEmployer returns BackendRouting created already, check those in trafficTopology.status
// check expected whether created already? no need, we made create/delete idempotent
func (t *TPControllerAdapter) GetCurrentEmployer(ctx context.Context, employer client.Object) ([]rsFrameController.IEmployer, error) {
	var current []rsFrameController.IEmployer

	trafficTopology, ok := employer.(*rolloutv1alpha1.TrafficTopology)
	if !ok {
		return current, fmt.Errorf("not type of TrafficTopology")
	}

	brNameTREmployerMap := make(map[string]TPEmployer)
	for _, topo := range trafficTopology.Status.Topologies {
		trEmployer, ok := brNameTREmployerMap[topo.BackendRoutingName]
		if !ok {
			br := &rolloutv1alpha1.BackendRouting{}
			err := t.Get(clusterinfo.WithCluster(ctx, clusterinfo.Fed), types.NamespacedName{
				Name:      topo.BackendRoutingName,
				Namespace: employer.GetNamespace(),
			}, br)
			if err != nil {
				if errors.IsNotFound(err) {
					continue
				}
				return current, err
			}
			brNameTREmployerMap[topo.BackendRoutingName] = TPEmployer{
				BackendRoutingName: br.Name,
				BackendRouting:     *br,
				Workloads: []rolloutv1alpha1.CrossClusterObjectNameReference{
					topo.WorkloadRef,
				},
			}
		} else {
			trEmployer.Workloads = append(trEmployer.Workloads, topo.WorkloadRef)
			brNameTREmployerMap[topo.BackendRoutingName] = trEmployer
		}
	}

	for _, trEmployer := range brNameTREmployerMap {
		current = append(current, trEmployer)
	}

	return current, nil
}

func (t *TPControllerAdapter) CreateEmployer(ctx context.Context, employer client.Object, toCreates []rsFrameController.IEmployer) ([]rsFrameController.IEmployer, []rsFrameController.IEmployer, error) {
	succCreated := make([]rsFrameController.IEmployer, 0)
	failCreated := make([]rsFrameController.IEmployer, 0)
	_, err := utils.SlowStartBatch(len(toCreates), 1, false, func(i int, _ error) error {
		toCreate := toCreates[i]
		br, ok := toCreate.(TPEmployer)
		if !ok {
			failCreated = append(failCreated, toCreate)
			return fmt.Errorf("not type of TPEmployer")
		}
		brGet := &rolloutv1alpha1.BackendRouting{}
		err := t.Get(clusterinfo.WithCluster(ctx, clusterinfo.Fed), types.NamespacedName{
			Name:      br.BackendRouting.Name,
			Namespace: br.BackendRouting.Namespace,
		}, brGet)
		if err == nil {
			succCreated = append(succCreated, toCreate)
			return nil
		}
		err = t.Create(clusterinfo.WithCluster(ctx, clusterinfo.Fed), &br.BackendRouting)
		if err != nil {
			failCreated = append(failCreated, toCreate)
			return fmt.Errorf("failed to create BackendRouting, err: %s", err.Error())
		}
		succCreated = append(succCreated, toCreate)
		return nil
	})
	return succCreated, failCreated, err
}

// UpdateEmployer won't do update now, since only BackendRouting's name compared
// TODO what we should do if BackendRouting's spec different
func (t *TPControllerAdapter) UpdateEmployer(ctx context.Context, employer client.Object, toUpdates []rsFrameController.IEmployer) ([]rsFrameController.IEmployer, []rsFrameController.IEmployer, error) {
	if len(toUpdates) > 0 {
		return nil, toUpdates, fmt.Errorf("no BackendRouting need to be updated, but toUpdate exist")
	}
	return nil, nil, nil
}

func (t *TPControllerAdapter) DeleteEmployer(ctx context.Context, employer client.Object, toDeletes []rsFrameController.IEmployer) ([]rsFrameController.IEmployer, []rsFrameController.IEmployer, error) {
	succDeleted := make([]rsFrameController.IEmployer, 0)
	failDeleted := make([]rsFrameController.IEmployer, 0)
	_, err := utils.SlowStartBatch(len(toDeletes), 1, false, func(i int, _ error) error {
		toDelete := toDeletes[i]
		br, ok := toDelete.(TPEmployer)
		if !ok {
			failDeleted = append(failDeleted, toDelete)
			return fmt.Errorf("not type of TPEmployer")
		}
		brGet := &rolloutv1alpha1.BackendRouting{}
		err := t.Get(clusterinfo.WithCluster(ctx, clusterinfo.Fed), types.NamespacedName{
			Name:      br.BackendRouting.Name,
			Namespace: br.BackendRouting.Namespace,
		}, brGet)
		if err != nil && errors.IsNotFound(err) {
			succDeleted = append(succDeleted, toDelete)
			return nil
		}
		err = t.Delete(clusterinfo.WithCluster(ctx, clusterinfo.Fed), &br.BackendRouting)
		if err != nil {
			failDeleted = append(failDeleted, toDelete)
			return fmt.Errorf("failed to create BackendRouting, err: %s", err.Error())
		}
		succDeleted = append(succDeleted, toDelete)
		return nil
	})
	return succDeleted, failDeleted, err
}

func (t *TPControllerAdapter) GetExpectedEmployee(ctx context.Context, employer client.Object) ([]rsFrameController.IEmployee, error) {
	return nil, nil
}

func (t *TPControllerAdapter) GetCurrentEmployee(ctx context.Context, employer client.Object) ([]rsFrameController.IEmployee, error) {
	return nil, nil
}

func (t *TPControllerAdapter) CreateEmployees(ctx context.Context, employer client.Object, toCreates []rsFrameController.IEmployee) ([]rsFrameController.IEmployee, []rsFrameController.IEmployee, error) {
	return nil, nil, nil
}

func (t *TPControllerAdapter) UpdateEmployees(ctx context.Context, employer client.Object, toUpdates []rsFrameController.IEmployee) ([]rsFrameController.IEmployee, []rsFrameController.IEmployee, error) {
	return nil, nil, nil
}

func (t *TPControllerAdapter) DeleteEmployees(ctx context.Context, employer client.Object, toDeletes []rsFrameController.IEmployee) ([]rsFrameController.IEmployee, []rsFrameController.IEmployee, error) {
	return nil, nil, nil
}

func (t *TPControllerAdapter) NewEmployer() client.Object {
	return &rolloutv1alpha1.TrafficTopology{}
}

func (t *TPControllerAdapter) NewEmployee() client.Object {
	return &rolloutv1alpha1.BackendRouting{}
}

func (t *TPControllerAdapter) EmployerEventHandler() handler.EventHandler {
	return &EnqueueTP{}
}

// EmployeeEventHandler return nil since we don't need BackendRouting's event trigger reconciling TrafficTopology
func (t *TPControllerAdapter) EmployeeEventHandler() handler.EventHandler {
	return &EnqueueTPByBR{}
}

func (t *TPControllerAdapter) EmployerPredicates() predicate.Funcs {
	return rolloutclasspredicate.RolloutClassMatchesPredicate
}

func (t *TPControllerAdapter) EmployeePredicates() predicate.Funcs {
	return predicate.Funcs{}
}

func (t *TPControllerAdapter) RecordStatuses(ctx context.Context, employer client.Object, cudEmployerResults rsFrameController.CUDEmployerResults, cudEmployeeResults rsFrameController.CUDEmployeeResults) error {
	trafficTopology, ok := employer.(*rolloutv1alpha1.TrafficTopology)
	if !ok {
		return fmt.Errorf("not type of TrafficTopology")
	}

	observedGeneration := trafficTopology.Generation

	// calculate status and compare status to check if update
	// succCreate + succUpdate + failUpdate + failDelete + unchanged
	var needRecordEmployers []rsFrameController.IEmployer
	needRecordEmployers = append(needRecordEmployers, cudEmployerResults.SuccCreated...)
	needRecordEmployers = append(needRecordEmployers, cudEmployerResults.SuccUpdated...)
	needRecordEmployers = append(needRecordEmployers, cudEmployerResults.FailUpdated...)
	needRecordEmployers = append(needRecordEmployers, cudEmployerResults.FailDeleted...)
	needRecordEmployers = append(needRecordEmployers, cudEmployerResults.Unchanged...)

	var expect []rolloutv1alpha1.TopologyInfo
	for _, needRecordEmployer := range needRecordEmployers {
		trEmployerStatues, ok := needRecordEmployer.GetEmployerStatuses().(TREmployerStatues)
		if !ok {
			return fmt.Errorf("not type of TREmployerStatuses")
		}
		for _, workload := range trEmployerStatues.Workloads {
			expect = append(expect, rolloutv1alpha1.TopologyInfo{
				BackendRoutingName: needRecordEmployer.GetEmployerId(),
				WorkloadRef:        workload,
			})
		}
	}

	needUpdateStatus := false
	var updateStatusFuncs []func(tp *rolloutv1alpha1.TrafficTopology)
	if !utils.SliceTopologyInfoEqual(expect, trafficTopology.Status.Topologies) {
		needUpdateStatus = true
		updateStatusFuncs = append(updateStatusFuncs, func(tp *rolloutv1alpha1.TrafficTopology) {
			tp.Status.Topologies = expect
		})
	}

	conditionReadyStatus := metav1.ConditionFalse
	if len(cudEmployerResults.FailCreated) == 0 && len(cudEmployerResults.FailUpdated) == 0 && len(cudEmployerResults.FailDeleted) == 0 &&
		(len(cudEmployerResults.SuccCreated) != 0 || len(cudEmployerResults.SuccUpdated) != 0 || len(cudEmployerResults.Unchanged) != 0) {
		conditionReadyStatus = metav1.ConditionTrue
	}

	updateConditionFunc := func(tp *rolloutv1alpha1.TrafficTopology) {
		exist := false
		for i, cond := range tp.Status.Conditions {
			if cond.Type != rolloutv1alpha1.TrafficTopologyConditionReady {
				continue
			}
			exist = true
			tp.Status.Conditions[i].Status = conditionReadyStatus
			tp.Status.Conditions[i].LastTransitionTime = metav1.Time{
				Time: time.Now(),
			}
			tp.Status.Conditions[i].LastUpdateTime = metav1.Time{
				Time: time.Now(),
			}
			break
		}
		if !exist {
			tp.Status.Conditions = append(tp.Status.Conditions, rolloutv1alpha1.Condition{
				Type:   rolloutv1alpha1.TrafficTopologyConditionReady,
				Status: conditionReadyStatus,
				LastTransitionTime: metav1.Time{
					Time: time.Now(),
				},
				LastUpdateTime: metav1.Time{
					Time: time.Now(),
				},
			})
		}
	}
	currentConditionReadyExist := false
	for _, condition := range trafficTopology.Status.Conditions {
		if condition.Type == rolloutv1alpha1.TrafficTopologyConditionReady {
			currentConditionReadyExist = true
			if condition.Status != conditionReadyStatus {
				needUpdateStatus = true
				updateStatusFuncs = append(updateStatusFuncs, updateConditionFunc)
			}
			break
		}
	}
	if !currentConditionReadyExist {
		needUpdateStatus = true
		updateStatusFuncs = append(updateStatusFuncs, updateConditionFunc)
	}

	var err error
	if needUpdateStatus {
		err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
			trGet := &rolloutv1alpha1.TrafficTopology{}
			err := t.Get(clusterinfo.WithCluster(ctx, clusterinfo.Fed), types.NamespacedName{
				Name:      trafficTopology.Name,
				Namespace: trafficTopology.Namespace,
			}, trGet)
			if err != nil {
				if errors.IsNotFound(err) {
					return nil
				}
				return err
			}
			for _, fn := range updateStatusFuncs {
				fn(trGet)
			}
			trGet.Status.ObservedGeneration = observedGeneration

			return t.Status().Update(clusterinfo.WithCluster(ctx, clusterinfo.Fed), trGet)
		})
	}

	return err
}

func (t *TPControllerAdapter) EmployeeFed() bool {
	return true
}

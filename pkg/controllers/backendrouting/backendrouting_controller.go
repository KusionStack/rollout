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

package backendrouting

import (
	"context"
	"fmt"
	"reflect"

	"go.uber.org/multierr"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"kusionstack.io/kube-utils/controller/mixin"
	"kusionstack.io/kube-utils/multicluster/clusterinfo"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"kusionstack.io/rollout/apis/rollout/v1alpha1"
	"kusionstack.io/rollout/pkg/backend"
	"kusionstack.io/rollout/pkg/route"
)

const (
	ControllerName = "backendrouting"
)

type BackendRoutingReconciler struct {
	*mixin.ReconcilerMixin
	backendRegistry backend.Registry
	routeRegistry   route.Registry
}

func NewReconciler(mgr manager.Manager, backendRegistry backend.Registry, routeRegistry route.Registry) *BackendRoutingReconciler {
	return &BackendRoutingReconciler{
		ReconcilerMixin: mixin.NewReconcilerMixin(ControllerName, mgr),
		backendRegistry: backendRegistry,
		routeRegistry:   routeRegistry,
	}
}

func (b *BackendRoutingReconciler) SetupWithManager(mgr manager.Manager) error {
	if b.backendRegistry == nil {
		return fmt.Errorf("backendRegistry must be set")
	}
	if b.routeRegistry == nil {
		return fmt.Errorf("routeRegistry must be set")
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.BackendRouting{}, builder.WithPredicates(predicate.ResourceVersionChangedPredicate{})).
		Complete(b)
}

//+kubebuilder:rbac:groups=rollout.kusionstack.io,resources=backendroutings,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=rollout.kusionstack.io,resources=backendroutings/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=rollout.kusionstack.io,resources=backendroutings/finalizers,verbs=update;patch
//+kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="networking.k8s.io",resources=ingresses,verbs=get;list;watch;create;update;patch;delete

func (b *BackendRoutingReconciler) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	br := &v1alpha1.BackendRouting{}
	err := b.Client.Get(clusterinfo.WithCluster(ctx, clusterinfo.Fed), types.NamespacedName{
		Name:      request.Name,
		Namespace: request.Namespace,
	}, br)
	if err != nil {
		if errors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	// todo: finalizers' management

	if br.GetDeletionTimestamp() != nil {
		return b.reconcileTerminatingBackendRouting(ctx, br)
	}

	if br.Spec.TrafficType == v1alpha1.MultiClusterTrafficType {
		return b.reconcileMultiClusterType(ctx, br)
	}

	// InClusterTrafficType
	if br.Spec.Forwarding != nil {
		// todo webhook should reject adding spec.forwarding if backendrouting.status.phase is not ready?
		return b.reconcileInClusterWithForwarding(ctx, br)
	}

	return b.reconcileInClusterWithoutForwarding(ctx, br)
}

func (b *BackendRoutingReconciler) reconcileTerminatingBackendRouting(_ context.Context, _ *v1alpha1.BackendRouting) (reconcile.Result, error) {
	// todo
	return reconcile.Result{}, nil
}

func (b *BackendRoutingReconciler) reconcileInClusterWithoutForwarding(ctx context.Context, br *v1alpha1.BackendRouting) (reconcile.Result, error) {
	backendsStatuses := br.Status.Backends
	// todo: discussion: if canary backend still exist, should we clean it?
	if backendsStatuses.Canary.Name != "" {
		return reconcile.Result{}, fmt.Errorf("canary backend still exist without forwarding spec")
	}

	routesStatuses := br.Status.RouteStatuses
	phase := br.Status.Phase

	needUpdateStatus := false
	if br.Status.ObservedGeneration != br.Generation {
		needUpdateStatus = true
	}

	// clean stable backends and routes
	if backendsStatuses.Stable.Name != "" {
		if !ptr.Deref(backendsStatuses.Stable.Conditions.Terminating, false) {
			// not deleting, do backend delete, change route's backend first
			var routeBackendChangeErr []error
			for i, currentRoute := range routesStatuses {
				iRoute, err := b.getRoute(ctx, br.Namespace, currentRoute.CrossClusterObjectReference)
				if err != nil {
					return reconcile.Result{}, b.handleErr(ctx, br, backendsStatuses, routesStatuses, phase, v1alpha1.BackendUpgrading, err)
				}
				err = iRoute.ChangeBackend(ctx, route.BackendChangeDetail{
					Src:        backendsStatuses.Stable.Name,
					Dst:        backendsStatuses.Origin.Name,
					Kind:       br.Spec.Backend.Kind,
					ApiVersion: br.Spec.Backend.APIVersion,
				})
				if err != nil {
					routeBackendChangeErr = append(routeBackendChangeErr, err)
				}
				// todo: discussion
				// only changed by route spec here, we haven't check if it synced,
				// and actually we can't delete stable backend if route not changed from stable -> origin
				currentRoute.Synced = false
				routesStatuses[i] = currentRoute
			}

			if len(routeBackendChangeErr) > 0 {
				return reconcile.Result{}, b.handleErr(ctx, br, backendsStatuses, routesStatuses, phase, v1alpha1.BackendUpgrading, multierr.Combine(routeBackendChangeErr...))
			}

			stableBackend, err := b.getBackend(ctx, br, backendsStatuses.Stable.Name)
			if err != nil {
				if !errors.IsNotFound(err) {
					return reconcile.Result{}, b.handleErr(ctx, br, backendsStatuses, routesStatuses, phase, v1alpha1.BackendUpgrading, err)
				}
			} else {
				if stableBackend.GetBackendObject().GetDeletionTimestamp() == nil {
					err = b.Client.Delete(clusterinfo.WithCluster(ctx, br.Spec.Backend.Cluster), stableBackend.GetBackendObject())
					if err != nil {
						return reconcile.Result{}, b.handleErr(ctx, br, backendsStatuses, routesStatuses, phase, v1alpha1.BackendUpgrading, err)
					}
				}
			}

			terminating := true
			backendsStatuses.Stable.Conditions.Terminating = &terminating

			phase = v1alpha1.RouteUpgrading
			needUpdateStatus = true
		} else {
			// deleting, check deleted
			var routeBackendChangeErr []error
			for _, currentRoute := range routesStatuses {
				iRoute, err := b.getRoute(ctx, br.Namespace, currentRoute.CrossClusterObjectReference)
				if err != nil {
					return reconcile.Result{}, b.handleErr(ctx, br, backendsStatuses, routesStatuses, phase, v1alpha1.BackendUpgrading, err)
				}
				err = iRoute.ChangeBackend(ctx, route.BackendChangeDetail{
					Src:        backendsStatuses.Stable.Name,
					Dst:        backendsStatuses.Origin.Name,
					Kind:       br.Spec.Backend.Kind,
					ApiVersion: br.Spec.Backend.APIVersion,
				})
				if err != nil {
					routeBackendChangeErr = append(routeBackendChangeErr, err)
				}
				// todo: discussion
				// only changed by route spec here, we haven't check if it synced,
				// and actually we can't delete stable backend if route not changed from stable -> origin
			}
			// route not synced yet
			if len(routeBackendChangeErr) > 0 {
				return reconcile.Result{}, b.handleErr(ctx, br, backendsStatuses, routesStatuses, phase, v1alpha1.BackendUpgrading, multierr.Combine(routeBackendChangeErr...))
			}

			_, err := b.getBackend(ctx, br, backendsStatuses.Stable.Name)
			if !errors.IsNotFound(err) {
				return reconcile.Result{}, b.handleErr(ctx, br, backendsStatuses, routesStatuses, phase, v1alpha1.BackendUpgrading, fmt.Errorf("stable backend not deleted yet"))
			}

			// finished
			backendsStatuses.Stable = v1alpha1.BackendStatus{}
			for i, currentRoute := range routesStatuses {
				currentRoute.Synced = true
				routesStatuses[i] = currentRoute
			}
			phase = v1alpha1.Ready
			needUpdateStatus = true
		}
	} else {
		// check backend ready(now just check exist)
		originBackend, err := b.getBackend(ctx, br, br.Spec.Backend.Name)
		if err != nil {
			return reconcile.Result{}, b.handleErr(ctx, br, backendsStatuses, routesStatuses, phase, v1alpha1.BackendUpgrading, err)
		}
		if backendsStatuses.Origin.Name != originBackend.GetBackendObject().GetName() {
			needUpdateStatus = true
			backendsStatuses.Origin.Name = originBackend.GetBackendObject().GetName()
			conditionTrue := true
			backendsStatuses.Origin.Conditions.Ready = &conditionTrue
		}
		// todo maybe we should check route -> origin?
		routesStatusesCur := make([]v1alpha1.BackendRouteStatus, len(br.Spec.Routes))
		for idx, curRoute := range br.Spec.Routes {
			routesStatusesCur[idx] = v1alpha1.BackendRouteStatus{
				CrossClusterObjectReference: v1alpha1.CrossClusterObjectReference{
					ObjectTypeRef: v1alpha1.ObjectTypeRef{
						APIVersion: curRoute.APIVersion,
						Kind:       curRoute.Kind,
					},
					CrossClusterObjectNameReference: v1alpha1.CrossClusterObjectNameReference{
						Cluster: curRoute.Cluster,
						Name:    curRoute.Name,
					},
				},
				Synced: true,
			}
		}
		if !reflect.DeepEqual(routesStatuses, routesStatusesCur) {
			routesStatuses = routesStatusesCur
			needUpdateStatus = true
		}
		if phase != v1alpha1.Ready {
			phase = v1alpha1.Ready
			needUpdateStatus = true
		}
	}

	if needUpdateStatus {
		return reconcile.Result{}, b.updateBackendRoutingStatus(ctx, br, backendsStatuses, routesStatuses, phase)
	}

	return reconcile.Result{}, nil
}

func (b *BackendRoutingReconciler) reconcileInClusterWithForwarding(ctx context.Context, br *v1alpha1.BackendRouting) (reconcile.Result, error) {
	if br.Spec.Forwarding.Canary.Name != "" {
		return reconcile.Result{}, b.ensureCanaryAdd(ctx, br)
	} else {
		// no canary, which means only has stable
		// if status has canary, delete it and update route
		if br.Status.Backends.Canary.Name != "" {
			return reconcile.Result{}, b.ensureCanaryRemove(ctx, br)
		} else {
			needUpdateStatus := false
			if br.Status.ObservedGeneration != br.Generation {
				needUpdateStatus = true
			}
			backendsStatuses := br.Status.Backends
			routesStatuses := br.Status.RouteStatuses
			if len(routesStatuses) == 0 {
				routesStatuses = make([]v1alpha1.BackendRouteStatus, len(br.Spec.Routes))
			}
			phase := br.Status.Phase

			// if status hasn't canary, check stable ready, check route -> stable
			_, err := b.getBackend(ctx, br, br.Spec.Forwarding.Stable.Name)
			if err != nil {
				if errors.IsNotFound(err) {
					// stable not create, do create
					originBackend, err := b.getBackend(ctx, br, br.Spec.Backend.Name)
					if err != nil {
						return reconcile.Result{}, b.handleErr(ctx, br, backendsStatuses, routesStatuses, phase, v1alpha1.BackendUpgrading, err)
					}
					stableForked := originBackend.ForkStable(br.Spec.Forwarding.Stable.Name, ControllerName)
					err = b.Client.Create(clusterinfo.WithCluster(ctx, br.Spec.Backend.Cluster), stableForked)
					if err != nil {
						return reconcile.Result{}, b.handleErr(ctx, br, backendsStatuses, routesStatuses, phase, v1alpha1.BackendUpgrading, err)
					}
				} else {
					return reconcile.Result{}, b.handleErr(ctx, br, backendsStatuses, routesStatuses, phase, v1alpha1.BackendUpgrading, err)
				}
			}

			if !ptr.Deref(backendsStatuses.Stable.Conditions.Ready, false) {
				needUpdateStatus = true
				conditionTrue := true
				backendsStatuses.Stable.Conditions.Ready = &conditionTrue
				backendsStatuses.Stable.Name = br.Spec.Forwarding.Stable.Name
			}

			var routeBackendChangeErr []error
			for idx, routeSpec := range br.Spec.Routes {
				iRoute, err := b.getRoute(ctx, br.Namespace, routeSpec)
				if err != nil {
					return reconcile.Result{}, b.handleErr(ctx, br, backendsStatuses, routesStatuses, phase, v1alpha1.BackendUpgrading, err)
				}
				err = iRoute.ChangeBackend(ctx, route.BackendChangeDetail{
					Src:        br.Spec.Backend.Name,
					Dst:        br.Spec.Forwarding.Stable.Name,
					Kind:       br.Spec.Backend.Kind,
					ApiVersion: br.Spec.Backend.APIVersion,
				})
				if err != nil {
					if routesStatuses[idx].Synced {
						routesStatuses[idx].Synced = false
						needUpdateStatus = true
					}
					routeBackendChangeErr = append(routeBackendChangeErr, err)
					continue
				}
				if !routesStatuses[idx].Synced {
					routesStatuses[idx].Synced = true
					needUpdateStatus = true
				}
			}

			if len(routeBackendChangeErr) > 0 {
				return reconcile.Result{}, b.handleErr(ctx, br, backendsStatuses, routesStatuses, phase, v1alpha1.BackendUpgrading, multierr.Combine(routeBackendChangeErr...))
			}

			if phase != v1alpha1.Ready {
				phase = v1alpha1.Ready
				needUpdateStatus = true
			}
			if needUpdateStatus {
				return reconcile.Result{}, b.updateBackendRoutingStatus(ctx, br, backendsStatuses, routesStatuses, phase)
			}
		}
	}
	return reconcile.Result{}, nil
}

func (b *BackendRoutingReconciler) ensureCanaryRemove(ctx context.Context, br *v1alpha1.BackendRouting) error {
	needUpdateStatus := false
	backendsStatuses := br.Status.Backends
	routesStatuses := br.Status.RouteStatuses
	if len(routesStatuses) == 0 {
		routesStatuses = make([]v1alpha1.BackendRouteStatus, len(br.Spec.Routes))
	}
	phase := br.Status.Phase

	if backendsStatuses.Canary.Conditions.Terminating == nil || !*backendsStatuses.Canary.Conditions.Terminating {
		// delete canary route
		var routeCanaryRemoveErr []error
		for idx, routeSpec := range br.Spec.Routes {
			iRoute, err := b.getRoute(ctx, br.Namespace, routeSpec)
			if err != nil {
				return b.handleErr(ctx, br, backendsStatuses, routesStatuses, phase, v1alpha1.RouteUpgrading, err)
			}
			err = iRoute.RemoveCanaryRoute(ctx)
			if err != nil {
				if routesStatuses[idx].Synced {
					routesStatuses[idx].Synced = false
				}
				routeCanaryRemoveErr = append(routeCanaryRemoveErr, err)
				continue
			}
			if !routesStatuses[idx].Synced {
				routesStatuses[idx].Synced = true
			}
		}
		if len(routeCanaryRemoveErr) > 0 {
			return b.handleErr(ctx, br, backendsStatuses, routesStatuses, phase, v1alpha1.RouteUpgrading, multierr.Combine(routeCanaryRemoveErr...))
		}

		// delete canary backend
		canaryBackend, err := b.getBackend(ctx, br, backendsStatuses.Canary.Name)
		if err != nil {
			if errors.IsNotFound(err) {
				// already deleted
				conditionTrue := true
				conditionFalse := false
				backendsStatuses.Canary.Conditions.Terminating = &conditionTrue
				backendsStatuses.Canary.Conditions.Ready = &conditionFalse
				phase = v1alpha1.Ready
				needUpdateStatus = true
			} else {
				return err
			}
		} else {
			if canaryBackend.GetBackendObject().GetDeletionTimestamp() == nil {
				err = b.Client.Delete(clusterinfo.WithCluster(ctx, br.Spec.Backend.Cluster), canaryBackend.GetBackendObject())
				if err != nil {
					return err
				}
			}
			conditionTrue := true
			conditionFalse := false
			backendsStatuses.Canary.Conditions.Terminating = &conditionTrue
			backendsStatuses.Canary.Conditions.Ready = &conditionFalse
			phase = v1alpha1.RouteUpgrading
			needUpdateStatus = true
		}
	} else {
		// check canary route deleted
		var routeCanaryRemoveErr []error
		for idx, routeSpec := range br.Spec.Routes {
			iRoute, err := b.getRoute(ctx, br.Namespace, routeSpec)
			if err != nil {
				return b.handleErr(ctx, br, backendsStatuses, routesStatuses, phase, v1alpha1.RouteUpgrading, err)
			}
			err = iRoute.RemoveCanaryRoute(ctx)
			if err != nil {
				if routesStatuses[idx].Synced {
					routesStatuses[idx].Synced = false
				}
				routeCanaryRemoveErr = append(routeCanaryRemoveErr, err)
				continue
			}
			if !routesStatuses[idx].Synced {
				routesStatuses[idx].Synced = true
			}
		}
		if len(routeCanaryRemoveErr) > 0 {
			return b.handleErr(ctx, br, backendsStatuses, routesStatuses, phase, v1alpha1.RouteUpgrading, multierr.Combine(routeCanaryRemoveErr...))
		}

		// check canary backend deleted
		_, err := b.getBackend(ctx, br, backendsStatuses.Canary.Name)
		if !errors.IsNotFound(err) {
			return b.handleErr(ctx, br, backendsStatuses, routesStatuses, phase, v1alpha1.RouteUpgrading, fmt.Errorf("canary backend not deleted yet"))
		}
		backendsStatuses.Canary = v1alpha1.BackendStatus{}
		needUpdateStatus = true
	}

	if needUpdateStatus {
		return b.updateBackendRoutingStatus(ctx, br, backendsStatuses, routesStatuses, phase)
	}
	return nil
}

func (b *BackendRoutingReconciler) ensureCanaryAdd(ctx context.Context, br *v1alpha1.BackendRouting) error {
	needUpdateStatus := false
	if br.Status.ObservedGeneration != br.Generation {
		needUpdateStatus = true
	}
	backendsStatuses := br.Status.Backends
	routesStatuses := br.Status.RouteStatuses
	if len(routesStatuses) == 0 {
		routesStatuses = make([]v1alpha1.BackendRouteStatus, len(br.Spec.Routes))
	}
	phase := br.Status.Phase

	// todo: discussion
	// should we check origin & stable here?
	// check canary backend and route
	_, err := b.getBackend(ctx, br, br.Spec.Forwarding.Canary.Name)
	if err != nil {
		if !errors.IsNotFound(err) {
			return b.handleErr(ctx, br, backendsStatuses, routesStatuses, phase, v1alpha1.RouteUpgrading, err)
		}
		// canary backend not exist, create canary backend, get origin backend first
		originBackend, err := b.getBackend(ctx, br, br.Spec.Backend.Name)
		if err != nil {
			return b.handleErr(ctx, br, backendsStatuses, routesStatuses, phase, v1alpha1.RouteUpgrading, err)
		}

		canaryForked := originBackend.ForkCanary(br.Spec.Forwarding.Canary.Name, ControllerName)
		err = b.Client.Create(clusterinfo.WithCluster(ctx, br.Spec.Backend.Cluster), canaryForked)
		if err != nil {
			return b.handleErr(ctx, br, backendsStatuses, routesStatuses, phase, v1alpha1.RouteUpgrading, err)
		}
	}
	if backendsStatuses.Canary.Name != br.Spec.Forwarding.Canary.Name || !ptr.Deref(backendsStatuses.Canary.Conditions.Ready, false) {
		backendsStatuses.Canary.Name = br.Spec.Forwarding.Canary.Name
		conditionTrue := true
		backendsStatuses.Canary.Conditions.Ready = &conditionTrue
		needUpdateStatus = true
	}

	var routeCanaryCreateErr []error
	for idx, routeSpec := range br.Spec.Routes {
		iRoute, err := b.getRoute(ctx, br.Namespace, routeSpec)
		if err != nil {
			return b.handleErr(ctx, br, backendsStatuses, routesStatuses, phase, v1alpha1.RouteUpgrading, err)
		}
		err = iRoute.AddCanaryRoute(ctx, br.Spec.Forwarding)
		if err != nil {
			if routesStatuses[idx].Synced {
				routesStatuses[idx].Synced = false
				needUpdateStatus = true
			}
			routeCanaryCreateErr = append(routeCanaryCreateErr, err)
			continue
		}
		if !routesStatuses[idx].Synced {
			routesStatuses[idx].Synced = true
			needUpdateStatus = true
		}
	}

	if len(routeCanaryCreateErr) > 0 {
		return b.handleErr(ctx, br, backendsStatuses, routesStatuses, phase, v1alpha1.RouteUpgrading, multierr.Combine(routeCanaryCreateErr...))
	}

	if phase != v1alpha1.Ready {
		phase = v1alpha1.Ready
		needUpdateStatus = true
	}

	if needUpdateStatus {
		return b.updateBackendRoutingStatus(ctx, br, backendsStatuses, routesStatuses, phase)
	}

	return nil
}

func (b *BackendRoutingReconciler) handleErr(ctx context.Context, br *v1alpha1.BackendRouting, backendsStatuses v1alpha1.BackendStatuses,
	routesStatuses []v1alpha1.BackendRouteStatus, phase, desiredPhase v1alpha1.BackendRoutingPhase, err error,
) error {
	if phase != desiredPhase {
		phase = desiredPhase
		_ = b.updateBackendRoutingStatus(ctx, br, backendsStatuses, routesStatuses, phase)
	}
	return err
}

func (b *BackendRoutingReconciler) getBackend(ctx context.Context, br *v1alpha1.BackendRouting, backendName string) (backend.IBackend, error) {
	backendStore, err := b.backendRegistry.Get(schema.FromAPIVersionAndKind(br.Spec.Backend.APIVersion, br.Spec.Backend.Kind))
	if err != nil {
		return nil, err
	}

	return backendStore.Get(ctx, br.Spec.Backend.Cluster, br.Namespace, backendName)
}

func (b *BackendRoutingReconciler) getRoute(ctx context.Context, namespace string, routeInfo v1alpha1.CrossClusterObjectReference) (route.IRoute, error) {
	routeStore, err := b.routeRegistry.Get(schema.FromAPIVersionAndKind(routeInfo.APIVersion, routeInfo.Kind))
	if err != nil {
		return nil, err
	}

	return routeStore.Get(ctx, routeInfo.Cluster, namespace, routeInfo.Name)
}

func (b *BackendRoutingReconciler) updateBackendRoutingStatus(ctx context.Context, br *v1alpha1.BackendRouting,
	backends v1alpha1.BackendStatuses, routes []v1alpha1.BackendRouteStatus, phase v1alpha1.BackendRoutingPhase,
) error {
	brGet := &v1alpha1.BackendRouting{}
	err := b.Client.Get(clusterinfo.WithCluster(ctx, clusterinfo.Fed), types.NamespacedName{
		Name:      br.Name,
		Namespace: br.Namespace,
	}, brGet)
	if err != nil {
		return err
	}
	brGet.Status.ObservedGeneration = br.Generation
	brGet.Status.Backends = backends
	brGet.Status.RouteStatuses = routes
	brGet.Status.Phase = phase
	return b.Client.Status().Update(clusterinfo.WithCluster(ctx, clusterinfo.Fed), brGet)
}

func (b *BackendRoutingReconciler) reconcileMultiClusterType(_ context.Context, _ *v1alpha1.BackendRouting) (reconcile.Result, error) {
	// todo
	return reconcile.Result{}, nil
}

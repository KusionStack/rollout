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
	"time"

	"github.com/go-logr/logr"
	"github.com/samber/lo"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/utils/ptr"
	rolloutapi "kusionstack.io/kube-api/rollout"
	rolloutv1alpha1 "kusionstack.io/kube-api/rollout/v1alpha1"
	clientutil "kusionstack.io/kube-utils/client"
	"kusionstack.io/kube-utils/controller/expectations"
	"kusionstack.io/kube-utils/controller/mixin"
	"kusionstack.io/kube-utils/multicluster/clusterinfo"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"kusionstack.io/rollout/pkg/controllers/registry"
	"kusionstack.io/rollout/pkg/features/rolloutclasspredicate"
	"kusionstack.io/rollout/pkg/trafficrouting/backend"
	"kusionstack.io/rollout/pkg/trafficrouting/route"
)

const (
	ControllerName = "backendrouting"
)

type BackendRoutingReconciler struct {
	*mixin.ReconcilerMixin
	backendRegistry registry.BackendRegistry
	routeRegistry   registry.RouteRegistry

	rvExpectation expectations.ResourceVersionExpectationInterface
}

func NewReconciler(mgr manager.Manager, backendRegistry registry.BackendRegistry, routeRegistry registry.RouteRegistry) *BackendRoutingReconciler {
	return &BackendRoutingReconciler{
		ReconcilerMixin: mixin.NewReconcilerMixin(ControllerName, mgr),
		backendRegistry: backendRegistry,
		routeRegistry:   routeRegistry,
		rvExpectation:   expectations.NewResourceVersionExpectation(),
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
		For(&rolloutv1alpha1.BackendRouting{}, builder.WithPredicates(
			predicate.ResourceVersionChangedPredicate{},
			// NOTE: This controller only watches one kind of resource,
			// so we can use predicate to filter events by rollout-class
			rolloutclasspredicate.RolloutClassMatchesPredicate,
		)).
		Complete(b)
}

//+kubebuilder:rbac:groups=rollout.kusionstack.io,resources=backendroutings,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=rollout.kusionstack.io,resources=backendroutings/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=rollout.kusionstack.io,resources=backendroutings/finalizers,verbs=update;patch
//+kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="networking.k8s.io",resources=ingresses,verbs=get;list;watch;create;update;patch;delete

func (b *BackendRoutingReconciler) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	key := request.String()
	logger := b.Logger.WithValues("backendrouting", key)
	// set logger into context
	ctx = logr.NewContext(ctx, logger)
	logger.V(4).Info("started reconciling backendrouting")
	defer logger.V(4).Info("finished reconciling backendrouting")

	obj := &rolloutv1alpha1.BackendRouting{}
	err := b.Client.Get(clusterinfo.WithCluster(ctx, clusterinfo.Fed), request.NamespacedName, obj)
	if err != nil {
		return reconcile.Result{}, client.IgnoreNotFound(err)
	}

	// todo: finalizers' management

	// terminating
	if obj.GetDeletionTimestamp() != nil {
		return b.reconcileTerminatingBackendRouting(ctx, obj)
	}

	// check resourceVersion expectation
	if !b.satisfiedExpectations(ctx, obj) {
		return ctrl.Result{}, nil
	}

	syncCtx, err := b.initSyncContext(ctx, obj)

	if err == nil {
		switch obj.Spec.TrafficType {
		case rolloutv1alpha1.InClusterTrafficType:
			err = b.syncInCluster(ctx, syncCtx)
		case rolloutv1alpha1.MultiClusterTrafficType:
			// TODO: implement multi cluster traffic type
		}
	}

	if err != nil {
		logger.Error(err, "failed to reconcile backendrouting")
	}

	// update status firstly
	updateStatusErr := b.updateStatusOnly(ctx, syncCtx)
	if updateStatusErr != nil {
		return reconcile.Result{}, updateStatusErr
	}

	// NOTE: we need to use IsStatusConditionTrue here rather than IsStatusConditionFalse
	//       to make sure backendrouting is not ready.
	if !meta.IsStatusConditionTrue(syncCtx.Object.Status.Conditions, rolloutv1alpha1.BackendRoutingReady) {
		return reconcile.Result{RequeueAfter: 5 * time.Second}, nil
	}

	return reconcile.Result{}, err
}

func (r *BackendRoutingReconciler) satisfiedExpectations(ctx context.Context, obj client.Object) bool {
	key := clientutil.ObjectKeyString(obj)
	logger := logr.FromContextOrDiscard(ctx)
	if !r.rvExpectation.SatisfiedExpectations(key, obj.GetResourceVersion()) {
		logger.Info("object does not statisfy resourceVersion expectation, skip reconciling")
		return false
	}
	return true
}

func (r *BackendRoutingReconciler) initSyncContext(ctx context.Context, obj *rolloutv1alpha1.BackendRouting) (*syncContext, error) {
	syncCtx := &syncContext{
		Object: obj,
		Routes: make([]route.RouteController, len(obj.Spec.Routes)),
	}
	syncCtx.SetDefaults()

	// find origin backend
	_, backendObj, err := r.findBackend(ctx, obj, obj.Spec.Backend.Name)
	if err != nil {
		syncCtx.NewStatus.Backends.Origin.Conditions.Ready = ptr.To(false)
		return syncCtx, err
	} else {
		syncCtx.NewStatus.Backends.Origin.Conditions.Ready = ptr.To(true)
	}

	syncCtx.BackendObject = backendObj

	err = r.findAllRouteControllers(ctx, syncCtx)
	if err != nil {
		return syncCtx, err
	}
	return syncCtx, nil
}

func (b *BackendRoutingReconciler) updateStatusOnly(ctx context.Context, syncCtx *syncContext) error {
	logger := logr.FromContextOrDiscard(ctx)
	newStatus := syncCtx.Status()
	if equality.Semantic.DeepEqual(syncCtx.Object.Status, newStatus) {
		return nil
	}
	_, err := clientutil.UpdateOnConflict(ctx, b.Client, b.Client.Status(), syncCtx.Object, func(in *rolloutv1alpha1.BackendRouting) error {
		in.Status = newStatus
		return nil
	})
	if err != nil {
		logger.Error(err, "failed to update status", "status", newStatus)
		return err
	}

	logger.V(4).Info("backendRouting status updated")
	key := clientutil.ObjectKeyString(syncCtx.Object)
	b.rvExpectation.ExpectUpdate(key, syncCtx.Object.ResourceVersion) // nolint
	return nil
}

func (b *BackendRoutingReconciler) reconcileTerminatingBackendRouting(_ context.Context, _ *rolloutv1alpha1.BackendRouting) (reconcile.Result, error) {
	// todo
	return reconcile.Result{}, nil
}

func (b *BackendRoutingReconciler) syncInCluster(ctx context.Context, syncCtx *syncContext) error {
	// sync backends
	err := b.syncInClusterBackends(ctx, syncCtx)
	if err != nil {
		return err
	}

	// sync route
	err = b.syncInClusterRoutes(ctx, syncCtx)
	if err != nil {
		return err
	}

	return nil
}

func (b *BackendRoutingReconciler) syncInClusterBackends(ctx context.Context, syncCtx *syncContext) error {
	obj := syncCtx.Object
	logger := logr.FromContextOrDiscard(ctx)

	if obj.Spec.ForkedBackends == nil {
		deleted, err := b.deleteBackendResource(ctx, syncCtx, syncCtx.NewStatus.Backends.Canary.Name)
		if err != nil {
			logger.Error(err, "failed delete canary backend resource", "backend", syncCtx.NewStatus.Backends.Canary.Name)
			return err
		}
		if deleted {
			syncCtx.NewStatus.Backends.Canary = rolloutv1alpha1.BackendStatus{}
		} else {
			syncCtx.NewStatus.Backends.Canary.Conditions = rolloutv1alpha1.BackendConditions{
				Terminating: ptr.To(true),
			}
		}
		deleted, err = b.deleteBackendResource(ctx, syncCtx, syncCtx.NewStatus.Backends.Stable.Name)
		if err != nil {
			logger.Error(err, "failed delete stable backend resource", "backend", syncCtx.NewStatus.Backends.Canary.Name)
			return err
		}
		if deleted {
			syncCtx.NewStatus.Backends.Stable = rolloutv1alpha1.BackendStatus{}
		} else {
			syncCtx.NewStatus.Backends.Stable.Conditions = rolloutv1alpha1.BackendConditions{
				Terminating: ptr.To(true),
			}
		}
		return nil
	}

	// ensure canary and stable backends
	canaryConfig := obj.Spec.ForkedBackends.Canary.DeepCopy()
	canaryConfig.ExtraLabelSelector = lo.Assign(canaryConfig.ExtraLabelSelector, map[string]string{
		rolloutapi.LabelTrafficLane: rolloutapi.LabelValueTrafficLaneCanary,
	})
	err := b.ensureBackendResource(ctx, syncCtx, *canaryConfig)
	if err != nil {
		return err
	}
	// change status
	syncCtx.NewStatus.Backends.Canary.Conditions.Ready = ptr.To(true)

	stableConfig := obj.Spec.ForkedBackends.Stable.DeepCopy()
	stableConfig.ExtraLabelSelector = lo.Assign(stableConfig.ExtraLabelSelector, map[string]string{
		rolloutapi.LabelTrafficLane: rolloutapi.LabelValueTrafficLaneStable,
	})
	err = b.ensureBackendResource(ctx, syncCtx, *stableConfig)
	if err != nil {
		return err
	}

	// change status
	syncCtx.NewStatus.Backends.Stable.Conditions.Ready = ptr.To(true)
	return nil
}

func (b *BackendRoutingReconciler) findBackend(ctx context.Context, br *rolloutv1alpha1.BackendRouting, name string) (backend.InClusterBackend, client.Object, error) {
	gvk := schema.FromAPIVersionAndKind(br.Spec.Backend.APIVersion, br.Spec.Backend.Kind)
	cluster := br.Spec.Backend.Cluster
	namespace := br.Namespace

	backendStore, err := b.backendRegistry.Get(gvk)
	if err != nil {
		return nil, nil, err
	}
	backendObj := backendStore.NewObject()
	err = b.Client.Get(clusterinfo.WithCluster(ctx, cluster), client.ObjectKey{
		Namespace: namespace,
		Name:      name,
	}, backendObj)
	return backendStore, backendObj, err
}

func (b *BackendRoutingReconciler) deleteBackendResource(ctx context.Context, syncCtx *syncContext, name string) (bool, error) {
	if len(name) == 0 {
		return true, nil
	}

	obj := syncCtx.Object
	ctx = clusterinfo.WithCluster(ctx, obj.Spec.Backend.Cluster)

	_, backendObj, err := b.findBackend(ctx, obj, name)
	if err != nil {
		if errors.IsNotFound(err) {
			// not found means already deleted
			return true, nil
		}
		return true, nil
	}

	if backendObj.GetDeletionTimestamp() != nil {
		// waiting for finalizers
		return false, nil
	}

	err = b.Client.Delete(ctx, backendObj)
	return false, err
}

func (b *BackendRoutingReconciler) ensureBackendResource(ctx context.Context, syncCtx *syncContext, config rolloutv1alpha1.ForkedBackend) error {
	obj := syncCtx.Object

	logger := logr.FromContextOrDiscard(ctx)
	logger = logger.WithValues("apiVersion", obj.Spec.Backend.APIVersion, "kind", obj.Spec.Backend.Kind, "name", config.Name)

	ctx = clusterinfo.WithCluster(ctx, obj.Spec.Backend.Cluster)
	backendStore, _, err := b.findBackend(ctx, obj, config.Name)
	if err == nil {
		// found
		return nil
	}
	if !errors.IsNotFound(err) {
		logger.Error(err, "failed to get backend resource")
		return err
	}

	// need to create
	newBackendObj := backendStore.Fork(syncCtx.BackendObject, config)
	// add label
	newBackendObj.SetLabels(lo.Assign(
		newBackendObj.GetLabels(),
		config.ExtraLabelSelector,
		map[string]string{
			rolloutapi.LabelCanaryResource: "true",
		},
	))

	err = b.Client.Create(ctx, newBackendObj)
	if err != nil {
		logger.Error(err, "failed to create backend resource")
		return err
	}
	logger.Info("backend resource created")
	return nil
}

func (b *BackendRoutingReconciler) syncInClusterRoutes(ctx context.Context, syncCtx *syncContext) error {
	syncRoute := func(i int, fn func(routeCtl route.RouteController) error) error {
		routeCtl := syncCtx.Routes[i]
		err := fn(routeCtl)
		if err != nil {
			// TODO: record error
			syncCtx.setRouteCondition(i, metav1.ConditionFalse, "SyncFailed", err.Error())
			return err
		}
		return nil
	}

	for i, routeStatus := range syncCtx.NewStatus.Routes {
		needCreate, needDelete := syncCtx.checkOriginRoute(routeStatus.Forwarding)

		if needCreate {
			err := syncRoute(i, func(routeCtl route.RouteController) error {
				return routeCtl.Initialize(ctx)
			})
			if err != nil {
				return err
			}
			syncCtx.NewStatus.Routes[i].Forwarding.Origin.Conditions.Ready = ptr.To(true)
		}
		if needDelete {
			// set condition firstly
			syncCtx.NewStatus.Routes[i].Forwarding.Origin.Conditions.Ready = nil
			syncCtx.NewStatus.Routes[i].Forwarding.Origin.Conditions.Terminating = ptr.To(true)

			err := syncRoute(i, func(routeCtl route.RouteController) error {
				return routeCtl.Reset(ctx)
			})
			if err != nil {
				return err
			}

			// delete forwarding status
			syncCtx.NewStatus.Routes[i].Forwarding.Origin = nil
		}

		needCreate, needDelete = syncCtx.checkCanaryRoute(routeStatus.Forwarding)
		if needCreate {
			err := syncRoute(i, func(routeCtl route.RouteController) error {
				return routeCtl.AddCanary(ctx)
			})
			if err != nil {
				return err
			}
			syncCtx.NewStatus.Routes[i].Forwarding.Canary.Conditions.Ready = ptr.To(true)
		}
		if needDelete {
			// set condition firstly
			syncCtx.NewStatus.Routes[i].Forwarding.Canary.Conditions.Ready = nil
			syncCtx.NewStatus.Routes[i].Forwarding.Canary.Conditions.Terminating = ptr.To(true)

			err := syncRoute(i, func(routeCtl route.RouteController) error {
				return routeCtl.DeleteCanary(ctx)
			})
			if err != nil {
				return err
			}
			syncCtx.NewStatus.Routes[i].Forwarding.Canary = nil
		}

		ctrl := syncCtx.Routes[i]
		status, reason, message := b.checkReadyExtension(ctx, ctrl)
		syncCtx.setRouteCondition(i, status, reason, message)
	}
	return nil
}

func (b *BackendRoutingReconciler) checkReadyExtension(_ context.Context, routeCtl route.RouteController) (metav1.ConditionStatus, string, string) {
	routeObj := routeCtl.GetRoute()

	conditionExt, err := route.GetConditionExtension(routeObj)
	if err != nil {
		return metav1.ConditionUnknown, "GetConditionsFailed", err.Error()
	}

	// if no condition extension found, return True
	if conditionExt == nil {
		return metav1.ConditionTrue, "NoConditionExtension", "route condition extension is not set"
	}

	generation := routeObj.GetGeneration()
	readCond := meta.FindStatusCondition(conditionExt.Conditions, rolloutv1alpha1.RouteConditionTypeReady)
	if readCond != nil {
		if generation != readCond.ObservedGeneration {
			return metav1.ConditionFalse, "OutOfSync", fmt.Sprintf("Ready condition is out of sync, route.Generation(%d) != condition.ObservedGeneration(%d)", generation, readCond.ObservedGeneration)
		}
		switch readCond.Status {
		case metav1.ConditionTrue:
			return readCond.Status, "ConditionExtensionReady", readCond.Message
		default:
			return metav1.ConditionFalse, "ConditionExtensionNotReady", readCond.Message
		}
	}
	syncedCond := meta.FindStatusCondition(conditionExt.Conditions, rolloutv1alpha1.RouteConditionTypeSynced)
	if syncedCond != nil {
		if generation != syncedCond.ObservedGeneration {
			return metav1.ConditionFalse, "OutOfSync", fmt.Sprintf("Scyned condition is out of sync, route.Generation(%d) != condition.ObservedGeneration(%d)", generation, syncedCond.ObservedGeneration)
		}
		switch syncedCond.Status {
		case metav1.ConditionTrue:
			if !syncedCond.LastTransitionTime.IsZero() && !metav1.Now().After(syncedCond.LastTransitionTime.Time.Add(30*time.Second)) {
				// if synced is true, we need to wait for 30 seconds
				return metav1.ConditionFalse, "SufficientDelayTime", "waiting 30 seconds to ensure sufficient time for routing rules to take effect."
			}
			return syncedCond.Status, "ConditionExtensionSynced", syncedCond.Message
		default:
			return metav1.ConditionFalse, "ConditionExtensionNotSynced", syncedCond.Message
		}
	}
	// no ready or synced condition found, return unknown
	return metav1.ConditionUnknown, "Unknown", "no Ready or Synced condition found in route condition extension"
}

func (b *BackendRoutingReconciler) findAllRouteControllers(ctx context.Context, syncCtx *syncContext) error {
	fn := func(routeInfo rolloutv1alpha1.CrossClusterObjectReference) (route.Route, client.Object, error) {
		routeAccessor, err := b.routeRegistry.Get(schema.FromAPIVersionAndKind(routeInfo.APIVersion, routeInfo.Kind))
		if err != nil {
			return nil, nil, err
		}

		routeObj := routeAccessor.NewObject()
		err = b.Client.Get(
			clusterinfo.WithCluster(ctx, routeInfo.Cluster),
			client.ObjectKey{Namespace: syncCtx.Object.Namespace, Name: routeInfo.Name},
			routeObj,
		)
		if err != nil {
			return nil, nil, err
		}
		return routeAccessor, routeObj, nil
	}

	errList := []error{}
	for i, routeInfo := range syncCtx.Object.Spec.Routes {
		accessor, routeObj, err := fn(routeInfo)
		if err != nil {
			syncCtx.setRouteCondition(i, metav1.ConditionUnknown, "Unknown", err.Error())
			errList = append(errList, err)
			continue
		}

		rctl, _ := accessor.GetController(b.Client, syncCtx.Object, routeObj, syncCtx.NewStatus.Routes[i])
		syncCtx.Routes[i] = rctl
	}

	return utilerrors.NewAggregate(errList)
}

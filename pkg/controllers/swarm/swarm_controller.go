// Copyright 2024 The KusionStack Authors
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

package swarm

import (
	"context"
	goerrors "errors"
	"fmt"
	"slices"

	"github.com/dominikbraun/graph"
	"github.com/go-logr/logr"
	"github.com/samber/lo"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	"kusionstack.io/kube-api/apps/condition"
	kusionstackappsv1alpha1 "kusionstack.io/kube-api/apps/v1alpha1"
	clientutil "kusionstack.io/kube-utils/client"
	"kusionstack.io/kube-utils/controller/history"
	"kusionstack.io/kube-utils/controller/mixin"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"kusionstack.io/rollout/pkg/controllers/swarm/workloadcontrol"
	"kusionstack.io/rollout/pkg/utils/eventhandler"
	"kusionstack.io/rollout/pkg/utils/expectations"
)

const (
	ControllerName = "swarm"
)

// SwarmReconciler reconciles a Rollout object
type SwarmReconciler struct {
	*mixin.ReconcilerMixin

	clsControl workloadcontrol.Interface

	historyManager history.HistoryManager

	expectation           expectations.ControllerExpectationsInterface
	rvExpectation         expectations.ResourceVersionExpectationInterface
	workloadRVExpectation expectations.ResourceVersionExpectationInterface
}

func NewReconciler(mgr manager.Manager) *SwarmReconciler {
	r := &SwarmReconciler{
		ReconcilerMixin:       mixin.NewReconcilerMixin(ControllerName, mgr),
		expectation:           expectations.NewControllerExpectations(),
		rvExpectation:         expectations.NewResourceVersionExpectation(),
		workloadRVExpectation: expectations.NewResourceVersionExpectation(),
	}

	r.clsControl = workloadcontrol.New(r.Client)
	r.historyManager = history.NewHistoryManager(history.NewRevisionControl(r.APIReader, r.Client), NewRevisionOwner(r.clsControl))

	return r
}

// SetupWithManager sets up the controller with the Manager.
func (r *SwarmReconciler) SetupWithManager(mgr ctrl.Manager) error {
	b := builder.ControllerManagedBy(mgr).
		For(&kusionstackappsv1alpha1.Swarm{}, builder.WithPredicates(predicate.ResourceVersionChangedPredicate{})).
		Watches(
			&source.Kind{Type: &kusionstackappsv1alpha1.CollaSet{}},
			eventhandler.EqueueRequestForOwnerWithCreationObserved(&kusionstackappsv1alpha1.Swarm{}, true, r.expectation),
		)

	return b.Complete(r)
}

//+kubebuilder:rbac:groups=apps.kusionstack.io,resources=swarms,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=apps.kusionstack.io,resources=swarms/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=apps.kusionstack.io,resources=swarms/finalizers,verbs=update
//+kubebuilder:rbac:groups=apps.kusionstack.io,resources=collasets,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *SwarmReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	key := req.String()
	logger := r.Logger.WithValues("swarm", key)
	ctx = logr.NewContext(ctx, logger)
	logger.V(4).Info("started reconciling swarm")
	defer logger.V(4).Info("finished reconciling swarm")

	obj := &kusionstackappsv1alpha1.Swarm{}
	err := r.Client.Get(ctx, req.NamespacedName, obj)
	if err != nil {
		if errors.IsNotFound(err) {
			r.expectation.DeleteExpectations(key)
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// TODO: deal with fianlizer

	if !r.satisfiedExpectations(ctx, obj) {
		return ctrl.Result{}, nil
	}

	inUse, toDelete, err := r.clsControl.ListAll(ctx, obj)
	if err != nil {
		return ctrl.Result{}, err
	}

	// delete collasets
	for _, cls := range toDelete {
		if err := r.clsControl.Delete(ctx, cls.Object); err != nil {
			return ctrl.Result{}, err
		}
	}

	currentRevision, updatedRevision, revisions, collisionCount, _, err := r.historyManager.ConstructRevisions(ctx, obj)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to construct revision for swarm: %v", err)
	}

	if !r.satisfiedWrokloadExpectations(ctx, inUse) {
		return ctrl.Result{}, nil
	}

	g, err := parseGraphFrom(obj, inUse)
	if err != nil {
		return ctrl.Result{}, err
	}

	swarmCtx := &swarmContext{
		Object:          obj,
		Graph:           g,
		CurrentRevision: currentRevision,
		UpdatedRevision: updatedRevision,
		Revisions:       revisions,
		CollisionCount:  collisionCount,
	}

	err = swarmCtx.StableTopologySortTraversal(ctx, r.syncWorkload)
	if err != nil {
		return reconcile.Result{}, err
	}

	err = r.updateStatusOnly(ctx, swarmCtx)
	if err != nil {
		return ctrl.Result{}, err
	}

	return reconcile.Result{}, nil
}

func (r *SwarmReconciler) updateStatusOnly(ctx context.Context, swarmCtx *swarmContext) error {
	// create new status
	newStatus := swarmCtx.NewStatus()
	obj := swarmCtx.Object
	if equality.Semantic.DeepEqual(obj.Status, *newStatus) {
		// no change
		return nil
	}
	_, err := clientutil.UpdateOnConflict(ctx, r.Client, r.Client, obj, func(in *kusionstackappsv1alpha1.Swarm) error {
		in.Status = *newStatus
		return nil
	})
	if err != nil {
		r.recordCondition(ctx, swarmCtx, reconcileOK, metav1.ConditionFalse, "FailedUpdateStatus", fmt.Sprintf("failed to update status: %v", err))
		return err
	}
	key := clientutil.ObjectKeyString(obj)
	logger := logr.FromContextOrDiscard(ctx)
	logger.V(4).Info("swarm status updated")
	r.rvExpectation.ExpectUpdate(key, obj.ResourceVersion) // nolint
	return nil
}

func (r *SwarmReconciler) satisfiedExpectations(ctx context.Context, obj client.Object) bool {
	key := clientutil.ObjectKeyString(obj)
	logger := logr.FromContextOrDiscard(ctx)

	if !r.rvExpectation.SatisfiedExpectations(key, obj.GetResourceVersion()) {
		logger.Info("swarm does not statisfy resourceVersion expectation, skip reconciling")
		return false
	}

	if !r.expectation.SatisfiedExpectations(key) {
		logger.Info("swarm does not statisfy controller expectation, skip reconciling")
		return false
	}
	return true
}

func (r *SwarmReconciler) satisfiedWrokloadExpectations(ctx context.Context, workloads map[string]*workloadcontrol.Workload) bool {
	logger := logr.FromContextOrDiscard(ctx)
	for _, workload := range workloads {
		if !r.workloadRVExpectation.SatisfiedExpectations(clientutil.ObjectKeyString(workload.Object), workload.Object.GetResourceVersion()) {
			logger.Info("workload does not statisfy controller expectation, skip reconciling")
			return false
		}
	}
	return true
}

func (r *SwarmReconciler) syncWorkload(ctx context.Context, swarmCtx *swarmContext, workload *workloadcontrol.Workload) (bool, error) {
	if workload.Spec.TopologyAware != nil {
		// here is a protect logic to make sure all dependency is ready
		// Usually we will call this function from topology order and all dependency is ready
		// But if we call this function from other place, we need to check it manually
		for _, dependency := range workload.Spec.TopologyAware.DependsOn {
			v, err := swarmCtx.Graph.Vertex(dependency)
			if err != nil {
				return false, fmt.Errorf("failed to get vertex(%s) in graph, err: %v ", dependency, err)
			}
			if !isCollaSetReady(v.Object) {
				r.recordCondition(ctx, swarmCtx, reconcileOK, metav1.ConditionFalse, "DependencyNotReady", fmt.Sprintf("the dependency(%s) of group (%s) is not ready", workload.Spec.Name, dependency))
				return false, nil
			}
		}
	}

	swarmKey := clientutil.ObjectKeyString(swarmCtx.Object)

	// generate new collaset from updated revision
	clsUpdated, err := generateCollaSetFromRevision(swarmCtx.Object, workload, swarmCtx.UpdatedRevision)
	injectTopology(clsUpdated, swarmCtx.Graph, workload)

	if workload.Object == nil {
		// NOTO: we have to set expectation before we create the rolloutRun to avoid
		//       that the creation event comes so fast that we don't have time to set it
		r.expectation.ExpectCreations(swarmKey, 1) // nolint

		// create workload
		err := r.Client.Create(ctx, clsUpdated)
		if err != nil {
			r.recordCondition(ctx, swarmCtx, reconcileOK, metav1.ConditionFalse, "FailedCreate", fmt.Sprintf("failed to create new collaset %s", clsUpdated.Name))
			// lower the expectation if create failed
			r.expectation.LowerExpectations(swarmKey, 1, 0)
			return false, fmt.Errorf("failed to create workload: %w", err)
		}

		// r.recordCondition(ctx, swarmCtx, reconcileOK, metav1.ConditionFalse, "Created", "collaset created")
		workload.Object = clsUpdated
		return false, nil
	}

	// try to update
	clsCurrent := workload.Object
	revisionCurrent := swarmCtx.GetRevisionFor(clsCurrent)
	clsOriginal, err := generateCollaSetFromRevision(swarmCtx.Object, workload, revisionCurrent)

	updateFn := func(inClusterObj *kusionstackappsv1alpha1.CollaSet) error {
		// always update replicas
		inClusterObj.Spec.Replicas = clsUpdated.Spec.Replicas

		// check revison hand injection hash
		currentRevisionHash := lo.ValueOr(inClusterObj.Labels, history.ControllerRevisionHashLabel, "")
		updatedRevisionHash := lo.ValueOr(clsUpdated.Labels, history.ControllerRevisionHashLabel, "")
		currentInjectionHash := lo.ValueOr(inClusterObj.Labels, InjectionHashLabel, "")
		updatedInjectionHash := lo.ValueOr(clsUpdated.Labels, InjectionHashLabel, "")
		if currentRevisionHash == updatedRevisionHash && len(updatedRevisionHash) > 0 &&
			currentInjectionHash == updatedInjectionHash {
			// if the revision hash is the same, we don't need to update the collaset
			return nil
		}

		inClusterObj.Labels = treeWayMergeMap(clsOriginal.Labels, clsUpdated.Labels, inClusterObj.Labels)
		inClusterObj.Annotations = treeWayMergeMap(clsOriginal.Annotations, clsUpdated.Annotations, inClusterObj.Annotations)
		inClusterObj.Spec.Template = clsUpdated.Spec.Template
		inClusterObj.Spec.VolumeClaimTemplates = clsUpdated.Spec.VolumeClaimTemplates
		return nil
	}

	changed, err := clientutil.UpdateOnConflict(ctx, r.Client, r.Client, clsCurrent, updateFn)
	if err != nil {
		r.recordCondition(ctx, swarmCtx, reconcileOK, metav1.ConditionFalse, "FailedUpdate", fmt.Sprintf("failed to update collaset %s: %v", clsCurrent.Name, err))
		return false, err
	}
	if changed {
		workload.Object = clsCurrent
		r.workloadRVExpectation.ExpectUpdate(clientutil.ObjectKeyString(clsCurrent), clsCurrent.GetResourceVersion()) //nolint
		return false, nil
	}

	return isCollaSetReady(clsCurrent), nil
}

const (
	reconcileOK string = "reconcileOK"
)

func (r *SwarmReconciler) recordCondition(ctx context.Context, swarmCtx *swarmContext, ctype string, expectedStatus metav1.ConditionStatus, reason, message string) {
	logger := logr.FromContextOrDiscard(ctx)

	if swarmCtx.conditions == nil {
		swarmCtx.conditions = swarmCtx.Object.Status.Conditions
	}

	// original := condition.GetCondition(c.conditions, ctype)
	eventtype := corev1.EventTypeNormal
	if expectedStatus != metav1.ConditionTrue {
		eventtype = corev1.EventTypeWarning
	}

	switch ctype {
	case reconcileOK:
		// NOTE: this is a special condition, it is used to print log and event
		if expectedStatus != metav1.ConditionTrue {
			logger.Error(goerrors.New("reconcile error"), "", "reason", reason, "message", message)
			r.Recorder.Eventf(swarmCtx.Object, eventtype, reason, message)
		}
		// stop at here
		return
	}
	cond := condition.NewCondition(ctype, expectedStatus, reason, message)
	swarmCtx.conditions = condition.SetCondition(swarmCtx.conditions, *cond)
}

type swarmContext struct {
	Object          *kusionstackappsv1alpha1.Swarm
	Graph           graph.Graph[string, *workloadcontrol.Workload]
	CurrentRevision *appsv1.ControllerRevision
	UpdatedRevision *appsv1.ControllerRevision
	Revisions       []*appsv1.ControllerRevision
	CollisionCount  int32

	conditions []metav1.Condition
}

func (c *swarmContext) NewStatus() *kusionstackappsv1alpha1.SwarmStatus {
	// create new status
	newStatus := c.Object.Status.DeepCopy()
	newStatus.ObservedGeneration = c.Object.Generation
	newStatus.CollisionCount = &c.CollisionCount
	newStatus.CurrentRevision = c.CurrentRevision.APIVersion
	newStatus.UpdatedRevision = c.UpdatedRevision.Name
	groupNames := lo.Map(c.Object.Spec.PodGroups, func(pg kusionstackappsv1alpha1.SwarmPodGroupSpec, _ int) string {
		return pg.Name
	})
	newStatus.PodGroupStatuses = getPodGroupStatusFromGraph(groupNames, c.Graph)
	if len(c.conditions) > 0 {
		newStatus.Conditions = c.conditions
	}
	return newStatus
}

func (c *swarmContext) GetRevisionFor(obj client.Object) *appsv1.ControllerRevision {
	labels := obj.GetLabels()
	if len(labels) == 0 {
		return c.CurrentRevision
	}
	revision, ok := c.getRevision(labels[history.ControllerRevisionHashLabel])
	if !ok {
		return c.CurrentRevision
	}
	return revision
}

func (c *swarmContext) getRevision(name string) (*appsv1.ControllerRevision, bool) {
	if name == c.CurrentRevision.Name {
		return c.CurrentRevision, true
	} else if name == c.UpdatedRevision.Name {
		return c.UpdatedRevision, true
	}
	for _, revision := range c.Revisions {
		if revision.Name == name {
			return revision, true
		}
	}
	return nil, false
}

func (c *swarmContext) StableTopologySortTraversal(
	ctx context.Context,
	visitFn func(ctx context.Context, topo *swarmContext, workload *workloadcontrol.Workload) (bool, error),
) error {
	// stable topology sort by Kahn's algorithm
	logger := logr.FromContextOrDiscard(ctx)
	predecessorMap, err := c.Graph.PredecessorMap()
	if err != nil {
		return fmt.Errorf("failed to get predecessor map: %w", err)
	}

	queue := make([]string, 0)
	queued := sets.NewString()

	for vertex, predecessors := range predecessorMap {
		if len(predecessors) == 0 {
			// get all in degree 0 vertex
			queue = append(queue, vertex)
			queued.Insert(vertex)
		}
	}

	slices.Sort(queue)

	errs := []error{}

	// the queue contains all vertexes which has zero in-degree
	for len(queue) > 0 {
		currentVertex := queue[0]
		queue = queue[1:]

		logger.V(5).Info("processing vertex", "currentVertex", currentVertex)

		workload, _ := c.Graph.Vertex(currentVertex)
		ready, err := visitFn(ctx, c, workload)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if !ready {
			// if workload is not synced, do not delete it from predecessorMap
			// It will block all subsequent vertexes
			logger.V(4).Info("workload is not ready, check it next time", "collaset", workload.Object.Name)
			continue
		}

		logger.V(5).Info("workload is ready now, delete all edge from this workload", "collaset", workload.Object.Name)

		frontier := make([]string, 0)
		for vertex, predecessors := range predecessorMap {
			// delete all edge from currentVertex to vertex
			delete(predecessors, currentVertex)
			if len(predecessors) > 0 {
				// vertex still has in degree, skip it
				continue
			}
			if queued.Has(vertex) {
				// vertex has been added to queue, skip it
				continue
			}

			// vertex has zero in-degree, add to queue
			logger.V(5).Info("append vertex to queue", "vertext", vertex)
			frontier = append(frontier, vertex)
			queued.Insert(vertex)
		}

		slices.Sort(frontier)
		queue = append(queue, frontier...)
	}

	if len(errs) > 0 {
		return fmt.Errorf("failed to sync swarm: %w", utilerrors.NewAggregate(errs))
	}
	return nil
}

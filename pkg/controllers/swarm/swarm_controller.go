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
	"fmt"

	"github.com/dominikbraun/graph"
	"github.com/go-logr/logr"
	"github.com/samber/lo"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/utils/ptr"
	kusionstackappsv1alpha1 "kusionstack.io/kube-api/apps/v1alpha1"
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
	"kusionstack.io/rollout/pkg/utils"
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

	currentRevision, updatedRevision, _, collisionCount, _, err := r.historyManager.ConstructRevisions(ctx, obj)
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

	err = r.sync(ctx, obj, g)
	if err != nil {
		return reconcile.Result{}, err
	}

	// create new status
	newStatus := obj.Status.DeepCopy()
	newStatus.ObservedGeneration = obj.Generation
	newStatus.CollisionCount = &collisionCount
	newStatus.CurrentRevision = currentRevision.Name
	newStatus.UpdatedRevision = updatedRevision.Name
	groupNames := lo.Map(obj.Spec.PodGroups, func(pg kusionstackappsv1alpha1.SwarmPodGroupSpec, _ int) string {
		return pg.Name
	})
	podGroupStatus := getPodGroupStatusFromGraph(groupNames, g)
	newStatus.PodGroupStatuses = podGroupStatus

	err = r.updateStatusOnly(ctx, obj, newStatus)
	if err != nil {
		return ctrl.Result{}, err
	}

	return reconcile.Result{}, nil
}

func (r *SwarmReconciler) updateStatusOnly(ctx context.Context, obj *kusionstackappsv1alpha1.Swarm, newStatus *kusionstackappsv1alpha1.SwarmStatus) error {
	if equality.Semantic.DeepEqual(obj.Status, *newStatus) {
		// no change
		return nil
	}
	key := utils.ObjectKeyString(obj)
	_, err := utils.UpdateOnConflict(ctx, r.Client, r.Client.Status(), obj, func() error {
		obj.Status = *newStatus
		return nil
	})
	if err != nil {
		return err
	}
	r.Logger.Info("update swarm status", "swarm", key)
	r.rvExpectation.ExpectUpdate(key, obj.ResourceVersion) // nolint
	return nil
}

func (r *SwarmReconciler) satisfiedExpectations(ctx context.Context, obj client.Object) bool {
	key := utils.ObjectKeyString(obj)
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
		if !r.workloadRVExpectation.SatisfiedExpectations(utils.ObjectKeyString(workload.Object), workload.Object.GetResourceVersion()) {
			logger.Info("workload does not statisfy controller expectation, skip reconciling")
			return false
		}
	}
	return true
}

func (r *SwarmReconciler) sync(ctx context.Context, obj *kusionstackappsv1alpha1.Swarm, g graph.Graph[string, *workloadcontrol.Workload]) error {
	// topology sort by Kahn's algorithm
	logger := logr.FromContextOrDiscard(ctx)
	predecessorMap, err := g.PredecessorMap()
	if err != nil {
		return fmt.Errorf("failed to get predecessor map: %w", err)
	}

	queue := make([]string, 0)
	processOnce := sets.NewString()

	for vertex, predecessors := range predecessorMap {
		if len(predecessors) == 0 {
			// get all in degree 0 vertex
			queue = append(queue, vertex)
			processOnce.Insert(vertex)
		}
	}

	errs := []error{}

	// the queue contains all vertexes which has zero in-degree
	for len(queue) > 0 {
		currentVertex := queue[0]
		queue = queue[1:]

		logger.Info("processing vertex", "currentVertex", currentVertex)
		// visite currentVertex
		processOnce.Insert(currentVertex)

		workload, _ := g.Vertex(currentVertex)
		done, err := r.syncWorkload(ctx, obj, g, workload)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if !done {
			// if workload is not synced, do not delete it from predecessorMap
			// It will block all subsequent vertexes
			logger.Info("workload is not ready, check it next time", "collaset", workload.Object.Name)
			continue
		}

		logger.Info("workload is ready now, delete all edge from this workload", "collaset", workload.Object.Name)
		for vertex, predecessors := range predecessorMap {
			if processOnce.Has(vertex) {
				// vertex has been added to queue, skip it
				continue
			}

			// delete all edge from currentVertex to vertex
			delete(predecessors, currentVertex)

			if len(predecessors) == 0 {
				// vertex has zero in-degree, add to queue
				logger.Info("append vertex to queue", "vertext", vertex)
				queue = append(queue, vertex)
				processOnce.Insert(vertex)
			}
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("failed to sync swarm: %w", utilerrors.NewAggregate(errs))
	}

	return nil
}

func (r *SwarmReconciler) syncWorkload(ctx context.Context, obj *kusionstackappsv1alpha1.Swarm, g graph.Graph[string, *workloadcontrol.Workload], workload *workloadcontrol.Workload) (bool, error) {
	logger := logr.FromContextOrDiscard(ctx)

	if workload.Spec.TopologyAware != nil {
		// here is a protect logic to make sure all dependency is ready
		// Usually we will call this function from topology order and all dependency is ready
		// But if we call this function from other place, we need to check it manually
		for _, dependency := range workload.Spec.TopologyAware.DependsOn {
			v, err := g.Vertex(dependency)
			if err != nil {
				return false, fmt.Errorf("failed to get vertex(%s) in graph, err: %v ", dependency, err)
			}
			if !isCollaSetSynced(v.Object) {
				logger.Info("group's dependency is not ready, skip it", "currentGroup", workload.Spec.Name, "dependency", dependency)
				return false, nil
			}
		}
	}

	generated, err := generateCollaSet(obj, g, workload)
	if err != nil {
		return false, err
	}

	swarmKey := utils.ObjectKeyString(obj)

	if workload.Object == nil {
		// NOTO: we have to set expectation before we create the rolloutRun to avoid
		//       that the creation event comes so fast that we don't have time to set it
		r.expectation.ExpectCreations(swarmKey, 1) // nolint

		// create workload
		err := r.Client.Create(ctx, generated)
		if err != nil {
			logger.Error(err, "failed to create collaset")
			// lower the expectation if create failed
			r.expectation.LowerExpectations(swarmKey, 1, 0)
			return false, fmt.Errorf("failed to create workload: %w", err)
		}

		logger.Info("collaset created", "collaset", generated.Name)
		workload.Object = generated
		return false, nil
	}
	// try to update

	// calculate revision hash

	cls := workload.Object
	changed, err := utils.UpdateOnConflict(ctx, r.Client, r.Client, cls, func() error {
		existingHash := lo.ValueOr(cls.Labels, history.ControllerRevisionHashLabel, "")
		generatedHash := lo.ValueOr(generated.Labels, history.ControllerRevisionHashLabel, "")
		if existingHash == generatedHash && len(generatedHash) > 0 {
			// if the revision hash is the same, we don't need to update the collaset
			return nil
		}
		// hash changed, we need to update the collaset
		cls.Labels = lo.Assign(cls.Labels, generated.Labels)
		// cls.Labels = controllerutils.MergeMaps(cls.Labels, generated.Labels)
		cls.Annotations = lo.Assign(cls.Annotations, generated.Annotations)
		cls.Spec.Replicas = generated.Spec.Replicas
		cls.Spec.Template = generated.Spec.Template
		cls.Spec.VolumeClaimTemplates = generated.Spec.VolumeClaimTemplates
		return nil
	})
	if err != nil {
		return false, err
	}
	if changed {
		logger.Info("update collaset", "collaset", cls.Name)
		workload.Object = cls
		r.workloadRVExpectation.ExpectUpdate(utils.ObjectKeyString(cls), cls.GetResourceVersion()) //nolint
		return false, nil
	}

	return isCollaSetSynced(cls), nil
}

func isCollaSetSynced(cls *kusionstackappsv1alpha1.CollaSet) bool {
	if cls == nil {
		return false
	}
	return cls.DeletionTimestamp == nil &&
		cls.Generation == cls.Status.ObservedGeneration &&
		ptr.Deref(cls.Spec.Replicas, 0) == cls.Status.UpdatedAvailableReplicas
}

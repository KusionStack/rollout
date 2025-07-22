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

package rollout

import (
	"context"
	"encoding/json"
	goerrors "errors"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/go-logr/logr"
	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	errorsutil "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/validation"
	"kusionstack.io/kube-api/rollout"
	rolloutv1alpha1 "kusionstack.io/kube-api/rollout/v1alpha1"
	"kusionstack.io/kube-api/rollout/v1alpha1/condition"
	kubeutilclient "kusionstack.io/kube-utils/client"
	"kusionstack.io/kube-utils/controller/mixin"
	"kusionstack.io/kube-utils/multicluster"
	"kusionstack.io/kube-utils/multicluster/clusterinfo"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"kusionstack.io/rollout/pkg/controllers/registry"
	"kusionstack.io/rollout/pkg/features"
	"kusionstack.io/rollout/pkg/features/ontimestrategy"
	"kusionstack.io/rollout/pkg/features/rolloutclasspredicate"
	"kusionstack.io/rollout/pkg/utils"
	"kusionstack.io/rollout/pkg/utils/eventhandler"
	"kusionstack.io/rollout/pkg/utils/expectations"
	"kusionstack.io/rollout/pkg/workload"
)

const (
	ControllerName = "rollout"
)

const (
	reconcileOK rolloutv1alpha1.ConditionType = "reconcileOK"
)

// RolloutReconciler reconciles a Rollout object
type RolloutReconciler struct {
	*mixin.ReconcilerMixin

	workloadRegistry registry.WorkloadRegistry

	expectation   expectations.ControllerExpectationsInterface
	rvExpectation expectations.ResourceVersionExpectationInterface
}

func NewReconciler(mgr manager.Manager, workloadRegistry registry.WorkloadRegistry) *RolloutReconciler {
	return &RolloutReconciler{
		ReconcilerMixin:  mixin.NewReconcilerMixin(ControllerName, mgr),
		expectation:      expectations.NewControllerExpectations(),
		rvExpectation:    expectations.NewResourceVersionExpectation(),
		workloadRegistry: workloadRegistry,
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *RolloutReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.workloadRegistry == nil {
		return fmt.Errorf("workload manager must be set")
	}

	b := ctrl.NewControllerManagedBy(mgr).
		For(&rolloutv1alpha1.Rollout{}, builder.WithPredicates(predicate.ResourceVersionChangedPredicate{})).
		Watches(
			multicluster.FedKind(&source.Kind{Type: &rolloutv1alpha1.RolloutRun{}}),
			eventhandler.EqueueRequestForOwnerWithCreationObserved(&rolloutv1alpha1.Rollout{}, true, r.expectation),
		).
		Watches(
			multicluster.FedKind(&source.Kind{Type: &rolloutv1alpha1.RolloutStrategy{}}),
			enqueueRolloutForStrategyHandler(r.Client, r.Logger),
		)

	allworkloads := GetWatchableWorkloads(r.workloadRegistry, r.Logger, r.Client, r.Config)
	for _, accessor := range allworkloads {
		gvk := accessor.GroupVersionKind()
		r.Logger.Info("add watcher for workload", "gvk", gvk.String())
		b.Watches(
			multicluster.ClustersKind(&source.Kind{Type: accessor.NewObject()}),
			enqueueRolloutForWorkloadHandler(r.Client, r.Scheme, r.Logger),
		)
	}

	return b.Complete(r)
}

//+kubebuilder:rbac:groups=rollout.kusionstack.io,resources=rollouts,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=rollout.kusionstack.io,resources=rollouts/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=rollout.kusionstack.io,resources=rollouts/finalizers,verbs=update

//+kubebuilder:rbac:groups=rollout.kusionstack.io,resources=rolloutstrategies,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *RolloutReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	key := req.String()
	logger := r.Logger.WithValues("rollout", key)
	ctx = logr.NewContext(ctx, logger)
	logger.V(4).Info("started reconciling rollout")
	defer logger.V(4).Info("finished reconciling rollout")

	obj := &rolloutv1alpha1.Rollout{}
	err := r.Client.Get(clusterinfo.WithCluster(ctx, clusterinfo.Fed), req.NamespacedName, obj)
	if err != nil {
		if errors.IsNotFound(err) {
			r.expectation.DeleteExpectations(key)
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	// filter rollout event by rollout class
	//
	// NOTE: rollout can not use RolloutClassMatchesPredicate because it will be trigger
	//       by other objects(rolloutRun, rolloutStrategy and workloads)
	if features.DefaultFeatureGate.Enabled(features.RolloutClassPredicate) {
		if !rolloutclasspredicate.IsRolloutClassMatches(obj) {
			logger.V(4).Info("skipped, rollout class does not matched")
			return reconcile.Result{}, nil
		}
	}

	// 1. check if expectations is satisfied
	if !r.satisfiedExpectations(ctx, obj) {
		return reconcile.Result{}, nil
	}

	// 2. calculate new status
	newStatus := r.calculateStatus(obj)

	// 3. add or delete finalizer if necessary
	err = r.ensureFinalizer(ctx, obj, newStatus)
	if err != nil {
		return reconcile.Result{}, err
	}

	// 4. get all workloads references
	_, workloads, err := r.findWorkloadsCrossCluster(ctx, obj)
	if err != nil {
		return reconcile.Result{}, err
	}

	// 5. get all rolloutRun
	curRun, oldRuns, err := r.getAllRolloutRun(ctx, obj)
	if err != nil {
		return reconcile.Result{}, err
	}

	// 6. clean up old rolloutRun
	err = r.cleanupHistory(ctx, obj, oldRuns)
	if err != nil {
		return reconcile.Result{}, err
	}

	// 7. do syncing
	switch newStatus.Phase {
	case rolloutv1alpha1.RolloutPhaseDisabled:
		// sync status only
	case rolloutv1alpha1.RolloutPhaseTerminating:
		// finalize rollout
		err = r.handleFinalizing(ctx, obj, workloads, newStatus)
	default:
		// waiting or progressing
		err = r.handleProgressing(ctx, obj, curRun, workloads, newStatus)
	}

	// 8. update status firstly
	updateStatusError := r.updateStatusOnly(ctx, obj, newStatus)
	if updateStatusError != nil {
		return reconcile.Result{}, updateStatusError
	}

	return reconcile.Result{}, err
}

func (r *RolloutReconciler) satisfiedExpectations(ctx context.Context, obj *rolloutv1alpha1.Rollout) bool {
	key := utils.ObjectKeyString(obj)
	logger := logr.FromContextOrDiscard(ctx)

	if !r.rvExpectation.SatisfiedExpectations(key, obj.ResourceVersion) {
		logger.Info("rollout does not statisfy resourceVersion expectation, skip reconciling")
		return false
	}

	if !r.expectation.SatisfiedExpectations(key) {
		logger.Info("rollout does not statisfy controller expectation, skip reconciling")
		return false
	}
	return true
}

func (r *RolloutReconciler) ensureFinalizer(ctx context.Context, obj *rolloutv1alpha1.Rollout, newStatus *rolloutv1alpha1.RolloutStatus) error {
	logger := logr.FromContextOrDiscard(ctx)
	if obj.DeletionTimestamp == nil {
		// ensure finalizer
		if err := kubeutilclient.AddFinalizerAndUpdate(ctx, r.Client, obj, rollout.FinalizerRolloutProtection); err != nil {
			r.recordCondition(obj, newStatus, reconcileOK, metav1.ConditionFalse, "FailedAddFinalizer", fmt.Sprintf("failed to add finalizer %s, err: %v", rollout.FinalizerRolloutProtection, err))
			return err
		}
		return nil
	}

	if condition.IsTerminationCompleted(obj.Status.Conditions) {
		// remove finalizer
		if err := kubeutilclient.RemoveFinalizerAndUpdate(ctx, r.Client, obj, rollout.FinalizerRolloutProtection); err != nil {
			r.recordCondition(obj, newStatus, reconcileOK, metav1.ConditionFalse, "FailedRemoveFinalizer", fmt.Sprintf("failed to remove finalizer %s, err: %v", rollout.FinalizerRolloutProtection, err))
			return err
		}
		logger.Info("clean up finalizer in rollout", "finalizer", rollout.FinalizerRolloutProtection)
	}

	return nil
}

func (r *RolloutReconciler) calculateStatus(obj *rolloutv1alpha1.Rollout) *rolloutv1alpha1.RolloutStatus {
	newStatus := obj.Status.DeepCopy()
	newStatus.ObservedGeneration = obj.Generation

	if obj.DeletionTimestamp != nil {
		if newStatus.Phase != rolloutv1alpha1.RolloutPhaseTerminating {
			newStatus.Phase = rolloutv1alpha1.RolloutPhaseTerminating
			r.recordCondition(obj, newStatus, rolloutv1alpha1.RolloutConditionTerminating, metav1.ConditionTrue, "Terminating", "starting to finalize rollout")
		}
		return newStatus
	}

	if obj.Spec.Disabled {
		newStatus.Phase = rolloutv1alpha1.RolloutPhaseDisabled
		return newStatus
	}

	if newStatus.Phase != rolloutv1alpha1.RolloutPhaseProgressing {
		// change phase to initialized if rollout is not running
		newStatus.Phase = rolloutv1alpha1.RolloutPhaseInitialized
	}

	return newStatus
}

func (r *RolloutReconciler) handleFinalizing(ctx context.Context, obj *rolloutv1alpha1.Rollout, workloads []*workload.Info, newStatus *rolloutv1alpha1.RolloutStatus) error {
	if obj.DeletionTimestamp == nil || condition.IsTerminationCompleted(obj.Status.Conditions) {
		return nil
	}

	// delete workloads label
	errs := []error{}
	for _, info := range workloads {
		_, err := info.UpdateOnConflict(ctx, r.Client, func(obj client.Object) error {
			utils.MutateLabels(obj, func(labels map[string]string) {
				delete(labels, rollout.LabelWorkload)
				delete(labels, rollout.LabelControlledBy) // for backward compatibility
			})
			utils.MutateAnnotations(obj, func(annotations map[string]string) {
				delete(annotations, rollout.AnnoRolloutName)
			})
			return nil
		})
		if err != nil {
			errs = append(errs, err)
			continue
		}
	}

	if len(errs) > 0 {
		err := errorsutil.NewAggregate(errs)
		r.recordCondition(obj, newStatus, reconcileOK, metav1.ConditionFalse, "FailedUpdateWorkload", fmt.Sprintf("failed to delete rollout label on workloads, err: %v", err))
		return err
	}

	// TODO: wait for all rolloutRun to be deleted

	r.recordCondition(obj, newStatus, rolloutv1alpha1.RolloutConditionTerminating, metav1.ConditionTrue, rolloutv1alpha1.RolloutReasonTerminatingCompleted, "finalize rollout successfully")
	return nil
}

func (r *RolloutReconciler) handleProgressing(ctx context.Context, obj *rolloutv1alpha1.Rollout, curRun *rolloutv1alpha1.RolloutRun, workloads []*workload.Info, newStatus *rolloutv1alpha1.RolloutStatus) error {
	// 1. pre-check if dependent resources exist
	ros, _, errs := r.getDependentResources(ctx, obj)
	if len(errs) > 0 {
		err := errorsutil.NewAggregate(errs)
		// record in status
		r.recordCondition(obj, newStatus, rolloutv1alpha1.RolloutConditionAvailable, metav1.ConditionFalse, "DependencyFailure", err.Error())
	} else {
		// valid rollout
		r.recordCondition(obj, newStatus, rolloutv1alpha1.RolloutConditionAvailable, metav1.ConditionTrue, "Available", "all dependencies are valid")

		// 2. add rollout label to workloads if rollout is available
		err := r.ensureWorkloadsLabelAndAnnotations(ctx, obj.Name, workloads)
		if err != nil {
			r.recordCondition(obj, newStatus, reconcileOK, metav1.ConditionFalse, "FailedUpdateWorkload", err.Error())
			return err
		}
	}

	return r.syncRun(ctx, obj, curRun, ros, workloads, newStatus)
}

func (r *RolloutReconciler) getDependentResources(ctx context.Context, obj *rolloutv1alpha1.Rollout) (ros *rolloutv1alpha1.RolloutStrategy, ttopos []*rolloutv1alpha1.TrafficTopology, errs []error) {
	ctx = clusterinfo.WithCluster(ctx, clusterinfo.Fed)
	var strategy rolloutv1alpha1.RolloutStrategy
	err := r.Client.Get(
		ctx,
		client.ObjectKey{Namespace: obj.Namespace, Name: obj.Spec.StrategyRef},
		&strategy,
	)
	if err != nil {
		errs = append(errs, err)
	} else {
		ros = &strategy
	}

	for _, tt := range obj.Spec.TrafficTopologyRefs {
		var topology rolloutv1alpha1.TrafficTopology
		err := r.Client.Get(
			ctx,
			client.ObjectKey{Namespace: obj.Namespace, Name: tt},
			&topology,
		)
		if err != nil {
			errs = append(errs, err)
		} else {
			ttopos = append(ttopos, &topology)
		}
	}
	return ros, ttopos, errs
}

func (r *RolloutReconciler) ensureWorkloadsLabelAndAnnotations(ctx context.Context, name string, workloads []*workload.Info) error {
	// TODO: we need to clean up label on workload if rollout does not select it any more.
	errs := []error{}
	for _, info := range workloads {
		kind := strings.ToLower(info.Kind)
		_, err := info.UpdateOnConflict(ctx, r.Client, func(obj client.Object) error {
			utils.MutateLabels(obj, func(labels map[string]string) {
				labels[rollout.LabelWorkload] = kind
			})
			utils.MutateAnnotations(obj, func(annotations map[string]string) {
				// The rollout name may be too long and exceed the max length (63) of label value
				annotations[rollout.AnnoRolloutName] = name
			})
			return nil
		})
		if err != nil {
			errs = append(errs, err)
			continue
		}
	}

	return errorsutil.NewAggregate(errs)
}

func (r *RolloutReconciler) syncRun(
	ctx context.Context,
	obj *rolloutv1alpha1.Rollout,
	curRun *rolloutv1alpha1.RolloutRun,
	ros *rolloutv1alpha1.RolloutStrategy,
	workloads []*workload.Info,
	newStatus *rolloutv1alpha1.RolloutStatus,
) error {
	key := utils.ObjectKeyString(obj)

	// defer function to cleanup annotations on after process ends
	defer func() {
		if tempErr := r.cleanupAnnotation(ctx, obj); tempErr != nil {
			r.recordCondition(obj, newStatus, reconcileOK, metav1.ConditionFalse, "FailedUpdated", fmt.Sprintf("failed to cleanup annotations: %v", tempErr))
		}
	}()

	var curRunName string
	if curRun != nil {
		curRunName = curRun.Name
	}

	// 1. check if rolloutRun is still running
	if curRun != nil && !curRun.IsCompleted() {
		setStatusPhase(newStatus, curRunName, rolloutv1alpha1.RolloutPhaseProgressing)
		r.recordCondition(obj, newStatus, rolloutv1alpha1.RolloutConditionProgressing, metav1.ConditionTrue, rolloutv1alpha1.RolloutReasonProgressingRunning, "rolloutRun is running")

		// do manual command
		err := r.handleRunManualCommand(ctx, obj, curRun)
		if err != nil {
			r.recordCondition(obj, newStatus, reconcileOK, metav1.ConditionFalse, "FailedUpdated", fmt.Sprintf("failed to process manual command: %v", err))
			return err
		}
		// apply one time strategy
		if features.DefaultFeatureGate.Enabled(features.OneTimeStrategy) {
			err := r.applyOneTimeStrategy(obj, curRun, workloads, newStatus)
			if err != nil {
				return err
			}
		}
		return nil
	}

	// 2. rolloutRun is completed or not found, reset phase and condition
	setStatusPhase(newStatus, curRunName, rolloutv1alpha1.RolloutPhaseInitialized)
	r.recordCondition(obj, newStatus, rolloutv1alpha1.RolloutConditionProgressing, metav1.ConditionFalse, "", "")

	// 3. check if trigger is satisfied
	rolloutID, triggered := r.shouldTrigger(obj, workloads, newStatus)
	if !triggered {
		return nil
	}

	// 4. trigger a new rollout progress
	curRun = constructRolloutRun(obj, ros, workloads, rolloutID)
	r.recordCondition(obj, newStatus, rolloutv1alpha1.RolloutConditionTrigger, metav1.ConditionTrue, "Create", fmt.Sprintf("construct a new rolloutRun %s", curRun.Name))

	// NOTO: we have to set expectation before we create the rolloutRun to avoid
	//       that the creation event comes so fast that we don't have time to set it
	r.expectation.ExpectCreations(key, 1) // nolint

	if err := r.Client.Create(clusterinfo.ContextFed, curRun); err != nil {
		r.expectation.DeleteExpectations(key)
		// do not change status phase here if rolloutRun is not created
		r.recordCondition(obj, newStatus, rolloutv1alpha1.RolloutConditionTrigger, metav1.ConditionFalse, "FailedCreate", fmt.Sprintf("failed to create a new rolloutRun %s: %v", curRun.Name, err))
		return err
	}

	// update status
	setStatusPhase(newStatus, curRun.Name, rolloutv1alpha1.RolloutPhaseProgressing)
	r.recordCondition(obj, newStatus, rolloutv1alpha1.RolloutConditionTrigger, metav1.ConditionTrue, "SucceedCreate", fmt.Sprintf("rolloutRun %s is created", curRun.Name))
	r.recordCondition(obj, newStatus, rolloutv1alpha1.RolloutConditionProgressing, metav1.ConditionTrue, "Triggered", "")

	return nil
}

func (r *RolloutReconciler) shouldTrigger(obj *rolloutv1alpha1.Rollout, workloads []*workload.Info, newStatus *rolloutv1alpha1.RolloutStatus) (string, bool) {
	if !condition.IsAvailable(newStatus.Conditions) {
		return "", false
	}

	rolloutID := generateRolloutID(obj.Name)

	triggerName, ok := utils.GetMapValue(obj.Annotations, rollout.AnnoRolloutTrigger)
	if ok {
		if len(validation.IsQualifiedName(triggerName)) == 0 {
			// use user defined trigger name as rolloutID
			rolloutID = triggerName
		}
		return rolloutID, true
	}

	if obj.Spec.TriggerPolicy == rolloutv1alpha1.ManualTriggerPolicy {
		return "", false
	}

	total := len(workloads)

	if total == 0 {
		return "", false
	}

	triggered := 0
	pendings := []string{}

	// TODO: we neet to find a new way to check if the workload is waiting for rollout
	// .     otherwise we can not cancel the cannary release.
	for _, info := range workloads {
		if workload.IsWaitingRollout(*info) {
			triggered++
		} else {
			pendings = append(pendings, info.String())
		}
	}

	r.Logger.V(2).Info("check if rollout should be triggered",
		"rollout", obj.Name,
		"total", total,
		"triggered", triggered,
		"waitfor", strings.Join(pendings, " | "),
	)

	if total == triggered {
		return rolloutID, true
	}
	return "", false
}

func (r *RolloutReconciler) findWorkloadsCrossCluster(ctx context.Context, obj *rolloutv1alpha1.Rollout) (workload.Accessor, []*workload.Info, error) {
	gvk := schema.FromAPIVersionAndKind(obj.Spec.WorkloadRef.APIVersion, obj.Spec.WorkloadRef.Kind)
	rest, err := r.workloadRegistry.Get(gvk)
	if err != nil {
		return nil, nil, err
	}
	workloads, _, err := workload.List(ctx, r.Client, rest, obj.GetNamespace(), obj.Spec.WorkloadRef.Match)
	if err != nil {
		return nil, nil, err
	}
	return rest, workloads, nil
}

func (r *RolloutReconciler) getAllRolloutRun(ctx context.Context, obj *rolloutv1alpha1.Rollout) (*rolloutv1alpha1.RolloutRun, []*rolloutv1alpha1.RolloutRun, error) {
	wList := &rolloutv1alpha1.RolloutRunList{}
	err := r.Client.List(clusterinfo.WithCluster(ctx, clusterinfo.Fed), wList, client.InNamespace(obj.Namespace))
	if err != nil {
		return nil, nil, err
	}

	oldRuns := []*rolloutv1alpha1.RolloutRun{}
	var curRun *rolloutv1alpha1.RolloutRun

	for i := range wList.Items {
		ror := &wList.Items[i]
		owner := metav1.GetControllerOf(ror)
		if owner == nil {
			continue
		}
		if owner.Kind != "Rollout" || owner.Name != obj.Name {
			continue
		}

		if len(obj.Status.RolloutID) == 0 && curRun == nil && !ror.IsCompleted() {
			// use the first active rolloutRun as current if rolloutID is empty
			curRun = ror
			continue
		} else if ror.Name == obj.Status.RolloutID {
			curRun = ror
			continue
		}
		oldRuns = append(oldRuns, ror)
	}

	return curRun, oldRuns, nil
}

func (r *RolloutReconciler) cleanupHistory(ctx context.Context, obj *rolloutv1alpha1.Rollout, oldRuns []*rolloutv1alpha1.RolloutRun) error {
	// The HistoryLimit can start from 0 (no retained replicasSet). When set to math.MaxInt32,
	// the Deployment will keep all revisions.
	if obj.Spec.HistoryLimit == nil || *obj.Spec.HistoryLimit == math.MaxInt32 {
		// no limit
		return nil
	}

	// filter completed and alive runs
	completedRun := lo.Filter(oldRuns, func(r *rolloutv1alpha1.RolloutRun, _ int) bool {
		return r.IsCompleted() && r.ObjectMeta.DeletionTimestamp == nil
	})

	diff := int32(len(completedRun)) - *obj.Spec.HistoryLimit

	if diff <= 0 {
		return nil
	}

	sort.Sort(RolloutRunByCreationTimestamp(completedRun))

	r.Logger.V(1).Info("start to cleanup old rolloutRun", "rollout", obj.Name, "count", diff)

	for i := range diff {
		run := completedRun[i]
		// rolloutRun controller will delete protection finalizer if rollouRun is completed
		if err := r.Client.Delete(clusterinfo.WithCluster(ctx, clusterinfo.Fed), run); err != nil {
			return err
		}
		r.Logger.V(2).Info("succeed to delete old rolloutRun", "rolloutRun", run.Name)
	}
	return nil
}

func (r *RolloutReconciler) updateStatusOnly(ctx context.Context, obj *rolloutv1alpha1.Rollout, newStatus *rolloutv1alpha1.RolloutStatus) error {
	if equality.Semantic.DeepEqual(obj.Status, *newStatus) {
		// no change
		return nil
	}
	key := utils.ObjectKeyString(obj)
	now := metav1.Now()
	newStatus.LastUpdateTime = &now
	_, err := utils.UpdateOnConflict(clusterinfo.WithCluster(ctx, clusterinfo.Fed), r.Client, r.Client.Status(), obj, func() error {
		obj.Status = *newStatus
		return nil
	})
	if err != nil {
		r.recordCondition(obj, newStatus, reconcileOK, metav1.ConditionFalse, "FailedUpdateStatus", fmt.Sprintf("failed to update rollout status: %v", err))
		return err
	}

	r.rvExpectation.ExpectUpdate(key, obj.ResourceVersion) // nolint
	return nil
}

func (r *RolloutReconciler) handleRunManualCommand(ctx context.Context, obj *rolloutv1alpha1.Rollout, run *rolloutv1alpha1.RolloutRun) error {
	if run == nil || run.IsCompleted() {
		return nil
	}
	command, ok := utils.GetMapValue(obj.Annotations, rollout.AnnoManualCommandKey)
	if !ok {
		return nil
	}

	// update manual command to rollout run
	_, err := utils.UpdateOnConflict(clusterinfo.WithCluster(ctx, clusterinfo.Fed), r.Client, r.Client, run, func() error {
		if run.Annotations == nil {
			run.Annotations = make(map[string]string)
		}
		run.Annotations[rollout.AnnoManualCommandKey] = command
		return nil
	})
	return err
}

func (r *RolloutReconciler) cleanupAnnotation(ctx context.Context, obj *rolloutv1alpha1.Rollout) error {
	// delete manual command annotations from rollout
	_, err := utils.UpdateOnConflict(clusterinfo.WithCluster(ctx, clusterinfo.Fed), r.Client, r.Client, obj, func() error {
		delete(obj.Annotations, rollout.AnnoManualCommandKey)
		delete(obj.Annotations, rollout.AnnoRolloutTrigger)
		if features.DefaultFeatureGate.Enabled(features.OneTimeStrategy) {
			delete(obj.Annotations, ontimestrategy.AnnoOneTimeStrategy)
		}
		return nil
	})
	if err != nil {
		return err
	}
	key := utils.ObjectKeyString(obj)
	r.rvExpectation.ExpectUpdate(key, obj.ResourceVersion) // nolint
	return nil
}

func (r *RolloutReconciler) applyOneTimeStrategy(obj *rolloutv1alpha1.Rollout, run *rolloutv1alpha1.RolloutRun, workloads []*workload.Info, newStatus *rolloutv1alpha1.RolloutStatus) error {
	if run == nil || run.IsCompleted() {
		return nil
	}
	// run is progressing
	strategyStr, ok := utils.GetMapValue(obj.Annotations, ontimestrategy.AnnoOneTimeStrategy)
	if !ok {
		return nil
	}

	strategy := ontimestrategy.OneTimeStrategy{}
	err := json.Unmarshal([]byte(strategyStr), &strategy)
	if err != nil {
		r.recordCondition(obj, newStatus, rolloutv1alpha1.RolloutConditionTrigger, metav1.ConditionFalse, "InvalidStrategy", fmt.Sprintf("failed to unmarshal one time strategy: %v", err))
		// do not block
		return nil
	}

	batch := rolloutv1alpha1.RolloutRunBatchStrategy{
		Batches:    constructRolloutRunBatches(&strategy.Batch, workloads),
		Toleration: strategy.Batch.Toleration,
	}

	if batch.Toleration == nil {
		batch.Toleration = run.Spec.Batch.Toleration
	}

	if equality.Semantic.DeepEqual(batch, run.Spec.Batch) {
		// nothing changed
		r.recordCondition(obj, newStatus, rolloutv1alpha1.RolloutConditionTrigger, metav1.ConditionTrue, "OneTimeStrategyIgnored", "batch strategy is not changed")
		return nil
	}

	// update rolloutRun
	_, err = utils.UpdateOnConflict(clusterinfo.ContextFed, r.Client, r.Client, run, func() error {
		if run.Annotations == nil {
			run.Annotations = make(map[string]string)
		}
		// update strategy in annotation
		run.Annotations[ontimestrategy.AnnoOneTimeStrategy] = strategyStr
		// update batch in spec
		run.Spec.Batch = &batch
		return nil
	})
	if err != nil {
		r.recordCondition(obj, newStatus, rolloutv1alpha1.RolloutConditionTrigger, metav1.ConditionFalse, "FailedUpdate", fmt.Sprintf("failed to apply one time stratey to RolloutRun: %v", err))
		// allow retry
		return err
	}

	r.recordCondition(obj, newStatus, rolloutv1alpha1.RolloutConditionTrigger, metav1.ConditionTrue, "ApplyOneTimeStrategy", "succeed to apply one time strategy")
	return nil
}

func (r *RolloutReconciler) recordCondition(obj *rolloutv1alpha1.Rollout, newStatus *rolloutv1alpha1.RolloutStatus, ctype rolloutv1alpha1.ConditionType, expectedStatus metav1.ConditionStatus, reason, message string) {
	logger := r.Logger.WithValues("rollout", utils.ObjectKeyString(obj))

	original := condition.GetCondition(newStatus.Conditions, ctype)
	eventtype := corev1.EventTypeNormal
	if expectedStatus != metav1.ConditionTrue {
		eventtype = corev1.EventTypeWarning
	}

	switch ctype {
	case reconcileOK:
		// NOTE: this is a special condition, it is used to print log and event
		if expectedStatus != metav1.ConditionTrue {
			logger.Error(goerrors.New("reconcile error"), "", "reason", reason, "message", message)
			r.Recorder.Eventf(obj, eventtype, reason, message)
		}
		// stop at here
		return
	case rolloutv1alpha1.RolloutConditionAvailable:
		if original == nil || original.Status != expectedStatus {
			r.Recorder.Eventf(obj, eventtype, reason, message)
			if expectedStatus == metav1.ConditionTrue {
				logger.Info("rollout condition changed", "condition", ctype, "status", expectedStatus, "reason", reason, "message", message)
			} else {
				logger.Error(goerrors.New("condition error"), "rollout condition changed", "condition", ctype, "status", expectedStatus, "reason", reason, "message", message)
			}
		}
	case rolloutv1alpha1.RolloutConditionTrigger,
		rolloutv1alpha1.RolloutConditionTerminating:
		r.Recorder.Eventf(obj, eventtype, reason, message)
		if expectedStatus == metav1.ConditionTrue {
			logger.Info("rollout condition changed", "condition", ctype, "status", expectedStatus, "reason", reason, "message", message)
		} else {
			logger.Error(goerrors.New("condition error"), "rollout condition changed", "condition", ctype, "status", expectedStatus, "reason", reason, "message", message)
		}
	}
	cond := condition.NewCondition(ctype, expectedStatus, reason, message)
	newStatus.Conditions = condition.SetCondition(newStatus.Conditions, *cond)
}

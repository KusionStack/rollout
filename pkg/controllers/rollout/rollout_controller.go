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
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	errorsutil "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/client-go/discovery"
	memory "k8s.io/client-go/discovery/cached"
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

	"kusionstack.io/rollout/apis/rollout"
	rolloutv1alpha1 "kusionstack.io/rollout/apis/rollout/v1alpha1"
	"kusionstack.io/rollout/apis/rollout/v1alpha1/condition"
	"kusionstack.io/rollout/pkg/features"
	"kusionstack.io/rollout/pkg/features/ontimestrategy"
	"kusionstack.io/rollout/pkg/utils"
	"kusionstack.io/rollout/pkg/utils/eventhandler"
	"kusionstack.io/rollout/pkg/utils/expectations"
	"kusionstack.io/rollout/pkg/workload"
)

const (
	ControllerName = "rollout"
)

// RolloutReconciler reconciles a Rollout object
type RolloutReconciler struct {
	*mixin.ReconcilerMixin

	workloadRegistry workload.Registry

	expectation   expectations.ControllerExpectationsInterface
	rvExpectation expectations.ResourceVersionExpectationInterface

	supportedGVK sets.String
}

func NewReconciler(mgr manager.Manager, workloadRegistry workload.Registry) *RolloutReconciler {
	return &RolloutReconciler{
		ReconcilerMixin:  mixin.NewReconcilerMixin(ControllerName, mgr),
		expectation:      expectations.NewControllerExpectations(),
		rvExpectation:    expectations.NewResourceVersionExpectation(),
		workloadRegistry: workloadRegistry,
		supportedGVK:     sets.NewString(),
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

	var discoveryClient multicluster.PartialCachedDiscoveryInterface
	client, ok := r.Client.(multicluster.MultiClusterDiscovery)
	if ok {
		discoveryClient = client.MembersCachedDiscoveryInterface()
	} else {
		discoveryClient = memory.NewMemCacheClient(discovery.NewDiscoveryClientForConfigOrDie(mgr.GetConfig()))
	}

	allworkloads := getWatchableWorkloads(r.workloadRegistry, r.Logger, discoveryClient)
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

	logger.V(2).Info("start reconciling rollout")
	defer logger.V(2).Info("finish reconciling rollout")

	instance := &rolloutv1alpha1.Rollout{}
	err := r.Client.Get(clusterinfo.WithCluster(ctx, clusterinfo.Fed), req.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			r.expectation.DeleteExpectations(key)
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	if !r.satisfiedExpectations(instance) {
		return reconcile.Result{}, nil
	}

	// add or delete finalizer if necessary
	err = r.ensureFinalizer(instance)
	if err != nil {
		return reconcile.Result{}, err
	}

	// calculate new status
	newStatus := r.calculateStatus(instance)

	// get all workloads references
	_, workloads, err := r.findWorkloadsCrossCluster(ctx, instance)
	if err != nil {
		return reconcile.Result{}, err
	}

	// do syncing
	switch newStatus.Phase {
	case rolloutv1alpha1.RolloutPhaseDisabled:
		// sync status only
	case rolloutv1alpha1.RolloutPhaseTerminating:
		// finalize rollout
		err = r.handleFinalizing(ctx, instance, workloads, newStatus)
	default:
		// waiting or progressing
		err = r.handleProgressing(ctx, instance, workloads, newStatus)
	}

	if tempErr := r.cleanupAnnotation(ctx, instance); tempErr != nil {
		logger.Error(tempErr, "failed to clean up annotation")
	}

	// update status firstly
	updateStatusError := r.updateStatusOnly(ctx, instance, newStatus)
	if updateStatusError != nil {
		logger.Error(err, "failed to update status")
		return reconcile.Result{}, updateStatusError
	}

	if err != nil {
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

func (r *RolloutReconciler) satisfiedExpectations(instance *rolloutv1alpha1.Rollout) bool {
	key := utils.ObjectKeyString(instance)
	logger := r.Logger.WithValues("rollout", key)

	if !r.rvExpectation.SatisfiedExpectations(key, instance.ResourceVersion) {
		logger.Info("rollout does not statisfy resourceVersion expectation, skip reconciling")
		return false
	}

	if !r.expectation.SatisfiedExpectations(key) {
		logger.Info("rollout does not statisfy controller expectation, skip reconciling")
		return false
	}
	return true
}

func (r *RolloutReconciler) ensureFinalizer(instance *rolloutv1alpha1.Rollout) error {
	logger := r.Logger.WithValues("rollout", utils.ObjectKeyString(instance))
	if instance.DeletionTimestamp == nil {
		// ensure finalizer
		if err := utils.AddAndUpdateFinalizer(r.Client, instance, rollout.FinalizerRolloutProtection); err != nil {
			r.Recorder.Eventf(instance, corev1.EventTypeWarning, "FailedUpdateFinalizer", "failed to add finalizer %s, err: %v", rollout.FinalizerRolloutProtection, err)
			logger.Error(err, "failed to add finalizer", "finalizer", rollout.FinalizerRolloutProtection)
			return err
		}
		return nil
	}

	if condition.IsTerminationCompleted(instance.Status.Conditions) {
		// remove finalizer
		if err := utils.RemoveAndUpdateFinalizer(r.Client, instance, rollout.FinalizerRolloutProtection); err != nil {
			r.Recorder.Eventf(instance, corev1.EventTypeWarning, "FailedUpdateFinalizer", "failed to remove finalizer %s, err: %v", rollout.FinalizerRolloutProtection, err)
			logger.Error(err, "failed to remove finalizer", "finalizer", rollout.FinalizerRolloutProtection)
			return err
		}
		logger.Info("clean up finalizer in rollout", "finalizer", rollout.FinalizerRolloutProtection)
	}

	return nil
}

func (r *RolloutReconciler) calculateStatus(instance *rolloutv1alpha1.Rollout) *rolloutv1alpha1.RolloutStatus {
	newStatus := instance.Status.DeepCopy()
	newStatus.ObservedGeneration = instance.Generation

	if instance.DeletionTimestamp != nil {
		if newStatus.Phase != rolloutv1alpha1.RolloutPhaseTerminating {
			newStatus.Phase = rolloutv1alpha1.RolloutPhaseTerminating
			terminatingCond := condition.NewCondition(rolloutv1alpha1.RolloutConditionTerminating, metav1.ConditionTrue, "Terminating", "rollout is deleted")
			newStatus.Conditions = condition.SetCondition(newStatus.Conditions, *terminatingCond)
		}
		return newStatus
	}

	if instance.Spec.Disabled {
		newStatus.Phase = rolloutv1alpha1.RolloutPhaseDisabled
		return newStatus
	}

	if newStatus.Phase != rolloutv1alpha1.RolloutPhaseProgressing {
		// change phase to initialized if rollout is not running
		newStatus.Phase = rolloutv1alpha1.RolloutPhaseInitialized
	}

	return newStatus
}

func (r *RolloutReconciler) handleFinalizing(ctx context.Context, instance *rolloutv1alpha1.Rollout, workloads []*workload.Info, newStatus *rolloutv1alpha1.RolloutStatus) error {
	if instance.DeletionTimestamp == nil {
		return nil
	}

	// check if terminating is completed
	if condition.IsTerminationCompleted(instance.Status.Conditions) {
		// finalize completed
		return nil
	}

	logger := r.Logger.WithValues("rollout", utils.ObjectKeyString(instance))
	// TODO: do traffic finalizing here

	// delete workloads label
	errs := []error{}
	for _, info := range workloads {
		_, err := info.UpdateOnConflict(ctx, r.Client, func(obj client.Object) error {
			utils.MutateLabels(obj, func(labels map[string]string) {
				delete(labels, rollout.LabelWorkload)
			})
			return nil
		})
		if err != nil {
			errs = append(errs, err)
			continue
		}
	}

	if len(errs) > 0 {
		return errorsutil.NewAggregate(errs)
	}

	setStatusCondition(newStatus, rolloutv1alpha1.RolloutConditionTerminating, metav1.ConditionTrue, rolloutv1alpha1.RolloutReasonTerminatingCompleted, "rollout is finalized")
	logger.Info("finalize rollout successfully")
	return nil
}

func (r *RolloutReconciler) handleProgressing(ctx context.Context, obj *rolloutv1alpha1.Rollout, workloads []*workload.Info, newStatus *rolloutv1alpha1.RolloutStatus) error {
	key := utils.ObjectKeyString(obj)
	logger := r.Logger.WithValues("rollout", key)

	// add labels to workloads
	err := r.ensureWorkloadsLabels(ctx, workloads)
	if err != nil {
		r.Recorder.Eventf(obj, corev1.EventTypeWarning, "FailedUpdateWorkload", "failed to ensure rollout label on workloads, err = %v", err)
		logger.Error(err, "failed to add labels into workloads")
		return err
	}

	run, err := r.getCurrentRolloutRun(ctx, obj)
	if err != nil {
		logger.Error(err, "failed to get current rolloutRun of rollout")
		return err
	}

	if run != nil && !run.IsCompleted() {
		// NOTE: rollout will not sync strategy modification to running rolloutRun
		return r.syncRun(ctx, obj, run, workloads, newStatus)
	}

	// TODO: filter out-of-control workloads
	// check trigger satisfied
	rolloutID, needTrigger := r.needTrigger(obj, workloads)
	if !needTrigger {
		// sync status with current
		return r.syncRun(ctx, obj, run, workloads, newStatus)
	}

	// trigger a new rollout progress
	strategy, err := r.parseUpgradeStrategy(obj)
	if err != nil {
		return err
	}

	logger.Info("rollout has been triggered, about to construct rolloutRun", "rolloutRun", rolloutID)

	run = constructRolloutRun(obj, strategy, workloads, rolloutID)

	// NOTO: we have to set expectation before we create the rolloutRun to avoid
	//       that the creation event comes so fast that we don't have time to set it
	r.expectation.ExpectCreations(key, 1) // nolint

	if err = r.Client.Create(clusterinfo.ContextFed, run); err != nil {
		r.expectation.DeleteExpectations(key)

		r.Recorder.Eventf(obj, corev1.EventTypeWarning, "FailedCreate", "failed to create a new rolloutRun %s: %v", run.Name, err)
		logger.Error(err, "failed to create rolloutRun", "rolloutRun", run.Name)
		resetRolloutStatus(newStatus, run.Name, rolloutv1alpha1.RolloutPhaseProgressing)
		setStatusCondition(newStatus, rolloutv1alpha1.RolloutConditionProgressing, metav1.ConditionFalse, "FailedCreate", fmt.Sprintf("failed to create a new rolloutRun %s", run.Name))
		return err
	}

	r.Recorder.Eventf(obj, corev1.EventTypeNormal, "SucceedCreate", "create a new rolloutRun %s", run.Name)
	logger.Info("a new rolloutRun has been created", "rolloutRun", rolloutID)
	// update status
	resetRolloutStatus(newStatus, run.Name, rolloutv1alpha1.RolloutPhaseProgressing)
	setStatusCondition(newStatus, rolloutv1alpha1.RolloutConditionProgressing, metav1.ConditionTrue, "SucceedCreate", "a new rolloutRun is created")
	return nil
}

func (r *RolloutReconciler) ensureWorkloadsLabels(ctx context.Context, workloads []*workload.Info) error {
	errs := []error{}
	for _, info := range workloads {
		kind := strings.ToLower(info.Kind)
		_, err := info.UpdateOnConflict(ctx, r.Client, func(obj client.Object) error {
			utils.MutateLabels(obj, func(labels map[string]string) {
				labels[rollout.LabelWorkload] = kind
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

func (r *RolloutReconciler) needTrigger(instance *rolloutv1alpha1.Rollout, workloads []*workload.Info) (string, bool) {
	rolloutID := generateRolloutID(instance.Name)

	triggerName, ok := utils.GetMapValue(instance.Annotations, rollout.AnnoRolloutTrigger)
	if ok {
		if len(validation.IsQualifiedName(triggerName)) == 0 {
			// use user defined trigger name as rolloutID
			rolloutID = triggerName
		}
		return rolloutID, true
	}

	if instance.Spec.TriggerPolicy == rolloutv1alpha1.ManualTriggerPolicy {
		return "", false
	}

	count := len(workloads)

	if count == 0 {
		return "", false
	}

	waiting := 0
	pendings := []string{}

	for _, info := range workloads {
		if workload.IsWaitingRollout(*info) {
			waiting++
		} else {
			pendings = append(pendings, info.String())
		}
	}

	r.Logger.Info("check if rollout need to be triggered",
		"rollout", instance.Name,
		"count", count,
		"triggered", waiting,
		"pendings", strings.Join(pendings, " | "),
	)

	if count == waiting {
		return rolloutID, true
	}
	return "", false
}

func (r *RolloutReconciler) parseUpgradeStrategy(instance *rolloutv1alpha1.Rollout) (*rolloutv1alpha1.RolloutStrategy, error) {
	ctx := clusterinfo.WithCluster(context.TODO(), clusterinfo.Fed)
	strategyRef := instance.Spec.StrategyRef
	if strategyRef == "" {
		return nil, fmt.Errorf("empty strategyRef")
	}

	strategy := &rolloutv1alpha1.RolloutStrategy{}
	err := r.Client.Get(ctx, types.NamespacedName{
		Name:      strategyRef,
		Namespace: instance.GetNamespace(),
	}, strategy)
	if err != nil {
		return nil, err
	}

	if strategy.Batch == nil {
		return nil, fmt.Errorf("invalid BatchStrategy")
	}

	return strategy, nil
}

func (r *RolloutReconciler) findWorkloadsCrossCluster(ctx context.Context, obj *rolloutv1alpha1.Rollout) (workload.Accessor, []*workload.Info, error) {
	gvk := schema.FromAPIVersionAndKind(obj.Spec.WorkloadRef.APIVersion, obj.Spec.WorkloadRef.Kind)
	rest, err := r.workloadRegistry.Get(gvk)
	if err != nil {
		return nil, nil, err
	}

	workloads, err := workload.List(ctx, r.Client, rest, obj.GetNamespace(), obj.Spec.WorkloadRef.Match)
	if err != nil {
		return nil, nil, err
	}
	return rest, workloads, nil
}

func (r *RolloutReconciler) updateStatusOnly(ctx context.Context, instance *rolloutv1alpha1.Rollout, newStatus *rolloutv1alpha1.RolloutStatus) error {
	if equality.Semantic.DeepEqual(instance.Status, *newStatus) {
		// no change
		return nil
	}
	key := utils.ObjectKeyString(instance)
	now := metav1.Now()
	newStatus.LastUpdateTime = &now
	_, err := utils.UpdateOnConflict(clusterinfo.WithCluster(ctx, clusterinfo.Fed), r.Client, r.Client.Status(), instance, func() error {
		instance.Status = *newStatus
		return nil
	})
	if err != nil {
		r.Recorder.Eventf(instance, corev1.EventTypeWarning, "FailedUpdateStatus", "failed to update rollout %q status: %v", key, err)
		r.Logger.Error(err, "failed to update rollout status", "rollout", key)
		return err
	}

	r.Logger.V(2).Info("succeed to update rollout status", "rollout", key)
	r.rvExpectation.ExpectUpdate(key, instance.ResourceVersion) // nolint
	return nil
}

func (r *RolloutReconciler) getCurrentRolloutRun(ctx context.Context, instance *rolloutv1alpha1.Rollout) (*rolloutv1alpha1.RolloutRun, error) {
	logger := r.Logger.WithValues("rollout", utils.ObjectKeyString(instance))
	if instance.Status.RolloutID == "" {
		// get current active rolloutRun
		wList := &rolloutv1alpha1.RolloutRunList{}
		err := r.Client.List(clusterinfo.WithCluster(ctx, clusterinfo.Fed), wList, client.InNamespace(instance.Namespace), client.MatchingLabels{
			rollout.LabelControl:     "true",
			rollout.LabelGeneratedBy: instance.Name,
		})
		if err != nil {
			return nil, err
		}
		for i := range wList.Items {
			w := &wList.Items[i]
			if !w.IsCompleted() {
				return w, nil
			}
		}
		return nil, nil
	}

	run := &rolloutv1alpha1.RolloutRun{}
	err := r.Client.Get(clusterinfo.ContextFed, types.NamespacedName{
		Name:      instance.Status.RolloutID,
		Namespace: instance.GetNamespace(),
	}, run)
	if err != nil {
		if errors.IsNotFound(err) {
			// the rolloutRun may be manually deleted
			logger.Error(err, "failed to find current rolloutRun recorded in rollout status", "rolloutID", instance.Status.RolloutID)
			return nil, nil
		}
		return nil, err
	}
	return run, nil
}

func (r *RolloutReconciler) syncRun(ctx context.Context, obj *rolloutv1alpha1.Rollout, run *rolloutv1alpha1.RolloutRun, workloads []*workload.Info, newStatus *rolloutv1alpha1.RolloutStatus) error {
	if run == nil {
		resetRolloutStatus(newStatus, "", rolloutv1alpha1.RolloutPhaseInitialized)
		setStatusCondition(newStatus, rolloutv1alpha1.RolloutConditionProgressing, metav1.ConditionFalse, rolloutv1alpha1.RolloutReasonProgressingUnTriggered, "rollout is not triggered")
		return nil
	}

	// update status
	newStatus.RolloutID = run.Name
	newStatus.Phase = rolloutv1alpha1.RolloutPhaseProgressing

	if run.IsCompleted() {
		// wait for next trigger event
		resetRolloutStatus(newStatus, run.Name, rolloutv1alpha1.RolloutPhaseInitialized)
		setStatusCondition(newStatus, rolloutv1alpha1.RolloutConditionProgressing, metav1.ConditionFalse, rolloutv1alpha1.RolloutReasonProgressingCompleted, "rolloutRun is completed")
	} else {
		setStatusCondition(newStatus, rolloutv1alpha1.RolloutConditionProgressing, metav1.ConditionTrue, rolloutv1alpha1.RolloutReasonProgressingRunning, "rolloutRun is running")
	}

	err := r.handleRunManualCommand(ctx, obj, run)
	if err != nil {
		return err
	}

	if features.DefaultFeatureGate.Enabled(features.OneTimeStrategy) {
		err := r.applyOneTimeStrategy(obj, run, workloads, newStatus)
		if err != nil {
			return err
		}
	}

	return nil
}

func (r *RolloutReconciler) handleRunManualCommand(ctx context.Context, obj *rolloutv1alpha1.Rollout, run *rolloutv1alpha1.RolloutRun) error {
	command, ok := utils.GetMapValue(obj.Annotations, rollout.AnnoManualCommandKey)
	if !ok {
		return nil
	}
	if run.IsCompleted() {
		return nil
	}

	// TODO: add support for one time strategy

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

	logger := r.Logger.WithValues("rollout", utils.ObjectKeyString(obj), "rolloutRun", utils.ObjectKeyString(run))

	strategy := ontimestrategy.OneTimeStrategy{}
	err := json.Unmarshal([]byte(strategyStr), &strategy)
	if err != nil {
		setStatusCondition(newStatus, rolloutv1alpha1.RolloutConditionTrigger, metav1.ConditionFalse, "InvalidStrategy", fmt.Sprintf("failed to unmarshal one time strategy: %v", err))
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
		setStatusCondition(newStatus, rolloutv1alpha1.RolloutConditionTrigger, metav1.ConditionTrue, "OneTimeStrategyIgnored", "batch strategy is not changed")
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
		logger.Error(err, "failed to apply one time strategy to rolloutRun")
		setStatusCondition(newStatus, rolloutv1alpha1.RolloutConditionTrigger, metav1.ConditionFalse, "FailedUpdate", fmt.Sprintf("failed to apply one time stratey to RolloutRun: %v", err))
		// allow retry
		return err
	}

	setStatusCondition(newStatus, rolloutv1alpha1.RolloutConditionTrigger, metav1.ConditionTrue, "ApplyOneTimeStrategy", "succeed to apply one time strategy")
	return nil
}

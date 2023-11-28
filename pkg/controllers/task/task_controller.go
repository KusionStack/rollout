/*
Copyright 2023.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package task

import (
	"context"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/util/retry"
	"kusionstack.io/kube-utils/controller/mixin"
	"kusionstack.io/kube-utils/multicluster/clusterinfo"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"kusionstack.io/rollout/apis/workflow/v1alpha1"
	"kusionstack.io/rollout/pkg/event"
	pkgtask "kusionstack.io/rollout/pkg/task"
)

const (
	ControllerName = "task-controller"
)

// TaskReconciler reconciles a Task object
type TaskReconciler struct {
	*mixin.ReconcilerMixin
}

func InitFunc(mgr manager.Manager) (bool, error) {
	err := NewReconciler(mgr).SetupWithManager(mgr)
	if err != nil {
		return false, err
	}
	return true, nil
}

func NewReconciler(mgr manager.Manager) *TaskReconciler {
	return &TaskReconciler{
		ReconcilerMixin: mixin.NewReconcilerMixin(ControllerName, mgr),
	}
}

//+kubebuilder:rbac:groups=rollout.kusionstack.io,resources=tasks,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=rollout.kusionstack.io,resources=tasks/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=rollout.kusionstack.io,resources=tasks/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Task object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.13.1/pkg/reconcile
func (r *TaskReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	ctx = clusterinfo.WithCluster(ctx, clusterinfo.Fed)
	logger := log.FromContext(ctx)
	logger.Info("Reconciling Task")

	var task v1alpha1.Task
	if err := r.Client.Get(ctx, req.NamespacedName, &task); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		logger.Error(err, "unable to fetch Task")
		return ctrl.Result{}, err
	}

	// todo: deal with terminal state
	if task.DeletionTimestamp != nil {
		return ctrl.Result{}, nil
	}

	originStatus := task.Status.DeepCopy()
	beforeCondition := task.Status.GetCondition(v1alpha1.WorkflowConditionSucceeded)

	if !task.HasStarted() {
		task.Status.Initialize()
		r.Recorder.Eventf(&task, corev1.EventTypeNormal, "Initialized", "Initialized task %s", task.Name)

		// we already send event for initialized, so we update `beforeCondition` with current condition
		beforeCondition = task.Status.GetCondition(v1alpha1.WorkflowConditionSucceeded)
	}

	var d time.Duration
	var err error
	d, err = r.doReconcile(ctx, &task)
	if err != nil {
		logger.Error(err, "unable to reconcile Task")
		event.EmitError(r.Recorder, err, &task)
	}

	afterCondition := task.Status.GetCondition(v1alpha1.WorkflowConditionSucceeded)
	event.Emit(r.Recorder, beforeCondition, afterCondition, &task)

	// Update status if needed
	if !equality.Semantic.DeepEqual(originStatus, task.Status) {
		if err := r.updateStatus(ctx, &task); err != nil {
			logger.Error(err, "unable to update Task status")
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{RequeueAfter: d}, err
}

// SetupWithManager sets up the controller with the Manager.
func (r *TaskReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.Task{}).
		Complete(r)
}

func (r *TaskReconciler) doReconcile(ctx context.Context, task *v1alpha1.Task) (time.Duration, error) {
	logger := log.FromContext(ctx)

	if task.IsDone() {
		return 0, nil
	}

	if task.IsCanceled() {
		task.Status.Cancel("task is canceled")
		return 0, nil
	}

	if task.IsPending() {
		task.Status.Pending("task is pending")
		return 0, nil
	}

	if task.IsPaused() {
		// TODO(common_release/rollout#16): pause tasks
		task.Status.Pause("task is paused")
		return 0, nil
	}

	executor, err := pkgtask.NewTaskExecutor(r.Client, task)
	if err != nil {
		return 0, errors.WithMessage(err, "NewTaskExecutor failed")
	}

	d, err := executor.Run(ctx)
	if err != nil {
		logger.Error(err, "unable to run task")
		return 0, err
	}
	if d > 0 {
		logger.Info("requeue task", "requeueDuration", d)
	}

	executor.CalculateStatus()
	return d, nil
}

// updateStatus updates the status of the Task resource.
func (r *TaskReconciler) updateStatus(ctx context.Context, task *v1alpha1.Task) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		var existing v1alpha1.Task
		if err := r.Client.Get(ctx, client.ObjectKeyFromObject(task), &existing); err != nil {
			return client.IgnoreNotFound(err)
		}
		// do not update if the status is not changed
		if equality.Semantic.DeepEqual(task.Status, existing.Status) {
			return nil
		}
		task.Status.ObservedGeneration = task.Generation
		task.Status.LastUpdatedAt = &metav1.Time{Time: time.Now()}
		existing.Status = task.Status
		return r.Client.Status().Update(ctx, &existing)
	})
}

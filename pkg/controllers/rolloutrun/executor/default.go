package executor

import (
	"time"

	"github.com/go-logr/logr"
	rolloutapis "kusionstack.io/kube-api/rollout"
	rolloutv1alpha1 "kusionstack.io/kube-api/rollout/v1alpha1"
	ctrl "sigs.k8s.io/controller-runtime"

	"kusionstack.io/rollout/pkg/utils"
)

type Executor struct {
	logger   logr.Logger
	canary   *canaryExecutor
	batch    *batchExecutor
	rollback *rollbackExecutor
}

func NewDefaultExecutor(logger logr.Logger) *Executor {
	webhookExec := newWebhookExecutor(time.Second)
	canaryExec := newCanaryExecutor(webhookExec)
	batchExec := newBatchExecutor(webhookExec)
	rollbackExec := newRollbackExecutor(webhookExec)
	e := &Executor{
		logger:   logger,
		canary:   canaryExec,
		batch:    batchExec,
		rollback: rollbackExec,
	}
	return e
}

// Do execute the lifecycle for rollout run, and will return new status
func (r *Executor) Do(ctx *ExecutorContext) (bool, ctrl.Result, error) {
	// init NewStatus
	ctx.Initialize()

	logger := ctx.WithLogger(r.logger)

	newStatus := ctx.NewStatus
	rolloutRun := ctx.RolloutRun
	prePhase := newStatus.Phase

	defer func() {
		if prePhase != newStatus.Phase {
			logger.Info("rolloutRun status phase transition", "phase.from", prePhase, "phase.to", newStatus.Phase)
		}
	}()

	// if command exist, do command
	if _, exist := utils.GetMapValue(rolloutRun.Annotations, rolloutapis.AnnoManualCommandKey); exist {
		return false, r.doCommand(ctx), nil
	}

	return r.lifecycle(ctx)
}

// lifecycle
func (r *Executor) lifecycle(executorContext *ExecutorContext) (done bool, result ctrl.Result, err error) {
	newStatus := executorContext.NewStatus
	result = ctrl.Result{Requeue: true}
	rolloutRun := executorContext.RolloutRun

	// treat deletion as canceling and requeue
	if !rolloutRun.DeletionTimestamp.IsZero() && newStatus.Phase != rolloutv1alpha1.RolloutRunPhaseCanceling {
		newStatus.Phase = rolloutv1alpha1.RolloutRunPhaseCanceling
		// leave a phase transition log
		return false, result, nil
	}

	// determine whether to transit rolloutrun phase to rollbacking or not
	if rolloutRun.Spec.Rollback != nil && len(rolloutRun.Spec.Rollback.Batches) > 0 && rolloutRun.DeletionTimestamp.IsZero() &&
	 newStatus.Phase != rolloutv1alpha1.RolloutRunPhaseCanceling && newStatus.Phase != rolloutv1alpha1.RolloutRunPhaseRollbacking {
		if newStatus.Phase == rolloutv1alpha1.RolloutRunPhasePaused {
			rollback, ok := utils.GetMapValue(rolloutRun.Annotations, rolloutapis.AnnoRolloutPhaseRollbacking)
			if !ok || rollback != "true" {
				newStatus.Phase = rolloutv1alpha1.RolloutRunPhaseRollbacking
				return false, result, nil
			}
		} else {
			newStatus.Phase = rolloutv1alpha1.RolloutRunPhaseRollbacking
			return false, result, nil
		}
	}

	switch newStatus.Phase {
	case rolloutv1alpha1.RolloutRunPhaseInitial:
		newStatus.Phase = rolloutv1alpha1.RolloutRunPhasePreRollout
	case rolloutv1alpha1.RolloutRunPhasePausing:
		newStatus.Phase = rolloutv1alpha1.RolloutRunPhasePaused
	case rolloutv1alpha1.RolloutRunPhaseCanceling:
		var canceled bool
		canceled, result, err = r.doCanceling(executorContext)
		if canceled {
			newStatus.Phase = rolloutv1alpha1.RolloutRunPhaseCanceled
		}
	case rolloutv1alpha1.RolloutRunPhasePreRollout:
		newStatus.Phase = rolloutv1alpha1.RolloutRunPhaseProgressing
	case rolloutv1alpha1.RolloutRunPhaseProgressing:
		var processingDone bool
		processingDone, result, err = r.doProcessing(executorContext)
		if processingDone {
			newStatus.Phase = rolloutv1alpha1.RolloutRunPhasePostRollout
		}
	case rolloutv1alpha1.RolloutRunPhaseRollbacking:
		var rollbacked bool
		rollbacked, result, err = r.doRollbacking(executorContext)
		if rollbacked {
			newStatus.Phase = rolloutv1alpha1.RolloutRunPhaseRollbacked
		}
	case rolloutv1alpha1.RolloutRunPhasePostRollout:
		newStatus.Phase = rolloutv1alpha1.RolloutRunPhaseSucceeded
	case rolloutv1alpha1.RolloutRunPhasePaused:
		// rolloutRun is paused, do not requeue
		result.Requeue = false
	case rolloutv1alpha1.RolloutRunPhaseSucceeded, rolloutv1alpha1.RolloutRunPhaseCanceled, rolloutv1alpha1.RolloutRunPhaseRollbacked:
		done = true
		result.Requeue = false
	}
	return done, result, err
}

// doProcessing process canary and batch one-by-one
func (r *Executor) doProcessing(ctx *ExecutorContext) (bool, ctrl.Result, error) {
	rolloutRun := ctx.RolloutRun
	newStatus := ctx.NewStatus

	logger := ctx.GetLogger()

	if newStatus.Error != nil {
		// if error occurred, do nothing
		return false, ctrl.Result{Requeue: true}, nil
	}

	if ctx.inCanary() {
		canaryDone, result, err := r.canary.Do(ctx)
		if err != nil {
			return false, result, err
		}
		if !canaryDone {
			return false, result, nil
		}
		// canary is done, continue to do batch
	}

	if rolloutRun.Spec.Batch != nil && len(rolloutRun.Spec.Batch.Batches) > 0 {
		// init BatchStatus
		if len(newStatus.BatchStatus.CurrentBatchState) == 0 {
			newStatus.BatchStatus.CurrentBatchState = StepNone
		}
		preCurrentBatchIndex := newStatus.BatchStatus.CurrentBatchIndex
		preCurrentBatchState := newStatus.BatchStatus.CurrentBatchState
		defer func() {
			if preCurrentBatchIndex != newStatus.BatchStatus.CurrentBatchIndex ||
				preCurrentBatchState != newStatus.BatchStatus.CurrentBatchState {
				logger.Info("batch state trasition",
					"current.index", preCurrentBatchIndex,
					"current.state", preCurrentBatchState,
					"next.index", newStatus.BatchStatus.CurrentBatchIndex,
					"next.state", newStatus.BatchStatus.CurrentBatchState,
				)
			}
		}()
		return r.batch.Do(ctx)
	}

	return true, ctrl.Result{Requeue: true}, nil
}

func (r *Executor) doCanceling(ctx *ExecutorContext) (bool, ctrl.Result, error) {
	rolloutRun := ctx.RolloutRun
	newStatus := ctx.NewStatus

	if ctx.inCanary() {
		canceled, result, err := r.canary.Cancel(ctx)
		if err != nil {
			return false, result, err
		}
		return canceled, result, nil
	}
	if rolloutRun.Spec.Rollback != nil && len(rolloutRun.Spec.Rollback.Batches) > 0 {
		// init RollbackStatus
		if len(newStatus.RollbackStatus.CurrentBatchState) == 0 {
			newStatus.RollbackStatus.CurrentBatchState = StepNone
		}
		return r.rollback.Cancel(ctx)
	}
	if rolloutRun.Spec.Batch != nil && len(rolloutRun.Spec.Batch.Batches) > 0 {
		// init BatchStatus
		if len(newStatus.BatchStatus.CurrentBatchState) == 0 {
			newStatus.BatchStatus.CurrentBatchState = StepNone
		}
		return r.batch.Cancel(ctx)
	}

	return true, ctrl.Result{Requeue: true}, nil
}

func (r *Executor) doRollbacking(ctx *ExecutorContext) (bool, ctrl.Result, error) {
	rolloutRun := ctx.RolloutRun
	newStatus := ctx.NewStatus

	logger := ctx.GetLogger()

	if newStatus.Error != nil {
		// if error occurred, do nothing
		return false, ctrl.Result{Requeue: true}, nil
	}

	if rolloutRun.Spec.Rollback != nil && len(rolloutRun.Spec.Rollback.Batches) > 0 {
		// init RollbackStatus
		if len(newStatus.RollbackStatus.CurrentBatchState) == 0 {
			newStatus.RollbackStatus.CurrentBatchState = StepNone
		}
		preCurrentBatchIndex := newStatus.RollbackStatus.CurrentBatchIndex
		preCurrentBatchState := newStatus.RollbackStatus.CurrentBatchState
		defer func() {
			if preCurrentBatchIndex != newStatus.RollbackStatus.CurrentBatchIndex ||
				preCurrentBatchState != newStatus.RollbackStatus.CurrentBatchState {
				logger.Info("rollback batch state trasition",
					"current.index", preCurrentBatchIndex,
					"current.state", preCurrentBatchState,
					"next.index", newStatus.RollbackStatus.CurrentBatchIndex,
					"next.state", newStatus.RollbackStatus.CurrentBatchState,
				)
			}
		}()
		return r.rollback.Do(ctx)
	}

	return true, ctrl.Result{Requeue: true}, nil
}

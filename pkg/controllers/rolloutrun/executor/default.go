package executor

import (
	"time"

	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"

	rolloutapis "kusionstack.io/rollout/apis/rollout"
	rolloutv1alpha1 "kusionstack.io/rollout/apis/rollout/v1alpha1"
	"kusionstack.io/rollout/pkg/utils"
)

type Executor struct {
	logger logr.Logger
	canary *canaryExecutor
	batch  *batchExecutor
}

func NewDefaultExecutor(logger logr.Logger) *Executor {
	webhookExec := newWebhookExecutor(logger, time.Second)
	canaryExec := newCanaryExecutor(logger, webhookExec)
	batchExec := newBatchExecutor(logger, webhookExec)
	e := &Executor{
		logger: logger,
		canary: canaryExec,
		batch:  batchExec,
	}
	return e
}

// Do execute the lifecycle for rollout run, and will return new status
func (r *Executor) Do(ctx *ExecutorContext) (bool, ctrl.Result, error) {
	logger := ctx.loggerWithContext(r.logger)

	// init NewStatus
	ctx.Initialize()

	newStatus := ctx.NewStatus
	rolloutRun := ctx.RolloutRun
	prePhase := newStatus.Phase

	defer func() {
		if prePhase != newStatus.Phase {
			logger.Info("rolloutRun status phase transition", "phase.from", prePhase, "phase.to", newStatus.Phase)
		}
	}()

	// treat deletion as canceling and requeue
	if !rolloutRun.DeletionTimestamp.IsZero() && newStatus.Phase != rolloutv1alpha1.RolloutRunPhaseCanceling {
		newStatus.Phase = rolloutv1alpha1.RolloutRunPhaseCanceling
		return false, ctrl.Result{Requeue: true}, nil
	}

	// if command exist, do command
	if _, exist := utils.GetMapValue(rolloutRun.Annotations, rolloutapis.AnnoManualCommandKey); exist {
		return false, r.doCommand(ctx), nil
	}

	// if paused, do nothing
	if newStatus.Phase == rolloutv1alpha1.RolloutRunPhasePaused {
		logger.V(2).Info("rolloutRun is paused, do nothing")
		return false, ctrl.Result{}, nil
	}

	// if batchError exist, do nothing
	if newStatus.Error != nil {
		logger.V(2).Info("rolloutRun.status has error, do nothing")
		return false, ctrl.Result{}, nil
	}

	return r.lifecycle(ctx)
}

// lifecycle
func (r *Executor) lifecycle(executorContext *ExecutorContext) (done bool, result ctrl.Result, err error) {
	newStatus := executorContext.NewStatus
	result = ctrl.Result{Requeue: true}
	switch newStatus.Phase {
	case rolloutv1alpha1.RolloutRunPhaseInitial:
		newStatus.Phase = rolloutv1alpha1.RolloutRunPhasePreRollout
	case rolloutv1alpha1.RolloutRunPhasePausing:
		newStatus.Phase = rolloutv1alpha1.RolloutRunPhasePaused
	case rolloutv1alpha1.RolloutRunPhaseCanceling:
		newStatus.Phase = rolloutv1alpha1.RolloutRunPhaseCanceled
	case rolloutv1alpha1.RolloutRunPhasePreRollout:
		newStatus.Phase = rolloutv1alpha1.RolloutRunPhaseProgressing
	case rolloutv1alpha1.RolloutRunPhaseProgressing:
		var processingDone bool
		processingDone, result, err = r.doProcessing(executorContext)
		if processingDone {
			newStatus.Phase = rolloutv1alpha1.RolloutRunPhasePostRollout
		}
	case rolloutv1alpha1.RolloutRunPhasePostRollout:
		newStatus.Phase = rolloutv1alpha1.RolloutRunPhaseSucceeded
	case rolloutv1alpha1.RolloutRunPhasePaused:
		// rolloutRun is paused, do not requeue
		result.Requeue = false
	case rolloutv1alpha1.RolloutRunPhaseSucceeded, rolloutv1alpha1.RolloutRunPhaseCanceled:
		done = true
		result.Requeue = false
	}
	return done, result, err
}

// doProcessing process canary and batch one-by-one
func (r *Executor) doProcessing(ctx *ExecutorContext) (bool, ctrl.Result, error) {
	rolloutRun := ctx.RolloutRun
	newStatus := ctx.NewStatus

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
			newStatus.BatchStatus.CurrentBatchState = StepPending
		}
		preCurrentBatchIndex := newStatus.BatchStatus.CurrentBatchIndex
		preCurrentBatchState := newStatus.BatchStatus.CurrentBatchState
		defer func() {
			if preCurrentBatchIndex != newStatus.BatchStatus.CurrentBatchIndex ||
				preCurrentBatchState != newStatus.BatchStatus.CurrentBatchState {
				r.logger.Info("batch state trasition",
					"index.current", preCurrentBatchIndex,
					"state.current", preCurrentBatchState,
					"index.next", newStatus.BatchStatus.CurrentBatchIndex,
					"state.next", newStatus.BatchStatus.CurrentBatchState,
				)
			}
		}()
		return r.batch.Do(ctx)
	}

	return true, ctrl.Result{Requeue: true}, nil
}

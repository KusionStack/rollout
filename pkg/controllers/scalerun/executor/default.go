package executor

import (
	"time"

	"github.com/go-logr/logr"
	rolloutapis "kusionstack.io/kube-api/rollout"
	rolloutv1alpha1 "kusionstack.io/kube-api/rollout/v1alpha1"
	ctrl "sigs.k8s.io/controller-runtime"

	rorexecutor "kusionstack.io/rollout/pkg/controllers/rolloutrun/executor"
	"kusionstack.io/rollout/pkg/utils"
)

type Executor struct {
	logger logr.Logger
	batch  *batchExecutor
}

func NewDefaultExecutor(logger logr.Logger) *Executor {
	webhookExec := newWebhookExecutor(time.Second)
	batchExec := newBatchExecutor(webhookExec)
	e := &Executor{
		logger: logger,
		batch:  batchExec,
	}
	return e
}

// Do execute the lifecycle for rollout run, and will return new status
func (r *Executor) Do(ctx *ExecutorContext) (bool, ctrl.Result, error) {
	// init NewStatus
	ctx.Initialize()

	logger := ctx.WithLogger(r.logger)

	newStatus := ctx.NewStatus
	scaleRun := ctx.ScaleRun
	prePhase := newStatus.Phase

	defer func() {
		if prePhase != newStatus.Phase {
			logger.Info("scaleRun status phase transition", "phase.from", prePhase, "phase.to", newStatus.Phase)
		}
	}()

	// if command exist, do command
	if _, exist := utils.GetMapValue(scaleRun.Annotations, rolloutapis.AnnoManualCommandKey); exist {
		return false, r.doCommand(ctx), nil
	}

	return r.lifecycle(ctx)
}

// lifecycle
func (r *Executor) lifecycle(executorContext *ExecutorContext) (done bool, result ctrl.Result, err error) {
	newStatus := executorContext.NewStatus
	result = ctrl.Result{Requeue: true}
	scaleRun := executorContext.ScaleRun

	// treat deletion as canceling and requeue
	if !scaleRun.DeletionTimestamp.IsZero() && newStatus.Phase != rolloutv1alpha1.RolloutRunPhaseCanceling {
		newStatus.Phase = rolloutv1alpha1.RolloutRunPhaseCanceling
		// leave a phase transition log
		return false, result, nil
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
	scaleRun := ctx.ScaleRun
	newStatus := ctx.NewStatus

	logger := ctx.GetLogger()

	if newStatus.Error != nil {
		// if error occurred, do nothing
		return false, ctrl.Result{Requeue: true}, nil
	}

	if scaleRun.Spec.Batch != nil && len(scaleRun.Spec.Batch.Batches) > 0 {
		// init BatchStatus
		if len(newStatus.Batches.CurrentBatchState) == 0 {
			newStatus.Batches.CurrentBatchState = rorexecutor.StepNone
		}
		preCurrentBatchIndex := newStatus.Batches.CurrentBatchIndex
		preCurrentBatchState := newStatus.Batches.CurrentBatchState
		defer func() {
			if preCurrentBatchIndex != newStatus.Batches.CurrentBatchIndex ||
				preCurrentBatchState != newStatus.Batches.CurrentBatchState {
				logger.Info("scalerun batch state trasition",
					"current.index", preCurrentBatchIndex,
					"current.state", preCurrentBatchState,
					"next.index", newStatus.Batches.CurrentBatchIndex,
					"next.state", newStatus.Batches.CurrentBatchState,
				)
			}
		}()
		return r.batch.Do(ctx)
	}

	return true, ctrl.Result{Requeue: true}, nil
}

func (r *Executor) doCanceling(ctx *ExecutorContext) (bool, ctrl.Result, error) {
	scaleRun := ctx.ScaleRun
	newStatus := ctx.NewStatus

	if scaleRun.Spec.Batch != nil && len(scaleRun.Spec.Batch.Batches) > 0 {
		// init BatchStatus
		if len(newStatus.Batches.CurrentBatchState) == 0 {
			newStatus.Batches.CurrentBatchState = rorexecutor.StepNone
		}
		return r.batch.Cancel(ctx)
	}

	return true, ctrl.Result{Requeue: true}, nil
}

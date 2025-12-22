package executor

import (
	"time"

	"github.com/samber/lo"
	"k8s.io/apimachinery/pkg/util/clock"
	"k8s.io/utils/ptr"
	rolloutv1alpha1 "kusionstack.io/kube-api/rollout/v1alpha1"

	"kusionstack.io/rollout/pkg/controllers/rolloutrun/webhook"
)

const (
	ReasonWebhookNotExist                 = "WebhookNotExist"
	ReasonWebhookExecuteError             = "WebhookExecuteError"
	ReasonWebhookFailurePolicyInvalid     = "WebhookFailurePolicyInvalid"
	ReasonWebhookReviewStatusCodeUnknown  = "WebhookReviewStatusCodeUnknown"
	ReasonWebhookFailureThresholdExceeded = "WebhookFailureThresholdExceeded"
)

type webhookExecutor interface {
	Do(ctx *ExecutorContext, hookType rolloutv1alpha1.HookType) (bool, time.Duration, error)
	Cancel(ctx *ExecutorContext)
}

type webhookExecutorImpl struct {
	webhookManager  webhook.Manager
	webhookInitTime time.Duration
}

func newWebhookExecutor(webhookInitTime time.Duration) webhookExecutor {
	return &webhookExecutorImpl{
		webhookManager:  webhook.NewManager(clock.RealClock{}),
		webhookInitTime: webhookInitTime,
	}
}

func (r *webhookExecutorImpl) Do(ctx *ExecutorContext, hookType rolloutv1alpha1.HookType) (bool, time.Duration, error) {
	curWebhook, nextWebhook := r.findCurrentAndNextWebhook(ctx, hookType)
	if curWebhook == nil {
		return true, retryImmediately, nil
	}

	logger := ctx.GetLogger()
	logger.Info("processing webhook", "hookType", hookType, "webhook", curWebhook.Name)

	hookResult, _, err := r.startOrGetWebhookWorker(ctx, hookType, *curWebhook.RolloutWebhook, curWebhook.status)
	if err != nil {
		logger.Error(err, "failed to get webhook result")
		return false, retryImmediately, err
	}

	logger.V(2).Info("get webhook result", "hookType", hookType, "webhook", curWebhook.Name, "result", hookResult)

	ctx.SetWebhookStatus(rolloutv1alpha1.RolloutWebhookStatus(*hookResult))

	if hookResult.State == rolloutv1alpha1.WebhookOnHold &&
		hookResult.Code == rolloutv1alpha1.WebhookReviewCodeError &&
		ctx.NewStatus.Error == nil {
		// set error if possible
		ctx.NewStatus.Error = &hookResult.CodeReasonMessage
	}
	if hookResult.State != rolloutv1alpha1.WebhookCompleted {
		// the webhook sill running, requeue after defaultRequeueAfter duration
		return false, retryDefault, nil
	}

	if nextWebhook != nil {
		// add empty status to start next webhook
		ctx.SetWebhookStatus(rolloutv1alpha1.RolloutWebhookStatus{
			HookType: hookType,
			Name:     nextWebhook.Name,
		})
		return false, retryImmediately, nil
	}

	// NOTE:
	// The code up to this point indicates that the webhooks have all been completed, and we can safely clean up the results.
	// However, there is still one scenario where, if the current webhook status is not updated successfully, the executor will come back
	// and execute the last webhook again. Because the webhook is idempotent, it is safe to re-execute it.
	logger.Info("clean up final webhook", "hookType", hookType)
	r.webhookManager.Stop(ctx.ScaleRun.UID)

	return true, retryImmediately, nil
}

func (r *webhookExecutorImpl) Cancel(ctx *ExecutorContext) {
	logger := ctx.GetLogger()
	logger.Info("cancel webhook", "scaleRun", ctx.ScaleRun.Name)
	r.webhookManager.Stop(ctx.ScaleRun.UID)
}

type webhookWithStatus struct {
	*rolloutv1alpha1.RolloutWebhook
	status *rolloutv1alpha1.RolloutWebhookStatus
}

func (r *webhookExecutorImpl) findCurrentAndNextWebhook(executorContext *ExecutorContext, hookType rolloutv1alpha1.HookType) (*webhookWithStatus, *rolloutv1alpha1.RolloutWebhook) {
	webhooks, latestStatus := executorContext.GetWebhooksAndLatestStatusBy(hookType)
	if len(webhooks) == 0 {
		// no webhooks
		return nil, nil
	}

	index := 0
	var currentWebhookStatus *rolloutv1alpha1.RolloutWebhookStatus

	if latestStatus != nil {
		_, tempI, found := lo.FindIndexOf(webhooks, func(rw rolloutv1alpha1.RolloutWebhook) bool {
			return rw.Name == latestStatus.Name
		})
		if found {
			// last status found in webhooks, it is current webhook
			currentWebhookStatus = latestStatus
			index = tempI
		}
	}

	current, next := getCurrentAndNext(webhooks, index)
	if current == nil {
		return nil, nil
	}

	currentWebhook := &webhookWithStatus{
		RolloutWebhook: current,
		status:         currentWebhookStatus,
	}

	return currentWebhook, next
}

func (r *webhookExecutorImpl) startOrGetWebhookWorker(ctx *ExecutorContext, hookType rolloutv1alpha1.HookType, webhookCfg rolloutv1alpha1.RolloutWebhook, lastStatus *rolloutv1alpha1.RolloutWebhookStatus) (*webhook.Result, bool, error) {
	run := ctx.ScaleRun
	key := run.UID
	logger := ctx.GetLogger()
	worker, ok := r.webhookManager.Get(key)
	if ok {
		// webhook already started
		curResult := worker.Result()
		if curResult.Name == webhookCfg.Name && curResult.HookType == hookType {
			if lastStatus != nil && lastStatus.State == rolloutv1alpha1.WebhookOnHold {
				// lastStatus is onHold, that means it should be retry
				worker.Retry()
				// return a temporary result
				curResult.State = rolloutv1alpha1.WebhookRunning
			}
			return &curResult, false, nil
		}

		// webhook name or type not match, stop it and start a new one
		logger.Info("stop the old webhook worker", "webhook", curResult.Name, "type", curResult.HookType)
		worker.Stop()
	}

	logger.Info("start a new webhook worker and wait for the result for a brief period.", "webhook", webhookCfg.Name, "type", hookType)

	review := ctx.makeRolloutWebhookReview(hookType, webhookCfg)
	worker, err := r.webhookManager.Start(key, webhookCfg, review)
	if err != nil {
		return nil, false, err
	}

	// Delay briefly and attempt to retrieve the webhook result immediately.
	time.Sleep(r.webhookInitTime)

	return ptr.To(worker.Result()), true, nil
}

func getCurrentAndNext[T any](input []T, index int) (*T, *T) {
	length := len(input)
	var current, next *T

	if index < length {
		current = &input[index]
	}
	if index < length-1 {
		next = &input[index+1]
	}
	return current, next
}

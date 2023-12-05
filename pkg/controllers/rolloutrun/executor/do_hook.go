package executor

import (
	"context"
	"errors"
	"fmt"
	"github.com/go-logr/logr"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"

	rolloutv1alpha1 "kusionstack.io/rollout/apis/rollout/v1alpha1"
	"kusionstack.io/rollout/pkg/utils"
	"kusionstack.io/rollout/pkg/webhook"
)

const (
	ReasonWebhookNotExist                 = "WebhookNotExist"
	ReasonWebhookExecuteError             = "WebhookExecuteError"
	ReasonWebhookFailurePolicyInvalid     = "WebhookFailurePolicyInvalid"
	ReasonWebhookFailureThresholdExceeded = "WebhookFailureThresholdExceeded"
)

// detectRequeueAfter detect webhook requeue interval
func detectRequeueAfter(webhook *rolloutv1alpha1.RolloutWebhook) time.Duration {
	if webhook.ClientConfig.PeriodSeconds != 0 {
		return time.Duration(defaultRequeueAfter) * time.Second
	}
	return time.Duration(webhook.ClientConfig.PeriodSeconds)
}

// newHookCodeReasonMessage construct CodeReasonMessage
func newHookCodeReasonMessage(hookType rolloutv1alpha1.HookType, reason string, msg string) *rolloutv1alpha1.CodeReasonMessage {
	return &rolloutv1alpha1.CodeReasonMessage{Code: fmt.Sprintf("%sError", string(hookType)), Reason: reason, Message: msg}
}

// moveToNextWebhook
func moveToNextWebhook(webhookName string, hookType rolloutv1alpha1.HookType, batchStatusRecord *rolloutv1alpha1.RolloutRunBatchStatusRecord) {
	_, exist := utils.Find(
		batchStatusRecord.Webhooks,
		func(w *rolloutv1alpha1.BatchWebhookStatus) bool {
			if w == nil {
				return false
			}
			return w.HookType == hookType && w.Name == webhookName
		},
	)
	if !exist {
		batchStatusRecord.Webhooks = append(
			batchStatusRecord.Webhooks,
			rolloutv1alpha1.BatchWebhookStatus{Name: webhookName, HookType: hookType},
		)
	}
}

// makeRolloutWebhookReview construct RolloutWebhookReview
func makeRolloutWebhookReview(hookType rolloutv1alpha1.HookType, webhook *rolloutv1alpha1.RolloutWebhook, executorContext *ExecutorContext) *rolloutv1alpha1.RolloutWebhookReview {
	rollout := executorContext.rollout
	rolloutRun := executorContext.rolloutRun
	batchStatus := executorContext.newStatus.BatchStatus
	return &rolloutv1alpha1.RolloutWebhookReview{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   rollout.Namespace,
			Name:        rollout.Name,
			Labels:      map[string]string{},
			Annotations: map[string]string{},
		},
		Spec: rolloutv1alpha1.RolloutWebhookReviewSpec{
			RolloutName:      rollout.Name,
			RolloutNamespace: rollout.Namespace,
			RolloutID:        rolloutRun.Name,
			HookType:         hookType,
			BatchIndex:       batchStatus.CurrentBatchIndex,
			Properties:       webhook.Properties,
			TargetType:       rolloutRun.Spec.TargetType,
			Targets:          rolloutRun.Spec.Batch.Batches[batchStatus.CurrentBatchIndex].Targets,
		},
	}
}

// doPreBatchHook process PreBatchHook state
func (r *Executor) doPreBatchHook(ctx context.Context, executorContext *ExecutorContext) (bool, ctrl.Result, error) {
	logger := logr.FromContext(ctx)

	newBatchStatus := executorContext.newStatus.BatchStatus

	currentBatchIndex := newBatchStatus.CurrentBatchIndex
	logger.Info(
		"DefaultExecutor begin to doPreBatchHook", "currentBatchIndex", currentBatchIndex,
	)

	var (
		done   bool
		err    error
		result ctrl.Result
	)
	task := func() {
		done, result, err = r.runRolloutWebhooks(ctx, rolloutv1alpha1.HookTypePreBatchStep, executorContext)
	}
	if timeoutErr := utils.RunWithTimeout(ctx, defaultRunRolloutWebhooksTimeout, task); timeoutErr != nil {
		if err == nil {
			err = timeoutErr
		}
		logger.Error(timeoutErr, "DefaultExecutor doPreBatchHook error since timeout")
	}

	if done {
		newBatchStatus.CurrentBatchState = rolloutv1alpha1.BatchStepStateRunning
		newBatchStatus.Records[currentBatchIndex].State = rolloutv1alpha1.BatchStepStateRunning
	}

	return done, result, err
}

// todo add analysis logic
// doPostBatchHook process doPostBatchHook state
func (r *Executor) doPostBatchHook(ctx context.Context, executorContext *ExecutorContext) (bool, ctrl.Result, error) {
	logger := logr.FromContext(ctx)

	rolloutRun := executorContext.rolloutRun
	newBatchStatus := executorContext.newStatus.BatchStatus

	currentBatchIndex := newBatchStatus.CurrentBatchIndex
	logger.Info(
		"DefaultExecutor begin to doPostBatchHook,", "currentBatchIndex", currentBatchIndex,
	)

	var (
		done   bool
		err    error
		result ctrl.Result
	)
	task := func() {
		done, result, err = r.runRolloutWebhooks(ctx, rolloutv1alpha1.HookTypePostBatchStep, executorContext)
	}
	if timeoutErr := utils.RunWithTimeout(ctx, defaultRunRolloutWebhooksTimeout, task); timeoutErr != nil {
		if err == nil {
			err = timeoutErr
		}
		logger.Error(timeoutErr, "DefaultExecutor doPostBatchHook error since timeout")
	}

	if done {
		pause := rolloutRun.Spec.Batch.Batches[currentBatchIndex].Pause
		if pause != nil || *pause {
			logger.Info("DefaultExecutor will pause")
			newBatchStatus.CurrentBatchState = rolloutv1alpha1.BatchStepStatePaused
			newBatchStatus.Records[currentBatchIndex].State = rolloutv1alpha1.BatchStepStatePaused
		} else {
			newBatchStatus.CurrentBatchState = rolloutv1alpha1.BatchStepStateSucceeded
			if newBatchStatus.Records[currentBatchIndex].FinishTime == nil {
				newBatchStatus.Records[currentBatchIndex].FinishTime = &metav1.Time{Time: time.Now()}
			}
			newBatchStatus.Records[currentBatchIndex].State = rolloutv1alpha1.BatchStepStateSucceeded
		}
	}

	return done, result, err
}

// runRolloutWebhooks process webhooks
// todo add backoff when status Processing or failureCount not exceed failureThreshold
func (r *Executor) runRolloutWebhooks(ctx context.Context, hookType rolloutv1alpha1.HookType, executorContext *ExecutorContext) (bool, ctrl.Result, error) {
	logger := log.FromContext(ctx)

	rolloutRun := executorContext.rolloutRun
	newBatchStatus := executorContext.newStatus.BatchStatus

	webhooks := utils.Filter(
		rolloutRun.Spec.Webhooks,
		func(w *rolloutv1alpha1.RolloutWebhook) bool {
			if w != nil &&
				len(hookType) > 0 &&
				len(w.HookTypes) > 0 {
				for _, item := range w.HookTypes {
					if item == hookType {
						return true
					}
				}
			}
			return false
		},
	)

	if len(webhooks) == 0 {
		return true, ctrl.Result{Requeue: true}, nil
	}

	currentBatchIndex := newBatchStatus.CurrentBatchIndex
	var batchWebhookStatus rolloutv1alpha1.BatchWebhookStatus
	batchStatusRecord := newBatchStatus.Records[currentBatchIndex]
	if len(batchStatusRecord.Webhooks) == 0 {
		batchWebhookStatus = rolloutv1alpha1.BatchWebhookStatus{}
		batchWebhookStatus.HookType = hookType
		batchWebhookStatus.Name = webhooks[0].Name
		batchStatusRecord.Webhooks = append(batchStatusRecord.Webhooks, batchWebhookStatus)
	} else {
		batchWebhookStatus = batchStatusRecord.Webhooks[len(batchStatusRecord.Webhooks)-1]
		exists := utils.Any(
			webhooks,
			func(item *rolloutv1alpha1.RolloutWebhook) bool {
				return item.Name == batchWebhookStatus.Name
			},
		)
		if !exists {
			newBatchStatus.CurrentBatchError = newHookCodeReasonMessage(
				hookType,
				ReasonWebhookNotExist,
				fmt.Sprintf("Webhook(%s) not found", batchWebhookStatus.Name),
			)
			return false, ctrl.Result{}, fmt.Errorf(newBatchStatus.CurrentBatchError.Message)
		}
	}

	for idx, item := range webhooks {
		if item.Name != batchWebhookStatus.Name {
			continue
		}

		failureThreshold := int32(3)
		if item.FailureThreshold >= 1 {
			failureThreshold = item.FailureThreshold
		}

		status, err := webhook.RunRolloutWebhook(
			ctx, &item, makeRolloutWebhookReview(hookType, &item, executorContext),
		)
		if err != nil {
			logger.Error(err, "Webhook execute error", "Webhook", item.Name)
			if status == nil {
				status = &rolloutv1alpha1.RolloutWebhookReviewStatus{
					CodeReasonMessage: *newHookCodeReasonMessage(
						hookType,
						ReasonWebhookExecuteError,
						fmt.Sprintf("Webhook(%s) execute error, err=%v", item.Name, err),
					),
				}
			}
		}
		if status != nil {
			batchWebhookStatus.CodeReasonMessage = status.CodeReasonMessage
			batchWebhookStatus.CodeReasonMessage.Message = utils.Abbreviate(status.Message, 32)
		}

		if status.Code == rolloutv1alpha1.WebhookReviewCodeOK {
			if idx >= (len(webhooks) - 1) {
				return true, ctrl.Result{}, nil
			}
			moveToNextWebhook(webhooks[idx+1].Name, hookType, &batchStatusRecord)
		} else if status.Code == rolloutv1alpha1.WebhookReviewCodeProcessing {
			return false, ctrl.Result{RequeueAfter: detectRequeueAfter(&item)}, nil
		} else {
			// error
			batchWebhookStatus.FailureCount++
			if status.Code != rolloutv1alpha1.WebhookReviewCodeError {
				logger.Info(
					"Webhook status illegal", "Webhook", item.Name, "status", status,
				)
			}
			if batchWebhookStatus.FailureCount <= failureThreshold {
				return false, ctrl.Result{RequeueAfter: detectRequeueAfter(&item)}, nil
			} else {
				if item.FailurePolicy == nil ||
					rolloutv1alpha1.Ignore == *item.FailurePolicy {
					if idx >= (len(webhooks) - 1) {
						return true, ctrl.Result{}, nil
					}
					moveToNextWebhook(webhooks[idx+1].Name, hookType, &batchStatusRecord)
				} else if rolloutv1alpha1.Fail == *item.FailurePolicy {
					newBatchStatus.CurrentBatchError = newHookCodeReasonMessage(
						hookType,
						ReasonWebhookFailureThresholdExceeded,
						fmt.Sprintf(
							"Webhook(%s) failed since failCnt(%d) exceeded",
							item.Name, batchWebhookStatus.FailureCount,
						),
					)
					return false, ctrl.Result{}, fmt.Errorf(newBatchStatus.CurrentBatchError.Message)
				} else {
					newBatchStatus.CurrentBatchError = newHookCodeReasonMessage(
						hookType,
						ReasonWebhookFailurePolicyInvalid,
						fmt.Sprintf(
							"webhook(%s) failed since FailurePolicy(%s) invalid",
							item.Name, *item.FailurePolicy,
						),
					)
					return false, ctrl.Result{}, fmt.Errorf(newBatchStatus.CurrentBatchError.Message)
				}
			}
		}
	}

	return false, ctrl.Result{}, errors.New("RunRolloutWebhook unexpected error found")
}

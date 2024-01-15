package executor

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	rolloutv1alpha1 "kusionstack.io/rollout/apis/rollout/v1alpha1"
	"kusionstack.io/rollout/pkg/utils"
	"kusionstack.io/rollout/pkg/webhook"
)

const (
	ReasonWebhookNotExist                 = "WebhookNotExist"
	ReasonWebhookExecuteError             = "WebhookExecuteError"
	ReasonWebhookFailurePolicyInvalid     = "WebhookFailurePolicyInvalid"
	ReasonWebhookReviewStatusCodeUnknown  = "WebhookReviewStatusCodeUnknown"
	ReasonWebhookFailureThresholdExceeded = "WebhookFailureThresholdExceeded"
)

// newCode construct Code
func newCode(hookType rolloutv1alpha1.HookType) string {
	return fmt.Sprintf("%sError", string(hookType))
}

// newHookCodeReasonMessage construct CodeReasonMessage
func newHookCodeReasonMessage(webhookStatus *rolloutv1alpha1.BatchWebhookStatus, hookType rolloutv1alpha1.HookType, reason string, msg string) *rolloutv1alpha1.CodeReasonMessage {
	if webhookStatus != nil {
		webhookStatus.FailureCountAtError = webhookStatus.FailureCount
	}
	return &rolloutv1alpha1.CodeReasonMessage{Code: newCode(hookType), Reason: reason, Message: msg}
}

// doBatchPreBatchHook process PreBatchHook state
func (r *Executor) doBatchPreBatchHook(ctx context.Context, executorContext *ExecutorContext) (ctrl.Result, error) {
	logger := r.logger

	newBatchStatus := executorContext.NewStatus.BatchStatus
	currentBatchIndex := newBatchStatus.CurrentBatchIndex
	logger.Info(
		"DefaultExecutor begin to doPreBatchHook", "currentBatchIndex", currentBatchIndex,
	)

	hookType := rolloutv1alpha1.HookTypePreBatchStep
	done, result, err := r.runRolloutWebhooks(ctx, hookType, executorContext)
	if done {
		newBatchStatus.CurrentBatchState = BatchStateUpgrading
	}

	newBatchStatus.Records[currentBatchIndex].State = newBatchStatus.CurrentBatchState

	return result, err
}

// todo add analysis logic
// doBatchPostBatchHook process doPostBatchHook state
func (r *Executor) doBatchPostBatchHook(ctx context.Context, executorContext *ExecutorContext) (ctrl.Result, error) {
	newBatchStatus := executorContext.NewStatus.BatchStatus
	currentBatchIndex := newBatchStatus.CurrentBatchIndex
	r.logger.Info(
		"DefaultExecutor begin to doPostBatchHook,", "currentBatchIndex", currentBatchIndex,
	)

	hookType := rolloutv1alpha1.HookTypePostBatchStep
	done, result, err := r.runRolloutWebhooks(ctx, hookType, executorContext)

	if done {
		newBatchStatus.CurrentBatchState = BatchStateSucceeded
		if newBatchStatus.Records[currentBatchIndex].FinishTime == nil {
			newBatchStatus.Records[currentBatchIndex].FinishTime = &metav1.Time{Time: time.Now()}
		}
	}
	newBatchStatus.Records[currentBatchIndex].State = newBatchStatus.CurrentBatchState

	return result, err
}

// detectRequeueAfter detect webhook requeue interval
func detectRequeueAfter(webhook *rolloutv1alpha1.RolloutWebhook) time.Duration {
	if webhook.ClientConfig.PeriodSeconds <= 0 {
		return defaultRequeueAfter
	}
	return time.Duration(webhook.ClientConfig.PeriodSeconds) * time.Second
}

// detectTimeout
func detectTimeout() time.Duration {
	return time.Duration(defaultRunTimeout) * time.Second
}

// moveToNextWebhook
func moveToNextWebhook(webhookName string, hookType rolloutv1alpha1.HookType, batchStatusRecord *rolloutv1alpha1.RolloutRunBatchStatusRecord) {
	_, exist := utils.Find(
		batchStatusRecord.Webhooks,
		func(w *rolloutv1alpha1.BatchWebhookStatus) bool {
			return w != nil && w.HookType == hookType && w.Name == webhookName
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
	rollout := executorContext.Rollout
	rolloutRun := executorContext.RolloutRun
	newBatchStatus := executorContext.NewStatus.BatchStatus
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
			Properties:       webhook.Properties,
			TargetType:       rolloutRun.Spec.TargetType,
			Batch: &rolloutv1alpha1.RolloutWebhookReviewBatch{
				BatchIndex: newBatchStatus.CurrentBatchIndex,
				Targets:    rolloutRun.Spec.Batch.Batches[newBatchStatus.CurrentBatchIndex].Targets,
				Properties: rolloutRun.Spec.Batch.Batches[newBatchStatus.CurrentBatchIndex].Properties,
			},
		},
	}
}

// detectWebhooks return webhooks met hookType
func detectWebhooks(hookType rolloutv1alpha1.HookType, rolloutRun *rolloutv1alpha1.RolloutRun) []rolloutv1alpha1.RolloutWebhook {
	return utils.Filter(
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
}

// detectLatestWebhookStatus detect latest webhook to invoke
func detectLatestWebhookStatus(hookType rolloutv1alpha1.HookType, webhooks []rolloutv1alpha1.RolloutWebhook, executorContext *ExecutorContext) *rolloutv1alpha1.BatchWebhookStatus {
	newStatus := executorContext.NewStatus
	newBatchStatus := executorContext.NewStatus.BatchStatus
	currentBatchIndex := newBatchStatus.CurrentBatchIndex
	currentBatchRecord := &newBatchStatus.Records[currentBatchIndex]
	if len(currentBatchRecord.Webhooks) == 0 {
		firstHook := rolloutv1alpha1.BatchWebhookStatus{}
		firstHook.HookType = hookType
		firstHook.Name = webhooks[0].Name
		currentBatchRecord.Webhooks = append(currentBatchRecord.Webhooks, firstHook)
	} else {
		exists := utils.Any(
			currentBatchRecord.Webhooks,
			func(w *rolloutv1alpha1.BatchWebhookStatus) bool {
				return w.HookType == hookType
			},
		)
		if !exists {
			firstHook := rolloutv1alpha1.BatchWebhookStatus{}
			firstHook.HookType = hookType
			firstHook.Name = webhooks[0].Name
			currentBatchRecord.Webhooks = append(currentBatchRecord.Webhooks, firstHook)
		}
	}

	webhookStatus := &currentBatchRecord.Webhooks[len(currentBatchRecord.Webhooks)-1]

	// double check
	{
		exists := utils.Any(
			webhooks, func(w *rolloutv1alpha1.RolloutWebhook) bool {
				typeExists := utils.Any(
					w.HookTypes, func(t *rolloutv1alpha1.HookType) bool { return *t == hookType },
				)
				return w.Name == webhookStatus.Name && typeExists
			},
		)
		if !exists {
			newStatus.Error = newHookCodeReasonMessage(
				nil,
				hookType,
				ReasonWebhookNotExist,
				fmt.Sprintf("Webhook(%s) not found", webhookStatus.Name),
			)
		}
	}

	return webhookStatus
}

// runRolloutWebhooks process webhooks
// todo add backoff when status Processing or failureCount not exceed failureThreshold
func (r *Executor) runRolloutWebhooks(ctx context.Context, hookType rolloutv1alpha1.HookType, executorContext *ExecutorContext) (bool, ctrl.Result, error) {
	logger := r.logger

	webhooks := detectWebhooks(hookType, executorContext.RolloutRun)
	if len(webhooks) == 0 {
		return true, ctrl.Result{Requeue: true}, nil
	}

	// detect webhook to invoke
	newStatus := executorContext.NewStatus
	newBatchStatus := executorContext.NewStatus.BatchStatus
	currentBatchIndex := newBatchStatus.CurrentBatchIndex
	currentBatchRecord := &newBatchStatus.Records[currentBatchIndex]
	webhookStatus := detectLatestWebhookStatus(hookType, webhooks, executorContext)
	if newStatus.Error != nil {
		return false, ctrl.Result{}, fmt.Errorf(newStatus.Error.Message)
	}

	// invoke webhook iteratively
	deadline := time.Now().Add(detectTimeout())
	for idx, item := range webhooks {
		if item.Name != webhookStatus.Name {
			continue
		}

		failureThreshold := int32(3)
		if item.FailureThreshold >= 1 {
			failureThreshold = item.FailureThreshold
		}

		failurePolicy := item.FailurePolicy
		if len(failurePolicy) == 0 {
			failurePolicy = rolloutv1alpha1.Ignore
		}

		// timeout check
		if deadline.Before(time.Now()) {
			return false, ctrl.Result{RequeueAfter: detectRequeueAfter(&item)}, nil
		}

		newCtx := logr.NewContext(ctx, r.logger)
		status, err := webhook.RunRolloutWebhook(
			newCtx, &item, makeRolloutWebhookReview(hookType, &item, executorContext),
		)
		if err != nil {
			logger.Error(err, "Webhook execute error", "Webhook", item.Name)
			if status == nil {
				status = &rolloutv1alpha1.RolloutWebhookReviewStatus{
					CodeReasonMessage: rolloutv1alpha1.CodeReasonMessage{
						Code:    rolloutv1alpha1.WebhookReviewCodeError,
						Reason:  ReasonWebhookExecuteError,
						Message: fmt.Sprintf("Webhook(%s) execute error, err=[%v]", item.Name, err),
					},
				}
			}
		}
		webhookStatus.CodeReasonMessage = status.CodeReasonMessage
		webhookStatus.CodeReasonMessage.Message = utils.Abbreviate(status.Message, 1024)

		if status.Code == rolloutv1alpha1.WebhookReviewCodeOK {
			if idx >= (len(webhooks) - 1) {
				return true, ctrl.Result{Requeue: true}, nil
			}
			moveToNextWebhook(webhooks[idx+1].Name, hookType, currentBatchRecord)
			webhookStatus = &currentBatchRecord.Webhooks[len(currentBatchRecord.Webhooks)-1]
		} else if status.Code == rolloutv1alpha1.WebhookReviewCodeProcessing {
			return false, ctrl.Result{RequeueAfter: detectRequeueAfter(&item)}, nil
		} else {
			// error
			webhookStatus.FailureCount++
			if status.Code != rolloutv1alpha1.WebhookReviewCodeError {
				logger.Info(
					"Webhook status illegal", "Webhook", item.Name, "status", status,
				)
				message := fmt.Sprintf("%v", status)
				status = &rolloutv1alpha1.RolloutWebhookReviewStatus{
					CodeReasonMessage: rolloutv1alpha1.CodeReasonMessage{
						Message: message,
						Reason:  ReasonWebhookReviewStatusCodeUnknown,
						Code:    rolloutv1alpha1.WebhookReviewCodeError,
					},
				}
				webhookStatus.CodeReasonMessage = status.CodeReasonMessage
				webhookStatus.CodeReasonMessage.Message = utils.Abbreviate(status.Message, 1024)
			}
			if webhookStatus.FailureCount < (failureThreshold + webhookStatus.FailureCountAtError) {
				return false, ctrl.Result{RequeueAfter: detectRequeueAfter(&item)}, nil
			} else {
				if rolloutv1alpha1.Ignore == failurePolicy {
					if idx >= (len(webhooks) - 1) {
						return true, ctrl.Result{Requeue: true}, nil
					}
					moveToNextWebhook(webhooks[idx+1].Name, hookType, currentBatchRecord)
					webhookStatus = &currentBatchRecord.Webhooks[len(currentBatchRecord.Webhooks)-1]
				} else if rolloutv1alpha1.Fail == failurePolicy {
					newStatus.Error = newHookCodeReasonMessage(
						webhookStatus,
						hookType,
						ReasonWebhookFailureThresholdExceeded,
						fmt.Sprintf(
							"Webhook(%s) failed since failCnt(%d) exceeded",
							item.Name, webhookStatus.FailureCount,
						),
					)
					return false, ctrl.Result{}, fmt.Errorf(newStatus.Error.Message)
				} else {
					newStatus.Error = newHookCodeReasonMessage(
						webhookStatus,
						hookType,
						ReasonWebhookFailurePolicyInvalid,
						fmt.Sprintf(
							"webhook(%s) failed since FailurePolicy(%s) invalid", item.Name, failurePolicy,
						),
					)
					return false, ctrl.Result{}, fmt.Errorf(newStatus.Error.Message)
				}
			}
		}
	}

	return false, ctrl.Result{}, errors.New("RunRolloutWebhook unexpected error found")
}

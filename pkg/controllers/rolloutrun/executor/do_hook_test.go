package executor

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"k8s.io/utils/ptr"

	rolloutv1alpha1 "kusionstack.io/rollout/apis/rollout/v1alpha1"
)

var (
	webhook1 = rolloutv1alpha1.RolloutWebhook{
		Name: "webhook-1",
		HookTypes: []rolloutv1alpha1.HookType{
			rolloutv1alpha1.PreCanaryStepHook,
			rolloutv1alpha1.PostCanaryStepHook,
			rolloutv1alpha1.PreBatchStepHook,
			rolloutv1alpha1.PostBatchStepHook,
		},
		ClientConfig: rolloutv1alpha1.WebhookClientConfig{
			URL:            "webhook-1.test.url",
			TimeoutSeconds: 1,
			PeriodSeconds:  1,
		},
		FailureThreshold: 1,
		FailurePolicy:    rolloutv1alpha1.Ignore,
	}

	webhook1Error = rolloutv1alpha1.CodeReasonMessage{
		Code:    rolloutv1alpha1.WebhookReviewCodeError,
		Reason:  "DoRequestError",
		Message: `Post "webhook-1.test.url": unsupported protocol scheme ""`,
	}

	webhook2 = rolloutv1alpha1.RolloutWebhook{
		Name: "webhook-2",
		HookTypes: []rolloutv1alpha1.HookType{
			rolloutv1alpha1.PreCanaryStepHook,
			rolloutv1alpha1.PostCanaryStepHook,
			rolloutv1alpha1.PreBatchStepHook,
			rolloutv1alpha1.PostBatchStepHook,
		},
		ClientConfig: rolloutv1alpha1.WebhookClientConfig{
			URL:            "webhook-2.test.url",
			TimeoutSeconds: 1,
			PeriodSeconds:  1,
		},
		FailureThreshold: 1,
		FailurePolicy:    rolloutv1alpha1.Fail,
	}
	webhook2Error = rolloutv1alpha1.CodeReasonMessage{
		Code:    rolloutv1alpha1.WebhookReviewCodeError,
		Reason:  "DoRequestError",
		Message: `Post "webhook-2.test.url": unsupported protocol scheme ""`,
	}

	webhook3 = rolloutv1alpha1.RolloutWebhook{
		Name: "webhook-3",
		HookTypes: []rolloutv1alpha1.HookType{
			rolloutv1alpha1.PreCanaryStepHook,
			rolloutv1alpha1.PostCanaryStepHook,
			rolloutv1alpha1.PreBatchStepHook,
			rolloutv1alpha1.PostBatchStepHook,
		},
		ClientConfig: rolloutv1alpha1.WebhookClientConfig{
			URL:            "webhook-3.test.url",
			TimeoutSeconds: 1,
			PeriodSeconds:  3,
		},
		FailureThreshold: 3,
		FailurePolicy:    rolloutv1alpha1.Fail,
	}

	webhook3Error = rolloutv1alpha1.CodeReasonMessage{
		Code:    rolloutv1alpha1.WebhookReviewCodeError,
		Reason:  "DoRequestError",
		Message: `Post "webhook-3.test.url": unsupported protocol scheme ""`,
	}
)

type fakeWebhookExecutor struct{}

func (e *fakeWebhookExecutor) Do(ctx *ExecutorContext, hookType rolloutv1alpha1.HookType) (bool, time.Duration, error) {
	return true, retryImmediately, nil
}

func newFakeWebhookExecutor() *fakeWebhookExecutor {
	return &fakeWebhookExecutor{}
}

type webhookTestCase struct {
	name         string
	getObjects   func() (*rolloutv1alpha1.Rollout, *rolloutv1alpha1.RolloutRun)
	assertResult func(assert *assert.Assertions, done bool, result time.Duration, err error)
	assertStatus func(assert *assert.Assertions, status *rolloutv1alpha1.RolloutRunStatus)
}

func newTestWebhookExecutor() webhookExecutor {
	return newWebhookExecutor(newTestLogger(), 2*time.Second)
}

func runWebhookTestCases(t *testing.T, hookType rolloutv1alpha1.HookType, tests []webhookTestCase) {
	exe := newTestWebhookExecutor()

	for i := range tests {
		tt := tests[i]
		t.Run(tt.name, func(t *testing.T) {
			rollout, rolloutRun := tt.getObjects()
			ctx := createTestExecutorContext(rollout, rolloutRun, nil)
			done, got, err := exe.Do(ctx, hookType)
			assert := assert.New(t)
			tt.assertResult(assert, done, got, err)
			tt.assertStatus(assert, ctx.NewStatus)
		})
	}
}

func Test_webhook_Retry(t *testing.T) {
	exe := newTestWebhookExecutor()

	hookType := rolloutv1alpha1.PreCanaryStepHook
	rollout := testRollout.DeepCopy()
	rolloutRun := testRolloutRun.DeepCopy()

	rolloutRun.Spec.Webhooks = []rolloutv1alpha1.RolloutWebhook{
		*webhook2.DeepCopy(),
	}
	rolloutRun.Spec.Canary = &rolloutv1alpha1.RolloutRunCanaryStrategy{
		Targets: unimportantTargets,
	}
	rolloutRun.Status.CanaryStatus = &rolloutv1alpha1.RolloutRunStepStatus{
		State: StepPreCanaryStepHook,
	}

	ctx := createTestExecutorContext(rollout, rolloutRun, nil)
	_, _, err := exe.Do(ctx, hookType)
	assert.Nil(t, err)
	assert.NotNil(t, ctx.NewStatus.Error)
	if assert.Len(t, ctx.NewStatus.CanaryStatus.Webhooks, 1) {
		assert.Equal(t, rolloutv1alpha1.RolloutWebhookStatus{
			State:             rolloutv1alpha1.WebhookOnHold,
			HookType:          hookType,
			Name:              webhook2.Name,
			CodeReasonMessage: webhook2Error,
			FailureCount:      1,
		}, ctx.NewStatus.CanaryStatus.Webhooks[0])
	}

	// set new status
	rolloutRun.Status = *ctx.NewStatus
	// delete error to retry
	rolloutRun.Status.Error = nil
	ctx = createTestExecutorContext(rollout, rolloutRun, nil)
	// do it again
	_, _, err = exe.Do(ctx, hookType)
	assert.Nil(t, err)
	assert.Nil(t, ctx.NewStatus.Error)
	if assert.Len(t, ctx.NewStatus.CanaryStatus.Webhooks, 1) {
		assert.Equal(t, rolloutv1alpha1.RolloutWebhookStatus{
			State:             rolloutv1alpha1.WebhookRunning,
			HookType:          hookType,
			Name:              webhook2.Name,
			CodeReasonMessage: webhook2Error,
			FailureCount:      1,
		}, ctx.NewStatus.CanaryStatus.Webhooks[0])
	}

	// must wait for a while to get next expected result
	time.Sleep(1 * time.Second)

	// set new status
	rolloutRun.Status = *ctx.NewStatus
	ctx = createTestExecutorContext(rollout, rolloutRun, nil)
	// get result again
	_, _, err = exe.Do(ctx, hookType)
	assert.Nil(t, err)
	assert.NotNil(t, ctx.NewStatus.Error)
	if assert.Len(t, ctx.NewStatus.CanaryStatus.Webhooks, 1) {
		assert.Equal(t, ctx.NewStatus.CanaryStatus.Webhooks[0], rolloutv1alpha1.RolloutWebhookStatus{
			State:             rolloutv1alpha1.WebhookOnHold,
			HookType:          hookType,
			Name:              webhook2.Name,
			CodeReasonMessage: webhook2Error,
			FailureCount:      2,
		}, ctx.NewStatus.CanaryStatus.Webhooks[0])
	}
}

func Test_webhook_PreCanaryHookStep(t *testing.T) {
	hookType := rolloutv1alpha1.PreCanaryStepHook
	tests := []webhookTestCase{
		{
			name: "webhook 1 has failed but will be ignored, no error in status",
			getObjects: func() (*rolloutv1alpha1.Rollout, *rolloutv1alpha1.RolloutRun) {
				rollout := testRollout.DeepCopy()
				rolloutRun := testRolloutRun.DeepCopy()
				rolloutRun.Spec.Webhooks = []rolloutv1alpha1.RolloutWebhook{
					*webhook1.DeepCopy(),
				}
				rolloutRun.Spec.Canary = &rolloutv1alpha1.RolloutRunCanaryStrategy{
					Targets: unimportantTargets,
				}
				rolloutRun.Status.CanaryStatus = &rolloutv1alpha1.RolloutRunStepStatus{
					State: StepPreCanaryStepHook,
				}
				return rollout, rolloutRun
			},
			assertResult: func(assert *assert.Assertions, done bool, retry time.Duration, err error) {
				assert.True(done)
				assert.Nil(err)
				assert.Equal(retryImmediately, retry)
			},
			assertStatus: func(assert *assert.Assertions, status *rolloutv1alpha1.RolloutRunStatus) {
				assert.Nil(status.Error, "This webhook has encountered a failure, but it will be ignored, no error will be reported")
				if assert.Len(status.CanaryStatus.Webhooks, 1) {
					assert.Equal(rolloutv1alpha1.RolloutWebhookStatus{
						State:             rolloutv1alpha1.WebhookCompleted,
						HookType:          hookType,
						Name:              webhook1.Name,
						CodeReasonMessage: webhook1Error,
						FailureCount:      1,
					}, status.CanaryStatus.Webhooks[0])
				}
			},
		},
		{
			name: "webhook 2 has failed, error should be set",
			getObjects: func() (*rolloutv1alpha1.Rollout, *rolloutv1alpha1.RolloutRun) {
				rollout := testRollout.DeepCopy()
				rolloutRun := testRolloutRun.DeepCopy()
				rolloutRun.Spec.Webhooks = []rolloutv1alpha1.RolloutWebhook{
					*webhook1.DeepCopy(),
					*webhook2.DeepCopy(),
				}
				rolloutRun.Spec.Canary = &rolloutv1alpha1.RolloutRunCanaryStrategy{
					Targets: unimportantTargets,
				}
				rolloutRun.Status.CanaryStatus = &rolloutv1alpha1.RolloutRunStepStatus{
					State: StepPreCanaryStepHook,
					Webhooks: []rolloutv1alpha1.RolloutWebhookStatus{
						{
							State:             rolloutv1alpha1.WebhookCompleted,
							HookType:          rolloutv1alpha1.PreCanaryStepHook,
							Name:              webhook1.Name,
							CodeReasonMessage: webhook1Error,
							FailureCount:      1,
						},
						{
							HookType: rolloutv1alpha1.PreCanaryStepHook,
							Name:     webhook2.Name, // current webhook
						},
					},
				}
				return rollout, rolloutRun
			},
			assertResult: func(assert *assert.Assertions, done bool, retry time.Duration, err error) {
				assert.False(done)
				assert.Nil(err)
				assert.Equal(retryDefault, retry)
			},
			assertStatus: func(assert *assert.Assertions, status *rolloutv1alpha1.RolloutRunStatus) {
				if assert.NotNil(status.Error, "The webhook has failed, and the corresponding error should be set") {
					assert.EqualValues(webhook2Error, *status.Error)
				}
				if assert.Len(status.CanaryStatus.Webhooks, 2) {
					assert.Equal(rolloutv1alpha1.RolloutWebhookStatus{
						State:             rolloutv1alpha1.WebhookCompleted,
						HookType:          rolloutv1alpha1.PreCanaryStepHook,
						Name:              webhook1.Name,
						CodeReasonMessage: webhook1Error,
						FailureCount:      1,
					}, status.CanaryStatus.Webhooks[0])
					assert.Equal(rolloutv1alpha1.RolloutWebhookStatus{
						State:             rolloutv1alpha1.WebhookOnHold,
						HookType:          rolloutv1alpha1.PreCanaryStepHook,
						Name:              webhook2.Name,
						CodeReasonMessage: webhook2Error,
						FailureCount:      1,
					}, status.CanaryStatus.Webhooks[1])
				}
			},
		},
		{
			name: "webhook 3 failed once, but it is still running, no error",
			getObjects: func() (*rolloutv1alpha1.Rollout, *rolloutv1alpha1.RolloutRun) {
				rollout := testRollout.DeepCopy()
				rolloutRun := testRolloutRun.DeepCopy()
				rolloutRun.Spec.Webhooks = []rolloutv1alpha1.RolloutWebhook{
					*webhook3.DeepCopy(),
				}
				rolloutRun.Spec.Canary = &rolloutv1alpha1.RolloutRunCanaryStrategy{
					Targets: unimportantTargets,
				}
				rolloutRun.Status.CanaryStatus = &rolloutv1alpha1.RolloutRunStepStatus{
					State: StepPreCanaryStepHook,
				}
				return rollout, rolloutRun
			},
			assertResult: func(assert *assert.Assertions, done bool, retry time.Duration, err error) {
				assert.False(done)
				assert.Nil(err)
				assert.Equal(retryDefault, retry)
			},
			assertStatus: func(assert *assert.Assertions, status *rolloutv1alpha1.RolloutRunStatus) {
				assert.Nil(status.Error, "This webhook has encountered a failure, but it is still running, no error will be reported")
				if assert.Len(status.CanaryStatus.Webhooks, 1) {
					assert.Equal(rolloutv1alpha1.RolloutWebhookStatus{
						State:             rolloutv1alpha1.WebhookRunning,
						HookType:          rolloutv1alpha1.PreCanaryStepHook,
						Name:              webhook3.Name,
						CodeReasonMessage: webhook3Error,
						FailureCount:      1,
					}, status.CanaryStatus.Webhooks[0])
				}
			},
		},
		{
			name: "webhook 1 finished, then webhook 2 will be started next time",
			getObjects: func() (*rolloutv1alpha1.Rollout, *rolloutv1alpha1.RolloutRun) {
				rollout := testRollout.DeepCopy()
				rolloutRun := testRolloutRun.DeepCopy()
				rolloutRun.Spec.Webhooks = []rolloutv1alpha1.RolloutWebhook{
					*webhook1.DeepCopy(),
					*webhook2.DeepCopy(),
				}
				rolloutRun.Spec.Canary = &rolloutv1alpha1.RolloutRunCanaryStrategy{
					Targets: unimportantTargets,
				}
				rolloutRun.Status.CanaryStatus = &rolloutv1alpha1.RolloutRunStepStatus{
					State: StepPreCanaryStepHook,
				}
				return rollout, rolloutRun
			},
			assertResult: func(assert *assert.Assertions, done bool, retry time.Duration, err error) {
				assert.False(done)
				assert.Nil(err)
				assert.Equal(retryImmediately, retry)
			},
			assertStatus: func(assert *assert.Assertions, status *rolloutv1alpha1.RolloutRunStatus) {
				assert.Nil(status.Error)
				if assert.Len(status.CanaryStatus.Webhooks, 2) {
					assert.Equal(rolloutv1alpha1.RolloutWebhookStatus{
						State:             rolloutv1alpha1.WebhookCompleted,
						HookType:          rolloutv1alpha1.PreCanaryStepHook,
						Name:              webhook1.Name,
						CodeReasonMessage: webhook1Error,
						FailureCount:      1,
					}, status.CanaryStatus.Webhooks[0])
					assert.Equal(rolloutv1alpha1.RolloutWebhookStatus{
						HookType: rolloutv1alpha1.PreCanaryStepHook,
						Name:     webhook2.Name,
					}, status.CanaryStatus.Webhooks[1])
				}
			},
		},
		{
			name: "no webhooks",
			getObjects: func() (*rolloutv1alpha1.Rollout, *rolloutv1alpha1.RolloutRun) {
				rollout := testRollout.DeepCopy()
				rolloutRun := testRolloutRun.DeepCopy()
				rolloutRun.Spec.Webhooks = []rolloutv1alpha1.RolloutWebhook{}
				rolloutRun.Spec.Canary = &rolloutv1alpha1.RolloutRunCanaryStrategy{
					Targets: unimportantTargets,
				}
				rolloutRun.Spec.Webhooks = []rolloutv1alpha1.RolloutWebhook{
					{
						Name:      "webhook",
						HookTypes: []rolloutv1alpha1.HookType{}, // the hookType matches no one
					},
				}
				rolloutRun.Status.CanaryStatus = &rolloutv1alpha1.RolloutRunStepStatus{
					State: StepPreCanaryStepHook,
				}
				return rollout, rolloutRun
			},
			assertResult: func(assert *assert.Assertions, done bool, retry time.Duration, err error) {
				assert.True(done)
				assert.Nil(err)
				assert.Equal(retryImmediately, retry)
			},
			assertStatus: func(assert *assert.Assertions, status *rolloutv1alpha1.RolloutRunStatus) {
				assert.Nil(status.Error)
				assert.Len(status.CanaryStatus.Webhooks, 0)
			},
		},
	}

	runWebhookTestCases(t, hookType, tests)
}

func Test_webhook_PostCanaryHookStep(t *testing.T) {
	hookType := rolloutv1alpha1.PostCanaryStepHook
	tests := []webhookTestCase{
		{
			name: "webhook 1 has failed but will be ignored, no error in status",
			getObjects: func() (*rolloutv1alpha1.Rollout, *rolloutv1alpha1.RolloutRun) {
				rollout := testRollout.DeepCopy()
				rolloutRun := testRolloutRun.DeepCopy()
				rolloutRun.Spec.Webhooks = []rolloutv1alpha1.RolloutWebhook{
					*webhook1.DeepCopy(),
				}
				rolloutRun.Spec.Canary = &rolloutv1alpha1.RolloutRunCanaryStrategy{
					Targets: unimportantTargets,
				}
				rolloutRun.Status.CanaryStatus = &rolloutv1alpha1.RolloutRunStepStatus{
					State: StepPostCanaryStepHook,
				}
				return rollout, rolloutRun
			},
			assertResult: func(assert *assert.Assertions, done bool, retry time.Duration, err error) {
				assert.True(done)
				assert.Nil(err)
				assert.Equal(retryImmediately, retry)
			},
			assertStatus: func(assert *assert.Assertions, status *rolloutv1alpha1.RolloutRunStatus) {
				assert.Nil(status.Error, "This webhook has encountered a failure, but it will be ignored, no error will be reported.")
				if assert.Len(status.CanaryStatus.Webhooks, 1) {
					assert.Equal(rolloutv1alpha1.RolloutWebhookStatus{
						State:             rolloutv1alpha1.WebhookCompleted,
						HookType:          rolloutv1alpha1.PostCanaryStepHook,
						Name:              webhook1.Name,
						CodeReasonMessage: webhook1Error,
						FailureCount:      1,
					}, status.CanaryStatus.Webhooks[0])
				}
			},
		},
		{
			name: "webhook 2 has failed, error should be set",
			getObjects: func() (*rolloutv1alpha1.Rollout, *rolloutv1alpha1.RolloutRun) {
				rollout := testRollout.DeepCopy()
				rolloutRun := testRolloutRun.DeepCopy()
				rolloutRun.Spec.Webhooks = []rolloutv1alpha1.RolloutWebhook{
					*webhook2.DeepCopy(),
				}
				rolloutRun.Spec.Canary = &rolloutv1alpha1.RolloutRunCanaryStrategy{
					Targets: unimportantTargets,
				}
				rolloutRun.Status.CanaryStatus = &rolloutv1alpha1.RolloutRunStepStatus{
					State: StepPostCanaryStepHook,
				}
				return rollout, rolloutRun
			},
			assertResult: func(assert *assert.Assertions, done bool, retry time.Duration, err error) {
				assert.False(done)
				assert.Nil(err)
				assert.Equal(retryDefault, retry)
			},
			assertStatus: func(assert *assert.Assertions, status *rolloutv1alpha1.RolloutRunStatus) {
				if assert.NotNil(status.Error, "The webhook has failed, and the corresponding error should be set.") {
					assert.EqualValues(webhook2Error, *status.Error)
				}
				if assert.Len(status.CanaryStatus.Webhooks, 1) {
					assert.Equal(rolloutv1alpha1.RolloutWebhookStatus{
						State:             rolloutv1alpha1.WebhookOnHold,
						HookType:          rolloutv1alpha1.PostCanaryStepHook,
						Name:              webhook2.Name,
						CodeReasonMessage: webhook2Error,
						FailureCount:      1,
					}, status.CanaryStatus.Webhooks[0])
				}
			},
		},
	}

	runWebhookTestCases(t, hookType, tests)
}

func Test_webhook_PreBatchHookStep(t *testing.T) {
	hookType := rolloutv1alpha1.PreBatchStepHook
	tests := []webhookTestCase{
		{
			name: "webhook 1 failed but ignored, no error in status",
			getObjects: func() (*rolloutv1alpha1.Rollout, *rolloutv1alpha1.RolloutRun) {
				rollout := testRollout.DeepCopy()
				rolloutRun := testRolloutRun.DeepCopy()
				rolloutRun.Spec.Webhooks = []rolloutv1alpha1.RolloutWebhook{
					*webhook1.DeepCopy(),
				}
				rolloutRun.Spec.Batch.Batches = []rolloutv1alpha1.RolloutRunStep{{
					Targets: unimportantTargets,
				}}

				return rollout, rolloutRun
			},
			assertResult: func(assert *assert.Assertions, done bool, retry time.Duration, err error) {
				assert.True(done)
				assert.Nil(err)
				assert.Equal(retryImmediately, retry)
			},
			assertStatus: func(assert *assert.Assertions, status *rolloutv1alpha1.RolloutRunStatus) {
				assert.Nil(status.Error, "This webhook has encountered a failure, but it will be ignored, no error will be reported.")
				if assert.Len(status.BatchStatus.Records[0].Webhooks, 1) {
					assert.Equal(rolloutv1alpha1.RolloutWebhookStatus{
						State:             rolloutv1alpha1.WebhookCompleted,
						HookType:          hookType,
						Name:              webhook1.Name,
						CodeReasonMessage: webhook1Error,
						FailureCount:      1,
					}, status.BatchStatus.Records[0].Webhooks[0])
				}
			},
		},
		{
			name: "webhook 2 has failed, error should be set",
			getObjects: func() (*rolloutv1alpha1.Rollout, *rolloutv1alpha1.RolloutRun) {
				rollout := testRollout.DeepCopy()
				rolloutRun := testRolloutRun.DeepCopy()
				rolloutRun.Spec.Webhooks = []rolloutv1alpha1.RolloutWebhook{
					*webhook2.DeepCopy(),
				}
				rolloutRun.Spec.Batch.Batches = []rolloutv1alpha1.RolloutRunStep{{
					Targets: unimportantTargets,
				}}
				rolloutRun.Status.BatchStatus = &rolloutv1alpha1.RolloutRunBatchStatus{
					RolloutBatchStatus: rolloutv1alpha1.RolloutBatchStatus{
						CurrentBatchState: StepPreBatchStepHook,
					},
					Records: []rolloutv1alpha1.RolloutRunStepStatus{
						{
							Index: ptr.To[int32](0),
							State: StepPreBatchStepHook,
						},
					},
				}
				return rollout, rolloutRun
			},
			assertResult: func(assert *assert.Assertions, done bool, retry time.Duration, err error) {
				assert.False(done)
				assert.Nil(err)
				assert.Equal(retryDefault, retry)
			},
			assertStatus: func(assert *assert.Assertions, status *rolloutv1alpha1.RolloutRunStatus) {
				if assert.NotNil(status.Error, "The webhook has failed, and the corresponding error should be set.") {
					assert.EqualValues(webhook2Error, *status.Error)
				}
				if assert.Len(status.BatchStatus.Records[0].Webhooks, 1) {
					assert.Equal(rolloutv1alpha1.RolloutWebhookStatus{
						State:             rolloutv1alpha1.WebhookOnHold,
						HookType:          hookType,
						Name:              webhook2.Name,
						CodeReasonMessage: webhook2Error,
						FailureCount:      1,
					}, status.BatchStatus.Records[0].Webhooks[0])
				}
			},
		},
	}

	runWebhookTestCases(t, hookType, tests)
}

func Test_webhook_PostBatchHookStep(t *testing.T) {
	hookType := rolloutv1alpha1.PostBatchStepHook
	tests := []webhookTestCase{
		{
			name: "webhook 1 failed but ignored, no error in status",
			getObjects: func() (*rolloutv1alpha1.Rollout, *rolloutv1alpha1.RolloutRun) {
				rollout := testRollout.DeepCopy()
				rolloutRun := testRolloutRun.DeepCopy()
				rolloutRun.Spec.Webhooks = []rolloutv1alpha1.RolloutWebhook{
					*webhook1.DeepCopy(),
				}
				rolloutRun.Spec.Batch.Batches = []rolloutv1alpha1.RolloutRunStep{{
					Targets: unimportantTargets,
				}}

				return rollout, rolloutRun
			},
			assertResult: func(assert *assert.Assertions, done bool, retry time.Duration, err error) {
				assert.True(done)
				assert.Nil(err)
				assert.Equal(retryImmediately, retry)
			},
			assertStatus: func(assert *assert.Assertions, status *rolloutv1alpha1.RolloutRunStatus) {
				assert.Nil(status.Error, "This webhook has encountered a failure, but it will be ignored, no error will be reported.")
				if assert.Len(status.BatchStatus.Records[0].Webhooks, 1) {
					assert.Equal(rolloutv1alpha1.RolloutWebhookStatus{
						State:             rolloutv1alpha1.WebhookCompleted,
						HookType:          hookType,
						Name:              webhook1.Name,
						CodeReasonMessage: webhook1Error,
						FailureCount:      1,
					}, status.BatchStatus.Records[0].Webhooks[0])
				}
			},
		},
		{
			name: "webhook 2 has failed, error should be set",
			getObjects: func() (*rolloutv1alpha1.Rollout, *rolloutv1alpha1.RolloutRun) {
				rollout := testRollout.DeepCopy()
				rolloutRun := testRolloutRun.DeepCopy()
				rolloutRun.Spec.Webhooks = []rolloutv1alpha1.RolloutWebhook{
					*webhook2.DeepCopy(),
				}
				rolloutRun.Spec.Batch.Batches = []rolloutv1alpha1.RolloutRunStep{{
					Targets: unimportantTargets,
				}}
				rolloutRun.Status.BatchStatus = &rolloutv1alpha1.RolloutRunBatchStatus{
					RolloutBatchStatus: rolloutv1alpha1.RolloutBatchStatus{
						CurrentBatchState: StepPostBatchStepHook,
					},
					Records: []rolloutv1alpha1.RolloutRunStepStatus{
						{
							Index: ptr.To[int32](0),
							State: StepPostBatchStepHook,
						},
					},
				}
				return rollout, rolloutRun
			},
			assertResult: func(assert *assert.Assertions, done bool, retry time.Duration, err error) {
				assert.False(done)
				assert.Nil(err)
				assert.Equal(retryDefault, retry)
			},
			assertStatus: func(assert *assert.Assertions, status *rolloutv1alpha1.RolloutRunStatus) {
				if assert.NotNil(status.Error, "The webhook has failed, and the corresponding error should be set.") {
					assert.EqualValues(webhook2Error, *status.Error)
				}
				if assert.Len(status.BatchStatus.Records[0].Webhooks, 1) {
					assert.Equal(rolloutv1alpha1.RolloutWebhookStatus{
						State:             rolloutv1alpha1.WebhookOnHold,
						HookType:          hookType,
						Name:              webhook2.Name,
						CodeReasonMessage: webhook2Error,
						FailureCount:      1,
					}, status.BatchStatus.Records[0].Webhooks[0])
				}
			},
		},
	}

	runWebhookTestCases(t, hookType, tests)
}

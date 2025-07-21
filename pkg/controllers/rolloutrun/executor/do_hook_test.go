package executor

import (
	"time"

	"github.com/stretchr/testify/suite"
	"k8s.io/utils/ptr"
	rolloutv1alpha1 "kusionstack.io/kube-api/rollout/v1alpha1"
)

type webhookExecutorTestSuite struct {
	suite.Suite

	executor webhookExecutor

	webhook1      rolloutv1alpha1.RolloutWebhook
	webhook1Error rolloutv1alpha1.CodeReasonMessage
	webhook2      rolloutv1alpha1.RolloutWebhook
	webhook2Error rolloutv1alpha1.CodeReasonMessage
	webhook3      rolloutv1alpha1.RolloutWebhook
	webhook3Error rolloutv1alpha1.CodeReasonMessage

	rollout    *rolloutv1alpha1.Rollout
	rolloutRun *rolloutv1alpha1.RolloutRun
}

func (s *webhookExecutorTestSuite) SetupSuite() {
	s.executor = newWebhookExecutor(2 * time.Second)

	s.webhook1 = rolloutv1alpha1.RolloutWebhook{
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

	s.webhook2 = rolloutv1alpha1.RolloutWebhook{
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

	s.webhook3 = rolloutv1alpha1.RolloutWebhook{
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

	s.webhook1Error = rolloutv1alpha1.CodeReasonMessage{
		Code:    rolloutv1alpha1.WebhookReviewCodeError,
		Reason:  "DoRequestError",
		Message: `Post "webhook-1.test.url": unsupported protocol scheme ""`,
	}
	s.webhook2Error = rolloutv1alpha1.CodeReasonMessage{
		Code:    rolloutv1alpha1.WebhookReviewCodeError,
		Reason:  "DoRequestError",
		Message: `Post "webhook-2.test.url": unsupported protocol scheme ""`,
	}
	s.webhook3Error = rolloutv1alpha1.CodeReasonMessage{
		Code:    rolloutv1alpha1.WebhookReviewCodeError,
		Reason:  "DoRequestError",
		Message: `Post "webhook-3.test.url": unsupported protocol scheme ""`,
	}
}

func (s *webhookExecutorTestSuite) SetupTest() {
	s.rollout = testRollout.DeepCopy()
	s.rolloutRun = testRolloutRun.DeepCopy()
}

type webhookTestCase struct {
	name         string
	getObjects   func() (*rolloutv1alpha1.Rollout, *rolloutv1alpha1.RolloutRun)
	assertResult func(done bool, result time.Duration, err error)
	assertStatus func(status *rolloutv1alpha1.RolloutRunStatus)
}

func (s *webhookExecutorTestSuite) runWebhookTestCases(hookType rolloutv1alpha1.HookType, tests []webhookTestCase) {
	for i := range tests {
		tt := tests[i]
		s.Run(tt.name, func() {
			rollout, rolloutRun := tt.getObjects()
			ctx := createTestExecutorContext(rollout, rolloutRun)
			done, got, err := s.executor.Do(ctx, hookType)
			tt.assertResult(done, got, err)
			tt.assertStatus(ctx.NewStatus)
		})
	}
}

func (s *webhookExecutorTestSuite) Test_Webhook_Retry() {
	hookType := rolloutv1alpha1.PreCanaryStepHook
	rollout := s.rollout
	rolloutRun := s.rolloutRun

	rolloutRun.Spec.Webhooks = []rolloutv1alpha1.RolloutWebhook{
		*s.webhook2.DeepCopy(),
	}
	rolloutRun.Spec.Canary = &rolloutv1alpha1.RolloutRunCanaryStrategy{
		Targets: unimportantTargets,
	}
	rolloutRun.Status.CanaryStatus = &rolloutv1alpha1.RolloutRunStepStatus{
		State: StepPreCanaryStepHook,
	}

	ctx := createTestExecutorContext(rollout, rolloutRun)
	_, _, err := s.executor.Do(ctx, hookType)
	s.Require().NoError(err)
	s.NotNil(ctx.NewStatus.Error)
	if s.Len(ctx.NewStatus.CanaryStatus.Webhooks, 1) {
		s.Equal(rolloutv1alpha1.RolloutWebhookStatus{
			State:             rolloutv1alpha1.WebhookOnHold,
			HookType:          hookType,
			Name:              s.webhook2.Name,
			CodeReasonMessage: s.webhook2Error,
			FailureCount:      1,
		}, ctx.NewStatus.CanaryStatus.Webhooks[0])
	}

	// set new status
	rolloutRun.Status = *ctx.NewStatus
	// delete error to retry
	rolloutRun.Status.Error = nil
	ctx = createTestExecutorContext(rollout, rolloutRun)
	// do it again
	_, _, err = s.executor.Do(ctx, hookType)
	s.Require().NoError(err)
	s.Nil(ctx.NewStatus.Error)
	if s.Len(ctx.NewStatus.CanaryStatus.Webhooks, 1) {
		s.Equal(rolloutv1alpha1.RolloutWebhookStatus{
			State:             rolloutv1alpha1.WebhookRunning,
			HookType:          hookType,
			Name:              s.webhook2.Name,
			CodeReasonMessage: s.webhook2Error,
			FailureCount:      1,
		}, ctx.NewStatus.CanaryStatus.Webhooks[0])
	}

	// must wait for a while to get next expected result
	time.Sleep(1 * time.Second)

	// set new status
	rolloutRun.Status = *ctx.NewStatus
	ctx = createTestExecutorContext(rollout, rolloutRun)
	// get result again
	_, _, err = s.executor.Do(ctx, hookType)
	s.Require().NoError(err)
	s.NotNil(ctx.NewStatus.Error)
	if s.Len(ctx.NewStatus.CanaryStatus.Webhooks, 1) {
		s.Equal(rolloutv1alpha1.RolloutWebhookStatus{
			State:             rolloutv1alpha1.WebhookOnHold,
			HookType:          hookType,
			Name:              s.webhook2.Name,
			CodeReasonMessage: s.webhook2Error,
			FailureCount:      2,
		}, ctx.NewStatus.CanaryStatus.Webhooks[0])
	}
}

func (s *webhookExecutorTestSuite) Test_Webhook_PreCanaryHookStep() {
	hookType := rolloutv1alpha1.PreCanaryStepHook
	tests := []webhookTestCase{
		{
			name: "webhook 1 has failed but will be ignored, no error in status",
			getObjects: func() (*rolloutv1alpha1.Rollout, *rolloutv1alpha1.RolloutRun) {
				rollout := s.rollout.DeepCopy()
				rolloutRun := s.rolloutRun.DeepCopy()
				rolloutRun.Spec.Webhooks = []rolloutv1alpha1.RolloutWebhook{
					*s.webhook1.DeepCopy(),
				}
				rolloutRun.Spec.Canary = &rolloutv1alpha1.RolloutRunCanaryStrategy{
					Targets: unimportantTargets,
				}
				rolloutRun.Status.CanaryStatus = &rolloutv1alpha1.RolloutRunStepStatus{
					State: StepPreCanaryStepHook,
				}
				return rollout, rolloutRun
			},
			assertResult: func(done bool, retry time.Duration, err error) {
				s.Require().NoError(err)
				s.Require().True(done)
				s.Equal(retryImmediately, retry)
			},
			assertStatus: func(status *rolloutv1alpha1.RolloutRunStatus) {
				s.Nil(status.Error, "This webhook has encountered a failure, but it will be ignored, no error will be reported")
				if s.Len(status.CanaryStatus.Webhooks, 1) {
					s.Equal(rolloutv1alpha1.RolloutWebhookStatus{
						State:             rolloutv1alpha1.WebhookCompleted,
						HookType:          hookType,
						Name:              s.webhook1.Name,
						CodeReasonMessage: s.webhook1Error,
						FailureCount:      1,
					}, status.CanaryStatus.Webhooks[0])
				}
			},
		},
		{
			name: "webhook 2 has failed, error should be set",
			getObjects: func() (*rolloutv1alpha1.Rollout, *rolloutv1alpha1.RolloutRun) {
				rollout := s.rollout.DeepCopy()
				rolloutRun := s.rolloutRun.DeepCopy()
				rolloutRun.Spec.Webhooks = []rolloutv1alpha1.RolloutWebhook{
					*s.webhook1.DeepCopy(),
					*s.webhook2.DeepCopy(),
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
							Name:              s.webhook1.Name,
							CodeReasonMessage: s.webhook1Error,
							FailureCount:      1,
						},
						{
							HookType: rolloutv1alpha1.PreCanaryStepHook,
							Name:     s.webhook2.Name, // current webhook
						},
					},
				}
				return rollout, rolloutRun
			},
			assertResult: func(done bool, retry time.Duration, err error) {
				s.Require().NoError(err)
				s.False(done)
				s.Equal(retryDefault, retry)
			},
			assertStatus: func(status *rolloutv1alpha1.RolloutRunStatus) {
				if s.NotNil(status.Error, "The webhook has failed, and the corresponding error should be set") {
					s.Equal(s.webhook2Error, *status.Error)
				}
				if s.Len(status.CanaryStatus.Webhooks, 2) {
					s.Equal(rolloutv1alpha1.RolloutWebhookStatus{
						State:             rolloutv1alpha1.WebhookCompleted,
						HookType:          rolloutv1alpha1.PreCanaryStepHook,
						Name:              s.webhook1.Name,
						CodeReasonMessage: s.webhook1Error,
						FailureCount:      1,
					}, status.CanaryStatus.Webhooks[0])
					s.Equal(rolloutv1alpha1.RolloutWebhookStatus{
						State:             rolloutv1alpha1.WebhookOnHold,
						HookType:          rolloutv1alpha1.PreCanaryStepHook,
						Name:              s.webhook2.Name,
						CodeReasonMessage: s.webhook2Error,
						FailureCount:      1,
					}, status.CanaryStatus.Webhooks[1])
				}
			},
		},
		{
			name: "webhook 3 failed once, but it is still running, no error",
			getObjects: func() (*rolloutv1alpha1.Rollout, *rolloutv1alpha1.RolloutRun) {
				rollout := s.rollout.DeepCopy()
				rolloutRun := s.rolloutRun.DeepCopy()
				rolloutRun.Spec.Webhooks = []rolloutv1alpha1.RolloutWebhook{
					*s.webhook3.DeepCopy(),
				}
				rolloutRun.Spec.Canary = &rolloutv1alpha1.RolloutRunCanaryStrategy{
					Targets: unimportantTargets,
				}
				rolloutRun.Status.CanaryStatus = &rolloutv1alpha1.RolloutRunStepStatus{
					State: StepPreCanaryStepHook,
				}
				return rollout, rolloutRun
			},
			assertResult: func(done bool, retry time.Duration, err error) {
				s.Require().NoError(err)
				s.False(done)
				s.Equal(retryDefault, retry)
			},
			assertStatus: func(status *rolloutv1alpha1.RolloutRunStatus) {
				s.Nil(status.Error, "This webhook has encountered a failure, but it is still running, no error will be reported")
				if s.Len(status.CanaryStatus.Webhooks, 1) {
					s.Equal(rolloutv1alpha1.RolloutWebhookStatus{
						State:             rolloutv1alpha1.WebhookRunning,
						HookType:          rolloutv1alpha1.PreCanaryStepHook,
						Name:              s.webhook3.Name,
						CodeReasonMessage: s.webhook3Error,
						FailureCount:      1,
					}, status.CanaryStatus.Webhooks[0])
				}
			},
		},
		{
			name: "webhook 1 finished, then webhook 2 will be started next time",
			getObjects: func() (*rolloutv1alpha1.Rollout, *rolloutv1alpha1.RolloutRun) {
				rollout := s.rollout.DeepCopy()
				rolloutRun := s.rolloutRun.DeepCopy()
				rolloutRun.Spec.Webhooks = []rolloutv1alpha1.RolloutWebhook{
					*s.webhook1.DeepCopy(),
					*s.webhook2.DeepCopy(),
				}
				rolloutRun.Spec.Canary = &rolloutv1alpha1.RolloutRunCanaryStrategy{
					Targets: unimportantTargets,
				}
				rolloutRun.Status.CanaryStatus = &rolloutv1alpha1.RolloutRunStepStatus{
					State: StepPreCanaryStepHook,
				}
				return rollout, rolloutRun
			},
			assertResult: func(done bool, retry time.Duration, err error) {
				s.Require().NoError(err)
				s.False(done)
				s.Equal(retryImmediately, retry)
			},
			assertStatus: func(status *rolloutv1alpha1.RolloutRunStatus) {
				s.Nil(status.Error)
				if s.Len(status.CanaryStatus.Webhooks, 2) {
					s.Equal(rolloutv1alpha1.RolloutWebhookStatus{
						State:             rolloutv1alpha1.WebhookCompleted,
						HookType:          rolloutv1alpha1.PreCanaryStepHook,
						Name:              s.webhook1.Name,
						CodeReasonMessage: s.webhook1Error,
						FailureCount:      1,
					}, status.CanaryStatus.Webhooks[0])
					s.Equal(rolloutv1alpha1.RolloutWebhookStatus{
						HookType: rolloutv1alpha1.PreCanaryStepHook,
						Name:     s.webhook2.Name,
					}, status.CanaryStatus.Webhooks[1])
				}
			},
		},
		{
			name: "no webhooks",
			getObjects: func() (*rolloutv1alpha1.Rollout, *rolloutv1alpha1.RolloutRun) {
				rollout := s.rollout.DeepCopy()
				rolloutRun := s.rolloutRun.DeepCopy()
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
			assertResult: func(done bool, retry time.Duration, err error) {
				s.Require().NoError(err)
				s.True(done)
				s.Equal(retryImmediately, retry)
			},
			assertStatus: func(status *rolloutv1alpha1.RolloutRunStatus) {
				s.Nil(status.Error)
				s.Empty(status.CanaryStatus.Webhooks)
			},
		},
	}

	s.runWebhookTestCases(hookType, tests)
}

func (s *webhookExecutorTestSuite) Test_webhook_PostCanaryHookStep() {
	hookType := rolloutv1alpha1.PostCanaryStepHook
	tests := []webhookTestCase{
		{
			name: "webhook 1 has failed but will be ignored, no error in status",
			getObjects: func() (*rolloutv1alpha1.Rollout, *rolloutv1alpha1.RolloutRun) {
				rollout := s.rollout.DeepCopy()
				rolloutRun := s.rolloutRun.DeepCopy()
				rolloutRun.Spec.Webhooks = []rolloutv1alpha1.RolloutWebhook{
					*s.webhook1.DeepCopy(),
				}
				rolloutRun.Spec.Canary = &rolloutv1alpha1.RolloutRunCanaryStrategy{
					Targets: unimportantTargets,
				}
				rolloutRun.Status.CanaryStatus = &rolloutv1alpha1.RolloutRunStepStatus{
					State: StepPostCanaryStepHook,
				}
				return rollout, rolloutRun
			},
			assertResult: func(done bool, retry time.Duration, err error) {
				s.Require().NoError(err)
				s.True(done)
				s.Equal(retryImmediately, retry)
			},
			assertStatus: func(status *rolloutv1alpha1.RolloutRunStatus) {
				s.Nil(status.Error, "This webhook has encountered a failure, but it will be ignored, no error will be reported.")
				if s.Len(status.CanaryStatus.Webhooks, 1) {
					s.Equal(rolloutv1alpha1.RolloutWebhookStatus{
						State:             rolloutv1alpha1.WebhookCompleted,
						HookType:          rolloutv1alpha1.PostCanaryStepHook,
						Name:              s.webhook1.Name,
						CodeReasonMessage: s.webhook1Error,
						FailureCount:      1,
					}, status.CanaryStatus.Webhooks[0])
				}
			},
		},
		{
			name: "webhook 2 has failed, error should be set",
			getObjects: func() (*rolloutv1alpha1.Rollout, *rolloutv1alpha1.RolloutRun) {
				rollout := s.rollout.DeepCopy()
				rolloutRun := s.rolloutRun.DeepCopy()
				rolloutRun.Spec.Webhooks = []rolloutv1alpha1.RolloutWebhook{
					*s.webhook2.DeepCopy(),
				}
				rolloutRun.Spec.Canary = &rolloutv1alpha1.RolloutRunCanaryStrategy{
					Targets: unimportantTargets,
				}
				rolloutRun.Status.CanaryStatus = &rolloutv1alpha1.RolloutRunStepStatus{
					State: StepPostCanaryStepHook,
				}
				return rollout, rolloutRun
			},
			assertResult: func(done bool, retry time.Duration, err error) {
				s.Require().NoError(err)
				s.False(done)
				s.Equal(retryDefault, retry)
			},
			assertStatus: func(status *rolloutv1alpha1.RolloutRunStatus) {
				if s.NotNil(status.Error, "The webhook has failed, and the corresponding error should be set.") {
					s.Equal(s.webhook2Error, *status.Error)
				}
				if s.Len(status.CanaryStatus.Webhooks, 1) {
					s.Equal(rolloutv1alpha1.RolloutWebhookStatus{
						State:             rolloutv1alpha1.WebhookOnHold,
						HookType:          rolloutv1alpha1.PostCanaryStepHook,
						Name:              s.webhook2.Name,
						CodeReasonMessage: s.webhook2Error,
						FailureCount:      1,
					}, status.CanaryStatus.Webhooks[0])
				}
			},
		},
	}

	s.runWebhookTestCases(hookType, tests)
}

func (s *webhookExecutorTestSuite) Test_webhook_PreBatchHookStep() {
	hookType := rolloutv1alpha1.PreBatchStepHook
	tests := []webhookTestCase{
		{
			name: "webhook 1 failed but ignored, no error in status",
			getObjects: func() (*rolloutv1alpha1.Rollout, *rolloutv1alpha1.RolloutRun) {
				rollout := s.rollout.DeepCopy()
				rolloutRun := s.rolloutRun.DeepCopy()
				rolloutRun.Spec.Webhooks = []rolloutv1alpha1.RolloutWebhook{
					*s.webhook1.DeepCopy(),
				}
				rolloutRun.Spec.Batch.Batches = []rolloutv1alpha1.RolloutRunStep{{
					Targets: unimportantTargets,
				}}

				return rollout, rolloutRun
			},
			assertResult: func(done bool, retry time.Duration, err error) {
				s.Require().NoError(err)
				s.True(done)
				s.Equal(retryImmediately, retry)
			},
			assertStatus: func(status *rolloutv1alpha1.RolloutRunStatus) {
				s.Nil(status.Error, "This webhook has encountered a failure, but it will be ignored, no error will be reported.")
				if s.Len(status.BatchStatus.Records[0].Webhooks, 1) {
					s.Equal(rolloutv1alpha1.RolloutWebhookStatus{
						State:             rolloutv1alpha1.WebhookCompleted,
						HookType:          hookType,
						Name:              s.webhook1.Name,
						CodeReasonMessage: s.webhook1Error,
						FailureCount:      1,
					}, status.BatchStatus.Records[0].Webhooks[0])
				}
			},
		},
		{
			name: "webhook 2 has failed, error should be set",
			getObjects: func() (*rolloutv1alpha1.Rollout, *rolloutv1alpha1.RolloutRun) {
				rollout := s.rollout.DeepCopy()
				rolloutRun := s.rolloutRun.DeepCopy()
				rolloutRun.Spec.Webhooks = []rolloutv1alpha1.RolloutWebhook{
					*s.webhook2.DeepCopy(),
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
			assertResult: func(done bool, retry time.Duration, err error) {
				s.Require().NoError(err)
				s.False(done)
				s.Equal(retryDefault, retry)
			},
			assertStatus: func(status *rolloutv1alpha1.RolloutRunStatus) {
				if s.NotNil(status.Error, "The webhook has failed, and the corresponding error should be set.") {
					s.Equal(s.webhook2Error, *status.Error)
				}
				if s.Len(status.BatchStatus.Records[0].Webhooks, 1) {
					s.Equal(rolloutv1alpha1.RolloutWebhookStatus{
						State:             rolloutv1alpha1.WebhookOnHold,
						HookType:          hookType,
						Name:              s.webhook2.Name,
						CodeReasonMessage: s.webhook2Error,
						FailureCount:      1,
					}, status.BatchStatus.Records[0].Webhooks[0])
				}
			},
		},
	}

	s.runWebhookTestCases(hookType, tests)
}

func (s *webhookExecutorTestSuite) Test_webhook_PostBatchHookStep() {
	hookType := rolloutv1alpha1.PostBatchStepHook
	tests := []webhookTestCase{
		{
			name: "webhook 1 failed but ignored, no error in status",
			getObjects: func() (*rolloutv1alpha1.Rollout, *rolloutv1alpha1.RolloutRun) {
				rollout := s.rollout.DeepCopy()
				rolloutRun := s.rolloutRun.DeepCopy()
				rolloutRun.Spec.Webhooks = []rolloutv1alpha1.RolloutWebhook{
					*s.webhook1.DeepCopy(),
				}
				rolloutRun.Spec.Batch.Batches = []rolloutv1alpha1.RolloutRunStep{{
					Targets: unimportantTargets,
				}}

				return rollout, rolloutRun
			},
			assertResult: func(done bool, retry time.Duration, err error) {
				s.Require().NoError(err)
				s.True(done)
				s.Equal(retryImmediately, retry)
			},
			assertStatus: func(status *rolloutv1alpha1.RolloutRunStatus) {
				s.Nil(status.Error, "This webhook has encountered a failure, but it will be ignored, no error will be reported.")
				if s.Len(status.BatchStatus.Records[0].Webhooks, 1) {
					s.Equal(rolloutv1alpha1.RolloutWebhookStatus{
						State:             rolloutv1alpha1.WebhookCompleted,
						HookType:          hookType,
						Name:              s.webhook1.Name,
						CodeReasonMessage: s.webhook1Error,
						FailureCount:      1,
					}, status.BatchStatus.Records[0].Webhooks[0])
				}
			},
		},
		{
			name: "webhook 2 has failed, error should be set",
			getObjects: func() (*rolloutv1alpha1.Rollout, *rolloutv1alpha1.RolloutRun) {
				rollout := s.rollout.DeepCopy()
				rolloutRun := s.rolloutRun.DeepCopy()
				rolloutRun.Spec.Webhooks = []rolloutv1alpha1.RolloutWebhook{
					*s.webhook2.DeepCopy(),
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
			assertResult: func(done bool, retry time.Duration, err error) {
				s.Require().NoError(err)
				s.False(done)
				s.Equal(retryDefault, retry)
			},
			assertStatus: func(status *rolloutv1alpha1.RolloutRunStatus) {
				if s.NotNil(status.Error, "The webhook has failed, and the corresponding error should be set.") {
					s.Equal(s.webhook2Error, *status.Error)
				}
				if s.Len(status.BatchStatus.Records[0].Webhooks, 1) {
					s.Equal(rolloutv1alpha1.RolloutWebhookStatus{
						State:             rolloutv1alpha1.WebhookOnHold,
						HookType:          hookType,
						Name:              s.webhook2.Name,
						CodeReasonMessage: s.webhook2Error,
						FailureCount:      1,
					}, status.BatchStatus.Records[0].Webhooks[0])
				}
			},
		},
	}

	s.runWebhookTestCases(hookType, tests)
}

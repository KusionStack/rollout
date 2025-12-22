/**
 * Copyright 2024 The KusionStack Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package webhook

import (
	"time"

	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/clock"
	"k8s.io/utils/ptr"
	rolloutv1alpha1 "kusionstack.io/kube-api/rollout/v1alpha1"

	"kusionstack.io/rollout/pkg/controllers/rolloutrun/webhook/probe"
)

const (
	testWebhookKey = types.UID("test-rollout-run-uid")
)

var (
	testWebhook = rolloutv1alpha1.RolloutWebhook{
		Name: "test-webhook",
		ClientConfig: rolloutv1alpha1.WebhookClientConfig{
			TimeoutSeconds: 2,
			PeriodSeconds:  1,
		},
		FailurePolicy: rolloutv1alpha1.Fail,
	}

	testWebhookReview = rolloutv1alpha1.RolloutWebhookReview{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-webhook",
			Namespace: "default",
		},
		Spec: rolloutv1alpha1.RolloutWebhookReviewSpec{
			Kind:        "Rollout",
			RolloutName: "test-rollout",
			RolloutID:   "test-rollout-run",
			HookType:    rolloutv1alpha1.PreBatchStepHook,
			Batch: &rolloutv1alpha1.RolloutWebhookReviewBatch{
				BatchIndex: 1,
			},
		},
	}
)

type fakeProber struct {
	resultCode string
}

func newFakeProber(code string) *fakeProber {
	return &fakeProber{
		resultCode: code,
	}
}

func (p *fakeProber) Probe(_ *rolloutv1alpha1.RolloutWebhookReview) probe.Result {
	return probe.Result{
		Code: p.resultCode,
	}
}

func newTestWorker(manager *manager, fakeProber probe.WebhookProber) *worker {
	w := newWorker(manager, testWebhookKey, testWebhook, testWebhookReview)
	w.prober = fakeProber
	return w
}

type workerTestSuite struct {
	suite.Suite

	manager *manager
	clock   *clock.FakeClock
}

func (s *workerTestSuite) SetupSuite() {
	s.clock = clock.NewFakeClock(time.Now())
	s.manager = NewManager(s.clock).(*manager)
}

func (s *workerTestSuite) Test_DoProbe_Once() {
	tests := []struct {
		name          string
		getWorker     func() *worker
		wantKeepGoing bool
		wantResult    Result
	}{
		{
			name: "webhook probe return ok, worker is stopped",
			getWorker: func() *worker {
				prober := newFakeProber(rolloutv1alpha1.WebhookReviewCodeOK)
				w := newTestWorker(s.manager, prober)
				return w
			},
			wantKeepGoing: false,
			wantResult: Result{
				State:    rolloutv1alpha1.WebhookCompleted,
				HookType: testWebhookReview.Spec.HookType,
				Name:     testWebhookReview.Name,
				CodeReasonMessage: rolloutv1alpha1.CodeReasonMessage{
					Code: rolloutv1alpha1.WebhookReviewCodeOK,
				},
				StartTime:  ptr.To(metav1.NewTime(s.clock.Now())),
				FinishTime: ptr.To(metav1.NewTime(s.clock.Now())),
			},
		},
		{
			name: "webhook probe return progressing, worker is still running",
			getWorker: func() *worker {
				prober := newFakeProber(rolloutv1alpha1.WebhookReviewCodeProcessing)
				w := newTestWorker(s.manager, prober)
				return w
			},
			wantKeepGoing: true,
			wantResult: Result{
				State:    rolloutv1alpha1.WebhookRunning,
				HookType: testWebhookReview.Spec.HookType,
				Name:     testWebhookReview.Name,
				CodeReasonMessage: rolloutv1alpha1.CodeReasonMessage{
					Code: rolloutv1alpha1.WebhookReviewCodeProcessing,
				},
				StartTime: ptr.To(metav1.NewTime(s.clock.Now())),
			},
		},
		{
			name: "prober return error, webhook policy is failed, then worker is onhold",
			getWorker: func() *worker {
				prober := newFakeProber(rolloutv1alpha1.WebhookReviewCodeError)
				w := newTestWorker(s.manager, prober)
				return w
			},
			wantKeepGoing: true,
			wantResult: Result{
				State:    rolloutv1alpha1.WebhookOnHold,
				HookType: testWebhookReview.Spec.HookType,
				Name:     testWebhookReview.Name,
				CodeReasonMessage: rolloutv1alpha1.CodeReasonMessage{
					Code: rolloutv1alpha1.WebhookReviewCodeError,
				},
				FailureCount: 1,
				StartTime:    ptr.To(metav1.NewTime(s.clock.Now())),
			},
		},
		{
			name: "prober return error, webhook policy is ignore, then worker is stopped",
			getWorker: func() *worker {
				prober := newFakeProber(rolloutv1alpha1.WebhookReviewCodeError)
				w := newTestWorker(s.manager, prober)
				w.failurePolicy = rolloutv1alpha1.Ignore
				return w
			},
			wantKeepGoing: false,
			wantResult: Result{
				State:    rolloutv1alpha1.WebhookCompleted,
				HookType: testWebhookReview.Spec.HookType,
				Name:     testWebhookReview.Name,
				CodeReasonMessage: rolloutv1alpha1.CodeReasonMessage{
					Code: rolloutv1alpha1.WebhookReviewCodeError,
				},
				FailureCount: 1,
				StartTime:    ptr.To(metav1.NewTime(s.clock.Now())),
				FinishTime:   ptr.To(metav1.NewTime(s.clock.Now())),
			},
		},
	}
	for i := range tests {
		tt := tests[i]
		s.Run(tt.name, func() {
			w := tt.getWorker()
			gotKeepGoing := w.doProbe()
			s.Equal(tt.wantKeepGoing, gotKeepGoing, "keep going not match")
			gotResult := w.Result()
			s.Equal(tt.wantResult, gotResult)
		})
	}
}

func (s *workerTestSuite) Test_DoProbe_MultiTimes() {
	type pipeline struct {
		modifyWorker  func(w *worker)
		wantKeepGoing bool
		checkResut    func(result Result)
	}
	tests := []struct {
		name      string
		getWorker func() *worker
		pipeline  []pipeline
	}{
		{
			name: "happy path",
			getWorker: func() *worker {
				prober := newFakeProber(rolloutv1alpha1.WebhookReviewCodeProcessing)
				return newTestWorker(s.manager, prober)
			},
			pipeline: []pipeline{
				{
					wantKeepGoing: true,
					checkResut: func(result Result) {
						s.Equal(rolloutv1alpha1.WebhookRunning, result.State)
						s.Equal(rolloutv1alpha1.WebhookReviewCodeProcessing, result.Code)
						s.EqualValues(0, result.FailureCount)
					},
				},
				{
					modifyWorker: func(w *worker) {
						w.prober = newFakeProber(rolloutv1alpha1.WebhookReviewCodeOK)
					},
					wantKeepGoing: false,
					checkResut: func(result Result) {
						s.Equal(rolloutv1alpha1.WebhookCompleted, result.State)
						s.Equal(rolloutv1alpha1.WebhookReviewCodeOK, result.Code)
						s.EqualValues(0, result.FailureCount)
					},
				},
			},
		},
		{
			name: "error occur, worker is onhold",
			getWorker: func() *worker {
				prober := newFakeProber(rolloutv1alpha1.WebhookReviewCodeError)
				w := newTestWorker(s.manager, prober)
				w.failureThreshold = 2
				return w
			},
			pipeline: []pipeline{
				{
					wantKeepGoing: true,
					checkResut: func(result Result) {
						s.Equal(rolloutv1alpha1.WebhookRunning, result.State)
						s.Equal(rolloutv1alpha1.WebhookReviewCodeError, result.Code)
						s.EqualValues(1, result.FailureCount)
					},
				},
				{
					wantKeepGoing: true,
					checkResut: func(result Result) {
						s.Equal(rolloutv1alpha1.WebhookOnHold, result.State)
						s.Equal(rolloutv1alpha1.WebhookReviewCodeError, result.Code)
						s.EqualValues(2, result.FailureCount)
					},
				},
				{
					modifyWorker: func(w *worker) {
						// manual trigger
						w.onHold = false
						w.failureCount = 0
					},
					wantKeepGoing: true,
					checkResut: func(result Result) {
						s.Equal(rolloutv1alpha1.WebhookRunning, result.State)
						s.Equal(rolloutv1alpha1.WebhookReviewCodeError, result.Code)
						s.EqualValues(3, result.FailureCount)
					},
				},
				{
					wantKeepGoing: true,
					checkResut: func(result Result) {
						s.Equal(rolloutv1alpha1.WebhookOnHold, result.State)
						s.Equal(rolloutv1alpha1.WebhookReviewCodeError, result.Code)
						s.EqualValues(4, result.FailureCount)
					},
				},
			},
		},
	}
	for i := range tests {
		tt := tests[i]
		s.Run(tt.name, func() {
			w := tt.getWorker()
			for _, p := range tt.pipeline {
				if p.modifyWorker != nil {
					p.modifyWorker(w)
				}
				gotKeepGoing := w.doProbe()
				s.Equal(p.wantKeepGoing, gotKeepGoing, "keep going not match")
				p.checkResut(w.Result())
			}
		})
	}
}

func (s *workerTestSuite) Test_Run() {
	fakeP := newFakeProber(rolloutv1alpha1.WebhookReviewCodeProcessing)
	worker := newTestWorker(s.manager, fakeP)

	go worker.run()
	defer worker.Stop()

	time.Sleep(time.Second)

	result := worker.Result()
	s.Equal(rolloutv1alpha1.WebhookRunning, result.State)
	s.Equal(rolloutv1alpha1.WebhookReviewCodeProcessing, result.Code)

	worker.prober = newFakeProber(rolloutv1alpha1.WebhookReviewCodeError)
	time.Sleep(2 * time.Second)
	result = worker.Result()
	s.Equal(rolloutv1alpha1.WebhookOnHold, result.State)
	s.Equal(rolloutv1alpha1.WebhookReviewCodeError, result.Code)

	worker.prober = newFakeProber(rolloutv1alpha1.WebhookReviewCodeOK)
	worker.Retry()
	time.Sleep(1 * time.Second)
	result = worker.Result()
	s.Equal(rolloutv1alpha1.WebhookCompleted, result.State)
	s.Equal(rolloutv1alpha1.WebhookReviewCodeOK, result.Code)
}

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
	"sync"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/utils/ptr"
	rolloutv1alpha1 "kusionstack.io/kube-api/rollout/v1alpha1"

	"kusionstack.io/rollout/pkg/controllers/rolloutrun/webhook/probe"
	"kusionstack.io/rollout/pkg/controllers/rolloutrun/webhook/probe/http"
	"kusionstack.io/rollout/pkg/utils"
)

type Result rolloutv1alpha1.RolloutWebhookStatus

// WebhookWorker handles the periodic webhook works.
type WebhookWorker interface {
	// Result returns the last probe result.
	Result() Result

	// Retry manually triggers the webhook probe.
	Retry()

	// Stop stops the probe worker. and clean up the result in cache.
	// The worker handles cleanup and removes itself from its manager.
	// It's safe to call Stop multiple times.
	Stop()
}

// worker handles the periodic webhook works.
type worker struct {
	stopOnce sync.Once
	// Channel for stopping the probe.
	stopCh chan struct{}
	// Channel for triggering the probe manually.
	retryTriggerCh chan struct{}

	webhookManager *manager

	key types.UID

	review rolloutv1alpha1.RolloutWebhookReview

	periodDuration time.Duration

	prober     probe.WebhookProber
	lastResult Result
	resultLock sync.RWMutex

	failureThreshold  int
	failurePolicy     rolloutv1alpha1.FailurePolicyType
	failureCount      int
	totalFailureCount int

	onHold bool
}

func newWorker(m *manager, key types.UID, webhook rolloutv1alpha1.RolloutWebhook, review rolloutv1alpha1.RolloutWebhookReview) *worker {
	w := &worker{
		stopOnce:         sync.Once{},
		stopCh:           make(chan struct{}),
		retryTriggerCh:   make(chan struct{}, 1),
		webhookManager:   m,
		key:              key,
		review:           review,
		periodDuration:   getWorkerPeriod(webhook.ClientConfig.PeriodSeconds),
		prober:           newProber(webhook),
		failureThreshold: int(webhook.FailureThreshold),
		failurePolicy:    webhook.FailurePolicy,
	}

	// init result
	w.lastResult = Result{
		Name:      w.review.Name,
		HookType:  w.review.Spec.HookType,
		State:     rolloutv1alpha1.WebhookRunning,
		StartTime: ptr.To(w.now()),
		CodeReasonMessage: rolloutv1alpha1.CodeReasonMessage{
			Code:    rolloutv1alpha1.WebhookReviewCodeProcessing,
			Reason:  "Processing",
			Message: "webhook is running",
		},
	}
	return w
}

func (w *worker) Result() Result {
	w.resultLock.RLock()
	defer w.resultLock.RUnlock()
	return w.lastResult
}

func (w *worker) Retry() {
	if w.onHold {
		w.resultLock.Lock()
		w.lastResult.State = rolloutv1alpha1.WebhookRunning
		w.resultLock.Unlock()

		w.retryTriggerCh <- struct{}{}
	}
}

// stop stops the probe worker. and clean up the result in cache.
// The worker handles cleanup and removes itself from its manager.
// It is safe to call stop multiple times.
func (w *worker) Stop() {
	w.stopOnce.Do(func() {
		close(w.stopCh)
		w.webhookManager.removeWorker(w.key)
	})
}

func getWorkerPeriod(sec int32) time.Duration {
	if sec == 0 {
		return 10 * time.Second
	}
	return time.Second * time.Duration(sec)
}

func (w *worker) run() {
	probeTicker := time.NewTicker(w.periodDuration)

	defer func() {
		probeTicker.Stop()
	}()

probeLoop:
	for w.doProbe() {
		select {
		case <-w.stopCh:
			break probeLoop
		case <-probeTicker.C:
			// continue
		case <-w.retryTriggerCh:
			// reset
			w.onHold = false
			w.failureCount = 0
			// continue
		}
	}
}

func (w *worker) doProbe() (keepGoing bool) {
	defer func() {
		recover() //nolint
	}()
	defer runtime.HandleCrash(func(_ interface{}) {
		keepGoing = true
	})

	keepGoing = true

	if w.onHold {
		// do nothing
		return keepGoing
	}

	probeResult := w.prober.Probe(&w.review)
	result := Result{
		HookType:          w.review.Spec.HookType,
		Name:              w.review.Name,
		State:             rolloutv1alpha1.WebhookRunning,
		CodeReasonMessage: probeResult,
	}
	if w.lastResult.StartTime != nil {
		result.StartTime = w.lastResult.StartTime
	}

	switch result.Code {
	case rolloutv1alpha1.WebhookReviewCodeOK:
		// webhook success, stop probe loop
		result.State = rolloutv1alpha1.WebhookCompleted
		keepGoing = false
	case rolloutv1alpha1.WebhookReviewCodeError:
		w.failureCount++
		w.totalFailureCount++
		result.FailureCount = int32(w.totalFailureCount)
		if w.failureCount >= w.failureThreshold {
			if w.failurePolicy == rolloutv1alpha1.Ignore {
				// ignore webhook failure, stop probe loop
				result.State = rolloutv1alpha1.WebhookCompleted
				keepGoing = false
			} else {
				// waiting for user confirm
				w.onHold = true
				result.State = rolloutv1alpha1.WebhookOnHold
			}
		}
	default:
		// progressing
		result.State = rolloutv1alpha1.WebhookRunning
	}

	if result.State == rolloutv1alpha1.WebhookCompleted {
		result.FinishTime = ptr.To(w.now())
	}

	// shorten long message
	result.Message = utils.Abbreviate(result.Message, 1024)

	func() {
		// change result with lock
		w.resultLock.Lock()
		defer w.resultLock.Unlock()
		w.lastResult = result
	}()

	return keepGoing
}

// We need special handling for time. The worker Result contains metav1.Time which serializes
// using time.RFC3339 format, losing nanoseconds precision. If we use metav1.Now() directly, it
// would cause inconsistency in precision between locally saved time and serialized time from
// cluster, leading to unnecessary updates
func (w *worker) now() metav1.Time {
	now := metav1.NewTime(w.webhookManager.clock.Now())
	data, _ := now.MarshalJSON()
	now.UnmarshalJSON(data) //nolint
	return now
}

func newProber(webhook rolloutv1alpha1.RolloutWebhook) probe.WebhookProber {
	provider := ptr.Deref(webhook.Provider, "")
	if len(provider) > 0 {
		panic("webhook provider is not supported now")
	}

	return http.New(webhook.ClientConfig)
}

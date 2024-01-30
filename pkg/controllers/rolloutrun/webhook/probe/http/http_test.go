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

package http

import (
	"testing"

	"github.com/stretchr/testify/assert"

	rolloutv1alpha1 "kusionstack.io/rollout/apis/rollout/v1alpha1"
	"kusionstack.io/rollout/pkg/controllers/rolloutrun/webhook/probe"
)

func Test_httpProber_Probe(t *testing.T) {
	testServer := NewTestHTTPServer()
	defer testServer.Close()

	tests := []struct {
		name    string
		url     string
		payload *rolloutv1alpha1.RolloutWebhookReview
		want    probe.Result
	}{
		{
			name: "invalid url",
			url:  "invalid",
			want: probe.Result{
				Code:    rolloutv1alpha1.WebhookReviewCodeError,
				Reason:  "DoRequestError",
				Message: `Post "invalid": unsupported protocol scheme ""`,
			},
		},
		{
			name: "404",
			url:  testServer.URL + "/notFound",
			want: probe.Result{
				Code:    rolloutv1alpha1.WebhookReviewCodeError,
				Reason:  "HTTPResponseError",
				Message: `HTTP probe failed with statuscode: 404, body: ""`,
			},
		},
		{
			name: "OK",
			url:  testServer.URL + "/ok",
			payload: &rolloutv1alpha1.RolloutWebhookReview{
				Spec: rolloutv1alpha1.RolloutWebhookReviewSpec{
					RolloutName: "test-rollout-name",
				},
			},
			want: probe.Result{
				Code: rolloutv1alpha1.WebhookReviewCodeOK,
			},
		},
		{
			name: "Progressing",
			url:  testServer.URL + "/progressing",
			payload: &rolloutv1alpha1.RolloutWebhookReview{
				Spec: rolloutv1alpha1.RolloutWebhookReviewSpec{
					RolloutName: "test-rollout-name",
				},
			},
			want: probe.Result{
				Code: rolloutv1alpha1.WebhookReviewCodeProcessing,
			},
		},
	}
	for i := range tests {
		tt := tests[i]
		t.Run(tt.name, func(t *testing.T) {
			p := New(rolloutv1alpha1.WebhookClientConfig{
				URL: tt.url,
			})
			got := p.Probe(tt.payload)
			assert.Equal(t, tt.want, got)
		})
	}
}

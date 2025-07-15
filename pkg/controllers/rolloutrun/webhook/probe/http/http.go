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
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"time"

	"k8s.io/client-go/transport"

	rolloutv1alpha1 "kusionstack.io/kube-api/rollout/v1alpha1"
	"kusionstack.io/rollout/pkg/controllers/rolloutrun/webhook/probe"
)

var defaultTimeout = 10 * time.Second

const (
	internalErrorReason   = "InternalError"
	doRequesttErrorReason = "DoRequestError"
)

// New creates Prober that will skip TLS verification while probing.
func New(config rolloutv1alpha1.WebhookClientConfig) probe.WebhookProber {
	transportCfg := &transport.Config{
		TLS: transport.TLSConfig{
			CAData: config.CABundle,
		},
		DisableCompression: true,
		UserAgent:          "kusionstack-rollout-http-prober",
	}

	if len(config.CABundle) == 0 {
		transportCfg.TLS.Insecure = true
	}
	rt, err := transport.New(transportCfg)
	if err != nil {
		// This is a non-recoverable error, so throw an error.
		panic(err)
	}

	timeout := defaultTimeout
	if config.TimeoutSeconds != 0 {
		timeout = time.Duration(config.TimeoutSeconds) * time.Second
	}

	client := &http.Client{
		Timeout:   timeout,
		Transport: rt,
	}

	return &httpProber{
		url:    config.URL,
		client: client,
	}
}

type httpProber struct {
	url    string
	client *http.Client
}

// Probe returns a ProbeRunner capable of running an HTTP check.
func (p *httpProber) Probe(payload *rolloutv1alpha1.RolloutWebhookReview) probe.Result {
	return DoHTTPProbe(p.url, payload, p.client)
}

// HTTPClientInterface is an interface for making HTTP requests, that returns a response and error.
type HTTPClientInterface interface {
	Do(req *http.Request) (*http.Response, error)
}

// DoHTTPProbe checks if a POST request to the url succeeds.
// If the HTTP response code is successful (i.e. code == 200 or 201), it returns Success.
// If the HTTP response code is unsuccessful or HTTP communication fails, it returns Failure.
// This is exported because some other packages may want to do direct HTTP probes.
func DoHTTPProbe(url string, payload *rolloutv1alpha1.RolloutWebhookReview, client HTTPClientInterface) probe.Result {
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return errToResult(err, internalErrorReason)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(bodyBytes))
	if err != nil {
		// Convert errors into failures to catch timeouts.
		return errToResult(err, internalErrorReason)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	res, err := client.Do(req)
	if err != nil {
		// Convert errors into failures to catch timeouts.
		return errToResult(err, doRequesttErrorReason)
	}
	defer res.Body.Close()

	// read body
	b, err := io.ReadAll(res.Body)
	if err != nil {
		return errToResult(err, internalErrorReason)
	}

	if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusCreated {
		return probe.Result{
			Code:    rolloutv1alpha1.WebhookReviewCodeError,
			Reason:  "HTTPResponseError",
			Message: fmt.Sprintf("HTTP probe failed with statuscode: %d, body: %q", res.StatusCode, string(b)),
		}
	}

	// unmarshal to webhookReview
	respBody := rolloutv1alpha1.RolloutWebhookReview{}
	err = json.Unmarshal(b, &respBody)
	if err != nil {
		return errToResult(err, internalErrorReason)
	}

	return respBody.Status.CodeReasonMessage
}

func errToResult(err error, reason string) probe.Result {
	return probe.Result{
		Code:    rolloutv1alpha1.WebhookReviewCodeError,
		Reason:  reason,
		Message: err.Error(),
	}
}

// NewTestHTTPServer mock http server
func NewTestHTTPServer() *httptest.Server {
	return httptest.NewServer(testHTTPHandler())
}

// nolint
func testHTTPHandler() http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		path := request.URL.Path
		if path != "/ok" && path != "/progressing" && path != "/error" {
			writer.WriteHeader(http.StatusNotFound)
			return
		}

		body, err := io.ReadAll(request.Body)
		defer request.Body.Close()
		if err != nil {
			http.Error(writer, err.Error(), http.StatusInternalServerError)
			return
		}

		writer.Header().Set("Content-Type", "application/json")
		review := &rolloutv1alpha1.RolloutWebhookReview{}
		err = json.Unmarshal(body, &review)
		if err != nil {
			http.Error(writer, err.Error(), http.StatusBadRequest)
			return
		}

		switch path {
		case "/progressing":
			writer.WriteHeader(201)
			review.Status.Code = rolloutv1alpha1.WebhookReviewCodeProcessing
		case "/ok":
			writer.WriteHeader(200)
			review.Status.Code = rolloutv1alpha1.WebhookReviewCodeOK
		case "/error":
			writer.WriteHeader(200)
			review.Status.Code = rolloutv1alpha1.WebhookReviewCodeError
			review.Status.Reason = "TestHTTPServer"
		}
		respBody, _ := json.Marshal(review)
		writer.Write(respBody)
	}
}

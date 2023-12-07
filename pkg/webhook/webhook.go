package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"

	rolloutv1alpha1 "kusionstack.io/rollout/apis/rollout/v1alpha1"
	"kusionstack.io/rollout/pkg/utils"
)

const (
	defaultWebhookTimeoutSeconds int32 = 10
)

// RunRolloutWebhook process webhook
func RunRolloutWebhook(ctx context.Context, webhook *rolloutv1alpha1.RolloutWebhook, payload *rolloutv1alpha1.RolloutWebhookReview) (*rolloutv1alpha1.RolloutWebhookReviewStatus, error) {
	var (
		err    error
		status *rolloutv1alpha1.RolloutWebhookReviewStatus
	)

	// log
	logger := logr.FromContext(ctx)
	logger.Info(fmt.Sprintf("RunRolloutWebhook start to process. name=%s", webhook.Name))
	defer logger.Info(
		fmt.Sprintf(
			"RunRolloutWebhook process finished. name=%s, status=%v", webhook.Name, status,
		),
	)

	// todo provider
	if webhook.Provider != nil &&
		len(*webhook.Provider) > 0 {
		status := &rolloutv1alpha1.RolloutWebhookReviewStatus{
			CodeReasonMessage: rolloutv1alpha1.CodeReasonMessage{
				Code:    rolloutv1alpha1.WebhookReviewCodeError,
				Reason:  "ProviderIsNotSupportedYet",
				Message: "RunRolloutWebhook failed since provider is not supported yet",
			},
		}
		return status, nil
	}

	clientConfig := webhook.ClientConfig

	var requestUrl *url.URL
	if requestUrl, err = url.Parse(clientConfig.URL); err != nil {
		return nil, errors.Wrapf(
			err,
			"RunRolloutWebhook failed since url(%s) is invalid", clientConfig.URL,
		)
	}

	var client *http.Client
	client, err = utils.ClientManagerSingleton.Client(
		&utils.ClientConfig{CABundle: clientConfig.CABundle},
	)
	if err != nil {
		return nil, fmt.Errorf(
			"RunRolloutWebhook failed "+
				"since get client failed, clientConfig=%v err=[%v]", clientConfig, err,
		)
	}

	// payload
	var requestBody []byte
	requestBody, err = json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf(
			"RunRolloutWebhook failed since marshal payload failed, err=[%v]", err,
		)
	}

	var req *http.Request
	req, err = http.NewRequestWithContext(
		ctx, "POST", requestUrl.String(), bytes.NewBuffer(requestBody),
	)
	if err != nil {
		return nil, fmt.Errorf(
			"RunRolloutWebhook failed since new request failed, err=[%v]", err,
		)
	}

	// timeout
	timeout := clientConfig.TimeoutSeconds
	if timeout <= 0 {
		timeout = defaultWebhookTimeoutSeconds
	}
	requestCtx, cancel := context.WithTimeout(
		req.Context(), time.Duration(timeout)*time.Second,
	)
	defer cancel()

	// header
	req.Header.Set("Content-Type", "application/json")

	// do
	var response *http.Response
	response, err = client.Do(req.WithContext(requestCtx))
	if err != nil {
		return nil, fmt.Errorf(
			"RunRolloutWebhook failed since request failed, err=[%v]", err,
		)
	}
	defer response.Body.Close()

	// handle response
	if response.StatusCode > 202 ||
		response.StatusCode < 200 {
		return nil, fmt.Errorf(
			"RunRolloutWebhook failed "+
				"since response statusCode error, code=%d", response.StatusCode,
		)
	}

	var body []byte
	body, err = io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf(
			"RunRolloutWebhook failed since read body failed, err=[%v]", err,
		)
	}

	rolloutWebhookReviewStatus := &rolloutv1alpha1.RolloutWebhookReviewStatus{}
	if err = json.Unmarshal(body, rolloutWebhookReviewStatus); err != nil {
		return nil, fmt.Errorf(
			"RunRolloutWebhook failed "+
				"since unmarshal body failed, body=%s, err=[%v]", string(body), err,
		)
	}

	return rolloutWebhookReviewStatus, err
}

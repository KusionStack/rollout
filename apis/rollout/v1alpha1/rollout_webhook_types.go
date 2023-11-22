// Copyright 2023 The KusionStack Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type RolloutWebhook struct {
	// Name is the identity of webhook
	Name string `json:"name,omitempty"`
	// HookTypes defines when to communicate with the hook, specifies the types of events
	// that trigger the webhook.
	// Required
	HookTypes []HookType `json:"hookTypes,omitempty"`
	// ClientConfig defines how to communicate with the hook.
	// Required
	ClientConfig WebhookClientConfig `json:"clientConfig,omitempty"`
	// Minimum consecutive successes for the probe to be considered successful after having failed.
	// Defaults to 1. Must be 1 for liveness and startup. Minimum value is 1.
	// +optional
	SuccessThreshold int32 `json:"successThreshold,omitempty" protobuf:"varint,5,opt,name=successThreshold"`
	// Minimum consecutive failures for the probe to be considered failed after having succeeded.
	// Defaults to 3. Minimum value is 1.
	// +optional
	FailureThreshold int32 `json:"failureThreshold,omitempty" protobuf:"varint,6,opt,name=failureThreshold"`
	// FailurePolicy defines how unrecognized errors from the admission endpoint are handled -
	// allowed values are Ignore or Fail. Defaults to Ignore.
	// +optional
	FailurePolicy *FailurePolicyType `json:"failurePolicy,omitempty"`
	// Properties provide additional data for webhook.
	// +optional
	Properties map[string]string `json:"properties,omitempty"`
	// By default, rollout communicates with the webhook through the structure RolloutWebhookReview.
	// If provider is set, then the protocol of the interaction will be determined by the provider
	// +optional
	Provider *string `json:"provider,omitempty"`
}

// FailurePolicyType specifies a failure policy that defines how unrecognized errors from the admission endpoint are handled.
type FailurePolicyType string

const (
	// Ignore means that an error calling the webhook is ignored.
	Ignore FailurePolicyType = "Ignore"
	// Fail means that an error calling the webhook causes the admission to fail.
	Fail FailurePolicyType = "Fail"
)

// WebhookClientConfig contains the information to make a TLS
// connection with the webhook
type WebhookClientConfig struct {
	// `url` gives the location of the webhook, in standard URL form
	// (`scheme://host:port/path`). Exactly one of `url` or `service`
	// must be specified.
	//
	// The `host` should not refer to a service running in the cluster; use
	// the `service` field instead. The host might be resolved via external
	// DNS in some apiservers (e.g., `kube-apiserver` cannot resolve
	// in-cluster DNS as that would be a layering violation). `host` may
	// also be an IP address.
	//
	// Please note that using `localhost` or `127.0.0.1` as a `host` is
	// risky unless you take great care to run this webhook on all hosts
	// which run an apiserver which might need to make calls to this
	// webhook. Such installs are likely to be non-portable, i.e., not easy
	// to turn up in a new cluster.
	//
	// The scheme must be "https"; the URL must begin with "https://".
	//
	// A path is optional, and if present may be any string permissible in
	// a URL. You may use the path to pass an arbitrary string to the
	// webhook, for example, a cluster identifier.
	//
	// Attempting to use a user or basic auth e.g. "user:password@" is not
	// allowed. Fragments ("#...") and query parameters ("?...") are not
	// allowed, either.
	URL string `json:"url,omitempty" protobuf:"bytes,3,opt,name=url"`

	// `caBundle` is a PEM encoded CA bundle which will be used to validate the webhook's server certificate.
	// If unspecified, system trust roots on the apiserver are used.
	// +optional
	CABundle []byte `json:"caBundle,omitempty" protobuf:"bytes,2,opt,name=caBundle"`

	// TimeoutSeconds specifies the timeout for this webhook. After the timeout passes,
	// the webhook call will be ignored or the API call will fail based on the
	// failure policy.
	// Default to 10 seconds.
	// +optional
	TimeoutSeconds int32 `json:"timeoutSeconds,omitempty"`

	// How often (in seconds) to perform the probe.
	// Default to 10 seconds. Minimum value is 1.
	// +optional
	PeriodSeconds int32 `json:"periodSeconds,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:skipversion

type RolloutWebhookReview struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RolloutWebhookReviewSpec   `json:"spec,omitempty"`
	Status RolloutWebhookReviewStatus `json:"status,omitempty"`
}

type RolloutWebhookReviewSpec struct {
	// Rollout Name
	RolloutName string `json:"rolloutName,omitempty"`
	// Rollout Namespace
	RolloutNamespace string            `json:"rolloutNamespace,omitempty"`
	RolloutID        string            `json:"rolloutID,omitempty"`
	HookType         HookType          `json:"hookType,omitempty"`
	BatchIndex       int32             `json:"batchIndex,omitempty"`
	Properties       map[string]string `json:"properties,omitempty"`
}

type HookType string

const (
	HookTypePreRollout    HookType = "PreRollout"
	HookTypePostRollout   HookType = "PostRollout"
	HookTypePreBatchStep  HookType = "PreBatchStep"
	HookTypePostBatchStep HookType = "PostBatchStep"
)

type RolloutWebhookReviewStatus struct {
	Code    WebhookReviewCode `json:"code,omitempty"`
	Reason  string            `json:"reason,omitempty"`
	Message string            `json:"message,omitempty"`
}

type WebhookReviewCode string

const (
	WebhookReviewCodeOK         WebhookReviewCode = "OK"
	WebhookReviewCodeError      WebhookReviewCode = "Error"
	WebhookReviewCodeProcessing WebhookReviewCode = "Processing"
)

/**
 * Copyright 2024 The KusionStack Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     https://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package generic

import (
	"context"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"
	"kusionstack.io/kube-utils/controller/mixin"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/runtime/inject"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"kusionstack.io/rollout/pkg/utils"
)

type GenericAdmissionHandler struct {
	*mixin.WebhookAdmissionHandlerMixin
	webhookType string
	gk          schema.GroupKind
	delegate    admission.Handler
}

var _ admission.Handler = &GenericAdmissionHandler{}

func NewAdmissionHandler(webhookType string, gk schema.GroupKind, handler admission.Handler) *GenericAdmissionHandler {
	return &GenericAdmissionHandler{
		webhookType:                  webhookType,
		gk:                           gk,
		delegate:                     handler,
		WebhookAdmissionHandlerMixin: mixin.NewWebhookHandlerMixin(),
	}
}

// Handle handles admission requests.
// It will only handle Pod creation and update. It will query the workload that manages it via the pod's ownerReference,
// and then apply the processing label from the workload onto the pod. In special cases where the pod's ownerReference
// is a ReplicaSet, it will continue to query its ownerReference to find the corresponding Deployment workload.
func (h *GenericAdmissionHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	if ptr.Deref(req.DryRun, false) {
		return admission.Allowed("dry run")
	}

	logger := h.Logger.WithValues(
		"kind", req.Kind.Kind,
		"key", utils.AdmissionRequestObjectKeyString(req),
		"op", req.Operation,
		"type", h.webhookType,
	)
	ctx = logr.NewContext(ctx, logger)

	logger.V(4).Info("webhook handler start")
	defer logger.V(4).Info("webhook handler end")

	if req.Kind.Group != h.gk.Group || req.Kind.Kind != h.gk.Kind {
		return admission.Allowed("not handled")
	}
	return h.delegate.Handle(ctx, req)
}

// InjectDecoder implements admission.DecoderInjector.
func (m *GenericAdmissionHandler) InjectDecoder(d *admission.Decoder) error {
	m.Decoder = d
	admission.InjectDecoderInto(d, m.delegate) // nolint
	return nil
}

// InjectLogger implements inject.Logger.
func (m *GenericAdmissionHandler) InjectLogger(l logr.Logger) error {
	m.Logger = l
	inject.LoggerInto(l, m.delegate) //nolint
	return nil
}

// InjectClient implements inject.Client.
func (m *GenericAdmissionHandler) InjectClient(c client.Client) error {
	m.Client = c
	inject.ClientInto(c, m.delegate) //nolint
	return nil
}

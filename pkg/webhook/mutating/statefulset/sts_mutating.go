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

package statefulset

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/go-logr/logr"
	admissionv1 "k8s.io/api/admission/v1"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"
	"kusionstack.io/kube-utils/controller/mixin"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"kusionstack.io/rollout/pkg/webhook/generic"
	"kusionstack.io/rollout/pkg/workload"
)

// +kubebuilder:webhook:path=/webhooks/mutating/statefulset,mutating=true,failurePolicy=fail,sideEffects=None,admissionReviewVersions=v1;v1beta1,groups="apps",resources=statefulsets,verbs=update,versions=v1,name=statefulset.apps.k8s.io

const WebhookInitialzierName = "mutate-apps"

func NewMutatingHandlers(_ manager.Manager) map[schema.GroupKind]admission.Handler {
	gks := []schema.GroupKind{
		appsv1.SchemeGroupVersion.WithKind("StatefulSet").GroupKind(),
	}
	handlers := map[schema.GroupKind]admission.Handler{}
	delegate := &mutatingHandler{
		WebhookAdmissionHandlerMixin: mixin.NewWebhookHandlerMixin(),
	}
	for _, gk := range gks {
		handlers[gk] = generic.NewAdmissionHandler("mutating", gk, delegate)
	}
	return handlers
}

var _ admission.Handler = &mutatingHandler{}

// mutatingHandler handles StatefulSet update.
// It should be wrapped by generic.AdmissionHandler.
type mutatingHandler struct {
	*mixin.WebhookAdmissionHandlerMixin
}

// Handle handles admission requests.
// It will only handle Pod creation and update. It will query the workload that manages it via the pod's ownerReference,
// and then apply the processing label from the workload onto the pod. In special cases where the pod's ownerReference
// is a ReplicaSet, it will continue to query its ownerReference to find the corresponding Deployment workload.
func (h *mutatingHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	switch req.Kind.Kind {
	case "StatefulSet":
		return h.mutateStatefulSet(ctx, req)
	}
	return admission.Allowed("")
}

func (h *mutatingHandler) mutateStatefulSet(ctx context.Context, req admission.Request) admission.Response {
	if req.Operation != admissionv1.Update || req.SubResource != "" {
		return admission.Allowed("only care about update events of collaset")
	}

	logger := logr.FromContextOrDiscard(ctx)

	obj := &appsv1.StatefulSet{}
	err := h.Decoder.Decode(req, obj)
	if err != nil {
		logger.Error(err, "failed to decode admission request")
		return admission.Errored(http.StatusBadRequest, err)
	}

	if !workload.IsControlledByRollout(obj) {
		return admission.Allowed("skip this object because it is not controlled by rollout")
	}

	// skip by label
	if obj.Spec.UpdateStrategy.Type != appsv1.RollingUpdateStatefulSetStrategyType {
		return admission.Allowed("skip this statefulset because its UpdateStrategy is not RollingUpdate")
	}

	oldObj := &appsv1.StatefulSet{}
	err = h.Decoder.DecodeRaw(req.OldObject, oldObj)
	if err != nil {
		logger.Error(err, "failed to decode old object in admission request")
		return admission.Errored(http.StatusBadRequest, err)
	}

	// check if pod template is changed
	if equality.Semantic.DeepEqual(oldObj.Spec.Template, obj.Spec.Template) {
		return admission.Allowed("pod template is not changed")
	}

	logger.Info("pod template is changed and it is controlled by rollout, set Partition to replicas to pause RollingUpdate", "replicas", ptr.Deref(obj.Spec.Replicas, 0))
	obj.Spec.UpdateStrategy.RollingUpdate = &appsv1.RollingUpdateStatefulSetStrategy{
		Partition: obj.Spec.Replicas,
	}
	marshaled, err := json.Marshal(obj)
	if err != nil {
		logger.Error(err, "failed to marshal statefulset to json")
		return admission.Errored(http.StatusInternalServerError, err)
	}

	return admission.PatchResponseFromRaw(req.AdmissionRequest.Object.Raw, marshaled)
}

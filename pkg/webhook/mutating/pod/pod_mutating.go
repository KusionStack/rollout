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

package pod

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/go-logr/logr"
	"github.com/tidwall/gjson"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"kusionstack.io/kube-utils/controller/mixin"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"kusionstack.io/rollout/apis/rollout"
	"kusionstack.io/rollout/pkg/controllers/registry"
	"kusionstack.io/rollout/pkg/utils"
	"kusionstack.io/rollout/pkg/webhook/generic"
	"kusionstack.io/rollout/pkg/workload"
)

// +kubebuilder:webhook:path=/webhooks/mutating/pod,mutating=true,failurePolicy=fail,sideEffects=None,admissionReviewVersions=v1;v1beta1,groups="",resources=pods,verbs=create;update,versions=v1,name=pods.core.k8s.io

const WebhookInitializerName = "mutate-pod"

func NewMutatingHandlers(_ manager.Manager) map[schema.GroupKind]admission.Handler {
	gks := []schema.GroupKind{
		corev1.SchemeGroupVersion.WithKind("Pod").GroupKind(),
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

// mutatingHandler handles Pod creation and update.
type mutatingHandler struct {
	*mixin.WebhookAdmissionHandlerMixin
}

// Handle handles admission requests.
// It will only handle Pod creation and update. It will query the workload that manages it via the pod's ownerReference,
// and then apply the processing label from the workload onto the pod. In special cases where the pod's ownerReference
// is a ReplicaSet, it will continue to query its ownerReference to find the corresponding Deployment workload.
func (h *mutatingHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	if (req.Operation != admissionv1.Create && req.Operation != admissionv1.Update) || req.SubResource != "" {
		return admission.Allowed("only care about pod create and update event")
	}

	logger := logr.FromContextOrDiscard(ctx)

	pod := &corev1.Pod{}
	err := h.Decoder.Decode(req, pod)
	if err != nil {
		logger.Error(err, "failed to decode admission request")
		return admission.Errored(http.StatusBadRequest, err)
	}

	ownerObj, _, err := registry.Workloads.GetPodOwnerWorkload(ctx, h.Client, pod)
	if err != nil {
		logger.Error(err, "failed to get pod owner workload")
		return admission.Errored(http.StatusInternalServerError, err)
	}
	if ownerObj == nil {
		// not found
		return admission.Allowed("skip this pod because it is not controlled by known workload")
	}

	if !workload.IsControlledByRollout(ownerObj) {
		// skip this pod because it is controlled by a rollout
		return admission.Allowed("skip this pod because its owner workload is not controlled by a rollout")
	}

	// update pod annotations if needed
	if changed := mutatePod(ownerObj.GetAnnotations(), pod); !changed {
		return admission.Allowed("Not changed")
	}

	newPodData, err := json.Marshal(pod)
	if err != nil {
		logger.Error(err, "failed to marshal pod json")
		return admission.Errored(http.StatusInternalServerError, err)
	}
	return admission.PatchResponseFromRaw(req.Object.Raw, newPodData)
}

func mutatePod(ownerAnnos map[string]string, pod *corev1.Pod) bool {
	ownerInfo := utils.GetMapValueByDefault(ownerAnnos, rollout.AnnoRolloutProgressingInfo, "")
	podInfo := utils.GetMapValueByDefault(pod.Annotations, rollout.AnnoRolloutProgressingInfo, "")
	if ownerInfo == "" || podInfo == ownerInfo {
		return false
	}
	idInOwner := gjson.Get(ownerInfo, "rolloutID").String()
	idInPod := gjson.Get(podInfo, "rolloutID").String()
	if idInOwner == idInPod {
		return false
	}
	// need update
	utils.MutateAnnotations(pod, func(annotations map[string]string) {
		annotations[rollout.AnnoRolloutProgressingInfo] = ownerInfo
	})
	return true
}
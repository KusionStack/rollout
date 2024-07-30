package mutating

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/tidwall/gjson"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	"kusionstack.io/kube-utils/controller/mixin"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"kusionstack.io/rollout/apis/rollout"
	"kusionstack.io/rollout/pkg/controllers/registry"
	"kusionstack.io/rollout/pkg/utils"
	"kusionstack.io/rollout/pkg/workload"
)

// +kubebuilder:webhook:path=/webhooks/mutating/pod,mutating=true,failurePolicy=fail,sideEffects=None,admissionReviewVersions=v1;v1beta1,groups="",resources=pods,verbs=create;update,versions=v1,name=pods.core.v1

const MutatingPod = "mutate-pod"

// PodCreateUpdateHandler handles Pod creation and update.
type PodCreateUpdateHandler struct {
	*mixin.WebhookAdmissionHandlerMixin
}

var _ admission.Handler = &PodCreateUpdateHandler{}

func NewMutatingHandler(_ manager.Manager) map[runtime.Object]http.Handler {
	return map[runtime.Object]http.Handler{
		&corev1.Pod{}: &webhook.Admission{Handler: newPodCreateUpdateHandler()},
	}
}

func newPodCreateUpdateHandler() *PodCreateUpdateHandler {
	return &PodCreateUpdateHandler{
		WebhookAdmissionHandlerMixin: mixin.NewWebhookHandlerMixin(),
	}
}

// Handle handles admission requests.
// It will only handle Pod creation and update. It will query the workload that manages it via the pod's ownerReference,
// and then apply the processing label from the workload onto the pod. In special cases where the pod's ownerReference
// is a ReplicaSet, it will continue to query its ownerReference to find the corresponding Deployment workload.
func (h *PodCreateUpdateHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	if ptr.Deref(req.DryRun, false) {
		return admission.Allowed("dry run")
	}

	if req.Kind.Kind != "Pod" ||
		req.SubResource != "" ||
		(req.Operation != admissionv1.Create && req.Operation != admissionv1.Update) {
		return admission.Allowed("PodCreateUpdateHandler only care about pod create and update event")
	}

	logger := h.Logger.WithValues(
		"kind", req.Kind.Kind,
		"key", utils.AdmissionRequestObjectKeyString(req),
		"op", req.Operation,
	)

	logger.V(4).Info("PodCreateUpdateHandler Handle start")
	defer logger.V(4).Info("PodCreateUpdateHandler Handle end")
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

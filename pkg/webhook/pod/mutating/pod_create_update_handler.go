package mutating

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/elliotchance/pie/v2"
	"github.com/tidwall/gjson"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"kusionstack.io/kube-utils/controller/mixin"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"kusionstack.io/rollout/apis/rollout"
	"kusionstack.io/rollout/pkg/controllers/registry"
	"kusionstack.io/rollout/pkg/utils"
)

// +kubebuilder:webhook:path=/pods/mutating,mutating=true,failurePolicy=ignore,sideEffects=None,admissionReviewVersions=v1;v1beta1,groups="",resources=pods,verbs=create;update,versions=v1,name=mpod.kb.io

const webhookPathPodMutating = "/pods/mutating"

// MiddleResources is the list of resources that are considered as middle resources, such as ReplicaSet.
// We need to find the corresponding owner workload for these middle resources.
var MiddleResources = []schema.GroupVersionKind{
	{Group: "apps", Version: "v1", Kind: "ReplicaSet"},
}

// PodCreateUpdateHandler handles Pod creation and update.
type PodCreateUpdateHandler struct {
	*mixin.WebhookAdmissionHandlerMixin
}

var _ admission.Handler = &PodCreateUpdateHandler{}

// NewMutatingHandler returns a new PodCreateUpdateHandler.
func NewMutatingHandler() admission.Handler {
	return &PodCreateUpdateHandler{
		WebhookAdmissionHandlerMixin: mixin.NewWebhookHandlerMixin(),
	}
}

// RegisterHandler registers a handler to the webhook.
func RegisterHandler(mgr manager.Manager) {
	mgr.GetWebhookServer().Register(webhookPathPodMutating, &webhook.Admission{Handler: NewMutatingHandler()})
}

// Handle handles admission requests.
// It will only handle Pod creation and update. It will query the workload that manages it via the pod's ownerReference,
// and then apply the processing label from the workload onto the pod. In special cases where the pod's ownerReference
// is a ReplicaSet, it will continue to query its ownerReference to find the corresponding Deployment workload.
func (h *PodCreateUpdateHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	if req.DryRun != nil && *req.DryRun {
		return admission.Allowed("dry run")
	}

	if req.SubResource != "" {
		return admission.Allowed("")
	}

	logger := h.Logger.WithValues(
		"kind", req.Kind.Kind,
		"key", utils.AdmissionRequestObjectKeyString(req),
		"op", req.Operation,
	)

	logger.V(5).Info("PodCreateUpdateHandler Handle start")
	defer logger.V(5).Info("PodCreateUpdateHandler Handle end")

	if req.Kind.Kind != "Pod" {
		logger.Error(fmt.Errorf("InvalidKind"), "Kind not Pod, but "+req.Kind.Kind)
		return admission.Allowed("Invalid kind")
	}

	if req.Operation != admissionv1.Create && req.Operation != admissionv1.Update {
		logger.Error(fmt.Errorf("InvalidOperation"), "Operation not Create or Update, but "+string(req.Operation))
		return admission.Allowed("Invalid operation")
	}

	pod := &corev1.Pod{}
	err := h.Decoder.Decode(req, pod)
	if err != nil {
		logger.Error(err, "failed to decode admission request")
		return admission.Errored(http.StatusBadRequest, err)
	}

	ownerRef := metav1.GetControllerOf(pod)
	if ownerRef == nil {
		logger.Info("no owner reference found")
		return admission.Allowed("no owner reference found")
	}

	ownerGVK, err := gvkOfOwner(ownerRef)
	if err != nil {
		logger.Error(err, "failed to get owner gvk")
		return admission.Errored(http.StatusInternalServerError, err)
	}

	// Check if owner is controlled by rollout
	if !controlledByRollout(ownerGVK) {
		logger.Info("owner not controlled by rollout", "owner", ownerGVK)
		return admission.Allowed("owner not controlled by rollout")
	}

	// Check if owner is middle resource, which means we need to find the corresponding owner's owner
	if isMiddleResource(ownerGVK) {
		ownerObj, err := getObjByGVK(ctx, h.Client, ownerGVK, pod.Namespace, ownerRef.Name)
		if err != nil {
			logger.Error(err, "failed to get owner object")
			return admission.Errored(http.StatusInternalServerError, err)
		}
		ownerRef = metav1.GetControllerOf(ownerObj)
		ownerGVK, err = gvkOfOwner(ownerRef)
		if err != nil {
			logger.Error(err, "failed to get owner gvk")
			return admission.Errored(http.StatusInternalServerError, err)
		}
		if !controlledByRollout(ownerGVK) {
			logger.Info("owner not controlled by rollout", "owner", ownerRef)
			return admission.Allowed("owner not controlled by rollout")
		}
	}

	// get owner object
	ownerObj, err := getObjByGVK(ctx, h.Client, ownerGVK, pod.Namespace, ownerRef.Name)
	if err != nil {
		logger.Error(err, "failed to get owner object")
		return admission.Errored(http.StatusInternalServerError, err)
	}

	// update pod annotations if needed
	if changed := mutatePod(ownerObj.GetAnnotations(), pod); !changed {
		return admission.Allowed("Not changed")
	}

	marshaled, err := json.Marshal(pod)
	if err != nil {
		logger.Error(err, "failed to marshal pod json")
		return admission.Errored(http.StatusInternalServerError, err)
	}
	return admission.PatchResponseFromRaw(req.Object.Raw, marshaled)
}

func gvkOfOwner(ownerRef *metav1.OwnerReference) (schema.GroupVersionKind, error) {
	ownerGV, err := schema.ParseGroupVersion(ownerRef.APIVersion)
	if err != nil {
		return schema.GroupVersionKind{}, err
	}
	return ownerGV.WithKind(ownerRef.Kind), nil
}

func getObjByGVK(ctx context.Context, cli client.Client, gvk schema.GroupVersionKind, namespace, name string) (client.Object, error) {
	store, err := registry.Workloads.Get(gvk)
	if err != nil {
		return nil, err
	}
	obj := store.NewObject()
	err = cli.Get(ctx, client.ObjectKey{
		Namespace: namespace,
		Name:      name,
	}, obj)
	if err != nil {
		return nil, err
	}
	return obj, nil
}

func controlledByRollout(gvk schema.GroupVersionKind) bool {
	_, err := registry.Workloads.Get(gvk)
	return err == nil
}

func isMiddleResource(gvk schema.GroupVersionKind) bool {
	return pie.Any[schema.GroupVersionKind](MiddleResources, func(value schema.GroupVersionKind) bool {
		return value == gvk
	})
}

func mutatePod(ownerAnnos map[string]string, pod *corev1.Pod) bool {
	ownerInfo := ownerAnnos[rollout.AnnoRolloutProgressingInfo]
	podInfo := pod.Annotations[rollout.AnnoRolloutProgressingInfo]
	if ownerInfo == "" || podInfo == ownerInfo {
		return false
	}
	idInOwner := gjson.Get(ownerInfo, "rolloutID").String()
	idInPod := gjson.Get(podInfo, "rolloutID").String()
	if idInOwner == idInPod {
		return false
	}

	// need update
	if pod.Annotations == nil {
		pod.Annotations = make(map[string]string)
	}
	pod.Annotations[rollout.AnnoRolloutProgressingInfo] = ownerInfo
	return true
}

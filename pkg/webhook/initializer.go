package webhook

import (
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"kusionstack.io/kube-utils/controller/initializer"
	ctrl "sigs.k8s.io/controller-runtime"

	podmutating "kusionstack.io/rollout/pkg/webhook/pod/mutating"
	rolloutvalidating "kusionstack.io/rollout/pkg/webhook/rollout/validating"
)

// Initializer is the initializer for webhook.
var Initializer = initializer.NewNamed("webhooks")

type webhookType string

const (
	mutatingWebhook   = "mutating"
	validatingWebhook = "validating"
)

func init() {
	utilruntime.Must(Initializer.Add(podmutating.MutatingPod, func(mgr ctrl.Manager) (bool, error) {
		handlers := podmutating.NewMutatingHandler(mgr)
		for obj, h := range handlers {
			err := setupWebhook(mgr, mutatingWebhook, obj, h)
			if err != nil {
				return false, err
			}
		}
		return true, nil
	}))

	utilruntime.Must(Initializer.Add(rolloutvalidating.ValidatingRollout, func(mgr ctrl.Manager) (bool, error) {
		handlers := rolloutvalidating.NewValidatingHandler(mgr)
		for obj, h := range handlers {
			err := setupWebhook(mgr, validatingWebhook, obj, h)
			if err != nil {
				return false, err
			}
		}
		return true, nil
	}))
}

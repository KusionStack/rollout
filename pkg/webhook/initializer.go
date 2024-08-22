package webhook

import (
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"kusionstack.io/kube-utils/controller/initializer"
	ctrl "sigs.k8s.io/controller-runtime"
)

// Initializer is the initializer for webhook.
var Initializer = initializer.NewNamed("webhooks")

type webhookType string

const (
	mutatingWebhook   = "mutating"
	validatingWebhook = "validating"
)

func addInitializer() {
	for key, newFuc := range mutatingWebhooks {
		utilruntime.Must(Initializer.Add(key, func(mgr ctrl.Manager) (bool, error) {
			handlers := newFuc(mgr)
			for gk, h := range handlers {
				setupWebhook(mgr, mutatingWebhook, gk, h)
			}
			return true, nil
		}))
	}

	for key, newFuc := range validatingWebhooks {
		utilruntime.Must(Initializer.Add(key, func(mgr ctrl.Manager) (bool, error) {
			handlers := newFuc(mgr)
			for gk, h := range handlers {
				setupWebhook(mgr, validatingWebhook, gk, h)
			}
			return true, nil
		}))
	}
}

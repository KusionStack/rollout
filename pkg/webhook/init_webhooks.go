package webhook

import (
	"net/http"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	kuperatormutating "kusionstack.io/rollout/pkg/webhook/mutating/kuperator"
	podmutating "kusionstack.io/rollout/pkg/webhook/mutating/pod"
	stsmutating "kusionstack.io/rollout/pkg/webhook/mutating/statefulset"
	rolloutvalidating "kusionstack.io/rollout/pkg/webhook/validating/rollout"
)

type NewWebhookHandler func(manager.Manager) map[runtime.Object]http.Handler

var (
	mutatingWebhooks   = map[string]NewWebhookHandler{}
	validatingWebhooks = map[string]NewWebhookHandler{}
)

func init() {
	// setup mutating webhook handlers
	mutatingWebhooks[podmutating.WebhookInitializerName] = podmutating.NewMutatingHandler
	mutatingWebhooks[stsmutating.WebhookInitialzierName] = stsmutating.NewMutatingHandlers
	mutatingWebhooks[kuperatormutating.WebhookInitialzierName] = kuperatormutating.NewMutatingHandlers
	// setup validating webhook handlers
	validatingWebhooks[rolloutvalidating.WebhookInitializerName] = rolloutvalidating.NewValidatingHandlers

	addInitializer()
}

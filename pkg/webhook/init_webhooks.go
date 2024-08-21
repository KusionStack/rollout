package webhook

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	kuperatormutating "kusionstack.io/rollout/pkg/webhook/mutating/kuperator"
	podmutating "kusionstack.io/rollout/pkg/webhook/mutating/pod"
	stsmutating "kusionstack.io/rollout/pkg/webhook/mutating/statefulset"
	rolloutvalidating "kusionstack.io/rollout/pkg/webhook/validating/rollout"
)

type NewWebhookHandler func(manager.Manager) map[schema.GroupKind]admission.Handler

var (
	mutatingWebhooks   = map[string]NewWebhookHandler{}
	validatingWebhooks = map[string]NewWebhookHandler{}
)

func init() {
	// setup mutating webhook handlers
	mutatingWebhooks[podmutating.WebhookInitializerName] = podmutating.NewMutatingHandlers
	mutatingWebhooks[stsmutating.WebhookInitialzierName] = stsmutating.NewMutatingHandlers
	mutatingWebhooks[kuperatormutating.WebhookInitialzierName] = kuperatormutating.NewMutatingHandlers
	// setup validating webhook handlers
	validatingWebhooks[rolloutvalidating.WebhookInitializerName] = rolloutvalidating.NewValidatingHandlers

	addInitializer()
}

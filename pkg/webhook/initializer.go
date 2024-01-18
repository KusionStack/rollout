package webhook

import (
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"kusionstack.io/kube-utils/controller/initializer"
	ctrl "sigs.k8s.io/controller-runtime"

	podmutating "kusionstack.io/rollout/pkg/webhook/pod/mutating"
)

// Initializer is the initializer for webhook.
var Initializer = initializer.NewNamed("webhooks")

const (
	initializerNameMutatingPod = "pod-mutating"
)

func init() {
	utilruntime.Must(Initializer.Add(initializerNameMutatingPod, func(mgr ctrl.Manager) (bool, error) {
		return SetupWithManager(mgr, podmutating.RegisterHandler)
	}))
}

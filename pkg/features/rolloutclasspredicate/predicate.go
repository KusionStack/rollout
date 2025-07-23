package rolloutclasspredicate

import (
	"os"

	"kusionstack.io/kube-api/rollout"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"kusionstack.io/rollout/pkg/features"
)

var RolloutClassMatchesPredicate = predicate.Funcs{
	CreateFunc: func(e event.CreateEvent) bool {
		return IsRolloutClassMatches(e.Object)
	},
	UpdateFunc: func(e event.UpdateEvent) bool {
		return IsRolloutClassMatches(e.ObjectNew)
	},
	DeleteFunc: func(e event.DeleteEvent) bool {
		return IsRolloutClassMatches(e.Object)
	},
	GenericFunc: func(e event.GenericEvent) bool {
		return IsRolloutClassMatches(e.Object)
	},
}

func IsRolloutClassMatches(obj client.Object) bool {
	if features.DefaultFeatureGate.Enabled(features.RolloutClassPredicate) {
		envClass := os.Getenv("ROLLOUT_CLASS")
		claas, labeled := obj.GetLabels()[rollout.LabelRolloutClass]

		if len(envClass) == 0 {
			// if env:ROLLOUT_CLASS is not set or set to ""
			// the controller can handle objects whithout rolloutClass label
			return !labeled
		}
		// if env:ROLLOUT_CLASS is set
		// the controller can only handle objects with rolloutClass label and value is equal to env:ROLLOUT_CLASS
		return labeled && claas == envClass
	}

	// if feature is disabled
	// the controller can handle all objects
	return true
}

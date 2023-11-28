package controllers

import (
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"

	"kusionstack.io/rollout/pkg/controllers/rollout"
	"kusionstack.io/rollout/pkg/controllers/rolloutrun"
)

func init() {
	utilruntime.Must(Initialzier.Add(rollout.ControllerName, rollout.InitFunc))
	utilruntime.Must(Initialzier.Add(rolloutrun.ControllerName, rolloutrun.InitFunc))
}

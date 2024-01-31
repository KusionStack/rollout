package controllers

import (
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"

	"kusionstack.io/rollout/pkg/controllers/traffictopology"
)

func init() {
	utilruntime.Must(Initializer.Add(traffictopology.ControllerName, traffictopology.InitFunc))
}

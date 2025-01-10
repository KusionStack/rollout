package initializers

import (
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"

	"kusionstack.io/rollout/pkg/controllers/swarm"
)

func init() {
	// init rollout controller
	utilruntime.Must(Controllers.Add(swarm.ControllerName, swarm.InitFunc))
}

package controllers

import (
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"

	"kusionstack.io/rollout/pkg/controllers/task"
	"kusionstack.io/rollout/pkg/controllers/workflow"
)

func init() {
	utilruntime.Must(Initialzier.Add(workflow.ControllerName, workflow.InitFunc))
	utilruntime.Must(Initialzier.Add(task.ControllerName, task.InitFunc))
}

package api

const (
	LabelRolloutControl   = "kafe.kusionstack.io/rollout-control"
	LabelRolloutReference = "kafe.kusionstack.io/rollout-reference"
	LabelRolloutID        = "kafe.kusionstack.io/rollout-id"

	// LabelRolloutManualCommand is set in Rollout for users to manipulate workflow
	LabelRolloutManualCommand       = "kafe.kusionstack.io/rollout-manual-command"
	LabelRolloutManualCommandResume = "Resume"
	LabelRolloutManualCommandPause  = "Pause"
	LabelRolloutManualCommandCancel = "Cancel"

	LabelAliZone = "meta.k8s.alipay.com/zone"

	LabelRolloutBatchName        = "kafe.kusionstack.io/rollout-batch-name"
	LabelRolloutCluster          = "kafe.kusionstack.io/rollout-cluster"
	LabelRolloutWorkload         = "kafe.kusionstack.io/rollout-workload"
	LabelRolloutAnalysisProvider = "kafe.kusionstack.io/rollout-analysis-provider"
	LabelSuspendName             = "kafe.kusionstack.io/rollout-suspend-name"
	LabelRolloutTaskID           = "kafe.kusionstack.io/rollout-task-id"

	LabelGeneratedBy = "kafe.kusionstack.io/generated-by"
)

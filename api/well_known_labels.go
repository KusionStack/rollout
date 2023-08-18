/*
 * Copyright 2023 The KusionStack Authors.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package api

const (
	LabelRolloutControl   = "rollout.kusionstack.io/rollout-control"
	LabelRolloutReference = "rollout.kusionstack.io/rollout-reference"
	LabelRolloutID        = "rollout.kusionstack.io/rollout-id"

	// LabelRolloutManualCommand is set in Rollout for users to manipulate workflow
	LabelRolloutManualCommand       = "rollout.kusionstack.io/rollout-manual-command"
	LabelRolloutManualCommandResume = "Resume"
	LabelRolloutManualCommandPause  = "Pause"
	LabelRolloutManualCommandCancel = "Cancel"

	LabelRolloutBatchName        = "rollout.kusionstack.io/rollout-batch-name"
	LabelRolloutCluster          = "rollout.kusionstack.io/rollout-cluster"
	LabelRolloutWorkload         = "rollout.kusionstack.io/rollout-workload"
	LabelRolloutAnalysisProvider = "rollout.kusionstack.io/rollout-analysis-provider"
	LabelSuspendName             = "rollout.kusionstack.io/rollout-suspend-name"
	LabelRolloutTaskID           = "rollout.kusionstack.io/rollout-task-id"

	LabelGeneratedBy = "rollout.kusionstack.io/generated-by"
)

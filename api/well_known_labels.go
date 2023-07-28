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

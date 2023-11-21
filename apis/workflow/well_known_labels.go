// Copyright 2023 The KusionStack Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package workflow

const (
	// Used by workflow
	LabelBatchIndex      = "rollout.kusionstack.io/batch-index"
	LabelCluster         = "rollout.kusionstack.io/cluster"
	LabelWebhookProvider = "rollout.kusionstack.io/webhook-provider"
	LabelSuspendName     = "rollout.kusionstack.io/suspend-name"
	LabelTaskID          = "rollout.kusionstack.io/task-id"

	LabelKind        = "rollout.kusionstack.io/kind"
	LabelOpsScenario = "rollout.kusionstack.io/ops-scenario"

	LabelBatchName = "rollout.kusionstack.io/batch-name"
	LabelBatchNum  = "rollout.kusionstack.io/batch-num"
)

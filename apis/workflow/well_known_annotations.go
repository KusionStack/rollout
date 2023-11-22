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
	AnnoWorkflowBatchInfo           = "rollout.kusionstack.io/batch-info"
	AnnoWorkflowResumeSuspendTasks  = "rollout.kusionstack.io/resume-suspend-tasks"
	AnnoWorkflowWebhookCheckOrderId = "rollout.kusionstack.io/webhook-check-order-id"

	AnnoWorkflowOpsCloudTrace = "rollout.kusionstack.io/opscloud-trace"

	AnnoTaskWebhookVerdict     = "rollout.kusionstack.io/webhook-verdict-info"
	AnnoTaskWebhookSubmitCheck = "rollout.kusionstack.io/webhook-submit-check"
	AnnoTaskWebhookCheckId     = "rollout.kusionstack.io/webhook-check-id"

	AnnoRolloutResumeContext   = "rollout.kusionstack.io/resume-context"
	AnnoRolloutParallelBatches = "rollout.kusionstack.io/parallel-batches"
	AnnoRolloutTotalBatches    = "rollout.kusionstack.io/total-batches"
)

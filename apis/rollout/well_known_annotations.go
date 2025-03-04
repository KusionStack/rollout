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

package rollout

const (
	// LabelRolloutManualCommand is set in Rollout for users to manipulate rolloutRun
	AnnoManualCommandKey = "rollout.kusionstack.io/manual-command"
	// Deprecated: use continue
	AnnoManualCommandResume   = "resume"
	AnnoManualCommandContinue = "continue"
	AnnoManualCommandRetry    = "retry"
	AnnoManualCommandSkip     = "skip"
	AnnoManualCommandPause    = "pause"
	AnnoManualCommandCancel   = "cancel"

	AnnoRolloutTrigger = "rollout.kusionstack.io/trigger"

	// AnnoRolloutProgressingInfo contains the current progressing info on workload.
	// The value is a json string of ProgressingInfo.
	AnnoRolloutProgressingInfo = "rollout.kusionstack.io/progressing-info"

	// AnnoPodRolloutProgressingInfos contains a slice of progressing infos on a pod.
	AnnoPodRolloutProgressingInfos = "rollout.kusionstack.io/pod-progressing-infos"

	// AnnoModelRolloutProgressingInfos contains a slice of progressing infos on a model.
	AnnoModelRolloutProgressingInfos = "rollout.kusionstack.io/model-progressing-infos"
)

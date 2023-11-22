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

package ontimestrategy

import (
	"encoding/json"

	rolloutv1alpha1 "kusionstack.io/rollout/apis/rollout/v1alpha1"
)

const (
	// Alpha feature OneTimeStrategy
	AnnoOneTimeStrategy = "rollout.kusionstack.io/one-time-strategy"

	UpdateStrategyCondition rolloutv1alpha1.ConditionType = "UpdatedStrategy"
)

type OneTimeStrategy struct {
	Batch rolloutv1alpha1.BatchStrategy `json:"batch,omitempty"`
}

func (s *OneTimeStrategy) JSONData() []byte {
	data, _ := json.Marshal(s)
	return data
}

func ConvertFrom(in *rolloutv1alpha1.RolloutStrategy) *OneTimeStrategy {
	return &OneTimeStrategy{
		Batch: *in.Batch,
	}
}

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

	rolloutv1alpha1 "kusionstack.io/kube-api/rollout/v1alpha1"
)

const (
	// Alpha feature OneTimeStrategy
	AnnoOneTimeStrategy = "rollout.kusionstack.io/one-time-strategy"

	UpdateStrategyCondition rolloutv1alpha1.ConditionType = "UpdatedStrategy"
)

type OneTimeStrategy struct {
	// Batch is the V1 field for StrategyRef scenario.
	// Used when referencing a RolloutStrategy CRD with V1 match/replicas.
	// Mutually exclusive with BatchV2.
	Batch rolloutv1alpha1.BatchStrategy `json:"batch,omitempty"`

	// BatchV2 is for V2 batch strategy scenario.
	// Used when RolloutStrategy uses BatchV2 configuration.
	// Mutually exclusive with Batch.
	BatchV2 *rolloutv1alpha1.BatchStrategyV2 `json:"batchV2,omitempty"`
}

func (s *OneTimeStrategy) JSONData() []byte {
	data, _ := json.Marshal(s)
	return data
}

// ConvertFrom creates a OneTimeStrategy from RolloutStrategy
func ConvertFrom(in *rolloutv1alpha1.RolloutStrategy) *OneTimeStrategy {
	if in == nil {
		return &OneTimeStrategy{}
	}
	if in.BatchV2 != nil {
		return &OneTimeStrategy{
			BatchV2: in.BatchV2,
		}
	}
	if in.Batch != nil {
		return &OneTimeStrategy{
			Batch: *in.Batch,
		}
	}
	return &OneTimeStrategy{}
}

// ConvertFromV2 creates a OneTimeStrategy from BatchStrategyV2
func ConvertFromV2(batchStrategy *rolloutv1alpha1.BatchStrategyV2) *OneTimeStrategy {
	if batchStrategy == nil {
		return &OneTimeStrategy{}
	}
	return &OneTimeStrategy{
		BatchV2: batchStrategy,
	}
}

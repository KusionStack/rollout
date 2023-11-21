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

package v1alpha1

// ResourceTaskActionType is the type of the action of resource task
type ResourceTaskActionType string

const (
	ResourceTaskActionTypeCreate ResourceTaskActionType = "Create"
	ResourceTaskActionTypeUpdate ResourceTaskActionType = "Update"
	ResourceTaskActionTypeDelete ResourceTaskActionType = "Delete"
	ResourceTaskActionTypeGet    ResourceTaskActionType = "Get"
	ResourceTaskActionTypePatch  ResourceTaskActionType = "Patch"
	ResourceTaskActionTypeApply  ResourceTaskActionType = "Apply"
)

// MergeStrategyType is the type of the merge strategy of resource task
type MergeStrategyType string

const (
	MergeStrategyTypeStrategic MergeStrategyType = "strategic"
	MergeStrategyTypeJSON      MergeStrategyType = "json"
	MergeStrategyTypeMerge     MergeStrategyType = "merge"
)

type ManifestFrom struct {
	Artifact `json:",inline"`
}

// ResourceTask is a task that manipulates k8s resources
type ResourceTask struct {

	// Action is the action to be performed on the resource
	Action ResourceTaskActionType `json:"action"`

	// MergeStrategy is the strategy to merge a patch. Defaults to "strategic".
	// +optional
	MergeStrategy MergeStrategyType `json:"mergeStrategy,omitempty"`

	// Manifest is the manifest of the resource
	// +optional
	Manifest string `json:"manifest,omitempty"`

	// ManifestFrom is the source of the manifest
	// +optional
	ManifestFrom *ManifestFrom `json:"manifestFrom,omitempty"`

	// Checker is the checker to check if the task is successful
	// +optional
	Checker *Checker `json:"checker,omitempty"`
}

// Checker is the checker to check if the task is successful
type Checker struct {
	// SuccessCondition is the condition to check if the task is successful
	// +optional
	SuccessCondition string `json:"successCondition,omitempty"`

	// FailureCondition is the condition to check if the task is failed
	// +optional
	FailureCondition string `json:"failureCondition,omitempty"`

	// BuiltInChecker is the built-in checker to check if the task is successful
	// +optional
	BuiltInChecker *BuiltInChecker `json:"builtInChecker,omitempty"`
}

// BuiltInChecker is the built-in checker to check if the task is successful
type BuiltInChecker struct {
	// Type is the type of the built-in checker
	Type BuiltInCheckerType `json:"type"`

	// Params is the parameters of the built-in checker
	Params Params `json:"params,omitempty"`
}

// BuiltInCheckerType is the type of the built-in checker
type BuiltInCheckerType string

const (
	// BuiltInCheckerTypeCafeDeploymentRollout is the built-in checker to check if the rollout of CafeDeployment is successful
	BuiltInCheckerTypeCafeDeploymentRollout BuiltInCheckerType = "CafeDeploymentRollout"
)

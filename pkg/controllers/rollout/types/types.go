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

package types

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	rolloutv1alpha1 "github.com/KusionStack/rollout/api/v1alpha1"
	"github.com/KusionStack/rollout/pkg/workload"
)

type UpgradeBatchDetail struct {
	BatchNum  int32
	BatchName string

	IsBeta        bool
	HasBreakPoint bool
	NeedPause     bool

	Replicas  intstr.IntOrString
	Workloads []workload.Interface

	FailureThreshold *intstr.IntOrString
	WaitTimeSeconds  int32
	AnalysisRules    []*rolloutv1alpha1.AnalysisRule
	Selector         *metav1.LabelSelector

	PauseSuspendTask      *rolloutv1alpha1.WorkflowTask
	BreakPointSuspendTask *rolloutv1alpha1.WorkflowTask
}

type WorkloadBasicInfo struct {
	Kind    string `json:"kind,omitempty"`
	Name    string `json:"name,omitempty"`
	Cluster string `json:"cluster,omitempty"`
}

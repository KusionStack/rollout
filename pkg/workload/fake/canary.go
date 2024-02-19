/**
 * Copyright 2024 The KusionStack Authors
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

package fake

import (
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"kusionstack.io/rollout/apis/rollout/v1alpha1"
	"kusionstack.io/rollout/pkg/workload"
)

func (w *workloadImpl) CanaryStrategy() (workload.CanaryStrategy, error) {
	canaryName := w.Name + "-canary"
	canary := New(w.ClusterName, w.Namespace, canaryName).(*workloadImpl)

	return &canaryStrategy{
		stable: w,
		canary: canary,
	}, nil
}

var _ workload.CanaryStrategy = &canaryStrategy{}

type canaryStrategy struct {
	stable *workloadImpl
	canary *workloadImpl
}

// CreateOrUpdate implements workload.CanaryStrategy.
func (s *canaryStrategy) CreateOrUpdate(canaryReplicas intstr.IntOrString, podTemplatePatch *v1alpha1.MetadataPatch) (controllerutil.OperationResult, error) {
	replicas, err := workload.CalculatePartitionReplicas(&s.stable.Partition, canaryReplicas)
	if err != nil {
		return controllerutil.OperationResultNone, err
	}
	s.canary.ChangeStatus(replicas, replicas, replicas)
	return controllerutil.OperationResultNone, nil
}

// Delete implements workload.CanaryStrategy.
func (*canaryStrategy) Delete() error {
	return nil
}

// Finalize implements workload.CanaryStrategy.
func (*canaryStrategy) Finalize() error {
	return nil
}

// GetCanaryInfo implements workload.CanaryStrategy.
func (s *canaryStrategy) GetCanaryInfo() workload.Info {
	return s.canary.GetInfo()
}

// GetStableInfo implements workload.CanaryStrategy.
func (s *canaryStrategy) GetStableInfo() workload.Info {
	return s.stable.GetInfo()
}

// Initialize implements workload.CanaryStrategy.
func (*canaryStrategy) Initialize(rollout string, rolloutRun string) error {
	return nil
}

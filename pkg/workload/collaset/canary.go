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

package collaset

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/cri-api/pkg/errors"
	operatingv1alpha1 "kusionstack.io/kube-api/apps/v1alpha1"
	"kusionstack.io/kube-utils/multicluster/clusterinfo"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	rolloutapi "kusionstack.io/rollout/apis/rollout"
	"kusionstack.io/rollout/apis/rollout/v1alpha1"
	"kusionstack.io/rollout/pkg/workload"
)

func (w *workloadImpl) CanaryStrategy() (workload.CanaryStrategy, error) {
	canary, err := w.canaryWorkload()
	if err != nil {
		return nil, err
	}
	return &canaryStrategy{
		stable: w,
		canary: canary,
	}, nil
}

func (w *workloadImpl) canaryWorkload() (*workloadImpl, error) {
	stableObj := w.obj
	canaryName := stableObj.Name + "-canary"
	var canaryObj operatingv1alpha1.CollaSet
	err := w.client.Get(
		clusterinfo.WithCluster(context.TODO(), w.info.ClusterName),
		client.ObjectKey{Namespace: stableObj.Namespace, Name: canaryName},
		&canaryObj,
	)
	if client.IgnoreNotFound(err) != nil {
		return nil, err
	}

	if errors.IsNotFound(err) {
		canaryObj = operatingv1alpha1.CollaSet{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:   stableObj.Namespace,
				Name:        canaryName,
				Labels:      stableObj.Labels,
				Annotations: stableObj.Annotations,
				Finalizers: []string{
					rolloutapi.FinalizerCanaryResourceProtection,
				},
			},
			Spec: *stableObj.Spec.DeepCopy(),
		}
	}

	return newFrom(w.info.ClusterName, w.client, &canaryObj), nil
}

var _ workload.CanaryStrategy = &canaryStrategy{}

type canaryStrategy struct {
	stable *workloadImpl
	canary *workloadImpl
}

// CreateOrUpdate implements workload.CanaryStrategy.
func (*canaryStrategy) CreateOrUpdate(canaryReplicas intstr.IntOrString, podTemplatePatch *v1alpha1.MetadataPatch) (controllerutil.OperationResult, error) {
	panic("unimplemented")
}

// Delete implements workload.CanaryStrategy.
func (*canaryStrategy) Delete() error {
	panic("unimplemented")
}

// Finalize implements workload.CanaryStrategy.
func (*canaryStrategy) Finalize() error {
	panic("unimplemented")
}

// GetCanaryInfo implements workload.CanaryStrategy.
func (*canaryStrategy) GetCanaryInfo() workload.Info {
	panic("unimplemented")
}

// GetStableInfo implements workload.CanaryStrategy.
func (*canaryStrategy) GetStableInfo() workload.Info {
	panic("unimplemented")
}

// Initialize implements workload.CanaryStrategy.
func (*canaryStrategy) Initialize(rollout string, rolloutRun string) error {
	panic("unimplemented")
}

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

package traffic

import (
	"context"

	"github.com/go-logr/logr"
	"kusionstack.io/kube-utils/multicluster/clusterinfo"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	rolloutv1alpha1 "kusionstack.io/rollout/apis/rollout/v1alpha1"
	"kusionstack.io/rollout/pkg/utils"
)

type Manager struct {
	client client.Client
	logger logr.Logger

	topoligies map[rolloutv1alpha1.CrossClusterObjectNameReference]*topology

	targets  []rolloutv1alpha1.RolloutRunStepTarget
	strategy *rolloutv1alpha1.TrafficStrategy
}

func NewManager(c client.Client, logger logr.Logger, topologies []rolloutv1alpha1.TrafficTopology) (*Manager, error) {
	m := &Manager{
		client:     c,
		logger:     logger.WithName("traffic"),
		topoligies: make(map[rolloutv1alpha1.CrossClusterObjectNameReference]*topology),
	}
	for _, obj := range topologies {
		for _, info := range obj.Status.Topologies {
			ref := info.WorkloadRef
			var routing rolloutv1alpha1.BackendRouting
			key := client.ObjectKey{Namespace: obj.Namespace, Name: info.BackendRoutingName}
			err := m.client.Get(context.TODO(), key, &routing)
			if err != nil {
				return nil, err
			}

			topo, ok := m.topoligies[ref]
			if !ok {
				topo = &topology{
					workload: ref,
					routings: make([]*rolloutv1alpha1.BackendRouting, 0),
				}
				m.topoligies[ref] = topo
			}
			topo.routings = append(topo.routings, &routing)
		}
	}

	return m, nil
}

func (m *Manager) With(logger logr.Logger, workloads []rolloutv1alpha1.RolloutRunStepTarget, strategy *rolloutv1alpha1.TrafficStrategy) {
	m.logger = logger.WithName("traffic")
	m.targets = workloads
	m.strategy = strategy
}

func (m *Manager) ForkStable() (controllerutil.OperationResult, error) {
	return m.mutateRouting(func(routing *rolloutv1alpha1.BackendRouting) error {
		if routing.Spec.Forwarding == nil {
			routing.Spec.Forwarding = &rolloutv1alpha1.BackendForwarding{}
		}
		routing.Spec.Forwarding.Stable = rolloutv1alpha1.StableBackendRule{
			Name: routing.Spec.Backend.Name + "-stable",
		}
		return nil
	})
}

func (m *Manager) ForkCanary() (controllerutil.OperationResult, error) {
	return m.mutateRouting(func(routing *rolloutv1alpha1.BackendRouting) error {
		if routing.Spec.Forwarding == nil {
			routing.Spec.Forwarding = &rolloutv1alpha1.BackendForwarding{}
		}
		routing.Spec.Forwarding.Canary = rolloutv1alpha1.CanaryBackendRule{
			Name:            routing.Spec.Backend.Name + "-canary",
			TrafficStrategy: *m.strategy,
		}
		return nil
	})
}

func (m *Manager) RevertCanary() (controllerutil.OperationResult, error) {
	return m.mutateRouting(func(routing *rolloutv1alpha1.BackendRouting) error {
		if routing.Spec.Forwarding == nil {
			return nil
		}
		routing.Spec.Forwarding.Canary = rolloutv1alpha1.CanaryBackendRule{}
		return nil
	})
}

func (m *Manager) RevertStable() (controllerutil.OperationResult, error) {
	return m.mutateRouting(func(routing *rolloutv1alpha1.BackendRouting) error {
		routing.Spec.Forwarding = nil
		return nil
	})
}

func (m *Manager) mutateRouting(mutateFn func(routing *rolloutv1alpha1.BackendRouting) error) (controllerutil.OperationResult, error) {
	operation := controllerutil.OperationResultNone
	if m.strategy == nil {
		m.logger.Info("no traffic strategy found, skip it")
		return operation, nil
	}
	ctx := clusterinfo.WithCluster(context.Background(), clusterinfo.Fed)

	for _, workload := range m.targets {
		topo, ok := m.topoligies[workload.CrossClusterObjectNameReference]
		if !ok {
			m.logger.Info("no traffic topology found for workload", "workload", workload.CrossClusterObjectNameReference)
			continue
		}
		for i := range topo.routings {
			routing := topo.routings[i]
			updated, err := utils.UpdateOnConflict(ctx, m.client, m.client, routing, func() error {
				return mutateFn(routing)
			})
			if err != nil {
				return controllerutil.OperationResultNone, err
			}
			if updated {
				topo.routings[i] = routing
				operation = controllerutil.OperationResultUpdated
			}
		}
	}
	return operation, nil
}

func (m *Manager) CheckReady() bool {
	for _, workload := range m.targets {
		topo, ok := m.topoligies[workload.CrossClusterObjectNameReference]
		if !ok {
			continue
		}
		for _, routing := range topo.routings {
			if routing.Generation == routing.Status.ObservedGeneration &&
				routing.Status.Phase == rolloutv1alpha1.Ready {
				continue
			}
			return false
		}
	}
	return true
}

type topology struct {
	workload rolloutv1alpha1.CrossClusterObjectNameReference
	routings []*rolloutv1alpha1.BackendRouting
}

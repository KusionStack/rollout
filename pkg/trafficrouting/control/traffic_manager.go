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

package control

import (
	"context"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/meta"
	rolloutv1alpha1 "kusionstack.io/kube-api/rollout/v1alpha1"
	clientutil "kusionstack.io/kube-utils/client"
	"kusionstack.io/kube-utils/multicluster/clusterinfo"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

type Manager struct {
	client client.Client

	topoligies map[rolloutv1alpha1.CrossClusterObjectNameReference]*topology

	targets  []rolloutv1alpha1.RolloutRunStepTarget
	strategy *rolloutv1alpha1.TrafficStrategy
}

func NewManager(ctx context.Context, c client.Client, topologies []rolloutv1alpha1.TrafficTopology) (*Manager, error) {
	m := &Manager{
		client:     c,
		topoligies: make(map[rolloutv1alpha1.CrossClusterObjectNameReference]*topology),
	}

	logger := logr.FromContextOrDiscard(ctx)
	ctx = clusterinfo.WithCluster(ctx, clusterinfo.Fed)

	for _, obj := range topologies {
		for _, info := range obj.Status.Topologies {
			ref := info.WorkloadRef
			routing := &rolloutv1alpha1.BackendRouting{}
			key := client.ObjectKey{Namespace: obj.Namespace, Name: info.BackendRoutingName}
			err := m.client.Get(ctx, key, routing)
			if err != nil {
				logger.Error(err, "failed to get backend routing", "backendrouting", info.BackendRoutingName)
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
			topo.routings = append(topo.routings, routing)
		}
	}

	return m, nil
}

func (m *Manager) With(workloads []rolloutv1alpha1.RolloutRunStepTarget, strategy *rolloutv1alpha1.TrafficStrategy) {
	m.targets = workloads
	m.strategy = strategy
}

func (m *Manager) ForkBackends(ctx context.Context) (controllerutil.OperationResult, error) {
	return m.mutateRouting(ctx, func(routing *rolloutv1alpha1.BackendRouting) error {
		if routing.Spec.ForkedBackends == nil {
			routing.Spec.ForkedBackends = &rolloutv1alpha1.ForkedBackends{}
		}

		routing.Spec.ForkedBackends.Stable = rolloutv1alpha1.ForkedBackend{
			Name: routing.Spec.Backend.Name + "-stable",
		}

		routing.Spec.ForkedBackends.Canary = rolloutv1alpha1.ForkedBackend{
			Name: routing.Spec.Backend.Name + "-canary",
		}
		return nil
	})
}

func (m *Manager) DeleteForkedBackends(ctx context.Context) (controllerutil.OperationResult, error) {
	return m.mutateRouting(ctx, func(routing *rolloutv1alpha1.BackendRouting) error {
		routing.Spec.ForkedBackends = nil
		return nil
	})
}

func (m *Manager) InitializeRoute(ctx context.Context) (controllerutil.OperationResult, error) {
	return m.mutateRouting(ctx, func(routing *rolloutv1alpha1.BackendRouting) error {
		if routing.Spec.Forwarding == nil {
			routing.Spec.Forwarding = &rolloutv1alpha1.BackendForwarding{
				HTTP: &rolloutv1alpha1.HTTPForwarding{},
			}
		}
		if m.strategy.HTTP.StableTraffic == nil {
			routing.Spec.Forwarding.HTTP.Origin = &rolloutv1alpha1.OriginHTTPForwarding{
				BackendName: routing.Spec.ForkedBackends.Stable.Name,
			}
		} else {
			routing.Spec.Forwarding.HTTP.Stable = &rolloutv1alpha1.StableHTTPForwarding{
				BackendName:   routing.Spec.ForkedBackends.Stable.Name,
				HTTPRouteRule: *m.strategy.HTTP.StableTraffic,
			}
		}
		return nil
	})
}

func (m *Manager) ResetRoute(ctx context.Context) (controllerutil.OperationResult, error) {
	return m.mutateRouting(ctx, func(routing *rolloutv1alpha1.BackendRouting) error {
		routing.Spec.Forwarding = nil
		return nil
	})
}

func (m *Manager) AddCanaryRoute(ctx context.Context) (controllerutil.OperationResult, error) {
	return m.mutateRouting(ctx, func(routing *rolloutv1alpha1.BackendRouting) error {
		if routing.Spec.Forwarding == nil {
			routing.Spec.Forwarding = &rolloutv1alpha1.BackendForwarding{
				HTTP: &rolloutv1alpha1.HTTPForwarding{},
			}
		}
		routing.Spec.Forwarding.HTTP.Canary = &rolloutv1alpha1.CanaryHTTPForwarding{
			BackendName:         routing.Spec.ForkedBackends.Canary.Name,
			CanaryHTTPRouteRule: m.strategy.HTTP.CanaryHTTPRouteRule,
		}
		return nil
	})
}

func (m *Manager) DeleteCanaryRoute(ctx context.Context) (controllerutil.OperationResult, error) {
	return m.mutateRouting(ctx, func(routing *rolloutv1alpha1.BackendRouting) error {
		if routing.Spec.Forwarding != nil &&
			routing.Spec.Forwarding.HTTP != nil &&
			routing.Spec.Forwarding.HTTP.Canary != nil {
			routing.Spec.Forwarding.HTTP.Canary = nil
		}
		return nil
	})
}

func (m *Manager) mutateRouting(ctx context.Context, mutateFn func(routing *rolloutv1alpha1.BackendRouting) error) (controllerutil.OperationResult, error) {
	ctx = clusterinfo.WithCluster(ctx, clusterinfo.Fed)
	logger := logr.FromContextOrDiscard(ctx)
	operation := controllerutil.OperationResultNone
	if m.strategy == nil {
		logger.Info("no trafficrouting strategy found, skip it")
		return operation, nil
	}

	for _, workload := range m.targets {
		topo, ok := m.topoligies[workload.CrossClusterObjectNameReference]
		if !ok {
			logger.Info("no trafficrouting topology found for workload", "workload", workload.CrossClusterObjectNameReference)
			continue
		}
		for i := range topo.routings {
			routing := topo.routings[i]
			updated, err := clientutil.UpdateOnConflict(ctx, m.client, m.client, routing, func(routing *rolloutv1alpha1.BackendRouting) error {
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

func (m *Manager) CheckReady(ctx context.Context) bool {
	for _, workload := range m.targets {
		topo, ok := m.topoligies[workload.CrossClusterObjectNameReference]
		if !ok {
			continue
		}
		for _, routing := range topo.routings {
			if routing.Generation == routing.Status.ObservedGeneration &&
				meta.IsStatusConditionTrue(routing.Status.Conditions, rolloutv1alpha1.BackendRoutingReady) {
				continue
			}
			logger := logr.FromContextOrDiscard(ctx)
			logger.Info("backendRouting is not ready", "backendrouting", routing.Name)
			return false
		}
	}
	return true
}

type topology struct {
	workload rolloutv1alpha1.CrossClusterObjectNameReference
	routings []*rolloutv1alpha1.BackendRouting
}

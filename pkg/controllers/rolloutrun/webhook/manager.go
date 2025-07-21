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

package webhook

import (
	"fmt"
	"sync"

	"k8s.io/apimachinery/pkg/types"
	rolloutv1alpha1 "kusionstack.io/kube-api/rollout/v1alpha1"
)

type Manager interface {
	Start(runUID types.UID, webhook rolloutv1alpha1.RolloutWebhook, payload rolloutv1alpha1.RolloutWebhookReview) (WebhookWorker, error)
	Get(key types.UID) (WebhookWorker, bool)
	Stop(key types.UID)
}

type manager struct {
	// Map of active workers for probes
	workers map[types.UID]*worker
	// Lock for accessing & mutating workers
	workerLock sync.RWMutex
}

func NewManager() Manager {
	return &manager{
		workerLock: sync.RWMutex{},
		workers:    make(map[types.UID]*worker),
	}
}

func (m *manager) Get(key types.UID) (WebhookWorker, bool) {
	m.workerLock.RLock()
	defer m.workerLock.RUnlock()
	worker, ok := m.workers[key]
	if !ok {
		return nil, false
	}
	return worker, true
}

func (m *manager) removeWorker(key types.UID) {
	m.workerLock.Lock()
	defer m.workerLock.Unlock()
	delete(m.workers, key)
}

func (m *manager) Start(runUID types.UID, webhook rolloutv1alpha1.RolloutWebhook, review rolloutv1alpha1.RolloutWebhookReview) (WebhookWorker, error) {
	m.workerLock.Lock()
	defer m.workerLock.Unlock()

	_, ok := m.workers[runUID]
	if ok {
		return nil, fmt.Errorf("the same worker is still running, webhook: %v, review: %v", webhook, review)
	}

	worker := newWorker(m, runUID, webhook, review)
	go worker.run()
	m.workers[runUID] = worker
	return worker, nil
}

func (m *manager) Stop(key types.UID) {
	worker, ok := m.Get(key)
	if ok {
		worker.Stop()
	}
}

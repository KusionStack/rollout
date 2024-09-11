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

package options

import (
	"time"

	"github.com/spf13/pflag"
	corev1 "k8s.io/api/core/v1"

	rolloutv1alpha1 "kusionstack.io/rollout/apis/rollout/v1alpha1"
)

var GroupKindConcurrency = map[string]int{
	rolloutv1alpha1.SchemeGroupVersion.WithKind("Rollout").GroupKind().String():         10,
	rolloutv1alpha1.SchemeGroupVersion.WithKind("RolloutRun").GroupKind().String():      20,
	rolloutv1alpha1.SchemeGroupVersion.WithKind("TrafficTopology").GroupKind().String(): 10,
	rolloutv1alpha1.SchemeGroupVersion.WithKind("BackendRouting").GroupKind().String():  10,
	corev1.SchemeGroupVersion.WithKind("Pod").GroupKind().String():                      50,
}

var _ subOptions = &ControllerOptions{}

type ControllerOptions struct {
	LeaderElect             bool
	LeaderElectionNamespace string
	LeaderElectionID        string
	FederatedMode           bool
	MaxConcurrentWorkers    int

	GroupKindConcurrency map[string]int
	CacheSyncTimeout     time.Duration
}

func NewControllerOptions() *ControllerOptions {
	return &ControllerOptions{
		LeaderElect:             true,
		LeaderElectionNamespace: "kusionstack-rollout",
		LeaderElectionID:        "rollout-controller",
		FederatedMode:           true,
		MaxConcurrentWorkers:    10,
		GroupKindConcurrency:    GroupKindConcurrency,
		CacheSyncTimeout:        10 * time.Minute,
	}
}

// BindFlags implements suboptions.
func (o *ControllerOptions) BindFlags(fs *pflag.FlagSet) {
	fs.BoolVar(&o.LeaderElect, "leader-elect", o.LeaderElect,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	fs.StringVar(&o.LeaderElectionNamespace, "leader-election-namespace", o.LeaderElectionNamespace, "Namespace used to store the leader lock.")
	fs.StringVar(&o.LeaderElectionID, "leader-election-id", o.LeaderElectionID, "The name of the resource that leader election.")
	fs.BoolVar(&o.FederatedMode, "federated-mode", o.FederatedMode, "Enable federated mode for controller manager.")
	fs.IntVar(&o.MaxConcurrentWorkers, "max-concurrent-workers", o.MaxConcurrentWorkers, "The number of concurrent workers for the controller.")
	fs.StringToIntVar(&o.GroupKindConcurrency, "group-kind-concurrency", o.GroupKindConcurrency, "The number of concurrent workers for each controller group kind. The key is expected to be consistent in form with GroupKind.String()")
	fs.DurationVar(&o.CacheSyncTimeout, "cache-sync-timeout", o.CacheSyncTimeout, "The time limit set to wait for syncing caches.")
}

// Validate implements suboptions.
func (o *ControllerOptions) Validate() []error {
	return nil
}

// Complete implements suboptions.
func (o *ControllerOptions) Complete() error {
	return nil
}

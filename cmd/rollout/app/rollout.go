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

package app

import (
	"os"
	"time"

	"github.com/spf13/cobra"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/component-base/version/verflag"
	"kusionstack.io/kube-utils/multicluster"
	"kusionstack.io/kube-utils/multicluster/clusterinfo"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/healthz"

	"kusionstack.io/rollout/cmd/rollout/app/options"
	"kusionstack.io/rollout/pkg/controllers/initializers"
	"kusionstack.io/rollout/pkg/utils/cli"
	"kusionstack.io/rollout/pkg/webhook"
)

var setupLog = ctrl.Log.WithName("setup")

func NewRolloutCommand(opt *options.Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:          "rollout",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			// validate options
			if errs := opt.Validate(); len(errs) > 0 {
				return utilerrors.NewAggregate(errs)
			}

			if err := opt.Complete(); err != nil {
				return err
			}

			verflag.PrintAndExitIfRequested()
			cli.PrintFlags(setupLog, cmd.Flags())

			return Run(opt)
		},
	}

	cli.AddFlagsAndUsage(cmd, opt.Flags(initializers.Controllers, webhook.Initializer))

	return cmd
}

func Run(opt *options.Options) error {
	ctx := ctrl.SetupSignalHandler()

	options := ctrl.Options{
		Scheme:                 scheme.Scheme,
		MetricsBindAddress:     opt.MetricsBindAddress,
		Port:                   9443,
		CertDir:                opt.CertDir,
		HealthProbeBindAddress: opt.HealthProbeBindAddress,
		LeaderElection:         opt.LeaderElect,
		LeaderElectionID:       "rollout.kusionstack.io",
		// LeaderElectionReleaseOnCancel defines if the leader should step down voluntarily
		// when the Manager ends. This requires the binary to immediately end when the
		// Manager is stopped, otherwise, this setting is unsafe. Setting this significantly
		// speeds up voluntary leader transitions as the new leader don't have to wait
		// LeaseDuration time first.
		//
		// In the default scaffold provided, the program ends immediately after
		// the manager stops, so would be fine to enable this option. However,
		// if you are doing or is intended to do any operation such as perform cleanups
		// after the manager stops then its usage might be unsafe.
		// LeaderElectionReleaseOnCancel: true,
	}

	restConfig := GetRESTConfigOrDie()

	if opt.FederatedMode {
		setupLog.Info("federated mode enabled")
		// multiClusterManager manages syncing clusters
		multiClusterCfg := &multicluster.ManagerConfig{
			ClusterProvider: opt.ClusterProvider,
			FedConfig:       restConfig,
			ClusterScheme:   scheme.Scheme,
			ResyncPeriod:    0 * time.Second,
			Log:             ctrl.Log.WithName("multicluster"),
		}
		multiClusterManager, newMultiClusterCache, newMultiClusterClient, err := multicluster.NewManager(multiClusterCfg, multicluster.Options{})
		if err != nil {
			setupLog.Error(err, "unable to start multiClusterManager")
			os.Exit(1)
		}
		options.NewClient = newMultiClusterClient
		options.NewCache = newMultiClusterCache

		go func() {
			if err := multiClusterManager.Run(opt.ControllerConcurrentWorkers, ctx); err != nil {
				setupLog.Error(err, "unable to run multiClusterManager")
				os.Exit(1)
			}
		}()
		multiClusterManager.WaitForSynced(ctx)
		ctx = clusterinfo.WithCluster(ctx, clusterinfo.Fed)
		setupLog.Info("multiClusterManager synced", "clusters", multiClusterManager.SyncedClusters())
	} else {
		setupLog.Info("federated mode disabled")
	}

	mgr, err := ctrl.NewManager(restConfig, options)
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		return err
	}

	err = initializers.Background.SetupWithManager(mgr)
	if err != nil {
		setupLog.Error(err, "failed to setup background initializers")
		return err
	}

	err = initializers.Controllers.SetupWithManager(mgr)
	if err != nil {
		setupLog.Error(err, "failed to setup controller initializers")
		return err
	}

	err = webhook.Initializer.SetupWithManager(mgr)
	if err != nil {
		setupLog.Error(err, "failed to setup webhooks initializers")
		return err
	}

	//+kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "failed to setup health check")
		return err
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "failed to setup ready check")
		return err
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctx); err != nil {
		setupLog.Error(err, "failed to start controller manager")
		return err
	}
	return nil
}

func GetRESTConfigOrDie() *rest.Config {
	restConfig := config.GetConfigOrDie()
	restConfig.QPS = 100
	restConfig.Burst = 200
	return restConfig
}

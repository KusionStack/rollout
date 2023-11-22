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

package main

import (
	"flag"
	"os"
	"time"

	"github.com/spf13/pflag"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
	"kusionstack.io/kube-utils/multicluster"
	"kusionstack.io/kube-utils/multicluster/clusterinfo"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.

	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"

	workflowv1alpha1 "kusionstack.io/rollout/apis/workflow/v1alpha1"
	"kusionstack.io/rollout/pkg/controllers/rollout"
	"kusionstack.io/rollout/pkg/controllers/rolloutrun"
	"kusionstack.io/rollout/pkg/controllers/task"
	"kusionstack.io/rollout/pkg/controllers/workflow"
	"kusionstack.io/rollout/pkg/features"
	//+kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	var federatedMode bool
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.BoolVar(&federatedMode, "federated-mode", true, "Enable federated mode for controller manager.")

	// init klog flags
	klog.InitFlags(nil)
	defer klog.Flush()

	// init zap flags
	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)

	features.DefaultMutableFeatureGate.AddFlag(pflag.CommandLine)

	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)
	pflag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	ctx := ctrl.SetupSignalHandler()

	options := ctrl.Options{
		Scheme:                 scheme,
		MetricsBindAddress:     metricsAddr,
		Port:                   9443,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "08fb1aad.kafe.kusionstack.io",
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

	if federatedMode {
		setupLog.Info("federated mode enabled")
		// multiClusterManager manages syncing clusters
		multiClusterCfg := &multicluster.ManagerConfig{
			FedConfig:     config.GetConfigOrDie(),
			ClusterScheme: scheme,
			ResyncPeriod:  0 * time.Second,
			Log:           ctrl.Log.WithName("multicluster"),
		}
		multiClusterManager, newMultiClusterCache, newMultiClusterClient, err := multicluster.NewManager(multiClusterCfg)
		if err != nil {
			setupLog.Error(err, "unable to start multiClusterManager")
			os.Exit(1)
		}
		options.NewClient = newMultiClusterClient
		options.NewCache = newMultiClusterCache

		go func() {
			if err := multiClusterManager.Run(1, ctx); err != nil {
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

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), options)
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	if err = (&rollout.RolloutReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Rollout")
		os.Exit(1)
	}
	if err = (&rolloutrun.RolloutRunReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Rollout")
		os.Exit(1)
	}

	if err = (&task.TaskReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor("task-controller"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Task")
		os.Exit(1)
	}
	if err = (&workflow.WorkflowReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor("workflow-controller"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Workflow")
		os.Exit(1)
	}
	if os.Getenv("ENABLE_WEBHOOKS_WORKFLOW") == "true" {
		if err = (&workflowv1alpha1.Workflow{}).SetupWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "Workflow")
			os.Exit(1)
		}
		setupLog.Info("webhook enabled")
	}

	//+kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctx); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

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

package rollout

import (
	"context"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"time"

	"github.com/stretchr/testify/suite"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
	"k8s.io/klog/v2/klogr"
	"k8s.io/utils/ptr"
	rolloutapi "kusionstack.io/kube-api/rollout"
	rolloutv1alpha1 "kusionstack.io/kube-api/rollout/v1alpha1"
	"kusionstack.io/kube-api/rollout/v1alpha1/condition"
	"kusionstack.io/kube-utils/multicluster"
	"kusionstack.io/kube-utils/multicluster/clusterinfo"
	"kusionstack.io/kube-utils/multicluster/clusterprovider"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"kusionstack.io/rollout/pkg/controllers/registry"
	"kusionstack.io/rollout/pkg/features"
	"kusionstack.io/rollout/pkg/features/ontimestrategy"
)

type rolloutTestSuite struct {
	suite.Suite

	fedClient      client.Client
	cluster1Client client.Client
	cluster2Client client.Client

	rollout *rolloutv1alpha1.Rollout

	// Environment cleanup
	testClusterEnv *envtest.Environment
	ctx            context.Context
	cancel         context.CancelFunc
}

func (s *rolloutTestSuite) setupCluster(env *envtest.Environment) (*rest.Config, client.Client) {
	config, err := env.Start()
	s.Require().NoError(err)
	s.Require().NotNil(config)

	c, err := client.New(config, client.Options{Scheme: env.Scheme})
	s.Require().NoError(err)
	s.Require().NotNil(c)

	return config, c
}

func (s *rolloutTestSuite) SetupSuite() {
	local := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	klog.InitFlags(local)
	local.Set("v", "3")

	// Enable OneTimeStrategy feature gate for tests
	if err := features.DefaultMutableFeatureGate.Set("OneTimeStrategy=true"); err != nil {
		panic(err)
	}

	logf.SetLogger(klogr.New())

	// Create cancellable context for cleanup
	s.ctx, s.cancel = context.WithCancel(context.Background())

	// fed
	testscheme := scheme.Scheme
	err := rolloutv1alpha1.AddToScheme(testscheme)
	s.Require().NoError(err)

	s.testClusterEnv = &envtest.Environment{
		Scheme: testscheme,
		CRDInstallOptions: envtest.CRDInstallOptions{
			Paths: []string{filepath.Join("..", "..", "..", "config", "crd", "bases")},
		},
	}

	var (
		fedConfig      *rest.Config
		cluster1Config *rest.Config
		cluster2Config *rest.Config
	)

	fedConfig, s.fedClient = s.setupCluster(s.testClusterEnv)
	cluster1Config, s.cluster1Client = s.setupCluster(s.testClusterEnv)
	cluster2Config, s.cluster2Client = s.setupCluster(s.testClusterEnv)

	// manager
	os.Setenv(clusterinfo.EnvClusterAllowList, "cluster1,cluster2")

	clusterMgr, newCacheFunc, newClientFunc, err := multicluster.NewManager(&multicluster.ManagerConfig{
		ClusterProvider: clusterprovider.NewSimpleClusterProvider(map[string]*rest.Config{
			"cluster1": cluster1Config,
			"cluster2": cluster2Config,
			"fed":      fedConfig,
		}),
		FedConfig:     fedConfig,
		ClusterScheme: testscheme,
		ResyncPeriod:  10 * time.Minute,
	}, multicluster.Options{})

	s.Require().NoError(err)

	go func() {
		// start multi cluster client manager
		err := clusterMgr.Run(s.ctx)
		if err != nil {
			panic(err)
		}
	}()

	// wait for cache synced
	s.Require().True(clusterMgr.WaitForSynced(s.ctx))

	s.ctx = clusterinfo.WithCluster(s.ctx, clusterinfo.Fed)

	mgr, err := manager.New(fedConfig, manager.Options{
		Scheme:                 testscheme,
		NewClient:              newClientFunc,
		NewCache:               newCacheFunc,
		MetricsBindAddress:     "0",
		HealthProbeBindAddress: "0",
	})

	s.Require().NoError(err)

	_, err = registry.InitWorkloadRegistry(mgr)
	s.Require().NoError(err)

	_, err = InitFunc(mgr)
	s.Require().NoError(err)

	go func() {
		err := mgr.Start(s.ctx)
		if err != nil {
			panic(err)
		}
	}()
}

func (s *rolloutTestSuite) TearDownSuite() {
	// Cancel context to stop manager and cluster manager
	if s.cancel != nil {
		s.cancel()
	}
	// Stop envtest environment
	if s.testClusterEnv != nil {
		_ = s.testClusterEnv.Stop()
	}
}

func (s *rolloutTestSuite) SetupTest() {
	namespace := "default"
	s.rollout = newTestRollout("test-rollout", namespace)
}

func (s *rolloutTestSuite) TearDownTest() {
	err := s.fedClient.Delete(context.Background(), s.rollout)
	s.Require().NoError(client.IgnoreNotFound(err))
}

func (s *rolloutTestSuite) rolloutShouldBeAvailable(obj *rolloutv1alpha1.Rollout) bool {
	if obj.Generation != obj.Status.ObservedGeneration {
		logf.Log.Info("generation not equal", "observedGeneration", obj.Status.ObservedGeneration, "generation", obj.Generation)
		return false
	}
	if !condition.IsAvailable(obj.Status.Conditions) {
		logf.Log.Info("Available condition should be true", "conditions", obj.Status.Conditions)
		return false
	}
	return true
}

func (s *rolloutTestSuite) rolloutShouldHaveFinalizer(obj *rolloutv1alpha1.Rollout) bool {
	for _, f := range obj.Finalizers {
		if f == rolloutapi.FinalizerRolloutProtection {
			return true
		}
	}
	return false
}

type RolloutInitializationTestSuite struct {
	rolloutTestSuite
}

func (s *RolloutInitializationTestSuite) Test_CreateRollout() {
	// create rollout
	err := s.fedClient.Create(context.Background(), s.rollout)
	s.Require().NoError(err)

	s.Require().Eventually(func() bool {
		obj := &rolloutv1alpha1.Rollout{}
		err := s.fedClient.Get(context.Background(), client.ObjectKeyFromObject(s.rollout), obj)
		if err != nil {
			return false
		}

		// check finalizer
		if !s.rolloutShouldHaveFinalizer(obj) {
			logf.Log.Info("finalizer not found")
			return false
		}

		if obj.Generation != obj.Status.ObservedGeneration {
			logf.Log.Info("generation not equal", "observedGeneration", obj.Status.ObservedGeneration, "generation", obj.Generation)
			return false
		}

		if obj.Status.Phase != rolloutv1alpha1.RolloutPhaseInitialized {
			logf.Log.Info("phase should be Initialized", "phase", obj.Status.Phase)
			return false
		}

		return true
	}, 30*time.Second, 2*time.Second)
}

func (s *RolloutInitializationTestSuite) Test_CreateRolloutWithStrategyRef() {
	// create rollout strategy
	strategy := newTestRolloutStrategy("test-strategy", s.rollout.Namespace)
	err := s.fedClient.Create(context.Background(), strategy)
	s.Require().NoError(err)

	defer func() {
		_ = s.fedClient.Delete(context.Background(), strategy)
	}()

	// create rollout with strategy ref
	s.rollout.Spec.StrategyRef = strategy.Name
	s.rollout.Name = "test-rollout-with-strategy"
	err = s.fedClient.Create(context.Background(), s.rollout)
	s.Require().NoError(err)

	s.Require().Eventually(func() bool {
		obj := &rolloutv1alpha1.Rollout{}
		err := s.fedClient.Get(context.Background(), client.ObjectKeyFromObject(s.rollout), obj)
		if err != nil {
			return false
		}

		return s.rolloutShouldBeAvailable(obj)
	}, 30*time.Second, 2*time.Second)
}

func (s *RolloutInitializationTestSuite) Test_CreateRolloutWithV2BatchStrategy() {
	// create RolloutStrategy with V2 batch strategy
	strategy := &rolloutv1alpha1.RolloutStrategy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-strategy-v2-batch",
			Namespace: s.rollout.Namespace,
		},
		BatchV2: &rolloutv1alpha1.BatchStrategyV2{
			Batches: []rolloutv1alpha1.RolloutBatchStrategyStep{
				{
					Breakpoint: true,
					Targets: []rolloutv1alpha1.RolloutStrategyTargets{
						{Replicas: intstr.FromString("100%")},
					},
				},
			},
		},
	}
	err := s.fedClient.Create(context.Background(), strategy)
	s.Require().NoError(err)

	defer func() {
		_ = s.fedClient.Delete(context.Background(), strategy)
	}()

	// create rollout referencing the V2 strategy
	s.rollout.Name = "test-rollout-v2-batch"
	s.rollout.Spec.StrategyRef = strategy.Name
	err = s.fedClient.Create(context.Background(), s.rollout)
	s.Require().NoError(err)

	s.Require().Eventually(func() bool {
		obj := &rolloutv1alpha1.Rollout{}
		err := s.fedClient.Get(context.Background(), client.ObjectKeyFromObject(s.rollout), obj)
		if err != nil {
			return false
		}

		if !s.rolloutShouldHaveFinalizer(obj) {
			return false
		}

		return s.rolloutShouldBeAvailable(obj)
	}, 30*time.Second, 2*time.Second)
}

func (s *RolloutInitializationTestSuite) Test_CreateRolloutWithV2CanaryStrategy() {
	// create RolloutStrategy with V2 canary + batch strategy
	strategy := &rolloutv1alpha1.RolloutStrategy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-strategy-v2-canary",
			Namespace: s.rollout.Namespace,
		},
		CanaryV2: &rolloutv1alpha1.CanaryStrategyV2{
			Targets: []rolloutv1alpha1.RolloutStrategyTargets{
				{Replicas: intstr.FromString("10%")},
			},
		},
		BatchV2: &rolloutv1alpha1.BatchStrategyV2{
			Batches: []rolloutv1alpha1.RolloutBatchStrategyStep{
				{
					Breakpoint: true,
					Targets: []rolloutv1alpha1.RolloutStrategyTargets{
						{Replicas: intstr.FromString("100%")},
					},
				},
			},
		},
	}
	err := s.fedClient.Create(context.Background(), strategy)
	s.Require().NoError(err)

	defer func() {
		_ = s.fedClient.Delete(context.Background(), strategy)
	}()

	// create rollout referencing the V2 canary+batch strategy
	s.rollout.Name = "test-rollout-v2-canary"
	s.rollout.Spec.StrategyRef = strategy.Name
	err = s.fedClient.Create(context.Background(), s.rollout)
	s.Require().NoError(err)

	s.Require().Eventually(func() bool {
		obj := &rolloutv1alpha1.Rollout{}
		err := s.fedClient.Get(context.Background(), client.ObjectKeyFromObject(s.rollout), obj)
		if err != nil {
			return false
		}

		if !s.rolloutShouldHaveFinalizer(obj) {
			return false
		}

		return s.rolloutShouldBeAvailable(obj)
	}, 30*time.Second, 2*time.Second)
}

type RolloutControllerTestSuite struct {
	rolloutTestSuite
}

func (s *RolloutControllerTestSuite) TearDownTest() {
	// Override parent's TearDownTest to do nothing since each test manages its own cleanup
}

func (s *RolloutControllerTestSuite) Test_StrategyRefNotFound() {
	ctx := context.Background()
	rollout := newTestRollout("test-rollout-no-strategy", "default")
	rollout.Spec.StrategyRef = "non-existent-strategy"

	err := s.fedClient.Create(ctx, rollout)
	s.Require().NoError(err)

	// Cleanup
	defer func() {
		_ = s.fedClient.Delete(ctx, rollout)
	}()

	s.Require().Eventually(func() bool {
		obj := &rolloutv1alpha1.Rollout{}
		err := s.fedClient.Get(ctx, client.ObjectKeyFromObject(rollout), obj)
		if err != nil {
			return false
		}

		// should not be available when strategy ref not found
		if condition.IsAvailable(obj.Status.Conditions) {
			logf.Log.Info("Available condition should be false when strategy ref not found")
			return false
		}

		return true
	}, 20*time.Second, 2*time.Second)
}

func (s *RolloutControllerTestSuite) Test_TriggerRolloutRunWithStrategyRef() {
	ctx := context.Background()
	namespace := "default"
	stsName := "test-sts-strategyref"

	// 1. Create StatefulSet as workload
	sts := newTestStatefulSet(stsName, namespace)
	err := s.cluster1Client.Create(ctx, sts)
	s.Require().NoError(err)

	// Cleanup
	defer func() {
		_ = s.cluster1Client.Delete(ctx, sts)
	}()

	// 2. Create RolloutStrategy
	strategy := &rolloutv1alpha1.RolloutStrategy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-strategy-for-run",
			Namespace: namespace,
		},
		Batch: &rolloutv1alpha1.BatchStrategy{
			Batches: []rolloutv1alpha1.RolloutStep{
				{
					Breakpoint: true,
					Replicas:   intstr.FromString("25%"),
				},
				{
					Replicas: intstr.FromString("50%"),
				},
				{
					Replicas: intstr.FromString("100%"),
				},
			},
		},
	}
	err = s.fedClient.Create(ctx, strategy)
	s.Require().NoError(err)

	// Cleanup
	defer func() {
		_ = s.fedClient.Delete(ctx, strategy)
	}()

	// 3. Create Rollout with StrategyRef and trigger annotation
	rollout := newTestRollout("test-rollout-strategyref-run", namespace)
	rollout.Spec.StrategyRef = strategy.Name
	rollout.Spec.WorkloadRef.Match = rolloutv1alpha1.ResourceMatch{
		Names: []rolloutv1alpha1.CrossClusterObjectNameReference{
			{
				Cluster: "cluster1",
				Name:    stsName,
			},
		},
	}
	rollout.Annotations = map[string]string{
		rolloutapi.AnnoRolloutTrigger: "trigger-strategyref-run",
	}
	err = s.fedClient.Create(ctx, rollout)
	s.Require().NoError(err)

	// Cleanup
	defer func() {
		_ = s.fedClient.Delete(ctx, rollout)
	}()

	// 4. Wait for Rollout to become Available first
	s.Require().Eventually(func() bool {
		obj := &rolloutv1alpha1.Rollout{}
		err := s.fedClient.Get(ctx, client.ObjectKeyFromObject(rollout), obj)
		if err != nil {
			return false
		}
		return s.rolloutShouldBeAvailable(obj)
	}, 30*time.Second, 2*time.Second)

	// 5. Wait for RolloutRun to be created and verify batch info
	var run *rolloutv1alpha1.RolloutRun
	s.Require().Eventually(func() bool {
		runList := &rolloutv1alpha1.RolloutRunList{}
		err := s.fedClient.List(ctx, runList, client.InNamespace(namespace))
		if err != nil {
			return false
		}

		for i := range runList.Items {
			r := &runList.Items[i]
			owner := metav1.GetControllerOf(r)
			if owner != nil && owner.Name == rollout.Name && r.Name == "trigger-strategyref-run" {
				run = r
				return true
			}
		}
		return false
	}, 30*time.Second, 2*time.Second)

	defer func() {
		_ = s.fedClient.Delete(ctx, run)
	}()

	// 5. Verify RolloutRun spec
	s.Require().NotNil(run.Spec.Batch, "Batch should not be nil")
	s.Require().Len(run.Spec.Batch.Batches, 3, "Should have 3 batches")
	s.Require().True(run.Spec.Batch.Batches[0].Breakpoint, "First batch should have breakpoint")
	s.Require().False(run.Spec.Batch.Batches[1].Breakpoint, "Second batch should not have breakpoint")
	s.Require().False(run.Spec.Batch.Batches[2].Breakpoint, "Third batch should not have breakpoint")

	// Verify targets are set correctly
	s.Require().Len(run.Spec.Batch.Batches[0].Targets, 1, "First batch should have 1 target")
	s.Require().Equal("cluster1", run.Spec.Batch.Batches[0].Targets[0].Cluster)
	s.Require().Equal(stsName, run.Spec.Batch.Batches[0].Targets[0].Name)
}

func (s *RolloutControllerTestSuite) Test_TriggerRolloutRunWithV2BatchStrategy() {
	ctx := context.Background()
	namespace := "default"
	stsName := "test-sts-v2-batch"

	// 1. Create StatefulSet as workload
	sts := newTestStatefulSet(stsName, namespace)
	err := s.cluster1Client.Create(ctx, sts)
	s.Require().NoError(err)

	// Cleanup
	defer func() {
		_ = s.cluster1Client.Delete(ctx, sts)
	}()

	// 2. Create RolloutStrategy with BatchV2
	strategy := &rolloutv1alpha1.RolloutStrategy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-strategy-v2-batch-run",
			Namespace: namespace,
		},
		BatchV2: &rolloutv1alpha1.BatchStrategyV2{
			Batches: []rolloutv1alpha1.RolloutBatchStrategyStep{
				{
					Targets: []rolloutv1alpha1.RolloutStrategyTargets{
						{Replicas: intstr.FromString("30%")},
					},
				},
				{
					Breakpoint: true,
					Targets: []rolloutv1alpha1.RolloutStrategyTargets{
						{Replicas: intstr.FromString("100%")},
					},
				},
			},
		},
	}
	err = s.fedClient.Create(ctx, strategy)
	s.Require().NoError(err)

	// Cleanup
	defer func() {
		_ = s.fedClient.Delete(ctx, strategy)
	}()

	// 3. Create Rollout with StrategyRef (V2) and trigger annotation
	rollout := newTestRollout("test-rollout-v2-batch-run", namespace)
	rollout.Spec.StrategyRef = strategy.Name
	rollout.Spec.WorkloadRef.Match = rolloutv1alpha1.ResourceMatch{
		Names: []rolloutv1alpha1.CrossClusterObjectNameReference{
			{
				Cluster: "cluster1",
				Name:    stsName,
			},
		},
	}
	rollout.Annotations = map[string]string{
		rolloutapi.AnnoRolloutTrigger: "trigger-v2-batch-run",
	}
	err = s.fedClient.Create(ctx, rollout)
	s.Require().NoError(err)

	// Cleanup
	defer func() {
		_ = s.fedClient.Delete(ctx, rollout)
	}()

	// 3. Wait for Rollout to become Available first
	s.Require().Eventually(func() bool {
		obj := &rolloutv1alpha1.Rollout{}
		err := s.fedClient.Get(ctx, client.ObjectKeyFromObject(rollout), obj)
		if err != nil {
			return false
		}
		return s.rolloutShouldBeAvailable(obj)
	}, 30*time.Second, 2*time.Second)

	// 4. Wait for RolloutRun to be created and verify batch info
	var run *rolloutv1alpha1.RolloutRun
	s.Require().Eventually(func() bool {
		runList := &rolloutv1alpha1.RolloutRunList{}
		err := s.fedClient.List(ctx, runList, client.InNamespace(namespace))
		if err != nil {
			return false
		}

		for i := range runList.Items {
			r := &runList.Items[i]
			owner := metav1.GetControllerOf(r)
			if owner != nil && owner.Name == rollout.Name && r.Name == "trigger-v2-batch-run" {
				run = r
				return true
			}
		}
		return false
	}, 30*time.Second, 2*time.Second)

	defer func() {
		_ = s.fedClient.Delete(ctx, run)
	}()

	// 5. Verify RolloutRun spec matches inline batch strategy
	s.Require().NotNil(run.Spec.Batch, "Batch should not be nil")
	s.Require().Len(run.Spec.Batch.Batches, 2, "Should have 2 batches")

	// Verify first batch
	s.Require().Len(run.Spec.Batch.Batches[0].Targets, 1, "First batch should have 1 target")
	s.Require().Equal("cluster1", run.Spec.Batch.Batches[0].Targets[0].Cluster)
	s.Require().Equal(stsName, run.Spec.Batch.Batches[0].Targets[0].Name)
	s.Require().Equal("30%", run.Spec.Batch.Batches[0].Targets[0].Replicas.String())

	// Verify second batch
	s.Require().True(run.Spec.Batch.Batches[1].Breakpoint, "Second batch should have breakpoint")
	s.Require().Len(run.Spec.Batch.Batches[1].Targets, 1, "Second batch should have 1 target")
	s.Require().Equal("100%", run.Spec.Batch.Batches[1].Targets[0].Replicas.String())
}

func (s *RolloutControllerTestSuite) Test_ApplyOneTimeStrategyWithV2Batch() {
	ctx := context.Background()
	namespace := "default"
	stsName := "test-sts-onetime-v2"

	// 1. Create StatefulSet as workload
	sts := newTestStatefulSet(stsName, namespace)
	err := s.cluster1Client.Create(ctx, sts)
	s.Require().NoError(err)

	// Cleanup
	defer func() {
		_ = s.cluster1Client.Delete(ctx, sts)
	}()

	// 2. Create RolloutStrategy with BatchV2
	strategy := &rolloutv1alpha1.RolloutStrategy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-strategy-onetime-v2",
			Namespace: namespace,
		},
		BatchV2: &rolloutv1alpha1.BatchStrategyV2{
			Batches: []rolloutv1alpha1.RolloutBatchStrategyStep{
				{
					Breakpoint: true,
					Targets: []rolloutv1alpha1.RolloutStrategyTargets{
						{Replicas: intstr.FromString("50%")},
					},
				},
			},
		},
	}
	err = s.fedClient.Create(ctx, strategy)
	s.Require().NoError(err)

	// Cleanup
	defer func() {
		_ = s.fedClient.Delete(ctx, strategy)
	}()

	// 3. Create Rollout with StrategyRef (V2) and trigger annotation
	rollout := newTestRollout("test-rollout-onetime-v2", namespace)
	rollout.Spec.StrategyRef = strategy.Name
	rollout.Spec.WorkloadRef.Match = rolloutv1alpha1.ResourceMatch{
		Names: []rolloutv1alpha1.CrossClusterObjectNameReference{
			{
				Cluster: "cluster1",
				Name:    stsName,
			},
		},
	}
	rollout.Annotations = map[string]string{
		rolloutapi.AnnoRolloutTrigger: "trigger-onetime-v2",
	}
	err = s.fedClient.Create(ctx, rollout)
	s.Require().NoError(err)

	// Cleanup
	defer func() {
		_ = s.fedClient.Delete(ctx, rollout)
	}()

	// 3. Wait for Rollout to become Available first
	s.Require().Eventually(func() bool {
		obj := &rolloutv1alpha1.Rollout{}
		err := s.fedClient.Get(ctx, client.ObjectKeyFromObject(rollout), obj)
		if err != nil {
			return false
		}
		return s.rolloutShouldBeAvailable(obj)
	}, 30*time.Second, 2*time.Second)

	// 4. Wait for RolloutRun to be created
	var run *rolloutv1alpha1.RolloutRun
	s.Require().Eventually(func() bool {
		runList := &rolloutv1alpha1.RolloutRunList{}
		err := s.fedClient.List(ctx, runList, client.InNamespace(namespace))
		if err != nil {
			return false
		}

		for i := range runList.Items {
			r := &runList.Items[i]
			owner := metav1.GetControllerOf(r)
			if owner != nil && owner.Name == rollout.Name && r.Name == "trigger-onetime-v2" {
				run = r
				return true
			}
		}
		return false
	}, 30*time.Second, 2*time.Second)

	// 5. Verify initial batch
	s.Require().NotNil(run.Spec.Batch, "Batch should not be nil")
	s.Require().Len(run.Spec.Batch.Batches, 1, "Should have 1 batch initially")
	s.Require().Equal("50%", run.Spec.Batch.Batches[0].Targets[0].Replicas.String())

	// 6. Apply one-time strategy via annotation using BatchV2
	oneTimeStrategy := ontimestrategy.OneTimeStrategy{
		BatchV2: &rolloutv1alpha1.BatchStrategyV2{
			Batches: []rolloutv1alpha1.RolloutBatchStrategyStep{
				{
					Targets: []rolloutv1alpha1.RolloutStrategyTargets{
						{Replicas: intstr.FromString("20%")},
					},
				},
				{
					Targets: []rolloutv1alpha1.RolloutStrategyTargets{
						{Replicas: intstr.FromString("40%")},
					},
				},
				{
					Targets: []rolloutv1alpha1.RolloutStrategyTargets{
						{Replicas: intstr.FromString("60%")},
					},
				},
			},
		},
	}
	oneTimeData, err := json.Marshal(oneTimeStrategy)
	s.Require().NoError(err)

	// Update rollout with one-time strategy annotation
	err = s.fedClient.Get(ctx, client.ObjectKeyFromObject(rollout), rollout)
	s.Require().NoError(err)
	if rollout.Annotations == nil {
		rollout.Annotations = make(map[string]string)
	}
	rollout.Annotations[ontimestrategy.AnnoOneTimeStrategy] = string(oneTimeData)
	err = s.fedClient.Update(ctx, rollout)
	s.Require().NoError(err)

	// 7. Verify RolloutRun is updated with new batch strategy
	s.Require().Eventually(func() bool {
		err := s.fedClient.Get(ctx, client.ObjectKey{Name: run.Name, Namespace: namespace}, run)
		if err != nil {
			return false
		}
		return len(run.Spec.Batch.Batches) == 3
	}, 30*time.Second, 2*time.Second)

	defer func() {
		_ = s.fedClient.Delete(ctx, run)
	}()

	s.Require().Len(run.Spec.Batch.Batches, 3, "Should have 3 batches after one-time strategy applied")
	s.Require().Equal("20%", run.Spec.Batch.Batches[0].Targets[0].Replicas.String())
	s.Require().Equal("40%", run.Spec.Batch.Batches[1].Targets[0].Replicas.String())
	s.Require().Equal("60%", run.Spec.Batch.Batches[2].Targets[0].Replicas.String())

	// 8. Verify one-time strategy annotation is on RolloutRun
	s.Require().Contains(run.Annotations, ontimestrategy.AnnoOneTimeStrategy, "RolloutRun should have one-time strategy annotation")
}

func (s *RolloutControllerTestSuite) Test_ApplyOneTimeStrategyWithStrategyRef() {
	ctx := context.Background()
	namespace := "default"
	stsName := "test-sts-onetime-strategyref"

	// 1. Create StatefulSet as workload
	sts := newTestStatefulSet(stsName, namespace)
	err := s.cluster1Client.Create(ctx, sts)
	s.Require().NoError(err)

	// Cleanup
	defer func() {
		_ = s.cluster1Client.Delete(ctx, sts)
	}()

	// 2. Create RolloutStrategy
	strategy := &rolloutv1alpha1.RolloutStrategy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-strategy-onetime",
			Namespace: namespace,
		},
		Batch: &rolloutv1alpha1.BatchStrategy{
			Batches: []rolloutv1alpha1.RolloutStep{
				{
					Replicas: intstr.FromString("30%"),
				},
				{
					Breakpoint: true,
					Replicas:   intstr.FromString("60%"),
				},
				{
					Breakpoint: true,
					Replicas:   intstr.FromString("100%"),
				},
			},
		},
	}
	err = s.fedClient.Create(ctx, strategy)
	s.Require().NoError(err)

	// Cleanup
	defer func() {
		_ = s.fedClient.Delete(ctx, strategy)
	}()

	// 3. Create Rollout with StrategyRef and trigger annotation
	rollout := newTestRollout("test-rollout-onetime-strategyref", namespace)
	rollout.Spec.StrategyRef = strategy.Name
	rollout.Spec.WorkloadRef.Match = rolloutv1alpha1.ResourceMatch{
		Names: []rolloutv1alpha1.CrossClusterObjectNameReference{
			{
				Cluster: "cluster1",
				Name:    stsName,
			},
		},
	}
	rollout.Annotations = map[string]string{
		rolloutapi.AnnoRolloutTrigger: "trigger-onetime-strategyref",
	}
	err = s.fedClient.Create(ctx, rollout)
	s.Require().NoError(err)

	// Cleanup
	defer func() {
		_ = s.fedClient.Delete(ctx, rollout)
	}()

	// 4. Wait for Rollout to become Available first
	s.Require().Eventually(func() bool {
		obj := &rolloutv1alpha1.Rollout{}
		err := s.fedClient.Get(ctx, client.ObjectKeyFromObject(rollout), obj)
		if err != nil {
			return false
		}
		return s.rolloutShouldBeAvailable(obj)
	}, 30*time.Second, 2*time.Second)

	// 5. Wait for RolloutRun to be created
	var run *rolloutv1alpha1.RolloutRun
	s.Require().Eventually(func() bool {
		runList := &rolloutv1alpha1.RolloutRunList{}
		err := s.fedClient.List(ctx, runList, client.InNamespace(namespace))
		if err != nil {
			return false
		}

		for i := range runList.Items {
			r := &runList.Items[i]
			owner := metav1.GetControllerOf(r)
			if owner != nil && owner.Name == rollout.Name && r.Name == "trigger-onetime-strategyref" {
				run = r
				return true
			}
		}
		return false
	}, 30*time.Second, 2*time.Second)

	// 6. Verify initial batch (from StrategyRef)
	s.Require().NotNil(run.Spec.Batch, "Batch should not be nil")
	s.Require().Len(run.Spec.Batch.Batches, 3, "Should have 3 batches from StrategyRef initially")
	s.Require().True(run.Spec.Batch.Batches[1].Breakpoint, "Second batch should have breakpoint")
	s.Require().True(run.Spec.Batch.Batches[2].Breakpoint, "Third batch should have breakpoint")
	s.Require().Len(run.Spec.Batch.Batches[0].Targets, 1, "First batch should have 1 target")
	s.Require().Equal("30%", run.Spec.Batch.Batches[0].Targets[0].Replicas.String())

	// 7. Apply one-time strategy via annotation using InlineBatch
	oneTimeStrategy := ontimestrategy.OneTimeStrategy{
		Batch: rolloutv1alpha1.BatchStrategy{
			Batches: []rolloutv1alpha1.RolloutStep{
				{
					Replicas: intstr.FromString("30%"),
				},
				{
					Breakpoint: false,
					Replicas:   intstr.FromString("60%"),
				},
				{
					Breakpoint: false,
					Replicas:   intstr.FromString("100%"),
				},
			},
		},
	}
	oneTimeData, err := json.Marshal(oneTimeStrategy)
	s.Require().NoError(err)

	// Update rollout with one-time strategy annotation
	err = s.fedClient.Get(ctx, client.ObjectKeyFromObject(rollout), rollout)
	s.Require().NoError(err)
	if rollout.Annotations == nil {
		rollout.Annotations = make(map[string]string)
	}
	rollout.Annotations[ontimestrategy.AnnoOneTimeStrategy] = string(oneTimeData)
	err = s.fedClient.Update(ctx, rollout)
	s.Require().NoError(err)

	// 8. Verify RolloutRun is updated with new batch strategy
	s.Require().Eventually(func() bool {
		err := s.fedClient.Get(ctx, client.ObjectKey{Name: run.Name, Namespace: namespace}, run)
		if err != nil {
			return false
		}
		return len(run.Spec.Batch.Batches) == 3
	}, 30*time.Second, 2*time.Second)

	defer func() {
		_ = s.fedClient.Delete(ctx, run)
	}()

	s.Require().Len(run.Spec.Batch.Batches, 3, "Should have 3 batches after one-time strategy applied")
	s.Require().False(run.Spec.Batch.Batches[1].Breakpoint, "Second batch should have no breakpoint")
	s.Require().False(run.Spec.Batch.Batches[2].Breakpoint, "Third batch should have no breakpoint")

	// 9. Verify one-time strategy annotation is on RolloutRun
	s.Require().Contains(run.Annotations, ontimestrategy.AnnoOneTimeStrategy, "RolloutRun should have one-time strategy annotation")
}

// Helper functions

func newTestRollout(name, namespace string) *rolloutv1alpha1.Rollout {
	return &rolloutv1alpha1.Rollout{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: rolloutv1alpha1.RolloutSpec{
			StrategyRef: "default-strategy",
			WorkloadRef: rolloutv1alpha1.WorkloadRef{
				APIVersion: "apps/v1",
				Kind:       "StatefulSet",
			},
		},
	}
}

func newTestRolloutStrategy(name, namespace string) *rolloutv1alpha1.RolloutStrategy {
	return &rolloutv1alpha1.RolloutStrategy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Batch: &rolloutv1alpha1.BatchStrategy{
			Batches: []rolloutv1alpha1.RolloutStep{
				{
					Breakpoint: true,
					Replicas:   intstr.FromString("25%"),
				},
				{
					Replicas: intstr.FromString("100%"),
				},
			},
		},
	}
}

func newTestStatefulSet(name, namespace string) *appsv1.StatefulSet {
	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"app": name,
			},
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: ptr.To[int32](3),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": name,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": name,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "nginx",
							Image: "nginx:1.14.2",
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: 80,
								},
							},
						},
					},
				},
			},
		},
	}
}

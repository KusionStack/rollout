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

package backendrouting

import (
	"context"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/cluster"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"kusionstack.io/kube-utils/multicluster"
	"kusionstack.io/kube-utils/multicluster/clusterinfo"
	"kusionstack.io/kube-utils/multicluster/controller"

	"kusionstack.io/rollout/apis/rollout/v1alpha1"
	"kusionstack.io/rollout/pkg/controllers/registry"
)

var (
	fedEnv       *envtest.Environment
	fedClient    client.Client
	fedClientSet *kubernetes.Clientset

	clusterEnv1    *envtest.Environment
	clusterClient1 client.Client // cluster 1 client

	clusterEnv2    *envtest.Environment
	clusterClient2 client.Client // cluster 2 client

	ctx    context.Context
	cancel context.CancelFunc

	mgr *multicluster.Manager
)

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(os.Stdout), zap.UseDevMode(true)))

	ctx, cancel = context.WithCancel(context.TODO())
	By("bootstrapping test environment")

	// fed
	testscheme := scheme.Scheme
	err := v1alpha1.AddToScheme(testscheme)
	Expect(err).NotTo(HaveOccurred())

	fedEnv = &envtest.Environment{
		Scheme:            testscheme,
		CRDDirectoryPaths: []string{filepath.Join("..", "..", "..", "config", "crd", "bases")},
	}
	fedConfig, err := fedEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(fedConfig).NotTo(BeNil())

	fedClientSet, _ = kubernetes.NewForConfig(fedConfig)

	fedClient, err = client.New(fedConfig, client.Options{Scheme: testscheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(fedClient).NotTo(BeNil())

	clusterEnv1 = &envtest.Environment{
		Scheme: testscheme,
	}
	clusterConfig1, err := clusterEnv1.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(clusterConfig1).NotTo(BeNil())

	clusterClient1, err = client.New(clusterConfig1, client.Options{Scheme: testscheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(clusterClient1).NotTo(BeNil())

	// cluster 2
	clusterEnv2 = &envtest.Environment{
		Scheme: testscheme,
	}
	clusterConfig2, err := clusterEnv2.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(clusterConfig2).NotTo(BeNil())

	clusterClient2, err = client.New(clusterConfig2, client.Options{Scheme: testscheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(clusterClient2).NotTo(BeNil())

	// manager
	var (
		newCacheFunc  cache.NewCacheFunc
		newClientFunc cluster.NewClientFunc
	)
	os.Setenv(clusterinfo.EnvClusterAllowList, "cluster1,cluster2")
	mgr, newCacheFunc, newClientFunc, err = multicluster.NewManager(&multicluster.ManagerConfig{
		ClusterProvider: &controller.TestClusterProvider{
			GroupVersionResource: schema.GroupVersionResource{
				Group:    "apps",
				Version:  "v1",
				Resource: "deployments",
			},
			ClusterNameToConfig: map[string]*rest.Config{
				"cluster1": clusterConfig1,
				"cluster2": clusterConfig2,
				"fed":      fedConfig,
			},
		},
		FedConfig:     fedConfig,
		ClusterScheme: testscheme,
		ResyncPeriod:  10 * time.Minute,
	}, multicluster.Options{})
	Expect(err).NotTo(HaveOccurred())
	Expect(mgr).NotTo(BeNil())
	Expect(newCacheFunc).NotTo(BeNil())
	Expect(newClientFunc).NotTo(BeNil())

	go func() {
		mgr.Run(2, ctx)
		Expect(err).To(BeNil())
	}()

	ctx = clusterinfo.WithCluster(ctx, clusterinfo.Fed)

	scheme := scheme.Scheme
	err = v1alpha1.AddToScheme(scheme)
	Expect(err).NotTo(HaveOccurred())

	ctrlMgr, err := manager.New(fedConfig, manager.Options{
		NewClient:              newClientFunc,
		NewCache:               newCacheFunc,
		MetricsBindAddress:     "0",
		HealthProbeBindAddress: "0",
	})
	Expect(err).NotTo(HaveOccurred())

	_, err = registry.InitWorkloadRegistry(ctrlMgr)
	Expect(err).NotTo(HaveOccurred())
	_, err = registry.InitBackendRegistry(ctrlMgr)
	Expect(err).NotTo(HaveOccurred())
	_, err = registry.InitRouteRegistry(ctrlMgr)
	Expect(err).NotTo(HaveOccurred())

	_, err = InitFunc(ctrlMgr)
	Expect(err).NotTo(HaveOccurred())

	go ctrlMgr.Start(ctx)
})

var _ = AfterSuite(func() {
	cancel()
	By("tearing down the test environment")

	err := fedEnv.Stop()
	Expect(err).NotTo(HaveOccurred())

	err = clusterEnv1.Stop()
	Expect(err).NotTo(HaveOccurred())

	err = clusterEnv2.Stop()
	Expect(err).NotTo(HaveOccurred())
})

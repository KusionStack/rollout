/**
 * Copyright 2023 KusionStack Authors.
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

package traffictopology

import (
	"context"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"kusionstack.io/kube-utils/multicluster"
	"kusionstack.io/kube-utils/multicluster/clusterinfo"
	"kusionstack.io/kube-utils/multicluster/clusterprovider"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/cluster"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"kusionstack.io/kube-api/rollout/v1alpha1"
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

	clusterClient client.Client // multi cluster client
	clusterCache  cache.Cache   // multi cluster cache

	ctx    context.Context
	cancel context.CancelFunc

	mgr *multicluster.Manager
)

var _ = BeforeSuite(func() {
	defer GinkgoRecover()
	logf.SetLogger(zap.New(zap.WriteTo(os.Stdout), zap.UseDevMode(true)))

	ctx, cancel = context.WithCancel(context.TODO())
	By("bootstrapping test environment")

	// fed
	fedScheme := runtime.NewScheme()
	err := appsv1.SchemeBuilder.AddToScheme(fedScheme) // deployment
	Expect(err).NotTo(HaveOccurred())
	err = v1alpha1.AddToScheme(fedScheme)
	Expect(err).NotTo(HaveOccurred())

	fedEnv = &envtest.Environment{
		Scheme:            fedScheme,
		CRDDirectoryPaths: []string{filepath.Join("..", "..", "..", "config", "crd", "bases")},
	}
	fedConfig, err := fedEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(fedConfig).NotTo(BeNil())

	fedClientSet, _ = kubernetes.NewForConfig(fedConfig)

	fedClient, err = client.New(fedConfig, client.Options{Scheme: fedScheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(fedClient).NotTo(BeNil())

	// cluster 1
	clusterScheme := runtime.NewScheme()
	err = corev1.SchemeBuilder.AddToScheme(clusterScheme) // configmap and service
	Expect(err).NotTo(HaveOccurred())
	err = v1alpha1.AddToScheme(clusterScheme)
	Expect(err).NotTo(HaveOccurred())
	err = appsv1.SchemeBuilder.AddToScheme(clusterScheme) // deployment
	Expect(err).NotTo(HaveOccurred())

	clusterEnv1 = &envtest.Environment{
		Scheme: clusterScheme,
	}
	clusterConfig1, err := clusterEnv1.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(clusterConfig1).NotTo(BeNil())

	clusterClient1, err = client.New(clusterConfig1, client.Options{Scheme: clusterScheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(clusterClient1).NotTo(BeNil())

	// cluster 2
	clusterEnv2 = &envtest.Environment{
		Scheme: clusterScheme,
	}
	clusterConfig2, err := clusterEnv2.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(clusterConfig2).NotTo(BeNil())

	clusterClient2, err = client.New(clusterConfig2, client.Options{Scheme: clusterScheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(clusterClient2).NotTo(BeNil())

	// manager
	var (
		newCacheFunc  cache.NewCacheFunc
		newClientFunc cluster.NewClientFunc
	)
	os.Setenv(clusterinfo.EnvClusterAllowList, "cluster1,cluster2")

	mgr, newCacheFunc, newClientFunc, err = multicluster.NewManager(&multicluster.ManagerConfig{
		ClusterProvider: clusterprovider.NewSimpleClusterProvider(map[string]*rest.Config{
			"cluster1": clusterConfig1,
			"cluster2": clusterConfig2,
			"fed":      fedConfig,
		}),
		FedConfig:     fedConfig,
		ClusterScheme: clusterScheme,
		ResyncPeriod:  10 * time.Minute,
	}, multicluster.Options{})
	Expect(err).NotTo(HaveOccurred())
	Expect(mgr).NotTo(BeNil())
	Expect(newCacheFunc).NotTo(BeNil())
	Expect(newClientFunc).NotTo(BeNil())

	// multiClusterCache
	mapper, err := apiutil.NewDynamicRESTMapper(fedConfig)
	Expect(err).NotTo(HaveOccurred())

	clusterCache, err = newCacheFunc(fedConfig, cache.Options{
		Scheme: fedScheme,
		Mapper: mapper,
	})
	Expect(err).NotTo(HaveOccurred())
	go clusterCache.Start(ctx)

	// multiClusterClient
	clusterClient, err = newClientFunc(clusterCache, fedConfig, client.Options{
		Scheme: fedScheme,
		Mapper: mapper,
	})
	Expect(err).NotTo(HaveOccurred())
	Expect(clusterClient).NotTo(BeNil())

	go mgr.Run(ctx)

	ctrlMgr, err := manager.New(fedConfig, manager.Options{
		NewClient:              newClientFunc,
		NewCache:               newCacheFunc,
		MetricsBindAddress:     "0",
		HealthProbeBindAddress: "0",
	})
	Expect(err).NotTo(HaveOccurred())

	scheme := ctrlMgr.GetScheme()
	err = appsv1.SchemeBuilder.AddToScheme(scheme)
	Expect(err).NotTo(HaveOccurred())
	err = v1alpha1.AddToScheme(scheme)
	Expect(err).NotTo(HaveOccurred())

	_, err = registry.InitWorkloadRegistry(ctrlMgr)
	Expect(err).NotTo(HaveOccurred())

	_, err = InitFunc(ctrlMgr)
	Expect(err).NotTo(HaveOccurred())

	go ctrlMgr.Start(ctx)
})

var _ = AfterSuite(func() {
	cancel()
	By("tearing down the test environment")

	if fedEnv != nil {
		err := fedEnv.Stop()
		Expect(err).NotTo(HaveOccurred())
	}
	if clusterEnv1 != nil {
		err := clusterEnv1.Stop()
		Expect(err).NotTo(HaveOccurred())
	}
	if clusterEnv2 != nil {
		err := clusterEnv2.Stop()
		Expect(err).NotTo(HaveOccurred())
	}
})

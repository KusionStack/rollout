package rollout

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/klog/v2"
	"k8s.io/klog/v2/klogr"
	rolloutv1alpha1 "kusionstack.io/kube-api/rollout/v1alpha1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"kusionstack.io/rollout/pkg/controllers/registry"
)

var (
	env *envtest.Environment
	mgr manager.Manager

	ctx    context.Context
	cancel context.CancelFunc
	c      client.Client
)

var _ = BeforeSuite(func() {
	By("bootstrapping test environment")

	ctx, cancel = context.WithCancel(context.TODO())
	logf.SetLogger(zap.New(zap.WriteTo(os.Stdout), zap.UseDevMode(true)))

	testScheme := scheme.Scheme
	err := rolloutv1alpha1.AddToScheme(testScheme)
	Expect(err).NotTo(HaveOccurred())

	env = &envtest.Environment{
		CRDDirectoryPaths:       []string{filepath.Join("..", "..", "..", "config", "crd", "bases")},
		ControlPlaneStopTimeout: 60, // 60 seconds timeout for apiserver and etcd to stop
	}

	config, err := env.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(config).NotTo(BeNil())

	klog.InitFlags(nil)
	defer klog.Flush()
	ctrl.SetLogger(klogr.New())
	mgr, err = manager.New(config, manager.Options{
		MetricsBindAddress: "0",
		Scheme:             testScheme,
		Logger:             ctrl.Log,
	})
	Expect(err).NotTo(HaveOccurred())

	c = mgr.GetClient()

	// Use empty workload registry for testing
	// Controller will handle workload lookup failures gracefully
	registry.InitWorkloadRegistry(mgr)
	Expect(NewReconciler(mgr, registry.Workloads).SetupWithManager(mgr)).NotTo(HaveOccurred())

	go func() {
		Expect(mgr.Start(ctx)).NotTo(HaveOccurred())
	}()
})

var _ = AfterSuite(func() {
	By("tearing down the test environment")

	cancel()
	// Give manager time to stop gracefully before stopping envtest
	time.Sleep(3 * time.Second)
	_ = env.Stop()
})

func TestRolloutReconciler(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Testing Rollout Suite")
}

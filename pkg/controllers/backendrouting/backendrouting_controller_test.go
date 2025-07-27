package backendrouting

import (
	"context"
	"flag"
	"os"
	"path/filepath"
	"time"

	"github.com/stretchr/testify/suite"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
	"k8s.io/klog/v2/klogr"
	"k8s.io/utils/ptr"
	rolloutapi "kusionstack.io/kube-api/rollout"
	rolloutv1alpha1 "kusionstack.io/kube-api/rollout/v1alpha1"
	clientutil "kusionstack.io/kube-utils/client"
	"kusionstack.io/kube-utils/multicluster"
	"kusionstack.io/kube-utils/multicluster/clusterinfo"
	"kusionstack.io/kube-utils/multicluster/clusterprovider"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	gatewayapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	"kusionstack.io/rollout/pkg/controllers/registry"
)

type backendRoutingTestSuite struct {
	suite.Suite

	fedClient      client.Client
	cluster1Client client.Client
	cluster2Client client.Client

	backendRouting *rolloutv1alpha1.BackendRouting
	service        *corev1.Service
	ingress        *networkingv1.Ingress
}

func (s *backendRoutingTestSuite) setupCluster(env *envtest.Environment) (*rest.Config, client.Client) {
	config, err := env.Start()
	s.Require().NoError(err)
	s.Require().NotNil(config)

	c, err := client.New(config, client.Options{Scheme: env.Scheme})
	s.Require().NoError(err)
	s.Require().NotNil(c)

	return config, c
}

func (s *backendRoutingTestSuite) SetupSuite() {
	local := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	klog.InitFlags(local)
	local.Set("v", "3")

	logf.SetLogger(klogr.New())

	ctx := context.Background()

	// fed
	testscheme := scheme.Scheme
	err := rolloutv1alpha1.AddToScheme(testscheme)
	s.Require().NoError(err)

	testClusterEnv := &envtest.Environment{
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

	fedConfig, s.fedClient = s.setupCluster(testClusterEnv)
	cluster1Config, s.cluster1Client = s.setupCluster(testClusterEnv)
	cluster2Config, s.cluster2Client = s.setupCluster(testClusterEnv)

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
		err := clusterMgr.Run(ctx)
		if err != nil {
			panic(err)
		}
	}()

	// wait for cache synced
	s.Require().True(clusterMgr.WaitForSynced(ctx))

	ctx = clusterinfo.WithCluster(ctx, clusterinfo.Fed)

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
	_, err = registry.InitBackendRegistry(mgr)
	s.Require().NoError(err)
	_, err = registry.InitRouteRegistry(mgr)
	s.Require().NoError(err)

	_, err = InitFunc(mgr)
	s.Require().NoError(err)

	go func() {
		err := mgr.Start(ctx)
		if err != nil {
			panic(err)
		}
	}()
}

func (s *backendRoutingTestSuite) SetupTest() {
	namespace := "default"
	s.service = &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-incluster-service",
			Namespace: namespace,
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Protocol:   corev1.ProtocolTCP,
					Port:       80,
					TargetPort: intstr.IntOrString{IntVal: 80},
				},
			},
		},
	}

	s.ingress = &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-incluster-ingress",
			Namespace: namespace,
		},
		Spec: networkingv1.IngressSpec{
			Rules: []networkingv1.IngressRule{
				{
					IngressRuleValue: networkingv1.IngressRuleValue{
						HTTP: &networkingv1.HTTPIngressRuleValue{
							Paths: []networkingv1.HTTPIngressPath{
								{
									Backend: networkingv1.IngressBackend{
										Service: &networkingv1.IngressServiceBackend{
											Name: s.service.Name,
											Port: networkingv1.ServiceBackendPort{
												Number: int32(80),
											},
										},
									},
									Path:     "/",
									PathType: ptr.To(networkingv1.PathTypePrefix),
								},
							},
						},
					},
				},
			},
		},
	}

	s.backendRouting = &rolloutv1alpha1.BackendRouting{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-incluster-backendrouting",
			Namespace:  namespace,
			Generation: int64(1),
		},
		Spec: rolloutv1alpha1.BackendRoutingSpec{
			TrafficType: rolloutv1alpha1.InClusterTrafficType,
			Backend: rolloutv1alpha1.CrossClusterObjectReference{
				ObjectTypeRef: rolloutv1alpha1.ObjectTypeRef{
					APIVersion: corev1.SchemeGroupVersion.String(),
					Kind:       "Service",
				},
				CrossClusterObjectNameReference: rolloutv1alpha1.CrossClusterObjectNameReference{
					Cluster: "cluster1",
					Name:    s.service.Name,
				},
			},
			Routes: []rolloutv1alpha1.CrossClusterObjectReference{
				{
					ObjectTypeRef: rolloutv1alpha1.ObjectTypeRef{
						APIVersion: networkingv1.SchemeGroupVersion.String(),
						Kind:       "Ingress",
					},
					CrossClusterObjectNameReference: rolloutv1alpha1.CrossClusterObjectNameReference{
						Cluster: "cluster1",
						Name:    s.ingress.Name,
					},
				},
			},
		},
	}
}

func (s *backendRoutingTestSuite) TearDownTest() {
	err := s.fedClient.Delete(context.Background(), s.backendRouting)
	s.Require().NoError(client.IgnoreNotFound(err))
	err = s.cluster1Client.DeleteAllOf(context.Background(), s.service, &client.DeleteAllOfOptions{ListOptions: client.ListOptions{Namespace: s.service.Namespace}})
	s.Require().NoError(err)
	err = s.cluster1Client.DeleteAllOf(context.Background(), s.ingress, &client.DeleteAllOfOptions{ListOptions: client.ListOptions{Namespace: s.ingress.Namespace}})
	s.Require().NoError(err)
}

func (s *backendRoutingTestSuite) backendRoutingShouldBeReady(obj *rolloutv1alpha1.BackendRouting) bool {
	if obj.Generation != obj.Status.ObservedGeneration {
		logf.Log.Info("generation not equal", "observedGeneration", obj.Status.ObservedGeneration, "generation", obj.Generation)
		return false
	}
	if !meta.IsStatusConditionTrue(obj.Status.Conditions, rolloutv1alpha1.BackendRoutingReady) {
		logf.Log.Info("Ready condition should be true", "conditions", obj.Status.Conditions)
		return false
	}
	return true
}

type BackendRoutingInitializationTestSuite struct {
	backendRoutingTestSuite
}

func (s *BackendRoutingInitializationTestSuite) Test_WithoutBackendAndRoute() {
	// create backendrouting
	err := s.fedClient.Create(context.Background(), s.backendRouting)
	s.Require().NoError(err)

	s.Require().Eventually(func() bool {
		obj := &rolloutv1alpha1.BackendRouting{}
		err := s.fedClient.Get(context.Background(), client.ObjectKeyFromObject(s.backendRouting), obj)
		if err != nil {
			return false
		}

		if obj.Generation != obj.Status.ObservedGeneration {
			logf.Log.Info("generation not equal", "observedGeneration", obj.Status.ObservedGeneration, "generation", obj.Generation)
			return false
		}

		if !meta.IsStatusConditionFalse(obj.Status.Conditions, rolloutv1alpha1.BackendRoutingBackendReady) {
			logf.Log.Info("BackendReady condition should be false", "condition", meta.FindStatusCondition(obj.Status.Conditions, rolloutv1alpha1.BackendRoutingBackendReady))
			return false
		}
		if !meta.IsStatusConditionFalse(obj.Status.Conditions, rolloutv1alpha1.BackendRoutingRouteReady) {
			logf.Log.Info("RouteReady condition should be false", "condition", meta.FindStatusCondition(obj.Status.Conditions, rolloutv1alpha1.BackendRoutingRouteReady))
			return false
		}
		if !meta.IsStatusConditionFalse(obj.Status.Conditions, rolloutv1alpha1.BackendRoutingReady) {
			logf.Log.Info("Ready condition should be false", "condition", meta.FindStatusCondition(obj.Status.Conditions, rolloutv1alpha1.BackendRoutingReady))
			return false
		}
		return true
	}, 10*time.Second, 5*time.Second)
}

func (s *BackendRoutingInitializationTestSuite) Test_CreateWtihoutRoute() {
	// create backendrouting
	err := s.fedClient.Create(context.Background(), s.backendRouting)
	s.Require().NoError(err)
	// create origin service
	err = s.cluster1Client.Create(context.Background(), s.service)
	s.Require().NoError(err)

	s.Require().Eventually(func() bool {
		svc := &corev1.Service{}
		err := s.cluster1Client.Get(context.Background(), client.ObjectKeyFromObject(s.service), svc)
		if err != nil {
			logf.Log.Info("svc not found")
			return false
		}

		obj := &rolloutv1alpha1.BackendRouting{}
		err = s.fedClient.Get(context.Background(), client.ObjectKeyFromObject(s.backendRouting), obj)
		if err != nil {
			return false
		}

		if obj.Generation != obj.Status.ObservedGeneration {
			logf.Log.Info("generation not equal", "observedGeneration", obj.Status.ObservedGeneration, "generation", obj.Generation)
			return false
		}

		if !meta.IsStatusConditionTrue(obj.Status.Conditions, rolloutv1alpha1.BackendRoutingBackendReady) {
			logf.Log.Info("BackendReady condition should be true", "condition", meta.FindStatusCondition(obj.Status.Conditions, rolloutv1alpha1.BackendRoutingBackendReady))
			return false
		}
		if !meta.IsStatusConditionFalse(obj.Status.Conditions, rolloutv1alpha1.BackendRoutingRouteReady) {
			logf.Log.Info("RouteReady condition should be false", "condition", meta.FindStatusCondition(obj.Status.Conditions, rolloutv1alpha1.BackendRoutingRouteReady))
			return false
		}
		if !meta.IsStatusConditionFalse(obj.Status.Conditions, rolloutv1alpha1.BackendRoutingReady) {
			logf.Log.Info("Ready condition should be false", "condition", meta.FindStatusCondition(obj.Status.Conditions, rolloutv1alpha1.BackendRoutingReady))
			return false
		}
		return true
	}, 60*time.Second, 5*time.Second)
}

func (s *BackendRoutingInitializationTestSuite) Test_Create() {
	// create backendrouting
	err := s.fedClient.Create(context.Background(), s.backendRouting)
	s.Require().NoError(err)
	// create service
	err = s.cluster1Client.Create(context.Background(), s.service)
	s.Require().NoError(err)
	// create ingress
	err = s.cluster1Client.Create(context.Background(), s.ingress)
	s.Require().NoError(err)

	s.Require().Eventually(func() bool {
		svc := &corev1.Service{}
		err := s.cluster1Client.Get(context.Background(), client.ObjectKeyFromObject(s.service), svc)
		if err != nil {
			logf.Log.Info("svc not found")
			return false
		}
		ingress := &networkingv1.Ingress{}
		err = s.cluster1Client.Get(context.Background(), client.ObjectKeyFromObject(s.ingress), ingress)
		if err != nil {
			logf.Log.Info("ingress not found")
			return false
		}
		obj := &rolloutv1alpha1.BackendRouting{}
		err = s.fedClient.Get(context.Background(), client.ObjectKeyFromObject(s.backendRouting), obj)
		if err != nil {
			return false
		}

		return s.backendRoutingShouldBeReady(obj)
	}, 60*time.Second, 5*time.Second)
}

type BackendRoutingControllerTestSuite struct {
	backendRoutingTestSuite
}

func (s *BackendRoutingControllerTestSuite) SetupTest() {
	s.backendRoutingTestSuite.SetupTest()
	// create service
	err := s.cluster1Client.Create(context.Background(), s.service)
	s.Require().NoError(err)
	// create ingress
	err = s.cluster1Client.Create(context.Background(), s.ingress)
	s.Require().NoError(err)
	// create backendrouting
	err = s.fedClient.Create(context.Background(), s.backendRouting)
	s.Require().NoError(err)
}

func (s *BackendRoutingControllerTestSuite) Test_ForBackends() {
	stableSvcName := s.backendRouting.Spec.Backend.Name + "-stable"
	canarySvcName := s.backendRouting.Spec.Backend.Name + "-canary"
	_, err := clientutil.UpdateOnConflict(context.Background(), s.fedClient, s.fedClient, s.backendRouting, func(in *rolloutv1alpha1.BackendRouting) error {
		// add fored backends
		in.Spec.ForkedBackends = &rolloutv1alpha1.ForkedBackends{
			Stable: rolloutv1alpha1.ForkedBackend{
				Name: stableSvcName,
			},
			Canary: rolloutv1alpha1.ForkedBackend{
				Name: canarySvcName,
			},
		}
		return nil
	})
	s.Require().NoError(err)

	var (
		stableSVC = &corev1.Service{}
		canarySVC = &corev1.Service{}
	)

	s.Require().Eventually(func() bool {
		err = s.cluster1Client.Get(context.Background(), client.ObjectKey{Namespace: s.backendRouting.Namespace, Name: stableSvcName}, stableSVC)
		if err != nil {
			logf.Log.Info("stable svc not found")
			return false
		}
		err = s.cluster1Client.Get(context.Background(), client.ObjectKey{Namespace: s.backendRouting.Namespace, Name: canarySvcName}, canarySVC)
		if err != nil {
			logf.Log.Info("canary svc not found")
			return false
		}

		s.Require().Equal(rolloutapi.StableTrafficLane, stableSVC.Spec.Selector[rolloutapi.TrafficLaneLabelKey])
		s.Require().Contains(stableSVC.Labels, rolloutapi.CanaryResourceLabelKey)
		s.Require().Equal(rolloutapi.CanaryTrafficLane, canarySVC.Spec.Selector[rolloutapi.TrafficLaneLabelKey])
		s.Require().Contains(canarySVC.Labels, rolloutapi.CanaryResourceLabelKey)

		return true
	}, 60*time.Second, 5*time.Second, "stable and canary should be created")

	s.Require().Eventually(func() bool {
		obj := &rolloutv1alpha1.BackendRouting{}
		err := s.fedClient.Get(context.Background(), client.ObjectKeyFromObject(s.backendRouting), obj)
		if err != nil {
			return false
		}

		ready := s.backendRoutingShouldBeReady(obj)
		if !ready {
			return false
		}

		s.Require().NotNil(obj.Status.Backends.Canary.Conditions.Ready, "canary backend should be ready")
		s.Require().True(*obj.Status.Backends.Canary.Conditions.Ready, "canary backend should be ready")
		s.Require().NotNil(obj.Status.Backends.Stable.Conditions.Ready, "stable backend should be ready")
		s.Require().True(*obj.Status.Backends.Stable.Conditions.Ready, "stable backend should be ready")

		return true
	}, 60*time.Second, 5*time.Second, "backend routing should be ready")

	// delete fored backends
	_, err = clientutil.UpdateOnConflict(context.Background(), s.fedClient, s.fedClient, s.backendRouting, func(in *rolloutv1alpha1.BackendRouting) error {
		// add fored backends
		in.Spec.ForkedBackends = nil
		return nil
	})
	s.Require().NoError(err)

	s.Require().Eventually(func() bool {
		err = s.cluster1Client.Get(context.Background(), client.ObjectKey{Namespace: s.backendRouting.Namespace, Name: stableSvcName}, stableSVC)
		if err == nil || !errors.IsNotFound(err) {
			return false
		}
		err = s.cluster1Client.Get(context.Background(), client.ObjectKey{Namespace: s.backendRouting.Namespace, Name: canarySvcName}, canarySVC)
		if err == nil || !errors.IsNotFound(err) {
			return false
		}
		return true
	}, 60*time.Second, 5*time.Second, "stable and canary should be deleted")

	s.Require().Eventually(func() bool {
		obj := &rolloutv1alpha1.BackendRouting{}
		err := s.fedClient.Get(context.Background(), client.ObjectKeyFromObject(s.backendRouting), obj)
		if err != nil {
			return false
		}

		ready := s.backendRoutingShouldBeReady(obj)
		if !ready {
			return false
		}

		s.Require().Nil(obj.Status.Backends.Canary.Conditions.Ready, "canary backend should be deleted")
		s.Require().Nil(obj.Status.Backends.Stable.Conditions.Ready, "stable backend should be deleted")

		return true
	}, 60*time.Second, 5*time.Second)
}

func (s *BackendRoutingControllerTestSuite) Test_Route() {
	stableSvcName := s.backendRouting.Spec.Backend.Name + "-stable"
	canarySvcName := s.backendRouting.Spec.Backend.Name + "-canary"

	s.changeBackendRouting(func(in *rolloutv1alpha1.BackendRouting) error {
		// add fored backends
		in.Spec.ForkedBackends = &rolloutv1alpha1.ForkedBackends{
			Stable: rolloutv1alpha1.ForkedBackend{
				Name: stableSvcName,
			},
			Canary: rolloutv1alpha1.ForkedBackend{
				Name: canarySvcName,
			},
		}
		return nil
	})

	var (
		stableSVC = &corev1.Service{}
		canarySVC = &corev1.Service{}
	)

	// wating for svc created
	s.Require().Eventually(func() bool {
		err := s.cluster1Client.Get(context.Background(), client.ObjectKey{Namespace: s.backendRouting.Namespace, Name: stableSvcName}, stableSVC)
		if err != nil {
			return false
		}
		err = s.cluster1Client.Get(context.Background(), client.ObjectKey{Namespace: s.backendRouting.Namespace, Name: canarySvcName}, canarySVC)
		if err != nil {
			return false
		}
		return true
	}, 60*time.Second, 5*time.Second, "stable and canary should be created")

	// change original backend
	s.changeBackendRouting(func(in *rolloutv1alpha1.BackendRouting) error {
		in.Spec.Forwarding = &rolloutv1alpha1.BackendForwarding{
			HTTP: &rolloutv1alpha1.HTTPForwarding{
				Origin: &rolloutv1alpha1.OriginHTTPForwarding{
					BackendName: stableSvcName,
				},
			},
		}
		return nil
	})

	// waiting for ingress changed
	s.Require().Eventually(func() bool {
		ingress := &networkingv1.Ingress{}
		err := s.cluster1Client.Get(context.Background(), client.ObjectKeyFromObject(s.ingress), ingress)
		if err != nil {
			return false
		}
		s.Require().Equal(stableSvcName, ingress.Spec.Rules[0].HTTP.Paths[0].Backend.Service.Name)
		return true
	}, 60*time.Second, 5*time.Second, "ingress should be ready")

	// backendrouding should be ready
	s.checkBackendRoutingReady()

	// add canary route
	s.changeBackendRouting(func(in *rolloutv1alpha1.BackendRouting) error {
		// add fored backends
		if in.Spec.Forwarding.HTTP.Canary == nil {
			in.Spec.Forwarding.HTTP.Canary = &rolloutv1alpha1.CanaryHTTPForwarding{
				BackendName: canarySvcName,
			}
		}
		in.Spec.Forwarding.HTTP.Canary.Weight = ptr.To[int32](50)
		in.Spec.Forwarding.HTTP.Canary.Filters = []gatewayapiv1.HTTPRouteFilter{
			{
				Type: gatewayapiv1.HTTPRouteFilterRequestHeaderModifier,
				RequestHeaderModifier: &gatewayapiv1.HTTPHeaderFilter{
					Set: []gatewayapiv1.HTTPHeader{
						{
							Name:  "x-mse-tag",
							Value: "canary",
						},
					},
				},
			},
		}
		return nil
	})

	// waiting for canary ingress created
	s.Require().Eventually(func() bool {
		ingress := &networkingv1.Ingress{}
		key := client.ObjectKeyFromObject(s.ingress)
		key.Name = s.ingress.Name + "-canary"
		err := s.cluster1Client.Get(context.Background(), key, ingress)
		if err != nil {
			return false
		}
		s.Require().Contains(ingress.Labels, rolloutapi.CanaryResourceLabelKey)
		return true
	}, 60*time.Second, 5*time.Second, "canary ingress should be ready")

	// backendrouding should be ready
	s.checkBackendRoutingReady()

	// delete canary route
	s.changeBackendRouting(func(in *rolloutv1alpha1.BackendRouting) error {
		in.Spec.Forwarding.HTTP.Canary = nil
		return nil
	})

	// backendrouding should be ready
	s.checkBackendRoutingReady()

	// waiting for canary ingress deleted
	s.Require().Eventually(func() bool {
		ingress := &networkingv1.Ingress{}
		key := client.ObjectKeyFromObject(s.ingress)
		key.Name = s.ingress.Name + "-canary"
		err := s.cluster1Client.Get(context.Background(), key, ingress)
		if err == nil {
			return false
		}

		if !errors.IsNotFound(err) {
			s.Require().NoError(err)
			return false
		}
		return true
	}, 60*time.Second, 5*time.Second, "canary ingress should be deleted")

	// revert original backend
	s.changeBackendRouting(func(in *rolloutv1alpha1.BackendRouting) error {
		in.Spec.Forwarding = nil
		return nil
	})

	s.checkBackendRoutingReady()

	s.Require().Eventually(func() bool {
		ingress := &networkingv1.Ingress{}
		err := s.cluster1Client.Get(context.Background(), client.ObjectKeyFromObject(s.ingress), ingress)
		if err != nil {
			return false
		}
		s.Require().Equal(s.backendRouting.Spec.Backend.Name, ingress.Spec.Rules[0].HTTP.Paths[0].Backend.Service.Name)
		return true
	}, 60*time.Second, 5*time.Second, "ingress should be revert")

	// delete fored backends
	s.changeBackendRouting(func(in *rolloutv1alpha1.BackendRouting) error {
		// add fored backends
		in.Spec.ForkedBackends = nil
		return nil
	})

	s.Require().Eventually(func() bool {
		err := s.cluster1Client.Get(context.Background(), client.ObjectKey{Namespace: s.backendRouting.Namespace, Name: stableSvcName}, stableSVC)
		if err == nil || !errors.IsNotFound(err) {
			return false
		}
		err = s.cluster1Client.Get(context.Background(), client.ObjectKey{Namespace: s.backendRouting.Namespace, Name: canarySvcName}, canarySVC)
		if err == nil || !errors.IsNotFound(err) {
			return false
		}
		return true
	}, 60*time.Second, 5*time.Second, "stable and canary should be deleted")

	s.checkBackendRoutingReady()
}

func (s *BackendRoutingControllerTestSuite) changeBackendRouting(fn func(in *rolloutv1alpha1.BackendRouting) error) {
	_, err := clientutil.UpdateOnConflict(context.Background(), s.fedClient, s.fedClient, s.backendRouting, fn)
	s.Require().NoError(err)
}

func (s *BackendRoutingControllerTestSuite) checkBackendRoutingReady() {
	s.Require().Eventually(func() bool {
		err := s.fedClient.Get(context.Background(), client.ObjectKeyFromObject(s.backendRouting), s.backendRouting)
		if err != nil {
			return false
		}

		ready := s.backendRoutingShouldBeReady(s.backendRouting)
		if !ready {
			return false
		}

		return true
	}, 60*time.Second, 5*time.Second)
}

package registry

import (
	"context"
	"fmt"
	"reflect"
	"sync"

	"github.com/elliotchance/pie/v2"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/conversion"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"kusionstack.io/kube-utils/multicluster"
	"kusionstack.io/kube-utils/multicluster/clusterinfo"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	rolloutapi "kusionstack.io/rollout/apis/rollout"
	rolloutv1alpha1 "kusionstack.io/rollout/apis/rollout/v1alpha1"
	"kusionstack.io/rollout/pkg/utils"
	"kusionstack.io/rollout/pkg/workload"
)

// Standard workload store interface
type Store interface {
	GroupVersionKind() schema.GroupVersionKind
	// NewObject returns a new instance of the workload type
	NewObject() client.Object
	// NewObjectList returns a new instance of the workload list type
	NewObjectList() client.ObjectList
	// Watchable indicates whether this workload type can be watched from the API server.
	Watchable() bool
	// Wrap get a client.Object and returns a workload interface
	Wrap(cluster string, obj client.Object) (workload.Interface, error)
	// Get returns a wrapped workload interface
	Get(ctx context.Context, cluster, namespace, name string) (workload.Interface, error)
	// GetWorkloadList finds workload by match
	List(ctx context.Context, namespace string, match rolloutv1alpha1.ResourceMatch) ([]workload.Interface, error)
}

type Registry interface {
	// SetupWithManger initialize registry with manager.
	SetupWithManger(mgr manager.Manager)
	// Register add a new workload store for given gvk
	Register(store Store)
	// Delete delete a new workload store for given gvk
	Delete(gvk schema.GroupVersionKind)
	// If the gvk is registered and supported by all member clusters, Get returns the workload rest store.
	Get(gvk schema.GroupVersionKind) (Store, error)
	// WatchableStores return all watchable and supported stores.
	WatchableStores() []Store
}

type workloadRegistry struct {
	workloads        sync.Map // map[schema.GroupVersionKind]Store
	logger           logr.Logger
	discoveryFed     discovery.DiscoveryInterface
	discoveryMembers multicluster.PartialCachedDiscoveryInterface
}

var _ Registry = &workloadRegistry{}

func New() Registry {
	m := &workloadRegistry{
		workloads: sync.Map{},
	}
	return m
}

// SetupWithManger implements Registry.
func (r *workloadRegistry) SetupWithManger(mgr manager.Manager) {
	r.logger = mgr.GetLogger().WithName("workloadRegistry")
	var fedDiscovery discovery.DiscoveryInterface
	var membersDiscovery multicluster.PartialCachedDiscoveryInterface
	c := mgr.GetClient()
	client, ok := c.(multicluster.MultiClusterDiscovery)
	if ok {
		fedDiscovery = client.FedDiscoveryInterface()
		membersDiscovery = client.MembersCachedDiscoveryInterface()
	} else {
		discoveryClient := memory.NewMemCacheClient(discovery.NewDiscoveryClientForConfigOrDie(mgr.GetConfig()))
		fedDiscovery = discoveryClient
		membersDiscovery = discoveryClient
	}
	r.discoveryFed = fedDiscovery
	r.discoveryMembers = membersDiscovery
}

// Get implements Registry.
func (m *workloadRegistry) Get(gvk schema.GroupVersionKind) (Store, error) {
	value, ok := m.workloads.Load(gvk)
	if !ok {
		return nil, fmt.Errorf("unregistered gvk(%s) in workload registry", gvk.String())
	}

	return value.(Store), nil
}

func (m *workloadRegistry) Register(store Store) {
	m.workloads.Store(store.GroupVersionKind(), store)
}

func (m *workloadRegistry) Delete(gvk schema.GroupVersionKind) {
	m.workloads.Delete(gvk)
}

func (m *workloadRegistry) WatchableStores() []Store {
	result := make([]Store, 0)

	m.workloads.Range(func(key, value any) bool {
		store := value.(Store)
		gvk := store.GroupVersionKind()

		if !store.Watchable() {
			// skip it
			m.logger.Info("gvk store says it is not watchable, skip it", "gvk", gvk.String())
			return true
		}

		supported, err := m.isGVKSupportedInMembers(gvk)
		if err != nil {
			m.logger.Error(err, "failed to get discovery result from member clusters, skip it", "gvk", gvk.String())
			return true
		}
		if !supported {
			m.logger.Info("gvk is not supported in all members clusters, skip it", "gvk", gvk.String())
			return true
		}

		result = append(result, store)

		return true
	})

	return result
}

func (m *workloadRegistry) isGVKSupportedInMembers(gvk schema.GroupVersionKind) (bool, error) {
	if m.discoveryMembers == nil {
		return false, fmt.Errorf("member clusters discovery interface is not set, please use SetupWithManager() firstly")
	}

	_, resources, err := m.discoveryMembers.ServerGroupsAndResources()
	if err != nil {
		return false, err
	}

	for _, resourceList := range resources {
		if resourceList.GroupVersion != gvk.GroupVersion().String() {
			continue
		}
		found := pie.Any[metav1.APIResource](resourceList.APIResources, func(value metav1.APIResource) bool {
			return value.Kind == gvk.Kind
		})
		if found {
			return true, nil
		}
	}
	return false, nil
}

// GetWorkloadList implements WorkloadManager.
func GetWorkloadList(ctx context.Context, c client.Client, store Store, namespace string, match rolloutv1alpha1.ResourceMatch) ([]workload.Interface, error) {
	listObj := store.NewObjectList()
	if err := c.List(clusterinfo.WithCluster(ctx, clusterinfo.Clusters), listObj, &client.ListOptions{Namespace: namespace}); err != nil {
		return nil, err
	}

	matcher := workload.MatchAsMatcher(match)

	listPtr, err := meta.GetItemsPtr(listObj)
	if err != nil {
		return nil, err
	}

	v, err := conversion.EnforcePtr(listPtr)
	if err != nil || v.Kind() != reflect.Slice {
		return nil, fmt.Errorf("neet ptr to slice: %v", err)
	}

	length := v.Len()
	workloads := make([]workload.Interface, 0)

	for i := 0; i < length; i++ {
		elemPtr := v.Index(i).Addr().Interface()
		obj, ok := elemPtr.(client.Object)
		if !ok {
			return nil, fmt.Errorf("can not convert element to client.Object")
		}

		canary := utils.GetMapValueByDefault(obj.GetLabels(), rolloutapi.LabelCanary, "false")
		if canary == "true" {
			// ignore canary workload here, you should get canary worload from CanaryStrategy interface
			continue
		}

		cluster := workload.GetClusterFromLabel(obj.GetLabels())
		if !matcher.Matches(cluster, obj.GetName(), obj.GetLabels()) {
			continue
		}
		workload, err := store.Wrap(cluster, obj)
		if err != nil {
			return nil, err
		}
		workloads = append(workloads, workload)
	}
	return workloads, nil
}

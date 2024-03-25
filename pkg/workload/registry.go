package workload

import (
	"fmt"
	"sync"

	"github.com/elliotchance/pie/v2"
	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"kusionstack.io/kube-utils/multicluster"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

type Registry interface {
	// SetupWithManger initialize registry with manager.
	SetupWithManger(mgr manager.Manager)
	// Register add a new workload Accessor for given gvk
	Register(accessor Accessor)
	// Delete delete a new workload Accessor for given gvk
	Delete(gvk schema.GroupVersionKind)
	// If the gvk is registered and supported by all member clusters, Get returns the workload Accessor.
	Get(gvk schema.GroupVersionKind) (Accessor, error)
	// Watchables return all watchable and supported Accessor.
	Watchables() []Accessor
}

var _ Registry = &registryImpl{}

type registryImpl struct {
	accessor         sync.Map // map[schema.GroupVersionKind]workload.Accessor
	logger           logr.Logger
	discoveryFed     discovery.DiscoveryInterface
	discoveryMembers multicluster.PartialCachedDiscoveryInterface
}

func NewRegistry() Registry {
	m := &registryImpl{
		accessor: sync.Map{},
	}
	return m
}

// SetupWithManger implements Registry.
func (r *registryImpl) SetupWithManger(mgr manager.Manager) {
	r.logger = mgr.GetLogger().WithName("registry").WithName("workload")
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

func (m *registryImpl) Get(gvk schema.GroupVersionKind) (Accessor, error) {
	value, ok := m.accessor.Load(gvk)
	if !ok {
		return nil, fmt.Errorf("unregistered gvk(%s) in workload registry", gvk.String())
	}

	return value.(Accessor), nil
}

func (m *registryImpl) Register(accessor Accessor) {
	m.accessor.Store(accessor.GroupVersionKind(), accessor)
}

func (m *registryImpl) Delete(gvk schema.GroupVersionKind) {
	m.accessor.Delete(gvk)
}

func (m *registryImpl) Watchables() []Accessor {
	result := make([]Accessor, 0)

	m.accessor.Range(func(key, value any) bool {
		inter := value.(Accessor)
		gvk := inter.GroupVersionKind()

		if !inter.Watchable() {
			// skip it
			m.logger.Info("workload interface does not support watch, skip it", "gvk", gvk.String())
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

		result = append(result, inter)

		return true
	})

	return result
}

func (m *registryImpl) isGVKSupportedInMembers(gvk schema.GroupVersionKind) (bool, error) {
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
		found := pie.Any(resourceList.APIResources, func(value metav1.APIResource) bool {
			return value.Kind == gvk.Kind
		})
		if found {
			return true, nil
		}
	}
	return false, nil
}

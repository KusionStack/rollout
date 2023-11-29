package registry

import (
	"context"
	"fmt"
	"reflect"
	"sync"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/conversion"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/util/retry"
	"kusionstack.io/kube-utils/multicluster"
	"kusionstack.io/kube-utils/multicluster/clusterinfo"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	rolloutv1alpha1 "kusionstack.io/rollout/apis/rollout/v1alpha1"
	"kusionstack.io/rollout/pkg/workload"
)

// Standard workload store interface
type Store interface {
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
	// Register add a new workload store for given gvk
	Register(gvk schema.GroupVersionKind, store Store)
	// If the gvk is registered, Get returns the workload rest store.
	Get(gvk schema.GroupVersionKind) (Store, error)
	// AddWatcher lets controller watch all registered workloads' types
	AddWatcher(mgr manager.Manager, c controller.Controller) error
}

type workloadRegistry struct {
	workloads sync.Map // map[schema.GroupVersionKind]Store
}

var _ Registry = &workloadRegistry{}

func New() Registry {
	m := &workloadRegistry{
		workloads: sync.Map{},
	}
	return m
}

// Get implements Registry.
func (m *workloadRegistry) Get(gvk schema.GroupVersionKind) (Store, error) {
	value, ok := m.workloads.Load(gvk)
	if !ok {
		return nil, fmt.Errorf("unregistered gvk in workload registry %s", gvk.String())
	}

	return value.(Store), nil
}

func (m *workloadRegistry) Register(gvk schema.GroupVersionKind, store Store) {
	m.workloads.Store(gvk, store)
}

func (m *workloadRegistry) AddWatcher(mgr manager.Manager, c controller.Controller) error {
	var retErr error

	discoveryClient := discovery.NewDiscoveryClientForConfigOrDie(mgr.GetConfig())
	client := mgr.GetClient()
	scheme := mgr.GetScheme()
	logger := c.GetLogger()

	m.workloads.Range(func(key, value any) bool {
		gvk := key.(schema.GroupVersionKind)
		store := value.(Store)

		if !store.Watchable() {
			// skip it
			return true
		}

		if err := m.discoveryGVK(discoveryClient, gvk); err != nil {
			logger.Error(err, "failed to discovery resource in cluster", "group", gvk.Group, "version", gvk.Version, "kind", gvk.Kind)
			retErr = err
			return false
		}

		err := c.Watch(
			multicluster.ClustersKind(&source.Kind{Type: store.NewObject()}),
			enqueueRolloutForWorkloadHandler(client, scheme, logger),
		)
		if err != nil {
			retErr = err
			return false
		}

		return true
	})

	if retErr != nil {
		return retErr
	}

	return nil
}

func (m *workloadRegistry) discoveryGVK(discoveryClient discovery.DiscoveryInterface, gvk schema.GroupVersionKind) error {
	err := retry.OnError(
		retry.DefaultRetry,
		func(err error) bool {
			return !errors.IsNotFound(err)
		},
		func() error {
			resources, err := discoveryClient.ServerResourcesForGroupVersion(gvk.GroupVersion().String())
			if err != nil {
				return err
			}
			found := false
			for _, r := range resources.APIResources {
				if gvk.Kind == r.Kind {
					found = true
				}
			}
			if !found {
				return errors.NewNotFound(schema.GroupResource{Group: gvk.Group, Resource: gvk.Kind}, "")
			}
			return nil
		},
	)

	if err != nil {
		return err
	}

	return nil
}

func enqueueRolloutForWorkloadHandler(reader client.Reader, scheme *runtime.Scheme, logger logr.Logger) handler.EventHandler {
	mapFunc := func(obj client.Object) []reconcile.Request {
		key := types.NamespacedName{
			Namespace: obj.GetNamespace(),
			Name:      obj.GetName(),
		}
		kinds, _, err := scheme.ObjectKinds(obj)
		if err != nil {
			logger.Error(err, "failed to get ObjectKind from object", "key", key.String())
			return nil
		}
		gvk := kinds[0]

		cluster := workload.GetClusterFromLabel(obj.GetLabels())
		info := workload.NewInfo(cluster, gvk, obj)
		rollout, err := getRolloutForWorkload(reader, logger, info)
		if err != nil {
			logger.Error(err, "failed to get rollout for workload", "key", key.String(), "gvk", gvk.String())
			return nil
		}
		if rollout == nil {
			// TODO: if workload has rollout label but no rollout matches it, we need to clean the label
			// no matched
			// logger.V(5).Info("no matched rollout found for workload", "workload", key.String(), "gvk", gvk.String())
			return nil
		}

		logger.V(1).Info("get matched rollout for workload", "workload", key.String(), "rollout", rollout.Name)
		req := types.NamespacedName{Namespace: rollout.GetNamespace(), Name: rollout.GetName()}
		return []reconcile.Request{{NamespacedName: req}}
	}

	return handler.EnqueueRequestsFromMapFunc(mapFunc)
}

func getRolloutForWorkload(
	reader client.Reader,
	logger logr.Logger,
	workloadInfo workload.Info,
) (*rolloutv1alpha1.Rollout, error) {
	rList := &rolloutv1alpha1.RolloutList{}
	ctx := clusterinfo.WithCluster(context.TODO(), clusterinfo.Fed)
	if err := reader.List(ctx, rList, client.InNamespace(workloadInfo.Namespace)); err != nil {
		logger.Error(err, "failed to list rollouts")
		return nil, err
	}

	for i := range rList.Items {
		rollout := rList.Items[i]
		workloadRef := rollout.Spec.WorkloadRef
		refGV, err := schema.ParseGroupVersion(workloadRef.APIVersion)
		if err != nil {
			logger.Error(err, "failed to parse rollout workload ref group version", "rollout", rollout.Name, "apiVersion", workloadRef.APIVersion)
			continue
		}
		refGVK := refGV.WithKind(workloadRef.Kind)

		if !reflect.DeepEqual(refGVK, workloadInfo.GVK) {
			// group version kind not match
			// logger.Info("gvk not match", "gvk", workloadInfo.GVK.String(), "refGVK", refGVK)
			continue
		}

		macher := workload.MatchAsMatcher(workloadRef.Match)
		if macher.Matches(workloadInfo.Cluster, workloadInfo.Name, workloadInfo.Labels) {
			return &rollout, nil
		}
	}

	// not found
	return nil, nil
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

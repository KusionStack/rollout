package backendrouting

import (
	"sync"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	rolloutv1alpha1 "kusionstack.io/kube-api/rollout/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"kusionstack.io/rollout/pkg/trafficrouting/route"
)

type syncContext struct {
	once          sync.Once
	Object        *rolloutv1alpha1.BackendRouting
	BackendObject client.Object
	NewStatus     *rolloutv1alpha1.BackendRoutingStatus
	Routes        []route.RouteController
}

func (c *syncContext) SetDefaults() {
	c.once.Do(func() {
		if c.NewStatus == nil {
			c.NewStatus = c.Object.Status.DeepCopy()
		}

		newStatus := c.NewStatus
		newStatus.ObservedGeneration = c.Object.Generation
		if newStatus.Conditions == nil {
			newStatus.Conditions = make([]metav1.Condition, 0)
		}

		newStatus.Backends.Origin.Name = c.Object.Spec.Backend.Name

		// set defaults
		if newStatus.Routes == nil {
			newStatus.Routes = make([]rolloutv1alpha1.BackendRouteStatus, len(c.Object.Spec.Routes))
			for i, routeSpec := range c.Object.Spec.Routes {
				newStatus.Routes[i] = rolloutv1alpha1.BackendRouteStatus{
					CrossClusterObjectReference: routeSpec,
					Condition: &metav1.Condition{
						Type:               rolloutv1alpha1.BackendRoutingRouteReady,
						Status:             metav1.ConditionUnknown,
						LastTransitionTime: metav1.Now(),
						Reason:             "Unknown",
					},
				}
			}
		}

		if c.Object.Spec.Forwarding != nil && c.Object.Spec.Forwarding.HTTP != nil {
			for i, routeStatus := range newStatus.Routes {
				if routeStatus.Forwarding == nil {
					newStatus.Routes[i].Forwarding = &rolloutv1alpha1.BackendRouteForwardingStatuses{}
				}

				if c.Object.Spec.Forwarding.HTTP.Origin != nil && newStatus.Routes[i].Forwarding.Origin == nil {
					newStatus.Routes[i].Forwarding.Origin = &rolloutv1alpha1.BackendRouteForwardingStatus{
						BackendName: c.Object.Spec.Forwarding.HTTP.Origin.BackendName,
						Conditions: rolloutv1alpha1.BackendConditions{
							Ready: ptr.To(false),
						},
					}
					c.setRouteCondition(i, metav1.ConditionFalse, "WaitForSync", "spec updated, waiting for sync")
				}

				if c.Object.Spec.Forwarding.HTTP.Stable != nil && newStatus.Routes[i].Forwarding.Stable == nil {
					newStatus.Routes[i].Forwarding.Stable = &rolloutv1alpha1.BackendRouteForwardingStatus{
						BackendName: c.Object.Spec.Forwarding.HTTP.Stable.BackendName,
						Conditions: rolloutv1alpha1.BackendConditions{
							Ready: ptr.To(false),
						},
					}
					c.setRouteCondition(i, metav1.ConditionFalse, "WaitForSync", "spec updated, waiting for sync")
				}

				if c.Object.Spec.Forwarding.HTTP.Canary != nil && newStatus.Routes[i].Forwarding.Canary == nil {
					newStatus.Routes[i].Forwarding.Canary = &rolloutv1alpha1.BackendRouteForwardingStatus{
						BackendName: c.Object.Spec.Forwarding.HTTP.Canary.BackendName,
						Conditions: rolloutv1alpha1.BackendConditions{
							Ready: ptr.To(false),
						},
					}
					c.setRouteCondition(i, metav1.ConditionFalse, "WaitForSync", "spec updated, waiting for sync")
				}
			}
		}

		if c.Object.Spec.ForkedBackends != nil {
			newStatus.Backends.Canary.Name = c.Object.Spec.ForkedBackends.Canary.Name
			if newStatus.Backends.Canary.Conditions.Ready == nil {
				newStatus.Backends.Canary.Conditions.Ready = ptr.To(false)
			}
			newStatus.Backends.Stable.Name = c.Object.Spec.ForkedBackends.Stable.Name
			if newStatus.Backends.Stable.Conditions.Ready == nil {
				newStatus.Backends.Stable.Conditions.Ready = ptr.To(false)
			}
		}
	})
}

func (c *syncContext) checkOriginRoute(forwardingStatus *rolloutv1alpha1.BackendRouteForwardingStatuses) (created, deleted bool) {
	forwardingSpec := c.Object.Spec.Forwarding

	// origin is not ready and not in terminating
	if forwardingSpec != nil && forwardingSpec.HTTP != nil && forwardingSpec.HTTP.Origin != nil &&
		forwardingStatus.Origin != nil && !ptr.Deref(forwardingStatus.Origin.Conditions.Ready, false) &&
		!ptr.Deref(forwardingStatus.Origin.Conditions.Terminating, false) {
		created = true
	}

	// forwardingSpec.Origin is nil and forwardingStatus.Origin is not nil
	if (forwardingSpec == nil || forwardingSpec.HTTP == nil || forwardingSpec.HTTP.Origin == nil) &&
		forwardingStatus != nil && forwardingStatus.Origin != nil {
		deleted = true
	}

	return created, deleted
}

func (c *syncContext) checkCanaryRoute(forwardingStatus *rolloutv1alpha1.BackendRouteForwardingStatuses) (created, deleted bool) {
	forwardingSpec := c.Object.Spec.Forwarding

	if forwardingSpec != nil && forwardingSpec.HTTP != nil && forwardingSpec.HTTP.Canary != nil &&
		forwardingStatus.Canary != nil && !ptr.Deref(forwardingStatus.Canary.Conditions.Ready, false) &&
		!ptr.Deref(forwardingStatus.Canary.Conditions.Terminating, false) {
		created = true
	}

	if (forwardingSpec == nil || forwardingSpec.HTTP == nil || forwardingSpec.HTTP.Canary == nil) &&
		forwardingStatus != nil && forwardingStatus.Canary != nil {
		deleted = true
	}
	return created, deleted
}

func (c *syncContext) setCondition(condType string, status metav1.ConditionStatus, reason string) {
	meta.SetStatusCondition(&c.NewStatus.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: c.Object.Generation,
		Reason:             reason,
	})
}

func (c *syncContext) setRouteCondition(routeIndex int, status metav1.ConditionStatus, reason, msg string) {
	if routeIndex >= len(c.NewStatus.Routes) {
		return
	}
	if c.NewStatus.Routes[routeIndex].Condition.Status != status {
		c.NewStatus.Routes[routeIndex].Condition.Status = status
		c.NewStatus.Routes[routeIndex].Condition.LastTransitionTime = metav1.Now()
	}
	c.NewStatus.Routes[routeIndex].Condition.Reason = reason
	c.NewStatus.Routes[routeIndex].Condition.Message = msg
}

func (c *syncContext) Status() rolloutv1alpha1.BackendRoutingStatus {
	c.SetDefaults()

	backendReady := true
	reason := "Ready"
	if len(c.NewStatus.Backends.Origin.Name) > 0 {
		if !ptr.Deref(c.NewStatus.Backends.Origin.Conditions.Ready, false) {
			backendReady = false
			reason = "OriginBackendNotReady"
		}
	}
	if len(c.NewStatus.Backends.Stable.Name) > 0 {
		if !ptr.Deref(c.NewStatus.Backends.Stable.Conditions.Ready, false) || c.NewStatus.Backends.Stable.Conditions.Terminating != nil {
			backendReady = false
			reason = "StableBackendNotReady"
		}
	}
	if len(c.NewStatus.Backends.Canary.Name) > 0 {
		if !ptr.Deref(c.NewStatus.Backends.Canary.Conditions.Ready, false) || c.NewStatus.Backends.Canary.Conditions.Terminating != nil {
			backendReady = false
			reason = "CanaryBackendNotReady"
		}
	}

	if backendReady {
		c.setCondition(rolloutv1alpha1.BackendRoutingBackendReady, metav1.ConditionTrue, "Ready")
	} else {
		c.setCondition(rolloutv1alpha1.BackendRoutingBackendReady, metav1.ConditionFalse, reason)
	}

	routesReady := true
	for i := range c.NewStatus.Routes {
		if c.NewStatus.Routes[i].Condition.Status != metav1.ConditionTrue {
			routesReady = false
			reason = c.NewStatus.Routes[i].Condition.Reason
			break
		}
	}

	if routesReady {
		c.setCondition(rolloutv1alpha1.BackendRoutingRouteReady, metav1.ConditionTrue, "Ready")
	} else {
		c.setCondition(rolloutv1alpha1.BackendRoutingRouteReady, metav1.ConditionFalse, reason)
	}

	if backendReady && routesReady {
		c.setCondition(rolloutv1alpha1.BackendRoutingReady, metav1.ConditionTrue, "Ready")
	} else {
		c.setCondition(rolloutv1alpha1.BackendRoutingReady, metav1.ConditionFalse, "NotReady")
	}
	return *c.NewStatus
}

package httproute

import (
	"fmt"

	rolloutv1alpha1 "kusionstack.io/kube-api/rollout/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	"kusionstack.io/rollout/pkg/trafficrouting/route"
	"kusionstack.io/rollout/pkg/utils/accessor"
)

var GVK = gatewayapiv1.SchemeGroupVersion.WithKind("HTTPRoute")

var _ route.Route = &routeImpl{}

type routeImpl struct {
	accessor.ObjectAccessor
}

func New() route.Route {
	return &routeImpl{
		ObjectAccessor: accessor.NewObjectAccessor(
			GVK,
			&gatewayapiv1.HTTPRoute{},
			&gatewayapiv1.HTTPRouteList{},
		),
	}
}


func (r *routeImpl) GetController(client client.Client, br *rolloutv1alpha1.BackendRouting, route client.Object, routeStatus rolloutv1alpha1.BackendRouteStatus) (route.RouteController, error) {
	routeObj, ok := route.(*gatewayapiv1.HTTPRoute)
	if !ok {
		return nil, fmt.Errorf("input route is not networkingv1.Ingress")
	}
	return &httpRouteControl{
		client:         client,
		backendrouting: br,
		routeObj:       routeObj,
		routeStatus:    routeStatus,
	}, nil
}

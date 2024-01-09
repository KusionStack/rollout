package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:openapi-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true
// +kubebuilder:storageversion
// +kubebuilder:subresource:status

// TrafficTopologies defines the networking traffic relationships between
// workloads, backend services, and routes.
type TrafficTopology struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TrafficTopologySpec   `json:"spec,omitempty"`
	Status TrafficTopologyStatus `json:"status,omitempty"`
}

// TrafficTopologyList is a list of TrafficTopology resources.
type TrafficTopologyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []TrafficTopology `json:"items"`
}

// TrafficTopologySpec is the spec for a TrafficTopology resource.
type TrafficTopologySpec struct {
	// WorkloadRef is the reference to a kind of workloads
	WorkloadRef WorkloadRef `json:"workloadRef"`

	// TrafficTopologies defines the networking traffic relationships between
	// workloads, backend services, and routes.
	TrafficTopologies []TrafficBackendTopology `json:"trafficTopologies,omitempty"`
}

type TrafficBackendTopology struct {
	// TrafficType defines the type of traffic
	TrafficType TrafficType `json:"trafficType"`

	// Backend defines the reference to a kind of backend
	Backend BackendRef `json:"backend"`

	// Routes defines the list of routes
	Routes []RouteRef `json:"routes,omitempty"`

	// workloads is a subset match of all workloads
	Workloads *ResourceMatch `json:"workloads,omitempty"`
}

type TrafficType string

const (
	MultiClusterTrafficType TrafficType = "MultiCluster"
	InClusterTrafficType    TrafficType = "InCluster"
)

type BackendRef struct {
	// Group is the group of the referent. For example, "gateway.networking.k8s.io".
	// When unspecified or empty string, core API group is inferred.
	//
	// +optional
	// +kubebuilder:default=""
	APIVersion *string `json:"apiVersion,omitempty"`

	// Kind is the Kubernetes resource kind of the referent. For example
	// "Service".
	//
	// Defaults to "Service" when not specified.
	//
	// ExternalName services can refer to CNAME DNS records that may live
	// outside of the cluster and as such are difficult to reason about in
	// terms of conformance. They also may not be safe to forward to (see
	// CVE-2021-25740 for more information). Implementations SHOULD NOT
	// support ExternalName Services.
	//
	// Support: Core (Services with a type other than ExternalName)
	//
	// Support: Implementation-specific (Services with type ExternalName)
	//
	// +optional
	// +kubebuilder:default=Service
	Kind *string `json:"kind,omitempty"`

	// Name is the name of the referent.
	Name string `json:"name"`
}

type RouteRef struct {
	// APIVersion is the group/version of the referent. For example, "gateway.networking.k8s.io/v1".
	//
	// Defaults to "gateway.networking.k8s.io/v1" when not specified.
	//
	// +optional
	// +kubebuilder:default="gateway.networking.k8s.io/v1"
	APIVersion *string `json:"apiVersion,omitempty"`
	// Kind is the Kubernetes resource kind of the referent. For example
	// "HTTPRoute".
	//
	// Defaults to "HTTPRoute" when not specified.
	//
	// +optional
	// +kubebuilder:default=HTTPRoute
	Kind *string `json:"kind,omitempty"`
	// Name is the name of the custom route.
	Name string `json:"name"`
}

type TrafficTopologyStatus struct {
	// ObservedGeneration is the most recent generation observed.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// Conditions is the list of conditions
	Conditions []Condition `json:"conditions,omitempty"`
	// Topologies information aggregated by workload
	Topologies []TopologyInfo `json:"topologies,omitempty"`
}

type TopologyInfo struct {
	// workload reference name and cluster
	WorkloadRef CrossClusterObjectNameReference `json:"workloadRef,omitempty"`
	// backend routing references
	BackendRoutings []string `json:"backendRoutings,omitempty"`
}

// +genclient
// +k8s:openapi-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true
// +kubebuilder:storageversion
// +kubebuilder:subresource:status

// BackendRouting defines defines the association between frontend routes and
// backend service, and it allows the user to define forwarding rules for canary scenario.
type BackendRouting struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BackendRoutingSpec   `json:"spec,omitempty"`
	Status BackendRoutingStatus `json:"status,omitempty"`
}

// BackendRoutingList is a list of BackendRouting resources.
type BackendRoutingList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []BackendRouting `json:"items"`
}

type BackendRoutingSpec struct {
	// TrafficType defines the type of traffic
	TrafficType TrafficType `json:"trafficType"`
	// Backend defines the reference to a kind of backend
	Backend CrossClusterObjectReference `json:"backend"`
	// Routes defines the list of routes
	Routes []CrossClusterObjectReference `json:"routes,omitempty"`
	// Forwarding defines the forwarding rules for canary scenario
	Forwarding *BackendForwarding `json:"forwarding,omitempty"`
}

type BackendForwarding struct {
	Stable StableBackendRule `json:"stable,omitempty"`
	Canary CanaryBackendRule `json:"canary,omitempty"`
}

type StableBackendRule struct {
	// the temporary stable backend service name, generally it is the {originServiceName}-stable
	Name string `json:"name,omitempty"`
}

type CanaryBackendRule struct {
	// the temporary canary backend service name, generally it is the {originServiceName}-canary
	Name            string          `json:"name,omitempty"`
	TrafficStrategy TrafficStrategy `json:",inline"`
}

type TrafficStrategy struct {
	// Weight indicate how many percentage of traffic the canary pods should receive
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=100
	Weight   *int32         `json:"weight,omitempty"`
	HTTPRule *HTTPRouteRule `json:"http,omitempty"`
}

type BackendRoutingStatus struct {
	// ObservedGeneration is the most recent generation observed.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// Phase indicates the current phase of this object.
	Phase BackendRoutingPhase `json:"phase,omitempty"`
	// current backends routing
	Backends BackendStatuses `json:"backends,omitempty"`
	// route statuses
	RouteStatuses []BackendRouteStatus `json:"routeStatuses,omitempty"`
}

type BackendStatuses struct {
	// Origin backend status
	Origin BackendStatus `json:"origin,omitempty"`
	// Stable backend status
	Stable BackendStatus `json:"stable,omitempty"`
	// Canary backend status
	Canary BackendStatus `json:"canary,omitempty"`
}

type BackendStatus struct {
	// Name is the name of the referent.
	Name string `json:"name"`
	// Conditions represents the current condition of an backend.
	Conditions BackendConditions `json:"conditions,omitempty"`
}

// Backendonditions represents the current condition of an backend.
type BackendConditions struct {
	// ready indicates that this endpoint is prepared to receive traffic,
	// according to whatever system is managing the endpoint. A nil value
	// indicates an unknown state. In most cases consumers should interpret this
	// unknown state as ready. For compatibility reasons, ready should never be
	// "true" for terminating endpoints.
	// +optional
	Ready *bool `json:"ready,omitempty" protobuf:"bytes,1,name=ready"`

	// terminating indicates that this endpoint is terminating. A nil value
	// indicates an unknown state. Consumers should interpret this unknown state
	// to mean that the endpoint is not terminating.
	// +optional
	Terminating *bool `json:"terminating,omitempty" protobuf:"bytes,3,name=terminating"`
}

type BackendRoutingPhase string

const (
	BackendUpgrading BackendRoutingPhase = "BackendUpgrading"
	RouteUpgrading   BackendRoutingPhase = "RouteSyncing"
	Ready            BackendRoutingPhase = "Ready"
)

// BackendRouteStatus defines the status of a backend route.
type BackendRouteStatus struct {
	// CrossClusterObjectReference defines the reference to a kind of route resource.
	CrossClusterObjectReference `json:",inline"`
	// Synced indicates whether the backend route is synced.
	Synced bool `json:"synced,omitempty"`
}

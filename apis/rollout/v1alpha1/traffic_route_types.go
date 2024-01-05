package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayapiv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// +genclient
// +k8s:openapi-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true
// +kubebuilder:storageversion
// +kubebuilder:subresource:status

// TrafficTopology is a specification for a TrafficTopology resource.
type TrafficTopology struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TrafficTopologySpec   `json:"spec,omitempty"`
	Status TrafficTopologyStatus `json:"status,omitempty"`
}

// TrafficTopologySpec is the spec for a TrafficTopology resource.
type TrafficTopologySpec struct {
	WorkloadRef WorkloadRef              `json:"workloadRef"`
	Topologies  []TrafficBackendTopology `json:"topologies"`
}

type TrafficBackendTopology struct {
	TrafficType TrafficType    `json:"trafficType"`
	Backend     BackendRef     `json:"backend,omitempty"`
	Workloads   *ResourceMatch `json:"workloads"`
	Routes      []RouteRef     `json:"routes,omitempty"`
}

type TrafficType string

const (
	MultiClusterTraffic TrafficType = "MultiCluster"
	InClusterTraffic    TrafficType = "InCluster"
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
	ObservedGeneration int64          `json:"observedGeneration,omitempty"`
	Conditions         []Condition    `json:"conditions,omitempty"`
	Topologies         []TopologyInfo `json:"topologies,omitempty"`
}

type TopologyInfo struct {
	WorkloadRef  CrossClusterObjectNameReference `json:"workloadRef,omitempty"`
	BackendRules []string                        `json:"backendRules,omitempty"`
}

// TrafficTopologyList is a list of TrafficTopology resources.
type TrafficTopologyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []TrafficTopology `json:"items"`
}

type BackendControlRule struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BackendControlRuleSpec   `json:"spec,omitempty"`
	Status BackendControlRuleStatus `json:"status,omitempty"`
}

type BackendControlRuleSpec struct {
	TrafficType TrafficType                   `json:"trafficType"`
	Backend     CrossClusterObjectReference   `json:"backend"`
	Routes      []CrossClusterObjectReference `json:"routes,omitempty"`
	Canary      *BackendCanaryRule            `json:"canary,omitempty"`
}

type BackendCanaryRule struct {
	CanaryStep      string          `json:"canaryStep,omitempty"`
	TrafficStrategy TrafficStrategy `json:",inline"`
}

type BackendCanaryStep string

const (
	TrafficCanaryStepInitialize BackendCanaryStep = "Initialize"
	TrafficCanaryStepDoCanary   BackendCanaryStep = "DoCanary"
)

type TrafficStrategy struct {
	// Weight indicate how many percentage of traffic the canary pods should receive
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=100
	Weight   *int32         `json:"weight,omitempty"`
	HTTPRule *HTTPRouteRule `json:"http,omitempty"`
}

type BackendControlRuleStatus struct {
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	Conditions []Condition `json:"conditions,omitempty"`

	HTTPBackends HTTPBackends `json:"httpBackend,omitempty"`

	Phase BackendControlRulePhase `json:"phase,omitempty"`
}

type HTTPBackends struct {
	StableBackend gatewayapiv1.BackendRef `json:"stable,omitempty"`
	CanaryBackend CanaryHTTPBackend       `json:"canary,omitempty"`
}

type CanaryHTTPBackend struct {
	gatewayapiv1.BackendRef `json:",inline"`
	HTTPRouteRule           `json:",inline"`
	Conditions              BackendConditions `json:"conditions,omitempty"`
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

type BackendControlRulePhase string

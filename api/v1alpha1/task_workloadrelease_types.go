package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// WorkloadReleaseTask is the task for workload release
type WorkloadReleaseTask struct {
	// Workload is the workload of the task
	Workload Workload `json:"workload"`

	// Partition is the partition of the workload release
	// +optional
	Partition intstr.IntOrString `json:"partition,omitempty"`

	// ReadyCondition is the condition to check if the task is successful
	// +optional
	ReadyCondition *ReadyCondition `json:"readyCondition,omitempty"`
}

// Workload is the workload of the task
type Workload struct {
	metav1.TypeMeta `json:",inline"`

	// Name is the name of the workload
	// +optional
	Name string `json:"name,omitempty"`

	// Namespace is the namespace of the workload
	// +optional
	Namespace string `json:"namespace,omitempty"`

	// Cluster is the cluster of the task
	// +optional
	Cluster string `json:"cluster,omitempty"`
}

// ReadyCondition is the condition to check if the task is successful
type ReadyCondition struct {
	// AtLeastWaitingSeconds is the seconds to wait at least
	// +optional
	AtLeastWaitingSeconds int32 `json:"atLeastWaitingSeconds,omitempty"`

	// FailureThreshold indicates how many failed pods can be tolerated before marking the task as success
	// +optional
	FailureThreshold intstr.IntOrString `json:"failureThreshold,omitempty"`
}

// WorkloadReleaseTaskStatus defines the status of the workload release task
type WorkloadReleaseTaskStatus struct {
	// Name is the name of the workload
	Name string `json:"name,omitempty"`

	// ObservedGenerationUpdated indicates if the observedGeneration is updated to the latest generation
	ObservedGenerationUpdated bool `json:"observedGenerationUpdated"`

	// Replicas is the number of replicas
	Replicas int32 `json:"replicas"`

	// UpdatedReplicas is the number of updated replicas
	UpdatedReplicas int32 `json:"updatedReplicas"`

	// UpdatedReadyReplicas is the number of updated ready replicas
	UpdatedReadyReplicas int32 `json:"updatedReadyReplicas"`

	// UpdatedAvailableReplicas is the number of updated available replicas
	UpdatedAvailableReplicas int32 `json:"updatedAvailableReplicas"`
}

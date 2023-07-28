package builder

import (
	"github.com/KusionStack/rollout/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	defaultWorkloadNamespace = DefaultNamespace
	defaultWorkloadName      = DefaultName
	defaultWorkloadCluster   = DefaultCluster
	defaultTaskType          = v1alpha1.TaskTypeWorkloadRelease
)

var defaultWorkloadPartition = &intstr.IntOrString{
	Type:   intstr.Int,
	IntVal: 1,
}

// TaskBuilder is a builder for Task
type TaskBuilder struct {
	builder
	taskType          v1alpha1.TaskType
	workloadNamespace string
	workloadName      string
	workloadCluster   string
	workloadPartition *intstr.IntOrString
}

// NewTask returns a Task builder
func NewTask() *TaskBuilder {
	return &TaskBuilder{}
}

// WorkloadNamespace sets the namespace of the workload
func (b *TaskBuilder) WorkloadNamespace(workloadNamespace string) *TaskBuilder {
	b.workloadNamespace = workloadNamespace
	return b
}

// WorkloadName sets the name of the workload
func (b *TaskBuilder) WorkloadName(workloadName string) *TaskBuilder {
	b.workloadName = workloadName
	return b
}

// WorkloadCluster sets the cluster of the workload
func (b *TaskBuilder) WorkloadCluster(workloadCluster string) *TaskBuilder {
	b.workloadCluster = workloadCluster
	return b
}

// WorkloadPartition sets the partition of the workload
func (b *TaskBuilder) WorkloadPartition(partition *intstr.IntOrString) *TaskBuilder {
	b.workloadPartition = partition
	return b
}

// Type sets the type of the Task
func (b *TaskBuilder) Type(taskType v1alpha1.TaskType) *TaskBuilder {
	b.taskType = taskType
	return b
}

// Build returns a Task
func (b *TaskBuilder) Build() *v1alpha1.Task {
	b.complete()

	return &v1alpha1.Task{
		TypeMeta: metav1.TypeMeta{
			APIVersion: v1alpha1.GroupVersion.String(),
			Kind:       "Task",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      b.name,
			Namespace: b.namespace,
		},
		Spec: b.buildTaskSpec(),
	}
}

// buildTaskSpec returns a TaskSpec
func (b *TaskBuilder) buildTaskSpec() v1alpha1.TaskSpec {
	switch b.taskType {
	case v1alpha1.TaskTypeWorkloadRelease:
		return v1alpha1.TaskSpec{
			WorkloadRelease: &v1alpha1.WorkloadReleaseTask{
				Workload: v1alpha1.Workload{
					TypeMeta: metav1.TypeMeta{
						// TODO: add workload type
						APIVersion: "",
						Kind:       "CollaSet",
					},
					Name:      b.workloadName,
					Namespace: b.workloadNamespace,
					Cluster:   b.workloadCluster,
				},
				Partition: *b.workloadPartition,
			},
		}
	}
	return v1alpha1.TaskSpec{}
}

// complete sets default values for the Task builder
func (b *TaskBuilder) complete() {
	b.builder.complete()
	if b.taskType == "" {
		b.taskType = defaultTaskType
	}
	if b.taskType == v1alpha1.TaskTypeWorkloadRelease {
		b.completeWorkloadReleaseTask()
	}
}

// completeWorkloadReleaseTask sets default values for the WorkloadReleaseTask builder
func (b *TaskBuilder) completeWorkloadReleaseTask() {
	if b.workloadNamespace == "" {
		b.workloadNamespace = defaultWorkloadNamespace
	}
	if b.workloadName == "" {
		b.workloadName = defaultWorkloadName
	}
	if b.workloadCluster == "" {
		b.workloadCluster = defaultWorkloadCluster
	}
	if b.workloadPartition == nil {
		b.workloadPartition = defaultWorkloadPartition
	}
}

package v1alpha1

import "time"

// SuspendTask is a task that suspends the workflow
type SuspendTask struct {
	// Approved indicates whether the task is approved to continue
	// +optional
	Approved *bool `json:"approved,omitempty"`

	// Duration is the duration to suspend the workflow
	// +optional
	Duration *time.Duration `json:"duration,omitempty"`
}

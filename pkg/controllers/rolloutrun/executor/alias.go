package executor

import rolloutv1alpha1 "kusionstack.io/rollout/apis/rollout/v1alpha1"

const (
	BatchStateInitial       = rolloutv1alpha1.BatchStepStatePending
	BatchStatePaused        = rolloutv1alpha1.BatchStepStatePaused
	BatchStatePreBatchHook  = rolloutv1alpha1.BatchStepStatePreBatchStepHook
	BatchStateRunning       = rolloutv1alpha1.BatchStepStateRunning
	BatchStatePostBatchHook = rolloutv1alpha1.BatchStepStatePostBatchStepHook
	BatchStateSucceeded     = rolloutv1alpha1.BatchStepStateSucceeded
)

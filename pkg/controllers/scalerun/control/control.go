/**
 * Copyright 2024 The KusionStack Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     https://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package control

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/go-logr/logr"
	rolloutapi "kusionstack.io/kube-api/rollout"
	rolloutv1alpha1 "kusionstack.io/kube-api/rollout/v1alpha1"
	"kusionstack.io/kube-utils/multicluster/clusterinfo"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"kusionstack.io/rollout/pkg/utils"
	"kusionstack.io/rollout/pkg/workload"
)

type BatchScaleControl struct {
	workload workload.Accessor
	control  workload.BatchScaleControl
	client   client.Client
}

func NewBatchScaleControl(impl workload.Accessor, c client.Client) *BatchScaleControl {
	return &BatchScaleControl{
		workload: impl,
		control:  impl.(workload.BatchScaleControl),
		client:   c,
	}
}

func (c *BatchScaleControl) Initialize(ctx context.Context, info *workload.Info, scaleRun string, batchIndex int32) error {
	// pre-check
	if err := c.control.ScalePreCheck(info.Object); err != nil {
		return TerminalError(err)
	}

	// add progressing annotation
	pInfo := rolloutv1alpha1.ProgressingInfo{
		Kind:      "RollingScale",
		RolloutID: scaleRun,
		Batch: &rolloutv1alpha1.BatchProgressingInfo{
			CurrentBatchIndex: batchIndex,
		},
	}
	progress, _ := json.Marshal(pInfo)

	_, err := info.UpdateOnConflict(ctx, c.client, func(obj client.Object) error {
		utils.MutateAnnotations(obj, func(annotations map[string]string) {
			annotations[rolloutapi.AnnoRolloutProgressingInfo] = string(progress)
		})
		return nil
	})
	return err
}

func (c *BatchScaleControl) UpdateReplicas(ctx context.Context, info *workload.Info, updatedReplicas int32) (bool, error) {
	ctx = clusterinfo.WithCluster(ctx, info.ClusterName)
	obj := info.Object
	return utils.PatchOnConflict(ctx, c.client, c.client, obj, func() error {
		return c.control.ApplyReplicas(obj, updatedReplicas)
	})
}

func (c *BatchScaleControl) Finalize(ctx context.Context, info *workload.Info) error {
	// delete progressing annotation
	changed, err := info.UpdateOnConflict(ctx, c.client, func(obj client.Object) error {
		utils.MutateAnnotations(obj, func(annotations map[string]string) {
			delete(annotations, rolloutapi.AnnoRolloutProgressingInfo)
		})
		return nil
	})

	if changed {
		logger := logr.FromContextOrDiscard(ctx)
		logger.Info("delete progressing info on workload", "name", info.Name, "gvk", info.GroupVersionKind.String())
	}
	return err
}

// TerminalError is an error that will not be retried but still be logged
// and recorded in metrics.
//
// TODO: delete this error when controller-runtime version is grather than v0.15
func TerminalError(wrapped error) error {
	return &terminalError{err: wrapped}
}

type terminalError struct {
	err error
}

// This function will return nil if te.err is nil.
func (te *terminalError) Unwrap() error {
	return te.err
}

func (te *terminalError) Error() string {
	if te.err == nil {
		return "nil terminal error"
	}
	return "terminal error: " + te.err.Error()
}

func (te *terminalError) Is(target error) bool {
	tp := &terminalError{}
	return errors.As(target, &tp)
}

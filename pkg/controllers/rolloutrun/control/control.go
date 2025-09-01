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
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	rolloutapi "kusionstack.io/kube-api/rollout"
	rolloutv1alpha1 "kusionstack.io/kube-api/rollout/v1alpha1"
	kubeutilclient "kusionstack.io/kube-utils/client"
	"kusionstack.io/kube-utils/multicluster/clusterinfo"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"kusionstack.io/rollout/pkg/utils"
	"kusionstack.io/rollout/pkg/workload"
)

type BatchReleaseControl struct {
	workload workload.Accessor
	control  workload.BatchReleaseControl
	client   client.Client
}

func NewBatchReleaseControl(impl workload.Accessor, c client.Client) *BatchReleaseControl {
	return &BatchReleaseControl{
		workload: impl,
		control:  impl.(workload.BatchReleaseControl),
		client:   c,
	}
}

func (c *BatchReleaseControl) Initialize(ctx context.Context, info *workload.Info, ownerKind, ownerName, rolloutRun string, batchIndex int32) error {
	// pre-check
	if err := c.control.BatchPreCheck(info.Object); err != nil {
		return TerminalError(err)
	}

	// add progressing annotation
	pInfo := rolloutv1alpha1.ProgressingInfo{
		Kind:        ownerKind,
		RolloutName: ownerName,
		RolloutID:   rolloutRun,
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

func (c *BatchReleaseControl) UpdatePartition(ctx context.Context, info *workload.Info, expectedUpdated int32) (bool, error) {
	ctx = clusterinfo.WithCluster(ctx, info.ClusterName)
	obj := info.Object
	return utils.UpdateOnConflict(ctx, c.client, c.client, obj, func() error {
		return c.control.ApplyPartition(obj, expectedUpdated)
	})
}

func (c *BatchReleaseControl) Finalize(ctx context.Context, info *workload.Info) error {
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

type CanaryReleaseControl struct {
	workload workload.Accessor
	control  workload.CanaryReleaseControl
	client   client.Client
}

func NewCanaryReleaseControl(impl workload.Accessor, c client.Client) *CanaryReleaseControl {
	return &CanaryReleaseControl{
		workload: impl,
		control:  impl.(workload.CanaryReleaseControl),
		client:   c,
	}
}

func (c *CanaryReleaseControl) Initialize(ctx context.Context, stable *workload.Info, ownerKind, ownerName, rolloutRun string) error {
	// pre check
	if err := c.control.CanaryPreCheck(stable.Object); err != nil {
		return TerminalError(err)
	}

	// add progressing annotation
	info := rolloutv1alpha1.ProgressingInfo{
		Kind:        ownerKind,
		RolloutName: ownerName,
		RolloutID:   rolloutRun,
		Canary:      &rolloutv1alpha1.CanaryProgressingInfo{},
	}
	progress, _ := json.Marshal(info)

	// set progressing info
	_, err := stable.UpdateOnConflict(ctx, c.client, func(obj client.Object) error {
		utils.MutateAnnotations(obj, func(annotations map[string]string) {
			annotations[rolloutapi.AnnoRolloutProgressingInfo] = string(progress)
		})
		return nil
	})

	return err
}

func (c *CanaryReleaseControl) Finalize(ctx context.Context, stable *workload.Info) error {
	canaryObj, err := c.GetCanaryObject(stable.ClusterName, stable.Namespace, stable.Name)
	if client.IgnoreNotFound(err) != nil {
		return err
	}
	if apierrors.IsNotFound(err) {
		return nil
	}

	err = kubeutilclient.RemoveFinalizerAndDelete(
		clusterinfo.WithCluster(ctx, stable.ClusterName),
		c.client,
		canaryObj,
		rolloutapi.FinalizerCanaryResourceProtection,
	)
	if client.IgnoreNotFound(err) != nil {
		return err
	}

	// delete progressing annotation
	_, err = stable.UpdateOnConflict(ctx, c.client, func(obj client.Object) error {
		utils.MutateAnnotations(obj, func(annotations map[string]string) {
			delete(annotations, rolloutapi.AnnoRolloutProgressingInfo)
		})
		return nil
	})
	return err
}

func (c *CanaryReleaseControl) CreateOrUpdate(ctx context.Context, stable *workload.Info, replicas intstr.IntOrString, podTemplatePatch *rolloutv1alpha1.MetadataPatch) (controllerutil.OperationResult, *workload.Info, error) {
	canaryObj, found, err := c.canaryObject(stable)
	if err != nil {
		return controllerutil.OperationResultNone, nil, err
	}

	cluster := stable.ClusterName
	ctx = clusterinfo.WithCluster(ctx, cluster)

	canaryReplicas, err := workload.CalculateUpdatedReplicas(&stable.Status.Replicas, replicas)
	if err != nil {
		return controllerutil.OperationResultNone, nil, err
	}

	if !found {
		// create
		c.applyCanaryDefaults(canaryObj)
		c.control.Scale(canaryObj, canaryReplicas)              // nolint
		c.control.ApplyCanaryPatch(canaryObj, podTemplatePatch) // nolint
		err := c.client.Create(ctx, canaryObj)
		if err != nil {
			return controllerutil.OperationResultNone, nil, err
		}
		canaryInfo, err := c.workload.GetInfo(cluster, canaryObj)
		if err != nil {
			return controllerutil.OperationResultNone, nil, err
		}
		return controllerutil.OperationResultCreated, canaryInfo, nil
	}

	// update
	updated, err := utils.UpdateOnConflict(ctx, c.client, c.client, canaryObj, func() error {
		c.applyCanaryDefaults(canaryObj)
		c.control.Scale(canaryObj, canaryReplicas) // nolint
		return nil
	})
	if err != nil {
		return controllerutil.OperationResultNone, nil, err
	}
	canaryInfo, err := c.workload.GetInfo(cluster, canaryObj)
	if err != nil {
		return controllerutil.OperationResultNone, nil, err
	}
	if !updated {
		return controllerutil.OperationResultNone, canaryInfo, nil
	}
	return controllerutil.OperationResultUpdated, canaryInfo, nil
}

func (c *CanaryReleaseControl) GetCanaryObject(cluster, namespace, name string) (client.Object, error) {
	if strings.HasSuffix(name, "-canary") {
		return nil, fmt.Errorf("input name should not end with -canary, got=%s", name)
	}
	canaryName := workload.GetCanaryName(name)
	canaryObj := c.workload.NewObject()
	err := c.client.Get(
		clusterinfo.WithCluster(context.TODO(), cluster),
		client.ObjectKey{Namespace: namespace, Name: canaryName},
		canaryObj,
	)
	return canaryObj, err
}

func (c *CanaryReleaseControl) canaryObject(stable *workload.Info) (client.Object, bool, error) {
	// retrieve canary object
	canaryObj, err := c.GetCanaryObject(stable.ClusterName, stable.Namespace, stable.Name)
	if client.IgnoreNotFound(err) != nil {
		return nil, false, err
	}

	found := true
	if apierrors.IsNotFound(err) {
		found = false
		// deepcopy object
		var ok bool
		canaryObj, ok = stable.Object.DeepCopyObject().(client.Object)
		if !ok {
			return nil, false, fmt.Errorf("object can not convert to client.Object")
		}

		// cleanup stable metadata
		canaryObj.SetUID(types.UID(""))
		canaryObj.SetResourceVersion("")
		canaryObj.SetSelfLink("")
		canaryObj.SetGeneration(0)
		canaryObj.SetCreationTimestamp(metav1.Time{})
		canaryObj.SetDeletionTimestamp(nil)
		canaryObj.SetOwnerReferences(nil)
		canaryObj.SetFinalizers(nil)
		canaryObj.SetManagedFields(nil)
		// set canary metadata
		canaryObj.SetName(workload.GetCanaryName(stable.Name))
	}

	return canaryObj, found, nil
}

func (c *CanaryReleaseControl) applyCanaryDefaults(canaryObj client.Object) {
	controllerutil.AddFinalizer(canaryObj, rolloutapi.FinalizerCanaryResourceProtection)
	utils.MutateLabels(canaryObj, func(labels map[string]string) {
		labels[rolloutapi.CanaryResourceLabelKey] = "true"
	})
}

type RollbackReleaseControl struct {
	workload workload.Accessor
	control  workload.RollbackReleaseControl
	client   client.Client
}

func NewRollbackReleaseControl(impl workload.Accessor, c client.Client) *RollbackReleaseControl {
	return &RollbackReleaseControl{
		workload: impl,
		control:  impl.(workload.RollbackReleaseControl),
		client:   c,
	}
}

func (c *RollbackReleaseControl) Initialize(ctx context.Context, info *workload.Info, ownerKind, ownerName, rolloutRun string, batchIndex int32) error {
	// pre-check
	if !c.control.Rollbackable() {
		return fmt.Errorf("workload is not support rollback")
	}

	if err := c.control.RollbackPreCheck(info.Object); err != nil {
		return TerminalError(err)
	}

	// add progressing annotation
	pInfo := rolloutv1alpha1.ProgressingInfo{
		Kind:        ownerKind,
		RolloutName: ownerName,
		RolloutID:   rolloutRun,
		Rollback: &rolloutv1alpha1.BatchProgressingInfo{
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

func (c *RollbackReleaseControl) Revert(ctx context.Context, info *workload.Info) error {
	// pre-check
	if !c.control.Rollbackable() {
		return fmt.Errorf("workload is not support rollback")
	}

	if err := c.control.RollbackPreCheck(info.Object); err != nil {
		return TerminalError(err)
	}

	_, err := info.UpdateForRevisionOnConflict(ctx, c.client, func(obj client.Object, revison *appsv1.ControllerRevision) error {
		return c.control.RevertRevision(obj, revison)
	})

	return err
}

func (c *RollbackReleaseControl) UpdatePartition(ctx context.Context, info *workload.Info, expectedUpdated int32) (bool, error) {
	ctx = clusterinfo.WithCluster(ctx, info.ClusterName)
	obj := info.Object
	return utils.UpdateOnConflict(ctx, c.client, c.client, obj, func() error {
		return c.control.ApplyPartition(obj, expectedUpdated)
	})
}

func (c *RollbackReleaseControl) Finalize(ctx context.Context, info *workload.Info) error {
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

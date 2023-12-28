//go:build !ignore_autogenerated
// +build !ignore_autogenerated

// Copyright 2023 The KusionStack Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Code generated by controller-gen. DO NOT EDIT.

package v1alpha1

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *BatchStrategy) DeepCopyInto(out *BatchStrategy) {
	*out = *in
	if in.Batches != nil {
		in, out := &in.Batches, &out.Batches
		*out = make([]RolloutStep, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.Toleration != nil {
		in, out := &in.Toleration, &out.Toleration
		*out = new(TolerationStrategy)
		(*in).DeepCopyInto(*out)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new BatchStrategy.
func (in *BatchStrategy) DeepCopy() *BatchStrategy {
	if in == nil {
		return nil
	}
	out := new(BatchStrategy)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *BatchWebhookStatus) DeepCopyInto(out *BatchWebhookStatus) {
	*out = *in
	out.CodeReasonMessage = in.CodeReasonMessage
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new BatchWebhookStatus.
func (in *BatchWebhookStatus) DeepCopy() *BatchWebhookStatus {
	if in == nil {
		return nil
	}
	out := new(BatchWebhookStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *CodeReasonMessage) DeepCopyInto(out *CodeReasonMessage) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new CodeReasonMessage.
func (in *CodeReasonMessage) DeepCopy() *CodeReasonMessage {
	if in == nil {
		return nil
	}
	out := new(CodeReasonMessage)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Condition) DeepCopyInto(out *Condition) {
	*out = *in
	in.LastTransitionTime.DeepCopyInto(&out.LastTransitionTime)
	in.LastUpdateTime.DeepCopyInto(&out.LastUpdateTime)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Condition.
func (in *Condition) DeepCopy() *Condition {
	if in == nil {
		return nil
	}
	out := new(Condition)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *CrossClusterObjectNameReference) DeepCopyInto(out *CrossClusterObjectNameReference) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new CrossClusterObjectNameReference.
func (in *CrossClusterObjectNameReference) DeepCopy() *CrossClusterObjectNameReference {
	if in == nil {
		return nil
	}
	out := new(CrossClusterObjectNameReference)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *HTTPHeader) DeepCopyInto(out *HTTPHeader) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new HTTPHeader.
func (in *HTTPHeader) DeepCopy() *HTTPHeader {
	if in == nil {
		return nil
	}
	out := new(HTTPHeader)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *HTTPHeaderFilter) DeepCopyInto(out *HTTPHeaderFilter) {
	*out = *in
	if in.Set != nil {
		in, out := &in.Set, &out.Set
		*out = make([]HTTPHeader, len(*in))
		copy(*out, *in)
	}
	if in.Add != nil {
		in, out := &in.Add, &out.Add
		*out = make([]HTTPHeader, len(*in))
		copy(*out, *in)
	}
	if in.Remove != nil {
		in, out := &in.Remove, &out.Remove
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new HTTPHeaderFilter.
func (in *HTTPHeaderFilter) DeepCopy() *HTTPHeaderFilter {
	if in == nil {
		return nil
	}
	out := new(HTTPHeaderFilter)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *HTTPHeaderMatch) DeepCopyInto(out *HTTPHeaderMatch) {
	*out = *in
	if in.Type != nil {
		in, out := &in.Type, &out.Type
		*out = new(HeaderMatchType)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new HTTPHeaderMatch.
func (in *HTTPHeaderMatch) DeepCopy() *HTTPHeaderMatch {
	if in == nil {
		return nil
	}
	out := new(HTTPHeaderMatch)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *HTTPRouteMatch) DeepCopyInto(out *HTTPRouteMatch) {
	*out = *in
	if in.Headers != nil {
		in, out := &in.Headers, &out.Headers
		*out = make([]HTTPHeaderMatch, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new HTTPRouteMatch.
func (in *HTTPRouteMatch) DeepCopy() *HTTPRouteMatch {
	if in == nil {
		return nil
	}
	out := new(HTTPRouteMatch)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *HTTPTrafficStrategy) DeepCopyInto(out *HTTPTrafficStrategy) {
	*out = *in
	if in.Matches != nil {
		in, out := &in.Matches, &out.Matches
		*out = make([]HTTPRouteMatch, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.RequestHeaderModifier != nil {
		in, out := &in.RequestHeaderModifier, &out.RequestHeaderModifier
		*out = new(HTTPHeaderFilter)
		(*in).DeepCopyInto(*out)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new HTTPTrafficStrategy.
func (in *HTTPTrafficStrategy) DeepCopy() *HTTPTrafficStrategy {
	if in == nil {
		return nil
	}
	out := new(HTTPTrafficStrategy)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ObjectTypeRef) DeepCopyInto(out *ObjectTypeRef) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ObjectTypeRef.
func (in *ObjectTypeRef) DeepCopy() *ObjectTypeRef {
	if in == nil {
		return nil
	}
	out := new(ObjectTypeRef)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ResourceMatch) DeepCopyInto(out *ResourceMatch) {
	*out = *in
	if in.Selector != nil {
		in, out := &in.Selector, &out.Selector
		*out = new(v1.LabelSelector)
		(*in).DeepCopyInto(*out)
	}
	if in.Names != nil {
		in, out := &in.Names, &out.Names
		*out = make([]CrossClusterObjectNameReference, len(*in))
		copy(*out, *in)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ResourceMatch.
func (in *ResourceMatch) DeepCopy() *ResourceMatch {
	if in == nil {
		return nil
	}
	out := new(ResourceMatch)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Rollout) DeepCopyInto(out *Rollout) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Rollout.
func (in *Rollout) DeepCopy() *Rollout {
	if in == nil {
		return nil
	}
	out := new(Rollout)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *Rollout) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RolloutBatchStatus) DeepCopyInto(out *RolloutBatchStatus) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RolloutBatchStatus.
func (in *RolloutBatchStatus) DeepCopy() *RolloutBatchStatus {
	if in == nil {
		return nil
	}
	out := new(RolloutBatchStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RolloutList) DeepCopyInto(out *RolloutList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]Rollout, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RolloutList.
func (in *RolloutList) DeepCopy() *RolloutList {
	if in == nil {
		return nil
	}
	out := new(RolloutList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *RolloutList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RolloutReplicasSummary) DeepCopyInto(out *RolloutReplicasSummary) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RolloutReplicasSummary.
func (in *RolloutReplicasSummary) DeepCopy() *RolloutReplicasSummary {
	if in == nil {
		return nil
	}
	out := new(RolloutReplicasSummary)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RolloutRun) DeepCopyInto(out *RolloutRun) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RolloutRun.
func (in *RolloutRun) DeepCopy() *RolloutRun {
	if in == nil {
		return nil
	}
	out := new(RolloutRun)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *RolloutRun) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RolloutRunBatchStatus) DeepCopyInto(out *RolloutRunBatchStatus) {
	*out = *in
	out.RolloutBatchStatus = in.RolloutBatchStatus
	if in.Error != nil {
		in, out := &in.Error, &out.Error
		*out = new(CodeReasonMessage)
		**out = **in
	}
	if in.Context != nil {
		in, out := &in.Context, &out.Context
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
	if in.Records != nil {
		in, out := &in.Records, &out.Records
		*out = make([]RolloutRunBatchStatusRecord, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RolloutRunBatchStatus.
func (in *RolloutRunBatchStatus) DeepCopy() *RolloutRunBatchStatus {
	if in == nil {
		return nil
	}
	out := new(RolloutRunBatchStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RolloutRunBatchStatusRecord) DeepCopyInto(out *RolloutRunBatchStatusRecord) {
	*out = *in
	if in.Index != nil {
		in, out := &in.Index, &out.Index
		*out = new(int32)
		**out = **in
	}
	if in.Error != nil {
		in, out := &in.Error, &out.Error
		*out = new(CodeReasonMessage)
		**out = **in
	}
	if in.StartTime != nil {
		in, out := &in.StartTime, &out.StartTime
		*out = (*in).DeepCopy()
	}
	if in.FinishTime != nil {
		in, out := &in.FinishTime, &out.FinishTime
		*out = (*in).DeepCopy()
	}
	if in.Targets != nil {
		in, out := &in.Targets, &out.Targets
		*out = make([]RolloutWorkloadStatus, len(*in))
		copy(*out, *in)
	}
	if in.Webhooks != nil {
		in, out := &in.Webhooks, &out.Webhooks
		*out = make([]BatchWebhookStatus, len(*in))
		copy(*out, *in)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RolloutRunBatchStatusRecord.
func (in *RolloutRunBatchStatusRecord) DeepCopy() *RolloutRunBatchStatusRecord {
	if in == nil {
		return nil
	}
	out := new(RolloutRunBatchStatusRecord)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RolloutRunBatchStrategy) DeepCopyInto(out *RolloutRunBatchStrategy) {
	*out = *in
	if in.Batches != nil {
		in, out := &in.Batches, &out.Batches
		*out = make([]RolloutRunStep, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.Toleration != nil {
		in, out := &in.Toleration, &out.Toleration
		*out = new(TolerationStrategy)
		(*in).DeepCopyInto(*out)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RolloutRunBatchStrategy.
func (in *RolloutRunBatchStrategy) DeepCopy() *RolloutRunBatchStrategy {
	if in == nil {
		return nil
	}
	out := new(RolloutRunBatchStrategy)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RolloutRunList) DeepCopyInto(out *RolloutRunList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]RolloutRun, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RolloutRunList.
func (in *RolloutRunList) DeepCopy() *RolloutRunList {
	if in == nil {
		return nil
	}
	out := new(RolloutRunList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *RolloutRunList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RolloutRunSpec) DeepCopyInto(out *RolloutRunSpec) {
	*out = *in
	out.TargetType = in.TargetType
	if in.Webhooks != nil {
		in, out := &in.Webhooks, &out.Webhooks
		*out = make([]RolloutWebhook, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	in.Batch.DeepCopyInto(&out.Batch)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RolloutRunSpec.
func (in *RolloutRunSpec) DeepCopy() *RolloutRunSpec {
	if in == nil {
		return nil
	}
	out := new(RolloutRunSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RolloutRunStatus) DeepCopyInto(out *RolloutRunStatus) {
	*out = *in
	if in.Conditions != nil {
		in, out := &in.Conditions, &out.Conditions
		*out = make([]Condition, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.LastUpdateTime != nil {
		in, out := &in.LastUpdateTime, &out.LastUpdateTime
		*out = (*in).DeepCopy()
	}
	if in.BatchStatus != nil {
		in, out := &in.BatchStatus, &out.BatchStatus
		*out = new(RolloutRunBatchStatus)
		(*in).DeepCopyInto(*out)
	}
	if in.TargetStatuses != nil {
		in, out := &in.TargetStatuses, &out.TargetStatuses
		*out = make([]RolloutWorkloadStatus, len(*in))
		copy(*out, *in)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RolloutRunStatus.
func (in *RolloutRunStatus) DeepCopy() *RolloutRunStatus {
	if in == nil {
		return nil
	}
	out := new(RolloutRunStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RolloutRunStep) DeepCopyInto(out *RolloutRunStep) {
	*out = *in
	in.TrafficStrategy.DeepCopyInto(&out.TrafficStrategy)
	if in.Targets != nil {
		in, out := &in.Targets, &out.Targets
		*out = make([]RolloutRunStepTarget, len(*in))
		copy(*out, *in)
	}
	if in.Properties != nil {
		in, out := &in.Properties, &out.Properties
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RolloutRunStep.
func (in *RolloutRunStep) DeepCopy() *RolloutRunStep {
	if in == nil {
		return nil
	}
	out := new(RolloutRunStep)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RolloutRunStepTarget) DeepCopyInto(out *RolloutRunStepTarget) {
	*out = *in
	out.CrossClusterObjectNameReference = in.CrossClusterObjectNameReference
	out.Replicas = in.Replicas
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RolloutRunStepTarget.
func (in *RolloutRunStepTarget) DeepCopy() *RolloutRunStepTarget {
	if in == nil {
		return nil
	}
	out := new(RolloutRunStepTarget)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RolloutSpec) DeepCopyInto(out *RolloutSpec) {
	*out = *in
	in.WorkloadRef.DeepCopyInto(&out.WorkloadRef)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RolloutSpec.
func (in *RolloutSpec) DeepCopy() *RolloutSpec {
	if in == nil {
		return nil
	}
	out := new(RolloutSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RolloutStatus) DeepCopyInto(out *RolloutStatus) {
	*out = *in
	if in.Conditions != nil {
		in, out := &in.Conditions, &out.Conditions
		*out = make([]Condition, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.LastUpdateTime != nil {
		in, out := &in.LastUpdateTime, &out.LastUpdateTime
		*out = (*in).DeepCopy()
	}
	if in.BatchStatus != nil {
		in, out := &in.BatchStatus, &out.BatchStatus
		*out = new(RolloutBatchStatus)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RolloutStatus.
func (in *RolloutStatus) DeepCopy() *RolloutStatus {
	if in == nil {
		return nil
	}
	out := new(RolloutStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RolloutStep) DeepCopyInto(out *RolloutStep) {
	*out = *in
	in.TrafficStrategy.DeepCopyInto(&out.TrafficStrategy)
	out.Replicas = in.Replicas
	if in.Match != nil {
		in, out := &in.Match, &out.Match
		*out = new(ResourceMatch)
		(*in).DeepCopyInto(*out)
	}
	if in.Properties != nil {
		in, out := &in.Properties, &out.Properties
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RolloutStep.
func (in *RolloutStep) DeepCopy() *RolloutStep {
	if in == nil {
		return nil
	}
	out := new(RolloutStep)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RolloutStrategy) DeepCopyInto(out *RolloutStrategy) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	if in.Batch != nil {
		in, out := &in.Batch, &out.Batch
		*out = new(BatchStrategy)
		(*in).DeepCopyInto(*out)
	}
	if in.Webhooks != nil {
		in, out := &in.Webhooks, &out.Webhooks
		*out = make([]RolloutWebhook, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RolloutStrategy.
func (in *RolloutStrategy) DeepCopy() *RolloutStrategy {
	if in == nil {
		return nil
	}
	out := new(RolloutStrategy)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *RolloutStrategy) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RolloutStrategyList) DeepCopyInto(out *RolloutStrategyList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]RolloutStrategy, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RolloutStrategyList.
func (in *RolloutStrategyList) DeepCopy() *RolloutStrategyList {
	if in == nil {
		return nil
	}
	out := new(RolloutStrategyList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *RolloutStrategyList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RolloutWebhook) DeepCopyInto(out *RolloutWebhook) {
	*out = *in
	if in.HookTypes != nil {
		in, out := &in.HookTypes, &out.HookTypes
		*out = make([]HookType, len(*in))
		copy(*out, *in)
	}
	in.ClientConfig.DeepCopyInto(&out.ClientConfig)
	if in.Properties != nil {
		in, out := &in.Properties, &out.Properties
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
	if in.Provider != nil {
		in, out := &in.Provider, &out.Provider
		*out = new(string)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RolloutWebhook.
func (in *RolloutWebhook) DeepCopy() *RolloutWebhook {
	if in == nil {
		return nil
	}
	out := new(RolloutWebhook)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RolloutWebhookReview) DeepCopyInto(out *RolloutWebhookReview) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	out.Status = in.Status
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RolloutWebhookReview.
func (in *RolloutWebhookReview) DeepCopy() *RolloutWebhookReview {
	if in == nil {
		return nil
	}
	out := new(RolloutWebhookReview)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *RolloutWebhookReview) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RolloutWebhookReviewSpec) DeepCopyInto(out *RolloutWebhookReviewSpec) {
	*out = *in
	out.TargetType = in.TargetType
	if in.Targets != nil {
		in, out := &in.Targets, &out.Targets
		*out = make([]RolloutRunStepTarget, len(*in))
		copy(*out, *in)
	}
	if in.Properties != nil {
		in, out := &in.Properties, &out.Properties
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RolloutWebhookReviewSpec.
func (in *RolloutWebhookReviewSpec) DeepCopy() *RolloutWebhookReviewSpec {
	if in == nil {
		return nil
	}
	out := new(RolloutWebhookReviewSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RolloutWebhookReviewStatus) DeepCopyInto(out *RolloutWebhookReviewStatus) {
	*out = *in
	out.CodeReasonMessage = in.CodeReasonMessage
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RolloutWebhookReviewStatus.
func (in *RolloutWebhookReviewStatus) DeepCopy() *RolloutWebhookReviewStatus {
	if in == nil {
		return nil
	}
	out := new(RolloutWebhookReviewStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RolloutWorkloadStatus) DeepCopyInto(out *RolloutWorkloadStatus) {
	*out = *in
	out.RolloutReplicasSummary = in.RolloutReplicasSummary
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RolloutWorkloadStatus.
func (in *RolloutWorkloadStatus) DeepCopy() *RolloutWorkloadStatus {
	if in == nil {
		return nil
	}
	out := new(RolloutWorkloadStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *TolerationStrategy) DeepCopyInto(out *TolerationStrategy) {
	*out = *in
	if in.WorkloadFailureThreshold != nil {
		in, out := &in.WorkloadFailureThreshold, &out.WorkloadFailureThreshold
		*out = new(intstr.IntOrString)
		**out = **in
	}
	if in.TaskFailureThreshold != nil {
		in, out := &in.TaskFailureThreshold, &out.TaskFailureThreshold
		*out = new(intstr.IntOrString)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new TolerationStrategy.
func (in *TolerationStrategy) DeepCopy() *TolerationStrategy {
	if in == nil {
		return nil
	}
	out := new(TolerationStrategy)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *TrafficStrategy) DeepCopyInto(out *TrafficStrategy) {
	*out = *in
	if in.Weight != nil {
		in, out := &in.Weight, &out.Weight
		*out = new(int32)
		**out = **in
	}
	if in.HTTPStrategy != nil {
		in, out := &in.HTTPStrategy, &out.HTTPStrategy
		*out = new(HTTPTrafficStrategy)
		(*in).DeepCopyInto(*out)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new TrafficStrategy.
func (in *TrafficStrategy) DeepCopy() *TrafficStrategy {
	if in == nil {
		return nil
	}
	out := new(TrafficStrategy)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *WebhookClientConfig) DeepCopyInto(out *WebhookClientConfig) {
	*out = *in
	if in.CABundle != nil {
		in, out := &in.CABundle, &out.CABundle
		*out = make([]byte, len(*in))
		copy(*out, *in)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new WebhookClientConfig.
func (in *WebhookClientConfig) DeepCopy() *WebhookClientConfig {
	if in == nil {
		return nil
	}
	out := new(WebhookClientConfig)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *WorkloadRef) DeepCopyInto(out *WorkloadRef) {
	*out = *in
	in.Match.DeepCopyInto(&out.Match)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new WorkloadRef.
func (in *WorkloadRef) DeepCopy() *WorkloadRef {
	if in == nil {
		return nil
	}
	out := new(WorkloadRef)
	in.DeepCopyInto(out)
	return out
}

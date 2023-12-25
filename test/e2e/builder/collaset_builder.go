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

package builder

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	operatingv1alpha1 "kusionstack.io/kube-api/apps/v1alpha1"

	"kusionstack.io/kube-utils/multicluster/clusterinfo"
)

type CollsetBuilder struct {
	builder
}

func NewCollsetBuilder() *CollsetBuilder {
	return &CollsetBuilder{}
}

func (b *CollsetBuilder) Namespace(namespace string) *CollsetBuilder {
	b.namespace = namespace
	return b
}

func (b *CollsetBuilder) Name(name string) *CollsetBuilder {
	b.name = name
	return b
}

func (b *CollsetBuilder) NamePrefix(namePrefix string) *CollsetBuilder {
	b.namePrefix = namePrefix
	return b
}

func (b *CollsetBuilder) AppName(appName string) *CollsetBuilder {
	b.appName = appName
	return b
}

func (b *CollsetBuilder) Cluster(cluster string) *CollsetBuilder {
	b.cluster = cluster
	return b
}

func (b *CollsetBuilder) Build() *operatingv1alpha1.CollaSet {
	b.complete()

	return &operatingv1alpha1.CollaSet{
		TypeMeta: metav1.TypeMeta{
			Kind:       "CollaSet",
			APIVersion: operatingv1alpha1.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      b.name,
			Namespace: b.namespace,
			Annotations: map[string]string{
				"app.kubernetes.io/name": b.appName,
			},
			Labels: map[string]string{
				clusterinfo.ClusterLabelKey:  b.cluster,
				"app.kubernetes.io/name":     b.appName,
				"app.kubernetes.io/instance": b.appName,
			},
		},
		Spec: operatingv1alpha1.CollaSetSpec{
			Replicas: ptr.To(int32(5)),
			ScaleStrategy: operatingv1alpha1.ScaleStrategy{
				Context: "foo",
			},
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app.kubernetes.io/name": b.appName,
				},
			},
			UpdateStrategy: operatingv1alpha1.UpdateStrategy{
				RollingUpdate: &operatingv1alpha1.RollingUpdateCollaSetStrategy{
					ByPartition: &operatingv1alpha1.ByPartition{
						Partition: ptr.To(int32(0)),
					},
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app.kubernetes.io/name": b.appName,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "nginx",
							Image: "nginx:1.16.1",
						},
					},
				},
			},
		},
	}
}

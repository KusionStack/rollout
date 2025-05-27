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
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"kusionstack.io/kube-utils/multicluster/clusterinfo"
)

type StatefulSetBuilder struct {
	builder
}

func NewStatefulSet() *StatefulSetBuilder {
	return &StatefulSetBuilder{}
}

func (b *StatefulSetBuilder) Namespace(namespace string) *StatefulSetBuilder {
	b.namespace = namespace
	return b
}

func (b *StatefulSetBuilder) Name(name string) *StatefulSetBuilder {
	b.name = name
	return b
}

func (b *StatefulSetBuilder) NamePrefix(namePrefix string) *StatefulSetBuilder {
	b.namePrefix = namePrefix
	return b
}

func (b *StatefulSetBuilder) AppName(appName string) *StatefulSetBuilder {
	b.appName = appName
	return b
}

func (b *StatefulSetBuilder) Cluster(cluster string) *StatefulSetBuilder {
	b.cluster = cluster
	return b
}

func (b *StatefulSetBuilder) Build() *appsv1.StatefulSet {
	b.complete()

	return &appsv1.StatefulSet{
		TypeMeta: metav1.TypeMeta{
			Kind:       "StatefulSet",
			APIVersion: appsv1.SchemeGroupVersion.String(),
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
		Spec: appsv1.StatefulSetSpec{
			ServiceName: b.appName,
			Replicas:    ptr.To(int32(5)),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app.kubernetes.io/name": b.appName,
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
			UpdateStrategy: appsv1.StatefulSetUpdateStrategy{
				Type:          appsv1.RollingUpdateStatefulSetStrategyType,
				RollingUpdate: &appsv1.RollingUpdateStatefulSetStrategy{Partition: ptr.To(int32(0))},
			},
		},
	}
}

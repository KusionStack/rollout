package swarm

import (
	"bytes"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	kusionstackappsv1alpha1 "kusionstack.io/kube-api/apps/v1alpha1"
)

func Test_convert(t *testing.T) {
	obj := &kusionstackappsv1alpha1.Swarm{
		Spec: kusionstackappsv1alpha1.SwarmSpec{
			PodGroups: []kusionstackappsv1alpha1.SwarmPodGroupSpec{
				{
					Name:     "0",
					Replicas: ptr.To[int32](1),
					Labels: map[string]string{
						"role": "0",
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							CreationTimestamp: metav1.Now(),
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "test",
									Image: "nginx:v1",
								},
							},
						},
					},
				},
				{
					Name:     "1",
					Replicas: ptr.To[int32](2),
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							CreationTimestamp: metav1.Now(),
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "test",
									Image: "nginx:v1",
								},
							},
						},
					},
				},
			},
		},
	}
	result, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	assert.NoError(t, err)
	spew.Dump(result)
}

func Test_getSwarmPatch(t *testing.T) {
	swarm := &kusionstackappsv1alpha1.Swarm{
		Spec: kusionstackappsv1alpha1.SwarmSpec{
			PodGroups: []kusionstackappsv1alpha1.SwarmPodGroupSpec{
				{
					Name:     "0",
					Replicas: ptr.To[int32](1),
					Labels: map[string]string{
						"role": "0",
					},
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "test",
									Image: "nginx:v1",
								},
							},
						},
					},
				},
				{
					Name:     "1",
					Replicas: ptr.To[int32](2),
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "test",
									Image: "nginx:v1",
								},
							},
						},
					},
				},
			},
		},
	}

	data, err := getSwarmPatch(swarm)
	assert.NoError(t, err)
	trimed := bytes.TrimSpace(data)
	assert.EqualValues(t, trimed, data, "patch data should not contains any leading or trailing white space")
}

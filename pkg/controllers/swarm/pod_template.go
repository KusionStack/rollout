package swarm

import (
	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
)

type TemplateModifier interface {
	MergeLabels(labels map[string]string)
	AddEnvs(envs ...corev1.EnvVar)
}

func NewTemplateModifier(template *corev1.PodTemplateSpec) TemplateModifier {
	return &templateModifier{template: template}
}

type templateModifier struct {
	template *corev1.PodTemplateSpec
}

func (t *templateModifier) MergeLabels(labels map[string]string) {
	t.template.Labels = lo.Assign(t.template.Labels, labels)
}

func (t *templateModifier) AddEnvs(envs ...corev1.EnvVar) {
	for _, env := range envs {
		for i := range t.template.Spec.InitContainers {
			createOrUpdateEnv(&t.template.Spec.InitContainers[i], env)
		}
		for i := range t.template.Spec.Containers {
			createOrUpdateEnv(&t.template.Spec.Containers[i], env)
		}
	}
}

func createOrUpdateEnv(container *corev1.Container, env corev1.EnvVar) {
	for i, envVar := range container.Env {
		if envVar.Name == env.Name {
			container.Env[i] = env
			return
		}
	}
	container.Env = append(container.Env, env)
}

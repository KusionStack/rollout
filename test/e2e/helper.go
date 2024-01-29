package e2e

import (
	corev1 "k8s.io/api/core/v1"
)

func mergeEnvVar(original []corev1.EnvVar, add corev1.EnvVar) []corev1.EnvVar {
	newEnvs := make([]corev1.EnvVar, 0)
	for _, env := range original {
		if add.Name == env.Name {
			continue
		}
		newEnvs = append(newEnvs, env)
	}
	newEnvs = append(newEnvs, add)
	return newEnvs
}

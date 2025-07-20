package backend

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func ForkObjectMeta(in client.Object, newName string) metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:        newName,
		Namespace:   in.GetNamespace(),
		Labels:      in.GetLabels(),
		Annotations: in.GetAnnotations(),
	}
}

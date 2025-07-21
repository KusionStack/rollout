package inferencepool

import (
	rolloutapi "kusionstack.io/kube-api/rollout"
	rolloutv1alpha1 "kusionstack.io/kube-api/rollout/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwapiv1alpha2 "sigs.k8s.io/gateway-api-inference-extension/api/v1alpha2"

	"kusionstack.io/rollout/pkg/trafficrouting/backend"
	"kusionstack.io/rollout/pkg/utils/accessor"
)

var GVK = gwapiv1alpha2.SchemeGroupVersion.WithKind("InferencePool")

var _ backend.InClusterBackend = &accessorImpl{}

type accessorImpl struct {
	accessor.ObjectAccessor
}

func New() backend.InClusterBackend {
	return &accessorImpl{
		ObjectAccessor: accessor.NewObjectAccessor(GVK, &gwapiv1alpha2.InferencePool{}, &gwapiv1alpha2.InferencePoolList{}),
	}
}

func (b *accessorImpl) Fork(origin client.Object, config rolloutv1alpha1.ForkedBackend) client.Object {
	obj := origin.(*gwapiv1alpha2.InferencePool)

	forkedObj := &gwapiv1alpha2.InferencePool{}
	// fork metadata
	forkedObj.ObjectMeta = backend.ForkObjectMeta(obj, config.Name)
	if forkedObj.Labels == nil {
		forkedObj.Labels = make(map[string]string)
	}
	forkedObj.Labels[rolloutapi.LabelTemporaryResource] = "true"
	// fork spec
	forkedObj.Spec = *obj.Spec.DeepCopy()
	// change spec
	if forkedObj.Spec.Selector == nil {
		forkedObj.Spec.Selector = make(map[gwapiv1alpha2.LabelKey]gwapiv1alpha2.LabelValue)
	}
	for k, v := range config.ExtraLabelSelector {
		forkedObj.Spec.Selector[gwapiv1alpha2.LabelKey(k)] = gwapiv1alpha2.LabelValue(v)
		forkedObj.Labels[k] = v
	}
	return forkedObj
}

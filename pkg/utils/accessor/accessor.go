package accessor

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ObjectAccessor interface {
	// GroupVersionKind returns the GroupVersionKind of the workload
	GroupVersionKind() schema.GroupVersionKind
	// NewObject returns a new instance of the workload type
	NewObject() client.Object
	// NewObjectList returns a new instance of the workload list type
	NewObjectList() client.ObjectList
}

type genericAccessorImpl struct {
	gvk     schema.GroupVersionKind
	obj     client.Object
	listObj client.ObjectList
}

func NewObjectAccessor(gvk schema.GroupVersionKind, obj client.Object, listObj client.ObjectList) ObjectAccessor {
	return &genericAccessorImpl{
		gvk:     gvk,
		obj:     obj,
		listObj: listObj,
	}
}

func (g *genericAccessorImpl) GroupVersionKind() schema.GroupVersionKind {
	return g.gvk
}

func (g *genericAccessorImpl) NewObject() client.Object {
	return g.obj.DeepCopyObject().(client.Object)
}

func (g *genericAccessorImpl) NewObjectList() client.ObjectList {
	return g.listObj.DeepCopyObject().(client.ObjectList)
}

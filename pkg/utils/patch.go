package utils

import (
	"fmt"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func DeleteAnnotation(patchType types.PatchType, key string) client.Patch {
	return client.RawPatch(patchType, []byte(fmt.Sprintf(
		`[{"op": "remove", "path": "/metadata/annotations/%s"}]`, Escape(key),
	)))
}

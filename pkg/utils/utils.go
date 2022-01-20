package utils

import (
	"fmt"

	k8t "github.com/xenolog/k8s-utils/pkg/types"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/cache"
)

// KeyToNamespacedName convert key ("namespace/name" string) to types.NamespacedName
func KeyToNamespacedName(ref string) (rv types.NamespacedName) {
	namespace, name, err := cache.SplitMetaNamespaceKey(ref)
	if err != nil {
		return types.NamespacedName{}
	}
	return types.NamespacedName{
		Namespace: namespace,
		Name:      name,
	}
}

func GetRuntimeObjectNamespacedName(rtobj runtime.Object) (types.NamespacedName, error) {
	obj, ok := rtobj.(k8t.K8sObject)
	if !ok {
		return types.NamespacedName{}, fmt.Errorf("GetRuntimeObjectNamespacedName: %w: given object is not a k8s object", k8t.ErrorWrongParametr)
	}
	return types.NamespacedName{
		Namespace: obj.GetNamespace(),
		Name:      obj.GetName(),
	}, nil
}

func GetRuntimeObjectKey(rtobj runtime.Object) (rv string, err error) {
	nn, err := GetRuntimeObjectNamespacedName(rtobj)
	if err != nil {
		return "", err
	}
	if nn.Namespace == "" {
		rv = nn.Name
	} else {
		rv = nn.String()
	}
	return rv, err
}

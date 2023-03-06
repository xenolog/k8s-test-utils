package utils

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"

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

// IdentTextBlock -- returns indented multiline text block
//
// by default the extra `\n` will be added before block
// if negative `indent` given the `\n` at the start will not be added
func IdentTextBlock(indent int, miltilineTextBlock string) string {
	searchRE := regexp.MustCompile(`^(\s+)`)
	iLen := -1
	rv := strings.Builder{}

	if indent < 0 {
		indent *= -1
	} else {
		if _, err := rv.WriteString("\n"); err != nil {
			panic(err)
		}
	}

	// calculate minimal length of space prefix
	rr := bufio.NewReader(bytes.NewBufferString(miltilineTextBlock))
idtExLoop:
	for {
		line, _, err := rr.ReadLine()
		switch {
		case err == io.EOF:
			break idtExLoop
		case err != nil:
			panic(err)
		default:
			if len(line) > 0 {
				pref := searchRE.FindString(string(line))
				l := len(pref)
				if (iLen == -1) || (iLen > 0 && l < iLen) {
					iLen = l
				}
			}
		}
	}

	idt := ""
	for i := 0; i < indent; i++ {
		idt += " "
	}

	// build response string
	rr = bufio.NewReader(bytes.NewBufferString(miltilineTextBlock))
exLoop:
	for {
		line, _, err := rr.ReadLine()
		switch {
		case err == io.EOF:
			break exLoop
		case err != nil:
			panic(err)
		default:
			if len(line) > 0 {
				if _, err := rv.WriteString(idt); err != nil {
					panic(err)
				}
				if _, err := rv.Write(line[iLen:]); err != nil {
					panic(err)
				}
				if _, err := rv.WriteString("\n"); err != nil {
					panic(err)
				}
			}
		}
	}
	return strings.TrimSuffix(rv.String(), "\n")
}

func RecursiveUnwrap(err error) (rv error) {
	tmp := err
	for tmp != nil {
		rv = tmp
		tmp = errors.Unwrap(tmp)
	}
	return rv
}

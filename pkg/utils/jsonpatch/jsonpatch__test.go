package jsonpatch_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/xenolog/k8s-utils/pkg/utils/jsonpatch"
)

func Test__Q(t *testing.T) {
	tt := assert.New(t)

	tt.EqualValues("aaa~1bbb", jsonpatch.Q("aaa/bbb"))
	tt.EqualValues("aaa~0bbb", jsonpatch.Q("aaa~bbb"))
	tt.EqualValues("aaa~0bbb~1ccc~02ddd~03eee~0afff~0.ggg", jsonpatch.Q("aaa~0bbb~1ccc~2ddd~3eee~afff~.ggg"))
}

func Test__CreatePatchForJson(t *testing.T) {
	tt := assert.New(t)

	a := []byte(`{"aaa":1,"bbb":"b","ccc":[1,2,3],"ddd":{"a1":1,"a2":2},"eee":{"e1":1,"e2":"e2","e3":[1,2,3]}}`)
	b := []byte(`{"aaa":2,"bbb":"bb","ccc":[1,2,33],"ddd":{"a1":1,"a2":22},"eee":{"e1":1,"e2":"e2","e3":[1,2,33]}}`)
	diff, err := jsonpatch.CreatePatchForJson(a, b)
	tt.NoError(err)
	tt.EqualValues(
		`[{"op":"replace","path":"/aaa","value":2},{"op":"replace","path":"/bbb","value":"bb"},{"op":"replace","path":"/ccc/2","value":33},{"op":"replace","path":"/ddd/a2","value":22},{"op":"replace","path":"/eee/e3/2","value":33}]`,
		diff.String(),
	)

	_, err = jsonpatch.CreatePatchForJson(a, []byte("xxx"))
	tt.ErrorIs(err, jsonpatch.ErrBadJSONDoc)
	_, err = jsonpatch.CreatePatchForJson([]byte("xxx"), b)
	tt.ErrorIs(err, jsonpatch.ErrBadJSONDoc)
}

func Test__CreatePatch(t *testing.T) {
	tt := assert.New(t)

	a := struct {
		A int
		B int64
		C string
	}{1, 9223372036854775800, "qqq"} //revive:disable:add-constant
	b := struct {
		A int
		B int64
		D string
	}{1, -9223372036854775800, "xxx"} //revive:disable:add-constant

	diff, err := jsonpatch.CreatePatch(&a, &b)
	tt.NoError(err)
	tt.EqualValues(
		`[{"op":"replace","path":"/B","value":-9223372036854776000},{"op":"remove","path":"/C"},{"op":"add","path":"/D","value":"xxx"}]`,
		diff.String(),
	)
}

package jsonpatch

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	jsonPatchConstructor "github.com/mattbaird/jsonpatch"
)

// ----------------------------------------------------------------------------
type Operation = jsonPatchConstructor.JsonPatchOperation

type Patch []Operation // todo(SV): should not contains duplicate paths

var NewOperation = jsonPatchConstructor.NewPatch //nolint:gochecknoglobals

// Len for https://godoc.org/sort#Interface
func (r Patch) Len() int {
	return len(r)
}

// Less for https://godoc.org/sort#Interface
func (r Patch) Less(i, j int) bool {
	return r[i].Path < r[j].Path
}

// Swap for https://godoc.org/sort#Interface
func (r Patch) Swap(i, j int) {
	r[i], r[j] = r[j], r[i]
}

func (r Patch) Sort() Patch {
	sort.Sort(r)
	return r
}

func (r Patch) Json() string {
	accum := make([]string, 0, len(r))
	for i := range r.Sort() {
		accum = append(accum, r[i].Json())
	}
	return "[" + strings.Join(accum, ",") + "]"
}

func (r Patch) String() string {
	return r.Json()
}

func (r Patch) Bytes() []byte {
	return []byte(r.Json())
}

func CreatePatchForJson(a, b []byte) (Patch, error) {
	rv, err := jsonPatchConstructor.CreatePatch(a, b)
	if err != nil {
		err = ErrBadJSONDoc
	}
	return rv, err
}

func CreatePatch(a, b any) (Patch, error) {
	var (
		aj, bj []byte
		err    error
	)
	if aj, err = json.Marshal(a); err != nil {
		return Patch{}, fmt.Errorf("%w: %w", ErrMarshalJSON, err)
	}
	if bj, err = json.Marshal(b); err != nil {
		return Patch{}, fmt.Errorf("%w: %w", ErrMarshalJSON, err)
	}
	return CreatePatchForJson(aj, bj)
}

// do not touch already escaped '/' and '~', i.e. '~0' and '~1'
var rfc6901re0 = regexp.MustCompile(`(~([^01]))`)

// Escaping some characters, corresponds to https://www.rfc-editor.org/rfc/rfc6901#section-3
func Q(s string) string {
	s = rfc6901re0.ReplaceAllString(s, `~0${2}`)
	s = strings.ReplaceAll(s, "/", `~1`)
	return s
}

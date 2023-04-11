/*
Copyright © 2020 Mirantis

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

   http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package jsonpatch

import (
	"sort"
	"strings"

	jsonpatch "gomodules.xyz/jsonpatch/v2"
)

// ----------------------------------------------------------------------------
type Operation = jsonpatch.Operation

type Patch []Operation

var NewOperation = jsonpatch.NewOperation //nolint:gochecknoglobals

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

func (r Patch) String() string {
	accum := make([]string, 0, len(r))
	for i := range r.Sort() {
		accum = append(accum, r[i].Json())
	}
	return "[" + strings.Join(accum, ",") + "]"
}

func (r Patch) Bytes() []byte {
	return []byte(r.String())
}

// Escaping some characters, corresponds to https://www.rfc-editor.org/rfc/rfc6901#section-3
func Q(s string) string {
	s = strings.ReplaceAll(s, "~", `~0`)
	s = strings.ReplaceAll(s, "/", `~1`)
	return s
}

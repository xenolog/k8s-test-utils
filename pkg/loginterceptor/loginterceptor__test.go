package loginterceptor_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/xenolog/k8s-utils/pkg/loginterceptor"
)

func TestStringNoDuplicateLines(t *testing.T) {
	tt := assert.New(t)
	//nolint: dupword
	inBuff := bytes.NewBufferString(`
aaaa
aaaa
aaaa
bb
ccc
fffff
fffff
dddd
`)
	tt.Equal([]string{
		"aaaa",
		"bb",
		"ccc",
		"fffff",
		"dddd",
		"",
	}, loginterceptor.StringNoDuplicateLines(inBuff))
}

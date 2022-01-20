package kloginterceptor

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStringNoDuplicateLines(t *testing.T) {
	tt := assert.New(t)
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
	}, StringNoDuplicateLines(inBuff))
}

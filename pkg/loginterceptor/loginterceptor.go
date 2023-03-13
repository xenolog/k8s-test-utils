package loginterceptor

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/k0kubun/pp"
)

func PublishOutputIfFailed(t *testing.T) func() {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		panic(err)
	}
	sourceStdOut := os.Stdout
	sourceStdErr := os.Stderr
	os.Stdout = w
	os.Stderr = w
	out := make(chan string, 1)

	go func() {
		buf := bytes.NewBuffer(nil)
		if _, err := io.Copy(buf, r); err != nil {
			panic(err)
		}
		out <- buf.String()
	}()

	return func() {
		if err := w.Close(); err != nil && !errors.Is(err, os.ErrClosed) {
			panic(err)
		}
		os.Stdout = sourceStdOut
		os.Stderr = sourceStdErr
		if t.Failed() {
			// t.Logf("Operator log:\n%s\n", strings.Join(StringNoDuplicateLines(&logBuff), "\n"))
			if _, err := fmt.Fprintf(os.Stdout, "Operator log:\n%s\n", <-out); err != nil {
				panic(err)
			}
		}
	}
}

func PublishObjDumpIfFailed(memo string, t *testing.T, obj any) func() {
	return func() {
		if t.Failed() {
			t.Logf("k8s object %s:\n%s\n", memo, pp.Sprintln(obj))
		}
	}
}

func StringNoDuplicateLines(buff *bytes.Buffer) (rv []string) {
	prev := ""
	for {
		line, err := buff.ReadString('\n')
		if prev != "" && prev != line {
			rv = append(rv, strings.TrimRight(line, "\n"))
		}
		if err != nil {
			break
		}
		prev = line
	}
	return rv
}

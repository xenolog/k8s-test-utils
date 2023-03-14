package loginterceptor

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/k0kubun/pp"
	klogv1 "k8s.io/klog"
	klogv2 "k8s.io/klog/v2"
)

//nolint:gochecknoglobals
var (
	kLogOutputBuffStack      []*bytes.Buffer
	klogv1Flags, klogv2Flags *flag.FlagSet
)

//nolint:gochecknoinits
func init() {
	kLogOutputBuffStack = []*bytes.Buffer{}
}

// stderr/stdout interceptor, MUST be called as `defer loginterceptor.PublishOutputIfFailed(t)()`
func PublishOutputIfFailed(t *testing.T) func() {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		panic(err)
	}
	// sourceStdOut := os.Stdout
	sourceStdErr := os.Stderr
	// os.Stdout = w
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
		// os.Stdout = sourceStdOut
		os.Stderr = sourceStdErr
		if t.Failed() {
			if _, err := fmt.Fprintf(os.Stdout, "Operator log:\n%s\n", <-out); err != nil {
				panic(err)
			}
		}
	}
}

func InterceptKlogOutput() (logBuff *bytes.Buffer, cancel func()) {
	if len(kLogOutputBuffStack) == 0 {
		// setup ability of redirect should be once
		klogv2Flags = flag.NewFlagSet("", flag.PanicOnError)
		klogv2.CopyStandardLogTo("INFO")
		klogv2.InitFlags(klogv2Flags) // start a trick to permit use .SetOutput(...) in the k8s.io/klog
		if err := klogv2Flags.Set("logtostderr", "false"); err != nil {
			panic(err)
		}
		if err := klogv2Flags.Set("alsologtostderr", "false"); err != nil {
			panic(err)
		}
		if err := klogv2Flags.Set("stderrthreshold", "4"); err != nil {
			panic(err)
		}
		flag.Parse() // it was official trick :)

		klogv1Flags = flag.NewFlagSet("", flag.PanicOnError)
		klogv1.InitFlags(klogv1Flags)
		if err := klogv1Flags.Set("logtostderr", "false"); err != nil { // By default klog v1 logs to stderr, switch that off
			panic(err)
		}
		if err := klogv1Flags.Set("stderrthreshold", "FATAL"); err != nil {
			panic(err)
		}
		if err := klogv1Flags.Set("stderrthreshold", "4"); err != nil {
			panic(err)
		}
	}
	logBuff = &bytes.Buffer{}
	kLogOutputBuffStack = append(kLogOutputBuffStack, logBuff)

	klogv2.SetOutput(logBuff)
	klogv1.SetOutput(logBuff)

	cancel = func() {
		klogv1.Flush()
		klogv2.Flush()

		if len(kLogOutputBuffStack) == 0 {
			panic("Parent log buffer not registered, may be .cancel() called twice.")
		}
		kLogOutputBuffStack = kLogOutputBuffStack[:len(kLogOutputBuffStack)-1] // remove self

		if len(kLogOutputBuffStack) == 0 {
			// restore non-intersepted logging
			if err := klogv2Flags.Set("logtostderr", "true"); err != nil {
				panic(err)
			}
			flag.Parse()
			klogv1.SetOutput(nil)
			klogv2.SetOutput(nil)
		} else {
			// set output to buff of parent test
			prevLogBuff := kLogOutputBuffStack[len(kLogOutputBuffStack)-1]
			klogv1.SetOutput(prevLogBuff)
			klogv2.SetOutput(prevLogBuff)
		}
	}
	return logBuff, cancel
}

// klog output interceptor, MUST be called as `defer loginterceptor.PublishKlogOutputIfFailed(t)()`
func PublishKlogOutputIfFailed(t *testing.T) func() {
	logBuff, cancel := InterceptKlogOutput()

	return func() {
		cancel()
		if t.Failed() {
			t.Logf("Operator log:\n%s\n", strings.Join(StringNoDuplicateLines(logBuff), "\n"))
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

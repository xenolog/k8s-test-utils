package kloginterceptor

import (
	"bytes"
	"flag"
	"strings"
	"testing"

	"github.com/k0kubun/pp"
	klogv1 "k8s.io/klog"
	klogv2 "k8s.io/klog/v2"
)

func PublishKlogOutputIfFailed(t *testing.T) func() {
	klogv2.InitFlags(nil)

	flagSet := flag.NewFlagSet("", flag.PanicOnError)
	klogv1.CopyStandardLogTo("INFO")
	klogv1.InitFlags(flagSet) // start a trick to permit use .SetOutput(...) in the k8s.io/klog
	if err := flagSet.Set("logtostderr", "false"); err != nil {
		panic(err)
	}
	if err := flagSet.Set("alsologtostderr", "false"); err != nil {
		panic(err)
	}
	if err := flagSet.Set("stderrthreshold", "4"); err != nil {
		panic(err)
	}
	flag.Parse() // it was official trick :)
	logBuff := bytes.Buffer{}
	klogv1.SetOutput(&logBuff)
	klogv2.SetOutput(&logBuff)

	return func() {
		klogv1.Flush()
		klogv2.Flush()
		if t.Failed() {
			t.Logf("Operator log:\n%s\n", strings.Join(StringNoDuplicateLines(&logBuff), "\n"))
		}
		// restore non-intersepted logging
		if err := flagSet.Set("logtostderr", "true"); err != nil {
			panic(err)
		}
		flag.Parse()
		klogv1.SetOutput(nil)
		klogv2.SetOutput(nil)
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

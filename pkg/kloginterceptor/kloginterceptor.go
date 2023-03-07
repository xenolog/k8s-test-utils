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
	// klogv2.InitFlags(nil)

	klogv2Flags := flag.NewFlagSet("", flag.PanicOnError)
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
	logBuff := bytes.Buffer{}
	klogv2.SetOutput(&logBuff)

	klogv1Flags := flag.FlagSet{}
	klogv1.InitFlags(&klogv1Flags)
	if err := klogv1Flags.Set("logtostderr", "false"); err != nil { // By default klog v1 logs to stderr, switch that off
		panic(err)
	}
	if err := klogv1Flags.Set("stderrthreshold", "FATAL"); err != nil { // stderrthreshold defaults to ERROR, use this if you
		panic(err)
	}
	if err := klogv1Flags.Set("stderrthreshold", "4"); err != nil {
		panic(err)
	}
	klogv1.SetOutput(&logBuff)

	return func() {
		klogv1.Flush()
		klogv2.Flush()
		if t.Failed() {
			t.Logf("Operator log:\n%s\n", strings.Join(StringNoDuplicateLines(&logBuff), "\n"))
		}
		// restore non-intersepted logging
		if err := klogv2Flags.Set("logtostderr", "true"); err != nil {
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

package kloginterceptor

import (
	"bytes"
	"flag"
	"strings"
	"testing"

	"github.com/k0kubun/pp"
	"k8s.io/klog"
)

func PublishKlogOutputIfFailed(t *testing.T) func() {
	flagSet := flag.NewFlagSet("", flag.PanicOnError)
	klog.CopyStandardLogTo("INFO")
	klog.InitFlags(flagSet) // start a trick to permit use .SetOutput(...) in the k8s.io/klog
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
	klog.SetOutput(&logBuff)

	return func() {
		klog.Flush()
		if t.Failed() {
			t.Logf("Operator log:\n%s\n", strings.Join(StringNoDuplicateLines(&logBuff), "\n"))
		}
		// restore non-intersepted logging
		if err := flagSet.Set("logtostderr", "true"); err != nil {
			panic(err)
		}
		flag.Parse()
		klog.SetOutput(nil)
	}
}

func PublishObjDumpIfFailed(memo string, t *testing.T, obj interface{}) func() {
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

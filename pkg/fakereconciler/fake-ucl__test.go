package fakereconciler_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/xenolog/k8s-utils/pkg/fakereconciler"
	"k8s.io/apimachinery/pkg/runtime"
)

func Test__GetPauseTime(t *testing.T) {
	tt := assert.New(t)
	sch := runtime.NewScheme()
	q := fakereconciler.NewFakeReconciler(nil, sch)
	tt.LessOrEqual(q.GetPauseTime(), time.Second)
}

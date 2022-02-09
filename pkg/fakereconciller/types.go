package fakereconciller

import (
	"context"
	"time"

	k8t "github.com/xenolog/k8s-utils/pkg/types"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type FakeReconciller interface {

	// Run main loop to watch create/delete/reconcile requests.
	// Context will be stored to use as fallback
	Run(ctx context.Context)

	// todo(sv): will be better to implement in the future
	// RunAndDeferWaitToFinish -- run fakeReconciller loop and defer
	// Wait(...) function with infinity time to wait.
	// may be used as `defer rcl.RunAndDeferWaitToFinish(ctx)()` call
	// RunAndDeferWaitToFinish(context.Context) func()

	// Wait -- wait to finish all running fake reconcile loops
	// and user requested create/reconcile calls. Like sync.Wait()
	// context, passed to Run(...) will be used to cancel all waiters.
	Wait()

	// Reconcile -- invoke to reconcile the corresponded resource.
	// Returns chan which can be used to obtain reconcile response and timings
	Reconcile(kindName, key string) (chan *ReconcileResponce, error)

	// LockReconciller -- lock watchers/reconcillers for the specifyed Kind type.
	// returns callable to Unock thread
	LockReconciller(kindName string) func()

	// WaitToBeCreated -- block gorutine while corresponded CRD will be created.
	// If isReconcilled if false just reconciliation record (fact) will be probed,
	// else (if true) -- reconcilated result (status exists) will be waited.
	// If ctx == nil -- context, stored in the Run(...) will be used
	WaitToBeCreated(ctx context.Context, kindName, key string, isReconcilled bool) error

	// WatchToBeCreated -- run gorutine to wait while corresponded CRD will be created.
	// If isReconcilled if false just reconciliation record (fact) will be probed,
	// else (if true) -- reconcilated result (status exists) will be waited.
	// Does not block current gorutine,  error chan returned to obtain result if need
	// If ctx == nil -- context, stored in the Run(...) will be used
	WatchToBeCreated(ctx context.Context, kindName, key string, isReconcilled bool) (chan error, error)

	// WaitToBeReconciled -- block gorutine while corresponded CRD will be reconciled.
	// if reconciledAfter if zero just reconciliation record (fact) will be probed,
	// else (if real time passed) only fresh reconciliation (after given time) will be accounted
	// If ctx == nil -- context, stored in the Run(...) will be used
	WaitToBeReconciled(ctx context.Context, kindName, key string, reconciledAfter time.Time) error

	// WatchToBeReconciled -- run gorutine to wait while corresponded CRD will be reconciled.
	// if reconciledAfter if zero just reconciliation record (fact) will be probed,
	// else (if real time passed) only fresh reconciliation (after given time) will be accounted
	// Does not block current gorutine,  error chan returned to obtain result if need
	// If ctx == nil -- context, stored in the Run(...) will be used
	WatchToBeReconciled(ctx context.Context, kindName, key string, reconciledAfter time.Time) (chan error, error)

	// AddController -- add reconciller to the monitor loop
	AddController(gvk *schema.GroupVersionKind, rcl NativeReconciller) error

	GetClient() client.WithWatch
	GetScheme() *runtime.Scheme
}

type ReconcileResponce struct {
	Err             error
	Result          reconcile.Result
	StartFinishTime k8t.TimeInterval
}

// NativeReconciller -- any k8s operator reconcilable type
type NativeReconciller interface {
	Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error)
}

// ----------------------------------------------------------------------------

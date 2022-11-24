package fakereconciler

import (
	"context"
	"time"

	k8t "github.com/xenolog/k8s-utils/pkg/types"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type FakeReconciler interface {

	// Run main loop to watch create/delete/reconcile requests.
	// Context will be stored to future use
	Run(ctx context.Context)

	// todo(sv): will be better to implement in the future
	// RunAndDeferWaitToFinish -- run fakeReconciler loop and defer
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

	// LockReconciler -- lock watchers/reconcilers for the specifyed Kind type.
	// returns callable to Unock thread
	LockReconciler(kindName string) (unlock func())

	// WaitToBeCreated -- block gorutine while corresponded CRD will be created.
	// If isReconciled is false just reconciliation record (fact) will be probed,
	// else (if true) -- reconciled result (status exists and not empty) will be waited.
	// Pass nil instead context, to use fakeReconciler .Run(ctx) context
	WaitToBeCreated(ctx context.Context, kindName, key string, isReconciled bool) error

	// WatchToBeCreated -- run gorutine to wait while corresponded CRD will be created.
	// If isReconciled is false just reconciliation record (fact) will be probed,
	// else (if true) -- reconciled result (status exists and not empty) will be watched.
	// Does not block current gorutine,  error chan returned to obtain result if need
	// Pass nil instead context, to use fakeReconciler .Run(ctx) context
	WatchToBeCreated(ctx context.Context, kindName, key string, isReconciled bool) (chan error, error)

	// WaitToBeReconciled -- block gorutine while corresponded CRD will be reconciled.
	// if reconciledAfter if zero just reconciliation record (fact) will be probed,
	// else (if real time passed) only fresh reconciliation (after given time) will be accounted
	// Pass nil instead context, to use fakeReconciler .Run(ctx) context
	WaitToBeReconciled(ctx context.Context, kindName, key string, reconciledAfter time.Time) error

	// WatchToBeReconciled -- run gorutine to wait while corresponded CRD will be reconciled.
	// if reconciledAfter if zero just reconciliation record (fact) will be probed,
	// else (if real time passed) only fresh reconciliation (after given time) will be accounted
	// Does not block current gorutine,  error chan returned to obtain result if need
	// Pass nil instead context, to use fakeReconciler .Run(ctx) context
	WatchToBeReconciled(ctx context.Context, kindName, key string, reconciledAfter time.Time) (chan error, error)

	// WaitToFieldSatisfyRE -- block gorutine while corresponded CRD field will be satisfy to the given regexp.
	// The dot '.' is a separator in the fieldPath
	// Pass nil instead context, to use fakeReconciler .Run(ctx) context
	WaitToFieldSatisfyRE(ctx context.Context, kind, key, fieldPath, reString string) error

	// WatchToFieldSatisfyRE -- run gorutine to wait while corresponded CRD field will be satisfy to the given regexp.
	// Pass nil instead context, to use stored early
	WatchToFieldSatisfyRE(ctx context.Context, kind, key, fieldPath, reString string) (chan error, error)

	// WaitToFieldBeChecked -- block gorutine while corresponded CRD field will be exists and checked by callback function.
	// The dot '.' is a separator in the fieldPath
	// Pass nil instead context, to use fakeReconciler .Run(ctx) context
	WaitToFieldBeChecked(ctx context.Context, kind, key, fieldPath string, callbackFunc func(any) bool) error

	// WatchToFieldBeChecked -- run gorutine to wait while corresponded CRD field will be exists and checked by callback function.
	// Pass nil instead context, to use fakeReconciler .Run(ctx) context
	WatchToFieldBeChecked(ctx context.Context, kind, key, fieldPath string, callbackFunc func(any) bool) (chan error, error)

	// AddController -- add reconciler to the monitor loop while setup (before .Run(...) call)
	AddController(gvk *schema.GroupVersionKind, rcl reconcile.Reconciler) error
	AddControllerByType(m schema.ObjectKind, rcl reconcile.Reconciler) error

	GetClient() client.WithWatch
	GetScheme() *runtime.Scheme
}

type ReconcileResponce struct {
	Err             error
	Result          reconcile.Result
	StartFinishTime k8t.TimeInterval
}

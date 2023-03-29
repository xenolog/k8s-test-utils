package fakereconciler

import (
	"context"
	"time"

	k8t "github.com/xenolog/k8s-utils/pkg/types"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	controllerRTclient "sigs.k8s.io/controller-runtime/pkg/client"
	controllerRTreconcile "sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type FakeReconciler interface {
	// Run main loop to watch create/delete/reconcile requests.
	// Context will be stored to future use
	Run(ctx context.Context) error

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
	//
	// If isReconciled is false just reconciliation record (fact) will be probed,
	// else (if true) -- reconciled result (status exists and not empty) will be waited.
	// Pass nil instead context, to use fakeReconciler .Run(ctx) context
	WaitToBeCreated(ctx context.Context, kindName, key string, isReconciled bool) error

	// WatchToBeCreated -- run gorutine to wait while corresponded CRD will be created.
	//
	// If isReconciled is false just reconciliation record (fact) will be probed,
	// else (if true) -- reconciled result (status exists and not empty) will be watched.
	// Does not block current gorutine,  error chan returned to obtain result if need
	// Pass nil instead context, to use fakeReconciler .Run(ctx) context
	WatchToBeCreated(ctx context.Context, kindName, key string, isReconciled bool) (chan error, error)

	// WaitToBeReconciled -- block gorutine while corresponded CRD will be reconciled.
	//
	// if reconciledAfter if zero just reconciliation record (fact) will be probed,
	// else (if real time passed) only fresh reconciliation (after given time) will be accounted
	// Pass nil instead context, to use fakeReconciler .Run(ctx) context
	// Throw error if kind not found, use Wait/WatchToBeCreated(...) to check it.
	WaitToBeReconciled(ctx context.Context, kindName, key string, reconciledAfter time.Time) error

	// WatchToBeReconciled -- run gorutine to wait while corresponded CRD will be reconciled.
	//
	// if reconciledAfter if zero just reconciliation record (fact) will be probed,
	// else (if real time passed) only fresh reconciliation (after given time) will be accounted
	// Does not block current gorutine,  error chan returned to obtain result if need
	// Pass nil instead context, to use fakeReconciler .Run(ctx) context
	// Throw error if kind not found, use Wait/WatchToBeCreated(...) to check it.
	WatchToBeReconciled(ctx context.Context, kindName, key string, reconciledAfter time.Time) (chan *ReconcileResponce, error)

	// WaitToFieldSatisfyRE -- block gorutine while corresponded CRD field will be satisfy to the given regexp.
	//
	// The dot '.' is a separator in the fieldPath
	// Pass nil instead context, to use fakeReconciler .Run(ctx) context
	WaitToFieldSatisfyRE(ctx context.Context, kind, key, fieldPath, reString string) error

	// WatchToFieldSatisfyRE -- run gorutine to wait while corresponded CRD field will be satisfy to the given regexp.
	//
	// The dot '.' is a separator in the fieldPath
	// Pass nil instead context, to use stored early
	WatchToFieldSatisfyRE(ctx context.Context, kind, key, fieldPath, reString string) (chan error, error)

	// WaitToFieldBeChecked -- block gorutine while corresponded CRD field will be exists and checked by callback function.
	//
	// The dot '.' is a separator in the fieldPath
	// Pass nil instead context, to use fakeReconciler .Run(ctx) context
	WaitToFieldBeChecked(ctx context.Context, kind, key, fieldPath string, callbackFunc func(any) bool) error

	// WatchToFieldBeChecked -- run gorutine to wait while corresponded CRD field will be exists and checked by callback function.
	//
	// The dot '.' is a separator in the fieldPath
	// Pass nil instead context, to use fakeReconciler .Run(ctx) context
	WatchToFieldBeChecked(ctx context.Context, kind, key, fieldPath string, callbackFunc func(any) bool) (chan error, error)

	// WaitToFieldBeNotFound -- block gorutine while corresponded CRD field will be not exists (cleaned).
	//
	// The dot '.' is a separator in the fieldPath
	// Pass nil instead context, to use fakeReconciler .Run(ctx) context
	WaitToFieldBeNotFound(ctx context.Context, kind, key, fieldpath string) error

	// WatchToFieldBeNotFound -- run gorutine to wait while corresponded CRD field will be not exists (cleaned).
	//
	// The dot '.' is a separator in the fieldPath
	// Pass nil instead context, to use fakeReconciler .Run(ctx) context
	WatchToFieldBeNotFound(ctx context.Context, kind, key, fieldpath string) (chan error, error)

	// WaitToBeDeleted -- block gorutine while corresponded CRD will be deleted.
	//
	// Pass nil instead context, to use fakeReconciler .Run(ctx) context
	// If requireValidDeletion is `true` object should be really deleted,
	// otherwise valid deletionTimestamp is enough
	WaitToBeDeleted(ctx context.Context, kindName, key string, requireValidDeletion bool) error

	// WatchToBeDeleted -- run gorutine to wait while corresponded CRD will be deleted.
	//
	// Does not block current gorutine,  error chan returned to obtain result if need
	// Pass nil instead context, to use fakeReconciler .Run(ctx) context
	// If requireValidDeletion is `true` object should be really deleted,
	// otherwise valid deletionTimestamp is enough
	WatchToBeDeleted(ctx context.Context, kindName, key string, requireValidDeletion bool) (chan error, error)

	// WaitToBeFinished -- block current goroutine and wait all channels reports about finish or error.
	// If error received, deadline happens or context cancelled error will be returned
	//
	// Pass nil instead context, to use fakeReconciler .Run(ctx) context
	WaitToBeFinished(ctx context.Context, chanList []chan error) error

	// AddController -- add reconciler to the monitor loop while setup (before .Run(...) call)
	AddController(gvk *schema.GroupVersionKind, rcl controllerRTreconcile.Reconciler) error
	AddControllerByType(m schema.ObjectKind, rcl controllerRTreconcile.Reconciler) error

	GetContext() context.Context
	GetClient() controllerRTclient.WithWatch
	GetScheme() *runtime.Scheme
}

type ReconcileResponce struct {
	Err             error
	Result          controllerRTreconcile.Result
	StartFinishTime k8t.TimeInterval
}

func (in *ReconcileResponce) IsRequeued() bool {
	if in.Result.Requeue || in.Result.RequeueAfter != 0 {
		return true
	}
	return false
}

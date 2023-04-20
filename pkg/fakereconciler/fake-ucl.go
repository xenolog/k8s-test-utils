package fakereconciler

import (
	"context"
	"fmt"
	"regexp"
	"testing"
	"time"

	"github.com/itchyny/gojq"
	"github.com/thoas/go-funk"
	k8t "github.com/xenolog/k8s-utils/pkg/types"
	k8u "github.com/xenolog/k8s-utils/pkg/utils"
	apimErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	klog "k8s.io/klog/v2"
	yaml "sigs.k8s.io/yaml"
)

func (r *fakeReconciler) SetPauseTime(ms time.Duration) {
	r.Lock()
	defer r.Unlock()
	r.pauseTime = ms
}

func (r *fakeReconciler) GetPauseTime() time.Duration {
	randomAddition := time.Duration(funk.RandomInt(1, int(r.pauseTime/4)))
	return r.pauseTime + randomAddition
}

func (r *fakeReconciler) WatchToBeDeleted(ctx context.Context, kindName, key string, requireValidDeletion bool) (chan error, error) {
	if r.mainloopContext == nil {
		return nil, fmt.Errorf(k8t.FmtKW, MsgUnableToWatch, MsgMainLoopIsNotStarted)
	}
	if ctx == nil {
		ctx = r.mainloopContext //nolint: contextcheck
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf(k8t.FmtErrKW, MsgUnableToWatch, err)
	}

	if _, err := r.getKindStruct(kindName); err != nil {
		return nil, err
	}
	respChan := make(chan error, 1) // buffered to push-and-close result

	r.userTasksWG.Add(1)
	go func(kindName, key string, respChan chan error, rvd bool) {
		defer r.userTasksWG.Done()
		defer close(respChan)
		logKey := fmt.Sprintf("RCL: WaitingToBeDeleted %v [%s] '%s'", r.pauseTime, kindName, key)

		kwd, err := r.getKindStruct(kindName)
		if err != nil {
			respChan <- err
		}
		nName := k8u.KeyToNamespacedName(key)
		obj := &unstructured.Unstructured{}
		obj.SetGroupVersionKind(*kwd.gvk)
		for {
			// check object exists
			err = r.client.Get(ctx, nName, obj)
			switch {
			case k8u.IsNotFound(err):
				klog.Warningf("%s, object removed successfully", logKey)
				respChan <- nil
				return
			case err != nil:
				respChan <- err
				return
			case obj.GetDeletionTimestamp().IsZero():
				klog.Warningf("%s...", logKey)
			default:
				dts := obj.GetDeletionTimestamp().UTC()
				klog.Warningf("%s, deletionTimestamp is '%s'", logKey, dts.Format(k8t.FmtRFC3339))
				if !rvd {
					klog.Warningf("%s, it's enough", logKey)
					respChan <- nil
					return
				}
			}

			select {
			case <-r.mainloopContext.Done():
				klog.Warningf(k8t.FmtKW, logKey, r.mainloopContext.Err())
				respChan <- r.mainloopContext.Err()
				return
			case <-ctx.Done():
				klog.Warningf(k8t.FmtKW, logKey, ctx.Err())
				respChan <- ctx.Err()
				return
			case <-time.After(r.GetPauseTime()):
				continue
			}
		}
	}(kindName, key, respChan, requireValidDeletion)

	return respChan, nil
}

func (r *fakeReconciler) WaitToBeDeleted(ctx context.Context, kindName, key string, requireValidDeletion bool) error {
	respCh, err := r.WatchToBeDeleted(ctx, kindName, key, requireValidDeletion)
	if err == nil {
		receivedErr, ok := <-respCh
		switch {
		case !ok:
			err = fmt.Errorf(k8t.FmtResponseChanUClosed, k8t.ErrSomethingWentWrong)
		case receivedErr != nil:
			err = receivedErr
		}
	}
	return err
}

// -----------------------------------------------------------------------------
func (r *fakeReconciler) WatchToBeReconciled(ctx context.Context, kindName, key string, reconciledAfter time.Time) (chan *ReconcileResponce, error) {
	if r.mainloopContext == nil {
		return nil, fmt.Errorf(k8t.FmtKW, MsgUnableToWatch, MsgMainLoopIsNotStarted)
	}
	if ctx == nil {
		ctx = r.mainloopContext //nolint: contextcheck
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf(k8t.FmtErrKW, MsgUnableToWatch, err)
	}

	if _, err := r.getKindStruct(kindName); err != nil {
		return nil, err
	}
	respChan := make(chan *ReconcileResponce, 1) // buffered to push-and-close result

	r.userTasksWG.Add(1)
	go func(kindName, key string, respChan chan *ReconcileResponce) {
		defer r.userTasksWG.Done()
		defer close(respChan)
		logKey := fmt.Sprintf("RCL: WaitingToBeReconciled %v [%s] '%s'", r.pauseTime, kindName, key)

		for {
			kwd, err := r.getKindStruct(kindName)
			if err != nil {
				respChan <- &ReconcileResponce{Err: err}
			}

			// check object exists
			nName := k8u.KeyToNamespacedName(key)
			obj := &unstructured.Unstructured{}
			obj.SetGroupVersionKind(*kwd.gvk)
			if err := r.client.Get(ctx, nName, obj); err != nil {
				respChan <- &ReconcileResponce{Err: fmt.Errorf("RCL: Unable to reconcile %s '%s': %w", kindName, key, err)}
				return
			}

			objRec, ok := kwd.GetObj(key)
		analizeLastReconcileResponse:
			switch {
			case !ok:
				// object record may be absent if object really not found or if object exists, but never reconciled
				klog.Warningf("%s: never reconciled, continue waiting...", logKey)
			case len(objRec.log) > 0:
				// object reconcile log is exists and not emplty
				lastLogRecord := objRec.log[len(objRec.log)-1]
				if !reconciledAfter.IsZero() {
					lastReconcileTs := lastLogRecord.StartFinishTime[0]
					switch {
					case lastReconcileTs.Before(reconciledAfter):
						klog.Warningf("%s: reconciled at '%s', earlier than '%s', continue waiting...", logKey, lastReconcileTs.UTC().Format(k8t.FmtRFC3339), reconciledAfter.UTC().Format(k8t.FmtRFC3339))
						break analizeLastReconcileResponse
					case apimErrors.IsConflict(lastLogRecord.Err):
						klog.Warningf("%s: reconciled at '%s', but conflict", logKey, lastReconcileTs.UTC().Format(k8t.FmtRFC3339))
						break analizeLastReconcileResponse
					}
				}
				respChan <- &lastLogRecord
				return
			}
			select {
			case <-r.mainloopContext.Done():
				klog.Warningf(k8t.FmtKW, logKey, r.mainloopContext.Err())
				respChan <- &ReconcileResponce{Err: r.mainloopContext.Err()}
				return
			case <-ctx.Done():
				klog.Warningf(k8t.FmtKW, logKey, ctx.Err())
				respChan <- &ReconcileResponce{Err: ctx.Err()}
				return
			case <-time.After(r.GetPauseTime()):
				continue
			}
		}
	}(kindName, key, respChan)

	return respChan, nil
}

func (r *fakeReconciler) WaitToBeReconciled(ctx context.Context, kindName, key string, reconciledAfter time.Time) error {
	respCh, err := r.WatchToBeReconciled(ctx, kindName, key, reconciledAfter)
	if err == nil {
		resp, ok := <-respCh
		switch {
		case !ok:
			err = fmt.Errorf(k8t.FmtResponseChanUClosed, k8t.ErrSomethingWentWrong)
		case resp.Err != nil:
			err = resp.Err
		}
	}
	return err
}

//-----------------------------------------------------------------------------

func (r *fakeReconciler) WatchToBeCreated(ctx context.Context, kind, key string, isReconciled bool) (chan error, error) { //revive:disable:flag-parameter
	logKey := fmt.Sprintf("RCL: WaitingToBeCreated %v [%s] '%s'", r.pauseTime, kind, key)
	return r.watchToFieldBeChecked(ctx, logKey, kind, key, "status", func(in any) bool {
		if !isReconciled {
			return true
		}
		status, ok := in.(map[string]any) // status exists if resource reconciled
		return ok && len(status) > 0
	})
}

func (r *fakeReconciler) WaitToBeCreated(ctx context.Context, kind, key string, isReconciled bool) error {
	respCh, err := r.WatchToBeCreated(ctx, kind, key, isReconciled)
	if err == nil {
		receivedErr, ok := <-respCh
		switch {
		case !ok:
			err = fmt.Errorf(k8t.FmtResponseChanUClosed, k8t.ErrSomethingWentWrong)
		case receivedErr != nil:
			err = receivedErr
		default:
			klog.Warningf("RCL: WaitingToBeCreated [%s] '%s', created successfully", kind, key)
		}
	}
	return err
}

//-----------------------------------------------------------------------------

func (r *fakeReconciler) WaitToSatisfyJQ(ctx context.Context, kind, key, jqString string) error {
	respCh, err := r.WatchToSatisfyJQ(ctx, kind, key, jqString)
	if err == nil {
		receivedErr, ok := <-respCh
		switch {
		case !ok:
			err = fmt.Errorf(k8t.FmtResponseChanUClosed, k8t.ErrSomethingWentWrong)
		case receivedErr != nil:
			err = receivedErr
		}
	}
	return err
}

func (r *fakeReconciler) WatchToSatisfyJQ(ctx context.Context, kindName, key, jqString string) (chan error, error) {
	if r.mainloopContext == nil {
		return nil, fmt.Errorf(k8t.FmtKW, MsgUnableToWatch, MsgMainLoopIsNotStarted)
	}
	if ctx == nil {
		ctx = r.mainloopContext //nolint: contextcheck
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf(k8t.FmtErrKW, MsgUnableToWatch, err)
	}

	jq, err := gojq.Parse(jqString)
	if err != nil {
		return nil, fmt.Errorf(k8t.FmtErrKW, MsgUnableToWatch, err)
	}

	rr, err := r.getKindStruct(kindName)
	if err != nil {
		return nil, err
	}
	respChan := make(chan error, 1) // buffered to push-and-close result

	r.userTasksWG.Add(1)
	go func(kind, key string, jq *gojq.Query, respChan chan error) {
		defer r.userTasksWG.Done()
		defer close(respChan)
		logKey := fmt.Sprintf("RCL: WaitingToSatisfyJQ %v [%s] '%s'", r.pauseTime, kindName, key)
		nName := k8u.KeyToNamespacedName(key)
		obj := &unstructured.Unstructured{}
		obj.SetGroupVersionKind(*rr.gvk)
		for {
			klog.Warningf("%s...", logKey)
			err := r.client.Get(ctx, nName, obj)
			switch {
			case k8u.IsNotFound(err):
				klog.Warningf("%s: obj [%s] '%s' is not found, waiting to be created...", logKey, kind, key)
			case err != nil:
				klog.Warningf("%s: Error while fetching obj: %s", logKey, err)
			default:
				iter := jq.RunWithContext(ctx, obj.Object)
				for {
					v, ok := iter.Next()
					if !ok {
						break // for
					}
					if err, ok := v.(error); ok {
						respChan <- err
						return
					}
					res, ok := v.(bool)
					switch {
					case !ok:
						respChan <- fmt.Errorf("JQ query result is not boolean")
						return
					case res:
						respChan <- nil
						return
					default:
						klog.Warningf("%s: is not satisfy to given JQ conditions, continue waiting...", logKey)
					}
				}

				// res, ok, err := unstructured.NestedFieldCopy(obj.Object, pathSlice...)
				// switch {
				// case err != nil:
				// 	respChan <- err
				// 	return
				// case ok && callback(res):
				// 	respChan <- nil
				// 	return
				// case ok:
				// 	klog.Warningf("%s: field '%s' is not satisfy to given conditions, continue waiting...", logKey, fp)
				// default:
				// 	klog.Warningf("%s: field '%s' is not found, continue waiting...", logKey, fp)
				// }
			}
			select {
			case <-r.mainloopContext.Done():
				klog.Warningf(k8t.FmtKW, logKey, r.mainloopContext.Err())
				respChan <- r.mainloopContext.Err()
				return
			case <-ctx.Done():
				klog.Warningf(k8t.FmtKW, logKey, ctx.Err())
				respChan <- ctx.Err()
				return
			case <-time.After(r.GetPauseTime()):
				continue
			}
		}
	}(kindName, key, jq, respChan)

	return respChan, err
}

//-----------------------------------------------------------------------------

func (r *fakeReconciler) WatchToFieldSatisfyRE(ctx context.Context, kind, key, fieldpath, reString string) (chan error, error) {
	logKey := fmt.Sprintf("RCL: WaitingToFieldSatisfyRE %v [%s] '%s'", r.pauseTime, kind, key)
	re := regexp.MustCompile(reString)
	return r.watchToFieldBeChecked(ctx, logKey, kind, key, fieldpath, func(in any) bool {
		str, ok := in.(string)
		if !ok {
			return false
		}
		return re.MatchString(str)
	})
}

func (r *fakeReconciler) WaitToFieldSatisfyRE(ctx context.Context, kind, key, fieldpath, reString string) error {
	respCh, err := r.WatchToFieldSatisfyRE(ctx, kind, key, fieldpath, reString)
	if err == nil {
		receivedErr, ok := <-respCh
		switch {
		case !ok:
			err = fmt.Errorf(k8t.FmtResponseChanUClosed, k8t.ErrSomethingWentWrong)
		case receivedErr != nil:
			err = receivedErr
		}
	}
	return err
}

//-----------------------------------------------------------------------------

var fieldPathSplitRE = regexp.MustCompile(`[.:/]`)

func (r *fakeReconciler) watchToFieldBeChecked(ctx context.Context, logKey, kindName, key, fieldpath string, callback func(any) bool) (chan error, error) {
	if r.mainloopContext == nil {
		return nil, fmt.Errorf(k8t.FmtKW, MsgUnableToWatch, MsgMainLoopIsNotStarted)
	}
	if ctx == nil {
		ctx = r.mainloopContext //nolint: contextcheck
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf(k8t.FmtErrKW, MsgUnableToWatch, err)
	}

	rr, err := r.getKindStruct(kindName)
	if err != nil {
		return nil, err
	}
	respChan := make(chan error, 1) // buffered to push-and-close result

	r.userTasksWG.Add(1)
	go func(kind, key, fp, logKey string, respChan chan error) {
		defer r.userTasksWG.Done()
		defer close(respChan)
		nName := k8u.KeyToNamespacedName(key)
		obj := &unstructured.Unstructured{}
		obj.SetGroupVersionKind(*rr.gvk)
		pathSlice := fieldPathSplitRE.Split(fp, -1)
		for {
			klog.Warningf("%s...", logKey)
			err := r.client.Get(ctx, nName, obj)
			switch {
			case k8u.IsNotFound(err):
				klog.Warningf("%s: obj [%s] '%s' is not found, waiting to be created...", logKey, kind, key)
			case err != nil:
				klog.Warningf("%s: Error while fetching obj: %s", logKey, err)
			default:
				res, ok, err := unstructured.NestedFieldCopy(obj.Object, pathSlice...)
				switch {
				case err != nil:
					respChan <- err
					return
				case ok && callback(res):
					respChan <- nil
					return
				case ok:
					klog.Warningf("%s: field '%s' is not satisfy to given conditions, continue waiting...", logKey, fp)
				default:
					klog.Warningf("%s: field '%s' is not found, continue waiting...", logKey, fp)
				}
			}
			select {
			case <-r.mainloopContext.Done():
				klog.Warningf(k8t.FmtKW, logKey, r.mainloopContext.Err())
				respChan <- r.mainloopContext.Err()
				return
			case <-ctx.Done():
				klog.Warningf(k8t.FmtKW, logKey, ctx.Err())
				respChan <- ctx.Err()
				return
			case <-time.After(r.GetPauseTime()):
				continue
			}
		}
	}(kindName, key, fieldpath, logKey, respChan)

	return respChan, err
}

func (r *fakeReconciler) WatchToFieldBeChecked(ctx context.Context, kind, key, fieldpath string, callback func(any) bool) (chan error, error) { //revive:disable:confusing-naming
	logKey := fmt.Sprintf("RCL: WaitingToFieldBeChecked %v [%s] '%s'", r.pauseTime, kind, key)
	return r.watchToFieldBeChecked(ctx, logKey, kind, key, fieldpath, callback)
}

func (r *fakeReconciler) WaitToFieldBeChecked(ctx context.Context, kind, key, fieldpath string, callback func(any) bool) error {
	respCh, err := r.WatchToFieldBeChecked(ctx, kind, key, fieldpath, callback)
	if err == nil {
		if _, ok := <-respCh; !ok {
			err = fmt.Errorf(k8t.FmtResponseChanUClosed, k8t.ErrSomethingWentWrong)
		}
	}
	return err
}

// -----------------------------------------------------------------------------

func (r *fakeReconciler) WaitToBeFinished(ctx context.Context, chanList []chan error) error { //nolint: contextcheck
	if r.mainloopContext == nil {
		return fmt.Errorf(k8t.FmtKW, MsgUnableToWatch, MsgMainLoopIsNotStarted)
	}
	if ctx == nil {
		ctx = r.mainloopContext
	}

exLoop:
	for len(chanList) != 0 {
		for i := range chanList {
			select {
			case <-ctx.Done():
				return fmt.Errorf("%w, %d left", ctx.Err(), len(chanList))
			case err, ok := <-chanList[i]:
				switch {
				case !ok:
					return fmt.Errorf("channel was unexpectedly closed by sender")
				case err != nil:
					return err
				default:
					chanList = append(chanList[:i], chanList[i+1:]...)
					continue exLoop
				}
			default:
				continue // non-blocking select
			}
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("%w, %d left", ctx.Err(), len(chanList))
		case <-time.After(r.GetPauseTime()):
			continue
		}
	}
	return nil
}

// -----------------------------------------------------------------------------

func (r *fakeReconciler) watchToFieldBeNotFound(ctx context.Context, logKey, kindName, key, fieldpath string) (chan error, error) {
	if r.mainloopContext == nil {
		return nil, fmt.Errorf(k8t.FmtKW, MsgUnableToWatch, MsgMainLoopIsNotStarted)
	}
	if ctx == nil {
		ctx = r.mainloopContext //nolint: contextcheck
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf(k8t.FmtErrKW, MsgUnableToWatch, err)
	}

	rr, err := r.getKindStruct(kindName)
	if err != nil {
		return nil, err
	}
	respChan := make(chan error, 1) // buffered to push-and-close result

	r.userTasksWG.Add(1)
	go func(_, key, fp, logKey string, respChan chan error) {
		defer r.userTasksWG.Done()
		defer close(respChan)
		nName := k8u.KeyToNamespacedName(key)
		obj := &unstructured.Unstructured{}
		obj.SetGroupVersionKind(*rr.gvk)
		pathSlice := fieldPathSplitRE.Split(fp, -1)
		for {
			err := r.client.Get(ctx, nName, obj)
			switch {
			case err != nil:
				klog.Warningf("%s: Error while fetching obj: %s", logKey, err)
				respChan <- err
				return
			default:
				_, ok, err := unstructured.NestedFieldNoCopy(obj.Object, pathSlice...)
				switch {
				case err != nil:
					respChan <- err
					return
				case !ok:
					// field not found
					respChan <- nil
					return
				default:
					klog.Warningf("%s: field '%s' is found, continue waiting...", logKey, fp)
				}
			}
			select {
			case <-r.mainloopContext.Done():
				klog.Warningf(k8t.FmtKW, logKey, r.mainloopContext.Err())
				respChan <- r.mainloopContext.Err()
				return
			case <-ctx.Done():
				klog.Warningf(k8t.FmtKW, logKey, ctx.Err())
				respChan <- ctx.Err()
				return
			case <-time.After(r.GetPauseTime()):
				continue
			}
		}
	}(kindName, key, fieldpath, logKey, respChan)

	return respChan, err
}

func (r *fakeReconciler) WatchToFieldBeNotFound(ctx context.Context, kind, key, fieldpath string) (chan error, error) { //revive:disable:confusing-naming
	logKey := fmt.Sprintf("RCL: WaitingToFieldBeNotFound %v [%s] '%s'", r.pauseTime, kind, key)
	return r.watchToFieldBeNotFound(ctx, logKey, kind, key, fieldpath)
}

func (r *fakeReconciler) WaitToFieldBeNotFound(ctx context.Context, kind, key, fieldpath string) error {
	respCh, err := r.WatchToFieldBeNotFound(ctx, kind, key, fieldpath)
	if err == nil {
		if _, ok := <-respCh; !ok {
			err = fmt.Errorf(k8t.FmtResponseChanUClosed, k8t.ErrSomethingWentWrong)
		}
	}
	return err
}

// -----------------------------------------------------------------------------

func (r *fakeReconciler) GetAndPublishIfTestFailed(ctx context.Context, t *testing.T, kindName, key string) func() {
	run, _ := r.GetAndPublishIfTestFailedWithCancel(ctx, t, kindName, key)
	return run
}

func (r *fakeReconciler) GetAndPublishIfTestFailedWithCancel(ctx context.Context, t *testing.T, kindName, key string) (func(), func()) {
	do := true
	return func() {
			if t.Failed() && do {
				if r.mainloopContext == nil {
					t.Logf("ERR: %s", MsgMainLoopIsNotStarted)
					return
				}
				if ctx == nil {
					ctx = r.mainloopContext
				}
				if err := ctx.Err(); err != nil {
					t.Logf("ERR: %s", err)
					return
				}
				rr, err := r.getKindStruct(kindName)
				if err != nil {
					t.Logf("ERR: %s", err)
					return
				}

				nName := k8u.KeyToNamespacedName(key)
				obj := &unstructured.Unstructured{}
				obj.SetGroupVersionKind(*rr.gvk)
				err = r.client.Get(ctx, nName, obj)
				if err != nil {
					t.Logf("ERR while fetching obj: %s", err)
					return
				}
				buff, err := yaml.Marshal(obj.Object)
				if err != nil {
					t.Logf("ERR while YAML marshaling: %s", err)
					return
				}
				t.Logf("[%s] %s:\n%s\n", kindName, nName.String(), string(buff))
			}
		}, func() {
			do = false
		}
}

// -----------------------------------------------------------------------------

// Reconcile -- invoke to reconcile the corresponded resource
// returns chan which can be used to obtain reconcile responcce and timings
func (r *fakeReconciler) Reconcile(kind, key string) (chan *ReconcileResponce, error) {
	klog.Infof("RCL: user-Req to reconcile %s '%s'", kind, key)
	return r.reconcile(kind, key)
}

// Lock -- lock watchers/reconcilers for the specifyed Kind type.
// returns callable to Unock thread
func (r *fakeReconciler) LockReconciler(kind string) func() {
	watcherRec, err := r.getKindStruct(kind)
	if err != nil {
		klog.Warningf("RCL-LOOP: try to lock unsupported Kind '%s': %s", kind, err)
		return func() {
			klog.Warningf("RCL-LOOP: try to unlock unsupported Kind '%s': %s", kind, err)
		}
	}
	watcherRec.Lock()
	klog.Warningf("RCL-LOOP: '%s' locked", kind)
	return func() {
		klog.Warningf("RCL-LOOP: '%s' unlocked", kind)
		watcherRec.Unlock()
	}
}

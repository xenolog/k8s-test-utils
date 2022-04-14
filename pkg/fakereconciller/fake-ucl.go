package fakereconciller

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	k8t "github.com/xenolog/k8s-utils/pkg/types"
	"github.com/xenolog/k8s-utils/pkg/utils"
	apimErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/klog"
)

func (r *fakeReconciller) WatchToBeReconciled(ctx context.Context, kindName, key string, reconciledAfter time.Time) (chan error, error) {
	if ctx == nil {
		ctx = r.mainloopContext
	}
	kindWatcherData, err := r.getKindStruct(kindName)
	if err != nil {
		return nil, err
	}
	respChan := make(chan error, ControlChanBuffSize)
	logKey := fmt.Sprintf("RCL: WaitingToBeReconciled [%s] '%s'", kindName, key)

	r.userTasksWG.Add(1)
	go func() {
		defer r.userTasksWG.Done()
		defer close(respChan)
		for {
			r.Lock()
			objRec, ok := kindWatcherData.processedObjs[key]
			r.Unlock()
			if ok && !objRec.running && len(objRec.log) > 0 {
				// record about reconcile passed found
				if reconciledAfter.IsZero() {
					respChan <- nil
					return
				}
				if objRec.log[len(objRec.log)-1].StartFinishTime[0].After(reconciledAfter) {
					respChan <- nil
					return
				}
				klog.Warningf("%s: reconciled earlier, than '%s' , waiting to fresh reconcile", logKey, reconciledAfter)
			}
			select {
			case <-r.mainloopContext.Done():
				klog.Warningf("%s: %s", logKey, errStoppedFromTheOutside)
				respChan <- errStoppedFromTheOutside
				return
			case <-ctx.Done():
				klog.Warningf("%s: %s", logKey, ctx.Err())
				respChan <- ctx.Err()
				return
			case <-time.After(PauseTime):
				continue
			}
		}
	}()

	return respChan, nil
}

func (r *fakeReconciller) WaitToBeReconciled(ctx context.Context, kindName, key string, reconciledAfter time.Time) error {
	respCh, err := r.WatchToBeReconciled(ctx, kindName, key, reconciledAfter)
	if err == nil {
		if _, ok := <-respCh; !ok {
			err = fmt.Errorf("%w: Response chan unexpectable closed.", k8t.ErrorSomethingWentWrong)
		}
	}
	return err
}

func (r *fakeReconciller) WatchToBeCreated(ctx context.Context, kind, key string, isReconcilled bool) (chan error, error) {
	if ctx == nil {
		ctx = r.mainloopContext
	}
	rr, err := r.getKindStruct(kind)
	if err != nil {
		return nil, err
	}
	respChan := make(chan error, ControlChanBuffSize)
	logKey := fmt.Sprintf("RCL: WaitingToCreate [%s] '%s'", kind, key)
	nName := utils.KeyToNamespacedName(key)
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(*rr.gvk)

	r.userTasksWG.Add(1)
	go func() {
		defer r.userTasksWG.Done()
		defer close(respChan)
		for {
			err := r.client.Get(ctx, nName, obj)
			switch {
			case err != nil && !apimErrors.IsNotFound(err):
				klog.Warningf("%s: Error while fetching obj: %s", logKey, err)
			case err == nil:
				if !isReconcilled {
					// status exists is not valuable
					respChan <- nil
					return
				}
				if _, ok, _ := unstructured.NestedMap(obj.Object, "status"); ok {
					// status exist
					respChan <- nil
					return
				}
				klog.Warningf("%s: object exists, but status not found, waiting to reconcile", logKey)
			}
			select {
			case <-r.mainloopContext.Done():
				klog.Warningf("%s: %s", logKey, errStoppedFromTheOutside)
				respChan <- errStoppedFromTheOutside
				return
			case <-ctx.Done():
				klog.Warningf("%s: %s", logKey, ctx.Err())
				respChan <- ctx.Err()
				return
			case <-time.After(PauseTime):
				continue
			}
		}
	}()

	return respChan, err
}

func (r *fakeReconciller) WaitToBeCreated(ctx context.Context, kind, key string, isReconcilled bool) error {
	respCh, err := r.WatchToBeCreated(ctx, kind, key, isReconcilled)
	if err == nil {
		if _, ok := <-respCh; !ok {
			err = fmt.Errorf("%w: Response chan unexpectable closed.", k8t.ErrorSomethingWentWrong)
		}
	}
	return err
}

func (r *fakeReconciller) WatchToFieldSatisfyRE(ctx context.Context, kind, key, fieldpath, reString string) (chan string, error) {
	if ctx == nil {
		ctx = r.mainloopContext
	}
	rr, err := r.getKindStruct(kind)
	if err != nil {
		return nil, err
	}
	re, err := regexp.Compile(reString)
	if err != nil {
		return nil, err
	}
	respChan := make(chan string, ControlChanBuffSize)
	// logKey := fmt.Sprintf("RCL: WatchingToFieldSatisfyRE [%s] '%s' %s~%s", kind, key,fieldpath, reString)
	logKey := fmt.Sprintf("RCL: WaitingToFieldSatisfyRE [%s] '%s'", kind, key)
	nName := utils.KeyToNamespacedName(key)
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(*rr.gvk)
	pathSlice := strings.Split(fieldpath, ".")

	r.userTasksWG.Add(1)
	go func() {
		defer r.userTasksWG.Done()
		defer close(respChan)
		for {
			err := r.client.Get(ctx, nName, obj)
			switch {
			case err != nil && !apimErrors.IsNotFound(err):
				klog.Warningf("%s: Error while fetching obj: %s", logKey, err)
			case apimErrors.IsNotFound(err):
				klog.Warningf("%s: obj [%s] '%s' not found, waiting to create", logKey, kind, key)
			case err == nil:
				res, ok, err := unstructured.NestedString(obj.Object, pathSlice...)
				switch {
				case err != nil:
					respChan <- fmt.Sprintf("fakeRclERR: %s", err)
					return
				case ok && re.MatchString(res):
					respChan <- res
					return
				case ok:
					klog.Warningf("%s: field '%s' is not satisfy /%s/, waiting next reconcile", logKey, fieldpath, reString)
				default:
					klog.Warningf("%s: field '%s' is not found, waiting next reconcile", logKey, fieldpath)
				}
			}
			select {
			case <-r.mainloopContext.Done():
				klog.Warningf("%s: %s", logKey, errStoppedFromTheOutside)
				respChan <- fmt.Sprintf("fakeRclERR: %s", errStoppedFromTheOutside)
				return
			case <-ctx.Done():
				klog.Warningf("%s: %s", logKey, ctx.Err())
				respChan <- fmt.Sprintf("fakeRclERR: %s", errStoppedFromTheOutside)
				return
			case <-time.After(PauseTime):
				continue
			}
		}
	}()

	return respChan, err
}

func (r *fakeReconciller) WaitToFieldSatisfyRE(ctx context.Context, kind, key, fieldpath, reString string) (string, error) {
	var (
		rv string
		ok bool
	)
	respCh, err := r.WatchToFieldSatisfyRE(ctx, kind, key, fieldpath, reString)
	if err == nil {
		if rv, ok = <-respCh; !ok {
			err = fmt.Errorf("%w: Response chan unexpectable closed.", k8t.ErrorSomethingWentWrong)
		}
	}
	return rv, err
}
func (r *fakeReconciller) WatchToFieldBeChecked(ctx context.Context, kind, key, fieldpath string, callback func(interface{}) bool) (chan error, error) {
	if ctx == nil {
		ctx = r.mainloopContext
	}
	rr, err := r.getKindStruct(kind)
	if err != nil {
		return nil, err
	}
	respChan := make(chan error, ControlChanBuffSize)
	logKey := fmt.Sprintf("RCL: WaitingToFieldBeChecked [%s] '%s'", kind, key)
	nName := utils.KeyToNamespacedName(key)
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(*rr.gvk)
	pathSlice := strings.Split(fieldpath, ".")

	r.userTasksWG.Add(1)
	go func() {
		defer r.userTasksWG.Done()
		defer close(respChan)
		for {
			err := r.client.Get(ctx, nName, obj)
			switch {
			case err != nil && !apimErrors.IsNotFound(err):
				klog.Warningf("%s: Error while fetching obj: %s", logKey, err)
			case apimErrors.IsNotFound(err):
				klog.Warningf("%s: obj [%s] '%s' not found, waiting to create", logKey, kind, key)
			case err == nil:
				res, ok, err := unstructured.NestedFieldCopy(obj.Object, pathSlice...)
				switch {
				case err != nil:
					respChan <- err
					return
				case ok && callback(res):
					respChan <- nil
					return
				case ok:
					klog.Warningf("%s: field '%s' is not satisfy given callback function, waiting next reconcile", logKey, fieldpath)
				default:
					klog.Warningf("%s: field '%s' is not found, waiting next reconcile", logKey, fieldpath)
				}
			}
			select {
			case <-r.mainloopContext.Done():
				klog.Warningf("%s: %s", logKey, errStoppedFromTheOutside)
				respChan <- errStoppedFromTheOutside
				return
			case <-ctx.Done():
				klog.Warningf("%s: %s", logKey, ctx.Err())
				respChan <- errStoppedFromTheOutside
				return
			case <-time.After(PauseTime):
				continue
			}
		}
	}()

	return respChan, err
}

func (r *fakeReconciller) WaitToFieldBeChecked(ctx context.Context, kind, key, fieldpath string, callback func(interface{}) bool) error {
	respCh, err := r.WatchToFieldBeChecked(ctx, kind, key, fieldpath, callback)
	if err == nil {
		if _, ok := <-respCh; !ok {
			err = fmt.Errorf("%w: Response chan unexpectable closed.", k8t.ErrorSomethingWentWrong)
		}
	}
	return err
}

// Reconcile -- invoke to reconcile the corresponded resource
// returns chan which can be used to obtain reconcile responcce and timings
func (r *fakeReconciller) Reconcile(kind, key string) (chan *ReconcileResponce, error) {
	var respChan chan *ReconcileResponce
	rr, err := r.getKindStruct(kind)
	if err == nil {
		respChan = make(chan *ReconcileResponce, ControlChanBuffSize)
		rr.askToReconcile <- &reconcileRequest{
			Key:      key,
			RespChan: respChan,
		}
	}
	return respChan, err
}

// Lock -- lock watchers/reconcillers for the specifyed Kind type.
// returns callable to Unock thread
func (r *fakeReconciller) LockReconciller(kind string) func() {
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

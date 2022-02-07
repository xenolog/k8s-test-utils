package fakereconciller

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	k8t "github.com/xenolog/k8s-utils/pkg/types"
	"github.com/xenolog/k8s-utils/pkg/utils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	apimTypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	PauseTime           = 127 * time.Millisecond
	ControlChanBuffSize = 8
)

var (
	errStoppedFromTheOutside = errors.New("Stopped from the outside (main loop context cancelled or deadline expired)")
)

type reconcileRequest struct {
	Key      string
	RespChan chan *ReconcileResponce
}

type reconcileStatus struct {
	sync.Mutex
	log     []ReconcileResponce
	nName   apimTypes.NamespacedName
	running bool
}

type kindWatcherData struct {
	gvk            *schema.GroupVersionKind
	kind           string
	askToReconcile chan *reconcileRequest
	reconciler     NativeReconciller
	processedObjs  map[string]*reconcileStatus
}

type fakeReconciller struct {
	sync.Mutex
	scheme          *runtime.Scheme
	kinds           map[string]*kindWatcherData
	client          client.WithWatch
	mainloopContext context.Context
	watchersWG      sync.WaitGroup
	userTasksWG     sync.WaitGroup
}

func (r *fakeReconciller) GetClient() client.WithWatch {
	return r.client
}
func (r *fakeReconciller) GetScheme() *runtime.Scheme {
	return r.scheme
}

func (r *fakeReconciller) getKindStruct(kind string) (*kindWatcherData, error) {
	r.Lock()
	defer r.Unlock()
	rv, ok := r.kinds[kind]
	if !ok {
		return nil, fmt.Errorf("Kind '%s' does not served by this reconcile loop, %w", kind, k8t.ErrorDoNothing)
	}
	return rv, nil
}

func (r *fakeReconciller) doReconcile(ctx context.Context, kindName string, obj client.Object, respChan chan *ReconcileResponce) *ReconcileResponce {
	var (
		err           error
		res           reconcile.Result
		reconcileResp ReconcileResponce
		startTime     time.Time
		endTime       time.Time
	)

	kindWatcherData, err := r.getKindStruct(kindName)
	if err != nil {
		return &ReconcileResponce{Err: err, StartFinishTime: k8t.TimeInterval{startTime, time.Now()}}
	}

	nName, err := utils.GetRuntimeObjectNamespacedName(obj)
	if err != nil {
		return &ReconcileResponce{Err: err, StartFinishTime: k8t.TimeInterval{startTime, time.Now()}}
	}

	r.Lock()
	objRec, ok := kindWatcherData.processedObjs[nName.String()]
	if !ok {
		kindWatcherData.processedObjs[nName.String()] = &reconcileStatus{}
		objRec = kindWatcherData.processedObjs[nName.String()]
	}
	r.Unlock()

	startTime = time.Now()
	objRec.Lock()
	objRec.nName = nName
	objRec.log = append(objRec.log, ReconcileResponce{
		StartFinishTime: k8t.TimeInterval{startTime, time.Time{}},
	})
	objRec.running = true
	objRec.Unlock()

	defer func() {
		objRec.Lock()
		idx := len(objRec.log) - 1
		objRec.log[idx].StartFinishTime[1] = endTime
		objRec.log[idx].Result = res
		objRec.log[idx].Err = err
		objRec.running = false
		objRec.Unlock()
	}()

	ensureUID(ctx, r.client, obj)

	res, err = kindWatcherData.reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nName})
	endTime = time.Now()
	reconcileResp = ReconcileResponce{
		Result:          res,
		Err:             err,
		StartFinishTime: k8t.TimeInterval{startTime, endTime},
	}
	if respChan != nil {
		respChan <- &reconcileResp
		close(respChan)
	}
	return &reconcileResp
}

// Watch C/R/M/D events from fakeClient or user request.
// run one instance per Kind type
func (r *fakeReconciller) doWatch(ctx context.Context, watcher watch.Interface, kind string) {
	defer r.watchersWG.Done()
	r.Lock()
	taskChan := r.kinds[kind].askToReconcile
	rcl := r.kinds[kind].reconciler
	r.Unlock()

	if rcl == nil {
		panic(fmt.Sprintf("Native reconciller for %s undefined.", kind))
	}

	for {
		select {
		case <-ctx.Done():
			watcher.Stop()
			klog.Infof("RCL: Watcher for %s finished", kind)
			return
		case req := <-taskChan:
			klog.Infof("RCL: Req to reconcile: %s(%s)", kind, req.Key)
			nName := utils.KeyToNamespacedName(req.Key)
			obj := &unstructured.Unstructured{}
			r.Lock()
			obj.SetGroupVersionKind(*r.kinds[kind].gvk)
			r.Unlock()
			if err := r.client.Get(ctx, nName, obj); err != nil {
				klog.Errorf("RCL error: %s", err)
				break
			}

			rv := r.doReconcile(ctx, kind, obj, req.RespChan)
			rvString := fmt.Sprintf("RCL: Reconcile %s(%s) result: %s", kind, req.Key, rv)
			if rv.Err != nil {
				klog.Errorf(rvString)
			} else {
				klog.Infof(rvString)
			}
		case in := <-watcher.ResultChan():
			nName, err := utils.GetRuntimeObjectNamespacedName(in.Object)
			if err != nil {
				panic("Wrong object passed from watcher")
			}
			k8sObj, ok := in.Object.(client.Object)
			if !ok {
				panic("Wrong object passed from watcher")
			}
			tmp := strings.Split(reflect.TypeOf(in.Object).String(), ".")
			objType := tmp[len(tmp)-1]
			klog.Infof("RCL: income event: %s   %s(%s)", in.Type, objType, nName)
			switch in.Type {
			case watch.Deleted:
				if len(k8sObj.GetFinalizers()) == 0 {
					// no finalizers, object will be deleted by fake client
					klog.Infof("RCL: deletion of [%s] '%s' done, no finalizers.", objType, nName)
				} else {
					// at least one finalizer found, DeletionTimestamp of the object should be set if absent
					if k8sObj.GetDeletionTimestamp().IsZero() {
						now := metav1.Now()
						k8sObj.SetDeletionTimestamp(&now)
						if err := r.client.Update(ctx, k8sObj); err != nil {
							klog.Errorf("RCL Obj '%s' deletion error: %s", nName, err)
						}
						// MODIFY event will initiate Reconcile automatically
						klog.Infof("RCL: DeletionTimestamp of [%s] '%s' is set.", objType, nName)
					} else {
						klog.Infof("RCL: DeletionTimestamp of [%s] '%s' is already found, the object was planned to delete earlier, try to reconcile.", objType, nName)
						// Reconcile(...) should be initiated explicitly
						if _, err := r.Reconcile(kind, nName.String()); err != nil {
							klog.Errorf("RCL error: %s", err)
						}
					}
				}
			case watch.Added, watch.Modified:
				if _, err := r.Reconcile(kind, nName.String()); err != nil {
					klog.Errorf("RCL error: %s", err)
				}
			}
		}
	}
}

func (r *fakeReconciller) Run(ctx context.Context) {
	r.Lock()
	defer r.Unlock()
	r.mainloopContext = ctx
	for kind := range r.kinds {
		list := &unstructured.UnstructuredList{}
		list.SetKind(kind)
		list.SetGroupVersionKind(*r.kinds[kind].gvk)
		watcher, err := r.client.Watch(ctx, list)
		if err != nil {
			panic(err)
		}
		r.watchersWG.Add(1)
		go r.doWatch(ctx, watcher, kind)
	}
}

// todo(sv): will be better to implement in the future
// RunAndDeferWaitToFinish -- run fakeReconciller loop and defer
// Wait(...) function with infinity time to wait.
// may be used as `defer rcl.RunAndDeferWaitToFinish(ctx)()` call
// func (r *fakeReconciller) RunAndDeferWaitToFinish(ctx context.Context) func() {
// 	r.Run(ctx)
// 	klog.Warningf("RCL: deffered waiting of finishing loops is not implementing now.")
// 	return func() {}
// }

// Wait -- wait to finish all running fake reconcile loops
// and user requested create/reconcile calls. Like sync.Wait()
// context, passed to Run(...) will be used to cancel all waiters.
func (r *fakeReconciller) Wait() {
	r.userTasksWG.Wait() // should be before watchersWG.Wait() !!!
	r.watchersWG.Wait()
}

func (r *fakeReconciller) AddController(gvk *schema.GroupVersionKind, rcl NativeReconciller) error {
	kind := gvk.Kind
	if k, ok := r.kinds[kind]; ok {
		return fmt.Errorf("Kind '%s' already set up (%s)", kind, k.gvk.String()) //nolint
	}

	r.kinds[kind] = &kindWatcherData{
		kind:           kind,
		askToReconcile: make(chan *reconcileRequest, ControlChanBuffSize),
		processedObjs:  map[string]*reconcileStatus{},
		gvk:            gvk,
		reconciler:     rcl,
	}
	return nil
}

func NewFakeReconciller(fakeClient client.WithWatch, scheme *runtime.Scheme) FakeReconciller {
	rv := &fakeReconciller{
		kinds:  map[string]*kindWatcherData{},
		scheme: scheme,
		client: fakeClient,
	}
	return rv
}

func (r *ReconcileResponce) String() string {
	return fmt.Sprintf("{Err:%v  Requeue:%v/%v  Took:%v}", r.Err, r.Result.Requeue, r.Result.RequeueAfter, r.StartFinishTime[1].Sub(r.StartFinishTime[0]))
}

func ensureUID(ctx context.Context, cl client.WithWatch, obj client.Object) {
	uid := obj.GetUID()
	if uid == "" {
		obj.SetUID(apimTypes.UID(uuid.NewString()))
		if err := cl.Update(ctx, obj); err != nil {
			klog.Errorf("RCL: unable to fix UID on '%s/%s': %s", obj.GetNamespace(), obj.GetName(), err)
		} else {
			klog.Warningf("RCL: UID fixed on '%s/%s': %s", obj.GetNamespace(), obj.GetName(), obj.GetUID())
		}
	}
}

package fakereconciler

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"sort"
	"strconv"
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
	klog "k8s.io/klog/v2"
	controllerRTclient "sigs.k8s.io/controller-runtime/pkg/client"
	controllerRTreconcile "sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	BasePauseTime       = 373 // ms
	ControlChanBuffSize = 255

	MsgUnableToWatch        = "Unable to watch"
	MsgMainLoopIsNotStarted = "MainLoop is not started"
)

type reconcileRequest struct {
	Key      string
	RespChan chan *ReconcileResponce
}

type reconcileStatus struct {
	log   []ReconcileResponce
	nName apimTypes.NamespacedName
}

type kindWatcherData struct {
	sync.Mutex
	gvk            *schema.GroupVersionKind
	kind           string
	askToReconcile chan *reconcileRequest
	reconciler     controllerRTreconcile.Reconciler
	processedObjs  map[string]*reconcileStatus
}

type fakeReconciler struct {
	sync.Mutex
	scheme          *runtime.Scheme
	kinds           map[string]*kindWatcherData
	client          controllerRTclient.WithWatch
	mainloopContext context.Context //nolint: containedctx
	watchersWG      sync.WaitGroup
	userTasksWG     sync.WaitGroup
}

func (r *fakeReconciler) GetClient() controllerRTclient.WithWatch {
	return r.client
}

func (r *fakeReconciler) GetScheme() *runtime.Scheme {
	return r.scheme
}

func (r *fakeReconciler) getKindStruct(kind string) (*kindWatcherData, error) {
	r.Lock()
	defer r.Unlock()
	rv, ok := r.kinds[kind]
	if !ok {
		return nil, fmt.Errorf("Kind '%s' does not served by this reconcile loop, %w", kind, k8t.ErrorDoNothing)
	}
	return rv, nil
}

func (r *kindWatcherData) GetObj(key string) (*reconcileStatus, bool) {
	r.Lock()
	defer r.Unlock()
	rv, ok := r.processedObjs[key]
	return rv, ok
}

func (r *kindWatcherData) DeleteObj(key string) error {
	r.Lock()
	defer r.Unlock()

	if _, ok := r.processedObjs[key]; !ok {
		return fmt.Errorf("%s '%s' %w", r.kind, key, k8t.ErrorNotFound)
	}
	delete(r.processedObjs, key)
	return nil
}

func (r *fakeReconciler) doReconcile(ctx context.Context, kindName string, obj controllerRTclient.Object) *ReconcileResponce {
	var (
		err       error
		res       controllerRTreconcile.Result
		startTime time.Time
		endTime   time.Time
	)

	kindWatcherData, err := r.getKindStruct(kindName)
	if err != nil {
		return &ReconcileResponce{Err: err, StartFinishTime: k8t.TimeInterval{startTime, time.Now()}}
	}

	nName, err := utils.GetRuntimeObjectNamespacedName(obj)
	if err != nil {
		return &ReconcileResponce{Err: err, StartFinishTime: k8t.TimeInterval{startTime, time.Now()}}
	}

	objRec, ok := kindWatcherData.GetObj(nName.String())
	if !ok {
		kindWatcherData.Lock()
		objRec = &reconcileStatus{}
		kindWatcherData.processedObjs[nName.String()] = objRec
		kindWatcherData.Unlock()
	}

	startTime = time.Now().UTC()
	objRec.nName = nName

	ensureRequiredMetaFields(ctx, r.client, obj)
	res, err = kindWatcherData.reconciler.Reconcile(ctx, controllerRTreconcile.Request{NamespacedName: nName})
	endTime = time.Now().UTC()

	rResponse := ReconcileResponce{
		Result:          res,
		Err:             err,
		StartFinishTime: k8t.TimeInterval{startTime, endTime},
	}
	objRec.log = append(objRec.log, rResponse)

	return &rResponse
}

// Watch C/R/M/D events from fakeClient or user request.
// run one instance per Kind type
func (r *fakeReconciler) doWatch(ctx context.Context, watcher watch.Interface, kind string) {
	defer r.watchersWG.Done()
	r.Lock()
	kindWD := r.kinds[kind]
	r.Unlock()

	if kindWD.reconciler == nil {
		panic(fmt.Sprintf("Native reconciler for %s undefined.", kind))
	}

	for {
		select {
		case <-ctx.Done():
			watcher.Stop()
			klog.Infof("RCL: Watcher for %s finished", kind)
			return
		case req := <-kindWD.askToReconcile:
			var rv *ReconcileResponce
			if req.RespChan == nil {
				klog.Errorf("RCL: Unable to process reconcile request for %s '%s': empty response chan given", kind, req.Key)
				break
			}

			klog.Infof("RCL: %s '%s' going to reconcile", kind, req.Key)
			nName := utils.KeyToNamespacedName(req.Key)
			obj := &unstructured.Unstructured{}
			obj.SetGroupVersionKind(*kindWD.gvk)
			if err := r.client.Get(ctx, nName, obj); err != nil {
				klog.Errorf("RCL: Unable to process reconcile request for %s '%s': %s", kind, req.Key, err)
				now := time.Now()
				rv = &ReconcileResponce{Err: err, StartFinishTime: k8t.TimeInterval{now, now}}
			} else {
				// object fetched, call Reconcile(...)
				rv = r.doReconcile(ctx, kind, obj)
				msg := fmt.Sprintf("RCL: %s '%s' Reconcile result: %s", kind, req.Key, rv)
				if rv.Err != nil {
					klog.Errorf(msg)
				} else {
					klog.Infof(msg)
				}
			}
			req.RespChan <- rv  // should
			close(req.RespChan) // be together
		case in := <-watcher.ResultChan():
			nName, err := utils.GetRuntimeObjectNamespacedName(in.Object)
			if err != nil {
				panic("Wrong object passed from watcher")
			}
			k8sObj, ok := in.Object.(controllerRTclient.Object)
			if !ok {
				panic("Wrong object passed from watcher")
			}
			klog.Infof("RCL-ev: %s  %s '%s'", in.Type, kindWD.kind, nName)
			switch in.Type {
			case watch.Deleted:
				if len(k8sObj.GetFinalizers()) == 0 {
					// no finalizers, object will be deleted by fake controllerRTclient
					// fakeReconciler should to forget about deleted object
					kwd, err := r.getKindStruct(nName.String())
					if err == nil {
						if err := kwd.DeleteObj(nName.String()); err != nil {
							klog.Errorf("RCL: Unable to delete object record: %s", err)
						}
					}
					klog.Infof("RCL: deletion of [%s] '%s' done, no finalizers.", kindWD.kind, nName)
				} else {
					// at least one finalizer found, DeletionTimestamp of the object should be set if absent
					klog.Infof("RCL: deletion of [%s] '%s' done, there are %v finalizers found.", kindWD.kind, nName, k8sObj.GetFinalizers())
					if k8sObj.GetDeletionTimestamp().IsZero() {
						now := metav1.Now()
						k8sObj.SetDeletionTimestamp(&now)
						if err := r.client.Update(ctx, k8sObj); err != nil && !utils.IsNotFound(err) {
							klog.Errorf("RCL: Obj '%s' deletion error: %s", nName, err)
						}
						// MODIFY event will initiate Reconcile automatically
						klog.Infof("RCL: DeletionTimestamp of [%s] '%s' is set.", kindWD.kind, nName)
					} else {
						klog.Infof("RCL: DeletionTimestamp of [%s] '%s' is already found, the object was planned to delete earlier, try to reconcile.", kindWD.kind, nName)
						// Reconcile(...) should be initiated explicitly
						if _, err := r.reconcile(kind, nName.String()); err != nil {
							klog.Errorf("RCL error: %s", err)
						}
					}
				}
			case watch.Added, watch.Modified:
				if _, err := r.reconcile(kind, nName.String()); err != nil {
					klog.Errorf("RCL error: %s", err)
				}
			case watch.Bookmark, watch.Error:
				fallthrough //nolint: gocritic
			default:
				klog.Warning("RCL: unsupported event")
			}
		}
	}
}

func (r *fakeReconciler) reconcile(kind, key string) (chan *ReconcileResponce, error) {
	var respChan chan *ReconcileResponce

	if r.mainloopContext == nil {
		return nil, fmt.Errorf("Unable to reconcile, MainLoop is not started")
	}
	if err := r.mainloopContext.Err(); err != nil {
		return nil, fmt.Errorf("Unable to reconcile: %w", err)
	}

	rr, err := r.getKindStruct(kind)
	if err != nil {
		return nil, err
	}
	respChan = make(chan *ReconcileResponce, 1) // buffered to push-and-close result
	rr.askToReconcile <- &reconcileRequest{
		Key:      key,
		RespChan: respChan,
	}
	return respChan, nil
}

func (r *fakeReconciler) Run(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("Unable to run fakeReconciler: %w", err)
	}

	deadline, ok := ctx.Deadline()
	if !ok {
		return fmt.Errorf("Unable to run fakeReconciler: context deadline not set")
	}

	klog.Infof(" ") // klog, sometime, eats 1st line of log. why not .
	klog.Infof("RCL-LOOP: running, deadline expired in %v", time.Until(deadline).Round(time.Second))
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
	return nil
}

// todo(sv): will be better to implement in the future
// RunAndDeferWaitToFinish -- run fakeReconciler loop and defer
// Wait(...) function with infinity time to wait.
// may be used as `defer rcl.RunAndDeferWaitToFinish(ctx)()` call
// func (r *fakeReconciler) RunAndDeferWaitToFinish(ctx context.Context) func() {
// 	r.Run(ctx)
// 	klog.Warningf("RCL: deffered waiting of finishing loops is not implementing now.")
// 	return func() {}
// }

// Wait -- wait to finish all running fake reconcile loops
// and user requested create/reconcile calls. Like sync.Wait()
// context, passed to Run(...) will be used to cancel all waiters.
func (r *fakeReconciler) Wait() {
	r.userTasksWG.Wait() // should be before watchersWG.Wait() !!!
	r.watchersWG.Wait()
}

func (r *fakeReconciler) AddControllerByType(m schema.ObjectKind, rcl controllerRTreconcile.Reconciler) error {
	gvk := m.GroupVersionKind()
	return r.AddController(&gvk, rcl)
}

func (r *fakeReconciler) AddController(gvk *schema.GroupVersionKind, rcl controllerRTreconcile.Reconciler) error {
	kind := gvk.Kind
	if k, ok := r.kinds[kind]; ok {
		return fmt.Errorf("Kind '%s' already set up (%s)", kind, k.gvk.String())
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

func NewFakeReconciler(fakeClient controllerRTclient.WithWatch, scheme *runtime.Scheme) FakeReconciler {
	rv := &fakeReconciler{
		kinds:  map[string]*kindWatcherData{},
		scheme: scheme,
		client: fakeClient,
	}
	return rv
}

func (r *ReconcileResponce) String() string {
	return fmt.Sprintf("{Err:%v  Requeue:%v/%v  Took:%v}", r.Err, r.Result.Requeue, r.Result.RequeueAfter, r.StartFinishTime[1].Sub(r.StartFinishTime[0]))
}

func ensureRequiredMetaFields(ctx context.Context, cl controllerRTclient.WithWatch, obj controllerRTclient.Object) {
	const (
		operation  = "replace"
		pathPrefix = "/metadata/"
	)
	type patchSection struct {
		Op    string `json:"op"`
		Path  string `json:"path"`
		Value any    `json:"value"`
	}
	type patchSet []patchSection

	objType := obj.GetObjectKind().GroupVersionKind().Kind
	patch := patchSet{}

	if obj.GetUID() == "" {
		patch = append(patch, patchSection{Path: "uid", Value: uuid.NewString()})
	}
	if ts := obj.GetCreationTimestamp(); ts.IsZero() {
		patch = append(patch, patchSection{Path: "creationTimestamp", Value: time.Now().UTC()})
	}
	if obj.GetGeneration() < 1 {
		patch = append(patch, patchSection{Path: "generation", Value: int64(1)})
	}
	if g, err := strconv.Atoi(obj.GetResourceVersion()); err != nil || g < 1 {
		patch = append(patch, patchSection{Path: "resourceVersion", Value: fmt.Sprint(6000 + rand.Intn(100))}) //nolint
	}
	if len(patch) > 0 {
		fields := sort.StringSlice{}
		for k := range patch {
			patch[k].Op = operation
			fields = append(fields, patch[k].Path)
			patch[k].Path = pathPrefix + patch[k].Path
		}
		fields.Sort()
		buff, err := json.Marshal(patch)
		if err != nil {
			klog.Errorf("RCL: unable to marshal Meta patch for %s '%s/%s': %s", objType, obj.GetNamespace(), obj.GetName(), err)
		}
		if err := cl.Patch(ctx, obj, controllerRTclient.RawPatch(apimTypes.JSONPatchType, buff)); err != nil {
			klog.Errorf("RCL: unable to fix %v Meta fields for %s '%s/%s': %s", fields, objType, obj.GetNamespace(), obj.GetName(), err)
		} else {
			klog.Warningf("RCL: fixed Meta fields %v for %s '%s/%s'", fields, objType, obj.GetNamespace(), obj.GetName())
		}
	}
}

func GetPauseTime() time.Duration {
	return time.Duration(BasePauseTime+rand.Intn(BasePauseTime/4)) * time.Millisecond //nolint
}

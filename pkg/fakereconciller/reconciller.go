package fakereconciller

import (
	"context"
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
	scheme *runtime.Scheme
	kinds  map[string]*kindWatcherData
	client client.WithWatch
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

func (r *fakeReconciller) doReconcile(ctx context.Context, kindName string, obj client.Object, respChan chan *ReconcileResponce) error {
	var (
		err       error
		res       reconcile.Result
		startTime time.Time
		endTime   time.Time
	)

	kindWatcherData, err := r.getKindStruct(kindName)
	if err != nil {
		return err
	}

	nName, err := utils.GetRuntimeObjectNamespacedName(obj)
	if err != nil {
		return err
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
	if respChan != nil {
		respChan <- &ReconcileResponce{
			Result:          res,
			Err:             err,
			StartFinishTime: k8t.TimeInterval{startTime, endTime},
		}
		close(respChan)
	}
	return err
}

func (r *fakeReconciller) doWatch(ctx context.Context, watcher watch.Interface, kind string) {
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
			klog.V(4).Infof("RCL: %s finished", kind)
			return
		case req := <-taskChan:
			klog.Infof("RCL: Ask to reconcile: %s '%s'", kind, req.Key)
			nName := utils.KeyToNamespacedName(req.Key)
			obj := &unstructured.Unstructured{}
			r.Lock()
			obj.SetGroupVersionKind(*r.kinds[kind].gvk)
			r.Unlock()
			if err := r.client.Get(ctx, nName, obj); err != nil {
				klog.Errorf("RCL error: %s", err)
				break
			}

			if err := r.doReconcile(ctx, kind, obj, req.RespChan); err != nil {
				klog.Errorf("RCL error: %s", err)
			}
		case in := <-watcher.ResultChan():
			// do_lock()
			// defer do_unlock()
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
			klog.Infof("RCL: income event: %s [%s] '%s'", in.Type, objType, nName)
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
				if err := r.doReconcile(ctx, kind, k8sObj, nil); err != nil {
					klog.Errorf("RCL error: %s", err)
				}
			}
		}
	}
}

func (r *fakeReconciller) Run(ctx context.Context) {
	r.Lock()
	defer r.Unlock()
	for kind := range r.kinds {
		list := &unstructured.UnstructuredList{}
		list.SetKind(kind)
		list.SetGroupVersionKind(*r.kinds[kind].gvk)
		watcher, err := r.client.Watch(ctx, list)
		if err != nil {
			panic(err)
		}
		go r.doWatch(ctx, watcher, kind)
	}
}

func (r *fakeReconciller) RunAndDeferWaitToFinish(ctx context.Context) func() {
	r.Run(ctx)
	klog.Warningf("RCL: deffered waiting of finishing loops is not implementing now.")
	return func() {}
}

// WaitToFinish -- wait to successfully finish all running fake reconcile loops
// and user requested create/reconcile calls.
func (r *fakeReconciller) WaitToFinish(waitTime time.Duration) error {
	klog.Warningf("RCL: waiting of finishing loops is not implementing now.")
	return nil
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

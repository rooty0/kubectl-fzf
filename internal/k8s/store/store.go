package store

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/rooty0/kubectl-fzf/v3/internal/k8s/resources"
	"github.com/rooty0/kubectl-fzf/v3/internal/util"

	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/cache"
)

// Store stores the current state of k8s resources
type Store struct {
	data         map[string]resources.K8sResource
	resourceCtor func(obj interface{}, config resources.CtorConfig) resources.K8sResource
	ctorConfig   resources.CtorConfig
	resourceType resources.ResourceType
	currentFile  *os.File
	storeConfig  *StoreConfig
	firstWrite   bool

	// mutex guards data, dumpRequired and lastFullDump. They are touched by the
	// watch handlers, the poller, the dump ticker and the stats HTTP handler,
	// which all run on their own goroutine.
	mutex sync.RWMutex

	dumpRequired bool
	lastFullDump time.Time
}

// NewStore creates a new store
func NewStore(
	ctx context.Context,
	storeConfig *StoreConfig,
	ctorConfig resources.CtorConfig,
	resourceType resources.ResourceType,
) *Store {
	k := Store{}
	k.data = make(map[string]resources.K8sResource)
	k.resourceCtor = resources.ResourceTypeToCtor(resourceType)
	k.resourceType = resourceType
	k.currentFile = nil
	k.storeConfig = storeConfig
	k.firstWrite = true
	k.ctorConfig = ctorConfig
	k.lastFullDump = time.Time{}
	go k.fullDumpTicker(ctx)

	return &k
}

func (k *Store) fullDumpTicker(ctx context.Context) {
	timeBetweenFullDump := k.storeConfig.GetTimeBetweenFullDump()
	logrus.Debugf("Starting ticker loop for %s: will do full dump every %s", k.resourceType, timeBetweenFullDump)
	t := time.NewTicker(timeBetweenFullDump)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			logrus.Debugf("Stopping full dump ticker for %s", k.resourceType)
			return
		case <-t.C:
			if err := k.DumpFullState(); err != nil {
				logrus.Errorf("Full dump of %s failed: %v", k.resourceType, err)
			}
		}
	}
}

func resourceKey(obj interface{}) string {
	name := "None"
	namespace := "None"
	switch v := obj.(type) {
	case metav1.ObjectMetaAccessor:
		o := v.GetObjectMeta()
		namespace = o.GetNamespace()
		name = o.GetName()
	case *unstructured.Unstructured:
		metadata := v.Object["metadata"].(map[string]interface{})
		name = metadata["name"].(string)
		namespace = metadata["namespace"].(string)
	default:
		logrus.Warningf("Unknown type %v", obj)
	}
	return fmt.Sprintf("%s_%s", namespace, name)
}

// AddResourceList clears current state add the objects to the store.
// It will trigger a full dump
// This is used for polled resources
func (k *Store) AddResourceList(lstRuntime []runtime.Object) {
	data := make(map[string]resources.K8sResource, len(lstRuntime))
	for _, runtimeObject := range lstRuntime {
		key := resourceKey(runtimeObject)
		resource := k.resourceCtor(runtimeObject, k.ctorConfig)
		data[key] = resource
	}
	k.mutex.Lock()
	defer k.mutex.Unlock()
	k.data = data
	k.dumpRequired = true
}

// AddResource adds a new k8s object to the store
func (k *Store) AddResource(obj interface{}) {
	key := resourceKey(obj)
	newObj := k.resourceCtor(obj, k.ctorConfig)
	logrus.Tracef("%s added: %s", k.resourceType, key)
	k.mutex.Lock()
	defer k.mutex.Unlock()
	k.data[key] = newObj
	k.dumpRequired = true
}

// DeleteResource removes an existing k8s object to the store
func (k *Store) DeleteResource(obj interface{}) {
	key := "Unknown"
	switch v := obj.(type) {
	case cache.DeletedFinalStateUnknown:
		key = resourceKey(v.Obj)
	case unstructured.Unstructured:
	case metav1.ObjectMetaAccessor:
		key = resourceKey(obj)
	default:
		logrus.Debugf("Unknown object type %v", obj)
		return
	}
	logrus.Tracef("%s deleted: %s", k.resourceType, key)
	k.mutex.Lock()
	defer k.mutex.Unlock()
	delete(k.data, key)
	k.dumpRequired = true
}

// UpdateResource update an existing k8s object
func (k *Store) UpdateResource(oldObj, newObj interface{}) {
	key := resourceKey(newObj)
	k8sObj := k.resourceCtor(newObj, k.ctorConfig)
	k.mutex.Lock()
	defer k.mutex.Unlock()
	// An update for a resource the store has never seen is a change by
	// definition. Asking HasChanged is not an option: most implementations
	// type-assert their argument and panic on the nil held for a missing key.
	if old, ok := k.data[key]; ok && !k8sObj.HasChanged(old) {
		return
	}
	logrus.Tracef("%s changed: %s", k.resourceType, key)
	k.data[key] = k8sObj
	k.dumpRequired = true
}

func (k *Store) GetStats() *Stats {
	k.mutex.RLock()
	defer k.mutex.RUnlock()
	itemPerNamespaces := make(map[string]int, 0)
	for _, r := range k.data {
		namespace := r.GetNamespace()
		_, ok := itemPerNamespaces[namespace]
		if !ok {
			itemPerNamespaces[namespace] = 1
		} else {
			itemPerNamespaces[namespace]++
		}
	}
	return &Stats{
		ResourceType:     k.resourceType,
		ItemPerNamespace: itemPerNamespaces,
		LastDumped:       k.lastFullDump,
	}
}

// DumpFullState writes the full state to the cache file
func (k *Store) DumpFullState() error {
	k.mutex.Lock()
	defer k.mutex.Unlock()
	if !k.dumpRequired {
		logrus.Tracef("No change of %s detected, skipping dump", k.resourceType)
		return nil
	}
	now := time.Now()
	delta := now.Sub(k.lastFullDump)
	if delta < k.storeConfig.GetTimeBetweenFullDump() {
		logrus.Infof("Last full dump for %s happened %s ago, ignoring it", k.resourceType, delta)
		return nil
	}
	k.dumpRequired = false
	k.lastFullDump = now
	logrus.Infof("Doing full dump of %d %s", len(k.data), k.resourceType)
	destFile := k.storeConfig.GetResourceStorePath(k.resourceType)
	return util.EncodeToFile(k.data, destFile)
}

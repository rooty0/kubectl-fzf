package store

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"sync"
	"testing"
	"time"

	"github.com/rooty0/kubectl-fzf/v3/internal/k8s/clusterconfig"
	"github.com/rooty0/kubectl-fzf/v3/internal/k8s/resources"
	"github.com/rooty0/kubectl-fzf/v3/internal/util"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestMain(m *testing.M) {
	logrus.SetLevel(logrus.DebugLevel)
	code := m.Run()
	os.Exit(code)
}

func TestDumpAPIResources(t *testing.T) {
	resource := map[string]resources.K8sResource{}

	list := resources.APIResourceList{}
	list.GroupVersion = "v1"

	a := resources.APIResource{}
	a.Shortnames = []string{"short"}
	a.Name = "name"
	list.ApiResources = append(list.ApiResources, a)

	resource["v1"] = &list
	tempDir, err := ioutil.TempDir("/tmp/", "cacheTest")
	require.NoError(t, err)

	apiResourcesFilePath := path.Join(tempDir, "apiresources")
	err = util.EncodeToFile(resource, apiResourcesFilePath)
	require.NoError(t, err)

	loadResource := map[string]resources.K8sResource{}
	err = util.LoadGobFromFile(&loadResource, apiResourcesFilePath)
	require.NoError(t, err)
}

// TestUpdateResourceOfUnknownKey covers an update arriving for a resource the
// store does not hold, which happens after a delete or a poller replacing the
// whole map. Most HasChanged implementations type-assert their argument, so
// handing them the nil of a missing key used to panic the whole server.
func TestUpdateResourceOfUnknownKey(t *testing.T) {
	tempDir, err := ioutil.TempDir("/tmp/", "cacheTest")
	require.NoError(t, err)
	defer util.RemoveTempDir(tempDir)

	storeConfig := NewStoreConfig(&StoreConfigCli{
		ClusterConfigCli: &clusterconfig.ClusterConfigCli{
			ClusterName: "test", CacheDir: tempDir},
		TimeBetweenFullDump: time.Hour,
	})
	require.NoError(t, storeConfig.CreateDestDir())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	k8sStore := NewStore(ctx, storeConfig, resources.CtorConfig{}, resources.ResourceTypePod)

	pod := corev1.Pod{
		TypeMeta:   metav1.TypeMeta{Kind: "Pod"},
		ObjectMeta: metav1.ObjectMeta{Name: "Test1", Namespace: "ns1"},
	}
	require.NotPanics(t, func() { k8sStore.UpdateResource(&pod, &pod) })
	assert.Contains(t, k8sStore.data, "ns1_Test1", "the unknown resource should be stored")
}

// TestConcurrentAccess exercises the goroutines that touch a store at the same
// time in production: watch handlers mutating resources, the dump ticker
// encoding the map and the stats HTTP handler ranging over it. Run under -race,
// it guards the store's locking. Without it, GetStats ranging over the map
// while a watch handler writes to it takes the whole server down with a fatal
// "concurrent map iteration and map write".
func TestConcurrentAccess(t *testing.T) {
	tempDir, err := ioutil.TempDir("/tmp/", "cacheTest")
	require.NoError(t, err)
	defer util.RemoveTempDir(tempDir)

	storeConfig := NewStoreConfig(&StoreConfigCli{
		ClusterConfigCli: &clusterconfig.ClusterConfigCli{
			ClusterName: "test", CacheDir: tempDir},
		TimeBetweenFullDump: time.Millisecond,
	})
	require.NoError(t, storeConfig.CreateDestDir())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	k8sStore := NewStore(ctx, storeConfig, resources.CtorConfig{}, resources.ResourceTypePod)

	newPod := func(i int) *corev1.Pod {
		return &corev1.Pod{
			TypeMeta: metav1.TypeMeta{Kind: "Pod"},
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("pod-%d", i),
				Namespace: fmt.Sprintf("ns-%d", i%4),
			},
		}
	}

	const iterations = 300
	var wg sync.WaitGroup
	for _, worker := range []func(i int){
		func(i int) { k8sStore.AddResource(newPod(i)) },
		func(i int) { k8sStore.UpdateResource(newPod(i), newPod(i)) },
		func(i int) { k8sStore.DeleteResource(newPod(i)) },
		func(i int) { k8sStore.AddResourceList([]runtime.Object{newPod(i), newPod(i + 1)}) },
		func(i int) { k8sStore.GetStats() },
		func(i int) { assert.NoError(t, k8sStore.DumpFullState()) },
	} {
		wg.Add(1)
		go func(work func(i int)) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				work(i)
			}
		}(worker)
	}
	wg.Wait()
}

// TestFullDumpTickerStopsOnContextCancel guards against the dump ticker
// outliving its store. Stores are recreated whenever the watched cluster
// changes, so a ticker that ignores its context leaks a goroutine and keeps
// the whole resource map of the discarded store alive.
func TestFullDumpTickerStopsOnContextCancel(t *testing.T) {
	tempDir, err := ioutil.TempDir("/tmp/", "cacheTest")
	require.NoError(t, err)
	defer util.RemoveTempDir(tempDir)

	storeConfig := NewStoreConfig(&StoreConfigCli{
		ClusterConfigCli: &clusterconfig.ClusterConfigCli{
			ClusterName: "test", CacheDir: tempDir},
		TimeBetweenFullDump: 50 * time.Millisecond,
	})
	require.NoError(t, storeConfig.CreateDestDir())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	k8sStore := NewStore(ctx, storeConfig, resources.CtorConfig{}, resources.ResourceTypePod)

	pod := corev1.Pod{
		TypeMeta:   metav1.TypeMeta{Kind: "Pod"},
		ObjectMeta: metav1.ObjectMeta{Name: "Test1", Namespace: "ns1"},
	}
	k8sStore.AddResource(&pod)

	podFilePath := path.Join(tempDir, "test", "pods")
	require.Eventually(t, func() bool {
		_, err := os.Stat(podFilePath)
		return err == nil
	}, 2*time.Second, 20*time.Millisecond, "ticker should dump while the context is alive")

	cancel()
	time.Sleep(200 * time.Millisecond)

	k8sStore.AddResource(&pod)
	infoBefore, err := os.Stat(podFilePath)
	require.NoError(t, err)
	time.Sleep(300 * time.Millisecond)
	infoAfter, err := os.Stat(podFilePath)
	require.NoError(t, err)
	assert.Equal(t, infoBefore.ModTime(), infoAfter.ModTime(),
		"ticker should stop dumping once its context is cancelled")
}

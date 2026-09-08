package storetest

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/rooty0/kubectl-fzf/v3/internal/k8s/clusterconfig"
	"github.com/rooty0/kubectl-fzf/v3/internal/k8s/resources"
	"github.com/rooty0/kubectl-fzf/v3/internal/k8s/store"
)

// labelApp is the label key the storetest fixtures organize pods by.
const labelApp = "app"

func podResource(name string, ns string, labels map[string]string) corev1.Pod {
	meta := corev1.Pod{
		TypeMeta: metav1.TypeMeta{Kind: "Pod"},
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         ns,
			Labels:            labels,
			CreationTimestamp: metav1.Time{Time: time.Now()},
		},
		Spec:   corev1.PodSpec{},
		Status: corev1.PodStatus{},
	}
	return meta
}

func GetTestPodStore(t *testing.T) (string, *store.Store) {
	tempDir := t.TempDir()
	storeConfigCli := &store.StoreConfigCli{
		ClusterConfigCli: &clusterconfig.ClusterConfigCli{
			ClusterName: "test", CacheDir: tempDir},
		TimeBetweenFullDump: 500 * time.Millisecond}
	storeConfig := store.NewStoreConfig(storeConfigCli)
	err := storeConfig.CreateDestDir()
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	ctorConfig := resources.CtorConfig{}
	k8sStore := store.NewStore(ctx, storeConfig, ctorConfig, resources.ResourceTypePod)
	pods := []corev1.Pod{
		podResource("Test1", "ns1", map[string]string{labelApp: "app1"}),
		podResource("Test2", "ns2", map[string]string{labelApp: "app2"}),
		podResource("Test3", "ns2", map[string]string{labelApp: "app2"}),
		podResource("Test4", "aaa", map[string]string{labelApp: "app3"}),
	}
	for _, pod := range pods {
		k8sStore.AddResource(&pod)
	}
	return tempDir, k8sStore
}

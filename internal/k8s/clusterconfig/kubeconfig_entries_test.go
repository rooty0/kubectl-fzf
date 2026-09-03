package clusterconfig

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func loadedTestConfig(t *testing.T) ClusterConfig {
	t.Helper()
	t.Setenv("KUBECONFIG", "testdata/kubeconfig")
	c := NewClusterConfig(&ClusterConfigCli{ClusterName: "minikube", CacheDir: "testdata"})
	require.NoError(t, c.LoadClusterConfig())
	return c
}

func TestGetContextEntries(t *testing.T) {
	c := loadedTestConfig(t)

	entries, err := c.GetContextEntries()

	require.NoError(t, err)
	require.Equal(t, []ContextEntry{
		{Name: "minikube", Cluster: "minikube", Namespace: "kube-system", User: "minikube"},
		// A context naming no namespace is the default one, which is what kubectl
		// would act on, so that is what the picker has to show.
		{Name: "no-namespace", Cluster: "minikube", Namespace: "default", User: "minikube"},
		{Name: "prod", Cluster: "prod", Namespace: "prod-ns", User: "prod-token"},
	}, entries)
}

func TestGetClusterEntries(t *testing.T) {
	c := loadedTestConfig(t)

	entries, err := c.GetClusterEntries()

	require.NoError(t, err)
	require.Equal(t, []ClusterEntry{
		{Name: "minikube", Server: "https://192.168.49.2:8443"},
		{Name: "prod", Server: "https://prod.example.com"},
	}, entries)
}

func TestGetUserEntries(t *testing.T) {
	c := loadedTestConfig(t)

	entries, err := c.GetUserEntries()

	require.NoError(t, err)
	require.Equal(t, []UserEntry{
		// A kubeconfig of exec users is told apart by the plugin they run.
		{Name: "exec-user", Auth: "exec:aws-iam-authenticator"},
		{Name: "minikube", Auth: "-"},
		{Name: "prod-token", Auth: "token"},
	}, entries)
}

// TestKubeconfigEntriesWithoutLoadedConfig covers the same ground as the
// GetNamespace case: these read apiConfig, so they must report the config was
// never loaded rather than dereference nil.
func TestKubeconfigEntriesWithoutLoadedConfig(t *testing.T) {
	c := NewClusterConfig(&ClusterConfigCli{ClusterName: "minikube", CacheDir: "testdata"})

	contexts, err := c.GetContextEntries()
	require.Error(t, err)
	assert.Empty(t, contexts)

	clusters, err := c.GetClusterEntries()
	require.Error(t, err)
	assert.Empty(t, clusters)

	users, err := c.GetUserEntries()
	require.Error(t, err)
	assert.Empty(t, users)
}

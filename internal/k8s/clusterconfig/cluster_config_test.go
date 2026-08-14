package clusterconfig

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetNamespaceWithoutLoadedConfig(t *testing.T) {
	c := NewClusterConfig(&ClusterConfigCli{ClusterName: "minikube", CacheDir: "testdata"})

	ns, err := c.GetNamespace()

	require.Error(t, err, "GetNamespace must not panic when the kubeconfig was never loaded")
	assert.Empty(t, ns)
}

func TestGetNamespaceFromLoadedConfig(t *testing.T) {
	t.Setenv("KUBECONFIG", "testdata/kubeconfig")
	c := NewClusterConfig(&ClusterConfigCli{ClusterName: "minikube", CacheDir: "testdata"})
	require.NoError(t, c.LoadClusterConfig())

	ns, err := c.GetNamespace()

	require.NoError(t, err)
	assert.Equal(t, "kube-system", ns)
	assert.Equal(t, "minikube", c.GetContext())
}

func TestGetNamespaceOfUnknownContext(t *testing.T) {
	t.Setenv("KUBECONFIG", "testdata/kubeconfig")
	c := NewClusterConfig(&ClusterConfigCli{ClusterName: "minikube", CacheDir: "testdata"})
	require.NoError(t, c.LoadClusterConfig())
	c.apiConfig.CurrentContext = "does-not-exist"

	_, err := c.GetNamespace()

	require.Error(t, err)
}

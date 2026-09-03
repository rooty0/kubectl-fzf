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
	c.clusterName = "does-not-exist"

	_, err := c.GetNamespace()

	require.Error(t, err)
}

// A --context on the command line being completed asks about another cluster,
// which means another cache directory and another default namespace.
func TestSetContext(t *testing.T) {
	t.Setenv("KUBECONFIG", "testdata/kubeconfig")
	c := NewClusterConfig(&ClusterConfigCli{ClusterName: "minikube", CacheDir: "testdata"})
	require.NoError(t, c.LoadClusterConfig())
	require.True(t, c.IsCurrentContext())

	require.NoError(t, c.SetContext("prod"))

	assert.Equal(t, "prod", c.GetContext())
	assert.Equal(t, "testdata/prod", c.destDir)
	assert.False(t, c.IsCurrentContext(), "prod is not what the kubeconfig calls current")
	ns, err := c.GetNamespace()
	require.NoError(t, err)
	assert.Equal(t, "prod-ns", ns, "the namespace has to come from the context asked for")
}

func TestSetContextOfUnknownName(t *testing.T) {
	t.Setenv("KUBECONFIG", "testdata/kubeconfig")
	c := NewClusterConfig(&ClusterConfigCli{ClusterName: "minikube", CacheDir: "testdata"})
	require.NoError(t, c.LoadClusterConfig())

	err := c.SetContext("does-not-exist")

	require.Error(t, err)
	assert.Equal(t, "minikube", c.GetContext(), "a refused context must change nothing")
	assert.Equal(t, "testdata/minikube", c.destDir)
}

func TestSetContextWithoutLoadedConfig(t *testing.T) {
	c := NewClusterConfig(&ClusterConfigCli{ClusterName: "minikube", CacheDir: "testdata"})

	require.Error(t, c.SetContext("prod"))
}

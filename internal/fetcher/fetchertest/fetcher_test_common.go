package fetchertest

import (
	"fmt"
	"testing"

	"github.com/rooty0/kubectl-fzf/v3/internal/fetcher"
	"github.com/rooty0/kubectl-fzf/v3/internal/k8s/clusterconfig"
	"github.com/stretchr/testify/require"
)

func GetTestFetcher(t *testing.T, clusterName string, port int) (*fetcher.Fetcher, string) {
	tempDir := t.TempDir()
	fetchCli := &fetcher.FetcherCli{
		FetcherCachePath: tempDir,
		ClusterConfigCli: &clusterconfig.ClusterConfigCli{
			ClusterName: clusterName,
			CacheDir:    "testdata",
		},
		HttpEndpoint: fmt.Sprintf("localhost:%d", port),
	}
	f := fetcher.NewFetcher(fetchCli)
	return f, tempDir
}

// GetTestFetcherWithDefaults returns a fetcher with its kubeconfig loaded from
// the caller's testdata. Completion defaults to the current context's namespace,
// so the tests need a context with a known name and namespace.
func GetTestFetcherWithDefaults(t *testing.T) *fetcher.Fetcher {
	const kubeconfigPath = "testdata/kubeconfig"
	require.FileExists(t, kubeconfigPath,
		"GetTestFetcherWithDefaults needs a kubeconfig fixture in the calling package's testdata")
	t.Setenv("KUBECONFIG", kubeconfigPath)

	f, _ := GetTestFetcher(t, "minikube", 18080)
	require.NoError(t, f.LoadClusterConfig())
	return f
}

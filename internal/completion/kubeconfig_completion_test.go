package completion

import (
	"context"
	"testing"

	"github.com/bonnefoa/kubectl-fzf/v3/internal/fetcher/fetchertest"
	"github.com/bonnefoa/kubectl-fzf/v3/internal/parse"
	"github.com/stretchr/testify/require"
)

func TestProcessContext(t *testing.T) {
	fetchConfig := fetchertest.GetTestFetcherWithDefaults(t)
	cmdArgs := []cmdArg{
		{"get", []string{"pods", "--context", ""}},
		{"logs", []string{"mypod", "--context", " "}},
		{"get", []string{"pods", "-n", "kube-system", "--context", ""}},
	}
	for _, cmdArg := range cmdArgs {
		completionResults, err := processCommandArgsWithFetchConfig(context.Background(), fetchConfig, cmdArg.verb, cmdArg.args)
		require.NoError(t, err, "args %s", cmdArg.args)
		require.Equal(t, "Context\tCluster\tNamespace\tUser", completionResults.Header)
		require.Equal(t, []string{
			"minikube\tminikube\tkube-system\tminikube",
			"staging\tstaging\tdefault\tstaging-user",
		}, completionResults.Completions)
	}
}

func TestProcessCluster(t *testing.T) {
	fetchConfig := fetchertest.GetTestFetcherWithDefaults(t)

	completionResults, err := processCommandArgsWithFetchConfig(context.Background(), fetchConfig,
		"get", []string{"pods", "--cluster", ""})

	require.NoError(t, err)
	require.Equal(t, "Cluster\tServer", completionResults.Header)
	require.Equal(t, []string{
		"minikube\thttps://192.168.49.2:8443",
		"staging\thttps://staging.example.com",
	}, completionResults.Completions)
}

func TestProcessUser(t *testing.T) {
	fetchConfig := fetchertest.GetTestFetcherWithDefaults(t)

	completionResults, err := processCommandArgsWithFetchConfig(context.Background(), fetchConfig,
		"get", []string{"pods", "--user", ""})

	require.NoError(t, err)
	require.Equal(t, "User\tAuth", completionResults.Header)
	require.Equal(t, []string{"minikube\t-", "staging-user\ttoken"}, completionResults.Completions)
}

// TestKubeconfigCompletionNeedsNoResource pins the reason this path exists: the
// answer comes from the kubeconfig, so it holds even for a verb that names no
// resource at all, where resource completion would give up.
func TestKubeconfigCompletionNeedsNoResource(t *testing.T) {
	fetchConfig := fetchertest.GetTestFetcherWithDefaults(t)

	completionResults, err := processCommandArgsWithFetchConfig(context.Background(), fetchConfig,
		"config", []string{"view", "--context", ""})

	require.NoError(t, err)
	require.Len(t, completionResults.Completions, 2)
}

func TestKubeconfigCompletionOfAnotherFlag(t *testing.T) {
	fetchConfig := fetchertest.GetTestFetcherWithDefaults(t)

	_, _, err := kubeconfigCompletion(parse.FlagLabel, fetchConfig)

	require.Error(t, err)
}

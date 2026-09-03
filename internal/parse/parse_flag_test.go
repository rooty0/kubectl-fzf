package parse

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUnmanagedArgs(t *testing.T) {
	cmdArgs := [][]string{
		{"-t"},
		{"-i"},
		{"--field-selector"},
		{"--selector"},
	}
	for _, args := range cmdArgs {
		r := CheckFlagManaged("get", args)
		require.Equal(t, FlagUnmanaged.String(), r.String())
	}
}

type flagTest struct {
	verb   string
	flag   []string
	result FlagCompletion
}

func TestManagedArgs(t *testing.T) {
	cmdArgs := []flagTest{
		{"get", []string{"--selector="}, FlagLabel},
		{"get", []string{"--field-selector", ""}, FlagFieldSelector},
		{"get", []string{"--field-selector="}, FlagFieldSelector},
		{"get", []string{"--all-namespaces", ""}, FlagNone},
		{"get", []string{"-t", ""}, FlagNone},
		{"get", []string{"-i", ""}, FlagNone},
		{"get", []string{"-ti", ""}, FlagNone},
		{"get", []string{"-it", ""}, FlagNone},
		{"get", []string{"-n"}, FlagNamespace},
		{"get", []string{"-n="}, FlagNamespace},
		{"get", []string{"-n", " "}, FlagNamespace},
		{"get", []string{"--namespace", ""}, FlagNamespace},
	}
	for _, args := range cmdArgs {
		r := CheckFlagManaged(args.verb, args.flag)
		require.Equal(t, args.result.String(), r.String(), "args %s", args.flag)
	}
}

// TestKubeconfigFlagValues pins the flags kubectl-fzf has no cache for. Before
// they were known, "get pods --context <TAB>" offered the pod list where a
// context name belongs.
func TestKubeconfigFlagValues(t *testing.T) {
	flags := []string{
		"--kubeconfig", "--server", "--token", "--as", "--as-group",
		"--request-timeout", "--username",
	}
	for _, flag := range flags {
		r := CheckFlagManaged("get", []string{"pods", flag, ""})
		require.Equal(t, FlagUnmanaged.String(), r.String(), "flag %s", flag)
	}
}

// TestKubeconfigNamedValues covers the three whose values the kubeconfig names,
// so they are completed from it rather than handed back to kubectl.
func TestKubeconfigNamedValues(t *testing.T) {
	cmdArgs := []flagTest{
		{"get", []string{"pods", "--context", ""}, FlagContext},
		{"get", []string{"pods", "--cluster", ""}, FlagCluster},
		{"get", []string{"pods", "--user", ""}, FlagUser},
		// A value already typed is a query, not a new position.
		{"get", []string{"pods", "--context", "mini"}, FlagContext},
		{"logs", []string{"mypod", "--context", ""}, FlagContext},
	}
	for _, args := range cmdArgs {
		r := CheckFlagManaged(args.verb, args.flag)
		require.Equal(t, args.result.String(), r.String(), "args %s", args.flag)
		require.True(t, r.IsKubeconfig(), "%s should be a kubeconfig completion", r)
	}
	require.False(t, FlagNamespace.IsKubeconfig(),
		"a namespace lives in the cluster, not in the kubeconfig")
	require.False(t, FlagNone.IsKubeconfig())
}

// TestFlagCompletionString guards the names against the enum growing out of
// step with them, which would have gone unnoticed and read as "Unknown".
func TestFlagCompletionString(t *testing.T) {
	require.Equal(t, "Context", FlagContext.String())
	require.Equal(t, "Cluster", FlagCluster.String())
	require.Equal(t, "User", FlagUser.String())
	require.Equal(t, "Unknown", FlagCompletion(99).String())
}

// TestBooleanFlagsStillCompleteResources guards the other direction: a flag that
// stands alone leaves the next word a resource position, so the fzf picker has
// to stay.
func TestBooleanFlagsStillCompleteResources(t *testing.T) {
	flags := []string{
		"-A", "--all-namespaces", "--all", "--show-labels", "--no-headers",
		"-w", "--watch", "--ignore-not-found", "-R", "--recursive",
		"--insecure-skip-tls-verify", "--dry-run", "--show-managed-fields",
	}
	for _, flag := range flags {
		r := CheckFlagManaged("get", []string{"pods", flag, ""})
		require.Equal(t, FlagNone.String(), r.String(), "flag %s", flag)
	}
}

func TestPreviousFlagCompletion(t *testing.T) {
	cmdArgs := []flagTest{
		// A value already attached leaves the next word a resource position.
		{"get", []string{"pods", "--output=json", ""}, FlagNone},
		{"get", []string{"pods", "-nkube-system", ""}, FlagNone},
		{"get", []string{"pods", "-oyaml", ""}, FlagNone},
		// A cluster of booleans ending on a value taking flag is governed by
		// that last flag.
		{"exec", []string{"mypod", "-tic", ""}, FlagUnmanaged},
		{"get", []string{"pods", "-An", ""}, FlagNamespace},
		{"get", []string{"pods", "-Al", ""}, FlagLabel},
		// An unknown flag must not claim the word after it.
		{"get", []string{"pods", "--newly-added-kubectl-flag", ""}, FlagUnmanaged},
		{"get", []string{"pods", "-Z", ""}, FlagUnmanaged},
		// Same letter, different command.
		{"logs", []string{"mypod", "-f", ""}, FlagNone},
		{"get", []string{"pods", "-f", ""}, FlagUnmanaged},
		{"logs", []string{"mypod", "-p", ""}, FlagNone},
		{"patch", []string{"deploy", "mydeploy", "-p", ""}, FlagUnmanaged},
		{"logs", []string{"mypod", "-c", ""}, FlagUnmanaged},
		{"logs", []string{"mypod", "--tail", ""}, FlagUnmanaged},
		{"get", []string{"pods", "--sort-by", ""}, FlagUnmanaged},
		{"get", []string{"pods", "--template", ""}, FlagUnmanaged},
		// Everything past -- is positional, so kubectl knows better.
		{"exec", []string{"mypod", "--", ""}, FlagUnmanaged},
	}
	for _, args := range cmdArgs {
		r := CheckFlagManaged(args.verb, args.flag)
		require.Equal(t, args.result.String(), r.String(), "%s %s", args.verb, args.flag)
	}
}

// TestKubeconfigFlagsComeFromClientGo checks the flag knowledge is really being
// taken from client-go, so a client-go bump keeps it current rather than
// silently leaving it stale.
func TestKubeconfigFlagsComeFromClientGo(t *testing.T) {
	flagSet := kubeconfigFlags()
	require.NotNil(t, flagSet.Lookup("context"), "client-go no longer registers --context")
	require.NotNil(t, flagSet.Lookup("kubeconfig"))
	require.Equal(t, "namespace", shorthandName("get", 'n'))
	require.Equal(t, flagKindValue, flagKindOf("context"))
	require.Equal(t, flagKindBoolean, flagKindOf("insecure-skip-tls-verify"))
	require.Equal(t, flagKindUnknown, flagKindOf("newly-added-kubectl-flag"))
}

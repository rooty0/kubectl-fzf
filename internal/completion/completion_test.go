package completion

import (
	"context"
	"os"
	"path"
	"sort"
	"strings"
	"testing"

	"github.com/bonnefoa/kubectl-fzf/v3/internal/fetcher/fetchertest"
	"github.com/bonnefoa/kubectl-fzf/v3/internal/httpserver/httpservertest"
	"github.com/bonnefoa/kubectl-fzf/v3/internal/k8s/resources"
	"github.com/bonnefoa/kubectl-fzf/v3/internal/parse"
	"github.com/sirupsen/logrus"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	logrus.SetLevel(logrus.DebugLevel)
	code := m.Run()
	os.Exit(code)
}

type cmdArg struct {
	verb string
	args []string
}

func TestParseRequest(t *testing.T) {
	testDatas := []struct {
		name               string
		cmdArgs            []string
		expectedWords      []string
		expectedStructured bool
	}{
		{
			name:          "legacy single string, as the bash plugin sends it",
			cmdArgs:       []string{"get pods "},
			expectedWords: []string{"get", "pods", " "},
		},
		{
			name:               "words, one per argv entry",
			cmdArgs:            []string{"--protocol=2", "--", "get", "pods", ""},
			expectedWords:      []string{"get", "pods", " "},
			expectedStructured: true,
		},
		{
			// The whole point of the words protocol: a value holding a space is
			// one word, which a single command line string cannot express.
			name:               "a value holding a space stays one word",
			cmdArgs:            []string{"--protocol=2", "--", "get", "pods", "-l", "app=my app"},
			expectedWords:      []string{"get", "pods", "-l", "app=my app"},
			expectedStructured: true,
		},
		{
			// Only the first separator introduces the words, a later one is part
			// of the command line itself.
			name:               "a second separator belongs to the command line",
			cmdArgs:            []string{"--protocol=2", "--", "exec", "mypod", "--", "sh", ""},
			expectedWords:      []string{"exec", "mypod", "--", "sh", " "},
			expectedStructured: true,
		},
		{
			name:          "words without asking for the structured response",
			cmdArgs:       []string{"--", "get", "pods", ""},
			expectedWords: []string{"get", "pods", " "},
		},
		{
			name:               "a word that is only being started",
			cmdArgs:            []string{"--protocol=2", "--", "get", ""},
			expectedWords:      []string{"get", " "},
			expectedStructured: true,
		},
	}
	for _, testData := range testDatas {
		t.Run(testData.name, func(t *testing.T) {
			request := ParseRequest(testData.cmdArgs)
			require.NotNil(t, request)
			assert.Equal(t, testData.expectedWords, request.Words)
			assert.Equal(t, testData.expectedStructured, request.Structured)
		})
	}
}

func TestParseRequestCursor(t *testing.T) {
	testDatas := []struct {
		name           string
		cmdArgs        []string
		expectedWords  []string
		expectedCursor int
	}{
		{
			// The cursor is what tells the words being completed from the ones
			// that only say what the line is about.
			name:           "a cursor left of the last word",
			cmdArgs:        []string{"--cursor=1", "--", "get", "sca", "-n", "kube-system"},
			expectedWords:  []string{"get", "sca", "-n", "kube-system"},
			expectedCursor: 1,
		},
		{
			name:           "no cursor sent leaves it on the last word",
			cmdArgs:        []string{"--", "get", "pods", "cor"},
			expectedWords:  []string{"get", "pods", "cor"},
			expectedCursor: 2,
		},
		{
			name:           "a word being started under the cursor",
			cmdArgs:        []string{"--cursor=2", "--", "get", "pods", "", "-n", "kube-system"},
			expectedWords:  []string{"get", "pods", " ", "-n", "kube-system"},
			expectedCursor: 2,
		},
		{
			// A shell miscounting must not take the completion down with it.
			name:           "a cursor past the last word falls back to it",
			cmdArgs:        []string{"--cursor=9", "--", "get", "pods"},
			expectedWords:  []string{"get", "pods"},
			expectedCursor: 1,
		},
		{
			name:           "a cursor that is not a number is ignored",
			cmdArgs:        []string{"--cursor=x", "--", "get", "pods"},
			expectedWords:  []string{"get", "pods"},
			expectedCursor: 1,
		},
		{
			// --context is a word of the command line, not an option of ours.
			name:           "an option looking word past the separator",
			cmdArgs:        []string{"--protocol=2", "--cursor=3", "--", "get", "pods", "--context", "prod"},
			expectedWords:  []string{"get", "pods", "--context", "prod"},
			expectedCursor: 3,
		},
	}
	for _, testData := range testDatas {
		t.Run(testData.name, func(t *testing.T) {
			request := ParseRequest(testData.cmdArgs)
			require.NotNil(t, request)
			assert.Equal(t, testData.expectedWords, request.Words)
			assert.Equal(t, testData.expectedCursor, request.Cursor)
		})
	}
}

func TestParseRequestWithNothingToComplete(t *testing.T) {
	testDatas := []struct {
		name    string
		cmdArgs []string
	}{
		{"no arguments at all", []string{}},
		{"only the protocol flag", []string{"--protocol=2"}},
		{"a separator with no words behind it", []string{"--protocol=2", "--"}},
		{"several strings, none of them words", []string{"get pods", "extra"}},
	}
	for _, testData := range testDatas {
		t.Run(testData.name, func(t *testing.T) {
			assert.Nil(t, ParseRequest(testData.cmdArgs))
		})
	}
}

func TestPrepareCmdArgs(t *testing.T) {
	testDatas := []struct {
		cmdArgs        []string
		expectedResult []string
	}{
		{[]string{"get pods"}, []string{"get", "pods"}},
		{[]string{"get pods "}, []string{"get", "pods", " "}},
	}
	for _, testData := range testDatas {
		cmdArgs := PrepareCmdArgs(testData.cmdArgs)
		require.Equal(t, testData.expectedResult, cmdArgs)
	}

}

func TestProcessResourceName(t *testing.T) {
	fetchConfig := fetchertest.GetTestFetcherWithDefaults(t)
	cmdArgs := []cmdArg{
		{"get", []string{"pods", ""}},
		{"get", []string{"po", ""}},
		{"logs", []string{""}},
		{"exec", []string{"-ti", ""}},
	}
	for _, cmdArg := range cmdArgs {
		completionResults, err := processCommandArgsWithFetchConfig(context.Background(), fetchConfig, cmdArg.verb, cmdArg.args, -1)
		require.NoError(t, err)
		require.Greater(t, len(completionResults.Completions), 0)
		require.Contains(t, completionResults.Completions[0], "kube-system\tcoredns-6d4b75cb6d-m6m4q\t172.17.0.3\t192.168.49.2\tminikube\tRunning\tBurstable\tcoredns\tCriticalAddonsOnly:,node-role.kubernetes.io/master:NoSchedule,node-role.kubernetes.io/control-plane:NoSchedule\tNone")
	}
}

// A flag sitting to the right of the cursor still says which namespace the line
// is about, but it is not the thing being completed: "get pods <TAB> -n kube-system"
// asks for the pods of kube-system, not for a namespace.
func TestProcessResourceWithFlagsAfterCursor(t *testing.T) {
	fetchConfig := fetchertest.GetTestFetcherWithDefaults(t)
	testDatas := []struct {
		name   string
		args   []string
		cursor int
	}{
		{"a word being started", []string{"pods", " ", "-n", "kube-system"}, 1},
		{"a name being typed", []string{"pods", "cor", "-n", "kube-system"}, 1},
		{"a flag further right still", []string{"pods", " ", "-n", "kube-system", "-o", "yaml"}, 1},
	}
	for _, testData := range testDatas {
		t.Run(testData.name, func(t *testing.T) {
			completionResults, err := processCommandArgsWithFetchConfig(context.Background(),
				fetchConfig, "get", testData.args, testData.cursor)
			require.NoError(t, err)
			require.Greater(t, len(completionResults.Completions), 0)
			assert.Equal(t, resources.ResourceToHeader(resources.ResourceTypePod), completionResults.Header)
			for _, completion := range completionResults.Completions {
				assert.True(t, strings.HasPrefix(completion, "kube-system\t"),
					"completion %q should be filtered to the namespace named after the cursor", completion)
			}
		})
	}
}

// A --context on the line asks about another cluster, so the answer comes from
// that context's own cache directory.
func TestProcessResourceOfAnotherContext(t *testing.T) {
	fetchConfig := fetchertest.GetTestFetcherWithDefaults(t)
	require.NoError(t, fetchConfig.SetContext("staging"))

	_, err := processCommandArgsWithFetchConfig(context.Background(),
		fetchConfig, "get", []string{"pods", " "}, -1)

	// staging has no cache, and the running kubectl-fzf server answers for the
	// current context alone. Saying nothing sends the shell to kubectl's own
	// completion; answering would mean minikube's pods under a staging heading.
	require.Error(t, err)
	assert.Contains(t, err.Error(), "staging")
}

// The reported line, "k get sca<TAB> -n kube-system": the cursor is on the resource
// type, so the resource types are what gets listed, and the flag behind it must
// not turn the request into a namespace completion.
func TestProcessResourceTypeWithFlagsAfterCursor(t *testing.T) {
	fetchConfig := fetchertest.GetTestFetcherWithDefaults(t)
	completionResults, err := processCommandArgsWithFetchConfig(context.Background(),
		fetchConfig, "get", []string{"sca", "-n", "kube-system"}, 0)
	require.NoError(t, err)
	require.Greater(t, len(completionResults.Completions), 0)
	assert.Equal(t, resources.ResourceToHeader(resources.ResourceTypeApiResource), completionResults.Header)
}

// The same words with the cursor at the end are a namespace being completed, so
// the cursor is what decides, not the words.
func TestProcessNamespaceValueUnderTheCursor(t *testing.T) {
	fetchConfig := fetchertest.GetTestFetcherWithDefaults(t)
	completionResults, err := processCommandArgsWithFetchConfig(context.Background(),
		fetchConfig, "get", []string{"pods", "-n", "kube-system"}, 2)
	require.NoError(t, err)
	assert.Equal(t, resources.ResourceToHeader(resources.ResourceTypeNamespace), completionResults.Header)
}

func TestProcessNamespace(t *testing.T) {
	fetchConfig := fetchertest.GetTestFetcherWithDefaults(t)
	cmdArgs := []cmdArg{
		{"get", []string{"pods", "-n"}},
		{"get", []string{"pods", "-n", " "}},
		{"get", []string{"po", "-n="}},
		{"logs", []string{"--namespace", ""}},
		{"logs", []string{"--namespace="}},
	}
	for _, cmdArg := range cmdArgs {
		completionResults, err := processCommandArgsWithFetchConfig(context.Background(), fetchConfig, cmdArg.verb, cmdArg.args, -1)
		require.NoError(t, err)
		require.Greater(t, len(completionResults.Completions), 0)
		require.Contains(t, completionResults.Completions[0], "default\t")
	}
}

func TestProcessLabelCompletion(t *testing.T) {
	fetchConfig := fetchertest.GetTestFetcherWithDefaults(t)
	cmdArgs := []cmdArg{
		{"get", []string{"pods", "-l="}},
		{"get", []string{"pods", "-l"}},
		{"get", []string{"pods", "-l", ""}},
		{"get", []string{"pods", "--selector", ""}},
		{"get", []string{"pods", "--selector="}},
	}
	for _, cmdArg := range cmdArgs {
		completionResults, err := processCommandArgsWithFetchConfig(context.Background(), fetchConfig, cmdArg.verb, cmdArg.args, -1)
		require.NoError(t, err)
		require.Equal(t, "kube-system\ttier=control-plane\t4", completionResults.Completions[0])
		require.Len(t, completionResults.Completions, 12)
	}
}

func TestProcessFieldSelectorCompletion(t *testing.T) {
	fetchConfig := fetchertest.GetTestFetcherWithDefaults(t)
	cmdArgs := []cmdArg{
		{"get", []string{"pods", "--field-selector", ""}},
		{"get", []string{"pods", "--field-selector="}},
	}
	for _, cmdArg := range cmdArgs {
		completionResults, err := processCommandArgsWithFetchConfig(context.Background(), fetchConfig, cmdArg.verb, cmdArg.args, -1)
		require.NoError(t, err)
		assert.Equal(t, "kube-system\tspec.nodeName=minikube\t7", completionResults.Completions[0])
	}
}

func TestUnmanagedCompletion(t *testing.T) {
	fetchConfig := fetchertest.GetTestFetcherWithDefaults(t)
	cmdArgs := []cmdArg{
		{"get", []string{"-t"}},
		{"get", []string{"-i"}},
		{"get", []string{"--field-selector"}},
		{"get", []string{"--selector"}},
		{"get", []string{"--all-namespaces"}},
		{"get", []string{"pods", "aPod", ">", "/tmp"}},
	}
	for _, cmdArg := range cmdArgs {
		_, err := processCommandArgsWithFetchConfig(context.Background(), fetchConfig, cmdArg.verb, cmdArg.args, -1)
		require.Errorf(t, err, "cmdArgs %s should have returned unmanaged", cmdArg)
		require.IsType(t, parse.UnmanagedFlagError(""), err)
	}
}

func TestManagedCompletion(t *testing.T) {
	fetchConfig := fetchertest.GetTestFetcherWithDefaults(t)
	cmdArgs := []cmdArg{
		{"get", []string{"pods", "--selector", ""}},
		{"get", []string{"pods", "--selector="}},
		{"get", []string{"pods", "--field-selector", ""}},
		{"get", []string{"pods", "--field-selector="}},
		{"get", []string{"pods", "-t", ""}},
		{"get", []string{"pods", "-i", ""}},
		{"get", []string{"pods", "-ti", ""}},
		{"get", []string{"pods", "-it", ""}},
		{"get", []string{"-n"}},
		{"get", []string{"-n", ""}},
		{"get", []string{"pods", "--all-namespaces", ""}},
	}
	for _, cmdArg := range cmdArgs {
		completionResults, err := processCommandArgsWithFetchConfig(context.Background(), fetchConfig, cmdArg.verb, cmdArg.args, -1)
		require.NoError(t, err)
		require.NotNil(t, completionResults)
	}
}

func TestPodCompletionFile(t *testing.T) {
	fetchConfig := fetchertest.GetTestFetcherWithDefaults(t)
	res, err := getResourceCompletion(context.Background(), resources.ResourceTypePod, nil, fetchConfig)
	require.NoError(t, err)
	t.Log(res)
	assert := assert.New(t)
	assert.Contains(res[0], "kube-system\t")
	assert.Len(res, 7)
}

func TestNamespaceFilterFile(t *testing.T) {
	fetchConfig := fetchertest.GetTestFetcherWithDefaults(t)

	// everything is filtered
	namespace := "test"
	res, err := getResourceCompletion(context.Background(), resources.ResourceTypePod, &namespace, fetchConfig)
	require.NoError(t, err)
	t.Log(res)
	assert := assert.New(t)
	assert.Len(res, 0)

	// all results match
	namespace = "kube-system"
	res, err = getResourceCompletion(context.Background(), resources.ResourceTypePod, &namespace, fetchConfig)
	assert.Len(res, 7)
	require.NoError(t, err)
}

func TestApiResourcesFile(t *testing.T) {
	fetchConfig := fetchertest.GetTestFetcherWithDefaults(t)
	res, err := getResourceCompletion(context.Background(), resources.ResourceTypeApiResource, nil, fetchConfig)
	require.NoError(t, err)
	assert := assert.New(t)
	sort.Strings(res)
	assert.Contains(res[0], "apiservices\tNone\tapiregistration.k8s.io/v1\tfalse\tAPIService")
}

func TestHttpServerApiCompletion(t *testing.T) {
	fzfHttpServer := httpservertest.StartTestHttpServer(t)
	f, tempDir := fetchertest.GetTestFetcher(t, "nothing", fzfHttpServer.Port)
	res, err := getResourceCompletion(context.Background(), resources.ResourceTypeApiResource, nil, f)
	require.NoError(t, err)
	sort.Strings(res)
	assert.Contains(t, res[0], "apiservices\tNone\tapiregistration.k8s.io/v1\tfalse\tAPIService")
	assert.Len(t, res, 56)

	expectedPath := path.Join(tempDir, "nothing", resources.ResourceTypeApiResource.String())
	assert.FileExists(t, expectedPath)
}

func TestHttpServerPodCompletion(t *testing.T) {
	fzfHttpServer := httpservertest.StartTestHttpServer(t)
	f, tempDir := fetchertest.GetTestFetcher(t, "nothing", fzfHttpServer.Port)
	res, err := getResourceCompletion(context.Background(), resources.ResourceTypePod, nil, f)
	require.NoError(t, err)
	assert.Contains(t, res[0], "kube-system\t")
	assert.Len(t, res, 7)

	expectedPath := path.Join(tempDir, "nothing", resources.ResourceTypePod.String())
	assert.FileExists(t, expectedPath)
}

func TestHttpUnknownResourceCompletion(t *testing.T) {
	fzfHttpServer := httpservertest.StartTestHttpServer(t)
	f, tempDir := fetchertest.GetTestFetcher(t, "nothing", fzfHttpServer.Port)
	_, err := getResourceCompletion(context.Background(), resources.ResourceTypePersistentVolume, nil, f)
	require.Error(t, err)

	expectedPath := path.Join(tempDir, "nothing")
	assert.NoFileExists(t, expectedPath)
}

func TestHttpServerCachePod(t *testing.T) {
	fzfHttpServer := httpservertest.StartTestHttpServer(t)
	f, tempDir := fetchertest.GetTestFetcher(t, "nothing", fzfHttpServer.Port)
	res, err := getResourceCompletion(context.Background(), resources.ResourceTypePod, nil, f)
	require.NoError(t, err)
	err = f.SaveFetcherState()
	require.NoError(t, err)
	assert.Len(t, res, 7)

	podCache := path.Join(tempDir, "nothing", resources.ResourceTypePod.String())
	assert.FileExists(t, podCache)
	require.Equal(t, fzfHttpServer.ResourceHit, 1)
	fetcher_state := path.Join(tempDir, "fetcher_state")
	assert.FileExists(t, fetcher_state)

	res, err = getResourceCompletion(context.Background(), resources.ResourceTypePod, nil, f)
	require.Equal(t, fzfHttpServer.ResourceHit, 1)
}

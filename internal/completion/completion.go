package completion

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/rooty0/kubectl-fzf/v3/internal/fetcher"
	"github.com/rooty0/kubectl-fzf/v3/internal/k8s/resources"
	"github.com/rooty0/kubectl-fzf/v3/internal/parse"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

const (
	// StructuredProtocolFlag asks for the key=value response instead of the bare
	// completion string. It lets the shell be told about words it has to drop
	// from the command line, which a single completion string cannot express.
	StructuredProtocolFlag = "--protocol=2"

	// WordsSeparator introduces the command line to complete, one word per argv
	// entry. Handing the words over individually is what keeps a value holding a
	// space, such as -l 'app=my app', in one piece.
	WordsSeparator = "--"

	// CursorFlagPrefix says which word is being completed. Without it the cursor
	// is taken to be on the last word, which is where a shell that only sends the
	// left half of the line leaves it.
	CursorFlagPrefix = "--cursor="
)

// Request is what the shell asked for.
type Request struct {
	// Words is the command line to complete, the kubectl verb first. A word that
	// is only being started is a single space.
	Words []string
	// Cursor indexes Words at the word being completed. The words after it are
	// part of the request too: they say which namespace or context the line is
	// about, even though they are not what is being completed.
	Cursor int
	// Structured asks for the key=value response.
	Structured bool
}

// ParseRequest reads the arguments of the k8s_completion subcommand, and returns
// nil when there is nothing to complete. That subcommand has flag parsing
// disabled so the command line reaches us untouched, hence doing this by hand.
func ParseRequest(cmdArgs []string) *Request {
	request := &Request{Cursor: -1}
	// Only the options in front of the words are ours. Past the separator every
	// argument belongs to the command line, --context included.
	for len(cmdArgs) > 0 && cmdArgs[0] != WordsSeparator && strings.HasPrefix(cmdArgs[0], "--") {
		option := cmdArgs[0]
		cmdArgs = cmdArgs[1:]
		if option == StructuredProtocolFlag {
			request.Structured = true
			continue
		}
		value, isCursor := strings.CutPrefix(option, CursorFlagPrefix)
		if !isCursor {
			logrus.Warnf("Ignoring unknown completion option %s", option)
			continue
		}
		cursor, err := strconv.Atoi(value)
		if err != nil {
			logrus.Warnf("Ignoring malformed %s: %s", option, err)
			continue
		}
		request.Cursor = cursor
	}
	request.Words = parseWords(cmdArgs)
	if len(request.Words) == 0 {
		return nil
	}
	if request.Cursor < 0 || request.Cursor >= len(request.Words) {
		request.Cursor = len(request.Words) - 1
	}
	// A word being started is empty, and the rest of the code has always known
	// that state as a single space.
	if request.Words[request.Cursor] == "" {
		request.Words[request.Cursor] = " "
	}
	return request
}

func parseWords(cmdArgs []string) []string {
	// The first separator introduces the words: a later one belongs to the
	// command line itself, as in "kubectl exec pod -- sh".
	for i, arg := range cmdArgs {
		if arg == WordsSeparator {
			return prepareCmdWords(cmdArgs[i+1:])
		}
	}
	return PrepareCmdArgs(cmdArgs)
}

// prepareCmdWords copies the words handed over one argv entry each, so that the
// normalisation done afterwards does not write into the caller's slice.
func prepareCmdWords(words []string) []string {
	if len(words) == 0 {
		return nil
	}
	args := make([]string, len(words))
	copy(args, words)
	return args
}

// PrepareCmdArgs splits a command line that arrived as a single string, which is
// what the bash plugin sends. Word boundaries are guessed here, so a quoted value
// holding a space does not survive; the words protocol above avoids that.
func PrepareCmdArgs(cmdArgs []string) []string {
	if len(cmdArgs) != 1 {
		return nil
	}
	argsStr := cmdArgs[0]

	args := strings.Fields(argsStr)
	if strings.HasSuffix(argsStr, " ") {
		args = append(args, " ")
	}
	return args
}

func getResourceCompletion(ctx context.Context, r resources.ResourceType, namespace *string,
	fetchConfig *fetcher.Fetcher) ([]string, error) {
	resources, err := fetchConfig.GetResources(ctx, r)
	if err != nil {
		return nil, err
	}
	comps := []string{}
	if namespace == nil {
		logrus.Debug("Filtering disabled, listing all namespaces")
	} else {
		logrus.Debugf("Filtering with namespace %s", *namespace)
	}
	for _, resource := range resources {
		if namespace == nil || *namespace == resource.GetNamespace() {
			comps = append(comps, resource.ToStrings()...)
		}
	}
	return comps, nil
}

func ExtractQueryFromArgs(cmdArgs []string) string {
	if len(cmdArgs) == 0 {
		return ""
	}
	latestArg := cmdArgs[len(cmdArgs)-1]
	if latestArg == " " {
		return ""
	}
	// If the last token is just the namespace flag itself, don't use it as the query.
	// This covers:
	//   k get pods -n<TAB>          -> last == "-n"
	//   k get pods --namespace<TAB> -> last == "--namespace"
	if latestArg == "-n" || latestArg == "--namespace" {
		return ""
	}
	return latestArg
}

func processCommandArgsWithFetchConfig(
	ctx context.Context,
	fetchConfig *fetcher.Fetcher,
	cmdVerb string,
	args []string,
	cursor int,
) (*CompletionResult, error) {
	var err error

	// What is being completed is decided by the words up to the cursor. Anything
	// past it is context: a -n further right still says which namespace to list,
	// but it is not the thing being completed.
	completed := parse.WordsUpToCursor(args, cursor)

	// 0. Special case: completing the value of -n / --namespace
	if isNamespaceValueCompletion(completed) {
		completionResult := &CompletionResult{
			Cluster: fetchConfig.GetContext(),
		}

		// Use the Namespace resource type; this will list namespaces from the store.
		resourceType := resources.ResourceTypeNamespace
		completionResult.Header = resources.ResourceToHeader(resourceType)

		// Namespaces are cluster-scoped, so no namespace filter here.
		completionResult.Completions, err = getResourceCompletion(ctx, resourceType, nil, fetchConfig)
		if err != nil {
			return completionResult, errors.Wrap(err, "error getting namespace completion")
		}
		sort.Strings(completionResult.Completions)
		return completionResult, nil
	}

	// 1. Normal resource/flag completion flow
	resourceType, flagCompletion, err := parse.ParseFlagAndResources(cmdVerb, completed)
	if err != nil {
		return nil, err
	}
	logrus.Debugf("Call Get Fun with %+v, resource type detected %s, flag detected %s", args, resourceType, flagCompletion)

	completionResult := &CompletionResult{Cluster: fetchConfig.GetContext()}

	// 2. Values the kubeconfig names itself, such as --context. They are known
	// without a cluster, so this comes before anything namespace related.
	if flagCompletion.IsKubeconfig() {
		completionResult.Header, completionResult.Completions, err =
			kubeconfigCompletion(flagCompletion, fetchConfig)
		return completionResult, err
	}

	// Figure out which namespace to use for completion:
	// 1) -n/--namespace wins
	// 2) -A/--all-namespaces → no filtering (all namespaces)
	// 3) otherwise, for namespaced resources, use the current context's namespace
	namespace := parse.ParseNamespaceFromArgs(args)
	allNamespaces := parse.HasAllNamespacesFlag(args)

	if allNamespaces || !resourceType.IsNamespaced() {
		// Explicitly all namespaces, or a resource type that lives in none. A
		// cluster scoped object is in no namespace, so filtering on one would
		// answer nothing at all.
		namespace = nil
	} else if namespace == nil {
		// Default to the current context's namespace for namespaced resources
		if ns, err := fetchConfig.GetNamespace(); err == nil {
			// kubeconfig can omit namespace to mean "default"
			if ns == "" {
				ns = "default"
			}
			namespace = &ns
		} else {
			// If we can't get the namespace for some reason, fall back to "all"
			logrus.Debugf("Could not determine current namespace, falling back to all: %v", err)
		}
	}

	if flagCompletion == parse.FlagLabel {
		completionResult.Header, completionResult.Completions, err = GetTagResourceCompletion(ctx, resourceType, namespace, fetchConfig, TagTypeLabel)
		return completionResult, err
	} else if flagCompletion == parse.FlagFieldSelector {
		completionResult.Header, completionResult.Completions, err = GetTagResourceCompletion(ctx, resourceType, namespace, fetchConfig, TagTypeFieldSelector)
		return completionResult, err
	}

	completionResult.Header = resources.ResourceToHeader(resourceType)
	completionResult.Completions, err = getResourceCompletion(ctx, resourceType, namespace, fetchConfig)
	if err != nil {
		return completionResult, errors.Wrap(err, "error getting resource completion")
	}
	sort.Strings(completionResult.Completions)
	return completionResult, err
}

func ProcessCommandArgs(cmdVerb string, args []string, cursor int, f *fetcher.Fetcher) (*CompletionResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	completionResult, err := processCommandArgsWithFetchConfig(ctx, f, cmdVerb, args, cursor)
	cancel()
	return completionResult, err
}

// isNamespaceValueCompletion returns true when the current completion context
// is for the value of -n/--namespace, e.g.:
//
//	get pods -n
//	get pods -n cor
func isNamespaceValueCompletion(args []string) bool {
	if len(args) == 0 {
		return false
	}
	lastIdx := len(args) - 1

	for i, arg := range args {
		if arg == "-n" || arg == "--namespace" {
			// Cases:
			//   ... -n<TAB>           -> args [..., "-n"]        (i == lastIdx)
			//   ... -n cor<TAB>       -> args [..., "-n", "cor"] (i+1 == lastIdx)
			if i == lastIdx || i+1 == lastIdx {
				return true
			}
		}
	}
	return false
}

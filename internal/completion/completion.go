package completion

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/bonnefoa/kubectl-fzf/v3/internal/fetcher"
	"github.com/bonnefoa/kubectl-fzf/v3/internal/k8s/resources"
	"github.com/bonnefoa/kubectl-fzf/v3/internal/parse"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

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
	logrus.Debugf("Filterting with namespace %v", namespace)
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
) (*CompletionResult, error) {
	var err error

	// 0. Special case: completing the value of -n / --namespace
	if isNamespaceValueCompletion(args) {
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
	resourceType, flagCompletion, err := parse.ParseFlagAndResources(cmdVerb, args)
	if err != nil {
		return nil, err
	}
	logrus.Debugf("Call Get Fun with %+v, resource type detected %s, flag detected %s", args, resourceType, flagCompletion)

	completionResult := &CompletionResult{Cluster: fetchConfig.GetContext()}

	// Figure out which namespace to use for completion:
	// 1) -n/--namespace wins
	// 2) -A/--all-namespaces → no filtering (all namespaces)
	// 3) otherwise, for namespaced resources, use the current context's namespace
	namespace := parse.ParseNamespaceFromArgs(args)
	allNamespaces := parse.HasAllNamespacesFlag(args)

	if allNamespaces {
		// Explicitly, all namespaces: do not filter
		namespace = nil
	} else if namespace == nil && resourceType.IsNamespaced() {
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

func ProcessCommandArgs(cmdVerb string, args []string, f *fetcher.Fetcher) (*CompletionResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	completionResult, err := processCommandArgsWithFetchConfig(ctx, f, cmdVerb, args)
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

package results

import (
	"fmt"
	"strings"

	"github.com/rooty0/kubectl-fzf/v3/internal/fetcher"
	"github.com/rooty0/kubectl-fzf/v3/internal/k8s/resources"
	"github.com/rooty0/kubectl-fzf/v3/internal/parse"
	"github.com/rooty0/kubectl-fzf/v3/internal/util"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/spf13/pflag"
)

// Result is what the shell has to apply to the command line.
type Result struct {
	// Completion replaces the word being completed.
	Completion string
	// RemoveWords lists arguments the shell must delete from the command line
	// because they contradict the completion. Selecting a single object pins a
	// namespace, and kubectl refuses to retrieve a resource by name across all
	// namespaces, so -A cannot survive.
	RemoveWords []string
}

// ProcessResult handles fzf output and provides completion to use
// The fzfResult should have the first 3 columns of the fzf preview
func ProcessResult(cmdUse string, cmdArgs []string, cursor int,
	f *fetcher.Fetcher, fzfResult string) (*Result, error) {
	logrus.Debugf("Processing fzf result %s", fzfResult)
	logrus.Debugf("Cmd command %s", cmdArgs)
	namespace, err := f.GetNamespace()
	if err != nil {
		return nil, err
	}
	return processResultWithNamespace(cmdUse, cmdArgs, cursor, fzfResult, namespace)
}

func parseNamespaceFlag(cmdArgs []string) (*string, error) {
	fs := pflag.NewFlagSet("f1", pflag.ContinueOnError)
	fs.ParseErrorsWhitelist.UnknownFlags = true
	cmdNamespace := fs.StringP("namespace", "n", "", "")
	logrus.Debugf("Parsing namespace from %v", cmdArgs)
	err := fs.Parse(cmdArgs)
	return cmdNamespace, err
}

func processResultWithNamespace(cmdUse string, cmdArgs []string, cursor int, fzfResult string, currentNamespace string) (*Result, error) {
	// If apiresource:
	// 0 -> fullname, 1 -> shortname, 2 -> groupversion
	// If namespaceless resource:
	// 0 -> name, 1 -> age
	// Otherwise:
	// 0 -> namespace, 1 -> value
	resultFields := strings.Fields(fzfResult)
	if len(resultFields) < 2 {
		return nil, fmt.Errorf("fzf result should have at least 3 elements, got %v", resultFields)
	}
	logrus.Debugf("Processing fzfResult '%s', cmdArgs '%s', current namespace '%s'", fzfResult, cmdArgs, currentNamespace)
	// The word being completed, and so what the fzf output means, is settled by
	// the words up to the cursor. The namespace flags read further down are read
	// from the whole line, since one sitting past the cursor binds all the same.
	completed := parse.WordsUpToCursor(cmdArgs, cursor)
	resourceType, flagCompletion, err := parse.ParseFlagAndResources(cmdUse, completed)
	if err != nil {
		return nil, err
	}
	logrus.Debugf("Resource type %s, flagCompletion %s", resourceType, flagCompletion)

	if resourceType == resources.ResourceTypeApiResource {
		return &Result{Completion: resultFields[0]}, nil
	}

	// A kubeconfig value is the name in the first column, and it pins no
	// namespace: there is nothing to suffix and nothing to drop.
	if flagCompletion.IsKubeconfig() {
		return &Result{Completion: resultFields[0]}, nil
	}

	// -A only ever selects what fzf lists. A selector value stays valid cluster
	// wide, so -A is kept and the namespace suffix dropped, since -A silently
	// overrides -n. Anything naming a single object pins a namespace, so -A goes.
	allNamespacesFlags := parse.AllNamespacesFlags(cmdArgs)
	selectorCompletion := flagCompletion == parse.FlagLabel || flagCompletion == parse.FlagFieldSelector
	keepAllNamespaces := len(allNamespacesFlags) > 0 && selectorCompletion
	var removeWords []string
	if len(allNamespacesFlags) > 0 && !keepAllNamespaces {
		removeWords = allNamespacesFlags
	}

	// Generic resource
	resultNamespace := resultFields[0]
	resultValue := resultFields[1]
	if !resourceType.IsNamespaced() {
		resultValue = resultFields[0]
		resultNamespace = ""
	}

	if resourceType == resources.ResourceTypeNamespace {
		resultValue = resultFields[0]
	}

	logrus.Debugf("Result namespace: %s, resultValue: %s", resultNamespace, resultValue)

	var cmdNamespace *string
	if flagCompletion != parse.FlagNamespace {
		cmdNamespace, err = parseNamespaceFlag(cmdArgs)
		if err != nil {
			return nil, errors.Wrapf(err, "Error parsing commands %s", cmdArgs)
		}
		logrus.Debugf("Namespace parsed: %s", *cmdNamespace)
	}
	lastWord := completed[len(completed)-1]
	// add flag to the completion
	lastFlags := []string{"-l=", "-l", "--field-selector=", "--selector=", "-n=", "--namespace=", "-n"}
	if util.IsStringIn(lastWord, lastFlags) {
		resultValue = fmt.Sprintf("%s%s", lastWord, resultValue)
	}

	if cmdNamespace != nil && *cmdNamespace == resultNamespace {
		return &Result{Completion: resultValue, RemoveWords: removeWords}, nil
	}

	if resultNamespace != currentNamespace && flagCompletion != parse.FlagNamespace && !keepAllNamespaces {
		completion := fmt.Sprintf("%s -n %s", resultValue, resultNamespace)
		return &Result{Completion: completion, RemoveWords: removeWords}, nil
	}
	return &Result{Completion: resultValue, RemoveWords: removeWords}, nil
}

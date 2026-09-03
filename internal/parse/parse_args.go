package parse

import (
	"strings"

	"github.com/bonnefoa/kubectl-fzf/v3/internal/k8s/resources"
	"github.com/sirupsen/logrus"
)

type UnmanagedFlagError string

func (u UnmanagedFlagError) Error() string {
	return string(u)
}

func ParseFlagAndResources(cmdVerb string, cmdArgs []string) (resourceType resources.ResourceType, flagCompletion FlagCompletion, err error) {
	resourceType = resources.ResourceTypeUnknown
	flagCompletion = CheckFlagManaged(cmdVerb, cmdArgs)
	if flagCompletion == FlagUnmanaged {
		logrus.Infof("Flag is unmanaged in %s, bailing out", cmdArgs)
		err = UnmanagedFlagError(strings.Join(cmdArgs, " "))
		return
	}
	logrus.Infof("Flag parsed: %s", flagCompletion.String())

	if flagCompletion == FlagNamespace {
		resourceType = resources.ResourceTypeNamespace
		return
	}
	// A kubeconfig value names no resource, so resource detection would only
	// find whatever the verb happens to mention and then be ignored.
	if flagCompletion.IsKubeconfig() {
		return
	}
	resourceType = resources.GetResourceType(cmdVerb, cmdArgs)

	if resourceType == resources.ResourceTypeUnknown {
		err = resources.UnknownResourceError{ResourceStr: strings.Join(cmdArgs, " ")}
		return
	}
	return
}

// WordsUpToCursor returns the words the completion is decided by: everything up
// to and including the one under the cursor. A cursor outside the slice is taken
// to be on the last word, which is where a shell that sends none leaves it.
func WordsUpToCursor(args []string, cursor int) []string {
	if cursor < 0 || cursor >= len(args) {
		return args
	}
	return args[:cursor+1]
}

// WordsBesideCursor returns every word but the one under the cursor. That word
// is being typed rather than meant, so nothing should be decided on it.
func WordsBesideCursor(args []string, cursor int) []string {
	if cursor < 0 || cursor >= len(args) {
		return args
	}
	beside := make([]string, 0, len(args)-1)
	beside = append(beside, args[:cursor]...)
	return append(beside, args[cursor+1:]...)
}

// ParseContextFromArgs returns the context the command line names, which is the
// cluster the completion is about whatever the kubeconfig calls current.
// Scanning stops at "--" because everything past it is handed to the executed
// command, so a "kubectl exec pod -- prog --context x" names none.
func ParseContextFromArgs(args []string) *string {
	for k, arg := range args {
		if arg == "--" {
			return nil
		}
		if value, found := strings.CutPrefix(arg, "--context="); found {
			return contextName(value)
		}
		if arg == "--context" && len(args) > k+1 {
			return contextName(args[k+1])
		}
	}
	return nil
}

// contextName rejects what cannot be one: a word only being started, and a flag,
// which is what stands where the value would be when the value is missing.
func contextName(value string) *string {
	if value == "" || value == " " || strings.HasPrefix(value, "-") {
		return nil
	}
	return &value
}

func ParseNamespaceFromArgs(args []string) *string {
	for k, arg := range args {
		if (arg == "-n" || arg == "--namespace") && len(args) > k+1 && args[k+1] != " " {
			return &args[k+1]
		}
		if strings.HasPrefix(arg, "--namespace=") {
			return &strings.Split(arg, "=")[1]
		}
	}
	return nil
}

// AllNamespacesFlags returns the arguments switching kubectl to all-namespaces
// mode, in the order they appear. Scanning stops at "--" because everything past
// it is handed to the executed command and is none of kubectl's business, so a
// "kubectl exec pod -- prog -A" must not be read as all-namespaces.
func AllNamespacesFlags(args []string) []string {
	var flags []string
	for _, arg := range args {
		if arg == "--" {
			break
		}
		name, value, hasValue := strings.Cut(arg, "=")
		if name != "-A" && name != "--all-namespaces" {
			continue
		}
		// An explicit --all-namespaces=false leaves kubectl namespace scoped.
		if hasValue && !isTruthy(value) {
			continue
		}
		flags = append(flags, arg)
	}
	return flags
}

func isTruthy(value string) bool {
	switch strings.ToLower(value) {
	case "1", "t", "true":
		return true
	}
	return false
}

func HasAllNamespacesFlag(args []string) bool {
	return len(AllNamespacesFlags(args)) > 0
}

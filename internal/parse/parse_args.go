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
	flagCompletion = CheckFlagManaged(cmdArgs)
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
	resourceType = resources.GetResourceType(cmdVerb, cmdArgs)

	if resourceType == resources.ResourceTypeUnknown {
		err = resources.UnknownResourceError{ResourceStr: strings.Join(cmdArgs, " ")}
		return
	}
	return
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

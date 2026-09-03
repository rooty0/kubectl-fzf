package parse

import (
	"strings"
	"sync"

	"github.com/sirupsen/logrus"
	"github.com/spf13/pflag"
	"k8s.io/client-go/tools/clientcmd"
)

// flagKind tells whether a flag swallows the word that follows it. Knowing this
// is what separates "kubectl get pods -A <here>", where a resource name goes,
// from "kubectl get pods --context <here>", where a context name goes.
type flagKind int

const (
	flagKindUnknown flagKind = iota
	flagKindBoolean
	flagKindValue
)

// managedFlags are the flag values kubectl-fzf completes from its own cache.
// They all take a value.
var managedFlags = map[string]FlagCompletion{
	"selector":       FlagLabel,
	"field-selector": FlagFieldSelector,
	"namespace":      FlagNamespace,
	"context":        FlagContext,
	"cluster":        FlagCluster,
	"user":           FlagUser,
}

// kubectlFlags covers the per-command kubectl flags. The kubeconfig ones are
// deliberately absent: clientcmd knows them, see kubeconfigFlags.
//
// A flag missing from here is assumed to take a value, so the boolean half is
// the one worth keeping stocked. Missing a boolean only costs the fzf picker at
// that spot, since completion is handed back to kubectl.
var kubectlFlags = map[string]flagKind{
	"all":                         flagKindBoolean,
	"all-namespaces":              flagKindBoolean,
	"allow-missing-template-keys": flagKindBoolean,
	// --dry-run reads a value but has a default for the bare form, so it never
	// consumes the next word.
	"dry-run":             flagKindBoolean,
	"follow":              flagKindBoolean,
	"force":               flagKindBoolean,
	"ignore-not-found":    flagKindBoolean,
	"local":               flagKindBoolean,
	"no-headers":          flagKindBoolean,
	"overwrite":           flagKindBoolean,
	"previous":            flagKindBoolean,
	"prune":               flagKindBoolean,
	"quiet":               flagKindBoolean,
	"recursive":           flagKindBoolean,
	"server-side":         flagKindBoolean,
	"show-kind":           flagKindBoolean,
	"show-labels":         flagKindBoolean,
	"show-managed-fields": flagKindBoolean,
	"stdin":               flagKindBoolean,
	"timestamps":          flagKindBoolean,
	"tty":                 flagKindBoolean,
	"wait":                flagKindBoolean,
	"watch":               flagKindBoolean,
	"watch-only":          flagKindBoolean,

	"chunk-size":       flagKindValue,
	"container":        flagKindValue,
	"field-manager":    flagKindValue,
	"filename":         flagKindValue,
	"kustomize":        flagKindValue,
	"limit-bytes":      flagKindValue,
	"max-log-requests": flagKindValue,
	"output":           flagKindValue,
	"patch":            flagKindValue,
	"since":            flagKindValue,
	"since-time":       flagKindValue,
	"sort-by":          flagKindValue,
	"tail":             flagKindValue,
	"template":         flagKindValue,
	"type":             flagKindValue,
}

// shorthands are the per-command short flags, again leaving the kubeconfig ones
// to clientcmd.
var shorthands = map[byte]string{
	'A': "all-namespaces",
	'R': "recursive",
	'c': "container",
	'f': "filename",
	'i': "stdin",
	'k': "kustomize",
	'l': "selector",
	'o': "output",
	'p': "previous",
	'q': "quiet",
	't': "tty",
	'w': "watch",
}

// verbShorthands holds the short flags kubectl gives a different meaning to
// depending on the command: -f is --filename for get but --follow for logs, and
// -p is --previous for logs but --patch for patch. Getting these wrong is what
// decides between offering a pod list and offering nothing.
var verbShorthands = map[string]map[byte]string{
	"attach": {'c': "container"},
	"exec":   {'c': "container"},
	"logs":   {'c': "container", 'f': "follow", 'p': "previous"},
	"patch":  {'p': "patch"},
}

var (
	kubeconfigFlagsOnce sync.Once
	kubeconfigFlagSet   *pflag.FlagSet
)

// kubeconfigFlags is kubectl's set of global flags: --context, --cluster, --user
// and the rest. Taking them from client-go rather than spelling them out here
// keeps them right, and in step with whatever client-go the build pins.
func kubeconfigFlags() *pflag.FlagSet {
	kubeconfigFlagsOnce.Do(func() {
		flagSet := pflag.NewFlagSet("kubeconfig", pflag.ContinueOnError)
		clientcmd.BindOverrideFlags(&clientcmd.ConfigOverrides{}, flagSet,
			clientcmd.RecommendedConfigOverrideFlags(""))
		// The overrides do not include the path to the file holding them.
		flagSet.String(clientcmd.RecommendedConfigPathFlag, "", "")
		kubeconfigFlagSet = flagSet
	})
	return kubeconfigFlagSet
}

func flagKindOf(name string) flagKind {
	if kind, ok := kubectlFlags[name]; ok {
		return kind
	}
	if _, ok := managedFlags[name]; ok {
		return flagKindValue
	}
	if flag := kubeconfigFlags().Lookup(name); flag != nil {
		// pflag records a default for the bare form of a boolean flag only.
		if flag.NoOptDefVal != "" {
			return flagKindBoolean
		}
		return flagKindValue
	}
	return flagKindUnknown
}

// shorthandName resolves a single letter to the long flag name it stands for,
// and returns an empty string when the letter is unknown.
func shorthandName(cmdVerb string, shorthand byte) string {
	if verb, ok := verbShorthands[cmdVerb]; ok {
		if name, ok := verb[shorthand]; ok {
			return name
		}
	}
	if name, ok := shorthands[shorthand]; ok {
		return name
	}
	if flag := kubeconfigFlags().ShorthandLookup(string(shorthand)); flag != nil {
		return flag.Name
	}
	return ""
}

// flagValueCompletion says what completing the value of a long flag means.
//
// An unknown flag is taken to swallow the next word. Guessing that way costs a
// fallback to kubectl's own completion, whereas guessing the other way offers a
// pod list where a context name belongs.
func flagValueCompletion(name string) FlagCompletion {
	if completion, ok := managedFlags[name]; ok {
		return completion
	}
	if flagKindOf(name) == flagKindBoolean {
		return FlagNone
	}
	return FlagUnmanaged
}

// shorthandCompletion resolves a cluster of short flags such as -ti or -tic.
// Only its last letter can govern the next word, every letter before that has
// to be a boolean for the cluster to be valid at all.
func shorthandCompletion(cmdVerb, cluster string) FlagCompletion {
	for i := 0; i < len(cluster); i++ {
		name := shorthandName(cmdVerb, cluster[i])
		if name == "" {
			// An unrecognised letter may well swallow the rest of the cluster, so
			// nothing that follows it can be trusted.
			return FlagUnmanaged
		}
		if i == len(cluster)-1 {
			return flagValueCompletion(name)
		}
		if flagKindOf(name) != flagKindBoolean {
			// A value taking flag before the end carries its value inside the same
			// word, as in -nkube-system, so the next word starts fresh.
			return FlagNone
		}
	}
	// A bare "-", which kubectl reads as stdin rather than as a flag.
	return FlagNone
}

// previousFlagCompletion says what the word after the given flag completes to.
func previousFlagCompletion(cmdVerb, flag string) FlagCompletion {
	completion := FlagNone
	switch {
	// --output=json carries its value, so the next word starts fresh.
	case strings.Contains(flag, "="):
	case strings.HasPrefix(flag, "--"):
		completion = flagValueCompletion(strings.TrimPrefix(flag, "--"))
	default:
		completion = shorthandCompletion(cmdVerb, strings.TrimPrefix(flag, "-"))
	}
	logrus.Debugf("Flag '%s' of '%s' precedes a %s completion", flag, cmdVerb, completion)
	return completion
}

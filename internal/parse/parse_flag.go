package parse

import (
	"strings"

	"github.com/sirupsen/logrus"
)

type FlagCompletion int

const (
	FlagLabel FlagCompletion = iota
	FlagFieldSelector
	FlagNamespace
	FlagNone
	FlagUnmanaged
	FlagContext
	FlagCluster
	FlagUser
)

func (f FlagCompletion) String() string {
	flagStr := [...]string{"Label", "FieldSelector", "Namespace", "None", "Unmanaged",
		"Context", "Cluster", "User"}
	if int(f) >= len(flagStr) {
		return "Unknown"
	}
	return flagStr[f]
}

// IsKubeconfig reports whether the value being completed is named by the
// kubeconfig rather than held in a cluster, which means it needs no cache and no
// request to answer.
func (f FlagCompletion) IsKubeconfig() bool {
	return f == FlagContext || f == FlagCluster || f == FlagUser
}

func parseLastFlag(s string) FlagCompletion {
	logrus.Debugf("Parsing last flag '%s'", s)
	switch s {
	case "-l":
		fallthrough
	case "-l=":
		fallthrough
	case "--selector=":
		return FlagLabel
	case "-n":
		fallthrough
	case "-n=":
		fallthrough
	case "--namespace=":
		return FlagNamespace
	case "--field-selector=":
		return FlagFieldSelector
	}
	return FlagUnmanaged
}

// CheckFlagManaged says whether kubectl-fzf owns the completion of the last
// word of args, and if so what that word completes to. cmdVerb is the kubectl
// command being completed, needed because a short flag such as -f or -p means
// different things from one command to the next.
func CheckFlagManaged(cmdVerb string, args []string) FlagCompletion {
	logrus.Infof("Checking Managed Flag '%s'", args)
	if len(args) == 0 {
		return FlagNone
	}
	for _, arg := range args {
		if arg == ">" {
			return FlagUnmanaged
		}
	}
	lastArg := args[len(args)-1]
	if strings.HasPrefix(lastArg, "-") {
		return parseLastFlag(lastArg)
	}
	if len(args) >= 2 {
		penultimateArg := args[len(args)-2]
		if strings.HasPrefix(penultimateArg, "-") {
			return previousFlagCompletion(cmdVerb, penultimateArg)
		}
	}
	return FlagNone
}

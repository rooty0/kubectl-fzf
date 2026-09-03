package completion

import (
	"fmt"
	"strings"

	"github.com/bonnefoa/kubectl-fzf/v3/internal/fetcher"
	"github.com/bonnefoa/kubectl-fzf/v3/internal/parse"
	"github.com/pkg/errors"
)

// kubeconfigCompletion lists what the kubeconfig names for the flag being
// completed. None of it lives in a cluster, so there is no cache to read and no
// request to make: the file has already been parsed by the time we get here.
//
// Every kind is given a second column. It carries something worth knowing, and
// a single column row would leave the caller nothing to tell a name from a
// header.
func kubeconfigCompletion(flagCompletion parse.FlagCompletion,
	fetchConfig *fetcher.Fetcher) (header string, comps []string, err error) {
	switch flagCompletion {
	case parse.FlagContext:
		entries, err := fetchConfig.GetContextEntries()
		if err != nil {
			return "", nil, errors.Wrap(err, "error listing kubeconfig contexts")
		}
		comps = make([]string, 0, len(entries))
		for _, entry := range entries {
			comps = append(comps, strings.Join([]string{
				entry.Name, entry.Cluster, entry.Namespace, entry.User}, "\t"))
		}
		return "Context\tCluster\tNamespace\tUser", comps, nil

	case parse.FlagCluster:
		entries, err := fetchConfig.GetClusterEntries()
		if err != nil {
			return "", nil, errors.Wrap(err, "error listing kubeconfig clusters")
		}
		comps = make([]string, 0, len(entries))
		for _, entry := range entries {
			comps = append(comps, strings.Join([]string{entry.Name, entry.Server}, "\t"))
		}
		return "Cluster\tServer", comps, nil

	case parse.FlagUser:
		entries, err := fetchConfig.GetUserEntries()
		if err != nil {
			return "", nil, errors.Wrap(err, "error listing kubeconfig users")
		}
		comps = make([]string, 0, len(entries))
		for _, entry := range entries {
			comps = append(comps, strings.Join([]string{entry.Name, entry.Auth}, "\t"))
		}
		return "User\tAuth", comps, nil
	}
	return "", nil, fmt.Errorf("%s is not completed from the kubeconfig", flagCompletion)
}

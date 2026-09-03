package clusterconfig

import (
	"path/filepath"
	"sort"

	"github.com/pkg/errors"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

// missingField stands in for a field the kubeconfig leaves out. An empty one
// would collapse a column and misalign the row it belongs to.
const missingField = "-"

// ContextEntry is one context of the kubeconfig.
type ContextEntry struct {
	Name      string
	Cluster   string
	Namespace string
	User      string
}

// ClusterEntry is one cluster of the kubeconfig.
type ClusterEntry struct {
	Name   string
	Server string
}

// UserEntry is one user of the kubeconfig. Auth names how the user proves who
// they are, which is what tells otherwise similar entries apart.
type UserEntry struct {
	Name string
	Auth string
}

func (c *ClusterConfig) checkLoaded() error {
	if c.apiConfig == nil {
		return errors.New("kubeconfig is not loaded, call LoadClusterConfig before")
	}
	return nil
}

// GetContextEntries lists the contexts of the kubeconfig, by name.
func (c *ClusterConfig) GetContextEntries() ([]ContextEntry, error) {
	if err := c.checkLoaded(); err != nil {
		return nil, err
	}
	entries := make([]ContextEntry, 0, len(c.apiConfig.Contexts))
	for name, kubeContext := range c.apiConfig.Contexts {
		namespace := kubeContext.Namespace
		if namespace == "" {
			// kubectl reads a context that names no namespace as the default one.
			namespace = "default"
		}
		entries = append(entries, ContextEntry{
			Name:      name,
			Cluster:   orMissing(kubeContext.Cluster),
			Namespace: namespace,
			User:      orMissing(kubeContext.AuthInfo),
		})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
	return entries, nil
}

// GetClusterEntries lists the clusters of the kubeconfig, by name.
func (c *ClusterConfig) GetClusterEntries() ([]ClusterEntry, error) {
	if err := c.checkLoaded(); err != nil {
		return nil, err
	}
	entries := make([]ClusterEntry, 0, len(c.apiConfig.Clusters))
	for name, cluster := range c.apiConfig.Clusters {
		entries = append(entries, ClusterEntry{Name: name, Server: orMissing(cluster.Server)})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
	return entries, nil
}

// GetUserEntries lists the users of the kubeconfig, by name.
func (c *ClusterConfig) GetUserEntries() ([]UserEntry, error) {
	if err := c.checkLoaded(); err != nil {
		return nil, err
	}
	entries := make([]UserEntry, 0, len(c.apiConfig.AuthInfos))
	for name, authInfo := range c.apiConfig.AuthInfos {
		entries = append(entries, UserEntry{Name: name, Auth: authKind(authInfo)})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
	return entries, nil
}

// authKind names the credential a user entry carries. The exec plugin is named
// too, since a kubeconfig full of exec users is told apart by nothing else.
func authKind(authInfo *clientcmdapi.AuthInfo) string {
	switch {
	case authInfo == nil:
		return missingField
	case authInfo.Exec != nil:
		return "exec:" + filepath.Base(authInfo.Exec.Command)
	case authInfo.AuthProvider != nil:
		return "auth-provider:" + authInfo.AuthProvider.Name
	case authInfo.Token != "" || authInfo.TokenFile != "":
		return "token"
	case authInfo.ClientCertificate != "" || len(authInfo.ClientCertificateData) > 0:
		return "client-certificate"
	case authInfo.Username != "":
		return "basic"
	}
	return missingField
}

func orMissing(value string) string {
	if value == "" {
		return missingField
	}
	return value
}

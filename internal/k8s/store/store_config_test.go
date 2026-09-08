package store

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/rooty0/kubectl-fzf/v3/internal/k8s/clusterconfig"
	"github.com/rooty0/kubectl-fzf/v3/internal/k8s/resources"
)

func TestFileStoreExists(t *testing.T) {
	c := &StoreConfigCli{
		ClusterConfigCli: &clusterconfig.ClusterConfigCli{
			ClusterName: "minikube",
			CacheDir:    "./testdata",
		}, TimeBetweenFullDump: 1 * time.Second}
	s := NewStoreConfig(c)
	assert.True(t, s.FileStoreExists(resources.ResourceTypePod))
	assert.False(t, s.FileStoreExists(resources.ResourceTypeApiResource))
}

package store

import (
	"time"

	"github.com/rooty0/kubectl-fzf/v3/internal/k8s/clusterconfig"
)

// StoreConfig defines configuration to store
// This is shared between all resources
type StoreConfig struct {
	clusterconfig.ClusterConfig
	timeBetweenFullDump  time.Duration
	watchPreviousContext bool
}

func NewStoreConfig(storeConfigCli *StoreConfigCli) *StoreConfig {
	s := StoreConfig{}
	s.ClusterConfig = clusterconfig.NewClusterConfig(storeConfigCli.ClusterConfigCli)
	s.timeBetweenFullDump = storeConfigCli.TimeBetweenFullDump
	s.watchPreviousContext = storeConfigCli.WatchPreviousContext
	return &s
}

func (s *StoreConfig) GetTimeBetweenFullDump() time.Duration {
	return s.timeBetweenFullDump
}

func (s *StoreConfig) GetWatchPreviousContext() bool {
	return s.watchPreviousContext
}

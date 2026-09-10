package store

import (
	"time"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	"github.com/rooty0/kubectl-fzf/v3/internal/k8s/clusterconfig"
)

type StoreConfigCli struct {
	*clusterconfig.ClusterConfigCli
	TimeBetweenFullDump  time.Duration
	WatchPreviousContext bool
}

func SetStoreConfigCli(fs *pflag.FlagSet) {
	clusterconfig.SetClusterConfigCli(fs)
	fs.Duration("time-between-full-dump", 10*time.Second, "Buffer changes and only do full dump every x secondes")
	fs.Bool("watch-previous-context", false, "Also watch the previously selected kubeconfig context, keeping its cache fresh. Roughly doubles heap and apiserver watch traffic.")
}

func GetStoreConfigCli() StoreConfigCli {
	s := StoreConfigCli{
		ClusterConfigCli: clusterconfig.GetClusterConfigCli(),
	}
	s.TimeBetweenFullDump = viper.GetDuration("time-between-full-dump")
	s.WatchPreviousContext = viper.GetBool("watch-previous-context")
	return s
}

package util

import (
	"errors"
	"flag"
	"os"
	"path/filepath"
	"runtime/pprof"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

func SetCommonCliFlags(fs *pflag.FlagSet, defaultLogLevel string) {
	fs.String("log-level", defaultLogLevel, "Log level to use")
	fs.String("cpu-profile", "", "Destination file for cpu profiling")
	fs.String("mem-profile", "", "Destination file for memory profiling")
}

func CommonInitialization() {
	configureLog()
	cpuProfile := viper.GetString("cpu-profile")
	if cpuProfile != "" {
		f, err := os.Create(cpuProfile)
		if err != nil {
			logrus.Fatal(err)
		}
		if err := pprof.StartCPUProfile(f); err != nil {
			logrus.Fatal(err)
		}
	}
}

func ConfigureViper() {
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)
	err := viper.BindPFlags(pflag.CommandLine)
	FatalIf(err)

	viper.SetEnvPrefix("KUBECTL_FZF")
	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))

	viper.SetConfigName(".kubectl_fzf")
	// Developer convenience: a config next to the binary outranks the
	// system paths (viper searches paths in the order they are added),
	// so a local .kubectl_fzf.yaml wins for quick experiments.
	if exe, err := os.Executable(); err == nil {
		if exe, err := filepath.EvalSymlinks(exe); err == nil {
			viper.AddConfigPath(filepath.Dir(exe))
		}
	}
	viper.AddConfigPath("/etc/kubectl_fzf/")
	viper.AddConfigPath("$HOME")
	err = viper.ReadInConfig()

	// A missing config file is fine (all settings have defaults and env
	// overrides); anything else means the given config was unreadable.
	var configFileNotFoundError viper.ConfigFileNotFoundError
	if !errors.As(err, &configFileNotFoundError) {
		FatalIf(err)
	}
}

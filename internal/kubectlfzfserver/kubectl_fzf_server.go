package kubectlfzfserver

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"net/http"
	//nolint:gosec // G108: the pprof endpoints are the point — they are the
	// documented profiling interface, and they bind to localhost inside the
	// pod only.
	_ "net/http/pprof"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	"github.com/rooty0/kubectl-fzf/v3/internal/httpserver"
	"github.com/rooty0/kubectl-fzf/v3/internal/k8s/resourcewatcher"
	"github.com/rooty0/kubectl-fzf/v3/internal/k8s/store"
	"github.com/rooty0/kubectl-fzf/v3/internal/util"
)

func startWatchOnCluster(ctx context.Context,
	resourceWatcherCli resourcewatcher.ResourceWatcherCli,
	storeConfig *store.StoreConfig) (*resourcewatcher.ResourceWatcher, []*store.Store, error) {
	cluster := storeConfig.GetContext()
	watcher, err := resourcewatcher.NewResourceWatcher(cluster, resourceWatcherCli, storeConfig)
	if err != nil {
		return nil, nil, errors.Wrap(err, "error creating resource watcher")
	}
	err = watcher.FetchNamespaces(ctx)
	if err != nil {
		return nil, nil, errors.Wrap(err, "error fetching namespaces")
	}
	watchConfigs, err := watcher.GetWatchConfigs()
	if err != nil {
		return nil, nil, errors.Wrap(err, "error getting watchdog configs")
	}
	logrus.Infof("Start cache build on cluster %s", cluster)
	stores := make([]*store.Store, 0)
	for _, watchConfig := range watchConfigs {
		resourceStore := watcher.Start(ctx, watchConfig)
		stores = append(stores, resourceStore)
	}
	err = watcher.DumpAPIResources()
	if err != nil {
		return nil, nil, errors.Wrap(err, "error when dumping api resources")
	}
	return watcher, stores, nil
}

func handleSignals(cancel context.CancelFunc) {
	sigIn := make(chan os.Signal, 100)
	signal.Notify(sigIn)
	for sig := range sigIn {
		switch sig {
		case syscall.SIGINT, syscall.SIGTERM:
			logrus.Infof("Caught signal '%s' (%d); terminating.", sig, sig)
			cancel()
		}
	}
}

// clusterGeneration is one watched context: the watcher to stop and the
// stores holding its in-memory state.
type clusterGeneration struct {
	watcher *resourcewatcher.ResourceWatcher
	stores  []*store.Store
}

// desiredContexts is the set of contexts the daemon should watch given the
// current and previously selected kubeconfig contexts. Current first, stable
// order, so callers can treat index 0 as the current context. Kept pure so
// the reconcile diff can be tested without a cluster.
func desiredContexts(current, previous string, watchPrevious bool) []string {
	if current == "" {
		return nil
	}
	if !watchPrevious || previous == "" || previous == current {
		return []string{current}
	}
	return []string{current, previous}
}

// startGeneration points a fresh store config at contextName and starts
// watching it. Teardown goes through generation.watcher.Stop.
func startGeneration(ctx context.Context,
	resourceWatcherCli resourcewatcher.ResourceWatcherCli,
	storeConfigCli *store.StoreConfigCli,
	contextName string) (*clusterGeneration, error) {
	storeConfig := store.NewStoreConfig(storeConfigCli)
	err := storeConfig.LoadClusterConfig()
	if err != nil {
		return nil, err
	}
	if storeConfig.GetContext() != contextName {
		err = storeConfig.SetContext(contextName)
		if err != nil {
			return nil, err
		}
	}
	err = storeConfig.CreateDestDir()
	if err != nil {
		return nil, err
	}
	watcher, stores, err := startWatchOnCluster(ctx, resourceWatcherCli, storeConfig)
	if err != nil {
		return nil, err
	}
	return &clusterGeneration{watcher: watcher, stores: stores}, nil
}

// reconcileGenerations brings the running generations in line with desired:
// generations for contexts no longer wanted are stopped, missing ones are
// started, and the combined store list is returned. The current context is
// desired[0] and its startup failure stays fatal, matching the single-context
// behavior. A failure on any other context (deleted from the kubeconfig,
// expired credentials, unreachable cluster) degrades to a warning so the
// previous-context cache never kills the current one.
func reconcileGenerations(ctx context.Context,
	resourceWatcherCli resourcewatcher.ResourceWatcherCli,
	storeConfigCli *store.StoreConfigCli,
	generations map[string]*clusterGeneration,
	desired []string) []*store.Store {
	inDesired := func(name string) bool {
		for _, d := range desired {
			if d == name {
				return true
			}
		}
		return false
	}
	for name, gen := range generations {
		if !inDesired(name) {
			logrus.Infof("Stopping watch of context %s", name)
			gen.watcher.Stop()
			delete(generations, name)
		}
	}
	allStores := make([]*store.Store, 0)
	for i, name := range desired {
		gen, ok := generations[name]
		if !ok {
			newGen, err := startGeneration(ctx, resourceWatcherCli, storeConfigCli, name)
			if err != nil {
				if i == 0 {
					logrus.Fatalf("error starting watch of current context %s: %v", name, err)
				}
				logrus.Warnf("Skipping watch of previous context %s: %v", name, err)
				continue
			}
			generations[name] = newGen
			gen = newGen
		}
		allStores = append(allStores, gen.stores...)
	}
	return allStores
}

func StartKubectlFzfServer() {
	ctx, cancel := context.WithCancel(context.Background())
	go handleSignals(cancel)

	storeConfigCli := store.GetStoreConfigCli()
	storeConfig := store.NewStoreConfig(&storeConfigCli)
	err := storeConfig.LoadClusterConfig()
	if err != nil {
		logrus.Fatal("Couldn't get current context: ", err)
	}
	err = storeConfig.CreateDestDir()
	if err != nil {
		logrus.Fatalf("error creating destination dir: %s", err)
	}

	resourceWatcherCli := resourcewatcher.GetResourceWatcherCli()
	watcher, stores, err := startWatchOnCluster(ctx, resourceWatcherCli, storeConfig)
	util.FatalIf(err)
	currentContext := storeConfig.GetContext()
	previousContext := ""
	generations := map[string]*clusterGeneration{
		currentContext: {watcher: watcher, stores: stores},
	}
	ticker := time.NewTicker(time.Second * 5)

	httpServerConfCli := httpserver.GetHttpServerConfigCli()
	fzfHttpServer, err := httpserver.StartHttpServer(ctx, &httpServerConfCli, storeConfig, stores)
	if err != nil {
		logrus.Fatalf("Error starting http server: %s", err)
	}

	// Profiling endpoint, off unless asked for: a heap dump leaks cluster
	// object metadata to any local process, and only developers profiling
	// the daemon ever read it. ReadHeaderTimeout satisfies the Slowloris
	// hardening without hurting long-lived pprof scrapes.
	if httpServerConfCli.HttpProfAddress != "" {
		pprofServer := &http.Server{
			Addr:              httpServerConfCli.HttpProfAddress,
			ReadHeaderTimeout: 5 * time.Second,
		}
		go func() {
			logrus.Println(pprofServer.ListenAndServe())
		}()
	}

	for {
		select {
		case <-ctx.Done():
			logrus.Info("Context done, exiting")
			return
		case <-ticker.C:
			err = storeConfig.LoadClusterConfig()
			util.FatalIf(err)
			newContext := storeConfig.GetContext()
			logrus.Debugf("Checking config %s %s ", currentContext, newContext)
			if newContext != currentContext {
				logrus.Infof("Detected context change %s != %s", newContext, currentContext)
				previousContext = currentContext
				currentContext = newContext
			}
			watchPrevious := storeConfig.GetWatchPreviousContext()
			desired := desiredContexts(currentContext, previousContext, watchPrevious)
			allStores := reconcileGenerations(ctx, resourceWatcherCli, &storeConfigCli, generations, desired)
			// fzfHttpServer is nil when no listen address is configured.
			if fzfHttpServer != nil {
				fzfHttpServer.SetStores(allStores)
			}
		}
	}
}

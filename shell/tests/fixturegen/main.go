// fixturegen writes a local-file cache (gob encoded resources) for the e2e
// shell test: `go run ./shell/tests/fixturegen -out <cache root>`. The cache is
// what kubectl-fzf-server would have written, so the completion binary answers
// from it without a cluster.
package main

import (
	"flag"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/rooty0/kubectl-fzf/v3/internal/k8s/resources"
	"github.com/rooty0/kubectl-fzf/v3/internal/util"

	core_v1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func pod(name, namespace string) core_v1.Pod {
	return core_v1.Pod{
		TypeMeta: meta_v1.TypeMeta{Kind: "Pod"},
		ObjectMeta: meta_v1.ObjectMeta{
			Name:              name,
			Namespace:         namespace,
			Labels:            map[string]string{"app": name},
			CreationTimestamp: meta_v1.Time{Time: time.Now()},
		},
	}
}

func main() {
	out := flag.String("out", "", "cache root, the per-context directory is created inside")
	contextName := flag.String("context", "e2e", "context the cache answers for")
	flag.Parse()
	if *out == "" {
		log.Fatal("-out is required")
	}

	pods := []core_v1.Pod{
		pod("web-server-1", "web"),
		pod("web-server-2", "web"),
		pod("db-1", "web"),
		pod("metrics-1", "monitoring"),
	}
	ctorConfig := resources.CtorConfig{}
	data := map[string]resources.K8sResource{}
	for i := range pods {
		p := &pods[i]
		data[p.Namespace+"_"+p.Name] = resources.NewPodFromRuntime(p, ctorConfig)
	}

	dir := filepath.Join(*out, *contextName)
	if err := os.MkdirAll(dir, 0755); err != nil {
		log.Fatal(err)
	}
	if err := util.EncodeToFile(data, filepath.Join(dir, "pods")); err != nil {
		log.Fatal(err)
	}
}

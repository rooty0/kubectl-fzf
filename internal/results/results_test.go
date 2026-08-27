package results

import (
	"os"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	logrus.SetLevel(logrus.DebugLevel)
	code := m.Run()
	os.Exit(code)
}

func TestParseNamespaceFlag(t *testing.T) {
	r, err := parseNamespaceFlag([]string{"get", "pods", "-ntest"})
	require.NoError(t, err)
	assert.Equal(t, "test", *r)

	r, err = parseNamespaceFlag([]string{"get", "pods", "--namespace", "kube-system"})
	require.NoError(t, err)
	assert.Equal(t, "kube-system", *r)

	r, err = parseNamespaceFlag([]string{"get", "pods", "--context", "minikube", "--namespace", "kube-system"})
	require.NoError(t, err)
	assert.Equal(t, "kube-system", *r)
}

func TestResult(t *testing.T) {
	testDatas := []struct {
		fzfResult        string
		cmdUse           string
		cmdArgs          []string
		currentNamespace string
		expectedResult   string
		expectedRemovals []string
	}{
		{"kube-system kube-controller-manager-minikube", "get", []string{"pods", " "}, "kube-system", "kube-controller-manager-minikube", nil},
		{"kube-system coredns-64897985d-nrblm", "get", []string{"pods", "--context", "minikube", "--namespace", "kube-system", ""}, "default", "coredns-64897985d-nrblm", nil},
		{"kube-system kube-controller-manager-minikube", "get", []string{"pods", " "}, "default", "kube-controller-manager-minikube -n kube-system", nil},
		{"kube-system kube-controller-manager-minikube", "get", []string{"pods", "-nkube-system", " "}, "default", "kube-controller-manager-minikube", nil},

		{"kfzf kubectl-fzf-788969b7cb-vf85b", "exec", []string{"-ti", ""}, "default", "kubectl-fzf-788969b7cb-vf85b -n kfzf", nil},
		// Namespace
		{"default 30d kubernetes.io/metadata.name=default", "get", []string{"pods", "-n="}, "default", "-n=default", nil},
		{"default 30d kubernetes.io/metadata.name=default", "get", []string{"pods", "-n"}, "default", "-ndefault", nil},
		{"default 30d kubernetes.io/metadata.name=default", "get", []string{"pods", "-n", " "}, "default", "default", nil},
		// Label
		{"kube-system tier=control-plane", "get", []string{"pods", "-l="}, "default", "-l=tier=control-plane -n kube-system", nil},
		{"kube-system tier=control-plane", "get", []string{"pods", "-l", " "}, "default", "tier=control-plane -n kube-system", nil},
		{"kube-system tier=control-plane", "get", []string{"pods", "-l"}, "default", "-ltier=control-plane -n kube-system", nil},
		// Namespaceless label
		{"beta.kubernetes.io/arch=amd64 1", "get", []string{"nodes", "-l"}, "default", "-lbeta.kubernetes.io/arch=amd64", nil},
		// Field selector
		{"kube-system spec.nodeName=minikube", "get", []string{"pods", "--field-selector="}, "default", "--field-selector=spec.nodeName=minikube -n kube-system", nil},
		{"kube-system spec.nodeName=minikube", "get", []string{"pods", "--field-selector", " "}, "default", "spec.nodeName=minikube -n kube-system", nil},
		{"kube-system coredns-64897985d-nrblm", "get", []string{"pods", "c"}, "default", "coredns-64897985d-nrblm -n kube-system", nil},
		{"apiservices.apiregistration.k8s.io None apiregistration.k8s.io/v1", "get", []string{" "}, "default", "apiservices.apiregistration.k8s.io", nil},

		// -A only widens what fzf lists. Naming one object pins a namespace and
		// kubectl rejects a named resource across all namespaces, so -A has to go.
		{"web web-server-1", "get", []string{"pods", "-A", " "}, "default",
			"web-server-1 -n web", []string{"-A"}},
		// Same, but the object already sits in the current namespace: no -n is
		// added and -A must still go, otherwise kubectl errors out.
		{"web web-server-1", "get", []string{"pods", "-A", " "}, "web",
			"web-server-1", []string{"-A"}},
		{"web web-server-1", "get", []string{"pods", "--all-namespaces", " "}, "default",
			"web-server-1 -n web", []string{"--all-namespaces"}},
		{"web web-server-1", "get", []string{"pods", "--all-namespaces=true", " "}, "default",
			"web-server-1 -n web", []string{"--all-namespaces=true"}},
		// An explicit false keeps kubectl namespace scoped, nothing to remove.
		{"web web-server-1", "get", []string{"pods", "--all-namespaces=false", " "}, "default",
			"web-server-1 -n web", nil},
		// A cluster scoped resource tolerates -A, drop it anyway for consistency.
		{"minikube 1", "get", []string{"nodes", "-A", " "}, "default", "minikube", []string{"-A"}},
		// Selector values stay valid cluster wide, so -A is kept and the namespace
		// suffix dropped: -A would silently override the -n anyway.
		{"kube-system tier=control-plane", "get", []string{"pods", "-A", "-l", " "}, "default",
			"tier=control-plane", nil},
		{"kube-system spec.nodeName=minikube", "get", []string{"pods", "-A", "--field-selector", " "}, "default",
			"spec.nodeName=minikube", nil},
		// Choosing a namespace explicitly contradicts -A, so -A goes.
		{"web 30d kubernetes.io/metadata.name=web", "get", []string{"pods", "-A", "-n", " "}, "default",
			"web", []string{"-A"}},
	}
	for _, testData := range testDatas {
		res, err := processResultWithNamespace(testData.cmdUse, testData.cmdArgs, testData.fzfResult, testData.currentNamespace)
		require.NoError(t, err)
		require.Equal(t, testData.expectedResult, res.Completion,
			"Fzf result %s, cmdUse %s, cmdArgs %s, current namespace %s, res: %s", testData.fzfResult, testData.cmdUse,
			testData.cmdArgs, testData.currentNamespace, res.Completion)
		require.Equal(t, testData.expectedRemovals, res.RemoveWords,
			"Fzf result %s, cmdUse %s, cmdArgs %s, current namespace %s", testData.fzfResult, testData.cmdUse,
			testData.cmdArgs, testData.currentNamespace)
	}
}

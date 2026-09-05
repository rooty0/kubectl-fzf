package storetest

import (
	"errors"
	"os"
	"path"
	"testing"
	"time"

	"github.com/rooty0/kubectl-fzf/v3/internal/k8s/resources"
	"github.com/rooty0/kubectl-fzf/v3/internal/util"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	logrus.SetLevel(logrus.DebugLevel)
	code := m.Run()
	os.Exit(code)
}

// loadDumpedPods returns the pods held in the dumped cache file, or nil if the
// ticker has not written it yet. A decode failure is always reported: dumps are
// written atomically, so a reader must never see a partially written file.
func loadDumpedPods(t *testing.T, podFilePath string) map[string]resources.K8sResource {
	t.Helper()
	if _, err := os.Stat(podFilePath); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	pods := map[string]resources.K8sResource{}
	if err := util.LoadGobFromFile(&pods, podFilePath); err != nil {
		t.Errorf("loading dumped pods from %s failed: %v", podFilePath, err)
		return nil
	}
	return pods
}

func TestDumpPodFullState(t *testing.T) {
	tempDir, k := GetTestPodStore(t)
	defer util.RemoveTempDir(tempDir)

	err := k.DumpFullState()
	require.NoError(t, err)
	podFilePath := path.Join(tempDir, "test", "pods")
	assert.FileExists(t, podFilePath)

	pods := map[string]resources.K8sResource{}
	err = util.LoadGobFromFile(&pods, podFilePath)
	require.NoError(t, err)

	assert.Equal(t, 4, len(pods))
	assert.Contains(t, pods, "ns1_Test1")
	assert.Contains(t, pods, "ns2_Test2")
	assert.Contains(t, pods, "ns2_Test3")
	assert.Contains(t, pods, "aaa_Test4")
}

func TestTickerPodDumpFullState(t *testing.T) {
	tempDir, s := GetTestPodStore(t)
	defer util.RemoveTempDir(tempDir)

	podFilePath := path.Join(tempDir, "test", "pods")
	require.Eventually(t, func() bool {
		return loadDumpedPods(t, podFilePath) != nil
	}, 5*time.Second, 50*time.Millisecond, "ticker should dump the cache file")
	assert.Equal(t, 4, len(loadDumpedPods(t, podFilePath)))

	pod := podResource("Test5", "ns3", map[string]string{"app": "app5"})
	s.AddResource(&pod)

	require.Eventually(t, func() bool {
		_, ok := loadDumpedPods(t, podFilePath)["ns3_Test5"]
		return ok
	}, 5*time.Second, 50*time.Millisecond,
		"ticker should dump the cache file again after a resource change")
}

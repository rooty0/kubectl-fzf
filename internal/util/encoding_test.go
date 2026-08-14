package util

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEncoding(t *testing.T) {
	data := "test"

	f, err := ioutil.TempFile("", "encoding")
	require.NoError(t, err)
	defer os.Remove(f.Name())

	err = EncodeToFile(data, f.Name())
	require.NoError(t, err)

	var res string
	err = LoadGobFromFile(&res, f.Name())
	require.NoError(t, err)
	assert.Equal(t, "test", res)
}

// TestEncodeToFileIsAtomicForReaders guards the invariant that a reader never
// observes a partially written file. The server rewrites the cache files on a
// ticker while completion processes read them, so a non-atomic write surfaces
// as an "unexpected EOF" in the middle of a completion.
func TestEncodeToFileIsAtomicForReaders(t *testing.T) {
	filePath := filepath.Join(t.TempDir(), "pods")

	// Big enough that a write cannot plausibly land between two reads.
	data := make([]string, 20000)
	for i := range data {
		data[i] = fmt.Sprintf("resource-%d-%s", i, strings.Repeat("x", 64))
	}
	require.NoError(t, EncodeToFile(data, filePath))

	done := make(chan struct{})
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(done)
		for i := 0; i < 50; i++ {
			if err := EncodeToFile(data, filePath); err != nil {
				t.Errorf("write %d failed: %v", i, err)
				return
			}
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-done:
				return
			default:
			}
			var res []string
			if err := LoadGobFromFile(&res, filePath); err != nil {
				t.Errorf("concurrent read observed a partial file: %v", err)
				return
			}
			if len(res) != len(data) {
				t.Errorf("concurrent read got %d entries, want %d", len(res), len(data))
				return
			}
		}
	}()

	wg.Wait()

	leftovers, err := filepath.Glob(filePath + ".tmp*")
	require.NoError(t, err)
	assert.Empty(t, leftovers, "temporary files should not be left behind")
}

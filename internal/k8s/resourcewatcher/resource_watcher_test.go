package resourcewatcher

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/rooty0/kubectl-fzf/v3/internal/k8s/resources"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest/fake"
)

// The ListWatch produced by getCacheListWatch must pin ResourceVersion="0"
// on LISTs only. On WATCHes it has to leave the resourceVersion filled in by
// the reflector untouched, otherwise every re-watch loses its resume point.
func TestGetCacheListWatchResourceVersion(t *testing.T) {
	var mu sync.Mutex
	var queries []url.Values
	restClient := &fake.RESTClient{
		NegotiatedSerializer: scheme.Codecs.WithoutConversion(),
		GroupVersion:         corev1.SchemeGroupVersion,
		VersionedAPIPath:     "/api/v1",
		Client: fake.CreateHTTPClient(func(req *http.Request) (*http.Response, error) {
			mu.Lock()
			queries = append(queries, req.URL.Query())
			mu.Unlock()
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body: io.NopCloser(strings.NewReader(
					`{"kind":"PodList","apiVersion":"v1","metadata":{"resourceVersion":"99"},"items":[]}`)),
			}, nil
		}),
	}

	watcher := &ResourceWatcher{}
	lw := watcher.getCacheListWatch(WatchConfig{
		resourceType: resources.ResourceTypePod,
		getter:       restClient,
	}, "")

	_, err := lw.ListWithContext(context.Background(), metav1.ListOptions{})
	require.NoError(t, err)

	// Decoding a PodList as a watch stream is expected to fail; what matters
	// is the request sent on the wire.
	w, _ := lw.WatchWithContext(context.Background(), metav1.ListOptions{ResourceVersion: "12345"})
	if w != nil {
		w.Stop()
	}

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, queries, 2)
	assert.Equal(t, "0", queries[0].Get("resourceVersion"))
	assert.Empty(t, queries[0].Get("watch"))
	assert.Equal(t, "12345", queries[1].Get("resourceVersion"))
	assert.Equal(t, "true", queries[1].Get("watch"))
}

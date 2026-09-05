package util

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetFromHttpServerSuccess(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Test-Header", "test value")
		fmt.Fprint(w, "hello")
	}))
	defer ts.Close()

	headers, body, err := GetFromHttpServer(context.Background(), ts.URL)
	require.NoError(t, err)
	assert.Equal(t, "test value", headers.Get("X-Test-Header"))
	assert.Equal(t, "hello", string(body))
}

func TestGetFromHttpServerNon200(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	_, _, err := GetFromHttpServer(context.Background(), ts.URL)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}

func TestGetFromHttpServerTimeout(t *testing.T) {
	// Hang until the client goes away, mimicking a stuck port-forward.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, _, err := GetFromHttpServer(ctx, ts.URL)
	elapsed := time.Since(start)

	require.Error(t, err)
	assert.True(t, errors.Is(err, context.DeadlineExceeded), "expected deadline exceeded, got %v", err)
	assert.Less(t, elapsed, 2*time.Second, "the get must fail fast instead of hanging")
}

func TestHeadFromHttpServerSuccess(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Last-Modified", "Mon, 02 Jan 2006 15:04:05 GMT")
	}))
	defer ts.Close()

	headers, err := HeadFromHttpServer(context.Background(), ts.URL)
	require.NoError(t, err)
	assert.Equal(t, "Mon, 02 Jan 2006 15:04:05 GMT", headers.Get("Last-Modified"))
}

func TestHeadFromHttpServerTimeout(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := HeadFromHttpServer(ctx, ts.URL)
	elapsed := time.Since(start)

	require.Error(t, err)
	assert.True(t, errors.Is(err, context.DeadlineExceeded), "expected deadline exceeded, got %v", err)
	assert.Less(t, elapsed, 2*time.Second, "the head must fail fast instead of hanging")
}

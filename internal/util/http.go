package util

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/pkg/errors"
)

// GetFromHttpServer fetches url with a GET. The request carries ctx, so the
// caller's deadline (e.g. the completion timeout) bounds the whole call: a
// stuck server or port-forward makes this fail instead of hanging forever.
func GetFromHttpServer(ctx context.Context, url string) (http.Header, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "error creating request for %s", url)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "error on get of %s", url)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, nil, fmt.Errorf("error retrieving resource from server: %s", resp.Status)
	}
	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, errors.Wrap(err, "error reading response body")
	}
	return resp.Header, b, nil
}

// HeadFromHttpServer sends a HEAD for url. Like GetFromHttpServer, the
// caller-provided ctx bounds the call.
func HeadFromHttpServer(ctx context.Context, url string) (http.Header, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		return nil, errors.Wrapf(err, "error creating request for %s", url)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, errors.Wrapf(err, "error on get of %s", url)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("error retrieving resource from server: %s", resp.Status)
	}
	return resp.Header, nil
}

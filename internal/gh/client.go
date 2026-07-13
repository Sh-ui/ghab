// Package gh is ghab's data layer: a Client wrapping go-gh's REST client
// with an in-memory TTL cache. Typed fetchers expose only the fields the
// UI shows.
package gh

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/cli/go-gh/v2/pkg/api"
)

// Client fetches GitHub data through go-gh's REST client (auth resolved
// free: GH_TOKEN env, then gh's stored OAuth token) with a TTL cache in
// front of it. A second client (raw) is identical except it defaults to
// the "application/vnd.github.raw+json" Accept header, for the file/readme
// fetchers that want file bytes back instead of a JSON envelope. A third
// (assets) defaults to "application/octet-stream", for DownloadAsset.
type Client struct {
	rest   *api.RESTClient
	raw    *api.RESTClient
	assets *api.RESTClient
	cache  *cache
}

// NewClient builds a Client. ttl is the cache lifetime for every
// endpoint, from [behavior].cache_ttl.
func NewClient(ttl time.Duration) (*Client, error) {
	rest, err := api.DefaultRESTClient()
	if err != nil {
		return nil, fmt.Errorf("gh auth: %w", err)
	}
	raw, err := api.NewRESTClient(api.ClientOptions{
		Headers: map[string]string{"Accept": "application/vnd.github.raw+json"},
	})
	if err != nil {
		return nil, fmt.Errorf("gh auth: %w", err)
	}
	assets, err := api.NewRESTClient(api.ClientOptions{
		Headers: map[string]string{"Accept": "application/octet-stream"},
	})
	if err != nil {
		return nil, fmt.Errorf("gh auth: %w", err)
	}
	return &Client{rest: rest, raw: raw, assets: assets, cache: newCache(ttl)}, nil
}

// NotFoundError marks a 404 from the GitHub API so callers can render
// "not found or no access" instead of a raw HTTP error.
type NotFoundError struct {
	Path string
}

func (e *NotFoundError) Error() string {
	return fmt.Sprintf("%s: not found or no access", e.Path)
}

func (c *Client) get(path string, out interface{}) error {
	if err := c.rest.Get(path, out); err != nil {
		return wrapHTTPError(path, err)
	}
	return nil
}

// getRaw issues a GET through the raw (Accept: .raw+json) client and
// returns the response body verbatim -- used by FileRaw and Readme, which
// want file bytes rather than a JSON-wrapped base64 blob.
func (c *Client) getRaw(path string) ([]byte, error) {
	resp, err := c.raw.Request(http.MethodGet, path, nil)
	if err != nil {
		return nil, wrapHTTPError(path, err)
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

func wrapHTTPError(path string, err error) error {
	var httpErr *api.HTTPError
	if errors.As(err, &httpErr) && httpErr.StatusCode == 404 {
		return &NotFoundError{Path: path}
	}
	return err
}

// escapePath percent-encodes each "/"-separated segment of a repo-relative
// path for use in a contents/{path} URL -- GitHub paths can contain
// characters (spaces, #, etc.) that must be escaped per segment, never as
// a whole (that would also escape the separating slashes).
func escapePath(path string) string {
	if path == "" {
		return path
	}
	segments := strings.Split(path, "/")
	for i, s := range segments {
		segments[i] = url.PathEscape(s)
	}
	return strings.Join(segments, "/")
}

// escapeRef percent-encodes a branch/ref name as a single URL path segment
// -- GitHub's git/trees/{branch} endpoint requires slash-containing branch
// names (e.g. "release/1.0") to arrive as a literal "%2F", which
// url.PathEscape already produces for a value with no other segments.
func escapeRef(ref string) string {
	return url.PathEscape(ref)
}

// Refresh busts the cache entry for path, forcing the next fetch to hit
// the network. Used by the "r" refresh binding.
func (c *Client) Refresh(path string) {
	c.cache.bust(path)
}

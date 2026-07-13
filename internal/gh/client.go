// Package gh is ghab's data layer: a Client wrapping go-gh's REST client
// with an in-memory TTL cache. Typed fetchers expose only the fields the
// UI shows.
package gh

import (
	"errors"
	"fmt"
	"time"

	"github.com/cli/go-gh/v2/pkg/api"
)

// Client fetches GitHub data through go-gh's REST client (auth resolved
// free: GH_TOKEN env, then gh's stored OAuth token) with a TTL cache in
// front of it.
type Client struct {
	rest  *api.RESTClient
	cache *cache
}

// NewClient builds a Client. ttl is the cache lifetime for every
// endpoint, from [behavior].cache_ttl.
func NewClient(ttl time.Duration) (*Client, error) {
	rest, err := api.DefaultRESTClient()
	if err != nil {
		return nil, fmt.Errorf("gh auth: %w", err)
	}
	return &Client{rest: rest, cache: newCache(ttl)}, nil
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
		var httpErr *api.HTTPError
		if errors.As(err, &httpErr) && httpErr.StatusCode == 404 {
			return &NotFoundError{Path: path}
		}
		return err
	}
	return nil
}

// Refresh busts the cache entry for path, forcing the next fetch to hit
// the network. Used by the "r" refresh binding.
func (c *Client) Refresh(path string) {
	c.cache.bust(path)
}

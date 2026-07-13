package gh

import (
	"errors"
	"fmt"
	"time"
)

// Asset is one release asset from repos/{o}/{r}/releases: name, size, and
// both download URLs -- BrowserDownloadURL for a human link, URL (the API
// endpoint) for DownloadAsset, which needs the API URL to stream through
// auth via the Accept: application/octet-stream trick.
type Asset struct {
	Name               string `json:"name"`
	Size               int64  `json:"size"`
	DownloadCount      int    `json:"download_count"`
	BrowserDownloadURL string `json:"browser_download_url"`
	URL                string `json:"url"`
}

// Release is the subset of repos/{o}/{r}/releases the UI shows.
type Release struct {
	TagName     string    `json:"tag_name"`
	Name        string    `json:"name"`
	Body        string    `json:"body"`
	PublishedAt time.Time `json:"published_at"`
	Prerelease  bool      `json:"prerelease"`
	Draft       bool      `json:"draft"`
	Assets      []Asset   `json:"assets"`
}

// releasesPath builds the releases endpoint URL -- factored out so
// RefreshReleases busts the exact same cache key Releases fetches under.
func releasesPath(owner, repo string, perPage int) string {
	return fmt.Sprintf("repos/%s/%s/releases?per_page=%d", owner, repo, perPage)
}

// Releases fetches repos/{owner}/{repo}/releases?per_page={n}, serving
// from cache when fresh. A 404 (or an empty body) becomes an empty slice
// rather than an error -- a repo with no releases is a normal, expected
// state (same rationale as NoReadmeError, but releases don't need a typed
// error since "no releases" isn't worth a distinct render branch beyond
// an empty list).
func (c *Client) Releases(owner, repo string, perPage int) ([]Release, error) {
	path := releasesPath(owner, repo, perPage)
	if cached, ok := c.cache.get(path); ok {
		if r, ok := cached.([]Release); ok {
			return r, nil
		}
	}

	var releases []Release
	if err := c.get(path, &releases); err != nil {
		var nf *NotFoundError
		if errors.As(err, &nf) {
			c.cache.set(path, []Release{})
			return []Release{}, nil
		}
		return nil, err
	}
	c.cache.set(path, releases)
	return releases, nil
}

// RefreshReleases busts the releases cache entry, forcing the next
// Releases call to hit the network. Used by the "r" refresh binding when
// the releases tab is active.
func (c *Client) RefreshReleases(owner, repo string, perPage int) {
	c.cache.bust(releasesPath(owner, repo, perPage))
}

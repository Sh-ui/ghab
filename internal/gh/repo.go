package gh

import "fmt"

// RepoMeta is the subset of GET repos/{owner}/{repo} the UI shows.
type RepoMeta struct {
	FullName        string   `json:"full_name"`
	Description     string   `json:"description"`
	StargazersCount int      `json:"stargazers_count"`
	DefaultBranch   string   `json:"default_branch"`
	Topics          []string `json:"topics"`
	OpenIssuesCount int      `json:"open_issues_count"`
	HTMLURL         string   `json:"html_url"`
	Fork            bool     `json:"fork"`
	License         *struct {
		SPDXID string `json:"spdx_id"`
	} `json:"license"`
	Parent *struct {
		FullName string `json:"full_name"`
	} `json:"parent"`
}

// LicenseSPDX returns the license's SPDX identifier, or "" if the repo
// has no license.
func (r RepoMeta) LicenseSPDX() string {
	if r.License == nil {
		return ""
	}
	return r.License.SPDXID
}

// ParentFullName returns the parent repo's "owner/name" if this repo is
// a fork, or "" otherwise.
func (r RepoMeta) ParentFullName() string {
	if r.Parent == nil {
		return ""
	}
	return r.Parent.FullName
}

// RepoMeta fetches repos/{owner}/{repo}, serving from cache when fresh.
func (c *Client) RepoMeta(owner, repo string) (RepoMeta, error) {
	path := fmt.Sprintf("repos/%s/%s", owner, repo)
	if cached, ok := c.cache.get(path); ok {
		if meta, ok := cached.(RepoMeta); ok {
			return meta, nil
		}
		// wrong type under this key -- treat as a miss and refetch
	}
	var meta RepoMeta
	if err := c.get(path, &meta); err != nil {
		return RepoMeta{}, err
	}
	c.cache.set(path, meta)
	return meta, nil
}

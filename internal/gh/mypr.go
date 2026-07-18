package gh

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// SearchIssue is one entry from GitHub's search/issues endpoint -- the
// shape ghab's only cross-repo list ("my PRs", issue #11) fetches. It
// carries everything Issue does plus RepositoryURL, since search results
// span repos and the UI needs to know which repo each row belongs to.
type SearchIssue struct {
	Number        int             `json:"number"`
	Title         string          `json:"title"`
	State         string          `json:"state"`
	User          issueUser       `json:"user"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
	Comments      int             `json:"comments"`
	Body          string          `json:"body"`
	PullRequest   json.RawMessage `json:"pull_request,omitempty"`
	RepositoryURL string          `json:"repository_url"`
}

// Author returns the PR author's login, or "" if the API omitted the user
// object.
func (i SearchIssue) Author() string { return i.User.Login }

// RepoFullName extracts "owner/repo" from RepositoryURL
// ("https://api.github.com/repos/{owner}/{repo}") -- "" if the field is
// missing or doesn't carry the expected "/repos/" marker.
func (i SearchIssue) RepoFullName() string {
	const marker = "/repos/"
	idx := strings.LastIndex(i.RepositoryURL, marker)
	if idx == -1 {
		return ""
	}
	return i.RepositoryURL[idx+len(marker):]
}

// myPRsResponse is the search/issues envelope (identical shape to
// search/repositories -- see SearchRepos).
type myPRsResponse struct {
	TotalCount int           `json:"total_count"`
	Items      []SearchIssue `json:"items"`
}

// myPRsPath builds the search/issues endpoint URL -- factored out so
// RefreshMyPRs busts the exact cache key MyPRs fetches under.
func myPRsPath(query string, perPage int) string {
	return fmt.Sprintf("search/issues?q=%s&per_page=%d", url.QueryEscape(query), perPage)
}

// MyPRs runs GitHub's search/issues endpoint with query -- config's
// [behavior].my_prs_query, default "is:pr is:open involves:@me" (issue
// #11: browsing PRs across every repo without leaving the terminal).
// Same tight search rate bucket as SearchRepos (30 req/min) -- fired once
// per screen push, never per keystroke.
func (c *Client) MyPRs(query string, perPage int) (total int, items []SearchIssue, err error) {
	path := myPRsPath(query, perPage)
	if cached, ok := c.cache.get(path); ok {
		if resp, ok := cached.(myPRsResponse); ok {
			return resp.TotalCount, resp.Items, nil
		}
	}
	var resp myPRsResponse
	if err := c.get(path, &resp); err != nil {
		return 0, nil, err
	}
	c.cache.set(path, resp)
	return resp.TotalCount, resp.Items, nil
}

// RefreshMyPRs busts the my-PRs search cache entry for query/perPage.
// Used by the "r" refresh binding on the my-PRs screen.
func (c *Client) RefreshMyPRs(query string, perPage int) {
	c.cache.bust(myPRsPath(query, perPage))
}

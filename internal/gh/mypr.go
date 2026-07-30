package gh

import (
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strconv"
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
	// TotalCount is decoded to record the envelope's shape but is not
	// surfaced: with several queries merged, summing their total_counts
	// would double-count any PR that matched more than one. MyPRs reports
	// the distinct-item count instead.
	TotalCount int           `json:"total_count"`
	Items      []SearchIssue `json:"items"`
}

// myPRsPath builds the search/issues endpoint URL -- factored out so
// RefreshMyPRs busts the exact cache key MyPRs fetches under.
func myPRsPath(query string, perPage int) string {
	return fmt.Sprintf("search/issues?q=%s&per_page=%d", url.QueryEscape(query), perPage)
}

// searchIssueKey identifies one PR across repos -- repository URL plus
// number, since #5 in two repos are two different PRs. Used to
// de-duplicate the merged result of several queries.
func searchIssueKey(i SearchIssue) string {
	return i.RepositoryURL + "#" + strconv.Itoa(i.Number)
}

// MergeSearchIssues concatenates several queries' result pages into one
// list: de-duplicated by repo+number (a PR matching two queries appears
// once) and ordered most-recently-updated first, so the merged list reads
// like a single ranked list rather than one query's page followed by
// another's. Pure and unit-testable without a network round trip.
func MergeSearchIssues(pages [][]SearchIssue) []SearchIssue {
	seen := map[string]bool{}
	var merged []SearchIssue
	for _, page := range pages {
		for _, item := range page {
			key := searchIssueKey(item)
			if seen[key] {
				continue
			}
			seen[key] = true
			merged = append(merged, item)
		}
	}
	sort.SliceStable(merged, func(a, b int) bool {
		return merged[a].UpdatedAt.After(merged[b].UpdatedAt)
	})
	return merged
}

// MyPRs runs GitHub's search/issues endpoint once per configured query
// and merges the results -- config's [behavior].my_prs_query, default the
// authored-or-review-requested pair "is:pr is:open author:@me" +
// "is:pr is:open review-requested:@me" (issue #11: browsing PRs across
// every repo without leaving the terminal).
//
// It is two calls rather than one because GitHub's search syntax has no
// OR between qualifiers -- `author:@me review-requested:@me` in one query
// means BOTH, which matches nothing. The wider `involves:@me` does fit in
// one query but is a different scope: it also pulls in PRs you were
// merely mentioned on or commented on.
//
// total is the count of distinct PRs returned, not the sum of GitHub's
// per-query total_count (which would double-count any overlap).
//
// Same tight search rate bucket as SearchRepos (30 req/min) -- fired once
// per screen push, never per keystroke; the per-query cache entries mean
// a repeat push costs nothing.
func (c *Client) MyPRs(queries []string, perPage int) (total int, items []SearchIssue, err error) {
	pages := make([][]SearchIssue, 0, len(queries))
	for _, q := range queries {
		page, err := c.myPRsPage(q, perPage)
		if err != nil {
			return 0, nil, err
		}
		pages = append(pages, page)
	}
	merged := MergeSearchIssues(pages)
	return len(merged), merged, nil
}

// myPRsPage fetches (or replays from cache) one query's result page.
func (c *Client) myPRsPage(query string, perPage int) ([]SearchIssue, error) {
	path := myPRsPath(query, perPage)
	if cached, ok := c.cache.get(path); ok {
		if resp, ok := cached.(myPRsResponse); ok {
			return resp.Items, nil
		}
	}
	var resp myPRsResponse
	if err := c.get(path, &resp); err != nil {
		return nil, err
	}
	c.cache.set(path, resp)
	return resp.Items, nil
}

// RefreshMyPRs busts every my-PRs search cache entry for queries/perPage.
// Used by the "r" refresh binding on the my-PRs screen.
func (c *Client) RefreshMyPRs(queries []string, perPage int) {
	for _, q := range queries {
		c.cache.bust(myPRsPath(q, perPage))
	}
}

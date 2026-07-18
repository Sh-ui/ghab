package gh

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// issueUser is the "user" object embedded in issues/comments -- only the
// login is shown.
type issueUser struct {
	Login string `json:"login"`
}

// Issue is one entry from repos/{o}/{r}/issues?state=all -- GitHub's
// issues endpoint returns pull requests too, distinguished only by the
// presence of a "pull_request" key (real issues never carry that key at
// all, not even as null). PullRequest is a json.RawMessage purely as a
// presence marker; its contents are never decoded further.
type Issue struct {
	Number      int             `json:"number"`
	Title       string          `json:"title"`
	State       string          `json:"state"`
	User        issueUser       `json:"user"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
	Comments    int             `json:"comments"`
	Body        string          `json:"body"`
	PullRequest json.RawMessage `json:"pull_request,omitempty"`
}

// Author returns the issue/PR author's login, or "" if the API omitted
// the user object.
func (i Issue) Author() string { return i.User.Login }

// IsPullRequest reports whether this issues-endpoint entry is actually a
// pull request.
func (i Issue) IsPullRequest() bool { return len(i.PullRequest) > 0 }

// Comment is one entry from repos/{o}/{r}/issues/{n}/comments.
type Comment struct {
	User      issueUser `json:"user"`
	CreatedAt time.Time `json:"created_at"`
	Body      string    `json:"body"`
}

// Author returns the comment author's login, or "" if the API omitted
// the user object.
func (c Comment) Author() string { return c.User.Login }

// IssueDetail is the combined result of fetching one issue/PR plus its
// comments (two GETs -- see Client.IssueDetail).
type IssueDetail struct {
	Issue    Issue
	Comments []Comment
}

// issuesPath builds the issues endpoint URL -- factored out so
// RefreshIssues busts the exact same cache key Issues fetches under.
func issuesPath(owner, repo string, perPage int) string {
	return fmt.Sprintf("repos/%s/%s/issues?state=all&per_page=%d", owner, repo, perPage)
}

// Issues fetches repos/{owner}/{repo}/issues?state=all&per_page={n} once
// and splits the result client-side into issues (no pull_request key) and
// prs (has it). One network round trip feeds both the issues and PRs
// tabs -- the second caller (whichever tab asks second) is served from
// cache, since both request the identical URL. A 404 (or empty body)
// yields two empty slices rather than an error, same rationale as
// Releases.
func (c *Client) Issues(owner, repo string, perPage int) (issues []Issue, prs []Issue, err error) {
	path := issuesPath(owner, repo, perPage)

	var all []Issue
	found := false
	if cached, ok := c.cache.get(path); ok {
		if a, ok := cached.([]Issue); ok {
			all = a
			found = true
		}
	}
	if !found {
		if err := c.get(path, &all); err != nil {
			var nf *NotFoundError
			if errors.As(err, &nf) {
				c.cache.set(path, []Issue{})
				return nil, nil, nil
			}
			return nil, nil, err
		}
		c.cache.set(path, all)
	}

	issues, prs = splitIssues(all)
	return issues, prs, nil
}

// splitIssues is Issues' pure split step, factored out so it's
// unit-testable without a network round trip: entries carrying a
// "pull_request" key go to prs, everything else to issues.
func splitIssues(all []Issue) (issues, prs []Issue) {
	for _, it := range all {
		if it.IsPullRequest() {
			prs = append(prs, it)
		} else {
			issues = append(issues, it)
		}
	}
	return issues, prs
}

// RefreshIssues busts the issues cache entry, forcing the next Issues
// call to hit the network. Used by the "r" refresh binding when the
// issues or prs tab is active -- one bust covers both, since they share
// the one underlying fetch.
func (c *Client) RefreshIssues(owner, repo string, perPage int) {
	c.cache.bust(issuesPath(owner, repo, perPage))
}

// IssueDetail fetches a single issue/PR (repos/{o}/{r}/issues/{n}) plus
// its comments (repos/{o}/{r}/issues/{n}/comments) -- two GETs combined
// into one typed result.
func (c *Client) IssueDetail(owner, repo string, number int) (IssueDetail, error) {
	issuePath := fmt.Sprintf("repos/%s/%s/issues/%d", owner, repo, number)
	commentsPath := fmt.Sprintf("repos/%s/%s/issues/%d/comments", owner, repo, number)

	var issue Issue
	found := false
	if cached, ok := c.cache.get(issuePath); ok {
		if it, ok := cached.(Issue); ok {
			issue = it
			found = true
		}
	}
	if !found {
		if err := c.get(issuePath, &issue); err != nil {
			return IssueDetail{}, err
		}
		c.cache.set(issuePath, issue)
	}

	var comments []Comment
	found = false
	if cached, ok := c.cache.get(commentsPath); ok {
		if cs, ok := cached.([]Comment); ok {
			comments = cs
			found = true
		}
	}
	if !found {
		if err := c.get(commentsPath, &comments); err != nil {
			return IssueDetail{}, err
		}
		c.cache.set(commentsPath, comments)
	}

	return IssueDetail{Issue: issue, Comments: comments}, nil
}

// RefreshIssueDetail busts the cache entries for one issue/PR's detail
// (the issue GET plus its comments list), forcing the next IssueDetail
// call to hit the network. Distinct from RefreshIssues, which busts the
// list fetch feeding the issues/prs tabs -- used by the "r" refresh
// binding when a PR's own detail/conversation view is open.
func (c *Client) RefreshIssueDetail(owner, repo string, number int) {
	c.cache.bust(fmt.Sprintf("repos/%s/%s/issues/%d", owner, repo, number))
	c.cache.bust(fmt.Sprintf("repos/%s/%s/issues/%d/comments", owner, repo, number))
}

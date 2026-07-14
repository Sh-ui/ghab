package gh

import (
	"fmt"
	"net/url"
	"time"
)

// User is the subset of GET users/{login} the profile screen shows.
type User struct {
	Login       string `json:"login"`
	Name        string `json:"name"`
	Bio         string `json:"bio"`
	Location    string `json:"location"`
	Company     string `json:"company"`
	Blog        string `json:"blog"`
	Followers   int    `json:"followers"`
	Following   int    `json:"following"`
	PublicRepos int    `json:"public_repos"`
	HTMLURL     string `json:"html_url"`
}

// UserRepo is one repo row on the profile screen (and, since the search
// endpoint returns the same shape, one search result row).
type UserRepo struct {
	Name            string    `json:"name"`
	FullName        string    `json:"full_name"`
	Description     string    `json:"description"`
	StargazersCount int       `json:"stargazers_count"`
	Language        string    `json:"language"`
	Fork            bool      `json:"fork"`
	Archived        bool      `json:"archived"`
	UpdatedAt       time.Time `json:"updated_at"`
	HTMLURL         string    `json:"html_url"`
}

func userPath(login string) string      { return "users/" + escapeRef(login) }
func userReposPath(login string) string { return userPath(login) + "/repos?sort=updated&per_page=100" }

// User fetches users/{login}, serving from cache when fresh.
func (c *Client) User(login string) (User, error) {
	path := userPath(login)
	if cached, ok := c.cache.get(path); ok {
		if u, ok := cached.(User); ok {
			return u, nil
		}
	}
	var u User
	if err := c.get(path, &u); err != nil {
		return User{}, err
	}
	c.cache.set(path, u)
	return u, nil
}

// UserRepos fetches users/{login}/repos sorted by most recently updated
// (first page of 100 -- the profile hop is a browse, not an archive dump).
func (c *Client) UserRepos(login string) ([]UserRepo, error) {
	path := userReposPath(login)
	if cached, ok := c.cache.get(path); ok {
		if repos, ok := cached.([]UserRepo); ok {
			return repos, nil
		}
	}
	var repos []UserRepo
	if err := c.get(path, &repos); err != nil {
		return nil, err
	}
	c.cache.set(path, repos)
	return repos, nil
}

// RefreshUser busts the profile screen's cache entries (user card +
// repo list) so its "r" binding refetches both.
func (c *Client) RefreshUser(login string) {
	c.cache.bust(userPath(login))
	c.cache.bust(userReposPath(login))
}

// searchResponse is the search/repositories envelope.
type searchResponse struct {
	TotalCount int        `json:"total_count"`
	Items      []UserRepo `json:"items"`
}

// SearchRepos queries search/repositories. The search rate bucket is
// tight (30 req/min) -- callers fire this only on an explicit enter,
// never per keystroke (BUILD.md's "HARD debounce").
func (c *Client) SearchRepos(query string, perPage int) (total int, items []UserRepo, err error) {
	path := fmt.Sprintf("search/repositories?q=%s&per_page=%d", url.QueryEscape(query), perPage)
	if cached, ok := c.cache.get(path); ok {
		if resp, ok := cached.(searchResponse); ok {
			return resp.TotalCount, resp.Items, nil
		}
	}
	var resp searchResponse
	if err := c.get(path, &resp); err != nil {
		return 0, nil, err
	}
	c.cache.set(path, resp)
	return resp.TotalCount, resp.Items, nil
}

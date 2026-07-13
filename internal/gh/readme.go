package gh

import (
	"errors"
	"fmt"
)

// NoReadmeError marks a repo with no readme (a 404 on the readme
// endpoint): the repo tab shows a muted "no readme" line for this, not an
// error banner -- it's a normal, expected repo state.
type NoReadmeError struct {
	Owner, Repo string
}

func (e *NoReadmeError) Error() string {
	return fmt.Sprintf("%s/%s: no readme", e.Owner, e.Repo)
}

// Readme fetches repos/{owner}/{repo}/readme raw (glamour renders it
// client-side). A 404 becomes a typed NoReadmeError rather than the
// generic NotFoundError, since "no readme" is expected for plenty of
// repos.
func (c *Client) Readme(owner, repo string) (string, error) {
	path := fmt.Sprintf("repos/%s/%s/readme", owner, repo)
	if cached, ok := c.cache.get(path); ok {
		if s, ok := cached.(string); ok {
			return s, nil
		}
	}
	data, err := c.getRaw(path)
	if err != nil {
		var nf *NotFoundError
		if errors.As(err, &nf) {
			return "", &NoReadmeError{Owner: owner, Repo: repo}
		}
		return "", err
	}
	s := string(data)
	c.cache.set(path, s)
	return s, nil
}

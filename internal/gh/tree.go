package gh

import "fmt"

// Tree entry types, as reported by the GitHub git trees API.
const (
	treeTypeBlob = "blob"
	treeTypeTree = "tree"
)

// TreeEntry is one entry from repos/{o}/{r}/git/trees/{branch}?recursive=1.
type TreeEntry struct {
	Path string `json:"path"`
	Type string `json:"type"` // "blob" (file), "tree" (dir), or "commit" (submodule)
	Size int64  `json:"size"`
	SHA  string `json:"sha"`
}

// IsDir reports whether the entry is a directory.
func (e TreeEntry) IsDir() bool { return e.Type == treeTypeTree }

// Tree is the result of a recursive tree fetch.
type Tree struct {
	Entries   []TreeEntry `json:"tree"`
	Truncated bool        `json:"truncated"`
}

// Tree fetches repos/{owner}/{repo}/git/trees/{branch}?recursive=1, serving
// from cache when fresh. GitHub caps recursive listings; if Truncated is
// true the response is incomplete and callers should fall back to Dir for
// lazy per-directory listing of whichever parts of the tree matter.
func (c *Client) Tree(owner, repo, branch string) (Tree, error) {
	path := fmt.Sprintf("repos/%s/%s/git/trees/%s?recursive=1", owner, repo, escapeRef(branch))
	if cached, ok := c.cache.get(path); ok {
		if t, ok := cached.(Tree); ok {
			return t, nil
		}
	}
	var t Tree
	if err := c.get(path, &t); err != nil {
		return Tree{}, err
	}
	c.cache.set(path, t)
	return t, nil
}

// DirEntry is one entry from repos/{o}/{r}/contents/{path}: a single
// directory's non-recursive listing, used only when Tree reports
// Truncated.
type DirEntry struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Type string `json:"type"` // "file" or "dir"
	Size int64  `json:"size"`
	SHA  string `json:"sha"`
}

// IsDir reports whether the entry is a directory.
func (e DirEntry) IsDir() bool { return e.Type == "dir" }

// Dir fetches repos/{owner}/{repo}/contents/{path}: one directory's
// listing. Used for lazy expansion when Tree's recursive fetch was
// truncated and the initial payload didn't include this directory's
// children.
func (c *Client) Dir(owner, repo, path string) ([]DirEntry, error) {
	reqPath := fmt.Sprintf("repos/%s/%s/contents/%s", owner, repo, escapePath(path))
	if cached, ok := c.cache.get(reqPath); ok {
		if d, ok := cached.([]DirEntry); ok {
			return d, nil
		}
	}
	var entries []DirEntry
	if err := c.get(reqPath, &entries); err != nil {
		return nil, err
	}
	c.cache.set(reqPath, entries)
	return entries, nil
}

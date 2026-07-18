package gh

import (
	"errors"
	"fmt"
	"sort"
	"time"
)

// PullFile is one entry from repos/{o}/{r}/pulls/{n}/files -- the
// files-changed list with +/- counts, plus GitHub's own per-file unified
// diff ("patch"). GitHub omits Patch for binary files, for files over its
// size threshold, and for pure renames with no content change -- see
// IsBinary/IsPureRename, which the diff view uses to render a placeholder
// instead of an empty pane.
type PullFile struct {
	Filename         string `json:"filename"`
	PreviousFilename string `json:"previous_filename"`
	Status           string `json:"status"` // added, removed, modified, renamed, copied, changed, unchanged
	Additions        int    `json:"additions"`
	Deletions        int    `json:"deletions"`
	Changes          int    `json:"changes"`
	Patch            string `json:"patch"`
}

// IsRenamed reports whether this entry is a rename (with or without
// content changes).
func (f PullFile) IsRenamed() bool { return f.Status == "renamed" }

// IsPureRename reports a rename with no content change -- GitHub omits
// Patch for these (distinct from a binary file, which also omits Patch).
func (f PullFile) IsPureRename() bool {
	return f.Patch == "" && f.Status == "renamed" && f.Additions == 0 && f.Deletions == 0
}

// IsBinary reports whether GitHub omitted a patch for a reason other than
// a pure rename -- the diff view's binary-file placeholder case.
func (f PullFile) IsBinary() bool { return f.Patch == "" && !f.IsPureRename() }

// pullFilesPath builds the files-changed endpoint URL -- factored out so
// RefreshPullFiles busts the exact cache key PullFiles fetches under.
func pullFilesPath(owner, repo string, number, perPage int) string {
	return fmt.Sprintf("repos/%s/%s/pulls/%d/files?per_page=%d", owner, repo, number, perPage)
}

// PullFiles fetches a PR's changed-files list, each with a per-file
// unified diff when GitHub provides one. A 404 (or empty body) yields an
// empty slice rather than an error, same rationale as Releases/Issues.
func (c *Client) PullFiles(owner, repo string, number, perPage int) ([]PullFile, error) {
	path := pullFilesPath(owner, repo, number, perPage)
	if cached, ok := c.cache.get(path); ok {
		if f, ok := cached.([]PullFile); ok {
			return f, nil
		}
	}
	var files []PullFile
	if err := c.get(path, &files); err != nil {
		var nf *NotFoundError
		if errors.As(err, &nf) {
			c.cache.set(path, []PullFile{})
			return nil, nil
		}
		return nil, err
	}
	c.cache.set(path, files)
	return files, nil
}

// RefreshPullFiles busts the cache entry for one PR's files-changed list.
// Used by the "r" refresh binding when the PR detail view is open.
func (c *Client) RefreshPullFiles(owner, repo string, number, perPage int) {
	c.cache.bust(pullFilesPath(owner, repo, number, perPage))
}

// ReviewComment is one entry from repos/{o}/{r}/pulls/{n}/comments -- a
// line-anchored review comment, distinct from the issue-style top-level
// comments IssueDetail fetches (BUILD.md extension: "review threads
// grouped by file:line").
type ReviewComment struct {
	ID           int64     `json:"id"`
	Path         string    `json:"path"`
	Line         int       `json:"line"`          // current-side line; 0 if the comment anchors to a since-removed/outdated line
	OriginalLine int       `json:"original_line"` // stable even after a force-push moves Line
	Side         string    `json:"side"`          // "RIGHT" (new file, the common case) or "LEFT" (old file); "" treated as RIGHT
	Body         string    `json:"body"`
	User         issueUser `json:"user"`
	CreatedAt    time.Time `json:"created_at"`
}

// Author returns the comment author's login, or "" if the API omitted the
// user object.
func (c ReviewComment) Author() string { return c.User.Login }

// AnchorLine returns the line this comment groups under: Line when
// present, else OriginalLine -- a comment on a line since edited or
// removed from the live diff still needs a stable grouping key.
func (c ReviewComment) AnchorLine() int {
	if c.Line > 0 {
		return c.Line
	}
	return c.OriginalLine
}

// IsLeftSide reports whether this comment anchors to the old (removed)
// side of the diff rather than the new (added/context) side.
func (c ReviewComment) IsLeftSide() bool { return c.Side == "LEFT" }

func pullCommentsPath(owner, repo string, number int) string {
	return fmt.Sprintf("repos/%s/%s/pulls/%d/comments?per_page=100", owner, repo, number)
}

// PullReviewComments fetches a PR's line-anchored review comments. A 404
// (or empty body) yields an empty slice rather than an error.
func (c *Client) PullReviewComments(owner, repo string, number int) ([]ReviewComment, error) {
	path := pullCommentsPath(owner, repo, number)
	if cached, ok := c.cache.get(path); ok {
		if cs, ok := cached.([]ReviewComment); ok {
			return cs, nil
		}
	}
	var comments []ReviewComment
	if err := c.get(path, &comments); err != nil {
		var nf *NotFoundError
		if errors.As(err, &nf) {
			c.cache.set(path, []ReviewComment{})
			return nil, nil
		}
		return nil, err
	}
	c.cache.set(path, comments)
	return comments, nil
}

// RefreshPullReviewComments busts the cache entry for one PR's review
// comments. Used by the "r" refresh binding when the PR detail view is
// open.
func (c *Client) RefreshPullReviewComments(owner, repo string, number int) {
	c.cache.bust(pullCommentsPath(owner, repo, number))
}

// LineThread is every comment anchored to one line within one file,
// ordered oldest-first (a conversation thread).
type LineThread struct {
	Line   int
	Side   string // "RIGHT" or "LEFT" -- see ReviewComment.Side
	Thread []ReviewComment
}

// FileThread groups one file's review comments by anchor line, lines in
// ascending order (BUILD.md extension: "review threads grouped by
// file:line").
type FileThread struct {
	Path  string
	Lines []LineThread
}

// lineKey is the (side, line) grouping key within one file -- a RIGHT-side
// line 12 and a LEFT-side line 12 are comments on different code and must
// not merge into one thread.
type lineKey struct {
	side string
	line int
}

// GroupReviewThreads buckets comments by path, then by (side, line)
// within each path -- files sorted alphabetically, lines ascending within
// each file, comments oldest-first within each line. Pure and
// unit-testable without a network round trip (see pulls_test.go).
func GroupReviewThreads(comments []ReviewComment) []FileThread {
	byPath := map[string]map[lineKey][]ReviewComment{}
	for _, cm := range comments {
		side := cm.Side
		if side == "" {
			side = "RIGHT"
		}
		key := lineKey{side: side, line: cm.AnchorLine()}
		if byPath[cm.Path] == nil {
			byPath[cm.Path] = map[lineKey][]ReviewComment{}
		}
		byPath[cm.Path][key] = append(byPath[cm.Path][key], cm)
	}

	paths := make([]string, 0, len(byPath))
	for p := range byPath {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	result := make([]FileThread, 0, len(paths))
	for _, p := range paths {
		byKey := byPath[p]
		keys := make([]lineKey, 0, len(byKey))
		for k := range byKey {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool {
			if keys[i].line != keys[j].line {
				return keys[i].line < keys[j].line
			}
			return keys[i].side < keys[j].side
		})

		lines := make([]LineThread, 0, len(keys))
		for _, k := range keys {
			thread := byKey[k]
			sort.Slice(thread, func(i, j int) bool { return thread[i].CreatedAt.Before(thread[j].CreatedAt) })
			lines = append(lines, LineThread{Line: k.line, Side: k.side, Thread: thread})
		}
		result = append(result, FileThread{Path: p, Lines: lines})
	}
	return result
}

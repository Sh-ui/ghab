package ui

import (
	"sort"
	"strings"

	"github.com/Sh-ui/ghab/internal/gh"
)

// treeNode is one row of the code tab's file tree: a client-side tree
// built from gh.Tree's flat entry list (or, for a truncated tree, lazily
// filled in per-directory from gh.Dir).
type treeNode struct {
	name     string
	path     string
	isDir    bool
	size     int64
	depth    int // 1 for a root-level entry
	expanded bool
	loaded   bool // children are known-complete; expanding won't trigger a Dir() fetch
	err      error
	children []*treeNode
}

// buildTree turns a recursive tree fetch's flat entry list into a nested
// treeNode structure rooted at a synthetic (always-expanded) root, so
// root-level entries are visible immediately per BUILD.md.
func buildTree(entries []gh.TreeEntry) *treeNode {
	root := &treeNode{isDir: true, expanded: true}
	index := map[string]*treeNode{"": root}

	// Process shallower paths before deeper ones so every entry's parent
	// directory node already exists in index by the time we reach it --
	// recursive tree responses aren't guaranteed to list a directory
	// before its contents.
	sorted := make([]gh.TreeEntry, len(entries))
	copy(sorted, entries)
	sort.Slice(sorted, func(i, j int) bool {
		return strings.Count(sorted[i].Path, "/") < strings.Count(sorted[j].Path, "/")
	})

	for _, e := range sorted {
		parentPath, name := splitTreePath(e.Path)
		parent, ok := index[parentPath]
		if !ok {
			// A blob/tree whose parent directory never appeared as its
			// own "tree" entry -- defensive skip rather than a crash;
			// shouldn't happen for a well-formed GitHub response.
			continue
		}
		node := &treeNode{
			name:  name,
			path:  e.Path,
			isDir: e.IsDir(),
			size:  e.Size,
			depth: parent.depth + 1,
		}
		parent.children = append(parent.children, node)
		if node.isDir {
			index[e.Path] = node
		}
	}

	sortChildren(root)
	return root
}

// markLoaded marks every directory node (including root) as loaded,
// trusting a non-truncated recursive tree fetch to already hold the full
// listing -- expanding a dir never triggers a lazy Dir() fetch in that
// case (BUILD.md: "lazy per-dir fetch only in the truncated case").
func markLoaded(n *treeNode) {
	n.loaded = true
	for _, c := range n.children {
		if c.isDir {
			markLoaded(c)
		}
	}
}

// sortChildren sorts n's children dirs-first then alphabetical, and
// recurses -- BUILD.md's code-tab ordering rule.
func sortChildren(n *treeNode) {
	sort.Slice(n.children, func(i, j int) bool {
		a, b := n.children[i], n.children[j]
		if a.isDir != b.isDir {
			return a.isDir
		}
		return a.name < b.name
	})
	for _, c := range n.children {
		sortChildren(c)
	}
}

func splitTreePath(path string) (parent, name string) {
	i := strings.LastIndex(path, "/")
	if i < 0 {
		return "", path
	}
	return path[:i], path[i+1:]
}

// flatten returns the tree's currently visible rows: a pre-order walk that
// only descends into expanded directories.
func flatten(root *treeNode) []*treeNode {
	var out []*treeNode
	var walk func(n *treeNode)
	walk = func(n *treeNode) {
		for _, c := range n.children {
			out = append(out, c)
			if c.isDir && c.expanded {
				walk(c)
			}
		}
	}
	walk(root)
	return out
}

// findNode looks up a node by its full repo-relative path, for routing a
// lazy dirMsg back to the node that requested it.
func findNode(root *treeNode, path string) *treeNode {
	if path == "" {
		return root
	}
	segments := strings.Split(path, "/")
	n := root
	for _, seg := range segments {
		found := false
		for _, c := range n.children {
			if c.name == seg {
				n = c
				found = true
				break
			}
		}
		if !found {
			return nil
		}
	}
	return n
}

// dirEntriesToNodes converts a lazy Dir() listing into sorted child nodes
// for a truncated-tree expansion.
func dirEntriesToNodes(entries []gh.DirEntry, parentDepth int) []*treeNode {
	out := make([]*treeNode, 0, len(entries))
	for _, e := range entries {
		out = append(out, &treeNode{
			name:  e.Name,
			path:  e.Path,
			isDir: e.IsDir(),
			size:  e.Size,
			depth: parentDepth + 1,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].isDir != out[j].isDir {
			return out[i].isDir
		}
		return out[i].name < out[j].name
	})
	return out
}

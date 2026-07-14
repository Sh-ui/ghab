package ui

import (
	"fmt"
	"strings"

	"github.com/Sh-ui/ghab/internal/gh"
	"github.com/Sh-ui/ghab/internal/ui/style"
)

// repoList is the scrollable repo-row list shared by the profile screen
// (a user's repos) and the search screen (search results): cursor
// movement, viewport windowing, and the one-line-per-repo row rendering.
type repoList struct {
	theme style.Theme

	items  []gh.UserRepo
	cursor int
	offset int
	rows   int // visible rows, set by resize
	width  int
}

func newRepoList(theme style.Theme) *repoList {
	return &repoList{theme: theme, rows: 1}
}

func (l *repoList) setItems(items []gh.UserRepo) {
	l.items = items
	l.cursor = 0
	l.offset = 0
}

func (l *repoList) resize(width, rows int) {
	l.width = width
	if rows < 1 {
		rows = 1
	}
	l.rows = rows
	l.clamp()
}

func (l *repoList) move(delta int) {
	l.cursor += delta
	l.clamp()
}

func (l *repoList) clamp() {
	if l.cursor < 0 {
		l.cursor = 0
	}
	if l.cursor > len(l.items)-1 {
		l.cursor = len(l.items) - 1
	}
	if l.cursor < l.offset {
		l.offset = l.cursor
	}
	if l.cursor >= l.offset+l.rows {
		l.offset = l.cursor - l.rows + 1
	}
	if l.offset < 0 {
		l.offset = 0
	}
}

// selected returns the repo under the cursor, or nil for an empty list.
func (l *repoList) selected() *gh.UserRepo {
	if len(l.items) == 0 || l.cursor >= len(l.items) {
		return nil
	}
	return &l.items[l.cursor]
}

// View renders the visible window: one line per repo -- name in the
// accent-alt (repo-name) color, stats + age muted, description dim,
// truncated to width. The cursor row is the house full-width selection bar.
func (l *repoList) View() string {
	if len(l.items) == 0 {
		return l.theme.MutedText.Render("no repositories")
	}

	end := l.offset + l.rows
	if end > len(l.items) {
		end = len(l.items)
	}

	var b strings.Builder
	for i := l.offset; i < end; i++ {
		r := l.items[i]

		meta := fmt.Sprintf("★ %d", r.StargazersCount)
		if r.Language != "" {
			meta += "  " + r.Language
		}
		meta += "  " + relativeAge(r.UpdatedAt)
		if r.Fork {
			meta += "  (fork)"
		}
		if r.Archived {
			meta += "  (archived)"
		}

		desc := r.Description

		if i == l.cursor {
			line := truncateTo(fmt.Sprintf("%s  %s  %s", r.Name, meta, desc), l.width)
			pad := l.width - len([]rune(line))
			if pad > 0 {
				line += strings.Repeat(" ", pad)
			}
			b.WriteString(l.theme.Selection.Render(line))
		} else {
			plainLen := len([]rune(r.Name)) + 2 + len([]rune(meta)) + 2
			descRoom := l.width - plainLen
			line := l.theme.AccentAltText.Render(r.Name) + "  " + l.theme.MutedText.Render(meta)
			if descRoom > 3 && desc != "" {
				line += "  " + l.theme.DimText.Render(truncateTo(desc, descRoom))
			}
			b.WriteString(line)
		}
		if i < end-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}

// truncateTo hard-truncates s to max runes with a ".." tail.
func truncateTo(s string, max int) string {
	if max <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	if max <= 2 {
		return string(runes[:max])
	}
	return string(runes[:max-2]) + ".."
}

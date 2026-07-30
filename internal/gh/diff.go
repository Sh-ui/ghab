package gh

import (
	"fmt"
	"strconv"
	"strings"
)

// DiffLineKind classifies one parsed line of a unified diff patch, for
// the PR detail view's per-file diff coloring (BUILD.md extension:
// "add/remove line coloring").
type DiffLineKind int

const (
	DiffContext DiffLineKind = iota
	DiffAdd
	DiffRemove
	DiffHunkHeader
)

// DiffLine is one rendered line of a parsed patch. OldLine/NewLine are the
// 1-based line numbers on the removed/added side of the diff -- 0 when
// not applicable to this line's kind (an add line has no OldLine, a
// remove line has no NewLine, a hunk header has neither). They exist so
// the diff view can place review threads (anchored to a specific
// old/new-side line) right after the diff line they belong to.
type DiffLine struct {
	Kind    DiffLineKind
	Text    string
	OldLine int
	NewLine int
}

// noNewlineMarker is unified diff's marker for a hunk's final line having
// no trailing newline in the source file -- it appears mid-hunk (right
// after the add/remove/context line it describes) and is not itself a
// line of either file's content, so it must not advance either line
// counter (see ParsePatch).
const noNewlineMarker = `\ No newline at end of file`

// ParsePatch splits a GitHub "patch" string (a per-file unified diff body
// -- GitHub's /pulls/{n}/files response omits the "--- a/f" / "+++ b/f"
// file header lines other diff tools include, starting straight at the
// first "@@" hunk header) into typed, line-numbered lines. Empty or
// whitespace-only input (GitHub omits Patch for binary files, pure
// renames, and oversized text diffs -- see PullFile.Omission) yields nil.
func ParsePatch(patch string) []DiffLine {
	if strings.TrimSpace(patch) == "" {
		return nil
	}
	raw := strings.Split(patch, "\n")
	lines := make([]DiffLine, 0, len(raw))
	var oldLine, newLine int
	for _, l := range raw {
		switch {
		case strings.HasPrefix(l, "@@"):
			oldLine, newLine = parseHunkHeader(l)
			lines = append(lines, DiffLine{Kind: DiffHunkHeader, Text: l})
		case strings.HasPrefix(l, noNewlineMarker):
			// Not a line of either file -- record it (still rendered as
			// context) but don't advance oldLine/newLine, or every
			// subsequent line in the hunk would be off by one.
			lines = append(lines, DiffLine{Kind: DiffContext, Text: l})
		case strings.HasPrefix(l, "+"):
			lines = append(lines, DiffLine{Kind: DiffAdd, Text: l, NewLine: newLine})
			newLine++
		case strings.HasPrefix(l, "-"):
			lines = append(lines, DiffLine{Kind: DiffRemove, Text: l, OldLine: oldLine})
			oldLine++
		default:
			lines = append(lines, DiffLine{Kind: DiffContext, Text: l, OldLine: oldLine, NewLine: newLine})
			oldLine++
			newLine++
		}
	}
	return lines
}

// parseHunkHeader extracts the starting old/new line numbers from a
// "@@ -a,b +c,d @@ ..." hunk header. A malformed header (should not
// happen against a real GitHub patch) falls back to 0,0 -- fail-soft,
// same philosophy as config's bad-value handling: never crash on
// untrusted-shaped input, just degrade the feature (line numbers omitted
// for that hunk) gracefully.
func parseHunkHeader(h string) (oldStart, newStart int) {
	parts := strings.SplitN(h, "@@", 3)
	if len(parts) < 2 {
		return 0, 0
	}
	fields := strings.Fields(parts[1]) // e.g. ["-12,7", "+12,9"]
	if len(fields) < 2 {
		return 0, 0
	}
	return parseHunkNum(fields[0]), parseHunkNum(fields[1])
}

func parseHunkNum(f string) int {
	f = strings.TrimPrefix(f, "+")
	f = strings.TrimPrefix(f, "-")
	if idx := strings.Index(f, ","); idx >= 0 {
		f = f[:idx]
	}
	n, err := strconv.Atoi(f)
	if err != nil {
		return 0
	}
	return n
}

// TrimContext caps each run of consecutive DiffContext lines at
// contextLines per side, collapsing any excess into a single placeholder
// line -- the [behavior].diff_context_lines knob. GitHub's own patch
// already caps context at ~3 lines per hunk, so this only visibly bites
// when contextLines is set below GitHub's default (a config-first knob
// for narrower terminals / less scrolling); contextLines <= 0 disables
// trimming entirely (fail-soft: a bad config value floors at the compiled
// default in config.go, but this guard also protects a deliberate 0).
func TrimContext(lines []DiffLine, contextLines int) []DiffLine {
	if contextLines <= 0 {
		return lines
	}
	out := make([]DiffLine, 0, len(lines))
	i := 0
	for i < len(lines) {
		if lines[i].Kind != DiffContext {
			out = append(out, lines[i])
			i++
			continue
		}
		j := i
		for j < len(lines) && lines[j].Kind == DiffContext {
			j++
		}
		run := lines[i:j]
		if len(run) <= contextLines*2 {
			out = append(out, run...)
		} else {
			out = append(out, run[:contextLines]...)
			hidden := len(run) - contextLines*2
			out = append(out, DiffLine{Kind: DiffContext, Text: fmt.Sprintf("... %d unchanged lines ...", hidden)})
			out = append(out, run[len(run)-contextLines:]...)
		}
		i = j
	}
	return out
}

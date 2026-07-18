package gh

import (
	"reflect"
	"testing"
)

func TestParsePatch(t *testing.T) {
	cases := []struct {
		name  string
		patch string
		want  []DiffLine
	}{
		{
			name:  "empty patch",
			patch: "",
			want:  nil,
		},
		{
			name:  "whitespace-only patch",
			patch: "   \n\t",
			want:  nil,
		},
		{
			name:  "one hunk, add and remove",
			patch: "@@ -1,3 +1,3 @@\n context\n-old line\n+new line\n context2",
			want: []DiffLine{
				{Kind: DiffHunkHeader, Text: "@@ -1,3 +1,3 @@"},
				{Kind: DiffContext, Text: " context", OldLine: 1, NewLine: 1},
				{Kind: DiffRemove, Text: "-old line", OldLine: 2},
				{Kind: DiffAdd, Text: "+new line", NewLine: 2},
				{Kind: DiffContext, Text: " context2", OldLine: 3, NewLine: 3},
			},
		},
		{
			name:  "two hunks re-anchor line numbers",
			patch: "@@ -1,1 +1,1 @@\n-a\n+b\n@@ -10,1 +10,2 @@\n context\n+added",
			want: []DiffLine{
				{Kind: DiffHunkHeader, Text: "@@ -1,1 +1,1 @@"},
				{Kind: DiffRemove, Text: "-a", OldLine: 1},
				{Kind: DiffAdd, Text: "+b", NewLine: 1},
				{Kind: DiffHunkHeader, Text: "@@ -10,1 +10,2 @@"},
				{Kind: DiffContext, Text: " context", OldLine: 10, NewLine: 10},
				{Kind: DiffAdd, Text: "+added", NewLine: 11},
			},
		},
		{
			name:  "malformed hunk header falls back to zero line numbers",
			patch: "@@ garbage @@\n+added",
			want: []DiffLine{
				{Kind: DiffHunkHeader, Text: "@@ garbage @@"},
				{Kind: DiffAdd, Text: "+added", NewLine: 0},
			},
		},
		{
			// A "\ No newline at end of file" marker describes the line
			// above it, not a line of its own -- it must not advance
			// either line counter, or every line after it in the hunk
			// would be miscounted by one (the bug this case guards).
			name:  "no-newline marker does not skew subsequent line numbers",
			patch: "@@ -1,2 +1,2 @@\n-old\n\\ No newline at end of file\n+new\n context",
			want: []DiffLine{
				{Kind: DiffHunkHeader, Text: "@@ -1,2 +1,2 @@"},
				{Kind: DiffRemove, Text: "-old", OldLine: 1},
				{Kind: DiffContext, Text: `\ No newline at end of file`},
				{Kind: DiffAdd, Text: "+new", NewLine: 1},
				{Kind: DiffContext, Text: " context", OldLine: 2, NewLine: 2},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ParsePatch(tc.patch)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("ParsePatch(%q) = %#v, want %#v", tc.patch, got, tc.want)
			}
		})
	}
}

func TestTrimContext(t *testing.T) {
	mkContext := func(n int) []DiffLine {
		out := make([]DiffLine, n)
		for i := range out {
			out[i] = DiffLine{Kind: DiffContext, Text: "ctx"}
		}
		return out
	}

	cases := []struct {
		name              string
		lines             []DiffLine
		contextLines      int
		wantLen           int
		wantPlaceholderAt int // index of the collapsed placeholder, -1 if none expected
	}{
		{
			name:              "contextLines <= 0 disables trimming",
			lines:             mkContext(20),
			contextLines:      0,
			wantLen:           20,
			wantPlaceholderAt: -1,
		},
		{
			name:              "run shorter than 2x contextLines is untouched",
			lines:             mkContext(4),
			contextLines:      3,
			wantLen:           4,
			wantPlaceholderAt: -1,
		},
		{
			name:              "long run collapses to head + placeholder + tail",
			lines:             mkContext(10),
			contextLines:      3,
			wantLen:           3 + 1 + 3,
			wantPlaceholderAt: 3,
		},
		{
			name: "non-context lines are never collapsed",
			lines: append(append(mkContext(10),
				DiffLine{Kind: DiffAdd, Text: "+x"}),
				mkContext(10)...),
			contextLines:      2,
			wantLen:           2 + 1 + 2 + 1 + 2 + 1 + 2,
			wantPlaceholderAt: -1, // multiple placeholders; checked by length only
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := TrimContext(tc.lines, tc.contextLines)
			if len(got) != tc.wantLen {
				t.Fatalf("TrimContext len = %d, want %d (%#v)", len(got), tc.wantLen, got)
			}
			if tc.wantPlaceholderAt >= 0 {
				if got[tc.wantPlaceholderAt].Text == "ctx" {
					t.Errorf("expected a collapsed placeholder at index %d, got %q", tc.wantPlaceholderAt, got[tc.wantPlaceholderAt].Text)
				}
			}
		})
	}
}

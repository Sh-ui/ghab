package gh

import (
	"testing"
	"time"
)

func TestPullFileClassification(t *testing.T) {
	cases := []struct {
		name           string
		file           PullFile
		wantBinary     bool
		wantPureRename bool
	}{
		{
			name:           "normal modified file with a patch",
			file:           PullFile{Status: "modified", Patch: "@@ -1 +1 @@\n-a\n+b", Additions: 1, Deletions: 1},
			wantBinary:     false,
			wantPureRename: false,
		},
		{
			name:           "binary file (no patch, not a rename)",
			file:           PullFile{Status: "modified", Patch: ""},
			wantBinary:     true,
			wantPureRename: false,
		},
		{
			name:           "pure rename (no content change)",
			file:           PullFile{Status: "renamed", Patch: "", Additions: 0, Deletions: 0},
			wantBinary:     false,
			wantPureRename: true,
		},
		{
			name:           "rename with content change carries a patch",
			file:           PullFile{Status: "renamed", Patch: "@@ -1 +1 @@\n-a\n+b", Additions: 1, Deletions: 1},
			wantBinary:     false,
			wantPureRename: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.file.IsBinary(); got != tc.wantBinary {
				t.Errorf("IsBinary() = %v, want %v", got, tc.wantBinary)
			}
			if got := tc.file.IsPureRename(); got != tc.wantPureRename {
				t.Errorf("IsPureRename() = %v, want %v", got, tc.wantPureRename)
			}
		})
	}
}

func TestReviewCommentAnchorLine(t *testing.T) {
	cases := []struct {
		name string
		c    ReviewComment
		want int
	}{
		{name: "current line present", c: ReviewComment{Line: 42, OriginalLine: 10}, want: 42},
		{name: "line removed, falls back to original", c: ReviewComment{Line: 0, OriginalLine: 10}, want: 10},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.c.AnchorLine(); got != tc.want {
				t.Errorf("AnchorLine() = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestGroupReviewThreads(t *testing.T) {
	t1 := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	t2 := t1.Add(time.Hour)

	comments := []ReviewComment{
		{Path: "b.go", Line: 5, Side: "RIGHT", Body: "b5-second", CreatedAt: t2},
		{Path: "a.go", Line: 20, Side: "RIGHT", Body: "a20", CreatedAt: t1},
		{Path: "b.go", Line: 5, Side: "RIGHT", Body: "b5-first", CreatedAt: t1},
		{Path: "a.go", Line: 3, Side: "RIGHT", Body: "a3", CreatedAt: t1},
		{Path: "a.go", Line: 3, Side: "LEFT", Body: "a3-left", CreatedAt: t1},
	}

	got := GroupReviewThreads(comments)

	if len(got) != 2 {
		t.Fatalf("expected 2 files, got %d (%#v)", len(got), got)
	}
	if got[0].Path != "a.go" || got[1].Path != "b.go" {
		t.Fatalf("expected files sorted alphabetically, got %q, %q", got[0].Path, got[1].Path)
	}

	a := got[0]
	if len(a.Lines) != 3 {
		t.Fatalf("expected 3 distinct (side,line) groups in a.go, got %d (%#v)", len(a.Lines), a.Lines)
	}
	if a.Lines[0].Line != 3 || a.Lines[1].Line != 3 || a.Lines[2].Line != 20 {
		t.Fatalf("expected lines ascending (3, 3, 20), got %v", []int{a.Lines[0].Line, a.Lines[1].Line, a.Lines[2].Line})
	}
	// The two line-3 groups must be split by side, never merged.
	sides := map[string]bool{a.Lines[0].Side: true, a.Lines[1].Side: true}
	if !sides["LEFT"] || !sides["RIGHT"] {
		t.Fatalf("expected one LEFT and one RIGHT group at line 3, got %v", []string{a.Lines[0].Side, a.Lines[1].Side})
	}

	b := got[1]
	if len(b.Lines) != 1 || len(b.Lines[0].Thread) != 2 {
		t.Fatalf("expected b.go line 5 to hold a 2-comment thread, got %#v", b.Lines)
	}
	if b.Lines[0].Thread[0].Body != "b5-first" || b.Lines[0].Thread[1].Body != "b5-second" {
		t.Fatalf("expected thread ordered oldest-first, got %q then %q", b.Lines[0].Thread[0].Body, b.Lines[0].Thread[1].Body)
	}
}

func TestGroupReviewThreadsEmpty(t *testing.T) {
	if got := GroupReviewThreads(nil); len(got) != 0 {
		t.Errorf("expected no file groups for no comments, got %#v", got)
	}
}

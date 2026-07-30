package gh

import (
	"testing"
	"time"
)

func TestSearchIssueRepoFullName(t *testing.T) {
	cases := []struct {
		name string
		url  string
		want string
	}{
		{name: "normal repo url", url: "https://api.github.com/repos/Sh-ui/example", want: "Sh-ui/example"},
		{name: "nested-looking owner name", url: "https://api.github.com/repos/foo/bar-repos-baz", want: "foo/bar-repos-baz"},
		{name: "empty", url: "", want: ""},
		{name: "missing marker", url: "not-a-url", want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			i := SearchIssue{RepositoryURL: tc.url}
			if got := i.RepoFullName(); got != tc.want {
				t.Errorf("RepoFullName() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestMyPRsPath(t *testing.T) {
	got := myPRsPath("is:pr is:open author:@me", 30)
	want := "search/issues?q=is%3Apr+is%3Aopen+author%3A%40me&per_page=30"
	if got != want {
		t.Errorf("myPRsPath() = %q, want %q", got, want)
	}
}

// Each configured query gets its own cache key, so RefreshMyPRs busting
// "the" entry is not enough -- it has to bust every one of them.
func TestMyPRsPathDistinctPerQuery(t *testing.T) {
	a := myPRsPath("is:pr is:open author:@me", 30)
	b := myPRsPath("is:pr is:open review-requested:@me", 30)
	if a == b {
		t.Fatalf("expected distinct cache keys per query, both = %q", a)
	}
}

func TestMergeSearchIssues(t *testing.T) {
	const repoA = "https://api.github.com/repos/Sh-ui/example"
	const repoB = "https://api.github.com/repos/Sh-ui/other"

	older := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	newer := time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC)
	newest := time.Date(2026, 7, 29, 0, 0, 0, 0, time.UTC)

	authored := []SearchIssue{
		{Number: 16, RepositoryURL: repoA, UpdatedAt: newer},
		{Number: 3, RepositoryURL: repoA, UpdatedAt: older},
	}
	reviewRequested := []SearchIssue{
		// Same PR as authored[0] -- must collapse to one row, not two.
		{Number: 16, RepositoryURL: repoA, UpdatedAt: newer},
		// Same NUMBER as authored[1] but a different repo -- a distinct
		// PR, must survive the de-duplication.
		{Number: 3, RepositoryURL: repoB, UpdatedAt: newest},
	}

	got := MergeSearchIssues([][]SearchIssue{authored, reviewRequested})
	if len(got) != 3 {
		t.Fatalf("MergeSearchIssues() returned %d items, want 3: %+v", len(got), got)
	}
	want := []struct {
		number int
		repo   string
	}{
		{3, repoB},  // newest
		{16, repoA}, // newer
		{3, repoA},  // older
	}
	for i, w := range want {
		if got[i].Number != w.number || got[i].RepositoryURL != w.repo {
			t.Errorf("item %d = #%d %s, want #%d %s", i, got[i].Number, got[i].RepositoryURL, w.number, w.repo)
		}
	}
}

func TestMergeSearchIssuesEmpty(t *testing.T) {
	if got := MergeSearchIssues(nil); got != nil {
		t.Errorf("MergeSearchIssues(nil) = %+v, want nil", got)
	}
	if got := MergeSearchIssues([][]SearchIssue{{}, {}}); got != nil {
		t.Errorf("MergeSearchIssues(empty pages) = %+v, want nil", got)
	}
}

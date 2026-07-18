package gh

import "testing"

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
	got := myPRsPath("is:pr is:open involves:@me", 30)
	want := "search/issues?q=is%3Apr+is%3Aopen+involves%3A%40me&per_page=30"
	if got != want {
		t.Errorf("myPRsPath() = %q, want %q", got, want)
	}
}

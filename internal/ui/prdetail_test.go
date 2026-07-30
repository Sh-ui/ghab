package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"

	"github.com/Sh-ui/ghab/internal/config"
	"github.com/Sh-ui/ghab/internal/gh"
	"github.com/Sh-ui/ghab/internal/ui/style"
)

// diffModelWithOpenDiff builds a prDetailModel sitting in the diff view on
// one file, exactly as it would be after enterDiff -- no client, since
// every path under test here is local (the fetch results arrive as
// messages). The zero-value Theme renders plain text, which is what these
// assertions want to match against.
func diffModelWithOpenDiff(t *testing.T, file gh.PullFile) *prDetailModel {
	t.Helper()
	vp := viewport.New(80, 20)
	vp.SetHorizontalStep(diffHorizontalStep)
	p := &prDetailModel{
		cfg:               config.Defaults(),
		theme:             style.Theme{},
		owner:             "Sh-ui",
		repo:              "dev-root",
		number:            16,
		perPage:           30,
		spinner:           spinner.New(spinner.WithSpinner(spinner.Dot)),
		diffVp:            vp,
		convVp:            viewport.New(80, 20),
		width:             80,
		rows:              20,
		fileRows:          18,
		renderedConvWidth: -1,
		renderedDiffWidth: -1,
		files:             []gh.PullFile{file},
		view:              prViewDiff,
	}
	f := file
	p.diffFile = &f
	p.applyDiffContent()
	return p
}

// filesArrived feeds the model the response a refetch would produce.
func filesArrived(p *prDetailModel, files []gh.PullFile) {
	p.Update(prFilesMsg{owner: p.owner, repo: p.repo, number: p.number, files: files})
}

// The finding: "r" refreshed the file list but left the rendered diff
// alone, so an open diff kept showing the pre-refresh patch.
func TestRefreshBustsOpenDiff(t *testing.T) {
	const oldPatch = "@@ -1 +1 @@\n-old line\n+PRE_REFRESH_CONTENT"
	const newPatch = "@@ -1 +1 @@\n-old line\n+POST_REFRESH_CONTENT"

	p := diffModelWithOpenDiff(t, gh.PullFile{
		Filename: "a.go", Status: "modified", Patch: oldPatch, Additions: 1, Deletions: 1, Changes: 2,
	})
	if !strings.Contains(p.diffVp.View(), "PRE_REFRESH_CONTENT") {
		t.Fatalf("setup: diff viewport does not show the original patch:\n%s", p.diffVp.View())
	}

	// What "r" does locally, minus the network: mark the fetches in
	// flight and invalidate the render.
	p.convLoading, p.filesLoading, p.threadsLoading = true, true, true
	p.invalidateDiffRender()
	p.applyDiffContent()

	if got := p.diffVp.View(); strings.Contains(got, "PRE_REFRESH_CONTENT") {
		t.Errorf("in-flight refresh still shows pre-refresh content:\n%s", got)
	}
	if got := p.diffView(); !strings.Contains(got, "loading diff") {
		t.Errorf("in-flight refresh does not show a loading state:\n%s", got)
	}

	filesArrived(p, []gh.PullFile{{
		Filename: "a.go", Status: "modified", Patch: newPatch, Additions: 1, Deletions: 1, Changes: 2,
	}})

	got := p.diffVp.View()
	if strings.Contains(got, "PRE_REFRESH_CONTENT") {
		t.Errorf("diff still shows pre-refresh content after the refetch landed:\n%s", got)
	}
	if !strings.Contains(got, "POST_REFRESH_CONTENT") {
		t.Errorf("diff does not show the refreshed patch:\n%s", got)
	}
	if p.diffFile.Patch != newPatch {
		t.Errorf("diffFile still points at the pre-refresh entry")
	}
}

// A refresh that drops the open file from the PR entirely (force-push,
// dropped commit) has no diff left to show, so the view steps back up to
// the files list rather than holding a stale one.
func TestRefreshDroppingOpenFileFallsBackToFilesView(t *testing.T) {
	p := diffModelWithOpenDiff(t, gh.PullFile{
		Filename: "gone.go", Status: "modified", Patch: "@@ -1 +1 @@\n-a\n+b", Additions: 1, Deletions: 1, Changes: 2,
	})

	p.convLoading, p.filesLoading, p.threadsLoading = true, true, true
	p.invalidateDiffRender()
	filesArrived(p, []gh.PullFile{{
		Filename: "other.go", Status: "modified", Patch: "@@ -1 +1 @@\n-c\n+d", Additions: 1, Deletions: 1, Changes: 2,
	}})

	if p.view != prViewFiles {
		t.Errorf("view = %v, want prViewFiles after the open file vanished", p.view)
	}
	if p.diffFile != nil {
		t.Errorf("diffFile = %+v, want nil after the open file vanished", p.diffFile)
	}
}

// The render cache keys on (filename, width), neither of which a refresh
// changes -- so an un-busted cache would replay the old content even
// though diffFile itself was updated. Guards the cache specifically.
func TestSameFileNewPatchRerendersAfterInvalidate(t *testing.T) {
	p := diffModelWithOpenDiff(t, gh.PullFile{
		Filename: "a.go", Status: "modified", Patch: "@@ -1 +1 @@\n-x\n+FIRST", Additions: 1, Deletions: 1, Changes: 2,
	})
	filesArrived(p, []gh.PullFile{{
		Filename: "a.go", Status: "modified", Patch: "@@ -1 +1 @@\n-x\n+SECOND", Additions: 1, Deletions: 1, Changes: 2,
	}})
	got := p.diffVp.View()
	if strings.Contains(got, "FIRST") || !strings.Contains(got, "SECOND") {
		t.Errorf("render cache replayed the old patch:\n%s", got)
	}
}

// The binary mislabel: an oversized text diff and a real binary both
// arrive with no patch, and must not read the same way.
func TestDiffPlaceholdersDistinguishBinaryFromWithheldDiff(t *testing.T) {
	cases := []struct {
		name       string
		file       gh.PullFile
		wantBody   string
		unwantBody string
		wantRow    string
	}{
		{
			name:       "binary",
			file:       gh.PullFile{Filename: "logo.png", Status: "modified"},
			wantBody:   "binary file",
			unwantBody: "diff not loaded",
			wantRow:    "(binary)",
		},
		{
			name:       "withheld text diff",
			file:       gh.PullFile{Filename: "huge.json", Status: "modified", Additions: 9000, Deletions: 12, Changes: 9012},
			wantBody:   "diff not loaded",
			unwantBody: "binary file",
			wantRow:    "(diff not loaded)",
		},
		{
			name:       "pure rename",
			file:       gh.PullFile{Filename: "new.go", PreviousFilename: "old.go", Status: "renamed"},
			wantBody:   "renamed",
			unwantBody: "binary file",
			wantRow:    "(renamed, no changes)",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := diffModelWithOpenDiff(t, tc.file)
			body := p.buildDiffContent()
			if !strings.Contains(body, tc.wantBody) {
				t.Errorf("diff body = %q, want it to mention %q", body, tc.wantBody)
			}
			if strings.Contains(body, tc.unwantBody) {
				t.Errorf("diff body = %q, must not mention %q", body, tc.unwantBody)
			}
			row := p.renderFileRow(tc.file, false)
			if !strings.Contains(row, tc.wantRow) {
				t.Errorf("file row = %q, want it to mention %q", row, tc.wantRow)
			}
		})
	}
}

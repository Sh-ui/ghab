package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/Sh-ui/ghab/internal/config"
	"github.com/Sh-ui/ghab/internal/gh"
	"github.com/Sh-ui/ghab/internal/ui/style"
)

// repoTabs are the tab bar entries, in order. l/h cycle through them.
// Every tab is a dedicated sub-model as of M3 (readmeTab, codeTab,
// releasesTab, issuesTab x2) -- there are no placeholder viewports left.
var repoTabs = []string{"readme", "code", "releases", "issues", "prs"}

// repoMetaMsg carries the async result of fetching repos/{owner}/{repo}.
type repoMetaMsg struct {
	meta gh.RepoMeta
	err  error
}

// issuesFetchMsg carries the async result of the single
// repos/{owner}/{repo}/issues?state=all fetch that feeds both the issues
// and prs tabs (see fetchIssuesCmd and BUILD.md's M3 "one fetch feeds
// both tabs" note).
type issuesFetchMsg struct {
	issues []gh.Issue
	prs    []gh.Issue
	err    error
}

// RepoScreen shows a repo's framed header (owner/name, description,
// stars, branch, topics) plus the readme/code/releases/issues/prs tab
// bar. Each tab is owned by its own sub-model, kept alive (not rebuilt)
// across tab switches so background fetches and scroll position survive.
type RepoScreen struct {
	cfg    config.Config
	theme  style.Theme
	client *gh.Client

	owner, repo string

	loading bool
	err     error
	meta    gh.RepoMeta
	spinner spinner.Model

	active   int
	readme   *readmeTab
	code     *codeTab
	releases *releasesTab
	issues   *issuesTab
	prs      *issuesTab

	width, height int
}

// NewRepoScreen builds a Repo screen for owner/repo. Fetching starts in
// Init. readmeStylePath is the glamour stylesheet resolved once at app
// startup (main.go, via config.ResolveReadmeStyle) -- "" means fall back
// to glamour's built-in "dark" standard style. It's reused (not
// re-resolved) for the releases and issues/PRs tabs' detail bodies, which
// render through the same glamour setup as the readme tab.
func NewRepoScreen(cfg config.Config, theme style.Theme, client *gh.Client, readmeStylePath, owner, repo string) *RepoScreen {
	sp := spinner.New(spinner.WithSpinner(spinner.Dot), spinner.WithStyle(theme.Spinner))
	styleOpt := readmeStyleOption{stylePath: readmeStylePath}
	return &RepoScreen{
		cfg:      cfg,
		theme:    theme,
		client:   client,
		owner:    owner,
		repo:     repo,
		loading:  true,
		spinner:  sp,
		readme:   newReadmeTab(theme, client, styleOpt, owner, repo),
		code:     newCodeTab(cfg, theme, client, owner, repo),
		releases: newReleasesTab(cfg, theme, client, styleOpt, owner, repo),
		issues:   newIssuesTab(cfg, theme, client, styleOpt, owner, repo, issueKindIssue),
		prs:      newIssuesTab(cfg, theme, client, styleOpt, owner, repo, issueKindPR),
	}
}

func (r *RepoScreen) fetchCmd() tea.Cmd {
	owner, repo := r.owner, r.repo
	client := r.client
	return func() tea.Msg {
		meta, err := client.RepoMeta(owner, repo)
		return repoMetaMsg{meta: meta, err: err}
	}
}

// fetchIssuesCmd issues the ONE repos/{o}/{r}/issues?state=all fetch that
// feeds both the issues and prs tabs -- see issuesFetchMsg.
func (r *RepoScreen) fetchIssuesCmd() tea.Cmd {
	owner, repo, perPage, client := r.owner, r.repo, r.cfg.Behavior.PageSize, r.client
	return func() tea.Msg {
		issues, prs, err := client.Issues(owner, repo, perPage)
		return issuesFetchMsg{issues: issues, prs: prs, err: err}
	}
}

func (r *RepoScreen) Init() tea.Cmd {
	return tea.Batch(
		r.spinner.Tick,
		r.fetchCmd(),
		r.readme.start(),
		r.releases.start(),
		r.issues.tick(),
		r.prs.tick(),
		r.fetchIssuesCmd(),
	)
}

func (r *RepoScreen) Update(msg tea.Msg) (Screen, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		r.width, r.height = msg.Width, msg.Height
		r.resizeTabs()
		// no early return: fall through to the shared dispatch below so
		// every sub-tab's viewport also sees the resize even when inactive.

	case repoMetaMsg:
		r.loading = false
		r.err = msg.err
		if msg.err != nil {
			return r, nil
		}
		r.meta = msg.meta
		// The code tab can't fetch its tree until it knows the default
		// branch; it starts here rather than in Init.
		return r, r.code.start(r.meta.DefaultBranch)

	case issuesFetchMsg:
		// Split the one shared fetch into each tab's own message so
		// issuesTab stays symmetric with the other sub-models (an
		// Update-driven typed msg, own loading state) even though it
		// doesn't own the fetch itself.
		return r, tea.Batch(
			r.issues.Update(issuesResultMsg{items: msg.issues, err: msg.err}),
			r.prs.Update(issuesResultMsg{items: msg.prs, err: msg.err}),
		)

	case spinner.TickMsg:
		if r.loading {
			var cmd tea.Cmd
			r.spinner, cmd = r.spinner.Update(msg)
			// fall through: every sub-tab's own spinner (separate IDs)
			// also needs a look at every tick while it's loading.
			return r, tea.Batch(cmd, r.readme.Update(msg), r.code.Update(msg),
				r.releases.Update(msg), r.issues.Update(msg), r.prs.Update(msg))
		}

	case tea.KeyMsg:
		switch {
		case matchesKey(msg, r.cfg.Keys.Quit):
			return r, tea.Quit
		case matchesKey(msg, r.cfg.Keys.Back):
			// Releases/issues/prs intercept back at their own detail
			// level (pop detail -> list) rather than letting it bubble
			// here -- see releasesTab/issuesTab's struct comments. Only
			// forward when the active tab is actually showing detail;
			// otherwise (list level, or readme/code which have no
			// detail/list split) back pops the whole screen as usual.
			if tab := r.activeDetailTab(); tab != nil {
				return r, tab.Update(msg)
			}
			return r, popScreen()
		case matchesKey(msg, r.cfg.Keys.TabNext):
			r.active = (r.active + 1) % len(repoTabs)
			return r, nil
		case matchesKey(msg, r.cfg.Keys.TabPrev):
			r.active = (r.active - 1 + len(repoTabs)) % len(repoTabs)
			return r, nil
		case matchesKey(msg, r.cfg.Keys.Refresh):
			return r, r.refreshActive()
		}

		// Not a chrome key -- only the active tab sees it, so an
		// inactive tab's keymap can't steal keystrokes meant for
		// whichever tab is on screen.
		switch repoTabs[r.active] {
		case "readme":
			return r, r.readme.Update(msg)
		case "code":
			return r, r.code.Update(msg)
		case "releases":
			return r, r.releases.Update(msg)
		case "issues":
			return r, r.issues.Update(msg)
		case "prs":
			return r, r.prs.Update(msg)
		}
	}

	// Fetch results, resizes, and other non-key, non-chrome messages
	// always reach every sub-tab (regardless of which is active) so a
	// background load for an inactive tab still completes -- per-tab
	// state persists across switches (BUILD.md's Async pattern).
	return r, tea.Batch(r.readme.Update(msg), r.code.Update(msg),
		r.releases.Update(msg), r.issues.Update(msg), r.prs.Update(msg))
}

// detailTab is the subset of a sub-tab's interface RepoScreen needs to
// route the back key at the detail level, common to releasesTab and
// issuesTab (readme/code have no list/detail split, so they aren't part
// of this).
type detailTab interface {
	atDetail() bool
	Update(msg tea.Msg) tea.Cmd
}

// activeDetailTab returns the active tab as a detailTab if it's currently
// showing a detail view, or nil otherwise (list level, or a tab with no
// detail/list split at all).
func (r *RepoScreen) activeDetailTab() detailTab {
	var t detailTab
	switch repoTabs[r.active] {
	case "releases":
		t = r.releases
	case "issues":
		t = r.issues
	case "prs":
		t = r.prs
	default:
		return nil
	}
	if t.atDetail() {
		return t
	}
	return nil
}

// refreshActive busts the caches relevant to the active tab and refetches
// (BUILD.md's M3 refresh spec: "repo meta for header always; plus tree
// for code, readme for readme, releases for releases, issues for issues
// and prs").
func (r *RepoScreen) refreshActive() tea.Cmd {
	r.client.Refresh(fmt.Sprintf("repos/%s/%s", r.owner, r.repo))
	r.loading = true
	r.err = nil
	cmds := []tea.Cmd{r.spinner.Tick, r.fetchCmd()}

	switch repoTabs[r.active] {
	case "readme":
		r.client.RefreshReadme(r.owner, r.repo)
		cmds = append(cmds, r.readme.refresh())
	case "code":
		r.client.RefreshTree(r.owner, r.repo, r.meta.DefaultBranch)
		cmds = append(cmds, r.code.refresh())
	case "releases":
		r.client.RefreshReleases(r.owner, r.repo, r.cfg.Behavior.PageSize)
		cmds = append(cmds, r.releases.refresh())
	case "issues", "prs":
		r.client.RefreshIssues(r.owner, r.repo, r.cfg.Behavior.PageSize)
		r.issues.loading = true
		r.prs.loading = true
		cmds = append(cmds, r.fetchIssuesCmd())
	}
	return tea.Batch(cmds...)
}

func (r *RepoScreen) resizeTabs() {
	// Header takes 5 lines (top border + 3 body lines + bottom border),
	// plus a blank line and the tab bar line before the tab body.
	bodyHeight := r.height - 7
	if bodyHeight < 1 {
		bodyHeight = 1
	}
	r.readme.resize(r.width, bodyHeight)
	r.code.resize(r.width, bodyHeight)
	r.releases.resize(r.width, bodyHeight)
	r.issues.resize(r.width, bodyHeight)
	r.prs.resize(r.width, bodyHeight)
}

func (r *RepoScreen) View(width, height int) string {
	var b strings.Builder

	if r.loading {
		b.WriteString(r.spinner.View())
		b.WriteString(" fetching " + r.owner + "/" + r.repo + "...")
		return b.String()
	}

	if r.err != nil {
		b.WriteString(r.theme.ErrorText.Render("error: " + r.err.Error()))
		b.WriteString("\n")
		b.WriteString(r.theme.MutedText.Render("press " + r.cfg.Keys.Refresh + " to retry"))
		return b.String()
	}

	b.WriteString(style.Frame(r.theme, r.meta.FullName, r.headerLines(), width))
	b.WriteString("\n\n")
	b.WriteString(style.TabBar(r.theme, repoTabs, r.active))
	b.WriteString("\n\n")

	switch repoTabs[r.active] {
	case "readme":
		b.WriteString(r.readme.View())
	case "code":
		b.WriteString(r.code.View())
	case "releases":
		b.WriteString(r.releases.View())
	case "issues":
		b.WriteString(r.issues.View())
	case "prs":
		b.WriteString(r.prs.View())
	}

	return b.String()
}

func (r *RepoScreen) headerLines() []string {
	desc := r.meta.Description
	if desc == "" {
		desc = "(no description)"
	}

	stats := fmt.Sprintf("★ %d   branch: %s   issues: %d", r.meta.StargazersCount, r.meta.DefaultBranch, r.meta.OpenIssuesCount)
	if lic := r.meta.LicenseSPDX(); lic != "" {
		stats += "   license: " + lic
	}
	if parent := r.meta.ParentFullName(); parent != "" {
		stats += "   fork of " + parent
	}

	lines := []string{
		r.theme.Body.Render(desc),
		r.theme.DimText.Render(stats),
	}

	if len(r.meta.Topics) > 0 {
		lines = append(lines, r.theme.MutedText.Render("topics: "+strings.Join(r.meta.Topics, ", ")))
	}

	return lines
}

// Footer builds this screen's contextual keybind hints from the live
// keymap, prefixed with hints specific to whichever tab is active.
// "tab" (pane focus) is bubbletea's literal Tab key, not a
// config-remappable binding -- there's no [keys] entry for it (BUILD.md's
// [keys] table has no pane-focus slot), so it's hardcoded here the same
// way Ctrl+C-to-quit is hardcoded in app.go.
func (r *RepoScreen) Footer() []style.KeyHint {
	var hints []style.KeyHint

	switch repoTabs[r.active] {
	case "readme":
		hints = append(hints, style.KeyHint{Keys: r.cfg.Keys.Down + "/" + r.cfg.Keys.Up, Label: "scroll"})
	case "code":
		hints = append(hints,
			style.KeyHint{Keys: r.cfg.Keys.Down + "/" + r.cfg.Keys.Up, Label: "move"},
			style.KeyHint{Keys: r.cfg.Keys.Open, Label: "open"},
			style.KeyHint{Keys: r.cfg.Keys.Edit, Label: "edit"},
			style.KeyHint{Keys: "tab", Label: "focus"},
		)
	case "releases":
		hints = append(hints, r.releases.footerHints(r.cfg)...)
	case "issues":
		hints = append(hints, r.issues.footerHints(r.cfg)...)
	case "prs":
		hints = append(hints, r.prs.footerHints(r.cfg)...)
	}

	hints = append(hints,
		style.KeyHint{Keys: r.cfg.Keys.TabPrev + "/" + r.cfg.Keys.TabNext, Label: "tab"},
		style.KeyHint{Keys: r.cfg.Keys.Refresh, Label: "refresh"},
		style.KeyHint{Keys: r.cfg.Keys.Back, Label: "back"},
		style.KeyHint{Keys: r.cfg.Keys.Quit, Label: "quit"},
	)
	return hints
}

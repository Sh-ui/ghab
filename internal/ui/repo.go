package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/Sh-ui/ghab/internal/config"
	"github.com/Sh-ui/ghab/internal/gh"
	"github.com/Sh-ui/ghab/internal/ui/style"
)

// repoTabs are the tab bar entries, in order. l/h cycle through them.
var repoTabs = []string{"readme", "code", "releases", "issues", "prs"}

// placeholderTabs are the tabs still driven by an inert viewport --
// they gain real fetchers in M3. readme and code are handled by their own
// dedicated sub-models (readmeTab, codeTab) as of M2.
var placeholderTabs = []string{"releases", "issues", "prs"}

// tabPlaceholder is the placeholder body shown in each placeholder tab's
// viewport until its real content lands in M3.
var tabPlaceholder = map[string]string{
	"releases": "release list + assets arrive in M3.",
	"issues":   "issue list + detail view arrive in M3.",
	"prs":      "pull request list + detail view arrive in M3.",
}

// repoMetaMsg carries the async result of fetching repos/{owner}/{repo}.
type repoMetaMsg struct {
	meta gh.RepoMeta
	err  error
}

// RepoScreen shows a repo's framed header (owner/name, description,
// stars, branch, topics) plus the readme/code/releases/issues/prs tab
// bar. Tab bodies are empty placeholder viewports in M1.
type RepoScreen struct {
	cfg    config.Config
	theme  style.Theme
	client *gh.Client

	owner, repo string

	loading bool
	err     error
	meta    gh.RepoMeta
	spinner spinner.Model

	active        int
	viewports     map[string]viewport.Model // placeholderTabs only (releases/issues/prs -- M3)
	readme        *readmeTab
	code          *codeTab
	width, height int
}

// NewRepoScreen builds a Repo screen for owner/repo. Fetching starts in
// Init. readmeStylePath is the glamour stylesheet resolved once at app
// startup (main.go, via config.ResolveReadmeStyle) -- "" means fall back
// to glamour's built-in "dark" standard style.
func NewRepoScreen(cfg config.Config, theme style.Theme, client *gh.Client, readmeStylePath, owner, repo string) *RepoScreen {
	sp := spinner.New(spinner.WithSpinner(spinner.Dot), spinner.WithStyle(theme.Spinner))
	vps := make(map[string]viewport.Model, len(placeholderTabs))
	for _, t := range placeholderTabs {
		vp := viewport.New(0, 0)
		vp.SetContent(theme.MutedText.Render(tabPlaceholder[t]))
		vps[t] = vp
	}
	return &RepoScreen{
		cfg:       cfg,
		theme:     theme,
		client:    client,
		owner:     owner,
		repo:      repo,
		loading:   true,
		spinner:   sp,
		viewports: vps,
		readme:    newReadmeTab(theme, client, readmeStyleOption{stylePath: readmeStylePath}, owner, repo),
		code:      newCodeTab(cfg, theme, client, owner, repo),
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

func (r *RepoScreen) Init() tea.Cmd {
	return tea.Batch(r.spinner.Tick, r.fetchCmd(), r.readme.start())
}

func (r *RepoScreen) Update(msg tea.Msg) (Screen, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		r.width, r.height = msg.Width, msg.Height
		r.resizeViewports()
		// no early return: fall through to the shared dispatch below so
		// both sub-tabs' viewports also see the resize even when inactive.

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

	case spinner.TickMsg:
		if r.loading {
			var cmd tea.Cmd
			r.spinner, cmd = r.spinner.Update(msg)
			// fall through: the readme/code tabs' own spinners (separate
			// IDs) also need a look at every tick while they're loading.
			return r, tea.Batch(cmd, r.readme.Update(msg), r.code.Update(msg))
		}

	case tea.KeyMsg:
		switch {
		case matchesKey(msg, r.cfg.Keys.Quit):
			return r, tea.Quit
		case matchesKey(msg, r.cfg.Keys.Back):
			return r, popScreen()
		case matchesKey(msg, r.cfg.Keys.TabNext):
			r.active = (r.active + 1) % len(repoTabs)
			return r, nil
		case matchesKey(msg, r.cfg.Keys.TabPrev):
			r.active = (r.active - 1 + len(repoTabs)) % len(repoTabs)
			return r, nil
		case matchesKey(msg, r.cfg.Keys.Refresh):
			r.client.Refresh(fmt.Sprintf("repos/%s/%s", r.owner, r.repo))
			r.loading = true
			r.err = nil
			return r, tea.Batch(r.spinner.Tick, r.fetchCmd())
		}

		// Not a chrome key -- only the active tab sees it, so an
		// inactive tab's keymap can't steal keystrokes meant for
		// whichever tab is on screen.
		switch repoTabs[r.active] {
		case "readme":
			return r, r.readme.Update(msg)
		case "code":
			return r, r.code.Update(msg)
		default:
			vp := r.viewports[repoTabs[r.active]]
			var cmd tea.Cmd
			vp, cmd = vp.Update(msg)
			r.viewports[repoTabs[r.active]] = vp
			return r, cmd
		}
	}

	// Fetch results, resizes, and other non-key, non-chrome messages
	// always reach both sub-tabs (regardless of which is active) so a
	// background load for the inactive tab still completes -- per-tab
	// state persists across switches (BUILD.md's Async pattern).
	return r, tea.Batch(r.readme.Update(msg), r.code.Update(msg))
}

func (r *RepoScreen) resizeViewports() {
	// Header takes 5 lines (top border + 3 body lines + bottom border),
	// plus a blank line and the tab bar line before the viewport.
	vpHeight := r.height - 7
	if vpHeight < 1 {
		vpHeight = 1
	}
	for t, vp := range r.viewports {
		vp.Width = r.width
		vp.Height = vpHeight
		r.viewports[t] = vp
	}
	r.readme.resize(r.width, vpHeight)
	r.code.resize(r.width, vpHeight)
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
	default:
		b.WriteString(r.viewports[repoTabs[r.active]].View())
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
// keymap, prefixed with hints specific to whichever tab is active
// (BUILD.md: "code tab shows: j/k move, enter open, e edit, tab focus,
// l/h tab, q quit; readme tab: j/k scroll, l/h tab, ..."). "tab" (pane
// focus) is bubbletea's literal Tab key, not a config-remappable binding
// -- there's no [keys] entry for it (BUILD.md's [keys] table has no
// pane-focus slot), so it's hardcoded here the same way Ctrl+C-to-quit is
// hardcoded in app.go.
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
	}

	hints = append(hints,
		style.KeyHint{Keys: r.cfg.Keys.TabPrev + "/" + r.cfg.Keys.TabNext, Label: "tab"},
		style.KeyHint{Keys: r.cfg.Keys.Refresh, Label: "refresh"},
		style.KeyHint{Keys: r.cfg.Keys.Back, Label: "back"},
		style.KeyHint{Keys: r.cfg.Keys.Quit, Label: "quit"},
	)
	return hints
}

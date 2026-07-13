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

// tabPlaceholder is the placeholder body shown in each tab's viewport
// until its real content lands (M2 for readme/code, M3 for
// releases/issues/prs).
var tabPlaceholder = map[string]string{
	"readme":   "readme rendering arrives in M2 (glamour + ombre stylesheet).",
	"code":     "file tree + preview arrives in M2 (chroma + gutter).",
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
	viewports     map[string]viewport.Model
	width, height int
}

// NewRepoScreen builds a Repo screen for owner/repo. Fetching starts in
// Init.
func NewRepoScreen(cfg config.Config, theme style.Theme, client *gh.Client, owner, repo string) *RepoScreen {
	sp := spinner.New(spinner.WithSpinner(spinner.Dot), spinner.WithStyle(theme.Spinner))
	vps := make(map[string]viewport.Model, len(repoTabs))
	for _, t := range repoTabs {
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
	return tea.Batch(r.spinner.Tick, r.fetchCmd())
}

func (r *RepoScreen) Update(msg tea.Msg) (Screen, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		r.width, r.height = msg.Width, msg.Height
		r.resizeViewports()
		// no return: fall through so the active viewport also sees the message

	case repoMetaMsg:
		r.loading = false
		r.err = msg.err
		if msg.err == nil {
			r.meta = msg.meta
		}
		return r, nil

	case spinner.TickMsg:
		if !r.loading {
			return r, nil
		}
		var cmd tea.Cmd
		r.spinner, cmd = r.spinner.Update(msg)
		return r, cmd

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
	}

	vp := r.viewports[repoTabs[r.active]]
	var cmd tea.Cmd
	vp, cmd = vp.Update(msg)
	r.viewports[repoTabs[r.active]] = vp
	return r, cmd
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
	b.WriteString(r.viewports[repoTabs[r.active]].View())

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

func (r *RepoScreen) Footer() []style.KeyHint {
	return []style.KeyHint{
		{Keys: r.cfg.Keys.TabPrev + "/" + r.cfg.Keys.TabNext, Label: "tab"},
		{Keys: r.cfg.Keys.Refresh, Label: "refresh"},
		{Keys: r.cfg.Keys.Back, Label: "back"},
		{Keys: r.cfg.Keys.Quit, Label: "quit"},
	}
}

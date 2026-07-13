package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/Sh-ui/ghab/internal/config"
	"github.com/Sh-ui/ghab/internal/gh"
	"github.com/Sh-ui/ghab/internal/ui/style"
)

// issueKind distinguishes the two issuesTab instances RepoScreen owns: one
// filtered to real issues, one to PRs (BUILD.md M3: "one shared sub-model
// type instantiated twice, filtered"). Both are fed from the single
// repos/{o}/{r}/issues fetch RepoScreen makes -- see issuesResultMsg.
type issueKind int

const (
	issueKindIssue issueKind = iota
	issueKindPR
)

// issuesResultMsg carries this tab's slice of RepoScreen's shared issues
// fetch (RepoScreen.fetchIssuesCmd splits the one network round trip into
// two of these -- one per tab instance -- rather than each tab fetching
// on its own).
type issuesResultMsg struct {
	items []gh.Issue
	err   error
}

// issueDetailMsg carries the async result of fetching one issue/PR's full
// detail (body + comments).
type issueDetailMsg struct {
	number int
	detail gh.IssueDetail
	err    error
}

// issuesTab owns one issues-or-PRs tab's state: the filtered list (fed by
// RepoScreen, not self-fetched) and a list/detail split (selected != nil
// means detail). Survives tab switches, matching the other tabs' pattern.
//
// Back-key interception: RepoScreen only forwards the back key to this
// tab's Update when atDetail() is true; at list level RepoScreen pops the
// whole screen itself. Same convention as releasesTab -- see its struct
// comment for the rationale.
type issuesTab struct {
	cfg      config.Config
	theme    style.Theme
	client   *gh.Client
	styleOpt readmeStyleOption
	kind     issueKind

	owner, repo string

	spinner spinner.Model
	loading bool
	err     error
	list    []gh.Issue

	cursor    int
	scrollOff int
	rows      int
	width     int

	selected        *gh.Issue
	detailLoading   bool
	detailErr       error
	detail          gh.IssueDetail
	detailVp        viewport.Model
	renderedContent string
	renderedWidth   int
}

func newIssuesTab(cfg config.Config, theme style.Theme, client *gh.Client, styleOpt readmeStyleOption, owner, repo string, kind issueKind) *issuesTab {
	sp := spinner.New(spinner.WithSpinner(spinner.Dot), spinner.WithStyle(theme.Spinner))
	return &issuesTab{
		cfg:           cfg,
		theme:         theme,
		client:        client,
		styleOpt:      styleOpt,
		kind:          kind,
		owner:         owner,
		repo:          repo,
		spinner:       sp,
		loading:       true,
		detailVp:      viewport.New(0, 0),
		renderedWidth: -1,
	}
}

// tick starts this tab's own spinner chain. issuesTab has no owned
// fetch (RepoScreen fetches once and feeds both instances -- see
// issuesResultMsg), so unlike readmeTab/codeTab/releasesTab's start(),
// something else has to kick off spinner.Tick; RepoScreen.Init calls this
// directly for both instances.
func (t *issuesTab) tick() tea.Cmd {
	return t.spinner.Tick
}

// atDetail reports whether the tab is showing an issue/PR's detail view
// rather than the list.
func (t *issuesTab) atDetail() bool {
	return t.selected != nil
}

func (t *issuesTab) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case issuesResultMsg:
		t.loading = false
		t.err = msg.err
		if msg.err == nil {
			t.list = msg.items
			t.clampCursor()
		}
		return nil

	case issueDetailMsg:
		if t.selected == nil || t.selected.Number != msg.number {
			return nil // stale response for a since-deselected issue
		}
		t.detailLoading = false
		t.detailErr = msg.err
		if msg.err == nil {
			t.detail = msg.detail
			t.renderedWidth = -1
		}
		t.applyDetailContent()
		return nil

	case spinner.TickMsg:
		if !t.loading && !t.detailLoading {
			return nil
		}
		var cmd tea.Cmd
		t.spinner, cmd = t.spinner.Update(msg)
		return cmd

	case tea.KeyMsg:
		return t.handleKey(msg)
	}
	return nil
}

func (t *issuesTab) handleKey(msg tea.KeyMsg) tea.Cmd {
	if t.selected == nil {
		return t.handleListKey(msg)
	}
	return t.handleDetailKey(msg)
}

func (t *issuesTab) handleListKey(msg tea.KeyMsg) tea.Cmd {
	switch {
	case matchesKey(msg, t.cfg.Keys.Down):
		t.moveCursor(1)
	case matchesKey(msg, t.cfg.Keys.Up):
		t.moveCursor(-1)
	case matchesKey(msg, t.cfg.Keys.Open):
		return t.enterDetail()
	}
	return nil
}

// handleDetailKey: back pops detail -> list (never bubbles to RepoScreen
// from here -- see the struct comment). j/k scroll the body+comments
// viewport via its default keymap.
func (t *issuesTab) handleDetailKey(msg tea.KeyMsg) tea.Cmd {
	if matchesKey(msg, t.cfg.Keys.Back) {
		t.exitDetail()
		return nil
	}
	var cmd tea.Cmd
	t.detailVp, cmd = t.detailVp.Update(msg)
	return cmd
}

func (t *issuesTab) moveCursor(delta int) {
	if len(t.list) == 0 {
		return
	}
	t.cursor += delta
	t.clampCursor()
}

func (t *issuesTab) clampCursor() {
	if t.cursor < 0 {
		t.cursor = 0
	}
	if t.cursor >= len(t.list) {
		t.cursor = len(t.list) - 1
	}
	if t.cursor < 0 {
		t.cursor = 0
	}
	if t.rows <= 0 {
		return
	}
	if t.cursor < t.scrollOff {
		t.scrollOff = t.cursor
	}
	if t.cursor >= t.scrollOff+t.rows {
		t.scrollOff = t.cursor - t.rows + 1
	}
	if t.scrollOff < 0 {
		t.scrollOff = 0
	}
}

func (t *issuesTab) enterDetail() tea.Cmd {
	if len(t.list) == 0 || t.cursor >= len(t.list) {
		return nil
	}
	issue := t.list[t.cursor]
	t.selected = &issue
	t.detailLoading = true
	t.detailErr = nil
	t.detail = gh.IssueDetail{}
	t.renderedWidth = -1
	t.applyDetailContent()

	owner, repo, number, client := t.owner, t.repo, issue.Number, t.client
	return tea.Batch(t.spinner.Tick, func() tea.Msg {
		d, err := client.IssueDetail(owner, repo, number)
		return issueDetailMsg{number: number, detail: d, err: err}
	})
}

func (t *issuesTab) exitDetail() {
	t.selected = nil
}

// resize sets the tab's pane geometry from the shared tab-body area
// (width x rows -- the same area every repo tab renders into).
func (t *issuesTab) resize(width, rows int) {
	t.width = width
	t.rows = rows
	if t.rows < 1 {
		t.rows = 1
	}
	t.clampCursor()

	t.detailVp.Width = width
	t.detailVp.Height = rows
	if t.selected != nil {
		t.applyDetailContent()
	}
}

// applyDetailContent rebuilds the detail viewport's content: a loading
// line, an error line, or the title/meta header + glamour body + glamour
// comments, cached by width like readmeTab.renderContent (glamour bakes
// word wrap in at construction, so a width change invalidates the cache).
func (t *issuesTab) applyDetailContent() {
	if t.selected == nil || t.detailVp.Width <= 0 {
		return
	}
	if t.detailLoading {
		t.detailVp.SetContent(t.spinner.View() + " loading...")
		return
	}
	if t.detailErr != nil {
		t.detailVp.SetContent(t.theme.ErrorText.Render("error: " + t.detailErr.Error()))
		return
	}
	if t.renderedWidth == t.detailVp.Width && t.renderedContent != "" {
		t.detailVp.SetContent(t.renderedContent)
		return
	}

	issue := t.detail.Issue
	stateStyle := t.theme.OpenText
	if issue.State != "open" {
		stateStyle = t.theme.ClosedText
	}

	var b strings.Builder
	b.WriteString(t.theme.Body.Render(fmt.Sprintf("#%d %s", issue.Number, issue.Title)))
	b.WriteString("\n")
	b.WriteString(stateStyle.Render("● " + issue.State))
	b.WriteString(t.theme.MutedText.Render("  " + issue.Author() + "  " + relativeAge(issue.CreatedAt)))
	b.WriteString("\n\n")
	b.WriteString(t.renderMarkdown(issue.Body))
	b.WriteString("\n")

	for _, c := range t.detail.Comments {
		b.WriteString("\n")
		b.WriteString(t.theme.MutedText.Render(strings.Repeat("-", 40)))
		b.WriteString("\n")
		b.WriteString(t.theme.MutedText.Render(c.Author() + "  " + relativeAge(c.CreatedAt)))
		b.WriteString("\n")
		b.WriteString(t.renderMarkdown(c.Body))
		b.WriteString("\n")
	}

	t.renderedContent = b.String()
	t.renderedWidth = t.detailVp.Width
	t.detailVp.SetContent(t.renderedContent)
}

func (t *issuesTab) renderMarkdown(body string) string {
	if strings.TrimSpace(body) == "" {
		return t.theme.MutedText.Render("(no description)")
	}
	out, err := t.styleOpt.render(body, t.detailVp.Width)
	if err != nil {
		return t.theme.ErrorText.Render("error rendering body: " + err.Error())
	}
	return out
}

func (t *issuesTab) View() string {
	if t.loading {
		return t.spinner.View() + " loading " + t.kindLabel() + "..."
	}
	if t.err != nil {
		return t.theme.ErrorText.Render("error: " + t.err.Error())
	}
	if t.selected != nil {
		return t.detailVp.View()
	}
	if len(t.list) == 0 {
		return t.theme.MutedText.Render("no " + t.kindLabel())
	}
	return strings.Join(t.listBodyLines(), "\n")
}

func (t *issuesTab) kindLabel() string {
	if t.kind == issueKindPR {
		return "pull requests"
	}
	return "issues"
}

func (t *issuesTab) listBodyLines() []string {
	rows := make([]string, 0, t.rows)
	for i := 0; i < t.rows; i++ {
		idx := t.scrollOff + i
		if idx >= len(t.list) {
			break
		}
		rows = append(rows, t.renderListRow(t.list[idx], idx == t.cursor))
	}
	return rows
}

func (t *issuesTab) renderListRow(issue gh.Issue, selected bool) string {
	comments := ""
	if issue.Comments > 0 {
		comments = fmt.Sprintf("  %d comments", issue.Comments)
	}
	plain := fmt.Sprintf("● #%d  %s  %s  %s%s", issue.Number, issue.Title, issue.Author(), relativeAge(issue.CreatedAt), comments)

	width := t.width
	if width < 1 {
		width = lipgloss.Width(plain)
	}

	if selected {
		return lipgloss.NewStyle().Width(width).MaxWidth(width).
			Background(t.theme.SelectionBg).Foreground(t.theme.Fg).Render(plain)
	}

	bulletStyle := t.theme.OpenText
	if issue.State != "open" {
		bulletStyle = t.theme.ClosedText
	}
	styled := bulletStyle.Render("●") + " " +
		t.theme.MutedText.Render(fmt.Sprintf("#%d", issue.Number)) + "  " +
		t.theme.Body.Render(issue.Title) + "  " +
		t.theme.MutedText.Render(issue.Author()+"  "+relativeAge(issue.CreatedAt)+comments)
	return lipgloss.NewStyle().Width(width).MaxWidth(width).Render(styled)
}

// footerHints builds this tab's contextual keybind hints for
// RepoScreen.Footer, per BUILD.md's M3 footer spec.
func (t *issuesTab) footerHints(cfg config.Config) []style.KeyHint {
	if t.atDetail() {
		return []style.KeyHint{{Keys: cfg.Keys.Down + "/" + cfg.Keys.Up, Label: "scroll"}}
	}
	return []style.KeyHint{
		{Keys: cfg.Keys.Down + "/" + cfg.Keys.Up, Label: "move"},
		{Keys: cfg.Keys.Open, Label: "open"},
	}
}

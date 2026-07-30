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

// prView is which of a PR's three sub-views is on screen.
type prView int

const (
	prViewConversation prView = iota
	prViewFiles
	prViewDiff
)

// diffHorizontalStep is how many columns h/l scroll the diff viewport per
// press. No [keys] slot governs the step size, same "no natural config
// slot" precedent as the h/l binding itself (see prDetailModel's struct
// comment) -- a diff line runs well past this screen's width, so a
// single-column step would take many presses to reveal truncated content.
const diffHorizontalStep = 10

// prConvMsg / prFilesMsg / prThreadsMsg carry the async results of
// prDetailModel's three parallel fetches (conversation body+comments,
// changed-files list, line-anchored review comments). Each carries the
// owner/repo/number the fetch was made for so a stale response (a
// since-closed detail, re-opened on a different PR before the old fetch
// lands) is dropped rather than misapplied -- same guard style as
// codeTab's fileMsg/dirMsg. Owner/repo ride along, not just number: the
// cross-repo my-PRs flow can open repoA#5 then repoB#5 before repoA's
// fetch lands, and a number-only guard would misapply repoA's response to
// repoB's screen on that same-number collision.
type prConvMsg struct {
	owner, repo string
	number      int
	detail      gh.IssueDetail
	err         error
}

type prFilesMsg struct {
	owner, repo string
	number      int
	files       []gh.PullFile
	err         error
}

type prThreadsMsg struct {
	owner, repo string
	number      int
	threads     []gh.FileThread
	err         error
}

// prDetailModel is a PR's detail view -- conversation (body + comments,
// same shape as issuesTab's existing issue detail), a files-changed list
// with +/- counts, and a per-file diff view with line-anchored review
// threads inlined (BUILD.md extension, issue #11's PR-view build).
//
// It is a plain sub-model (no Screen methods) reused in two places: the
// top-level "my PRs" hop wraps one in PRDetailScreen (prdetail_screen.go);
// the per-repo PRs tab (issuesTab, kind == issueKindPR) owns one directly,
// swapping it in for the plain body+comments viewport that issues use,
// per BUILD.md's "new view AND existing per-repo PRs tab" requirement.
//
// Horizontal scroll in the diff view rides bubbles viewport's own
// built-in keymap (h/l and left/right, alongside j/k for vertical) -- but
// that keymap's ScrollLeft/ScrollRight are no-ops until horizontalStep is
// set (bubbles v1.0.0 disables h-scroll by default), so diffVp gets
// SetHorizontalStep at construction below. There is no ghab [keys] slot
// for the h/l binding itself, matching the "tab" pane-focus precedent
// (codeTab, releasesTab) of leaving mechanics with no natural config slot
// hardcoded. The conversation view never needs this: its content is
// glamour-rendered and word-wrapped to width, so no line ever exceeds the
// viewport and there's nothing an h-scroll would reveal. When the diff
// view is embedded in RepoScreen's PRs tab, RepoScreen must not steal h/l
// for tab-switching while it's open -- see issuesTab.wantsHorizontalKeys.
type prDetailModel struct {
	cfg      config.Config
	theme    style.Theme
	client   *gh.Client
	styleOpt readmeStyleOption

	owner, repo string
	number      int
	perPage     int

	spinner spinner.Model

	convLoading       bool
	convErr           error
	conv              gh.IssueDetail
	convVp            viewport.Model
	renderedConv      string
	renderedConvWidth int

	filesLoading  bool
	filesErr      error
	files         []gh.PullFile
	fileCursor    int
	fileScrollOff int
	fileRows      int

	threadsLoading bool
	threadsErr     error
	threadsByPath  map[string][]gh.LineThread

	view                prView
	diffFile            *gh.PullFile
	diffVp              viewport.Model
	renderedDiffFile    string
	renderedDiffWidth   int
	renderedDiffContent string

	width, rows int
}

func newPRDetailModel(cfg config.Config, theme style.Theme, client *gh.Client, styleOpt readmeStyleOption, owner, repo string, number int) *prDetailModel {
	sp := spinner.New(spinner.WithSpinner(spinner.Dot), spinner.WithStyle(theme.Spinner))
	diffVp := viewport.New(0, 0)
	diffVp.SetHorizontalStep(diffHorizontalStep)
	return &prDetailModel{
		cfg:               cfg,
		theme:             theme,
		client:            client,
		styleOpt:          styleOpt,
		owner:             owner,
		repo:              repo,
		number:            number,
		perPage:           cfg.Behavior.PageSize,
		spinner:           sp,
		convLoading:       true,
		filesLoading:      true,
		threadsLoading:    true,
		convVp:            viewport.New(0, 0),
		diffVp:            diffVp,
		renderedConvWidth: -1,
		renderedDiffWidth: -1,
	}
}

// start kicks off all three fetches in parallel (conversation, files,
// review comments) plus this model's own spinner chain -- called once
// from whichever caller pushes/enters this PR's detail.
func (p *prDetailModel) start() tea.Cmd {
	owner, repo, number, perPage, client := p.owner, p.repo, p.number, p.perPage, p.client
	return tea.Batch(
		p.spinner.Tick,
		func() tea.Msg {
			d, err := client.IssueDetail(owner, repo, number)
			return prConvMsg{owner: owner, repo: repo, number: number, detail: d, err: err}
		},
		func() tea.Msg {
			files, err := client.PullFiles(owner, repo, number, perPage)
			return prFilesMsg{owner: owner, repo: repo, number: number, files: files, err: err}
		},
		func() tea.Msg {
			comments, err := client.PullReviewComments(owner, repo, number)
			var threads []gh.FileThread
			if err == nil {
				threads = gh.GroupReviewThreads(comments)
			}
			return prThreadsMsg{owner: owner, repo: repo, number: number, threads: threads, err: err}
		},
	)
}

// refresh busts all three cache entries and refetches -- the "r" refresh
// binding, routed in from whichever caller owns the chrome key (RepoScreen
// for the embedded case, PRDetailScreen for the standalone one).
func (p *prDetailModel) refresh() tea.Cmd {
	p.client.RefreshIssueDetail(p.owner, p.repo, p.number)
	p.client.RefreshPullFiles(p.owner, p.repo, p.number, p.perPage)
	p.client.RefreshPullReviewComments(p.owner, p.repo, p.number)
	p.convLoading, p.filesLoading, p.threadsLoading = true, true, true
	p.convErr, p.filesErr, p.threadsErr = nil, nil, nil
	// Bust the rendered diff as well as the caches behind it. The render
	// cache keys on (filename, width) and a refresh changes neither, so
	// without this an open diff keeps replaying renderedDiffContent while
	// the file list refetches underneath it -- the file list refreshes and
	// the diff on screen never moves.
	p.invalidateDiffRender()
	p.applyDiffContent()
	return p.start()
}

// invalidateDiffRender drops the cached diff render so the next
// applyDiffContent rebuilds from live data instead of replaying the
// previous one. Cheap: the rebuild is a string walk over an
// already-fetched patch, no network.
func (p *prDetailModel) invalidateDiffRender() {
	p.renderedDiffFile = ""
	p.renderedDiffWidth = -1
	p.renderedDiffContent = ""
}

// rebindDiffFile re-points diffFile at the freshly fetched entry for the
// same path and rebuilds the render, so a refresh with the diff view open
// replaces the patch on screen rather than leaving the pre-refresh one
// there. A file that vanished from the new list (force-push, dropped
// commit) has no diff left to show, so the view steps back up to the
// files list instead of holding a diff for a file the PR no longer
// touches.
func (p *prDetailModel) rebindDiffFile() {
	if p.diffFile == nil {
		return
	}
	name := p.diffFile.Filename
	p.invalidateDiffRender()
	for i := range p.files {
		if p.files[i].Filename == name {
			f := p.files[i]
			p.diffFile = &f
			p.applyDiffContent()
			return
		}
	}
	p.diffFile = nil
	if p.view == prViewDiff {
		p.view = prViewFiles
	}
}

func (p *prDetailModel) atDiff() bool { return p.view == prViewDiff }

func (p *prDetailModel) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case prConvMsg:
		if msg.number != p.number || msg.owner != p.owner || msg.repo != p.repo {
			return nil
		}
		p.convLoading = false
		p.convErr = msg.err
		if msg.err == nil {
			p.conv = msg.detail
			p.renderedConvWidth = -1
			p.applyConvContent()
		}
		return nil

	case prFilesMsg:
		if msg.number != p.number || msg.owner != p.owner || msg.repo != p.repo {
			return nil
		}
		p.filesLoading = false
		p.filesErr = msg.err
		if msg.err == nil {
			p.files = msg.files
			p.clampFileCursor()
			p.rebindDiffFile()
		}
		return nil

	case prThreadsMsg:
		if msg.number != p.number || msg.owner != p.owner || msg.repo != p.repo {
			return nil
		}
		p.threadsLoading = false
		p.threadsErr = msg.err
		if msg.err == nil {
			p.threadsByPath = groupThreadsByPath(msg.threads)
			if p.view == prViewDiff {
				p.invalidateDiffRender() // force a rebuild now that threads landed
				p.applyDiffContent()
			}
		}
		return nil

	case spinner.TickMsg:
		if !p.convLoading && !p.filesLoading && !p.threadsLoading {
			return nil
		}
		var cmd tea.Cmd
		p.spinner, cmd = p.spinner.Update(msg)
		return cmd
	}
	return nil
}

func groupThreadsByPath(threads []gh.FileThread) map[string][]gh.LineThread {
	m := make(map[string][]gh.LineThread, len(threads))
	for _, ft := range threads {
		m[ft.Path] = ft.Lines
	}
	return m
}

// handleKey processes one key while this PR's detail is open. exit is
// true only when back is pressed at the outermost level (conversation or
// files) -- the caller (PRDetailScreen or issuesTab) is responsible for
// leaving detail entirely in that case; back from the diff sub-view just
// steps up to the files list (exit stays false).
func (p *prDetailModel) handleKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	if matchesKey(msg, p.cfg.Keys.Back) {
		if p.view == prViewDiff {
			p.view = prViewFiles
			return nil, false
		}
		return nil, true
	}

	if msg.Type == tea.KeyTab {
		switch p.view {
		case prViewConversation:
			p.view = prViewFiles
		case prViewFiles:
			p.view = prViewConversation
		}
		return nil, false
	}

	switch p.view {
	case prViewConversation:
		var cmd tea.Cmd
		p.convVp, cmd = p.convVp.Update(msg)
		return cmd, false

	case prViewFiles:
		switch {
		case matchesKey(msg, p.cfg.Keys.Down):
			p.moveFileCursor(1)
		case matchesKey(msg, p.cfg.Keys.Up):
			p.moveFileCursor(-1)
		case matchesKey(msg, p.cfg.Keys.Open):
			p.enterDiff()
		}
		return nil, false

	case prViewDiff:
		var cmd tea.Cmd
		p.diffVp, cmd = p.diffVp.Update(msg)
		return cmd, false
	}
	return nil, false
}

func (p *prDetailModel) moveFileCursor(delta int) {
	if len(p.files) == 0 {
		return
	}
	p.fileCursor += delta
	p.clampFileCursor()
}

func (p *prDetailModel) clampFileCursor() {
	if p.fileCursor < 0 {
		p.fileCursor = 0
	}
	if p.fileCursor >= len(p.files) {
		p.fileCursor = len(p.files) - 1
	}
	if p.fileCursor < 0 {
		p.fileCursor = 0
	}
	if p.fileRows <= 0 {
		return
	}
	if p.fileCursor < p.fileScrollOff {
		p.fileScrollOff = p.fileCursor
	}
	if p.fileCursor >= p.fileScrollOff+p.fileRows {
		p.fileScrollOff = p.fileCursor - p.fileRows + 1
	}
	if p.fileScrollOff < 0 {
		p.fileScrollOff = 0
	}
}

// enterDiff opens the diff view for the file under the cursor, resetting
// both scroll axes so a previously-viewed file's scroll position never
// leaks into a new one.
func (p *prDetailModel) enterDiff() {
	if len(p.files) == 0 || p.fileCursor >= len(p.files) {
		return
	}
	f := p.files[p.fileCursor]
	p.diffFile = &f
	p.diffVp.SetXOffset(0)
	p.diffVp.GotoTop()
	p.view = prViewDiff
	p.applyDiffContent()
}

// resize sets every sub-view's viewport geometry from the shared body
// area (width x rows). Files and diff each reserve a 2-line header (a
// stats/title line + a blank separator) above their scrolling body,
// matching RepoScreen's own header+blank convention.
func (p *prDetailModel) resize(width, rows int) {
	p.width = width
	p.rows = rows
	if p.rows < 1 {
		p.rows = 1
	}

	p.convVp.Width = width
	p.convVp.Height = rows
	p.applyConvContent()

	bodyRows := rows - 2
	if bodyRows < 1 {
		bodyRows = 1
	}
	p.fileRows = bodyRows
	p.clampFileCursor()

	p.diffVp.Width = width
	p.diffVp.Height = bodyRows
	p.applyDiffContent()
}

// applyConvContent (re-)renders the conversation viewport's content: a
// title/state/author/age header plus the glamour body and comments, cached
// by width like readmeTab.renderContent (glamour bakes word wrap in at
// construction).
func (p *prDetailModel) applyConvContent() {
	if p.convVp.Width <= 0 {
		return
	}
	if p.convLoading {
		p.convVp.SetContent(p.spinner.View() + " loading...")
		return
	}
	if p.convErr != nil {
		p.convVp.SetContent(p.theme.ErrorText.Render("error: " + p.convErr.Error()))
		return
	}
	if p.renderedConvWidth == p.convVp.Width && p.renderedConv != "" {
		p.convVp.SetContent(p.renderedConv)
		return
	}

	issue := p.conv.Issue
	stateStyle := p.theme.OpenText
	if issue.State != "open" {
		stateStyle = p.theme.ClosedText
	}

	var b strings.Builder
	b.WriteString(p.theme.Body.Render(fmt.Sprintf("#%d %s", issue.Number, issue.Title)))
	b.WriteString("\n")
	b.WriteString(stateStyle.Render("● " + issue.State))
	b.WriteString(p.theme.MutedText.Render("  " + issue.Author() + "  " + relativeAge(issue.CreatedAt)))
	b.WriteString("\n\n")
	b.WriteString(p.renderMarkdown(issue.Body))
	b.WriteString("\n")

	for _, c := range p.conv.Comments {
		b.WriteString("\n")
		b.WriteString(p.theme.MutedText.Render(strings.Repeat("-", 40)))
		b.WriteString("\n")
		b.WriteString(p.theme.MutedText.Render(c.Author() + "  " + relativeAge(c.CreatedAt)))
		b.WriteString("\n")
		b.WriteString(p.renderMarkdown(c.Body))
		b.WriteString("\n")
	}

	p.renderedConv = b.String()
	p.renderedConvWidth = p.convVp.Width
	p.convVp.SetContent(p.renderedConv)
}

func (p *prDetailModel) renderMarkdown(body string) string {
	if strings.TrimSpace(body) == "" {
		return p.theme.MutedText.Render("(no description)")
	}
	out, err := p.styleOpt.render(body, p.convVp.Width)
	if err != nil {
		return p.theme.ErrorText.Render("error rendering body: " + err.Error())
	}
	return out
}

// applyDiffContent (re-)renders the diff viewport's content for the
// currently-selected file, cached by (file, width) -- the diff lines
// themselves don't need re-wrapping on a width change (bubbles viewport
// truncates lines rather than wrapping them, and h-scrolls via
// SetHorizontalStep -- see diffHorizontalStep), but the glamour-rendered
// review-comment bodies interleaved with them do.
func (p *prDetailModel) applyDiffContent() {
	if p.diffVp.Width <= 0 {
		return
	}
	if p.filesLoading {
		// A refetch is in flight, so whatever patch this model still holds
		// is pre-refresh content by definition -- show the loading state
		// rather than re-rendering it.
		p.diffVp.SetContent(p.spinner.View() + " loading diff...")
		return
	}
	if p.diffFile == nil {
		return
	}
	if p.renderedDiffFile == p.diffFile.Filename && p.renderedDiffWidth == p.diffVp.Width {
		p.diffVp.SetContent(p.renderedDiffContent)
		return
	}
	content := p.buildDiffContent()
	p.diffVp.SetContent(content)
	p.renderedDiffFile = p.diffFile.Filename
	p.renderedDiffWidth = p.diffVp.Width
	p.renderedDiffContent = content
}

// buildDiffContent renders one file's diff: the placeholder matching
// GitHub's reason for omitting the patch (see gh.PullFile.Omission --
// pure rename, binary, or a text diff withheld for size), or the
// parsed+context-trimmed patch with add/remove/hunk
// coloring, review threads inlined right after the diff line they anchor
// to, and any thread that never matched a shown line (an outdated comment,
// or one trimmed away by diff_context_lines) appended in a trailer rather
// than silently dropped.
func (p *prDetailModel) buildDiffContent() string {
	f := p.diffFile
	switch f.Omission() {
	case gh.PatchOmittedRename:
		return p.theme.MutedText.Render(fmt.Sprintf("renamed: %s -> %s (no content change)", f.PreviousFilename, f.Filename))
	case gh.PatchOmittedBinary:
		return p.theme.MutedText.Render("binary file -- no diff shown")
	case gh.PatchOmittedTooLarge:
		return p.theme.MutedText.Render(fmt.Sprintf(
			"diff not loaded -- GitHub omitted the patch for this file (too large): +%d -%d",
			f.Additions, f.Deletions))
	}

	lines := gh.TrimContext(gh.ParsePatch(f.Patch), p.cfg.Behavior.DiffContextLines)
	threads := p.threadsByPath[f.Filename]
	// LineThread carries a slice field (Thread) so it can't be a map key
	// directly -- track "already placed" by index into threads instead.
	used := make([]bool, len(threads))

	var b strings.Builder
	for i, l := range lines {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(p.renderDiffLine(l))
		for ti, th := range threads {
			if diffLineMatchesThread(l, th) {
				used[ti] = true
				b.WriteString("\n")
				b.WriteString(p.renderThread(th))
			}
		}
	}

	var leftover []gh.LineThread
	for ti, th := range threads {
		if !used[ti] {
			leftover = append(leftover, th)
		}
	}
	if len(leftover) > 0 {
		b.WriteString("\n\n")
		b.WriteString(p.theme.MutedText.Render(strings.Repeat("-", 40)))
		b.WriteString("\n")
		b.WriteString(p.theme.MutedText.Render("other review comments on this file (outside the shown diff):"))
		for _, th := range leftover {
			b.WriteString("\n")
			b.WriteString(p.renderThread(th))
		}
	}
	return b.String()
}

// diffLineMatchesThread reports whether l is the diff line th anchors to:
// a LEFT-side thread matches by OldLine (the removed/original side), a
// RIGHT-side (or side-less, treated as RIGHT) thread matches by NewLine.
func diffLineMatchesThread(l gh.DiffLine, th gh.LineThread) bool {
	if th.Side == "LEFT" {
		return l.OldLine > 0 && l.OldLine == th.Line
	}
	return l.NewLine > 0 && l.NewLine == th.Line
}

func (p *prDetailModel) renderDiffLine(l gh.DiffLine) string {
	switch l.Kind {
	case gh.DiffAdd:
		return p.theme.OpenText.Render(l.Text)
	case gh.DiffRemove:
		return p.theme.ClosedText.Render(l.Text)
	case gh.DiffHunkHeader:
		return p.theme.AccentText.Render(l.Text)
	default:
		return p.theme.Body.Render(l.Text)
	}
}

// renderThread renders one line-anchored comment thread, each comment's
// body through glamour, indented two spaces to read as nested under the
// diff line above it.
func (p *prDetailModel) renderThread(th gh.LineThread) string {
	var b strings.Builder
	for i, cm := range th.Thread {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(p.theme.AccentAltText.Render("  " + cm.Author()))
		b.WriteString(p.theme.MutedText.Render("  " + relativeAge(cm.CreatedAt)))
		body := p.renderMarkdown(cm.Body)
		for _, ln := range strings.Split(strings.TrimRight(body, "\n"), "\n") {
			b.WriteString("\n  ")
			b.WriteString(ln)
		}
	}
	return b.String()
}

func (p *prDetailModel) View() string {
	switch p.view {
	case prViewFiles:
		return p.filesView()
	case prViewDiff:
		return p.diffView()
	default:
		if p.convLoading {
			return p.spinner.View() + " loading..."
		}
		return p.convVp.View()
	}
}

func (p *prDetailModel) filesView() string {
	if p.filesLoading {
		return p.spinner.View() + " loading files..."
	}
	if p.filesErr != nil {
		return p.theme.ErrorText.Render("error: " + p.filesErr.Error())
	}
	if len(p.files) == 0 {
		return p.theme.MutedText.Render("no files changed")
	}

	var totalAdd, totalDel int
	for _, f := range p.files {
		totalAdd += f.Additions
		totalDel += f.Deletions
	}
	header := p.theme.Body.Render(fmt.Sprintf("%d files changed", len(p.files))) + "  " +
		p.theme.OpenText.Render(fmt.Sprintf("+%d", totalAdd)) + " " +
		p.theme.ClosedText.Render(fmt.Sprintf("-%d", totalDel))

	var b strings.Builder
	b.WriteString(header)
	b.WriteString("\n\n")
	b.WriteString(strings.Join(p.fileRowLines(), "\n"))
	return b.String()
}

func (p *prDetailModel) fileRowLines() []string {
	rows := make([]string, 0, p.fileRows)
	for i := 0; i < p.fileRows; i++ {
		idx := p.fileScrollOff + i
		if idx >= len(p.files) {
			break
		}
		rows = append(rows, p.renderFileRow(p.files[idx], idx == p.fileCursor))
	}
	return rows
}

func (p *prDetailModel) renderFileRow(f gh.PullFile, selected bool) string {
	marker, markerStyle := p.fileStatusMarker(f.Status)
	name := f.Filename
	if f.IsRenamed() && f.PreviousFilename != "" {
		name = f.PreviousFilename + " -> " + f.Filename
	}
	counts := fmt.Sprintf("+%d -%d", f.Additions, f.Deletions)
	switch f.Omission() {
	case gh.PatchOmittedRename:
		counts = "(renamed, no changes)"
	case gh.PatchOmittedBinary:
		counts = "(binary)"
	case gh.PatchOmittedTooLarge:
		counts += "  (diff not loaded)"
	}
	plain := fmt.Sprintf("%s %s  %s", marker, name, counts)

	width := p.width
	if width < 1 {
		width = lipgloss.Width(plain)
	}

	if selected {
		return lipgloss.NewStyle().Width(width).MaxWidth(width).
			Background(p.theme.SelectionBg).Foreground(p.theme.Fg).Render(plain)
	}

	styled := markerStyle.Render(marker) + " " + p.theme.Body.Render(name) + "  " +
		p.theme.MutedText.Render(counts)
	return lipgloss.NewStyle().Width(width).MaxWidth(width).Render(styled)
}

// fileStatusMarker maps a PullFile.Status to a one-letter marker + the
// theme color it renders in, reusing the ombre open/closed/accent slots
// rather than inventing new ones (BUILD.md: "add/remove line coloring
// consistent with ghab's ombre theme").
func (p *prDetailModel) fileStatusMarker(status string) (string, lipgloss.Style) {
	switch status {
	case "added":
		return "A", p.theme.OpenText
	case "removed":
		return "D", p.theme.ClosedText
	case "renamed", "copied":
		return "R", p.theme.AccentAltText
	default: // modified, changed, unchanged
		return "M", p.theme.AccentText
	}
}

func (p *prDetailModel) diffView() string {
	if p.filesErr != nil {
		return p.theme.ErrorText.Render("error: " + p.filesErr.Error())
	}
	if p.diffFile == nil {
		return p.theme.MutedText.Render("select a file")
	}
	if p.filesLoading {
		// Mid-refresh: the file identity still holds, the diff body does
		// not. Keep the header, replace the body with the spinner.
		return p.theme.Body.Render(p.diffFile.Filename) + "\n\n" +
			p.spinner.View() + " loading diff..."
	}

	header := p.theme.Body.Render(p.diffFile.Filename) + "  " +
		p.theme.OpenText.Render(fmt.Sprintf("+%d", p.diffFile.Additions)) + " " +
		p.theme.ClosedText.Render(fmt.Sprintf("-%d", p.diffFile.Deletions))
	switch {
	case p.threadsLoading:
		header += "  " + p.theme.MutedText.Render(p.spinner.View()+" loading review comments...")
	case p.threadsErr != nil:
		header += "  " + p.theme.ErrorText.Render("(review comments failed to load: "+p.threadsErr.Error()+")")
	}

	return header + "\n\n" + p.diffVp.View()
}

// footerHints builds this view's contextual keybind hints. Vertical
// scroll cites the config down/up keys (matching issuesTab/releasesTab's
// existing detail-view hints) even though the underlying viewport's own
// keymap is what actually answers j/k -- a pre-existing characteristic of
// every viewport-backed detail view in ghab, not new here. Horizontal
// scroll has no [keys] slot (same "no natural config slot" precedent as
// the hardcoded "tab" pane-focus hint), so it's spelled out literally.
func (p *prDetailModel) footerHints(cfg config.Config) []style.KeyHint {
	switch p.view {
	case prViewFiles:
		return []style.KeyHint{
			{Keys: cfg.Keys.Down + "/" + cfg.Keys.Up, Label: "move"},
			{Keys: cfg.Keys.Open, Label: "diff"},
			{Keys: "tab", Label: "conversation"},
		}
	case prViewDiff:
		return []style.KeyHint{
			{Keys: cfg.Keys.Down + "/" + cfg.Keys.Up, Label: "scroll"},
			{Keys: "h/l", Label: "h-scroll"},
			{Keys: cfg.Keys.Back, Label: "files"},
		}
	default:
		return []style.KeyHint{
			{Keys: cfg.Keys.Down + "/" + cfg.Keys.Up, Label: "scroll"},
			{Keys: "tab", Label: "files"},
		}
	}
}

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

// releaseFocus is which region of the releases tab's detail view is
// receiving j/k/enter -- the glamour body or the assets list below it.
// Same idea as codeTab's tree/preview focus split (Tab key, hardcoded,
// not config-remappable -- see codeTab.handleKey's comment).
type releaseFocus int

const (
	focusBody releaseFocus = iota
	focusAssets
)

// releasesMsg carries the async result of fetching
// repos/{owner}/{repo}/releases.
type releasesMsg struct {
	releases []gh.Release
	err      error
}

// assetProgressMsg / assetDoneMsg carry a download's lifecycle from the
// background goroutine startDownload spawns, through a channel a
// self-resubscribing tea.Cmd polls one message at a time -- bubbletea's
// standard pattern for a progress bar fed by a blocking operation.
type assetProgressMsg struct {
	done, total int64
}

type assetDoneMsg struct {
	err  error
	dest string
}

// releasesTab owns the releases tab's state: the fetched list, a
// list/detail split (selected != nil means detail), and -- in detail --
// an optional in-progress asset download. Survives tab switches (owned by
// RepoScreen, not rebuilt), matching readmeTab/codeTab's pattern.
//
// Back-key interception: RepoScreen only forwards the back key to this
// tab's Update when atDetail() is true (see RepoScreen.Update's Back
// case); at list level RepoScreen pops the whole screen itself, so this
// tab never needs to check the back key at list level.
type releasesTab struct {
	cfg      config.Config
	theme    style.Theme
	client   *gh.Client
	styleOpt readmeStyleOption

	owner, repo string
	perPage     int

	spinner spinner.Model
	loading bool
	err     error
	list    []gh.Release

	cursor    int
	scrollOff int
	rows      int
	width     int

	// detail state
	selected      *gh.Release
	focus         releaseFocus
	assetCursor   int
	detailVp      viewport.Model
	renderedBody  string
	renderedWidth int

	downloading    bool
	downloadAsset  *gh.Asset
	downloadDone   int64
	downloadTotal  int64
	downloadErr    error
	downloadedPath string
	progressCh     chan assetProgressMsg
	doneCh         chan assetDoneMsg
}

func newReleasesTab(cfg config.Config, theme style.Theme, client *gh.Client, styleOpt readmeStyleOption, owner, repo string) *releasesTab {
	sp := spinner.New(spinner.WithSpinner(spinner.Dot), spinner.WithStyle(theme.Spinner))
	return &releasesTab{
		cfg:           cfg,
		theme:         theme,
		client:        client,
		styleOpt:      styleOpt,
		owner:         owner,
		repo:          repo,
		perPage:       cfg.Behavior.PageSize,
		spinner:       sp,
		loading:       true,
		detailVp:      viewport.New(0, 0),
		renderedWidth: -1,
	}
}

// start kicks off the releases fetch. Called from RepoScreen.Init, in
// parallel with the other tabs' fetches.
func (t *releasesTab) start() tea.Cmd {
	owner, repo, perPage, client := t.owner, t.repo, t.perPage, t.client
	return tea.Batch(t.spinner.Tick, func() tea.Msg {
		list, err := client.Releases(owner, repo, perPage)
		return releasesMsg{releases: list, err: err}
	})
}

// refresh re-arms the loading state and refetches -- called by
// RepoScreen's "r" handler when the releases tab is active, after it
// busts the cache entry.
func (t *releasesTab) refresh() tea.Cmd {
	t.loading = true
	t.err = nil
	return t.start()
}

// atDetail reports whether the tab is showing a release's detail view
// rather than the list.
func (t *releasesTab) atDetail() bool {
	return t.selected != nil
}

func (t *releasesTab) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case releasesMsg:
		t.loading = false
		t.err = msg.err
		if msg.err == nil {
			t.list = msg.releases
			t.clampCursor()
		}
		return nil

	case spinner.TickMsg:
		if !t.loading {
			return nil
		}
		var cmd tea.Cmd
		t.spinner, cmd = t.spinner.Update(msg)
		return cmd

	case assetProgressMsg:
		if !t.downloading {
			return nil
		}
		t.downloadDone = msg.done
		if msg.total > 0 {
			t.downloadTotal = msg.total
		}
		return t.listenProgress()

	case assetDoneMsg:
		if !t.downloading {
			return nil
		}
		t.downloading = false
		t.downloadErr = msg.err
		if msg.err == nil {
			t.downloadedPath = msg.dest
			t.downloadDone = t.downloadTotal
		}
		return nil

	case tea.KeyMsg:
		return t.handleKey(msg)
	}
	return nil
}

func (t *releasesTab) handleKey(msg tea.KeyMsg) tea.Cmd {
	if t.selected == nil {
		return t.handleListKey(msg)
	}
	return t.handleDetailKey(msg)
}

func (t *releasesTab) handleListKey(msg tea.KeyMsg) tea.Cmd {
	switch {
	case matchesKey(msg, t.cfg.Keys.Down):
		t.moveCursor(1)
	case matchesKey(msg, t.cfg.Keys.Up):
		t.moveCursor(-1)
	case matchesKey(msg, t.cfg.Keys.Open):
		t.enterDetail()
	}
	return nil
}

// handleDetailKey: back pops detail -> list (never bubbles to RepoScreen
// from here -- see the struct comment). Tab switches focus between the
// body and the assets list, mirroring codeTab's tree/preview split.
func (t *releasesTab) handleDetailKey(msg tea.KeyMsg) tea.Cmd {
	if matchesKey(msg, t.cfg.Keys.Back) {
		t.exitDetail()
		return nil
	}
	if msg.Type == tea.KeyTab {
		if t.focus == focusBody {
			t.focus = focusAssets
		} else {
			t.focus = focusBody
		}
		t.applyDetailContent()
		return nil
	}

	switch t.focus {
	case focusBody:
		var cmd tea.Cmd
		t.detailVp, cmd = t.detailVp.Update(msg)
		return cmd
	case focusAssets:
		switch {
		case matchesKey(msg, t.cfg.Keys.Down):
			t.moveAssetCursor(1)
		case matchesKey(msg, t.cfg.Keys.Up):
			t.moveAssetCursor(-1)
		case matchesKey(msg, t.cfg.Keys.Open):
			return t.downloadSelectedAsset()
		}
	}
	return nil
}

func (t *releasesTab) moveCursor(delta int) {
	if len(t.list) == 0 {
		return
	}
	t.cursor += delta
	t.clampCursor()
}

func (t *releasesTab) clampCursor() {
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

func (t *releasesTab) moveAssetCursor(delta int) {
	if t.selected == nil || len(t.selected.Assets) == 0 {
		return
	}
	t.assetCursor += delta
	if t.assetCursor < 0 {
		t.assetCursor = 0
	}
	if t.assetCursor >= len(t.selected.Assets) {
		t.assetCursor = len(t.selected.Assets) - 1
	}
	t.applyDetailContent()
}

func (t *releasesTab) enterDetail() {
	if len(t.list) == 0 || t.cursor >= len(t.list) {
		return
	}
	r := t.list[t.cursor]
	t.selected = &r
	t.focus = focusBody
	t.assetCursor = 0
	t.renderedWidth = -1
	t.downloading = false
	t.downloadAsset = nil
	t.downloadErr = nil
	t.downloadedPath = ""
	t.applyDetailContent()
}

func (t *releasesTab) exitDetail() {
	t.selected = nil
}

// downloadSelectedAsset starts a download of the asset under assetCursor,
// refusing to start a second one while one is already in flight.
func (t *releasesTab) downloadSelectedAsset() tea.Cmd {
	if t.downloading || t.selected == nil || t.assetCursor >= len(t.selected.Assets) {
		return nil
	}
	return t.startDownload(t.selected.Assets[t.assetCursor])
}

// startDownload runs the asset download in a background goroutine,
// feeding progress through progressCh (a bubbletea "listen" Cmd polls one
// message at a time and re-subscribes -- see listenProgress) and the
// final result through doneCh. progressCh is closed once the download
// finishes so a still-pending listenProgress Cmd unblocks cleanly instead
// of leaking.
func (t *releasesTab) startDownload(a gh.Asset) tea.Cmd {
	t.downloading = true
	asset := a
	t.downloadAsset = &asset
	t.downloadDone = 0
	t.downloadTotal = a.Size
	t.downloadErr = nil
	t.downloadedPath = ""
	t.applyDetailContent()

	progressCh := make(chan assetProgressMsg, 1)
	doneCh := make(chan assetDoneMsg, 1)
	t.progressCh = progressCh
	t.doneCh = doneCh

	dest := gh.AssetDestPath(t.cfg.Behavior.CloneDir, a.Name)
	client := t.client

	go func() {
		err := client.DownloadAsset(a.URL, dest, func(done, total int64) {
			select {
			case progressCh <- assetProgressMsg{done: done, total: total}:
			default:
				// a progress update is already queued; this one will be
				// superseded once listenProgress catches up
			}
		})
		doneCh <- assetDoneMsg{err: err, dest: dest}
		close(progressCh)
	}()

	return tea.Batch(t.listenProgress(), t.listenDone())
}

func (t *releasesTab) listenProgress() tea.Cmd {
	ch := t.progressCh
	return func() tea.Msg {
		p, ok := <-ch
		if !ok {
			return nil
		}
		return p
	}
}

func (t *releasesTab) listenDone() tea.Cmd {
	ch := t.doneCh
	return func() tea.Msg {
		return <-ch
	}
}

// resize sets the tab's pane geometry from the shared tab-body area
// (width x rows -- the same area every repo tab renders into).
func (t *releasesTab) resize(width, rows int) {
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

// renderBodyIfNeeded (re-)renders the release body through glamour,
// cached by the width it was last rendered at -- same rationale as
// readmeTab.renderContent (glamour bakes word wrap in at construction).
func (t *releasesTab) renderBodyIfNeeded() {
	if t.selected == nil || t.detailVp.Width <= 0 {
		return
	}
	if t.renderedWidth == t.detailVp.Width && t.renderedBody != "" {
		return
	}
	body := strings.TrimSpace(t.selected.Body)
	if body == "" {
		t.renderedBody = t.theme.MutedText.Render("(no release notes)")
	} else if out, err := t.styleOpt.render(t.selected.Body, t.detailVp.Width); err != nil {
		t.renderedBody = t.theme.ErrorText.Render("error rendering release notes: " + err.Error())
	} else {
		t.renderedBody = out
	}
	t.renderedWidth = t.detailVp.Width
}

// applyDetailContent rebuilds the detail viewport's content: the
// (cached) rendered body followed by the assets list. Called whenever
// something the assets section depends on changes (focus, asset cursor,
// download progress) since that part is cheap to re-render every time,
// unlike the glamour body.
func (t *releasesTab) applyDetailContent() {
	if t.selected == nil || t.detailVp.Width <= 0 {
		return
	}
	t.renderBodyIfNeeded()

	var b strings.Builder
	b.WriteString(t.renderedBody)
	b.WriteString("\n\n")
	b.WriteString(t.theme.Body.Render("assets:"))
	b.WriteString("\n")
	if len(t.selected.Assets) == 0 {
		b.WriteString(t.theme.MutedText.Render("(no assets)"))
	} else {
		rows := make([]string, len(t.selected.Assets))
		for i, a := range t.selected.Assets {
			rows[i] = t.renderAssetRow(a, i)
		}
		b.WriteString(strings.Join(rows, "\n"))
	}

	t.detailVp.SetContent(b.String())
}

func (t *releasesTab) renderAssetRow(a gh.Asset, idx int) string {
	marker := "  "
	hostHint := ""
	if hostAssetMatch(a.Name) {
		marker = "▸ "
		hostHint = " (this machine)"
	}
	meta := fmt.Sprintf(" -- %s, %d downloads", gh.HumanBytes(a.Size), a.DownloadCount)
	plain := marker + a.Name + hostHint + meta

	width := t.detailVp.Width
	if width < 1 {
		width = lipgloss.Width(plain)
	}

	selected := t.focus == focusAssets && idx == t.assetCursor
	var row string
	if selected {
		row = lipgloss.NewStyle().Width(width).MaxWidth(width).
			Background(t.theme.SelectionBg).Foreground(t.theme.Fg).Render(plain)
	} else {
		styled := t.theme.AccentText.Render(marker) + a.Name + t.theme.MutedText.Render(hostHint+meta)
		row = lipgloss.NewStyle().Width(width).MaxWidth(width).Render(styled)
	}

	if status := t.assetStatusLine(a); status != "" {
		row += "\n" + t.theme.MutedText.Render(status)
	}
	return row
}

func (t *releasesTab) assetStatusLine(a gh.Asset) string {
	if t.downloadAsset == nil || t.downloadAsset.Name != a.Name {
		return ""
	}
	if t.downloadErr != nil {
		return "  download error: " + t.downloadErr.Error()
	}
	if t.downloadedPath != "" {
		return "  downloaded -> " + t.downloadedPath
	}
	if t.downloading {
		pct := 0
		if t.downloadTotal > 0 {
			pct = int(t.downloadDone * 100 / t.downloadTotal)
		}
		return fmt.Sprintf("  downloading... %d%%  (%s / %s)", pct, gh.HumanBytes(t.downloadDone), gh.HumanBytes(t.downloadTotal))
	}
	return ""
}

func (t *releasesTab) View() string {
	if t.loading {
		return t.spinner.View() + " loading releases..."
	}
	if t.err != nil {
		return t.theme.ErrorText.Render("error: " + t.err.Error())
	}
	if t.selected != nil {
		return t.detailVp.View()
	}
	if len(t.list) == 0 {
		return t.theme.MutedText.Render("no releases")
	}
	return strings.Join(t.listBodyLines(), "\n")
}

func (t *releasesTab) listBodyLines() []string {
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

func (t *releasesTab) renderListRow(r gh.Release, selected bool) string {
	name := r.Name
	if name == "" {
		name = r.TagName
	}
	pre := ""
	if r.Prerelease {
		pre = " (pre)"
	}
	plain := fmt.Sprintf("%s  %s  %s%s", r.TagName, name, relativeAge(r.PublishedAt), pre)

	width := t.width
	if width < 1 {
		width = lipgloss.Width(plain)
	}

	if selected {
		return lipgloss.NewStyle().Width(width).MaxWidth(width).
			Background(t.theme.SelectionBg).Foreground(t.theme.Fg).Render(plain)
	}

	styled := t.theme.ReleaseText.Render(r.TagName) + "  " + t.theme.Body.Render(name) +
		"  " + t.theme.MutedText.Render(relativeAge(r.PublishedAt)+pre)
	return lipgloss.NewStyle().Width(width).MaxWidth(width).Render(styled)
}

// footerHints builds this tab's contextual keybind hints for
// RepoScreen.Footer, per BUILD.md's M3 footer spec.
func (t *releasesTab) footerHints(cfg config.Config) []style.KeyHint {
	if t.atDetail() {
		hints := []style.KeyHint{
			{Keys: cfg.Keys.Down + "/" + cfg.Keys.Up, Label: "scroll/move"},
			{Keys: "tab", Label: "focus"},
		}
		if t.focus == focusAssets {
			hints = append(hints, style.KeyHint{Keys: cfg.Keys.Open, Label: "download"})
		}
		return hints
	}
	return []style.KeyHint{
		{Keys: cfg.Keys.Down + "/" + cfg.Keys.Up, Label: "move"},
		{Keys: cfg.Keys.Open, Label: "open"},
	}
}

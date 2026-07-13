package ui

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/Sh-ui/ghab/internal/config"
	"github.com/Sh-ui/ghab/internal/gh"
	"github.com/Sh-ui/ghab/internal/ui/style"
)

// codeFocus is which of the code tab's two panes is receiving j/k/enter.
type codeFocus int

const (
	focusTree codeFocus = iota
	focusPreview
)

// minTreeWidth / minPreviewWidth are the code tab's split-pane floors, per
// BUILD.md ("~40% width, min 24 cols" for the tree; the remainder for
// preview, floored so a narrow terminal never collapses the preview to
// nothing).
const (
	minTreeWidth    = 24
	minPreviewWidth = 20
)

// treeMsg carries the async result of the recursive tree fetch.
type treeMsg struct {
	tree gh.Tree
	err  error
}

// dirMsg carries the async result of a lazy per-directory fetch (the
// truncated-tree fallback).
type dirMsg struct {
	path    string
	entries []gh.DirEntry
	err     error
}

// fileMsg carries the async result of a file preview fetch.
type fileMsg struct {
	path string
	data []byte
	err  error
}

// editDoneMsg carries the exit result of the editor process spawned by the
// edit hook.
type editDoneMsg struct {
	err error
}

// codeTab owns the code tab's state: the file tree (with lazy per-dir
// expansion for a truncated tree), the focused pane, and the currently
// previewed file. It survives tab switches (owned by RepoScreen, not
// rebuilt) and its own background fetches (tree, lazy dir, file preview)
// keep running whether or not the code tab is the active one.
type codeTab struct {
	cfg    config.Config
	theme  style.Theme
	client *gh.Client

	owner, repo, branch string
	started             bool // true once branch is known and the tree fetch has been kicked off

	spinner spinner.Model

	loadingTree bool
	treeErr     error
	truncated   bool
	root        *treeNode
	flat        []*treeNode
	cursor      int
	scrollOff   int
	loadingDir  map[string]bool // paths with an in-flight lazy Dir() fetch

	focus codeFocus

	selected     *treeNode
	loadingFile  bool
	fileErr      error
	previewData  []byte   // raw bytes of the currently previewed file; nil unless a text preview succeeded (gates the edit hook)
	previewLines []string // gutter-prefixed, chroma-highlighted lines, untruncated by pane width
	previewVp    viewport.Model

	editErr error

	treeWidth, previewWidth int
	interiorRows            int
}

func newCodeTab(cfg config.Config, theme style.Theme, client *gh.Client, owner, repo string) *codeTab {
	sp := spinner.New(spinner.WithSpinner(spinner.Dot), spinner.WithStyle(theme.Spinner))
	return &codeTab{
		cfg:        cfg,
		theme:      theme,
		client:     client,
		owner:      owner,
		repo:       repo,
		spinner:    sp,
		root:       &treeNode{isDir: true, expanded: true},
		loadingDir: map[string]bool{},
		previewVp:  viewport.New(0, 0),
		focus:      focusTree,
	}
}

// start kicks off the recursive tree fetch once the repo's default branch
// is known (called from RepoScreen after repoMetaMsg lands -- unlike the
// readme tab, the code tab can't fetch anything until it knows which
// branch to fetch). Calling it more than once is a no-op.
func (c *codeTab) start(branch string) tea.Cmd {
	if c.started {
		return nil
	}
	c.started = true
	c.branch = branch
	c.loadingTree = true
	return tea.Batch(c.spinner.Tick, c.fetchTreeCmd())
}

// refresh re-arms the tree-loading state and refetches, bypassing the
// started guard start() uses (branch is already known past the first
// load) -- called by RepoScreen's "r" handler when the code tab is
// active, after it busts the tree cache entry.
func (c *codeTab) refresh() tea.Cmd {
	c.loadingTree = true
	c.treeErr = nil
	return tea.Batch(c.spinner.Tick, c.fetchTreeCmd())
}

func (c *codeTab) fetchTreeCmd() tea.Cmd {
	owner, repo, branch, client := c.owner, c.repo, c.branch, c.client
	return func() tea.Msg {
		t, err := client.Tree(owner, repo, branch)
		return treeMsg{tree: t, err: err}
	}
}

func (c *codeTab) fetchDirCmd(n *treeNode) tea.Cmd {
	c.loadingDir[n.path] = true
	owner, repo, client, path := c.owner, c.repo, c.client, n.path
	return func() tea.Msg {
		entries, err := client.Dir(owner, repo, path)
		return dirMsg{path: path, entries: entries, err: err}
	}
}

func (c *codeTab) fetchFileCmd(n *treeNode) tea.Cmd {
	owner, repo, branch, client, path, size := c.owner, c.repo, c.branch, c.client, n.path, n.size
	return func() tea.Msg {
		data, err := client.FileRaw(owner, repo, path, branch, size)
		return fileMsg{path: path, data: data, err: err}
	}
}

func (c *codeTab) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case treeMsg:
		c.loadingTree = false
		c.treeErr = msg.err
		if msg.err == nil {
			c.truncated = msg.tree.Truncated
			c.root = buildTree(msg.tree.Entries)
			if !c.truncated {
				markLoaded(c.root)
			}
			c.rebuildFlat()
		}
		return nil

	case dirMsg:
		delete(c.loadingDir, msg.path)
		node := findNode(c.root, msg.path)
		if node == nil {
			return nil
		}
		if msg.err != nil {
			node.err = msg.err
			return nil
		}
		node.children = dirEntriesToNodes(msg.entries, node.depth)
		node.loaded = true
		c.rebuildFlat()
		return nil

	case fileMsg:
		if c.selected == nil || c.selected.path != msg.path {
			return nil // stale response for a since-deselected file
		}
		c.loadingFile = false
		c.fileErr = msg.err
		if msg.err != nil {
			c.previewData = nil
			c.previewLines = nil
			c.applyPreviewContent()
			return nil
		}
		lines, err := renderPreview(c.theme, msg.path, string(msg.data))
		if err != nil {
			c.fileErr = err
			c.previewData = nil
			c.previewLines = nil
		} else {
			c.previewData = msg.data
			c.previewLines = lines
		}
		c.applyPreviewContent()
		return nil

	case editDoneMsg:
		c.editErr = msg.err
		return nil

	case spinner.TickMsg:
		if !c.loadingTree && !c.loadingFile {
			return nil
		}
		var cmd tea.Cmd
		c.spinner, cmd = c.spinner.Update(msg)
		return cmd

	case tea.KeyMsg:
		return c.handleKey(msg)
	}
	return nil
}

func (c *codeTab) handleKey(msg tea.KeyMsg) tea.Cmd {
	if msg.Type == tea.KeyTab {
		if c.focus == focusTree {
			c.focus = focusPreview
		} else {
			c.focus = focusTree
		}
		return nil
	}

	if matchesKey(msg, c.cfg.Keys.Edit) {
		return c.startEdit()
	}

	switch c.focus {
	case focusTree:
		return c.handleTreeKey(msg)
	case focusPreview:
		var cmd tea.Cmd
		c.previewVp, cmd = c.previewVp.Update(msg)
		return cmd
	}
	return nil
}

func (c *codeTab) handleTreeKey(msg tea.KeyMsg) tea.Cmd {
	switch {
	case matchesKey(msg, c.cfg.Keys.Down):
		c.moveCursor(1)
	case matchesKey(msg, c.cfg.Keys.Up):
		c.moveCursor(-1)
	case matchesKey(msg, c.cfg.Keys.Open):
		return c.activateSelected()
	}
	return nil
}

func (c *codeTab) moveCursor(delta int) {
	if len(c.flat) == 0 {
		return
	}
	c.cursor += delta
	if c.cursor < 0 {
		c.cursor = 0
	}
	if c.cursor >= len(c.flat) {
		c.cursor = len(c.flat) - 1
	}
	c.clampScroll()
}

func (c *codeTab) clampScroll() {
	if c.interiorRows <= 0 {
		return
	}
	if c.cursor < c.scrollOff {
		c.scrollOff = c.cursor
	}
	if c.cursor >= c.scrollOff+c.interiorRows {
		c.scrollOff = c.cursor - c.interiorRows + 1
	}
	if c.scrollOff < 0 {
		c.scrollOff = 0
	}
}

func (c *codeTab) rebuildFlat() {
	c.flat = flatten(c.root)
	if c.cursor >= len(c.flat) {
		c.cursor = len(c.flat) - 1
	}
	if c.cursor < 0 {
		c.cursor = 0
	}
	c.clampScroll()
}

// activateSelected handles enter/open on the current cursor row: toggles a
// dir's expansion (lazy-fetching its children if this is a truncated tree
// and they aren't loaded yet), or selects a file and kicks off its preview
// fetch.
func (c *codeTab) activateSelected() tea.Cmd {
	if len(c.flat) == 0 || c.cursor >= len(c.flat) {
		return nil
	}
	n := c.flat[c.cursor]

	if n.isDir {
		n.expanded = !n.expanded
		var cmd tea.Cmd
		if n.expanded && !n.loaded && !c.loadingDir[n.path] {
			cmd = c.fetchDirCmd(n)
		}
		c.rebuildFlat()
		return cmd
	}

	c.selected = n
	c.loadingFile = true
	c.fileErr = nil
	c.previewData = nil
	c.previewLines = nil
	c.editErr = nil
	c.applyPreviewContent()
	return tea.Batch(c.spinner.Tick, c.fetchFileCmd(n))
}

// startEdit writes the currently previewed file's blob to the editor cache
// dir and launches [behavior].editor on it via tea.ExecProcess. A no-op
// when nothing text-previewable is loaded (TooLarge/Binary files never
// populate previewData, so there's nothing to hand the editor).
func (c *codeTab) startEdit() tea.Cmd {
	if c.previewData == nil || c.selected == nil {
		return nil
	}
	dest, err := writeEditorCache(c.owner, c.repo, c.selected.path, c.previewData)
	if err != nil {
		c.editErr = err
		return nil
	}
	c.editErr = nil
	cmd := buildEditorCommand(c.cfg.Behavior.Editor, dest)
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		return editDoneMsg{err: err}
	})
}

// resize sets the code tab's pane geometry from the shared tab-body area
// (totalWidth x rows -- the same area every repo tab renders into).
func (c *codeTab) resize(totalWidth, rows int) {
	treeWidth := totalWidth * 4 / 10
	if treeWidth < minTreeWidth {
		treeWidth = minTreeWidth
	}
	if treeWidth > totalWidth-minPreviewWidth {
		treeWidth = totalWidth - minPreviewWidth
	}
	if treeWidth < 1 {
		treeWidth = 1
	}
	previewWidth := totalWidth - treeWidth
	if previewWidth < 1 {
		previewWidth = 1
	}
	c.treeWidth = treeWidth
	c.previewWidth = previewWidth

	interior := rows - 2 // Frame() draws a top+bottom border on top of the body lines it's given
	if interior < 1 {
		interior = 1
	}
	c.interiorRows = interior

	c.previewVp.Width = previewWidth - 2
	c.previewVp.Height = interior
	c.clampScroll()
	c.applyPreviewContent()
}

// applyPreviewContent (re-)renders the preview viewport's content from
// current state: an error/placeholder line, a loading line, or the
// gutter+highlight lines truncated to the pane's current width.
func (c *codeTab) applyPreviewContent() {
	if c.previewVp.Width <= 0 {
		return
	}
	if c.fileErr != nil {
		c.previewVp.SetContent(previewErrorLine(c.theme, c.fileErr))
		return
	}
	if len(c.previewLines) == 0 {
		c.previewVp.SetContent("")
		return
	}
	lineStyle := lipgloss.NewStyle().Width(c.previewVp.Width).MaxWidth(c.previewVp.Width)
	trunc := make([]string, len(c.previewLines))
	for i, l := range c.previewLines {
		trunc[i] = lineStyle.Render(l)
	}
	c.previewVp.SetContent(strings.Join(trunc, "\n"))
}

// renderPreview highlights source with chroma and prefixes each line with
// a right-aligned, muted line-number gutter (BUILD.md: "single space then
// content").
func renderPreview(th style.Theme, path, source string) ([]string, error) {
	highlighted, err := style.Highlight(th, path, source)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(highlighted, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	width := len(strconv.Itoa(len(lines)))
	if width < 2 {
		width = 2
	}
	for i, line := range lines {
		lines[i] = th.MutedText.Render(fmt.Sprintf("%*d", width, i+1)) + " " + line
	}
	return lines, nil
}

// previewErrorLine renders the muted TooLarge/Binary placeholders BUILD.md
// specifies verbatim ("binary file (243 KB)", "file too large (2.1 MB)"),
// or a generic error line for anything else (network failure, 404, ...).
func previewErrorLine(th style.Theme, err error) string {
	var tooLarge *gh.TooLargeError
	if errors.As(err, &tooLarge) {
		return th.MutedText.Render(fmt.Sprintf("file too large (%s)", gh.HumanBytes(tooLarge.Size)))
	}
	var binary *gh.BinaryError
	if errors.As(err, &binary) {
		return th.MutedText.Render(fmt.Sprintf("binary file (%s)", gh.HumanBytes(binary.Size)))
	}
	return th.ErrorText.Render("error: " + err.Error())
}

// View renders the tree pane and preview pane side by side.
func (c *codeTab) View() string {
	treeBorder, previewBorder := c.theme.Frame, c.theme.Frame
	if c.focus == focusTree {
		treeBorder = c.theme.Accent
	} else {
		previewBorder = c.theme.Accent
	}

	treeBox := style.FrameColor(c.theme, "files", c.treeBodyLines(), c.treeWidth, treeBorder)
	previewBox := style.FrameColor(c.theme, c.previewTitle(), c.previewBodyLines(), c.previewWidth, previewBorder)
	return lipgloss.JoinHorizontal(lipgloss.Top, treeBox, previewBox)
}

func (c *codeTab) previewTitle() string {
	if c.selected == nil {
		return "preview"
	}
	return c.selected.path
}

func (c *codeTab) treeBodyLines() []string {
	innerWidth := c.treeWidth - 2
	if innerWidth < 1 {
		innerWidth = 1
	}
	rows := make([]string, c.interiorRows)

	if c.loadingTree {
		rows[0] = c.spinner.View() + " loading tree..."
		return padTruncRows(rows, innerWidth)
	}
	if c.treeErr != nil {
		rows[0] = c.theme.ErrorText.Render("error: " + c.treeErr.Error())
		return padTruncRows(rows, innerWidth)
	}
	if len(c.flat) == 0 {
		rows[0] = c.theme.MutedText.Render("(empty)")
		return padTruncRows(rows, innerWidth)
	}

	for i := range rows {
		idx := c.scrollOff + i
		if idx >= len(c.flat) {
			continue
		}
		rows[i] = c.renderTreeRow(c.flat[idx], idx == c.cursor, innerWidth)
	}
	return rows
}

func (c *codeTab) renderTreeRow(n *treeNode, selected bool, innerWidth int) string {
	indent := strings.Repeat("  ", n.depth-1)
	marker := "  "
	if n.isDir {
		if n.expanded {
			marker = "▾ "
		} else {
			marker = "▸ "
		}
	}
	plain := indent + marker + n.name

	if selected {
		return lipgloss.NewStyle().
			Width(innerWidth).MaxWidth(innerWidth).
			Background(c.theme.SelectionBg).
			Foreground(c.theme.Fg).
			Render(plain)
	}

	var styled string
	switch {
	case n.isDir && n.err != nil:
		// A lazy Dir() fetch failed for this directory (truncated-tree
		// case) -- mark it in error color rather than silently showing an
		// empty expansion with no feedback.
		styled = indent + c.theme.ErrorText.Render(marker+n.name)
	case n.isDir:
		styled = indent + c.theme.MutedText.Render(marker) + c.theme.Body.Render(n.name)
	default:
		styled = indent + marker + c.theme.Body.Render(n.name)
	}
	return lipgloss.NewStyle().Width(innerWidth).MaxWidth(innerWidth).Render(styled)
}

func (c *codeTab) previewBodyLines() []string {
	if c.selected == nil {
		return padRows(c.interiorRows, c.theme.MutedText.Render("select a file to preview"))
	}
	if c.loadingFile {
		return padRows(c.interiorRows, c.spinner.View()+c.theme.MutedText.Render(" loading "+c.selected.name+"..."))
	}

	// c.previewVp.View() always returns exactly c.interiorRows lines
	// (viewport pads/truncates to its configured Height).
	body := strings.Split(c.previewVp.View(), "\n")
	if c.editErr != nil {
		errLine := c.theme.ErrorText.Render("edit: " + c.editErr.Error())
		body = append([]string{errLine}, body...)
		if len(body) > c.interiorRows {
			body = body[:c.interiorRows]
		}
	}
	return body
}

// padTruncRows truncates/pads each row to width -- used for the tree
// pane's single-line status messages (loading/error/empty), which are
// otherwise plain strings without per-row width handling.
func padTruncRows(rows []string, width int) []string {
	if width <= 0 {
		return rows
	}
	style := lipgloss.NewStyle().Width(width).MaxWidth(width)
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = style.Render(r)
	}
	return out
}

// padRows builds a rows-line slice with first in row 0 and the rest blank
// -- keeps the preview pane's placeholder/loading states the same height
// as the tree pane so the two frames line up.
func padRows(rows int, first string) []string {
	out := make([]string, rows)
	if rows > 0 {
		out[0] = first
	}
	return out
}

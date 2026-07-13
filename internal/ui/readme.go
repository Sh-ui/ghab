package ui

import (
	"errors"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"

	"github.com/Sh-ui/ghab/internal/gh"
	"github.com/Sh-ui/ghab/internal/ui/style"
)

// readmeStyleOption is the glamour stylesheet resolution done once at app
// startup (config.ResolveReadmeStyle + config.ColorMode, in main.go):
// stylePath is the tracked ombre JSON to use, or a "standard:<name>"
// sentinel selecting one of glamour's built-in styles so the fallback
// still follows the color mode (dark text on a light terminal and vice
// versa would fail the both-modes bar). BUILD.md: fail-soft, one stderr
// warning at load -- this is a load-time value, not re-resolved per render.
type readmeStyleOption struct {
	stylePath string
}

// render builds a fresh glamour TermRenderer at width (glamour bakes word
// wrap in at construction, so there's no way to reuse one renderer across
// widths) and renders markdown through it.
func (o readmeStyleOption) render(markdown string, width int) (string, error) {
	opts := []glamour.TermRendererOption{glamour.WithWordWrap(width)}
	switch {
	case strings.HasPrefix(o.stylePath, "standard:"):
		opts = append(opts, glamour.WithStandardStyle(strings.TrimPrefix(o.stylePath, "standard:")))
	case o.stylePath != "":
		opts = append(opts, glamour.WithStylePath(o.stylePath))
	default:
		opts = append(opts, glamour.WithStandardStyle("dark"))
	}
	r, err := glamour.NewTermRenderer(opts...)
	if err != nil {
		return "", err
	}
	return r.Render(markdown)
}

// readmeMsg carries the async result of fetching repos/{owner}/{repo}/readme.
type readmeMsg struct {
	content string
	err     error
}

// readmeTab owns the readme tab's state: the fetched markdown source, the
// glamour-rendered string cached by the width it was rendered at (BUILD.md:
// "re-render on resize ... cache the rendered string keyed by width"), and
// the viewport it's scrolled through. It survives tab switches (owned by
// RepoScreen, not rebuilt).
type readmeTab struct {
	theme    style.Theme
	client   *gh.Client
	styleOpt readmeStyleOption

	owner, repo string

	vp      viewport.Model
	spinner spinner.Model

	loading  bool
	err      error
	noReadme bool
	raw      string

	renderedWidth int // -1 until the first render; vp.Width the content was last rendered at
}

func newReadmeTab(theme style.Theme, client *gh.Client, styleOpt readmeStyleOption, owner, repo string) *readmeTab {
	sp := spinner.New(spinner.WithSpinner(spinner.Dot), spinner.WithStyle(theme.Spinner))
	return &readmeTab{
		theme:         theme,
		client:        client,
		styleOpt:      styleOpt,
		owner:         owner,
		repo:          repo,
		vp:            viewport.New(0, 0),
		spinner:       sp,
		loading:       true,
		renderedWidth: -1,
	}
}

// start kicks off the readme fetch. Called from RepoScreen.Init so it
// fetches in parallel with the repo-meta request, not gated on it.
func (t *readmeTab) start() tea.Cmd {
	owner, repo, client := t.owner, t.repo, t.client
	return tea.Batch(t.spinner.Tick, func() tea.Msg {
		content, err := client.Readme(owner, repo)
		return readmeMsg{content: content, err: err}
	})
}

func (t *readmeTab) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case readmeMsg:
		t.loading = false
		if msg.err != nil {
			var noReadme *gh.NoReadmeError
			if errors.As(msg.err, &noReadme) {
				t.noReadme = true
			} else {
				t.err = msg.err
			}
			return nil
		}
		t.raw = msg.content
		t.renderContent()
		return nil

	case spinner.TickMsg:
		if !t.loading {
			return nil
		}
		var cmd tea.Cmd
		t.spinner, cmd = t.spinner.Update(msg)
		return cmd

	case tea.KeyMsg:
		var cmd tea.Cmd
		t.vp, cmd = t.vp.Update(msg)
		return cmd
	}
	return nil
}

// resize sets the viewport's dimensions and re-renders if the width
// changed (readme content wraps to width, so a width change invalidates
// the cached render).
func (t *readmeTab) resize(width, height int) {
	t.vp.Width = width
	t.vp.Height = height
	t.renderContent()
}

func (t *readmeTab) renderContent() {
	if t.raw == "" || t.vp.Width <= 0 || t.vp.Width == t.renderedWidth {
		return
	}
	out, err := t.styleOpt.render(t.raw, t.vp.Width)
	if err != nil {
		t.vp.SetContent(t.theme.ErrorText.Render("error rendering readme: " + err.Error()))
		return
	}
	t.renderedWidth = t.vp.Width
	t.vp.SetContent(out)
}

func (t *readmeTab) View() string {
	if t.loading {
		return t.spinner.View() + " loading readme..."
	}
	if t.noReadme {
		return t.theme.MutedText.Render("no readme")
	}
	if t.err != nil {
		return t.theme.ErrorText.Render("error: " + t.err.Error())
	}
	return t.vp.View()
}

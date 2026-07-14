// Package ui is ghab's tea model tree: a root App owning a screen stack
// (push/pop, back pops), plus the Home and Repo screens for M1.
package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Sh-ui/ghab/internal/config"
	"github.com/Sh-ui/ghab/internal/gh"
	"github.com/Sh-ui/ghab/internal/ui/style"
)

// App is the root bubbletea model: it owns the screen stack and the
// shared config/theme/client every screen is built from.
type App struct {
	cfg    config.Config
	theme  style.Theme
	client *gh.Client

	width, height int
	stack         []Screen
}

// NewApp builds the root model with Home as the base of the stack.
// readmeStylePath is the glamour stylesheet resolved once at app startup
// (main.go); it threads down to every Repo screen the app ever pushes. If
// jumpTo is non-empty, a Repo screen ("owner/repo") or Profile screen
// (bare "owner") is pushed on top so the app opens straight into it
// (main.go's positional-arg jump).
func NewApp(cfg config.Config, theme style.Theme, client *gh.Client, readmeStylePath, jumpTo string) *App {
	a := &App{
		cfg:    cfg,
		theme:  theme,
		client: client,
		stack:  []Screen{NewHomeScreen(cfg, theme, client, readmeStylePath)},
	}
	if owner, repo, ok := splitOwnerRepo(jumpTo); ok {
		a.stack = append(a.stack, NewRepoScreen(cfg, theme, client, readmeStylePath, owner, repo))
	} else if jumpTo != "" && !strings.Contains(jumpTo, "/") {
		a.stack = append(a.stack, NewProfileScreen(cfg, theme, client, readmeStylePath, jumpTo))
	}
	return a
}

func (a *App) Init() tea.Cmd {
	var cmds []tea.Cmd
	for _, s := range a.stack {
		if cmd := s.Init(); cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	return tea.Batch(cmds...)
}

func (a *App) top() Screen { return a.stack[len(a.stack)-1] }

func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width, a.height = msg.Width, msg.Height
	case tea.KeyMsg:
		if msg.Type == tea.KeyCtrlC {
			return a, tea.Quit
		}
	case pushScreenMsg:
		a.stack = append(a.stack, msg.screen)
		// Every repo visit lands in Home's session-only recents list,
		// regardless of which screen pushed it (home input, profile hop,
		// search result).
		if r, ok := msg.screen.(*RepoScreen); ok {
			if h, ok := a.stack[0].(*HomeScreen); ok {
				h.recordVisit(r.owner + "/" + r.repo)
			}
		}
		return a, msg.screen.Init()
	case popScreenMsg:
		if len(a.stack) > 1 {
			a.stack = a.stack[:len(a.stack)-1]
		}
		return a, nil
	}

	updated, cmd := a.top().Update(msg)
	a.stack[len(a.stack)-1] = updated
	return a, cmd
}

func (a *App) View() string {
	if a.width == 0 {
		return ""
	}
	bodyHeight := a.height - 1 // reserve the footer line
	if bodyHeight < 0 {
		bodyHeight = 0
	}
	body := a.top().View(a.width, bodyHeight)
	footer := style.Footer(a.theme, a.top().Footer())
	return body + "\n" + footer
}

// splitOwnerRepo parses "owner/repo"; ok is false for anything else
// (empty string, bare owner, multiple slashes).
func splitOwnerRepo(s string) (owner, repo string, ok bool) {
	if s == "" || strings.Count(s, "/") != 1 {
		return "", "", false
	}
	parts := strings.SplitN(s, "/", 2)
	if parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}

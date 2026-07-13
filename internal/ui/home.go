package ui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/Sh-ui/ghab/internal/config"
	"github.com/Sh-ui/ghab/internal/gh"
	"github.com/Sh-ui/ghab/internal/ui/style"
)

// HomeScreen is the app's landing screen: a single "owner/repo" text
// input. A valid "owner/repo" pushes the Repo screen; anything else
// shows an inline "search comes in M4" notice, per the M1 scope in
// BUILD.md's Milestones section (the fuller home screen -- bare-owner
// profile hop, search results, recent visits -- lands in M4).
type HomeScreen struct {
	cfg    config.Config
	theme  style.Theme
	client *gh.Client

	// readmeStylePath is threaded into every Repo screen this screen
	// pushes -- resolved once at app startup (main.go), not re-resolved
	// per repo.
	readmeStylePath string

	input  textinput.Model
	notice string
}

// NewHomeScreen builds the Home screen.
func NewHomeScreen(cfg config.Config, theme style.Theme, client *gh.Client, readmeStylePath string) *HomeScreen {
	ti := textinput.New()
	ti.Placeholder = "owner/repo"
	ti.Prompt = "> "
	ti.CharLimit = 256
	ti.Focus()
	return &HomeScreen{cfg: cfg, theme: theme, client: client, readmeStylePath: readmeStylePath, input: ti}
}

func (h *HomeScreen) Init() tea.Cmd {
	return textinput.Blink
}

func (h *HomeScreen) Update(msg tea.Msg) (Screen, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		h.input.Width = msg.Width - len(h.input.Prompt) - 2
	case tea.KeyMsg:
		switch {
		case matchesKey(msg, h.cfg.Keys.Open):
			value := strings.TrimSpace(h.input.Value())
			if owner, repo, ok := splitOwnerRepo(value); ok {
				h.notice = ""
				return h, pushScreen(NewRepoScreen(h.cfg, h.theme, h.client, h.readmeStylePath, owner, repo))
			}
			if value != "" {
				h.notice = "search comes in M4"
			}
			return h, nil
		case matchesKey(msg, h.cfg.Keys.Back):
			if h.input.Value() == "" {
				return h, tea.Quit
			}
			h.input.SetValue("")
			h.notice = ""
			return h, nil
		case matchesKey(msg, h.cfg.Keys.Quit) && h.input.Value() == "":
			return h, tea.Quit
		}
	}

	var cmd tea.Cmd
	h.input, cmd = h.input.Update(msg)
	return h, cmd
}

func (h *HomeScreen) View(width, height int) string {
	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(h.theme.AccentAltText.Render("ghab") + h.theme.DimText.Render(" -- GitHub, from the terminal"))
	b.WriteString("\n\n")
	b.WriteString(h.input.View())
	b.WriteString("\n")
	if h.notice != "" {
		b.WriteString("\n")
		b.WriteString(h.theme.MutedText.Render(h.notice))
	}
	return b.String()
}

func (h *HomeScreen) Footer() []style.KeyHint {
	hints := []style.KeyHint{
		{Keys: h.cfg.Keys.Open, Label: "go"},
	}
	if h.input.Value() == "" {
		hints = append(hints, style.KeyHint{Keys: h.cfg.Keys.Quit, Label: "quit"})
	} else {
		hints = append(hints, style.KeyHint{Keys: h.cfg.Keys.Back, Label: "clear"})
	}
	return hints
}

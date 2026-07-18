package ui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/Sh-ui/ghab/internal/config"
	"github.com/Sh-ui/ghab/internal/gh"
	"github.com/Sh-ui/ghab/internal/ui/style"
)

// HomeScreen is the app's landing screen: one text input with a small
// grammar (BUILD.md's Home spec) -- "owner/repo" jumps into the repo,
// a bare "owner" opens their profile, and anything with whitespace or a
// leading "?" runs a repo search. Recent visits this session are listed
// below the input (display-only; persistence is a non-goal for genesis).
type HomeScreen struct {
	cfg    config.Config
	theme  style.Theme
	client *gh.Client

	// readmeStylePath is threaded into every Repo screen this screen
	// pushes -- resolved once at app startup (main.go), not re-resolved
	// per repo.
	readmeStylePath string

	input   textinput.Model
	recents []string
}

// NewHomeScreen builds the Home screen.
func NewHomeScreen(cfg config.Config, theme style.Theme, client *gh.Client, readmeStylePath string) *HomeScreen {
	ti := textinput.New()
	ti.Placeholder = "owner/repo, user, or ?query"
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
			switch {
			case value == "":
				return h, nil
			case strings.HasPrefix(value, "?"):
				query := strings.TrimSpace(strings.TrimPrefix(value, "?"))
				if query == "" {
					return h, nil
				}
				return h, pushScreen(NewSearchScreen(h.cfg, h.theme, h.client, h.readmeStylePath, query))
			case strings.ContainsAny(value, " \t"):
				return h, pushScreen(NewSearchScreen(h.cfg, h.theme, h.client, h.readmeStylePath, value))
			default:
				if owner, repo, ok := splitOwnerRepo(value); ok {
					return h, pushScreen(NewRepoScreen(h.cfg, h.theme, h.client, h.readmeStylePath, owner, repo))
				}
				if strings.Contains(value, "/") {
					return h, nil // malformed owner/repo -- ignore rather than guess
				}
				return h, pushScreen(NewProfileScreen(h.cfg, h.theme, h.client, h.readmeStylePath, value))
			}
		case matchesKey(msg, h.cfg.Keys.Back):
			if h.input.Value() == "" {
				return h, tea.Quit
			}
			h.input.SetValue("")
			return h, nil
		case matchesKey(msg, h.cfg.Keys.Quit) && h.input.Value() == "":
			return h, tea.Quit
		case matchesKey(msg, h.cfg.Keys.MyPRs):
			// Default binding is a chord (ctrl+p), not a bare letter --
			// deliberately, so it can fire regardless of what's already
			// typed (or being typed) into the input without ever eating a
			// character of real text entry. A bare-letter default ("p")
			// was tried first and rejected: gated to an empty input, it
			// still stole the very first keystroke of any owner/repo or
			// search value starting with that letter (pigeon, piper,
			// prusa3d, pytorch, ...), and textinput only inserts on
			// printable-rune key events, so a chord like this never
			// reaches it as text no matter the input's contents.
			return h, pushScreen(NewMyPRsScreen(h.cfg, h.theme, h.client, h.readmeStylePath))
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
	if len(h.recents) > 0 {
		b.WriteString("\n")
		b.WriteString(h.theme.DimText.Render("recent"))
		b.WriteString("\n")
		for _, r := range h.recents {
			b.WriteString(h.theme.MutedText.Render("  " + r))
			b.WriteString("\n")
		}
	}
	return b.String()
}

// recordVisit prepends fullName ("owner/repo") to the session's
// recent-visits list, deduplicated, capped at 8. Called by the root App
// whenever a Repo screen is pushed (from any screen, not just Home).
func (h *HomeScreen) recordVisit(fullName string) {
	recents := []string{fullName}
	for _, r := range h.recents {
		if r != fullName && len(recents) < 8 {
			recents = append(recents, r)
		}
	}
	h.recents = recents
}

func (h *HomeScreen) Footer() []style.KeyHint {
	hints := []style.KeyHint{
		{Keys: h.cfg.Keys.Open, Label: "go"},
		// my_prs is a chord by default (ctrl+p), not a bare letter, so
		// unlike quit it works no matter what's typed into the input --
		// its hint isn't gated on an empty value the way quit's is.
		{Keys: h.cfg.Keys.MyPRs, Label: "my PRs"},
	}
	if h.input.Value() == "" {
		hints = append(hints, style.KeyHint{Keys: h.cfg.Keys.Quit, Label: "quit"})
	} else {
		hints = append(hints, style.KeyHint{Keys: h.cfg.Keys.Back, Label: "clear"})
	}
	return hints
}

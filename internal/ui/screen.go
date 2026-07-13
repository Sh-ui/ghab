package ui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/Sh-ui/ghab/internal/ui/style"
)

// Screen is one entry in the app's navigation stack. Each screen owns
// its own key handling (including when to quit or emit a pop), since
// screens with a focused text input need to swallow most keystrokes.
type Screen interface {
	Init() tea.Cmd
	Update(msg tea.Msg) (Screen, tea.Cmd)
	View(width, height int) string
	// Footer returns this screen's contextual keybind hints, built from
	// the live keymap -- never hardcoded key labels.
	Footer() []style.KeyHint
}

// pushScreenMsg asks the root App to push a new screen onto the stack
// and initialize it. Screens can't mutate the stack directly (Update
// only returns themselves), so navigation goes through this message.
type pushScreenMsg struct {
	screen Screen
}

// popScreenMsg asks the root App to pop the current screen, returning to
// whatever is beneath it.
type popScreenMsg struct{}

// pushScreen returns a Cmd that navigates to screen.
func pushScreen(screen Screen) tea.Cmd {
	return func() tea.Msg { return pushScreenMsg{screen: screen} }
}

// popScreen returns a Cmd that pops back to the previous screen.
func popScreen() tea.Cmd {
	return func() tea.Msg { return popScreenMsg{} }
}

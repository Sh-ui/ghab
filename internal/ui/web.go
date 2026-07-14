package ui

import (
	"os/exec"
	"runtime"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Sh-ui/ghab/internal/config"
)

// webDoneMsg carries the result of an open-on-web invocation. Errors show
// inline; success needs no message.
type webDoneMsg struct {
	err error
}

// resolveOpenURLCommand turns [behavior].open_url into an argv prefix.
// "auto" prefers wend (the Pi's browser router) when it's on PATH, then
// falls back to the platform opener (open / xdg-open). Any other value is
// used verbatim (split on whitespace), with the URL appended.
func resolveOpenURLCommand(setting string) []string {
	if setting == "" || setting == "auto" {
		if _, err := exec.LookPath("wend"); err == nil {
			return []string{"wend"}
		}
		if runtime.GOOS == "darwin" {
			return []string{"open"}
		}
		return []string{"xdg-open"}
	}
	return strings.Fields(setting)
}

// openURLCmd opens url via the configured opener. tea.ExecProcess
// suspends the TUI for the duration, which is required when the opener
// is itself a terminal program (wend can route to w3m) and harmless for
// GUI openers, which return immediately.
func openURLCmd(cfg config.Config, url string) tea.Cmd {
	argv := append(resolveOpenURLCommand(cfg.Behavior.OpenURL), url)
	return tea.ExecProcess(exec.Command(argv[0], argv[1:]...), func(err error) tea.Msg {
		return webDoneMsg{err: err}
	})
}

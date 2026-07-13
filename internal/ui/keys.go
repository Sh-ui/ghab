package ui

import tea "github.com/charmbracelet/bubbletea"

// matchesKey reports whether msg is the single key bound to keyStr in
// the live (config-merged) keymap. Every key comparison in ui goes
// through this instead of a hardcoded literal.
func matchesKey(msg tea.KeyMsg, keyStr string) bool {
	return keyStr != "" && msg.String() == keyStr
}

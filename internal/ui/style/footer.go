package style

import "strings"

// KeyHint is one entry in a footer keybind-hint line: the key(s) and
// what they do, e.g. {"j/k", "move"}.
type KeyHint struct {
	Keys  string
	Label string
}

// Footer renders the slate keybind-hint line every screen shows at the
// bottom, e.g. "j/k move  l/h tab  enter open  ? help  q quit". Hints
// come from the live (config-merged) keymap, never hardcoded key labels.
func Footer(th Theme, hints []KeyHint) string {
	parts := make([]string, 0, len(hints))
	for _, h := range hints {
		if h.Keys == "" || h.Label == "" {
			continue
		}
		parts = append(parts, h.Keys+" "+h.Label)
	}
	return th.Footer.Render(strings.Join(parts, "  "))
}

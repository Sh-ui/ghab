// Package style builds every lipgloss style ghab uses from the resolved
// config theme. No hex literals live here (or anywhere else in ui code)
// -- colors always come through config.ResolvedTheme or the fixed ANSI
// neutral slots below. Square borders only, per docs/desktop/tui-visual-style.md.
package style

import (
	"github.com/alecthomas/chroma/v2"
	"github.com/charmbracelet/lipgloss"

	"github.com/Sh-ui/ghab/internal/config"
)

// Fixed ANSI neutral slots (house rule, not user-configurable -- see
// tui-visual-style.md "Neutrals in a bespoke in-terminal TUI must follow
// the mode"). Background is left unset everywhere so it falls through to
// the terminal default in both light and dark modes.
const (
	ansiFg    = "15" // full foreground (body text)
	ansiDim   = "7"  // dim text
	ansiMuted = "8"  // slate: hints, placeholders, frame default
)

// Theme is every lipgloss primitive ghab draws with, built once from the
// resolved config theme at startup.
type Theme struct {
	// Colors, exposed for callers that need to compose their own styles
	// (e.g. the framed-box title bar).
	Accent       lipgloss.Color
	AccentAlt    lipgloss.Color
	OpenMarker   lipgloss.Color
	ClosedMarker lipgloss.Color
	ReleaseTag   lipgloss.Color
	Frame        lipgloss.Color
	SelectionBg  lipgloss.Color
	Fg           lipgloss.Color
	Dim          lipgloss.Color
	Muted        lipgloss.Color

	// Ready-made styles for common roles.
	Body          lipgloss.Style
	DimText       lipgloss.Style
	MutedText     lipgloss.Style
	AccentText    lipgloss.Style
	AccentAltText lipgloss.Style
	OpenText      lipgloss.Style
	ClosedText    lipgloss.Style
	ReleaseText   lipgloss.Style
	Footer        lipgloss.Style
	TabActive     lipgloss.Style
	TabInactive   lipgloss.Style
	Selection     lipgloss.Style
	Spinner       lipgloss.Style
	ErrorText     lipgloss.Style

	// Chroma is the code tab's syntax-highlight style, built from the same
	// resolved ombre palette (see ChromaStyle) -- used by both color modes
	// since accents are luma-balanced for both grounds.
	Chroma *chroma.Style
}

// New builds a Theme from a resolved config theme and the resolved ombre
// palette (for ChromaStyle -- code-preview syntax colors need palette names
// like "purple"/"blue" that ResolvedTheme doesn't carry).
func New(t config.ResolvedTheme, palette config.Palette) Theme {
	th := Theme{
		Accent:       lipgloss.Color(t.Accent),
		AccentAlt:    lipgloss.Color(t.AccentAlt),
		OpenMarker:   lipgloss.Color(t.OpenMarker),
		ClosedMarker: lipgloss.Color(t.ClosedMarker),
		ReleaseTag:   lipgloss.Color(t.ReleaseTag),
		Frame:        lipgloss.Color(t.Frame),
		SelectionBg:  lipgloss.Color(t.SelectionBg),
		Fg:           lipgloss.Color(ansiFg),
		Dim:          lipgloss.Color(ansiDim),
		Muted:        lipgloss.Color(ansiMuted),
	}

	th.Body = lipgloss.NewStyle().Foreground(th.Fg)
	th.DimText = lipgloss.NewStyle().Foreground(th.Dim)
	th.MutedText = lipgloss.NewStyle().Foreground(th.Muted)
	th.AccentText = lipgloss.NewStyle().Foreground(th.Accent)
	th.AccentAltText = lipgloss.NewStyle().Foreground(th.AccentAlt)
	th.OpenText = lipgloss.NewStyle().Foreground(th.OpenMarker)
	th.ClosedText = lipgloss.NewStyle().Foreground(th.ClosedMarker)
	th.ReleaseText = lipgloss.NewStyle().Foreground(th.ReleaseTag)
	th.Footer = lipgloss.NewStyle().Foreground(th.Muted)
	th.TabActive = lipgloss.NewStyle().Foreground(th.Accent).Underline(true).Bold(true)
	th.TabInactive = lipgloss.NewStyle().Foreground(th.Dim)
	th.Selection = lipgloss.NewStyle().Background(th.SelectionBg).Foreground(th.Fg)
	th.Spinner = lipgloss.NewStyle().Foreground(th.Accent)
	th.ErrorText = lipgloss.NewStyle().Foreground(th.ClosedMarker)

	th.Chroma = ChromaStyle(palette)

	return th
}

// Border is the one border style ghab ever draws: square corners, no
// exceptions (rounded corners are the "modern app" tell the house style
// explicitly avoids).
func Border() lipgloss.Border {
	return lipgloss.NormalBorder()
}

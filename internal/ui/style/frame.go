package style

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Frame draws a square-cornered box with the title riding the top border
// (`┌─ ghab ─┐`), per tui-visual-style.md. Body lines are left-aligned
// and padded/truncated to fit; width is the box's total outer width
// (including the border columns). The border is drawn in th.Frame; use
// FrameColor for a pane whose border color changes (e.g. with focus).
func Frame(th Theme, title string, bodyLines []string, width int) string {
	return FrameColor(th, title, bodyLines, width, th.Frame)
}

// FrameColor is Frame with an explicit border color, for panes whose frame
// changes with state -- e.g. the code tab's tree/preview split, where the
// focused pane's frame uses th.Accent and the unfocused one uses th.Frame.
func FrameColor(th Theme, title string, bodyLines []string, width int, borderColor lipgloss.Color) string {
	b := Border()
	frameStyle := lipgloss.NewStyle().Foreground(borderColor)
	innerWidth := width - 2
	if innerWidth < 0 {
		innerWidth = 0
	}

	var out []string
	out = append(out, titledTopBorder(th, b, title, innerWidth, borderColor))
	for _, line := range bodyLines {
		content := lipgloss.NewStyle().Width(innerWidth).MaxWidth(innerWidth).Render(line)
		out = append(out, frameStyle.Render(b.Left)+content+frameStyle.Render(b.Right))
	}
	out = append(out, frameStyle.Render(b.BottomLeft+strings.Repeat(b.Bottom, innerWidth)+b.BottomRight))

	return strings.Join(out, "\n")
}

func titledTopBorder(th Theme, b lipgloss.Border, title string, innerWidth int, borderColor lipgloss.Color) string {
	frameStyle := lipgloss.NewStyle().Foreground(borderColor)
	if title == "" {
		return frameStyle.Render(b.TopLeft + strings.Repeat(b.Top, innerWidth) + b.TopRight)
	}

	label := " " + title + " "
	labelWidth := lipgloss.Width(label)
	if labelWidth+2 > innerWidth {
		// Not enough room for the title; fall back to a bare rule
		// rather than overflowing the frame.
		return frameStyle.Render(b.TopLeft + strings.Repeat(b.Top, innerWidth) + b.TopRight)
	}

	leftRule := 1
	rightRule := innerWidth - labelWidth - leftRule
	titleStyled := th.Body.Render(label)

	return frameStyle.Render(b.TopLeft+strings.Repeat(b.Top, leftRule)) +
		titleStyled +
		frameStyle.Render(strings.Repeat(b.Top, rightRule)+b.TopRight)
}

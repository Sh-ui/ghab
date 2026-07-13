package ui

import (
	"fmt"
	"time"
)

// relativeAge formats t as a short relative age string ("3d ago", "2mo
// ago", "just now") -- shared by the releases and issues/PRs sub-models
// for release-publish, issue/PR-open, and comment timestamps (BUILD.md:
// "author + relative age muted"). Callers apply MutedText themselves;
// this returns plain text.
func relativeAge(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	d := time.Since(t)
	if d < 0 {
		d = 0
	}
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d/time.Minute))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d/time.Hour))
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d/(24*time.Hour)))
	case d < 365*24*time.Hour:
		return fmt.Sprintf("%dmo ago", int(d/(30*24*time.Hour)))
	default:
		return fmt.Sprintf("%dy ago", int(d/(365*24*time.Hour)))
	}
}

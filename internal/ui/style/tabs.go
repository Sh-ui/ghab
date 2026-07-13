package style

import "strings"

// TabBar renders the "readme | code | releases | issues | prs" bar with
// the active tab picked out by an accent underline; l/h cycles between
// them (see ui.RepoScreen).
func TabBar(th Theme, labels []string, active int) string {
	parts := make([]string, len(labels))
	for i, label := range labels {
		if i == active {
			parts[i] = th.TabActive.Render(label)
		} else {
			parts[i] = th.TabInactive.Render(label)
		}
	}
	sep := th.MutedText.Render(" | ")
	return strings.Join(parts, sep)
}

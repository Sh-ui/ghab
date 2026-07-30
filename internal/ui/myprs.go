package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/Sh-ui/ghab/internal/config"
	"github.com/Sh-ui/ghab/internal/gh"
	"github.com/Sh-ui/ghab/internal/ui/style"
)

// myPRsResultMsg carries the async result of the my-PRs search fetch.
type myPRsResultMsg struct {
	total int
	items []gh.SearchIssue
	err   error
}

// MyPRsScreen is the cross-repo "my PRs" screen (issue #11): every open
// PR authored by or requesting review from the signed-in user, across
// every repo, without leaving the terminal. Fired once when pushed --
// same hard search-endpoint debounce as SearchScreen, never per
// keystroke. Row grammar: state bullet, repo, #num, title, age.
type MyPRsScreen struct {
	cfg    config.Config
	theme  style.Theme
	client *gh.Client

	readmeStylePath string
	queries         []string
	perPage         int

	loading bool
	err     error
	total   int
	spinner spinner.Model

	items     []gh.SearchIssue
	cursor    int
	scrollOff int
	rows      int
	width     int

	height int
}

// NewMyPRsScreen builds the my-PRs screen. queries is
// [behavior].my_prs_query (config-first: Ian narrows or widens the scope
// without a rebuild) -- several queries because GitHub search has no OR
// between qualifiers, merged and de-duplicated by gh.MyPRs.
func NewMyPRsScreen(cfg config.Config, theme style.Theme, client *gh.Client, readmeStylePath string) *MyPRsScreen {
	sp := spinner.New(spinner.WithSpinner(spinner.Dot), spinner.WithStyle(theme.Spinner))
	return &MyPRsScreen{
		cfg:             cfg,
		theme:           theme,
		client:          client,
		readmeStylePath: readmeStylePath,
		queries:         cfg.Behavior.MyPRsQueries,
		perPage:         cfg.Behavior.PageSize,
		loading:         true,
		spinner:         sp,
	}
}

func (s *MyPRsScreen) fetchCmd() tea.Cmd {
	queries, perPage, client := s.queries, s.perPage, s.client
	return func() tea.Msg {
		total, items, err := client.MyPRs(queries, perPage)
		return myPRsResultMsg{total: total, items: items, err: err}
	}
}

func (s *MyPRsScreen) Init() tea.Cmd {
	return tea.Batch(s.spinner.Tick, s.fetchCmd())
}

func (s *MyPRsScreen) Update(msg tea.Msg) (Screen, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		s.width, s.height = msg.Width, msg.Height
		s.rows = s.height - 3 // header line + blank + status line
		if s.rows < 1 {
			s.rows = 1
		}
		s.clampCursor()
		return s, nil

	case myPRsResultMsg:
		s.loading = false
		s.err = msg.err
		if msg.err == nil {
			s.total = msg.total
			s.items = msg.items
			s.cursor = 0
			s.scrollOff = 0
		}
		return s, nil

	case spinner.TickMsg:
		if !s.loading {
			return s, nil
		}
		var cmd tea.Cmd
		s.spinner, cmd = s.spinner.Update(msg)
		return s, cmd

	case tea.KeyMsg:
		switch {
		case matchesKey(msg, s.cfg.Keys.Quit):
			return s, tea.Quit
		case matchesKey(msg, s.cfg.Keys.Back):
			return s, popScreen()
		case matchesKey(msg, s.cfg.Keys.Down):
			s.moveCursor(1)
			return s, nil
		case matchesKey(msg, s.cfg.Keys.Up):
			s.moveCursor(-1)
			return s, nil
		case matchesKey(msg, s.cfg.Keys.Open):
			if item := s.selected(); item != nil {
				if owner, repo, ok := splitOwnerRepo(item.RepoFullName()); ok {
					return s, pushScreen(NewPRDetailScreen(s.cfg, s.theme, s.client, s.readmeStylePath, owner, repo, item.Number))
				}
			}
			return s, nil
		case matchesKey(msg, s.cfg.Keys.Refresh):
			s.client.RefreshMyPRs(s.queries, s.perPage)
			s.loading = true
			s.err = nil
			return s, tea.Batch(s.spinner.Tick, s.fetchCmd())
		}
	}
	return s, nil
}

func (s *MyPRsScreen) moveCursor(delta int) {
	if len(s.items) == 0 {
		return
	}
	s.cursor += delta
	s.clampCursor()
}

func (s *MyPRsScreen) clampCursor() {
	if s.cursor < 0 {
		s.cursor = 0
	}
	if s.cursor >= len(s.items) {
		s.cursor = len(s.items) - 1
	}
	if s.cursor < 0 {
		s.cursor = 0
	}
	if s.rows <= 0 {
		return
	}
	if s.cursor < s.scrollOff {
		s.scrollOff = s.cursor
	}
	if s.cursor >= s.scrollOff+s.rows {
		s.scrollOff = s.cursor - s.rows + 1
	}
	if s.scrollOff < 0 {
		s.scrollOff = 0
	}
}

func (s *MyPRsScreen) selected() *gh.SearchIssue {
	if len(s.items) == 0 || s.cursor >= len(s.items) {
		return nil
	}
	return &s.items[s.cursor]
}

func (s *MyPRsScreen) View(width, height int) string {
	if s.loading {
		return s.spinner.View() + " loading your PRs..."
	}
	if s.err != nil {
		return s.theme.ErrorText.Render("error: "+s.err.Error()) + "\n" +
			s.theme.MutedText.Render("press "+s.cfg.Keys.Refresh+" to retry")
	}

	var b strings.Builder
	header := s.theme.AccentAltText.Render("my PRs") + "  " +
		s.theme.MutedText.Render(fmt.Sprintf("%d open (%s)", s.total, s.queryLabel()))
	b.WriteString(header)
	b.WriteString("\n\n")

	if len(s.items) == 0 {
		b.WriteString(s.theme.MutedText.Render("no open PRs match " + s.queryLabel()))
		return b.String()
	}
	b.WriteString(strings.Join(s.listBodyLines(), "\n"))
	return b.String()
}

// queryLabel renders the configured query set for the header/empty-state
// line: one query prints as itself, several join with " | " so the scope
// on screen is the scope actually fetched.
func (s *MyPRsScreen) queryLabel() string {
	return strings.Join(s.queries, " | ")
}

func (s *MyPRsScreen) listBodyLines() []string {
	rows := make([]string, 0, s.rows)
	for i := 0; i < s.rows; i++ {
		idx := s.scrollOff + i
		if idx >= len(s.items) {
			break
		}
		rows = append(rows, s.renderRow(s.items[idx], idx == s.cursor))
	}
	return rows
}

// renderRow builds one row: state bullet, repo, #num, title, age --
// mirrors issuesTab.renderListRow's row grammar, plus the repo column a
// cross-repo list needs.
func (s *MyPRsScreen) renderRow(item gh.SearchIssue, selected bool) string {
	repo := item.RepoFullName()
	plain := fmt.Sprintf("● %s  #%d  %s  %s", repo, item.Number, item.Title, relativeAge(item.UpdatedAt))

	width := s.width
	if width < 1 {
		width = lipgloss.Width(plain)
	}

	if selected {
		return lipgloss.NewStyle().Width(width).MaxWidth(width).
			Background(s.theme.SelectionBg).Foreground(s.theme.Fg).Render(plain)
	}

	bulletStyle := s.theme.OpenText
	if item.State != "open" {
		bulletStyle = s.theme.ClosedText
	}
	styled := bulletStyle.Render("●") + " " +
		s.theme.AccentAltText.Render(repo) + "  " +
		s.theme.MutedText.Render(fmt.Sprintf("#%d", item.Number)) + "  " +
		s.theme.Body.Render(item.Title) + "  " +
		s.theme.MutedText.Render(relativeAge(item.UpdatedAt))
	return lipgloss.NewStyle().Width(width).MaxWidth(width).Render(styled)
}

func (s *MyPRsScreen) Footer() []style.KeyHint {
	return []style.KeyHint{
		{Keys: s.cfg.Keys.Down + "/" + s.cfg.Keys.Up, Label: "move"},
		{Keys: s.cfg.Keys.Open, Label: "open"},
		{Keys: s.cfg.Keys.Refresh, Label: "refresh"},
		{Keys: s.cfg.Keys.Back, Label: "back"},
		{Keys: s.cfg.Keys.Quit, Label: "quit"},
	}
}

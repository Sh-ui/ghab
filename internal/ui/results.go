package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/Sh-ui/ghab/internal/config"
	"github.com/Sh-ui/ghab/internal/gh"
	"github.com/Sh-ui/ghab/internal/ui/style"
)

// searchResultsMsg carries the async result of a repo search.
type searchResultsMsg struct {
	total int
	items []gh.UserRepo
	err   error
}

// SearchScreen shows repo search results for one query, fired once when
// the screen is pushed (never per keystroke -- the search rate bucket is
// 30 req/min, see gh.SearchRepos). Enter pushes the selected repo.
type SearchScreen struct {
	cfg    config.Config
	theme  style.Theme
	client *gh.Client

	readmeStylePath string
	query           string

	loading bool
	err     error
	total   int
	spinner spinner.Model

	list *repoList

	width, height int
}

// NewSearchScreen builds the search-results screen for query.
func NewSearchScreen(cfg config.Config, theme style.Theme, client *gh.Client, readmeStylePath, query string) *SearchScreen {
	sp := spinner.New(spinner.WithSpinner(spinner.Dot), spinner.WithStyle(theme.Spinner))
	return &SearchScreen{
		cfg:             cfg,
		theme:           theme,
		client:          client,
		readmeStylePath: readmeStylePath,
		query:           query,
		loading:         true,
		spinner:         sp,
		list:            newRepoList(theme),
	}
}

func (s *SearchScreen) fetchCmd() tea.Cmd {
	query, perPage, client := s.query, s.cfg.Behavior.PageSize, s.client
	return func() tea.Msg {
		total, items, err := client.SearchRepos(query, perPage)
		return searchResultsMsg{total: total, items: items, err: err}
	}
}

func (s *SearchScreen) Init() tea.Cmd {
	return tea.Batch(s.spinner.Tick, s.fetchCmd())
}

func (s *SearchScreen) Update(msg tea.Msg) (Screen, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		s.width, s.height = msg.Width, msg.Height
		s.list.resize(s.width, s.height-2) // header line + blank line
		return s, nil

	case searchResultsMsg:
		s.loading = false
		s.err = msg.err
		if msg.err == nil {
			s.total = msg.total
			s.list.setItems(searchItems(msg.items))
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
			s.list.move(1)
			return s, nil
		case matchesKey(msg, s.cfg.Keys.Up):
			s.list.move(-1)
			return s, nil
		case matchesKey(msg, s.cfg.Keys.Open):
			if r := s.list.selected(); r != nil {
				if owner, repo, ok := splitOwnerRepo(r.FullName); ok {
					return s, pushScreen(NewRepoScreen(s.cfg, s.theme, s.client, s.readmeStylePath, owner, repo))
				}
			}
			return s, nil
		}
	}
	return s, nil
}

func (s *SearchScreen) View(width, height int) string {
	if s.loading {
		return s.spinner.View() + " searching \"" + s.query + "\"..."
	}
	if s.err != nil {
		return s.theme.ErrorText.Render("error: " + s.err.Error())
	}

	var b strings.Builder
	header := s.theme.AccentAltText.Render("search: "+s.query) + "  " +
		s.theme.MutedText.Render(fmt.Sprintf("%d results", s.total))
	b.WriteString(header)
	b.WriteString("\n\n")
	b.WriteString(s.list.View())
	return b.String()
}

func (s *SearchScreen) Footer() []style.KeyHint {
	return []style.KeyHint{
		{Keys: s.cfg.Keys.Down + "/" + s.cfg.Keys.Up, Label: "move"},
		{Keys: s.cfg.Keys.Open, Label: "open repo"},
		{Keys: s.cfg.Keys.Back, Label: "back"},
		{Keys: s.cfg.Keys.Quit, Label: "quit"},
	}
}

// repoList rows show a bare Name; search results span owners, so swap in
// the FullName before handing items over.
func searchItems(items []gh.UserRepo) []gh.UserRepo {
	out := make([]gh.UserRepo, len(items))
	copy(out, items)
	for i := range out {
		out[i].Name = out[i].FullName
	}
	return out
}

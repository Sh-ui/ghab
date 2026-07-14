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

// profileUserMsg / profileReposMsg carry the async results of the two
// profile fetches (user card, repo list) -- fired in parallel from Init.
type profileUserMsg struct {
	user gh.User
	err  error
}

type profileReposMsg struct {
	repos []gh.UserRepo
	err   error
}

// ProfileScreen is the user-profile hop: a framed user card (name, bio,
// counts) over the user's public repos sorted by most recently updated.
// Enter on a repo pushes its Repo screen -- and that repo's "u" pushes
// its owner's profile, so the hop chains both directions through the
// screen stack (BUILD.md: "THIS IS THE HOP -- must be seamless").
type ProfileScreen struct {
	cfg    config.Config
	theme  style.Theme
	client *gh.Client

	readmeStylePath string
	login           string

	loadingUser  bool
	loadingRepos bool
	userErr      error
	reposErr     error
	user         gh.User
	spinner      spinner.Model

	list *repoList

	width, height int
}

// NewProfileScreen builds the profile screen for login. Fetching starts
// in Init.
func NewProfileScreen(cfg config.Config, theme style.Theme, client *gh.Client, readmeStylePath, login string) *ProfileScreen {
	sp := spinner.New(spinner.WithSpinner(spinner.Dot), spinner.WithStyle(theme.Spinner))
	return &ProfileScreen{
		cfg:             cfg,
		theme:           theme,
		client:          client,
		readmeStylePath: readmeStylePath,
		login:           login,
		loadingUser:     true,
		loadingRepos:    true,
		spinner:         sp,
		list:            newRepoList(theme),
	}
}

func (p *ProfileScreen) fetchCmds() tea.Cmd {
	login, client := p.login, p.client
	return tea.Batch(
		func() tea.Msg {
			u, err := client.User(login)
			return profileUserMsg{user: u, err: err}
		},
		func() tea.Msg {
			repos, err := client.UserRepos(login)
			return profileReposMsg{repos: repos, err: err}
		},
	)
}

func (p *ProfileScreen) Init() tea.Cmd {
	return tea.Batch(p.spinner.Tick, p.fetchCmds())
}

func (p *ProfileScreen) Update(msg tea.Msg) (Screen, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		p.width, p.height = msg.Width, msg.Height
		p.resizeList()
		return p, nil

	case profileUserMsg:
		p.loadingUser = false
		p.userErr = msg.err
		if msg.err == nil {
			p.user = msg.user
		}
		p.resizeList() // card height depends on which lines it has
		return p, nil

	case profileReposMsg:
		p.loadingRepos = false
		p.reposErr = msg.err
		if msg.err == nil {
			p.list.setItems(msg.repos)
		}
		return p, nil

	case spinner.TickMsg:
		if !p.loadingUser && !p.loadingRepos {
			return p, nil
		}
		var cmd tea.Cmd
		p.spinner, cmd = p.spinner.Update(msg)
		return p, cmd

	case tea.KeyMsg:
		switch {
		case matchesKey(msg, p.cfg.Keys.Quit):
			return p, tea.Quit
		case matchesKey(msg, p.cfg.Keys.Back):
			return p, popScreen()
		case matchesKey(msg, p.cfg.Keys.Down):
			p.list.move(1)
			return p, nil
		case matchesKey(msg, p.cfg.Keys.Up):
			p.list.move(-1)
			return p, nil
		case matchesKey(msg, p.cfg.Keys.Open):
			if r := p.list.selected(); r != nil {
				if owner, repo, ok := splitOwnerRepo(r.FullName); ok {
					return p, pushScreen(NewRepoScreen(p.cfg, p.theme, p.client, p.readmeStylePath, owner, repo))
				}
			}
			return p, nil
		case matchesKey(msg, p.cfg.Keys.Web):
			if p.user.HTMLURL != "" {
				return p, openURLCmd(p.cfg, p.user.HTMLURL)
			}
			return p, nil
		case matchesKey(msg, p.cfg.Keys.Refresh):
			p.client.RefreshUser(p.login)
			p.loadingUser = true
			p.loadingRepos = true
			p.userErr = nil
			p.reposErr = nil
			return p, tea.Batch(p.spinner.Tick, p.fetchCmds())
		}
	}
	return p, nil
}

// cardLines builds the user card's body lines -- only the ones that have
// content, so the frame stays tight.
func (p *ProfileScreen) cardLines() []string {
	var lines []string

	title := p.user.Name
	if title == "" {
		title = p.user.Login
	}
	lines = append(lines, p.theme.Body.Render(title))

	if p.user.Bio != "" {
		lines = append(lines, p.theme.MutedText.Render(p.user.Bio))
	}

	stats := fmt.Sprintf("repos: %d   followers: %d   following: %d",
		p.user.PublicRepos, p.user.Followers, p.user.Following)
	lines = append(lines, p.theme.DimText.Render(stats))

	var where []string
	if p.user.Location != "" {
		where = append(where, p.user.Location)
	}
	if p.user.Company != "" {
		where = append(where, p.user.Company)
	}
	if p.user.Blog != "" {
		where = append(where, p.user.Blog)
	}
	if len(where) > 0 {
		lines = append(lines, p.theme.MutedText.Render(strings.Join(where, "   ")))
	}

	return lines
}

// cardHeight is the rendered card's line count: body lines + 2 border rows.
func (p *ProfileScreen) cardHeight() int {
	return len(p.cardLines()) + 2
}

func (p *ProfileScreen) resizeList() {
	// card + one blank line between card and list
	rows := p.height - p.cardHeight() - 1
	p.list.resize(p.width, rows)
}

func (p *ProfileScreen) View(width, height int) string {
	if p.loadingUser {
		return p.spinner.View() + " fetching @" + p.login + "..."
	}
	if p.userErr != nil {
		return p.theme.ErrorText.Render("error: "+p.userErr.Error()) + "\n" +
			p.theme.MutedText.Render("press "+p.cfg.Keys.Refresh+" to retry")
	}

	var b strings.Builder
	b.WriteString(style.Frame(p.theme, "@"+p.user.Login, p.cardLines(), width))
	b.WriteString("\n")

	switch {
	case p.loadingRepos:
		b.WriteString(p.spinner.View() + " loading repos...")
	case p.reposErr != nil:
		b.WriteString(p.theme.ErrorText.Render("error loading repos: " + p.reposErr.Error()))
	default:
		b.WriteString(p.list.View())
	}
	return b.String()
}

func (p *ProfileScreen) Footer() []style.KeyHint {
	return []style.KeyHint{
		{Keys: p.cfg.Keys.Down + "/" + p.cfg.Keys.Up, Label: "move"},
		{Keys: p.cfg.Keys.Open, Label: "open repo"},
		{Keys: p.cfg.Keys.Web, Label: "web"},
		{Keys: p.cfg.Keys.Refresh, Label: "refresh"},
		{Keys: p.cfg.Keys.Back, Label: "back"},
		{Keys: p.cfg.Keys.Quit, Label: "quit"},
	}
}

package ui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Sh-ui/ghab/internal/config"
	"github.com/Sh-ui/ghab/internal/gh"
	"github.com/Sh-ui/ghab/internal/ui/style"
)

// PRDetailScreen is the top-level PR-detail hop pushed from MyPRsScreen
// (issue #11's cross-repo entry point): a thin Screen adapter around
// prDetailModel, which does all the actual work and is reused as-is by
// the per-repo PRs tab (issuesTab, kind == issueKindPR) -- see
// prdetail.go's struct comment.
type PRDetailScreen struct {
	cfg   config.Config
	model *prDetailModel

	width, height int
}

// NewPRDetailScreen builds a PR detail screen for owner/repo#number.
func NewPRDetailScreen(cfg config.Config, theme style.Theme, client *gh.Client, readmeStylePath, owner, repo string, number int) *PRDetailScreen {
	styleOpt := readmeStyleOption{stylePath: readmeStylePath}
	return &PRDetailScreen{
		cfg:   cfg,
		model: newPRDetailModel(cfg, theme, client, styleOpt, owner, repo, number),
	}
}

func (s *PRDetailScreen) Init() tea.Cmd {
	return s.model.start()
}

func (s *PRDetailScreen) Update(msg tea.Msg) (Screen, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		s.width, s.height = msg.Width, msg.Height
		bodyRows := s.height - 2 // this screen's own "owner/repo #N" header + blank
		if bodyRows < 1 {
			bodyRows = 1
		}
		s.model.resize(s.width, bodyRows)
		return s, nil

	case tea.KeyMsg:
		switch {
		case matchesKey(msg, s.cfg.Keys.Quit):
			return s, tea.Quit
		case matchesKey(msg, s.cfg.Keys.Refresh):
			return s, s.model.refresh()
		}
		cmd, exit := s.model.handleKey(msg)
		if exit {
			return s, popScreen()
		}
		return s, cmd
	}

	return s, s.model.Update(msg)
}

func (s *PRDetailScreen) View(width, height int) string {
	return s.headerLine(width) + "\n\n" + s.model.View()
}

// headerLine identifies which repo/PR this screen is showing -- unlike
// the embedded-in-RepoScreen case, there's no repo header frame above
// this screen to supply that context, and it needs to stay visible across
// all three sub-views (conversation/files/diff), not just conversation.
// Truncated (not wrapped) to width, plain text first so the trailing
// style escape never gets cut mid-sequence.
func (s *PRDetailScreen) headerLine(width int) string {
	title := fmt.Sprintf("%s/%s #%d", s.model.owner, s.model.repo, s.model.number)
	if !s.model.convLoading && s.model.convErr == nil && s.model.conv.Issue.Title != "" {
		title += "  " + s.model.conv.Issue.Title
	}
	return s.model.theme.AccentAltText.Render(truncateTo(title, width))
}

func (s *PRDetailScreen) Footer() []style.KeyHint {
	hints := s.model.footerHints(s.cfg)
	return append(hints,
		style.KeyHint{Keys: s.cfg.Keys.Refresh, Label: "refresh"},
		style.KeyHint{Keys: s.cfg.Keys.Back, Label: "back"},
		style.KeyHint{Keys: s.cfg.Keys.Quit, Label: "quit"},
	)
}

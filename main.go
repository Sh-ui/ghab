// Command ghab is a terminal GitHub browser: readme / code / releases /
// issues / prs for any repo, plus user profile hops. See BUILD.md for
// the full spec; this file only does flag parsing and wiring.
package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Sh-ui/ghab/internal/config"
	"github.com/Sh-ui/ghab/internal/gh"
	"github.com/Sh-ui/ghab/internal/ui"
	"github.com/Sh-ui/ghab/internal/ui/style"
)

// version is bumped by hand per release; ghab has no build-info wiring
// yet (M5 territory).
const version = "0.1.0-m1"

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	var (
		showVersion bool
		checkConfig bool
	)
	fs := newFlagSet(&showVersion, &checkConfig)
	if err := fs.Parse(args); err != nil {
		return 2
	}

	if showVersion {
		fmt.Println("ghab " + version)
		return 0
	}

	path := config.ConfigPath()
	cfg, warnings, err := config.Load(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ghab: %v\n", err)
		return 1
	}
	for _, w := range warnings {
		fmt.Fprintf(os.Stderr, "ghab: config warning: %s\n", w)
	}

	paletteResult, paletteWarnings := config.ResolvePalette(cfg.Palette.File)
	for _, w := range paletteWarnings {
		fmt.Fprintf(os.Stderr, "ghab: config warning: %s\n", w)
	}

	resolvedTheme, themeWarnings := config.ResolveTheme(cfg.Theme, paletteResult.Colors)
	for _, w := range themeWarnings {
		fmt.Fprintf(os.Stderr, "ghab: config warning: %s\n", w)
	}

	if checkConfig {
		printCheckConfig(path, cfg, paletteResult, resolvedTheme, warnings, paletteWarnings, themeWarnings)
		return 0
	}

	theme := style.New(resolvedTheme)

	client, err := gh.NewClient(cfg.Behavior.CacheTTLDuration)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ghab: %v\n", err)
		return 1
	}

	jumpTo := ""
	if fs.NArg() > 0 {
		jumpTo = fs.Arg(0)
	}

	app := ui.NewApp(cfg, theme, client, jumpTo)
	p := tea.NewProgram(app, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "ghab: %v\n", err)
		return 1
	}
	return 0
}

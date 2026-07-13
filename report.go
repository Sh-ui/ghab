package main

import (
	"fmt"
	"os"

	"github.com/Sh-ui/ghab/internal/config"
)

// printCheckConfig prints the full resolved config, palette resolution,
// and every warning collected while loading it. Called for
// --check-config; also useful as a paste-in-an-issue diagnostic dump.
func printCheckConfig(path string, cfg config.Config, palette config.PaletteResult, theme config.ResolvedTheme,
	warnings, paletteWarnings, themeWarnings []config.Warning) {

	fmt.Printf("ghab config check\n")
	fmt.Printf("  file:    %s", path)
	if _, err := os.Stat(path); err != nil {
		fmt.Printf(" (not found; using compiled defaults)\n")
	} else {
		fmt.Printf("\n")
	}
	fmt.Printf("  palette: %s\n\n", palette.Source)

	fmt.Println("  [theme]")
	fmt.Printf("    accent        = %-14s -> %s\n", cfg.Theme.Accent, theme.Accent)
	fmt.Printf("    accent_alt    = %-14s -> %s\n", cfg.Theme.AccentAlt, theme.AccentAlt)
	fmt.Printf("    open_marker   = %-14s -> %s\n", cfg.Theme.OpenMarker, theme.OpenMarker)
	fmt.Printf("    closed_marker = %-14s -> %s\n", cfg.Theme.ClosedMarker, theme.ClosedMarker)
	fmt.Printf("    release_tag   = %-14s -> %s\n", cfg.Theme.ReleaseTag, theme.ReleaseTag)
	fmt.Printf("    frame         = %-14s -> %s\n", cfg.Theme.Frame, theme.Frame)
	fmt.Printf("    selection_bg  = %-14s -> %s\n\n", cfg.Theme.SelectionBg, theme.SelectionBg)

	fmt.Println("  [readme]")
	fmt.Printf("    style_dark  = %s\n", cfg.Readme.StyleDark)
	fmt.Printf("    style_light = %s\n\n", cfg.Readme.StyleLight)

	fmt.Println("  [behavior]")
	fmt.Printf("    clone_dir = %s\n", cfg.Behavior.CloneDir)
	fmt.Printf("    editor    = %s\n", cfg.Behavior.Editor)
	fmt.Printf("    open_url  = %s\n", cfg.Behavior.OpenURL)
	fmt.Printf("    page_size = %d\n", cfg.Behavior.PageSize)
	fmt.Printf("    cache_ttl = %s\n\n", cfg.Behavior.CacheTTL)

	fmt.Println("  [keys]")
	fmt.Printf("    quit=%s help=%s search=%s back=%s\n", cfg.Keys.Quit, cfg.Keys.Help, cfg.Keys.Search, cfg.Keys.Back)
	fmt.Printf("    tab_next=%s tab_prev=%s down=%s up=%s\n", cfg.Keys.TabNext, cfg.Keys.TabPrev, cfg.Keys.Down, cfg.Keys.Up)
	fmt.Printf("    open=%s profile=%s clone=%s edit=%s web=%s refresh=%s\n\n",
		cfg.Keys.Open, cfg.Keys.Profile, cfg.Keys.Clone, cfg.Keys.Edit, cfg.Keys.Web, cfg.Keys.Refresh)

	all := append(append(append([]config.Warning{}, warnings...), paletteWarnings...), themeWarnings...)
	if len(all) == 0 {
		fmt.Println("  warnings: (none)")
		return
	}
	fmt.Println("  warnings:")
	for _, w := range all {
		fmt.Printf("    - %s\n", w)
	}
}

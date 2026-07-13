package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Palette maps ombre palette names ("coffee", "teal", ...) to hex strings.
type Palette map[string]string

// CompiledFallbackPalette is the exact hex set from BUILD.md, used only
// when no palette JSON file can be found in the search path.
func CompiledFallbackPalette() Palette {
	return Palette{
		"cream":   "#F8F2E9",
		"red":     "#ED4B40",
		"pink":    "#D35D92",
		"magenta": "#C353CC",
		"purple":  "#9668E6",
		"blue":    "#1987E8",
		"teal":    "#00ACC1",
		"mint":    "#1BBAA0",
		"green":   "#43A047",
		"yellow":  "#BF9800",
		"orange":  "#C47623",
		"coffee":  "#AD774E",
		"slate":   "#797B86",
		"grey":    "#7E7E7E",
		"umbra":   "#1F1E1D",
	}
}

// PaletteResult is the outcome of resolving [palette].file.
type PaletteResult struct {
	Colors Palette
	// Source describes where the palette came from: a file path, or
	// "compiled fallback".
	Source string
}

// autoSearchPaths returns the "auto" search order from BUILD.md:
// $GHAB_ROOT/config/ombre-palette.json,
// ~/dev-root/config/ombre-palette.json,
// ~/Developer/dev-root/config/ombre-palette.json.
func autoSearchPaths() []string {
	var paths []string
	if v := os.Getenv("GHAB_ROOT"); v != "" {
		paths = append(paths, filepath.Join(v, "config", "ombre-palette.json"))
	}
	home, err := os.UserHomeDir()
	if err == nil {
		paths = append(paths,
			filepath.Join(home, "dev-root", "config", "ombre-palette.json"),
			filepath.Join(home, "Developer", "dev-root", "config", "ombre-palette.json"),
		)
	}
	return paths
}

func parsePaletteJSON(data []byte) (Palette, error) {
	var p Palette
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, err
	}
	return p, nil
}

// ResolvePalette resolves the [palette].file setting: "auto" (or empty)
// searches the standard locations in order; anything else is treated as
// an explicit path (with ~ expansion). Any failure to find/parse a
// palette falls back to the compiled palette with a warning.
func ResolvePalette(fileSetting string) (PaletteResult, []Warning) {
	if fileSetting != "" && fileSetting != "auto" {
		path := expandHome(fileSetting)
		data, err := os.ReadFile(path)
		if err != nil {
			return PaletteResult{Colors: CompiledFallbackPalette(), Source: "compiled fallback"},
				[]Warning{{Key: "palette.file", Message: fmt.Sprintf("cannot read %s: %v; using compiled fallback", path, err)}}
		}
		p, err := parsePaletteJSON(data)
		if err != nil {
			return PaletteResult{Colors: CompiledFallbackPalette(), Source: "compiled fallback"},
				[]Warning{{Key: "palette.file", Message: fmt.Sprintf("cannot parse %s: %v; using compiled fallback", path, err)}}
		}
		return PaletteResult{Colors: p, Source: path}, nil
	}

	for _, path := range autoSearchPaths() {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		p, err := parsePaletteJSON(data)
		if err != nil {
			continue
		}
		return PaletteResult{Colors: p, Source: path}, nil
	}

	return PaletteResult{Colors: CompiledFallbackPalette(), Source: "compiled fallback"},
		[]Warning{{Key: "palette.file", Message: "no ombre-palette.json found in search path; using compiled fallback"}}
}

func expandHome(path string) string {
	if !strings.HasPrefix(path, "~") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	return filepath.Join(home, strings.TrimPrefix(path, "~"))
}

// ResolvedTheme is the [theme] table with every value resolved to a
// lipgloss-ready color string: either a "#rrggbb" hex literal or a bare
// decimal ANSI index (from an "ansi:N" setting). The ui/style package
// turns these strings into lipgloss.Color values; config stays free of
// any UI dependency.
type ResolvedTheme struct {
	Accent       string
	AccentAlt    string
	OpenMarker   string
	ClosedMarker string
	ReleaseTag   string
	Frame        string
	SelectionBg  string
}

// ResolveTheme resolves every RawTheme field against the palette. A
// value that is not a valid "#hex", "ansi:N", or known palette name
// falls back to the compiled default for that field (also resolved) and
// produces a Warning.
func ResolveTheme(raw RawTheme, palette Palette) (ResolvedTheme, []Warning) {
	defaults := Defaults().Theme
	var warnings []Warning

	resolve := func(key, value, defaultValue string) string {
		if v, ok := resolveColorToken(value, palette); ok {
			return v
		}
		if v, ok := resolveColorToken(defaultValue, palette); ok {
			warnings = append(warnings, Warning{
				Key:     "theme." + key,
				Message: fmt.Sprintf("unresolvable color %q; keeping default %q", value, defaultValue),
			})
			return v
		}
		// The compiled default itself failed to resolve (e.g. palette
		// fallback is missing a name it should never be missing) --
		// this should not happen, but never crash.
		warnings = append(warnings, Warning{
			Key:     "theme." + key,
			Message: fmt.Sprintf("unresolvable color %q and default %q; using palette fallback", value, defaultValue),
		})
		return "#FFFFFF"
	}

	return ResolvedTheme{
		Accent:       resolve("accent", raw.Accent, defaults.Accent),
		AccentAlt:    resolve("accent_alt", raw.AccentAlt, defaults.AccentAlt),
		OpenMarker:   resolve("open_marker", raw.OpenMarker, defaults.OpenMarker),
		ClosedMarker: resolve("closed_marker", raw.ClosedMarker, defaults.ClosedMarker),
		ReleaseTag:   resolve("release_tag", raw.ReleaseTag, defaults.ReleaseTag),
		Frame:        resolve("frame", raw.Frame, defaults.Frame),
		SelectionBg:  resolve("selection_bg", raw.SelectionBg, defaults.SelectionBg),
	}, warnings
}

// resolveColorToken resolves one theme value: "#hex" literal, "ansi:N"
// passthrough, or a palette name looked up in palette.
func resolveColorToken(value string, palette Palette) (string, bool) {
	switch {
	case strings.HasPrefix(value, "#"):
		if isHexColor(value) {
			return value, true
		}
		return "", false
	case strings.HasPrefix(value, "ansi:"):
		idx := strings.TrimPrefix(value, "ansi:")
		if n, err := strconv.Atoi(idx); err == nil && n >= 0 && n <= 255 {
			return idx, true
		}
		return "", false
	default:
		if hex, ok := palette[value]; ok {
			return hex, true
		}
		return "", false
	}
}

func isHexColor(s string) bool {
	if len(s) != 4 && len(s) != 7 {
		return false
	}
	if s[0] != '#' {
		return false
	}
	for _, c := range s[1:] {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ColorMode reads ~/.config/ghab/color-mode: "dark" or "light". A
// missing file, unreadable file, or any other content falls back to "dark"
// -- the house rule is "never pin cream on cream", and dark is the safe
// default when the mode file can't be read (BUILD.md [readme]).
func ColorMode() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "dark"
	}
	data, err := os.ReadFile(filepath.Join(home, ".config", "lab", "color-mode"))
	if err != nil {
		return "dark"
	}
	if strings.TrimSpace(string(data)) == "light" {
		return "light"
	}
	return "dark"
}

// readmeAutoSearchPaths returns the "auto" search order for the glamour
// stylesheet matching mode ("dark" or "light"):
// <root>/ghab/styles/readme-ombre-{mode}.json for each vaultSearchRoots()
// entry -- the same root list the palette resolver searches, per BUILD.md's
// "[readme] auto ... via the same search roots as the palette resolver".
func readmeAutoSearchPaths(mode string) []string {
	var paths []string
	for _, root := range vaultSearchRoots() {
		paths = append(paths, filepath.Join(root, "ghab", "styles", "readme-ombre-"+mode+".json"))
	}
	return paths
}

// ResolveReadmeStyle resolves the glamour stylesheet path to use for mode
// ("dark" or "light"), per [readme].style_dark / [readme].style_light. An
// explicit setting (anything but "" or "auto") wins outright; "auto"
// searches readmeAutoSearchPaths. A returned path is guaranteed to exist
// and parse as JSON. When no usable stylesheet is found it returns the
// sentinel "standard:<mode>" with a Warning -- the renderer maps that to
// glamour's built-in style of the SAME mode, so the fallback never pins
// dark text on a light terminal (fail-soft, matching the rest of the
// config port: never crash over a missing/unparseable stylesheet).
func ResolveReadmeStyle(cfg ReadmeConfig, mode string) (path string, warnings []Warning) {
	setting := cfg.StyleDark
	key := "readme.style_dark"
	if mode == "light" {
		setting = cfg.StyleLight
		key = "readme.style_light"
	}
	fallback := "standard:" + mode

	if setting != "" && setting != "auto" {
		p := expandHome(setting)
		if err := validateGlamourStyle(p); err != nil {
			return fallback, []Warning{{Key: key, Message: fmt.Sprintf("cannot use %s: %v; falling back to glamour's built-in %s style", p, err, mode)}}
		}
		return p, nil
	}

	for _, p := range readmeAutoSearchPaths(mode) {
		if err := validateGlamourStyle(p); err == nil {
			return p, nil
		}
	}
	return fallback, []Warning{{Key: key, Message: fmt.Sprintf("no readme-ombre-%s.json found in search path; falling back to glamour's built-in %s style", mode, mode)}}
}

// validateGlamourStyle checks that path exists and parses as JSON. It does
// not validate glamour's own StyleConfig shape -- glamour.WithStylePath
// does that at render time -- this is just enough to fail soft here rather
// than handing glamour a path that doesn't exist.
func validateGlamourStyle(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var v map[string]interface{}
	return json.Unmarshal(data, &v)
}

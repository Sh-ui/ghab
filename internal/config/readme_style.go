package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ColorMode resolves the render color mode from [readme].color_mode:
// "dark" or "light" win outright; "auto" (the default) reads the
// single-word mode file at <config dir>/color-mode, so a terminal theme
// switcher can flip one file instead of rewriting TOML. A missing,
// unreadable, or unrecognized mode file falls back to "dark" -- the
// safe default, since light-on-light is the failure this guards.
func ColorMode(cfg ReadmeConfig) string {
	switch cfg.ColorMode {
	case "dark", "light":
		return cfg.ColorMode
	}
	data, err := os.ReadFile(filepath.Join(configDir(), "color-mode"))
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
// <config dir>/styles/readme-ombre-{mode}.json -- the styles/ subdir of
// the same directory config.toml lives in. The repo ships a dark/light
// pair under styles/ ready to copy there; [readme].style_dark and
// [readme].style_light point anywhere else explicitly.
func readmeAutoSearchPaths(mode string) []string {
	return []string{filepath.Join(configDir(), "styles", "readme-ombre-"+mode+".json")}
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

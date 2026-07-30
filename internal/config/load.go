package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

// ConfigPath resolves ~/.config/ghab/config.toml, respecting
// XDG_CONFIG_HOME.
func ConfigPath() string {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			home = "."
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "ghab", "config.toml")
}

// Load reads and validates the config file at path. A missing file is
// not an error: it yields the compiled defaults. An existing-but-unreadable
// file (permissions, is a directory, etc.) or a TOML syntax error is
// returned as err; the caller (main / --check-config) is responsible for
// turning that into exit 1. Every other problem -- a present but invalid
// key -- is fail-soft: it produces a Warning and the field keeps its
// compiled default.
func Load(path string) (Config, []Warning, error) {
	cfg := Defaults()

	data, readErr := os.ReadFile(path)
	if readErr != nil {
		if errors.Is(readErr, os.ErrNotExist) {
			return cfg, nil, nil
		}
		return cfg, nil, fmt.Errorf("reading %s: %w", path, readErr)
	}

	var raw map[string]interface{}
	if _, err := toml.Decode(string(data), &raw); err != nil {
		return cfg, nil, fmt.Errorf("parsing %s: %w", path, err)
	}

	var warnings []Warning
	warn := func(key, msg string) {
		warnings = append(warnings, Warning{Key: key, Message: msg})
	}

	mergePalette(&cfg.Palette, section(raw, "palette"), warn)
	mergeTheme(&cfg.Theme, section(raw, "theme"), warn)
	mergeReadme(&cfg.Readme, section(raw, "readme"), warn)
	mergeBehavior(&cfg.Behavior, section(raw, "behavior"), warn)
	mergeKeys(&cfg.Keys, section(raw, "keys"), warn)

	return cfg, warnings, nil
}

func section(raw map[string]interface{}, name string) map[string]interface{} {
	v, ok := raw[name]
	if !ok {
		return nil
	}
	m, ok := v.(map[string]interface{})
	if !ok {
		return nil
	}
	return m
}

// mergeString copies sec[key] into *dst if present and a string,
// otherwise warns and leaves *dst (already the default) untouched.
func mergeString(sec map[string]interface{}, key string, dst *string, warn func(key, msg string)) {
	if sec == nil {
		return
	}
	v, ok := sec[key]
	if !ok {
		return
	}
	s, ok := v.(string)
	if !ok {
		warn(key, fmt.Sprintf("expected string, got %T; keeping default %q", v, *dst))
		return
	}
	*dst = s
}

// mergeQueries merges a key whose value may be a single string (one
// query) or an array of strings (several, merged client-side) into dst,
// dropping blank entries. Anything else -- a non-string, a mixed array,
// or a value with no usable entry left -- keeps the compiled default and
// warns, the same fail-soft contract as every other key.
func mergeQueries(sec map[string]interface{}, key string, dst *[]string, warn func(key, msg string)) {
	if sec == nil {
		return
	}
	v, ok := sec[key]
	if !ok {
		return
	}
	var raw []string
	switch tv := v.(type) {
	case string:
		raw = []string{tv}
	case []interface{}:
		for _, e := range tv {
			s, ok := e.(string)
			if !ok {
				warn(key, fmt.Sprintf("expected string entries, got %T; keeping default %v", e, *dst))
				return
			}
			raw = append(raw, s)
		}
	default:
		warn(key, fmt.Sprintf("expected string or array of strings, got %T; keeping default %v", v, *dst))
		return
	}
	out := make([]string, 0, len(raw))
	for _, s := range raw {
		if strings.TrimSpace(s) != "" {
			out = append(out, s)
		}
	}
	if len(out) == 0 {
		warn(key, "empty query not allowed; keeping default")
		return
	}
	*dst = out
}

func mergePalette(p *PaletteSetting, sec map[string]interface{}, warn func(key, msg string)) {
	mergeString(sec, "file", &p.File, func(key, msg string) { warn("palette."+key, msg) })
	if p.File == "" {
		p.File = "auto"
	}
}

func mergeTheme(t *RawTheme, sec map[string]interface{}, warn func(key, msg string)) {
	prefixed := func(key, msg string) { warn("theme."+key, msg) }
	mergeString(sec, "accent", &t.Accent, prefixed)
	mergeString(sec, "accent_alt", &t.AccentAlt, prefixed)
	mergeString(sec, "open_marker", &t.OpenMarker, prefixed)
	mergeString(sec, "closed_marker", &t.ClosedMarker, prefixed)
	mergeString(sec, "release_tag", &t.ReleaseTag, prefixed)
	mergeString(sec, "frame", &t.Frame, prefixed)
	mergeString(sec, "selection_bg", &t.SelectionBg, prefixed)
}

func mergeReadme(r *ReadmeConfig, sec map[string]interface{}, warn func(key, msg string)) {
	prefixed := func(key, msg string) { warn("readme."+key, msg) }
	mergeString(sec, "style_dark", &r.StyleDark, prefixed)
	mergeString(sec, "style_light", &r.StyleLight, prefixed)
}

func mergeBehavior(b *BehaviorConfig, sec map[string]interface{}, warn func(key, msg string)) {
	prefixed := func(key, msg string) { warn("behavior."+key, msg) }

	mergeString(sec, "clone_dir", &b.CloneDir, prefixed)
	mergeString(sec, "editor", &b.Editor, prefixed)
	mergeString(sec, "open_url", &b.OpenURL, prefixed)

	if sec != nil {
		if v, ok := sec["page_size"]; ok {
			n, ok := toInt(v)
			if !ok || n <= 0 {
				warn("behavior.page_size", fmt.Sprintf("expected positive integer, got %v; keeping default %d", v, b.PageSize))
			} else {
				b.PageSize = n
			}
		}
	}

	rawTTL := b.CacheTTL
	mergeString(sec, "cache_ttl", &rawTTL, prefixed)
	if d, err := time.ParseDuration(rawTTL); err != nil {
		warn("behavior.cache_ttl", fmt.Sprintf("invalid duration %q; keeping default %q", rawTTL, b.CacheTTL))
	} else {
		b.CacheTTL = rawTTL
		b.CacheTTLDuration = d
	}

	// my_prs_query drives the search/issues queries the "my PRs" screen
	// fires (issue #11). It takes a single string or an array of strings,
	// because GitHub's search has no OR between qualifiers: the documented
	// authored-or-review-requested scope needs one query per qualifier,
	// merged client-side. An empty query would search every open PR on
	// GitHub, so -- unlike clone_dir/editor/open_url, where an empty
	// string is at least a plausible (if odd) value -- empties are
	// rejected like a keybinding: keep the default and warn.
	mergeQueries(sec, "my_prs_query", &b.MyPRsQueries, prefixed)

	if sec != nil {
		if v, ok := sec["diff_context_lines"]; ok {
			n, ok := toInt(v)
			if !ok || n < 0 {
				warn("behavior.diff_context_lines", fmt.Sprintf("expected non-negative integer, got %v; keeping default %d", v, b.DiffContextLines))
			} else {
				b.DiffContextLines = n
			}
		}
	}
}

func toInt(v interface{}) (int, bool) {
	switch n := v.(type) {
	case int64:
		return int(n), true
	case int:
		return n, true
	case float64:
		return int(n), true
	default:
		return 0, false
	}
}

func mergeKeys(k *KeyConfig, sec map[string]interface{}, warn func(key, msg string)) {
	prefixed := func(key, msg string) { warn("keys."+key, msg) }

	fields := []struct {
		name string
		dst  *string
	}{
		{"quit", &k.Quit}, {"help", &k.Help}, {"search", &k.Search}, {"back", &k.Back},
		{"tab_next", &k.TabNext}, {"tab_prev", &k.TabPrev}, {"down", &k.Down}, {"up", &k.Up},
		{"open", &k.Open}, {"profile", &k.Profile}, {"clone", &k.Clone}, {"edit", &k.Edit},
		{"web", &k.Web}, {"refresh", &k.Refresh}, {"my_prs", &k.MyPRs},
	}
	for _, f := range fields {
		before := *f.dst
		mergeString(sec, f.name, f.dst, prefixed)
		if *f.dst == "" {
			*f.dst = before
			prefixed(f.name, "empty binding not allowed; keeping default")
		}
	}
}

// Package config implements the ghab config port: TOML load, compiled
// defaults, fail-soft per-key validation, palette resolution and keymap
// merge. See BUILD.md "Config port" for the binding schema.
package config

import "time"

// Config is the fully resolved, always-valid ghab configuration. Every
// field carries a compiled default; Load never returns a Config with an
// invalid value -- bad input is warned about and replaced with the
// default instead.
type Config struct {
	Palette  PaletteSetting
	Theme    RawTheme
	Readme   ReadmeConfig
	Behavior BehaviorConfig
	Keys     KeyConfig
}

// PaletteSetting is the [palette] table.
type PaletteSetting struct {
	// File is "auto" (search the standard locations), an explicit path,
	// or empty (treated as "auto").
	File string
}

// RawTheme is the [theme] table as written in TOML: each field is either
// a palette name, an "ansi:N" literal, or a "#hex" literal. Resolve it
// against a Palette (see palette.go) to get usable color strings.
type RawTheme struct {
	Accent       string
	AccentAlt    string
	OpenMarker   string
	ClosedMarker string
	ReleaseTag   string
	Frame        string
	SelectionBg  string
}

// ReadmeConfig is the [readme] table.
type ReadmeConfig struct {
	StyleDark  string
	StyleLight string
}

// BehaviorConfig is the [behavior] table. CacheTTL is stored both as the
// raw string (for --check-config display) and pre-parsed as a duration.
type BehaviorConfig struct {
	CloneDir         string
	Editor           string
	OpenURL          string
	PageSize         int
	CacheTTL         string
	CacheTTLDuration time.Duration
	MyPRsQuery       string // search/issues query for the "my PRs" screen -- issue #11
	DiffContextLines int    // unchanged context lines shown per side of a PR diff hunk; 0 = no trimming
}

// KeyConfig is the [keys] table: every binding, remappable.
type KeyConfig struct {
	Quit    string
	Help    string
	Search  string
	Back    string
	TabNext string
	TabPrev string
	Down    string
	Up      string
	Open    string
	Profile string
	Clone   string
	Edit    string
	Web     string
	Refresh string
	MyPRs   string
}

// Defaults returns the compiled default configuration, matching the TOML
// block in BUILD.md exactly.
func Defaults() Config {
	return Config{
		Palette: PaletteSetting{File: "auto"},
		Theme: RawTheme{
			Accent:       "coffee",
			AccentAlt:    "teal",
			OpenMarker:   "green",
			ClosedMarker: "red",
			ReleaseTag:   "yellow",
			Frame:        "ansi:8",
			SelectionBg:  "ansi:8",
		},
		Readme: ReadmeConfig{
			StyleDark:  "auto",
			StyleLight: "auto",
		},
		Behavior: BehaviorConfig{
			CloneDir:         "~/Developer",
			Editor:           "micro",
			OpenURL:          "auto",
			PageSize:         30,
			CacheTTL:         "5m",
			CacheTTLDuration: 5 * time.Minute,
			MyPRsQuery:       "is:pr is:open involves:@me",
			DiffContextLines: 3,
		},
		Keys: KeyConfig{
			Quit:    "q",
			Help:    "?",
			Search:  "s",
			Back:    "esc",
			TabNext: "l",
			TabPrev: "h",
			Down:    "j",
			Up:      "k",
			Open:    "enter",
			Profile: "u",
			Clone:   "c",
			Edit:    "e",
			Web:     "o",
			Refresh: "r",
			MyPRs:   "ctrl+p",
		},
	}
}

// Warning is one fail-soft validation notice: a key was present but
// invalid, so the compiled default was kept.
type Warning struct {
	Key     string
	Message string
}

func (w Warning) String() string {
	return w.Key + ": " + w.Message
}

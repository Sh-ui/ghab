package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// loadTOML writes body to a temp config file and loads it, so these tests
// exercise the real Load path (TOML decode included) rather than poking at
// the merge helpers directly.
func loadTOML(t *testing.T, body string) (Config, []Warning) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("writing temp config: %v", err)
	}
	cfg, warnings, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	return cfg, warnings
}

func warningFor(warnings []Warning, key string) (Warning, bool) {
	for _, w := range warnings {
		if w.Key == key {
			return w, true
		}
	}
	return Warning{}, false
}

// The documented my-PRs scope is authored-or-review-requested. GitHub
// search has no OR between qualifiers, so the compiled default has to be
// two queries -- and must not be the wider involves:@me, which also
// matches PRs the user merely commented on or was mentioned in.
func TestDefaultMyPRsQueriesMatchDocumentedScope(t *testing.T) {
	got := Defaults().Behavior.MyPRsQueries
	want := []string{
		"is:pr is:open author:@me",
		"is:pr is:open review-requested:@me",
	}
	if len(got) != len(want) {
		t.Fatalf("default my_prs_query = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("default my_prs_query[%d] = %q, want %q", i, got[i], want[i])
		}
	}
	for _, q := range got {
		if strings.Contains(q, "involves:@me") {
			t.Errorf("default my_prs_query still carries involves:@me: %q", q)
		}
	}
}

func TestMyPRsQueryAcceptsSingleString(t *testing.T) {
	cfg, warnings := loadTOML(t, "[behavior]\nmy_prs_query = \"is:pr is:open author:@me\"\n")
	if len(cfg.Behavior.MyPRsQueries) != 1 || cfg.Behavior.MyPRsQueries[0] != "is:pr is:open author:@me" {
		t.Errorf("my_prs_query = %v, want one entry", cfg.Behavior.MyPRsQueries)
	}
	if w, ok := warningFor(warnings, "behavior.my_prs_query"); ok {
		t.Errorf("unexpected warning: %s", w.Message)
	}
}

func TestMyPRsQueryAcceptsArray(t *testing.T) {
	cfg, warnings := loadTOML(t, "[behavior]\nmy_prs_query = [\"is:pr is:open author:@me\", \"is:pr is:open assignee:@me\"]\n")
	want := []string{"is:pr is:open author:@me", "is:pr is:open assignee:@me"}
	if len(cfg.Behavior.MyPRsQueries) != len(want) {
		t.Fatalf("my_prs_query = %v, want %v", cfg.Behavior.MyPRsQueries, want)
	}
	for i := range want {
		if cfg.Behavior.MyPRsQueries[i] != want[i] {
			t.Errorf("my_prs_query[%d] = %q, want %q", i, cfg.Behavior.MyPRsQueries[i], want[i])
		}
	}
	if w, ok := warningFor(warnings, "behavior.my_prs_query"); ok {
		t.Errorf("unexpected warning: %s", w.Message)
	}
}

// Fail-soft, same contract as every other key: a bad value warns once and
// keeps the compiled default rather than crashing or searching all of
// GitHub.
func TestMyPRsQueryBadValuesKeepDefault(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{name: "empty string", body: "[behavior]\nmy_prs_query = \"\"\n"},
		{name: "whitespace only", body: "[behavior]\nmy_prs_query = \"   \"\n"},
		{name: "empty array", body: "[behavior]\nmy_prs_query = []\n"},
		{name: "array of blanks", body: "[behavior]\nmy_prs_query = [\"\", \"  \"]\n"},
		{name: "wrong type", body: "[behavior]\nmy_prs_query = 7\n"},
		{name: "mixed array", body: "[behavior]\nmy_prs_query = [\"is:pr is:open author:@me\", 7]\n"},
	}
	def := Defaults().Behavior.MyPRsQueries
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg, warnings := loadTOML(t, tc.body)
			if len(cfg.Behavior.MyPRsQueries) != len(def) {
				t.Fatalf("my_prs_query = %v, want default %v", cfg.Behavior.MyPRsQueries, def)
			}
			for i := range def {
				if cfg.Behavior.MyPRsQueries[i] != def[i] {
					t.Errorf("my_prs_query[%d] = %q, want default %q", i, cfg.Behavior.MyPRsQueries[i], def[i])
				}
			}
			if _, ok := warningFor(warnings, "behavior.my_prs_query"); !ok {
				t.Errorf("expected a behavior.my_prs_query warning, got %v", warnings)
			}
		})
	}
}

// Blank entries alongside real ones are dropped, not passed to the search
// endpoint (an empty query matches every open PR on GitHub).
func TestMyPRsQueryDropsBlankEntries(t *testing.T) {
	cfg, _ := loadTOML(t, "[behavior]\nmy_prs_query = [\"is:pr is:open author:@me\", \"  \"]\n")
	if len(cfg.Behavior.MyPRsQueries) != 1 || cfg.Behavior.MyPRsQueries[0] != "is:pr is:open author:@me" {
		t.Errorf("my_prs_query = %v, want the single non-blank entry", cfg.Behavior.MyPRsQueries)
	}
}

// [readme].color_mode pins the render mode outright; anything but
// auto/dark/light keeps the default and warns (the fail-soft contract).
func TestReadmeColorMode(t *testing.T) {
	cfg, warnings := loadTOML(t, "[readme]\ncolor_mode = \"light\"\n")
	if cfg.Readme.ColorMode != "light" {
		t.Errorf("color_mode = %q, want %q", cfg.Readme.ColorMode, "light")
	}
	if got := ColorMode(cfg.Readme); got != "light" {
		t.Errorf("ColorMode() = %q, want pinned %q", got, "light")
	}
	if len(warnings) != 0 {
		t.Errorf("unexpected warnings: %v", warnings)
	}

	cfg, warnings = loadTOML(t, "[readme]\ncolor_mode = \"solarized\"\n")
	if cfg.Readme.ColorMode != "auto" {
		t.Errorf("color_mode = %q, want default %q", cfg.Readme.ColorMode, "auto")
	}
	if _, ok := warningFor(warnings, "readme.color_mode"); !ok {
		t.Errorf("expected a readme.color_mode warning, got %v", warnings)
	}
}

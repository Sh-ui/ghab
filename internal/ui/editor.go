package ui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// writeEditorCache writes data to
// $XDG_CACHE_HOME/ghab/{owner}/{repo}/{path} (falling back to ~/.cache
// when XDG_CACHE_HOME is unset), creating parent directories as needed,
// and returns the written path. The real filename is preserved (not a
// temp name) so the editor's own syntax detection works, per BUILD.md's
// edit-hook spec.
func writeEditorCache(owner, repo, path string, data []byte) (string, error) {
	base := os.Getenv("XDG_CACHE_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, ".cache")
	}
	root := filepath.Join(base, "ghab", owner, repo)
	dest := filepath.Join(root, filepath.FromSlash(path))
	// A tree path with enough ../ segments would escape the cache dir;
	// the API shouldn't produce one, but refuse rather than trust it.
	if rel, err := filepath.Rel(root, dest); err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("refusing path escaping cache dir: %q", path)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(dest, data, 0o644); err != nil {
		return "", err
	}
	return dest, nil
}

// buildEditorCommand builds the editor invocation from [behavior].editor:
// a literal "%s" token is substituted with path; otherwise path is
// appended as the last argument (BUILD.md's edit-hook spec). An empty
// editor setting can't happen (config.Defaults always carries "micro"),
// but falls back to it defensively rather than exec-ing an empty argv.
func buildEditorCommand(editor, path string) *exec.Cmd {
	parts := strings.Fields(editor)
	if len(parts) == 0 {
		parts = []string{"micro"}
	}
	substituted := false
	for i, p := range parts {
		if p == "%s" {
			parts[i] = path
			substituted = true
		}
	}
	if !substituted {
		parts = append(parts, path)
	}
	return exec.Command(parts[0], parts[1:]...)
}

package ui

import (
	"runtime"
	"strings"
)

// hostAssetMatch reports whether a release asset's filename looks built
// for the machine ghab is currently running on -- used by the releases
// tab to mark the asset matching the running host with an accent "▸"
// marker (BUILD.md: "runtime.GOOS + GOARCH normalization: arm64/aarch64,
// amd64/x86_64 -- simple substring match on asset name").
func hostAssetMatch(name string) bool {
	return matchesHost(name, runtime.GOOS, runtime.GOARCH)
}

// matchesHost is hostAssetMatch's pure core, taking goos/goarch as
// parameters so it's unit-testable without depending on the actual build
// target.
func matchesHost(name, goos, goarch string) bool {
	lower := strings.ToLower(name)
	return containsAny(lower, osTokens(goos)) && containsAny(lower, archTokens(goarch))
}

func containsAny(s string, tokens []string) bool {
	for _, t := range tokens {
		if strings.Contains(s, t) {
			return true
		}
	}
	return false
}

// osTokens returns the filename substrings a release asset built for
// goos is likely to carry. ghab only ships for darwin and linux
// (BUILD.md: "Runs on both hosts -- Mac + Pi, aarch64"), but the
// default falls back to the bare GOOS value so an unexpected host still
// gets a best-effort match instead of never matching.
func osTokens(goos string) []string {
	switch goos {
	case "darwin":
		return []string{"darwin", "macos", "mac", "osx"}
	case "linux":
		return []string{"linux"}
	case "windows":
		return []string{"windows", "win"}
	default:
		return []string{goos}
	}
}

// archTokens returns the filename substrings a release asset built for
// goarch is likely to carry, normalizing the aliases BUILD.md calls out
// (arm64/aarch64, amd64/x86_64).
func archTokens(goarch string) []string {
	switch goarch {
	case "arm64":
		return []string{"arm64", "aarch64"}
	case "amd64":
		return []string{"amd64", "x86_64", "x64"}
	default:
		return []string{goarch}
	}
}

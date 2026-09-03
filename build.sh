#!/usr/bin/env bash
# Cross-compile static ghab binaries into dist/ for the common
# desktop/server targets. CGO_ENABLED=0: no libc dependency, the
# binaries run anywhere their GOOS/GOARCH matches.
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")"

mkdir -p dist

version="$(git describe --tags --always --dirty 2>/dev/null || echo dev)"

build() {
	local os="$1"
	local arch="$2"
	local out="dist/ghab-${os}-${arch}"
	echo "building ${out} (version ${version})"
	CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build -ldflags "-X main.version=${version}" -o "$out" .
}

build darwin arm64
build darwin amd64
build linux arm64
build linux amd64

echo "done: $(ls dist)"

#!/usr/bin/env bash
# Cross-compile ghab for both ghab hosts (Mac + Pi, both aarch64)
# into dist/. Static builds: CGO_ENABLED=0, no libc dependency at runtime.
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")"

mkdir -p dist

version="$(git describe --tags --always --dirty 2>/dev/null || echo dev)"

build() {
	local os="$1"
	local arch="$2"
	local out="dist/ghab-${os}-${arch}"
	echo "building ${out} (version ${version})"
	CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build -o "$out" .
}

build darwin arm64
build linux arm64

echo "done: $(ls dist)"

#!/usr/bin/env bash
# Cross-compile ghab for both ghab hosts into dist/: the Mac at its
# native arch (this Mac is Intel -- don't assume arm64) and the device
# Pi (linux-arm64). Static builds: CGO_ENABLED=0, no libc dependency.
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

build darwin "$(go env GOHOSTARCH)"
build linux arm64

echo "done: $(ls dist)"

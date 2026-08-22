#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
OUTPUT_PATH="${OUTPUT_PATH:-$ROOT_DIR/emos-video-upload}"
BUILD_TIME="${BUILD_TIME:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}"

if [[ -z "${VERSION:-}" ]]; then
    VERSION="$(git -C "$ROOT_DIR" describe --tags --always --dirty 2>/dev/null || true)"
    VERSION="${VERSION:-dev}"
fi

if [[ -z "${GIT_VERSION:-}" ]]; then
    GIT_VERSION="$(git -C "$ROOT_DIR" rev-parse HEAD 2>/dev/null || true)"
    GIT_VERSION="${GIT_VERSION:-dev}"
fi

export GOOS=linux
export GOARCH=amd64
export CGO_ENABLED=0
export TMPDIR="${TMPDIR:-$ROOT_DIR/tmp/go-tmp}"
export GOTMPDIR="${GOTMPDIR:-$ROOT_DIR/tmp/go-tmp}"
export GOCACHE="${GOCACHE:-$ROOT_DIR/tmp/go-build-cache}"
export GOMODCACHE="${GOMODCACHE:-$ROOT_DIR/tmp/go-mod-cache}"

mkdir -p "$TMPDIR" "$GOTMPDIR" "$GOCACHE" "$GOMODCACHE"

pnpm --dir "$ROOT_DIR/web" run build

LDFLAGS=(
    "-s"
    "-w"
    "-buildid="
    "-X"
    "main.version=$VERSION"
    "-X"
    "main.buildTime=$BUILD_TIME"
    "-X"
    "main.gitVersion=$GIT_VERSION"
)

go build \
    -trimpath \
    -buildvcs=false \
    -ldflags="${LDFLAGS[*]}" \
    -o "$OUTPUT_PATH" \
    "$ROOT_DIR"

file "$OUTPUT_PATH" 2>/dev/null || true

#!/usr/bin/env sh
set -eu

ROOT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
cd "$ROOT_DIR"

# shellcheck disable=SC1091
. ./scripts/ci-versions.env

require_tool() {
  if ! command -v "$1" >/dev/null 2>&1; then
    printf 'missing required tool: %s\n' "$1" >&2
    exit 127
  fi
}

require_tool go
require_tool make
require_tool golangci-lint
require_tool govulncheck

actual_go_version="$(go env GOVERSION)"
if [ "$actual_go_version" != "go$GO_VERSION" ]; then
  printf 'Go version mismatch: expected go%s from scripts/ci-versions.env, got %s\n' "$GO_VERSION" "$actual_go_version" >&2
  exit 2
fi

make go-check
govulncheck ./...

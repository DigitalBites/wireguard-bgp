#!/usr/bin/env sh
set -eu

ROOT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
cd "$ROOT_DIR"

# shellcheck disable=SC1091
. ./scripts/ci-versions.env

ARCHES="${1:-$BUILD_ARCHES}"

old_ifs="$IFS"
IFS=","
set -- $ARCHES
IFS="$old_ifs"

json="["
sep=""
for arch in "$@"; do
  case "$arch" in
    amd64|arm64)
      json="${json}${sep}\"${arch}\""
      sep=","
      ;;
    "")
      ;;
    *)
      printf 'unsupported arch: %s\n' "$arch" >&2
      exit 2
      ;;
  esac
done
json="${json}]"

if [ "$json" = "[]" ]; then
  printf 'at least one arch is required\n' >&2
  exit 2
fi

printf '%s\n' "$json"

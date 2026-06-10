#!/usr/bin/env sh
set -eu

ROOT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
cd "$ROOT_DIR"

# shellcheck disable=SC1091
. ./scripts/ci-versions.env

tag="${GITHUB_REF_NAME:-}"
if [ "${GITHUB_REF_TYPE:-}" = "tag" ] && printf '%s\n' "$tag" | grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+$'; then
  printf '%s\n' "${tag#v}"
  exit 0
fi

latest_tag="$(git tag --merged HEAD --list 'v[0-9]*.[0-9]*.[0-9]*' --sort=-v:refname 2>/dev/null | head -n 1 || true)"
if [ -z "$latest_tag" ]; then
  printf '%s\n' "$INITIAL_VERSION"
  exit 0
fi

version="${latest_tag#v}"
major="${version%%.*}"
rest="${version#*.}"
minor="${rest%%.*}"
patch="${rest#*.}"

case "$major:$minor:$patch" in
  *[!0-9:]*)
    printf 'invalid semver tag: %s\n' "$latest_tag" >&2
    exit 2
    ;;
esac

printf '%s.%s.%s\n' "$major" "$minor" "$((patch + 1))"

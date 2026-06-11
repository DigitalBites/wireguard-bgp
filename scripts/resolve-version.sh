#!/usr/bin/env sh
set -eu

ROOT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
cd "$ROOT_DIR"

# shellcheck disable=SC1091
. ./scripts/ci-versions.env

tag="${GITHUB_REF_NAME:-}"
semver_tag_pattern='^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?$'
stable_tag_pattern='^v[0-9]+\.[0-9]+\.[0-9]+$'

if [ "${GITHUB_REF_TYPE:-}" = "tag" ]; then
  if ! printf '%s\n' "$tag" | grep -Eq "$semver_tag_pattern"; then
    printf 'invalid release tag: %s\n' "$tag" >&2
    printf 'expected vX.Y.Z or vX.Y.Z-prerelease, for example v0.1.0 or v0.1.0-beta.1\n' >&2
    exit 2
  fi
  printf '%s\n' "${tag#v}"
  exit 0
fi

latest_tag="$(git tag --merged HEAD --list 'v[0-9]*.[0-9]*.[0-9]*' --sort=-v:refname 2>/dev/null | grep -E "$stable_tag_pattern" | head -n 1 || true)"
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

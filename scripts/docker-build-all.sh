#!/usr/bin/env sh
set -eu

ROOT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
cd "$ROOT_DIR"

# shellcheck disable=SC1091
. ./scripts/ci-versions.env

old_ifs="$IFS"
IFS=","
set -- $BUILD_ARCHES
IFS="$old_ifs"

for arch in "$@"; do
  case "$arch" in
    amd64)
      platform="linux/amd64"
      ;;
    arm64)
      platform="linux/arm64"
      ;;
    "")
      continue
      ;;
    *)
      printf 'unsupported arch: %s\n' "$arch" >&2
      exit 2
      ;;
  esac

  ARCH="$arch" PLATFORM="$platform" ./scripts/docker-build.sh
done

#!/usr/bin/env sh
set -eu

ROOT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
cd "$ROOT_DIR"

# shellcheck disable=SC1091
. ./scripts/ci-versions.env

IMAGE_REF="${IMAGE_REF:-}"
IMAGE_PLATFORM="${IMAGE_PLATFORM:-${TRIVY_PLATFORM:-}}"
TRIVY_IMAGE="${TRIVY_IMAGE:-$TRIVY_IMAGE}"
TRIVY_SEVERITY="${TRIVY_SEVERITY:-HIGH,CRITICAL}"
TRIVY_EXIT_CODE="${TRIVY_EXIT_CODE:-1}"

if [ -z "$IMAGE_REF" ]; then
  printf 'IMAGE_REF is required\n' >&2
  exit 2
fi

if ! command -v docker >/dev/null 2>&1; then
  printf 'docker is required for image scanning\n' >&2
  exit 127
fi

mkdir -p .trivy-cache

set -- docker run --rm \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v "$ROOT_DIR/.trivy-cache:/root/.cache/" \
  "$TRIVY_IMAGE" image \
  --severity "$TRIVY_SEVERITY" \
  --ignore-unfixed \
  --exit-code "$TRIVY_EXIT_CODE" \
  --skip-version-check

if [ -n "$IMAGE_PLATFORM" ]; then
  set -- "$@" --platform "$IMAGE_PLATFORM"
fi

set -- "$@" "$IMAGE_REF"
"$@"

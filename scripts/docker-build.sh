#!/usr/bin/env sh
set -eu

ROOT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
cd "$ROOT_DIR"

# shellcheck disable=SC1091
. ./scripts/ci-versions.env

IMAGE_NAME="${IMAGE_NAME:-digitalbites/wireguard-bgp}"
CHANNEL="${CHANNEL:-local}"
BASE_VERSION="${BASE_VERSION:-$(./scripts/resolve-version.sh)}"
VERSION="${VERSION:-$BASE_VERSION}"
TAG_VERSION="${VERSION#v}"
TAG_VERSION="v${TAG_VERSION}"
BASE_TAG_VERSION="${BASE_VERSION#v}"
BASE_TAG_VERSION="v${BASE_TAG_VERSION}"
PLATFORM="${PLATFORM:-linux/amd64}"
ARCH="${ARCH:-${PLATFORM##*/}}"
PUSH="${PUSH:-false}"
LOAD="${LOAD:-false}"
BUILD_ID="${BUILD_ID:-local}"
SHORT_SHA="${SHORT_SHA:-$(git rev-parse --short=12 HEAD 2>/dev/null || printf 'nogit')}"
SHORT_SHA="$(printf '%.12s' "$SHORT_SHA")"
CREATED="${CREATED:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}"

case "$CHANNEL" in
  dev)
    PRIMARY_TAG="${IMAGE_NAME}:${BASE_TAG_VERSION}-${ARCH}-dev.${BUILD_ID}"
    EXTRA_TAGS="${IMAGE_NAME}:dev-${SHORT_SHA}-${ARCH} ${IMAGE_NAME}:dev-${ARCH}"
    ;;
  release)
    PRIMARY_TAG="${IMAGE_NAME}:${TAG_VERSION}-${ARCH}"
    EXTRA_TAGS="${IMAGE_NAME}:latest-${ARCH}"
    ;;
  local)
    PRIMARY_TAG="${IMAGE_NAME}:${TAG_VERSION}-${ARCH}-local"
    EXTRA_TAGS=""
    ;;
  *)
    printf 'CHANNEL must be one of: dev, release, local\n' >&2
    exit 2
    ;;
esac

set -- docker buildx build \
  --platform "$PLATFORM" \
  --label "org.opencontainers.image.created=$CREATED" \
  --label "org.opencontainers.image.revision=$SHORT_SHA" \
  --label "org.opencontainers.image.version=$TAG_VERSION" \
  --tag "$PRIMARY_TAG"

for tag in $EXTRA_TAGS; do
  set -- "$@" --tag "$tag"
done

if [ "$PUSH" = "true" ]; then
  set -- "$@" --push
elif [ "$LOAD" = "true" ]; then
  set -- "$@" --load
else
  set -- "$@" --output=type=cacheonly
fi

set -- "$@" .

printf 'building %s for %s\n' "$PRIMARY_TAG" "$PLATFORM"
"$@"

printf 'primary_tag=%s\n' "$PRIMARY_TAG"
if [ -n "${GITHUB_OUTPUT:-}" ]; then
  {
    printf 'primary_tag=%s\n' "$PRIMARY_TAG"
    printf 'arch=%s\n' "$ARCH"
    printf 'platform=%s\n' "$PLATFORM"
  } >> "$GITHUB_OUTPUT"
fi

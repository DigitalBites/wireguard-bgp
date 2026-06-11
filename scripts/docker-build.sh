#!/usr/bin/env sh
set -eu

ROOT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
cd "$ROOT_DIR"

# shellcheck disable=SC1091
. ./scripts/ci-versions.env

IMAGE_NAME="${IMAGE_NAME:-digitalbites/wireguard-bgp}"
GO_VERSION="${GO_VERSION:?GO_VERSION must be set by ./scripts/ci-versions.env}"
WIREGUARD_GO_VERSION="${WIREGUARD_GO_VERSION:?WIREGUARD_GO_VERSION must be set by ./scripts/ci-versions.env}"
WIREGUARD_GO_SOURCE="${WIREGUARD_GO_SOURCE:?WIREGUARD_GO_SOURCE must be set by ./scripts/ci-versions.env}"
WIREGUARD_GO_X_CRYPTO_VERSION="${WIREGUARD_GO_X_CRYPTO_VERSION:?WIREGUARD_GO_X_CRYPTO_VERSION must be set by ./scripts/ci-versions.env}"
WIREGUARD_GO_X_NET_VERSION="${WIREGUARD_GO_X_NET_VERSION:?WIREGUARD_GO_X_NET_VERSION must be set by ./scripts/ci-versions.env}"
WIREGUARD_GO_X_SYS_VERSION="${WIREGUARD_GO_X_SYS_VERSION:?WIREGUARD_GO_X_SYS_VERSION must be set by ./scripts/ci-versions.env}"
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
    BUILD_VERSION="${BASE_TAG_VERSION}-${ARCH}-dev.${BUILD_ID}"
    PRIMARY_TAG="${IMAGE_NAME}:${BASE_TAG_VERSION}-${ARCH}-dev.${BUILD_ID}"
    EXTRA_TAGS="${IMAGE_NAME}:dev-${SHORT_SHA}-${ARCH} ${IMAGE_NAME}:dev-${ARCH}"
    ;;
  release)
    BUILD_VERSION="${TAG_VERSION}-${ARCH}"
    PRIMARY_TAG="${IMAGE_NAME}:${TAG_VERSION}-${ARCH}"
    EXTRA_TAGS="${IMAGE_NAME}:latest-${ARCH}"
    ;;
  local)
    BUILD_VERSION="${APP_BUILD_VERSION:-}"
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
  --build-arg "GO_VERSION=$GO_VERSION" \
  --build-arg "WIREGUARD_GO_VERSION=$WIREGUARD_GO_VERSION" \
  --build-arg "WIREGUARD_GO_SOURCE=$WIREGUARD_GO_SOURCE" \
  --build-arg "WIREGUARD_GO_X_CRYPTO_VERSION=$WIREGUARD_GO_X_CRYPTO_VERSION" \
  --build-arg "WIREGUARD_GO_X_NET_VERSION=$WIREGUARD_GO_X_NET_VERSION" \
  --build-arg "WIREGUARD_GO_X_SYS_VERSION=$WIREGUARD_GO_X_SYS_VERSION" \
  --label "org.opencontainers.image.created=$CREATED" \
  --label "org.opencontainers.image.revision=$SHORT_SHA" \
  --label "org.opencontainers.image.version=$TAG_VERSION" \
  --tag "$PRIMARY_TAG"

if [ -n "${APP_BUILD_VERSION:-$BUILD_VERSION}" ]; then
  set -- "$@" --build-arg "APP_BUILD_VERSION=${APP_BUILD_VERSION:-$BUILD_VERSION}"
fi

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

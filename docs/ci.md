# CI and Container Publishing

This repository uses GitHub Actions for checks and image publishing, with the
same shell scripts available for local use.

## Required GitHub Settings

Set these repository secrets:

- `DOCKERHUB_USERNAME`
- `DOCKERHUB_TOKEN`

Optional repository variable:

- `DOCKERHUB_IMAGE`, for example `digitalbites/wireguard-bgp`
- `BUILD_ARCHES`, for example `amd64`, `arm64`, or `amd64,arm64`

If `DOCKERHUB_IMAGE` is unset, workflows use `digitalbites/wireguard-bgp`.
If `BUILD_ARCHES` is unset, workflows use the default from
`scripts/ci-versions.env`.

## Tool Versions

`scripts/ci-versions.env` is the shared source for pinned build and check tool
versions:

- `GO_VERSION` controls GitHub Actions `setup-go`, local `make check` version
  validation, and the Docker builder image.
- `GOLANGCI_LINT_VERSION` controls the installed `golangci-lint` version.
- `GOVULNCHECK_VERSION` controls the installed `govulncheck` version.
- `TRIVY_IMAGE` controls the image scanner container tag.

Keep `go.mod` aligned with `GO_VERSION`; the CI setup intentionally reads the
shared version file instead of inferring the toolchain from `go.mod`.

## Local Commands

Run the Go quality gate and vulnerability scan:

```sh
./scripts/install-ci-tools.sh
make check
```

Build a local amd64 image:

```sh
ARCH=amd64 PLATFORM=linux/amd64 CHANNEL=local LOAD=true ./scripts/docker-build.sh
```

Scan a local image:

```sh
IMAGE_REF=digitalbites/wireguard-bgp:v0.0.1-amd64-local ./scripts/image-scan.sh
```

Build an arm64 image without loading it into the local Docker image store:

```sh
ARCH=arm64 PLATFORM=linux/arm64 CHANNEL=local ./scripts/docker-build.sh
```

Build all configured arches locally:

```sh
BUILD_ARCHES=amd64,arm64 CHANNEL=local ./scripts/docker-build-all.sh
```

## Image Tags

Pull requests build images but do not push.

Pushes to `main` publish development tags per architecture:

- `v0.0.1-amd64-dev.<run_number>`
- `v0.0.1-arm64-dev.<run_number>`
- `dev-<commit_sha>-amd64`
- `dev-<commit_sha>-arm64`
- `dev-amd64`
- `dev-arm64`

The base development version is derived automatically from git tags:

- Before the first release tag, dev builds use `INITIAL_VERSION` from
  `scripts/ci-versions.env`.
- After a release tag such as `v0.0.1`, dev builds use the next patch version,
  such as `v0.0.2-<arch>-dev.<run_number>`.

Release tags such as `v0.1.0` publish:

- `v0.1.0-amd64`
- `v0.1.0-arm64`
- `latest-amd64`
- `latest-arm64`

The deployment path should prefer explicit architecture tags because some
Peplink container runtimes have had trouble with multi-architecture manifest
tags.

## Versioning

Use semantic versioning:

- Patch, such as `0.0.1` to `0.0.2`, for small fixes and internal hardening.
- Minor, such as `0.0.x` to `0.1.0`, when the app has a meaningful user-facing
  capability or deployment behavior worth calling a release line.
- Major can stay at `0` until the configuration/API behavior is stable.

Development builds use `v<next-patch>` after the latest reachable `vX.Y.Z` git
tag plus `-<arch>-dev.<run>`. Formal releases are created by pushing a semver
git tag:

```sh
git tag v0.1.0
git push origin main v0.1.0
```

Only tag builds publish `latest-<arch>`.

To publish `v0.0.2`, push tag `v0.0.2`. You do not need to first release
`v0.0.1`; the next-patch dev version is only a preview tag convention, not a
required release sequence.

## Checks

The CI workflow runs:

- `make check`, which calls `scripts/ci-check.sh`
- `govulncheck ./...`
- Dependency Review on pull requests
- CodeQL for Go

Tool installs and GitHub Actions are pinned to exact versions in workflow YAML;
avoid `@latest` and broad moving action tags for the build path.
Pinned Go/tool versions, the Trivy image tag, default build arches, and initial
image version live in `scripts/ci-versions.env`.

GitHub jobs use `ubuntu-24.04` instead of `ubuntu-latest`. This pins the runner
image family, but GitHub-hosted runners still receive patch refreshes from
GitHub; digest-level runner pinning would require a pinned job container or
self-hosted runner.

The container workflow builds `linux/amd64` and `linux/arm64` images, pushes
only on `main`, manual release dispatch, or version tags, and scans built images
with Trivy through `scripts/image-scan.sh`.

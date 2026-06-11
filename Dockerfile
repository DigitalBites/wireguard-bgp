ARG GO_VERSION=1.26.4
ARG WIREGUARD_GO_VERSION=0.0.20250522
ARG WIREGUARD_GO_SOURCE=https://github.com/WireGuard/wireguard-go.git
ARG WIREGUARD_GO_X_CRYPTO_VERSION=v0.52.0
ARG WIREGUARD_GO_X_NET_VERSION=v0.55.0
ARG WIREGUARD_GO_X_SYS_VERSION=v0.45.0

FROM --platform=$BUILDPLATFORM golang:${GO_VERSION}-alpine AS build

ARG TARGETOS
ARG TARGETARCH
ARG APP_BUILD_VERSION

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /out/peplink-wg-bgp ./cmd/peplink-wg-bgp

FROM --platform=$BUILDPLATFORM golang:${GO_VERSION}-alpine AS wireguard-go-build

ARG TARGETOS
ARG TARGETARCH
ARG WIREGUARD_GO_VERSION
ARG WIREGUARD_GO_SOURCE
ARG WIREGUARD_GO_X_CRYPTO_VERSION
ARG WIREGUARD_GO_X_NET_VERSION
ARG WIREGUARD_GO_X_SYS_VERSION

WORKDIR /src/wireguard-go

RUN apk add --no-cache git && \
    git clone --depth 1 --branch "$WIREGUARD_GO_VERSION" "$WIREGUARD_GO_SOURCE" . && \
    go get \
    "golang.org/x/crypto@${WIREGUARD_GO_X_CRYPTO_VERSION}" \
    "golang.org/x/net@${WIREGUARD_GO_X_NET_VERSION}" \
    "golang.org/x/sys@${WIREGUARD_GO_X_SYS_VERSION}" && \
    go mod tidy && \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /out/wireguard-go .

FROM alpine:3.24

ARG APP_BUILD_VERSION

RUN apk upgrade --no-cache && \
    apk add --no-cache \
    bird \
    ca-certificates \
    iproute2 \
    su-exec \
    wireguard-tools

RUN addgroup -S app && \
    adduser -S -D -H -h /nonexistent -s /sbin/nologin -G app app && \
    mkdir -p /app-state/wireguard /app-state/bird /run/bird /run/peplink-wg-bgp && \
    chown -R app:app /app-state && \
    chown root:app /run/peplink-wg-bgp && \
    chmod 700 /app-state /app-state/wireguard /app-state/bird && \
    chmod 2770 /run/peplink-wg-bgp

COPY --from=wireguard-go-build /out/wireguard-go /usr/bin/wireguard-go
COPY --from=build /out/peplink-wg-bgp /usr/local/bin/peplink-wg-bgp
COPY docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh
RUN chmod 0755 /usr/local/bin/docker-entrypoint.sh

EXPOSE 8080

ENV APP_CONFIG=/app-state/app.yaml
ENV APP_BUILD_VERSION=$APP_BUILD_VERSION

ENTRYPOINT ["/usr/local/bin/docker-entrypoint.sh"]

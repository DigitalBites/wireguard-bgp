ARG GO_VERSION=1.26.4
FROM --platform=$BUILDPLATFORM golang:${GO_VERSION}-alpine AS build

ARG TARGETOS
ARG TARGETARCH

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /out/peplink-wg-bgp ./cmd/peplink-wg-bgp

FROM alpine:3.22

RUN apk add --no-cache \
    bird \
    ca-certificates \
    iproute2 \
    su-exec \
    wireguard-go \
    wireguard-tools

RUN addgroup -S app && \
    adduser -S -D -H -h /nonexistent -s /sbin/nologin -G app app && \
    mkdir -p /app-state/wireguard /app-state/bird /run/bird /run/peplink-wg-bgp && \
    chown -R app:app /app-state && \
    chown root:app /run/peplink-wg-bgp && \
    chmod 700 /app-state /app-state/wireguard /app-state/bird && \
    chmod 2770 /run/peplink-wg-bgp

COPY --from=build /out/peplink-wg-bgp /usr/local/bin/peplink-wg-bgp
COPY docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh
RUN chmod 0755 /usr/local/bin/docker-entrypoint.sh

EXPOSE 8080

ENV APP_CONFIG=/app-state/app.yaml

ENTRYPOINT ["/usr/local/bin/docker-entrypoint.sh"]

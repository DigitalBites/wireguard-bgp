GO_PACKAGES := ./...
APP := ./cmd/peplink-wg-bgp

.PHONY: fmt fmt-check test vet lint build go-check check ci-check ci-tools docker-build docker-build-all image-scan clean

fmt:
	gofmt -w cmd internal web

fmt-check:
	@test -z "$$(gofmt -l cmd internal web)" || (gofmt -l cmd internal web && exit 1)

test:
	go test $(GO_PACKAGES)

vet:
	go vet $(GO_PACKAGES)

lint:
	golangci-lint run $(GO_PACKAGES)

build:
	go build $(APP)

go-check: fmt-check test vet lint build

check: ci-check

ci-tools:
	./scripts/install-ci-tools.sh

ci-check:
	./scripts/ci-check.sh

docker-build:
	./scripts/docker-build.sh

docker-build-all:
	./scripts/docker-build-all.sh

image-scan:
	./scripts/image-scan.sh

clean:
	rm -f peplink-wg-bgp

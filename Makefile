GO_PACKAGES := ./...
APP := ./cmd/peplink-wg-bgp

.PHONY: fmt fmt-check test vet lint build check clean

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

check: fmt-check test vet lint build

clean:
	rm -f peplink-wg-bgp

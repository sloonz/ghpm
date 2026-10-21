.PHONY: build release-dry-run test

BIN ?= ghpm
BUILD_PKG ?= .
GORELEASER ?= goreleaser

build:
	go build -o $(BIN) $(BUILD_PKG)

release-dry-run:
	$(GORELEASER) release --clean --snapshot --skip=publish

test:
	go test -tags=integration ./...

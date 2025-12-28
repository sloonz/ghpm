.PHONY: build release-dry-run test-integration

BIN ?= ghpm
BUILD_PKG ?= .
GORELEASER ?= goreleaser

build:
	go build -o $(BIN) $(BUILD_PKG)

release-dry-run:
	$(GORELEASER) release --clean --snapshot --skip=publish

test-integration:
	go test -tags=integration ./tests

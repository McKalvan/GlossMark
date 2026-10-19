# @gloss configures gmi:GlossCli scope=file
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo 0.0.0-dev)
LDFLAGS := -X github.com/mckalvan/glossmark/internal/version.Version=$(VERSION)

.PHONY: build test lint install release-check

build:
	go build -trimpath -ldflags "$(LDFLAGS)" -o gloss ./cmd/gloss

install:
	go install -trimpath -ldflags "$(LDFLAGS)" ./cmd/gloss

test:
	go vet ./... && go test ./...

lint:
	test -z "$$(gofmt -l .)" || (gofmt -l . && exit 1)

# What the release workflow does, locally: tag vX.Y.Z, then `git push origin vX.Y.Z`.
release-check: lint test
	@echo "version: $(VERSION)"

.PHONY: build install clean lint test help

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
LDFLAGS := -ldflags "-s -w -X github.com/ghostwriter/ghostwriter/internal/cli.Version=$(VERSION) -X github.com/ghostwriter/ghostwriter/internal/cli.Commit=$(COMMIT)"

## build: Build the gw binary
build:
	go build $(LDFLAGS) -o bin/gw ./cmd/gw

## install: Install gw to $GOPATH/bin
install:
	go install $(LDFLAGS) ./cmd/gw

## clean: Remove build artifacts
clean:
	rm -rf bin/

## lint: Run linters
lint:
	golangci-lint run ./...

## test: Run tests
test:
	go test -race -v ./...

## tidy: Tidy go modules
tidy:
	go mod tidy

## help: Show this help
help:
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/^## //' | column -t -s ':'

.PHONY: build test lint clean all cross-compile release

# Version information
VERSION ?= $(shell grep 'var Version' internal/version/version.go | cut -d'"' -f2)
GIT_COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

# Linker flags for version injection
LDFLAGS = -X github.com/trustin/ioetap/internal/version.Version=$(VERSION) \
          -X github.com/trustin/ioetap/internal/version.GitCommit=$(GIT_COMMIT) \
          -X github.com/trustin/ioetap/internal/version.BuildTime=$(BUILD_TIME)

# Default target
all: lint test build

# Build for current platform
build:
	go build -ldflags "$(LDFLAGS)" -o bin/ioetap ./cmd/ioetap

# Run all tests
test:
	go test -v -race ./...

# Run linter
lint:
	@which golangci-lint > /dev/null || (echo "golangci-lint not found. Install with: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest" && exit 1)
	golangci-lint run ./...

# Cross-compile for all supported platforms
cross-compile:
	GOOS=darwin GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o bin/ioetap-darwin-arm64 ./cmd/ioetap
	GOOS=darwin GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o bin/ioetap-darwin-amd64 ./cmd/ioetap
	GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o bin/ioetap-linux-amd64 ./cmd/ioetap
	GOOS=linux GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o bin/ioetap-linux-arm64 ./cmd/ioetap

# Build release binaries with a specific version
# Usage: make release VERSION=1.0.0
release:
ifndef VERSION
	$(error VERSION is required. Usage: make release VERSION=1.0.0)
endif
	@echo "Building release $(VERSION)..."
	GOOS=darwin GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o bin/ioetap-$(VERSION)-darwin-arm64 ./cmd/ioetap
	GOOS=darwin GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o bin/ioetap-$(VERSION)-darwin-amd64 ./cmd/ioetap
	GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o bin/ioetap-$(VERSION)-linux-amd64 ./cmd/ioetap
	GOOS=linux GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o bin/ioetap-$(VERSION)-linux-arm64 ./cmd/ioetap
	@echo "Release binaries created in bin/"

# Clean build artifacts
clean:
	rm -rf bin/
	rm -f *.jsonl

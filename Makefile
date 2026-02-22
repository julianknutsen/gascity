GOLANGCI_LINT_VERSION := 2.9.0

# Detect OS and arch for binary download.
GOOS   := $(shell go env GOOS)
GOARCH := $(shell go env GOARCH)

BIN_DIR := $(shell go env GOPATH)/bin
GOLANGCI_LINT := $(BIN_DIR)/golangci-lint

BINARY     := gc
BUILD_DIR  := bin
INSTALL_DIR := $(HOME)/.local/bin

# Version metadata injected via ldflags.
VERSION    := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT     := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

LDFLAGS := -X main.version=$(VERSION) \
           -X main.commit=$(COMMIT) \
           -X main.date=$(BUILD_TIME)

.PHONY: build check check-all check-bd check-dolt lint fmt-check fmt vet test test-integration test-cover cover install install-tools setup clean

## build: compile gc binary with version metadata
build:
	go build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY) ./cmd/gc
ifeq ($(shell uname),Darwin)
	@codesign -s - -f $(BUILD_DIR)/$(BINARY) 2>/dev/null || true
	@echo "Signed $(BINARY) for macOS"
endif

## install: build and install gc to ~/.local/bin
install: build
	@mkdir -p $(INSTALL_DIR)
	@rm -f $(INSTALL_DIR)/$(BINARY)
	@cp $(BUILD_DIR)/$(BINARY) $(INSTALL_DIR)/$(BINARY)
	@# Remove stale binaries that shadow the canonical location
	@for bad in $(HOME)/go/bin/$(BINARY) $(HOME)/bin/$(BINARY); do \
		if [ -f "$$bad" ]; then \
			echo "Removing stale $$bad (use make install, not go install)"; \
			rm -f "$$bad"; \
		fi; \
	done
	@echo "Installed $(BINARY) to $(INSTALL_DIR)/$(BINARY)"

## clean: remove build artifacts
clean:
	rm -f $(BUILD_DIR)/$(BINARY)

## check: run fast quality gates (pre-commit: unit tests only)
check: fmt-check lint vet test

## check-bd: verify bd (beads CLI) is installed
check-bd:
	@command -v bd >/dev/null 2>&1 || \
		(echo "Error: bd not found. Install beads: cd /data/projects/beads && make install" && exit 1)

## check-dolt: verify dolt is installed
check-dolt:
	@command -v dolt >/dev/null 2>&1 || \
		(echo "Error: dolt not found. Install: https://docs.dolthub.com/introduction/installation" && exit 1)

## check-all: run all quality gates including integration tests (CI)
check-all: fmt-check lint vet check-bd check-dolt test-integration

## lint: run golangci-lint
lint: $(GOLANGCI_LINT)
	$(GOLANGCI_LINT) run ./...

## fmt-check: fail if formatting would change files
fmt-check: $(GOLANGCI_LINT)
	$(GOLANGCI_LINT) fmt --diff ./...

## fmt: auto-fix formatting
fmt: $(GOLANGCI_LINT)
	$(GOLANGCI_LINT) fmt ./...

## vet: run go vet
vet:
	go vet ./...

## test: run unit tests (skip integration tests tagged with //go:build integration)
test:
	go test ./...

## test-integration: run all tests including integration (tmux, etc.)
test-integration:
	go test -tags integration ./...

# Packages for coverage — exclude noise:
#   session/tmux: integration-test-only, not meaningful for unit coverage
#   beadstest: conformance helper, runs under internal/beads coverage
#   internal/dolt: copied gastown code, tested upstream (build tag: doltserver_upstream)
COVER_PKGS := $(shell go list ./... | grep -v -e /session/tmux -e /beadstest -e /internal/dolt)

## test-cover: run all tests with coverage output (excludes tmux)
test-cover:
	go test -tags integration -coverprofile=coverage.txt $(COVER_PKGS)

## cover: run tests and show coverage report
cover: test-cover
	go tool cover -func=coverage.txt

## install-tools: install pinned golangci-lint
install-tools: $(GOLANGCI_LINT)

$(GOLANGCI_LINT):
	@echo "Installing golangci-lint v$(GOLANGCI_LINT_VERSION)..."
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/HEAD/install.sh | \
		sh -s -- -b $(BIN_DIR) v$(GOLANGCI_LINT_VERSION)

## setup: install tools and git hooks
setup: install-tools
	ln -sf ../../scripts/pre-commit .git/hooks/pre-commit
	@echo "Done. Tools installed, pre-commit hook active."

## help: show this help
help:
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## //' | column -t -s ':'

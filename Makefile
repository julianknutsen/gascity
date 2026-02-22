GOLANGCI_LINT_VERSION := 2.9.0

# Detect OS and arch for binary download.
GOOS   := $(shell go env GOOS)
GOARCH := $(shell go env GOARCH)

BIN_DIR := $(shell go env GOPATH)/bin
GOLANGCI_LINT := $(BIN_DIR)/golangci-lint

.PHONY: build check check-all lint fmt-check fmt vet test test-integration test-cover cover install-tools setup

## build: compile gc binary into bin/
build:
	go build -o bin/gc ./cmd/gc

## check: run fast quality gates (pre-commit: unit tests only)
check: fmt-check lint vet test

## check-all: run all quality gates including integration tests (CI)
check-all: fmt-check lint vet test-integration

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

## test-cover: run all tests with coverage output
test-cover:
	go test -tags integration -coverprofile=coverage.txt ./...

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

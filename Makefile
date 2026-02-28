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

.PHONY: build check check-all check-bd check-dolt check-docker lint fmt-check fmt vet test test-integration test-mcp-mail test-docker test-cover cover install install-tools install-buildx setup clean generate check-schema docker-base docker-agent

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

## generate: regenerate JSON schemas and reference docs
generate:
	go run ./cmd/genschema

## check-schema: verify generated docs are up to date
check-schema: generate
	@git diff --exit-code docs/schema/ docs/reference/ || \
		(echo "Error: generated docs stale. Run 'make generate'" && exit 1)

## clean: remove build artifacts
clean:
	rm -f $(BUILD_DIR)/$(BINARY)

## check: run fast quality gates (pre-commit: unit tests only)
check: fmt-check lint vet test

## check-bd: verify bd (beads CLI) is installed
check-bd:
	@command -v bd >/dev/null 2>&1 || \
		(echo "Error: bd not found. Install beads: cd /data/projects/beads && make install" && exit 1)

## check-docker: verify docker and buildx are available
check-docker:
	@command -v docker >/dev/null 2>&1 || \
		(echo "Error: docker not found. Install: https://docs.docker.com/engine/install/" && exit 1)
	@docker buildx version >/dev/null 2>&1 || \
		(echo "Error: docker buildx not found. Run: make install-buildx" && exit 1)

## check-dolt: verify dolt is installed
check-dolt:
	@command -v dolt >/dev/null 2>&1 || \
		(echo "Error: dolt not found. Install: https://docs.dolthub.com/introduction/installation" && exit 1)

## check-all: run all quality gates including integration tests (CI)
check-all: fmt-check lint vet check-bd check-dolt check-docker test-integration

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

## install-buildx: install docker buildx plugin
install-buildx:
	@mkdir -p $(HOME)/.docker/cli-plugins
	curl -sSfL "https://github.com/docker/buildx/releases/download/v0.21.2/buildx-v0.21.2.$$(go env GOOS)-$$(go env GOARCH)" \
		-o $(HOME)/.docker/cli-plugins/docker-buildx
	chmod +x $(HOME)/.docker/cli-plugins/docker-buildx
	@echo "Installed docker-buildx v0.21.2"

## test-mcp-mail: run mcp_agent_mail live conformance test (auto-starts server)
test-mcp-mail:
	GC_TEST_MCP_MAIL=1 go test ./internal/mail/exec/ -run TestMCPMailConformanceLive -v -count=1

## test-docker: run Docker session provider integration tests
test-docker: check-docker
	./scripts/test-docker-session

## setup: install tools and git hooks
setup: install-tools
	ln -sf ../../scripts/pre-commit .git/hooks/pre-commit
	@echo "Done. Tools installed, pre-commit hook active."

## docker-base: build base image with system dependencies (~2.5 min, rebuild rarely)
docker-base: check-docker
	docker build -f contrib/k8s/Dockerfile.base -t gc-agent-base:latest .

## docker-agent: build agent image with project binaries (~5s on top of base)
docker-agent: check-docker
	docker build -f contrib/k8s/Dockerfile.agent -t gc-agent:latest .

## help: show this help
help:
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## //' | column -t -s ':'

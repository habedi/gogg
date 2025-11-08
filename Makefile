# Variables
REPO := github.com/habedi/gogg
BINARY_NAME := $(or $(GOGG_BINARY), $(notdir $(REPO)))
BINARY := bin/$(BINARY_NAME)
COVER_PROFILE := coverage.txt
GO_FILES := $(shell find . -type f -name '*.go')
EXTRA_TMP_FILES := $(shell find . -type f -name 'gogg_*.csv' -o -name 'gogg_*.json' -o -name '*_output.txt')
GO ?= go
MAIN ?= ./main.go
ECHO := @echo
RELEASE_FLAGS := -ldflags="-s -w" -trimpath
FYNE_TAGS := -tags desktop,gl

# Adjust PATH if necessary (append /snap/bin if not present)
PATH := $(if $(findstring /snap/bin,$(PATH)),$(PATH),/snap/bin:$(PATH))

####################################################################################################
## Shell Settings
####################################################################################################
SHELL := /bin/bash
.SHELLFLAGS := -e -o pipefail -c

####################################################################################################
## Go Targets
####################################################################################################

# Default target
.DEFAULT_GOAL := help

help: ## Show this help message
	@echo "Usage: make <target>"
	@echo ""
	@echo "Targets:"
	@grep -E '^[a-zA-Z_-]+:.*## .*$$' Makefile | \
	awk 'BEGIN {FS = ":.*## "}; {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}'

.PHONY: format
format: ## Format Go files
	$(ECHO) "Formatting Go files..."
	@$(GO) fmt ./...

.PHONY: test
test: format ## Run tests with coverage
	$(ECHO) "Running all tests with coverage"
	@$(GO) test -v ./... --cover --coverprofile=$(COVER_PROFILE) --race

.PHONY: showcov
showcov: test ## Display test coverage report
	$(ECHO) "Displaying test coverage report..."
	@$(GO) tool cover -func=$(COVER_PROFILE)

.PHONY: build
build: format ## Build the binary for the current platform
	$(ECHO) "Tidying dependencies..."
	@$(GO) mod tidy
	$(ECHO) "Building the binary..."
	@$(GO) build $(FYNE_TAGS) -o $(BINARY) $(MAIN)
	@$(ECHO) "Build complete: $(BINARY)"

.PHONY: build-macos
build-macos: format ## Build binary for macOS (v14 and newer; arm64)
	$(ECHO) "Tidying dependencies..."
	@$(GO) mod tidy
	$(ECHO) "Building arm64 binary for macOS..."
	@mkdir -p bin
	export CGO_ENABLED=1 ;\
	export CGO_CFLAGS="$$(pkg-config --cflags glfw3)" ;\
	export CGO_LDFLAGS="$$(pkg-config --libs glfw3)" ;\
	GOARCH=arm64 $(GO) build -v $(FYNE_TAGS) -ldflags="-s -w" -o $(BINARY) $(MAIN) ;\
	echo "Build complete: $(BINARY)"

.PHONY: run
run: build ## Build and run the binary
	$(ECHO) "Running the $(BINARY) binary..."
	@./$(BINARY)

.PHONY: clean
clean: ## Remove artifacts and temporary files
	$(ECHO) "Cleaning up..."
	@$(GO) clean #-cache -testcache -modcache
	@find . -type f -name '*.got.*' -delete
	@find . -type f -name '*.out' -delete
	@find . -type f -name '*.snap' -delete
	@rm -f $(COVER_PROFILE)
	@rm -rf bin/
	@rm -f $(EXTRA_TMP_FILES)

####################################################################################################
## Dependency & Lint Targets
####################################################################################################

.PHONY: install-snap
install-snap: ## Install Snap (for Debian-based systems)
	$(ECHO) "Installing Snap..."
	@sudo apt-get update
	@sudo apt-get install -y snapd
	@sudo snap refresh

.PHONY: install-deps
install-deps: ## Install development dependencies (for Debian-based systems)
	$(ECHO) "Installing dependencies..."
	@sudo apt-get update
	@sudo apt-get install -y make libgl1-mesa-dev libx11-dev xorg-dev \
	libxcursor-dev libxrandr-dev libxinerama-dev libxi-dev pkg-config libasound2-dev
	@$(MAKE) install-snap
	@sudo snap install go --classic
	@sudo snap install golangci-lint --classic
	@$(GO) install mvdan.cc/gofumpt@latest
	@$(GO) install honnef.co/go/tools/cmd/staticcheck@latest
	@$(GO) install github.com/google/pprof@latest
	@$(GO) mod download

.PHONY: lint
lint: format ## Run linter checks
	$(ECHO) "Running go vet..."
	@$(GO) vet ./...
	$(ECHO) "Running staticcheck..."
	@command -v staticcheck >/dev/null 2>&1 && staticcheck ./... || echo "staticcheck not installed"
	$(ECHO) "Running golangci-lint..."
	@golangci-lint run ./...

.PHONY: gofumpt
gofumpt: ## Run gofumpt to format Go files
	$(ECHO) "Running gofumpt for formatting..."
	@gofumpt -l -w .

.PHONY: release
release: ## Build the release binary for current platform (Linux and Windows)
	$(ECHO) "Tidying dependencies..."
	@$(GO) mod tidy
	$(ECHO) "Building the release binary..."
	@$(GO) build $(RELEASE_FLAGS) $(FYNE_TAGS) -o $(BINARY) $(MAIN)
	@$(ECHO) "Build complete: $(BINARY)"

.PHONY: release-headless
release-headless: ## Build headless (CLI-only) release without GUI
	$(ECHO) "Tidying dependencies..."
	@$(GO) mod tidy
	$(ECHO) "Building headless release binary (CLI-only, no GUI)..."
	@$(GO) build $(RELEASE_FLAGS) -tags headless -o $(BINARY) $(MAIN)
	@$(ECHO) "Build complete: $(BINARY)"

.PHONY: release-macos
release-macos: ## Build release binary for macOS (v14 and newer; arm64)
	$(ECHO) "Tidying dependencies..."
	@$(GO) mod tidy
	$(ECHO) "Building arm64 release binary for macOS..."
	@mkdir -p bin
	export CGO_ENABLED=1 ;\
	export CGO_CFLAGS="$$(pkg-config --cflags glfw3)" ;\
	export CGO_LDFLAGS="$$(pkg-config --libs glfw3)" ;\
	GOARCH=arm64 $(GO) build $(RELEASE_FLAGS) $(FYNE_TAGS) -o $(BINARY) $(MAIN) ;\
	echo "Build complete: $(BINARY)"

.PHONY: setup-hooks
setup-hooks: ## Install Git hooks (pre-commit and pre-push)
	@echo "Setting up Git hooks..."
	@if ! command -v pre-commit &> /dev/null; then \
	   echo "pre-commit not found. Please install it using 'pip install pre-commit'"; \
	   exit 1; \
	fi
	@pre-commit install --hook-type pre-commit
	@pre-commit install --hook-type pre-push
	@pre-commit install-hooks

.PHONY: test-hooks
test-hooks: ## Test Git hooks on all files
	@echo "Testing Git hooks..."
	@pre-commit run --all-files --show-diff-on-failure --color=always

.PHONY: test-integration
test-integration: ## Run integration tests (needs network-mocked local servers)
	$(ECHO) "Running integration tests"
	@$(GO) test -tags=integration ./...

.PHONY: test-fuzz
test-fuzz: ## Run fuzz tests
	$(ECHO) "Running fuzz tests (short)"
	@$(GO) test ./client -run=^$$ -fuzz=FuzzParseSizeString -fuzztime=10s
	@$(GO) test ./client -run=^$$ -fuzz=FuzzParseGameData -fuzztime=10s

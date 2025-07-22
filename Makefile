# Variables
REPO := github.com/habedi/gogg
BINARY_NAME := $(or $(GOGG_BINARY), $(notdir $(REPO)))
BINARY := bin/$(BINARY_NAME)
COVER_PROFILE := coverage.txt
GO_FILES := $(shell find . -type f -name '*.go')
COVER_FLAGS := --cover --coverprofile=$(COVER_PROFILE)
EXTRA_TMP_FILES := $(shell find . -type f -name 'gogg_catalogue_*.csv')
GO ?= go
MAIN ?= ./main.go
ECHO := @echo
RELEASE_FLAGS := -ldflags="-s -w" -trimpath

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
test: format ## Run the tests
	$(ECHO) "Running non-UI tests..."
	@$(GO) test -v $(shell $(GO) list ./... | grep -v './gui') --cover --coverprofile=non_ui_coverage.out --race
	$(ECHO) "Running UI tests (individually to avoid race conditions)..."
	@$(GO) test -v -p 1 -run ^TestCatalogueListUI$ ./gui/... --cover --coverprofile=ui_coverage_1.out --race
	@$(GO) test -v -p 1 -run ^TestSearchCatalogueUI$ ./gui/... --cover --coverprofile=ui_coverage_2.out --race
	@$(GO) test -v -p 1 -run ^TestRefreshCatalogueUI$ ./gui/... --cover --coverprofile=ui_coverage_3.out --race
	@$(GO) test -v -p 1 -run ^TestExportCatalogueUI$ ./gui/... --cover --coverprofile=ui_coverage_4.out --race
	$(ECHO) "Merging coverage reports..."
	@echo "mode: set" > $(COVER_PROFILE)
	@tail -n +2 -q non_ui_coverage.out ui_coverage_*.out >> $(COVER_PROFILE)
	@rm -f non_ui_coverage.out ui_coverage_*.out

.PHONY: showcov
showcov: test ## Display test coverage report
	$(ECHO) "Displaying test coverage report..."
	@$(GO) tool cover -func=$(COVER_PROFILE)

.PHONY: build
build: format ## Build the binary for the current platform (Linux and Windows)
	$(ECHO) "Tidying dependencies..."
	@$(GO) mod tidy
	$(ECHO) "Building the binary..."
	@$(GO) build -o $(BINARY)
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
	GOARCH=arm64 $(GO) build -v -tags desktop,gl -ldflags="-s -w" -o $(BINARY) $(MAIN) ;\
	echo "Build complete: $(BINARY)"

.PHONY: run
run: build ## Build and run the binary
	$(ECHO) "Running the $(BINARY) binary..."
	@./$(BINARY)

.PHONY: clean
clean: ## Remove artifacts and temporary files
	$(ECHO) "Cleaning up..."
	@$(GO) clean -cache -testcache -modcache
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
	@sudo apt-get install -y make libgl1-mesa-dev libx11-dev xorg-dev \
	libxcursor-dev libxrandr-dev libxinerama-dev libxi-dev pkg-config
	@$(MAKE) install-snap
	@sudo snap install go --classic
	@sudo snap install golangci-lint --classic
	@$(GO) install mvdan.cc/gofumpt@latest
	@$(GO) install github.com/google/pprof@latest
	@$(GO) mod download

.PHONY: lint
lint: format ## Run the linters
	$(ECHO) "Linting Go files..."
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
	@$(GO) build $(RELEASE_FLAGS) -o $(BINARY) $(MAIN)
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
	GOARCH=arm64 $(GO) build $(RELEASE_FLAGS) -o $(BINARY) $(MAIN) ;\
	echo "Build complete: $(BINARY)"

.PHONY: setup-hooks
setup-hooks: ## Set up pre-commit hooks
	@echo "Setting up pre-commit hooks..."
	@if ! command -v pre-commit &> /dev/null; then \
	   echo "pre-commit not found. Please install it using 'pip install pre-commit'"; \
	   exit 1; \
	fi
	@pre-commit install --install-hooks

.PHONY: test-hooks
test-hooks: ## Test pre-commit hooks on all files
	@echo "Testing pre-commit hooks..."
	@pre-commit run --all-files

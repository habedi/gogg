## Variables
REPO := github.com/habedi/gogg
BINARY_NAME := $(or $(GOGG_BINARY), $(notdir $(REPO)))
BINARY := bin/$(BINARY_NAME)
COVER_PROFILE := coverage.txt
GO_FILES := $(shell find . -type f -name '*.go')
COVER_FLAGS := --cover --coverprofile=$(COVER_PROFILE)
GO ?= go
MAIN ?= ./main.go
ECHO := @echo

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

.PHONY: help
help: ## Show the help message for each target
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; \
	  {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

.PHONY: format
format: ## Format Go files
	$(ECHO) "Formatting Go files..."
	@$(GO) fmt ./...

.PHONY: test
test: format ## Run the tests
	$(ECHO) "Running tests..."
	@$(GO) test -v ./... $(COVER_FLAGS) --race

.PHONY: showcov
showcov: test ## Display test coverage report
	$(ECHO) "Displaying test coverage report..."
	@$(GO) tool cover -func=$(COVER_PROFILE)

.PHONY: build
build: format ## Build the binary for the current platform
	$(ECHO) "Tidying dependencies..."
	@$(GO) mod tidy
	$(ECHO) "Building the project..."
	@$(GO) build -o $(BINARY)

.PHONY: build-macos
build-macos: format ## Build a universal binary for macOS (x86_64 and arm64)
	$(ECHO) "Building universal binary for macOS..."
	@mkdir -p bin
	@GOARCH=amd64 $(GO) build -o bin/$(BINARY_NAME)-x86_64 $(MAIN)
	@GOARCH=arm64 $(GO) build -o bin/$(BINARY_NAME)-arm64 $(MAIN)
	@command -v lipo >/dev/null || { $(ECHO) "lipo not found. Please install Xcode command line tools."; exit 1; }
	@lipo -create -output $(BINARY) bin/$(BINARY_NAME)-x86_64 bin/$(BINARY_NAME)-arm64

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

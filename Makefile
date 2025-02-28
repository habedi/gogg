# Variables
PKG = github.com/habedi/gogg
BINARY_NAME = $(or $(GOGG_BINARY), $(notdir $(PKG)))
BINARY = bin/$(BINARY_NAME)
COVER_PROFILE = coverage.txt
GO_FILES = $(shell find . -type f -name '*.go')
COVER_FLAGS = --cover --coverprofile=$(COVER_PROFILE)
CUSTOM_SNAPCRAFT_BUILD_ENVIRONMENT = $(or $(SNAP_BACKEND), multipass)
PATH := /snap/bin:$(PATH)

################################################################################
## Go Targets
################################################################################

# Default target
.DEFAULT_GOAL := help

.PHONY: help
help: ## Show this help message
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

.PHONY: build
build: format ## Build the binary for the current platform
	@echo "Tidying dependencies..."
	@go mod tidy
	@echo "Building the project..."
	@go build -o $(BINARY)

.PHONY: format
format: ## Format Go files
	@echo "Formatting Go files..."
	@go fmt ./...

.PHONY: test
test: format ## Run the tests
	@echo "Running tests..."
	@go test -v ./... $(COVER_FLAGS) --race

.PHONY: showcov
showcov: test ## Display test coverage report
	@echo "Displaying test coverage report..."
	@go tool cover -func=$(COVER_PROFILE)

.PHONY: clean
clean: ## Remove artifacts and temporary files
	@echo "Cleaning up..."
	@find . -type f -name '*.got.*' -delete
	@find . -type f -name '*.out' -delete
	@find . -type f -name '*.snap' -delete
	@rm -f $(COVER_PROFILE)
	@rm -rf bin/

.PHONY: run
run: build ## Build and run the binary
	@echo "Running the $(BINARY) binary..."
	@./$(BINARY)

.PHONY: build-macos
build-macos: format ## Build a universal binary for macOS (x86_64 and arm64)
	@echo "Building universal binary for macOS..."
	@mkdir -p bin
	@GOARCH=amd64 go build -o bin/$(BINARY_NAME)-x86_64 ./main.go
	@GOARCH=arm64 go build -o bin/$(BINARY_NAME)-arm64 ./main.go
	@lipo -create -output $(BINARY) bin/$(BINARY_NAME)-x86_64 bin/$(BINARY_NAME)-arm64

.PHONY: snap-deps
snap-deps: ## Install Snapcraft dependencies
	@echo "Installing Snapcraft dependencies..."
	@sudo apt-get update
	@sudo apt-get install -y snapd
	@sudo snap refresh
	@sudo snap install snapcraft --classic
	@sudo snap install multipass --classic

.PHONY: install-deps
install-deps: ## Install development dependencies on Debian-based systems
	@echo "Installing dependencies..."
	@make snap-deps
	@sudo apt-get install -y chromium-browser build-essential chromium || true # ignore errors
	@sudo snap install chromium
	@sudo snap install go --classic
	@sudo snap install golangci-lint --classic
	@go mod download

.PHONY: lint
lint: format ## Run the linters
	@echo "Linting Go files..."
	@golangci-lint run ./...

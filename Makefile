# Declare PHONY targets
.PHONY: build format test test-cover clean run build-macos help snap-deps snap install-deps

# Variables
PKG = github.com/habedi/gogg
BINARY_NAME = $(or $(GOGG_BINARY), $(notdir $(PKG)))
BINARY = bin/$(BINARY_NAME)
COVER_PROFILE = cover.out
GO_FILES = $(shell find . -type f -name '*.go')
COVER_FLAGS = --cover --coverprofile=$(COVER_PROFILE)
CUSTOM_SNAPCRAFT_BUILD_ENVIRONMENT = $(or $(SNAP_BACKEND), multipass)

# Help target
help:
	@echo "Available targets:"
	@echo ""
	@echo "  build            Build the Go project"
	@echo "  format           Format Go files"
	@echo "  test             Run tests"
	@echo "  test-cover       Run tests with coverage report"
	@echo "  clean            Clean generated and temporary files"
	@echo "  run              Build and run the project"
	@echo "  build-macos      Build a universal binary for macOS"
	@echo "  snap-deps        Install Snapcraft dependencies"
	@echo "  snap             Build the Snap package"
	@echo "  install-deps     Install development dependencies on Debian-based systems"
	@echo "  lint             Lint Go files to check for potential errors"
	@echo "  codecov          Create test coverage report for Codecov"
	@echo "  help             Show this help message"

# Building the project
build: format
	@echo "Tidying dependencies..."
	go mod tidy
	@echo "Building the project..."
	go build -o $(BINARY)

# Formatting Go files
format:
	@echo "Formatting Go files..."
	go fmt ./...

# Running tests
test: format
	@echo "Running tests..."
	go test -v ./... --race

test-cover: format
	@echo "Running tests with coverage..."
	go test -v ./... --race $(COVER_FLAGS)
	@echo "Generating HTML coverage report..."
	go tool cover -html=$(COVER_PROFILE)

# Cleaning generated and temporary files
clean:
	@echo "Cleaning up..."
	find . -type f -name '*.got.*' -delete
	find . -type f -name '*.out' -delete
	find . -type f -name '*.snap' -delete
	rm -rf bin/

# Running the built executable
run: build
	@echo "Running the project..."
	./$(BINARY)

# Custom build for macOS
build-macos: format
	@echo "Building universal binary for macOS..."
	mkdir -p bin
	GOARCH=amd64 go build -o bin/$(BINARY_NAME)-x86_64 ./main.go
	GOARCH=arm64 go build -o bin/$(BINARY_NAME)-arm64 ./main.go
	lipo -create -output $(BINARY) bin/$(BINARY_NAME)-x86_64 bin/$(BINARY_NAME)-arm64

# Install Snapcraft Dependencies
snap-deps:
	@echo "Installing Snapcraft dependencies..."
	sudo apt-get update
	sudo apt-get install -y snapd
	sudo snap refresh
	sudo snap install snapcraft --classic
	sudo snap install multipass --classic

# Build Snap package
snap: build # snap-deps
	@echo "Building Snap package..."
	SNAPCRAFT_BUILD_ENVIRONMENT=$(CUSTOM_SNAPCRAFT_BUILD_ENVIRONMENT) snapcraft

# Install Dependencies on Debian-based systems like Ubuntu
install-deps:
	@echo "Installing dependencies..."
	make snap-deps
	sudo apt-get install -y chromium-browser build-essential
	sudo snap install chromium
	sudo snap install go --classic
	sudo snap install golangci-lint --classic

# Linting Go files
lint:
	@echo "Linting Go files..."
	golangci-lint run ./...

# Create Test Coverage Report for Codecov
codecov: format
	@echo "Uploading coverage report to Codecov..."
	go test -coverprofile=coverage.txt ./...

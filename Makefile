# Declare PHONY targets
.PHONY: test test-cover format build clean

# Variables
PKG = github.com/habedi/template-go-project
COVER_PROFILE = cover.out
GO_FILES = $(shell find . -type f -name '*.go')
COVER_FLAGS = --cover --coverprofile=$(COVER_PROFILE)

# Test with coverage and view HTML report
test-cover: format
	@echo "Running tests with coverage..."
	go test -v ./... --race $(COVER_FLAGS)
	go tool cover -html=$(COVER_PROFILE)

# Format Go files
format:
	@echo "Formatting Go files..."
	go fmt ./...

# Run tests
test: format
	@echo "Running tests..."
	go test -v ./... --race

# Build the project's executable
build: format
	@echo "Tidying dependencies..."
	go mod tidy
	@echo "Building the project..."
	go build -o bin/$(notdir $(PKG))

# Clean temporary and output files
clean:
	@echo "Cleaning up..."
	find . -type f -name '*.got.*' -delete
	find . -type f -name '*.out' -delete
	rm -rf bin/

# Run the built executable
run: build
	@echo "Running the project..."
	./bin/$(notdir $(PKG))

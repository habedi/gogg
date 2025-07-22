#!/bin/bash

# test_gogg_cli.sh - A script to test the CLI API of gogg

# Set colors for better readability
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Find the gogg executable
GOGG=$(command -v bin/gogg || command -v gogg || command -v ./gogg)

if [ -z "$GOGG" ]; then
    echo "Error: gogg executable not found. Make sure it's in your PATH or in the current directory."
    exit 1
fi

echo -e "${GREEN}=== Testing gogg CLI API ===${NC}"

# Function to run a test and report result
run_test() {
    local test_name="$1"
    local command="$2"

    echo -e "\n${YELLOW}Testing: ${test_name}${NC}"
    echo -e "${BLUE}Command: ${command}${NC}"

    # Run the command and capture exit code
    eval "$command"
    local exit_code=$?

    if [ $exit_code -eq 0 ]; then
        echo -e "${GREEN}✓ Test passed (exit code: $exit_code)${NC}"
    else
        echo -e "\033[0;31m✗ Test failed (exit code: $exit_code)\033[0m"
    fi

    # Add a small delay between tests
    sleep 1
}

# Test 1: Help command
run_test "Help command" "$GOGG --help"

# Test 2: Version command
run_test "Version command" "$GOGG version"

# Test 3: Catalogue commands
run_test "Catalogue help" "$GOGG catalogue --help"
run_test "Catalogue refresh" "$GOGG catalogue refresh"
run_test "Catalogue search" "$GOGG catalogue search 'Witcher'"
run_test "Catalogue export" "$GOGG catalogue export ./ --format=csv"

# Test 4: File commands
run_test "File help" "$GOGG file --help"
if [ -d "./games" ]; then
    run_test "File hash" "$GOGG file hash ./games --algo=md5"
    # Get a game ID from the catalogue if available
    GAME_ID=$(tail -n +2 gogg_catalogue_*.csv 2>/dev/null | head -n 1 | cut -d, -f1)
    if [ -n "$GAME_ID" ]; then
        run_test "File size" "$GOGG file size $GAME_ID --platform=windows --lang=en"
    fi
else
    echo -e "\033[0;33mSkipping file hash test as ./games directory doesn't exist${NC}"
fi

# Test 5: Download command (with --dry-run if available to avoid actual downloads)
run_test "Download help" "$GOGG download --help"
# Get a game ID from the catalogue if available
GAME_ID=$(tail -n +2 gogg_catalogue_*.csv 2>/dev/null | head -n 1 | cut -d, -f1)
if [ -n "$GAME_ID" ]; then
    # Check if --dry-run is supported
    if $GOGG download --help | grep -q "dry-run"; then
        run_test "Download dry run" "$GOGG download $GAME_ID ./games --platform=windows --lang=en --dry-run=true"
    else
        echo -e "\033[0;33mSkipping download test with actual game ID as --dry-run is not supported${NC}"
    fi
else
    # Use a sample ID for testing
    run_test "Download command syntax" "$GOGG download 1234567890 ./games --platform=windows --lang=en --threads=4 --dlcs=true --extras=false --resume=true --flatten=true"
fi

# Test 6: GUI command (just check if it exists, don't actually launch it)
if $GOGG --help | grep -q "gui"; then
    run_test "GUI help" "$GOGG gui --help"
else
    echo -e "\033[0;33mSkipping GUI test as the command doesn't exist${NC}"
fi

# Test 7: Login command (just check help, don't actually login)
if $GOGG --help | grep -q "login"; then
    run_test "Login help" "$GOGG login --help"
else
    echo -e "\033[0;33mSkipping login test as the command doesn't exist${NC}"
fi

echo -e "\n${GREEN}=== All tests completed ===${NC}"

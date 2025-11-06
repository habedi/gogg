#!/bin/bash

# Exit immediately if a command exits with a non-zero status.
set -e

# --- Colors for logging ---
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# --- Helper function to run and log a test ---
run_test() {
  local target="$1"
  local description="$2"
  echo -e "\n${YELLOW}---> Testing 'make ${target}': ${description}...${NC}"
  make "${target}"
  echo -e "${GREEN}---> 'make ${target}' successful.${NC}"
}

# --- Main test execution ---
echo -e "${YELLOW}===== Starting Makefile Test Suite =====${NC}"

run_test "clean" "Removing old build artifacts"
run_test "format" "Formatting Go files"
run_test "lint" "Running linter checks"
run_test "test" "Running unit tests"
run_test "test-integration" "Running integration tests"
run_test "test-fuzz" "Running fuzz tests"

# --- Final Success Message ---
echo -e "\n${GREEN}===== Makefile Test Suite Completed Successfully! =====${NC}"

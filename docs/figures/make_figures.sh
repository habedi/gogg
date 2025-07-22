#!/bin/bash

# You need to have Graphviz installed to run this script
# On Debian-based OSes, you can install it using: sudo apt-get install graphviz

# Directory containing .dot files (with default value)
ASSET_DIR=${1:-"."}

# Make figures from .dot files
for f in "${ASSET_DIR}"/*.dot; do
    dot -Tsvg "$f" -o "${f%.dot}.svg"
done

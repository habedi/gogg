#!/bin/bash

# This script removes unused JetBrains Mono font files from the assets directory.

# --- Configuration ---
# The directory where the .ttf files are located.
FONT_DIR="gui/assets/JetBrainsMono-2.304/fonts/ttf"

# An array of the font files we want to KEEP. All other .ttf files will be removed.
KEEP_FILES=(
  "JetBrainsMono-Regular.ttf"
  "JetBrainsMono-Bold.ttf"
)

# --- Safety Check ---
if [ ! -d "$FONT_DIR" ]; then
  echo "Error: Font directory not found at '$FONT_DIR'"
  exit 1
fi

echo "Scanning for unused fonts in '$FONT_DIR'..."
echo "Keeping the following files:"
for file_to_keep in "${KEEP_FILES[@]}"; do
  echo " - $file_to_keep"
done
echo "---"

# --- Main Logic ---
# Find all .ttf files and check them against the KEEP_FILES list.
# The 'find' command is robust and handles spaces or special characters in filenames.
find "$FONT_DIR" -name "*.ttf" | while read -r filepath; do
  filename=$(basename "$filepath")

  # Assume we should remove it unless we find it in the keep list.
  should_keep=false
  for file_to_keep in "${KEEP_FILES[@]}"; do
    if [[ "$filename" == "$file_to_keep" ]]; then
      should_keep=true
      break
    fi
  done

  # If the file is not in the keep list, delete it.
  if [ "$should_keep" = false ]; then
    echo "Removing unused font: $filename"
    rm "$filepath"
  fi
done

echo "---"
echo "Font cleanup complete."

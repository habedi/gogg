#!/bin/bash

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${GREEN}=========================== Download All Games (Linux Version) =============================${NC}"
echo -e "${GREEN}The code in this script downloads all games owned by the user on GOG.com with given options.${NC}"
echo -e "${GREEN}============================================================================================${NC}"

DEBUG_MODE=1 # Debug mode enabled
GOGG=$(command -v bin/gogg || command -v gogg)

LANG=en # Language English
PLATFORM=windows # Platform Windows
INCLUDE_DLC=1 # Include DLCs
INCLUDE_EXTRA_CONTENT=1 # Include extra content
RESUME_DOWNLOAD=1 # Resume download
NUM_THREADS=4 # Number of worker threads for downloading

# Function to clean up the CSV file
cleanup() {
    if [ -n "$latest_csv" ]; then
        rm -f "$latest_csv"
        if [ $? -eq 0 ]; then
            echo -e "${RED}Cleanup: removed $latest_csv${NC}"
        fi
    fi
}

# Trap SIGINT (Ctrl+C) and call cleanup
trap cleanup SIGINT

# Update game catalogue and export it to a CSV file
$GOGG catalogue refresh
$GOGG catalogue export --format csv --dir ./

# Find the newest catalogue file
latest_csv=$(ls -t gogg_catalogue_*.csv 2>/dev/null | head -n 1)

# Check if the catalogue file exists
if [ -z "$latest_csv" ]; then
    echo -e "${RED}No CSV file found.${NC}"
    exit 1
fi

echo -e "${GREEN}Using catalogue file: $latest_csv${NC}"

# Download each game listed in catalogue file, skipping the first line
tail -n +2 "$latest_csv" | while IFS=, read -r game_id game_title; do
    echo -e "${YELLOW}Game ID: $game_id, Title: $game_title${NC}"
    DEBUG_GOGG=$DEBUG_MODE $GOGG download --id $game_id --dir ./games --platform $PLATFORM --lang $LANG \
        --dlcs $INCLUDE_DLC --extras $INCLUDE_EXTRA_CONTENT --resume $RESUME_DOWNLOAD --threads $NUM_THREADS
    sleep 1
    #break # Comment out this line to download all games
done

# Clean up
cleanup

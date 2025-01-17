#!/bin/bash

## Simple examples of using Gogg commands

GOGG=$(command -v bin/gogg || command -v gogg)

# Show the Gogg version
$GOGG version

# Initialize Gogg's internal database and ask for user's credentials
$GOGG init

# Login to GOG.com (headless mode; no browser window is opened)
$GOGG login --show=false

# Update game catalogue with the data from GOG.com
$GOGG catalogue refresh --threads 10

# Search for games with specific terms in their titles
$GOGG catalogue search --term "Witcher"
$GOGG catalogue search --term "mess"

# Download a specific game ("The Messenger") with the given options
$GOGG download --id 1433116924 --dir ./games --platform all --lang en --threads 3 \
    --dlcs false --extras false --resume false

# Show the downloaded game files
tree ./games

#!/bin/bash

echo "Sample script to demonstrate Gogg's basic functionalities"
sleep 1

# Find the Gogg executable
GOGG=$(command -v bin/gogg || command -v gogg || command -v ./gogg)

echo "Show Gogg's top-level commands"
$GOGG --help
sleep 1

echo "Show the version"
$GOGG version
sleep 1

#echo "Login to GOG.com"
#$GOGG login
#sleep 1

echo "Update game catalogue with the data from GOG.com"
$GOGG catalogue refresh
sleep 1

echo "Search for games with specific terms in their titles"
$GOGG catalogue search "Witcher"
$GOGG catalogue search "mess"

echo "Download a specific game (\"The Messenger\") with the given options"
$GOGG download 1433116924 ./games --platform=all --lang=en --threads=4 \
    --dlcs=true --extras=false --resume=true --flatten=true

echo "Show the downloaded game files"
tree ./games

echo "Display hash values of the downloaded game files"
$GOGG file hash ./games --algo=md5

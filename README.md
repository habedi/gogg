<div align="center">
  <picture>
    <source media="(prefers-color-scheme: light)" srcset="logo.jpeg">
    <source media="(prefers-color-scheme: dark)" srcset="logo.jpeg">
    <img alt="Gogg logo" src="logo.jpeg" height="40%" width="40%">
  </picture>
</div>
<br>

<p align="center">
  <a href="https://github.com/habedi/gogg/actions/workflows/tests.yml">
    <img src="https://github.com/habedi/gogg/actions/workflows/tests.yml/badge.svg" alt="Tests">
  </a>
  <a href="https://github.com/habedi/gogg/actions/workflows/build_linux.yml">
    <img src="https://github.com/habedi/gogg/actions/workflows/build_linux.yml/badge.svg" alt="Linux Build">
  </a>
  <a href="https://github.com/habedi/gogg/actions/workflows/build_windows.yml">
    <img src="https://github.com/habedi/gogg/actions/workflows/build_windows.yml/badge.svg" alt="Windows Build">
  </a>
  <a href="https://github.com/habedi/gogg/actions/workflows/build_macos.yml">
    <img src="https://github.com/habedi/gogg/actions/workflows/build_macos.yml/badge.svg" alt="MacOS Build">
  </a>
  <br>
  <a href="https://goreportcard.com/report/github.com/habedi/gogg">
  <img src="https://goreportcard.com/badge/github.com/habedi/gogg" alt="Go Report Card">
  </a>
  <a href="https://pkg.go.dev/github.com/habedi/gogg">
    <img src="https://pkg.go.dev/badge/github.com/habedi/gogg.svg" alt="Go Reference">
  </a>
  <a href="https://codecov.io/gh/habedi/gogg">
    <img src="https://codecov.io/gh/habedi/gogg/graph/badge.svg?token=1RUL13T0VE" alt="Code Coverage">
  </a>
  <a href="https://www.codefactor.io/repository/github/habedi/gogg">
    <img src="https://www.codefactor.io/repository/github/habedi/gogg/badge" alt="CodeFactor">
  </a>
  <a href="https://github.com/habedi/gogg/releases/latest">
    <img src="https://img.shields.io/github/release/habedi/gogg.svg?style=flat-square" alt="Release">
  </a>
  <a href="https://github.com/habedi/gogg/releases">
  <img src="https://img.shields.io/github/downloads/habedi/gogg/total.svg" alt="Total Downloads">
  </a>
</p>

# Gogg

Gogg is a minimalistic command-line tool for downloading game files from [GOG.com](https://www.gog.com/).
It is written in [Go](https://golang.org/) and uses the
official [GOG API](https://gogapidocs.readthedocs.io/en/latest/index.html).

The main goal of Gogg is to provide a simple and easy-to-use interface for people who want to download their GOG games
for offline use or archival purposes.

## Features

Main features of Gogg:

- It can be used to fully automate the download process with a few simple commands.
- It can run anywhere (Windows, macOS, or Linux) that a Go compiler is available.

Additionally, it allows users to perform the following actions:

- List owned games
- Export the list of owned games to a file
- Search in the owned games
- Download game files (like installers, patches, and bonus content)
- Filter files to be downloaded by platform, language, and other attributes like content type
- Download files using multiple threads to speed up the process
- Resume interrupted downloads and only download missing or newer files
- Verify the integrity of downloaded files by calculating their hashes

## Getting Started

See the [documentation](docs/README.md) for how to install and use Gogg.

Run `gogg -h` to see the available commands and options.

### Examples

For more detailed examples, see the content of the [examples](docs/examples/) directory.

#### Login to GOG

```bash
# First-time using Gogg, you need to login to GOG to authenticate
gogg login
```

> [!IMPORTANT]
> You need to have [Google Chrome](https://www.google.com/chrome/) or [Chromium](https://www.chromium.org/) installed on
> your machine for the first-time authentication.
> So, make sure you have one of them installed and available in your system's PATH.

#### Syncing the Game Catalogue

```bash
# Will fetch the up-to-date information about the games you own on GOG
gogg catalogue refresh
```

#### Searching for Games

```bash
# Will show the game ID and title of the games that contain "Witcher" in their title
gogg catalogue search "Witcher"
```

#### Downloading a Game

```bash
# Will download the files for `The Witcher: Enhanced Edition` to `./games` directory (without extra content)
gogg download 1207658924 ./games --platform=windows --lang=en --dlcs=true --extras=false \
 --resume=true --threads 5 --flatten=true
```

#### File Hashes (For Verification)

```bash
# Will show the SHA1 hash of the downloaded files for `The Witcher: Enhanced Edition`
gogg file hash ./games/the-witcher-enhanced-edition --algo=sha1
```

## Demo

[![asciicast](https://asciinema.org/a/kXMGRUUV149R37IEmZKtTH7nI.svg)](https://asciinema.org/a/kXMGRUUV149R37IEmZKtTH7nI)

## Contributing

Please see the [CONTRIBUTING.md](CONTRIBUTING.md) file for information on how to contribute to Gogg.

<div align="center">
  <picture>
    <source media="(prefers-color-scheme: light)" srcset="assets/logo-v1.jpeg">
    <source media="(prefers-color-scheme: dark)" srcset="assets/logo-v1.jpeg">
    <img alt="Gogg logo" src="assets/logo-v1.jpeg" height="35%" width="35%">
  </picture>
</div>
<br>

<p align="center">
  <a href="https://github.com/habedi/gogg/actions/workflows/tests.yml">
    <img src="https://github.com/habedi/gogg/actions/workflows/tests.yml/badge.svg" alt="Tests">
  </a>
  <a href="https://goreportcard.com/report/github.com/habedi/gogg">
    <img src="https://goreportcard.com/badge/github.com/habedi/gogg" alt="Go Report Card">
  </a>
  <a href="https://pkg.go.dev/github.com/habedi/gogg">
    <img src="https://pkg.go.dev/badge/github.com/habedi/gogg.svg" alt="Go Reference">
  </a>
  <a href="https://github.com/habedi/gogg/releases/latest">
    <img src="https://img.shields.io/github/release/habedi/gogg.svg?style=flat-square" alt="Release">
  </a>
  <a href="https://snapcraft.io/gogg">
  <img src="https://snapcraft.io/gogg/badge.svg" alt="Snap Store">
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
- Resume interrupted downloads

## Getting Started

See the [documentation](docs/README.md) for how to install and use Gogg.

### Examples

For more detailed examples, see the content of the [examples](docs/examples/) directory.

#### First Time Setup

```bash
# Will ask for the GOG username and password
gogg init
```

#### Authentication with GOG

```bash
# On first time try will open a browser window to authenticate with GOG,
# afterwards running `auth` will reauthenticate with GOG without opening a browser window
gogg auth
```

> You must have [Google Chrome](https://www.google.com/chrome/) or [Chromium](https://www.chromium.org/) installed
on your machine for the first-time authentication. 
So, make sure you have one of them installed.

#### Syncing the Game Catalogue

```bash
# Will fetch the up-to-date information about the games you own on GOG
gogg catalogue refresh
```

#### Downloading a Game

```bash
# Will download the game files for `The Witcher: Enhanced Edition` to `./games` directory
gogg download --id 1207658924 --dir ./games --platform windows --lang en --dlcs true --extras true --resume true --threads 5
```

## Bugs and Features

Use the [issue tracker](github.com/habedi/gogg/issues) to report a bug or ask for a feature.

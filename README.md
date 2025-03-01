<div align="center">
  <picture>
    <source media="(prefers-color-scheme: light)" srcset="logo.jpeg">
    <source media="(prefers-color-scheme: dark)" srcset="logo.jpeg">
    <img alt="Gogg logo" src="logo.jpeg" height="40%" width="40%">
  </picture>
</div>
<br>

<div align="center">
    <a href="https://github.com/habedi/gogg/actions/workflows/tests.yml">
        <img src="https://img.shields.io/github/actions/workflow/status/habedi/gogg/tests.yml?label=tests&style=flat&labelColor=555555&logo=github" alt="Tests">
    </a>
    <a href="https://github.com/habedi/gogg/actions/workflows/lints.yml">
        <img src="https://img.shields.io/github/actions/workflow/status/habedi/gogg/lints.yml?label=linters&style=flat&labelColor=555555&logo=github" alt="Linters">
    </a>
    <a href="https://github.com/habedi/gogg/actions/workflows/build_linux.yml">
        <img src="https://img.shields.io/github/actions/workflow/status/habedi/gogg/build_linux.yml?label=linux%20build&style=flat&labelColor=555555&logo=linux" alt="Linux Build">
    </a>
    <a href="https://github.com/habedi/gogg/actions/workflows/build_windows.yml">
        <img src="https://img.shields.io/github/actions/workflow/status/habedi/gogg/build_windows.yml?label=windows%20build&style=flat&labelColor=555555&logo=github" alt="Windows Build">
    </a>
    <a href="https://github.com/habedi/gogg/actions/workflows/build_macos.yml">
        <img src="https://img.shields.io/github/actions/workflow/status/habedi/gogg/build_macos.yml?label=macos%20build&style=flat&labelColor=555555&logo=apple" alt="MacOS Build">
    </a>
    <br>
    <a href="docs">
        <img src="https://img.shields.io/badge/docs-latest-3776ab?style=flat&labelColor=555555&logo=readthedocs" alt="Docs">
    </a>
    <a href="https://github.com/habedi/gogg">
        <img src="https://img.shields.io/badge/license-MIT-007ec6?style=flat&labelColor=555555&logo=open-source-initiative" alt="License">
    </a>
    <a href="https://codecov.io/gh/habedi/gogg">
        <img src="https://img.shields.io/codecov/c/github/habedi/gogg?style=flat&labelColor=555555&logo=codecov" alt="Code Coverage">
    </a>
    <a href="https://www.codefactor.io/repository/github/habedi/gogg">
        <img src="https://img.shields.io/codefactor/grade/github/habedi/gogg?style=flat&labelColor=555555&logo=codefactor" alt="CodeFactor">
    </a>
    <a href="https://github.com/habedi/gogg/releases/latest">
        <img src="https://img.shields.io/github/release/habedi/gogg.svg?style=flat&labelColor=555555&logo=github" alt="Release">
    </a>
    <a href="https://github.com/habedi/gogg/releases">
        <img src="https://img.shields.io/github/downloads/habedi/gogg/total.svg?style=flat&labelColor=555555&logo=github" alt="Total Downloads">
    </a>
</div>

## Gogg

Gogg is a minimalistic command-line tool for downloading game files from [GOG.com](https://www.gog.com/).
It is written in [Go](https://golang.org/) and uses the
official [GOG API](https://gogapidocs.readthedocs.io/en/latest/index.html).

The main goal of Gogg is to provide a simple and easy-to-use interface for people who want to download their GOG games
for offline use or archival purposes.

### Features

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
- Calculate the total size of the files to be downloaded (for storage planning)

### Getting Started

See the [documentation](docs/README.md) for how to install and use Gogg.

Run `gogg -h` to see the available commands and options.

#### Examples

**For more detailed examples, see the content of the [examples](docs/examples/) directory.**

##### Login to GOG

```bash
# First-time using Gogg, you need to login to GOG to authenticate
gogg login
```

> [!IMPORTANT]
> You need to have [Google Chrome](https://www.google.com/chrome/) or [Chromium](https://www.chromium.org/) installed on
> your machine for the first-time authentication.
> So, make sure you have one of them installed and available in your system's PATH.

##### Syncing the Game Catalogue

```bash
# Will fetch the up-to-date information about the games you own on GOG
gogg catalogue refresh
```

##### Searching for Games

```bash
# Will show the game ID and title of the games that contain "Witcher" in their title
gogg catalogue search "Witcher"
```

##### Downloading a Game

```bash
# Will download the files for `The Witcher: Enhanced Edition` to `./games` directory (without extra content)
gogg download 1207658924 ./games --platform=windows --lang=en --dlcs=true --extras=false \
 --resume=true --threads 5 --flatten=true
```

##### File Hashes (For Verification)

```bash
# Will show the SHA1 hash of the downloaded files for `The Witcher: Enhanced Edition`
gogg file hash ./games/the-witcher-enhanced-edition --algo=sha1
```

##### Storage Size Calculation

```bash
# Will show the total size of the files to be downloaded for `The Witcher: Enhanced Edition`
DEBUG_GOGG=false gogg file size 1207658924 --platform=windows --lang=en --dlcs=true \
 --extras=false --unit=GB
```

### Demo

[![asciicast](https://asciinema.org/a/kXMGRUUV149R37IEmZKtTH7nI.svg)](https://asciinema.org/a/kXMGRUUV149R37IEmZKtTH7nI)

### Contributing

Please see the [CONTRIBUTING.md](CONTRIBUTING.md) file for information on how to contribute to Gogg.

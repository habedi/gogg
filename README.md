<div align="center">
  <picture>
    <img alt="Gogg Logo" src="logo.jpeg" height="40%" width="40%">
  </picture>
<br>

[![Tests](https://img.shields.io/github/actions/workflow/status/habedi/gogg/tests.yml?label=tests&style=flat&labelColor=555555&logo=github)](https://github.com/habedi/gogg/actions/workflows/tests.yml)
[![Code Coverage](https://img.shields.io/codecov/c/github/habedi/gogg?style=flat&labelColor=555555&logo=codecov)](https://codecov.io/gh/habedi/gogg)
[![Release](https://img.shields.io/github/release/habedi/gogg.svg?style=flat&labelColor=555555&logo=github)](https://github.com/habedi/gogg/releases/latest)
[![Docker Image](https://img.shields.io/badge/docker-ghcr.io-007ec6?style=flat&labelColor=282c34&logo=docker)](https://github.com/habedi/gogg/pkgs/container/gogg)
[![Docs](https://img.shields.io/badge/docs-read-3776ab?style=flat&labelColor=555555&logo=readthedocs)](docs)
[![License](https://img.shields.io/badge/license-MIT-3776ab?style=flat&labelColor=555555&logo=open-source-initiative)](LICENSE)
[![Total Downloads](https://img.shields.io/github/downloads/habedi/gogg/total.svg?style=flat&labelColor=555555&logo=github)](https://github.com/habedi/gogg/releases)

</div>
    
---

Gogg is a minimalistic tool for downloading game files from [GOG.com](https://www.gog.com/).
It is written in [Go](https://golang.org/) and uses the
official [GOG API](https://gogapidocs.readthedocs.io/en/latest/index.html).

The main goal of Gogg is to provide a simple and easy-to-use interface for people who want to download their GOG games
for offline use or archival purposes.

### Features

Main features of Gogg:

- It can be used to fully automate the download process with a few simple commands.
- It can run anywhere (Windows, macOS, or Linux) that a Go compiler is available.
- It has a graphical user interface (GUI) that lets users search and download games they own on GOG.

Additionally, it allows users to perform the following actions:

- List owned games
- Export the list of owned games to a file
- Search in the owned games
- Download game files (like installers, patches, and bonus content)
- Filter files to be downloaded by platform, language, and other attributes like content type
- Download files using multiple threads to speed up the process
- Resume interrupted downloads and only download missing or newer files
- Verify the integrity of downloaded files against the checksums GOG publishes
- Arrange downloads in folder layouts that RomM and Lutris can use directly
- Back up a game's GOG GALAXY cloud saves locally
- Calculate the total size of the files to be downloaded (for storage planning)

---

### Getting Started

See the [documentation](docs/README.md) for how to install and use Gogg.

Run `gogg -h` to see the available commands and options.

> [!NOTE]
> * Since version `0.4.1`, Gogg has a GUI besides its command line interface (CLI).
> The GUI supports the features of the CLI and adds a searchable library with covers, collections, download
> management, and cloud save backup.
> To start the GUI, run `gogg gui`.
> * Since version `0.4.2`, there are Docker images available for Gogg.
> See the [documentation](docs/README.md#containerization) for more information.

#### Examples

| File                                                                                     | Description                                                         |
|------------------------------------------------------------------------------------------|---------------------------------------------------------------------|
| [calculate_storage_for_all_games.ps1](docs/examples/calculate_storage_for_all_games.ps1) | PowerShell script to calculate storage size for all games user owns |
| [download_all_games.ps1](docs/examples/download_all_games.ps1)                           | PowerShell script to download all games user owns                   |
| [download_all_games.sh](docs/examples/download_all_games.sh)                             | Bash script to download all games user owns                         |
| [simple_example.sh](docs/examples/simple_example.sh)                                     | Simple examples of how to use Gogg from the command line            |

##### Log in to GOG

```bash
# First-time using Gogg, you need to log in to GOG to authenticate
gogg login
```

> [!IMPORTANT]
> You might need to have [Google Chrome](https://www.google.com/chrome/), [Chromium](https://www.chromium.org/), or
> [Microsoft Edge](https://www.microsoft.com/edge) browsers installed on your machine for the first-time authentication.
> So, make sure you have one of them installed and available in your system's PATH.

> [!IMPORTANT]
> Since version `0.5.0` users can log in to their GOG account from the GUI.
> So it's not strictly necessary for them to have of [Google Chrome](https://www.google.com/chrome/), [Chromium](https://www.chromium.org/), or
> [Microsoft Edge](https://www.microsoft.com/edge) browsers installed on their machines.
> **And Any modern web browser should work**.

<div align="center">
  <img alt="Log in from GUI" src="docs/screenshots/v0.5.0/8.png" width="100%">
</div>

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
 --resume=true --threads 5 --flatten=true --keep-latest=true
```

##### File Hashes (for Verification)

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

### CLI Demo

[![asciicast](https://asciinema.org/a/kXMGRUUV149R37IEmZKtTH7nI.svg)](https://asciinema.org/a/kXMGRUUV149R37IEmZKtTH7nI)

### GUI Screenshots

<div align="center">
  <img alt="Game Library" src="docs/screenshots/v0.5.0/6.png" width="100%">
</div>

<details>
<summary>Show more screenshots</summary>

<div align="center">
  <img alt="Start 1" src="docs/screenshots/v0.5.0/1.png" width="100%">
  <img alt="Start 2" src="docs/screenshots/v0.5.0/2.png" width="100%">
  <img alt="Settings" src="docs/screenshots/v0.5.0/3.png" width="100%">
  <img alt="Downloads" src="docs/screenshots/v0.5.0/4.png" width="100%">
  <img alt="Refreshing Library" src="docs/screenshots/v0.5.0/5.png" width="100%">
  <img alt="Downloaded Games" src="docs/screenshots/v0.5.0/7.png" width="100%">
</div>

</details>

---

### Contributing

Please see the [CONTRIBUTING.md](CONTRIBUTING.md) file for information on how to contribute to Gogg.

### License

Gogg is licensed under the [MIT License](LICENSE).

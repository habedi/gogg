> [!IMPORTANT]
> The current Gogg release needs [Google Chrome](https://www.google.com/chrome/) or
[Chromium](https://www.chromium.org/) as a dependency for the first-time authentication (logging into the GOG website
> using username and password).
> So, make sure you have one of them installed on your machine.

### Installation

You can download the binary builds of Gogg for your operating system
from the [releases page](https://github.com/habedi/gogg/releases).
You might want to add the binary to your system's PATH to use it from anywhere on your system.

#### Installation from Source

Alternatively, you can install Gogg from source using Go toolchain.
To do that, you need to have [Go](https://golang.org/) installed on your machine.

```bash
go install github.com/habedi/gogg@latest # Replace `latest` with the desired version (e.g., v0.4.0)
```

```bash
# Running Gogg
$GOPATH/bin/gogg <command>
```

### Usage

#### Login to GOG

Use the `login` command to login to your GOG account the first time you use Gogg.

```sh
gogg login
```

#### Game Catalogue

Gogg stores information about the games you own on GOG in a local database called the (game) catalogue.
The `catalogue` command and its subcommands allow you to interact with this database.

##### Updating the Catalogue

Use the `catalogue refresh` command to synchronize the catalogue with your GOG library.
This command will fetch the up-to-date information about the games you own on GOG and store it in the catalogue.

```sh
gogg catalogue refresh
```

You might want to run this command after purchasing new games on GOG to keep the catalogue synchronized.

##### Listing Games

To see the list of games in the catalogue, use the `catalogue list` command:

```sh
gogg catalogue list
```

##### Searching for Games

To search for games in the catalogue, you can use the `catalogue search` command.
The search can be done either by the game ID or by a search term.

```sh
# Search by search a term (default)
# The search term is case-insensitive and can be a partial match of the game title
gogg catalogue search <search_term>
```

```sh
# Search by the game ID (use the --id flag)
gogg catalogue search --id=true <game_id>
```

##### Game Details

To see detailed information about a game in the catalogue, use the `catalogue info` command.
The command requires the game ID as an argument.

```sh
# Displays the detailed information about a game from the catalogue
gogg catalogue info <game_id>
```

##### Exporting the Catalogue

You can export the catalogue to file using the `catalogue export` command.
The command requires the format of the file (CSV or JSON) and the directory path to save the file.

If the format is CSV, the file will include the game ID, title of every game in the catalogue.

```sh
# Export the catalogue as CSV to a file in the specified directory
gogg catalogue export --format=csv <output_dir>
```

If the format is JSON, the file will include the full information about every game in the catalogue.
The full information is the data that GOG provides about the game.

```sh
# Export the catalogue as JSON to a file in the specified directory
gogg catalogue export --format=json <output_dir>
```

#### Downloading Game Files

To download game files, use the `download` command and provide it with the game ID and the path to the directory
where you want to save the files.

```sh
gogg download <game_id> <download_dir>
```

The `download` command supports the following additional options:

- `--platform`: Filter the files to be downloaded by platform (all, windows, mac, linux) (default is windows)
- `--lang`: Filter the files to be downloaded by language (en, fr, de, es, it, ru, pl, pt-BR, zh-Hans, ja, ko) (default
  is en)
- `--dlcs`: Include DLC files in the download (default is true)
- `--extras`: Include extra files in the download like soundtracks, wallpapers, etc. (default is true)
- `--resume`: Resume interrupted downloads (default is true)
- `--threads`: Number of worker threads to use for downloading (default is 5)
- `--flatten`: Flatten the directory structure of the downloaded files (default is true)

For example, to download all files (English language) of a game with the ID `<game_id>` to the directory
`<download_dir>` with the specified options:

```sh
gogg download <game_id> <download_dir> --platform=all --lang=en --dlcs=true --extras=true \
--resume=true --threads=5 --flatten=true
```

### Enabling Debug Mode

To enable debug mode, set the `DEBUG_GOGG` environment variable to `true` or `1` when running Gogg.
In debug mode, Gogg will be much more verbose and print a lot of information to the console.

#### Linux and macOS

```sh
DEBUG_GOGG=true gogg <command>
```

#### Windows (PowerShell)

```powershell
$env:DEBUG_GOGG = "true"; gogg <command>
```


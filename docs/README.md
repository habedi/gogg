### Installation

You can download the binary builds of Gogg for your operating system
from the [release page](https://github.com/habedi/gogg/releases).
You might want to add the binary to your system's PATH to use it from anywhere on your system.

---

### Usage

#### Log in to GOG

Use the `login` command to log in to your GOG account the first time you use Gogg.

```sh
gogg login
````

> [\!IMPORTANT]
> The current Gogg release might need [Google Chrome](https://www.google.com/chrome/),
> [Chromium](https://www.chromium.org/), or [Microsoft Edge](https://www.microsoft.com/edge) (since version `0.4.2`)
> as a dependency for the first-time authentication (logging into the GOG website using username and password).
> So, make sure you have one of them installed on your machine.

> [\!NOTE]
> On Windows, you might need to temporarily update your `PATH` in PowerShell so `gogg` can find the
> Google Chrome or Chromium browser when `gogg login` is run.
> This is needed if the browser isn't already in your system `PATH`.
>
> **For Chrome:**
>
> ```powershell
> $env:PATH += ";C:\Program Files\Google\Chrome\Application\"
> $env:DEBUG_GOGG = 1
> gogg.exe login
> ```
>
> **For Chromium:**
>
> ```powershell
> $env:PATH += ";c:\Program Files\Chromium\Application\"
> $env:DEBUG_GOGG = 1
> gogg.exe login
> ```
>
> **For Microsoft Edge:**
>
> ```powershell
> $env:PATH += ";C:\Program Files (x86)\Microsoft\Edge\Application\"
> $env:DEBUG_GOGG = 1
> gogg.exe login
> ```
>
> If Chrome, Chromium, or Microsoft Edge is installed in a different location, update the path accordingly.

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
# Search by a term (default)
# The search term is case-insensitive and can be a partial match of the game title
gogg catalogue search <search_term>
```

```sh
# Search by the game ID (use the --id flag)
gogg catalogue search --id <game_id>
```

##### Game Details

To see detailed information about a game in the catalogue, use the `catalogue info` command.
The command requires the game ID as an argument.

```sh
# Displays the detailed information about a game from the catalogue
gogg catalogue info <game_id>
```

##### Exporting the Catalogue

You can export the catalogue to a file using the `catalogue export` command.
The command requires the format of the file (CSV or JSON) and the directory path to save the file.

If the format is CSV, the file will include the game ID and the title of every game in the catalogue.

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
- `--lang`: Filter the files to be downloaded by language (en, fr, de, es, it, ru, pl, pt-BR, zh-Hans, ja, ko) (default is en)
- `--dlcs`: Include DLC files in the download (default is true)
- `--extras`: Include extra files in the download like soundtracks, wallpapers, etc. (default is true)
- `--resume`: Resume interrupted downloads (default is true)
- `--threads`: Number of worker threads to use for downloading (default is 5)
- `--flatten`: Flatten the directory structure of the downloaded files (default is true)
- `--skip-patches`: Skip patches when downloading (default is false)
- `--keep-latest`: After a successful download, remove older installer versions and keep only the latest version (default is false)
- `--romm`: Use RomM compatible folder layout `platform/game` for better integration with ROM Manager (default is false)

> [!NOTE]
> The `--keep-latest` flag scans downloaded installer files whose names contain a version-like pattern of digits separated by dots (like `game_installer_1.2.3.exe`).
> For each prefix before the version (like `game_installer_`), it keeps only the file with the highest numeric version and removes older ones (like keeps `1.2.3` and removes `1.1.0`).
> Files without a detectable version pattern are left untouched.
> Supported installer extensions for pruning include: `.exe`, `.bin`, `.dmg`, `.sh`, `.zip`, `.tar.gz`, and `.rar`.

For example, to download all files (English language) of a game with the ID `<game_id>` to the directory
`<download_dir>` with the specified options:

```sh
gogg download <game_id> <download_dir> --platform=all --lang=en --dlcs=true --extras=true \
--resume=true --threads=5 --flatten=true --keep-latest=true
```

---

### Configuration

You can customize where Gogg stores its data (like the game database and download history) using environment variables.
The location is determined with the following priority:

1. `GOGG_HOME`: If this is set, its value will be used as the base directory.
   This is a direct override for full control.
2. `XDG_DATA_HOME`: If `GOGG_HOME` is not set, Gogg will respect this standard variable (common on Linux) and store
   data in `$XDG_DATA_HOME/gogg`.
3. Default: If neither is set, Gogg falls back to creating a `.gogg` folder in your user home directory (`~/.gogg`
   on Linux/macOS and `%USERPROFILE%\.gogg` on Windows).

#### Examples

##### Linux and macOS

```sh
# Use a custom directory for Gogg data
export GOGG_HOME="/path/to/my/gogg_data"
gogg catalogue list
```

##### Windows (PowerShell)

```powershell
# Use a custom directory for Gogg data
$env:GOGG_HOME = "D:\GoggData"; gogg catalogue list
```

---

### GUI

Since version `0.4.1`, Gogg has a GUI that provides most of the features of Gogg's CLI.
The GUI can be started by running `gogg gui` from the command line.

---

### Debug Mode

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

---

### Containerization

Starting with version `0.4.2`, Gogg is available as a Docker image.
This makes it easier to run Gogg on servers or in setups where you prefer using containers.
You can find the latest image on the [GitHub Container Registry](https://github.com/habedi/gogg/pkgs/container/gogg).

> [!IMPORTANT]
> In the containerized version, the GUI is not supported and only Gogg CLI commands are available.

#### Running Gogg in a Container

Gogg's Docker container uses two dedicated folders (or volumes) to save its data;
`/config` is for the database and application settings, while `/downloads` is for the game files you download.

You can run Gogg CLI commands from inside the Docker container.
For example, to refresh the game catalogue, you can use the following command:

```bash
docker run --rm \
  -v </your/host/path/for/config>:/config \
  -v </your/host/path/for/downloads>:/downloads \
  -e DEBUG_GOGG=false \
  ghcr.io/habedi/gogg:<gogg-version> catalogue refresh
```

You need to replace the following placeholders with your actual paths and version:

* `</your/host/path/for/config>`: the path on your computer where you want Gogg to store its database (`games.db` file).
* `</your/host/path/for/downloads>`: the path on your computer where you want your downloaded game files to be stored.
* `<gogg-version>`: the specific version of the Gogg image you want to use (like `latest` or `v0.4.2`).

You can also set `DEBUG_GOGG` to `true` or `1` if you want to enable debug mode for more verbose output.

##### Example

```bash
docker run --rm \
  -v /media/data/home/gogg/:/config \
  -v /media/data/home/gogg/games:/downloads \
  -e DEBUG_GOGG=true \
  ghcr.io/habedi/gogg:latest catalogue refresh
```

> [!NOTE]
> It's a good idea to use a specific Gogg image version (like `v0.4.2`) instead of `latest`.
> The `latest` tag usually points to the most recent development version, which might not be stable and could
> potentially break your setup.

#### Troubleshooting Permissions

If you encounter "permission denied" errors when setting up these storage folders:

1. Check that the user running Docker on your computer has permission to write to the host paths you've specified.
2. Make sure that the container's `gogg` user (which is not the root user) has the same user's ID and group
   ID as your host user.
   You can do this by adding `--user <YOUR_HOST_UID>:<YOUR_HOST_GID>` to your `docker run` command (for example,
   `--user 1000:1000` for a typical Linux user).

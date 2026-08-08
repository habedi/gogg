# AGENTS.md

This file provides guidance to coding agents collaborating on this repository.

## Mission

Gogg is a command-line and GUI tool for downloading game files from GOG.com.
It authenticates against the GOG API, manages a local catalogue of owned games, and handles file downloads with
progress tracking and resumption support.
Priorities, in order:

1. Correctness of authentication, API interaction, and file downloading.
2. Reliable local state management (SQLite catalogue and token storage).
3. Clean separation between the CLI (`cmd/`), API client (`client/`), persistence (`db/`), and GUI (`gui/`) layers.
4. Cross-platform support (Linux, macOS, and Windows).

## Core Rules

- Use English for code, comments, docs, and tests.
- Prefer small, focused changes over large refactoring.
- Add comments only when they clarify non-obvious behavior.
- Do not add features, error handling, or abstractions beyond what is needed for the current task.
- Keep external dependencies minimal: do not add new `go.mod` entries without prior discussion.

## Backward Compatibility

Gogg has real users. The following must stay backward compatible:

- CLI command names, flag names, and exit codes.
- The config file format read from `~/.config/gogg/config.json`.
- The SQLite schema: changes must be additive (new tables or columns with sensible zero values), so a
  database from an older version keeps working. A catalogue refresh must never drop data other tables hold.
- Files written next to downloads (`metadata.json`, `download_info.json`, `files.json`): fields may be added,
  not removed or renamed.
- Persisted enum values (such as download states in the history file): append new values, never renumber.

## Writing Style

- Use Oxford commas in inline lists: "a, b, and c" not "a, b, c".
- Do not use em dashes, in documentation or in code comments. Restructure the sentence, or use a colon or
  semicolon instead.
- Avoid colorful adjectives and adverbs. Write "rate limiter" not "smart rate limiter".
- Prefer noun phrases for checklist items over imperative verbs. Write "rate limit enforcement" not "enforce
  rate limits".
- Headings in Markdown files must be in title case: "Build from Source" not "Build from source". Minor words
  stay lowercase unless they are the first word: the articles (a, an, the), the coordinating conjunctions (and,
  but, or, nor, so, yet, for), and the short prepositions (in, on, at, to, by, of, up, as, from, with, into,
  over). The prepositions are named because "from" has to be lowercase for "Build from Source" to be correct.
- Do not bold the lead-in of a list item. Write "Unit tests: ..." not "**Unit tests**: ...".
- Use sentence case for the lead-in of a list item. Write "Seed selection: ..." not "Seed Selection: ...".
  Proper nouns keep their capitals.
- Capitalize only the first part of a hyphenated compound: "Owned-game Listing" in a heading, "Owned-game" at
  the start of a sentence, and "owned-game listing" elsewhere. Never write "Owned-Game".
- Start each sentence with a capital letter, capitalize proper nouns (Go, Fyne, GOG, SQLite), and leave common
  nouns lowercase in the middle of a sentence.
- Write correct and complete sentences.
- Avoid made-up words.
- Do not use a colon in place of a verb. Three uses are fine: joining two clauses inside a complete sentence
  (the replacement the em-dash rule above calls for), introducing the gloss of a list item, and introducing an
  enumeration, whether as a list or inline ("Targets: `make test`, `make lint`, ..."). What a colon must not do
  is turn a sentence into a label and a definition: write "Fetches file metadata, then streams bytes with a
  progress wrapper" rather than "Download pipeline: fetches file metadata". That shape belongs to a list item,
  and carrying it into prose (a doc comment summary, a paragraph) leaves a fragment where a sentence was
  required.
- Use participial phrases and abbreviations scarcely.

## Repository Layout

- `main.go`: Entry point; initializes the database and delegates to `cmd.Execute`.
- `cmd/`: Cobra command definitions (`cli.go`, `download.go`, `saves.go`, `catalogue.go`, `login.go`,
  `version.go`, `file.go`, `gui.go`). Each command wires flags and calls into `client/` or `db/`.
- `client/`: GOG API client; contains `login.go` (OAuth via chromedp), `games.go` (owned-game listing),
  `catalogue.go` (sync), `products.go` (owned-products listing with artwork and purchase order),
  `download.go` (file downloads with progress and the `files.json` manifest), `parallel.go` (range connections
  within one file), `checksum.go` (verification against GOG's published MD5), `stall.go` (the silence watchdog),
  `lutris.go` (the Lutris slug and cache layout), `cloudsaves.go` (read-only Galaxy cloud save backup),
  `prune.go` (old-installer removal), `metadata.go` (store-page lookups), `data.go` (data parsing), and
  `rate_limiter.go` (request throttling).
- `auth/`: Authentication service and interfaces wrapping GOG OAuth token lifecycle.
- `db/`: GORM/SQLite persistence; contains `db.go` (connection setup), `game.go` (game model), `token.go` (token
  model), `tag.go` (user marks such as favorite and hidden), `metadata.go` (stored store-page lookups), and
  `repository.go` (the repository interfaces and their GORM implementations).
- `gui/`: Fyne desktop GUI. The larger concerns each have a file: `window.go` (tabs and assembly), `library.go`
  (the catalogue tab), `pane.go` (details pane and download form), `status.go` (`libraryState` and update
  detection), `facts.go` (search facts and caches), `filters.go` (filter dialog), `history.go` (download-directory
  lookups), `updates.go` (version diffing), `manager.go` and `download.go` (download queue and execution),
  `metadata.go` and `covers.go` (store lookups and artwork), `stores.go` (the repositories the GUI is handed),
  `sidebar.go`, `gallery.go`, `grid.go`, `selection.go`, `settings.go`, `widgets.go`, and `theme.go`.
- `scripts/`: Shell scripts for integration testing and Docker entrypoint.
- `.github/workflows/`: CI workflows for tests and releases.
- `Makefile`: All developer tasks (build, test, lint, format, release).

## Architecture

### Layers

Gogg is organized into four layers that should not have upward dependencies:

1. `db/`: persistence only; no knowledge of the API or CLI.
2. `client/`: GOG API calls and file I/O; depends on `auth/` and `db/` but not on `cmd/` or `gui/`.
3. `cmd/`: Cobra command handlers; orchestrates `client/` and `db/` calls, formats output.
4. `gui/`: Fyne desktop interface; calls into `client/` and `db/` the same way `cmd/` does.

### Boundaries Worth Keeping

- The GUI reaches the database only through the repository interfaces in `db/repository.go`, bundled into the
  `stores` struct built by `openStores()` in `gui/stores.go`. That function is the one place in the GUI that
  reads the global handle; do not add another.
- `client.DownloadGameFiles` takes a `DownloadOptions` struct. Extend the struct rather than the parameter list.
- The GUI's per-library knowledge (statuses, parsed facts, sizes, tags, and genres) lives in `libraryState`,
  owned by the library tab. Do not add package-level mutable state to `gui/`; the package vars that remain are
  test seams, assets, and signal bindings.
- List rows and grid cells are bound through the `rowBinding` bundle. New per-row knowledge goes in there.
- Progress flows from `client/` to both frontends as JSON lines over an `io.Writer` (`ProgressUpdate`). The CLI
  and the GUI parse the same stream; changes to it must be additive.

### Authentication Flow

Login is handled by `client/login.go` using chromedp to drive a headless browser through GOG's OAuth flow.
Tokens are stored via `db/token.go` and retrieved by `auth/services.go` for subsequent API calls.

### Download Pipeline

`client/download.go` fetches file metadata, checks for existing partial downloads, and streams bytes with a
`progressbar` wrapper. Resumption is done via HTTP range requests against `.part` files that are renamed into
place on completion. Transient failures (5xx, 429, and dropped connections) are retried with backoff; refusals
such as 404 are not. A transfer that goes silent for `client.StallTimeout` is cut off by a watchdog and counts
as transient. With `DownloadOptions.Connections` above one, a file of 32 MB or more is split into regions that
several range requests fill at once (`client/parallel.go`); a sidecar beside the `.part` file records finished
chunks, and a `.part` file with a sidecar must never be appended to, because it has holes. Every completed
download records exact byte sizes, streaming MD5 checksums, and timestamps
in `files.json` beside `metadata.json`. When GOG publishes an MD5 for a file (the downlink endpoint in
`client/checksum.go`), the streamed checksum is verified against it: a mismatch deletes the file and retries,
and a missing manifest skips verification rather than failing the download.

### Build Tags

The GUI is gated behind the `desktop` and `gl` build tags. The `headless` tag produces a CLI-only binary with no
Fyne dependency. Use `-tags headless` when running in environments without a display server.

## Go Conventions

- Go version: 1.24 (as declared in `go.mod`).
- Formatting is enforced by `gofmt` (via `make format`) and optionally `gofumpt` (via `make gofumpt`). Run
  `make format` before committing.
- Naming follows Go standard conventions: `PascalCase` for exported identifiers, `camelCase` for unexported
  identifiers and local variables, and `SCREAMING_SNAKE_CASE` for top-level constants where idiomatic.
- Error values are wrapped with context using `fmt.Errorf("…: %w", err)` so callers can use `errors.Is`/`errors.As`.
- Use `zerolog` (already imported as `github.com/rs/zerolog/log`) for all logging; do not use `fmt.Print*` for
  diagnostic output.

## Required Validation

Run the relevant targets for any change:

| Target            | Command                 | What It Runs                                                      |
|-------------------|-------------------------|-------------------------------------------------------------------|
| Format            | `make format`           | `go fmt ./...`                                                    |
| Unit tests        | `make test`             | `go test ./...` with coverage and race detector                   |
| Integration tests | `make test-integration` | `go test -tags=integration ./...`                                 |
| Fuzz tests        | `make test-fuzz`        | Short fuzz runs for `FuzzParseSizeString` and `FuzzParseGameData` |
| Lint              | `make lint`             | `go vet`, `staticcheck`, and `golangci-lint`                      |
| Build             | `make build`            | Builds the desktop binary into `bin/`                             |
| Headless build    | `make release-headless` | Builds the CLI-only binary without GUI dependencies               |
| Coverage report   | `make showcov`          | Displays per-function coverage after running tests                |

## First Contribution Flow

1. Read the relevant package under `client/`, `cmd/`, `db/`, or `auth/`.
2. Implement the smallest change that covers the requirement.
3. Add or update `_test.go` files in the changed package to cover the new behavior.
4. Run `make test` and `make lint`.
5. If the change touches download, login, or catalogue logic, also run `make test-integration` and verify the
   relevant `cmd/` command still works end-to-end.

Good first tasks:

- New unit test for an existing untested helper in `client/data.go` or `client/games.go`.
- Error message refinement in `cmd/` handlers, paired with a test that asserts the exit code or output.
- New `make` target or script improvement in `Makefile` or `scripts/`.
- Additional fuzz corpus entries for `client/fuzz_test.go`.

## Testing Expectations

- Unit tests live in `_test.go` files alongside the package they cover.
- Integration tests are in `_integration_test.go` files and are gated by the `integration` build tag.
- Fuzz tests live in `client/fuzz_test.go`.
- Every new exported function or API-facing behavior must ship with at least one test that exercises it, including
  error paths where applicable.
- Tests that make HTTP calls must use a test HTTP server (`httptest.NewServer`) rather than the real GOG API.
  In `gui/` this includes setting `GOGG_API_BASE` before building any library fixture; a fixture without it can
  reach the real store from a background lookup.
- No public-facing behavior change is complete without a test covering the new or changed behavior.
- Run the race detector on anything that touches goroutines: `go test -race -count=1` on the affected packages,
  more than once. Several past bugs only surfaced on the second or third run.
- Slow-by-design behavior has a test seam: `statusWorker`, `parseGameData`, `notify`, `searchDebounce`,
  `metadataSweepPause` in `gui/`, and `client.RetryDelay`. Set them in `TestMain` or per test rather than
  sleeping the real delays out.

### GUI Test Rules

The Fyne test driver runs `fyne.Do` inline on the calling goroutine, so background deliveries that are
serialized onto the main thread in production run concurrently with the test body in tests. The rules that keep
the suite race-free:

- A background answer must not be able to land in a widget mid-test. Fixtures use `hangingStoreStub`, which
  holds every response until cleanup; a test that needs answers uses its own gated server and steps it.
- Cleanup order matters with gated servers: register `srv.Close` before the gate close, so the gate opens first
  and `Close` can finish. A `defer srv.Close()` above a gate cleanup deadlocks.
- Every library or cache a test builds must be closed on cleanup (`lt.close`, `cache.close`), or its goroutines
  outlive the test and touch the next test's database and widgets.
- Tests that set a binding and then read the result run their body through `offMain`; Fyne only queues binding
  listeners when the caller is the main goroutine.
- Assert on rendered widgets by walking with the helpers in `walk_test.go` (`buttonWithLabel`,
  `iconButtonWithTip`, `labelTexts`); list rows are exercised by calling `list.UpdateItem` with a fresh row.
- Never call `binding.UntypedList.Set` or `Append` while holding `DownloadManager.mu`; listeners run inline in
  tests and ask the manager for what it holds.
- A test that starts a real download ends with `awaitSettled` (in `queue_test.go`), not with a wait on the
  task's state. The state changes before the goroutine's announcement and slot release, so a state wait lets
  the goroutine outlive the test and read seams, such as `notify`, that the next test rewrites.

## Change Design Checklist

Before coding:

1. Packages affected by the change (`client`, `cmd`, `db`, `auth`, or `gui`).
2. Whether the change alters CLI flag names, output format, or exit codes visible to users or scripts.
3. Whether a new external dependency is required, and if so, whether it has been discussed.
4. Build-tag implications: does the change need to work under `-tags headless`?

Before submitting:

1. `make format` passes (no diff).
2. `make test` passes with race detector enabled.
3. `make lint` passes.
4. Integration tests run locally if the change touches auth, download, or catalogue logic.

## Commit and PR Hygiene

- Keep commits scoped to one logical change.
- PR descriptions should include:
    1. Behavioral change summary.
    2. Tests added or updated.
    3. Whether integration tests were run locally (yes/no), and on which OS.

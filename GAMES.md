# Funda CLI games — v0.1 release candidate

The `game` subcommand uses locally enriched listing records. It does not submit
real bids or rental applications. Simulated inspections, rival bids, and rental
lotteries are fictional; they are **not** claims about real listings or landlords.

## Demo
### funda guess
<img width="1099" height="448" alt="image" src="https://github.com/user-attachments/assets/cd1d9a10-40e6-4f9c-9d45-b73aaa67ccc2" />

### funda hunter
<img width="1104" height="486" alt="image" src="https://github.com/user-attachments/assets/de26f207-173e-463d-b3b8-656b07e3ed98" />



## Setup

Run your normal `sync` and `enrich` workflow first. The games need listings with
cached details and positive asking prices. Games do not scrape new listings.

```sh
go test ./...
go test -race ./...
go build -o funda ./cmd/funda
./funda game guess --mode buy --city utrecht
./funda game guess --mode rent --city utrecht --detective
./funda game guess --mode buy --city utrecht --daily
./funda game hunter --mode buy --city utrecht --budget 375000 --financing-certain
./funda game hunter --mode rent --city utrecht --income 4500 --income-multiplier 3 --household 2
```

`--seed N` makes games reproducible against the same dataset. `--daily` creates
a UTC-date-based seed, but **is not a global fair leaderboard** because users
may have different locally cached listings. Prices remain hidden in Guess until
submission; opening a listing is available only in Hunter.

## Terminal images

`--image-mode auto|kitty|iterm2|link|none` configures image capability selection.
The `internal/gameimage` package contains Kitty and iTerm2 image encoding,
conservative environment-based detection, URL validation, and fallback output.
**The game CLI does not yet display inline listing photos.** The current
`model.Listing` and SQLite schema have no image-gallery fields. In Hunter, press
`o` to open the source listing in your browser; Guess hides its source until
submission to prevent answer leakage. Auto detection is heuristic, not a
confirmed terminal protocol handshake. tmux/screen/SSH default to links.

## Known limitations

- No image URLs are extracted or stored from Funda/MVGM; no actual photo gallery.
- Completed-game history saves by default, and House Hunter supports save/resume. No online leaderboards or multiplayer.
- Hunter has simulated viewing/inspection/bid/selection rules, not a full map
  exploration game or legally meaningful rental application.
- Full-project Go 1.25 tests must be run on a machine with that toolchain.
- Review data/image licensing and access rules before redistributing listings.

## Optional local score history

Use `--history-file` to save a completed game's summary without recording listing
addresses, URLs, or hidden prices:

```sh
funda game guess --mode buy --city utrecht --daily --history-file "$HOME/.local/share/funda/game-history.json"
funda game hunter --mode rent --city utrecht --income 4500 --income-multiplier 3 --history-file "$HOME/.local/share/funda/game-history.json"
```

The JSON history is saved by default (use `--no-save` to disable it) and written atomically with owner-only permissions.
Quitting early does not add a completed score. House Hunter separately checkpoints unfinished progress and supports `--resume` / `--restart`.
Concurrent game processes should use separate history files until locking is added.

### Testing

```sh
go test ./...
go test -race ./...
go build -o funda ./cmd/funda
```

The full project requires Go 1.25. Core game and image package tests were also
run independently using a Go 1.23-compatible isolated test module.

## Game history (saved by default)

Completed game scores are automatically saved to the per-user application data folder.
Use `--no-save` to disable this, or `--history-file /path/to/history.json` to
select a custom file. On Linux, the default follows `XDG_DATA_HOME` or
`~/.local/share/funda/game-history.json`; on macOS it uses
`~/Library/Application Support/funda/game-history.json`; on Windows it uses
`%APPDATA%\funda\game-history.json`. This stores completed scores only,
not in-progress House Hunter sessions.

## House Hunter autosave and resume

House Hunter saves an unfinished game by default to `hunter-save.json` beside
`game-history.json` in the platform-specific application data directory.

```bash
funda game hunter --mode buy --city utrecht
# After quitting, resume with the same mode/city/source:
funda game hunter --mode buy --city utrecht --resume
# Discard an unfinished session and start over:
funda game hunter --mode buy --city utrecht --restart
# Override save location:
funda game hunter --mode rent --city utrecht --save-file ./my-hunt.json
# Play without saving:
funda game hunter --mode buy --city utrecht --no-save
```

An unfinished save blocks a fresh run unless you use `--resume` or `--restart`.
The saved property sequence is validated against cached listings before resuming;
if cached listings changed, the game refuses to resume rather than substituting
other houses. Saves contain only game state and listing identifiers, not user
financial documents. Completed games remove the checkpoint after writing history.

## Saving and daily play

`--daily` uses a UTC date-based deterministic seed, but users need identical local datasets for comparable challenges. Completed results save automatically unless `--no-save` is used. `--history-file` overrides the history path; `--save-file` overrides House Hunter progress storage. Use `--resume` to continue and `--restart` to discard an unfinished Hunter run.

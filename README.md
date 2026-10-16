# funda-cli v0.1

A lightweight local CLI for personal Dutch housing research.

The first version deliberately separates three concerns:

1. **Discovery** — read listing URLs from Funda sitemaps.
2. **Enrichment** — optionally fetch structured details through an adapter.
3. **Search** — query a local SQLite database without repeatedly hitting a remote service.

The bundled enrichment adapter calls Funda API, thanks for `pyfunda`. Funda does not provide a public consumer developer API; internal endpoints may change and use may be subject to Funda's terms. Keep request volume low and use this for personal research.

## Requirements

- Go 1.23+

## Build

```bash
go mod tidy
go build -o funda ./cmd/funda
```

## First run

Discover rental listings from the sitemap using a browser-profile Go HTTP client:

```bash
./funda sync --category rent --profile chrome
```

Available profiles are `chrome`, `firefox`, `safari`, `edge`, `ios`, and `chrome_android`. The transport reproduces browser-like TLS/HTTP behavior, but the CLI does not implement CAPTCHA/challenge solving, proxy rotation, or retries intended to defeat an explicit access restriction.

Review the references discovered by the sitemap with the same `search` command used for enriched listings:

```bash
./funda search --city utrecht --category rent
```

Only show references that do not have detail data yet:

```bash
./funda search --city utrecht --category rent --unfetched
```

Show the full Funda URL as well:

```bash
./funda search --unfetched --url
```

Unfetched rows show `-` for price, area, bedrooms, and energy label. These fields become available after detail enrichment.

Enrich a small batch:

```bash
./funda enrich --city utrecht --category rent --limit 20
```

Now search locally by structured fields:

```bash
./funda search \
  --city utrecht \
  --category rent \
  --max-price 2200 \
  --min-area 50 \
  --fetched
```

View one listing:

```bash
./funda view 12345678
```

## Database

Default database: `./funda.db`

Change it with:

```bash
export FUNDA_DB="$HOME/.local/share/funda/funda.db"
```

Tables:

```text
listing_refs   sitemap-discovered IDs, URLs, city, category, lastmod
listings       API-enriched price, area, rooms, energy label, etc.
```

## Why sitemap + local DB?

The CLI does not make a remote search request every time you type `funda search`.
The normal workflow is:

```text
sitemap -> listing_refs -> optional detail enrichment -> SQLite -> local search
```

This makes the user-facing search fast and keeps remote requests explicit and cacheable.

## Next milestones

- Throttle the request.
- Detect changed sitemap `lastmod` values and refresh only stale details.
- Add `--refresh` to `search`/`view`.
- Add JSON output for shell pipelines.
- Add saved searches and favorites.
- Add a Bubble Tea TUI.
- Add provider abstraction for other Dutch housing sources.

## MVGM integration

MVGM / ikwilhuren.nu support has been added. See [MVGM.md](MVGM.md).

```bash
funda sync --source mvgm --city utrecht
funda enrich --source mvgm --city utrecht --limit 50
funda search --source mvgm --city utrecht --fetched --income 60000 --savings 50000
```

## Terminal map

Fetched listings with latitude/longitude can be rendered as a Unicode Braille map using live OpenStreetMap data from the Overpass API:

```bash
./funda map --source funda 12345678
./funda map --source mvgm --radius 800 --width 100 --height 30 some-listing-id
./funda view --source funda --map 12345678
```

The renderer requests nearby roads, buildings, waterways, water, parks, and grass areas and rasterizes their real OSM geometry into a 2x4-dot Braille canvas per terminal cell. The property is marked by a cross at the center. `--radius` is in meters. Map rendering requires network access; normal local search/view behavior remains offline unless `--map` or the `map` command is used.

## Games (experimental, offline)

Funda CLI includes two local games, each supporting **buy** and **rent**:

- **Guess the Price:** Estimate the advertised asking price or monthly rent. Prices and source listing URLs are hidden until you submit your guess. Optional Detective hints cost 100 points.
- **House Hunter:** Explore cached properties, schedule viewings, perform **fictional** inspections, and make simulated buying offers or rental applications. No real offers or applications are sent.

Games use enriched listings already stored in the local SQLite database. Run `sync` and `enrich` first; games do not fetch fresh property data.

```bash
# Five-round guessing games
./funda game guess --mode buy --city utrecht
./funda game guess --mode rent --city utrecht --detective

# Daily guessing challenges (UTC-based seed)
./funda game guess --mode buy --city utrecht --daily
./funda game guess --mode rent --city utrecht --daily

# House Hunter: buying and renting
./funda game hunter --mode buy --city utrecht --budget 375000
./funda game hunter --mode rent --city utrecht --income 4500 --income-multiplier 3
```

### Daily challenges

`--daily` generates a deterministic seed from the UTC date, city, game, and mode. **This does not guarantee identical properties for different players**: rounds are selected from each player's local database. Scores should only be compared when datasets match. There is no online leaderboard or globally synchronized daily challenge.

### Saving progress and scores

Completed game results are saved to a local history file **by default**. House Hunter also checkpoints progress so you can quit and resume later.

```bash
./funda game hunter --mode buy --city utrecht
./funda game hunter --mode buy --city utrecht --resume
./funda game hunter --mode buy --city utrecht --restart

# Disable local saving, or override its location
./funda game guess --mode buy --no-save
./funda game guess --mode buy --history-file ./game-history.json
./funda game hunter --mode buy --save-file ./hunter-save.json
```

The default history location is `~/.local/share/funda/game-history.json` on Linux, `~/Library/Application Support/funda/game-history.json` on macOS, and `%AppData%\funda\game-history.json` on Windows. House Hunter saves beside the default history file. Resume validates the saved property sequence and asking prices; if the local database has changed, restart the game instead.

### Controls and images

In House Hunter, use `v` for viewing, `i` for a fictional inspection, `e` for cached details, `a` for a simulated bid/application, `s` to skip, and `o` to open the original listing in a browser. The simulation has a 20-turn limit.

Optional photo URLs can be supplied using `--photos-file`; the game currently displays **links**, not native inline photos. `--image-mode auto|kitty|iterm2|link|none` selects the image capability preference, but actual inline listing-image integration is not yet implemented. Automatic photo extraction is deferred.

### Test and limitations

```bash
go test ./...
go test -race ./...
go build -o funda ./cmd/funda
```

Inspections, rival bids, and rental-selection outcomes are fictional. They must not be used to assess actual property condition, valuation, or application chances. The complete application test suite still requires verification in the project's configured Go environment. See [GAMES.md](GAMES.md) for more details and [RELEASE_CANDIDATE.md](RELEASE_CANDIDATE.md) for release status.

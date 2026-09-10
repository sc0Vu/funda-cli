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

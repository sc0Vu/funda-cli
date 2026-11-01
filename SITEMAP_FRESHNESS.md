# Sitemap-aware enrichment

`funda check` and `funda watch` sync source listings, enrich candidates, persist details, match watches, then send notifications.

## What's new

- `listing_refs.lastmod` is retained from the source sitemap (when available).
- `listings.enriched_lastmod` records the sitemap value used for the most recent successful enrichment.
- A changed nonempty `lastmod` triggers re-enrichment even when `refresh_after` has not elapsed.
- The existing SQLite database is migrated automatically when opened.
- `lastmod` means URL modification, **not** listing publication. A new or changed sitemap entry does not establish that a property was newly published.

## Commands

```sh
funda watch list --config examples/monitor.json
funda notify test --config examples/monitor.json --provider telegram_personal
funda check --config examples/monitor.json
funda watch --config examples/monitor.json
```

## Limitations

- A source that does not provide `lastmod` is refreshed according to `refresh_after`.
- `enrich_limit` caps the number of candidate details fetched per watch and cycle.
- This release does not implement trustworthy `published_at` extraction or price-change event alerts.
- Test compilation and live source/notification integration before deploying; this environment has Go 1.23 while `go.mod` requires Go 1.25.

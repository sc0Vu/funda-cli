# Listing lifecycle and reopening

`funda check` / `funda watch` now use a separate `listing_lifecycle` table, migrated automatically on opening the SQLite database. Existing property rows and the old notification history are preserved. A new `notification_events` table tracks each listing generation separately, so a confirmed reactivation using the **same** `(source, listing_id)` can trigger another notification.

## Status checks

- A successful enrichment sets `active` only for a recognized availability indicator (`available`, `beschikbaar`, `te huur`, `te koop`). An unknown indicator is `unknown`, not automatically active.
- `under_offer`, `rented_subject_to_conditions`, etc. are `under_option`; sold/rented are `unavailable`.
- First HTTP 404: `suspected_removed`; second 404 or HTTP 410: `removed`. Neither is deleted from SQLite.
- HTTP 403, 429, 5xx, transport errors: retain stored property details but set lifecycle availability to `unknown` until a successful verification; do not send new alerts on uncertain availability.
- Reappearance with the same source ID after `removed`, `suspected_removed`, `unavailable` or `under_option` increments `generation` when confirmed active; notification keys include generation.
- The `status_refresh` configuration specifies intervals by status; `next_check_at` prevents rechecking removed entries every cycle.

## Reappearance with a different ID

A new `(source, id)` is stored as a **new listing**, not automatically merged with the previous one. Some portals republish the same address under a new ID, and matching by address alone is unsafe (apartments, reused addresses, different units). A future optional `property_identity` / `listing_episodes` model can compare normalized address + postcode + unit + source details, flag probable duplicates, and allow user-confirmed linking. Do not assume IDs remain stable.

## Current limitations

- The new lifecycle generation table is independent of older `notification_deliveries`; users upgrading should first run `funda check --no-notify` to inspect data and verify watch configuration, but note that `--no-notify` does **not** initialize a notification baseline in this release.
- The first full monitoring cycle may still baseline new listings discovered during that cycle; the initial baseline/new-listing detection issue needs separate work.
- A notification for a reactivated property may still be skipped if its details were not refreshed successfully or its status cannot be parsed.
- `check` is not cross-process locked; avoid running `watch` and cron concurrently on the same SQLite file.
- Existing delivery retries have no backoff. New `notification_events` preserves the existing behavior.
- Go 1.25 is required; tests were added but could not be executed in this environment.

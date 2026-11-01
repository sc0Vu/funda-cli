# Monitoring reliability update

## Changes
- One database-level lease prevents concurrent check cycles. Note: a check exceeding the two-hour lease can overlap a later check; a heartbeat is needed for truly unbounded runs.
- Initial watch baseline is captured **before** the first sync. Already-known references are suppressed when `notify_existing=false`; references discovered in that sync are eligible. `--no-notify` still initializes the watch, deliberately establishing a no-alert baseline.
- A single 404 is `suspected_removed`; a subsequent successful fetch does not increment the reopening generation. A confirmed removal (two 404s or 410), or an explicitly unavailable/under-option listing, followed by confirmed active status increments it.
- Notification jobs are stored in SQLite, claimed atomically, and retried with exponential backoff, up to five attempts. Delivery is at-least-once; a crash after sending but before committing can produce a duplicate.
- Overdue status checks are prioritized ahead of newly missing or modified entries, reducing starvation. `enrich_limit` still caps each watch's work.
- A confirmed price reduction is recorded separately and can generate `price_drop` alerts; previous listing-generation alerts are retained. Distinct drops to the same price after an intervening increase are not yet individually identified.
- HTTP 403/429/5xx preserve the last good property data but change availability to unknown, preventing new notifications until successful verification.

## Remaining limitations
- **Not build-verified** in this environment (Go 1.23; module requires Go 1.25, and dependencies are not cached).
- Live Funda/MVGM crawling, Telegram, and SMTP need end-to-end testing.
- Price drops are detected on successfully fetched details, but no dedicated property revision/episode identity exists for republished listings with different IDs.
- The refresh queue is improved but does not yet have separate strict quotas for new, changed and overdue listings.
- A long-running check should renew its lease; currently it expires after two hours.
- Existing notification_events history is preserved; the new notification_jobs table is used for future deliveries.

## Usage
Back up your SQLite DB before running the updated CLI. Then run `funda check --config examples/monitor.json` once manually and inspect the results before enabling systemd/cron.

# Docker monitoring with persistent SQLite

The Docker image runs `funda watch` continuously. It stores SQLite in the **host-mounted `./data` directory**, which survives container rebuilds and replacements. The database is not baked into the image.

## Quick start

```sh
cp examples/monitor.json monitor.json
mkdir -p data
# Linux: ensure the mounted data directory is writable by the container's `funda` user.
# Set the required secrets in .env (never commit it):
cat > .env <<'ENV'
TELEGRAM_BOT_TOKEN=replace-me
TELEGRAM_CHAT_ID=replace-me
SMTP_USERNAME=
SMTP_PASSWORD=
FUNDA_WEBHOOK_URL=
ENV
docker compose up -d --build
docker compose logs -f funda
docker compose exec funda funda health --max-age 30m
```

The Docker image does not expose HTTP by default. Docker health status is based on the most recent successful scan. See the Oracle section below.

## Existing database

To migrate an existing local database, stop other funda processes and copy `funda.db` into `./data/funda.db`. If SQLite uses WAL mode, cleanly stop the original process first (or use SQLite's backup API) to avoid copying an inconsistent database. Ensure directory ownership is writable by the container user. Never run the host CLI and container against the same database concurrently.

## Useful commands

```sh
docker compose exec funda funda watch list --config /config/monitor.json
docker compose exec funda funda notify test --config /config/monitor.json --provider telegram_personal
docker compose down  # data remains on host
```

The runtime image uses a non-root user. On Linux, if `./data` permissions prevent writes, set ownership or use a suitable bind-mount user mapping. Configuration is read from `./monitor.json`; restart the container after edits. Avoid committing `.env` or your database.

Build requires network access to fetch Go modules and a Go 1.25 builder image. The default runtime image is Alpine; the executable is built with CGO disabled. Validate your platform's HTTP impersonation behavior and integration tests before production deployment.

## Oracle Always Free / headless deployment (updated)

The default Docker container **does not expose or start an HTTP server**. The Dockerfile
healthcheck calls `funda health --max-age 30m` and reads a timestamp written **only after a
successful complete `watch` check cycle**. This catches stale crawlers, not merely a live process.
Set the max age higher than your configured check interval plus the worst-case crawl duration.
Docker marks stale containers `unhealthy` but does **not** automatically restart them; configure
external monitoring or a recovery policy if you require that behavior.

On a Linux VM, create the host data directory with permissions for UID 10001:

```sh
mkdir -p data
sudo chown 10001:10001 data
cp examples/monitor.json monitor.json
# Set Telegram credentials in .env; compose injects them as environment variables.
docker compose up -d --build
docker compose ps
docker compose exec funda funda health --max-age 30m
```

The initial scan can exceed 10 minutes; the healthcheck allows a 10-minute startup grace
period. Adjust it for large monitoring configurations. No Oracle ingress port needs opening.
If you still want the old HTTP endpoints, set `FUNDA_ENABLE_HTTP_HEALTH=1` and `HEALTH_ADDR=:8080`
and explicitly publish a loopback-only port; otherwise HTTP is disabled.

## GitHub CI

`.github/workflows/ci.yml` runs `go vet`, race-enabled Go tests, Linux AMD64 and ARM64 binary
builds, and a multi-platform Docker build on pushes and pull requests. It uploads both binaries
as workflow artifacts. It does **not** deploy, publish images, or run scheduled monitoring.

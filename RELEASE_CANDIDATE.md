# Funda CLI Games — v0.1 release candidate

## Included

- Guess the Price: buy/rent, detective hints, daily seed, history
- House Hunter: buy/rent, viewings, fictional inspections, simulated bids/applications, save/resume
- Optional photo URL manifest (no automatic extraction or inline gallery)
- Resume checks listing IDs **and original asking prices** for new save files
- 20-turn limit also applies to skipping

## Test status

The isolated `internal/game` and `internal/gameimage` packages passed `go test` and `go test -race` with a Go 1.23-compatible test harness. This is **not** full-project verification. The original project declares Go 1.25 and depends on modules not available to the current environment.

Run in a Go 1.25 environment:

```sh
go test ./...
go test -race ./...
go build -o funda ./cmd/funda
```

## Example commands

```sh
./funda game guess --mode buy --city utrecht --daily
./funda game guess --mode rent --city utrecht --detective
./funda game hunter --mode buy --city utrecht --budget 375000
./funda game hunter --mode rent --city utrecht --income 4500 --income-multiplier 3
./funda game hunter --mode buy --city utrecht --resume
```

Populate enriched cached listings before playing. No real property offers or rental applications are submitted.

## Known limitations

- No automatic property photo extraction or native terminal image gallery.
- Daily challenge comparability depends on the same cached listing dataset.
- Saves from previous versions that lack a price snapshot still load, but cannot verify price changes; restart for strict consistency.
- Full-project integration tests have not been run here.

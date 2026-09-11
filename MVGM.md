# MVGM integration

The CLI now supports MVGM / ikwilhuren.nu alongside Funda.

## Sync MVGM listings

```bash
funda sync --source mvgm --city utrecht
```

## Fetch MVGM detail pages

```bash
funda enrich --source mvgm --city utrecht --limit 50
```

MVGM detail data is stored in the existing `listings` table plus a new
`rental_details` table for rental-specific information:

- service cost
- deposit
- available-from text
- parsed income rule
- estimated required annual income
- raw income-condition text

Listing identity is stored as `(source, id)`. MVGM IDs are no longer prefixed; `source="mvgm"` distinguishes them from Funda IDs.

## Search both sources

```bash
funda search --source all --city utrecht --category rent --max-price 1600 --min-area 45 --fetched
```

Or only MVGM:

```bash
funda search --source mvgm --city utrecht --fetched
```

## Estimate MVGM income eligibility

```bash
funda search \
  --source mvgm \
  --city utrecht \
  --fetched \
  --income 60000 \
  --savings 50000 \
  --max-price 1600
```

For the estimate, qualifying income is:

```text
gross annual income + 10% of own assets
```

The parser preserves the raw listing restriction text because MVGM rules can
vary by property/complex. Current MVGM general guidance is 3.5–4x monthly rent;
some current Utrecht listings state 42x kale monthly rent for rent above €1,600.
The CLI therefore treats the computed eligibility as an estimate, not a final
approval decision.

## View one MVGM listing

```bash
funda view --source mvgm 'utrecht-3513db-72-jongeneelstraat-197fbf230a6a841cde0e761f0428cf01'
```

This includes MVGM-specific service cost, deposit, availability and income rule.

## HTTP impersonation

MVGM uses the same `internal/client` package as Funda, so the existing
`impersonate-http` browser profiles are reused:

```bash
funda sync --source mvgm --city utrecht --profile chrome
```


## Source-aware schema

`listing_refs`, `listings`, and `rental_details` store `source` explicitly and use `(source, id)` as their key. On startup, a legacy database is migrated automatically: old unprefixed IDs become `source=funda`, while `mvgm:<slug>` becomes `source=mvgm`, `id=<slug>`.

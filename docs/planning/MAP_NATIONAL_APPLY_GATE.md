# National map apply gate

Status: Phase 3 coverage and apply preparation is **file-only**. No database
write, migration, deploy, nginx/k8s change or app change was performed.

## Ready artifacts (2026-10-26 sample)

- `station-review`: 361 L1 reviews plus the hash-verified manifest.
- `map-load`: 565 L2 `osm-mapped` assertions in one combined corpus and one
  prepared envelope. The envelope was validated with
  `mapcontract.ParsePreparedOverlay` (Go).
- Coverage: 45,688 stations; 926 published (L1+L2, 2.03%), 1,760 candidates
  (L3+L4) and 43,002 with no point. This is pipeline coverage, not a national
  coverage claim: A1.a is still blocked and B2 only has the pilot CNEFE/RFB
  extracts.
- Private paths on `sorrimobi-vps`:
  - `/opt/abastevo-temp/map-national-20261026/work/phase2-match-final-20261026/`
  - `/opt/abastevo-temp/map-national-20261026/work/phase2-open-geocode-20261026/`
  - `/opt/abastevo-temp/map-national-20261026/work/phase3-coverage-20261026/`

## Blocked

- 1,574 L3 `ANP` points need owner decision D1.
- 186 L4 `address-point` candidates need owner decision D2.
- A1.a national API coverage is blocked by repeated HTTP 429; the current
  coverage is not national.
- `station-review` and `map-load` need an explicit staging DSN and explicit
  owner authorization. The generated plan contains only `<STAGING_DSN>`
  placeholders and never a credential.

## Reproduction

```sh
prepare_national_coverage \
  --a2 <A2 output dir> --b2 <B2 output dir> \
  --registry <assertions.jsonl> --output <coverage dir> \
  --generation <generation>

station-review --reviews <reviews.ndjson> --manifest <manifest.json> \
  --dsn <STAGING_DSN>

map-load --envelope <map-envelope.json> \
  --assertions <map-assertions-combined.json> --dsn <STAGING_DSN>
```

## Rules

- Do not write to the database from this branch without D1/D2 and explicit
  authorization.
- Do not promote L3/L4 while the decisions are open; they stay decisions and
  candidates.
- Never log or commit the staging DSN.
- Re-run coverage and the envelope validation after any input change; changed
  inputs invalidate the apply evidence.

# National map apply gate

Status: **Phase 3 A3 L1/L2 staging apply completed on 2026-10-26** in namespace
`abastevo-temp`. B3 L4 remains blocked by D2. No production namespace,
deployment or app was changed. See
[MAP_NATIONAL_APPLY_RECEIPT](MAP_NATIONAL_APPLY_RECEIPT.md).

## Applied on staging (2026-10-26)

- `station-review`: 361 L1 reviews applied, 0 failed; reviewed location
  revisions went from 0 to 361.
- `map-load`: generation `02ea6e7d-19ce-4942-a369-1aa7efdd03dd` is `ACTIVE`
  with source revision `phase3-coverage-sample-20261026` and 565 assertions;
  map generations went from 0 to 1.
- `/v1/map/assertions` exposed the active generation. The endpoint accepts
  `source=ANP`/`COMMUNITY` only; the applied OSM assertions are stored in the
  generation but are not served by this price endpoint. That is a separate
  product/API decision.
- Coverage: 45,688 stations; 926 published (L1+L2, 2.03%), 1,760 candidates
  (L3+L4) and 43,002 with no point. This is pipeline coverage, not a national
  coverage claim: A1.a is still blocked and B2 only has the pilot CNEFE/RFB
  extracts.
- Private paths on `sorrimobi-vps`:
  - `/opt/abastevo-temp/map-national-20261026/work/phase2-match-final-20261026/`
  - `/opt/abastevo-temp/map-national-20261026/work/phase2-open-geocode-20261026/`
  - `/opt/abastevo-temp/map-national-20261026/work/phase3-coverage-20261026/`

## Static overlay served (2026-10-26)

- `backend/cmd/mapoverlay` built release
  `phase3-coverage-sample-20261026-5e8d5a74` from the active generation and the
  latest official week: 565 assertions, 19 UFs, 102 with prices.
- `/dados/mapa/current/manifest.json` and per-UF files are served by the staging
  web tier with immutable caching; the staging edge returned `200`.
- See [MAP_NATIONAL_STATIC_OVERLAY_RECEIPT](MAP_NATIONAL_STATIC_OVERLAY_RECEIPT.md)
  and the [operator guide](../operator/map-static-overlay.md).

## Still blocked

- 1,574 L3 `ANP` points need owner decision D1.
- 186 L4 `address-point` candidates need owner decision D2.
- A1.a national API coverage is blocked by repeated HTTP 429; the current
  coverage is not national.
- Any future apply needs an explicit staging DSN and explicit owner
  authorization. The generated plan contains only `<STAGING_DSN>` placeholders
  and never a credential.

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

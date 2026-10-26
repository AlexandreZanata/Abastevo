# National map staging apply receipt

Status: **APPLIED on staging only**. Environment `abastevo-temp` (VPS
`sorrimobi-vps`), not production. No production namespace, deployment or app
was changed.

Applied at `2026-10-26T23:04:03Z`.

## Pre-apply state

- Reviewed location revisions: 0.
- Map generations: 0.
- Active map generation: none.

## Applied

### `station-review` (L1)

- Input: 361 reviews plus the hash-verified manifest.
- Result: 361 applied, 0 skipped, 0 failed.
- Reviewed location revisions after: 361.
- Report SHA-256:
  `008882818009b8e18c2b7fea206c0df89af604892fb6b364312760d2c2364253`.
- Dry-run report SHA-256:
  `d5dd25562a119e2a829fe4d7c8a9e3b8ea774d8adb2f69f54916107c47770bd7`.

### `map-load` (L2)

- Generation: `02ea6e7d-19ce-4942-a369-1aa7efdd03dd`.
- Status: `ACTIVE`.
- Source revision: `phase3-coverage-sample-20261026`.
- Assertions: 565.
- Map generations after: 1; active generations after: 1.
- Output SHA-256:
  `0f4162e84d74a3fd2ee43e55cb0698a3fa28fcbc6ce36281feb9e24b6de3476a`.

## Smoke

- `/v1/map/assertions` returned the active generation above.
- The endpoint accepts `source=ANP` or `source=COMMUNITY` only. The applied
  generation contains OSM `osm-mapped` assertions, so the ANP-filtered smoke
  returned an empty item list while exposing the active generation. The OSM
  assertions are stored in the generation; serving them through this price
  endpoint is a separate product/API decision and was not changed here.
- Reviewed location revisions were counted directly in the staging database.

## Blocked (not applied)

- 1,574 L3 `ANP` points: owner decision D1 is still open.
- 186 L4 `address-point` candidates: owner decision D2 is still open.
- A1.a national API coverage: blocked by repeated HTTP 429; the applied
  coverage is a pipeline sample, not a national claim.

## Evidence and rollback

- Receipt JSON on the private VPS root:
  `/opt/abastevo-temp/map-national-20261026/work/phase3-apply-receipt.json`,
  SHA-256 `c580a8a3e397201b9379527acb3983e7c740185a9126747b04d2db982c8e71e5`.
- Apply plan and coverage:
  `/opt/abastevo-temp/map-national-20261026/work/phase3-coverage-20261026/`.
- The temporary staging pod `abastevo-map-apply-20261026` was deleted after the
  apply.
- Rollback was not needed: both commands succeeded and there was no previous
  active map generation to restore.
- No database DSN or credential was printed, committed or stored in Git.

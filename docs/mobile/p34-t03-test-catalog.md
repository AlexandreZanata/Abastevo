# P34-T03 — Bounded synthetic integration dataset

Status: LOCAL_DONE on `codex/phase-34-vps-connection`. Task: P34-T03.
Binds B-BR-001 (source separation), B-BR-008 (stale honesty), B-BR-011
(no identity/GPS/photos in fixtures) and B-BR-014 (unknown location stays
explicit). Spec only: no VPS seeding, no population claim, no national
ingestion.

## Deterministic catalog (3 stations, owned markers)

Manifest: `contracts/testdata/p34/catalog.json` (`p34-staging-catalog v1`).

| key | display | CNPJ | IBGE | quality | coords |
|-----|---------|------|------|---------|--------|
| alfa | `[P34-TEST] Posto Alfa` | 04218406000104 | 3550308 | reviewed | -23.550, -46.633 |
| beta | `[P34-TEST] Posto Beta` | 11222333000181 | 3550308 | reviewed | -23.555, -46.640 |
| gama | `[P34-TEST] Posto Gama` | 12ABC34501DE35 | 3550308 | unknown | none |

Reuses the proven local alfa/beta/gama shape (reviewed + nearby + honest
unknown). All displays carry `[P34-TEST]`; CNPJs are the synthetic local-test
set, distinct per station; no emails, tokens, photos or precise user GPS.

## Exercise cases (P35-P37 journeys, no live dependency)

- list → `station-list-valid.json`; nearby → `nearby-valid.json` (sample point
  only, never echoed/stored); detail → `station-valid.json`.
- empty-price → `price-groups-valid.json` shape (200 empty page is valid).
- community-valid → `community-price-valid.json` (primary); disputed →
  `community-price-disputed-valid.json` (no amount).
- stale → expiry-vs-clock per B-BR-008 (covered by consensus/expiry suites).
- invalid-lat → 400 (`TestNearbyHonesty`); unknown-uuid → 404
  `station.not-found` (`TestDetailEnvelope`).

## Seeding, repeatability and cleanup (write scope only)

- Only with explicit write authorization, through existing mechanisms:
  `ResolveCNPJ` + `RecordLocation`/`ProjectLocation` per database (the
  `freshRouter` pattern) and the validated ANP import for official prices.
  No invented admin API, no global reset (`TRUNCATE` forbidden), no
  alteration of another app's data.
- Local repeatability: per-run disposable databases (`CREATE/DROP DATABASE`,
  migrations applied, constraints enforced); cleanup deletes only created
  station UUIDs. Staging, when authorized: check-then-create by CNPJ, owned
  `[P34-TEST]` markers only, cleanup by owned UUIDs.
- Accounts/contributions in later phases use synthetic test identities only;
  private photos stay out of Git/fixtures; all-copy 24h expiry is owned by
  P37-T03 with its storage-gated proof.

## Validation

- RED: `TestP34Catalog*` failed with missing `contracts/testdata/p34/catalog.json`.
- GREEN: `go test ./internal/platform/apicontract/ -run TestP34` → PASS (2/2).
- Regression: full `apicontract` suite PASS; `vacuum lint` PASS; real-PostGIS
  `go test -tags=integration ./internal/modules/directory/...` PASS (proves
  the reused seeding/read mechanisms, incl. 400/404 honesty).
- No migrations touched; no auth writes; no PII/secrets/GPS in manifest.
- `git diff --check` PASS; secret-surface review PASS.

## Limits and next

- NOT SEEDED: staging holds no P34 fixtures; population explicitly unclaimed.
- BLOCKED_LIVE: staging reads still unverified (TLS trust gap, `curl` 60).
- OWED: end manual/device batch, provider/media proof. No PR/CI/merge/wiki per
  ADR-018. iOS archived. G09 UNCERTIFIED.
- Next: P34 phase exit (LOCAL_DONE checkpoint + evidence), then P35
  (`codex/phase-35-live-discovery` via `--from-checkpoint`).

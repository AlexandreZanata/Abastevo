# P25-T02 — Bounded CSV snapshot staging

Status: LOCAL_DONE on `codex/phase-25-national-registry`. Task: P25-T02.
Binds B-BR-D02/D03/D05/D10 and BUC-D01/D06. Staging only: bounded
parser + run ledger/checksum/checkpoints; publication stays P25-T04.
Append-only migration; no station/price/trust writes.

## Behavior (TDD, critical-first)

- Migration `000031_registry_sources.sql` (append-only):
  `registry_source_runs` (source/snapshot/checksum/parser/state +
  accepted/duplicates/rejected/error_code) with
  `(source, snapshot_identity)` uniqueness, and `registry_assertions`
  (stable `source_key` = CNPJ text, row checksum, validated address/
  status/eligibility fields, nullable `station_id` for T04 resolution)
  with `(source, source_key, checksum)` replay idempotence. Publishers
  read complete runs only. (`authorization` renamed `auth_state`:
  reserved SQL keyword, caught by `sqlc vet`.)
- sqlc: directory stanza extended with the new migration;
  `db/queries/directory/registry.sql` (create/get/finish run, stage
  assertion with `ON CONFLICT DO NOTHING`, counts); `sqlc vet` +
  `generate` clean, no generated diff.
- `registry/stage.go` `StageCSV`: bounded read (100 MB cap → failed
  `oversize` run), sha256 snapshot checksum, create-or-join run
  (replay returns the existing report; concurrent stagers converge via
  the assertion key), header-name detection (missing/unknown →
  quarantined run, zero assertions), per-row validation (kernel CNPJ
  text, required identity/address, status→auth/eligibility mapping),
  row sha256, short batches, full accounting
  (accepted+duplicates+rejected = rows seen). Truncated input, row-cap
  breach and stage errors fail the run explicitly; empty input
  quarantines; the last-good publication is never touched (staging
  writes nothing outside its own tables).
- Fixed along the way from real failures (not weakened): reserved
  column rename, `CreateRegistryRun` no-row-on-conflict handling,
  duplicate counting via `RETURNING` absence, made-up CNPJs replaced
  by check-valid synthetic values.

## Validation

- Unit: `registry` package 11/11 PASS (`-race`): header
  require/unknown/missing, CNPJ preserve/reject, status freeze,
  eligibility matrix, manifest markers + no-PII, row accounting,
  replay idempotence, quarantine/oversize/truncated/empty/row-cap/
  stage-error paths (fake store).
- Integration (real PostGIS, `-race -tags=integration`): replay +
  4-way concurrency converge on 3 assertions; manifest sample
  quarantines end to end with 0 assertions; truncated preserves the
  single last-good complete run; empty quarantines with 0 complete
  runs. Fresh disposable DBs prove the append-only 000031 upgrade on
  previous schemas (empty/upgrade/recovery).
- Regression: full `directory/...` unit + integration PASS (8 pkgs
  each); `go vet` clean; `vacuum lint` + `apicontract` PASS
  (contracts untouched).
- `git diff --check` PASS; `scan-secrets.sh` PASS (no PII/secrets;
  all CNPJs synthetic `[P25-TEST]`).

## Limits and next

- No live ANP fetch was performed (sandbox egress + provisional
  columns); national-scale calibration stays P29-owned.
- Next: P25-T03 paginated/targeted ANP API discovery on this branch.

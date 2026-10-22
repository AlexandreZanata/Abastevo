# MEX-20b — Verified typed enforcement artifacts

Status: LOCAL_DONE
Validation: PASS
Integration: INTEGRATION_PENDING

Parent: `6a04d1ac` (maintained dev, PR #143 merge). Date: 2026-10-22.
Depends MEX-20a, accepted. Rules B-BR-MEX-10/11/12/16; BUC-MEX-07.
Risk: critical ingest/PII (typed boundary over source subjects; no SQL,
no persistence, no publication in this unit).

Before behavior: the Go side reads the file-backed native artifacts of
`prepare_enforcement` and produces verified typed values, reusing the
quality prepared-bridge pattern and no new package. The manifest is
strict and versioned: schema, source profile, source name, input
identity, reconciled counts, census shape and the exact output set are
all required. The record stream is bounded and integrity-checked
(bytes and SHA-256). Every record field is validated exactly — a valid
CNPJ only with a company shape, a two-letter UF, verbatim source
wording within bounds, a calendar day, a non-empty document number, a
locator inside the source census, and recomputed content hashes. The
census aggregates the manifest states are recomputed from the records
and compared exactly. Canonical IDs derive from the verified content
hash. A late tamper, truncation, cancellation, close, oversize or
unknown profile returns no partial batch. No record is bound to a
station, company or site here (B-BR-MEX-11 stays MEX-21); no fine,
guilt, payment or finality is derived (B-BR-MEX-10). Recovery: keep
live registration absent — this unit persists and publishes nothing;
atomic typed-boundary commit.

## Delivered

- `backend/internal/modules/quality/application/enforcement_source.go`:
  `EnforcementSourceRecord` — the typed XLSX record, separate from the
  CSV `EnforcementInput`. The document date is the source DATA DO DF
  value; the subject identifier stays empty unless admitted.
- `backend/internal/modules/quality/adapters/prepared/enforcement.go`:
  `LoadEnforcement` — strict manifest verification
  (`enforcement-native-manifest-v1` plus the pinned
  `anp-enforcement-xlsx-2019-v1` profile), bounded stream bridge over
  the single records file (200 MB output budget, the pinned revision
  emits ~128 MB), per-record exact validation with recomputed
  `record_hash` and `tuple_hash`, locator census (rows 2..input+1,
  unique), repeat/twin accounting, full census recomputation compared
  with the manifest, and canonical `enforcement-source:` IDs. No
  resolver, no station binding, no persistence.
- Native contract (MEX-20a code, fixed in this commit): the manifest
  carries `source_profile`, and each record carries `tuple_hash` (the
  distinct-tuple census key over the raw subject, so the typed boundary
  verifies the tuple census without ever receiving a personal
  identifier). Additive only; the reader, profile and minimization are
  unchanged.
- `.github/workflows/backend.yml`: build `prepare_enforcement` locked
  and export `ANPFUEL_TEST_ENFORCEMENT_PREPARER_PATH` for the
  Rust→Go round-trip.
- `backend/internal/modules/quality/adapters/prepared/enforcement_test.go`:
  five synthetic cases — verified batch with canonical IDs, tamper
  (stream and content hash), unknown profile/schema, six invalid-value
  shapes, and no-partial-batch (truncation, cancellation, missing port).
- `backend/internal/modules/quality/adapters/prepared/enforcement_integration_test.go`:
  synthetic XLSX built with the standard library → trusted preparer →
  bridge (runs in CI), and the full-source oracle on the pinned
  workbook (env-gated, local acceptance).

## Actual checks

From this worktree (2026-10-22, parent `6a04d1ac`):

- `go test -count=1 -run TestLoadEnforcement
  ./internal/modules/quality/adapters/prepared/` → PASS, 5 tests.
- `go test -race -count=1 -tags=integration
  ./internal/modules/quality/adapters/prepared/` (all five preparer
  paths set) → PASS, including the pre-existing quality/fine bridges
  with no regression.
- `TestRustEnforcementPreparerRoundTrip` → PASS: synthetic XLSX →
  `prepare_enforcement` → `LoadEnforcement`; company record typed with
  `2019-01-02`, personal record without identifier, no address leak.
- `TestRustEnforcementFullSourceOracle`
  (`ANPFUEL_TEST_ENFORCEMENT_XLSX_PATH` pointing at the pinned private
  workbook, 16,188,087 bytes) → PASS in 41.9 s: 244,247 records loaded,
  census recomputed by Go matches the Rust manifest on every checkable
  aggregate (documents, tuples via `tuple_hash`, result vocabulary, max
  chars, over-64, segments, shapes, admitted, dates); subject census
  242,948 company / 1,294 personal / 5 other with no identifier on a
  non-company record and day precision on every record.
- `go test -count=1 ./internal/modules/quality/...` → PASS, all
  packages; `go vet ./...` PASS; `GOTOOLCHAIN=go1.27.1 staticcheck
  ./internal/modules/quality/...` PASS.
- `cargo test --locked` → PASS, 168 tests, zero failure;
  `cargo fmt --check` PASS; `cargo clippy --all-targets --locked --
  -D warnings` PASS (tuple_hash assertion added to the synthetic
  profile test).
- `git diff --check` PASS; `bash scripts/scan-secrets.sh` PASS.
- No SQL, migration, OpenAPI or Android input changed; `sqlc` and
  `vacuum` have no affected surface.

## Limits

- This is the typed boundary only. There is no storage, migration or
  staging (MEX-20c), no publication or frozen read (MEX-20d), no
  worker, discovery or image (MEX-20e) and no whole-source acceptance
  (MEX-20f). The batch admits no source and activates nothing; the
  private workbook never enters Git, logs or fixtures.
- The bridge requires the pinned profile
  (`anp-enforcement-xlsx-2019-v1`) and the `tuple_hash` census key. A
  manifest or record without them is refused as an unknown profile, not
  adapted. Native artifacts produced before this commit (without those
  fields) are refused for the same reason.
- The tuple census is verified through `tuple_hash`, not raw subjects:
  Go never receives a personal identifier, and the distinct-tuple count
  agrees exactly because the hash is injective over the source tuples.
- `members`/`expanded_bytes` are budget-checked, not recomputed (the
  bridge does not open the ZIP); `nonnumeric_dates` is bounded by the
  quarantined count. Everything else in the census is recomputed.
- No device, no production, no deploy and no release acceptance is
  claimed.

Next: MEX-20c (append-only storage) needs its own authorization; it
consumes this tested typed boundary.

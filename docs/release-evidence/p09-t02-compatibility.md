# P09-T02 compatibility and documentation closure

Date: 2026-09-30. Candidate: `4ce5aaf` (P08 merge); rehearsal tree adds
only harness/docs (P09-T01) plus this closure. No product behavior change,
no Android source edits. Kotlin↔Go harness stays P10 work.

## A01–A13 resolution (CURRENT_STATE_AUDIT + BASELINE_VALIDATION)

- A01 FTS: Room FTS4 is the baseline; no replacement. Evidence: audit A01.
- A02 schema/deps: Room v4 + data→application graph kept; prose fixed in
  P01-T01. Evidence: audit A02, `AnpFuelDatabase.kt`, `data/schemas/1–4`.
- A03 corrections: backend revisioned publication; Android idempotent
  keep-first unchanged. Evidence: P02-T06 + audit A03.
- A04 units: milli-BRL integers, L/M3/KG_13 per contract; Android 2-decimal
  + REAL columns untouched. Evidence: `contracts/openapi/v1.yaml#Money`.
- A05 fuel names: wire has `GASOLINE_ADDITIVED`, never `GASOLINE_PREMIUM`;
  P10 maps legacy via named adapter. Evidence: openapi `FuelProduct`.
- A06 CNPJ: backend alphanumeric + leading zeroes + quarantine; Android
  numeric-only stays. Evidence: P02-T02/T03.
- A07 parser: fixture layout detection, not prose rows. Evidence: P02-T04.
- A08 geocoding: no bulk Nominatim; permitted provider + quotas (P02-T07).
- A09 privacy: old policy is imported-app context; backend notice is
  `docs/backend/PRIVACY_NOTICE.md` draft + P07 retention/runbooks.
- A10 API rules: public reads, no tenants, real PostGIS per ADR-004.
- A11 publishing: v3.1.0, UC-001…015, no `.local` links, no tag scripts.
- A12 license: MIT `LICENSE` verbatim retained.
- A13 secrets: tracked + changed-file scans in CI; untracked inspected
  separately; `make quick-verify` clean.

Frozen deltas: `contracts/testdata/compat/legacy-deltas.json`
(`legacy-deltas-v1`) pins the above for P10 adapters.

## Validation

- `vacuum lint` openapi: 0 errors, 9 infos, score 99/100 (A+).
- `go test ./internal/platform/apicontract/...`: ok (golden vectors).
- Android regression baseline (supported env, no source changes):
  `./gradlew :domain:test :application:test :data:testDebugUnitTest :app:testDebugUnitTest :app:assembleDebug --no-daemon`
  → BUILD SUCCESSFUL in 11s, 88 tasks up-to-date. Limitation: cached
  up-to-date (no Android inputs changed); not a fresh instrumentation run.
- `bash scripts/check-compat.sh`: ok. Harness `test-compat.sh`: happy +
  2 mutant refusals (broken enum, missing evidence).
- Runbooks present: `docs/operator/{deploy,backup,recovery,monitoring,edge-cache}.md`,
  `docs/MIGRATION_PLAN.md`, `docs/product/PRODUCT_CONTRACT.md`. Links are
  file-presence + contract cross-refs; no full link-crawl claimed.

## Non-claims

- No Kotlin↔Go fixture harness passing claimed; P10-T01 owns it.
- No instrumentation (`connectedDebugAndroidTest`), coverage, lint or live
  ANP import executed here.
- No breaking backend semantic introduced; wire enum and money/units frozen.

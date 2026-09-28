# Test strategy and gates

Tests below are planned unless explicitly reported in BASELINE_VALIDATION. No backend test suite exists yet. Avoid claiming a planned command ran.

## Risk-based checks

- Documentation-only: link/path/roadmap consistency, validateRepoBaseline where relevant, `git diff --check`; inspect untracked new files separately because ordinary diff cannot cover them.
- Domain: document B-BR and GIVEN/WHEN/THEN → a focused failing test → minimum implementation → refactor. Pure deterministic unit tests, injected clock/IDs, no real DB/network/files. Aim for meaningful ≥90% domain coverage, including boundary/invalid cases; percentage does not replace behavior review.
- Application: fake ports test authorization, atomic intent, error mapping and orchestration; no mocked assertion that merely mirrors implementation.
- Persistence/geo/jobs: real PostgreSQL/PostGIS, production migrations/SQL, concurrent transactions, lease expiry/fencing, uniqueness and dead-job recovery. Use a dedicated disposable Compose service initially; Testcontainers only if it solves a concrete CI need.
- HTTP/security: real handlers with httptest; request/response contract validation, body/time limits, auth, replay, quota, IDOR, owner-only status, no-store and error redaction. Proxy/staging tests cover authority/signature forwarding and actual CDN behavior.
- Evidence: malformed/oversize/decompression inputs, MIME spoof, overwrite-after-HEAD race, orphan lifecycle, repeated finalize, owner mismatch, EXIF removal and deletion. Local S3-compatible tests plus one staging R2 suite, because emulator compatibility is insufficient evidence.
- ANP/golden: immutable public samples, source checksums, exact money/units, all labels/dates/CNPJ forms, header change/quarantine, correction revisions and Kotlin legacy compatibility cases. Live source discovery is a scheduled/manual smoke, not a flaky prerequisite for every unit test.
- Migration: apply all from empty, upgrade previous released schema with representative rows, migration checksum drift, concurrent migrator lock, incompatible binary refusal, expand/contract behavior and failed-migration recovery. A destructive down migration is not the default rollback.
- E2E: register → upload → finalize → observe → validate → independent confirm → read source-separated projection; dispute/moderate; rights deletion/restore. Synthetic protocol clients until G09; no Android feature implementation to drive backend tests.
- Load/recovery: INFRASTRUCTURE_PLAN workload and latency budgets, full restore into new environment, DB/storage outage and worker crash. Store hardware/data/config/commit with results so runs can be compared.

## Reproducible command contract

Existing Android commands from repository root:

```sh
bash scripts/validate-repo-baseline.sh
./gradlew :domain:test :application:test
./gradlew :data:testDebugUnitTest :app:testDebugUnitTest
./gradlew :domain:jacocoTestCoverageVerification :app:lintDebug :app:assembleDebug
./gradlew :data:connectedDebugAndroidTest :app:connectedDebugAndroidTest
```

Requires JDK 17, Android SDK 35 and emulator/device for instrumentation. The local audit did not satisfy these prerequisites. Do not change toolchain versions merely to make this workstation pass.

Planned backend gate commands, **to be introduced in P01**; examples below run from `backend/` unless noted:

```sh
go test ./...
go test -race ./...
go vet ./...
staticcheck ./...
govulncheck ./...
sqlc vet
sqlc generate
git diff --exit-code -- internal/
go test -tags=integration ./...
```

Use `gofmt -l` with an explicit fail-if-output check (gofmt listing alone can exit zero). `sqlc generate` runs only after config/SQL exist; generated diff check belongs in a clean CI checkout. Pin staticcheck/sqlc/govulncheck and OpenAPI validators. The integration job must set a documented disposable DATABASE_URL and apply migrations; it must fail, not skip, if PostGIS is unavailable. P01 defines exact invocation/env in backend/README before later tasks rely on it.

Fast gate: compile, focused/all inexpensive unit tests, gofmt, vet, staticcheck; OpenAPI checks on contract changes; sqlc/migration integration on data changes; changed-file secret scan and dependency vulnerability scan. Full gate before backend release: all/race tests, real PostGIS, empty/upgrade migrations, end-to-end/auth/evidence, deploy/restore/load evidence, existing Android regression baseline and frozen cross-language fixture expectations. Full Kotlin↔Go harness is P10 because app work is gated.

## Fixture boundaries and anti-flakiness

`contracts/testdata/`: ANP normalization, price/condition/unit/fuel compatibility, identity signed-byte vectors, invalid signatures/replay, API errors, enum forward compatibility and consensus examples. Each fixture has ID/version/provenance and intentional Android differences. Synthetic identities/images only; never export production photos, keys or contributor traces.

Use fixed clock and UUID sequences, stable sort/tie-breaks, barriers rather than sleeps for races, bounded retry on external test infrastructure only. No random-based flaky assertions or wall-clock domain checks. Verify nondeterministic job backoff separately with bounded injected jitter. Every production bug gets a regression test before/with the fix.

## Gate evidence

For every task record commands, environment, pass/fail and limitation in docs/planning/PROGRESS.md or a linked release evidence file. No disabled/deleted tests to create green CI. A failed prerequisite is BLOCKED, not PASS. G09 cannot be checked off while restore/security/runtime/backend requirements remain theoretical.

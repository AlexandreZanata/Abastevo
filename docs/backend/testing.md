# Test strategy and gates

P01 platform and API-contract tests now exist; prior local runs are recorded in the P01 evidence archive and baseline validation. Business, end-to-end and deployed-infrastructure coverage below remains planned where its feature does not exist. Never infer a new successful run from historical evidence.

CI_PLAN owns cadence: focused local checks per task, specialized exit checks plus short CI per phase integration, and the complete matrix on the P09/G09 candidate. G01-FLOW and required Quick verification are active; G09-LOCAL permits app development and real G09 certification waits until G18. Targeted critical checks are never deferred to the end of the project.

## Risk-based checks

- Documentation-only: link/path/roadmap consistency, validateRepoBaseline where relevant, `git diff --check`; inspect untracked new files separately because ordinary diff cannot cover them.
- Domain: document B-BR and GIVEN/WHEN/THEN → a focused failing test → minimum implementation → refactor. Pure deterministic unit tests, injected clock/IDs, no real DB/network/files. Aim for meaningful ≥90% domain coverage, including boundary/invalid cases; percentage does not replace behavior review.
- Application: fake ports test authorization, atomic intent, error mapping and orchestration; no mocked assertion that merely mirrors implementation.
- Persistence/geo/jobs: real PostgreSQL/PostGIS, production migrations/SQL, concurrent transactions, lease expiry/fencing, uniqueness and dead-job recovery. Use a dedicated disposable Compose service initially; Testcontainers only if it solves a concrete CI need.
- HTTP/security: real handlers with httptest; request/response contract validation, body/time limits, auth, replay, quota, IDOR, owner-only status, no-store and error redaction. Proxy/staging tests cover authority/signature forwarding and actual CDN behavior.
- Evidence: malformed/oversize/decompression inputs, MIME spoof, overwrite-after-HEAD race, orphan lifecycle, repeated finalize, owner mismatch, EXIF removal and deletion. Local S3-compatible tests plus one staging R2 suite, because emulator compatibility is insufficient evidence.
- ANP/golden: immutable public samples, source checksums, exact money/units, all labels/dates/CNPJ forms, header change/quarantine, correction revisions and Kotlin legacy compatibility cases. Live source discovery is a scheduled/manual smoke, not a flaky prerequisite for every unit test.
- Migration: apply all from empty, upgrade previous released schema with representative rows, migration checksum drift, concurrent migrator lock, incompatible binary refusal, expand/contract behavior and failed-migration recovery. A destructive down migration is not the default rollback.
- E2E: register → upload → finalize → observe → validate → independent confirm → read source-separated projection; dispute/moderate; rights deletion/restore. Synthetic protocol clients remain local backend fixtures; after G09-LOCAL, shared/native clients add Android/iOS contract/device evidence.
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

Requires JDK 17, Android SDK 35 and emulator/device for instrumentation. P01 re-validation records a successful unit/build baseline after prerequisite setup; it does not imply instrumentation passed or every future environment has these prerequisites. Do not change toolchain versions merely to make a workstation pass.

Backend tools and initial gate scripts were introduced in P01; exact pins and disposable DB environment are in [backend/README](../../backend/README.md). The commands below are verification building blocks, **not a checklist to repeat after every task**. Examples run from `backend/` unless noted:

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

Use `gofmt -l` with an explicit fail-if-output check (gofmt listing alone can exit zero). `sqlc generate` runs only after config/SQL exist; generated diff check belongs in a clean CI checkout. Pin staticcheck/sqlc/govulncheck and OpenAPI validators. The integration job must set a documented disposable DATABASE_URL and apply migrations; it must fail, not skip, if PostGIS is unavailable. Follow the implemented invocation/env in backend/README; never point destructive integration fixtures at a persistent environment.

Target quick gate: format/build/vet, actual short critical/platform suites, reference/secret checks and applicable contract-generation checks, within a measured budget. A compile-only test command is not behavioral evidence. Migration/SQL/security changes require their relevant real DB/adversarial tests locally and at specialized phase exit; dependency changes require relevant vulnerability checks immediately. Do not turn the quick job into the whole release matrix.

Full gate before backend release: all unit/static/coverage/race tests, real PostGIS, empty/upgrade migrations, end-to-end/auth/evidence, dependency/image/security scans, deploy/restore/load evidence, existing Android regression baseline and frozen cross-language fixture expectations. Select the immutable candidate after phase integration and record the complete expected-result manifest. Reuse expensive evidence only for demonstrably matching artifacts/config/inputs under CI_PLAN. Full Kotlin↔Go↔Swift parity starts P12/P10 after G09-LOCAL; native/macOS evidence is mandatory where applicable.

## Enterprise sync resilience suite

One repeatable suite proves convergence of the enterprise sync failure cases
(lost response, wrong dependency order, duplicate and concurrent effects,
missing acknowledgement, killed process, truncated receipt upload and stale
cross-scope state) on both boundaries; see the
evidence and EDR-351..353.

```sh
# backend/: real PostGIS (ANPFUEL_TEST_DATABASE_URL) and the disposable
# ANPFUEL_TEST_STORAGE_* bucket of infra/compose.validation.yml (receipt test)
go test -race -count=1 -tags=integration -p 2 -run '^TestSyncResilience' ./internal/modules/organization/adapters/
# repository root, JDK 17: ordering rule, queue service and the device half
./gradlew :domain:test --tests 'com.anpfuel.domain.enterprise.EnterpriseQueueTest' \
  :application:test --tests 'com.anpfuel.application.enterprise.EnterpriseOperationQueueTest' \
  :data:testDebugUnitTest --tests 'com.anpfuel.data.enterprise.EnterpriseSyncResilienceTest'
```

The device half runs the real protected vault, queue, product producers,
queue service and signed HTTP client (a real child JVM for process death)
behind an edge ledger of the 10-offline-sync §10.2 rows; the server half runs
the real default enterprise runtime. The emulator stage is
`EnterpriseSyncResilienceNativeDeviceTest` (data androidTest, validation
build) against `TestReceiptNativeSourceFixture` (tags `integration
nativefixture`); it is owed until it runs on the owned emulator.

## Enterprise M4 gate scenario union

The M4 gate runs the owners' scenario tests for VAL-04, VAL-22 (part 2),
VAL-23, VAL-26, VAL-27, VAL-28 and VAL-02/VAL-21 over the F4 routes as one
command set; it adds no test. The mapping of each scenario clause to named
tests, the counts on the tested head and the owed device evidence are in the
gate evidence (EDR-360..362).

```sh
# backend/: real PostGIS (ANPFUEL_TEST_DATABASE_URL) and a disposable
# ANPFUEL_TEST_STORAGE_* bucket of infra/compose.validation.yml (down -v afterwards)
go test -race -count=1 -tags=integration -p 2 -timeout=150m \
  -run '^(TestLateIntake|TestTripLateIntake|TestStateIntake|TestSuspensionPreservesPersistedFieldReviewAfterReactivation|TestFuelingIntake|TestSyncResilience|TestSyncIntake|TestTrip|TestTracking|TestTrackProcessing|TestTrackRead|TestMeterCorrectionSuspectReplacementAmendmentIsolationAndRecalculation|TestMeterProjection|TestMeterSegment|TestChecklist|TestIncident|TestFleetReservation|TestAuthorizationMatrixActualCompanyHTTP|TestAuditorReadWriteMatrixActualCompanyHTTP)' \
  ./internal/modules/organization/adapters/
go test -race -count=1 -tags=integration -p 2 ./internal/modules/orgsync/adapters/ ./internal/modules/orgaudit/adapters/ ./internal/modules/fleet/adapters/
go test -race -count=1 -p 2 ./internal/modules/trip/... ./internal/modules/tracking/... ./internal/modules/orgsync/... \
  ./internal/modules/fleet/domain/... ./internal/modules/maintenance/... ./internal/platform/apicontract/...
go test -race -count=1 -run '^(TestAuthorizationInventory|TestFullProductionRouterEnterpriseContractAndDefaultOff)' ./cmd/api/
# repository root, JDK 17, serialized Gradle
./gradlew :domain:test --tests 'com.anpfuel.domain.enterprise.*' :application:test --tests 'com.anpfuel.application.enterprise.*' \
  :data:testDebugUnitTest --tests 'com.anpfuel.data.enterprise.*' :app:testDebugUnitTest --tests 'com.anpfuel.app.ui.enterprise.*'
bash scripts/check-mobile.sh --static-only
```

These are host and PostGIS proofs. The emulator stages and the real-device
tracking interruption matrix (at least three manufacturers) are listed in the
gate evidence as owed; a gate never records them as passed until they run.

## Enterprise M5 gate scenario union

Local M5 (`EMP-REL-006`, EDR-369..374) exercises `VAL-16`, `VAL-21`
(`org-permissions-v6`), `VAL-29`, `VAL-30`, and `EMP-ROUTE-010` across backend
PostGIS/MinIO, `business-web`, and Android host suites:

```sh
# backend/: real PostGIS (ANPFUEL_TEST_DATABASE_URL) and disposable compose storage (S3)
go test -tags=integration -count=1 -run '^(TestF5|TestAuthorizationMatrixActualCompanyHTTP|TestAuditorReadWriteMatrixActualCompanyHTTP|TestTracking)' ./internal/modules/organization/adapters/
go test -race -count=1 ./internal/modules/tracking/... ./internal/modules/orgaccess/... ./internal/platform/apicontract/... ./internal/platform/enterprisesuite/...
npm --prefix business-web run check && npm --prefix business-web test && python3 business-web/tests/caddy.py
bash scripts/check-mobile.sh --static-only
```

## Enterprise M6 gate scenario union

Local M6 (`EMP-REL-007`, `EMP-REL-010`, EDR-375..385) exercises `VAL-10`,
`VAL-11`, `VAL-15`, `VAL-20`, `VAL-21` (`org-permissions-v7`), `VAL-31`,
`VAL-32`, `VAL-37`, and `VAL-38` across backend PostGIS/MinIO, `business-web`,
OpenAPI contract tests, and Android host suites (while physical M4 device matrix,
VPS activation, and production `G09` remain explicitly owed):

```sh
# backend/: real PostGIS (ANPFUEL_TEST_DATABASE_URL) and disposable compose storage (S3)
go test -tags=integration -count=1 -run '^(TestManagementF6EndToEndFlow|TestAuditorReadWriteMatrixActualCompanyHTTP|TestAuthorizationMatrixActualCompanyHTTP|TestExportsThroughActualCompanyHTTPAndWorker)$' ./internal/modules/organization/adapters/
go test -tags=integration -count=1 ./internal/platform/runtimegrants/... ./internal/modules/orgexport/...
go test -count=1 ./internal/platform/apicontract/... ./internal/platform/orgapi/... ./internal/modules/intelligence/... ./internal/modules/fuelstock/... ./internal/modules/maintenance/... ./internal/platform/enterprisesuite/...
go test -bench=BenchmarkSyntheticFleet1000VehiclesKPIAndFindings -benchmem ./internal/modules/intelligence/domain/...
npm --prefix business-web run check && npm --prefix business-web test
bash scripts/check-mobile.sh --static-only
```

## Fixture boundaries and anti-flakiness

`contracts/testdata/`: ANP normalization, price/condition/unit/fuel compatibility, identity signed-byte vectors, invalid signatures/replay, API errors, enum forward compatibility and consensus examples. Each fixture has ID/version/provenance and intentional Android differences. Synthetic identities/images only; never export production photos, keys or contributor traces.

Use fixed clock and UUID sequences, stable sort/tie-breaks, barriers rather than sleeps for races, bounded retry on external test infrastructure only. No random-based flaky assertions or wall-clock domain checks. Verify nondeterministic job backoff separately with bounded injected jitter. Every production bug gets a regression test before/with the fix.

## Gate evidence

Record the commands you ran and their results in the pull request description.

## Local process validation

The public-process integration test in `cmd/api` starts the actual API with a
fresh database, requires an explicit disposable `ANPFUEL_TEST_DATABASE_URL`, and
checks schema readiness, challenge/enrollment, exact wire fuel/unit, idempotency,
replay, owner isolation and rotation. Set all `ANPFUEL_TEST_STORAGE_{ENDPOINT,
BUCKET,REGION,ACCESS_KEY_ID,SECRET_ACCESS_KEY}` values to enable the real worker
and private S3-compatible photo flow (valid JPEG, malformed image, independent
confirmation and dispute). Missing storage selects the deliberate 503 path;
it does not count as successful S3 validation.

`ANPFUEL_ANP_DISCOVERY_ENABLED=false` suppresses the external-source scheduler
for isolated process tests. Its default is true; disable it explicitly for local
synthetic runs. Local S3 and local HTTP/TLS results do not certify real R2/CDN,
production infrastructure or a public release. Private/no-store responses and
mutations must never become 304 through `If-None-Match`. Storage transports must
refuse redirects and omit signed URLs and remote response bodies from errors.

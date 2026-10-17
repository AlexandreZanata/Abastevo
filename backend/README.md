# Backend workspace

Status: P01-T01…T12 foundations exist: API/worker/migrator roots, typed config,
redacted telemetry, HTTP/health lifecycle, pgx pool, migration runner, sqlc,
container definitions and base OpenAPI vectors. Business modules and
production infrastructure remain ahead. See [current state](../docs/planning/PROGRESS.md)
and [prior task evidence](../docs/planning/history/P01_FOUNDATION_EVIDENCE.md);
this documentation revision does not rerun those suites or verify remote CI.

Tooling pins: Go toolchain `go1.27.2` (`go.mod`); `sqlc v1.31.1`
(`sqlc vet && sqlc generate` from `backend/`; generated packages committed).
`chi v5.3.2`, `pgx v5.11.0` (both MIT, permissive transitives only).
`staticcheck v0.8.1` (MIT), `govulncheck v1.8.0`, `vacuum v0.30.6` (MIT).
Static binaries must be built with Go ≥1.27 or typechecking fails; install
with `GOTOOLCHAIN=go1.27.2 go install <tool>@<version>`. The pinned
staticcheck predates the go1.27.2 export format, so the fast gate runs
`staticcheck` itself under `GOTOOLCHAIN=go1.27.1` (see
`scripts/check-backend-fast.sh`).

## Verification entry points (P01-T13)

Shared local/remote composition lives in `Makefile` and `scripts/`:

```sh
make quick-verify    # bounded gate: manifest + selection + fast checks + govulncheck
make verify-release  # quick + full-matrix report (foundation subset, NOT CERTIFIED)
make test-gate       # focused harness for selection/failure behavior
```

Manifest `scripts/check-manifest.txt` lists required quick packages and
checks; missing packages or zero matched tests fail. Docs-only changes run
docs checks plus contract reference integrity; code changes run the full
quick set. Unclassified paths, missing tools and secrets in working tree or
committed PR diff fail. `go test -run '^$'` is compile-only and never counts
as behavioral evidence. Measured quick budget is 300s warm cache.

Required CI is the always-on **Quick verification** job (`.github/workflows/quick.yml`):
it runs `make quick-verify` on every PR including drafts plus main pushes, with
no path filter, so docs-only changes still report a meaningful required result.
`backend.yml` fast/integration and `ci.yml` test keep running path-triggered as
non-required signal; their task-level and phase-exit evidence stays mandatory
per [CI_PLAN](../docs/planning/CI_PLAN.md).

Existing gates below remain binding until G01-FLOW T17 migrates CI/protection.

Fast gate, no database (from the repository root):

```sh
bash scripts/check-backend-fast.sh   # gofmt, build, vet, unit tests,
                                     # staticcheck, sqlc vet/diff, vacuum lint,
                                     # tracked + changed-file secret scans
cd backend && govulncheck ./...
```

Integration gate, real disposable PostGIS (from the repository root):

```sh
cargo +1.99.0 build --locked --manifest-path tools/station-prep/Cargo.toml --bin prepare_registry --bin prepare_quality --target-dir /tmp/abastevo-integration-rust
export ANPFUEL_TEST_PREPARER_PATH=/tmp/abastevo-integration-rust/debug/prepare_registry
chmod 755 /tmp/abastevo-integration-rust/debug/prepare_quality
export ANPFUEL_TEST_QUALITY_PREPARER_PATH=/tmp/abastevo-integration-rust/debug/prepare_quality
docker compose -f infra/compose.dev.yml up -d db
(
  cd backend
  export ANPFUEL_DATABASE_URL=postgres://anpfuel:anpfuel@127.0.0.1:5434/anpfuel?sslmode=disable
  export ANPFUEL_TEST_DATABASE_URL="$ANPFUEL_DATABASE_URL"
  go run ./cmd/migrate && go test -race -count=1 -tags=integration ./...
)
```

Integration fails when PostGIS is unreachable; it never skips. CI
(`.github/workflows/backend.yml`) runs both gates path-aware on backend,
contract, infra and gate-script changes; the Android workflow keeps running
`./gradlew test` on Android paths and skips pure docs/contract/backend edits.

Continue with [P01-T13](../ROADMAP.md#p01-t13); read [architecture](../docs/backend/TARGET_ARCHITECTURE.md) and [test strategy](../docs/backend/TEST_STRATEGY.md) as the selected task requires. P02 starts after G01 and G01-FLOW; subsequent phases add owned business modules and explicit SQL incrementally.

The Android source remains in its original root modules. Do not move it under a new mobile directory.

## Complete local backend validation

From the repository root, `bash scripts/tests/test-local-backend.sh` creates a
unique disposable Compose project with tmpfs PostGIS and a private S3 emulator.
It runs all unit/integration suites under the race detector, actual API + worker
HTTP/media flows, static/vulnerability checks, infrastructure/deploy harnesses,
encrypted backup, restored deletion-ledger replay, bounded load and DB outage.
It clears inherited runtime DSN/storage settings and never selects the shared
`infra/compose.dev.yml` database. Only its own project is removed on exit.
Reports remain in a printed `/tmp` directory; set `ANPFUEL_LOCAL_REPORT_DIR` to
choose a local output path. The container credentials are synthetic test values.
These tmpfs services lose all data on removal; they are not production storage.

For an already running synthetic API on a local/private literal IP, run:

```sh
ANPFUEL_LOCAL_API_URL=http://172.19.2.11:18093 bash scripts/tests/test-local-edge.sh
```

This starts the pinned production Caddy image with an ephemeral internal CA,
validates TLS 1.2/1.3, health and route isolation, and checks synthetic location/
signature/cookie markers are absent from logs. It does not modify system trust
or obtain public certificates. The test edge and its keys are removed on exit.
Local test success is distinct from G09 certification and public deployment.

### Photo-intake safety budget (RVA07)

`ANPFUEL_EVIDENCE_MAX_PENDING_PHOTOS` defaults to **0 (closed)**. The API
requires an explicit measured budget in 1..100 pending photo sessions and
private storage configuration. Keep the same budget across API replicas;
the source ceiling is a conservative guard, never a supported-user or VPS
capacity claim. A session can require both an original and a final copy.
Qualify the current binary/provider/DB resource envelope and deadline margin
before enabling intake in staging/production; record the measured lower
budget in deployment evidence. Increasing the source ceiling requires renewed
qualification. There is no production activation in this source checkpoint.

Every-minute evidence cleanup records a singleton epoch-fenced observation.
Startup, a running/failed/unknown pass, a successful observation older than
three minutes, a copy at its 22h cleanup boundary, a full pending budget or
a DB outage pauses new bytes. Missing configuration is not healthy by default.
Fresh reservations serialize the global capacity check and insertion;
confirmed byte replay emits no PUT. The existing worker process has four
serial safety dispatchers alongside one general dispatcher, with no new service.
The operational copy deadline stays first receipt +24h; cleanup starts at 22h.
Provider/version/backup and autonomous elapsed-time proof remain required.

## Bounded ANP location-candidate discovery

See the [Sorriso reliability audit](../docs/backend/STATION_RELIABILITY_AUDIT_20261013.md).
`go run ./cmd/station-discover --snapshot <immutable-edition> --uf MT --municipio SORRISO`
uses `ANPFUEL_DATABASE_URL` and stages only official records/candidate points.
It runs no migrations, canonical publication or location review. Exact snapshot
replay reuses stored results; new source editions need new snapshot identities.
Do not use a persistent database for integration tests.

## Fuel quality source reads (FQ-08/FQ-08H)

The additive public quality API has a live bounded revision catalog:
`GET /v1/quality/revisions?stream=pmqc-assay-v1&limit=20`, then
`GET /v1/quality/revisions/{id}?stream=pmqc-assay-v1` for frozen provenance.
Streams also include `enforcement-v1` and `sanction-v1`. Existing summary, assay,
sample, action and sanction routes accept optional `revision_id`; omission reads
current. Select the exact revision when continuing a historical record cursor.
Superseded verified history is explicitly labeled; withdrawn/legacy revisions
have unavailable facts and null denominators. Catalog metadata is live, so restart
catalog traversal after imports to include newly inserted revisions.

`ANPFUEL_QUALITY_READS_ENABLED=false` contains all official quality routes.

Native PMQC worker imports have a separate default-off switch,
`ANPFUEL_QUALITY_IMPORTS_ENABLED=false`. The general dispatcher always excludes
`quality-import` and `quality-discovery`; disabled source jobs retain zero claim
attempts. Explicit import enablement adds one fixed slot, using the trusted
`/prepare_quality` executable and a separate PostgreSQL advisory-lock connection
across fetch, preparation and publication (210-second attempt, five-minute job
lease). Safety slots remain independent. No live discovery schedule is installed
by this switch. Keep it disabled until image packaging, bounded live discovery
and post-publication Directory revalidation are accepted; see
[FQ-05W5](../docs/backend/evidence/fuel-quality/FQ05W5_WORKER_COMPOSITION.md).
Private signed document-free report commands/get/list remain separate; disabling
`ANPFUEL_QUALITY_REPORTS_ENABLED` contains new/edit intake while preserving signed
owner appeal, withdrawal and reads. Existing account write quota applies per account
transactionally; acknowledged replay costs no second unit. No payment/GPS/document
or business portal requirement is introduced. See [review](../docs/backend/evidence/fuel-quality/FQ11_DOCUMENT_FREE_REVIEW.md)
and [historical validation](../docs/backend/evidence/fuel-quality/FQ08H_HISTORICAL_READS.md).

Critical quality persistence/read/job checks, from backend/ against disposable
local PostGIS (never the staging database):

```sh
ANPFUEL_TEST_QUALITY_PREPARER_PATH=/tmp/abastevo-integration-rust/debug/prepare_quality go test -race -tags=integration ./internal/modules/quality/... -count=1
go test ./internal/platform/apicontract -count=1
```

Explicit `-count=1` is required after changing external OpenAPI/vector inputs;
Go test cache does not track those files. Local source acceptance does not imply
live native imports, score calibration, mobile/device or production acceptance.

The quality prepared bridge additionally checks the real Rust CLI and atomic SQL
publication; the integration workflow builds both source preparers from the same
locked crate. Prepared input is integrity-checked before any read-only station
resolution, and public sample keys include full source/sample/CNPJ/date/fuel scope.
See [FQ-05C bridge evidence](../docs/backend/evidence/fuel-quality/FQ05C_PREPARED_BRIDGE.md).
This checkpoint does not enable native discovery/import in the deployed worker.


The native PMQC archive adapter (FQ-05W2) is file-backed and source-bound. It
checks bounded ZIP metadata before allocating entries, preserves each monthly
CSV as a separate edition, and runs only the configured trusted preparer in a
private workspace. Unsupported source contracts fail before Directory resolution.
Native worker composition/activation is still pending; this adapter alone does
not import anything into a deployed database.

For explicit offline source evidence, acquire the official archives separately
into a private directory and run from backend/ (paths are local, never committed):

```sh
ANPFUEL_TEST_QUALITY_PREPARER_PATH=/tmp/abastevo-integration-rust/debug/prepare_quality \
ANPFUEL_TEST_QUALITY_SEPTEMBER_ZIP=/private/pmqc_2026_09.zip \
ANPFUEL_TEST_QUALITY_ANNUAL_ZIP=/private/pmqc_2025.zip \
go test -count=1 -v -tags=integration,qualitysource ./internal/modules/quality/adapters/native -run TestOfficialArchivePreparationManual
```

The manual source test pins exact acquisition hashes, logs counts only and
publishes nothing. January 2025 source clocks are preserved by [FQ-02C](../docs/backend/evidence/fuel-quality/FQ02C_SOURCE_CLOCKS.md), with zero date-format quarantine and unchanged September manifest. Native Directory/fetch/worker composition is still required before historical activation. See [FQ-05W2](../docs/backend/evidence/fuel-quality/FQ05W2_ARCHIVE_PROCESS.md).

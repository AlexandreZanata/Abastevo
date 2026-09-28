# Backend-first implementation roadmap

Planning baseline: 2026-09-28. **Implementation has not started.** Upstream Android has been imported unchanged. P00 audit/documentation is delivered; Android runtime baseline remains blocked by the environment described in [BASELINE_VALIDATION](docs/planning/BASELINE_VALIDATION.md).

## How to execute

Start with **P01-T01**. Read [AGENTS](AGENTS.md), [current progress](docs/planning/PROGRESS.md), the selected task and its inputs. All paths below are repository-relative; shorthand document titles resolve through [the documentation index](docs/README.md). Planned commands/paths are created by their prerequisite tasks, not available today. A package test path is adjusted to the actual narrowly owned package established by that task and recorded in its README; never skip a check because its template path does not yet exist.

Every task below is NOT STARTED. MUST belongs to the bounded MVP; SHOULD describes post-pilot improvements in PRODUCT_CONTRACT; LATER requires a separately validated product scope. A task should fit one focused session; if it cannot, split it into letter-suffixed IDs with the same acceptance dependencies before coding. No parallel agent delegation is implied.

**Hard dependency:** P10 (any Android improvements, including CameraX/OCR) starts only after G09 COMPLETE. P01–P09 build/test the backend with protocol clients and fixtures. Existing Android builds/tests may run earlier. Payments/account/sync are outside backend-MVP completeness, so they cannot hold the release gate open indefinitely.

DOD-1 is defined in [AI_ENGINEERING_CONTRACT](docs/AI_ENGINEERING_CONTRACT.md): documented rule, meaningful tests, appropriate static/compile/contract/security/migration checks, reviewed scoped diff, recovery, actual evidence and updated progress. Every task inherits DOD-1. Migration tasks append SQL and test empty/previous schemas. Phase gates are evidence requirements, not completed checkboxes.

## Phase order

P00 import/audit/planning → P01 foundations → P02 official catalog → P03 identity/jobs → P04 observations → P05 evidence → P06 trust/consensus → P07 moderation/privacy → P08 infrastructure/hardening → P09 backend release gate → P10 Android → P11 optional commercial services.

Elapsed dates and total cost are deliberately not promised before prerequisite, parser/geocoder and load evidence. Sequence by dependencies, not invented calendar deadlines.

## P01 — Backend foundations

Priority: **MUST**. Entry: No earlier implementation gate; planning baseline exists..

Exit gate: **G01: documented prerequisite report; Go build/unit/static checks; typed config; health/lifecycle; real local PostGIS; migrations from empty; image build and CI; base OpenAPI lint. Android baseline blockers are recorded and resolved before G09.**

<a id="p01-t01"></a>

### P01-T01 — Prerequisites and inherited baseline

- **ID / priority / status:** P01-T01 / MUST / NOT STARTED.
- **Goal:** Reproduce the imported Android baseline and pin a supported backend toolchain plan.
- **Why:** Avoid building on an unverified environment.
- **Inputs:** docs/backend/TARGET_ARCHITECTURE.md, docs/backend/TEST_STRATEGY.md; docs/planning/BASELINE_VALIDATION.md; docs/tech-stack.md.
- **Files/areas expected:** `docs/planning/BASELINE_VALIDATION.md; docs/backend/TOOLCHAIN.md; scoped legacy documentation` (area list, not a literal combined path).
- **Dependencies:** No earlier implementation gate; planning baseline exists.; all earlier phase gates.
- **Tests first:** Verify JDK 17/SDK 35 availability and current Gradle command failures before remediation.
- **Implementation outline:** Provision execution prerequisites without changing Android versions; verify Go/Docker/Compose; record version/license/support choices and A01/A02/A11 documentation corrections.
- **Acceptance criteria:** Baseline commands have exact outcomes; no false green; Go/PostGIS/tool choices recorded.
- **Validation commands:** `java -version; go version; docker version; docker compose version; ./gradlew :domain:test :application:test :data:testDebugUnitTest :app:testDebugUnitTest :app:assembleDebug --no-daemon`. Where prose names a gate, introduce/document the exact executable command in this task before claiming completion.
- **Risks:** Missing workstation SDK or daemon access is an environment blocker.
- **Rollback/Recovery:** Revert documentation/tool setup in its own scope; do not rewrite Gradle/source to bypass failures.
- **Definition of done:** DOD-1 plus this task's acceptance/validation evidence; update status/progress without claiming the next phase is complete.

<a id="p01-t02"></a>

### P01-T02 — Go module and process roots

- **ID / priority / status:** P01-T02 / MUST / NOT STARTED.
- **Goal:** Create minimal compilable API/worker/migrator entry points.
- **Why:** Establish one codebase and build boundary.
- **Inputs:** docs/backend/TARGET_ARCHITECTURE.md, docs/backend/TEST_STRATEGY.md; backend/README.md; ADR-005.
- **Files/areas expected:** `backend/go.mod; backend/go.sum when needed; backend/cmd/*` (area list, not a literal combined path).
- **Dependencies:** P01-T01; all earlier phase gates.
- **Tests first:** Build empty entry points and verify no Android dependency.
- **Implementation outline:** Choose module path for new repository, supported pinned Go version and minimal roots without business code.
- **Acceptance criteria:** Three commands compile; no speculative business packages.
- **Validation commands:** `cd backend && go test ./... && go build ./cmd/...`. Where prose names a gate, introduce/document the exact executable command in this task before claiming completion.
- **Risks:** Premature frameworks and dependency sprawl.
- **Rollback/Recovery:** Revert unshipped module files.
- **Definition of done:** DOD-1 plus this task's acceptance/validation evidence; update status/progress without claiming the next phase is complete.

<a id="p01-t03"></a>

### P01-T03 — Typed startup configuration

- **ID / priority / status:** P01-T03 / MUST / NOT STARTED.
- **Goal:** Validate configuration once at process startup.
- **Why:** Prevent half-configured production processes.
- **Inputs:** docs/backend/TARGET_ARCHITECTURE.md, docs/backend/TEST_STRATEGY.md; INFRASTRUCTURE_PLAN; SECURITY_PRIVACY.
- **Files/areas expected:** `backend/internal/platform/config; backend/.env.example; narrow .gitignore exception` (area list, not a literal combined path).
- **Dependencies:** P01-T02; all earlier phase gates.
- **Tests first:** Missing/invalid environment, DSN and limit values fail safely; secrets never print.
- **Implementation outline:** Implement typed parsing/defaults/environment modes; inject config into roots.
- **Acceptance criteria:** Fail-fast errors are redacted; sample contains no secrets.
- **Validation commands:** `cd backend && go test ./internal/platform/config/...`. Where prose names a gate, introduce/document the exact executable command in this task before claiming completion.
- **Risks:** Overpermissive defaults accidentally enable production mode.
- **Rollback/Recovery:** Revert config changes; keep old startup path until tests pass.
- **Definition of done:** DOD-1 plus this task's acceptance/validation evidence; update status/progress without claiming the next phase is complete.

<a id="p01-t04"></a>

### P01-T04 — Structured logging and redaction

- **ID / priority / status:** P01-T04 / MUST / NOT STARTED.
- **Goal:** Add slog factory and request/job context.
- **Why:** Make failures diagnosable without personal payloads.
- **Inputs:** docs/backend/TARGET_ARCHITECTURE.md, docs/backend/TEST_STRATEGY.md; SECURITY_PRIVACY; B-BR-011.
- **Files/areas expected:** `backend/internal/platform/telemetry` (area list, not a literal combined path).
- **Dependencies:** P01-T03; all earlier phase gates.
- **Tests first:** Assert body, GPS, tokens and URLs absent from representative logs.
- **Implementation outline:** Expose bounded fields/redaction helpers and injected sinks.
- **Acceptance criteria:** JSON records include request ID, role and stable code only.
- **Validation commands:** `cd backend && go test ./internal/platform/telemetry/...`. Where prose names a gate, introduce/document the exact executable command in this task before claiming completion.
- **Risks:** Secret leakage or high-cardinality fields.
- **Rollback/Recovery:** Revert logger adapter and rotate any accidentally exposed real secret if incident occurs.
- **Definition of done:** DOD-1 plus this task's acceptance/validation evidence; update status/progress without claiming the next phase is complete.

<a id="p01-t05"></a>

### P01-T05 — HTTP lifecycle

- **ID / priority / status:** P01-T05 / MUST / NOT STARTED.
- **Goal:** Implement bounded HTTP server and graceful shutdown.
- **Why:** Avoid leaked requests/connections during deploy.
- **Inputs:** docs/backend/TARGET_ARCHITECTURE.md, docs/backend/TEST_STRATEGY.md; TARGET_ARCHITECTURE; API_PLAN.
- **Files/areas expected:** `backend/internal/platform/httpserver; backend/cmd/api` (area list, not a literal combined path).
- **Dependencies:** P01-T04; all earlier phase gates.
- **Tests first:** httptest shutdown/cancellation and oversized/slow request cases.
- **Implementation outline:** Wire chi, timeout/body-limit middleware and signal-aware drain.
- **Acceptance criteria:** Server starts/stops predictably; no business rules in middleware.
- **Validation commands:** `cd backend && go test ./internal/platform/httpserver/...`. Where prose names a gate, introduce/document the exact executable command in this task before claiming completion.
- **Risks:** Timeouts terminating accepted work incorrectly.
- **Rollback/Recovery:** Rollback binary; ensure accepted jobs stay durable.
- **Definition of done:** DOD-1 plus this task's acceptance/validation evidence; update status/progress without claiming the next phase is complete.

<a id="p01-t06"></a>

### P01-T06 — Liveness and readiness

- **ID / priority / status:** P01-T06 / MUST / NOT STARTED.
- **Goal:** Separate process health from dependency readiness.
- **Why:** Support safe routing without restart loops.
- **Inputs:** docs/backend/TARGET_ARCHITECTURE.md, docs/backend/TEST_STRATEGY.md; INFRASTRUCTURE_PLAN.
- **Files/areas expected:** `backend/internal/platform/health` (area list, not a literal combined path).
- **Dependencies:** P01-T05; all earlier phase gates.
- **Tests first:** Healthy process with failed DB is live but not ready; no config disclosure.
- **Implementation outline:** Inject readiness checks; expose minimal status.
- **Acceptance criteria:** Health semantics match plan under dependency failure.
- **Validation commands:** `cd backend && go test ./internal/platform/health/...`. Where prose names a gate, introduce/document the exact executable command in this task before claiming completion.
- **Risks:** DB outage causing cascading restarts.
- **Rollback/Recovery:** Revert routing/check config and restore previous health handler.
- **Definition of done:** DOD-1 plus this task's acceptance/validation evidence; update status/progress without claiming the next phase is complete.

<a id="p01-t07"></a>

### P01-T07 — Local PostgreSQL/PostGIS service

- **ID / priority / status:** P01-T07 / MUST / NOT STARTED.
- **Goal:** Add disposable Compose database with persistent named local volume.
- **Why:** Enable real spatial integration tests.
- **Inputs:** docs/backend/TARGET_ARCHITECTURE.md, docs/backend/TEST_STRATEGY.md; DATA_MODEL; INFRASTRUCTURE_PLAN.
- **Files/areas expected:** `infra/compose.dev.yml; infra/docker; infra/README.md` (area list, not a literal combined path).
- **Dependencies:** P01-T06; all earlier phase gates.
- **Tests first:** Compose config validity, loopback binding and readiness probe.
- **Implementation outline:** Pin compatible supported image/digest, dev-only credentials and volume names.
- **Acceptance criteria:** Local DB boots; ports are not public; no production data involved.
- **Validation commands:** `docker compose -f infra/compose.dev.yml config; docker compose -f infra/compose.dev.yml up -d db`. Where prose names a gate, introduce/document the exact executable command in this task before claiming completion.
- **Risks:** Confusing disposable and persistent production volumes.
- **Rollback/Recovery:** Stop only project local services; never remove production volumes.
- **Definition of done:** DOD-1 plus this task's acceptance/validation evidence; update status/progress without claiming the next phase is complete.

<a id="p01-t08"></a>

### P01-T08 — Migration runner and PostGIS bootstrap

- **ID / priority / status:** P01-T08 / MUST / NOT STARTED.
- **Goal:** Apply ordered SQL with lock/checksum ledger.
- **Why:** Establish the only schema-change mechanism.
- **Inputs:** docs/backend/TARGET_ARCHITECTURE.md, docs/backend/TEST_STRATEGY.md; DATA_MODEL.
- **Files/areas expected:** `backend/cmd/migrate; backend/internal/platform/migrate; backend/db/migrations` (area list, not a literal combined path).
- **Dependencies:** P01-T07; all earlier phase gates.
- **Tests first:** Empty DB, second apply, checksum mutation and concurrent runner cases.
- **Implementation outline:** Choose minimal maintained migration mechanism or explicit runner by dependency record; enable PostGIS under deployment role.
- **Acceptance criteria:** PostGIS version query works; apply idempotent; checksum mismatch fails.
- **Validation commands:** `cd backend && go test -tags=integration ./internal/platform/migrate/...`. Where prose names a gate, introduce/document the exact executable command in this task before claiming completion.
- **Risks:** DDL locks/role privilege mistakes.
- **Rollback/Recovery:** For initial disposable DB recreate only local volume; future fixes append migrations.
- **Definition of done:** DOD-1 plus this task's acceptance/validation evidence; update status/progress without claiming the next phase is complete.

<a id="p01-t09"></a>

### P01-T09 — Database pool and SQL tooling

- **ID / priority / status:** P01-T09 / MUST / NOT STARTED.
- **Goal:** Set pgx pool limits and sqlc ownership structure.
- **Why:** Keep SQL visible and connections bounded.
- **Inputs:** docs/backend/TARGET_ARCHITECTURE.md, docs/backend/TEST_STRATEGY.md; DATA_MODEL; TARGET_ARCHITECTURE.
- **Files/areas expected:** `backend/internal/platform/database; backend/sqlc.yaml; backend/db/queries` (area list, not a literal combined path).
- **Dependencies:** P01-T08; all earlier phase gates.
- **Tests first:** Connection timeout/cancellation and generated query smoke with real DB.
- **Implementation outline:** Wire injectable transaction/pool ownership; generate minimal typed health query.
- **Acceptance criteria:** Pool closed on shutdown; SQL generation reproducible.
- **Validation commands:** `cd backend && go test -tags=integration ./internal/platform/database/... && sqlc vet && sqlc generate`. Where prose names a gate, introduce/document the exact executable command in this task before claiming completion.
- **Risks:** Connection starvation and cross-module SQL leakage.
- **Rollback/Recovery:** Rollback adapter config; preserve schema and accepted data.
- **Definition of done:** DOD-1 plus this task's acceptance/validation evidence; update status/progress without claiming the next phase is complete.

<a id="p01-t10"></a>

### P01-T10 — API and worker images

- **ID / priority / status:** P01-T10 / MUST / NOT STARTED.
- **Goal:** Build reproducible non-root process images.
- **Why:** Make later deploys reviewable and recoverable.
- **Inputs:** docs/backend/TARGET_ARCHITECTURE.md, docs/backend/TEST_STRATEGY.md; INFRASTRUCTURE_PLAN.
- **Files/areas expected:** `infra/docker; backend/.dockerignore; infra/README.md` (area list, not a literal combined path).
- **Dependencies:** P01-T09; all earlier phase gates.
- **Tests first:** Build/run smoke with injected fake/dev config; inspect image for secrets.
- **Implementation outline:** Use multi-stage pinned builders; same release provenance for roles.
- **Acceptance criteria:** Images boot and terminate; no source secrets or build cache shipped.
- **Validation commands:** `docker build -f infra/docker/Dockerfile --target api .`. Where prose names a gate, introduce/document the exact executable command in this task before claiming completion.
- **Risks:** Leaking credentials into layers or mismatched role versions.
- **Rollback/Recovery:** Discard unpromoted images and return to previous digest.
- **Definition of done:** DOD-1 plus this task's acceptance/validation evidence; update status/progress without claiming the next phase is complete.

<a id="p01-t11"></a>

### P01-T11 — Base OpenAPI and golden vocabulary

- **ID / priority / status:** P01-T11 / MUST / NOT STARTED.
- **Goal:** Encode base schemas/errors/security and contract checks.
- **Why:** Freeze money/units/enums before handlers expand.
- **Inputs:** docs/backend/TARGET_ARCHITECTURE.md, docs/backend/TEST_STRATEGY.md; API_PLAN; DOMAIN_MODEL.
- **Files/areas expected:** `contracts/openapi/v1.yaml; contracts/testdata/api; contract validator config` (area list, not a literal combined path).
- **Dependencies:** P01-T10; all earlier phase gates.
- **Tests first:** Valid/invalid amount, unit, enum and error envelope vectors.
- **Implementation outline:** Add common schemas, health/public API skeleton and auth profile references; pin linter.
- **Acceptance criteria:** Contract parses/lints; examples validate; future endpoints added before code.
- **Validation commands:** `Run the pinned OpenAPI lint/example commands documented in contracts/README.md`. Where prose names a gate, introduce/document the exact executable command in this task before claiming completion.
- **Risks:** Contract without semantics or silently changed legacy names.
- **Rollback/Recovery:** Revert unpublished schema change; never overwrite a published contract.
- **Definition of done:** DOD-1 plus this task's acceptance/validation evidence; update status/progress without claiming the next phase is complete.

<a id="p01-t12"></a>

### P01-T12 — CI fast and full gate wiring

- **ID / priority / status:** P01-T12 / MUST / NOT STARTED.
- **Goal:** Create path-aware backend checks and improve secret coverage.
- **Why:** Keep feedback fast without weakening release checks.
- **Inputs:** docs/backend/TARGET_ARCHITECTURE.md, docs/backend/TEST_STRATEGY.md; TEST_STRATEGY; AI_ENGINEERING_CONTRACT.
- **Files/areas expected:** `.github/workflows; backend/README.md; scripts/check-*` (area list, not a literal combined path).
- **Dependencies:** P01-T11; all earlier phase gates.
- **Tests first:** Deliberately malformed formatting/schema/secret fixture makes isolated gate fail.
- **Implementation outline:** Pin actions/tools; unit/static/security fast job; PostGIS integration job; retain Android CI.
- **Acceptance criteria:** CI checks cannot silently skip missing DB; docs edits avoid full Android rebuild.
- **Validation commands:** `Run the documented fast gate locally; inspect a CI run when branch is published`. Where prose names a gate, introduce/document the exact executable command in this task before claiming completion.
- **Risks:** CI secrets on fork PRs; false-green skipped integration.
- **Rollback/Recovery:** Revert workflow only; retain proven Android workflow.
- **Definition of done:** DOD-1 plus this task's acceptance/validation evidence; update status/progress without claiming the next phase is complete.

## P02 — Official catalog and ANP ingestion

Priority: **MUST**. Entry: G01.

Exit gate: **G02: canonical station identity, precise units/CNPJ, idempotent revisioned import, quarantine/reporting, permitted geolocation path and public official reads validated; no Android modifications.**

<a id="p02-t01"></a>

### P02-T01 — Shared ANP fixtures

- **ID / priority / status:** P02-T01 / MUST / NOT STARTED.
- **Goal:** Capture versioned source normalization cases.
- **Why:** Prevent parser divergence.
- **Inputs:** docs/backend/ANP_INGESTION.md, docs/backend/DATA_MODEL.md; CURRENT_STATE_AUDIT A03-A07.
- **Files/areas expected:** `contracts/testdata/anp; backend/testdata` (area list, not a literal combined path).
- **Dependencies:** G01; all earlier phase gates.
- **Tests first:** Verify sample hashes and expected legacy-versus-target outputs.
- **Implementation outline:** Create manifest and cases including labels, precision, unit, header shifts and alphanumeric CNPJ.
- **Acceptance criteria:** Each case has provenance and intentional compatibility classification.
- **Validation commands:** `Validate fixture JSON/schema and Go fixture-loader tests`. Where prose names a gate, introduce/document the exact executable command in this task before claiming completion.
- **Risks:** Copying production personal data or encoding a wrong assumption.
- **Rollback/Recovery:** Correct an unpublished fixture with rationale; version already-consumed cases.
- **Definition of done:** DOD-1 plus this task's acceptance/validation evidence; update status/progress without claiming the next phase is complete.

<a id="p02-t02"></a>

### P02-T02 — Price, product, condition and CNPJ values

- **ID / priority / status:** P02-T02 / MUST / NOT STARTED.
- **Goal:** Implement exact validated backend value objects.
- **Why:** Centralize stable import/community invariants.
- **Inputs:** docs/backend/ANP_INGESTION.md, docs/backend/DATA_MODEL.md; DOMAIN_MODEL B-BR-002; shared fixtures.
- **Files/areas expected:** `backend/internal/modules/directory/domain; backend/internal/modules/community/domain; minimal shared values` (area list, not a literal combined path).
- **Dependencies:** P02-T01; all earlier phase gates.
- **Tests first:** GIVEN valid/invalid numeric/alphanumeric CNPJ, units and integer price THEN expected value/error.
- **Implementation outline:** Implement separate small values; no I/O or float money; split into separate PRs if needed.
- **Acceptance criteria:** All edge fixtures pass; source letters/leading zeroes preserved.
- **Validation commands:** `cd backend && go test ./internal/modules/...`. Where prose names a gate, introduce/document the exact executable command in this task before claiming completion.
- **Risks:** Overbroad shared kernel or semantic drift.
- **Rollback/Recovery:** Revert value implementation before schema depends on it.
- **Definition of done:** DOD-1 plus this task's acceptance/validation evidence; update status/progress without claiming the next phase is complete.

<a id="p02-t03"></a>

### P02-T03 — Canonical station repository

- **ID / priority / status:** P02-T03 / MUST / NOT STARTED.
- **Goal:** Persist station identifiers and location revisions.
- **Why:** Avoid duplicate station identity across sources.
- **Inputs:** docs/backend/ANP_INGESTION.md, docs/backend/DATA_MODEL.md; DATA_MODEL Directory; B-BR-014.
- **Files/areas expected:** `backend/internal/modules/directory; backend/db/migrations; backend/db/queries/directory` (area list, not a literal combined path).
- **Dependencies:** P02-T02; all earlier phase gates.
- **Tests first:** Concurrent same CNPJ produces one station; aliases and missing location cases.
- **Implementation outline:** Add owned schema/repository and explicit resolver interface.
- **Acceptance criteria:** Stable UUID, constraints and indexes pass real DB tests.
- **Validation commands:** `cd backend && go test -tags=integration ./internal/modules/directory/...`. Where prose names a gate, introduce/document the exact executable command in this task before claiming completion.
- **Risks:** Merging unrelated businesses by address.
- **Rollback/Recovery:** Append correction/alias review; never delete linked observation history.
- **Definition of done:** DOD-1 plus this task's acceptance/validation evidence; update status/progress without claiming the next phase is complete.

<a id="p02-t04"></a>

### P02-T04 — Bounded ANP parser

- **ID / priority / status:** P02-T04 / MUST / NOT STARTED.
- **Goal:** Parse summary/station workbook streams safely.
- **Why:** Convert actual source files with explainable rejection.
- **Inputs:** docs/backend/ANP_INGESTION.md, docs/backend/DATA_MODEL.md; ANP_INGESTION; D06.
- **Files/areas expected:** `backend/internal/modules/official/adapters/anp` (area list, not a literal combined path).
- **Dependencies:** P02-T03; all earlier phase gates.
- **Tests first:** Header shift, unknown label, exact decimal, malformed XML/ZIP and size-limit failures.
- **Implementation outline:** Resolve parser library decision then implement discovery-independent parsing.
- **Acceptance criteria:** Fixtures pass; memory/input caps measured; no formula execution.
- **Validation commands:** `cd backend && go test ./internal/modules/official/adapters/anp/...`. Where prose names a gate, introduce/document the exact executable command in this task before claiming completion.
- **Risks:** Layout drift or decompression resource exhaustion.
- **Rollback/Recovery:** Keep last published revision; quarantine inputs and roll back parser.
- **Definition of done:** DOD-1 plus this task's acceptance/validation evidence; update status/progress without claiming the next phase is complete.

<a id="p02-t05"></a>

### P02-T05 — ANP source discovery and download

- **ID / priority / status:** P02-T05 / MUST / NOT STARTED.
- **Goal:** Download only approved official sources with checksum.
- **Why:** Avoid SSRF and unverifiable provenance.
- **Inputs:** docs/backend/ANP_INGESTION.md, docs/backend/DATA_MODEL.md; ANP_INGESTION.
- **Files/areas expected:** `backend/internal/modules/official/adapters/source` (area list, not a literal combined path).
- **Dependencies:** P02-T04; all earlier phase gates.
- **Tests first:** Redirect to disallowed/private host rejected; checksum/timeout/cancel tests.
- **Implementation outline:** Allowlist hosts/paths, conditional fetch, limits and temporary-file cleanup.
- **Acceptance criteria:** No arbitrary URL fetch; fixtures cover discovery changes.
- **Validation commands:** `cd backend && go test ./internal/modules/official/adapters/source/...`. Where prose names a gate, introduce/document the exact executable command in this task before claiming completion.
- **Risks:** Redirect bypass or stale catalog.
- **Rollback/Recovery:** Disable import scheduling and retain last known source revision.
- **Definition of done:** DOD-1 plus this task's acceptance/validation evidence; update status/progress without claiming the next phase is complete.

<a id="p02-t06"></a>

### P02-T06 — Revisioned publication

- **ID / priority / status:** P02-T06 / MUST / NOT STARTED.
- **Goal:** Stage and publish imports atomically.
- **Why:** Correct ANP revisions without overwriting history.
- **Inputs:** docs/backend/ANP_INGESTION.md, docs/backend/DATA_MODEL.md; B-BR-013; DATA_MODEL Official.
- **Files/areas expected:** `backend/internal/modules/official; backend/db/migrations; backend/db/queries/official` (area list, not a literal combined path).
- **Dependencies:** P02-T05; all earlier phase gates.
- **Tests first:** Same bytes no-op, corrected bytes new revision, partial batch invisible, quarantine threshold.
- **Implementation outline:** Persist runs/rows/revisions; validate then switch publication pointer.
- **Acceptance criteria:** Previous revision stays readable and retry is safe.
- **Validation commands:** `cd backend && go test -tags=integration ./internal/modules/official/...`. Where prose names a gate, introduce/document the exact executable command in this task before claiming completion.
- **Risks:** Publishing partial/empty source or duplicate revisions.
- **Rollback/Recovery:** Reset active pointer via audited application action; preserve both revisions.
- **Definition of done:** DOD-1 plus this task's acceptance/validation evidence; update status/progress without claiming the next phase is complete.

<a id="p02-t07"></a>

### P02-T07 — Catalog geolocation adapter

- **ID / priority / status:** P02-T07 / MUST / NOT STARTED.
- **Goal:** Store verified station coordinates with provenance.
- **Why:** Enable nearby queries without live geocoder calls.
- **Inputs:** docs/backend/ANP_INGESTION.md, docs/backend/DATA_MODEL.md; ANP_INGESTION; D05.
- **Files/areas expected:** `backend/internal/modules/directory/adapters/geocoder` (area list, not a literal combined path).
- **Dependencies:** P02-T06; all earlier phase gates.
- **Tests first:** Cache/quota/timeout/ambiguous result and missing-coordinate fixtures.
- **Implementation outline:** Select permitted provider for pilot, add global throttle/cache and reviewed location changes.
- **Acceptance criteria:** Unknown/centroid not treated as verified station; attribution recorded.
- **Validation commands:** `cd backend && go test -tags=integration ./internal/modules/directory/...`. Where prose names a gate, introduce/document the exact executable command in this task before claiming completion.
- **Risks:** Provider policy breach or false coordinates.
- **Rollback/Recovery:** Disable provider and retain last reviewed location revision.
- **Definition of done:** DOD-1 plus this task's acceptance/validation evidence; update status/progress without claiming the next phase is complete.

<a id="p02-t08"></a>

### P02-T08 — Station and official price read API

- **ID / priority / status:** P02-T08 / MUST / NOT STARTED.
- **Goal:** Expose source-labelled directory and official history.
- **Why:** Make the catalog usable before community write path.
- **Inputs:** docs/backend/ANP_INGESTION.md, docs/backend/DATA_MODEL.md; API_PLAN public reads.
- **Files/areas expected:** `contracts/openapi; backend/internal/modules/directory; backend/internal/modules/official` (area list, not a literal combined path).
- **Dependencies:** P02-T07; all earlier phase gates.
- **Tests first:** Contract/error/pagination/geo bounds and query plan tests.
- **Implementation outline:** Implement owned read adapters and explicit cache/no-store policies.
- **Acceptance criteria:** Station/nearby/history responses validate; missing location honest.
- **Validation commands:** `cd backend && go test -tags=integration ./internal/modules/directory/... ./internal/modules/official/...`. Where prose names a gate, introduce/document the exact executable command in this task before claiming completion.
- **Risks:** Precise query coordinates in logs/cache or expensive spatial scans.
- **Rollback/Recovery:** Rollback handler version; keep data and previous contract semantics.
- **Definition of done:** DOD-1 plus this task's acceptance/validation evidence; update status/progress without claiming the next phase is complete.

## P03 — Anonymous identity and durable work

Priority: **MUST**. Entry: G02.

Exit gate: **G03: verified registration/rotation/signatures, atomic replay defense, durable idempotency/quotas and lease-based queue, with concurrency and crash tests.**

<a id="p03-t01"></a>

### P03-T01 — Authentication byte-level vectors

- **ID / priority / status:** P03-T01 / MUST / NOT STARTED.
- **Goal:** Freeze interoperable signature profile.
- **Why:** Avoid unsafe custom serialization.
- **Inputs:** docs/backend/API_PLAN.md, docs/security/SECURITY_PRIVACY.md; API_PLAN; ADR-007.
- **Files/areas expected:** `contracts/testdata/identity; docs/security/identity-profile.md` (area list, not a literal combined path).
- **Dependencies:** G02; all earlier phase gates.
- **Tests first:** RFC vector verification and changed method/path/body/authority rejection.
- **Implementation outline:** Specify covered components, P-256 encoding, challenge binding and trusted proxy behavior.
- **Acceptance criteria:** Independent verifier agrees; no ambiguous header acceptance.
- **Validation commands:** `Run documented Go plus independent standard-library protocol-vector tests`. Where prose names a gate, introduce/document the exact executable command in this task before claiming completion.
- **Risks:** Signature format/proxy mismatch.
- **Rollback/Recovery:** Revise unpublished profile before endpoint implementation.
- **Definition of done:** DOD-1 plus this task's acceptance/validation evidence; update status/progress without claiming the next phase is complete.

<a id="p03-t02"></a>

### P03-T02 — Contributor registration

- **ID / priority / status:** P03-T02 / MUST / NOT STARTED.
- **Goal:** Register key ownership without personal-account fields.
- **Why:** Allow free contribution without email.
- **Inputs:** docs/backend/API_PLAN.md, docs/security/SECURITY_PRIVACY.md; BUC-002; B-BR-004.
- **Files/areas expected:** `backend/internal/modules/identity; backend/db/migrations; contracts/openapi` (area list, not a literal combined path).
- **Dependencies:** P03-T01; all earlier phase gates.
- **Tests first:** Bad key, proof failure, duplicate fingerprint and registration retry cases.
- **Implementation outline:** Persist candidate challenge and key proof atomically; public JWK only.
- **Acceptance criteria:** Same key cannot acquire another contributor; proof required.
- **Validation commands:** `cd backend && go test -tags=integration ./internal/modules/identity/...`. Where prose names a gate, introduce/document the exact executable command in this task before claiming completion.
- **Risks:** Unauthenticated registration flood.
- **Rollback/Recovery:** Disable registration temporarily; preserve existing valid identities.
- **Definition of done:** DOD-1 plus this task's acceptance/validation evidence; update status/progress without claiming the next phase is complete.

<a id="p03-t03"></a>

### P03-T03 — Signed requests and replay defense

- **ID / priority / status:** P03-T03 / MUST / NOT STARTED.
- **Goal:** Verify required proof and consume nonce atomically.
- **Why:** Protect all owner reads/writes.
- **Inputs:** docs/backend/API_PLAN.md, docs/security/SECURITY_PRIVACY.md; identity-profile; B-BR-004.
- **Files/areas expected:** `backend/internal/modules/identity; HTTP auth adapter` (area list, not a literal combined path).
- **Dependencies:** P03-T02; all earlier phase gates.
- **Tests first:** Concurrent same nonce accepts once; skew, expiry, revoked key, signed-read replay.
- **Implementation outline:** Use DB challenge state and strict request profile; no RAM-only authority.
- **Acceptance criteria:** Changed components rejected; original contributor derived server-side.
- **Validation commands:** `cd backend && go test -race -tags=integration ./internal/modules/identity/...`. Where prose names a gate, introduce/document the exact executable command in this task before claiming completion.
- **Risks:** Replay window or transaction race.
- **Rollback/Recovery:** Pause writes; roll back binary and expire pending challenges.
- **Definition of done:** DOD-1 plus this task's acceptance/validation evidence; update status/progress without claiming the next phase is complete.

<a id="p03-t04"></a>

### P03-T04 — Operation idempotency

- **ID / priority / status:** P03-T04 / MUST / NOT STARTED.
- **Goal:** Persist request outcome and permanent command identity.
- **Why:** Make mobile retries safe.
- **Inputs:** docs/backend/API_PLAN.md, docs/security/SECURITY_PRIVACY.md; B-BR-005; API_PLAN.
- **Files/areas expected:** `backend/internal/modules/identity/application; persistence adapter` (area list, not a literal combined path).
- **Dependencies:** P03-T03; all earlier phase gates.
- **Tests first:** Same key/body returns original; changed body conflicts; lost-response retry.
- **Implementation outline:** Transaction-aware idempotency repository; safe stored responses; seven-day expiry.
- **Acceptance criteria:** No double execution; new nonce allowed for same command.
- **Validation commands:** `cd backend && go test -tags=integration ./internal/modules/identity/...`. Where prose names a gate, introduce/document the exact executable command in this task before claiming completion.
- **Risks:** Conflating replay and retry or retaining sensitive body.
- **Rollback/Recovery:** Keep durable keys; correct handler via forward fix.
- **Definition of done:** DOD-1 plus this task's acceptance/validation evidence; update status/progress without claiming the next phase is complete.

<a id="p03-t05"></a>

### P03-T05 — Registration and write quotas

- **ID / priority / status:** P03-T05 / MUST / NOT STARTED.
- **Goal:** Enforce shared contributor/IP operation limits.
- **Why:** Bound abuse/cost across API replicas.
- **Inputs:** docs/backend/API_PLAN.md, docs/security/SECURITY_PRIVACY.md; B-BR-015; SECURITY_PRIVACY.
- **Files/areas expected:** `backend/internal/modules/identity; edge config plan` (area list, not a literal combined path).
- **Dependencies:** P03-T04; all earlier phase gates.
- **Tests first:** Concurrent quota boundary, NAT throttling response and expiry cleanup.
- **Implementation outline:** Atomic DB counters, rotating IP digest, 429/Retry-After; edge additive limits.
- **Acceptance criteria:** No successful operations exceed configured transactional quota.
- **Validation commands:** `cd backend && go test -race -tags=integration ./internal/modules/identity/...`. Where prose names a gate, introduce/document the exact executable command in this task before claiming completion.
- **Risks:** IP-only bans or memory-only counters.
- **Rollback/Recovery:** Adjust versioned quota config with audit; preserve abuse evidence within retention.
- **Definition of done:** DOD-1 plus this task's acceptance/validation evidence; update status/progress without claiming the next phase is complete.

<a id="p03-t06"></a>

### P03-T06 — Key rotation and revocation

- **ID / priority / status:** P03-T06 / MUST / NOT STARTED.
- **Goal:** Rotate with old/new proofs and preserve contributor.
- **Why:** Avoid losing or stealing reputation.
- **Inputs:** docs/backend/API_PLAN.md, docs/security/SECURITY_PRIVACY.md; BUC-002; API_PLAN.
- **Files/areas expected:** `backend/internal/modules/identity; contracts/openapi` (area list, not a literal combined path).
- **Dependencies:** P03-T05; all earlier phase gates.
- **Tests first:** Old/new proof failure, concurrent rotation, old key revoked, new key works.
- **Implementation outline:** Atomic key replacement, no anonymous lost-key recovery promise.
- **Acceptance criteria:** Rotation keeps contributor and disallows old signed writes.
- **Validation commands:** `cd backend && go test -tags=integration ./internal/modules/identity/...`. Where prose names a gate, introduce/document the exact executable command in this task before claiming completion.
- **Risks:** Lockout or unauthorized reputation takeover.
- **Rollback/Recovery:** Restricted incident procedure; never restore compromised key automatically.
- **Definition of done:** DOD-1 plus this task's acceptance/validation evidence; update status/progress without claiming the next phase is complete.

<a id="p03-t07"></a>

### P03-T07 — PostgreSQL job queue

- **ID / priority / status:** P03-T07 / MUST / NOT STARTED.
- **Goal:** Claim durable work with leases and fencing.
- **Why:** Support validation/import/retention without broker.
- **Inputs:** docs/backend/API_PLAN.md, docs/security/SECURITY_PRIVACY.md; TARGET_ARCHITECTURE jobs.
- **Files/areas expected:** `backend/internal/platform/jobs; backend/db/migrations` (area list, not a literal combined path).
- **Dependencies:** P03-T06; all earlier phase gates.
- **Tests first:** Two workers, lease expiry, stale ack, crash after effect, poison job cases.
- **Implementation outline:** Transactional enqueue; SKIP LOCKED claim; capped retries; dead-job replay command.
- **Acceptance criteria:** One durable business effect per dedupe/version despite repeated delivery.
- **Validation commands:** `cd backend && go test -race -tags=integration ./internal/platform/jobs/...`. Where prose names a gate, introduce/document the exact executable command in this task before claiming completion.
- **Risks:** Stale worker overwrites or infinite retry.
- **Rollback/Recovery:** Pause job type, fix handler, replay DEAD jobs with audited reason.
- **Definition of done:** DOD-1 plus this task's acceptance/validation evidence; update status/progress without claiming the next phase is complete.

<a id="p03-t08"></a>

### P03-T08 — Worker dispatch and recurring schedules

- **ID / priority / status:** P03-T08 / MUST / NOT STARTED.
- **Goal:** Schedule official imports and connect durable handlers.
- **Why:** Turn the queue into operational background processing.
- **Inputs:** docs/backend/API_PLAN.md, docs/security/SECURITY_PRIVACY.md; ANP_INGESTION; TARGET_ARCHITECTURE jobs.
- **Files/areas expected:** `backend/cmd/worker; backend/internal/platform/jobs; official/directory application adapters` (area list, not a literal combined path).
- **Dependencies:** P03-T07; all earlier phase gates.
- **Tests first:** Restart duplicate scheduling, one due job per period, cancellation and slow provider cases.
- **Implementation outline:** Register explicit typed handlers; persist schedule dedupe keys; enqueue daily discovery and bounded permitted geocoding; later tasks add their own handlers.
- **Acceptance criteria:** Worker process executes imports after restart without duplicate publications; unsupported payload versions fail safely.
- **Validation commands:** `cd backend && go test -race -tags=integration ./internal/platform/jobs/... ./internal/modules/official/...`. Where prose names a gate, introduce/document the exact executable command in this task before claiming completion.
- **Risks:** In-memory-only schedules or duplicate live imports.
- **Rollback/Recovery:** Pause affected schedule; replay durable jobs with dedupe and preserve published revisions.
- **Definition of done:** DOD-1 plus this task's acceptance/validation evidence; update status/progress without claiming the next phase is complete.

## P04 — Observation history and lifecycle

Priority: **MUST**. Entry: G03.

Exit gate: **G04: durable immutable observations and decisions, ownership/idempotency, safe status API, validation orchestration; no current-price claim at receipt.**

<a id="p04-t01"></a>

### P04-T01 — Observation aggregate

- **ID / priority / status:** P04-T01 / MUST / NOT STARTED.
- **Goal:** Create an immutable observation under domain invariants.
- **Why:** Capture facts without overwriting current price.
- **Inputs:** docs/backend/DOMAIN_MODEL.md, docs/backend/COMMUNITY_PRICING_SPEC.md; B-BR-001…005; BUC-003.
- **Files/areas expected:** `backend/internal/modules/community/domain` (area list, not a literal combined path).
- **Dependencies:** G03; all earlier phase gates.
- **Tests first:** Invalid amount/unit/condition/supersedes and server-time invariants.
- **Implementation outline:** Implement aggregate construction and PriceObserved event; injected clock/IDs.
- **Acceptance criteria:** No client-controlled trust/contributor/server timestamp.
- **Validation commands:** `cd backend && go test ./internal/modules/community/domain/...`. Where prose names a gate, introduce/document the exact executable command in this task before claiming completion.
- **Risks:** Mutable fact fields or accidental precision loss.
- **Rollback/Recovery:** Revert unshipped aggregate; corrections later append new facts.
- **Definition of done:** DOD-1 plus this task's acceptance/validation evidence; update status/progress without claiming the next phase is complete.

<a id="p04-t02"></a>

### P04-T02 — Validation state machine

- **ID / priority / status:** P04-T02 / MUST / NOT STARTED.
- **Goal:** Control admissibility through domain transitions.
- **Why:** Keep state changes out of HTTP handlers.
- **Inputs:** docs/backend/DOMAIN_MODEL.md, docs/backend/COMMUNITY_PRICING_SPEC.md; DOMAIN_MODEL state machine.
- **Files/areas expected:** `backend/internal/modules/community/domain` (area list, not a literal combined path).
- **Dependencies:** P04-T01; all earlier phase gates.
- **Tests first:** Enumerate valid/invalid transitions, terminal rejection and moderation path.
- **Implementation outline:** Separate immutable decision events from state projection and job processing.
- **Acceptance criteria:** Every transition has actor/reason/event tests.
- **Validation commands:** `cd backend && go test ./internal/modules/community/domain/...`. Where prose names a gate, introduce/document the exact executable command in this task before claiming completion.
- **Risks:** Conflating expired/confidence/dispute with validation state.
- **Rollback/Recovery:** Append correcting decision under policy; never edit old decision.
- **Definition of done:** DOD-1 plus this task's acceptance/validation evidence; update status/progress without claiming the next phase is complete.

<a id="p04-t03"></a>

### P04-T03 — Observation persistence

- **ID / priority / status:** P04-T03 / MUST / NOT STARTED.
- **Goal:** Persist facts, decisions, idempotency and job atomically.
- **Why:** Prevent accepted but unprocessed writes.
- **Inputs:** docs/backend/DOMAIN_MODEL.md, docs/backend/COMMUNITY_PRICING_SPEC.md; DATA_MODEL Community.
- **Files/areas expected:** `backend/internal/modules/community/adapters; backend/db/migrations; queries/community` (area list, not a literal combined path).
- **Dependencies:** P04-T02; all earlier phase gates.
- **Tests first:** Rollback on enqueue failure, concurrent submission ID, disallowed destructive update.
- **Implementation outline:** Implement repository/transaction and indexes with least-privilege roles.
- **Acceptance criteria:** Retry yields same observation; history protected.
- **Validation commands:** `cd backend && go test -tags=integration ./internal/modules/community/...`. Where prose names a gate, introduce/document the exact executable command in this task before claiming completion.
- **Risks:** Partial commit or duplicate history.
- **Rollback/Recovery:** Forward migration fix; retain original facts and dedupe keys.
- **Definition of done:** DOD-1 plus this task's acceptance/validation evidence; update status/progress without claiming the next phase is complete.

<a id="p04-t04"></a>

### P04-T04 — Observation submit and owner status

- **ID / priority / status:** P04-T04 / MUST / NOT STARTED.
- **Goal:** Expose contracted write/status/history endpoints.
- **Why:** Give mobile retries a safe acknowledgment.
- **Inputs:** docs/backend/DOMAIN_MODEL.md, docs/backend/COMMUNITY_PRICING_SPEC.md; API_PLAN; BUC-003.
- **Files/areas expected:** `contracts/openapi; backend/internal/modules/community HTTP/application` (area list, not a literal combined path).
- **Dependencies:** P04-T03; all earlier phase gates.
- **Tests first:** Authz/IDOR, malformed field, no-store, 201 RECEIVED and retry contract.
- **Implementation outline:** Map DTO through use case to repository; owner-only status pagination.
- **Acceptance criteria:** OpenAPI responses match; no claim of publication before validation.
- **Validation commands:** `cd backend && go test -tags=integration ./internal/modules/community/...`. Where prose names a gate, introduce/document the exact executable command in this task before claiming completion.
- **Risks:** Publishing raw evidence/GPS or accepting client contributor ID.
- **Rollback/Recovery:** Rollback routes while retaining accepted jobs/history.
- **Definition of done:** DOD-1 plus this task's acceptance/validation evidence; update status/progress without claiming the next phase is complete.

<a id="p04-t05"></a>

### P04-T05 — Validation orchestration

- **ID / priority / status:** P04-T05 / MUST / NOT STARTED.
- **Goal:** Process received observations through owned ports.
- **Why:** Prepare honest eligibility before evidence/consensus.
- **Inputs:** docs/backend/DOMAIN_MODEL.md, docs/backend/COMMUNITY_PRICING_SPEC.md; BUC-003; B-BR-010/014.
- **Files/areas expected:** `backend/internal/modules/community/application; worker wiring` (area list, not a literal combined path).
- **Dependencies:** P04-T04; all earlier phase gates.
- **Tests first:** Missing station/evidence pending, unknown location, transient failure and permanent reject.
- **Implementation outline:** Use read-only station/trust/evidence ports; persist transition plus downstream intent.
- **Acceptance criteria:** Metadata-only path explicit; media-required path waits up to policy deadline.
- **Validation commands:** `cd backend && go test ./internal/modules/community/application/...`. Where prose names a gate, introduce/document the exact executable command in this task before claiming completion.
- **Risks:** Accidentally treating unavailable signals as verified.
- **Rollback/Recovery:** Pause validator, replay after fix; preserve received facts.
- **Definition of done:** DOD-1 plus this task's acceptance/validation evidence; update status/progress without claiming the next phase is complete.

## P05 — Private evidence

Priority: **MUST**. Entry: G04.

Exit gate: **G05: direct upload with bounded quotas, frozen server-validated evidence, ownership, retry/orphan/retention behavior proven with real storage staging.**

<a id="p05-t01"></a>

### P05-T01 — Upload session domain

- **ID / priority / status:** P05-T01 / MUST / NOT STARTED.
- **Goal:** Reserve owned upload sessions under limits.
- **Why:** Reject expensive invalid intent early.
- **Inputs:** docs/security/SECURITY_PRIVACY.md, docs/backend/API_PLAN.md; B-BR-010/015; BUC-004.
- **Files/areas expected:** `backend/internal/modules/evidence/domain; application` (area list, not a literal combined path).
- **Dependencies:** G04; all earlier phase gates.
- **Tests first:** Quota, MIME, size, ownership and expiry cases.
- **Implementation outline:** Implement session state and reservation; no client object key.
- **Acceptance criteria:** Only allowed intent reaches object-store adapter.
- **Validation commands:** `cd backend && go test ./internal/modules/evidence/...`. Where prose names a gate, introduce/document the exact executable command in this task before claiming completion.
- **Risks:** State permits quota bypass.
- **Rollback/Recovery:** Expire unprocessed sessions and release unused reservation safely.
- **Definition of done:** DOD-1 plus this task's acceptance/validation evidence; update status/progress without claiming the next phase is complete.

<a id="p05-t02"></a>

### P05-T02 — S3 adapter and presigned upload

- **ID / priority / status:** P05-T02 / MUST / NOT STARTED.
- **Goal:** Issue short direct PUT authorization.
- **Why:** Avoid proxying photos through API handlers.
- **Inputs:** docs/security/SECURITY_PRIVACY.md, docs/backend/API_PLAN.md; ADR-010; API_PLAN.
- **Files/areas expected:** `backend/internal/modules/evidence/adapters/storage; contracts/openapi` (area list, not a literal combined path).
- **Dependencies:** P05-T01; all earlier phase gates.
- **Tests first:** Private ACL, signed headers, expired URL, wrong owner/object key.
- **Implementation outline:** Minimal S3-compatible adapter; server keys; least-privilege credentials.
- **Acceptance criteria:** Client can upload only its quarantine object; bucket stays private.
- **Validation commands:** `Run storage integration suite with emulator then isolated R2 staging bucket`. Where prose names a gate, introduce/document the exact executable command in this task before claiming completion.
- **Risks:** Assuming PUT enforces content size or one-time use.
- **Rollback/Recovery:** Revoke storage credential if leaked; expire sessions; preserve validated final objects.
- **Definition of done:** DOD-1 plus this task's acceptance/validation evidence; update status/progress without claiming the next phase is complete.

<a id="p05-t03"></a>

### P05-T03 — Safe image verification

- **ID / priority / status:** P05-T03 / MUST / NOT STARTED.
- **Goal:** Validate a bounded snapshot and sanitize media.
- **Why:** Stop metadata spoofing and upload overwrite races.
- **Inputs:** docs/security/SECURITY_PRIVACY.md, docs/backend/API_PLAN.md; SECURITY_PRIVACY evidence protocol.
- **Files/areas expected:** `backend/internal/modules/evidence/application; media adapter; testdata` (area list, not a literal combined path).
- **Dependencies:** P05-T02; all earlier phase gates.
- **Tests first:** Oversize, malformed/JPEG bomb, EXIF, swapped object and polyglot fixtures.
- **Implementation outline:** Bounded download/decode; server hash; re-encode to immutable final key.
- **Acceptance criteria:** Only verified snapshot becomes READY; EXIF stripped.
- **Validation commands:** `cd backend && go test ./internal/modules/evidence/...`. Where prose names a gate, introduce/document the exact executable command in this task before claiming completion.
- **Risks:** Decoder resource exhaustion or TOCTOU bug.
- **Rollback/Recovery:** Pause validation, quarantine affected objects, forward fix and replay.
- **Definition of done:** DOD-1 plus this task's acceptance/validation evidence; update status/progress without claiming the next phase is complete.

<a id="p05-t04"></a>

### P05-T04 — Completion and observation binding

- **ID / priority / status:** P05-T04 / MUST / NOT STARTED.
- **Goal:** Finalize asynchronously and bind owner evidence once.
- **Why:** Handle mobile duplicate finalize and lost responses.
- **Inputs:** docs/security/SECURITY_PRIVACY.md, docs/backend/API_PLAN.md; BUC-004; API_PLAN.
- **Files/areas expected:** `backend/internal/modules/evidence; community evidence port; queries` (area list, not a literal combined path).
- **Dependencies:** P05-T03; all earlier phase gates.
- **Tests first:** Duplicate complete, upload success/DB failure, reused evidence, owner mismatch.
- **Implementation outline:** Persist intent/job; READY binding transaction; safe status endpoint.
- **Acceptance criteria:** No publicly downloadable evidence; no duplicate binding.
- **Validation commands:** `cd backend && go test -tags=integration ./internal/modules/evidence/...`. Where prose names a gate, introduce/document the exact executable command in this task before claiming completion.
- **Risks:** Reference to mutable/deleted object.
- **Rollback/Recovery:** Reconcile session/object state and retry; do not fabricate readiness.
- **Definition of done:** DOD-1 plus this task's acceptance/validation evidence; update status/progress without claiming the next phase is complete.

<a id="p05-t05"></a>

### P05-T05 — Media cleanup and hash signals

- **ID / priority / status:** P05-T05 / MUST / NOT STARTED.
- **Goal:** Expire originals/orphans and retain bounded duplicate signals.
- **Why:** Control storage cost and privacy.
- **Inputs:** docs/security/SECURITY_PRIVACY.md, docs/backend/API_PLAN.md; B-BR-016; SECURITY_PRIVACY.
- **Files/areas expected:** `backend/internal/modules/evidence; retention jobs` (area list, not a literal combined path).
- **Dependencies:** P05-T04; all earlier phase gates.
- **Tests first:** Active lease not deleted, true orphan deleted, repeated cleanup safe, duplicate hash case.
- **Implementation outline:** Original cap 24 h, sanitized 14 days, case extension ≤30 days, hashes 90 days.
- **Acceptance criteria:** Lifecycle matches inventory and cleanup observable.
- **Validation commands:** `cd backend && go test -tags=integration ./internal/modules/evidence/...`. Where prose names a gate, introduce/document the exact executable command in this task before claiming completion.
- **Risks:** Deleting in-flight object or indefinite retention.
- **Rollback/Recovery:** Pause sweeper, fix eligibility; missing evidence remains marked unavailable.
- **Definition of done:** DOD-1 plus this task's acceptance/validation evidence; update status/progress without claiming the next phase is complete.

## P06 — Consensus and contributor trust

Priority: **MUST**. Entry: G05.

Exit gate: **G06: deterministic weighted projection with independent votes, conditions, expiry, disputes and conservative trust; golden/race tests prove invariants and payment isolation.**

<a id="p06-t01"></a>

### P06-T01 — Derived validation signals

- **ID / priority / status:** P06-T01 / MUST / NOT STARTED.
- **Goal:** Derive proximity, recency and risk without long-lived GPS.
- **Why:** Make confidence inputs explainable and minimized.
- **Inputs:** docs/backend/COMMUNITY_PRICING_SPEC.md, docs/backend/DATA_MODEL.md; B-BR-011/014; COMMUNITY_PRICING_SPEC.
- **Files/areas expected:** `backend/internal/modules/community/application; private payload store` (area list, not a literal combined path).
- **Dependencies:** G05; all earlier phase gates.
- **Tests first:** Missing/poor GPS, unknown station point, old capture, duplicate image, regional deviation.
- **Implementation outline:** Persist only derived bands/reason codes; purge exact data after derivation.
- **Acceptance criteria:** No single weak signal certifies truth; suspicious facts enter review.
- **Validation commands:** `cd backend && go test ./internal/modules/community/...`. Where prose names a gate, introduce/document the exact executable command in this task before claiming completion.
- **Risks:** False certainty or retention leak.
- **Rollback/Recovery:** Version signal policy; re-evaluate permitted facts without resurrecting deleted GPS.
- **Definition of done:** DOD-1 plus this task's acceptance/validation evidence; update status/progress without claiming the next phase is complete.

<a id="p06-t02"></a>

### P06-T02 — Confirmation and dispute commands

- **ID / priority / status:** P06-T02 / MUST / NOT STARTED.
- **Goal:** Record unique independent support and reports.
- **Why:** Let contributors corroborate or contest safely.
- **Inputs:** docs/backend/COMMUNITY_PRICING_SPEC.md, docs/backend/DATA_MODEL.md; B-BR-006; BUC-005.
- **Files/areas expected:** `backend/internal/modules/community; contracts/openapi; migrations` (area list, not a literal combined path).
- **Dependencies:** P06-T01; all earlier phase gates.
- **Tests first:** Self-confirmation, duplicates, concurrency, repeated dispute, wrong target state.
- **Implementation outline:** Unique votes/reports with transactionally enqueued recalculation.
- **Acceptance criteria:** One contributor cannot amplify support; reports do not automatically erase prices.
- **Validation commands:** `cd backend && go test -race -tags=integration ./internal/modules/community/...`. Where prose names a gate, introduce/document the exact executable command in this task before claiming completion.
- **Risks:** Coordinated reports deny valid data.
- **Rollback/Recovery:** Moderate cases and recompute; retain reports/audit within retention.
- **Definition of done:** DOD-1 plus this task's acceptance/validation evidence; update status/progress without claiming the next phase is complete.

<a id="p06-t03"></a>

### P06-T03 — Conservative trust ledger

- **ID / priority / status:** P06-T03 / MUST / NOT STARTED.
- **Goal:** Promote reliability only from independent reviewed outcomes.
- **Why:** Avoid self-reinforcing Sybil reputation.
- **Inputs:** docs/backend/COMMUNITY_PRICING_SPEC.md, docs/backend/DATA_MODEL.md; trust-v1; B-BR-009.
- **Files/areas expected:** `backend/internal/modules/trust; migrations; queries/trust` (area list, not a literal combined path).
- **Dependencies:** P06-T02; all earlier phase gates.
- **Tests first:** Age/day/outcome boundaries, blocked/reversed state, payment invariance.
- **Implementation outline:** Append trust decisions and expose read port; event to recalculate affected keys.
- **Acceptance criteria:** No raw volume/paid plan grants trust.
- **Validation commands:** `cd backend && go test -tags=integration ./internal/modules/trust/...`. Where prose names a gate, introduce/document the exact executable command in this task before claiming completion.
- **Risks:** Circular trust promotion or inherited purchase influence.
- **Rollback/Recovery:** Publish new trust decision/policy and recompute.
- **Definition of done:** DOD-1 plus this task's acceptance/validation evidence; update status/progress without claiming the next phase is complete.

<a id="p06-t04"></a>

### P06-T04 — Consensus pure algorithm

- **ID / priority / status:** P06-T04 / MUST / NOT STARTED.
- **Goal:** Implement deterministic consensus-v1 golden cases.
- **Why:** Compute explainable price instead of last-write-wins.
- **Inputs:** docs/backend/COMMUNITY_PRICING_SPEC.md, docs/backend/DATA_MODEL.md; COMMUNITY_PRICING_SPEC; B-BR-007/008.
- **Files/areas expected:** `backend/internal/modules/community/domain; contracts/testdata/consensus` (area list, not a literal combined path).
- **Dependencies:** P06-T03; all earlier phase gates.
- **Tests first:** All required golden cases plus permutation/ties/expiry/conflict and no paid weight.
- **Implementation outline:** Exact amount groups, one vote/contributor, recency/trust weights and confidence gates.
- **Acceptance criteria:** Same facts/time/version always produce same output.
- **Validation commands:** `cd backend && go test ./internal/modules/community/domain/...`. Where prose names a gate, introduce/document the exact executable command in this task before claiming completion.
- **Risks:** False-high confidence or underspecified ties.
- **Rollback/Recovery:** Switch back to prior version and rebuild restricted affected keys.
- **Definition of done:** DOD-1 plus this task's acceptance/validation evidence; update status/progress without claiming the next phase is complete.

<a id="p06-t05"></a>

### P06-T05 — Projection and read publication

- **ID / priority / status:** P06-T05 / MUST / NOT STARTED.
- **Goal:** Persist current prices with race-safe recomputation and expiry.
- **Why:** Keep reads cheap and never current past expiry.
- **Inputs:** docs/backend/COMMUNITY_PRICING_SPEC.md, docs/backend/DATA_MODEL.md; DATA_MODEL projection; API_PLAN.
- **Files/areas expected:** `backend/internal/modules/community; jobs; price response OpenAPI` (area list, not a literal combined path).
- **Dependencies:** P06-T04; all earlier phase gates.
- **Tests first:** Concurrent jobs, stale worker, timed decay, worker down at expiry, mixed sources.
- **Implementation outline:** Lock price key before load; version output; schedule boundary recompute; query-time expiry.
- **Acceptance criteria:** Indexed GET returns source-separated response without history replay.
- **Validation commands:** `cd backend && go test -race -tags=integration ./internal/modules/community/...`. Where prose names a gate, introduce/document the exact executable command in this task before claiming completion.
- **Risks:** Old worker wins or stale CDN price survives expiry.
- **Rollback/Recovery:** Invalidate cache and rebuild projection from eligible retained facts.
- **Definition of done:** DOD-1 plus this task's acceptance/validation evidence; update status/progress without claiming the next phase is complete.

## P07 — Moderation and privacy operations

Priority: **MUST**. Entry: G06.

Exit gate: **G07: restricted audited operations, appeal/abuse handling, export/erasure and retention jobs with restore-safe deletion ledger.**

<a id="p07-t01"></a>

### P07-T01 — Moderation case queue

- **ID / priority / status:** P07-T01 / MUST / NOT STARTED.
- **Goal:** Create bounded actionable cases from reports/signals.
- **Why:** Make abuse review feasible without a dashboard.
- **Inputs:** docs/security/SECURITY_PRIVACY.md, docs/backend/DOMAIN_MODEL.md; B-BR-012; BUC-006.
- **Files/areas expected:** `backend/internal/modules/moderation; migrations` (area list, not a literal combined path).
- **Dependencies:** G06; all earlier phase gates.
- **Tests first:** Duplicate case, priority ordering and safe evidence-reference tests.
- **Implementation outline:** Owned case repository and cursor query with minimal sensitive fields.
- **Acceptance criteria:** Operators can locate pending case and reasons.
- **Validation commands:** `cd backend && go test -tags=integration ./internal/modules/moderation/...`. Where prose names a gate, introduce/document the exact executable command in this task before claiming completion.
- **Risks:** Unbounded noisy queue or copied private payload.
- **Rollback/Recovery:** Tune case admission; retain original reports and processing state.
- **Definition of done:** DOD-1 plus this task's acceptance/validation evidence; update status/progress without claiming the next phase is complete.

<a id="p07-t02"></a>

### P07-T02 — Restricted operator commands

- **ID / priority / status:** P07-T02 / MUST / NOT STARTED.
- **Goal:** Apply audited invalidation, blocking and reviewed overrides.
- **Why:** Control privileged changes with accountable recovery.
- **Inputs:** docs/security/SECURITY_PRIVACY.md, docs/backend/DOMAIN_MODEL.md; BUC-006; SECURITY_PRIVACY.
- **Files/areas expected:** `backend/cmd/ops; moderation application; access runbook` (area list, not a literal combined path).
- **Dependencies:** P07-T01; all earlier phase gates.
- **Tests first:** Unauthorized operator denied; missing reason rejected; action/event/job atomic.
- **Implementation outline:** CLI over restricted operator access; audited evidence URL; appeal creates new decision/fact.
- **Acceptance criteria:** No anonymous/public admin path; history cannot be edited.
- **Validation commands:** `cd backend && go test -tags=integration ./internal/modules/moderation/...`. Where prose names a gate, introduce/document the exact executable command in this task before claiming completion.
- **Risks:** Privilege escalation or unaudited manual SQL.
- **Rollback/Recovery:** Revoke operator access; audited reversal and projection rebuild.
- **Definition of done:** DOD-1 plus this task's acceptance/validation evidence; update status/progress without claiming the next phase is complete.

<a id="p07-t03"></a>

### P07-T03 — Contributor export

- **ID / priority / status:** P07-T03 / MUST / NOT STARTED.
- **Goal:** Export authenticated owner data with short-lived access.
- **Why:** Support transparent data access.
- **Inputs:** docs/security/SECURITY_PRIVACY.md, docs/backend/DOMAIN_MODEL.md; BUC-007; API_PLAN.
- **Files/areas expected:** `backend/internal/modules/identity; privacy application; private export adapter` (area list, not a literal combined path).
- **Dependencies:** P07-T02; all earlier phase gates.
- **Tests first:** Wrong owner, expired download, redacted unrelated contributors.
- **Implementation outline:** Durable request/job; bounded archive; access after proof and private URL expiry.
- **Acceptance criteria:** Export complete for inventory without leaking other contributors.
- **Validation commands:** `Run privacy integration/E2E export tests`. Where prose names a gate, introduce/document the exact executable command in this task before claiming completion.
- **Risks:** Export becomes a public long-lived data dump.
- **Rollback/Recovery:** Expire/delete object and revoke exposed credential if applicable.
- **Definition of done:** DOD-1 plus this task's acceptance/validation evidence; update status/progress without claiming the next phase is complete.

<a id="p07-t04"></a>

### P07-T04 — Erasure and restore ledger

- **ID / priority / status:** P07-T04 / MUST / NOT STARTED.
- **Goal:** Remove personal data and reapply deletions after recovery.
- **Why:** Resolve immutable history versus privacy rights.
- **Inputs:** docs/security/SECURITY_PRIVACY.md, docs/backend/DOMAIN_MODEL.md; ADR-008; B-BR-016.
- **Files/areas expected:** `privacy application; cleanup jobs; deletion ledger; recovery runbook` (area list, not a literal combined path).
- **Dependencies:** P07-T03; all earlier phase gates.
- **Tests first:** Erasure removes links/media/location; repeated request safe; backup restore reapplies deletion.
- **Implementation outline:** Revoke writes, purge/unlink per reviewed policy, recompute affected prices; retain minimal bounded ledger.
- **Acceptance criteria:** No restored erased data served before replay; receipt auditable.
- **Validation commands:** `Run privacy integration tests and isolated backup-restore deletion test`. Where prose names a gate, introduce/document the exact executable command in this task before claiming completion.
- **Risks:** Retaining identifying history under an immutability pretext.
- **Rollback/Recovery:** Stop traffic after restore until ledger/retention checks pass.
- **Definition of done:** DOD-1 plus this task's acceptance/validation evidence; update status/progress without claiming the next phase is complete.

<a id="p07-t05"></a>

### P07-T05 — Retention scheduler and rights notice

- **ID / priority / status:** P07-T05 / MUST / NOT STARTED.
- **Goal:** Make every inventory period enforceable and visible.
- **Why:** Prevent a paper-only privacy plan.
- **Inputs:** docs/security/SECURITY_PRIVACY.md, docs/backend/DOMAIN_MODEL.md; SECURITY_PRIVACY inventory; BUC-007.
- **Files/areas expected:** `retention jobs; operator docs; draft backend privacy notice` (area list, not a literal combined path).
- **Dependencies:** P07-T04; all earlier phase gates.
- **Tests first:** Clock boundary/pagination/batch retry tests for each retained data category.
- **Implementation outline:** Schedule bounded purges; metrics on oldest overdue data; review lawful bases/contact process.
- **Acceptance criteria:** Every collected field has purpose, retention, access and deletion behavior.
- **Validation commands:** `Run retention integration suite with accelerated fake clock`. Where prose names a gate, introduce/document the exact executable command in this task before claiming completion.
- **Risks:** Legal assumptions or unbounded holds.
- **Rollback/Recovery:** Pause pilot collection for unresolved inventory item; fix cleanup then verify.
- **Definition of done:** DOD-1 plus this task's acceptance/validation evidence; update status/progress without claiming the next phase is complete.

## P08 — Infrastructure and production hardening

Priority: **MUST**. Entry: G07.

Exit gate: **G08: secured staging/prod topology and reproducible deployment, restricted roles, encrypted backups/restoration, monitoring, cache behavior, failure/load/security tests.**

<a id="p08-t01"></a>

### P08-T01 — Environment and network configuration

- **ID / priority / status:** P08-T01 / MUST / NOT STARTED.
- **Goal:** Prepare separate staging/production Compose and Caddy configs.
- **Why:** Remove dev defaults from production path.
- **Inputs:** docs/backend/INFRASTRUCTURE_PLAN.md, docs/backend/TEST_STRATEGY.md; INFRASTRUCTURE_PLAN; SECURITY_PRIVACY.
- **Files/areas expected:** `infra/compose.*.yml; infra/caddy; environment templates` (area list, not a literal combined path).
- **Dependencies:** G07; all earlier phase gates.
- **Tests first:** Config validation rejects dev secrets, public DB/metrics and untrusted forwarded headers.
- **Implementation outline:** Pin images, persistent volumes, resource limits, TLS/origin firewall and roles.
- **Acceptance criteria:** Reviewable config with no purchased/provisioned resources unless execution authorized.
- **Validation commands:** `docker compose -f infra/compose.staging.yml config; run network/TLS smoke`. Where prose names a gate, introduce/document the exact executable command in this task before claiming completion.
- **Risks:** Accidentally exposing database or shared environment keys.
- **Rollback/Recovery:** Revert network config; rotate exposed keys; never delete DB volume.
- **Definition of done:** DOD-1 plus this task's acceptance/validation evidence; update status/progress without claiming the next phase is complete.

<a id="p08-t02"></a>

### P08-T02 — Deployment and rollback procedure

- **ID / priority / status:** P08-T02 / MUST / NOT STARTED.
- **Goal:** Deploy one versioned monolith release safely.
- **Why:** Make migration/application ordering recoverable.
- **Inputs:** docs/backend/INFRASTRUCTURE_PLAN.md, docs/backend/TEST_STRATEGY.md; DATA_MODEL migration policy; INFRASTRUCTURE_PLAN.
- **Files/areas expected:** `infra/scripts/deploy; release runbook; CI publish design` (area list, not a literal combined path).
- **Dependencies:** P08-T01; all earlier phase gates.
- **Tests first:** Failed migration/readiness returns to previous compatible release.
- **Implementation outline:** Backup precheck, migration lock, digest rollout and smoke/monitor steps.
- **Acceptance criteria:** Staging rollback exercised; no destructive down migration required.
- **Validation commands:** `Run staging deployment/revert drill with synthetic data`. Where prose names a gate, introduce/document the exact executable command in this task before claiming completion.
- **Risks:** Mixed schema/binary versions.
- **Rollback/Recovery:** Use tested prior binary plus forward schema fix; pause writes if needed.
- **Definition of done:** DOD-1 plus this task's acceptance/validation evidence; update status/progress without claiming the next phase is complete.

<a id="p08-t03"></a>

### P08-T03 — Encrypted off-host backups

- **ID / priority / status:** P08-T03 / MUST / NOT STARTED.
- **Goal:** Create scheduled verifiable backup pipeline.
- **Why:** Protect against VPS/volume loss.
- **Inputs:** docs/backend/INFRASTRUCTURE_PLAN.md, docs/backend/TEST_STRATEGY.md; INFRASTRUCTURE_PLAN backup.
- **Files/areas expected:** `infra/scripts/backup; scheduler/runbook; credential scope` (area list, not a literal combined path).
- **Dependencies:** P08-T02; all earlier phase gates.
- **Tests first:** Failed upload/expired credential/stale backup alarm and manifest verification.
- **Implementation outline:** Daily dump with roles/extensions manifest and encryption; 7 daily/4 weekly copies.
- **Acceptance criteria:** Off-host backup downloadable with independent recovery credentials.
- **Validation commands:** `Run backup to isolated destination and verify checksum manifest`. Where prose names a gate, introduce/document the exact executable command in this task before claiming completion.
- **Risks:** Snapshot-only false assurance or keys stored on lost VPS.
- **Rollback/Recovery:** Repair backup/key custody before accepting production writes.
- **Definition of done:** DOD-1 plus this task's acceptance/validation evidence; update status/progress without claiming the next phase is complete.

<a id="p08-t04"></a>

### P08-T04 — Restore drill and deletion replay

- **ID / priority / status:** P08-T04 / MUST / NOT STARTED.
- **Goal:** Prove recovery in a new isolated environment.
- **Why:** Validate RPO/RTO and privacy after restore.
- **Inputs:** docs/backend/INFRASTRUCTURE_PLAN.md, docs/backend/TEST_STRATEGY.md; BUC-008; backup artifact.
- **Files/areas expected:** `infra/scripts/restore; docs/release-evidence` (area list, not a literal combined path).
- **Dependencies:** P08-T03; all earlier phase gates.
- **Tests first:** Restore wrong/corrupt artifact rejected; deleted data stays removed.
- **Implementation outline:** Restore full DB, verify counts/hashes, rebuild projections, replay ledger and check evidence references.
- **Acceptance criteria:** Measured RPO≤24 h/RTO≤4 h or gate fails with revised accepted objective.
- **Validation commands:** `Run documented isolated restore drill and smoke suite`. Where prose names a gate, introduce/document the exact executable command in this task before claiming completion.
- **Risks:** Backup cannot restore or restores erased data.
- **Rollback/Recovery:** Keep production closed; fix backup/replay and repeat drill.
- **Definition of done:** DOD-1 plus this task's acceptance/validation evidence; update status/progress without claiming the next phase is complete.

<a id="p08-t05"></a>

### P08-T05 — Metrics, logs and operator alerts

- **ID / priority / status:** P08-T05 / MUST / NOT STARTED.
- **Goal:** Monitor actionable service and data-quality failures.
- **Why:** Detect silent job/import/backup problems.
- **Inputs:** docs/backend/INFRASTRUCTURE_PLAN.md, docs/backend/TEST_STRATEGY.md; INFRASTRUCTURE_PLAN budgets.
- **Files/areas expected:** `backend/internal/platform/telemetry; infra monitoring; runbooks` (area list, not a literal combined path).
- **Dependencies:** P08-T04; all earlier phase gates.
- **Tests first:** Injected DB/job/backup failure triggers correct bounded alert; log redaction.
- **Implementation outline:** Add metrics route privately, key dashboards/queries and tested notification route.
- **Acceptance criteria:** No high-cardinality personal labels; owner knows response procedure.
- **Validation commands:** `Run failure injection and inspect metric/alert evidence`. Where prose names a gate, introduce/document the exact executable command in this task before claiming completion.
- **Risks:** Monitoring itself leaks personal data or floods operator.
- **Rollback/Recovery:** Disable leaking field/label; adjust alerts without hiding real failures.
- **Definition of done:** DOD-1 plus this task's acceptance/validation evidence; update status/progress without claiming the next phase is complete.

<a id="p08-t06"></a>

### P08-T06 — Public cache and proxy verification

- **ID / priority / status:** P08-T06 / MUST / NOT STARTED.
- **Goal:** Verify edge caching without identity/location leaks.
- **Why:** Reduce read cost while preserving freshness.
- **Inputs:** docs/backend/INFRASTRUCTURE_PLAN.md, docs/backend/TEST_STRATEGY.md; API_PLAN caching.
- **Files/areas expected:** `infra edge config documentation; staging API tests` (area list, not a literal combined path).
- **Dependencies:** P08-T05; all earlier phase gates.
- **Tests first:** Authenticated/private/media/nearby never shared cached; expiry/ETag/purge tested.
- **Implementation outline:** Allowlist public station GETs; canonical origin headers; bounded TTL.
- **Acceptance criteria:** Cache behavior measured at edge; signed writes still verify through proxy.
- **Validation commands:** `Run staging cache/header/signature regression suite`. Where prose names a gate, introduce/document the exact executable command in this task before claiming completion.
- **Risks:** Shared cache disclosure or stale disputed prices.
- **Rollback/Recovery:** Disable cache rule and purge; fall back to secured origin reads.
- **Definition of done:** DOD-1 plus this task's acceptance/validation evidence; update status/progress without claiming the next phase is complete.

<a id="p08-t07"></a>

### P08-T07 — Load and failure campaign

- **ID / priority / status:** P08-T07 / MUST / NOT STARTED.
- **Goal:** Measure admission, reads, queues and recovery under faults.
- **Why:** Choose pilot capacity from evidence.
- **Inputs:** docs/backend/INFRASTRUCTURE_PLAN.md, docs/backend/TEST_STRATEGY.md; INFRASTRUCTURE_PLAN workload.
- **Files/areas expected:** `backend/testdata/load; infra/scripts/load; docs/release-evidence` (area list, not a literal combined path).
- **Dependencies:** P08-T06; all earlier phase gates.
- **Tests first:** Seed deterministic data and assert workload/latency metrics before campaign.
- **Implementation outline:** Run origin/cache scenarios, DB/R2/geocoder/worker faults and disk-pressure simulation.
- **Acceptance criteria:** Budgets met or explicitly reduced/pending; no data loss or fake prices.
- **Validation commands:** `Run documented 30-minute acceptance load plus bounded fault suite`. Where prose names a gate, introduce/document the exact executable command in this task before claiming completion.
- **Risks:** Benchmarks on tiny data or misleading cache-only performance.
- **Rollback/Recovery:** Throttle admissions/scale measured bottleneck; repeat failing scenario.
- **Definition of done:** DOD-1 plus this task's acceptance/validation evidence; update status/progress without claiming the next phase is complete.

<a id="p08-t08"></a>

### P08-T08 — Security and dependency release review

- **ID / priority / status:** P08-T08 / MUST / NOT STARTED.
- **Goal:** Close launch-blocking auth/privacy/storage vulnerabilities.
- **Why:** Prevent unsafe pilot exposure.
- **Inputs:** docs/backend/INFRASTRUCTURE_PLAN.md, docs/backend/TEST_STRATEGY.md; SECURITY_PRIVACY STRIDE; TEST_STRATEGY.
- **Files/areas expected:** `security regression tests; dependency inventory; release evidence` (area list, not a literal combined path).
- **Dependencies:** P08-T07; all earlier phase gates.
- **Tests first:** Replay/IDOR/SSRF/oversize/role/secret cases; known-vulnerability scans.
- **Implementation outline:** Review auth vectors, storage ACLs, least privilege and policy inventory; remediate focused findings.
- **Acceptance criteria:** No unresolved critical/high findings; evidence linked.
- **Validation commands:** `cd backend && govulncheck ./...; run pinned secret/dependency/auth security gates`. Where prose names a gate, introduce/document the exact executable command in this task before claiming completion.
- **Risks:** False confidence from scan-only review.
- **Rollback/Recovery:** Block release until remediation verified; roll back insecure deployment.
- **Definition of done:** DOD-1 plus this task's acceptance/validation evidence; update status/progress without claiming the next phase is complete.

## P09 — Backend release gate before Android

Priority: **MUST**. Entry: G08.

Exit gate: **G09: backend MVP and infrastructure fully exercised, release evidence accepted, no blocker in security/restore/catalog/identity/community/privacy; only then P10 can start.**

<a id="p09-t01"></a>

### P09-T01 — End-to-end backend release rehearsal

- **ID / priority / status:** P09-T01 / MUST / NOT STARTED.
- **Goal:** Exercise all MVP flows against deployed candidate.
- **Why:** Prove modules work together beyond isolated tests.
- **Inputs:** docs/product/PRODUCT_CONTRACT.md, docs/backend/TEST_STRATEGY.md; BUC-001…008; frozen OpenAPI.
- **Files/areas expected:** `backend/testdata/e2e; docs/release-evidence` (area list, not a literal combined path).
- **Dependencies:** G08; all earlier phase gates.
- **Tests first:** Full register/upload/observe/confirm/dispute/moderate/export/erase flow plus denied paths.
- **Implementation outline:** Run synthetic clients only; inspect source separation and durable events/jobs.
- **Acceptance criteria:** All critical flows pass deployed topology.
- **Validation commands:** `Run documented deployed E2E and full backend gate`. Where prose names a gate, introduce/document the exact executable command in this task before claiming completion.
- **Risks:** Happy-path-only evidence or accidental real personal-data use.
- **Rollback/Recovery:** Rollback candidate and preserve synthetic failure evidence.
- **Definition of done:** DOD-1 plus this task's acceptance/validation evidence; update status/progress without claiming the next phase is complete.

<a id="p09-t02"></a>

### P09-T02 — Compatibility and documentation closure

- **ID / priority / status:** P09-T02 / MUST / NOT STARTED.
- **Goal:** Reconcile contract, baseline and runbooks.
- **Why:** Avoid passing the gate with unresolved inherited assumptions.
- **Inputs:** docs/product/PRODUCT_CONTRACT.md, docs/backend/TEST_STRATEGY.md; CURRENT_STATE_AUDIT; BASELINE_VALIDATION; MIGRATION_PLAN.
- **Files/areas expected:** `contracts; docs/planning; release evidence` (area list, not a literal combined path).
- **Dependencies:** P09-T01; all earlier phase gates.
- **Tests first:** Review every A01-A13 resolution/defer rationale and fixture expectation.
- **Implementation outline:** Rerun existing Android regression baseline; freeze known legacy adapter deltas; verify links and operational commands.
- **Acceptance criteria:** No unspecified breaking semantic; baseline environment blockers resolved.
- **Validation commands:** `Run existing Android unit/lint/build gates and contract validation`. Where prose names a gate, introduce/document the exact executable command in this task before claiming completion.
- **Risks:** Treating planned Kotlin-Go harness as already passing.
- **Rollback/Recovery:** Record blocked gate; no Android feature edits until prerequisites/evidence resolved.
- **Definition of done:** DOD-1 plus this task's acceptance/validation evidence; update status/progress without claiming the next phase is complete.

<a id="p09-t03"></a>

### P09-T03 — Backend readiness sign-off

- **ID / priority / status:** P09-T03 / MUST / NOT STARTED.
- **Goal:** Record the released bounded backend MVP and unblock P10.
- **Why:** Enforce the user-requested ordering.
- **Inputs:** docs/product/PRODUCT_CONTRACT.md, docs/backend/TEST_STRATEGY.md; G01…G08 evidence; D01…D12 decisions.
- **Files/areas expected:** `docs/release-evidence/G09.md; docs/planning/PROGRESS.md; ROADMAP.md status only` (area list, not a literal combined path).
- **Dependencies:** P09-T02; all earlier phase gates.
- **Tests first:** Checklist rejects missing restore/security/privacy/load evidence or open launch blockers.
- **Implementation outline:** Record commit/image/schema/policy/environment, operator readiness and rollback; mark gate only with proof.
- **Acceptance criteria:** G09 COMPLETE with links and next P10-T01; otherwise remains BLOCKED.
- **Validation commands:** `Review all release evidence and smoke actual release endpoints`. Where prose names a gate, introduce/document the exact executable command in this task before claiming completion.
- **Risks:** Checkbox-only completion or scope creep into paid products.
- **Rollback/Recovery:** Withdraw gate if material regression; keep Android integration unstarted.
- **Definition of done:** DOD-1 plus this task's acceptance/validation evidence; update status/progress without claiming the next phase is complete.

## P10 — Android integration after G09

Priority: **MUST**. Entry: G09 COMPLETE required before ANY task in this phase.

Exit gate: **G10: cross-language contracts, Room migration, identity, contribution/OCR, source/freshness UI, offline/outage tests and controlled pilot release; existing app preserved.**

<a id="p10-t01"></a>

### P10-T01 — Kotlin contract harness and compatibility adapters

- **ID / priority / status:** P10-T01 / MUST / NOT STARTED.
- **Goal:** Consume shared fixtures in the existing architecture.
- **Why:** Resolve legacy fuel/CNPJ/precision differences explicitly.
- **Inputs:** docs/MIGRATION_PLAN.md, docs/user-business-logic.md; G09 evidence; ANP_INGESTION fixture manifest.
- **Files/areas expected:** `domain tests; data adapters/tests; contracts` (area list, not a literal combined path).
- **Dependencies:** G09 COMPLETE required before ANY task in this phase; all earlier phase gates.
- **Tests first:** Legacy and target mapping fixtures, alpha CNPJ, exact decimal and unknown enums.
- **Implementation outline:** Add Kotlin harness; bounded adapters only; no package moves.
- **Acceptance criteria:** Go and Kotlin pass agreed fixtures; intended differences documented.
- **Validation commands:** `./gradlew :domain:test :data:testDebugUnitTest`. Where prose names a gate, introduce/document the exact executable command in this task before claiming completion.
- **Risks:** Accidentally changing historical semantics.
- **Rollback/Recovery:** Revert feature flag/adapters; retain original local data.
- **Definition of done:** DOD-1 plus this task's acceptance/validation evidence; update status/progress without claiming the next phase is complete.

<a id="p10-t02"></a>

### P10-T02 — Community read ports and local cache

- **ID / priority / status:** P10-T02 / MUST / NOT STARTED.
- **Goal:** Add backend reads without removing ANP offline paths.
- **Why:** Keep app useful on backend outage.
- **Inputs:** docs/MIGRATION_PLAN.md, docs/user-business-logic.md; API_PLAN; UC-004…009.
- **Files/areas expected:** `domain ports; application use cases; data HTTP/Room adapters` (area list, not a literal combined path).
- **Dependencies:** P10-T01; all earlier phase gates.
- **Tests first:** Backend down, no cache, stale cache and schema-4 upgrade tests.
- **Implementation outline:** Add source/version/expiry cache with explicit Room migration and feature flag.
- **Acceptance criteria:** Local ANP screens work unchanged; migrations preserve vehicles/history.
- **Validation commands:** `./gradlew :domain:test :application:test :data:testDebugUnitTest :data:connectedDebugAndroidTest`. Where prose names a gate, introduce/document the exact executable command in this task before claiming completion.
- **Risks:** Destructive local migration or mandatory network login.
- **Rollback/Recovery:** Disable community reads; use compatible schema and prior local cache.
- **Definition of done:** DOD-1 plus this task's acceptance/validation evidence; update status/progress without claiming the next phase is complete.

<a id="p10-t03"></a>

### P10-T03 — Device keys and anonymous protocol

- **ID / priority / status:** P10-T03 / MUST / NOT STARTED.
- **Goal:** Use Keystore and agreed proof on supported devices.
- **Why:** Authenticate contribution without accounts.
- **Inputs:** docs/MIGRATION_PLAN.md, docs/user-business-logic.md; ADR-007; identity vectors.
- **Files/areas expected:** `data security adapter; application identity flow` (area list, not a literal combined path).
- **Dependencies:** P10-T02; all earlier phase gates.
- **Tests first:** API26/current device key generation/signature interoperability, reinstall/rotation/lost key.
- **Implementation outline:** Implement non-exportable P-256, challenges and rotation; honest loss warning.
- **Acceptance criteria:** Real supported devices pass frozen protocol; no private key backup.
- **Validation commands:** `./gradlew :data:testDebugUnitTest :data:connectedDebugAndroidTest`. Where prose names a gate, introduce/document the exact executable command in this task before claiming completion.
- **Risks:** Unproven secure hardware support or key export.
- **Rollback/Recovery:** Disable new contribution flag and preserve usable anonymous identity.
- **Definition of done:** DOD-1 plus this task's acceptance/validation evidence; update status/progress without claiming the next phase is complete.

<a id="p10-t04"></a>

### P10-T04 — Capture and local OCR

- **ID / priority / status:** P10-T04 / MUST / NOT STARTED.
- **Goal:** Capture/crop/compress and confirm extracted price.
- **Why:** Reduce server cost and mistaken condition inference.
- **Inputs:** docs/MIGRATION_PLAN.md, docs/user-business-logic.md; COMMUNITY_PRICING_SPEC; new Android UC.
- **Files/areas expected:** `app capture UI; data CameraX/ML Kit adapter` (area list, not a literal combined path).
- **Dependencies:** P10-T03; all earlier phase gates.
- **Tests first:** Permission denied, cancel, multiple prices/conditions, low OCR confidence.
- **Implementation outline:** Record dependency rationale; local OCR candidates with human selection; no automatic lowest-price choice.
- **Acceptance criteria:** No capture upload until contributor confirms content/condition.
- **Validation commands:** `./gradlew :app:testDebugUnitTest :app:connectedDebugAndroidTest`. Where prose names a gate, introduce/document the exact executable command in this task before claiming completion.
- **Risks:** OCR hallucinated condition or unauthorized photo collection.
- **Rollback/Recovery:** Disable capture flag; retain metadata-only contribution.
- **Definition of done:** DOD-1 plus this task's acceptance/validation evidence; update status/progress without claiming the next phase is complete.

<a id="p10-t05"></a>

### P10-T05 — Contribution outbox and direct media flow

- **ID / priority / status:** P10-T05 / MUST / NOT STARTED.
- **Goal:** Retry durable commands safely while offline.
- **Why:** Handle real mobile disconnections.
- **Inputs:** docs/MIGRATION_PLAN.md, docs/user-business-logic.md; BUC-003/004; MIGRATION_PLAN outbox.
- **Files/areas expected:** `application contribution use cases; data Room/WorkManager/upload adapters` (area list, not a literal combined path).
- **Dependencies:** P10-T04; all earlier phase gates.
- **Tests first:** Process death, duplicate send, old draft, object success/finalize failure.
- **Implementation outline:** Stable command IDs, fresh nonce each retry, explicit queued/received/validated states.
- **Acceptance criteria:** One observation per command; old photo not relabelled as fresh.
- **Validation commands:** `./gradlew :application:test :data:testDebugUnitTest :data:connectedDebugAndroidTest`. Where prose names a gate, introduce/document the exact executable command in this task before claiming completion.
- **Risks:** Repeated submission or unbounded background upload.
- **Rollback/Recovery:** Pause WorkManager contribution queue; do not delete accepted history.
- **Definition of done:** DOD-1 plus this task's acceptance/validation evidence; update status/progress without claiming the next phase is complete.

<a id="p10-t06"></a>

### P10-T06 — Official and community price UI

- **ID / priority / status:** P10-T06 / MUST / NOT STARTED.
- **Goal:** Display source, condition, uncertainty and age separately.
- **Why:** Make hybrid data understandable.
- **Inputs:** docs/MIGRATION_PLAN.md, docs/user-business-logic.md; PRODUCT_CONTRACT; API_PLAN.
- **Files/areas expected:** `app price/station UI; viewmodels; strings en/pt-BR` (area list, not a literal combined path).
- **Dependencies:** P10-T05; all earlier phase gates.
- **Tests first:** UNKNOWN/DISPUTED/STALE, differing sources and APP qualifier display.
- **Implementation outline:** Add distinct sections and accessible explanations; no silent source blending.
- **Acceptance criteria:** Existing official browsing works; confidence not represented as guarantee.
- **Validation commands:** `./gradlew :app:testDebugUnitTest :app:lintDebug :app:connectedDebugAndroidTest`. Where prose names a gate, introduce/document the exact executable command in this task before claiming completion.
- **Risks:** Misleading cheapest-price ranking or missing units.
- **Rollback/Recovery:** Disable community panel via flag; keep ANP panel intact.
- **Definition of done:** DOD-1 plus this task's acceptance/validation evidence; update status/progress without claiming the next phase is complete.

<a id="p10-t07"></a>

### P10-T07 — Confirm and dispute flows

- **ID / priority / status:** P10-T07 / MUST / NOT STARTED.
- **Goal:** Expose structured corroboration and correction.
- **Why:** Complete free community participation.
- **Inputs:** docs/MIGRATION_PLAN.md, docs/user-business-logic.md; BUC-005; public representative observation reference.
- **Files/areas expected:** `app contribution UI; application/data adapters` (area list, not a literal combined path).
- **Dependencies:** P10-T06; all earlier phase gates.
- **Tests first:** Self-confirm denial, idempotent retry, replaced price and private reason.
- **Implementation outline:** Present conditions before confirmation; changed price creates observation.
- **Acceptance criteria:** Clear status and no repeated vote amplification.
- **Validation commands:** `./gradlew :application:test :app:testDebugUnitTest`. Where prose names a gate, introduce/document the exact executable command in this task before claiming completion.
- **Risks:** Confirming a stale/different condition accidentally.
- **Rollback/Recovery:** Disable affected action; preserve submitted records for review.
- **Definition of done:** DOD-1 plus this task's acceptance/validation evidence; update status/progress without claiming the next phase is complete.

<a id="p10-t08"></a>

### P10-T08 — Integrated offline and release checks

- **ID / priority / status:** P10-T08 / MUST / NOT STARTED.
- **Goal:** Prove upgrades and failure behavior on actual Android.
- **Why:** Protect existing users at pilot rollout.
- **Inputs:** docs/MIGRATION_PLAN.md, docs/user-business-logic.md; TEST_STRATEGY; MIGRATION_PLAN.
- **Files/areas expected:** `Android instrumentation; contracts; privacy notices; release evidence` (area list, not a literal combined path).
- **Dependencies:** P10-T07; all earlier phase gates.
- **Tests first:** Airplane mode, backend outage, wrong clock, schema-4 upgrade, old client/new API.
- **Implementation outline:** Run full targeted/instrumented suites, accessibility/i18n and privacy policy review.
- **Acceptance criteria:** No lost local data; public notices describe actual new collection.
- **Validation commands:** `./gradlew test :app:lintDebug :app:assembleDebug :data:connectedDebugAndroidTest :app:connectedDebugAndroidTest`. Where prose names a gate, introduce/document the exact executable command in this task before claiming completion.
- **Risks:** Paper privacy policy or untested existing-device migration.
- **Rollback/Recovery:** Stop rollout; disable community flag and use tested previous compatible release.
- **Definition of done:** DOD-1 plus this task's acceptance/validation evidence; update status/progress without claiming the next phase is complete.

<a id="p10-t09"></a>

### P10-T09 — Controlled community pilot

- **ID / priority / status:** P10-T09 / MUST / NOT STARTED.
- **Goal:** Release to limited cohort/city with measured quality.
- **Why:** Validate useful data before monetization.
- **Inputs:** docs/MIGRATION_PLAN.md, docs/user-business-logic.md; G10 test evidence; product metrics plan.
- **Files/areas expected:** `docs/product/pilot-results.md; runtime rollout configuration` (area list, not a literal combined path).
- **Dependencies:** P10-T08; all earlier phase gates.
- **Tests first:** Rehearse flag off/rollback; verify aggregate metrics without tracking.
- **Implementation outline:** Agree cohort/capacity/window, monitor freshness/disputes/cost and review outcomes.
- **Acceptance criteria:** Pilot report states evidence, limits and next product decisions.
- **Validation commands:** `Run release smoke and compare monitored pilot metrics to acceptance targets`. Where prose names a gate, introduce/document the exact executable command in this task before claiming completion.
- **Risks:** Premature broad release or unsupported confidence claims.
- **Rollback/Recovery:** Pause onboarding/contribution and keep ANP browsing available.
- **Definition of done:** DOD-1 plus this task's acceptance/validation evidence; update status/progress without claiming the next phase is complete.

## P11 — Optional hosted commercial services

Priority: **LATER**. Entry: Completed P10 pilot and explicit validated product scope.

Exit gate: **G11: optional account/billing/sync specifications and implementations pass provider/ownership/conflict tests; no effect on free trust or existing local data. Each task is independently respecified before execution.**

<a id="p11-t01"></a>

### P11-T01 — Validate hosted offering

- **ID / priority / status:** P11-T01 / LATER / NOT STARTED.
- **Goal:** Choose one measured paid convenience use case.
- **Why:** Avoid building speculative SaaS.
- **Inputs:** docs/product/OPEN_SOURCE_BUSINESS.md, docs/MIGRATION_PLAN.md; Pilot findings; D09/D10.
- **Files/areas expected:** `docs/product/hosted-offering.md; cost model` (area list, not a literal combined path).
- **Dependencies:** Completed P10 pilot and explicit validated product scope; all earlier phase gates.
- **Tests first:** Test demand assumptions and free-feature preservation checklist.
- **Implementation outline:** Define entitlement/grace/export scope, data inventory and lawful basis.
- **Acceptance criteria:** Concrete product acceptance, no paid trust feature.
- **Validation commands:** `Review product decision and updated unit-economics assumptions`. Where prose names a gate, introduce/document the exact executable command in this task before claiming completion.
- **Risks:** Building a large paid platform without demand.
- **Rollback/Recovery:** Keep free product; discard unvalidated experiment.
- **Definition of done:** DOD-1 plus this task's acceptance/validation evidence; update status/progress without claiming the next phase is complete.

<a id="p11-t02"></a>

### P11-T02 — Optional account linking

- **ID / priority / status:** P11-T02 / LATER / NOT STARTED.
- **Goal:** Bind contributor ownership to a recoverable account.
- **Why:** Offer device migration with proof.
- **Inputs:** docs/product/OPEN_SOURCE_BUSINESS.md, docs/MIGRATION_PLAN.md; New account UC/security ADR required.
- **Files/areas expected:** `identity/account module and adapters; contracts` (area list, not a literal combined path).
- **Dependencies:** P11-T01; all earlier phase gates.
- **Tests first:** Account takeover, stolen contributor ID, fresh reauth and key linking cases.
- **Implementation outline:** Use established identity provider; prove current key and account; specify recovery.
- **Acceptance criteria:** No unauthorized trust transfer; anonymous use continues.
- **Validation commands:** `Run account authz/recovery integration and E2E gates`. Where prose names a gate, introduce/document the exact executable command in this task before claiming completion.
- **Risks:** Recovery bypass becomes reputation theft.
- **Rollback/Recovery:** Revoke link, audit incident and keep original contributor attribution safe.
- **Definition of done:** DOD-1 plus this task's acceptance/validation evidence; update status/progress without claiming the next phase is complete.

<a id="p11-t03"></a>

### P11-T03 — Server-verified entitlement

- **ID / priority / status:** P11-T03 / LATER / NOT STARTED.
- **Goal:** Translate purchase lifecycle to provider-neutral access.
- **Why:** Never trust a premium boolean from client.
- **Inputs:** docs/product/OPEN_SOURCE_BUSINESS.md, docs/MIGRATION_PLAN.md; New billing UC; current Play provider docs at execution.
- **Files/areas expected:** `subscription module; billing adapter; contracts` (area list, not a literal combined path).
- **Dependencies:** P11-T02; all earlier phase gates.
- **Tests first:** Purchase replay/refund/revocation/notification duplication and stale state.
- **Implementation outline:** Verify server-side, reconcile provider events, persist entitlement version.
- **Acceptance criteria:** Payment independent from trust in regression tests.
- **Validation commands:** `Run provider sandbox verification and entitlement integration suite`. Where prose names a gate, introduce/document the exact executable command in this task before claiming completion.
- **Risks:** Fraudulent purchases or stale paid access.
- **Rollback/Recovery:** Disable hosted entitlement grants; reconcile without changing community votes.
- **Definition of done:** DOD-1 plus this task's acceptance/validation evidence; update status/progress without claiming the next phase is complete.

<a id="p11-t04"></a>

### P11-T04 — Explicit favorites/settings sync

- **ID / priority / status:** P11-T04 / LATER / NOT STARTED.
- **Goal:** Synchronize first simple domains with conflicts.
- **Why:** Avoid generic Room database replication.
- **Inputs:** docs/product/OPEN_SOURCE_BUSINESS.md, docs/MIGRATION_PLAN.md; New sync UC and data inventory.
- **Files/areas expected:** `cloudsync module; Android sync adapter after spec` (area list, not a literal combined path).
- **Dependencies:** P11-T03; all earlier phase gates.
- **Tests first:** Version conflict, two devices, tombstones, offline edit and subscription lapse.
- **Implementation outline:** Version/ETag and explicit per-field merge/409 policy; scoped exports.
- **Acceptance criteria:** No destructive replacement of local preferences.
- **Validation commands:** `Run cross-device sync conflict/offline E2E suite`. Where prose names a gate, introduce/document the exact executable command in this task before claiming completion.
- **Risks:** Silent last-write-wins data loss.
- **Rollback/Recovery:** Disable sync; retain local state/export and audited server revisions.
- **Definition of done:** DOD-1 plus this task's acceptance/validation evidence; update status/progress without claiming the next phase is complete.

<a id="p11-t05"></a>

### P11-T05 — Hosted backup and vehicle migration

- **ID / priority / status:** P11-T05 / LATER / NOT STARTED.
- **Goal:** Add paid convenience after simple sync proves reliable.
- **Why:** Support device change without reducing local free use.
- **Inputs:** docs/product/OPEN_SOURCE_BUSINESS.md, docs/MIGRATION_PLAN.md; New backup UC; pilot/sync evidence.
- **Files/areas expected:** `cloudsync module; Android opt-in backup` (area list, not a literal combined path).
- **Dependencies:** P11-T04; all earlier phase gates.
- **Tests first:** Restore preview/conflict/cancel, local vehicle preservation, expired entitlement.
- **Implementation outline:** Explicit consent and versioned domain export/import; exclude private signing keys.
- **Acceptance criteria:** Three free local vehicles remain; restore is recoverable.
- **Validation commands:** `Run backup/restore and old/new Android compatibility suite`. Where prose names a gate, introduce/document the exact executable command in this task before claiming completion.
- **Risks:** Paid plan causes local loss or leaks keys.
- **Rollback/Recovery:** Stop cloud writes; restore versioned user-selected snapshot.
- **Definition of done:** DOD-1 plus this task's acceptance/validation evidence; update status/progress without claiming the next phase is complete.

## Release-gate checklist G09

- [ ] G01…G08 evidence is linked to tested commits/images/schema/config.
- [ ] Catalog import, identity, observation, media, validation, consensus, moderation and privacy flows run end to end.
- [ ] Empty/upgrade migrations and duplicate/replay/concurrency cases pass.
- [ ] Current OpenAPI and shared fixtures are frozen; legacy Android differences documented.
- [ ] Existing Android regression baseline passes in a supported environment.
- [ ] TLS/origin restriction, private storage, role/secret/dependency checks and critical/high finding closure verified.
- [ ] Off-host encrypted backup restored into a new environment within accepted RPO/RTO; deletion ledger replayed.
- [ ] Runtime retention/export/erasure and user notice/legal review are complete.
- [ ] Load/fault/cache tests meet accepted capacity and freshness budgets.
- [ ] Operator can deploy, monitor, moderate, revoke, rollback and restore using tested runbooks.
- [ ] Release evidence signed off; P10-T01 is explicitly unblocked.


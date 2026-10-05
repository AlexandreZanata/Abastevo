# abastevo implementation roadmap

Planning revision: 2026-10-05; [commercial community plan](docs/planning/COMMERCIAL_COMMUNITY_PLAN.md) and ADR-016 own current app direction; [P25 static landing plan](docs/planning/STATIC_LANDING_PLAN.md) owns the independent website phase. Historical P01–P09 evidence is preserved in [progress history](docs/planning/history/P01_P09_PROGRESS_20260930.md); the [previous roadmap](docs/planning/history/ROADMAP_BEFORE_MULTIPLATFORM_20260930.md) retains original task wording. Current implementation/integration state is in [PROGRESS](docs/planning/PROGRESS.md), not the old initial NOT STARTED labels. Do not recreate completed work.

## How to execute

Read [AGENTS](AGENTS.md), [FAST_EXECUTION](docs/planning/FAST_EXECUTION.md), current [PROGRESS](docs/planning/PROGRESS.md) and the selected task. [DELIVERY_WORKFLOW](docs/planning/DELIVERY_WORKFLOW.md) and [CI_PLAN](docs/planning/CI_PLAN.md) govern one issue/atomic commit per task and one milestone/isolated branch per phase, local checkpoints and one cumulative final project batch PR under ADR-018. Helpers and required Quick verification are active; protection was reverified on 2026-09-30. Historical task labels remain as original estimates where not individually reconciled; Git and linked evidence determine actual state.

[Functional multiplatform delivery](docs/planning/MOBILE_DELIVERY_PLAN.md) is the new plan. **G09 is a deferred real-production RELEASE after G24-ANDROID-COMMERCIAL for Android/backend scope, not an app-entry blocker (ADR-016).** Historical full G18 remains unaccepted; iOS is archived until a new explicit user request. App work requires **G09-LOCAL**: the locally validated corrections integrated with current-head required CI. Free email-code/Google/Apple accounts and 280-character comments are user-confirmed; future functionality is NOT IMPLEMENTED merely by this plan. Existing Android regression remains allowed at any time.

Tests have three levels: immediate targeted task/risk checks; specialized phase exit plus one quick local/current-head remote gate; full immutable-candidate production certification only at actual G09. Critical auth/money/privacy/SQL/migration/job negative/concurrency/PostGIS checks never wait. DOD-1 is task acceptance, not release certification. No agent delegation is implied.

## Phase order

Completed backend/app history is indexed in PROGRESS. Current source construction order (ADR-019): **P34 → P35 → P36 → P37 → P38 code-ready → P25 → P26 → P27 → P29 code-ready → P30 → P31 → P32 → P33 code-ready → end Android manual/device acceptance → final cumulative PR/CI/merge/wiki → P09/G09 real-production certification → P10-T09 public pilot**. P28 sources/P11 paid benefits are optional; iOS remains archived. Preserve IDs and completed history; local source dependencies use validated checkpoints under ADR-018, while deployment/pilot/certification gates remain unchanged. [Android/VPS plan](docs/planning/ANDROID_VPS_PLAN.md) owns the next tasks; [catalog](#station-catalog-expansion) and [profile](#station-profile-and-representation) retain subsequent scope. No implementation or release is claimed by a plan.

Existing phase tasks below retain their detailed scoped checks; “all earlier phase gates” means this dependency graph, not ascending phase number or a dependency on deferred G09. Elapsed dates/costs are not invented. Split oversized tasks into letter-suffixed IDs with explicit acceptance before coding. New owning tasks must introduce/document executable acceptance commands when the plan names a descriptive gate.

**Independent website phase:** [P25 — Static landing page](#p25--static-landing-page) follows its own T01–T05 sequence after existing G09-LOCAL integration. It can be built locally before Play Store availability; it neither blocks nor accepts Android/backend release gates. This planning request authorizes local docs only, with website publication and store-link activation handled separately.

## P00 — Project identity documentation and visual preview

Entry: maintainer-selected name/logo (2026-10-01). Exit: publish the approved identity/assets/README via guarded phase merge and wiki mirror. Maintainer approved V2 publication on 2026-10-01. This task does not block or alter the ongoing functional phase.

<a id="p00-t01"></a>

### P00-T01 — abastevo identity and README artwork proposal

- **ID / priority / status:** P00-T01 / MUST / LOCAL_DONE — APPROVED_FOR_PUBLICATION.
- **Goal:** Reflect the selected abastevo name and supplied logo in current documentation; prepare one minimal professional README banner.
- **Why:** Replace the provisional project name with the maintainer's chosen identity.
- **Inputs:** Maintainer's 2026-10-01 request and attached logo; docs/brand/IDENTITY.md; existing attribution/rights notices.
- **Files/areas expected:** README/TRADEMARKS; current product docs/decisions; docs/assets/brand; local README.brand-preview.md.
- **Dependencies:** No runtime dependency; isolated worktree while P17 occupies the primary checkout.
- **Tests first:** Inspect original logo and current naming references; verify exact source-byte copy, banner wordmark/legibility, image links and unchanged source/license files.
- **Implementation outline:** Preserve the supplied logo source; use built-in imagegen for a white-space composition with lowercase wordmark; record exact prompt/provenance and show the banner before adoption.
- **Acceptance criteria:** Selected name recorded; one usable visual draft and README preview shown for approval. No premature published banner or runtime/package rename.
- **Validation commands:** `git diff --check`; scoped Markdown-link/image inspection, source hash equality and changed-file review. No backend/Android aggregate suite for docs/assets.
- **Risks:** Misrepresenting draft art as approved; disturbing the occupied phase checkout; changing historic attribution.
- **Rollback/Recovery:** Keep original/logo history and isolated edits; discard only owned draft files if requested.
- **Definition of done:** Local draft validated and delivered; visual approval/publication are separate pending states. No remote issue/PR/wiki operation during preview.

<a id="p00-t02"></a>

### P00-T02 — Vectorize the original abastevo logo for app assets

- **ID / priority / status:** P00-T02 / MUST / LOCAL_DONE (assets only; not runtime-integrated).
- **Goal:** Save a scalable native-vector rendition of the supplied original logo for future app use.
- **Why:** Avoid scaling a fixed-resolution PNG in future branding work.
- **Inputs:** Original logo asset and maintainer request 2026-10-01; docs/brand/IDENTITY.md.
- **Files/areas expected:** docs/assets/brand vector master/platform exports; usage/evidence notes.
- **Dependencies:** Supplied logo selected; no dependency on README banner approval.
- **Tests first:** Inspect alpha/contours and color folds; render fidelity/size checks; reject embedded raster, unsafe SVG and external references.
- **Implementation outline:** Trace cleaned contours as curves, reconstruct native gradients and preserve transparent negative space; validate platform exports.
- **Acceptance criteria:** Genuine vector geometry, recognizably faithful silhouette/gradients, original source preserved and future app import instructions.
- **Validation commands:** SVG XML/path inspection, Inkscape render/export and visual checks; git diff --check and scoped secret review. No aggregate runtime tests for assets.
- **Risks:** Posterized color bands, raster embedding, altered silhouette or falsely claiming pixel-exact lossless recovery from PNG.
- **Rollback/Recovery:** Keep original PNG and use versioned generated filenames; do not replace active launcher assets.
- **Definition of done:** Assets validated and saved locally with provenance; runtime adoption and remote publication remain separate.

<a id="p00-t03"></a>

### P00-T03 — Reference lettering and native-vector README composition

- **ID / priority / status:** P00-T03 / MUST / LOCAL_DONE — APPROVED_FOR_PUBLICATION.
- **Goal:** Publish reference lettering and the combined native-vector README composition.
- **Dependencies:** P00-T02; maintainer publication approval 2026-10-01.
- **Acceptance criteria:** Eight faithful uppercase glyphs, native paths, unchanged logo geometry, approved V2 SVG embedded in README.
- **Validation commands:** SVG safety/geometry/hash checks; Inkscape renders; local links and git diff --check; phase quick gate through finish.
- **Scope:** Trace only the reference ABASTEVO text; combine it with the unchanged P00-T02 SVG logo in the existing horizontal README layout. No source/runtime changes or remote publication.
- **Acceptance/checks:** Preserve eight uppercase glyphs/counters, native paths without raster/font/external content, unchanged logo hash, render/visual comparison, local links, whitespace and scoped secret review. Present the revised preview for visual approval.
- **Evidence:** docs/brand/IDENTITY.md and docs/assets/brand wordmark provenance; no backend/mobile aggregate tests for artwork.

<a id="p00-t04"></a>

### P00-T04 — Adopt the vector A as native Android/iOS app icon

- **ID / priority / status:** P00-T04 / MUST / LOCAL_DONE — ANDROID_BUILT / IOS_ASSETS_PREPARED_DEFERRED.
- **Goal:** White background and unchanged vector A in native launcher assets.
- **Scope:** Android VectorDrawable adaptive foreground, white background, monochrome API33 layer, density fallbacks and manifest; iPhone/iPad opaque AppIcon PNGs derived directly from SVG, asset catalog and host settings.
- **Dependencies:** Approved P00-T02 SVG; no Mac available, existing iOS SPM host has no Xcode application target.
- **Acceptance criteria:** Master bytes/shape/gradients preserved; adaptive safe-zone masks do not clip the A; Android compiles/links; iOS entries have exact dimensions and no alpha; native iOS build remains explicitly unverified.
- **Validation commands:** Deterministic vector export + structural/raster dimension/crop checks; :app:assembleDebug and affected resource lint; git diff --check/secret review. No backend suite for icon assets.
- **Evidence:** docs/brand/APP_ICONS.md; runtime scope excludes permissions/data/identifiers.

<a id="p00-t05"></a>

### P00-T05 — Commercial community plan, explicit iOS archive and wiki Home

- **ID / priority / status:** P00-T05 / MUST / LOCAL_DONE — PLANNING_ONLY.
- **Goal:** Document Android-first commercial community experience, new bounded phases and explicit iOS resumption policy; align wiki Home with README.
- **Scope:** Product/ADR/ROADMAP/current progress/release dependency and approved one-time Home adoption. No future social UX implementation, deployment or acceptance claim.
- **Dependencies:** Integrated P18 local slice PR #67, P00-T04; explicit user request 2026-10-01.
- **Acceptance criteria:** Community primary/ANP reference, temporary private photos, free social rules and preserved tools explicit; P19–P24 tasks/gates; iOS archived until explicit request; historical G18 not falsely accepted; README/Home share banner/organization and preserve manual wiki pages.
- **Validation commands:** Local links/task/dependency/state consistency; scripts/tests/test-wiki.sh for selected overview; git diff --check and scoped secret review; required phase quick/current-head CI through finish; merged-SHA Home verification.
- **Evidence:** docs/planning/COMMERCIAL_COMMUNITY_PLAN.md, docs/planning/archive/IOS_DEFERRED.md, docs/planning/WIKI_HOME_ADOPTION.md and current batch evidence.

## P01 — Backend foundations

Priority: **MUST**. Entry: No earlier implementation gate; planning baseline exists..

Exit gate: **G01: documented prerequisite report; Go build/unit/static checks; typed config; health/lifecycle; real local PostGIS; migrations from empty; image build and CI; base OpenAPI lint. Android baseline blockers are recorded and resolved before G09.**

<a id="p01-t01"></a>

### P01-T01 — Prerequisites and inherited baseline

- **ID / priority / status:** P01-T01 / MUST / LOCAL_DONE (prior evidence; remote integration not reverified).
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

- **ID / priority / status:** P01-T02 / MUST / LOCAL_DONE (prior evidence; remote integration not reverified).
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

- **ID / priority / status:** P01-T03 / MUST / LOCAL_DONE (prior evidence; remote integration not reverified).
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

- **ID / priority / status:** P01-T04 / MUST / LOCAL_DONE (prior evidence; remote integration not reverified).
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

- **ID / priority / status:** P01-T05 / MUST / LOCAL_DONE (prior evidence; remote integration not reverified).
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

- **ID / priority / status:** P01-T06 / MUST / LOCAL_DONE (prior evidence; remote integration not reverified).
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

- **ID / priority / status:** P01-T07 / MUST / LOCAL_DONE (prior evidence; remote integration not reverified).
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

- **ID / priority / status:** P01-T08 / MUST / LOCAL_DONE (prior evidence; remote integration not reverified).
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

- **ID / priority / status:** P01-T09 / MUST / LOCAL_DONE (prior evidence; remote integration not reverified).
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

- **ID / priority / status:** P01-T10 / MUST / LOCAL_DONE (prior evidence; remote integration not reverified).
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

- **ID / priority / status:** P01-T11 / MUST / LOCAL_DONE (prior evidence; remote integration not reverified).
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

### P01-T12 — CI fast and full gate wiring (initial cadence; superseded by G01-FLOW)

- **ID / priority / status:** P01-T12 / MUST / LOCAL_DONE (prior evidence; remote integration not reverified).
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

<a id="delivery-flow-transition"></a>

## P01 extension — Activate fast phase delivery before P02

Entry: foundation P01-T01…T12 exists; confirm its local evidence and actual repository state. **G01-FLOW** exits only when quick/full separation, safe branch/PR lifecycle, idempotent issue preparation, owned-page wiki preview and actual CI/protection migration are proven. Remote or wiki permissions unavailable mean an explicit pending operation, not an invented success. Wiki target unavailability can remain a recorded publication blocker if generated docs are valid; required CI/protection uncertainty blocks workflow activation and merge.

These are implementation tasks for the already delivered plan; this documentation edit does not mark them done. Use a new bounded phase-extension branch/PR and reuse existing completed foundation history. Remote phase publication requires the applicable session scope; LOCAL_ONLY can prepare/test all artifacts and previews without mutations.

<a id="p01-t13"></a>

### P01-T13 — Separate local quick and release verification entry points

- **ID / priority / status:** P01-T13 / MUST / NOT STARTED.
- **Goal:** Make task, integration and release checks explicit and cheap to select.
- **Why:** Prevent repeated full suites while preserving critical checks.
- **Inputs:** CI_PLAN; TEST_STRATEGY; existing check-backend-fast.sh.
- **Files/areas expected:** Makefile or equivalent small entry points; scripts/check-*; explicit check manifest; backend/README.
- **Dependencies:** P01-T12; original foundation evidence; current session publication scope.
- **Tests first:** Compile-only cannot claim tests; selected test filter executes cases; missing tool/unclassified change/secret in committed PR diff fails; docs-only checks meaningful.
- **Implementation outline:** Build quick-verify and full verification composition from existing gates; retain targeted real PostGIS/race tasks; cache/pin tools; scanner/parser failures must propagate.
- **Acceptance criteria:** Named entry points work; quick budget measured; full matrix remains complete; no product behavior change.
- **Validation commands:** Run focused harnesses for gate selection/failure and one measured quick run; inspect full dependency graph, without running all release infrastructure here. Record the exact created executable commands and outcomes in the task evidence.
- **Risks:** Masking errors or scanning only a clean working-tree status.
- **Rollback/Recovery:** Keep current mandatory CI intact until T17 activates replacement.
- **Definition of done:** DOD-1 local acceptance; G01-FLOW additionally needs T17 integration evidence. Do not claim publication/activation based on planning alone.

<a id="p01-t14"></a>

### P01-T14 — Phase branch and PR lifecycle controller

- **ID / priority / status:** P01-T14 / MUST / NOT STARTED.
- **Goal:** Automate isolation, draft PR sync and guarded phase closure.
- **Why:** Keep one commit per task and one PR per phase with no unsafe shortcuts.
- **Inputs:** DELIVERY_WORKFLOW; template PHASE_RECORD; P01-T13.
- **Files/areas expected:** scripts/git-flow.sh and its isolated tests; phase state/manifest.
- **Dependencies:** P01-T13; original foundation evidence; current session publication scope.
- **Tests first:** Wrong repo/main/dirty tree, missing required check, failed/skipped/cancelled check, changed head/base and unmerged-branch deletion all refuse; dry-run causes zero mutations.
- **Implementation outline:** Implement start/sync/status/finish with trusted repo validation, quick local, named check identity and PR head/base binding, compare-and-swap merge and safe ancestry cleanup.
- **Acceptance criteria:** Synthetic Git/fake GitHub lifecycle passes; missing authorization does not mutate; no full suite per task/phase.
- **Validation commands:** Run shell syntax/static checks and fake-remote lifecycle tests; actual PR smoke belongs to T17. Record the exact created executable commands and outcomes in the task evidence.
- **Risks:** Racing PR heads, accepting unknown status, deleting unrelated work.
- **Rollback/Recovery:** Abort before mutation where possible; preserve branch/PR/issues for safe retry.
- **Definition of done:** DOD-1 local acceptance; G01-FLOW additionally needs T17 integration evidence. Do not claim publication/activation based on planning alone.

<a id="p01-t15"></a>

### P01-T15 — Issue and milestone reconciliation

- **ID / priority / status:** P01-T15 / MUST / NOT STARTED.
- **Goal:** Create/reuse the current phase task records from canonical roadmap.
- **Why:** Avoid manual bookkeeping and duplicate issue creation.
- **Inputs:** DELIVERY_WORKFLOW issue schema; TASK_ISSUE template; P01-T14.
- **Files/areas expected:** scripts issue/milestone adapter; phase ledger; repository issue template if useful.
- **Dependencies:** P01-T14; original foundation evidence; current session publication scope.
- **Tests first:** Paginated duplicate/open/closed task markers, human notes preserved, retry after API failure, dry-run no writes.
- **Implementation outline:** Read active phase only; match stable markers; synchronize owned fields/labels; link milestone/PR; keep locally completed tasks open until merge.
- **Acceptance criteria:** Idempotent preview lists exact real/missing records; cannot close completed-looking but unmerged tasks.
- **Validation commands:** Run reconciliation fixtures with fake API; review upcoming phase preview before authorized publication. Record the exact created executable commands and outcomes in the task evidence.
- **Risks:** Duplicate spam, overwriting discussion or recreating historical P01 issues.
- **Rollback/Recovery:** Stop remote reconciliation; keep IDs/result ledger and retry only missing mutations.
- **Definition of done:** DOD-1 local acceptance; G01-FLOW additionally needs T17 integration evidence. Do not claim publication/activation based on planning alone.

<a id="p01-t16"></a>

### P01-T16 — Wiki mirror with owned-page manifest

- **ID / priority / status:** P01-T16 / MUST / NOT STARTED.
- **Goal:** Generate a navigable wiki snapshot from merged documentation.
- **Why:** Keep public progress current without per-task manual rewriting.
- **Inputs:** DELIVERY_WORKFLOW wiki rules; docs index; P01-T15.
- **Files/areas expected:** scripts wiki exporter/publisher; generated-page manifest; fixture wiki repo.
- **Dependencies:** P01-T15; original foundation evidence; current session publication scope.
- **Tests first:** Nested README collision, rewritten links/anchors/assets, manual page preserved, edited managed page conflict, obsolete-owned-only deletion, identical snapshot no-op.
- **Implementation outline:** Export allowlisted docs at explicit merged SHA; generate Home/sidebar; validate second remote/scope; publish only within authorized wiki scope.
- **Acceptance criteria:** Offline preview complete with source SHA; no blanket Markdown deletion; no production/private content; failure leaves WIKI_PENDING.
- **Validation commands:** Run exporter tests against temporary local Git/wiki repositories and link checks; inspect publish dry-run. Record the exact created executable commands and outcomes in the task evidence.
- **Risks:** Losing hand-written wiki content or leaking private logs.
- **Rollback/Recovery:** Keep prior wiki commit; stop on conflicts; retry docs sync without backend reruns.
- **Definition of done:** DOD-1 local acceptance; G01-FLOW additionally needs T17 integration evidence. Do not claim publication/activation based on planning alone.

<a id="p01-t17"></a>

### P01-T17 — Activate CI cadence and protected phase integration

- **ID / priority / status:** P01-T17 / MUST / NOT STARTED.
- **Goal:** Safely switch actual workflows/protection and prove end-to-end delivery.
- **Why:** Make the new plan operational without a protection gap.
- **Inputs:** CI_PLAN transition; P01-T13…T16 evidence.
- **Files/areas expected:** .github/workflows; configured branch protection; delivery docs/state; controlled PR evidence.
- **Dependencies:** P01-T16; original foundation evidence; current session publication scope.
- **Tests first:** Draft and docs-only PR run named quick; absent/old/changed-head check blocks; full jobs only selected release/tag; current protections remain until replacement verified.
- **Implementation outline:** Introduce quick, observe required result, update protections/triggers in reviewed order, exercise authorized phase PR; verify issue closure and wiki publication/pending record.
- **Acceptance criteria:** G01-FLOW complete only with actual observed checks/protection; origin confirmed; missing permissions or required checks remain BLOCKED.
- **Validation commands:** Validate workflow config and controller harness; inspect one controlled authorized PR check/head/base/merge and actual branch protection; review wiki result independently. Record the exact created executable commands and outcomes in the task evidence.
- **Risks:** Required check disappears during transition; plan falsely labelled deployed.
- **Rollback/Recovery:** Keep/restore prior checks until replacement works; never bypass main protection or publish untested release.
- **Definition of done:** DOD-1 local acceptance; G01-FLOW additionally needs T17 integration evidence. Do not claim publication/activation based on planning alone.

<a id="p01-t18"></a>

### P01-T18 — Deferred project batch CI and integration

- **Priority / status:** MUST / LOCAL_IMPLEMENTATION (2026-10-05); remote rollout PENDING.
- **Goal:** continue authorized phase construction without waiting for PR checks or merges, using exact local acceptance checkpoints and isolated dependency branches.
- **Scope:** ADR-018, AGENTS/rules/canonical cadence, Git helper/worktree state, draft job conditions and focused regression harnesses. No backend/app behavior or existing PR merge.
- **Dependencies:** existing G01-FLOW helpers; explicit maintainer cadence change. Preserve task-level immediate critical tests and G09 production gates.
- **Acceptance:** clean evidence checkpoint; stale/dirty/missing acceptance refused; next branch inherits tested head; no gh/CI/merge calls between phases; draft/missing/failed/skipped/cancelled checks block finalization; non-draft ready PR and main push still run checks.
- **Evidence:** [flow validation](docs/planning/PROJECT_BATCH_FLOW_VALIDATION.md). Local changes do not prove GitHub rollout or close a remote issue.


## P02 — Official catalog and ANP ingestion

Priority: **MUST**. Entry: G01 + G01-FLOW.

Exit gate: **G02: canonical station identity, precise units/CNPJ, idempotent revisioned import, quarantine/reporting, permitted geolocation path and public official reads validated; no Android modifications.**

<a id="p02-t01"></a>

### P02-T01 — Shared ANP fixtures

- **ID / priority / status:** P02-T01 / MUST / NOT STARTED.
- **Goal:** Capture versioned source normalization cases.
- **Why:** Prevent parser divergence.
- **Inputs:** docs/backend/ANP_INGESTION.md, docs/backend/DATA_MODEL.md; CURRENT_STATE_AUDIT A03-A07.
- **Files/areas expected:** `contracts/testdata/anp; backend/testdata` (area list, not a literal combined path).
- **Dependencies:** G01 + G01-FLOW; all earlier phase gates.
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

## P09 — Backend local integration and deferred real-production release

Priority: **MUST**. Entry: G08 for local corrections; G18 for real-production certification.

Exit gates: **G09-LOCAL** integrates locally validated corrections with required CI and permits app work. **G09 RELEASE** remains DEFERRED_UNTIL_APP_FUNCTIONAL; after G18 it requires full real-production security/restore/privacy/load evidence. See ADR-014.

<a id="p09-t01"></a>

### P09-T01 — End-to-end backend release rehearsal

- **ID / priority / status:** P09-T01 / MUST / LOCAL_REHEARSAL_DONE; production candidate rerun deferred until G18.
- **Goal:** Exercise all MVP flows against deployed candidate.
- **Why:** Prove modules work together beyond isolated tests.
- **Inputs:** docs/product/PRODUCT_CONTRACT.md, docs/backend/TEST_STRATEGY.md; BUC-001…008; frozen OpenAPI.
- **Files/areas expected:** `backend/testdata/e2e; docs/release-evidence` (area list, not a literal combined path).
- **Dependencies:** G08; all earlier phase gates.
- **Tests first:** Full register/upload/observe/confirm/dispute/moderate/export/erase flow plus denied paths.
- **Implementation outline:** Select an immutable candidate after P01–P08 merges; run the full release matrix once on it with synthetic clients and inspect source separation and durable events/jobs.
- **Acceptance criteria:** All critical flows pass deployed topology.
- **Validation commands:** `Run documented deployed E2E and full backend gate`. Where prose names a gate, introduce/document the exact executable command in this task before claiming completion.
- **Risks:** Happy-path-only evidence or accidental real personal-data use.
- **Rollback/Recovery:** Rollback candidate and preserve synthetic failure evidence.
- **Definition of done:** DOD-1 plus this task's acceptance/validation evidence; update status/progress without claiming the next phase is complete.

<a id="p09-t02"></a>

### P09-T02 — Compatibility and documentation closure

- **ID / priority / status:** P09-T02 / MUST / PRIOR_COMPATIBILITY_RECORDED; final candidate reconciliation deferred until G18.
- **Goal:** Reconcile contract, baseline and runbooks.
- **Why:** Avoid passing the gate with unresolved inherited assumptions.
- **Inputs:** docs/product/PRODUCT_CONTRACT.md, docs/backend/TEST_STRATEGY.md; CURRENT_STATE_AUDIT; BASELINE_VALIDATION; MIGRATION_PLAN.
- **Files/areas expected:** `contracts; docs/planning; release evidence` (area list, not a literal combined path).
- **Dependencies:** P09-T01; all earlier phase gates.
- **Tests first:** Review every A01-A13 resolution/defer rationale and fixture expectation.
- **Implementation outline:** Use the same candidate matrix to include the existing Android regression baseline; freeze known legacy adapter deltas; verify links/runbooks without re-running unchanged aggregate evidence.
- **Acceptance criteria:** No unspecified breaking semantic; baseline environment blockers resolved.
- **Validation commands:** `Run existing Android unit/lint/build gates and contract validation`. Where prose names a gate, introduce/document the exact executable command in this task before claiming completion.
- **Risks:** Treating planned Kotlin-Go harness as already passing.
- **Rollback/Recovery:** Record blocked gate; no Android feature edits until prerequisites/evidence resolved.
- **Definition of done:** DOD-1 plus this task's acceptance/validation evidence; update status/progress without claiming the next phase is complete.

<a id="p09-t03"></a>

### P09-T03 — Backend readiness sign-off

- **ID / priority / status:** P09-T03 / MUST / DEFERRED_UNTIL_APP_FUNCTIONAL (RELEASE).
- **Goal:** Certify the immutable real-production candidate after functional Android/iOS acceptance.
- **Why:** Enforce the user-requested ordering.
- **Inputs:** docs/product/PRODUCT_CONTRACT.md, docs/backend/TEST_STRATEGY.md; G01…G08 evidence; D01…D12 decisions.
- **Files/areas expected:** `docs/release-evidence/G09.md; docs/planning/PROGRESS.md; ROADMAP.md status only` (area list, not a literal combined path).
- **Dependencies:** P09-T02, G18 and all integrated backend extension gates; ADR-014.
- **Tests first:** Checklist rejects missing restore/security/privacy/load evidence or open launch blockers.
- **Implementation outline:** Record commit/image/schema/policy/environment, operator readiness and rollback; mark gate only with proof.
- **Acceptance criteria:** RELEASE_CERTIFIED only with complete actual production evidence after G18; otherwise deferred/blocked. This does not gate local app work after G09-LOCAL.
- **Validation commands:** `Review all release evidence and smoke actual release endpoints`. Where prose names a gate, introduce/document the exact executable command in this task before claiming completion.
- **Risks:** Checkbox-only completion or scope creep into paid products.
- **Rollback/Recovery:** Withdraw production approval if material regression; preserve local app work and block public launch.
- **Definition of done:** DOD-1 plus this task's acceptance/validation evidence; update status/progress without claiming the next phase is complete.


<a id="p09-t01a"></a>

### P09-T01A — Integrate local runtime corrections

- **ID / priority / status:** P09-T01A / MUST / LOCAL_DONE (8c5e795; awaiting phase merge).
- **Goal:** Integrate correction commit 8c5e795 and its local runtime evidence
- **Why:** Deliver the bounded functional requirement with explicit domain and native boundaries.
- **Inputs:** docs/planning/MOBILE_DELIVERY_PLAN.md; ADR-014/ADR-015; docs/release-evidence/p09-local-runtime-validation.md.
- **Files/areas expected:** backend; infra; scripts; docs/release-evidence.
- **Dependencies:** merged P09 rehearsal baseline c51fa03.
- **Tests first:** Reuse recorded changed-source negative/concurrency/PostGIS/E2E evidence; current-head quick integration.
- **Implementation outline:** Preserve implementation history; publish issue #11 and the phase PR.
- **Acceptance criteria:** Protected merge includes all corrections; LOCAL_DONE becomes INTEGRATED; no production certification.
- **Validation commands:** Run the scoped domain/consumer, native compilation/device or real-PostGIS/race suites named by this task; document the exact executable commands and required result manifest before implementation acceptance. For existing Android baseline use `./gradlew :domain:test :application:test :data:testDebugUnitTest :app:testDebugUnitTest :app:assembleDebug --no-daemon`; backend package commands follow backend/README.md and docs/backend/TEST_STRATEGY.md. No invented script is claimed available.
- **Risks:** critical; missing native/provider evidence or unsafe fallback blocks acceptance.
- **Rollback/Recovery:** Disable the new feature with a tested flag, preserve existing free/offline behavior and compatible API; append-only migrations, no destructive history rewrite.
- **Definition of done:** DOD-1, actual scoped evidence and updated PROGRESS; phase integration requires specialized exit plus current-head/base Quick verification; no release claim from a plan.

<a id="p09-t04"></a>

### P09-T04 — Reclassify release and plan functional KMP delivery

- **ID / priority / status:** P09-T04 / MUST / PLANNED (NOT IMPLEMENTED).
- **Goal:** Record user-confirmed next phases and deferred real-production gate
- **Why:** Deliver the bounded functional requirement with explicit domain and native boundaries.
- **Inputs:** docs/planning/MOBILE_DELIVERY_PLAN.md; ADR-014/ADR-015; docs/adr/014-functional-app-before-production-release.md; docs/mobile/KOTLIN_MULTIPLATFORM_AUDIT.md.
- **Files/areas expected:** ROADMAP; AGENTS; planning/product/security/mobile docs; G09 record validator.
- **Dependencies:** P09 local evidence and user decision 2026-09-30.
- **Tests first:** Deferred record accepted; unsupported certification, missing app prerequisite/legal/integration refused; links/dependencies checked.
- **Implementation outline:** Document B-BR/BUC, KMP version findings, free accounts and phased acceptance; reconcile old policy.
- **Acceptance criteria:** 280-character comments/replies, three free signup methods, 24-hour photo target and Swift parity have owning tasks; no claimed implementation.
- **Validation commands:** Run the scoped domain/consumer, native compilation/device or real-PostGIS/race suites named by this task; document the exact executable commands and required result manifest before implementation acceptance. For existing Android baseline use `./gradlew :domain:test :application:test :data:testDebugUnitTest :app:testDebugUnitTest :app:assembleDebug --no-daemon`; backend package commands follow backend/README.md and docs/backend/TEST_STRATEGY.md. No invented script is claimed available.
- **Risks:** docs plus executable release-record guard; missing native/provider evidence or unsafe fallback blocks acceptance.
- **Rollback/Recovery:** Disable the new feature with a tested flag, preserve existing free/offline behavior and compatible API; append-only migrations, no destructive history rewrite.
- **Definition of done:** DOD-1, actual scoped evidence and updated PROGRESS; phase integration requires specialized exit plus current-head/base Quick verification; no release claim from a plan.

<a id="p09-t05"></a>

### P09-T05 — Wiki snapshot overview without manual-page overwrite

- **ID / priority / status:** P09-T05 / MUST / LOCAL_DONE (issue #15; awaiting merge/wiki publication).
- **Goal:** Publish the canonical merged snapshot while preserving the unmanaged wiki Home and P01 report.
- **Why:** First mirror initialization currently refuses a Home.md collision.
- **Inputs:** docs/planning/DELIVERY_WORKFLOW.md; committed docs/planning/wiki-config.json; existing wiki ownership manifest protocol.
- **Files/areas expected:** scripts/wiki.sh; scripts/tests/test-wiki.sh; bounded snapshot configuration and phase evidence.
- **Dependencies:** P09-T04; protected phase merge before actual wiki publication.
- **Tests first:** Separate overview, preserved manual Home bytes/unowned manifest, rewritten links/sidebar, invalid path refusal and source-SHA-pinned config; existing dry-run/conflict/deletion/no-op tests.
- **Implementation outline:** Read a validated overview-page name from the committed snapshot; generate Project-Overview.md, never adopt/overwrite unmanaged Home.md.
- **Acceptance criteria:** Scoped harness passes; one separately verified merged-SHA publication preserves both manual pages; any conflict remains WIKI_PENDING.
- **Validation commands:** `bash -n scripts/wiki.sh scripts/tests/test-wiki.sh; bash scripts/tests/test-wiki.sh`; committed snapshot dry-run, then authorized merged-SHA publish only once.
- **Risks:** Ownership/path traversal or using working-tree configuration could overwrite manual pages.
- **Rollback/Recovery:** Refuse unsafe export without wiki mutations; retain prior wiki commit and retry documentation only.
- **Definition of done:** Scoped acceptance, required current-head/base CI and guarded phase merge; wiki outcome recorded separately in PR metadata.

## P10 — Functional app integration after G09-LOCAL

Priority: **MUST**. Entry: G09-LOCAL and G12–G16 for affected consumers. P10-T09 alone additionally requires real G09 release.

Exit gate: **G10-LOCAL: cross-language contracts, local migration, identity, contribution/OCR, source/freshness and offline/outage tests; existing app preserved. Public pilot P10-T09 waits for G18 and G09.**

<a id="p10-t01"></a>

### P10-T01 — Kotlin contract harness and compatibility adapters

- **ID / priority / status:** P10-T01 / MUST / NOT STARTED.
- **Goal:** Consume shared fixtures in the existing architecture.
- **Why:** Resolve legacy fuel/CNPJ/precision differences explicitly.
- **Inputs:** docs/MIGRATION_PLAN.md, docs/user-business-logic.md; G09-LOCAL evidence; ANP_INGESTION fixture manifest.
- **Files/areas expected:** `domain tests; data adapters/tests; contracts` (area list, not a literal combined path).
- **Dependencies:** G09-LOCAL required before ANY task in this phase; all earlier phase gates.
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
- **Dependencies:** P10-T08, G24-ANDROID-COMMERCIAL and RELEASE_CERTIFIED scoped real G09; all applicable phase gates.
- **Tests first:** Rehearse flag off/rollback; verify aggregate metrics without tracking.
- **Implementation outline:** Agree cohort/capacity/window, monitor freshness/disputes/cost and review outcomes.
- **Acceptance criteria:** Pilot report states evidence, limits and next product decisions.
- **Validation commands:** `Run release smoke and compare monitored pilot metrics to acceptance targets`. Where prose names a gate, introduce/document the exact executable command in this task before claiming completion.
- **Risks:** Premature broad release or unsupported confidence claims.
- **Rollback/Recovery:** Pause onboarding/contribution and keep ANP browsing available.
- **Definition of done:** DOD-1 plus this task's acceptance/validation evidence; update status/progress without claiming the next phase is complete.

## P11 — Optional hosted commercial services

Priority: **LATER**. Entry: Completed P10 pilot and explicit validated product scope.

Exit gate: **G11: optional paid benefit/billing/sync specifications and implementations pass provider/ownership/conflict tests; no effect on free trust or existing local data. Each task is independently respecified before execution.**

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

### P11-T02 — Account linking moved to free P13

- **ID / priority / status:** P11-T02 / MUST / SUPERSEDED by P13-T01…T05 (not implemented).
- **Goal:** Preserve this historical ID; implement free recoverable account linking in P13, without payment. Do not create a duplicate P11 implementation issue.
- **Why:** Offer device migration with proof.
- **Inputs:** docs/product/OPEN_SOURCE_BUSINESS.md, docs/MIGRATION_PLAN.md; New account UC/security ADR required.
- **Files/areas expected:** `identity/account module and adapters; contracts` (area list, not a literal combined path).
- **Dependencies:** Superseded by P13; no P11 billing or paid offering prerequisite for free accounts.
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
- **Dependencies:** G13 and P11-T01; all applicable release gates.
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


## P12 — Kotlin Multiplatform foundation

Priority: **MUST**. Entry: G09-LOCAL.

Exit gate: **G12: supported pins, Android regression parity, portable domain vectors and shared framework/Swift shell built on macOS.**

<a id="p12-t01"></a>

### P12-T01 — Toolchain and feature baseline

- **ID / priority / status:** P12-T01 / MUST / PLANNED (NOT IMPLEMENTED).
- **Goal:** Freeze compatible Kotlin/AGP/Gradle/KSP/Compose/Xcode pins and supported devices
- **Why:** Deliver the bounded functional requirement with explicit domain and native boundaries.
- **Inputs:** docs/planning/MOBILE_DELIVERY_PLAN.md; ADR-014/ADR-015; docs/product/STATION_FUEL_FEEDBACK.md; docs/security/FREE_ACCOUNT_ACCESS.md; docs/security/LOCAL_MEDIA_LOCATION_POLICY.md.
- **Files/areas expected:** gradle; module build files; docs/mobile.
- **Dependencies:** G09-LOCAL.
- **Tests first:** Inherited feature/test matrix; trial upgrade/plugin compatibility and rollback builds.
- **Implementation outline:** Audit official matrix/dependency need/license/security; establish startup/heap/photo baseline.
- **Acceptance criteria:** Exact pins and environment results recorded; no unsupported iOS readiness claim.
- **Validation commands:** Run the scoped domain/consumer, native compilation/device or real-PostGIS/race suites named by this task; document the exact executable commands and required result manifest before implementation acceptance. For existing Android baseline use `./gradlew :domain:test :application:test :data:testDebugUnitTest :app:testDebugUnitTest :app:assembleDebug --no-daemon`; backend package commands follow backend/README.md and docs/backend/TEST_STRATEGY.md. No invented script is claimed available.
- **Risks:** critical; missing native/provider evidence or unsafe fallback blocks acceptance.
- **Rollback/Recovery:** Disable the new feature with a tested flag, preserve existing free/offline behavior and compatible API; append-only migrations, no destructive history rewrite.
- **Definition of done:** DOD-1, actual scoped evidence and updated PROGRESS; phase integration requires specialized exit plus current-head/base Quick verification; no release claim from a plan.

<a id="p12-t02"></a>

### P12-T02 — Portable domain and money contracts

- **ID / priority / status:** P12-T02 / MUST / PLANNED (NOT IMPLEMENTED).
- **Goal:** Share pure domain logic without Java APIs or money drift
- **Why:** Deliver the bounded functional requirement with explicit domain and native boundaries.
- **Inputs:** docs/planning/MOBILE_DELIVERY_PLAN.md; ADR-014/ADR-015; docs/product/STATION_FUEL_FEEDBACK.md; docs/security/FREE_ACCOUNT_ACCESS.md; docs/security/LOCAL_MEDIA_LOCATION_POLICY.md.
- **Files/areas expected:** domain commonMain/jvm/android/ios adapters; shared fixtures.
- **Dependencies:** P12-T01.
- **Tests first:** Exact money/overflow/time/ID/Unicode golden parity; existing calculators.
- **Implementation outline:** Introduce portable types/ports incrementally; preserve com.anpfuel and MIT.
- **Acceptance criteria:** Common tests and Android consumers agree on all golden vectors.
- **Validation commands:** Run the scoped domain/consumer, native compilation/device or real-PostGIS/race suites named by this task; document the exact executable commands and required result manifest before implementation acceptance. For existing Android baseline use `./gradlew :domain:test :application:test :data:testDebugUnitTest :app:testDebugUnitTest :app:assembleDebug --no-daemon`; backend package commands follow backend/README.md and docs/backend/TEST_STRATEGY.md. No invented script is claimed available.
- **Risks:** critical; missing native/provider evidence or unsafe fallback blocks acceptance.
- **Rollback/Recovery:** Disable the new feature with a tested flag, preserve existing free/offline behavior and compatible API; append-only migrations, no destructive history rewrite.
- **Definition of done:** DOD-1, actual scoped evidence and updated PROGRESS; phase integration requires specialized exit plus current-head/base Quick verification; no release claim from a plan.

<a id="p12-t03"></a>

### P12-T03 — Application ports and offline state

- **ID / priority / status:** P12-T03 / MUST / PLANNED (NOT IMPLEMENTED).
- **Goal:** Share use cases while keeping native persistence/background libraries at the boundary
- **Why:** Deliver the bounded functional requirement with explicit domain and native boundaries.
- **Inputs:** docs/planning/MOBILE_DELIVERY_PLAN.md; ADR-014/ADR-015; docs/product/STATION_FUEL_FEEDBACK.md; docs/security/FREE_ACCOUNT_ACCESS.md; docs/security/LOCAL_MEDIA_LOCATION_POLICY.md.
- **Files/areas expected:** application; data native adapters.
- **Dependencies:** P12-T02.
- **Tests first:** Cancellation, retry, outbox revisions, offline reads and migration compatibility.
- **Implementation outline:** Explicit dependency injection and common use-case state; Room remains Android adapter.
- **Acceptance criteria:** No Android dependency in pure common code; imported offline behavior retained.
- **Validation commands:** Run the scoped domain/consumer, native compilation/device or real-PostGIS/race suites named by this task; document the exact executable commands and required result manifest before implementation acceptance. For existing Android baseline use `./gradlew :domain:test :application:test :data:testDebugUnitTest :app:testDebugUnitTest :app:assembleDebug --no-daemon`; backend package commands follow backend/README.md and docs/backend/TEST_STRATEGY.md. No invented script is claimed available.
- **Risks:** critical; missing native/provider evidence or unsafe fallback blocks acceptance.
- **Rollback/Recovery:** Disable the new feature with a tested flag, preserve existing free/offline behavior and compatible API; append-only migrations, no destructive history rewrite.
- **Definition of done:** DOD-1, actual scoped evidence and updated PROGRESS; phase integration requires specialized exit plus current-head/base Quick verification; no release claim from a plan.

<a id="p12-t04"></a>

### P12-T04 — Swift framework and native shell

- **ID / priority / status:** P12-T04 / MUST / PLANNED (NOT IMPLEMENTED).
- **Goal:** Produce a thin working iPhone host for shared use cases
- **Why:** Deliver the bounded functional requirement with explicit domain and native boundaries.
- **Inputs:** docs/planning/MOBILE_DELIVERY_PLAN.md; ADR-014/ADR-015; docs/product/STATION_FUEL_FEEDBACK.md; docs/security/FREE_ACCOUNT_ACCESS.md; docs/security/LOCAL_MEDIA_LOCATION_POLICY.md.
- **Files/areas expected:** shared umbrella framework; iosApp Swift/SwiftUI; build manifests.
- **Dependencies:** P12-T03.
- **Tests first:** macOS simulator/device build, errors/cancellation/lifecycle and interop types.
- **Implementation outline:** Supported framework/Objective-C interop plus Swift wrappers; native ports wired explicitly.
- **Acceptance criteria:** iosArm64/iosSimulatorArm64 compile and shell executes shared fixture use case.
- **Validation commands:** Run the scoped domain/consumer, native compilation/device or real-PostGIS/race suites named by this task; document the exact executable commands and required result manifest before implementation acceptance. For existing Android baseline use `./gradlew :domain:test :application:test :data:testDebugUnitTest :app:testDebugUnitTest :app:assembleDebug --no-daemon`; backend package commands follow backend/README.md and docs/backend/TEST_STRATEGY.md. No invented script is claimed available.
- **Risks:** critical; missing native/provider evidence or unsafe fallback blocks acceptance.
- **Rollback/Recovery:** Disable the new feature with a tested flag, preserve existing free/offline behavior and compatible API; append-only migrations, no destructive history rewrite.
- **Definition of done:** DOD-1, actual scoped evidence and updated PROGRESS; phase integration requires specialized exit plus current-head/base Quick verification; no release claim from a plan.

<a id="p12-t05"></a>

### P12-T05 — Foundation acceptance and gates

- **ID / priority / status:** P12-T05 / MUST / PLANNED (NOT IMPLEMENTED).
- **Goal:** Prove toolchain migration preserves existing functionality
- **Why:** Deliver the bounded functional requirement with explicit domain and native boundaries.
- **Inputs:** docs/planning/MOBILE_DELIVERY_PLAN.md; ADR-014/ADR-015; docs/product/STATION_FUEL_FEEDBACK.md; docs/security/FREE_ACCOUNT_ACCESS.md; docs/security/LOCAL_MEDIA_LOCATION_POLICY.md.
- **Files/areas expected:** docs/mobile; scoped platform CI; regression fixtures.
- **Dependencies:** P12-T04.
- **Tests first:** Android baseline and macOS native checks; dependency/security drift.
- **Implementation outline:** Introduce bounded KMP/iOS phase check selection without disabling Quick verification.
- **Acceptance criteria:** G12 evidence on actual artifacts and device matrix; no visual redesign.
- **Validation commands:** Run the scoped domain/consumer, native compilation/device or real-PostGIS/race suites named by this task; document the exact executable commands and required result manifest before implementation acceptance. For existing Android baseline use `./gradlew :domain:test :application:test :data:testDebugUnitTest :app:testDebugUnitTest :app:assembleDebug --no-daemon`; backend package commands follow backend/README.md and docs/backend/TEST_STRATEGY.md. No invented script is claimed available.
- **Risks:** critical; missing native/provider evidence or unsafe fallback blocks acceptance.
- **Rollback/Recovery:** Disable the new feature with a tested flag, preserve existing free/offline behavior and compatible API; append-only migrations, no destructive history rewrite.
- **Definition of done:** DOD-1, actual scoped evidence and updated PROGRESS; phase integration requires specialized exit plus current-head/base Quick verification; no release claim from a plan.

## P13 — Free accounts and recovery

Priority: **MUST**. Entry: G12.

Exit gate: **G13: free signup, secure provider/device binding, recovery, rights and native login evidence.**

<a id="p13-t01"></a>

### P13-T01 — Account rules and additive contracts

- **ID / priority / status:** P13-T01 / MUST / PLANNED (NOT IMPLEMENTED).
- **Goal:** Freeze FREE account/session/provider/recovery semantics
- **Why:** Deliver the bounded functional requirement with explicit domain and native boundaries.
- **Inputs:** docs/planning/MOBILE_DELIVERY_PLAN.md; ADR-014/ADR-015; docs/product/STATION_FUEL_FEEDBACK.md; docs/security/FREE_ACCOUNT_ACCESS.md; docs/security/LOCAL_MEDIA_LOCATION_POLICY.md.
- **Files/areas expected:** contracts; identity/account ports; docs/security.
- **Dependencies:** G12.
- **Tests first:** OTP and OIDC attack fixtures; enumeration; account-link takeover cases.
- **Implementation outline:** Document B-BR-A/BUC-A; OTP limits, auth scope, inventory/retention and compatible versioning.
- **Acceptance criteria:** No purchase requirement; exact subject/session/device proof boundary.
- **Validation commands:** Run the scoped domain/consumer, native compilation/device or real-PostGIS/race suites named by this task; document the exact executable commands and required result manifest before implementation acceptance. For existing Android baseline use `./gradlew :domain:test :application:test :data:testDebugUnitTest :app:testDebugUnitTest :app:assembleDebug --no-daemon`; backend package commands follow backend/README.md and docs/backend/TEST_STRATEGY.md. No invented script is claimed available.
- **Risks:** critical; missing native/provider evidence or unsafe fallback blocks acceptance.
- **Rollback/Recovery:** Disable the new feature with a tested flag, preserve existing free/offline behavior and compatible API; append-only migrations, no destructive history rewrite.
- **Definition of done:** DOD-1, actual scoped evidence and updated PROGRESS; phase integration requires specialized exit plus current-head/base Quick verification; no release claim from a plan.

<a id="p13-t02"></a>

### P13-T02 — Email access-code backend

- **ID / priority / status:** P13-T02 / MUST / PLANNED (NOT IMPLEMENTED).
- **Goal:** Implement free code enrollment and sessions
- **Why:** Deliver the bounded functional requirement with explicit domain and native boundaries.
- **Inputs:** docs/planning/MOBILE_DELIVERY_PLAN.md; ADR-014/ADR-015; docs/product/STATION_FUEL_FEEDBACK.md; docs/security/FREE_ACCOUNT_ACCESS.md; docs/security/LOCAL_MEDIA_LOCATION_POLICY.md.
- **Files/areas expected:** backend account domain/application/transport/mail adapter; append-only migrations.
- **Dependencies:** P13-T01.
- **Tests first:** Expired/replayed/concurrent code consume, resend quotas, enumeration, refresh reuse; real DB.
- **Implementation outline:** Domain RED/GREEN; hash codes, bounded attempts and revocable session families.
- **Acceptance criteria:** No plaintext code/log leak; FREE flow independent from billing.
- **Validation commands:** Run the scoped domain/consumer, native compilation/device or real-PostGIS/race suites named by this task; document the exact executable commands and required result manifest before implementation acceptance. For existing Android baseline use `./gradlew :domain:test :application:test :data:testDebugUnitTest :app:testDebugUnitTest :app:assembleDebug --no-daemon`; backend package commands follow backend/README.md and docs/backend/TEST_STRATEGY.md. No invented script is claimed available.
- **Risks:** critical; missing native/provider evidence or unsafe fallback blocks acceptance.
- **Rollback/Recovery:** Disable the new feature with a tested flag, preserve existing free/offline behavior and compatible API; append-only migrations, no destructive history rewrite.
- **Definition of done:** DOD-1, actual scoped evidence and updated PROGRESS; phase integration requires specialized exit plus current-head/base Quick verification; no release claim from a plan.

<a id="p13-t03"></a>

### P13-T03 — Google and Apple verification

- **ID / priority / status:** P13-T03 / MUST / PLANNED (NOT IMPLEMENTED).
- **Goal:** Verify external identity and secure link/unlink
- **Why:** Deliver the bounded functional requirement with explicit domain and native boundaries.
- **Inputs:** docs/planning/MOBILE_DELIVERY_PLAN.md; ADR-014/ADR-015; docs/product/STATION_FUEL_FEEDBACK.md; docs/security/FREE_ACCOUNT_ACCESS.md; docs/security/LOCAL_MEDIA_LOCATION_POLICY.md.
- **Files/areas expected:** backend OIDC adapters; contracts; provider sandbox evidence.
- **Dependencies:** P13-T02.
- **Tests first:** Wrong issuer/audience/nonce/key, replay/JWKS rotation, Apple relay and provider outage.
- **Implementation outline:** Use reviewed provider protocol adapter and verified subject binding, never email-only merge.
- **Acceptance criteria:** Both providers work in sandbox; compromised token cannot link another account.
- **Validation commands:** Run the scoped domain/consumer, native compilation/device or real-PostGIS/race suites named by this task; document the exact executable commands and required result manifest before implementation acceptance. For existing Android baseline use `./gradlew :domain:test :application:test :data:testDebugUnitTest :app:testDebugUnitTest :app:assembleDebug --no-daemon`; backend package commands follow backend/README.md and docs/backend/TEST_STRATEGY.md. No invented script is claimed available.
- **Risks:** critical; missing native/provider evidence or unsafe fallback blocks acceptance.
- **Rollback/Recovery:** Disable the new feature with a tested flag, preserve existing free/offline behavior and compatible API; append-only migrations, no destructive history rewrite.
- **Definition of done:** DOD-1, actual scoped evidence and updated PROGRESS; phase integration requires specialized exit plus current-head/base Quick verification; no release claim from a plan.

<a id="p13-t04"></a>

### P13-T04 — Recovery revocation and privacy

- **ID / priority / status:** P13-T04 / MUST / PLANNED (NOT IMPLEMENTED).
- **Goal:** Preserve contributor ownership through account/device changes
- **Why:** Deliver the bounded functional requirement with explicit domain and native boundaries.
- **Inputs:** docs/planning/MOBILE_DELIVERY_PLAN.md; ADR-014/ADR-015; docs/product/STATION_FUEL_FEEDBACK.md; docs/security/FREE_ACCOUNT_ACCESS.md; docs/security/LOCAL_MEDIA_LOCATION_POLICY.md.
- **Files/areas expected:** backend identity/privacy; native secure-storage ports.
- **Dependencies:** P13-T03.
- **Tests first:** Stolen contributor ID, fresh reauth, old-key/session revoke race and deletion/restore.
- **Implementation outline:** Bind current key plus account proof; recovery audits; minimized data inventory.
- **Acceptance criteria:** Recovery cannot steal reputation; suspension/erasure immediately blocks social writes.
- **Validation commands:** Run the scoped domain/consumer, native compilation/device or real-PostGIS/race suites named by this task; document the exact executable commands and required result manifest before implementation acceptance. For existing Android baseline use `./gradlew :domain:test :application:test :data:testDebugUnitTest :app:testDebugUnitTest :app:assembleDebug --no-daemon`; backend package commands follow backend/README.md and docs/backend/TEST_STRATEGY.md. No invented script is claimed available.
- **Risks:** critical; missing native/provider evidence or unsafe fallback blocks acceptance.
- **Rollback/Recovery:** Disable the new feature with a tested flag, preserve existing free/offline behavior and compatible API; append-only migrations, no destructive history rewrite.
- **Definition of done:** DOD-1, actual scoped evidence and updated PROGRESS; phase integration requires specialized exit plus current-head/base Quick verification; no release claim from a plan.

<a id="p13-t05"></a>

### P13-T05 — Shared and native login integration

- **ID / priority / status:** P13-T05 / MUST / PLANNED (NOT IMPLEMENTED).
- **Goal:** Make all three free signup methods functional on Android and iPhone
- **Why:** Deliver the bounded functional requirement with explicit domain and native boundaries.
- **Inputs:** docs/planning/MOBILE_DELIVERY_PLAN.md; ADR-014/ADR-015; docs/product/STATION_FUEL_FEEDBACK.md; docs/security/FREE_ACCOUNT_ACCESS.md; docs/security/LOCAL_MEDIA_LOCATION_POLICY.md.
- **Files/areas expected:** application auth use cases; app/iosApp; Keystore/Keychain adapters.
- **Dependencies:** P13-T04, G12.
- **Tests first:** Callback cancel/replay, deep-link substitution, process death, expired sessions and secure storage.
- **Implementation outline:** Native supported provider/browser flows plus shared auth state; minimal current-style screens.
- **Acceptance criteria:** G13 device evidence; signup has no payment wall; free browsing works logged out.
- **Validation commands:** Run the scoped domain/consumer, native compilation/device or real-PostGIS/race suites named by this task; document the exact executable commands and required result manifest before implementation acceptance. For existing Android baseline use `./gradlew :domain:test :application:test :data:testDebugUnitTest :app:testDebugUnitTest :app:assembleDebug --no-daemon`; backend package commands follow backend/README.md and docs/backend/TEST_STRATEGY.md. No invented script is claimed available.
- **Risks:** critical; missing native/provider evidence or unsafe fallback blocks acceptance.
- **Rollback/Recovery:** Disable the new feature with a tested flag, preserve existing free/offline behavior and compatible API; append-only migrations, no destructive history rewrite.
- **Definition of done:** DOD-1, actual scoped evidence and updated PROGRESS; phase integration requires specialized exit plus current-head/base Quick verification; no release claim from a plan.

## P14 — Station fuel feedback backend

Priority: **MUST**. Entry: G13.

Exit gate: **G14: secure versioned ratings/comments/replies/votes/moderation with reproducible projections.**

<a id="p14-t01"></a>

### P14-T01 — Feedback domain and contracts

- **ID / priority / status:** P14-T01 / MUST / PLANNED (NOT IMPLEMENTED).
- **Goal:** Freeze station/fuel target, 280-character text and percentage semantics
- **Why:** Deliver the bounded functional requirement with explicit domain and native boundaries.
- **Inputs:** docs/planning/MOBILE_DELIVERY_PLAN.md; ADR-014/ADR-015; docs/product/STATION_FUEL_FEEDBACK.md; docs/security/FREE_ACCOUNT_ACCESS.md; docs/security/LOCAL_MEDIA_LOCATION_POLICY.md.
- **Files/areas expected:** contracts; feedback domain spec and fixtures.
- **Dependencies:** G13.
- **Tests first:** 280/281 codepoints across Go/Kotlin/Swift; 1–5 range; zero-vote and revision vectors.
- **Implementation outline:** Document B-BR-F/BUC-F; freeze proposed reply depth and aggregate/erasure rules.
- **Acceptance criteria:** Stars, comment agreement and price confidence never share a score.
- **Validation commands:** Run the scoped domain/consumer, native compilation/device or real-PostGIS/race suites named by this task; document the exact executable commands and required result manifest before implementation acceptance. For existing Android baseline use `./gradlew :domain:test :application:test :data:testDebugUnitTest :app:testDebugUnitTest :app:assembleDebug --no-daemon`; backend package commands follow backend/README.md and docs/backend/TEST_STRATEGY.md. No invented script is claimed available.
- **Risks:** critical; missing native/provider evidence or unsafe fallback blocks acceptance.
- **Rollback/Recovery:** Disable the new feature with a tested flag, preserve existing free/offline behavior and compatible API; append-only migrations, no destructive history rewrite.
- **Definition of done:** DOD-1, actual scoped evidence and updated PROGRESS; phase integration requires specialized exit plus current-head/base Quick verification; no release claim from a plan.

<a id="p14-t02"></a>

### P14-T02 — Rating transactions and aggregates

- **ID / priority / status:** P14-T02 / MUST / PLANNED (NOT IMPLEMENTED).
- **Goal:** Implement one current rating per account/station/fuel
- **Why:** Deliver the bounded functional requirement with explicit domain and native boundaries.
- **Inputs:** docs/planning/MOBILE_DELIVERY_PLAN.md; ADR-014/ADR-015; docs/product/STATION_FUEL_FEEDBACK.md; docs/security/FREE_ACCOUNT_ACCESS.md; docs/security/LOCAL_MEDIA_LOCATION_POLICY.md.
- **Files/areas expected:** backend feedback domain/application; parametrized SQL; append-only migrations.
- **Dependencies:** P14-T01.
- **Tests first:** Duplicate/parallel rating/edit/delete, suspended session, wrong target, exact aggregate rebuild.
- **Implementation outline:** Unique constraints, idempotency and revisioned domain events; pure calculation.
- **Acceptance criteria:** Concurrent requests cannot inflate counts; public read exposes no private identity.
- **Validation commands:** Run the scoped domain/consumer, native compilation/device or real-PostGIS/race suites named by this task; document the exact executable commands and required result manifest before implementation acceptance. For existing Android baseline use `./gradlew :domain:test :application:test :data:testDebugUnitTest :app:testDebugUnitTest :app:assembleDebug --no-daemon`; backend package commands follow backend/README.md and docs/backend/TEST_STRATEGY.md. No invented script is claimed available.
- **Risks:** critical; missing native/provider evidence or unsafe fallback blocks acceptance.
- **Rollback/Recovery:** Disable the new feature with a tested flag, preserve existing free/offline behavior and compatible API; append-only migrations, no destructive history rewrite.
- **Definition of done:** DOD-1, actual scoped evidence and updated PROGRESS; phase integration requires specialized exit plus current-head/base Quick verification; no release claim from a plan.

<a id="p14-t03"></a>

### P14-T03 — Comments replies and ownership

- **ID / priority / status:** P14-T03 / MUST / PLANNED (NOT IMPLEMENTED).
- **Goal:** Implement bounded plain-text comments and replies
- **Why:** Deliver the bounded functional requirement with explicit domain and native boundaries.
- **Inputs:** docs/planning/MOBILE_DELIVERY_PLAN.md; ADR-014/ADR-015; docs/product/STATION_FUEL_FEEDBACK.md; docs/security/FREE_ACCOUNT_ACCESS.md; docs/security/LOCAL_MEDIA_LOCATION_POLICY.md.
- **Files/areas expected:** backend feedback transport/domain/storage.
- **Dependencies:** P14-T02.
- **Tests first:** Empty/281/invalid Unicode, cross-author edit/delete, stale revision, unsafe rendering and paging limits.
- **Implementation outline:** Normalized scalar-count fixtures; stable revision and reply target; signed authenticated writes.
- **Acceptance criteria:** Only active accounts write; no silent truncation; same 280 limit for replies.
- **Validation commands:** Run the scoped domain/consumer, native compilation/device or real-PostGIS/race suites named by this task; document the exact executable commands and required result manifest before implementation acceptance. For existing Android baseline use `./gradlew :domain:test :application:test :data:testDebugUnitTest :app:testDebugUnitTest :app:assembleDebug --no-daemon`; backend package commands follow backend/README.md and docs/backend/TEST_STRATEGY.md. No invented script is claimed available.
- **Risks:** critical; missing native/provider evidence or unsafe fallback blocks acceptance.
- **Rollback/Recovery:** Disable the new feature with a tested flag, preserve existing free/offline behavior and compatible API; append-only migrations, no destructive history rewrite.
- **Definition of done:** DOD-1, actual scoped evidence and updated PROGRESS; phase integration requires specialized exit plus current-head/base Quick verification; no release claim from a plan.

<a id="p14-t04"></a>

### P14-T04 — Validity votes and percentages

- **ID / priority / status:** P14-T04 / MUST / PLANNED (NOT IMPLEMENTED).
- **Goal:** Implement changeable one-account-one-revision votes
- **Why:** Deliver the bounded functional requirement with explicit domain and native boundaries.
- **Inputs:** docs/planning/MOBILE_DELIVERY_PLAN.md; ADR-014/ADR-015; docs/product/STATION_FUEL_FEEDBACK.md; docs/security/FREE_ACCOUNT_ACCESS.md; docs/security/LOCAL_MEDIA_LOCATION_POLICY.md.
- **Files/areas expected:** backend feedback vote/projection jobs; contracts.
- **Dependencies:** P14-T03.
- **Tests first:** Self-vote, duplicate/concurrent changes, revision edit, delete/suspension and job replay.
- **Implementation outline:** Use integer basis points, rebuildable V/I counts and idempotent unique transactions.
- **Acceptance criteria:** Zero votes is null; displayed denominator/percentage match shared fixtures.
- **Validation commands:** Run the scoped domain/consumer, native compilation/device or real-PostGIS/race suites named by this task; document the exact executable commands and required result manifest before implementation acceptance. For existing Android baseline use `./gradlew :domain:test :application:test :data:testDebugUnitTest :app:testDebugUnitTest :app:assembleDebug --no-daemon`; backend package commands follow backend/README.md and docs/backend/TEST_STRATEGY.md. No invented script is claimed available.
- **Risks:** critical; missing native/provider evidence or unsafe fallback blocks acceptance.
- **Rollback/Recovery:** Disable the new feature with a tested flag, preserve existing free/offline behavior and compatible API; append-only migrations, no destructive history rewrite.
- **Definition of done:** DOD-1, actual scoped evidence and updated PROGRESS; phase integration requires specialized exit plus current-head/base Quick verification; no release claim from a plan.

<a id="p14-t05"></a>

### P14-T05 — Moderation privacy and backend exit

- **ID / priority / status:** P14-T05 / MUST / PLANNED (NOT IMPLEMENTED).
- **Goal:** Complete reports/blocking/moderation and social rights
- **Why:** Deliver the bounded functional requirement with explicit domain and native boundaries.
- **Inputs:** docs/planning/MOBILE_DELIVERY_PLAN.md; ADR-014/ADR-015; docs/product/STATION_FUEL_FEEDBACK.md; docs/security/FREE_ACCOUNT_ACCESS.md; docs/security/LOCAL_MEDIA_LOCATION_POLICY.md.
- **Files/areas expected:** backend feedback/moderation/privacy; operator runbook.
- **Dependencies:** P14-T04.
- **Tests first:** Role escalation, reporter deletion attempt, hidden text leak, export/erase/restore and abuse limits.
- **Implementation outline:** Audited moderation states, minimal public reasons and projection reconciliation.
- **Acceptance criteria:** G14 real-DB/race/E2E evidence; account requirement enforced server-side.
- **Validation commands:** Run the scoped domain/consumer, native compilation/device or real-PostGIS/race suites named by this task; document the exact executable commands and required result manifest before implementation acceptance. For existing Android baseline use `./gradlew :domain:test :application:test :data:testDebugUnitTest :app:testDebugUnitTest :app:assembleDebug --no-daemon`; backend package commands follow backend/README.md and docs/backend/TEST_STRATEGY.md. No invented script is claimed available.
- **Risks:** critical; missing native/provider evidence or unsafe fallback blocks acceptance.
- **Rollback/Recovery:** Disable the new feature with a tested flag, preserve existing free/offline behavior and compatible API; append-only migrations, no destructive history rewrite.
- **Definition of done:** DOD-1, actual scoped evidence and updated PROGRESS; phase integration requires specialized exit plus current-head/base Quick verification; no release claim from a plan.

## P15 — Lightweight photos and 24-hour audit expiry

Priority: **MUST**. Entry: G14.

Exit gate: **G15: legible KiB photos, bounded memory and every app-owned media copy expires/deletes within policy.**

<a id="p15-t01"></a>

### P15-T01 — Media budgets and forward contract

- **ID / priority / status:** P15-T01 / MUST / PLANNED (NOT IMPLEMENTED).
- **Goal:** Freeze measured format/size/pixel/memory deadlines
- **Why:** Deliver the bounded functional requirement with explicit domain and native boundaries.
- **Inputs:** docs/planning/MOBILE_DELIVERY_PLAN.md; ADR-014/ADR-015; docs/product/STATION_FUEL_FEEDBACK.md; docs/security/FREE_ACCOUNT_ACCESS.md; docs/security/LOCAL_MEDIA_LOCATION_POLICY.md.
- **Files/areas expected:** contracts; media policy; benchmark fixtures.
- **Dependencies:** G12, G14.
- **Tests first:** Realistic low-resource camera/HEIF/PNG/JPEG legibility and decode-memory measurements.
- **Implementation outline:** Freeze ≤150 KiB target/≤256 KiB cap/1600 edge/2 MP hypotheses or evidence-backed limits; single deadline.
- **Acceptance criteria:** Protocol and notice target 24 h for all copies; migration and rollout compatibility documented.
- **Validation commands:** Run the scoped domain/consumer, native compilation/device or real-PostGIS/race suites named by this task; document the exact executable commands and required result manifest before implementation acceptance. For existing Android baseline use `./gradlew :domain:test :application:test :data:testDebugUnitTest :app:testDebugUnitTest :app:assembleDebug --no-daemon`; backend package commands follow backend/README.md and docs/backend/TEST_STRATEGY.md. No invented script is claimed available.
- **Risks:** critical; missing native/provider evidence or unsafe fallback blocks acceptance.
- **Rollback/Recovery:** Disable the new feature with a tested flag, preserve existing free/offline behavior and compatible API; append-only migrations, no destructive history rewrite.
- **Definition of done:** DOD-1, actual scoped evidence and updated PROGRESS; phase integration requires specialized exit plus current-head/base Quick verification; no release claim from a plan.

<a id="p15-t02"></a>

### P15-T02 — Native lightweight photo pipeline

- **ID / priority / status:** P15-T02 / MUST / PLANNED (NOT IMPLEMENTED).
- **Goal:** Optimize supported input formats locally without full-resolution allocation
- **Why:** Deliver the bounded functional requirement with explicit domain and native boundaries.
- **Inputs:** docs/planning/MOBILE_DELIVERY_PLAN.md; ADR-014/ADR-015; docs/product/STATION_FUEL_FEEDBACK.md; docs/security/FREE_ACCOUNT_ACCESS.md; docs/security/LOCAL_MEDIA_LOCATION_POLICY.md.
- **Files/areas expected:** Android camera/decoder; Swift ImageIO/camera; shared processing ports.
- **Dependencies:** P15-T01.
- **Tests first:** Large/corrupt/rotated/alpha inputs, bounded attempts, UI responsiveness, cache expiry and interrupted capture.
- **Implementation outline:** Native sample-decode/off-main encode, strip metadata; encrypted transient cache; no Gallery copy.
- **Acceptance criteria:** Legible result within frozen KiB/memory budget on baseline devices; safe unsupported-format errors.
- **Validation commands:** Run the scoped domain/consumer, native compilation/device or real-PostGIS/race suites named by this task; document the exact executable commands and required result manifest before implementation acceptance. For existing Android baseline use `./gradlew :domain:test :application:test :data:testDebugUnitTest :app:testDebugUnitTest :app:assembleDebug --no-daemon`; backend package commands follow backend/README.md and docs/backend/TEST_STRATEGY.md. No invented script is claimed available.
- **Risks:** critical; missing native/provider evidence or unsafe fallback blocks acceptance.
- **Rollback/Recovery:** Disable the new feature with a tested flag, preserve existing free/offline behavior and compatible API; append-only migrations, no destructive history rewrite.
- **Definition of done:** DOD-1, actual scoped evidence and updated PROGRESS; phase integration requires specialized exit plus current-head/base Quick verification; no release claim from a plan.

<a id="p15-t03"></a>

### P15-T03 — Bounded backend media processing

- **ID / priority / status:** P15-T03 / MUST / PLANNED (NOT IMPLEMENTED).
- **Goal:** Revalidate and sanitize with controlled RSS/concurrency
- **Why:** Deliver the bounded functional requirement with explicit domain and native boundaries.
- **Inputs:** docs/planning/MOBILE_DELIVERY_PLAN.md; ADR-014/ADR-015; docs/product/STATION_FUEL_FEEDBACK.md; docs/security/FREE_ACCOUNT_ACCESS.md; docs/security/LOCAL_MEDIA_LOCATION_POLICY.md.
- **Files/areas expected:** backend media/storage/worker; append-only migration.
- **Dependencies:** P15-T02.
- **Tests first:** Forged MIME/dimensions, decompression bombs, interrupted upload, expired finalize/retry and memory pressure.
- **Implementation outline:** Bounded streaming, header pixel limits, safe decode/reencode; one immutable expiry from first receipt.
- **Acceptance criteria:** No client-trusted budget/deadline; private media and measured worker memory bounds.
- **Validation commands:** Run the scoped domain/consumer, native compilation/device or real-PostGIS/race suites named by this task; document the exact executable commands and required result manifest before implementation acceptance. For existing Android baseline use `./gradlew :domain:test :application:test :data:testDebugUnitTest :app:testDebugUnitTest :app:assembleDebug --no-daemon`; backend package commands follow backend/README.md and docs/backend/TEST_STRATEGY.md. No invented script is claimed available.
- **Risks:** critical; missing native/provider evidence or unsafe fallback blocks acceptance.
- **Rollback/Recovery:** Disable the new feature with a tested flag, preserve existing free/offline behavior and compatible API; append-only migrations, no destructive history rewrite.
- **Definition of done:** DOD-1, actual scoped evidence and updated PROGRESS; phase integration requires specialized exit plus current-head/base Quick verification; no release claim from a plan.

<a id="p15-t04"></a>

### P15-T04 — Expiry deletion and restore enforcement

- **ID / priority / status:** P15-T04 / MUST / PLANNED (NOT IMPLEMENTED).
- **Goal:** Replace old 14/30-day retention with all-copy 24-hour enforcement
- **Why:** Deliver the bounded functional requirement with explicit domain and native boundaries.
- **Inputs:** docs/planning/MOBILE_DELIVERY_PLAN.md; ADR-014/ADR-015; docs/product/STATION_FUEL_FEEDBACK.md; docs/security/FREE_ACCOUNT_ACCESS.md; docs/security/LOCAL_MEDIA_LOCATION_POLICY.md.
- **Files/areas expected:** backend sweep/read auth; private storage adapters; backup/restore; app cache.
- **Dependencies:** P15-T03.
- **Tests first:** At-deadline read denial, delayed worker/delete failure, versioned copies, stale cache and restore replay.
- **Implementation outline:** Append-only timestamp migration; purge old media; provider policy checks, retries/alarms/intake circuit breaker.
- **Acceptance criteria:** Expiry never extends; physical overdue object is a failing policy result; no photo bytes in backups.
- **Validation commands:** Run the scoped domain/consumer, native compilation/device or real-PostGIS/race suites named by this task; document the exact executable commands and required result manifest before implementation acceptance. For existing Android baseline use `./gradlew :domain:test :application:test :data:testDebugUnitTest :app:testDebugUnitTest :app:assembleDebug --no-daemon`; backend package commands follow backend/README.md and docs/backend/TEST_STRATEGY.md. No invented script is claimed available.
- **Risks:** critical; missing native/provider evidence or unsafe fallback blocks acceptance.
- **Rollback/Recovery:** Disable the new feature with a tested flag, preserve existing free/offline behavior and compatible API; append-only migrations, no destructive history rewrite.
- **Definition of done:** DOD-1, actual scoped evidence and updated PROGRESS; phase integration requires specialized exit plus current-head/base Quick verification; no release claim from a plan.

<a id="p15-t05"></a>

### P15-T05 — Media privacy and device acceptance

- **ID / priority / status:** P15-T05 / MUST / PLANNED (NOT IMPLEMENTED).
- **Goal:** Prove the complete low-memory audit path
- **Why:** Deliver the bounded functional requirement with explicit domain and native boundaries.
- **Inputs:** docs/planning/MOBILE_DELIVERY_PLAN.md; ADR-014/ADR-015; docs/product/STATION_FUEL_FEEDBACK.md; docs/security/FREE_ACCOUNT_ACCESS.md; docs/security/LOCAL_MEDIA_LOCATION_POLICY.md.
- **Files/areas expected:** media E2E fixtures; Android/iOS device evidence; privacy notice.
- **Dependencies:** P15-T04.
- **Tests first:** Full capture/upload/sanitize/read/expiry, app restart/offline and log redaction.
- **Implementation outline:** Reconcile deleted photo versus retained fact explanations; freeze policy limitation for powered-off devices.
- **Acceptance criteria:** G15 evidence separates logical access expiry and actual deletion; target not claimed from emulator alone.
- **Validation commands:** Run the scoped domain/consumer, native compilation/device or real-PostGIS/race suites named by this task; document the exact executable commands and required result manifest before implementation acceptance. For existing Android baseline use `./gradlew :domain:test :application:test :data:testDebugUnitTest :app:testDebugUnitTest :app:assembleDebug --no-daemon`; backend package commands follow backend/README.md and docs/backend/TEST_STRATEGY.md. No invented script is claimed available.
- **Risks:** critical; missing native/provider evidence or unsafe fallback blocks acceptance.
- **Rollback/Recovery:** Disable the new feature with a tested flag, preserve existing free/offline behavior and compatible API; append-only migrations, no destructive history rewrite.
- **Definition of done:** DOD-1, actual scoped evidence and updated PROGRESS; phase integration requires specialized exit plus current-head/base Quick verification; no release claim from a plan.

## P16 — Location integrity across Android and iOS

Priority: **MUST**. Entry: G15.

Exit gate: **G16: simulated GPS blocks location-dependent claims; UNKNOWN is honest; backend distrust and native tests pass.**

<a id="p16-t01"></a>

### P16-T01 — Location risk contract

- **ID / priority / status:** P16-T01 / MUST / PLANNED (NOT IMPLEMENTED).
- **Goal:** Specify permitted denied/unknown/manual paths
- **Why:** Deliver the bounded functional requirement with explicit domain and native boundaries.
- **Inputs:** docs/planning/MOBILE_DELIVERY_PLAN.md; ADR-014/ADR-015; docs/product/STATION_FUEL_FEEDBACK.md; docs/security/FREE_ACCOUNT_ACCESS.md; docs/security/LOCAL_MEDIA_LOCATION_POLICY.md.
- **Files/areas expected:** contracts; location domain/ports; privacy fixtures.
- **Dependencies:** G15.
- **Tests first:** Mock/unknown/stale/coarse/clock/replay and fake client flag cases.
- **Implementation outline:** B-BR-L/BUC-L, risk bands/freshness and disclosure; no universal spoof-proof promise.
- **Acceptance criteria:** Unknown cannot become verified proximity; browse/offline preserved.
- **Validation commands:** Run the scoped domain/consumer, native compilation/device or real-PostGIS/race suites named by this task; document the exact executable commands and required result manifest before implementation acceptance. For existing Android baseline use `./gradlew :domain:test :application:test :data:testDebugUnitTest :app:testDebugUnitTest :app:assembleDebug --no-daemon`; backend package commands follow backend/README.md and docs/backend/TEST_STRATEGY.md. No invented script is claimed available.
- **Risks:** critical; missing native/provider evidence or unsafe fallback blocks acceptance.
- **Rollback/Recovery:** Disable the new feature with a tested flag, preserve existing free/offline behavior and compatible API; append-only migrations, no destructive history rewrite.
- **Definition of done:** DOD-1, actual scoped evidence and updated PROGRESS; phase integration requires specialized exit plus current-head/base Quick verification; no release claim from a plan.

<a id="p16-t02"></a>

### P16-T02 — Native mock and simulation adapters

- **ID / priority / status:** P16-T02 / MUST / PLANNED (NOT IMPLEMENTED).
- **Goal:** Detect platform simulation with lightweight native ports
- **Why:** Deliver the bounded functional requirement with explicit domain and native boundaries.
- **Inputs:** docs/planning/MOBILE_DELIVERY_PLAN.md; ADR-014/ADR-015; docs/product/STATION_FUEL_FEEDBACK.md; docs/security/FREE_ACCOUNT_ACCESS.md; docs/security/LOCAL_MEDIA_LOCATION_POLICY.md.
- **Files/areas expected:** Android LocationCompat; Swift Core Location; common risk use case.
- **Dependencies:** P16-T01.
- **Tests first:** Android mock provider, iOS software simulation/missing source info; release-debug exclusion.
- **Implementation outline:** OS-provided source signals; no app blacklists, busy polling or developer-option blanket denial.
- **Acceptance criteria:** Known simulation blocks location-sensitive actions on both platforms.
- **Validation commands:** Run the scoped domain/consumer, native compilation/device or real-PostGIS/race suites named by this task; document the exact executable commands and required result manifest before implementation acceptance. For existing Android baseline use `./gradlew :domain:test :application:test :data:testDebugUnitTest :app:testDebugUnitTest :app:assembleDebug --no-daemon`; backend package commands follow backend/README.md and docs/backend/TEST_STRATEGY.md. No invented script is claimed available.
- **Risks:** critical; missing native/provider evidence or unsafe fallback blocks acceptance.
- **Rollback/Recovery:** Disable the new feature with a tested flag, preserve existing free/offline behavior and compatible API; append-only migrations, no destructive history rewrite.
- **Definition of done:** DOD-1, actual scoped evidence and updated PROGRESS; phase integration requires specialized exit plus current-head/base Quick verification; no release claim from a plan.

<a id="p16-t03"></a>

### P16-T03 — Backend risk and proximity checks

- **ID / priority / status:** P16-T03 / MUST / PLANNED (NOT IMPLEMENTED).
- **Goal:** Treat device risk claims as untrusted inputs
- **Why:** Deliver the bounded functional requirement with explicit domain and native boundaries.
- **Inputs:** docs/planning/MOBILE_DELIVERY_PLAN.md; ADR-014/ADR-015; docs/product/STATION_FUEL_FEEDBACK.md; docs/security/FREE_ACCOUNT_ACCESS.md; docs/security/LOCAL_MEDIA_LOCATION_POLICY.md.
- **Files/areas expected:** backend validation/auth/privacy; additive API vectors.
- **Dependencies:** P16-T02.
- **Tests first:** Forged isMock=false, teleport/stale fixes, replay, time skew, exact-GPS/log leaks; real PostGIS.
- **Implementation outline:** Independently validate bounded proximity/freshness; persist allowed bands; optional attestation deferred decision.
- **Acceptance criteria:** Client flag alone cannot grant HIGH; no long-lived precise GPS.
- **Validation commands:** Run the scoped domain/consumer, native compilation/device or real-PostGIS/race suites named by this task; document the exact executable commands and required result manifest before implementation acceptance. For existing Android baseline use `./gradlew :domain:test :application:test :data:testDebugUnitTest :app:testDebugUnitTest :app:assembleDebug --no-daemon`; backend package commands follow backend/README.md and docs/backend/TEST_STRATEGY.md. No invented script is claimed available.
- **Risks:** critical; missing native/provider evidence or unsafe fallback blocks acceptance.
- **Rollback/Recovery:** Disable the new feature with a tested flag, preserve existing free/offline behavior and compatible API; append-only migrations, no destructive history rewrite.
- **Definition of done:** DOD-1, actual scoped evidence and updated PROGRESS; phase integration requires specialized exit plus current-head/base Quick verification; no release claim from a plan.

<a id="p16-t04"></a>

### P16-T04 — Location device and recovery acceptance

- **ID / priority / status:** P16-T04 / MUST / PLANNED (NOT IMPLEMENTED).
- **Goal:** Verify denial/degraded flows without blocking free use
- **Why:** Deliver the bounded functional requirement with explicit domain and native boundaries.
- **Inputs:** docs/planning/MOBILE_DELIVERY_PLAN.md; ADR-014/ADR-015; docs/product/STATION_FUEL_FEEDBACK.md; docs/security/FREE_ACCOUNT_ACCESS.md; docs/security/LOCAL_MEDIA_LOCATION_POLICY.md.
- **Files/areas expected:** app/iosApp location flow; release artifact assertions.
- **Dependencies:** P16-T03.
- **Tests first:** Permission revocation/coarse/offline resume/cancel plus energy/latency baseline.
- **Implementation outline:** Minimal current-style explanations and recovery; local/simulator tests plus actual supported devices.
- **Acceptance criteria:** G16 documented capabilities/limits; no unauthorized background tracking.
- **Validation commands:** Run the scoped domain/consumer, native compilation/device or real-PostGIS/race suites named by this task; document the exact executable commands and required result manifest before implementation acceptance. For existing Android baseline use `./gradlew :domain:test :application:test :data:testDebugUnitTest :app:testDebugUnitTest :app:assembleDebug --no-daemon`; backend package commands follow backend/README.md and docs/backend/TEST_STRATEGY.md. No invented script is claimed available.
- **Risks:** critical; missing native/provider evidence or unsafe fallback blocks acceptance.
- **Rollback/Recovery:** Disable the new feature with a tested flag, preserve existing free/offline behavior and compatible API; append-only migrations, no destructive history rewrite.
- **Definition of done:** DOD-1, actual scoped evidence and updated PROGRESS; phase integration requires specialized exit plus current-head/base Quick verification; no release claim from a plan.

## P17 — Social app functionality and iPhone parity

Priority: **MUST**. Entry: G10-LOCAL, G13–G16.

Exit gate: **G17: every required feature works on Android and Swift/iOS with shared fixtures and native lifecycle evidence.**

<a id="p17-t01"></a>

### P17-T01 — Shared feedback feature use cases

- **ID / priority / status:** P17-T01 / MUST / PLANNED (NOT IMPLEMENTED).
- **Goal:** Consume ratings/comments/replies/votes/moderation contracts
- **Why:** Deliver the bounded functional requirement with explicit domain and native boundaries.
- **Inputs:** docs/planning/MOBILE_DELIVERY_PLAN.md; ADR-014/ADR-015; docs/product/STATION_FUEL_FEEDBACK.md; docs/security/FREE_ACCOUNT_ACCESS.md; docs/security/LOCAL_MEDIA_LOCATION_POLICY.md.
- **Files/areas expected:** application feedback state; native data/cache adapters.
- **Dependencies:** G14, G10-LOCAL.
- **Tests first:** Contract errors, login requirement, optimistic rollback, paging, offline edits and stale revision.
- **Implementation outline:** DDD use cases and ports; bounded outbox retries with server auth/idempotency.
- **Acceptance criteria:** State and counts match backend; paid plan never blocks feedback.
- **Validation commands:** Run the scoped domain/consumer, native compilation/device or real-PostGIS/race suites named by this task; document the exact executable commands and required result manifest before implementation acceptance. For existing Android baseline use `./gradlew :domain:test :application:test :data:testDebugUnitTest :app:testDebugUnitTest :app:assembleDebug --no-daemon`; backend package commands follow backend/README.md and docs/backend/TEST_STRATEGY.md. No invented script is claimed available.
- **Risks:** critical; missing native/provider evidence or unsafe fallback blocks acceptance.
- **Rollback/Recovery:** Disable the new feature with a tested flag, preserve existing free/offline behavior and compatible API; append-only migrations, no destructive history rewrite.
- **Definition of done:** DOD-1, actual scoped evidence and updated PROGRESS; phase integration requires specialized exit plus current-head/base Quick verification; no release claim from a plan.

<a id="p17-t02"></a>

### P17-T02 — Android functional social flows

- **ID / priority / status:** P17-T02 / MUST / PLANNED (NOT IMPLEMENTED).
- **Goal:** Expose rating/comment/reply/vote/report functionality
- **Why:** Deliver the bounded functional requirement with explicit domain and native boundaries.
- **Inputs:** docs/planning/MOBILE_DELIVERY_PLAN.md; ADR-014/ADR-015; docs/product/STATION_FUEL_FEEDBACK.md; docs/security/FREE_ACCOUNT_ACCESS.md; docs/security/LOCAL_MEDIA_LOCATION_POLICY.md.
- **Files/areas expected:** app current-pattern screens/viewmodels; Android tests.
- **Dependencies:** P17-T01.
- **Tests first:** 280 limit, no-login action, revision edit, denied moderation, outage/retry.
- **Implementation outline:** Minimal existing design, source/percentage sample explanations and safe text rendering.
- **Acceptance criteria:** Each social action works end to end; no redesign or fake success states.
- **Validation commands:** Run the scoped domain/consumer, native compilation/device or real-PostGIS/race suites named by this task; document the exact executable commands and required result manifest before implementation acceptance. For existing Android baseline use `./gradlew :domain:test :application:test :data:testDebugUnitTest :app:testDebugUnitTest :app:assembleDebug --no-daemon`; backend package commands follow backend/README.md and docs/backend/TEST_STRATEGY.md. No invented script is claimed available.
- **Risks:** critical; missing native/provider evidence or unsafe fallback blocks acceptance.
- **Rollback/Recovery:** Disable the new feature with a tested flag, preserve existing free/offline behavior and compatible API; append-only migrations, no destructive history rewrite.
- **Definition of done:** DOD-1, actual scoped evidence and updated PROGRESS; phase integration requires specialized exit plus current-head/base Quick verification; no release claim from a plan.

<a id="p17-t03"></a>

### P17-T03 — Swift iPhone full feature integration

- **ID / priority / status:** P17-T03 / MUST / PLANNED (NOT IMPLEMENTED).
- **Goal:** Complete iOS ports and imported/new feature parity
- **Why:** Deliver the bounded functional requirement with explicit domain and native boundaries.
- **Inputs:** docs/planning/MOBILE_DELIVERY_PLAN.md; ADR-014/ADR-015; docs/product/STATION_FUEL_FEEDBACK.md; docs/security/FREE_ACCOUNT_ACCESS.md; docs/security/LOCAL_MEDIA_LOCATION_POLICY.md.
- **Files/areas expected:** iosApp Swift/SwiftUI; native persistence/location/camera/Keychain.
- **Dependencies:** P17-T02, G12–G16.
- **Tests first:** macOS build/simulator/device: login/camera/signature/offline/outbox/navigation/alerts; process death.
- **Implementation outline:** Thin Swift UI around shared use cases; actual native callbacks/storage/background limitations.
- **Acceptance criteria:** Feature checklist matches Android or explicit approved platform limitation; no stub marked ready.
- **Validation commands:** Run the scoped domain/consumer, native compilation/device or real-PostGIS/race suites named by this task; document the exact executable commands and required result manifest before implementation acceptance. For existing Android baseline use `./gradlew :domain:test :application:test :data:testDebugUnitTest :app:testDebugUnitTest :app:assembleDebug --no-daemon`; backend package commands follow backend/README.md and docs/backend/TEST_STRATEGY.md. No invented script is claimed available.
- **Risks:** critical; missing native/provider evidence or unsafe fallback blocks acceptance.
- **Rollback/Recovery:** Disable the new feature with a tested flag, preserve existing free/offline behavior and compatible API; append-only migrations, no destructive history rewrite.
- **Definition of done:** DOD-1, actual scoped evidence and updated PROGRESS; phase integration requires specialized exit plus current-head/base Quick verification; no release claim from a plan.

<a id="p17-t04"></a>

### P17-T04 — Cross-platform lifecycle and privacy exit

- **ID / priority / status:** P17-T04 / MUST / PLANNED (NOT IMPLEMENTED).
- **Goal:** Prove account/social/media/location flows through native boundaries
- **Why:** Deliver the bounded functional requirement with explicit domain and native boundaries.
- **Inputs:** docs/planning/MOBILE_DELIVERY_PLAN.md; ADR-014/ADR-015; docs/product/STATION_FUEL_FEEDBACK.md; docs/security/FREE_ACCOUNT_ACCESS.md; docs/security/LOCAL_MEDIA_LOCATION_POLICY.md.
- **Files/areas expected:** shared and native integration tests; docs/mobile evidence.
- **Dependencies:** P17-T03.
- **Tests first:** Same fixtures, account revocation across devices, erasure, media expiry and fake location denial.
- **Implementation outline:** Run scoped integrated local flows with synthetic server; review native dependency/licenses.
- **Acceptance criteria:** G17 supported Android/iPhone results; no email/GPS/photos in diagnostics.
- **Validation commands:** Run the scoped domain/consumer, native compilation/device or real-PostGIS/race suites named by this task; document the exact executable commands and required result manifest before implementation acceptance. For existing Android baseline use `./gradlew :domain:test :application:test :data:testDebugUnitTest :app:testDebugUnitTest :app:assembleDebug --no-daemon`; backend package commands follow backend/README.md and docs/backend/TEST_STRATEGY.md. No invented script is claimed available.
- **Risks:** critical; missing native/provider evidence or unsafe fallback blocks acceptance.
- **Rollback/Recovery:** Disable the new feature with a tested flag, preserve existing free/offline behavior and compatible API; append-only migrations, no destructive history rewrite.
- **Definition of done:** DOD-1, actual scoped evidence and updated PROGRESS; phase integration requires specialized exit plus current-head/base Quick verification; no release claim from a plan.

## P18 — Functional app acceptance and performance

Priority: **MUST**. Entry: G17.

Exit gate: **G18: integrated functional Android/iOS candidate accepted locally; real G09 release may now begin.** Historical full G18 remains NOT_ACCEPTED. Current state 2026-10-01: local validation slice INTEGRATED via PR #67 (`9c090c6`); native iOS is archived until explicit resumption. Outstanding Android/backend proof carries into P21/P24, not waived. ADR-016 replaces the both-platform prerequisite for the scoped Android commercial release only.

<a id="p18-t01"></a>

### P18-T01 — Feature matrix and final local acceptance

- **ID / priority / status:** P18-T01 / MUST / LOCAL_VALIDATION_IMPLEMENTED — LOCAL_SLICE_INTEGRATED / FULL_G18_NOT_ACCEPTED (PR #67; iOS deferred).
- **Goal:** Verify every imported and new requirement with actual result manifest
- **Why:** Deliver the bounded functional requirement with explicit domain and native boundaries.
- **Inputs:** docs/planning/MOBILE_DELIVERY_PLAN.md; ADR-014/ADR-015; docs/product/STATION_FUEL_FEEDBACK.md; docs/security/FREE_ACCOUNT_ACCESS.md; docs/security/LOCAL_MEDIA_LOCATION_POLICY.md.
- **Files/areas expected:** docs/mobile acceptance; local backend fixtures; device E2E.
- **Dependencies:** G17, G10-LOCAL.
- **Tests first:** All baseline feature mappings and failure/offline/recovery scenarios; no silent skips.
- **Implementation outline:** Freeze expected Android/iOS/account/feedback/photo/location matrix and candidate artifacts.
- **Acceptance criteria:** Every required feature has actual device/local-service proof; any missing result blocks G18.
- **Validation commands:** Run the scoped domain/consumer, native compilation/device or real-PostGIS/race suites named by this task; document the exact executable commands and required result manifest before implementation acceptance. For existing Android baseline use `./gradlew :domain:test :application:test :data:testDebugUnitTest :app:testDebugUnitTest :app:assembleDebug --no-daemon`; backend package commands follow backend/README.md and docs/backend/TEST_STRATEGY.md. No invented script is claimed available.
- **Risks:** critical; missing native/provider evidence or unsafe fallback blocks acceptance.
- **Rollback/Recovery:** Disable the new feature with a tested flag, preserve existing free/offline behavior and compatible API; append-only migrations, no destructive history rewrite.
- **Definition of done:** DOD-1, actual scoped evidence and updated PROGRESS; phase integration requires specialized exit plus current-head/base Quick verification; no release claim from a plan.

<a id="p18-t02"></a>

### P18-T02 — Performance and memory tuning

- **ID / priority / status:** P18-T02 / MUST / LOCAL_VALIDATION_IMPLEMENTED — LOCAL_SLICE_INTEGRATED / FULL_G18_NOT_ACCEPTED (PR #67; iOS deferred).
- **Goal:** Meet frozen budgets with bounded algorithms and native processing
- **Why:** Deliver the bounded functional requirement with explicit domain and native boundaries.
- **Inputs:** docs/planning/MOBILE_DELIVERY_PLAN.md; ADR-014/ADR-015; docs/product/STATION_FUEL_FEEDBACK.md; docs/security/FREE_ACCOUNT_ACCESS.md; docs/security/LOCAL_MEDIA_LOCATION_POLICY.md.
- **Files/areas expected:** changed hot-path domain/adapters; benchmark reports.
- **Dependencies:** P18-T01.
- **Tests first:** Baseline low-resource cold start/frame/heap/photo/outbox/battery and concurrent backend workloads.
- **Implementation outline:** Profile before optimization; targeted RED/GREEN regression; avoid unrelated refactors.
- **Acceptance criteria:** No main-thread decode/ANR/leak; legibility/KiB/RSS/latency meet measured frozen device budgets.
- **Validation commands:** Run the scoped domain/consumer, native compilation/device or real-PostGIS/race suites named by this task; document the exact executable commands and required result manifest before implementation acceptance. For existing Android baseline use `./gradlew :domain:test :application:test :data:testDebugUnitTest :app:testDebugUnitTest :app:assembleDebug --no-daemon`; backend package commands follow backend/README.md and docs/backend/TEST_STRATEGY.md. No invented script is claimed available.
- **Risks:** critical; missing native/provider evidence or unsafe fallback blocks acceptance.
- **Rollback/Recovery:** Disable the new feature with a tested flag, preserve existing free/offline behavior and compatible API; append-only migrations, no destructive history rewrite.
- **Definition of done:** DOD-1, actual scoped evidence and updated PROGRESS; phase integration requires specialized exit plus current-head/base Quick verification; no release claim from a plan.

<a id="p18-t03"></a>

### P18-T03 — Security compatibility and functional sign-off

- **ID / priority / status:** P18-T03 / MUST / LOCAL_VALIDATION_IMPLEMENTED — LOCAL_SLICE_INTEGRATED / FULL_G18_NOT_ACCEPTED (PR #67; iOS deferred).
- **Goal:** Hand off a functional app candidate to deferred production release
- **Why:** Deliver the bounded functional requirement with explicit domain and native boundaries.
- **Inputs:** docs/planning/MOBILE_DELIVERY_PLAN.md; ADR-014/ADR-015; docs/product/STATION_FUEL_FEEDBACK.md; docs/security/FREE_ACCOUNT_ACCESS.md; docs/security/LOCAL_MEDIA_LOCATION_POLICY.md.
- **Files/areas expected:** docs/release-evidence; support matrix; scoped native/security gates.
- **Dependencies:** P18-T02.
- **Tests first:** Auth/recovery/abuse/Unicode/expiry/GPS adversarial cases, upgrades and old-client compatibility.
- **Implementation outline:** Reconcile exact candidate, dependencies, platform limitations and critical findings; no cloud deploy.
- **Acceptance criteria:** G18 accepted only with real Android/iOS evidence; G09 remains uncertified until its separate production matrix.
- **Validation commands:** Run the scoped domain/consumer, native compilation/device or real-PostGIS/race suites named by this task; document the exact executable commands and required result manifest before implementation acceptance. For existing Android baseline use `./gradlew :domain:test :application:test :data:testDebugUnitTest :app:testDebugUnitTest :app:assembleDebug --no-daemon`; backend package commands follow backend/README.md and docs/backend/TEST_STRATEGY.md. No invented script is claimed available.
- **Risks:** critical; missing native/provider evidence or unsafe fallback blocks acceptance.
- **Rollback/Recovery:** Disable the new feature with a tested flag, preserve existing free/offline behavior and compatible API; append-only migrations, no destructive history rewrite.
- **Definition of done:** DOD-1, actual scoped evidence and updated PROGRESS; phase integration requires specialized exit plus current-head/base Quick verification; no release claim from a plan.

## Release-gate checklist G09

- [ ] P01–P08 required phase PRs/fixes are integrated; G01…G08 and G01-FLOW evidence is linked to actual checks/commits/images/schema/config. Historical P01 direct commits are recorded without inventing retroactive PRs.
- [ ] One immutable candidate SHA is selected after phase integration; full expected-result matrix is complete, including reconciliation of any reused specialized evidence. No missing/failed/skipped required release check or production deploy inferred from a phase merge.
- [ ] Catalog import, identity, observation, media, validation, consensus, moderation and privacy flows run end to end.
- [ ] Empty/upgrade migrations and duplicate/replay/concurrency cases pass.
- [ ] Current OpenAPI and shared fixtures are frozen; legacy Android differences documented.
- [ ] Existing Android regression baseline passes in a supported environment.
- [ ] TLS/origin restriction, private storage, role/secret/dependency checks and critical/high finding closure verified.
- [ ] Off-host encrypted backup restored into a new environment within accepted RPO/RTO; deletion ledger replayed.
- [ ] Runtime retention/export/erasure and user notice/legal review are complete.
- [ ] Load/fault/cache tests meet accepted capacity and freshness budgets.
- [ ] Operator can deploy, monitor, moderate, revoke, rollback and restore using tested runbooks.
- [ ] G24-ANDROID-COMMERCIAL functional Android acceptance is integrated; all required scoped G09 production evidence signed off before P10-T09 pilot. Historical G18 remains unaccepted; iOS deferred explicitly (ADR-016).

## Commercial Android phases — PLANNED

[ADR-016](docs/adr/016-android-commercial-community-ios-deferred.md) governs explicit iOS deferral. Detailed tasks/protocols/risks are in [COMMERCIAL_COMMUNITY_PLAN](docs/planning/COMMERCIAL_COMMUNITY_PLAN.md); none is runtime-complete merely because this plan was merged. Open only one current phase after authorization.

## P19 — Product and identity foundation

Priority: **MUST**. Entry: integrated Android/local backend, approved brand. Exit: **G19: commercial journey/theme/navigation foundation accepted**. State: PLANNED.

<a id="p19-t01"></a>

### P19-T01 — Inherited feature and toolchain baseline

- **ID / priority / status:** P19-T01 / MUST / PLANNED.
- **Goal:** Inventory actual implemented screens/contracts, preserve free/offline tools, audit supported toolchains and reconcile P18 Android gaps.
- **Inputs:** docs/product/COMMUNITY_EXPERIENCE.md B-BR-C01–C06 and BUC-C01–C05; owning section of docs/planning/COMMERCIAL_COMMUNITY_PLAN.md; existing module/security/feedback contracts.
- **Dependencies:** integrated Android/local backend, approved brand; affected backend extension integrated before its consumer.
- **Files/areas expected:** Existing domain/application/data/app or bounded backend adapters/read models as required; specify exact paths at task opening. iOS native work excluded.
- **Tests first:** Freeze positive/negative state fixtures; pure domain RED → GREEN → REFACTOR. Auth/money/privacy/SQL/jobs require immediate failure/concurrency/real-PostGIS proof if affected; native behavior needs supported-device evidence.
- **Acceptance criteria:** Owning commercial-plan task outcome demonstrated with actual evidence, preserved free/offline features and truthful source/privacy states. Missing required device/provider/storage proof remains incomplete.
- **Validation commands:** Select and record affected existing Gradle/backend test/compile/instrumentation commands before coding; docs-only slices use links/state checks. git diff --check and scoped secret review; specialized phase exit once plus actual finish --required "Quick verification" --pr <number> at phase closure only.
- **Risks / recovery:** Follow owning phase risks; preserve v1/legacy tools, append-only migration recovery and safe replay. Revert owned presentation changes without destroying contribution history.
- **Definition of done:** Targeted acceptance evidence, atomic task commit/issue, protected phase merge and wiki snapshot; no iOS acceptance, production certification or deployment inferred.

<a id="p19-t02"></a>

### P19-T02 — Community information architecture and usability prototype

- **ID / priority / status:** P19-T02 / MUST / PLANNED.
- **Goal:** Freeze B-BR-C01–C06/BUC-C01–C05 and validate guest discovery, progressive permissions, source/condition hierarchy and proposed navigation with novice drivers.
- **Inputs:** docs/product/COMMUNITY_EXPERIENCE.md B-BR-C01–C06 and BUC-C01–C05; owning section of docs/planning/COMMERCIAL_COMMUNITY_PLAN.md; existing module/security/feedback contracts.
- **Dependencies:** P19-T01; affected backend extension integrated before its consumer.
- **Files/areas expected:** Existing domain/application/data/app or bounded backend adapters/read models as required; specify exact paths at task opening. iOS native work excluded.
- **Tests first:** Freeze positive/negative state fixtures; pure domain RED → GREEN → REFACTOR. Auth/money/privacy/SQL/jobs require immediate failure/concurrency/real-PostGIS proof if affected; native behavior needs supported-device evidence.
- **Acceptance criteria:** Owning commercial-plan task outcome demonstrated with actual evidence, preserved free/offline features and truthful source/privacy states. Missing required device/provider/storage proof remains incomplete.
- **Validation commands:** Select and record affected existing Gradle/backend test/compile/instrumentation commands before coding; docs-only slices use links/state checks. git diff --check and scoped secret review; specialized phase exit once plus actual finish --required "Quick verification" --pr <number> at phase closure only.
- **Risks / recovery:** Follow owning phase risks; preserve v1/legacy tools, append-only migration recovery and safe replay. Revert owned presentation changes without destroying contribution history.
- **Definition of done:** Targeted acceptance evidence, atomic task commit/issue, protected phase merge and wiki snapshot; no iOS acceptance, production certification or deployment inferred.

<a id="p19-t03"></a>

### P19-T03 — Android brand and accessible component foundation

- **ID / priority / status:** P19-T03 / MUST / PLANNED.
- **Goal:** Apply approved identity/theme/display name and accessible reusable UI states without business rules in presentation.
- **Inputs:** docs/product/COMMUNITY_EXPERIENCE.md B-BR-C01–C06 and BUC-C01–C05; owning section of docs/planning/COMMERCIAL_COMMUNITY_PLAN.md; existing module/security/feedback contracts.
- **Dependencies:** P19-T02; affected backend extension integrated before its consumer.
- **Files/areas expected:** Existing domain/application/data/app or bounded backend adapters/read models as required; specify exact paths at task opening. iOS native work excluded.
- **Tests first:** Freeze positive/negative state fixtures; pure domain RED → GREEN → REFACTOR. Auth/money/privacy/SQL/jobs require immediate failure/concurrency/real-PostGIS proof if affected; native behavior needs supported-device evidence.
- **Acceptance criteria:** Owning commercial-plan task outcome demonstrated with actual evidence, preserved free/offline features and truthful source/privacy states. Missing required device/provider/storage proof remains incomplete.
- **Validation commands:** Select and record affected existing Gradle/backend test/compile/instrumentation commands before coding; docs-only slices use links/state checks. git diff --check and scoped secret review; specialized phase exit once plus actual finish --required "Quick verification" --pr <number> at phase closure only.
- **Risks / recovery:** Follow owning phase risks; preserve v1/legacy tools, append-only migration recovery and safe replay. Revert owned presentation changes without destroying contribution history.
- **Definition of done:** Targeted acceptance evidence, atomic task commit/issue, protected phase merge and wiki snapshot; no iOS acceptance, production certification or deployment inferred.

<a id="p19-t04"></a>

### P19-T04 — Navigation shell and preserved expert tools

- **ID / priority / status:** P19-T04 / MUST / PLANNED.
- **Goal:** Implement proposed shell, contribution entry point and secondary legacy tools, preserving deep-link/back/offline/auth state.
- **Inputs:** docs/product/COMMUNITY_EXPERIENCE.md B-BR-C01–C06 and BUC-C01–C05; owning section of docs/planning/COMMERCIAL_COMMUNITY_PLAN.md; existing module/security/feedback contracts.
- **Dependencies:** P19-T03; affected backend extension integrated before its consumer.
- **Files/areas expected:** Existing domain/application/data/app or bounded backend adapters/read models as required; specify exact paths at task opening. iOS native work excluded.
- **Tests first:** Freeze positive/negative state fixtures; pure domain RED → GREEN → REFACTOR. Auth/money/privacy/SQL/jobs require immediate failure/concurrency/real-PostGIS proof if affected; native behavior needs supported-device evidence.
- **Acceptance criteria:** Owning commercial-plan task outcome demonstrated with actual evidence, preserved free/offline features and truthful source/privacy states. Missing required device/provider/storage proof remains incomplete.
- **Validation commands:** Select and record affected existing Gradle/backend test/compile/instrumentation commands before coding; docs-only slices use links/state checks. git diff --check and scoped secret review; specialized phase exit once plus actual finish --required "Quick verification" --pr <number> at phase closure only.
- **Risks / recovery:** Follow owning phase risks; preserve v1/legacy tools, append-only migration recovery and safe replay. Revert owned presentation changes without destroying contribution history.
- **Definition of done:** Targeted acceptance evidence, atomic task commit/issue, protected phase merge and wiki snapshot; no iOS acceptance, production certification or deployment inferred.

## P20 — Community-first price discovery

Priority: **MUST**. Entry: G19. Exit: **G20: community discovery and station detail integrated with honest price/source states**. State: PLANNED.

<a id="p20-t01"></a>

### P20-T01 — Bounded discovery contract and read model

- **ID / priority / status:** P20-T01 / MUST / PLANNED.
- **Goal:** Reuse current projection; add only required paginated/indexed reads with exact-money/condition/source semantics and private-field protection.
- **Inputs:** docs/product/COMMUNITY_EXPERIENCE.md B-BR-C01–C06 and BUC-C01–C05; owning section of docs/planning/COMMERCIAL_COMMUNITY_PLAN.md; existing module/security/feedback contracts.
- **Dependencies:** G19; affected backend extension integrated before its consumer.
- **Files/areas expected:** Existing domain/application/data/app or bounded backend adapters/read models as required; specify exact paths at task opening. iOS native work excluded.
- **Tests first:** Freeze positive/negative state fixtures; pure domain RED → GREEN → REFACTOR. Auth/money/privacy/SQL/jobs require immediate failure/concurrency/real-PostGIS proof if affected; native behavior needs supported-device evidence.
- **Acceptance criteria:** Owning commercial-plan task outcome demonstrated with actual evidence, preserved free/offline features and truthful source/privacy states. Missing required device/provider/storage proof remains incomplete.
- **Validation commands:** Select and record affected existing Gradle/backend test/compile/instrumentation commands before coding; docs-only slices use links/state checks. git diff --check and scoped secret review; specialized phase exit once plus actual finish --required "Quick verification" --pr <number> at phase closure only.
- **Risks / recovery:** Follow owning phase risks; preserve v1/legacy tools, append-only migration recovery and safe replay. Revert owned presentation changes without destroying contribution history.
- **Definition of done:** Targeted acceptance evidence, atomic task commit/issue, protected phase merge and wiki snapshot; no iOS acceptance, production certification or deployment inferred.

<a id="p20-t02"></a>

### P20-T02 — Explore city and fuel journey

- **ID / priority / status:** P20-T02 / MUST / PLANNED.
- **Goal:** Implement simple list-first city/fuel/search/filter/sort with optional audited map, manual-city and offline-cache recovery.
- **Inputs:** docs/product/COMMUNITY_EXPERIENCE.md B-BR-C01–C06 and BUC-C01–C05; owning section of docs/planning/COMMERCIAL_COMMUNITY_PLAN.md; existing module/security/feedback contracts.
- **Dependencies:** P20-T01; affected backend extension integrated before its consumer.
- **Files/areas expected:** Existing domain/application/data/app or bounded backend adapters/read models as required; specify exact paths at task opening. iOS native work excluded.
- **Tests first:** Freeze positive/negative state fixtures; pure domain RED → GREEN → REFACTOR. Auth/money/privacy/SQL/jobs require immediate failure/concurrency/real-PostGIS proof if affected; native behavior needs supported-device evidence.
- **Acceptance criteria:** Owning commercial-plan task outcome demonstrated with actual evidence, preserved free/offline features and truthful source/privacy states. Missing required device/provider/storage proof remains incomplete.
- **Validation commands:** Select and record affected existing Gradle/backend test/compile/instrumentation commands before coding; docs-only slices use links/state checks. git diff --check and scoped secret review; specialized phase exit once plus actual finish --required "Quick verification" --pr <number> at phase closure only.
- **Risks / recovery:** Follow owning phase risks; preserve v1/legacy tools, append-only migration recovery and safe replay. Revert owned presentation changes without destroying contribution history.
- **Definition of done:** Targeted acceptance evidence, atomic task commit/issue, protected phase merge and wiki snapshot; no iOS acceptance, production certification or deployment inferred.

<a id="p20-t03"></a>

### P20-T03 — Station detail and dated ANP reference

- **ID / priority / status:** P20-T03 / MUST / PLANNED.
- **Goal:** Prioritize supported community cards/actions; show ANP dated secondary reference and explicit missing/stale/disputed states.
- **Inputs:** docs/product/COMMUNITY_EXPERIENCE.md B-BR-C01–C06 and BUC-C01–C05; owning section of docs/planning/COMMERCIAL_COMMUNITY_PLAN.md; existing module/security/feedback contracts.
- **Dependencies:** P20-T02; affected backend extension integrated before its consumer.
- **Files/areas expected:** Existing domain/application/data/app or bounded backend adapters/read models as required; specify exact paths at task opening. iOS native work excluded.
- **Tests first:** Freeze positive/negative state fixtures; pure domain RED → GREEN → REFACTOR. Auth/money/privacy/SQL/jobs require immediate failure/concurrency/real-PostGIS proof if affected; native behavior needs supported-device evidence.
- **Acceptance criteria:** Owning commercial-plan task outcome demonstrated with actual evidence, preserved free/offline features and truthful source/privacy states. Missing required device/provider/storage proof remains incomplete.
- **Validation commands:** Select and record affected existing Gradle/backend test/compile/instrumentation commands before coding; docs-only slices use links/state checks. git diff --check and scoped secret review; specialized phase exit once plus actual finish --required "Quick verification" --pr <number> at phase closure only.
- **Risks / recovery:** Follow owning phase risks; preserve v1/legacy tools, append-only migration recovery and safe replay. Revert owned presentation changes without destroying contribution history.
- **Definition of done:** Targeted acceptance evidence, atomic task commit/issue, protected phase merge and wiki snapshot; no iOS acceptance, production certification or deployment inferred.

<a id="p20-t04"></a>

### P20-T04 — Discovery acceptance and legacy regression

- **ID / priority / status:** P20-T04 / MUST / PLANNED.
- **Goal:** Prove local-service discovery, conditional-price comprehension, accessibility and retained expert/offline tools.
- **Inputs:** docs/product/COMMUNITY_EXPERIENCE.md B-BR-C01–C06 and BUC-C01–C05; owning section of docs/planning/COMMERCIAL_COMMUNITY_PLAN.md; existing module/security/feedback contracts.
- **Dependencies:** P20-T03; affected backend extension integrated before its consumer.
- **Files/areas expected:** Existing domain/application/data/app or bounded backend adapters/read models as required; specify exact paths at task opening. iOS native work excluded.
- **Tests first:** Freeze positive/negative state fixtures; pure domain RED → GREEN → REFACTOR. Auth/money/privacy/SQL/jobs require immediate failure/concurrency/real-PostGIS proof if affected; native behavior needs supported-device evidence.
- **Acceptance criteria:** Owning commercial-plan task outcome demonstrated with actual evidence, preserved free/offline features and truthful source/privacy states. Missing required device/provider/storage proof remains incomplete.
- **Validation commands:** Select and record affected existing Gradle/backend test/compile/instrumentation commands before coding; docs-only slices use links/state checks. git diff --check and scoped secret review; specialized phase exit once plus actual finish --required "Quick verification" --pr <number> at phase closure only.
- **Risks / recovery:** Follow owning phase risks; preserve v1/legacy tools, append-only migration recovery and safe replay. Revert owned presentation changes without destroying contribution history.
- **Definition of done:** Targeted acceptance evidence, atomic task commit/issue, protected phase merge and wiki snapshot; no iOS acceptance, production certification or deployment inferred.

## P21 — Photo-led contribution journey

Priority: **MUST**. Entry: G20. Exit: **G21: Android capture/review/submit/status and local private-photo expiry proven**. State: PLANNED.

<a id="p21-t01"></a>

### P21-T01 — Contribution state and evidence contract

- **ID / priority / status:** P21-T01 / MUST / PLANNED.
- **Goal:** Freeze photo-led UX states while preserving optional-evidence signed API compatibility, server timestamps and authoritative consensus.
- **Inputs:** docs/product/COMMUNITY_EXPERIENCE.md B-BR-C01–C06 and BUC-C01–C05; owning section of docs/planning/COMMERCIAL_COMMUNITY_PLAN.md; existing module/security/feedback contracts.
- **Dependencies:** G20; affected backend extension integrated before its consumer.
- **Files/areas expected:** Existing domain/application/data/app or bounded backend adapters/read models as required; specify exact paths at task opening. iOS native work excluded.
- **Tests first:** Freeze positive/negative state fixtures; pure domain RED → GREEN → REFACTOR. Auth/money/privacy/SQL/jobs require immediate failure/concurrency/real-PostGIS proof if affected; native behavior needs supported-device evidence.
- **Acceptance criteria:** Owning commercial-plan task outcome demonstrated with actual evidence, preserved free/offline features and truthful source/privacy states. Missing required device/provider/storage proof remains incomplete.
- **Validation commands:** Select and record affected existing Gradle/backend test/compile/instrumentation commands before coding; docs-only slices use links/state checks. git diff --check and scoped secret review; specialized phase exit once plus actual finish --required "Quick verification" --pr <number> at phase closure only.
- **Risks / recovery:** Follow owning phase risks; preserve v1/legacy tools, append-only migration recovery and safe replay. Revert owned presentation changes without destroying contribution history.
- **Definition of done:** Targeted acceptance evidence, atomic task commit/issue, protected phase merge and wiki snapshot; no iOS acceptance, production certification or deployment inferred.

<a id="p21-t02"></a>

### P21-T02 — Lightweight camera and editable price review

- **ID / priority / status:** P21-T02 / MUST / PLANNED.
- **Goal:** Integrate camera/encoder/OCR review and backend sanitizer, measuring low-end-device size/time/heap against existing P15 budgets.
- **Inputs:** docs/product/COMMUNITY_EXPERIENCE.md B-BR-C01–C06 and BUC-C01–C05; owning section of docs/planning/COMMERCIAL_COMMUNITY_PLAN.md; existing module/security/feedback contracts.
- **Dependencies:** P21-T01; affected backend extension integrated before its consumer.
- **Files/areas expected:** Existing domain/application/data/app or bounded backend adapters/read models as required; specify exact paths at task opening. iOS native work excluded.
- **Tests first:** Freeze positive/negative state fixtures; pure domain RED → GREEN → REFACTOR. Auth/money/privacy/SQL/jobs require immediate failure/concurrency/real-PostGIS proof if affected; native behavior needs supported-device evidence.
- **Acceptance criteria:** Owning commercial-plan task outcome demonstrated with actual evidence, preserved free/offline features and truthful source/privacy states. Missing required device/provider/storage proof remains incomplete.
- **Validation commands:** Select and record affected existing Gradle/backend test/compile/instrumentation commands before coding; docs-only slices use links/state checks. git diff --check and scoped secret review; specialized phase exit once plus actual finish --required "Quick verification" --pr <number> at phase closure only.
- **Risks / recovery:** Follow owning phase risks; preserve v1/legacy tools, append-only migration recovery and safe replay. Revert owned presentation changes without destroying contribution history.
- **Definition of done:** Targeted acceptance evidence, atomic task commit/issue, protected phase merge and wiki snapshot; no iOS acceptance, production certification or deployment inferred.

<a id="p21-t03"></a>

### P21-T03 — Outbox replay and contributor status

- **ID / priority / status:** P21-T03 / MUST / PLANNED.
- **Goal:** Integrate stable-ID submission, retries/cancellation/private owner status and honest location integrity with failure/concurrency tests.
- **Inputs:** docs/product/COMMUNITY_EXPERIENCE.md B-BR-C01–C06 and BUC-C01–C05; owning section of docs/planning/COMMERCIAL_COMMUNITY_PLAN.md; existing module/security/feedback contracts.
- **Dependencies:** P21-T02; affected backend extension integrated before its consumer.
- **Files/areas expected:** Existing domain/application/data/app or bounded backend adapters/read models as required; specify exact paths at task opening. iOS native work excluded.
- **Tests first:** Freeze positive/negative state fixtures; pure domain RED → GREEN → REFACTOR. Auth/money/privacy/SQL/jobs require immediate failure/concurrency/real-PostGIS proof if affected; native behavior needs supported-device evidence.
- **Acceptance criteria:** Owning commercial-plan task outcome demonstrated with actual evidence, preserved free/offline features and truthful source/privacy states. Missing required device/provider/storage proof remains incomplete.
- **Validation commands:** Select and record affected existing Gradle/backend test/compile/instrumentation commands before coding; docs-only slices use links/state checks. git diff --check and scoped secret review; specialized phase exit once plus actual finish --required "Quick verification" --pr <number> at phase closure only.
- **Risks / recovery:** Follow owning phase risks; preserve v1/legacy tools, append-only migration recovery and safe replay. Revert owned presentation changes without destroying contribution history.
- **Definition of done:** Targeted acceptance evidence, atomic task commit/issue, protected phase merge and wiki snapshot; no iOS acceptance, production certification or deployment inferred.

<a id="p21-t04"></a>

### P21-T04 — Local storage and all-copy expiry evidence

- **ID / priority / status:** P21-T04 / MUST / PLANNED.
- **Goal:** Resolve P18 local S3-compatible environment gap and prove object/cache/temp/retry/restore/access expiry and failure retry within 24h.
- **Inputs:** docs/product/COMMUNITY_EXPERIENCE.md B-BR-C01–C06 and BUC-C01–C05; owning section of docs/planning/COMMERCIAL_COMMUNITY_PLAN.md; existing module/security/feedback contracts.
- **Dependencies:** P21-T03; affected backend extension integrated before its consumer.
- **Files/areas expected:** Existing domain/application/data/app or bounded backend adapters/read models as required; specify exact paths at task opening. iOS native work excluded.
- **Tests first:** Freeze positive/negative state fixtures; pure domain RED → GREEN → REFACTOR. Auth/money/privacy/SQL/jobs require immediate failure/concurrency/real-PostGIS proof if affected; native behavior needs supported-device evidence.
- **Acceptance criteria:** Owning commercial-plan task outcome demonstrated with actual evidence, preserved free/offline features and truthful source/privacy states. Missing required device/provider/storage proof remains incomplete.
- **Validation commands:** Select and record affected existing Gradle/backend test/compile/instrumentation commands before coding; docs-only slices use links/state checks. git diff --check and scoped secret review; specialized phase exit once plus actual finish --required "Quick verification" --pr <number> at phase closure only.
- **Risks / recovery:** Follow owning phase risks; preserve v1/legacy tools, append-only migration recovery and safe replay. Revert owned presentation changes without destroying contribution history.
- **Definition of done:** Targeted acceptance evidence, atomic task commit/issue, protected phase merge and wiki snapshot; no iOS acceptance, production certification or deployment inferred.

## P22 — Social experience and moderation

Priority: **MUST**. Entry: G21 and integrated P14/P17 contracts. Exit: **G22: safe free-account station/fuel participation and operator moderation**. State: PLANNED.

<a id="p22-t01"></a>

### P22-T01 — Progressive free account journey

- **ID / priority / status:** P22-T01 / MUST / PLANNED.
- **Goal:** Integrate email-code/Google/Apple supported Android provider flows, recovery/rights and secure sessions with real provider proof.
- **Inputs:** docs/product/COMMUNITY_EXPERIENCE.md B-BR-C01–C06 and BUC-C01–C05; owning section of docs/planning/COMMERCIAL_COMMUNITY_PLAN.md; existing module/security/feedback contracts.
- **Dependencies:** G21 and integrated P14/P17 contracts; affected backend extension integrated before its consumer.
- **Files/areas expected:** Existing domain/application/data/app or bounded backend adapters/read models as required; specify exact paths at task opening. iOS native work excluded.
- **Tests first:** Freeze positive/negative state fixtures; pure domain RED → GREEN → REFACTOR. Auth/money/privacy/SQL/jobs require immediate failure/concurrency/real-PostGIS proof if affected; native behavior needs supported-device evidence.
- **Acceptance criteria:** Owning commercial-plan task outcome demonstrated with actual evidence, preserved free/offline features and truthful source/privacy states. Missing required device/provider/storage proof remains incomplete.
- **Validation commands:** Select and record affected existing Gradle/backend test/compile/instrumentation commands before coding; docs-only slices use links/state checks. git diff --check and scoped secret review; specialized phase exit once plus actual finish --required "Quick verification" --pr <number> at phase closure only.
- **Risks / recovery:** Follow owning phase risks; preserve v1/legacy tools, append-only migration recovery and safe replay. Revert owned presentation changes without destroying contribution history.
- **Definition of done:** Targeted acceptance evidence, atomic task commit/issue, protected phase merge and wiki snapshot; no iOS acceptance, production certification or deployment inferred.

<a id="p22-t02"></a>

### P22-T02 — Ratings comments replies and validity votes

- **ID / priority / status:** P22-T02 / MUST / PLANNED.
- **Goal:** Integrate stars/280 Unicode scalar comments/one-level replies/revision-bound votes; separate community agreement from price confidence.
- **Inputs:** docs/product/COMMUNITY_EXPERIENCE.md B-BR-C01–C06 and BUC-C01–C05; owning section of docs/planning/COMMERCIAL_COMMUNITY_PLAN.md; existing module/security/feedback contracts.
- **Dependencies:** P22-T01; affected backend extension integrated before its consumer.
- **Files/areas expected:** Existing domain/application/data/app or bounded backend adapters/read models as required; specify exact paths at task opening. iOS native work excluded.
- **Tests first:** Freeze positive/negative state fixtures; pure domain RED → GREEN → REFACTOR. Auth/money/privacy/SQL/jobs require immediate failure/concurrency/real-PostGIS proof if affected; native behavior needs supported-device evidence.
- **Acceptance criteria:** Owning commercial-plan task outcome demonstrated with actual evidence, preserved free/offline features and truthful source/privacy states. Missing required device/provider/storage proof remains incomplete.
- **Validation commands:** Select and record affected existing Gradle/backend test/compile/instrumentation commands before coding; docs-only slices use links/state checks. git diff --check and scoped secret review; specialized phase exit once plus actual finish --required "Quick verification" --pr <number> at phase closure only.
- **Risks / recovery:** Follow owning phase risks; preserve v1/legacy tools, append-only migration recovery and safe replay. Revert owned presentation changes without destroying contribution history.
- **Definition of done:** Targeted acceptance evidence, atomic task commit/issue, protected phase merge and wiki snapshot; no iOS acceptance, production certification or deployment inferred.

<a id="p22-t03"></a>

### P22-T03 — Reports moderation and support

- **ID / priority / status:** P22-T03 / MUST / PLANNED.
- **Goal:** Integrate authorized report/review/status/support/appeal workflow, auditable outcomes and staffing; define any blocking/muting contract before code.
- **Inputs:** docs/product/COMMUNITY_EXPERIENCE.md B-BR-C01–C06 and BUC-C01–C05; owning section of docs/planning/COMMERCIAL_COMMUNITY_PLAN.md; existing module/security/feedback contracts.
- **Dependencies:** P22-T02; affected backend extension integrated before its consumer.
- **Files/areas expected:** Existing domain/application/data/app or bounded backend adapters/read models as required; specify exact paths at task opening. iOS native work excluded.
- **Tests first:** Freeze positive/negative state fixtures; pure domain RED → GREEN → REFACTOR. Auth/money/privacy/SQL/jobs require immediate failure/concurrency/real-PostGIS proof if affected; native behavior needs supported-device evidence.
- **Acceptance criteria:** Owning commercial-plan task outcome demonstrated with actual evidence, preserved free/offline features and truthful source/privacy states. Missing required device/provider/storage proof remains incomplete.
- **Validation commands:** Select and record affected existing Gradle/backend test/compile/instrumentation commands before coding; docs-only slices use links/state checks. git diff --check and scoped secret review; specialized phase exit once plus actual finish --required "Quick verification" --pr <number> at phase closure only.
- **Risks / recovery:** Follow owning phase risks; preserve v1/legacy tools, append-only migration recovery and safe replay. Revert owned presentation changes without destroying contribution history.
- **Definition of done:** Targeted acceptance evidence, atomic task commit/issue, protected phase merge and wiki snapshot; no iOS acceptance, production certification or deployment inferred.

<a id="p22-t04"></a>

### P22-T04 — Social abuse and lifecycle acceptance

- **ID / priority / status:** P22-T04 / MUST / PLANNED.
- **Goal:** Prove ownership/rate-limit/revision/offline/erasure/concurrency/moderation and no private-field disclosure against local services.
- **Inputs:** docs/product/COMMUNITY_EXPERIENCE.md B-BR-C01–C06 and BUC-C01–C05; owning section of docs/planning/COMMERCIAL_COMMUNITY_PLAN.md; existing module/security/feedback contracts.
- **Dependencies:** P22-T03; affected backend extension integrated before its consumer.
- **Files/areas expected:** Existing domain/application/data/app or bounded backend adapters/read models as required; specify exact paths at task opening. iOS native work excluded.
- **Tests first:** Freeze positive/negative state fixtures; pure domain RED → GREEN → REFACTOR. Auth/money/privacy/SQL/jobs require immediate failure/concurrency/real-PostGIS proof if affected; native behavior needs supported-device evidence.
- **Acceptance criteria:** Owning commercial-plan task outcome demonstrated with actual evidence, preserved free/offline features and truthful source/privacy states. Missing required device/provider/storage proof remains incomplete.
- **Validation commands:** Select and record affected existing Gradle/backend test/compile/instrumentation commands before coding; docs-only slices use links/state checks. git diff --check and scoped secret review; specialized phase exit once plus actual finish --required "Quick verification" --pr <number> at phase closure only.
- **Risks / recovery:** Follow owning phase risks; preserve v1/legacy tools, append-only migration recovery and safe replay. Revert owned presentation changes without destroying contribution history.
- **Definition of done:** Targeted acceptance evidence, atomic task commit/issue, protected phase merge and wiki snapshot; no iOS acceptance, production certification or deployment inferred.

## P23 — Community operations and sustainable participation

Priority: **MUST**. Entry: G22. Exit: **G23: bounded-city operations, optional return journeys and minimized metrics ready locally**. State: PLANNED.

<a id="p23-t01"></a>

### P23-T01 — Favorites and justified opt-in return flows

- **ID / priority / status:** P23-T01 / MUST / PLANNED.
- **Goal:** Preserve favorites; implement follows/subscriptions only after measured need and ownership/unsubscribe/erase/notification contracts.
- **Inputs:** docs/product/COMMUNITY_EXPERIENCE.md B-BR-C01–C06 and BUC-C01–C05; owning section of docs/planning/COMMERCIAL_COMMUNITY_PLAN.md; existing module/security/feedback contracts.
- **Dependencies:** G22; affected backend extension integrated before its consumer.
- **Files/areas expected:** Existing domain/application/data/app or bounded backend adapters/read models as required; specify exact paths at task opening. iOS native work excluded.
- **Tests first:** Freeze positive/negative state fixtures; pure domain RED → GREEN → REFACTOR. Auth/money/privacy/SQL/jobs require immediate failure/concurrency/real-PostGIS proof if affected; native behavior needs supported-device evidence.
- **Acceptance criteria:** Owning commercial-plan task outcome demonstrated with actual evidence, preserved free/offline features and truthful source/privacy states. Missing required device/provider/storage proof remains incomplete.
- **Validation commands:** Select and record affected existing Gradle/backend test/compile/instrumentation commands before coding; docs-only slices use links/state checks. git diff --check and scoped secret review; specialized phase exit once plus actual finish --required "Quick verification" --pr <number> at phase closure only.
- **Risks / recovery:** Follow owning phase risks; preserve v1/legacy tools, append-only migration recovery and safe replay. Revert owned presentation changes without destroying contribution history.
- **Definition of done:** Targeted acceptance evidence, atomic task commit/issue, protected phase merge and wiki snapshot; no iOS acceptance, production certification or deployment inferred.

<a id="p23-t02"></a>

### P23-T02 — Contributor onboarding and community rules

- **ID / priority / status:** P23-T02 / MUST / PLANNED.
- **Goal:** Create plain-language contributor guidance/report/help policy; optional safe invite flow only where needed, no fake members/activity.
- **Inputs:** docs/product/COMMUNITY_EXPERIENCE.md B-BR-C01–C06 and BUC-C01–C05; owning section of docs/planning/COMMERCIAL_COMMUNITY_PLAN.md; existing module/security/feedback contracts.
- **Dependencies:** P23-T01; affected backend extension integrated before its consumer.
- **Files/areas expected:** Existing domain/application/data/app or bounded backend adapters/read models as required; specify exact paths at task opening. iOS native work excluded.
- **Tests first:** Freeze positive/negative state fixtures; pure domain RED → GREEN → REFACTOR. Auth/money/privacy/SQL/jobs require immediate failure/concurrency/real-PostGIS proof if affected; native behavior needs supported-device evidence.
- **Acceptance criteria:** Owning commercial-plan task outcome demonstrated with actual evidence, preserved free/offline features and truthful source/privacy states. Missing required device/provider/storage proof remains incomplete.
- **Validation commands:** Select and record affected existing Gradle/backend test/compile/instrumentation commands before coding; docs-only slices use links/state checks. git diff --check and scoped secret review; specialized phase exit once plus actual finish --required "Quick verification" --pr <number> at phase closure only.
- **Risks / recovery:** Follow owning phase risks; preserve v1/legacy tools, append-only migration recovery and safe replay. Revert owned presentation changes without destroying contribution history.
- **Definition of done:** Targeted acceptance evidence, atomic task commit/issue, protected phase merge and wiki snapshot; no iOS acceptance, production certification or deployment inferred.

<a id="p23-t03"></a>

### P23-T03 — Aggregate metrics and pilot operating model

- **ID / priority / status:** P23-T03 / MUST / PLANNED.
- **Goal:** Define purpose/denominators/windows/retention/minimization plus moderation staffing/capacity/cost baselines and low-coverage policy.
- **Inputs:** docs/product/COMMUNITY_EXPERIENCE.md B-BR-C01–C06 and BUC-C01–C05; owning section of docs/planning/COMMERCIAL_COMMUNITY_PLAN.md; existing module/security/feedback contracts.
- **Dependencies:** P23-T02; affected backend extension integrated before its consumer.
- **Files/areas expected:** Existing domain/application/data/app or bounded backend adapters/read models as required; specify exact paths at task opening. iOS native work excluded.
- **Tests first:** Freeze positive/negative state fixtures; pure domain RED → GREEN → REFACTOR. Auth/money/privacy/SQL/jobs require immediate failure/concurrency/real-PostGIS proof if affected; native behavior needs supported-device evidence.
- **Acceptance criteria:** Owning commercial-plan task outcome demonstrated with actual evidence, preserved free/offline features and truthful source/privacy states. Missing required device/provider/storage proof remains incomplete.
- **Validation commands:** Select and record affected existing Gradle/backend test/compile/instrumentation commands before coding; docs-only slices use links/state checks. git diff --check and scoped secret review; specialized phase exit once plus actual finish --required "Quick verification" --pr <number> at phase closure only.
- **Risks / recovery:** Follow owning phase risks; preserve v1/legacy tools, append-only migration recovery and safe replay. Revert owned presentation changes without destroying contribution history.
- **Definition of done:** Targeted acceptance evidence, atomic task commit/issue, protected phase merge and wiki snapshot; no iOS acceptance, production certification or deployment inferred.

<a id="p23-t04"></a>

### P23-T04 — Local community rehearsal and business hypotheses

- **ID / priority / status:** P23-T04 / MUST / PLANNED.
- **Goal:** Rehearse synthetic city lifecycle/rights/support locally; record pilot stop/rollback risks and optional P11 value hypotheses without launching.
- **Inputs:** docs/product/COMMUNITY_EXPERIENCE.md B-BR-C01–C06 and BUC-C01–C05; owning section of docs/planning/COMMERCIAL_COMMUNITY_PLAN.md; existing module/security/feedback contracts.
- **Dependencies:** P23-T03; affected backend extension integrated before its consumer.
- **Files/areas expected:** Existing domain/application/data/app or bounded backend adapters/read models as required; specify exact paths at task opening. iOS native work excluded.
- **Tests first:** Freeze positive/negative state fixtures; pure domain RED → GREEN → REFACTOR. Auth/money/privacy/SQL/jobs require immediate failure/concurrency/real-PostGIS proof if affected; native behavior needs supported-device evidence.
- **Acceptance criteria:** Owning commercial-plan task outcome demonstrated with actual evidence, preserved free/offline features and truthful source/privacy states. Missing required device/provider/storage proof remains incomplete.
- **Validation commands:** Select and record affected existing Gradle/backend test/compile/instrumentation commands before coding; docs-only slices use links/state checks. git diff --check and scoped secret review; specialized phase exit once plus actual finish --required "Quick verification" --pr <number> at phase closure only.
- **Risks / recovery:** Follow owning phase risks; preserve v1/legacy tools, append-only migration recovery and safe replay. Revert owned presentation changes without destroying contribution history.
- **Definition of done:** Targeted acceptance evidence, atomic task commit/issue, protected phase merge and wiki snapshot; no iOS acceptance, production certification or deployment inferred.

## P24 — Android commercial acceptance

Priority: **MUST**. Entry: G19–G23 integrated and required P18 Android/backend gaps resolved. Exit: **G24-ANDROID-COMMERCIAL: pinned commercial Android candidate accepted; G09 still uncertified**. State: PLANNED.

<a id="p24-t01"></a>

### P24-T01 — Android candidate and complete device matrix

- **ID / priority / status:** P24-T01 / MUST / PLANNED.
- **Goal:** Freeze actual support/candidate and prove every old/new functional/offline/provider/privacy/media/location/source row on required devices/services.
- **Inputs:** docs/product/COMMUNITY_EXPERIENCE.md B-BR-C01–C06 and BUC-C01–C05; owning section of docs/planning/COMMERCIAL_COMMUNITY_PLAN.md; existing module/security/feedback contracts.
- **Dependencies:** G19–G23 integrated and required P18 Android/backend gaps resolved; affected backend extension integrated before its consumer.
- **Files/areas expected:** Existing domain/application/data/app or bounded backend adapters/read models as required; specify exact paths at task opening. iOS native work excluded.
- **Tests first:** Freeze positive/negative state fixtures; pure domain RED → GREEN → REFACTOR. Auth/money/privacy/SQL/jobs require immediate failure/concurrency/real-PostGIS proof if affected; native behavior needs supported-device evidence.
- **Acceptance criteria:** Owning commercial-plan task outcome demonstrated with actual evidence, preserved free/offline features and truthful source/privacy states. Missing required device/provider/storage proof remains incomplete.
- **Validation commands:** Select and record affected existing Gradle/backend test/compile/instrumentation commands before coding; docs-only slices use links/state checks. git diff --check and scoped secret review; specialized phase exit once plus actual finish --required "Quick verification" --pr <number> at phase closure only.
- **Risks / recovery:** Follow owning phase risks; preserve v1/legacy tools, append-only migration recovery and safe replay. Revert owned presentation changes without destroying contribution history.
- **Definition of done:** Targeted acceptance evidence, atomic task commit/issue, protected phase merge and wiki snapshot; no iOS acceptance, production certification or deployment inferred.

<a id="p24-t02"></a>

### P24-T02 — Novice usability and accessibility acceptance

- **ID / priority / status:** P24-T02 / MUST / PLANNED.
- **Goal:** Run baseline-derived predeclared novice tasks and screen-reader/font-scale checks; remediate source/condition/contribution comprehension failures.
- **Inputs:** docs/product/COMMUNITY_EXPERIENCE.md B-BR-C01–C06 and BUC-C01–C05; owning section of docs/planning/COMMERCIAL_COMMUNITY_PLAN.md; existing module/security/feedback contracts.
- **Dependencies:** P24-T01; affected backend extension integrated before its consumer.
- **Files/areas expected:** Existing domain/application/data/app or bounded backend adapters/read models as required; specify exact paths at task opening. iOS native work excluded.
- **Tests first:** Freeze positive/negative state fixtures; pure domain RED → GREEN → REFACTOR. Auth/money/privacy/SQL/jobs require immediate failure/concurrency/real-PostGIS proof if affected; native behavior needs supported-device evidence.
- **Acceptance criteria:** Owning commercial-plan task outcome demonstrated with actual evidence, preserved free/offline features and truthful source/privacy states. Missing required device/provider/storage proof remains incomplete.
- **Validation commands:** Select and record affected existing Gradle/backend test/compile/instrumentation commands before coding; docs-only slices use links/state checks. git diff --check and scoped secret review; specialized phase exit once plus actual finish --required "Quick verification" --pr <number> at phase closure only.
- **Risks / recovery:** Follow owning phase risks; preserve v1/legacy tools, append-only migration recovery and safe replay. Revert owned presentation changes without destroying contribution history.
- **Definition of done:** Targeted acceptance evidence, atomic task commit/issue, protected phase merge and wiki snapshot; no iOS acceptance, production certification or deployment inferred.

<a id="p24-t03"></a>

### P24-T03 — Real-device performance and memory acceptance

- **ID / priority / status:** P24-T03 / MUST / PLANNED.
- **Goal:** Measure genuine cold-process start, scrolling/encoding/OCR/peak heap/background/battery on the frozen support matrix; optimize against predeclared budgets.
- **Inputs:** docs/product/COMMUNITY_EXPERIENCE.md B-BR-C01–C06 and BUC-C01–C05; owning section of docs/planning/COMMERCIAL_COMMUNITY_PLAN.md; existing module/security/feedback contracts.
- **Dependencies:** P24-T02; affected backend extension integrated before its consumer.
- **Files/areas expected:** Existing domain/application/data/app or bounded backend adapters/read models as required; specify exact paths at task opening. iOS native work excluded.
- **Tests first:** Freeze positive/negative state fixtures; pure domain RED → GREEN → REFACTOR. Auth/money/privacy/SQL/jobs require immediate failure/concurrency/real-PostGIS proof if affected; native behavior needs supported-device evidence.
- **Acceptance criteria:** Owning commercial-plan task outcome demonstrated with actual evidence, preserved free/offline features and truthful source/privacy states. Missing required device/provider/storage proof remains incomplete.
- **Validation commands:** Select and record affected existing Gradle/backend test/compile/instrumentation commands before coding; docs-only slices use links/state checks. git diff --check and scoped secret review; specialized phase exit once plus actual finish --required "Quick verification" --pr <number> at phase closure only.
- **Risks / recovery:** Follow owning phase risks; preserve v1/legacy tools, append-only migration recovery and safe replay. Revert owned presentation changes without destroying contribution history.
- **Definition of done:** Targeted acceptance evidence, atomic task commit/issue, protected phase merge and wiki snapshot; no iOS acceptance, production certification or deployment inferred.

<a id="p24-t04"></a>

### P24-T04 — Security compatibility and Android sign-off

- **ID / priority / status:** P24-T04 / MUST / PLANNED.
- **Goal:** Resolve all required adversarial/compatibility/privacy proofs and produce Android-only manifest; phase integration never certifies real production.
- **Inputs:** docs/product/COMMUNITY_EXPERIENCE.md B-BR-C01–C06 and BUC-C01–C05; owning section of docs/planning/COMMERCIAL_COMMUNITY_PLAN.md; existing module/security/feedback contracts.
- **Dependencies:** P24-T03; affected backend extension integrated before its consumer.
- **Files/areas expected:** Existing domain/application/data/app or bounded backend adapters/read models as required; specify exact paths at task opening. iOS native work excluded.
- **Tests first:** Freeze positive/negative state fixtures; pure domain RED → GREEN → REFACTOR. Auth/money/privacy/SQL/jobs require immediate failure/concurrency/real-PostGIS proof if affected; native behavior needs supported-device evidence.
- **Acceptance criteria:** Owning commercial-plan task outcome demonstrated with actual evidence, preserved free/offline features and truthful source/privacy states. Missing required device/provider/storage proof remains incomplete.
- **Validation commands:** Select and record affected existing Gradle/backend test/compile/instrumentation commands before coding; docs-only slices use links/state checks. git diff --check and scoped secret review; specialized phase exit once plus actual finish --required "Quick verification" --pr <number> at phase closure only.
- **Risks / recovery:** Follow owning phase risks; preserve v1/legacy tools, append-only migration recovery and safe replay. Revert owned presentation changes without destroying contribution history.
- **Definition of done:** Targeted acceptance evidence, atomic task commit/issue, protected phase merge and wiki snapshot; no iOS acceptance, production certification or deployment inferred.

<a id="station-catalog-expansion"></a>

## P25 — National ANP registry

Priority: **MUST for catalog scope**. Entry: G24-ANDROID-COMMERCIAL integrated; existing Directory baseline reconciled. Exit: **G25: validated national registry ingestion and canonical server publication, including stations with no prices**. State: PLANNED. Branch: `codex/phase-25-national-registry`.

<a id="p25-t01"></a>

### P25-T01 — Source contracts, eligibility and registry fixtures

- **ID / priority / status:** P25-T01 / MUST / PLANNED.
- **Goal:** Freeze independent source/authorization/operation/location/publication semantics before behavior.
- **Inputs / rules:** docs/planning/STATION_CATALOG_PLAN.md; docs/product/STATION_CATALOG.md B-BR-D01–D06/D10–D12; BUC-D01/D05/D08; existing owning account/media/location/moderation/price contracts.
- **Dependencies:** G24-ANDROID-COMMERCIAL integrated; existing Directory baseline reconciled.
- **Files/areas expected:** docs/product/STATION_CATALOG.md; docs/backend/DATA_MODEL.md and API_PLAN.md; public minimal contracts/testdata registry fixtures.
- **Risk / tests first:** docs; Representative CSV/API fields, numeric/alphanumeric CNPJ, publication-date semantics, coordinate quality, completeness and client-state compatibility.
- **Implementation outline:** Record access/rights/quotas, limits and resource/freshness policy; document fixtures with provenance; freeze schema and OpenAPI rollout boundaries.
- **Acceptance criteria:** Versioned field/state mapping, sufficient-evidence eligibility, source precedence/conflict handling, numerical safety limits and resource budgets documented; unsupported inputs explicitly quarantined.
- **Validation commands:** Scoped Markdown links, rule/use-case/task/dependency consistency and explicit new-file whitespace/secret review; `git diff --check`. No runtime gate inferred from fixtures or docs.
- **Risks / recovery:** Preserve stable UUID/history and last validated publication; disable the changed source/flow or restore the projection with audit. Applied migrations are append-only with previous-schema upgrade/recovery tests; no data-destructive rollback, broader trust, privacy exception or hidden failure.
- **Definition of done:** Actual scoped evidence and task issue/atomic commit, integrated through its guarded phase PR; specialized exit once and required current-head/base Quick verification through finish, then merged-SHA wiki once. Planning is not LOCAL_DONE, integration is not G09 certification, and optional unselected tasks are never marked complete.

<a id="p25-t02"></a>

### P25-T02 — Bounded CSV snapshot staging

- **ID / priority / status:** P25-T02 / MUST / PLANNED.
- **Goal:** Discover and stream the national registry without exposing partial imports.
- **Inputs / rules:** docs/planning/STATION_CATALOG_PLAN.md; docs/product/STATION_CATALOG.md B-BR-D02/D03/D05/D10; BUC-D01/D06; existing owning account/media/location/moderation/price contracts.
- **Dependencies:** P25-T01.
- **Files/areas expected:** backend/internal/modules/directory adapters/application; owned SQL/migrations; existing source transport/platform jobs; registry fixtures.
- **Risk / tests first:** critical; Headers/encoding/leading zeros, malformed/oversize input, repeated checksum, duplicate/conflicting records, truncated snapshot and concurrent import.
- **Implementation outline:** Introduce bounded parser and source-run staging/checksum/checkpoints; append-only migration with short batches and explicit completion.
- **Acceptance criteria:** All input rows accounted; incomplete/invalid snapshots preserve last-good state; identical replay adds no duplicate assertions; empty/upgrade/recovery and concurrency pass.
- **Validation commands:** From backend/: `go test -race ./internal/modules/directory/...` and `go test -race -tags=integration ./internal/modules/directory/...` using the disposable environment in backend/README; actual changed job/schema/transport consumers, empty/upgrade/recovery migrations and SQL generation/contract checks as affected. `git diff --check` and scoped secret review.
- **Risks / recovery:** Preserve stable UUID/history and last validated publication; disable the changed source/flow or restore the projection with audit. Applied migrations are append-only with previous-schema upgrade/recovery tests; no data-destructive rollback, broader trust, privacy exception or hidden failure.
- **Definition of done:** Actual scoped evidence and task issue/atomic commit, integrated through its guarded phase PR; specialized exit once and required current-head/base Quick verification through finish, then merged-SHA wiki once. Planning is not LOCAL_DONE, integration is not G09 certification, and optional unselected tasks are never marked complete.

<a id="p25-t03"></a>

### P25-T03 — Paginated and targeted ANP API discovery

- **ID / priority / status:** P25-T03 / MUST / PLANNED.
- **Goal:** Verify reported CNPJs and discover bounded scopes using the official API.
- **Inputs / rules:** docs/planning/STATION_CATALOG_PLAN.md; docs/product/STATION_CATALOG.md B-BR-D02/D03/D05/D06/D10; BUC-D01; existing owning account/media/location/moderation/price contracts.
- **Dependencies:** P25-T02.
- **Files/areas expected:** Directory source adapter and existing HTTP transport/jobs; contract fixtures.
- **Risk / tests first:** critical; Pagination gaps/reordering/duplicates, false success payload, 429/timeout/redirect, unsupported CNPJ forms, missing coordinates and invalid CRS.
- **Implementation outline:** Implement typed bounded page traversal and targeted lookup with global quota/cache/backoff; retain metadata and completeness, no fetch in public handlers.
- **Acceptance criteria:** Bounded queries resolve supported identifiers; partial traversal never masquerades as a full snapshot; unsupported identifier/provider behavior has a safe documented fallback, not identifier corruption.
- **Validation commands:** From backend/: `go test -race ./internal/modules/directory/...` and `go test -race -tags=integration ./internal/modules/directory/...` using the disposable environment in backend/README; actual changed job/schema/transport consumers, empty/upgrade/recovery migrations and SQL generation/contract checks as affected. `git diff --check` and scoped secret review.
- **Risks / recovery:** Preserve stable UUID/history and last validated publication; disable the changed source/flow or restore the projection with audit. Applied migrations are append-only with previous-schema upgrade/recovery tests; no data-destructive rollback, broader trust, privacy exception or hidden failure.
- **Definition of done:** Actual scoped evidence and task issue/atomic commit, integrated through its guarded phase PR; specialized exit once and required current-head/base Quick verification through finish, then merged-SHA wiki once. Planning is not LOCAL_DONE, integration is not G09 certification, and optional unselected tasks are never marked complete.

<a id="p25-t04"></a>

### P25-T04 — Canonical reconciliation and zero-price public reads

- **ID / priority / status:** P25-T04 / MUST / PLANNED.
- **Goal:** Publish verified catalog identities independently of price-survey participation.
- **Inputs / rules:** docs/planning/STATION_CATALOG_PLAN.md; docs/product/STATION_CATALOG.md B-BR-D01–D06/D12; BUC-D01/D05/D06; existing owning account/media/location/moderation/price contracts.
- **Dependencies:** P25-T03.
- **Files/areas expected:** Directory domain/application/repository/read/http; contracts/openapi/v1.yaml and shared fixtures; owned migration(s).
- **Risk / tests first:** critical; Concurrent CSV/API first creation, two nearby distinct CNPJs, succession, status chronology, missing-row no-closure, centroid no-proximity and no-price reads.
- **Implementation outline:** Reconcile staged assertions into stable UUIDs/identifier history/status projections; extend anonymous paginated list/detail with safe provenance/freshness.
- **Acceptance criteria:** Exactly one active identity per full validated identifier; no address-only merge or invented price/location; station with no price is searchable and detail-readable; old readers remain compatible.
- **Validation commands:** From backend/: `go test -race ./internal/modules/directory/...` and `go test -race -tags=integration ./internal/modules/directory/...` using the disposable environment in backend/README; actual changed job/schema/transport consumers, empty/upgrade/recovery migrations and SQL generation/contract checks as affected. `git diff --check` and scoped secret review.
- **Risks / recovery:** Preserve stable UUID/history and last validated publication; disable the changed source/flow or restore the projection with audit. Applied migrations are append-only with previous-schema upgrade/recovery tests; no data-destructive rollback, broader trust, privacy exception or hidden failure.
- **Definition of done:** Actual scoped evidence and task issue/atomic commit, integrated through its guarded phase PR; specialized exit once and required current-head/base Quick verification through finish, then merged-SHA wiki once. Planning is not LOCAL_DONE, integration is not G09 certification, and optional unselected tasks are never marked complete.

<a id="p25-t05"></a>

### P25-T05 — Registry jobs, outage recovery and phase acceptance

- **ID / priority / status:** P25-T05 / MUST / PLANNED.
- **Goal:** Prove resumable registry operations and safe publication under failure.
- **Inputs / rules:** docs/planning/STATION_CATALOG_PLAN.md; docs/product/STATION_CATALOG.md B-BR-D03/D05/D10/D12; BUC-D01/D06/D08; existing owning account/media/location/moderation/price contracts.
- **Dependencies:** P25-T04.
- **Files/areas expected:** Directory jobs and affected platform job tests; operator registry runbook and phase evidence.
- **Risk / tests first:** critical; Worker death, stale lease/fencing, duplicate enqueue, database outage, partial snapshot, retry exhaustion, late source and rollback of publication pointer.
- **Implementation outline:** Wire the proposed schedule after source-contract verification; add restricted pause/retry/quarantine/last-success controls and verify immediate risk tests.
- **Acceptance criteria:** G25 specialized exit recorded once; complete recovery conserves IDs and last-good state; counts/freshness and missing source guarantees visible; no Android visibility or production claim inferred.
- **Validation commands:** From backend/: `go test -race ./internal/modules/directory/...` and `go test -race -tags=integration ./internal/modules/directory/...` using the disposable environment in backend/README; actual changed job/schema/transport consumers, empty/upgrade/recovery migrations and SQL generation/contract checks as affected. `git diff --check` and scoped secret review.
- **Risks / recovery:** Preserve stable UUID/history and last validated publication; disable the changed source/flow or restore the projection with audit. Applied migrations are append-only with previous-schema upgrade/recovery tests; no data-destructive rollback, broader trust, privacy exception or hidden failure.
- **Definition of done:** Actual scoped evidence and task issue/atomic commit, integrated through its guarded phase PR; specialized exit once and required current-head/base Quick verification through finish, then merged-SHA wiki once. Planning is not LOCAL_DONE, integration is not G09 certification, and optional unselected tasks are never marked complete.

## P26 — DOU regulatory discovery

Priority: **MUST for catalog scope**. Entry: G25 integrated and INLABS access/rights available. Exit: **G26: deterministic regulatory-change ingestion and reviewed ambiguity/recovery**. State: PLANNED. Branch: `codex/phase-26-regulatory-discovery`.

<a id="p26-t01"></a>

### P26-T01 — Bounded INLABS editions and access

- **ID / priority / status:** P26-T01 / MUST / PLANNED.
- **Goal:** Fetch relevant editions safely without exposing source credentials.
- **Inputs / rules:** docs/planning/STATION_CATALOG_PLAN.md; docs/product/STATION_CATALOG.md B-BR-D03/D10/D11; BUC-D02; existing owning account/media/location/moderation/price contracts.
- **Dependencies:** G25 integrated and INLABS access/rights available.
- **Files/areas expected:** Directory DOU source adapter; operator secret configuration; synthetic/public minimized act fixtures.
- **Risk / tests first:** critical; Access denial, incomplete edition, XML entity/DTD/decompression attack, oversized text, duplicate edition and missing-day catch-up.
- **Implementation outline:** Document rights/access and use operator-configured credentials; bounded allowlisted XML/PDF discovery with edition identity and checkpoints.
- **Acceptance criteria:** No credentials or arbitrary remote URL in jobs/logs; absent access fails explicitly; only complete validated editions enter parsing; fixture-based tests do not hammer the source.
- **Validation commands:** From backend/: `go test -race ./internal/modules/directory/...` and `go test -race -tags=integration ./internal/modules/directory/...` using the disposable environment in backend/README; actual changed job/schema/transport consumers, empty/upgrade/recovery migrations and SQL generation/contract checks as affected. `git diff --check` and scoped secret review.
- **Risks / recovery:** Preserve stable UUID/history and last validated publication; disable the changed source/flow or restore the projection with audit. Applied migrations are append-only with previous-schema upgrade/recovery tests; no data-destructive rollback, broader trust, privacy exception or hidden failure.
- **Definition of done:** Actual scoped evidence and task issue/atomic commit, integrated through its guarded phase PR; specialized exit once and required current-head/base Quick verification through finish, then merged-SHA wiki once. Planning is not LOCAL_DONE, integration is not G09 certification, and optional unselected tasks are never marked complete.

<a id="p26-t02"></a>

### P26-T02 — ANP acts and chronological reconciliation

- **ID / priority / status:** P26-T02 / MUST / PLANNED.
- **Goal:** Turn relevant regulatory acts into verifiable assertions, not inferred openings.
- **Inputs / rules:** docs/planning/STATION_CATALOG_PLAN.md; docs/product/STATION_CATALOG.md B-BR-D02–D05/D10; BUC-D02/D06; existing owning account/media/location/moderation/price contracts.
- **Dependencies:** P26-T01.
- **Files/areas expected:** Directory pure act parser/application verifier/repository; edition/act contract vectors.
- **Risk / tests first:** critical; Multiple grants per act, unrelated products, amendment/republication, revocation before/after grant, operator change and unknown wording.
- **Implementation outline:** Parse deterministic identifiers/act types; verify official reference and reconcile through Directory; quarantine ambiguous language.
- **Acceptance criteria:** No unknown/free-text/LLM interpretation publishes authorization; effective chronology and corrections are preserved; duplicate acts converge; grant date never becomes inauguration date.
- **Validation commands:** From backend/: `go test -race ./internal/modules/directory/...` and `go test -race -tags=integration ./internal/modules/directory/...` using the disposable environment in backend/README; actual changed job/schema/transport consumers, empty/upgrade/recovery migrations and SQL generation/contract checks as affected. `git diff --check` and scoped secret review.
- **Risks / recovery:** Preserve stable UUID/history and last validated publication; disable the changed source/flow or restore the projection with audit. Applied migrations are append-only with previous-schema upgrade/recovery tests; no data-destructive rollback, broader trust, privacy exception or hidden failure.
- **Definition of done:** Actual scoped evidence and task issue/atomic commit, integrated through its guarded phase PR; specialized exit once and required current-head/base Quick verification through finish, then merged-SHA wiki once. Planning is not LOCAL_DONE, integration is not G09 certification, and optional unselected tasks are never marked complete.

<a id="p26-t03"></a>

### P26-T03 — Regulatory review, catch-up and phase acceptance

- **ID / priority / status:** P26-T03 / MUST / PLANNED.
- **Goal:** Operate regulatory discovery with audit and failure recovery.
- **Inputs / rules:** docs/planning/STATION_CATALOG_PLAN.md; docs/product/STATION_CATALOG.md B-BR-D03–D05/D10/D12; BUC-D02/D04/D06; existing owning account/media/location/moderation/price contracts.
- **Dependencies:** P26-T02.
- **Files/areas expected:** Existing moderation/operator commands through explicit ports; DOU runbook/evidence and affected job tests.
- **Risk / tests first:** critical; Edition outage/restart, conflicting CSV/API/DOU facts, review concurrency/role denial, late revocation and harmless replay.
- **Implementation outline:** Provide audited ambiguous-act review and bounded catch-up; record source freshness/lag and retry/escalation behavior.
- **Acceptance criteria:** G26 acceptance proves correct recovered state without duplicate or premature closure; unavailable access/facts remain pending and visible to operators; no national opening SLA asserted.
- **Validation commands:** From backend/: `go test -race ./internal/modules/directory/...` and `go test -race -tags=integration ./internal/modules/directory/...` using the disposable environment in backend/README; actual changed job/schema/transport consumers, empty/upgrade/recovery migrations and SQL generation/contract checks as affected. `git diff --check` and scoped secret review.
- **Risks / recovery:** Preserve stable UUID/history and last validated publication; disable the changed source/flow or restore the projection with audit. Applied migrations are append-only with previous-schema upgrade/recovery tests; no data-destructive rollback, broader trust, privacy exception or hidden failure.
- **Definition of done:** Actual scoped evidence and task issue/atomic commit, integrated through its guarded phase PR; specialized exit once and required current-head/base Quick verification through finish, then merged-SHA wiki once. Planning is not LOCAL_DONE, integration is not G09 certification, and optional unselected tasks are never marked complete.

## P27 — Account station intake and Android catalog

Priority: **MUST for catalog scope**. Entry: G26 integrated; existing P13/P15/P16 and moderation prerequisites verified. Exit: **G27: secure account intake, audited verification and actual Android catalog/suggestion visibility**. State: PLANNED. Branch: `codex/phase-27-station-intake`.

<a id="p27-t01"></a>

### P27-T01 — Signed station suggestions and private status

- **ID / priority / status:** P27-T01 / MUST / PLANNED.
- **Goal:** Let active free accounts propose a missing station or correction securely.
- **Inputs / rules:** docs/planning/STATION_CATALOG_PLAN.md; docs/product/STATION_CATALOG.md B-BR-D07–D10; BUC-D03/D04; existing owning account/media/location/moderation/price contracts.
- **Dependencies:** G26 integrated; existing P13/P15/P16 and moderation prerequisites verified.
- **Files/areas expected:** Directory application/intake/http/repository; explicit account/evidence ports; additive OpenAPI/fixtures/migrations.
- **Risk / tests first:** critical; Anonymous contributor proof, revoked account/session, forged/replayed signature, IDOR, foreign evidence, quota, same-key changed-body and parallel same-station suggestions.
- **Implementation outline:** Freeze structured input/quotas/retention and statuses before handlers; atomically store request, idempotency result and verification job; expose owner-only no-store status.
- **Acceptance criteria:** Free active accounts can submit; no proof bypass or private leak; concurrency preserves each request but converges on one canonical station; never grants ownership from CNPJ.
- **Validation commands:** Directory targeted unit/race + real-PostGIS integration and every affected account/evidence/moderation/privacy/job consumer, with failure/ownership/expiry tests; actual package list frozen at opening. Affected OpenAPI/golden/compatibility checks; `git diff --check` and scoped secret review.
- **Risks / recovery:** Preserve stable UUID/history and last validated publication; disable the changed source/flow or restore the projection with audit. Applied migrations are append-only with previous-schema upgrade/recovery tests; no data-destructive rollback, broader trust, privacy exception or hidden failure.
- **Definition of done:** Actual scoped evidence and task issue/atomic commit, integrated through its guarded phase PR; specialized exit once and required current-head/base Quick verification through finish, then merged-SHA wiki once. Planning is not LOCAL_DONE, integration is not G09 certification, and optional unselected tasks are never marked complete.

<a id="p27-t02"></a>

### P27-T02 — Verified decisions, corrections and reviewed locations

- **ID / priority / status:** P27-T02 / MUST / PLANNED.
- **Goal:** Resolve exact official matches and review remaining conflicts through audited authority.
- **Inputs / rules:** docs/planning/STATION_CATALOG_PLAN.md; docs/product/STATION_CATALOG.md B-BR-D02/D04–D10; BUC-D03/D04/D06; existing owning account/media/location/moderation/price contracts.
- **Dependencies:** P27-T01.
- **Files/areas expected:** Directory verifier/review commands and location revisions; existing moderation/evidence/privacy interfaces.
- **Risk / tests first:** critical; Exact match versus conflicting address/status, fake pin/centroid, reviewer role denial/stale decision, evidence expiry/rebinding, cancellation/appeal and concurrent source correction.
- **Implementation outline:** Freeze public eligibility and review transitions; automate only approved exact official cases; create audited decisions and reviewed geometry through existing location ports.
- **Acceptance criteria:** Unreviewed suggestion remains private; no arbitrary pin is canonical; only eligible catalog facts publish; 24h expiry survives retries/review; stale decisions fail safely and minimal decision data has retention.
- **Validation commands:** Directory targeted unit/race + real-PostGIS integration and every affected account/evidence/moderation/privacy/job consumer, with failure/ownership/expiry tests; actual package list frozen at opening. Affected OpenAPI/golden/compatibility checks; `git diff --check` and scoped secret review.
- **Risks / recovery:** Preserve stable UUID/history and last validated publication; disable the changed source/flow or restore the projection with audit. Applied migrations are append-only with previous-schema upgrade/recovery tests; no data-destructive rollback, broader trust, privacy exception or hidden failure.
- **Definition of done:** Actual scoped evidence and task issue/atomic commit, integrated through its guarded phase PR; specialized exit once and required current-head/base Quick verification through finish, then merged-SHA wiki once. Planning is not LOCAL_DONE, integration is not G09 certification, and optional unselected tasks are never marked complete.

<a id="p27-t03"></a>

### P27-T03 — Server catalog, offline cache and canonical action targets

- **ID / priority / status:** P27-T03 / MUST / PLANNED.
- **Goal:** Make stations without survey prices visible in the app and resolve Directory UUIDs for existing actions.
- **Inputs / rules:** docs/planning/STATION_CATALOG_PLAN.md; docs/product/STATION_CATALOG.md B-BR-D01/D02/D04/D06/D07; BUC-D05; existing owning account/media/location/moderation/price contracts.
- **Dependencies:** P27-T02; new backend intake/read contracts integrated before their consumer.
- **Files/areas expected:** Existing Kotlin domain/application/data/app discovery, HTTP adapters, Room cache and affected social/contribution consumers; shared contracts.
- **Risk / tests first:** critical; No-price station, missing coordinates, stale/unknown status, cursor/filter boundaries, legacy full CNPJ to UUID, missing/retired target, offline/refresh failure and cache upgrade.
- **Implementation outline:** Add portable station DTO/read port; integrate bounded server catalog with distinct nullable price sections and compatible local caching; use verified full-CNPJ resolver where legacy data needs a UUID.
- **Acceptance criteria:** Explore/detail shows registry-only station honestly; no price-row fabrication or CNPJ-as-UUID; unchanged expert tools preserved; contributions/comments/ratings/reports use a valid existing target; sorting missing prices is deterministic and explicit.
- **Validation commands:** Root: affected `:domain:test`, `:application:test`, `:data:testDebugUnitTest`, `:app:testDebugUnitTest`, `:app:assembleDebug`; actual affected Room/device instrumentation and backend contract/PostGIS checks. New schema migration and canonical-target negative cases mandatory. `git diff --check` and scoped secret review.
- **Risks / recovery:** Preserve stable UUID/history and last validated publication; disable the changed source/flow or restore the projection with audit. Applied migrations are append-only with previous-schema upgrade/recovery tests; no data-destructive rollback, broader trust, privacy exception or hidden failure.
- **Definition of done:** Actual scoped evidence and task issue/atomic commit, integrated through its guarded phase PR; specialized exit once and required current-head/base Quick verification through finish, then merged-SHA wiki once. Planning is not LOCAL_DONE, integration is not G09 certification, and optional unselected tasks are never marked complete.

<a id="p27-t04"></a>

### P27-T04 — Lightweight suggest-correct-status Android journey

- **ID / priority / status:** P27-T04 / MUST / PLANNED.
- **Goal:** Offer a clear free-account missing-station journey with progressive permissions.
- **Inputs / rules:** docs/planning/STATION_CATALOG_PLAN.md; docs/product/STATION_CATALOG.md B-BR-D06–D09; BUC-D03/D04/D05; existing owning account/media/location/moderation/price contracts.
- **Dependencies:** P27-T03; new backend intake/read contracts integrated before their consumer.
- **Files/areas expected:** Existing account/navigation/capture/outbox/status Kotlin and Android modules; portable state fixtures.
- **Risk / tests first:** critical; Guest sign-in, permission denial, duplicate hint, no-photo case, optional photo optimization, offline replay, revoked session and owner status recovery.
- **Implementation outline:** Implement search-before-submit, structured form, optional existing capture/evidence pipeline, reviewed confirmation and private status/correction journey; freeze transport retry semantics.
- **Acceptance criteria:** No duplicate visible station from offline retry; no background permission demand or long-lived photo; user sees pending/reviewed/unresolved truth; backend T01/T02 and catalog T03 integrate before this consumer.
- **Validation commands:** Root: affected `:domain:test`, `:application:test`, `:data:testDebugUnitTest`, `:app:testDebugUnitTest`, `:app:assembleDebug`; actual affected Room/device instrumentation and backend contract/PostGIS checks. New schema migration and canonical-target negative cases mandatory. `git diff --check` and scoped secret review.
- **Risks / recovery:** Preserve stable UUID/history and last validated publication; disable the changed source/flow or restore the projection with audit. Applied migrations are append-only with previous-schema upgrade/recovery tests; no data-destructive rollback, broader trust, privacy exception or hidden failure.
- **Definition of done:** Actual scoped evidence and task issue/atomic commit, integrated through its guarded phase PR; specialized exit once and required current-head/base Quick verification through finish, then merged-SHA wiki once. Planning is not LOCAL_DONE, integration is not G09 certification, and optional unselected tasks are never marked complete.

<a id="p27-t05"></a>

### P27-T05 — Intake abuse, privacy and lifecycle acceptance

- **ID / priority / status:** P27-T05 / MUST / PLANNED.
- **Goal:** Prove new source and account workflows together under failure.
- **Inputs / rules:** docs/planning/STATION_CATALOG_PLAN.md; docs/product/STATION_CATALOG.md B-BR-D01–D12; BUC-D03–D06; existing owning account/media/location/moderation/price contracts.
- **Dependencies:** P27-T04; new backend intake/read contracts integrated before their consumer.
- **Files/areas expected:** Affected Directory/account/evidence/moderation/privacy/platform and Android tests; P27 phase evidence.
- **Risk / tests first:** critical; Coordinated suggestions, signed account abuse, source race, expired evidence, erased/suspended author, restore deletion replay, offline retry and existing price/social regression.
- **Implementation outline:** Run targeted cross-module PostGIS/race/security/media and device checks, reconcile pending cases and document operator review capacity.
- **Acceptance criteria:** G27 scope passes with genuine required storage/device evidence; no missing row inferred green; public projections exclude private data and remain independent from prices/feedback trust.
- **Validation commands:** Immediate targeted backend real-PostGIS/race/account/privacy/storage suites plus affected Gradle/unit/compile/real-device acceptance from the catalog plan; freeze actual package/device/budget selection first. `git diff --check` and scoped secret review. No complete production certification here.
- **Risks / recovery:** Preserve stable UUID/history and last validated publication; disable the changed source/flow or restore the projection with audit. Applied migrations are append-only with previous-schema upgrade/recovery tests; no data-destructive rollback, broader trust, privacy exception or hidden failure.
- **Definition of done:** Actual scoped evidence and task issue/atomic commit, integrated through its guarded phase PR; specialized exit once and required current-head/base Quick verification through finish, then merged-SHA wiki once. Planning is not LOCAL_DONE, integration is not G09 certification, and optional unselected tasks are never marked complete.

## P28 — Optional licensed coverage complements

Priority: **SHOULD / SELECTED_ONLY**. Entry: G27 plus measured gap, explicit selected task range and rights/access decision. Exit: **G28 for selected tasks only: permitted source/representation implemented, attributed and recoverable**. State: PLANNED / optional tasks DEFERRED until selected. Branch: `codex/phase-28-catalog-complements`.

<a id="p28-t01"></a>

### P28-T01 — Source selection and rights assessment

- **ID / priority / status:** P28-T01 / SHOULD, selected scope only / DEFERRED until explicit selection.
- **Goal:** Choose a complementary source only if core coverage/accuracy measurements justify it.
- **Inputs / rules:** docs/planning/STATION_CATALOG_PLAN.md; docs/product/STATION_CATALOG.md B-BR-D06/D11/D12; BUC-D07; existing owning account/media/location/moderation/price contracts.
- **Dependencies:** G27 plus measured gap, explicit selected task range and rights/access decision.
- **Files/areas expected:** Source decision/ADR if needed; measured coverage/cost/rights records.
- **Risk / tests first:** docs; Representative coverage gaps, retained-field rights, attribution obligations, supplier outage and independent authorization limits.
- **Implementation outline:** Compare OSM extracts, contractable POI, partner feeds and no additional provider; record persistence/redistribution terms and budget.
- **Acceptance criteria:** One explicit selected scope or DEFERRED decision; no purchase or nationwide scrape inferred; legal/data-rights obligations and unavailable guarantees stay explicit.
- **Validation commands:** Scoped Markdown links, rule/use-case/task/dependency consistency and explicit new-file whitespace/secret review; `git diff --check`. No runtime gate inferred from fixtures or docs.
- **Risks / recovery:** Preserve stable UUID/history and last validated publication; disable the changed source/flow or restore the projection with audit. Applied migrations are append-only with previous-schema upgrade/recovery tests; no data-destructive rollback, broader trust, privacy exception or hidden failure.
- **Definition of done:** Actual scoped evidence and task issue/atomic commit, integrated through its guarded phase PR; specialized exit once and required current-head/base Quick verification through finish, then merged-SHA wiki once. Planning is not LOCAL_DONE, integration is not G09 certification, and optional unselected tasks are never marked complete.

<a id="p28-t02"></a>

### P28-T02 — Selected geometry or POI candidate adapter

- **ID / priority / status:** P28-T02 / SHOULD, selected scope only / DEFERRED until explicit selection.
- **Goal:** Supplement candidate location/identity with only a permitted selected dataset.
- **Inputs / rules:** docs/planning/STATION_CATALOG_PLAN.md; docs/product/STATION_CATALOG.md B-BR-D02–D06/D10/D11; BUC-D07; existing owning account/media/location/moderation/price contracts.
- **Dependencies:** P28-T01 selection for this task and G27; a different optional adapter/representation task is not an automatic prerequisite.
- **Files/areas expected:** One selected Directory source adapter and source-specific staging/location review; attribution/public projection if permitted.
- **Risk / tests first:** critical; POI duplicates/conflicts, centroid/bad CRS, stale/removed map item, bounded extract/diff replay and provider shutdown.
- **Implementation outline:** Implement only the chosen feed, quotas/diffs/provenance/attribution and review; keep restricted provider fields outside unrestricted views.
- **Acceptance criteria:** No restricted copy or unreviewed precise point; one-source failure does not delete official facts; disabled adapter leaves stable IDs and honest freshness; task stays deferred without selected rights.
- **Validation commands:** From backend/: `go test -race ./internal/modules/directory/...` and `go test -race -tags=integration ./internal/modules/directory/...` using the disposable environment in backend/README; actual changed job/schema/transport consumers, empty/upgrade/recovery migrations and SQL generation/contract checks as affected. `git diff --check` and scoped secret review.
- **Risks / recovery:** Preserve stable UUID/history and last validated publication; disable the changed source/flow or restore the projection with audit. Applied migrations are append-only with previous-schema upgrade/recovery tests; no data-destructive rollback, broader trust, privacy exception or hidden failure.
- **Definition of done:** Actual scoped evidence and task issue/atomic commit, integrated through its guarded phase PR; specialized exit once and required current-head/base Quick verification through finish, then merged-SHA wiki once. Planning is not LOCAL_DONE, integration is not G09 certification, and optional unselected tasks are never marked complete.

<a id="p28-t03"></a>

### P28-T03 — Authenticated partner opening-change feed

- **ID / priority / status:** P28-T03 / SHOULD, selected scope only / DEFERRED until explicit selection.
- **Goal:** Accept a real contracted partner feed with narrowly defined authority.
- **Inputs / rules:** docs/planning/STATION_CATALOG_PLAN.md; docs/product/STATION_CATALOG.md B-BR-D02–D05/D07/D10/D11; BUC-D07; existing owning account/media/location/moderation/price contracts.
- **Dependencies:** P28-T01 selection for this task and G27; a different optional adapter/representation task is not an automatic prerequisite.
- **Files/areas expected:** Bounded Directory partner adapter; operator credential/contract scope and audit; fixtures.
- **Risk / tests first:** critical; Forged/rotated credentials, replay/out-of-order event, tenant-like scope confusion, wrong branch CNPJ, partner correction and independent ANP verification.
- **Implementation outline:** Freeze partner schema/auth/event semantics from an actual agreement; implement explicit partner scope and verification via existing catalog pipeline.
- **Acceptance criteria:** Partner claim never grants regulatory status or arbitrary catalog editing; replay is idempotent and correction auditable; no partner service without measured need/agreement.
- **Validation commands:** Directory targeted unit/race + real-PostGIS integration and every affected account/evidence/moderation/privacy/job consumer, with failure/ownership/expiry tests; actual package list frozen at opening. Affected OpenAPI/golden/compatibility checks; `git diff --check` and scoped secret review.
- **Risks / recovery:** Preserve stable UUID/history and last validated publication; disable the changed source/flow or restore the projection with audit. Applied migrations are append-only with previous-schema upgrade/recovery tests; no data-destructive rollback, broader trust, privacy exception or hidden failure.
- **Definition of done:** Actual scoped evidence and task issue/atomic commit, integrated through its guarded phase PR; specialized exit once and required current-head/base Quick verification through finish, then merged-SHA wiki once. Planning is not LOCAL_DONE, integration is not G09 certification, and optional unselected tasks are never marked complete.

<a id="p28-t04"></a>

### P28-T04 — Independent owner representation verification

- **ID / priority / status:** P28-T04 / historical optional task / SUPERSEDED_BY_P30_P33 (2026-10-02).
- **Reason:** Maintainer explicitly requested the complete station profile/entity and verified representation workflow. The bounded optional idea is now fully owned by [P30–P33](#station-profile-and-representation), [profile plan](docs/planning/STATION_PROFILE_PLAN.md) and [target rules](docs/product/STATION_PROFILE_REPRESENTATION.md).
- **Recovery / delivery:** Preserve this ID and historical catalog scope; no implementation/issue/acceptance is complete. Do not open a duplicate owner-verification issue or mark this task DONE. P28-T01–T03 remain optional sources/partner feeds; partner credentials never grant profile administration.

## P29 — Catalog capacity and refreshed Android acceptance

Priority: **MUST for catalog scope**. Entry: G27; every selected enabled G28 task integrated; applicable G24 evidence mapped to unchanged inputs. Exit: **G29-CATALOG: measured catalog workload and complete affected Android acceptance, ready for separate P09/G09 certification**. State: PLANNED. Branch: `codex/phase-29-catalog-acceptance`.

<a id="p29-t01"></a>

### P29-T01 — Capacity, freshness and review-backlog campaign

- **ID / priority / status:** P29-T01 / MUST / PLANNED.
- **Goal:** Freeze and measure national-scale ingestion with simultaneous reads/writes and dependency recovery.
- **Inputs / rules:** docs/planning/STATION_CATALOG_PLAN.md; docs/product/STATION_CATALOG.md B-BR-D02/D03/D05/D10/D12; BUC-D06/D08; existing owning account/media/location/moderation/price contracts.
- **Dependencies:** G27; every selected enabled G28 task integrated; applicable G24 evidence mapped to unchanged inputs.
- **Files/areas expected:** Synthetic load/clock fixtures, existing operator/load harness extensions, source/queue metrics and phase evidence.
- **Risk / tests first:** critical; 100k synthetic stations/10k novel assertions per day hypothesis, 1k inputs per 10min burst, 48h outage/catch-up, duplicates, lease failure, query plans and manual-review backlog.
- **Implementation outline:** Freeze actual workload/configuration/latency/memory/recovery thresholds before campaign; use fixture upstreams; measure source lag separately from eligibility-to-publication lag.
- **Acceptance criteria:** Counts reconcile with no dropped input/identity split/partial publication; accepted predeclared read/resource/recovery budgets pass or remediation is required; 24/48h results and unresolved denominator reported without claiming live SLA.
- **Validation commands:** Directory targeted unit/race + real-PostGIS integration and every affected account/evidence/moderation/privacy/job consumer, with failure/ownership/expiry tests; actual package list frozen at opening. Affected OpenAPI/golden/compatibility checks; `git diff --check` and scoped secret review.
- **Risks / recovery:** Preserve stable UUID/history and last validated publication; disable the changed source/flow or restore the projection with audit. Applied migrations are append-only with previous-schema upgrade/recovery tests; no data-destructive rollback, broader trust, privacy exception or hidden failure.
- **Definition of done:** Actual scoped evidence and task issue/atomic commit, integrated through its guarded phase PR; specialized exit once and required current-head/base Quick verification through finish, then merged-SHA wiki once. Planning is not LOCAL_DONE, integration is not G09 certification, and optional unselected tasks are never marked complete.

<a id="p29-t02"></a>

### P29-T02 — Source-to-app lifecycle and affected device reacceptance

- **ID / priority / status:** P29-T02 / MUST / PLANNED.
- **Goal:** Accept the new immutable Android/backend candidate on affected real flows.
- **Inputs / rules:** docs/planning/STATION_CATALOG_PLAN.md; docs/product/STATION_CATALOG.md B-BR-D01–D12; BUC-D01–D08 applicable to enabled scope; existing owning account/media/location/moderation/price contracts.
- **Dependencies:** P29-T01.
- **Files/areas expected:** Affected existing Android/backend E2E tests and P24 device/accessibility/performance manifest; phase evidence.
- **Risk / tests first:** critical; Registry-only station to suggestion/review/location/price/comment/report, closed/stale source, offline replay, unknown states, denied GPS, photo expiry and low-end resources.
- **Implementation outline:** Map old G24 proof to matching inputs; re-run every changed required functional/device/security/accessibility/performance row and correct failures.
- **Acceptance criteria:** Changed catalog/intake/social-target flows have real acceptance evidence; no cached-home timing substitutes cold-process proof; no Android support/storage/auth gap waived; iOS remains deferred.
- **Validation commands:** Immediate targeted backend real-PostGIS/race/account/privacy/storage suites plus affected Gradle/unit/compile/real-device acceptance from the catalog plan; freeze actual package/device/budget selection first. `git diff --check` and scoped secret review. No complete production certification here.
- **Risks / recovery:** Preserve stable UUID/history and last validated publication; disable the changed source/flow or restore the projection with audit. Applied migrations are append-only with previous-schema upgrade/recovery tests; no data-destructive rollback, broader trust, privacy exception or hidden failure.
- **Definition of done:** Actual scoped evidence and task issue/atomic commit, integrated through its guarded phase PR; specialized exit once and required current-head/base Quick verification through finish, then merged-SHA wiki once. Planning is not LOCAL_DONE, integration is not G09 certification, and optional unselected tasks are never marked complete.

<a id="p29-t03"></a>

### P29-T03 — Pinned catalog candidate and production handoff

- **ID / priority / status:** P29-T03 / MUST / PLANNED.
- **Goal:** Provide an auditable integrated catalog candidate and operating model for existing release phases.
- **Inputs / rules:** docs/planning/STATION_CATALOG_PLAN.md; docs/product/STATION_CATALOG.md B-BR-D03/D05/D09–D12; BUC-D06/D08; existing owning account/media/location/moderation/price contracts.
- **Dependencies:** P29-T02.
- **Files/areas expected:** Catalog phase evidence/manifest; operator runbooks; P09/G09 and P10-T09 prerequisite notes.
- **Risk / tests first:** critical; Source pause/retry/review/disable, binary/projection rollback, migration recovery, privacy deletion-ledger restore and staffed backlog drill.
- **Implementation outline:** Collect exact candidate/config/check/limitations, support staffing and retention/cost measures; perform scoped recovery drills and define remaining real production checks.
- **Acceptance criteria:** G29 acceptance accounts for all enabled sources/tasks and required rows; actual launch waits for G09 certification and separate deployment authorization; optional-source exclusions and no nationwide inauguration promise explicit.
- **Validation commands:** Directory targeted unit/race + real-PostGIS integration and every affected account/evidence/moderation/privacy/job consumer, with failure/ownership/expiry tests; actual package list frozen at opening. Affected OpenAPI/golden/compatibility checks; `git diff --check` and scoped secret review.
- **Risks / recovery:** Preserve stable UUID/history and last validated publication; disable the changed source/flow or restore the projection with audit. Applied migrations are append-only with previous-schema upgrade/recovery tests; no data-destructive rollback, broader trust, privacy exception or hidden failure.
- **Definition of done:** Actual scoped evidence and task issue/atomic commit, integrated through its guarded phase PR; specialized exit once and required current-head/base Quick verification through finish, then merged-SHA wiki once. Planning is not LOCAL_DONE, integration is not G09 certification, and optional unselected tasks are never marked complete.

## Station profile and representation — PLANNED

<a id="station-profile-and-representation"></a>

[STATION_PROFILE_PLAN](docs/planning/STATION_PROFILE_PLAN.md) defines the full process; [STATION_PROFILE_REPRESENTATION](docs/product/STATION_PROFILE_REPRESENTATION.md) owns B-BR-P01–P16 / BUC-P01–P10; [ADR-017](docs/adr/017-station-profile-and-verified-representation.md) records identity/authority boundaries. Reuse Directory Station; new profile/claim/grant entities do not duplicate station IDs. Sequence after app-first P34–P38 and catalog checkpoints: G29 source checkpoint → P30 → P31 → P32 → P33 → end affected Android manual/device acceptance → final integration → existing P09/G09. P28-T04 is superseded, preserving its ID; no duplicate owner-verification implementation. All tasks below are PLANNED, local-only documentation, not runtime readiness or remote issues.

## P30 — Station profile and private claim foundations

Priority: **MUST for selected profile scope**. Entry: G29-CATALOG validated source checkpoint (full end device acceptance remains owed); actual Directory/account/moderation/media contracts reconciled. Exit: **G30: entities, unclaimed profile reads, signed claims and bounded private proof intake; no management authority yet**. State: PLANNED. Branch: `codex/phase-30-station-profiles`.

<a id="p30-t01"></a>

### P30-T01 — Entity, proof, permission and privacy contract freeze

- **ID / priority / status:** P30-T01 / MUST for selected profile scope / PLANNED.
- **Goal:** Freeze Station/Profile/operator/claim/proof/grant ownership, safe states and equivalent representation criteria.
- **Inputs / rules:** docs/planning/STATION_PROFILE_PLAN.md; docs/product/STATION_PROFILE_REPRESENTATION.md B-BR-P01–P08/P10/P15/P16; BUC-P01–P04/P09; ADR-017 and existing account/catalog/media/feedback/moderation contracts.
- **Dependencies:** G29-CATALOG integrated; actual Directory/account/moderation/media contracts reconciled.
- **Files/areas expected:** docs/product/STATION_PROFILE_REPRESENTATION.md; docs/backend/API_PLAN.md and DATA_MODEL.md; privacy inventory; permitted verifier decision and portable synthetic fixtures.
- **Risk / tests first:** docs; Full branch versus matrix, homonymous identity, joint authority, signature vs powers, claim-state transitions, document/scan categories and unknown provider capabilities.
- **Implementation outline:** Freeze wire enums, additional-auth/delegation policy, claim nonce/TTL/input/quota limits, field/scopes and private retention/backup handling; record source/provider license/security/access decisions.
- **Acceptance criteria:** No implementation starts with undefined proof-time/revocation/retention or broad OWNER role; official sources and document paths have permitted access and safe unresolved states.
- **Validation commands:** Scoped entity/rule/task/transition/source/dependency/Markdown consistency, new-file whitespace and secret review; `git diff --check`. A schema/handler/gate disguised as docs needs actual behavioral checks.
- **Risks / recovery:** Deny invalid/stale authority, retain stable station IDs/community facts, disable the changed capability or revert only safe projection/binary behavior. Applied migrations are append-only with recovery; restore replays deletion/revocation and purges expired proof; no rollback restores lost privileges or private evidence.
- **Definition of done:** Actual scoped evidence, one task issue/atomic commit, guarded phase integration and merged-SHA wiki once. Specialized exit plus required current-head/base Quick verification through `finish --required "Quick verification" --pr <actual-number>`; no skipped/failed/missing checks. Planning is not LOCAL_DONE; integrated profile is not G09 RELEASE_CERTIFIED. iOS stays explicitly deferred.

<a id="p30-t02"></a>

### P30-T02 — Canonical operator link and public unclaimed profile

- **ID / priority / status:** P30-T02 / MUST for selected profile scope / PLANNED.
- **Goal:** Extend the existing Station with a distinct profile and effective operator reference without duplicate station identity.
- **Inputs / rules:** docs/planning/STATION_PROFILE_PLAN.md; docs/product/STATION_PROFILE_REPRESENTATION.md B-BR-P01/P02/P11/P12/P15/P16; BUC-P01/P08; ADR-017 and existing account/catalog/media/feedback/moderation contracts.
- **Dependencies:** P30-T01.
- **Files/areas expected:** Directory declared operator read/revision port; new stationprofile domain/repository/public read DTO; owned SQL/migrations/OpenAPI/golden vectors.
- **Risk / tests first:** critical; Zero-price/no-location/no-representative reads, stale operator, unique station profile, same CNPJ root distinct branches, cursor/cache/old-reader compatibility and previous-schema upgrade.
- **Implementation outline:** Add minimal profile/operator-applicability schema and anonymous read projection with separated official/community/business fields; use existing canonical UUID and explicit ports.
- **Acceptance criteria:** No duplicate station or ownership assumption; profile exists read-only without grants; no private proof/person data in public DTO/cache; nullable facts and provenance honest; real migration/PostGIS tests pass.
- **Validation commands:** Use actual affected existing Directory/account/moderation/evidence/feedback/privacy/job suites and, after creation, stationprofile unit/race and disposable real-PostGIS integration from backend/README. Required auth/ownership/failure/concurrency tests immediately; changed SQL requires empty/upgrade/recovery and query-plan proof. Affected OpenAPI/golden/compatibility checks; `git diff --check` and scoped secret review. Freeze actual package/command list at opening; nonexistent verifier tests are not evidence.
- **Risks / recovery:** Deny invalid/stale authority, retain stable station IDs/community facts, disable the changed capability or revert only safe projection/binary behavior. Applied migrations are append-only with recovery; restore replays deletion/revocation and purges expired proof; no rollback restores lost privileges or private evidence.
- **Definition of done:** Actual scoped evidence, one task issue/atomic commit, guarded phase integration and merged-SHA wiki once. Specialized exit plus required current-head/base Quick verification through `finish --required "Quick verification" --pr <actual-number>`; no skipped/failed/missing checks. Planning is not LOCAL_DONE; integrated profile is not G09 RELEASE_CERTIFIED. iOS stays explicitly deferred.

<a id="p30-t03"></a>

### P30-T03 — Account-bound claim and one-use declaration

- **ID / priority / status:** P30-T03 / MUST for selected profile scope / PLANNED.
- **Goal:** Create private representation requests with server-bound exact authorization content.
- **Inputs / rules:** docs/planning/STATION_PROFILE_PLAN.md; docs/product/STATION_PROFILE_REPRESENTATION.md B-BR-P03/P04/P07/P09/P16; BUC-P02/P03; ADR-017 and existing account/catalog/media/feedback/moderation contracts.
- **Dependencies:** P30-T02.
- **Files/areas expected:** stationprofile domain/application/signed HTTP/private read/repository; explicit account/key ports; OpenAPI/fixtures and transaction/job schema.
- **Risk / tests first:** critical; Guest/anonymous proof, expired/revoked session, account substitution, claim IDOR, same-key changed payload, challenge reissue/replay and parallel competing requests.
- **Implementation outline:** Generate versioned exact declaration and nonce digest bound to account/station/operator/scopes; atomically store claim/idempotency/job intent; expose owner-only no-store status/export.
- **Acceptance criteria:** No grant from submission, CNPJ or client flags; one-use/idempotency/account ownership enforced under races; competing cases stay private; no source fetch under DB lock.
- **Validation commands:** Use actual affected existing Directory/account/moderation/evidence/feedback/privacy/job suites and, after creation, stationprofile unit/race and disposable real-PostGIS integration from backend/README. Required auth/ownership/failure/concurrency tests immediately; changed SQL requires empty/upgrade/recovery and query-plan proof. Affected OpenAPI/golden/compatibility checks; `git diff --check` and scoped secret review. Freeze actual package/command list at opening; nonexistent verifier tests are not evidence.
- **Risks / recovery:** Deny invalid/stale authority, retain stable station IDs/community facts, disable the changed capability or revert only safe projection/binary behavior. Applied migrations are append-only with recovery; restore replays deletion/revocation and purges expired proof; no rollback restores lost privileges or private evidence.
- **Definition of done:** Actual scoped evidence, one task issue/atomic commit, guarded phase integration and merged-SHA wiki once. Specialized exit plus required current-head/base Quick verification through `finish --required "Quick verification" --pr <actual-number>`; no skipped/failed/missing checks. Planning is not LOCAL_DONE; integrated profile is not G09 RELEASE_CERTIFIED. iOS stays explicitly deferred.

<a id="p30-t04"></a>

### P30-T04 — Private immutable signed-document intake and expiry

- **ID / priority / status:** P30-T04 / MUST for selected profile scope / PLANNED.
- **Goal:** Accept allowed proof bytes safely without treating PDF as an optimized photo or storing private keys.
- **Inputs / rules:** docs/planning/STATION_PROFILE_PLAN.md; docs/product/STATION_PROFILE_REPRESENTATION.md B-BR-P05/P08/P15/P16; BUC-P03/P09; ADR-017 and existing account/catalog/media/feedback/moderation contracts.
- **Dependencies:** P30-T03.
- **Files/areas expected:** Explicit private document/evidence capability and stationprofile ports; bounded parser/storage adapters; permission/expiry/delete/restore fixtures.
- **Risk / tests first:** critical; PFX/P12/private key, malformed/oversize/encrypted/active PDF, embedded scans, magic mismatch, arbitrary URL/SSRF, foreign claim proof, overwrite race, expiry and restore.
- **Implementation outline:** Use private server-generated immutable keys and exact-byte hashes; reject unsafe formats; enforce frozen photo/scan 24h cap and signed-authorization retention across copies; restricted owner/reviewer status only.
- **Acceptance criteria:** No public bytes/URLs/key/password intake; exact signed bytes preserved for verifier; query-time denial plus physical all-copy cleanup tested; missing document retention/storage enforcement blocks intake, not approval fallback.
- **Validation commands:** Use actual affected existing Directory/account/moderation/evidence/feedback/privacy/job suites and, after creation, stationprofile unit/race and disposable real-PostGIS integration from backend/README. Required auth/ownership/failure/concurrency tests immediately; changed SQL requires empty/upgrade/recovery and query-plan proof. Affected OpenAPI/golden/compatibility checks; `git diff --check` and scoped secret review. Freeze actual package/command list at opening; nonexistent verifier tests are not evidence.
- **Risks / recovery:** Deny invalid/stale authority, retain stable station IDs/community facts, disable the changed capability or revert only safe projection/binary behavior. Applied migrations are append-only with recovery; restore replays deletion/revocation and purges expired proof; no rollback restores lost privileges or private evidence.
- **Definition of done:** Actual scoped evidence, one task issue/atomic commit, guarded phase integration and merged-SHA wiki once. Specialized exit plus required current-head/base Quick verification through `finish --required "Quick verification" --pr <actual-number>`; no skipped/failed/missing checks. Planning is not LOCAL_DONE; integrated profile is not G09 RELEASE_CERTIFIED. iOS stays explicitly deferred.

## P31 — Verified representation, capabilities and lifecycle

Priority: **MUST for selected profile scope**. Entry: G30 integrated and selected signature/source access proved. Exit: **G31: independently verified authority, reviewed scoped grants and complete privileged lifecycle**. State: PLANNED. Branch: `codex/phase-31-verified-representation`.

<a id="p31-t01"></a>

### P31-T01 — Independent signature and declaration verification

- **ID / priority / status:** P31-T01 / MUST for selected profile scope / PLANNED.
- **Goal:** Verify cryptographic proof and exact server declaration using permitted maintained tooling.
- **Inputs / rules:** docs/planning/STATION_PROFILE_PLAN.md; docs/product/STATION_PROFILE_REPRESENTATION.md B-BR-P04–P08/P16; BUC-P03; ADR-017 and existing account/catalog/media/feedback/moderation contracts.
- **Dependencies:** G30 integrated and selected signature/source access proved.
- **Files/areas expected:** stationprofile pure verification-result policy and bounded signature adapter/operator validation entry; source trust/format fixtures.
- **Risk / tests first:** critical; Forged/test root in release, revocation/expiry/unknown status, wrong signed account/station/CNPJ, unsigned appended semantic changes, report screenshot forgery and source outage.
- **Implementation outline:** Implement approved verifier interface or independent restricted validation process; compare extracted signed content to immutable declaration; record safe indeterminate/invalid/valid results and consume binding correctly.
- **Acceptance criteria:** Valid signature fixture proves recognized trust separately from synthetic roots; invalid/indeterminate proof cannot approve; signature alone never grants powers; external validation outside locks with bounded resources.
- **Validation commands:** Use actual affected existing Directory/account/moderation/evidence/feedback/privacy/job suites and, after creation, stationprofile unit/race and disposable real-PostGIS integration from backend/README. Required auth/ownership/failure/concurrency tests immediately; changed SQL requires empty/upgrade/recovery and query-plan proof. Affected OpenAPI/golden/compatibility checks; `git diff --check` and scoped secret review. Freeze actual package/command list at opening; nonexistent verifier tests are not evidence.
- **Risks / recovery:** Deny invalid/stale authority, retain stable station IDs/community facts, disable the changed capability or revert only safe projection/binary behavior. Applied migrations are append-only with recovery; restore replays deletion/revocation and purges expired proof; no rollback restores lost privileges or private evidence.
- **Definition of done:** Actual scoped evidence, one task issue/atomic commit, guarded phase integration and merged-SHA wiki once. Specialized exit plus required current-head/base Quick verification through `finish --required "Quick verification" --pr <actual-number>`; no skipped/failed/missing checks. Planning is not LOCAL_DONE; integrated profile is not G09 RELEASE_CERTIFIED. iOS stays explicitly deferred.

<a id="p31-t02"></a>

### P31-T02 — Operating-company and corporate authority verification

- **ID / priority / status:** P31-T02 / MUST for selected profile scope / PLANNED.
- **Goal:** Establish applicant linkage and sufficient powers for the exact operating establishment.
- **Inputs / rules:** docs/planning/STATION_PROFILE_PLAN.md; docs/product/STATION_PROFILE_REPRESENTATION.md B-BR-P01/P06/P07/P10/P13; BUC-P03/P04/P08; ADR-017 and existing account/catalog/media/feedback/moderation contracts.
- **Dependencies:** P31-T01.
- **Files/areas expected:** stationprofile authority checker/reviewer evidence ports; declared Directory/CNPJ/corporate source adapters; current mandate/branch fixtures.
- **Risk / tests first:** critical; Valid e-CNPJ held by accountant, public CNPJ/ANP certificate, masked/homonymous identity, shareholder without powers, matrix/branch mismatch, expired mandate and missing joint signer.
- **Implementation outline:** Obtain authoritative facts independently; verify authentic corporate acts, applicant/recipient identity linkage, joint signature and explicit delegation scope; route uncertainty to review.
- **Acceptance criteria:** Company existence, signer identity and authority are separately supported; no name-only/brand/root match approves; equivalent manual route has issuer checks and independent confirmation, not weak automatic fallback.
- **Validation commands:** Use actual affected existing Directory/account/moderation/evidence/feedback/privacy/job suites and, after creation, stationprofile unit/race and disposable real-PostGIS integration from backend/README. Required auth/ownership/failure/concurrency tests immediately; changed SQL requires empty/upgrade/recovery and query-plan proof. Affected OpenAPI/golden/compatibility checks; `git diff --check` and scoped secret review. Freeze actual package/command list at opening; nonexistent verifier tests are not evidence.
- **Risks / recovery:** Deny invalid/stale authority, retain stable station IDs/community facts, disable the changed capability or revert only safe projection/binary behavior. Applied migrations are append-only with recovery; restore replays deletion/revocation and purges expired proof; no rollback restores lost privileges or private evidence.
- **Definition of done:** Actual scoped evidence, one task issue/atomic commit, guarded phase integration and merged-SHA wiki once. Specialized exit plus required current-head/base Quick verification through `finish --required "Quick verification" --pr <actual-number>`; no skipped/failed/missing checks. Planning is not LOCAL_DONE; integrated profile is not G09 RELEASE_CERTIFIED. iOS stays explicitly deferred.

<a id="p31-t03"></a>

### P31-T03 — Restricted case review and atomic scoped grants

- **ID / priority / status:** P31-T03 / MUST for selected profile scope / PLANNED.
- **Goal:** Allow independent authorized operators to decide claims without stale or self-approved authority.
- **Inputs / rules:** docs/planning/STATION_PROFILE_PLAN.md; docs/product/STATION_PROFILE_REPRESENTATION.md B-BR-P07/P09/P10/P14/P16; BUC-P04/P07; ADR-017 and existing account/catalog/media/feedback/moderation contracts.
- **Dependencies:** P31-T02.
- **Files/areas expected:** Explicit moderation target/action/CLI extensions via application ports; stationprofile decision/grant SQL/idempotency/job; tests/OpenAPI safe owner status.
- **Risk / tests first:** critical; Unknown moderation target, role denial/self-review, concurrent approve/cancel/operator succession/account deletion, duplicate grants and stale proof/reviewer version.
- **Implementation outline:** Introduce bounded claim target/actions; atomically record reviewed decision/grant/consumed proof and required projection/status intent with expected versions.
- **Acceptance criteria:** No public admin route or moderator role from representation; pending/denied claims grant nothing; races converge on policy-valid scopes and decisions; source/policy/account/applicability rechecked at commit.
- **Validation commands:** Use actual affected existing Directory/account/moderation/evidence/feedback/privacy/job suites and, after creation, stationprofile unit/race and disposable real-PostGIS integration from backend/README. Required auth/ownership/failure/concurrency tests immediately; changed SQL requires empty/upgrade/recovery and query-plan proof. Affected OpenAPI/golden/compatibility checks; `git diff --check` and scoped secret review. Freeze actual package/command list at opening; nonexistent verifier tests are not evidence.
- **Risks / recovery:** Deny invalid/stale authority, retain stable station IDs/community facts, disable the changed capability or revert only safe projection/binary behavior. Applied migrations are append-only with recovery; restore replays deletion/revocation and purges expired proof; no rollback restores lost privileges or private evidence.
- **Definition of done:** Actual scoped evidence, one task issue/atomic commit, guarded phase integration and merged-SHA wiki once. Specialized exit plus required current-head/base Quick verification through `finish --required "Quick verification" --pr <actual-number>`; no skipped/failed/missing checks. Planning is not LOCAL_DONE; integrated profile is not G09 RELEASE_CERTIFIED. iOS stays explicitly deferred.

<a id="p31-t04"></a>

### P31-T04 — Scoped business edits and official replies

- **ID / priority / status:** P31-T04 / MUST for selected profile scope / PLANNED.
- **Goal:** Enforce actual management capabilities while protecting canonical facts and community independence.
- **Inputs / rules:** docs/planning/STATION_PROFILE_PLAN.md; docs/product/STATION_PROFILE_REPRESENTATION.md B-BR-P02/P10–P12/P16; BUC-P05; ADR-017 and existing account/catalog/media/feedback/moderation contracts.
- **Dependencies:** P31-T03.
- **Files/areas expected:** stationprofile business commands/revisions/public projection; explicit feedback attribution/account/Directory ports; additive contracts and tested SQL.
- **Risk / tests first:** critical; Manager wrong station/scope, revoked session/grant, stale operator, optimistic conflict, unauthorized canonical edit, 280/281 reply, delete others text/votes and forged business label.
- **Implementation outline:** Implement bounded permitted field changes and existing feedback reply route with server-verified business attribution; preserve normal source, author and moderation semantics.
- **Acceptance criteria:** Only current approved scope edits allowed business fields; no price-trust/ranking/regulatory overwrite or criticism suppression; safe at-time reply attribution; every privilege enforced server-side before app buttons.
- **Validation commands:** Use actual affected existing Directory/account/moderation/evidence/feedback/privacy/job suites and, after creation, stationprofile unit/race and disposable real-PostGIS integration from backend/README. Required auth/ownership/failure/concurrency tests immediately; changed SQL requires empty/upgrade/recovery and query-plan proof. Affected OpenAPI/golden/compatibility checks; `git diff --check` and scoped secret review. Freeze actual package/command list at opening; nonexistent verifier tests are not evidence.
- **Risks / recovery:** Deny invalid/stale authority, retain stable station IDs/community facts, disable the changed capability or revert only safe projection/binary behavior. Applied migrations are append-only with recovery; restore replays deletion/revocation and purges expired proof; no rollback restores lost privileges or private evidence.
- **Definition of done:** Actual scoped evidence, one task issue/atomic commit, guarded phase integration and merged-SHA wiki once. Specialized exit plus required current-head/base Quick verification through `finish --required "Quick verification" --pr <actual-number>`; no skipped/failed/missing checks. Planning is not LOCAL_DONE; integrated profile is not G09 RELEASE_CERTIFIED. iOS stays explicitly deferred.

<a id="p31-t05"></a>

### P31-T05 — Contestation, suspension and immediate revocation

- **ID / priority / status:** P31-T05 / MUST for selected profile scope / PLANNED.
- **Goal:** Handle competing claims and impersonation reports without competitor takeover or queue-dependent authority.
- **Inputs / rules:** docs/planning/STATION_PROFILE_PLAN.md; docs/product/STATION_PROFILE_REPRESENTATION.md B-BR-P09/P10/P13/P14/P16; BUC-P07/P08; ADR-017 and existing account/catalog/media/feedback/moderation contracts.
- **Dependencies:** P31-T04.
- **Files/areas expected:** stationprofile dispute/grant checks; restricted moderation commands; public badge purge/TTL and private notice/status projection.
- **Risk / tests first:** critical; Mass false reports, rival document IDOR, substantiated conflict review, revoke vs write race, dead cleanup worker, stale cache badge and appeal of terminal claim.
- **Implementation outline:** Open private linked cases for new evidence; operator decides substantiated-risk suspension/revocation with audit; writes recheck validity and operator version; freeze/report badge-cache bound.
- **Acceptance criteria:** Report count cannot transfer/suspend automatically; revoked/suspended scope denied immediately despite job outage; private rivals not exposed; new linked appeal preserves closed-case history.
- **Validation commands:** Use actual affected existing Directory/account/moderation/evidence/feedback/privacy/job suites and, after creation, stationprofile unit/race and disposable real-PostGIS integration from backend/README. Required auth/ownership/failure/concurrency tests immediately; changed SQL requires empty/upgrade/recovery and query-plan proof. Affected OpenAPI/golden/compatibility checks; `git diff --check` and scoped secret review. Freeze actual package/command list at opening; nonexistent verifier tests are not evidence.
- **Risks / recovery:** Deny invalid/stale authority, retain stable station IDs/community facts, disable the changed capability or revert only safe projection/binary behavior. Applied migrations are append-only with recovery; restore replays deletion/revocation and purges expired proof; no rollback restores lost privileges or private evidence.
- **Definition of done:** Actual scoped evidence, one task issue/atomic commit, guarded phase integration and merged-SHA wiki once. Specialized exit plus required current-head/base Quick verification through `finish --required "Quick verification" --pr <actual-number>`; no skipped/failed/missing checks. Planning is not LOCAL_DONE; integrated profile is not G09 RELEASE_CERTIFIED. iOS stays explicitly deferred.

<a id="p31-t06"></a>

### P31-T06 — Scoped delegation, transfer and reviewed recovery

- **ID / priority / status:** P31-T06 / MUST for selected profile scope / PLANNED.
- **Goal:** Support multiple authorized representatives without unverified privilege escalation or lost-admin shortcuts.
- **Inputs / rules:** docs/planning/STATION_PROFILE_PLAN.md; docs/product/STATION_PROFILE_REPRESENTATION.md B-BR-P01/P04/P06/P10/P13/P14; BUC-P06/P08; ADR-017 and existing account/catalog/media/feedback/moderation contracts.
- **Dependencies:** P31-T05.
- **Files/areas expected:** stationprofile invitation/recipient proof/grant lifecycle ports and schema; explicit fresh-auth method; synthetic mandate/account fixtures.
- **Risk / tests first:** critical; Manager self-promotion, scope wider than mandate, expired/wrong-recipient invite, transfer without recipient authority, operator change, parallel revoke/accept and last-admin recovery.
- **Implementation outline:** Implement recipient-bound one-use invitation and fresh acceptance; reviewed transfer requires new scoped proof; reverify changed authority/operator; use existing permitted sharing/status rather than invent a mail service.
- **Acceptance criteria:** Invitation alone does not grant wider corporate powers; transfer/recovery has independent recipient proof and review; no generic business OWNER across stations or old-operator access after succession.
- **Validation commands:** Use actual affected existing Directory/account/moderation/evidence/feedback/privacy/job suites and, after creation, stationprofile unit/race and disposable real-PostGIS integration from backend/README. Required auth/ownership/failure/concurrency tests immediately; changed SQL requires empty/upgrade/recovery and query-plan proof. Affected OpenAPI/golden/compatibility checks; `git diff --check` and scoped secret review. Freeze actual package/command list at opening; nonexistent verifier tests are not evidence.
- **Risks / recovery:** Deny invalid/stale authority, retain stable station IDs/community facts, disable the changed capability or revert only safe projection/binary behavior. Applied migrations are append-only with recovery; restore replays deletion/revocation and purges expired proof; no rollback restores lost privileges or private evidence.
- **Definition of done:** Actual scoped evidence, one task issue/atomic commit, guarded phase integration and merged-SHA wiki once. Specialized exit plus required current-head/base Quick verification through `finish --required "Quick verification" --pr <actual-number>`; no skipped/failed/missing checks. Planning is not LOCAL_DONE; integrated profile is not G09 RELEASE_CERTIFIED. iOS stays explicitly deferred.

<a id="p31-t07"></a>

### P31-T07 — Proof, grant and profile privacy recovery

- **ID / priority / status:** P31-T07 / MUST for selected profile scope / PLANNED.
- **Goal:** Close rights/expiry and restore behavior across new authority and public history.
- **Inputs / rules:** docs/planning/STATION_PROFILE_PLAN.md; docs/product/STATION_PROFILE_REPRESENTATION.md B-BR-P08/P13–P16; BUC-P09/P10; ADR-017 and existing account/catalog/media/feedback/moderation contracts.
- **Dependencies:** P31-T06.
- **Files/areas expected:** stationprofile/account/privacy/evidence/moderation explicit rights workflows; deletion/revocation ledger and private storage restore tests.
- **Risk / tests first:** critical; Cross-owner export, delete account during approve, 24h scan expiry, signed-proof retention, replica/copy cleanup, grant replay after restore and at-time official-reply attribution.
- **Implementation outline:** Implement minimal-data export/erasure/expiry, revoke before cleanup, policy-bound audit and deletion/revocation replay; verify empty/upgrade/restore recovery.
- **Acceptance criteria:** No revoked grant or expired proof returns after restore/rollback; deletion does not erase station/others facts; export isolates owner documents; all-copy/private retention enforcement has genuine evidence.
- **Validation commands:** Use actual affected existing Directory/account/moderation/evidence/feedback/privacy/job suites and, after creation, stationprofile unit/race and disposable real-PostGIS integration from backend/README. Required auth/ownership/failure/concurrency tests immediately; changed SQL requires empty/upgrade/recovery and query-plan proof. Affected OpenAPI/golden/compatibility checks; `git diff --check` and scoped secret review. Freeze actual package/command list at opening; nonexistent verifier tests are not evidence.
- **Risks / recovery:** Deny invalid/stale authority, retain stable station IDs/community facts, disable the changed capability or revert only safe projection/binary behavior. Applied migrations are append-only with recovery; restore replays deletion/revocation and purges expired proof; no rollback restores lost privileges or private evidence.
- **Definition of done:** Actual scoped evidence, one task issue/atomic commit, guarded phase integration and merged-SHA wiki once. Specialized exit plus required current-head/base Quick verification through `finish --required "Quick verification" --pr <actual-number>`; no skipped/failed/missing checks. Planning is not LOCAL_DONE; integrated profile is not G09 RELEASE_CERTIFIED. iOS stays explicitly deferred.

## P32 — Android station profile and representation journeys

Priority: **MUST for selected profile scope**. Entry: G31 tested backend source checkpoint; actual contract/authority precedes app consumers and live deployment remains explicitly verified. Exit: **G32: functional profile, claim/status and bounded management journeys accepted on Android**. State: PLANNED. Branch: `codex/phase-32-station-profile-app`.

<a id="p32-t01"></a>

### P32-T01 — Public profile, provenance and representation badge

- **ID / priority / status:** P32-T01 / MUST for selected profile scope / PLANNED.
- **Goal:** Display the actual station profile and independent business/price/source states.
- **Inputs / rules:** docs/planning/STATION_PROFILE_PLAN.md; docs/product/STATION_PROFILE_REPRESENTATION.md B-BR-P01/P02/P11/P12/P16; BUC-P01; ADR-017 and existing account/catalog/media/feedback/moderation contracts.
- **Dependencies:** G31 integrated; backend contract/authority precedes app consumers; affected backend contract/permission integrated before consumer.
- **Files/areas expected:** Kotlin domain/application/data/profile DTO and read port; app station detail/navigation/public cache; portable fixtures.
- **Risk / tests first:** critical; No prices/no representative/no reviewed location, stale/revoked badge, unknown server state, large text/screen-reader labels and canonical UUID mapping.
- **Implementation outline:** Integrate anonymous profile read and cautious badge/time/source presentation, preserving current price/social hierarchy and legacy expert tools.
- **Acceptance criteria:** Public UI exposes no private proof/person; no badge conflated with fuel/price quality; stale cache cannot imply privileged access; server station exists without synthetic price row.
- **Validation commands:** Root affected `:domain:test`, `:application:test`, `:data:testDebugUnitTest`, `:app:testDebugUnitTest`, `:app:assembleDebug` and relevant real-device/file-picker/cache instrumentation; affected backend contract/security checks from backend/README. Freeze device/resource row selection before coding. `git diff --check` and scoped secret review; iOS excluded.
- **Risks / recovery:** Deny invalid/stale authority, retain stable station IDs/community facts, disable the changed capability or revert only safe projection/binary behavior. Applied migrations are append-only with recovery; restore replays deletion/revocation and purges expired proof; no rollback restores lost privileges or private evidence.
- **Definition of done:** Actual scoped evidence, one task issue/atomic commit, guarded phase integration and merged-SHA wiki once. Specialized exit plus required current-head/base Quick verification through `finish --required "Quick verification" --pr <actual-number>`; no skipped/failed/missing checks. Planning is not LOCAL_DONE; integrated profile is not G09 RELEASE_CERTIFIED. iOS stays explicitly deferred.

<a id="p32-t02"></a>

### P32-T02 — Free-account claim export-sign-import and private status

- **ID / priority / status:** P32-T02 / MUST for selected profile scope / PLANNED.
- **Goal:** Make the reviewed representation process usable without exposing certificate secrets.
- **Inputs / rules:** docs/planning/STATION_PROFILE_PLAN.md; docs/product/STATION_PROFILE_REPRESENTATION.md B-BR-P03–P08/P16; BUC-P02/P03/P04; ADR-017 and existing account/catalog/media/feedback/moderation contracts.
- **Dependencies:** P32-T01; affected backend contract/permission integrated before consumer.
- **Files/areas expected:** Existing Kotlin/Android auth/document picker/share/status ports and bounded claim outbox; application use cases.
- **Risk / tests first:** critical; Guest login, expired declaration, permission denied, original signed PDF import, scan cap, wrong file type, offline submit/status retry and owner mismatch.
- **Implementation outline:** Build structured role/scope request and server declaration export/import with externally signed original bytes, private progress/need-information/appeal states; no PDF rewrite or certificate-key collection.
- **Acceptance criteria:** User can complete a genuine signed-file flow; client never certifies signature/powers; original upload bounded/private and retry account-bound; existing media and free social flows preserved.
- **Validation commands:** Root affected `:domain:test`, `:application:test`, `:data:testDebugUnitTest`, `:app:testDebugUnitTest`, `:app:assembleDebug` and relevant real-device/file-picker/cache instrumentation; affected backend contract/security checks from backend/README. Freeze device/resource row selection before coding. `git diff --check` and scoped secret review; iOS excluded.
- **Risks / recovery:** Deny invalid/stale authority, retain stable station IDs/community facts, disable the changed capability or revert only safe projection/binary behavior. Applied migrations are append-only with recovery; restore replays deletion/revocation and purges expired proof; no rollback restores lost privileges or private evidence.
- **Definition of done:** Actual scoped evidence, one task issue/atomic commit, guarded phase integration and merged-SHA wiki once. Specialized exit plus required current-head/base Quick verification through `finish --required "Quick verification" --pr <actual-number>`; no skipped/failed/missing checks. Planning is not LOCAL_DONE; integrated profile is not G09 RELEASE_CERTIFIED. iOS stays explicitly deferred.

<a id="p32-t03"></a>

### P32-T03 — Representative management, invitations and contested access

- **ID / priority / status:** P32-T03 / MUST for selected profile scope / PLANNED.
- **Goal:** Expose only server-authorized business capabilities and reviewed lifecycle actions.
- **Inputs / rules:** docs/planning/STATION_PROFILE_PLAN.md; docs/product/STATION_PROFILE_REPRESENTATION.md B-BR-P09–P16; BUC-P05–P08; ADR-017 and existing account/catalog/media/feedback/moderation contracts.
- **Dependencies:** P32-T02; affected backend contract/permission integrated before consumer.
- **Files/areas expected:** Existing app profile/feedback/editor/status screens and Kotlin management use cases; account fresh-auth adapter selected in P30/P31.
- **Risk / tests first:** critical; Revoked/suspended role mid-edit, stale operator, missing required additional auth, invite/transfer/recovery status, 280/281 official reply and denied privileged retry.
- **Implementation outline:** Implement allowed business edits/replies and scoped invitation/contest/reverification UI; server remains authoritative; explain badge and review limitations in user-facing copy.
- **Acceptance criteria:** No optimistic false approval or UI-only permission gate; canonical correction path distinct; failed/revoked writes recover without replaying stale privilege; no in-app moderator or hidden critique control.
- **Validation commands:** Root affected `:domain:test`, `:application:test`, `:data:testDebugUnitTest`, `:app:testDebugUnitTest`, `:app:assembleDebug` and relevant real-device/file-picker/cache instrumentation; affected backend contract/security checks from backend/README. Freeze device/resource row selection before coding. `git diff --check` and scoped secret review; iOS excluded.
- **Risks / recovery:** Deny invalid/stale authority, retain stable station IDs/community facts, disable the changed capability or revert only safe projection/binary behavior. Applied migrations are append-only with recovery; restore replays deletion/revocation and purges expired proof; no rollback restores lost privileges or private evidence.
- **Definition of done:** Actual scoped evidence, one task issue/atomic commit, guarded phase integration and merged-SHA wiki once. Specialized exit plus required current-head/base Quick verification through `finish --required "Quick verification" --pr <actual-number>`; no skipped/failed/missing checks. Planning is not LOCAL_DONE; integrated profile is not G09 RELEASE_CERTIFIED. iOS stays explicitly deferred.

<a id="p32-t04"></a>

### P32-T04 — Offline, accessibility and real-device acceptance

- **ID / priority / status:** P32-T04 / MUST for selected profile scope / PLANNED.
- **Goal:** Prove new profile and claim flows with actual bounded signed-file/device behavior.
- **Inputs / rules:** docs/planning/STATION_PROFILE_PLAN.md; docs/product/STATION_PROFILE_REPRESENTATION.md B-BR-P01–P16; BUC-P01–P09; ADR-017 and existing account/catalog/media/feedback/moderation contracts.
- **Dependencies:** P32-T03; affected backend contract/permission integrated before consumer.
- **Files/areas expected:** Affected domain/application/data/app tests and device/Room/file URI/low-end evidence; P32 exit record.
- **Risk / tests first:** critical; Offline pending vs expired/revoked role, process restart, file URI permission expiry, no-photo cache, low-end parsing memory, screen reader/font scale and old API/cache fallback.
- **Implementation outline:** Run targeted device/privacy/canonical action regressions and measured budgets; preserve portable Kotlin and isolated Android adapters.
- **Acceptance criteria:** Required flows genuinely pass on supported devices; no PDF/photo processing freeze/unbounded memory or cached authority acceptance; no iOS or production provider proof inferred.
- **Validation commands:** Root affected `:domain:test`, `:application:test`, `:data:testDebugUnitTest`, `:app:testDebugUnitTest`, `:app:assembleDebug` and relevant real-device/file-picker/cache instrumentation; affected backend contract/security checks from backend/README. Freeze device/resource row selection before coding. `git diff --check` and scoped secret review; iOS excluded.
- **Risks / recovery:** Deny invalid/stale authority, retain stable station IDs/community facts, disable the changed capability or revert only safe projection/binary behavior. Applied migrations are append-only with recovery; restore replays deletion/revocation and purges expired proof; no rollback restores lost privileges or private evidence.
- **Definition of done:** Actual scoped evidence, one task issue/atomic commit, guarded phase integration and merged-SHA wiki once. Specialized exit plus required current-head/base Quick verification through `finish --required "Quick verification" --pr <actual-number>`; no skipped/failed/missing checks. Planning is not LOCAL_DONE; integrated profile is not G09 RELEASE_CERTIFIED. iOS stays explicitly deferred.

## P33 — Profile fraud, operations and candidate acceptance

Priority: **MUST for selected profile scope**. Entry: G32 integrated; enabled source/verifier/storage/device prerequisites available. Exit: **G33-PROFILE: adversarial lifecycle/privacy and refreshed Android candidate acceptance, separate from G09**. State: PLANNED. Branch: `codex/phase-33-profile-acceptance`.

<a id="p33-t01"></a>

### P33-T01 — Adversarial, resource and review-operations campaign

- **ID / priority / status:** P33-T01 / MUST for selected profile scope / PLANNED.
- **Goal:** Measure abuse controls and staffed review throughput under the declared workload.
- **Inputs / rules:** docs/planning/STATION_PROFILE_PLAN.md; docs/product/STATION_PROFILE_REPRESENTATION.md B-BR-P03–P10/P13–P16; BUC-P03/P04/P07/P10; ADR-017 and existing account/catalog/media/feedback/moderation contracts.
- **Dependencies:** G32 integrated; enabled source/verifier/storage/device prerequisites available.
- **Files/areas expected:** Synthetic claim/trust/PDF/source load fixtures, existing load/job harness extensions and operator runbooks.
- **Risk / tests first:** critical; 1k/day synthetic claim hypothesis with frozen ambiguous mix, rival-claim burst, signature parser exhaustion, provider downtime, stale job/fencing and grant race/read traffic.
- **Implementation outline:** Freeze workload/hardware/resource/query/queue/staffing budgets before campaign; use fixture upstreams; run pure/race/PostGIS/adversarial checks and measure genuine available-source smoke separately.
- **Acceptance criteria:** No unverified grant, private leak, silent dropped claim or budget failure; human review/indeterminate backlog and escalation measured; no fake fraud-free or instant-approval promise.
- **Validation commands:** Frozen affected backend real-PostGIS/race/critical proof/privacy/storage/restore tests plus exact enabled provider and Android device/compatibility/accessibility/performance rows. Record artifact/config/workload/commands/results; missing required row blocks gate. Specialized exit once; required Quick verification through finish only once; no unrelated full G09 repeat.
- **Risks / recovery:** Deny invalid/stale authority, retain stable station IDs/community facts, disable the changed capability or revert only safe projection/binary behavior. Applied migrations are append-only with recovery; restore replays deletion/revocation and purges expired proof; no rollback restores lost privileges or private evidence.
- **Definition of done:** Actual scoped evidence, one task issue/atomic commit, guarded phase integration and merged-SHA wiki once. Specialized exit plus required current-head/base Quick verification through `finish --required "Quick verification" --pr <actual-number>`; no skipped/failed/missing checks. Planning is not LOCAL_DONE; integrated profile is not G09 RELEASE_CERTIFIED. iOS stays explicitly deferred.

<a id="p33-t02"></a>

### P33-T02 — End-to-end authority, privacy and device reacceptance

- **ID / priority / status:** P33-T02 / MUST for selected profile scope / PLANNED.
- **Goal:** Accept the complete selected profile lifecycle on the updated candidate.
- **Inputs / rules:** docs/planning/STATION_PROFILE_PLAN.md; docs/product/STATION_PROFILE_REPRESENTATION.md B-BR-P01–P16; BUC-P01–P10; ADR-017 and existing account/catalog/media/feedback/moderation contracts.
- **Dependencies:** P33-T01.
- **Files/areas expected:** Existing backend process/account/Directory/profile/moderation/storage integration and Android device/accessibility/performance manifest.
- **Risk / tests first:** critical; Unclaimed profile→claim→signed proof→independent review→edit/reply→delegate→contest→revoke→operator change→erase/restore; wrong account/authority/provider and regression rows.
- **Implementation outline:** Execute meaningful lifecycle scenarios and real necessary provider/storage/device rows; carry forward prior evidence only for identical relevant artifacts/inputs; correct all required failures.
- **Acceptance criteria:** G33 manifest has no missing required anti-fraud/permissions/expiry/restore/device row; forged/wrong powers and stale grants denied; no simulation substituted for real approved trust-provider acceptance.
- **Validation commands:** Frozen affected backend real-PostGIS/race/critical proof/privacy/storage/restore tests plus exact enabled provider and Android device/compatibility/accessibility/performance rows. Record artifact/config/workload/commands/results; missing required row blocks gate. Specialized exit once; required Quick verification through finish only once; no unrelated full G09 repeat.
- **Risks / recovery:** Deny invalid/stale authority, retain stable station IDs/community facts, disable the changed capability or revert only safe projection/binary behavior. Applied migrations are append-only with recovery; restore replays deletion/revocation and purges expired proof; no rollback restores lost privileges or private evidence.
- **Definition of done:** Actual scoped evidence, one task issue/atomic commit, guarded phase integration and merged-SHA wiki once. Specialized exit plus required current-head/base Quick verification through `finish --required "Quick verification" --pr <actual-number>`; no skipped/failed/missing checks. Planning is not LOCAL_DONE; integrated profile is not G09 RELEASE_CERTIFIED. iOS stays explicitly deferred.

<a id="p33-t03"></a>

### P33-T03 — Pinned profile candidate and release operating handoff

- **ID / priority / status:** P33-T03 / MUST for selected profile scope / PLANNED.
- **Goal:** Provide the verified scoped candidate, reviewer runbooks and existing-release prerequisites.
- **Inputs / rules:** docs/planning/STATION_PROFILE_PLAN.md; docs/product/STATION_PROFILE_REPRESENTATION.md B-BR-P08/P13–P16; BUC-P08/P09/P10; ADR-017 and existing account/catalog/media/feedback/moderation contracts.
- **Dependencies:** P33-T02.
- **Files/areas expected:** P33 evidence/candidate manifest; operator access/review/suspension/privacy runbooks; P09/G09 and P10-T09 handoff.
- **Risk / tests first:** critical; Source/verifier access health, urgent impersonation review, feature disable/binary rollback, revocation/deletion restore, contact/source rights and actual staffing drill.
- **Implementation outline:** Record exact artifact/config/source/check scope and limitations; validate operator procedures and real-production unresolved checklist; G09 remains sole full certification.
- **Acceptance criteria:** G33 integration/readiness distinguished from G09 real release; no iOS acceptance or deployment inferred; selected profile tasks/source/retention/auth criteria covered with current evidence.
- **Validation commands:** Frozen affected backend real-PostGIS/race/critical proof/privacy/storage/restore tests plus exact enabled provider and Android device/compatibility/accessibility/performance rows. Record artifact/config/workload/commands/results; missing required row blocks gate. Specialized exit once; required Quick verification through finish only once; no unrelated full G09 repeat.
- **Risks / recovery:** Deny invalid/stale authority, retain stable station IDs/community facts, disable the changed capability or revert only safe projection/binary behavior. Applied migrations are append-only with recovery; restore replays deletion/revocation and purges expired proof; no rollback restores lost privileges or private evidence.
- **Definition of done:** Actual scoped evidence, one task issue/atomic commit, guarded phase integration and merged-SHA wiki once. Specialized exit plus required current-head/base Quick verification through `finish --required "Quick verification" --pr <actual-number>`; no skipped/failed/missing checks. Planning is not LOCAL_DONE; integrated profile is not G09 RELEASE_CERTIFIED. iOS stays explicitly deferred.

<a id="android-vps-integration"></a>

## P34–P38 — Android integration with the existing VPS (next construction)

Execution priority supersedes numerical phase order: [ADR-019](docs/adr/019-android-vps-integration-first.md), [Android/VPS plan](docs/planning/ANDROID_VPS_PLAN.md). All tasks below are **PLANNED / LOCAL_ONLY**. Current temporary origin: `https://teste.abastevo.com.br`. Actual source/risk acceptance precedes local checkpoints; remote CI/merge and the user-deferred manual/device union happen at end project closure. No live/production success follows from fixture tests or a health check.

### P34 — Staging connection and test catalog

State: PLANNED. Entry/exit, risk cases and evidence: [phase plan](docs/planning/ANDROID_VPS_PLAN.md#p34--staging-connection-and-a-useful-test-catalog).

<a id="p34-t01"></a>

#### P34-T01 — Inventory and verify the staging contract

- **Status / priority:** PLANNED / MUST for selected Android/VPS scope.
- **Goal:** Resolve HTTPS trust and verify public health/Directory reads; inventory deployed clients/flags without server writes.
- **Dependencies / acceptance / tests:** owning phase section in ANDROID_VPS_PLAN; documented contract/risk checks before consumers. Record exact meaningful commands/results and unresolved live/device prerequisites; no fabricated green.
- **Delivery:** atomic task commit, isolated phase branch and tested local checkpoint; CI/PR merge/wiki only at final construction batch closure under ADR-018.

<a id="p34-t02"></a>

#### P34-T02 — One explicit Android environment configuration

- **Status / priority:** PLANNED / MUST for selected Android/VPS scope.
- **Goal:** Wire all existing backend clients to one origin, isolate sessions/flags and prove routing/error/security behavior.
- **Dependencies / acceptance / tests:** owning phase section in ANDROID_VPS_PLAN; documented contract/risk checks before consumers. Record exact meaningful commands/results and unresolved live/device prerequisites; no fabricated green.
- **Delivery:** atomic task commit, isolated phase branch and tested local checkpoint; CI/PR merge/wiki only at final construction batch closure under ADR-018.

<a id="p34-t03"></a>

#### P34-T03 — Bounded synthetic integration dataset

- **Status / priority:** PLANNED / MUST for selected Android/VPS scope.
- **Goal:** Specify and, only with write scope, prepare reproducible owned fixtures through existing supported mechanisms; no global reset.
- **Dependencies / acceptance / tests:** owning phase section in ANDROID_VPS_PLAN; documented contract/risk checks before consumers. Record exact meaningful commands/results and unresolved live/device prerequisites; no fabricated green.
- **Delivery:** atomic task commit, isolated phase branch and tested local checkpoint; CI/PR merge/wiki only at final construction batch closure under ADR-018.

### P35 — Live discovery and community prices

State: PLANNED. Entry/exit, risk cases and evidence: [phase plan](docs/planning/ANDROID_VPS_PLAN.md#p35--explore-station-detail-and-actual-community-prices).

<a id="p35-t01"></a>

#### P35-T01 — Directory UUID ports and cache

- **Status / priority:** PLANNED / MUST for selected Android/VPS scope.
- **Goal:** Consume bounded canonical server station IDs and preserve legacy CNPJ/offline data; migrations need recovery proof.
- **Dependencies / acceptance / tests:** owning phase section in ANDROID_VPS_PLAN; documented contract/risk checks before consumers. Record exact meaningful commands/results and unresolved live/device prerequisites; no fabricated green.
- **Delivery:** atomic task commit, isolated phase branch and tested local checkpoint; CI/PR merge/wiki only at final construction batch closure under ADR-018.

<a id="p35-t02"></a>

#### P35-T02 — Explore and station detail integration

- **Status / priority:** PLANNED / MUST for selected Android/VPS scope.
- **Goal:** Wire community-first prices, dated ANP references, source/condition/time and honest empty/stale/error states.
- **Dependencies / acceptance / tests:** owning phase section in ANDROID_VPS_PLAN; documented contract/risk checks before consumers. Record exact meaningful commands/results and unresolved live/device prerequisites; no fabricated green.
- **Delivery:** atomic task commit, isolated phase branch and tested local checkpoint; CI/PR merge/wiki only at final construction batch closure under ADR-018.

<a id="p35-t03"></a>

#### P35-T03 — Search and degraded discovery regression

- **Status / priority:** PLANNED / MUST for selected Android/VPS scope.
- **Goal:** Fix search emission, prove paging/cache/restart/location denial/outage and preserve expert features.
- **Dependencies / acceptance / tests:** owning phase section in ANDROID_VPS_PLAN; documented contract/risk checks before consumers. Record exact meaningful commands/results and unresolved live/device prerequisites; no fabricated green.
- **Delivery:** atomic task commit, isolated phase branch and tested local checkpoint; CI/PR merge/wiki only at final construction batch closure under ADR-018.

### P36 — Free account and social journeys

State: PLANNED. Entry/exit, risk cases and evidence: [phase plan](docs/planning/ANDROID_VPS_PLAN.md#p36--free-accounts-and-stationfuel-social-participation).

<a id="p36-t01"></a>

#### P36-T01 — Staging account and provider integration

- **Status / priority:** PLANNED / MUST for selected Android/VPS scope.
- **Goal:** Wire email/provider/session/key flows; prove denial/replay/revocation and record owed actual provider evidence.
- **Dependencies / acceptance / tests:** owning phase section in ANDROID_VPS_PLAN; documented contract/risk checks before consumers. Record exact meaningful commands/results and unresolved live/device prerequisites; no fabricated green.
- **Delivery:** atomic task commit, isolated phase branch and tested local checkpoint; CI/PR merge/wiki only at final construction batch closure under ADR-018.

<a id="p36-t02"></a>

#### P36-T02 — Station and fuel feedback screens

- **Status / priority:** PLANNED / MUST for selected Android/VPS scope.
- **Goal:** Connect ratings/comments/replies/votes/reports to canonical targets with signed writes and 280-scalar limits.
- **Dependencies / acceptance / tests:** owning phase section in ANDROID_VPS_PLAN; documented contract/risk checks before consumers. Record exact meaningful commands/results and unresolved live/device prerequisites; no fabricated green.
- **Delivery:** atomic task commit, isolated phase branch and tested local checkpoint; CI/PR merge/wiki only at final construction batch closure under ADR-018.

<a id="p36-t03"></a>

#### P36-T03 — Private activity and account rights

- **Status / priority:** PLANNED / MUST for selected Android/VPS scope.
- **Goal:** Owner status/retry/deletion; export requires a bounded backend contract/privacy extension and IDOR tests.
- **Dependencies / acceptance / tests:** owning phase section in ANDROID_VPS_PLAN; documented contract/risk checks before consumers. Record exact meaningful commands/results and unresolved live/device prerequisites; no fabricated green.
- **Delivery:** atomic task commit, isolated phase branch and tested local checkpoint; CI/PR merge/wiki only at final construction batch closure under ADR-018.

### P37 — Capture and durable contributions

State: PLANNED. Entry/exit, risk cases and evidence: [phase plan](docs/planning/ANDROID_VPS_PLAN.md#p37--capture-upload-and-durable-contribution-status).

<a id="p37-t01"></a>

#### P37-T01 — Contextual capture and review

- **Status / priority:** PLANNED / MUST for selected Android/VPS scope.
- **Goal:** Station/fuel/photo/OCR/manual price/condition selection, explicit submit and existing location-integrity/bounded media rules.
- **Dependencies / acceptance / tests:** owning phase section in ANDROID_VPS_PLAN; documented contract/risk checks before consumers. Record exact meaningful commands/results and unresolved live/device prerequisites; no fabricated green.
- **Delivery:** atomic task commit, isolated phase branch and tested local checkpoint; CI/PR merge/wiki only at final construction batch closure under ADR-018.

<a id="p37-t02"></a>

#### P37-T02 — Live upload and outbox lifecycle

- **Status / priority:** PLANNED / MUST for selected Android/VPS scope.
- **Goal:** Actual identity/upload/observation/status/cancel and safe retry/recovery; backend defects tested before consumers.
- **Dependencies / acceptance / tests:** owning phase section in ANDROID_VPS_PLAN; documented contract/risk checks before consumers. Record exact meaningful commands/results and unresolved live/device prerequisites; no fabricated green.
- **Delivery:** atomic task commit, isolated phase branch and tested local checkpoint; CI/PR merge/wiki only at final construction batch closure under ADR-018.

<a id="p37-t03"></a>

#### P37-T03 — Private storage and all-copy expiry

- **Status / priority:** PLANNED / MUST for selected Android/VPS scope.
- **Goal:** Prove 24h deletion/cache/outbox/storage/restore; missing attached VPS media blocks live photo acceptance explicitly.
- **Dependencies / acceptance / tests:** owning phase section in ANDROID_VPS_PLAN; documented contract/risk checks before consumers. Record exact meaningful commands/results and unresolved live/device prerequisites; no fabricated green.
- **Delivery:** atomic task commit, isolated phase branch and tested local checkpoint; CI/PR merge/wiki only at final construction batch closure under ADR-018.

### P38 — Source regression and end acceptance handoff

State: PLANNED. Entry/exit, risk cases and evidence: [phase plan](docs/planning/ANDROID_VPS_PLAN.md#p38--app-regression-end-validation-and-release-handoff).

<a id="p38-t01"></a>

#### P38-T01 — Integrated code journey regression

- **Status / priority:** PLANNED / MUST for selected Android/VPS scope.
- **Goal:** Test full read/account/contribution/social/privacy/offline/error lifecycle on actual source contracts, then build affected APK.
- **Dependencies / acceptance / tests:** owning phase section in ANDROID_VPS_PLAN; documented contract/risk checks before consumers. Record exact meaningful commands/results and unresolved live/device prerequisites; no fabricated green.
- **Delivery:** atomic task commit, isolated phase branch and tested local checkpoint; CI/PR merge/wiki only at final construction batch closure under ADR-018.

<a id="p38-t02"></a>

#### P38-T02 — Consolidated end Android manual matrix

- **Status / priority:** PLANNED / MUST for selected Android/VPS scope.
- **Goal:** Plan the union of P24/P29/P33 provider/device/performance/novice/accessibility rows; execute only after all selected source phases.
- **Dependencies / acceptance / tests:** owning phase section in ANDROID_VPS_PLAN; documented contract/risk checks before consumers. Record exact meaningful commands/results and unresolved live/device prerequisites; no fabricated green.
- **Delivery:** atomic task commit, isolated phase branch and tested local checkpoint; CI/PR merge/wiki only at final construction batch closure under ADR-018.

<a id="p38-t03"></a>

#### P38-T03 — Candidate, final integration and G09 handoff

- **Status / priority:** PLANNED / MUST for selected Android/VPS scope.
- **Goal:** Record actual source/backend/config/evidence; required final PR/CI/reviews/merge/wiki happen at project closure, certification remains G09.
- **Dependencies / acceptance / tests:** owning phase section in ANDROID_VPS_PLAN; documented contract/risk checks before consumers. Record exact meaningful commands/results and unresolved live/device prerequisites; no fabricated green.
- **Delivery:** atomic task commit, isolated phase branch and tested local checkpoint; CI/PR merge/wiki only at final construction batch closure under ADR-018.

## P25 — Static landing page

Priority: **MUST within the authorized website scope**. State: **PLANNED**. Entry: existing G09-LOCAL integration and approved abastevo identity. Independent from the Android/backend release sequence. Exit: **G25-STATIC-READY**, demonstrated static-site acceptance; integration, website publication, store availability and Google field evidence remain distinct. Specification: [static landing plan](docs/planning/STATIC_LANDING_PLAN.md).

Current authority: local planning only (specification revision 2, including the maintainer-requested review: legal destinations, owner decisions D-L01–D-L08, hosting policy, icons/social/headers, visual contract, verification tooling). Future implementation branch: `codex/phase-25-static-landing`; T01–T05 run sequentially under one phase milestone/draft PR when delivery is authorized. Public hosting, Search Console ownership operations and the later real Play Store activation have their own prerequisites and authorization. No website/runtime is completed by this plan.

<a id="p25-t01"></a>

### P25-T01 — Landing content, visual and decision contract

- **ID / priority / status:** P25-T01 / MUST / LOCAL_DONE (2026-10-05, issue #106) — APPROVED. Output: [content contract](docs/product/LANDING_CONTENT_CONTRACT.md), [375 px](docs/assets/landing/wireframe-375.svg) and [1280 px](docs/assets/landing/wireframe-1280.svg) wireframes.
- **Goal:** Freeze a clear Portuguese product explanation, calm section layout, approved visual contract, legal-destination requirements, owner-decision status and truthful prelaunch/published-store states.
- **Inputs / rules:** [Landing specification](docs/planning/STATIC_LANDING_PLAN.md), B-BR-L01–L07, BUC-L01–L05; approved brand, [security/privacy plan](docs/security/SECURITY_PRIVACY.md) and current owning product/release evidence. Zaflas is a read-only design/SEO reference.
- **Dependencies:** Existing G09-LOCAL integration and approved identity; no public Play Store listing needed to plan the prelaunch state.
- **Files / risk:** Landing content/visual contract in docs/planning; docs risk. No runtime changes.
- **Acceptance criteria:** One short contract document covering section order, page inventory (home, privacy, account deletion, terms, 404), verified feature claims, source/date/condition explanation, illustrative comparison card, GitHub destination, restrained links and accessibility direction. Records status of D-L01–D-L08 (repository name, domain, host criteria, contact channel, delivery shape, crawler policy), the dated current Google Play policy check for legal destinations, reconciliation of retention wording (README 24 h vs retention table), and maintainer-approved 375/1280 wireframes plus design tokens with contrast ratios. No copied ratings or unsupported availability/coverage claims. No code.
- **Validation commands:** Changed-document links/anchors/task consistency, `git diff --check` and scoped secret/private-data review; no backend/mobile aggregate for this docs task.
- **Recovery / done:** Revise owned copy; preserve product contracts/brand/license. Actual task evidence and local acceptance recorded; remote integration follows the authorized phase workflow.

<a id="p25-t02"></a>

### P25-T02 — Static foundation and TypeScript compilation

- **ID / priority / status:** P25-T02 / MUST / LOCAL_DONE (2026-10-05, issue #107).
- **Goal:** Build deployable static HTML/CSS with plain TypeScript 7.0.2 compiled to browser JavaScript, content-hash fingerprinted assets and no framework/runtime dependency.
- **Inputs / rules:** Landing specification, B-BR-L03/L04/L06/L07, BUC-L01–L05 and T01 contract.
- **Dependencies:** P25-T01. Exact compiler artifact `typescript@7.0.2` verified, binary `tsc`, strict `tsconfig.json` with `noEmitOnError`, Node v26.3.1 support, Apache-2.0 license.
- **Files / risk:** `landing/`; bounded landing selection/check integration in `scripts/quick-verify.sh`, gate-selection tests in `scripts/tests/test-gate-selection.sh`. Standard risk; preserve required Quick verification.
- **Tests first:** Static essential content/links without JS; missing/invalid public origin; store prelaunch and valid/invalid listing state; reproducible build and deploy artifact contents.
- **Acceptance criteria:** Authored HTML/static CSS including the full `<head>` metadata skeleton and page inventory; strict exact-pinned TS compiler with lockfile and noEmitOnError; deterministic files-only output with content-hash fingerprinted CSS/JS/images; header/redirect source files for the selected host; no private/server/TS-source leakage; legal links present without JS. Executable landing changes receive real quick checks and unknown-path rejection is preserved.
- **Validation commands:** `npm --prefix landing ci`, `npm --prefix landing run typecheck`, `npm --prefix landing run build`, `npm --prefix landing run check`, `npm --prefix landing test` (11/11 PASS); `bash scripts/tests/test-gate-selection.sh` (15/15 PASS); `git diff --check` PASS; `bash scripts/scan-secrets.sh` PASS.
- **Recovery / done:** Revert owned static/tooling/selection changes without weakening required checks. Record compiler revision, actual commands and task evidence; do not claim an unavailable tool/script passes.

<a id="p25-t03"></a>

### P25-T03 — Visual polish, legal pages and progressive enhancement

- **ID / priority / status:** P25-T03 / MUST / LOCAL_DONE (2026-10-05, issue #108).
- **Goal:** Deliver an attractive, readable, easy-to-use page with the abastevo identity, restrained calls to action and truthful legal/help pages.
- **Inputs / rules:** Landing specification, B-BR-L01–L04/L06/L07, BUC-L01–L03/L05; T01 copy/wireframes/tokens and T02 static artifact.
- **Dependencies:** P25-T02.
- **Files / risk:** Landing HTML/CSS, approved assets, privacy/deletion/terms pages and minimal optional DOM interactions in TS (active-section highlight, copy-link helper, back-to-top); standard risk.
- **Tests first:** Meaningful regression checks for introduced interaction, including keyboard/JS failure and truthful link states. Native anchors/details handle navigation/disclosure where sufficient.
- **Acceptance criteria:** Implementation follows the approved tokens/wireframes; illustrative source comparison is labelled fictional; legal pages state actual behavior and the contact channel without claiming legal approval. Declared responsive widths/zoom/browser support, focus/contrast/landmarks/alt text and reduced motion checked; essential navigation survives JS failure. No popup, urgency, autoplay or repeated conversion banner.
- **Validation commands:** `npm --prefix landing ci`, `npm --prefix landing run typecheck`, `npm --prefix landing run build`, `npm --prefix landing run check`, `npm --prefix landing test` (13/13 PASS); `bash scripts/tests/test-gate-selection.sh` (15/15 PASS); `git diff --check` PASS; `bash scripts/scan-secrets.sh` PASS.
- **Recovery / done:** Revert owned presentation/enhancement changes while preserving static content and links. Record revision and observed usability/accessibility results; no Android emulator/device acceptance is implied.

<a id="p25-t04"></a>

### P25-T04 — SEO, icons and indexing contracts

- **ID / priority / status:** P25-T04 / MUST / LOCAL_DONE (2026-10-05, issue #109).
- **Goal:** Complete accurate, discoverable initial HTML, icons/manifest, social previews and response policy using the reference's applicable SEO principles and official Google guidance.
- **Inputs / rules:** Landing specification SEO acceptance, B-BR-L01/L03–L05, BUC-L04, D-L02/D-L07; actual content/assets/origin configuration.
- **Dependencies:** P25-T03. Final public origin is required for public-artifact acceptance; private local preview can proceed with explicit nonpublic configuration.
- **Files / risk:** Landing static head/JSON-LD/robots/sitemap, icon set/manifest, versioned 1200×630 social image, headers file, optional `llms.txt`/`security.txt` and emitted-artifact checks; standard risk.
- **Tests first:** Origin/canonical/sitemap/ID consistency, malformed JSON-LD, placeholders, nonexistent store links or rating claims, local-resource errors, public/preview noindex cases, title/description length bounds and security/cache header expectations.
- **Acceptance criteria:** Accurate title/description/semantic HTML, absolute canonical/social URLs with `pt_BR` locale and large-card Twitter metadata, icons/manifest/theme-color from approved brand assets, factual JSON-LD with real `sameAs` only, small valid sitemap (home plus legal pages) and correct robots/indexing state with recorded crawler policy. Header baseline (nosniff, referrer, permissions, strict CSP, cache) matches the chosen host. Omit unsupported entity/rating/offer facts; no rich-result or ranking promise. No unnecessary city pages/blog/translations.
- **Validation commands:** T02 checks plus emitted HTML/XML/metadata tests for both store-state fixtures and indexing modes; applicable official structured-data validation; `git diff --check` and scoped secret review.
- **Recovery / done:** Restore prior consistent metadata/origin configuration and static artifact; record exact checks and pending public Search Console/field evidence separately.

<a id="p25-t05"></a>

### P25-T05 — Landing acceptance and publication handoff

- **ID / priority / status:** P25-T05 / MUST / LOCAL_DONE (2026-10-05, issue #110).
- **Goal:** Demonstrate G25-STATIC-READY and prepare a concrete static hosting/rollback and later Play Store activation procedure.
- **Inputs / rules:** Landing specification budgets/exit, B-BR-L01–L07, BUC-L01–L05 and task evidence T01–T04.
- **Dependencies:** P25-T04. Site publication requires selected domain/host/owner access and separate authorization; public app listing is not needed for prelaunch acceptance.
- **Files / risk:** Landing README/runbook, `docs/release-evidence/p25-static-landing.md` and current progress; standard risk. No automatic deployment or release.
- **Acceptance criteria:** Final artifact passes declared build/links/metadata/negative/store-state/accessibility/performance checks, with pinned tool/config/revision evidence. Record owed manual/field evidence honestly, including WhatsApp/Telegram link-preview observation. Hosting handoff covers HTTPS/canonical redirects/404/cache/headers/indexing, the publication-time discoverability checklist (README/repository website field, Play listing website field, Search Console and Bing Webmaster) and immutable-artifact rollback; activation verifies the future public listing and updates single link/badge, attribution, JSON-LD, sitemap and copy together, re-checking current Play policy destinations.
- **Validation commands:** Final scoped landing exit once using actual T02 interfaces and documented browser/lab tooling; `git diff --check`, secret/private-data review. At authorized integration only: `scripts/git-flow.sh finish --required "Quick verification" --pr <actual-number>` once, current-head/base guards and separately authorized wiki mirror.
- **Recovery / done:** Restore a previous verified static artifact; deactivate a removed store listing to the honest prelaunch state. LOCAL_DONE/INTEGRATED/WEBSITE_PUBLISHED are separate; no G09/G24/G18 acceptance, public pilot or search-position result follows from this gate.

## P26 — Landing educational guides

Priority: **MUST within the authorized website scope**. State: **PLANNED**. Entry: G25-STATIC-READY and approved landing foundation. Specification: [educational guides contract](docs/product/LANDING_GUIDES_CONTRACT.md). Exit: **G26-GUIDES-READY**, demonstrated acceptance for static educational guides, index hub, sitemap, JSON-LD and zero runtime dependencies.

Branch: `codex/phase-26-landing-guides`; T01–T05 run sequentially under milestone 20.

<a id="p26-t01"></a>

### P26-T01 — Guides content contract and structure specification

- **ID / priority / status:** P26-T01 / MUST / LOCAL_DONE (2026-10-05, issue #112). Output: [guides contract](docs/product/LANDING_GUIDES_CONTRACT.md).
- **Goal:** Freeze Portuguese educational guide content contracts, directory structure, factual citations, and truthfulness guidelines for 3 evergreen guides (ANP survey methodology, ethanol vs gasoline parity, interpreting prices and sources) and the `/guias/` index.
- **Inputs / rules:** Landing specification, B-BR-L01, B-BR-L04, B-BR-L05, BUC-L01, BUC-L04.
- **Dependencies:** G25-STATIC-READY.
- **Files / risk:** `docs/product/LANDING_GUIDES_CONTRACT.md`, `ROADMAP.md`; docs risk.
- **Acceptance criteria:** Bounded contract defining 4 URLs (`/guias/`, `/guias/pesquisa-anp/`, `/guias/etanol-ou-gasolina/`, `/guias/como-ler-precos/`), title/description length limits, semantic headings, factual non-affiliated ANP citations, and breadcrumbs without JS.
- **Validation commands:** Links/task consistency, `git diff --check`, `bash scripts/scan-secrets.sh`.

<a id="p26-t02"></a>

### P26-T02 — Evergreen guides and guide index implementation

- **ID / priority / status:** P26-T02 / MUST / PLANNED (issue #113).
- **Goal:** Author and build static HTML pages for `/guias/`, `/guias/pesquisa-anp/`, `/guias/etanol-ou-gasolina/`, and `/guias/como-ler-precos/`, integrating discreet guide links in landing navigation and footer.
- **Inputs / rules:** P26-T01 contract, B-BR-L01, B-BR-L02, B-BR-L04, BUC-L01, BUC-L02.
- **Dependencies:** P26-T01.
- **Files / risk:** `landing/static/guias/`; standard risk.
- **Acceptance criteria:** Authored static HTML pages using existing design tokens, accessible breadcrumbs, zero runtime dependencies, WCAG AA compliance, and working navigation without JS.
- **Validation commands:** `npm --prefix landing run build`, `npm --prefix landing run check`, `npm --prefix landing test`.

<a id="p26-t03"></a>

### P26-T03 — SEO metadata, JSON-LD schema, sitemap and discovery

- **ID / priority / status:** P26-T03 / MUST / PLANNED (issue #114).
- **Goal:** Complete initial-response SEO metadata, Article/TechArticle and BreadcrumbList JSON-LD schemas, sitemap entries, and discovery files for all guides.
- **Inputs / rules:** P26-T01 contract, B-BR-L04, B-BR-L05, BUC-L04.
- **Dependencies:** P26-T02.
- **Files / risk:** `landing/static/sitemap.xml`, `landing/static/llms.txt`, guide HTML `<head>`, `landing/scripts/build.mjs`, `landing/scripts/check.mjs`.
- **Acceptance criteria:** Strict bounds on titles (30–65 chars) and descriptions (110–165 chars); factual JSON-LD with unique IDs; valid sitemap with all 4 guide URLs; Open Graph and Twitter large cards.
- **Validation commands:** `npm --prefix landing run build`, `npm --prefix landing run check`, `npm --prefix landing test`.

<a id="p26-t04"></a>

### P26-T04 — Automated regression tests and gate verification

- **ID / priority / status:** P26-T04 / MUST / PLANNED (issue #115).
- **Goal:** Add and execute automated unit and integration tests verifying guides content, accessibility, schemas, sitemap, and performance budgets.
- **Inputs / rules:** B-BR-L01–L07, BUC-L01–L05, performance budgets.
- **Dependencies:** P26-T03.
- **Files / risk:** `landing/tests/`; standard risk.
- **Acceptance criteria:** Automated tests in `landing/tests/guides-content.test.mjs` passing; sitemap and JSON-LD test suites passing; budgets (<25 KiB CSS, <10 KiB JS, <500 KiB page transfer) respected.
- **Validation commands:** `npm --prefix landing test`, `bash scripts/tests/test-gate-selection.sh`, `git diff --check`, `bash scripts/scan-secrets.sh`.

<a id="p26-t05"></a>

### P26-T05 — Release evidence, hosting handoff and phase closure

- **ID / priority / status:** P26-T05 / MUST / PLANNED (issue #116).
- **Goal:** Document release evidence in `docs/release-evidence/p26-landing-guides.md` for G26-GUIDES-READY, update README/progress, and integrate via git-flow.sh finish.
- **Inputs / rules:** P26-T01–T04 evidence, DELIVERY_WORKFLOW.
- **Dependencies:** P26-T04.
- **Files / risk:** `docs/release-evidence/p26-landing-guides.md`, `PROGRESS.md`, `ROADMAP.md`, `landing/README.md`.
- **Acceptance criteria:** Evidence recorded; quick verification passed; PR review satisfied; guarded merge preserving commits to `main`; wiki synced.
- **Validation commands:** `bash scripts/quick-verify.sh`, `scripts/git-flow.sh finish --required "Quick verification" --pr <number>`.

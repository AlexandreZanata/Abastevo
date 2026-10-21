# Mobile expansion execution plan — vehicles, fast discovery and ANP occurrences

**Selected EV amendment, 2026-10-20:** the requested
[full EV/hybrid extension](EV_FULL_SUPPORT_EXECUTION_PLAN.md) and
[source qualification research](EV_SOURCE_RESEARCH.md) are part of this combined
target. Read its section 4 and [dependency map](EV_MEX_DEPENDENCIES.json) before
vehicle/ledger/sync schema or UI work. Add EE-00–31 without renumbering MEX;
77 combined execution units include one conditional operator-access unit.
[EV rules](../product/EV_SUPPORT_CONTRACT.md), [combined prompt](EV_SUPPORT_EXECUTION_PROMPT.md).
Do not complete the old liquid-only architecture and retrofit EVs afterward.

Date: 2026-10-20. Status: PLANNING_LOCAL_DONE / IMPLEMENTATION_PLANNED.
Scope: MEX-00–MEX-39, 40 parent tasks; MEX-20 has six mandatory children,
yielding 45 execution units and at least 45 meaningful implementation checkpoints
where changes are necessary. Accepted existing work is reused, not recommitted.
Planning acceptance is recorded in
[validation](MOBILE_EXPANSION_PLAN_VALIDATION.md). Start with the
[execution prompt](MOBILE_EXPANSION_EXECUTION_PROMPT.md).
Product rules/use cases: [B-BR-MEX-01–20 / BUC-MEX-01–10](../product/MOBILE_EXPANSION_CONTRACT.md).

## 1. Authority and boundaries

The requested deliverable is this complete plan and a prompt for another agent,
not implementation or publication now. Executing the supplied implementation
prompt selects this bounded scope. Keep ADR-019 app/staging prerequisites and
ADR-018/020 sequential construction; this plan creates no release shortcut.
Use backend contracts/integration before dependent new mobile journeys.

Confirmed amendment: one free vehicle; additional vehicles require Premium.
Retain existing source ceiling three until an explicit higher limit/capacity
amendment. This ceiling is not a new sales promise. The maintainer did not change
the PRM paid/free financial-feature matrix, so retain it. No invented prices,
provider credentials, unlimited quota or manual entitlement-grant endpoint.
Retain legacy vehicles/history and PRM data exit; no forced deletion on upgrade.

Include fast offline state/city search, usable vehicle/financial/cloud Android
journeys, national price questions, national official occurrence browsing and
safe station-level occurrence filters. Reuse OPT-A loading/refresh work; if
another effort completes it, validate/reconcile that implementation instead of
reimplementing it. Future D remains held until extreme consolidation and thousands
of real clients, unless explicitly requested earlier for named D scope. This
request does not reopen receipts, business portal, iOS, GPS trip tracking,
earnings/profit/fleet/tax features or commercial activation.

## 2. Inspected baseline and canonical ownership

Planning source: dev fb85d4d1, isolated codex/mobile-expansion-plan. The occupied
main-reconciliation worktree contained Home/week-picker/manual-sync work; the
primary checkout also had another effort's edits. Neither is included or changed.
Refresh all branch/source states at implementation opening; do not copy these
historical SHAs into new acceptance declarations.

- Current MaxRegisteredVehiclesRule permits three local vehicles. SaveVehicle
  checks count before save; changing a constant alone does not prove concurrent
  admission. PRM contract/policy/backend ceilings still use the old allowance.
- PRM02–08 have source evidence in
  [PRM_NEXT_TEN](../backend/evidence/premium/PRM_NEXT_TEN.md). Backend cloudsync,
  ledger, metrics and subscription modules exist. PRM09-T01 has Kotlin/Room
  source evidence; complete Android purchase/cloud/ledger/report journeys are
  not established. Real payment/provider and runtime proofs remain owed. Old
  plan headers saying NOT_STARTED do not override inspected implementation.
- Local municipality FTS already exists. MunicipalitySearchRepositoryImpl's
  fallback loads the whole catalog, and SearchMunicipalityUseCase enriches
  results individually and requires an imported week. Measure actual cost and
  separate location lookup from source-specific price availability.
- FQ native fines cover seven historical resources in recorded local acceptance:
  35,911 source rows = 35,612 accepted + 293 CPF quarantined + six metadata rows.
  These are historical evidence, not a fresh run or all Brazilian stations.
- FQ-07R6E1 scoped company read ports exist, but this review found missing scoped
  EXPLAIN, observed read-transaction concurrency and negative-case acceptance.
  Its two new tests do not establish all declared mandatory acceptance. Close
  those gaps before dependent exposure; do not erase the historical evidence.
- FQ-07R6E2 HTTP/root/cursor/ETag station composition, actual FQ-06 XLSX pipeline,
  safe revalidation/rebound, complete linked Android and FQ-16/17 are still
  obligations. Do not claim the synthetic enforcement adapter accepts actual
  source bytes. See [current FQ plan](FUEL_QUALITY_IMPLEMENTATION_PLAN.md) and
  [handoff](FUEL_QUALITY_REMAINING_HANDOFF.md); the handoff contains old historical
  sections, so current PROGRESS/Git/evidence govern actual continuation.

Actual enforcement prerequisite previously inspected in FQ source work:
`dados-fisc-a-partir-2019.xlsx` is a real ZIP/XLSX, unlike the fine history's
misnamed 2021 CSV. Its inspected profile has 11 columns: UF, municipality,
neighborhood, address, CNPJ/CPF, economic agent, inspected segment, DF date,
document number, inspection procedure and result. Previous private inspection
recorded 241,306 data rows, numeric spreadsheet dates and many distinct results
sharing a document/procedure; these numbers are a revalidation starting point,
not current source acceptance. Raw identity/address fields are minimized before
artifacts/logs. No source extraction date may be invented from ZIP timestamps.
Reacquire/verify only through approved FQ source rules; never commit raw files.

MEX is an orchestration/extension namespace, not a replacement for PRM/FQ/OPT.
Each task records completed, reused, repaired and genuinely new work separately.
Keep one implementation and one canonical acceptance per owning task. Do not
renumber existing IDs or create duplicate trackers for an implemented phase.

## 3. Architecture and user experience

Android keeps domain/application/data/app, Room, existing Compose navigation,
WorkManager and origin/account isolation. Read local data first; save private
work durably and display pending until acknowledged. Go remains the modular
monolith with net/http/chi/pgx/sqlc/slog, PostgreSQL/PostGIS and existing jobs.
No generic database replication or public-to-private cache mixing.

Private journey: Profile → vehicles → vehicle → entries / indicators / month
comparison; reuse existing fields/components. Premium cloud settings expose
opt-in, last acknowledgement, conflicts, devices and restore preview. Existing
free data exit survives lapse. Verify missing entitlement/provider safely;
source completion is not paid service activation.

Public journey: reuse Explore as the discovery entry. Search accepts state/city,
supported price questions and typed official-record questions with visible
interpretation. A filter sheet exposes fuel/source/period/location and eligible
occurrence types. Do not redesign bottom navigation without a demonstrated need.

Two official-record surfaces are mandatory:

1. National source-record explorer with source/context/detail, unknown/unmatched
   evidence and honest denominators. Known non-retail agents are separate from
   verified fuel-retailer stations; unknown segment is never guessed.
2. Directory station list/detail with safely attributed records. Separate
   current-company association from proven historical site association. Current
   company facts cannot silently become a current-address infraction badge.

Suggested consumer labels (localized and reviewed in MEX-02): "ANP records",
"Enforcement actions", "Source-reported infractions", "Fine records" and
"Quality assays"; company-only records say "Associated with the current
company; historical site not established". Missing results say "No records
found in the selected available sources and period". Avoid "clean station",
"adulterated fuel", "worst offender" or "all infractions in Brazil" inference.

Read-model contracts bind filters and source-revision vectors. A single-source
lease cannot represent a mixed-source national result. Keep source-global reads
independent of station verification. Page/filter/group/join work is bounded;
fresh Directory verification must batch rather than issue one query per card.
Freshness, erasure, correction, withdrawal and Directory changes invalidate the
appropriate projections, cursors and ETags. Do not return a stale scoped 304.

## 4. Mandatory per-task execution protocol

Read AGENTS, FAST_EXECUTION, current PROGRESS, DELIVERY_WORKFLOW at opening, this
task, owning contract and relevant implementation/tests only. Inspect dirty work
and dependencies before edits. Work in maintained dev when safe; otherwise use
an isolated codex/ worktree from a verified committed head. Never stash/reset or
include foreign edits. Capture base/head and changed files in task evidence.

Before behavior document B-BR/BUC, exact inputs, expected output, risk, source
compatibility and acceptance commands. Domain RED → GREEN → REFACTOR. Split an
oversized task into suffixed children before coding, preserving the parent gate.
Do not split to evade mandatory failure/concurrency acceptance.

For each microtask: implement → targeted meaningful tests and immediate critical
checks → affected build/format/static/contract/secret/diff review → evidence →
atomic English commit → next accepted dependency. Fix known failures now. Keep
PROGRESS concise and detailed evidence linked. No CI polling, per-task PR/merge
wait or repeated aggregate suite for documentation-only edits.

Evidence uses docs/backend/evidence/mobile-expansion/MEX-NN.md with exact tested
revision/tree, command/results, reused evidence justification, source/config,
rollback and limitations. Only complete local acceptance gets Status: LOCAL_DONE
and Validation: PASS; integration/provider/device/release remain separate.
IN_PROGRESS, EXTERNAL_PROOF_OWED and DEPENDENCY_PENDING are truthful alternatives,
not acceptance. Document remote/device dependencies and continue independent work
without inventing their success.

## 5. Sequential microtasks

### MEX-00 — Reconcile current work and canonical task ownership

**2026-10-20 checkpoint:** [LOCAL_DONE / INTEGRATION_PENDING](../backend/evidence/mobile-expansion/MEX-00.md).
Only the foundation audit was selected; next units remain unexecuted.
[Current continuation boundary](MOBILE_EXPANSION_CONTINUATION.md).

- Depends: current committed source and app-first prerequisite inventory.
- Deliver: map PRM/FQ/OPT tasks to actual code, commits/evidence and outstanding
  acceptance; inventory dirty work, build tools and enabled runtime flags. Record
  a minimal dependency map without copying archives or overwriting other fronts.
- Rules/use cases: B-BR-MEX-20; BUC-MEX-10. Risk: process/source integrity.
- Acceptance: resolve every reused checkpoint, identify pending changes, verify
  required files/commands and show no source/data/remote mutation.
- Recovery/commit: retain original worktrees; docs-only audit commit.

### MEX-01 — Freeze vehicle entitlement and retained-data amendment

**2026-10-21 checkpoint:** [LOCAL_DONE / INTEGRATION_PENDING](../backend/evidence/mobile-expansion/MEX-01.md).
[Frozen v2 contract](../product/VEHICLE_REGISTRATION_POLICY.md); runtime enforcement
is still MEX-07/08, not accepted by this contract checkpoint.

- Depends: MEX-00. Owns amendment to PRM00/PRM02 contracts, not duplicate services.
- Deliver: align old three-free references, machine policy, local/hosted/offline
  capability and upgrade/lapse/import rules to one free plus verified Premium
  within the current ceiling. Preserve previous professional feature matrix.
- Rules/use cases: B-BR-MEX-01/02/06; BUC-MEX-01/04. Risk: money/access/privacy.
- Acceptance: worked free-first/second, legacy-two/three, expired Premium,
  concurrent import and retained-data examples; policy compatibility vectors.
- Recovery/commit: no runtime limit change yet; atomic contract amendment commit.

### MEX-02 — Freeze typed queries, occurrence semantics and Android journeys

**2026-10-21 checkpoint:** [LOCAL_DONE / INTEGRATION_PENDING](../backend/evidence/mobile-expansion/MEX-02.md).
[Contract](../product/MOBILE_EXPLORATION_CONTRACT.md) owns frozen examples;
runtime parser/SQL/HTTP/Android acceptance stays with the downstream units.

- Depends: MEX-00/01; canonical FQ/PRM/OPT contracts.
- Deliver: typed query/schema examples, source/date/coverage vocabulary, source
  versus company/site semantics, filter availability and navigation/copy. Record
  stable compatibility vectors; no broad new score or private-report exposure.
- Rules/use cases: B-BR-MEX-08–18; BUC-MEX-06–10. Risk: public factual attribution.
- Acceptance: ambiguous cheapest/average question, L versus m3, unknown fuel,
  repeated snapshot, unmatched company, unavailable source and no-price station.
- Recovery/commit: revise unpublished contract; no fabricated live endpoint.

### MEX-03 — Pin performance baselines and test budgets
- Depends: MEX-00/02; OPT-A00/A06 and current P35 evidence.
- Deliver: pinned cold/warm search, typing/debounce, first useful content, SQL,
  payload, memory, scroll and request baselines. Set measured acceptance budgets
  before optimizing; distinguish query time, UI delay and network time.
- Rules/use cases: B-BR-MEX-19; BUC-MEX-05/09/10. Risk: false performance claims.
- Acceptance: reproducible synthetic workloads; full local city catalog, dense
  station/occurrence sets and private history; record device/runtime tests OWED.
- Recovery/commit: baseline-only commit; no unsupported instant/battery promise.

### MEX-04 — Batch local place lookup and availability
- Depends: MEX-02/03; existing Room/FTS schema and selection contracts.
- Deliver: batch catalog/availability reads, reused prepared catalog, separate
  location discovery from imported-week/source availability. Avoid repeated full
  metadata loads and per-result queries; retain stable IBGE identity.
- Rules/use cases: B-BR-MEX-07; BUC-MEX-05. Risk: SQL/selection compatibility.
- Acceptance: actual SQLite query counts/plans, no imported week, no-price city,
  accent/UF/code, schema upgrade if needed and existing selection regressions.
- Recovery/commit: forward-safe schema only, retain catalog; targeted commit.

### MEX-05 — Bound typo search and retire obsolete computation
- Depends: MEX-04.
- Deliver: bounded indexed candidate selection, off-main-thread approximate
  ranking, normalized queries and bounded reusable results. Remove per-keystroke
  findAll fallback without degrading documented matching or minSdk behavior.
- Rules/use cases: B-BR-MEX-07/19; BUC-MEX-05. Risk: performance/search correctness.
- Acceptance: long/adversarial query, typo/accent/prefix, rapid reversal, cancelled
  work, cold/warm full-catalog benchmark and measured candidate/memory bounds.
- Recovery/commit: disable approximate supplement, keep exact/prefix lookup.

### MEX-06 — Integrate fast state/city UI and measure perceived latency
- Depends: MEX-05; existing location UI, no new backend contract needed.
- Deliver: immediate bounded local state/city results, cancellation/generation
  fencing and a measured debounce policy. Preserve query/selection/scroll and
  honest price availability, accessibility and locales.
- Rules/use cases: B-BR-MEX-07/18/19; BUC-MEX-05/10. Risk: Android lifecycle.
- Acceptance: UI/JVM rapid input/resume/offline/no-week tests, lint/APK and
  before/after keystroke-to-visible results; native timing remains separately owed.
- Recovery/commit: scoped search-policy rollback, no data reset.

### MEX-07 — Enforce hosted vehicle capability atomically
- Depends: MEX-01 and accepted PRM01/02 source contracts.
- Deliver: repair/reuse existing cloud vehicle creation/import/restore admission,
  with live account/capability checks and preserved IDs/legacy data. Any schema
  change is append-only; do not create a separate billing/vehicle module.
- Rules/use cases: B-BR-MEX-01/02/06; BUC-MEX-01/04. Risk: critical auth/SQL/races.
- Acceptance: real PostGIS simultaneous creates, replay/conflicting commands,
  fourth ceiling, lapse during commit, cross-owner, restore and upgrade/rollback.
- Recovery/commit: disable new paid creation only; retained data/exit stay usable.

### MEX-08 — Enforce local free/Premium vehicle admission
- Depends: MEX-07 and typed capability contract, PRM09-T01.
- Deliver: transaction-safe Room admission, verified bounded offline capability,
  honest second-vehicle Premium entry and legacy/lapse behavior. Editing an
  existing vehicle cannot consume a new slot. Do not use count-then-save alone.
- Rules/use cases: B-BR-MEX-01/02/05; BUC-MEX-01. Risk: persistence/access/races.
- Acceptance: real SQLite concurrent inserts/reopen/upgrade, first/second,
  legacy-two/three, forged/expired/rollback-clock lease, edit/delete/retry.
- Recovery/commit: preserve IDs/history and basic free use; no destructive wipe.

### MEX-09 — Reconcile Go/Kotlin financial and wire parity
- Depends: MEX-01/07, PRM00 financial-v1 and PRM09-T01.
- Deliver: validate/reuse pure values, DTOs and shared vectors; distinguish
  configured estimated km/L from measured intervals and supported fuels/units.
- Rules/use cases: B-BR-MEX-03/04; BUC-MEX-02/03. Risk: exact money/math.
- Acceptance: full/partial/full anchor exclusion, discounts, overflow, gaps,
  reset, unknown GNV/flex, weighted ratios and same-result Go/Kotlin vectors.
- Recovery/commit: disable unsupported derived metric; preserve raw entries.

### MEX-10 — Reaccept existing ledger backend and month reports
- Depends: MEX-09; owns reuse/repairs of PRM05/06, not a second ledger.
- Deliver: audit actual endpoints/jobs/ownership and fix only gaps in raw entry,
  correction/deletion, monthly/cost/consumption projection and comparison APIs.
- Rules/use cases: B-BR-MEX-03–06; BUC-MEX-02–04. Risk: critical money/SQL/privacy.
- Acceptance: real PostGIS correction invalidation, concurrent edit, timezone
  boundaries, mismatched denominators, no double counting, export and erasure.
- Recovery/commit: scoped derived-report disable; raw data and free exit survive.

### MEX-11 — Durable Android ledger, drafts and command outbox
- Depends: MEX-08/10; PRM09-T01/T05.
- Deliver: extend existing owner/origin Room data through forward migrations,
  durable drafts, exact command bodies, expected revisions and restart recovery.
- Rules/use cases: B-BR-MEX-03/05/06; BUC-MEX-02/04. Risk: private persistence.
- Acceptance: actual SQLite upgrade/reopen/rollback, double-save, late retry,
  account/origin switch, crypto/privacy policy and lost acknowledgement recovery.
- Recovery/commit: stop affected writes, preserve existing rows and pending work.

### MEX-12 — Vehicle refuel/expense Android journeys
- Depends: MEX-11; owns PRM09-T05 functional screens.
- Deliver: vehicle entry list/add/edit/delete, full/partial/unknown, gap/reset,
  optional odometer/station, estimated efficiency configuration and truthful
  local/pending/conflict states using existing design components/locales.
- Rules/use cases: B-BR-MEX-02–05/18; BUC-MEX-01/02. Risk: money input/lifecycle.
- Acceptance: pt-BR decimal parsing, absent fields, process/draft recovery,
  rapid save/correction, offline and inaccessible paid action without data loss.
- Recovery/commit: retain readable/exportable entries; scoped UI rollback.

### MEX-13 — Monthly indicators and comparison Android journeys
- Depends: MEX-10/12; owns PRM09-T06.
- Deliver: month/vehicle totals, measured versus estimated metrics, cost/km
  window explanation, coverage, unknown reasons, comparison and safe held-data
  export. Do not implement revenue/profit or guess a full month denominator.
- Rules/use cases: B-BR-MEX-03/04/18; BUC-MEX-03. Risk: financial presentation.
- Acceptance: golden parity, timezone/month switch, stale correction, zero
  baseline, unequal distances, mixed units, CSV safety, JVM/lint/APKs.
- Recovery/commit: suppress invalid metrics, keep raw totals and data exit.

### MEX-14 — Wire private sync transport and constrained worker
- Depends: MEX-10/11; owns PRM03 and PRM09-T03 consumer wiring.
- Deliver: explicit allowlisted opt-in, stable IDs/revisions/cursors, live owner
  proof/capability, durable per-command acknowledgement, reconnect scheduling
  and bounded retry. A local save is not a cloud acknowledgement.
- Rules/use cases: B-BR-MEX-05/06; BUC-MEX-04. Risk: critical auth/jobs/privacy.
- Acceptance: actual transport/SQL two-device import/update/delete, lost reply,
  duplicate workers, late owner switch, lapse/revoke, offline pending and limits.
- Recovery/commit: disable uploads while retaining local records/queue safely.

### MEX-15 — Conflict, devices, backup and restore Android controls
- Depends: MEX-14; owns PRM09-T03/T04/T08 and reuses PRM04.
- Deliver: explicit conflict resolution, dataset/device opt-in/revoke, backup
  last-ack status, restore preview/confirm and free exit; pending local edits are
  not advertised as backed up. Preserve source deletion epochs during restore.
- Rules/use cases: B-BR-MEX-02/05/06; BUC-MEX-04. Risk: restoration/data loss.
- Acceptance: actual conflict/retry, restore-old-after-erase, process restart,
  account switch, expired snapshot, partial failure and privacy object receipts.
- Recovery/commit: cancel uncommitted restore, keep recoverable pre-restore data.

### MEX-16 — Premium status, acquisition boundaries and provider handoff
- Depends: MEX-08/15; owns PRM09-T02/T08 and PRM10 handoff, not sales activation.
- Deliver: functional verified status/manage/restore entry and offline/lapse
  copy. Handle missing products/provider configuration as unavailable, without
  fake purchases or production grants. Separate source and real provider gates.
- Rules/use cases: B-BR-MEX-01/02/05/20; BUC-MEX-01/04/10. Risk: entitlements.
- Acceptance: pending/cancelled-paid-through/expired/revoked/unknown/forged status,
  free functions retained and actual sandbox proofs only when separately allowed.
- Recovery/commit: paid acquisition OFF; document EXTERNAL_PROOF_OWED accurately.

### MEX-17 — Freeze ANP source/filter/census inventory
- Depends: MEX-00/02; owns selected FQ-00/06/07 source contracts.
- Deliver: current approved resources, exact types/fields/date precision,
  retail-agent classification, case-key limits, duplicates and full census.
  Record covered/missing files and data revisions; raw PII stays private.
- Rules/use cases: B-BR-MEX-10–14; BUC-MEX-07/08. Risk: source/attribution/privacy.
- Acceptance: independent whole-source oracle, actual headers/encodings/bytes,
  unknown statuses/segments, repeated snapshots and unsupported fields explicit.
- Recovery/commit: quarantine unsupported resources, retain last verified source.

### MEX-18 — Close scoped read prerequisite acceptance
- Depends: MEX-17 and FQ-07R6E1 source; repairs its incomplete acceptance.
- Deliver: actual reader transaction locking, fresh empty/changed fingerprint,
  no-source/legacy/missing context, cancelled/outage/guard-failure behavior,
  historical profile mapping and scoped query-plan evidence.
- Rules/use cases: B-BR-MEX-11/14/17; BUC-MEX-07–09. Risk: critical SQL/concurrency.
- Acceptance: observed concurrent retirement/status writer blocks through page
  and detail transaction completion; cancellation releases locks/no partial rows;
  realistic unrelated rows EXPLAIN; global history stays accessible.
- Recovery/commit: keep station attribution disabled until all required cases pass.

### MEX-19 — Complete scoped HTTP, contracts and root composition
- Depends: MEX-18; owns FQ-07R6E2.
- Deliver: actual sanctioned station filter, typed Directory ports, root wiring,
  OpenAPI, exact page/detail source scope, sealed filter/revision/fingerprint
  cursors and ETags. Reject unsupported action station filters until supported.
- Rules/use cases: B-BR-MEX-11/13/17; BUC-MEX-08/09. Risk: public data attribution.
- Acceptance: actual HTTP/PostGIS wrong-station/revision/withdrawal, filter
  mutation, duplicate parameters, stale cursor/304 after Directory change,
  missing port/outage, kill switch, privacy and global compatibility.
- Recovery/commit: disable scoped endpoint only; never fall back to global rows.

### MEX-20 — Complete actual enforcement native pipeline
- Depends: MEX-17/18; owns outstanding real-source FQ-06 work, split if oversized.
- Deliver: actual approved XLSX bounded ZIP/XML/native parser, date-system and
  expansion budgets, minimized typed artifacts, Go admission, compatible ledger,
  publication/discovery/worker/image and raw-cleanup recovery. Reuse source
  leases/jobs. Review need/license/security before any dependency addition.
- Rules/use cases: B-BR-MEX-10–12/16; BUC-MEX-07. Risk: critical ingest/SQL/PII.
- Acceptance: real full-file independent census; CPF/name/address exclusion;
  distinct same-document results, numeric dates, malformed/zip-bomb bounds,
  tamper/cleanup/replay/correction/concurrency/upgrade and actual image pipeline.
- Recovery/commit: suffix into bounded tested checkpoints; activation stays OFF.

MEX-20 is an orchestration gate, not one large implementation commit. Execute
these children sequentially; MEX-21 requires all six accepted or proven reused:
All six inherit MEX-20's rules/use case, critical risk and the per-task evidence,
format/static/secret review and commit protocol.

#### MEX-20a — Actual XLSX native reader
- Depends: MEX-17/18. Areas: tools/station-prep existing enforcement code/CLI.
- Deliver: pinned real profile, bounded ZIP/XML/shared strings/rows, calendar
  date-system handling and minimized source records preserving distinct results.
- Acceptance: independent actual-file row/header/hash oracle; expansion/DTD/XML
  refusal, corrupt archive, CPF/name/address exclusion, date precision and existing
  Rust regressions/fmt/clippy. No Go runtime acceptance inferred.
- Recovery/commit: refuse unknown profiles, retain old adapter; atomic native commit.

#### MEX-20b — Verified typed enforcement artifacts
- Depends: MEX-20a. Areas: owning prepared/native adapters and application values.
- Deliver: strict versioned manifest, hashes/bytes/census, bounded stream bridge,
  canonical IDs and exact native→Go values. No generic synthetic adapter bypass.
- Acceptance: late tamper/truncation/cancel/close/oversize/unknown profile returns
  no partial batch; actual Rust→Go full-source oracle and static/build checks.
- Recovery/commit: keep live registration absent; atomic typed-boundary commit.

#### MEX-20c — Compatible append-only enforcement storage
- Depends: MEX-20b. Areas: quality-owned migrations/sqlc/transaction staging.
- Deliver: source profile/clock/context and necessary fields through append-only
  schema, same-revision constraints and locked staging preserving old revisions.
- Acceptance: actual PostGIS upgrade/replay with retained-row hashes, invalid
  date/field/census cases, concurrent writers, withdrawal barrier and full rollback;
  sqlc deterministic output before compilation.
- Recovery/commit: forward repair, no applied-migration edits; atomic storage commit.

#### MEX-20d — Atomic enforcement publication and frozen reads
- Depends: MEX-20c. Areas: existing publisher/read ports/OpenAPI as affected.
- Deliver: exact source identity, optional backward-compatible provenance keys,
  replay/correction, safe source clocks and complete frozen global source reads.
- Acceptance: multiwriter convergence, failed later row leaves old pointer/data,
  actual HTTP field/privacy/full-page traversal and no fake event or finality date.
- Recovery/commit: disable new producer, retain old reads; atomic publication commit.

#### MEX-20e — Bounded file worker, discovery and static image
- Depends: MEX-20d. Areas: existing jobs/native/root/Docker/CI composition.
- Deliver: approved HTTPS discovery, trusted executable/private verified files,
  source lease, cleanup-before-publish, recovery and default-off producer/schedule.
- Acceptance: actual native worker→PostGIS correction/replay/ack-loss, cleanup
  refusal, bounded claim/dead-attempt preservation and nonroot static image smoke.
- Recovery/commit: imports/schedules OFF, no VPS change; atomic runtime commit.

#### MEX-20f — Actual whole-source enforcement acceptance
- Depends: MEX-20e. Areas: independent oracle, source/worker/read integration tests.
- Deliver: private pinned actual full-source run, every accepted/duplicate/
  quarantined/metadata row reconciled, distinct results retained, all result pages
  reachable and zero raw PII written to logs/Git. Freeze tested policy/config.
- Acceptance: actual Rust→Go→worker→PostGIS→read replay/correction/full traversal,
  privacy and affected matrix; no network/source load campaign or activation.
- Recovery/commit: fix acceptance failures in new commits, keep unsafe producer OFF.

### MEX-21 — Revalidation and conservative station/company association
- Depends: MEX-19/20; owns FQ-05W6/FQ-07 association obligations.
- Deliver: durable bounded revalidation after identity/status changes; separate
  dated PMQC rules from current-company facts and source-backed enforcement
  linkage. Define conservative inactive-to-active rebound without resurrecting
  an immutable superseded publication key or resetting dead-job attempts.
- Rules/use cases: B-BR-MEX-11/12/17; BUC-MEX-07–09. Risk: attribution/jobs/races.
- Acceptance: retirement/succession/suspension/rebound, missed job/restart,
  replay/correction/withdrawal, batch plans and no Directory writes/fuzzy binding.
- Recovery/commit: unassociate uncertain links, preserve independently read facts.

### MEX-22 — Source-record identity, safe grouping and date semantics
- Depends: MEX-17/20/21; owns remaining FQ-07 correlation rules.
- Deliver: stable source-record identities, documented case links only, explicit
  observation versus issued-case counts and separate clock basis per filter.
  Preserve multiple results with the same DF/procedure; no speculative merges.
- Rules/use cases: B-BR-MEX-10/12/13; BUC-MEX-07/08. Risk: factual loss/double count.
- Acceptance: repeated snapshot not extra fine, genuinely distinct source rows
  retained, correction/alias/reference changes and missing-event-date UNKNOWN.
- Recovery/commit: remove unsupported case link; retain minimized raw facts.

### MEX-23 — National source-record read API and honest coverage
- Depends: MEX-19–22; extends FQ-08 through owning module ports.
- Deliver: bounded global occurrence pages/detail and facets for supported type,
  status, geography, period/date-basis and segment. Include unmatched/unknown
  records, per-source editions/coverage and valid counts, without contributor PII.
- Rules/use cases: B-BR-MEX-10/12–17; BUC-MEX-07. Risk: public SQL/privacy/load.
- Acceptance: full keyset traversal with no omissions/duplicates, cross-source
  revision-vector consistency, unknown facet, missing file, correction, queries
  indexed on realistic unrelated records, limits/cancel and HTTP/OpenAPI.
- Recovery/commit: disable new facets/API, keep original source reads available.

### MEX-24 — Station occurrence filter query and projection
- Depends: MEX-21–23; Directory/quality module ownership stays explicit.
- Deliver: page unique safely associated stations by eligible occurrence type,
  region/period/status/scope, including no-price stations. Use bounded batched
  identity checks; introduce a rebuildable projection only if baseline justifies
  it. Never restrict a global page in the app and call it a complete result.
- Rules/use cases: B-BR-MEX-11/13–17; BUC-MEX-08. Risk: critical join/attribution.
- Acceptance: actual SQL selective predicates before limit, station/record count
  separation, live retirement at read, correction/withdrawal invalidation,
  multibranch ambiguity, no-price station and query-count/EXPLAIN budgets.
- Recovery/commit: remove occurrence filter capability, not canonical stations.

### MEX-25 — Public occurrence Android types and bounded persistence
- Depends: MEX-23/24 accepted APIs; reuses FQ-16/OPT public-cache contracts.
- Deliver: strict DTOs, unknown enums, typed company/site/source scope, source
  revision vectors, cancellable transport and bounded origin/filter cache where
  HTTP policy permits. Private reports cannot enter this official cache.
- Rules/use cases: B-BR-MEX-10/11/14/17; BUC-MEX-07–09. Risk: privacy/cache truth.
- Acceptance: real JSON/HTTP contracts, corrupt/oversize body, wrong origin,
  enum evolution, offline stale link disclosure, 304/404/no-store and Room upgrade.
- Recovery/commit: disable affected cache generation, never wipe other modules.

### MEX-26 — National occurrence explorer and source detail Android
- Depends: MEX-25; completes relevant FQ-14 official history journey.
- Deliver: discover → official records → filters → lazy list → source detail,
  with unmatched/other-agent groups, source dates, references and coverage.
  Explicitly different action/infraction/fine/assay labels, no guilt/trust badge.
- Rules/use cases: B-BR-MEX-10–15/18; BUC-MEX-07/09. Risk: consumer meaning/UI.
- Acceptance: bounded traversal, unknown/missing source versus valid empty,
  source/status/period change, cancellation/process restart and accessible locales.
- Recovery/commit: hide only new entry, keep established station/history routes.

### MEX-27 — Station filters and separated official timeline Android
- Depends: MEX-24–26; extends existing station list/profile, no duplicate catalog.
- Deliver: concise filter sheet, result summary and company/site-separated
  timeline. Preserve selected filters/page windows, no-price stations and source
  detail navigation. No "no infractions" filter inferred from incomplete data.
- Rules/use cases: B-BR-MEX-10/11/13–15/18; BUC-MEX-08/09. Risk: attribution/UI.
- Acceptance: real wire station filter, wrong/retired binding, unknown fuel,
  multi-filter/cursor changes, empty/unavailable, preserved pagination and font size.
- Recovery/commit: disable station occurrence filter without hiding global facts.

### MEX-28 — Typed price exploration and aggregation backend
- Depends: MEX-02/03; uses existing source prices/importer and their actual scope.
- Deliver: exact metric definitions, indexed min/max/average/variation queries
  and stable top/bottom ordering by compatible fuel/unit/condition/source/time.
  National base must be server-available; do not rank only downloaded phone data.
- Rules/use cases: B-BR-MEX-08/09/16; BUC-MEX-06. Risk: money/statistics/SQL.
- Acceptance: hand-derived totals/ties/weights, sampled coverage, unknown prices,
  corrections/expiry-without-write and indexed representative global query plans.
- Recovery/commit: disable unsupported ranking, retain source observations.

### MEX-29 — Public ranking endpoint, cache and correction invalidation
- Depends: MEX-28; measure before adding a reusable read-model/job.
- Deliver: bounded typed API, OpenAPI and source-snapshot-bound cursor/ETag.
  Projection or reuse only if it measurably saves work; no source fetching in
  handler, no new broker/search cluster and no stale eligible-population 304.
- Rules/use cases: B-BR-MEX-09/16/17; BUC-MEX-06/09. Risk: freshness/load/contracts.
- Acceptance: real SQL/HTTP concurrent fill, expiry, revision switch, outage,
  cold/warm bytes and SQL separately, bounded memory, API body and cancellation.
- Recovery/commit: switch to exact bounded uncached query or disable endpoint.

### MEX-30 — Deterministic question interpretation and suggestions
- Depends: MEX-02/23/29 accepted typed query vocabulary.
- Deliver: localized supported question intents, suggestions and visible chips
  for price and official-record exploration. Resolve ambiguous metrics/dates
  explicitly; no LLM, arbitrary SQL, scripts or remote-request inputs.
- Rules/use cases: B-BR-MEX-08–13; BUC-MEX-06/07. Risk: query meaning/adversarial input.
- Acceptance: paraphrases, accents/UF/city, ambiguous "most expensive fuel",
  unsupported fuel/unit/date, injection/long input and exact typed-query vectors.
- Recovery/commit: fall back to structured filters and suggested questions.

### MEX-31 — Master discovery Android price/record journeys
- Depends: MEX-26/27/29/30; retain one coherent Explore entry.
- Deliver: question/suggestion → visible interpretation → price ranking or
  official-record result → filters/detail/source; preserve context on return.
  Do not require whole-national download or refresh per keystroke over network.
- Rules/use cases: B-BR-MEX-08–18; BUC-MEX-06–09. Risk: Android performance/meaning.
- Acceptance: actual client/API vectors, ambiguous refinement, period/source
  mismatch, network/offline/cache states, stable ties/continuations and accessibility.
- Recovery/commit: disable master entry, keep existing direct navigation.

### MEX-32 — Shared loading, refresh policy and concise copy reconciliation
- Depends: MEX-06/13/15/27/31; canonical OPT-A00–A05 owns shared changes.
- Deliver: reuse/reconcile other agent's accepted work for local-first reads,
  single-flight event-based refresh, offscreen/offline stop, bounded retry and
  manual action. Remove every affected fixed-15-second UI claim, retaining
  useful source/age/standard-price information and loaded page windows.
- Rules/use cases: B-BR-MEX-05/17/18; BUC-MEX-09/10. Risk: lifecycle/cache regressions.
- Acceptance: slow request completes, simultaneous triggers converge, expiry
  does not renew, city/account switch, reconnect, all locales, pagination retained.
- Recovery/commit: event/manual-only refresh, retain truthful useful local data.

### MEX-33 — Private end-to-end regression and free boundary
- Depends: MEX-07–16/32; reuse PRM08/09 acceptance when inputs demonstrably match.
- Deliver: signed actual-client backend journey and meaningful Android/SQLite
  checks for all requested vehicle/financial/sync capabilities and free data exit.
- Rules/use cases: B-BR-MEX-01–06; BUC-MEX-01–04/10. Risk: critical privacy/money.
- Acceptance: first/second creation, legacy, two-device conflict, lapse during
  write, offline edit/reconnect, correction→report invalidation, restore after
  erasure and no lost IDs/data. Provider/native proof stays separately named.
- Recovery/commit: correct failures before acceptance; no test-weakening commit.

### MEX-34 — Official source-to-mobile acceptance and denominators
- Depends: MEX-17–27/31/32; owns extended FQ-16/17 local scope.
- Deliver: actual native source→typed artifact→Go publication→global and safe
  station query→Android decoding/filter/detail acceptance with full census and
  source-revision lineage. Pin retail versus other/unknown agents and association
  counts. No numeric trust calibration/publication in this batch.
- Rules/use cases: B-BR-MEX-10–17; BUC-MEX-07–09. Risk: attribution/loss/coverage.
- Acceptance: complete resource/row/page traversal; all accepted, quarantined,
  metadata/duplicate/unmatched counters reconcile; company/site disclosure,
  withdrawal/correction/retirement, no PII and zero false completeness claims.
- Recovery/commit: quarantine unsafe source/link, preserve other verified facts.

### MEX-35 — Discovery performance and resource acceptance
- Depends: MEX-03/06/28–32/34; canonical OPT-A06/07 and P24/P35 measurement rules.
- Deliver: before/after pinned search/ranking/filters and combined workload
  p50/p95, query counts, payload, memory and first-content times. Check foreground
  request reduction and that safety/private jobs are not starved.
- Rules/use cases: B-BR-MEX-16/19; BUC-MEX-05–10. Risk: capacity/false optimization.
- Acceptance: cold/warm/full catalog/dense records/rapid typing/outage/correction;
  compare fixed budgets; prepare device Macrobenchmark where existing tooling
  allows, execute native/battery only under current authorization/gates.
- Recovery/commit: select simpler measured implementation; label external proofs OWED.

### MEX-36 — Privacy, access and all-copy recovery acceptance
- Depends: MEX-33/34/35; PRM/privacy existing ownership remains canonical.
- Deliver: verify no private ledger/report/owner overlay enters public search,
  ranking, source files, cache/logs/fixtures. Reaccept owner isolation, free
  export/delete and retained-data rights after cache, restore and account change.
- Rules/use cases: B-BR-MEX-02/05/06/14/17; BUC-MEX-04/07–10. Risk: critical privacy.
- Acceptance: actual DB/object negative erasure/restore/retry proof where changed,
  no real PII in Git/logs, unsafe CSV formula refusal and public kill-switch tests.
- Recovery/commit: disable affected capability; retain deletion receipts/recovery.

### MEX-37 — Accessibility, compatibility and final scoped union
- Depends: MEX-33–36; selected immutable combined code/config inputs.
- Deliver: complete affected local matrix once, navigation/free feature and
  locale review, append-only PostgreSQL/Room upgrade compatibility, docs/links,
  manifests and source feature-flag/capability inventory.
- Rules/use cases: B-BR-MEX-01–20; BUC-MEX-01–10. Risk: cumulative regressions.
- Acceptance: affected JVM/lint/APKs, real critical Go race/PostGIS/contracts,
  Rust pipeline as affected and measured scope; native TalkBack/font/offline/device
  scenarios prepared or run only when actually allowed. No false skipped PASS.
- Recovery/commit: source checkpoint with precise local/provider/native states.

### MEX-38 — Operational and external acceptance handoff
- Depends: MEX-37; owns relevant PRM10/FQ-17 operational handoff only.
- Deliver: immutable candidate/config/schema/source/policy/census manifest,
  forward repair, disable/rollback, staged resource/privacy/provider proofs owed.
  Reconcile actual staging revision; old readiness/schema95 is not evidence for
  current source109 or later. Planning does not enable any source or paid feature.
- Rules/use cases: B-BR-MEX-16/19/20; BUC-MEX-10. Risk: operations/release confusion.
- Acceptance: local runbook/rehearsal as scoped; list exact external blockers.
  Deployment, namespace mutation, paid activation and native execution require
  separately valid authority. D/iOS/G09/public pilot remain outside acceptance.
- Recovery/commit: honest LOCAL_DONE with separate EXTERNAL_PROOF_OWED manifest.

### MEX-39 — Final review and delivery boundary
- Depends: all required local gates; no known local failure or hidden TODO path.
- Deliver: Portuguese overview, task/commit/test/measurement mapping and pending
  external obligations. Verify all owned commits remain reachable and foreign
  work unchanged. Default implementation prompt ends at local committed delivery.
- Rules/use cases: B-BR-MEX-20; BUC-MEX-10. Risk: process/history integrity.
- Acceptance: complete status map; no speculative complete/synced/provider claim.
  If final remote delivery is explicitly authorized, reconcile fresh main
  normally, revalidate affected union, use protected dev→main PR and guarded
  finish with actual required Quick verification plus applicable checks/reviews;
  changed head/base invalidates prior evidence. Never force/admin/direct-main.
- Recovery/commit: retain dev/occupied worktrees; pending CI means CI_PENDING,
  no polling. Wiki mirrors only after an authorized merged batch; failed wiki
  means WIKI_PENDING, not a new backend suite or bookkeeping PR.

## 6. Validation commands and task completion

Resolve actual commands/tools in backend/README.md, docs/backend/TEST_STRATEGY.md
and existing Android task evidence. Record exact selectors before changing code;
never run a selector with zero matching tests and call it acceptance. Examples
are command families, not new already-existing test names:

```sh
# Root: focused Android module suites; select actual affected test classes.
./gradlew :domain:test :application:test :data:testDebugUnitTest :app:testDebugUnitTest
./gradlew :app:lintDebug :app:assembleDebug :app:assembleDebugAndroidTest :data:assembleDebugAndroidTest
# Backend directory: scoped package lists, not a repeated full release gate.
go test -race -count=1 -tags=integration ./internal/modules/quality/adapters/read ./internal/modules/quality/adapters/http
go test -race -count=1 -tags=integration ./internal/modules/cloudsync/... ./internal/modules/ledger/... ./internal/modules/metrics/...
go vet ./internal/modules/quality/...
sqlc vet
# Root: per-task hygiene, changed-file secret/PII review and repository guard.
git diff --check
```

For Go integration use only the documented disposable local PostGIS fixture;
tests create/drop their own databases. Never point destructive fixtures at VPS,
shared production or persisted customer data. Build/export the required Rust
preparers exactly as backend/README specifies. sqlc generate is sequential and
must finish with reviewed deterministic generated changes before compilation.
Rust tests/fmt/clippy use the actual pinned toolchain/lockfile. Kotlin/Go static
checks, real SQLite upgrade tests, OpenAPI/goldens and secret-surface review are
required when affected. Add dependencies only with recorded need/license/security.

Complete local scope means every required local BUC and failure path executes
against actual implementations, with supported source denominators and honest
unknown states. A mock-only happy path, compiled APK, accepted plan or old test
result cannot establish runtime/provider/current-head acceptance.

## 7. Delivery states and references

This planning batch is local documentation only. The execution prompt authorizes
local implementation/atomic commits when the human submits it, with no automatic
push/PR/merge/wiki/deploy. Reuse valid explicit later publication authority, not
a reference document or a historical PR number. Preserve all other fronts.

Canonical owners:
- [PRM plan](PREMIUM_BACKEND_APP_PLAN.md), [product](../product/PREMIUM_PRODUCT_CONTRACT.md),
  [financial method](../product/PREMIUM_FINANCIAL_METHOD.md), [source policy](../product/PREMIUM_SOURCE_POLICY.md).
- [FQ plan](FUEL_QUALITY_IMPLEMENTATION_PLAN.md), [E1 read evidence](../backend/evidence/fuel-quality/FQ07R6E1_SCOPED_COMPANY_READS.md).
- [OPT-A](HOME_OFFLINE_EFFICIENCY_PLAN.md), [A–D master](APP_CONSOLIDATION_PLAN.md).
- [Delivery workflow](DELIVERY_WORKFLOW.md), [test strategy](../backend/TEST_STRATEGY.md), [backend commands](../../backend/README.md).

# Forum Abastevo implementation plan

Status: PHASE 1 IN_PROGRESS. Planning completed 2026-11-05; Phase 1 opened at FOR-00.
Inspected source: **GitHub main `3fc77acc6d3dc85819cbc9d00cadaec056448d0a`**
(PR #191), normally fast-forwarded from the initially inspected PR #190
`b8e93a41` after main advanced during planning. The detached local checkout at `e22824db` and all its uncommitted
work were excluded. Use current GitHub main at every new phase opening; never
derive the baseline from an old local branch or old PROGRESS readiness claim.

## Read order and authority

1. AGENTS, FAST_EXECUTION, current PROGRESS, this task and DELIVERY_WORKFLOW.
2. [Product rules and use cases](../product/FORUM_ABASTEVO_CONTRACT.md).
3. [Architecture, schema and API](../backend/FORUM_ABASTEVO_ARCHITECTURE.md).
4. [Source/rights investigation](../backend/FORUM_ABASTEVO_SOURCES.md).
5. [Android screens and state](../mobile/FORUM_ABASTEVO_ANDROID.md).
6. [Test, operations and acceptance matrix](FORUM_ABASTEVO_VALIDATION.md).
7. [Short implementation prompt](FORUM_ABASTEVO_IMPLEMENTATION_PROMPT.md).

This is an explicitly requested future Forum overlay. It does not silently
reorder P34–P38, reopen iOS, certify G09, authorize source collection, enable
charges, deploy to staging/production or implement the feature now. A later
explicit implementation request selects the Forum task range. Existing
app/backend safety, account and privacy gates remain dependencies, not “done”
because the modules exist. `teste.abastevo.com.br` is staging only.

## Current-state diagnosis and traceability

Evidence below is from actual main source inspection. “Exists” means source and
test files exist; no existing suite was rerun to certify their runtime during
this documentation-only investigation.

| Capability | Already exists? | Reuse | Create/change for Forum | Evidence / affected files |
|---|---|---|---|---|
| Community entry and price activity | Yes, source + native/JVM tests | LazyColumn, city, contribution, station rows | One new item above contribution; separate route | [CommunityScreen](../../app/src/main/kotlin/com/anpfuel/app/ui/community/CommunityScreen.kt), [test](../../app/src/androidTest/kotlin/com/anpfuel/app/ui/community/CommunityFeedScreenTest.kt) |
| Price feed state/cache | Yes | Cancellation and bounded-cache patterns | Independent Forum state; no price model reuse | [ViewModel](../../app/src/main/kotlin/com/anpfuel/app/ui/community/CommunityFeedViewModel.kt), [cache](../../data/src/main/kotlin/com/anpfuel/data/repository/CommunityFeedCacheStore.kt) |
| Four-tab navigation | Yes | Existing graph and tab-base/back contracts | Forum route constants/composables and child selection mapping only | [graph](../../app/src/main/kotlin/com/anpfuel/app/navigation/AnpNavGraph.kt), [bar](../../app/src/main/kotlin/com/anpfuel/app/navigation/AbastevoBottomBar.kt), [native test](../../app/src/androidTest/kotlin/com/anpfuel/app/navigation/BottomTabNavigationDeviceTest.kt) |
| Compose design system | Yes | Material tokens, shared shape/outline, existing action controls | Text-first cards and accessibility states | [guidelines](../mobile/UI_CONTAINER_GUIDELINES.md), [ActionCardButton](../../app/src/main/kotlin/com/anpfuel/app/ui/components/ActionCardButton.kt) |
| Free account/session/key binding | Yes, source and tests | Account validation, proof verifier, current binding | Forum composition ports; transaction-time live checks | [API composition](../../backend/cmd/api/main.go), [account](../../backend/internal/modules/account/domain/ports.go), [proof](../../backend/internal/modules/identity/adapters/auth/auth.go) |
| Station comments/replies/edit/delete | Yes, target-specific | Validation/CAS/SQL test patterns | Forum-owned posts/comments; never station FK workaround | [comments](../../backend/internal/modules/feedback/application/comments.go), [schema](../../backend/db/migrations/000024_feedback_comments.sql) |
| Votes | Yes, feedback VALID/INVALID | Abuse lessons only | None in baseline; no like/relevance conversion | [votes](../../backend/internal/modules/feedback/application/votes.go) |
| Moderation | Yes, no Forum target | Existing cases, audited actions, restricted CLI | Typed Forum targets, SQL checks, visibility/dispatch and admin queue integration | [targets](../../backend/internal/modules/moderation/domain/case.go), [actions](../../backend/internal/modules/moderation/adapters/actions.go), [ops](../../backend/cmd/ops/moderation.go) |
| Admin web and staff roles | Yes; roles alone grant no permissions | Existing session/CSRF/fence/audit and UI shell | Forum editorial/review section under actual admin authority | [admin HTTP](../../backend/internal/modules/admin/adapters/http/http.go), [web](../../admin-web/src/app.ts), [roles](../../backend/internal/modules/staff/domain/catalog.go) |
| Queue/scheduler/worker | Yes, including isolated safety lanes | Existing infrastructure, bounded retries/leases | Forum source/bulletin/cleanup handlers and budgets | [scheduler](../../backend/internal/platform/jobs/schedule.go), [worker](../../backend/cmd/worker/main.go) |
| Idempotency and rate limiting | Yes, contributor-keyed replay has limits | Existing identity transaction runner and limiter | Account/operation create uniqueness; Forum admission policies | [runner](../../backend/internal/modules/identity/adapters/idempotency.go), [identity schema](../../backend/db/migrations/000004_identity.sql) |
| Official data and exact amounts | Yes, not ready-made Forum bulletins | Accepted revision provenance; bounded sums/counts/ranking | Owned aggregate port, deterministic methodology/template | [queries](../../backend/internal/modules/official/application/queries.go), [ranking SQL](../../backend/db/queries/official/ranking.sql) |
| Alerts/subscription | Both exist on current main | Optional alert infrastructure later | No subscription requirement; no Forum notifications in baseline | [alerts](../../backend/internal/modules/alerts/application/ports.go), [subscription](../../backend/internal/modules/subscription/application/ports.go) |
| Account privacy/erasure | Yes, Forum absent | Durable account recovery and restore ledger | Forum footprint export/erase and completion dependency | [recovery](../../backend/internal/modules/account/application/erasure.go), [worker](../../backend/cmd/worker/main.go) |
| Forum source ingestion, data, API, screens | Absent in inspected module/route/schema paths | Existing primitives above | All named Forum behavior and tests | New `backend/internal/modules/forum/`, Forum SQL/OpenAPI and Android files proposed in linked designs |

The initial occupied checkout was older and lacked some modules; that was not
used to conclude they were absent from GitHub main. Current main also contains
ADR-021 community-first contributions, so obsolete mandatory-GPS assumptions
must not leak into this independent Forum plan. Source presence is distinct
from tested local behavior, deployed staging and release certification.

## Scope and economics

Main recommendation: modular `forum`, same API/worker/PostgreSQL/admin, no service
addition and no mandatory paid provider. Manual editorial reading is the MVP.
No attachments/images, reactions, follows, personalized/relevant ranking, push,
live updates or new outbox. Later phases add the requested core discussion and
deterministic information without paying for LLMs. Operating costs are database
and transfer, modest scheduled requests and human editorial/moderation time;
the last is a real launch constraint, not “free because open source”.

Each task below is one meaningful atomic commit. If a task becomes larger than
one reviewable behavior change, add letter-suffixed subtasks with acceptance
before coding; keep one active subtask. No repeated broad suite for evidence-only
edits, no phase PR/CI waits, no invented pass results. The final construction
batch gets one protected PR and current-head required checks.

## Shared task contract

Before each task record B-BR/BUC IDs, exact current-main baseline, files, risks,
RED case, GREEN commands/results and rollback. Commit tests with implementation,
not a deliberately failing intermediate head. Update compact PROGRESS and the
phase evidence record with tested source, actual counts/failures/skips and next
task. Every commit runs affected formatting/static checks, `git diff --check`
and secret review. Critical SQL/auth/privacy/jobs require immediate real-DB,
negative/concurrency/failure tests, before a consumer is implemented.

Backend schema/API is validated before its Android consumer. Per-phase closure
is LOCAL_DONE / INTEGRATION_PENDING only for the named accepted range. Native
tests compiled but not executed are OWED; follow existing final-device cadence.
Do not turn missing source permission, operator coverage or device access into a
false whole-phase acceptance. Complete independent local work while preserving
the named blocked obligation. Test command groups are defined in the validation
document so a smaller model need not invent a gate.

## Phase 1 — editorial foundation / first useful release

Dependencies: current main reconciled; existing G09-LOCAL source integration and
account/admin/identity contracts inspected; no live-deploy assumption. Covers
B-BR-FOR-01–07/12/14/16 and BUC-FOR-01/02. SQL/auth changes are critical;
Compose entry is low-risk presentation. Proposed evidence:
`docs/backend/evidence/forum/FOR_PHASE_1.md` (created during implementation).

### FOR-00 — Reconcile actual main and freeze scope (LOCAL_DONE)

- Status: LOCAL_DONE. [Phase 1 evidence](../backend/evidence/forum/FOR_PHASE_1.md).
  Baseline reconciled to GitHub main `3fc77acc` and `dev` `6857ec63`.
  Database migration `000157` and Room version `37` allocated; worktrees
  isolated; scope frozen to B-BR-FOR-01–07/12/14/16 and BUC-FOR-01/02.
- Fetch GitHub main; inspect status/diff/worktrees, current API/admin/Room schema,
  accepted app-first evidence and source inventory. Synchronize maintained dev
  normally without overwriting occupied work. Record exact baseline and preserve
  unrelated commits/edits. [ADR-020](../adr/020-maintained-dev-main-delivery.md)
  governs delivery, even when implementation uses an isolated worktree.
- Files: this plan, current PROGRESS and phase evidence. Check all linked code
  paths, task/rule IDs and any changed assumptions. Acceptance: no inferred
  runtime gate; a clear owned task range and exact source head. Commit
  `docs(forum): reconcile main baseline and implementation scope`.

### FOR-01 — Freeze behavior, privacy and contracts

- Freeze proposed text/admission limits, premoderation, source rights modes,
  retained metadata, owner read and external-link policy. Add normative OpenAPI
  schemas/negative vectors only for the foundation routes being implemented;
  preserve non-Forum contracts. Record terms/operator and source approvals owed.
- Files: product/architecture/source docs, `contracts/openapi/v1.yaml`, future
  contract vectors and apicontract tests. Validate YAML/schema and malformed
  enum/body/visibility cases. Acceptance: one unambiguous MVP contract;
  PLANNED still means no routes claimed live. Commit `docs(forum): freeze v1
  behavior and editorial wire contract` (tests/contract changes in same commit).

### FOR-02 — Pure editorial domain and ports

- New Forum domain/application: post/source/category validation, rights/state
  transitions, public projection, edit revision, feature switches. Inject
  clock/IDs. No net/http/pgx in pure domain.
- RED→GREEN table tests: invalid UTF-8/scalars, missing source/date/permission,
  invalid transition, hidden/removed visibility, unknown category, default-off.
  Run B1. Acceptance: deterministic rules without persistence. Commit
  `feat(forum): define editorial domain and read ports`.

### FOR-03 — Foundation SQL and migration recovery

- Add four foundation tables/indices described in architecture, sqlc-owned
  queries and pgx adapters. Implement serialized publication sequence and
  bounded feed projection. Allocate current free migration number.
- Run B1+B2 immediately: real empty/upgrade/repeat migration, FK/check/unique
  negatives, same-key publication concurrency, late commits, hide/update
  revision rollback and EXPLAIN. Acceptance: no duplicate/order leak and no N+1.
  Commit `feat(forum): persist editorial posts and stable feed snapshots`.

### FOR-04 — Existing admin editorial publication

- Extend existing admin handlers/transaction audit and `admin-web/src` for
  draft/preview/edit/review/hide. No public author fields or independent login.
- Run B1+B2+W: missing/expired admin, Origin/CSRF/CIDR denial, repeated command,
  stale revision, audit failure rollback, disabled surface 404, missing rights.
  Acceptance: create and publish an original synthetic fixture in disposable
  tests only, and hide it consistently; original text never in audit/logs.
  Commit `feat(forum): add audited editorial controls to admin`.

### FOR-05 — Public read API and composition

- New Forum HTTP handlers wired in existing API; category/feed/detail only.
  Reuse HMAC cursor and error envelope; no-store initially.
- Run B1+B2+C: anonymous read, hidden/draft 404, query/body limits, cursor TTL/
  scope tamper, snapshot changes, disabled gate and DB failure. Acceptance:
  actual API process and sqlc/OpenAPI projections agree. Commit
  `feat(forum): expose public editorial read endpoints`.

### FOR-06 — Android domain/application contracts

- Add `domain/.../forum`, `application/.../usecase/forum` ports/use cases following
  existing modules. No Android imports in domain. Explicit supported/unknown
  kind and error results; exact source dates and safe link rules.
- Run A1 with named Forum tests: mapping, null provenance, stale scope,
  auth-needed and HTTPS link refusal. Acceptance: fixture-backed Go/Kotlin
  contract mapping, no speculative server fields. Commit
  `feat(forum): add Android forum models and use cases`.

### FOR-07 — HTTP, Room cache and Hilt

- Add Forum client/repository/DI plus versioned Room entities/DAO/migration.
  One configured origin; cancellation, byte caps, five-scope/2 MiB/24h cache.
- Run A1 and targeted Room migration/integration tests: malformed/oversized
  JSON, cancellation/late reply, expiry/eviction, origin separation, refresh
  replacing cursor, live 404 invalidation, upgrade preserves existing data.
  Acceptance: editorial works offline without private/UGC persistence. Commit
  `feat(forum): add bounded editorial transport and offline cache`.

### FOR-08 — Community card, Forum feed/detail and routes

- Add card in the exact requested position, separate screens/ViewModels/routes,
  minimal child-route selection mapping, strings in all supported locales.
  No other Community behavior change. Follow shared container rule/insets.
- Run A1+A2; add A3 cases for real controller back/scroll/tab selection and
  theme/font/empty/error states. Acceptance: enabled users can read real server
  data or honest empty state; contribution and all four tabs remain functional.
  Native execution may join the already deferred final batch, explicitly OWED.
  Commit `feat(forum): add Community entry and editorial reading screens`.

### FOR-09 — MVP checkpoint and safe degraded operation

- Prove original editorial admin→public API→Android contract flow, source-link
  refusal, network loss/cache expiry, independent switches and price/map
  non-regression. Conditional HTTP cache is optional only after its revocation
  tests; otherwise keep no-store. Record content/operator readiness separately.
- Run changed-scope B1+B2+C+A1+A2 and phase-native cases when due. Reuse matching
  previous outputs instead of rerunning the same aggregate. Acceptance: MVP
  source complete, device/live/legal obligations explicit; default rollout off.
  Commit `test(forum): record editorial MVP acceptance and rollback`.

Phase rollback: disable Forum reads/entry and collection; keep prices/maps and
append-only schema intact. Restore previous compatible API/Android build only
after checking schema compatibility. No down-migration/drop or production action
in local acceptance. Verify via gates plus human review of source attribution.

## Phase 2 — authorized news candidates

Depends on FOR-09; B-BR-FOR-02/03/08/12/16, BUC-FOR-03. Proposed evidence
`docs/backend/evidence/forum/FOR_PHASE_2.md`. Critical egress/jobs/SQL scope.

### FOR-10 — Approve one real source or record block

- Revalidate candidates, actual machine URL/redirects/dates, per-item title/
  excerpt rights and commercial fit; freeze safe permitted representative
  fixtures with provenance/notice. Never commit copied full articles/photos.
- Files: source register, source config seed/test fixture metadata, terms links.
  Acceptance: one explicit rights decision and fetchable parse contract, or
  NEWS_SOURCE_BLOCKED with editorial-only fallback. Tests alone cannot approve
  rights. Commit `docs(forum): record source access and publication decisions`.

### FOR-11 — Safe feed parser and fetch adapter

- Implement stdlib RSS/Atom metadata parser and bounded allowlisted HTTP client;
  add ingestion-run migration/sqlc only when needed. HTML-only sources stay off
  until their own adapter is justified. No generic crawler.
- Run B1+B2 security suite: private/mapped IP/metadata, rebinding, redirects,
  downgrade, oversized compressed body, deep/hostile XML, missing timezone,
  invalid URLs and cancelled connection. Acceptance: every egress decision fails
  closed and parser never executes external content. Commit
  `feat(forum): add bounded authorized-source feed adapters`.

### FOR-12 — Scheduled collection and deduplication

- Existing worker handler/schedule, source/run leases/checkpoint, conditional
  GET, transactional GUID/URL dedupe, deterministic classifier, capped retries.
- Run B1+B2+J: two workers/schedulers, same/different GUID+URL, restart after
  fetch/partial commit, 304/429/503, source disable/expiry, timeout, cancellation,
  poisoned entry and rights revocation. Acceptance: one draft and no direct
  publication; existing privacy safety jobs progress during blocked fetch.
  Commit `feat(forum): schedule deduplicated news candidate collection`.

### FOR-13 — Editorial review and news checkpoint

- Extend current admin candidate queue/rights view, correction workflow and
  run/source metrics; no new admin shell. Confirm source changes return to
  review and rejected duplicates do not reappear.
- Run W+B1+B2+J+C, review original-source credit and Android NEWS rendering.
  Acceptance: source enabled only with approval; otherwise implementation may
  be source-ready but operational news remains BLOCKED, never accepted.
  Commit `feat(forum): review and publish authorized news candidates`.

Rollback: disable source collection and auto-publication, retain last accepted
editorial content; hide affected source posts through audited action if rights
require it. No blanket deletion or pressure on evidence safety jobs.

## Phase 3 — safe discussion on published posts

Depends on FOR-09 and current admin/account/privacy source acceptance; news is
optional. B-BR-FOR-04–14, BUC-FOR-04/05/06/09. Evidence:
`docs/backend/evidence/forum/FOR_PHASE_3.md`. Critical auth/privacy/SQL scope.

### FOR-14 — Comment/reply domain and persistence

- Add pending-by-default comments/replies, scoped sequence, CAS/tombstone,
  account-operation uniqueness and budget transactions. Schema/sqlc/adapters.
- Run B1+B2: Unicode/280 bound, reply depth, foreign/cross-post parent,
  concurrent same intent/different keys, quota race, edit/remove/post-hide race,
  sequence rollback. Acceptance: no new rows leak before approval. Commit
  `feat(forum): persist bounded reviewed discussion threads`.

### FOR-15 — Signed owner and command admission

- Wire existing session/proof/binding and transaction runner; stable logical
  intent hash excluding refresh credentials; private owner projection. Extend
  OpenAPI/contract fixtures for comments/owner routes.
- Run B1+B2+C: missing/forged/reused/wrong-authority proof, expired/revoked/
  unbound session, other-owner IDOR, suspension/deletion during commit,
  cross-key replay, changed body and seven-day expiry. Acceptance: proof or
  account failure cannot publish/read private content. Commit
  `feat(forum): require bound signed accounts for discussion commands`.

### FOR-16 — Privacy, notices and moderation readiness

- Integrate Forum export and batched account erasure with existing recovery/
  restore, never completion before all modules succeed. Define reviewed terms,
  legal-notice contact, author/reporter decision/appeal handling and operator
  coverage in current policies. This is a release gate, not a new service.
- Run B1+B2+J: owned export excludes other owners; crash/restart, original text
  scrub, idempotency-response scrub, late writer, restored backup and failure
  acknowledgement. Acceptance: no resurrection and real operator responsibility
  recorded. Commit `feat(forum): integrate account rights and erasure recovery`.

### FOR-17 — Discussion HTTP endpoints

- Add public comment/reply pages and signed create/edit/remove/private owner
  read; safe receipts, no-store, size/rate errors, same operation retry.
- Run B1+B2+C: all error/status cases, pagination under concurrent insert/hide,
  actual signed API→PostGIS flow, expired access refresh, DB outage and replay.
  Acceptance: no forged author/private fields or public pending text. Commit
  `feat(forum): expose bounded discussion and owner commands`.

### FOR-18 — Reports, review queue and action integration

- Add receipt uniqueness and real Moderation FORUM_POST/FORUM_COMMENT targets,
  migration CHECK updates, transactional case open/audit, existing ops dispatch
  and admin queue/actions. Reuse privileges; badges still grant nothing.
- Run B1+B2+J+W: duplicate/account-target report race, report does not hide,
  target missing, audit failure, admin denial, hide vs remove/erase, stale
  restore, safe reporter/author reason/appeal receipts. Acceptance: users can
  report and operators can act with an auditable recovery path. Commit
  `feat(forum): integrate reports with existing moderation and admin`.

### FOR-19 — Android discussion and social checkpoint

- Add online comments/replies/editor, sign-in return, owner edit/remove,
  report dialog, pending-review and decision/appeal states; no private Room.
- Run A1+A2 and planned A3: session change cancels, double tap, unknown commit,
  stale edit, offline, 429, IME/back, 200% text and inaccessible actions.
  Acceptance: complete published-post discussion source flow; social switch
  remains off until privacy/operator/device obligations actually pass. Commit
  `feat(forum): add safe Android discussion and owner controls`.

Rollback: writes/reviewed publication off; public editorial reading can remain.
Keep account erasure, report handling and moderation available for existing data;
turning off writes is not permission to stop privacy recovery.

## Phase 4 — user topics and bounded discovery

Depends on FOR-19, premoderation/rights policy and operational coverage.
B-BR-FOR-05–11/13; BUC-FOR-05/06/07/09. Evidence:
`docs/backend/evidence/forum/FOR_PHASE_4.md`.

### FOR-20 — Topic writes and ownership SQL

- Enable TOPIC domain/state/author fields and unique create operation, CAS edits,
  removal, per-account topic quotas. No source forgery or paid privilege.
- Run B1+B2: simultaneous create/edit/delete, expired/bound account, quota race,
  kind spoof, hidden/erased owner and revision/audit rollback. Acceptance: topic
  rules independent of editorial and feedback. Commit
  `feat(forum): add bounded owned topics with reviewed publication`.

### FOR-21 — Topic API and OpenAPI fixtures

- Add signed topic commands; extend current private owner read/review queue,
  keep public projection shared and anonymous. No redundant route aliases.
- Run B1+B2+C: malformed/unknown fields, news-as-user spoof, foreign edit,
  duplicate key/body conflict, expired session and actual process round trip.
  Acceptance: one pending topic per intent, never optimistic publication.
  Commit `feat(forum): expose topic create edit and removal commands`.

### FOR-22 — Topic privacy and report integration

- Extend FOR-16 account export/erase/restore and FOR-18 actions to TOPIC,
  including comments beneath erased/removed topics and no inherited permissions.
- Run B1+B2+J+W: edited/hidden topic erasure, child visibility, operator restore
  refusal after removal, metadata-only audit, restart, restored deletion replay.
  Acceptance: complete privacy/moderation before Android public creation.
  Commit `test(forum): prove topic privacy and moderation recovery`.

### FOR-23 — Android topic forms

- New/edit route, category/optional coarse subject region, pending receipt,
  memory draft, discard/back/IME and safe owner controls. Existing free auth.
- Run A1+A2 and A3 form/navigation cases when due. Acceptance: valid submit,
  stale conflict, cancellation/unknown result and account change preserve intent
  safety; no attachment/precise GPS/payment. Commit
  `feat(forum): add accessible Android topic authoring`.

### FOR-24 — Regional filters, simple FTS and open-forum checkpoint

- Implement subject UF/IBGE/fuel/station filters and bounded Portuguese FTS if
  query plans justify the indexed path. Migrations/sqlc/OpenAPI and Android
  debounced/cancelled search/filter state; recent order only. If FTS is not
  justified, record SEARCH_DEFERRED explicitly, do not render a fake search box.
- Run B1+B2+C+A1+A2, search/stopword/accent/SQL injection cases, cursor-bound
  normalized filters, no-GPS and 10k/100k-post EXPLAIN. Acceptance: scoped query
  budgets hold and empty/offline/error results are honest; no relevant ranking,
  reactions or follows. Commit `feat(forum): add bounded category region and
  text discovery` (or record a truthful partial checkpoint if deferred).

Rollback: disable topic creation/search filter separately; retain published
reading, privacy and moderation. Do not migrate feedback comments into Forum.

## Phase 5 — deterministic Abastevo bulletins

Depends on FOR-09, accepted source dataset aggregate contract, exact arithmetic
and review; independent of optional push/personalization. B-BR-FOR-02/15/16,
BUC-FOR-08. Evidence: `docs/backend/evidence/forum/FOR_PHASE_5.md`.

### FOR-25 — Freeze methodology and aggregate port

- First bulletin: weekly **cross-sectional** official price mean by UF/fuel/unit
  comparing two accepted complete survey revisions. Use one latest accepted
  observation per station/fuel/unit/revision, following existing ranking SQL;
  describe this deduplication and do not call it ANP's official published mean.
  Return sum in milli-BRL,
  observation count, distinct sampled stations, period, revision/checksum/URL
  and coverage from an Official-owned bounded query. Do not equate changing
  samples with same-station price change or national fleet behavior.
- Require >=30 observed stations in each compared UF/fuel sample, same unit,
  positive prior mean, no missing/incomplete revision. This is a conservative
  product threshold, not statistical confidence. Community/charging excluded
  from first statistical template; accepted catalog changes can get their own
  later factual template after inventory acceptance.
- Run B1+B2: sample/coverage/units, exact sum overflow, duplicate observations,
  corrected revisions and absent region. Acceptance: methodology version and
  input manifest reproducible. Commit `feat(forum): define exact official
  bulletin aggregates and methodology`.

### FOR-26 — Templates, provenance and correction identity

- Generate original plain text without LLM/network. Keep mean as rational
  sum/count; absolute difference and percent derive from unrounded rationals.
  Freeze display rounding (half away from zero, three decimals BRL/unit and
  one decimal percentage) in Forum methodology with signed/tie fixtures; never
  alter stored Official prices. Key = template version + exact pair of source
  revision IDs + UF + fuel + unit. Same key no-op; new accepted correction
  links `supersedes_post_id`, never silently rewrites underlying source facts.
- Run B1: golden fixtures explicitly synthetic, signed deltas/ties/zero/missing/
  overflow, source checksum change, deterministic byte output. Acceptance:
  every sentence and amount traceable to actual accepted input. Commit
  `feat(forum): generate deterministic provenance-backed bulletins`.

### FOR-27 — Worker/admin/Android bulletin checkpoint

- Reuse worker job and editorial review. Trigger after accepted imports or
  bounded periodic accepted-revision check; never trigger on raw download.
  Add detail provenance/methodology view using existing screen.
- Run B1+B2+J+C+A1+A2: concurrent/restarted generator, missing source, correction
  visibility/cache invalidation, no-input no-post, API/Kotlin exact display.
  Acceptance: reproducible draft→review→reading and no false coverage; automatic
  publication remains off until a separately approved template policy.
  Commit `feat(forum): integrate reviewed deterministic data bulletins`.

Rollback: disable bulletin generation/auto-publication; preserve accepted source
datasets and correction links. No changes to Official/Directory/Charging trust.

## Final scoped construction closure

### FOR-28 — Capacity, failures and final device union

- Execute the validation matrix's declared load scenarios on disposable local
  fixtures; record hardware/data/head, measured limits and unresolved capacity.
  Exercise disable/source failure/DB outage/cache/privacy recovery, no price/map
  regression. Run the union of affected Android native, theme/font/TalkBack/
  back-stack/IME checks once at the existing end-device point.
- Files: performance harness/tests only as justified, phase evidence, PROGRESS.
  Acceptance: measured operating budget, no bypassed failure/skip; any device,
  operator, legal or external source gap named, writes kept off as needed.
  Commit `test(forum): validate scoped capacity failure and Android acceptance`.

### FOR-29 — Final handoff and authorized protected PR

- Fetch current GitHub main, reconcile normally and rerun only checks invalidated
  by actual changes. Verify docs/task/test/feature-switch truth against final
  code. Commit meaningful corrections/evidence per microtask, then one final
  batch PR via maintained dev to protected main. Include phase range, tested
  source, commands/counts, blockers, switches and rollback. Do not close
  incomplete source/runtime/release trackers as implemented.
- Current required check is `Quick verification`; reverify protection and actual
  head/base. No CI polling during construction. Merge only if separately
  authorized and every applicable current-head guard passes, using
  `bash scripts/git-flow.sh finish --base origin/main --pr NUMBER --required
  "Quick verification"`; preserve commits/dev. Wiki only after merged source
  and separately verified wiki publication scope. No deploy/tag/pilot implied.
- Acceptance: reviewable PR and truthful completion report. “Forum implemented”
  requires all selected mandatory source tasks and tests; source integration,
  source activation, device acceptance and RELEASE_CERTIFIED are separate.
  Record no self-referential commit SHA or bookkeeping-only PR.

## Deferred features and decisions that must remain visible

Reactions (including self-vote/duplicate/ranking rules), follows/subscriptions,
relevance ordering, personalized feed, opt-in alerts and token/delivery privacy,
public deep links, attachments/images and generic shared discussion extraction
are outside FOR-00–29. They require explicit next scope; do not add their tables,
endpoints or empty UI in advance. Basic source legal approval and operator
coverage remain human evidence; no coding assistant can fabricate them.

## What the maintainer should review later

Inspect the implementation task-by-task against B-BR/BUC and phase evidence,
actual commits, migration head, private/public projections, source rights,
test outputs (including failures/skips), source switches and real device state.
Challenge race/replay/erase/restore and negative permissions before cosmetic
polish. Any mismatch between planned and implemented behavior must be an
explicit contract change with tests, not a confident completion sentence.

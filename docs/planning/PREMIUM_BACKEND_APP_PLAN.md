# Premium delivery plan — complete backend, minimal Android UI

State: **FIRST TEN MICROTASKS AUTHORIZED**, 2026-10-13. Current acceptance: **first ten source microtasks LOCAL_DONE / INTEGRATION_PENDING**, [PR #131](https://github.com/AlexandreZanata/Abastevo/pull/131), [execution evidence](../backend/evidence/premium/PRM_FIRST_TEN.md). Original planning provenance is retained below. Requested scope: optional cloud services plus individual professional tools in one Premium offering. The original scope defines 62 microtasks in 11 phases; the 2026-10-13 OneSignal planning amendment adds 17 microtasks in PRM11–PRM14 (79 total, 15 phases). The [product contract](../product/PREMIUM_PRODUCT_CONTRACT.md) owns proposed B-BR-PR01–PR25, BUC-PR01–PR07 and D-PR01–D-PR05. Existing [P11](../../ROADMAP.md#p11--optional-hosted-commercial-services) is the commercial parent; these IDs refine it rather than create competing implementations.

## Authorization, ordering and readiness

### 2026-10-13 construction authorization

The maintainer explicitly requested a new isolated backend branch and a new PR to main, executing the first ten microtasks with one commit per task and publishing those commits. This supersedes the older planning-only/maintained-dev/priority restrictions for this bounded batch: PRM00-T01..T04 and PRM01-T01..T06, on codex/premium-backend from fetched origin/main 5dc9b2b. Do not merge this PR or edit the occupied dev/mobile checkout. Existing source security/release obligations remain tracked; no live readiness is inferred.

The maintainer confirmed that no Play subscription/payment module exists yet: prepare the backend with real payments OFF. Adapter/handler contracts, durable work and local fault tests are in scope; real Play credentials/catalog/payment-module wiring and sandbox proof are explicitly deferred by this instruction. No manual premium-grant endpoint or deployable fake provider is allowed. PRM01-T07, remaining Premium features and Android UI are outside this ten-task request. Acceptance here means tested source, not commercial/provider certification.

### 2026-10-13 next-ten continuation authorization

The maintainer authorized the next ten tasks on the same branch/PR, one commit each: PRM01-T07, PRM02-T01..T05, PRM03-T01..T04. Preserve occupied dev and real payments OFF. Source construction can consume the tested local predecessor; real Play sandbox remains PROOF_OWED. This does not authorize deployment, merge or Android implementation. [Next-ten evidence](../backend/evidence/premium/PRM_NEXT_TEN.md) records actual acceptance.

### Original planning authorization (historical)

At the original planning checkpoint, the request authorized **local planning only**. Do not implement code, open every future issue, charge customers, change providers, deploy, push, create PRs or mirror the wiki as a consequence of this plan. At that checkpoint all tasks were PLANNED / NOT STARTED; local documentation validation was not feature acceptance. The newer construction authorization and per-task statuses below govern the first ten.

Current priority remains active security corrections and ADR-019 Android/VPS P34–P38, then the already selected catalog/profile construction. Premium source construction starts on a later explicit continuation/prioritization request, after required free account/session/ownership/privacy contracts and reusable client paths are accepted. Reconcile current PROGRESS and actual commits at opening; unresolved security/deletion dependencies cannot be bypassed. The maintainer may explicitly change priority, but this plan does not change it itself.

Commercial activation still requires completed P10 pilot, actual applicable G24/G09 release evidence and P11 validation. Source construction can use tested local dependencies under ADR-018; production claims cannot. iOS remains archived. Android manual/device acceptance joins the end source batch unless the user explicitly authorizes an earlier scoped test. User request: finish the backend before implementing the minimal Android consumer; no partially implemented backend hidden behind UI mocks.

Execution order:

`PRM00 contracts → PRM01 billing/capabilities → PRM02 cloud records → PRM03 sync → PRM04 backup/restore → PRM05 private ledger → PRM06 metrics/reports → PRM07 advanced alerts → PRM08 complete backend acceptance → PRM09 minimal Android → PRM10 candidate/commercial handoff`.

- **G-PR-CONTRACT:** PRM00 complete; all implementation-blocking D-PR decisions frozen; release-only owner/provider decisions remain explicitly tracked.
- **G-PR-BACKEND:** PRM01–PRM08 complete with actual API/worker/SQL/private-storage/provider-sandbox evidence for all requested features. Missing provider evidence leaves this gate INCOMPLETE, even if independent source construction progresses. Android PRM09 starts only after this gate passes.
- **G-PR-ANDROID:** PRM09 functional journeys and deferred end-device matrix pass with the pinned backend. A build or polished mockup is insufficient.
- **G11 / G-PR-COMMERCIAL:** PRM10 candidate acceptance, pilot/value/cost decisions, current provider/listing obligations and protected integration pass. This is separate from G-PR-BACKEND and does not overwrite G09 certification.

## Existing source and intended module ownership

Opening baseline: `a3e01aac7f3bf1503b478d9010e7f263d2d4af93`, isolated worktree from dev, 2026-10-13. Primary checkout was occupied; its staged Android privacy changes and unstaged evidence adapter were preserved. Existing account, identity, privacy, jobs, API/worker, private S3, OpenAPI, Kotlin `domain/application/data/app`, Room, WorkManager and Compose are reused. Existing vehicles have no professional refuel/expense ledger; basic weekly price-drop alerts are already free. No Premium subscription/cloudsync/professional runtime is claimed.

Proposed modules, created only by their owning implementation tasks:

- `subscription`: verified provider purchases, account binding, lifecycle and capability read port. No community/trust dependencies.
- `cloudsync`: allowlisted private vehicles/favorites/portable preferences, revision/change feed, devices, snapshots, restore and dataset manifests. Existing following/favorites ownership is audited first; wrap existing application ports rather than duplicate account/follow subscriptions.
- `professional`: private refuel/expense ledger, odometer segments, pure calculation rules and report projections; reference private vehicles through explicit application ports. One account owns every record; no tenant model.
- `alerts`: advanced rule evaluation and delivery/inbox using read ports and existing PostgreSQL jobs. It cannot mutate price facts/consensus or replace basic alerts.

Keep Go/net/http/chi/pgx/sqlc/slog, one database and API/worker roles. No new broker, server framework, fleet service or generic sync platform. Distinguish private domain vehicle IDs from station UUIDs, contributor identities and Android installation IDs. Existing client vehicle UUIDs need a validated owner-bound import mapping, not regeneration on every sync. New ports enforce boundaries; modules never import another module's adapters/generated SQL. Auth/session proof and authorization reuse current contracts, with fresh owner checks on commands and background jobs.

## Task format and immediate verification

Each numbered task is one bounded outcome and one meaningful atomic implementation commit. All are **MUST within the requested Premium scope / PLANNED**, even though the parent release remains LATER. Task entries name dependencies/rules, file areas, tests first, acceptance and recovery. Phase validation profiles below are part of every task, selected for the changed surface; they are not instructions to run every repository suite repeatedly.

- **DOC:** local Markdown path/anchor/task/dependency and free-feature consistency checks, changed/untracked-document secret review and `git diff --check`. No backend/Android runtime tests for this planning-only change.
- **BE:** pure domain RED → GREEN → REFACTOR; targeted application/HTTP unit/race tests, affected `go vet`/staticcheck, compilation, gofmt fail-on-output and contract/golden checks; `sqlc vet`/generation-drift checks when SQL changes and vulnerability checks when dependencies change. At task opening record actual package/test selectors; future module names here are planned areas, not existing passing commands.
- **DB:** BE plus immediate disposable real PostgreSQL/PostGIS production-SQL tests for ownership, negative inputs, replay/idempotency, concurrent writes/fencing, query plans, rollback, empty/upgrade migrations and failure recovery. Append-only migrations. Follow the actual harness/environment in [backend/README](../../backend/README.md); never run destructive fixtures against staging/production or the shared development DB.
- **OBJ:** DB plus actual private S3-compatible PUT/HEAD/GET/DELETE and inventory/receipt/lost-response/crash/purge/restore races; prove provider-specific behavior in the selected private staging bucket before claiming it. Object emulation alone is not provider acceptance.
- **PROVIDER:** authenticated fake-provider fault matrix immediately, then real configured Play sandbox test purchases/lifecycle and selected push-provider acceptance. Missing credentials/merchant configuration is named PROOF_OWED, not PASS; synthetic test entitlements are isolated from deployable release configs. Secrets/tokens stay outside fixtures/logs.
- **ANDROID:** affected `:domain:test`, `:application:test`, `:data:testDebugUnitTest`, `:app:testDebugUnitTest`, `:app:lintDebug`, `:app:assembleDebug`; targeted Room/WorkManager/Compose instrumentation and genuine device evidence at the authorized end batch. Use Kotlin↔Go golden vectors for money/formulas. No iOS commands.

Common recovery: disable only the affected paid write/evaluation route, preserve free functions/local data, reconcile durable idempotent work and use forward migration repair. Every task ends with scoped secret review, `git diff --check`, actual tested revision/commands/outcomes in phase evidence and a concise PROGRESS update. SQL/auth/money/privacy/jobs failures are fixed immediately; no weakened/skipped tests. Phase closure is LOCAL_DONE / INTEGRATION_PENDING, not merge/release. Maintained dev/isolated worktree and protected final dev → main delivery follow ADR-020/ADR-018; do not reintroduce per-phase CI polling, PR waits or wiki sync.

## PRM00 — Scope, decisions and cross-language contracts (4 tasks)

Entry: explicit future construction request and accepted source prerequisites. Exit: G-PR-CONTRACT. Validation: DOC; add BE/contract checks only when actual fixtures/schema are introduced.

### PRM00-T01 — Reconcile baseline and freeze the free/Premium boundary

**Source checkpoint (2026-10-13): LOCAL_DONE / INTEGRATION_PENDING.** See [tested evidence](../backend/evidence/premium/PRM_FIRST_TEN.md); live payment proof remains explicitly deferred.

- **Dependencies/rules:** P11-T01; accepted account/privacy/free-app baseline; PR01–PR05/PR24; BUC-PR01/07.
- **Outcome/areas:** record exact source/runtime inventory, owned modules, existing free vehicles/history/alerts and the one-Premium bundle in product/planning docs. Record source order, deferred external gates and D-PR01 decisions.
- **Tests first:** inspect real source/tests, compare every proposed paywall against the existing free inventory; detect any overlap with P13 account linking or basic following alerts.
- **Acceptance/recovery:** no existing free feature moves behind payment; all requested features have one owner/task. Retain the free baseline if a proposed boundary fails review.

### PRM00-T02 — Freeze lifecycle, consent, retention and capacity policy

**Source checkpoint (2026-10-13): LOCAL_DONE / INTEGRATION_PENDING.** See [tested evidence](../backend/evidence/premium/PRM_FIRST_TEN.md); live payment proof remains explicitly deferred.

- **Dependencies/rules:** T01; PR02–PR12/PR23; D-PR02/03/05.
- **Outcome/areas:** publish capability/state matrix, finite stale/offline lease, opt-ins, exit/export path, snapshot/delete deadlines, cursor/device/rule/object/queue limits, backup cadence and key recovery. Review measured cost/headroom and purpose-based data inventory.
- **Tests first:** scenario walkthrough for paid-through cancellation, revocation during write, provider outage, no consent, logout/account switch, expiry during restore, stale offline device and restored deleted account.
- **Acceptance/recovery:** exact implementation-required limits and policies are frozen before schema. Unknown required inputs block only their dependent task; no indefinite retention/access promise. Recovery favors preserved local data and free exit.

### PRM00-T03 — Freeze ledger mathematics and shared golden examples

**Source checkpoint (2026-10-13): LOCAL_DONE / INTEGRATION_PENDING.** See [tested evidence](../backend/evidence/premium/PRM_FIRST_TEN.md); live payment proof remains explicitly deferred.

- **Dependencies/rules:** T01; PR13–PR20; D-PR04; BUC-PR04/05.
- **Outcome/areas:** synthetic versioned vectors for actual totals, milli-quantity/metre precision, rounding, full/partial/gap sequences, odometer segments, mixed fuels/GNV, period bounds and weighted comparisons in `contracts/testdata/` after ownership review.
- **Tests first:** manually derive worked full→partial→full case excluding the initial fill, zero/overflow/discount discrepancy, missing-fill/odometer, backdated correction, unequal vehicle distances and incomplete month denominators.
- **Acceptance/recovery:** every computable value and UNKNOWN reason has a reviewed expected result; no ratio of incompatible windows or claimed realized savings. Correct vectors before implementing formulas.

### PRM00-T04 — Freeze additive APIs, dataset ownership and verification selectors

**Source checkpoint (2026-10-13): LOCAL_DONE / INTEGRATION_PENDING.** See [tested evidence](../backend/evidence/premium/PRM_FIRST_TEN.md); live payment proof remains explicitly deferred.

- **Dependencies/rules:** T02/T03; PR06–PR12/PR20–PR25.
- **Outcome/areas:** proposed OpenAPI request/response/error/revision/pagination contracts, module read/write ports, schema ownership and exact future verification package/fixture manifest. Define owner/session proof for every route and background action; freeze alert predicate and delivery policy.
- **Tests first:** schema/golden checks for unknown enums/versions, cross-account IDs, partial batch results, malformed cursors, private no-store responses and free export without entitlement.
- **Acceptance/recovery:** backend and Android can implement the same frozen contract; no endpoint is claimed live. Version changes are additive and old clients fail safely.

## PRM01 — Subscription backend and capabilities (7 tasks)

Entry: PRM00. Exit: provider-verified capabilities and lifecycle/recovery tested; no real charging enabled. Areas: proposed subscription module, existing account/HTTP/jobs/config, append-only SQL, contracts. Validation: BE/DB/PROVIDER as affected.

### PRM01-T01 — Pure subscription state and capability policy

**Source checkpoint (2026-10-13): LOCAL_DONE / INTEGRATION_PENDING.** See [tested evidence](../backend/evidence/premium/PRM_FIRST_TEN.md); live payment proof remains explicitly deferred.

- **Dependencies/rules:** PRM00; PR01–PR05.
- **Outcome/areas:** separate purchase lifecycle, paid-through/access windows, capability projection and free exit policy in domain/application ports; injected clock and versioned decisions.
- **Tests first:** every provider state, canceled-but-still-paid, exact expiry boundary, unknown state, stale proof, revoked account and independent free community rights.
- **Acceptance/recovery:** deterministic capabilities cannot grant from pending/unknown proofs or affect trust; disable paid grants while free rights stay intact.

### PRM01-T02 — Owner-bound purchase persistence and replay protection

**Source checkpoint (2026-10-13): LOCAL_DONE / INTEGRATION_PENDING.** See [tested evidence](../backend/evidence/premium/PRM_FIRST_TEN.md); live payment proof remains explicitly deferred.

- **Dependencies/rules:** T01; PR02/03/08.
- **Outcome/areas:** unique provider/environment/token binding, minimal protected provider references, current lifecycle revision and atomic grant/job intent in SQL/sqlc.
- **Tests first:** duplicate token on same/different accounts, simultaneous claims, changed payload replay, transaction rollback, deletion during claim and empty/upgrade migration.
- **Acceptance/recovery:** exactly one legitimate owner per purchase chain, no token exposure in responses/logs; durable replay repairs incomplete verification without transferring ownership.

### PRM01-T03 — Secure Play verification adapter and purchase admission

**Source checkpoint (2026-10-13): LOCAL_DONE / INTEGRATION_PENDING.** See [tested evidence](../backend/evidence/premium/PRM_FIRST_TEN.md); live payment proof remains explicitly deferred.

- **Dependencies/rules:** T02; frozen current provider/package/product/account binding; PR02/03.
- **Outcome/areas:** server-side Developer API verification, supported plan mapping, restricted credentials, timeout/rate budgets, signed owner admission and acknowledgement intent. Record dependency/license/security review before any SDK addition.
- **Tests first:** forged/unpurchased/wrong-product/package/environment/account proof, pending purchase, timeout/5xx/redirect, response redaction and late verification after logout/deletion.
- **Acceptance/recovery:** only verified purchases commit rights; network calls hold no DB locks. Reconciliation recovers outage/lost responses with no manual premium Boolean.

### PRM01-T04 — Acknowledgement and authenticated notification inbox

**Source checkpoint (2026-10-13): LOCAL_DONE / INTEGRATION_PENDING.** See [tested evidence](../backend/evidence/premium/PRM_FIRST_TEN.md); live payment proof remains explicitly deferred.

- **Dependencies/rules:** T03; PR03/08; existing fenced jobs.
- **Outcome/areas:** durable acknowledgement retries and authenticated bounded provider notification intake; deduplicated inbox requests canonical provider refresh rather than trusting event payload state.
- **Tests first:** spoofed issuer/audience/signature, repeated/out-of-order notification, lost acknowledgement reply, crash after grant and during receipt, retry exhaustion and provider-required acknowledgement deadline.
- **Acceptance/recovery:** restart cannot lose acknowledgement intent or double-grant; dead work alerts/operators can replay bounded jobs. Sandbox verifies actual acknowledgement, not a mock success. If purchase initiation requires a native client, use a narrowly scoped debug/instrumentation-only billing probe for the verified existing package before this backend gate; it is test infrastructure, not the PRM09 product UI. Never add a deployable entitlement bypass.

### PRM01-T05 — Renewals, replacement tokens, cancellation and revocation

**Source checkpoint (2026-10-13): LOCAL_DONE / INTEGRATION_PENDING.** See [tested evidence](../backend/evidence/premium/PRM_FIRST_TEN.md); live payment proof remains explicitly deferred.

- **Dependencies/rules:** T04; PR03/04.
- **Outcome/areas:** reconcile renewals, linked replacement purchases, refund/revocation, hold/grace/pause/expiry and fresh capability checks at paid command/job boundaries.
- **Tests first:** old token after replacement, out-of-order events, renew versus revoke race, expiry during transaction/dispatch, account deletion and restored stale subscription rows.
- **Acceptance/recovery:** older events cannot reinstate revoked rights; cancellation respects paid-through mapping; existing local data/free exits survive. Re-query authoritative provider state to repair disagreement.

### PRM01-T06 — Account status, restore-purchase and free exit API

**Source checkpoint (2026-10-13): LOCAL_DONE / INTEGRATION_PENDING.** See [tested evidence](../backend/evidence/premium/PRM_FIRST_TEN.md); live payment proof remains explicitly deferred.

- **Dependencies/rules:** T05; free account proof; PR02/04/24.
- **Outcome/areas:** owner-only capability/status API, validated purchase reattachment for the same proven account, management destination and readable outage/expiry reasons; no purchase details leak.
- **Tests first:** IDOR, restored token with another app account, account merge/provider-link ambiguity, stale sessions, no-store/ETag behavior, free export/deletion while expired.
- **Acceptance/recovery:** purchase restore never restores a session/key or claims someone else's data; ambiguous ownership uses the established recovery path. Status errors remain explicit.

### PRM01-T07 — Bounded reconciliation, sandbox evidence and billing runbook

**Source checkpoint: LOCAL_DONE / INTEGRATION_PENDING; real sandbox PROOF_OWED.** [Evidence](../backend/evidence/premium/PRM_NEXT_TEN.md).

- **Dependencies/rules:** T06; PR02–PR05.
- **Outcome/areas:** recurring reconciliation with quotas/leases/checkpoints, minimal metrics and private operator commands; real Play sandbox matrix and credential/product setup runbook.
- **Tests first:** missed notifications, outage/backlog, restarted workers, revoked credentials, parallel reconciliation and restored stale entitlement.
- **Acceptance/recovery:** verified purchase→ack→renew/cancel/revoke/restore flows have actual evidence; unavailable provider proof remains owed. Document disable/replay controls; do not enable real purchases.

## PRM02 — Private cloud records and devices (5 tasks)

Entry: PRM00/PRM01. Exit: owned allowlisted data and device authorization persistence. Validation: BE/DB. Areas: cloudsync plus existing account/privacy and explicit favorites/following ports.

### PRM02-T01 — Owned private vehicle model and legacy ID mapping

**Source checkpoint: LOCAL_DONE / INTEGRATION_PENDING.** [Evidence](../backend/evidence/premium/PRM_NEXT_TEN.md).

- **Dependencies/rules:** PRM01 capability port; PR01/06/07/13.
- **Outcome/areas:** private vehicle schema/ports compatible with current local UUIDs and three-vehicle allowance; explicit account-bound mapping on initial opt-in.
- **Tests first:** cross-owner ID collision, repeat import, fourth-slot boundary, archived vehicle, fuel/unit compatibility and upgrade with existing users.
- **Acceptance/recovery:** vehicles import once without changing local identity or expanding paid slots; failed mapping preserves the original local vehicle.

### PRM02-T02 — Favorites and portable preference allowlist

**Source checkpoint: LOCAL_DONE / INTEGRATION_PENDING.** [Evidence](../backend/evidence/premium/PRM_NEXT_TEN.md).

- **Dependencies/rules:** T01; PR06/07/21; audited existing follows.
- **Outcome/areas:** canonical favorite station references and versioned portable fields using existing follow ports where present; keep OS permissions, auth state and device-only settings out.
- **Tests first:** unknown/retired station, wrong-owner favorite, unknown preference/version, existing free follow subscription, excluded key/permission/token injection.
- **Acceptance/recovery:** stable typed datasets preserve free favorite/follow semantics and source truth; invalid fields fail with recoverable errors, never generic map replication.

### PRM02-T03 — Device registration, revocation and origin isolation

**Source checkpoint: LOCAL_DONE / INTEGRATION_PENDING.** [Evidence](../backend/evidence/premium/PRM_NEXT_TEN.md).

- **Dependencies/rules:** T01/T02; PR07/23/25.
- **Outcome/areas:** random account-bound device records, capability-limited device sessions/reference binding, last-success status and immediate revoke API; no advertising ID or device fingerprint.
- **Tests first:** account/origin switch, token reuse, deleted session, device cap race, revoke versus queued write, foreign device ID and restored revoked device.
- **Acceptance/recovery:** revoked devices cannot upload/download/receive future private dispatch; re-registration requires fresh account proof. Data is not deleted merely by logout.

### PRM02-T04 — Versioned commands, limits and deletion epochs

**Source checkpoint: LOCAL_DONE / INTEGRATION_PENDING.** [Evidence](../backend/evidence/premium/PRM_NEXT_TEN.md).

- **Dependencies/rules:** T03; PR08/09/12.
- **Outcome/areas:** common cloud command receipts, expected revisions, account deletion epoch, bounded batch transactions and per-owner limits using existing admission/idempotency patterns.
- **Tests first:** exact/divergent retries, 12-way updates, byte/page/quota bounds, malformed IDs, stale epoch after account deletion, cancellation and transaction rollback.
- **Acceptance/recovery:** one durable result per operation and no cross-owner write; receipts/tombstones have explicit bounded retention and safe recovery.

### PRM02-T05 — Owner read APIs and privacy integration

**Source checkpoint: LOCAL_DONE / INTEGRATION_PENDING.** [Evidence](../backend/evidence/premium/PRM_NEXT_TEN.md).

- **Dependencies/rules:** T04; PR06/12.
- **Outcome/areas:** paginated private datasets/status, registered privacy export/deletion inventory and no-store/error redaction.
- **Tests first:** IDOR across every dataset, unauthenticated/revoked reads, malformed cursor, export with expired subscription, deleted-device cache and unknown inventory errors.
- **Acceptance/recovery:** denial does not leak object existence/financial details; privacy failures remain retryable/unknown rather than reporting a false deletion.

## PRM03 — Cross-device synchronization (5 tasks)

Entry: PRM02. Exit: two-device convergence/conflicts/deletion recovery, without bulk DB replication. Validation: BE/DB; HTTP/golden contracts.

### PRM03-T01 — Incremental change feed and safe cursor lifecycle

**Source checkpoint: LOCAL_DONE / INTEGRATION_PENDING.** [Evidence](../backend/evidence/premium/PRM_NEXT_TEN.md).

- **Dependencies/rules:** PRM02; PR08/09.
- **Outcome/areas:** monotonic private dataset revisions, indexed paginated delta/tombstone feed and owner/device/epoch/version-bound cursor.
- **Tests first:** interleaved writes/page boundaries, same-timestamp rows, foreign/forged/old cursor, bounded history scans and cursor expiry after compaction.
- **Acceptance/recovery:** no missed/duplicate-applied change and safe required resnapshot on stale cursor; never silently skip unavailable history.

### PRM03-T02 — Atomic write batches and recoverable conflicts

**Source checkpoint: LOCAL_DONE / INTEGRATION_PENDING.** [Evidence](../backend/evidence/premium/PRM_NEXT_TEN.md).

- **Dependencies/rules:** T01; PR08/09.
- **Outcome/areas:** conditionally apply stable commands, return per-operation results and base/current/proposed conflict data; field/set merge only where frozen.
- **Tests first:** two devices edit same/different fields, delete versus update, partial batch refusal, lost response/replay, entitlement expiry and SQL rollback.
- **Acceptance/recovery:** no silent last-arrival overwrite or duplicate record; explicit conflict resolution uses a fresh revision and preserves all local proposed edits.

### PRM03-T03 — Initial import, resnapshot and sync consent control

**Source checkpoint: LOCAL_DONE / INTEGRATION_PENDING.** [Evidence](../backend/evidence/premium/PRM_NEXT_TEN.md).

- **Dependencies/rules:** T02; PR06/07/09.
- **Outcome/areas:** preview-based merge/import for vehicles/favorites/preferences, dataset opt-in/revoke, safe resnapshot token and resumable bounded progress.
- **Tests first:** guest data with signed-in cloud data, conflicting UUID/vehicle counts, interrupted first import, stale server snapshot, account switch and revoked consent.
- **Acceptance/recovery:** no automatic guest upload/destructive replacement; staged import can be canceled and resumed with new proof/version checks.

### PRM03-T04 — Tombstone compaction and anti-resurrection recovery

**Source checkpoint: LOCAL_DONE / INTEGRATION_PENDING.** [Evidence](../backend/evidence/premium/PRM_NEXT_TEN.md).

- **Dependencies/rules:** T03; PR09/12.
- **Outcome/areas:** retention-bounded change/tombstone cleanup with deletion-epoch ledger and expired-client resnapshot protocol; register restore replay obligation.
- **Tests first:** offline edit after tombstone compaction, old snapshot restoration, account recreation, cleanup crash/parallel workers and vehicle dependencies.
- **Acceptance/recovery:** deleted resources cannot be republished by old devices/restores; conservative retention or resnapshot refusal is preferred to data resurrection.

### PRM03-T05 — Actual two-client protocol and failure acceptance

**Source checkpoint: LOCAL_DONE / INTEGRATION_PENDING.** [Evidence](../backend/evidence/premium/PRM_NEXT_TEN.md).

- **Dependencies/rules:** T04; PR05/08/09/25.
- **Outcome/areas:** real HTTP clients against disposable API/PostGIS, exercising reconnect, conflicts, revoke, lapse and resnapshot; evidence/protocol runbook.
- **Tests first:** A offline/B edit or delete, duplicate retry/lost response, account/origin crossover, worker/DB restart and large bounded delta stream.
- **Acceptance/recovery:** all private datasets converge or present explicit conflicts with intact pending edits. Synthetic server clients prove backend protocol only, not Android device acceptance.

## PRM04 — Automatic backup and assisted restoration (6 tasks)

Entry: PRM03. Exit: verified snapshots, preview/commit/recovery and expiry/erasure. Validation: BE/DB/OBJ.

### PRM04-T01 — Snapshot manifest and consistent dataset read

**Source checkpoint: LOCAL_DONE / INTEGRATION_PENDING.** [Evidence](../backend/evidence/premium/PRM_NEXT_TEN.md).

- **Dependencies/rules:** PRM03; PR06/10.
- **Outcome/areas:** versioned manifest and consistent acknowledged data snapshot with revision/count/checksum/exclusion list; dataset export ports allow later professional/rule registration.
- **Tests first:** writes during snapshot, pending client changes, inconsistent dataset revisions, oversize/unknown schema and forbidden credential/photo/cache fields.
- **Acceptance/recovery:** snapshot identifies exactly the recoverable cloud revision; failure never reports local pending data protected. No database dump.

### PRM04-T02 — Private encrypted snapshot storage and inventory

**Source checkpoint: LOCAL_DONE / INTEGRATION_PENDING.** [Evidence](../backend/evidence/premium/PRM_NEXT_TEN.md).

- **Dependencies/rules:** T01; PR10/12; frozen key recovery.
- **Outcome/areas:** restricted bucket/prefix, approved encryption/key management, immutable object keys and metadata-first fenced receipts; integrity readback before READY.
- **Tests first:** missing key, corrupt bytes, lost PUT/HEAD response, crash between metadata/object/receipt, duplicate jobs, late writer and cross-owner object access.
- **Acceptance/recovery:** only verified durable snapshots are selectable; orphan inventory remains tracked and is repaired/purged. Operator can recover keys under the documented policy.

### PRM04-T03 — Automatic schedule, quotas and retention jobs

**Source checkpoint: LOCAL_DONE / INTEGRATION_PENDING.** [Evidence](../backend/evidence/premium/PRM_NEXT_TEN.md).

- **Dependencies/rules:** T02; PR04/10/12; existing jobs/scheduler.
- **Outcome/areas:** explicit opt-in periodic snapshot jobs, per-account cadence/storage count/byte budgets, incremental checkpoints and retention deletion.
- **Tests first:** concurrent scheduler restart, offline unsynced data, no consent/lapsed entitlement, storage failure/backlog, quota races and cancellation during upload.
- **Acceptance/recovery:** schedule produces bounded verified revisions without duplicate cost; status exposes last successful snapshot and unprotected changes. Disable new snapshots while cleanup continues.

### PRM04-T04 — Snapshot list, compatibility and restore preview

**Source checkpoint: LOCAL_DONE / INTEGRATION_PENDING.** [Evidence](../backend/evidence/premium/PRM_NEXT_TEN.md).

- **Dependencies/rules:** T03; PR11/12.
- **Outcome/areas:** owner-only snapshot listing and staged validation/preview of datasets, account epoch, schema, current conflicts and excluded records; enforce bounded decoding/decompression.
- **Tests first:** foreign/expired/corrupt/oversize snapshot, old schema, deleted records, malicious archive, duplicate IDs and preview stale after intervening edits.
- **Acceptance/recovery:** incompatible snapshots cannot reach commit; preview has expiry and revision binding. Cancellation discards staged work with tracked cleanup.

### PRM04-T05 — Confirmed restore commit and interruption recovery

**Source checkpoint: LOCAL_DONE / INTEGRATION_PENDING.** [Evidence](../backend/evidence/premium/PRM_NEXT_TEN.md).

- **Dependencies/rules:** T04; PR08/09/11.
- **Outcome/areas:** idempotent restore command, live owner/epoch/entitlement checks, explicit merge policy and recoverable pre-restore revision; no security material import.
- **Tests first:** double confirmation, crash before/after transaction commit, concurrent edit/delete, revocation/expiry during restore, stale preview and failed rollback preparation.
- **Acceptance/recovery:** result is atomically committed or resumably pending/failed, never falsely restored; conflict resolution and pre-restore recovery do not resurrect erased data.

### PRM04-T06 — Snapshot erasure, expiry and infrastructure recovery proof

**Source checkpoint: LOCAL_DONE / INTEGRATION_PENDING.** [Evidence](../backend/evidence/premium/PRM_NEXT_TEN.md).


- **Dependencies/rules:** T05; PR04/12; existing privacy/backup runbooks.
- **Outcome/areas:** owner/record erasure fences, object purge receipts, retention notices/exit download and deletion-ledger replay after database/object backup recovery.
- **Tests first:** offline stale device plus old snapshot, late PUT during erasure, provider outage, restored bytes beside purge receipts, retry exhaustion and lost delete reply.
- **Acceptance/recovery:** erasure blocks reuse immediately and physical deletion meets frozen measured deadlines; unconfirmed bytes stay inventoried. Private staging-provider proof remains required.

## PRM05 — Individual refueling and expense backend (6 tasks)

Entry: PRM00 mathematics and PRM02–PRM04 infrastructure. Exit: private owned ledger, corrections and sync/backup/privacy integration. Validation: BE/DB; OBJ for changed snapshot deletion surfaces.

### PRM05-T01 — Pure refuel/expense records and validation

**Source checkpoint: LOCAL_DONE / INTEGRATION_PENDING.** [Evidence](../backend/evidence/premium/PRM_NEXT_TEN.md).

- **Dependencies/rules:** PRM00-T03; PR13–PR15.
- **Outcome/areas:** professional domain values, actual money/quantity/odometer/timezone, full/partial/unknown markers, expense category and bounded notes; no station/GPS required.
- **Tests first:** precision/rounding/overflow, currency/unit errors, optional odometer, discount discrepancy, invalid time/category and accepted incomplete inputs.
- **Acceptance/recovery:** exact private records preserve user input and explicit unknowns; invalid inputs cannot be silently repaired into false financial facts.

### PRM05-T02 — Owned ledger schema, indexes and conditional commands

**Source checkpoint: LOCAL_DONE / INTEGRATION_PENDING.** [Evidence](../backend/evidence/premium/PRM_NEXT_TEN.md).

- **Dependencies/rules:** T01; PRM02 vehicle port; PR08/13–PR15.
- **Outcome/areas:** append-only migrations/sqlc for records, categories, revision receipts, period indexes and vehicle ownership constraints; create/update/void/delete intents.
- **Tests first:** foreign/archived vehicle, duplicate/divergent command, simultaneous edit, backdated insertion, rollback, max values, empty/upgrade migration and realistic query plans.
- **Acceptance/recovery:** one private fact per command with atomic revision/invalidation intent; no financial record is written to public community tables.

### PRM05-T03 — Odometer segments, gaps and vehicle lifecycle

**Source checkpoint: LOCAL_DONE / INTEGRATION_PENDING.** [Evidence](../backend/evidence/premium/PRM_NEXT_TEN.md).

- **Dependencies/rules:** T02; PR09/15/16.
- **Outcome/areas:** explicit odometer reset/replacement segments, missing-fill declarations and archive/cascade policy for vehicle removal.
- **Tests first:** regression/identical odometers, reset between fills, edit crossing segments, missing fill later corrected, vehicle deletion with history and concurrent edits.
- **Acceptance/recovery:** invalid intervals are visibly unavailable; retained histories survive vehicle archive. Cascades require preview and consistent erasure across sync/backups.

### PRM05-T04 — Private CRUD/list/filter API and authorization

**Source checkpoint: LOCAL_DONE / INTEGRATION_PENDING.** [Evidence](../backend/evidence/premium/PRM_NEXT_TEN.md).

- **Dependencies/rules:** T03; PR02/04/13/20.
- **Outcome/areas:** exact owner-only create/read/correct/delete and paginated vehicle/time/category queries with actual lifecycle and no-store controls.
- **Tests first:** unsigned/expired/cross-account requests, free read/export/delete after lapse, new write refused on expiry, bounded notes/filter/page/bytes and safe errors.
- **Acceptance/recovery:** retained records remain accessible through the defined free exit; private prices never create public observations. Failed commands leave retryable local work intact.

### PRM05-T05 — Ledger sync, snapshots and privacy registration

**Source checkpoint: LOCAL_DONE / INTEGRATION_PENDING.** [Evidence](../backend/evidence/premium/PRM_NEXT_TEN.md).

- **Dependencies/rules:** T04; PRM03/04; PR06–PR12/15.
- **Outcome/areas:** professional dataset opt-in, revision delta/export/snapshot/restore adapters and erasure inventory; explicit separate consent for private financial uploads.
- **Tests first:** two-device edit/delete conflict, interrupted snapshot, stale restore after ledger deletion, cascade/expiry, foreign account/origin and replay of offline local correction.
- **Acceptance/recovery:** ledger and vehicle references restore consistently, with no photo/credential inclusion; disable professional uploads without affecting existing cloud/free data.

### PRM05-T06 — Real ledger HTTP/SQL recovery and adversarial acceptance

**Source checkpoint: LOCAL_DONE / INTEGRATION_PENDING.** [Evidence](../backend/evidence/premium/PRM_NEXT_TEN.md).

- **Dependencies/rules:** T05; PR13–PR15; BUC-PR04/07.
- **Outcome/areas:** actual API persistence/edit/delete/offline-command replay scenarios and operator-safe recovery evidence.
- **Tests first:** concurrent create/correct/delete, process/DB outage, account erasure, subscription revoke during commit and large bounded month/vehicle scans.
- **Acceptance/recovery:** no duplicate charges/entries or lost accepted correction; database integrity and privacy recovery pass under race detector with production SQL.

## PRM06 — Consumption, monthly reports and comparisons (5 tasks)

Entry: PRM05. Exit: exact explainable metrics and private bounded reports/exports. Validation: BE/DB; Go/Kotlin golden vectors established here.

### PRM06-T01 — Full-to-full consumption and coverage engine

**Source checkpoint: LOCAL_DONE / INTEGRATION_PENDING.** [Evidence](../backend/evidence/premium/PRM_NEXT_TEN.md).

- **Dependencies/rules:** PRM00-T03/PRM05; PR14–PR16.
- **Outcome/areas:** pure calculation version, eligible anchor intervals, partial-fill accumulation, mixed-fuel/segment/gap exclusion and explanation codes.
- **Tests first:** all reviewed vectors, initial-fill exclusion, multiple partials, edit/delete invalidation, unsupported gas/mixed method and incomplete intervals.
- **Acceptance/recovery:** km/L or km/m³ only where the frozen methodology is valid; no sum across units and no consumption guessed from purchase volume. Recompute from raw records after repair.

### PRM06-T02 — Cost/km and matching-window aggregates

**Source checkpoint: LOCAL_DONE / INTEGRATION_PENDING.** [Evidence](../backend/evidence/premium/PRM_NEXT_TEN.md).

- **Dependencies/rules:** T01; PR17/18.
- **Outcome/areas:** purchase-based interval fuel cost, all-expense matched-window cost, weighted totals and explicit incomplete-month results.
- **Tests first:** zero denominator, unmatched month cost/distance, double-counted fuel category, discounts, unequal distances and odometer boundary corrections.
- **Acceptance/recovery:** every ratio exposes its numerator/denominator/window/method; absent coverage is UNKNOWN. Calculation-version rollback regenerates projections without changing raw totals.

### PRM06-T03 — Monthly report projections and correction invalidation

**Source checkpoint: LOCAL_DONE / INTEGRATION_PENDING.** [Evidence](../backend/evidence/premium/PRM_NEXT_TEN.md).

- **Dependencies/rules:** T02; PR15/18.
- **Outcome/areas:** timezone-aware month/vehicle/category totals, revisioned report summaries, indexed reads and bounded rebuild jobs; register privacy/retention.
- **Tests first:** month/year/leap boundaries, DST, UTC event crossing local month, backdated edit/delete during rebuild, stale worker fencing, empty report and query plans.
- **Acceptance/recovery:** readers see a coherent revision or explicit pending/stale state; worker restart cannot publish a superseded report. Exact totals include only frozen categories/windows.

### PRM06-T04 — Vehicle/period comparison API

**Source checkpoint: LOCAL_DONE / INTEGRATION_PENDING.** [Evidence](../backend/evidence/premium/PRM_NEXT_TEN.md).

- **Dependencies/rules:** T03; PR19; BUC-PR05.
- **Outcome/areas:** selected vehicle/period comparisons with same metric/unit/version, eligible coverage, weighted ratios and optional valid percentage change.
- **Tests first:** missing/zero baseline, unequal coverage, different fuel units, mismatched timezone/method, foreign vehicle IDs and invalid/oversize period ranges.
- **Acceptance/recovery:** incompatible/incomplete comparisons explain why; no fabricated savings/profit or automatic winner. UI can render concise factual summaries without charts.

### PRM06-T05 — Report API, safe CSV and held-data export

**Source checkpoint: LOCAL_DONE / INTEGRATION_PENDING.** [Evidence](../backend/evidence/premium/PRM_NEXT_TEN.md).

- **Dependencies/rules:** T04; PR04/12/20.
- **Outcome/areas:** owner-only paginated reports and bounded CSV download/stream; free raw personal export separated from paid new analysis generation. Include locale/timezone/schema/method/coverage metadata.
- **Tests first:** CSV formulas/newlines/quotes, oversized requests, interrupted downloads, expired subscription, revoked account/download access and exclusion of private unrelated datasets.
- **Acceptance/recovery:** actual HTTP totals match golden/SQL ledger results and CSV is safe to open; stored artifacts have private expiry/erase receipts or are avoided by bounded streaming.

## PRM07 — Advanced combined alerts and delivery (6 tasks)

Entry: PRM00 rule policy, PRM01/02/03 and current price read ports. Exit: configurable rules, bounded scheduling, truthful inbox/push and revocation. Validation: BE/DB/PROVIDER. Basic alerts remain independent.

### PRM07-T01 — Pure rule model and schedule semantics

**Source checkpoint: LOCAL_DONE / INTEGRATION_PENDING.** [Evidence](../backend/evidence/premium/PRM_NEXT_TEN.md).

- **Dependencies/rules:** PRM00; PR21/22.
- **Outcome/areas:** bounded predicates/AND/OR, target station/city/fuel/unit/condition, source-aware threshold/change baseline, days/window/timezone/quiet hours/cooldown.
- **Tests first:** invalid/nested predicates, incompatible sources/conditions, midnight windows, DST folds/gaps, missing comparison baseline and timezone change.
- **Acceptance/recovery:** deterministic rule interpretation and reason codes; no arbitrary executable expression or guaranteed exact-clock price promise.

### PRM07-T02 — Owned rule/device inbox schema and APIs

**Source checkpoint: LOCAL_DONE / INTEGRATION_PENDING.** [Evidence](../backend/evidence/premium/PRM_NEXT_TEN.md).

- **Dependencies/rules:** T01; PRM02/03; PR07/08/21/23.
- **Outcome/areas:** private rule CRUD, revisions, enable/disable, rate limits, inbox/candidate records, sync/snapshot/privacy registration and device preferences.
- **Tests first:** wrong owner/station, expired subscription, concurrent enable/delete, duplicate operation, rule/device cap races and existing free following/basic-alert regression.
- **Acceptance/recovery:** rules restore/sync without restoring OS permission or revoked device authority; switch off rules while preserving basic free notifications.

### PRM07-T03 — Indexed source evaluation and transactional candidates

**Source checkpoint: LOCAL_DONE / INTEGRATION_PENDING.** [Evidence](../backend/evidence/premium/PRM_NEXT_TEN.md).

- **Dependencies/rules:** T02; existing directory/official/community application read ports; PR21–PR23.
- **Outcome/areas:** bounded fanout/evaluation cursors, fresh source snapshot, deterministic match and atomic candidate/inbox/job insertion using current jobs.
- **Tests first:** expired/disputed/unknown price, conditional versus standard, no supported coverage, large target fanout/query plans, parallel evaluators and transaction rollback.
- **Acceptance/recovery:** no paid path changes public prices; one logical candidate per frozen dedupe key. Resume evaluation from durable checkpoints without skipping source events.

### PRM07-T04 — Window dispatch, dedupe and late-candidate expiry

**Source checkpoint: LOCAL_DONE / INTEGRATION_PENDING.** [Evidence](../backend/evidence/premium/PRM_NEXT_TEN.md).

- **Dependencies/rules:** T03; PR04/22/23.
- **Outcome/areas:** due-window jobs, quiet-hour/cooldown handling, source/rule/account/entitlement recheck, TTL and distinct evaluated/queued/dispatched states.
- **Tests first:** scheduler restart/duplicate minute, expired price while queued, changed rule, unfollow/logout/lapse/delete race, stale lease and delayed backlog after quiet hours.
- **Acceptance/recovery:** invalid/stale offers never dispatch; retries cannot duplicate inbox meaning. Mark dropped/unknown states honestly and keep deletion jobs independent of paid access.

### PRM07-T05 — Push adapter and account-scoped delivery registration

**Source checkpoint: LOCAL_DONE / INTEGRATION_PENDING.** [Evidence](../backend/evidence/premium/PRM_NEXT_TEN.md).

- **Dependencies/rules:** T04; frozen provider/D-PR03/05; PR07/23.
- **Outcome/areas:** selected provider adapter behind a port, restricted credentials, opaque token enrollment/revoke, account-safe generic payload and documented per-device delivery policy; retain inbox fallback.
- **Tests first:** invalid token, provider 429/5xx/timeout/lost response, cross-account token reuse, switch/revoke before send, stale deep link and private data/log leakage.
- **Acceptance/recovery:** provider acceptance is distinct from device receipt/view; actual private provider test is recorded. If unavailable, report proof owed rather than claim reliable push from an emulator.

### PRM07-T06 — Real scheduler/provider and free-alert regression evidence

**Source checkpoint: LOCAL_DONE / INTEGRATION_PENDING.** [Evidence](../backend/evidence/premium/PRM_NEXT_TEN.md).

- **Dependencies/rules:** T05; PR01/21–PR23; BUC-PR06.
- **Outcome/areas:** actual worker/API/SQL rule→candidate→window→inbox/provider pipeline plus duplicate/drop/revocation metrics and recovery runbook.
- **Tests first:** combined-rule windows, outages/restart, credential rotation, timezone edits, mass bounded fanout and expiry while worker is paused; existing basic weekly alert behavior.
- **Acceptance/recovery:** advanced dispatch survives faults within frozen budgets with no stale assertion; free basic alerts run independently. Alert disable controls do not stop privacy cleanup.

## PRM08 — Complete backend acceptance (5 tasks)

Entry: PRM01–PRM07 locally complete; no Android feature UI yet (a test-only purchase probe may supply real provider events). Exit: **G-PR-BACKEND**. Validation: union of affected BE/DB/OBJ/PROVIDER checks once against pinned source/config, plus real recovery/load evidence.

### PRM08-T01 — Full private-data export, erasure and restore campaign

**Source checkpoint: LOCAL_DONE / INTEGRATION_PENDING.** [Evidence](../backend/evidence/premium/PRM_NEXT_TEN.md).

- **Dependencies/rules:** PRM01–PRM07; PR04/07/09/12/20/23.
- **Outcome/areas:** account-wide inventory covering purchase metadata, devices, cloud records, ledgers, reports, rules, inbox and snapshots; integrate existing privacy commands/recovery.
- **Tests first:** delete during purchase/sync/snapshot/report/dispatch; restore old DB/storage, reconnect stale device, provider outage and parallel erasure retries; bounded physical deletion deadlines.
- **Acceptance/recovery:** no accepted privacy request falsely completes and no restored bytes/record/job reactivates erased account data. Necessary retained metadata is minimized and justified.

### PRM08-T02 — Resource, quota, query-plan and failure budgets

**Source checkpoint: LOCAL_DONE / INTEGRATION_PENDING.** [Evidence](../backend/evidence/premium/PRM_NEXT_TEN.md).

- **Dependencies/rules:** T01; D-PR03; existing infrastructure limits.
- **Outcome/areas:** freeze actual workload/hardware/data profile, p95/error/CPU/RAM/DB/storage/cost/job budgets; measure ledger/report/sync/snapshot/alert workloads together without starving free traffic/safety loops.
- **Tests first:** abusive pages/periods/batches, many devices/rules, large histories, queue flood, full storage, DB/S3/provider outage and recovery.
- **Acceptance/recovery:** real measured capacity and isolation meet frozen bounds; tune within scope and revalidate failed budgets. Source ceilings alone are not deployment capacity proof.

### PRM08-T03 — API/worker integrated Premium backend journeys

**Source checkpoint: LOCAL_DONE / INTEGRATION_PENDING.** [Evidence](../backend/evidence/premium/PRM_NEXT_TEN.md).

- **Dependencies/rules:** T02; all BUC-PR01–PR07.
- **Outcome/areas:** actual signed/authenticated HTTP clients execute verified capability→two-device data→backup/restore→ledger→monthly/comparison→rule/inbox→lapse→free export/delete.
- **Tests first:** reverse-order retries, response loss, crash/restart, stale revisions, IDOR/account switch, lifecycle changes and negative cases from every owning phase.
- **Acceptance/recovery:** exact API/worker/database/object/provider revisions and results prove every requested backend outcome. No Android mock or compile-only result substitutes for this gate.

### PRM08-T04 — Operations, security and provider acceptance handoff

**Source checkpoint: LOCAL_DONE / INTEGRATION_PENDING.** [Evidence](../backend/evidence/premium/PRM_NEXT_TEN.md).

- **Dependencies/rules:** T03; current security/remediation evidence.
- **Outcome/areas:** route/import boundary review, dependency/image checks, credential rotation, minimal metrics, private operator replay, backup/key recovery and scoped deploy/rollback runbooks. Complete real Play sandbox and selected object/push provider proofs.
- **Tests first:** stale/forged entitlement, cross-owner imports, exposed metrics/logs/CSV, provider notification forgery, revoked keys and fail-closed startup/missing config.
- **Acceptance/recovery:** deployable configuration has no synthetic premium-grant switch; provider evidence is real or gate remains INCOMPLETE. No actual deployment is authorized by writing the runbook.

### PRM08-T05 — Pin complete backend checkpoint and consumer contract

**Source checkpoint: LOCAL_DONE / INTEGRATION_PENDING.** [Evidence](../backend/evidence/premium/PRM_NEXT_TEN.md).

- **Dependencies/rules:** T01–T04; G-PR-BACKEND manifest.
- **Outcome/areas:** complete requirement/task/result matrix, exact tested revision/config/schema/contracts, known nonblocking release-only obligations and minimal Android integration guide.
- **Tests first:** audit missing/skipped/zero-test selectors, stale evidence after source changes, unimplemented endpoint/job/provider rows and all free-preservation regressions.
- **Acceptance/recovery:** only full backend gate PASS permits PRM09. Persist LOCAL_DONE / INTEGRATION_PENDING separately from production certification; continue independent backend repairs if external proof is missing.

## PRM09 — Minimal Android implementation in the existing design system (9 tasks)

Entry: G-PR-BACKEND PASS and existing Android account/environment integration. Exit: G-PR-ANDROID source functionality plus actual end-batch device rows. Validation: ANDROID and affected backend contracts; UI polish later.

### PRM09-T01 — Typed ports, exact values and owner-scoped Room state

**Source checkpoint (2026-10-13): LOCAL_DONE / INTEGRATION_PENDING.** See [tested evidence](../backend/evidence/premium/PRM_NEXT_TEN.md#prm09-t01--typed-ports-exact-values-and-owner-scoped-room-state); G-PR-BACKEND remains pending (proceeding recorded without readiness claim), Play sandbox PROOF_OWED.

- **Dependencies/rules:** PRM08; PR05–PR09/PR14/PR25.
- **Outcome/areas:** Kotlin domain/application ports and DTOs, exact-value/golden parity, additive Room migrations, account/origin partitions and durable commands/cursors; HTTP adapter reuses proven session/signature transport.
- **Tests first:** wire unknown/version compatibility, migration with existing free vehicles, owner/origin switch, process death, replay, corrupted cursor and Go/Kotlin calculation parity.
- **Acceptance/recovery:** no generic Room replication/main-thread work; old free data survives. Roll back paid workers/entry while retaining stored user records.

### PRM09-T02 — Premium status and purchase/restore controls

- **Dependencies/rules:** T01; PRM01; PR02–PR05/24.
- **Outcome/areas:** one Perfil entry and simple status/manage/restore screen; selected current Play Billing adapter with dependency review and signed backend verification before activation. Hide production purchase until release gates.
- **Tests first:** guest/login return, pending/canceled purchase, server timeout, account mismatch, restore versus account recovery, refund/expiry and offline lease boundaries.
- **Acceptance/recovery:** server status drives capabilities; basic app remains reachable throughout errors. No real payment is taken during local development/sandbox testing.

### PRM09-T03 — Sync opt-in, devices and conflict resolution

- **Dependencies/rules:** T01/T02; PRM03; PR07–PR09/24/25.
- **Outcome/areas:** dataset switches, local-import preview, last-success/pending status, device revoke, simple base/current/proposed conflict screen and constrained WorkManager sync.
- **Tests first:** two-device conflict/delete, Wi-Fi constraints, logout/account switch during response, duplicate retry, process death and lapsed entitlement with local pending edits.
- **Acceptance/recovery:** conflict resolution is explicit and preserves local edits; successful acknowledgement is required for synced status. Disabling sync stops upload without clearing local data.

### PRM09-T04 — Automatic backup status and assisted restore screens

- **Dependencies/rules:** T03; PRM04; PR10/11/24/25.
- **Outcome/areas:** backup opt-in/status/last revision, snapshot list, compatibility/conflict preview, confirmation/progress/cancel and new-phone recovery path.
- **Tests first:** unsynced-data exclusion, corrupt/expired snapshot, interruption before/after commit, concurrent edit, rotated device/account and preserved local free vehicles.
- **Acceptance/recovery:** no automatic destructive replacement; report local application separately from server restore. Pre-restore state and idempotent result recover app restart.

### PRM09-T05 — Refuel and expense list/add/edit journeys

- **Dependencies/rules:** T01/T02; PRM05; PR13–PR15/24/25.
- **Outcome/areas:** existing vehicle entry→simple dated ledger→typed refuel/expense form, full/partial/unknown/gap/odometer segment controls, save/edit/delete and pending/conflict status; optional station only.
- **Tests first:** Portuguese decimal keyboard/rounding, absent odometer, discount, partial fill, reset/gap, rotation/draft recovery, double save, offline local-only mode and expiry.
- **Acceptance/recovery:** all inputs persist durably before upload; financial-data cloud opt-in is explicit. New local paid records obey bounded capability lease; retained-record exit/correction remains usable.

### PRM09-T06 — Monthly report, consumption and comparison screens

- **Dependencies/rules:** T05; PRM06; PR16–PR20/24/25.
- **Outcome/areas:** simple month/vehicle picker, totals/category list, supported consumption/cost explanations, vehicle/period comparison, revision/coverage and CSV share via safe Android file APIs.
- **Tests first:** golden parity, no-data/incomplete/zero baseline, mixed units, timezone month, stale report after edit, provider outage and free raw export after expiry.
- **Acceptance/recovery:** no fabricated zero/savings/profit; cached results are labelled. Text/list summaries are sufficient; no bespoke charts/PDF layout in this phase.

### PRM09-T07 — Advanced alert editor, inbox and notification handling

- **Dependencies/rules:** T03; PRM07; PR21–PR25.
- **Outcome/areas:** simple rule list/editor with supported combinations/windows/timezone, enable/disable, inbox, contextual OS permission and account-safe push deep link/token registration.
- **Tests first:** invalid rule, midnight/DST window, permission denial, no Google services/provider, expired push, process death, logout/account switch and basic free-alert regression.
- **Acceptance/recovery:** scheduling copy promises a window; push never shows another account's private details or an expired price as current. Inbox remains usable without delivered push.

### PRM09-T08 — Expiry, data exit, account switches and accessible states

- **Dependencies/rules:** T02–T07; PR01/04/05/07/12/24/25.
- **Outcome/areas:** consistent loading/empty/offline/conflict/error states, lapse/recovery copy, free export/delete entry, stale-lease handling and existing localized components/strings in supported locales.
- **Tests first:** late responses after account/origin change, navigation interruption, logout during restore/export, large font/TalkBack focus, missing translation and cached private data leakage.
- **Acceptance/recovery:** retained records and free journeys remain accessible; unpaid status never erases data or disables community participation. Narrow feature flags stop paid writes, not exit/privacy operations.

### PRM09-T09 — Integrated source and genuine device acceptance

- **Dependencies/rules:** T01–T08; end-batch device authorization; BUC-PR01–PR07.
- **Outcome/areas:** pinned backend + Android two-device/new-phone journeys, real Room/WorkManager, Play sandbox, push permission/provider, restart/offline/conflict/lapse/export/delete and accessibility/performance records.
- **Tests first:** full/partial/gap ledger and correction, stale snapshot, account switch, push after revocation, battery restrictions/large history and actual baseline free features.
- **Acceptance/recovery:** all functional rows pass on supported real devices with actual artifacts/config; unavailable rows remain OWED and gate incomplete. Visual refinement is a separate future request.

## PRM10 — Candidate, value and commercial release handoff (4 tasks)

Entry: accepted backend and Android source; release prerequisites must be proved before selling. Exit: G11 / G-PR-COMMERCIAL only with actual evidence. Validation: pinned candidate union once, affected reruns after changes and current provider/legal/listing review.

### PRM10-T01 — Final source candidate integration and immutable acceptance record

- **Dependencies/rules:** all source/backend/Android/provider gates and end-device evidence; separately authorized source delivery. Pilot/pricing and real paid activation are not prerequisites for merging tested source.
- **Outcome/areas:** exact source/API/worker/APK/schema/config matrix, one end-batch source acceptance union, protected dev→main PR with current required Quick verification and guarded finish; owned wiki mirror once after merge.
- **Tests first:** changed head/base invalidates affected evidence; missing/failed/skipped/canceled required checks block merge; free/paid/privacy/recovery source matrix contains no unimplemented row.
- **Acceptance/recovery:** preserve commits and distinguish INTEGRATED, WIKI_PENDING and RELEASE_CERTIFIED. No direct-main push, bypass, force push, recursive bookkeeping PR or CI wait loop. Keep real sales OFF pending T02–T04 and applicable release gates.

### PRM10-T02 — Pilot demand, packaging and sustainable price decision

- **Dependencies/rules:** completed public pilot and accepted feature candidate; P11-T01; PR01–PR05. Does not block earlier tested source integration.
- **Outcome/areas:** real opt-in feedback, adoption of each benefit, measured hosting/storage/push/support/moderation/provider fees and maintainer work; decide products/prices/trial/limits before listing.
- **Tests first:** verify willingness-to-pay assumptions and free usability, compare measured contribution margin with costs at documented volumes; no fabricated conversion/revenue forecast.
- **Acceptance/recovery:** the full requested feature scope remains accounted for; pricing/release can be postponed without forcing a paywall or advertising onto existing users.

### PRM10-T03 — Paid-customer terms, notices and current provider setup

- **Dependencies/rules:** T02; PRM08/09 privacy/provider evidence; integrated source candidate when publication is authorized.
- **Outcome/areas:** clear recurring charge/cancel/refund/support/retention/exit wording, app/store disclosure, verified product configuration, current billing policy and Android/provider compatibility review. Public contact/terms decisions need owner input before activation.
- **Tests first:** purchase displays actual localized price/period, account recovery stays free, expiry notice matches implemented policy, deletion works independently of payment and listing claims match evidence.
- **Acceptance/recovery:** no unverified revenue/service promise or forbidden payment shortcut; unresolved required commercial decisions block selling, not free use or prior source integration. Later implementation/config changes invalidate affected candidate evidence and use new commits/protected delivery.

### PRM10-T04 — Scoped rollout, rollback and paid operations acceptance

- **Dependencies/rules:** T01–T03; current real-production G09 and applicable G24/P10/G11 evidence; separate rollout authorization.
- **Outcome/areas:** staged paid activation with quotas/alerts/support, real purchase/recovery/cancel/exit checks, cost monitoring and paid-service disable runbook preserving privacy/free routes.
- **Tests first:** provider/DB/storage outage, billing mismatch, failed restore, expiring entitlement and cleanup during rollback; verify revocation/cancel and safe customer-data access.
- **Acceptance/recovery:** only proved immutable scope is released; disable new sales/paid work when needed, continue obligations and reconcile affected customers through approved support procedures. No messages sent merely because this plan names a support process.

## OneSignal notification amendment — planning only (2026-10-13)

The user additionally requested OneSignal notifications for new prices, promotions and Premium filters. For now, add phases and define message bodies only; no SDK/dependency, credential, provider account, notification send, deployment or Android code is authorized by this amendment. [Notification content and delivery contract](../product/NOTIFICATION_CONTENT_CONTRACT.md) contains proposed Portuguese titles/bodies, envelope, free/Premium boundary and cost constraints.

Provider choice for application push is now OneSignal behind the existing alert delivery port. Google Play billing RTDN remains a separate provider-to-backend mechanism; OneSignal does not replace it. The backend hosts scheduling/targeting/templates/inbox and calls OneSignal HTTPS; it does not self-host the OneSignal delivery service.

OneSignal ordering is dependency-based rather than numeric: PRM00–PRM06 → PRM11 content/contracts → PRM07 common alerts/inbox → PRM12 OneSignal backend → PRM08 complete backend → PRM09 minimal Android plus PRM13 push integration → PRM14 end acceptance → PRM10 candidate/commercial handoff. Preserve original task IDs. PRM07-T05 prepares the OneSignal delivery port and local fault cases; PRM12 owns the real adapter/configuration and provider proofs. G-PR-BACKEND includes PRM12 backend acceptance; G-PR-ANDROID includes PRM13; commercial notification activation also needs PRM14. None of the new phases changes the currently authorized first-ten backend task scope.

### PRM11 — Notification content, source and free-tier contract (5 tasks)

State: PLANNED / NOT STARTED. Entry: future explicit construction request and current source/free-alert contracts. Validation: DOC plus contract/golden tests when implemented. No sends.

- **PRM11-T01 — Freeze Portuguese content:** review the supplied new-price/promotion/Premium-filter templates, required placeholders, bounded title/body, neutral lockscreen alternative and source/time/condition labels. Tests first: absent/oversize/unsafe placeholder, misleading best-price/savings claim and stale price. Acceptance: content is useful and factual; recovery: fall back to generic inbox link, never invent values.
- **PRM11-T02 — Freeze event and envelope schema:** schema_version/event_id/type/template/locale/title/body/created_at/expires_at/source/condition and allowlisted navigation target; separate internal owner routing from provider payload. Tests first: unknown version/type, expired event, URL injection, private finance/GPS/token fields and illegal destination. Acceptance: synthetic positive/negative vectors and no public owner IDs. Depends T01; recovery: reject invalid envelope.
- **PRM11-T03 — Preferences, free alerts and Premium filters:** independent opt-ins for new prices and promotions; advanced server filters/time combinations require verified Premium. Preserve basic weekly alerts/free-follow rights, per-category mute, quiet hours/cooldown and logout/expiry. Tests: denied OS permission, guest, unsubscribe, disabled promotions, canceled paid-through, unknown provider state. Acceptance: free alerts never acquire a paywall. Depends T02; recovery: inbox plus current basic local alerts.
- **PRM11-T04 — OneSignal Free qualification and privacy:** recheck official limits/features/MAU counting, record server payload/identity data, approved SDK/license/security and data processing/retention. No paid upgrade, payment method or paid feature may be enabled automatically. Tests: free-cap approach/exceeded, multiple devices, opted-out counted device and provider outage. Acceptance: measured free-tier envelope or named blocker; backend hosting alone is not a zero-cost guarantee. Depends T03; recovery: pause external sends and retain inbox.
- **PRM11-T05 — Preview-only content review:** render static representative title/body/deep-link/inbox states for all templates in repository documentation or existing test UI only when implementation is later authorized. Tests: pt-BR money/time/unit, long station/fuel names, missing condition/validity, truncation and screen-reader text. Acceptance: user can review final copy without sending notifications. Depends T01–T04; recovery: revise templates/version, keep sends OFF.

### PRM12 — OneSignal backend integration and source eligibility (5 tasks)

State: PLANNED / NOT STARTED. Entry: PRM11, PRM07 local alert/inbox contracts and later explicit implementation authorization. Validation: BE/DB/PROVIDER, actual private provider tests before activation.

- **PRM12-T01 — Verified device/alias registration:** bind opaque OneSignal identity/subscription records to current server-proven owner/device/origin, with provider identity verification and logout/revoke/uninstall lifecycle. Tests: forged alias/subscription, cross-account takeover, reinstallation, duplicate/limit races and stale JWT. Acceptance: client-supplied external_id never grants another user's notifications. Recovery: unlink/re-enroll after fresh proof. Reuse existing account/device ports.
- **PRM12-T02 — New-price/promotion candidate authority:** generate events only from current supported price projections or a separately proved promotion source/representation/validity contract. Promotions require source, terms, expiration and audience consent; never become unconditional community prices, fuel-quality claims or paid trust. Tests: pending/disputed/stale observation, expired/fake promotion, revoked business authority, missing terms and duplicate source event. Acceptance: no event fabricated from a contribution alone. Depends T01/PRM07; recovery: disable affected event kind.
- **PRM12-T03 — Bounded OneSignal HTTPS sender:** server-only App API key, explicit App ID/environment, backend-owned targeting/schedule/template rendering, idempotency, TTL/timeout/no-redirect, capped payload/batch and redacted errors. Tests: 429/5xx/timeout/lost reply, wrong app/recipient, duplicate retry, unsupported Free feature and secret leakage. Acceptance: actual API acceptance separately recorded from receipt/open. Depends T02; recovery: bounded replay/inbox fallback; no new service/broker.
- **PRM12-T04 — Entitlement and consent recheck at dispatch:** evaluate Premium filters in our backend, not provider paid segments/journeys/timezone features. Recheck account/device/rule/source/consent/expiry and free-tier budget immediately before send. Tests: logout/unfollow/delete/revoke/lapse races, DST/quiet window, late delivery, large fanout and quota exhaustion. Acceptance: no stale/private/unsolicited push; zero spend without owner approval. Depends T03; recovery: discard expired candidates, preserve inbox.
- **PRM12-T05 — Provider/privacy/cost recovery evidence:** prove enrollment/send/revoke, private export/erasure, key rotation, provider outage and bounded resource use on allowed synthetic recipients. Tests: restore old DB/device mapping, disabled consent, unknown delivery status and Free-cap limits. Acceptance: actual OneSignal backend evidence or PROOF_OWED; G-PR-BACKEND remains incomplete when required proof is missing. Depends T04; recovery: external sends OFF, continue privacy cleanup/inbox.

### PRM13 — Minimal Android notification consumer (4 tasks)

State: PLANNED / NOT STARTED. Entry: G-PR-BACKEND including PRM12, PRM09 account/design-system work and future explicit Android authorization. Validation: ANDROID and provider/device proof.

- **PRM13-T01 — SDK and account-safe lifecycle:** reviewed current OneSignal Android SDK, initialization only under documented data/consent policy, existing account→opaque identity verification, logout/unlink and origin separation; no App API key in APK. Tests: guest/login/logout/account switch, denied permission, no services/offline, reinstall and late callback. Acceptance: no cross-owner identity or hidden tracking; recover through fresh registration.
- **PRM13-T02 — Channels, preferences and permission flow:** minimal existing-design controls for new prices, promotions and advanced filters, clear category opt-ins/mute and contextual Android permission; retain current basic channel. Tests: denial/revocation, system channel disabled, quiet hours, Premium expiry and no forced registration/paywall for free browsing. Depends T01; recovery: inbox/basic local alerts.
- **PRM13-T03 — Body, inbox and safe navigation:** native title/body rendering plus simple existing-design inbox; revalidate version/TTL/current account and source on tap. Allowlisted station/filter/promotion routes with unavailable/expired recovery. Tests: old/forged payload, injection, long content, rotated account, expired promotion/price and TalkBack/large font. Acceptance: body never claims unavailable price as current. Depends T02; recovery: generic inbox destination.
- **PRM13-T04 — Two-device genuine push and accessibility:** actual synthetic pushes on authorized supported devices in foreground/background/process death, duplicate/lost/delayed send, account changes, disabled channel and safe click destinations. Distinguish provider accepted/device received/notification displayed/user opened. Acceptance: real outcomes/limits recorded, no guaranteed exact-time delivery. Depends T03; recovery: disable provider consumer without losing inbox/local alerts.

### PRM14 — Notification end acceptance and free-cost launch guard (3 tasks)

State: PLANNED / NOT STARTED. Entry: PRM12/PRM13 and appropriate future end-batch authorization; commercial activation separately gated.

- **PRM14-T01 — Cross-stack event-to-app campaign:** source→candidate→entitlement/consent→inbox→OneSignal→device→tap, all new-price/promotion/filter templates and negative cases; actual provider/API/APK/config/source revisions. Acceptance: no missing required security/privacy/expiry/device case; recover via bounded replay or discard, never fabricated delivery.
- **PRM14-T02 — Free-tier budget and operations:** measure organization MAU/subscription counting against current Free limits, backend quotas, peak fanout, cost controls and opt-out/deletion/backups. Document threshold alerts and stop external sends before unapproved spend. Acceptance: no automatic paid upgrade or unsupported Free dependency; inbox remains functional. Depends T01; recheck pricing immediately before activation.
- **PRM14-T03 — Pinned notification release handoff:** reconcile source/provider/device/privacy/cost evidence and PRM10 commercial candidate; current notices/terms, source trust, rollback and support. Acceptance: only separately authorized proved scope activates; absent Free fit/provider proof leaves notification rollout blocked. Depends T02; preserve free local alerts and paid-data exit during rollback.

## P11 reconciliation and continuation

- **P11-T01:** PRM00 plus PRM10-T02/T03; product scope is now requested, demand/price/commercial activation still need evidence.
- **P11-T02:** remains SUPERSEDED by free P13 account linking; no paid duplicate.
- **P11-T03:** PRM01 subscription/capability tasks.
- **P11-T04:** PRM02/PRM03 private data/devices/sync and its PRM09 consumers.
- **P11-T05:** PRM04 backup/restore and its PRM09 consumers.
- **Additional requested Premium scope:** PRM05/PRM06 professional tools; PRM07 advanced alerts; PRM08 complete backend gate; PRM09 minimal Android; PRM10 separate commercial acceptance. One implementation per stable microtask ID; do not create duplicate parent/subtask delivery issues claiming the same outcome.

Future continuation: read AGENTS → FAST_EXECUTION → current PROGRESS → this plan/PRM00-T01 → relevant source/tests. Reconcile dev/main, isolate an occupied checkout, record the exact source and selected microtask, resolve its implementation-blocking decisions, then execute sequentially. Planning-only completion must not replace the current active security/app next action or imply G11 has started.

Planning evidence: [local document validation](PREMIUM_PLAN_VALIDATION.md). Implementation evidence files are created only during execution; absent future package/scripts/provider credentials must not be reported as passing tests.

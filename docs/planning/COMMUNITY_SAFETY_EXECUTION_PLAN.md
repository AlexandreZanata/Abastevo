# Community evaluation, abuse protection, reads and privacy

Date: 2026-10-11. Status: PLANNING_LOCAL_DONE / IMPLEMENTATION_NOT_STARTED.
Opening source: maintained `dev`, `0f3601c`. This document authorizes no runtime
change by itself. [Execution prompt](COMMUNITY_SAFETY_EXECUTION_PROMPT.md).

Offline extension: user-requested 2026-10-11, planning source `61f7c1b`.
CS15–CS25 are additional PLANNED phases. Existing CS00–CS14 IDs are preserved.
Execution order is **CS00–CS12 → CS15–CS25 → CS13 → CS14**: the connected-device
location acceptance remains last, after offline functionality is incorporated.
If a preceding phase is already accepted, reopen only its affected contracts/tests;
do not repeat completed construction or treat an old checkpoint as new evidence.

This is a bounded extension of P06/P07/P14/P15/P16/P22/P35/P37, especially
P37-PC05/PC06; it does not renumber or declare their outstanding gates passed.
Use existing modules, account/key proof, jobs, PostgreSQL/PostGIS, private media,
Compose/Caddy and Android surfaces. No speculative microservice, Redis, analytics
SDK, partition migration or paid fraud service is a default dependency.

## Accepted scope and decisions

- The user chose **helpful / not helpful contribution**, not price correctness.
  UI text: `Contribuição útil` / `Contribuição não útil`. Freeze wire names as
  `HELPFUL` / `NOT_HELPFUL` before implementation. Existing price confirmations,
  disputes and comment VALID/INVALID votes keep their separate meanings.
- Guests browse without an account. Free-account, signed writes remain the
  participation rule. Anonymous installation identification protects reads and
  protocol admission; it does not authorize voting, uploads or moderation.
- Cover useful-vote cards, separate reports, audited operator moderation,
  bounded city/fuel feeds, abuse/resource controls and optional image training.
- Training permission is optional, OFF initially, revocable in Settings, with
  per-image scope. All operational media retain the existing maximum 24-hour
  deadline, even when training is authorized. Only a separately minimized,
  authorized training derivative may remain without automatic TTL.
- The last phase is controlled Android Fake GPS and real-location acceptance
  through `https://teste.abastevo.com.br`, only in Abastevo's authorized scope.
  Fake GPS must not become trusted evidence in the normal application.
- Planning does not publish GitHub/wiki, change the VPS, install an APK or enable
  collection. During later execution, record source acceptance separately from
  deployment, independent review, device acceptance and production certification.
- Offline coverage targets every inventoried Android journey, including cached
  community prices and durable contribution intents. "100%" means every matrix
  row has a specified, tested offline outcome, not unlimited offline data, current
  remote facts, guaranteed OS scheduling or immunity to every possible attack.
  No internet means no server acknowledgement, new remote identity verification,
  remote moderation/erasure or knowledge of unseen price changes.

## Source inventory to reuse and reverify

The opening inventory is evidence of code, not live completion:

- `backend/internal/modules/community/application/votes.go`: independent price
  confirmations and structured disputes, quotas and transactional recompute.
  This is **not** the helpfulness feature requested here.
- `backend/internal/modules/feedback/application/votes.go`: change/remove one
  account/comment/revision VALID/INVALID vote, self-vote refusal and exact tally.
  Reuse its tested transaction patterns, not its semantics or historical rows.
- `backend/internal/modules/moderation/`, `backend/cmd/ops` and
  [moderation policy](../product/COMMUNITY_MODERATION.md): bounded cases, audited
  restricted operators, flagged/hidden states and appeal through a new case.
- `backend/internal/modules/identity/domain/quota.go` and adapters: keyed IP
  subjects and transactional registration/challenge/write quotas. Public-read
  protection is an explicit edge/application gap to audit, not an assumed gate.
- `backend/internal/modules/community/adapters/read/feed.go`,
  `backend/db/queries/community/feed.sql`, `backend/internal/platform/httpapi/cursor.go`:
  existing city/fuel feed, limit+one and signed 15-minute filter-bound cursors.
  Existing conditional ORDER BY and rating joins need measured plan review.
- `app/src/main/kotlin/com/anpfuel/app/community/`, the existing Community feed
  and station screens: reuse ViewModels, HTTP clients and durable command rules.
- `backend/internal/modules/{privacy,evidence}/`, location/capture in
  `backend/internal/modules/community/application/{location,location_verify,photo_capture}.go`,
  and `backend/internal/platform/jobs/`,
  [media/location policy](../security/LOCAL_MEDIA_LOCATION_POLICY.md),
  [training consent amendment](../security/AI_TRAINING_IMAGE_CONSENT_PLAN.md) and
  [photo contribution plan](PHOTO_PRICE_CONTRIBUTION_PLAN.md).
- [Portuguese notices draft](../product/COMMUNITY_PRIVACY_TERMS_PT_BR.md): product
  wording to wire only after its underlying controls exist.
- Offline reuse: `GetCommunityPriceGroupsUseCase.kt` currently tries HTTP before
  its station-price cache fallback; `CommunityPricesViewModel.kt` exposes stale
  results. `CityCommunityFeedHttpClient.kt` is network-only and
  `ui/community/CommunityFeedViewModel.kt` keeps paged cards in memory, with
  foreground refresh every 15 seconds and a 200-card bound. These paths do not
  prove persistent cache-first city-feed behavior after process death.
- `data/.../repository/RoomContributionOutboxRepository.kt`,
  `local/dao/ContributionOutboxDao.kt`, `worker/ContributionWorkScheduler.kt`,
  `application/.../contribution/DispatchContributionUseCase.kt`: existing frozen
  commands, leases, fresh attempt nonces, receipts and connectivity-constrained
  WorkManager. Current dispatch enumerates DAO `listAll()` before filtering:
  replace with bounded indexed selection during the owning offline task.
- Room database is version 9 at extension opening, with backend-price and server
  station caches, contribution outbox and upload-session tables. Discover its
  latest version before appending a migration; never hard-code the next number.
  Existing ANP imports, settings, vehicles and calculations remain in scope for
  regression, not replacement. `community/application/photo_capture.go` currently
  authorizes camera use online for two minutes and submission for 24 hours;
  it does not authorize a fresh trusted capture arbitrarily later while offline.

Opening delivery reservation: PR127's backend integration, fast and Quick checks
passed at `0f3601c`; Android `test` failed with `UncaughtExceptionsBeforeTest` in
CommunityPricesViewModelTest. JDK17 local debug/release reruns passed, which does
not resolve the remote failure. Verify/fix its actual cause before final merge;
do not rerun until green as a substitute for diagnosing coroutine isolation.

Extension snapshot: current PROGRESS records the separate Android scheduler fix
at `61f7c1b` and passing local reruns. That supersedes the opening local diagnosis
status, not remote check/merge acceptance. This planning extension does not read
or certify current remote CI, change the fix or publish its delivery.

## Behavior contract to implement before claiming completion

### B-BR-CS01 — A useful vote is not a truth signal

Bind helpfulness to a stable public contribution reference and immutable
revision, with server-resolved author. If an existing card represents a price
projection from multiple observations, first expose a safe contribution reference
or show the contributing items; never silently vote on the station or an entire
changing fuel group. Unknown/hidden/deleted/ineligible targets refuse writes.

One active vote per **account + contribution + revision**, across all device
keys. Repeat converges; changing choice replaces; remove is idempotent. Self-vote
is refused. Concurrent replacements/removal cannot inflate totals. Older-revision
votes never transfer to an edited contribution. Changing device/reinstalling does
not create a second account vote. Different accounts can still collude: risk
controls mitigate this, never claim one device equals one person.

Display both counts and the caller's own choice. If a helpfulness percentage is
shown, use the exact denominator and existing integer rounding convention;
zero votes yields no percentage. Do not add helpfulness to price confidence,
consensus weights, paid ranking or station star ratings. Public tallies reveal
no voter list, account/key identifiers, report author or private contribution data.

### B-BR-CS02 — Anonymous installation and bounded admission

Reuse an app-scoped Keystore key/proof where appropriate, with server-issued
short-lived admission credentials; avoid a second identity system. If an opaque
installation handle is needed, generate it locally and scope it only to Abastevo.
It may reset with reinstall/data removal. An arbitrary `X-Device-ID`, device model,
IP or `isMock=false` is never authentication. Verify a proof before assigning a
stronger quota class; key registration and token issuance themselves need limits.

Do not use IMEI, serial/MAC, advertising ID or invasive device fingerprinting.
No cross-app tracking, permanent raw-IP store or contributor GPS in telemetry.
Reuse rotating keyed IP digests with measured, finite retention; include key
rotation/IPv4-mapped IPv6 normalization tests. An unverified or missing app
credential gets the bounded public-read tier, not a new unbounded DB identity.
Guests retain usable cached/manual browsing and accessible recovery from limits.

Use layered per-route, per-account, verified-installation, trusted-peer-IP,
concurrency and global-cost budgets. A shared NAT alone is not a permanent ban.
Ignore forwarding headers unless the immediate proxy is explicitly trusted.
Bind signed commands to method/path/body/nonce/time/account as the current
protocol requires; mismatched owner, revoked key and replay fail before mutation.

### B-BR-CS03 — Resource exhaustion prevention

Reject over-budget work before heavy parsing, database access or image decoding.
Enforce request/header/URL/cursor/body dimensions, read/write/header timeouts,
bounded connections/streams, DB pool/statement/lock deadlines and bounded jobs.
Quota storage, in-memory maps, caches and metric labels also have cardinality,
byte and TTL limits; unique attacker keys cannot create unlimited limiter state.

Reserve bytes and concurrent sessions **atomically before upload authorization**;
cover per-account/key/IP and global admitted bytes, abandoned uploads, multipart
parts/versions, outstanding grants, decode work, queue depth and training storage.
Existing real runtime limits are audited before choosing new values. Do not
silently replace the media policy's frozen KiB/pixel contracts with guesses.
Reserve release after failure is idempotent; actual object reconciliation catches
provider upload overrun. Signing a URL alone does not enforce uploaded bytes.

When storage, deletion, quota or queue enforcement is unhealthy, close new costly
admissions with an explicit retryable result; preserve bounded public reads and
existing cleanup/status operations. No unbounded retries, automatic evidence
recovery after expiry, duplicate jobs, hot polling or growth of exception logs.
Use correct 429/`Retry-After`, 413, 409 and 503 outcomes without disclosing keys.

Application controls do not absorb a volumetric network DDoS. Audit existing
provider/edge protection and origin reachability separately. Changes to shared
edge, firewall, other workloads or paid provider services are outside the VPS
mutation authorization; record an external prerequisite rather than changing them.

### B-BR-CS04 — Reports and moderation remain separate

Keep separate targets for price observation, contribution and comment/reply;
reuse existing reasons where compatible. Freeze a bounded taxonomy for wrong
information, spam, harassment, sensitive/illegal content and other; cap any
explanation with a Unicode-aware contract. No report attachments/URLs by default.

Preserve one open case per target under the current moderation policy, with
idempotent admission and bounded counters. Record relevant immutable revision
and reason without creating a new open case for every reason/revision. Any change
to that uniqueness needs an explicit policy amendment before code. The case/API must not expose or retain
reporter identity beyond the existing policy. Keep any necessary replay/quota
token in the separate identity/admission scope with short documented retention;
never build a reverse reporter index in cases for convenience.

Dislikes and report counts never automatically delete a contribution or prove
fraud. Coordinated reports are signals for review. Reuse restricted audited ops,
reasoned decisions and explicit correction/appeal, with concurrency/fencing.
Test author deletion winning over later moderation, suspended/deleted-account
writes, self-moderation refusal, hidden-content cache leaks and recomputation.
Do not invent public admin routes, owner privileges or unplanned mute/block UX.

### B-BR-CS05 — Bounded and honest public feeds

Filter server-side by canonical municipality/UF, product, condition and supported
order. Preserve exact money, source separation, expiry and no-price/unknown
states. Keep the existing page contract unless an evidence-backed version is
needed: `httpapi/respond.go` currently defaults to 20 and allows at most 100.
Benchmark that actual maximum; any route-specific reduction to 50 needs an
explicit compatibility decision and client/error-contract tests, not a silent
change to the shared constant.
Read `limit+1`, return only the allowlist, `next_cursor` and `has_more`; avoid a
full COUNT on every page, deep OFFSET, fetching all history or one query per card.

Cursor version, filter hash, sort keys plus stable unique tie-breaker, expiry and
MAC are checked before SQL. Cap token length before base64/JSON allocation.
Sort/filters are allowlisted; never interpolate arbitrary ORDER BY. Bind any
caller-private overlay separately; public caches must not contain `my_vote`,
owner receipts or auth state. Batch caller overlays only for returned IDs.

Mutable prices, ratings and removals require an explicit consistency contract.
Default: live feed with documented changes between pages, client deduplication
and refresh; prove complete/no-duplicate traversal on an unchanged dataset and
safe behavior under insert/reprice/hide/expiry. Do not describe this as an
immutable snapshot. If strict stable ranking is needed, justify a bounded
versioned read projection/cursor-restart contract with measured TTL/storage
cost; a timestamp alone does not freeze mutable rows. Never keep a database
transaction or create an unlimited per-user snapshot while someone scrolls.

Measure every supported order and deep cursor on dense/sparse/empty cities.
LIMIT bounds returned rows, not database work. Use real PostGIS, EXPLAIN
(ANALYZE, BUFFERS), rows visited/removed, index use, sort spills and plan-vs-data
cardinality. Prefer query-specific indexed orders over CASE sorts only if proven.
Maintain module ownership for any read projection; do not add cross-module writes.

### B-BR-CS06 — Consent, revocation and deletion

Implement P37-PC05 rather than a parallel consent system. Server records purpose,
terms version/digest, account, decision timestamp, revision and per-image scope;
no checkbox/client boolean substitutes for proof. First use defaults OFF.
Setting ON applies only to future clearly disclosed image selections, with a
visible per-submission opt-out; it cannot retroactively authorize old photos.

Setting OFF locally stops new training permissions immediately, queues a signed
priority revocation if offline and displays `Revogação pendente` until the server
acknowledges. Do not claim server-side revocation while disconnected. The server
atomically invalidates outstanding old grants at revocation; stale offline ON
commands cannot reactivate consent. Re-enabling requires a fresh explicit grant
on the current server revision. Switching account/key/origin cannot replay a
different owner's privacy commands.

Product decision: OFF also starts deletion of existing owned training images
and labels, plus separate `Excluir imagens de treinamento` action. Access and
future selection stop at server receipt. Proposed internal physical deletion
SLO is at most 24 hours from that receipt, across owned derivatives, exports,
versions and backups; this is a product target, **not** a claimed statutory
deadline. Freeze/measure it before publishing an unconditional promise. Account
erasure covers the same assets. Retain only necessary minimized audit evidence
under a separately justified retention policy, not a shadow copy of deleted data.

Promotion runs before ordinary expiry, checks current server consent at commit,
crops/minimizes and excludes text/faces/plates/identifiers. EXIF stripping alone
is insufficient. Uncertain sanitization means no promotion. Restrict training
inventory, labels, roles and exports; no public gallery or automatic third-party
trainer. Every derived object has deletion/provenance ownership. Training jobs
or models are not part of this feature: their later provenance/withdrawal policy
must not promise that file deletion automatically untrains a model.

Operational first-receipt/capture deadlines never reset through retry/crop/consent.
Enforce access expiry even if cleanup fails; physical overdue objects are a failed
gate, not a successful deletion. Do not back up transient media. Restore replays
deletion/expiry before traffic. Devices powered off cannot physically delete:
enforce no expired access and cleanup on next execution, disclose this limitation.

### B-BR-CS07 — Location acceptance without a trust bypass

The ANP registry import alone is not a precise-coordinate source. Use a fixture
station with separately evidenced reviewed coordinates for boundary tests; never
overwrite a real station's point to fit a test. Unknown/centroid-only locations
must not mint a trusted 150m capture receipt. Keep public browsing available.

Test Android's real mock-location signal through the actual native adapter and
staging transport. Normal/release behavior refuses mock/stale/poor-accuracy/
replayed fixes. A clearly labeled nonrelease preview can exercise controlled
distance inputs with server-issued TEST_ONLY receipts, synthetic content and
isolated projections; it must neither claim genuine proximity nor publish normal
community trust. Release APK/backend production configuration must reject the
test-only mechanism. Client flags alone never enable a bypass.

### B-BR-CS08 — Offline availability, scope and freshness

Use persistent local data as the immediate UI read source; network refresh writes
validated results transactionally, without blanking usable local data or waiting
for an HTTP timeout. Freeze a capability inventory in CS15 across all routes,
background jobs and existing free tools. Each row records prerequisites, local
input/output, permitted cached data, queued action, server-only result, bounded
retention, failure state and executable acceptance. A hidden button is not proof.

Offline access covers previously fetched or explicitly downloaded, bounded city/
station windows. Offer an intentional "Disponibilizar cidade offline" download
with estimated size, chosen products, progress, cancel and completion manifest;
do not silently fetch Brazil or promise a never-downloaded city. Browse/search/
sort/page locally by indexed canonical keys, preserving source, money, units,
conditions and premium-product identity. A partial download is explicitly partial.
Keep verified completed data available during a failed replacement download.

Separate observation time, server generation/revalidation time, download time,
price validity and cache-retention deadline. Show "Salvo no aparelho" and
"Última atualização"; never manufacture freshness from cache reads. Once price
validity expires, any permitted last-known display is clearly historical,
"Preço vencido — confirme no posto", outside current cheapest/reliable-price
claims. Freeze finite last-known retention and privacy rules before enabling it.
Unavailable data is unknown, not zero. Offline ranking describes only the saved
subset, never all stations in the city. Changing wall clock cannot extend validity.

Public caches contain only public DTOs; keep owner overlays separately scoped
by account and origin. A disconnected device cannot learn a new remote hide/
deletion immediately. State that limit, enforce finite local content lifetimes
and revalidate/removal before renewed current display. Never cache private media
to make offline community cards work. No permanent station-specific GPS trace.

### B-BR-CS09 — Durable intent is not remote acceptance

An active local account context may save price/manual drafts, eligible photo
reviews, comments, useful-vote changes/removals, reports and privacy requests
offline. Guests may keep a local draft, but may not enqueue authenticated writes
as a fabricated account. Signing in later requires explicit review/binding; never
silently attach guest or another account's drafts. Account creation/login recovery
and remote entitlement decisions require the server; cached session presence
does not prove the account/key is still active.

Reuse the durable outbox, frozen body/revision, stable command ID and atomic local
transaction. Do not persist expiring signed URLs, bearer tokens or old signatures
as reusable credentials in payloads. At dispatch acquire current owner proof,
fresh request nonce/time and server validation while preserving operation identity.
Only a persisted matching server receipt may mark received/validated; display
"Salvo neste aparelho — aguardando sincronização" before it. A timeout after
commit triggers receipt/idempotent recovery, not a new command to evade quotas.

Order dependent commands explicitly; unsent edits may coalesce per target, but
never rewrite a frozen/in-flight body. Superseding an in-flight action waits for
its resolution. A remove/delete/revocation cannot be undone by an older delayed
create/vote/grant. Use expected revisions and existing conflict semantics;
keep reviewable user input without silently last-write-winning remote decisions.
Cancellation before dispatch differs from cancellation after server commit.

Server rechecks current account/key state, target/revision/visibility, quotas,
policy, capture age, location authorization and permission on every synchronized
write. Preserve the original claimed observation/capture time and its trust class;
never relabel an old manual price as observed at reconnect. Expired business
eligibility requires explicit re-review or refusal, not a rewritten timestamp.
Offline time grants no quota exemption or trusted timestamp. Freeze maximum
command age and server deduplication/receipt-retention horizon together; after
that horizon never replay as a new operation. Reauthentication is not permission
to move pending commands across account, environment or key-custody boundaries.

### B-BR-CS10 — Offline photos and security-sensitive capabilities

Always allow an explicit bounded local price draft where local storage permits;
clearly distinguish it from eligibility for a trusted photo contribution. The
existing two-minute capture receipt cannot be minted, refreshed or backdated
offline. Previously authorized captures may finish review and queue only within
their original camera/submission deadlines. Local-photo retention stays capture
+24h; evidence expiry is not extended to make delayed sync convenient.

CS20 must define and test a separate, versioned offline-capture admission contract
before claiming a full offline camera journey. Preferred bounded option to
evaluate: server-preissued, signed, single-use permits tied to account/key,
reviewed station/coordinate version, policy, finite issuance/byte quotas and
absolute capture/submission expiry. A permit authorizes a limited attempt, not
the truth of a client fix or time. Backend verifies single consumption, original
deadlines, policy/current revocation and capture-time risk evidence on reconnect.
Review what can be evidenced without persisting precise GPS, including secure
ephemeral proof; do not place raw fixes in general Room JSON or logs.

Clock/elapsed-time/reboot uncertainty, compromised devices, missing mock signal,
no reviewed station or absent/expired permit must not result in trusted proximity.
Backend freshness policy must explicitly distinguish delayed captured evidence
from a current fix; do not bypass age checks or substitute reconnect GPS. If a
safe contract cannot justify a claim, preserve the draft as unverified and ask
for a fresh capture when authorized; normal drafts cannot inherit TEST_ONLY trust.
Never present an online recapture requirement as completed fully offline capture.

Offline OCR must have its required engine/model already locally available.
Audit the current downloaded Play-Services model and first-install/no-services/
eviction cases. Provide manual review/input fallback without blocking the draft.
If actual first-use offline OCR is required, record a measured model/package/
license/security/size decision before adding an engine or bundling assets.
No silent cloud fallback, background tracking or expired-photo resurrection.

### B-BR-CS11 — Bounded recovery, privacy and truthful coverage

Freeze device and server ceilings for commands, payload/media bytes, pages,
offline city packs, jobs, retained receipts, retries and sync work per wake.
Do not evict unsent user work silently to admit newer work. On full storage give
a recoverable refusal and explicit user-managed removal; expire photo bytes on
time while retaining only minimized non-photo status/draft facts where allowed.
Cache eviction cannot accidentally delete queued evidence still within policy.

Use the existing WorkManager and DB lease/fencing with bounded batches, concurrency,
jitter/backoff, attempt/deadline caps and `Retry-After`. Network-connected does
not mean the API works: handle captive portal, DNS/TLS failure, timeout, 401/403,
409, 413, 429 and 5xx by their semantics. No insecure TLS fallback or hot polling.
Reconnect storms cannot run an unlimited worker per queued item. Apply server
quotas to queued submissions exactly as to live ones; locally saved commands are
untrusted input. Prioritize privacy/deletion/status with bounded fair scheduling,
so an attacker cannot starve cleanup or normal reading via a priority queue.

Logout/account erasure/origin switch pauses dispatch and removes accessible owner
overlays, with explicit draft disposition under the existing custody policy.
Offline erasure shows local effect and server request pending, not global deletion.
Training grants cannot be applied while revocation remains unresolved. Device
backup/restore/key loss cannot replay another installation's queue, regrant
consent or resurrect expired media; existing official data must survive safe
schema upgrades. Rooted-device manipulation remains a documented residual risk.

Acceptance is all inventoried rows tested across empty/warm/partial/expired local
data, guest/account state, process death/reboot and unavailable/flapping backend.
Mark first-time downloads, fresh remote prices, remote identity/moderation,
server publication and genuine physical deletion on a powered-off device as
network/OS-dependent where applicable. Never claim absolute abuse prevention,
zero data loss under hardware destruction or guaranteed immediate background sync.

## Use cases and acceptance identities

- BUC-CS01: guest reads bounded feed; technical admission improves quota only.
- BUC-CS02: active free account marks helpful, changes/removes and recovers a
  lost response without a second vote; another account/device cannot act as owner.
- BUC-CS03: report → bounded case → restricted decision → public visibility/cache
  update → new-case appeal, with no voter/reporter identity disclosure.
- BUC-CS04: scroll/retry/change filter/offline/reopen with bounded memory and
  correct sources/conditions, no query or retry explosion.
- BUC-CS05: opt-in per future photo → minimized private derivative → opt-out or
  erasure → revoked access and measured physical purge/restore exclusion.
- BUC-CS06: genuine/staging/test-only/mocked location follows distinct outcomes
  through UI, HTTP, receipt, worker and public projection.
- BUC-CS07: warm/partial/empty local city feed → offline local pagination/search
  → stale/expired last-known labels → bounded revalidation without mixed versions.
- BUC-CS08: offline draft/action → durable owner-bound pending state → restart/
  retry/reconnect → one verified server outcome or explicit conflict/expiry.
- BUC-CS09: offline OCR/photo/permit/clock uncertainty → truthful eligible or
  unverified state → no trusted promotion on replay, stale time or mocked location.
- BUC-CS10: offline revocation/logout/storage pressure/backup restore → no private
  leakage, stale grant, extended photo deadline or unbounded sync amplification.

## Sequential phases for an economical implementation agent

Execute one bounded task at a time. At each opening read only current PROGRESS,
the selected section, owning contract and relevant code/tests. If a phase is too
large, split into named A/B tasks with their own RED/GREEN and evidence. Do not
reread the repository, refactor surrounding modules or ask for permission already
present. Each phase names expected files; CS00 must replace any missing path with
its actual discovered owner before code. Phases below are PLANNED, not passed.

### CS00 — Audit and freeze the implementation slice

Read the inventory above, actual routes/DI, current migrations/indexes, quotas,
Android cards and policy versions. Reconcile dev/main and other contributors.
Produce a compact reused/missing inventory, target/reference mapping, supported
orders, exact quota/media constants, proposed API/error shapes and threat list.
Record PR127/Android failure as an independent delivery blocker. Freeze meaning
of helpfulness, all B-BR/BUC rules and benchmark stop limits before behavior.
Exit: testable scope/ownership and no invented endpoint, limit or live gate.

### CS01 — Helpfulness domain and contract

Extend the existing owning module with helpfulness commands/read DTOs; do not
reinterpret confirmation/dispute/VALID votes. RED tests for self-vote, inactive
account, target revision, edit/delete/hide, change/remove/retry and zero totals.
Freeze public reference and owner overlay, privacy/export/erase effects and
OpenAPI/shared money/product fixtures. Exit: pure tests and contract compatibility.

### CS02 — Atomic votes, HTTP and projections

Append migration(s) for unique account/target/revision votes plus compact tally
and rebuild jobs only as needed. Reuse explicit ports and signed account proof.
Real PostGIS: concurrent casts/flips/removals, crash after commit/lost response,
rebuild replay, upgrade/recovery/restricted role, erase/restore and old-key attacks.
Test malicious target ownership, public no-leak fields and counter reconciliation.
Exit: exact stored/tally agreement; helpfulness changes no price confidence.

### CS03 — Anonymous admission and multi-layer quotas

Audit/reuse identity/edge plumbing; add verified-installation admission only
where missing. Version operation-specific budgets, cardinality caps, TTL/cleanup,
trusted-proxy handling and retry outcomes. Test reinstall/key farm, forged headers,
IPv6 variation/rotation, NAT fairness, replay, depleted/failed quota store and
simultaneous requests. No real DDoS. Exit: finite CPU/state/DB work under locally
generated abuse, anonymous reads remain usable and account votes cannot multiply.

### CS04 — Storage, processing and queue admission

Atomic outstanding-byte/session reservations, bounded upload grants and provider
reconciliation; private-S3 tests for abandoned/multipart/oversize/corrupt/decode
bombs/duplicate finalize/job failure, full storage and purge outage. Use existing
worker/job lease, backoff/dead-letter and per-image limits. Bound total collection
and staged exports too, even when authorized images have no automatic TTL.
Exit: quotas cannot oversubscribe concurrently and no attacker-controlled memory,
disk, objects, jobs or logs grows without a configured ceiling.

### CS05 — Report and moderation completion

Extend existing report targets/reasons and reuse ops. Cases, counters and queue
queries bounded; no reporter identity leak. Test brigading/self-target/duplicate
reports, concurrent decisions, hide/erase/cache/recompute, operator role escalation,
audited corrections and new-case appeal. Freeze practical moderation staffing,
backlog alerts and response targets as operational prerequisites, not imaginary
automated safety. Exit: an accepted report has one traceable case and no automatic
removal or confidence bonus from its count.

### CS06 — Query and pagination implementation

Own the existing feed/SQL/cursor contract. Validate bounds before DB, keyset all
supported orders, batch public counters and account overlay, no per-card N+1.
Use parametrized SQL; append only justified indexes/read projections with upgrade
tests. Test ties, first/middle/deep/final pages, filter/order/secret changes, bad
and oversized cursors, expired rows, update/hide during scroll and no-price cities.
Exit: fixed-dataset traversal matches a full independent oracle; live mutation
behavior matches the documented consistency contract.

### CS07 — Reproducible read and abuse performance evidence

Build an isolated synthetic corpus with realistic city skew, all products,
conditions and sparse/dense/empty cells; retain manifest/seeds. Proposed local
scales: 10k/100k/1M contribution facts, subject to explicit disk/memory budgets;
do not load these synthetic facts into the normal staging feed. Run the actual
HTTP handler/process plus separate SQL diagnostics, not Reader calls labeled API.
Use pages 1/20/50/100 (current maximum), deep cursors, each supported sort, warm/cold-labelled states,
read-only and mixed vote/report/ingestion workloads. Start with a pilot; local
qualification uses at least five independent measured windows of >=120s at
selected safe rates, with a separate warmup. State all reduced/unfinished cells.

Record p50/p95/p99/max and sample count, offered/completed/accepted/rejected rates,
429 versus unexpected errors, timeouts/drops/cancellations, bytes, CPU/RSS/cgroup
peak, pool waits/connections, rows/blocks visited, WAL/temp spills, queue age/depth,
storage reservation/actual bytes and cleanup lag. Throughput uses wall elapsed
time for the concurrent window, never sum of concurrent durations. Include the
load generator's resource/correctness budget. Normal allowed traffic must remain
correct while excess traffic is rejected without growing state indefinitely.

Freeze provisional local p95/p99/query-work/memory budgets in CS00 against the
baseline and host envelope; never select them after observing the candidate.
Report regression and saturation points with confidence/uncertainty. No maximum
user claim without a documented user-action mix and sustained arrival model.
Staging later starts below the previously guarded 5 requests/s ceiling; that
ceiling is not permission to consume exhausted host reserve or claim capacity.
Exit: reproducible baseline/candidate report and raw private artifacts, no invented
speedup, partition adoption or capacity certification.

### CS08 — Android cards, reports and paged loading

Reuse Community/station cards and existing account/outbox clients. Show accessible
helpful/unhelpful counts, selected choice and separate `Denunciar`; guest actions
explain free login. Local intent is pending until acknowledged; rollback/retry
never shows false server success or silently repeats a report. Reuse stable
command IDs, serialize/cancel stale responses and enforce current account/origin.
Use existing paging tools if present; introduce a library only with recorded need.
One in-flight page/filter generation, bounded prefetch/cache/window, stable item
keys, honest loading/empty/error/footer/stale states. No infinite auto-retry,
duplicate append, excessive whole-list rebuild or per-card network call.
Exit: ViewModel/HTTP/Room lifecycle tests, compile/lint, TalkBack/large-text checks
for the new controls; final device matrix remains CS14-owned.

### CS09 — Server consent, inventory and rights

Implement the existing P37-PC05 contract using privacy/jobs/media ports. Append
server consent revision/receipt and derivative inventory with per-image ownership.
Owner-only signed read/grant/revoke/delete and account erase/export are consistent.
Real DB/S3 tests cover owner substitution, revoked keys, replay, absent consent,
old terms, stale offline ON, repeated OFF, multiple devices and account changes.
Exit: OFF stops selection atomically; a stale grant cannot undo revocation.

### CS10 — Promotion, 24-hour expiry and physical deletion

Bounded worker sanitization with fail-closed uncertain images; no model training.
Implement/check every original/quarantine/sanitized/thumb/outbox/derivative/export/
object-version/multipart/backup/restore path. Deadline and deletion-ledger tests
use injected time; actual storage evidence includes a controlled real 24-hour
soak with objects nearing expiry, periodic inventory and a restore drill. Do not
claim a virtual-clock test proves wall-clock scheduling. Test promotion/revoke/
delete races at commit, provider/worker outage and ledger replay before restored
traffic. Exit: access refuses at deadline; actual all-copy purge meets the frozen
SLO. A late object is a failed gate; collection stays OFF until corrected.

### CS11 — Settings, notices and privacy UX

Implement `Configurações > Privacidade > Melhorar o reconhecimento de preços`,
initially OFF, explicit per-photo permission, terms/version/date, server state,
pending offline revocation, delete progress and rights/contact entry. Toggle OFF
means revocation plus owned training-asset deletion per B-BR-CS06; also offer explicit
delete without silently granting/altering consent. Guest cannot claim server
consent. Settings never wipes the mandatory session merely to change a preference.
Wire the Portuguese draft only to behavior actually delivered. Replace production
controller/contact/processors/transfer/retention fields with verified values;
test `.invalid` contact remains restricted to staging. Exit: wording and all
ON/OFF/offline/reopen/account-switch/error states match server evidence.

### CS12 — Integrated adversarial acceptance and handoff

Run the affected journey through real HTTP + PostGIS + private S3 + workers and
Android transports. Audit abuse boundaries below, money/product/source invariants,
erasure, false-positive NAT impact and observer privacy. Resolve known failures;
freeze source/config/model/artifact hashes. Produce an independent reviewer pack,
not a self-awarded production certification. One end aggregate after remote gates
allow finalization; no per-phase CI polls or repeated whole-repository builds.
Exit: LOCAL_DONE with actual evidence; independent review remains explicitly due.

### CS13 — Guarded Abastevo-only staging preparation

Additional prerequisites: CS15–CS25 source/offline acceptance and affected
CS12 integration evidence. Numbering is stable; execute this phase after them.

Preflight `/home/iiii/I9-PROJETOS/VPS-HOSTINGER-I9-32GB/` documentation read-only,
then inventory the actual Abastevo container/namespace, images, limits, state,
private media, jobs, schema and domain. Do not infer Docker versus Kubernetes.
Use only existing authorized Abastevo scope (`abastevo-temp` when still current).
Never alter other containers/images/volumes, shared Caddy/firewall/host settings,
or issue global prune/down/restart commands. Preserve original data and deadlines.

Prove backup/recovery, compatibility and resource reserve first; rollout one
component at a time without recreating a media emptyDir. Check retained media,
API readiness, old/new-client compatibility, OOM/restarts and queue/deletion
health. The old developer preview expires 2026-10-15; do not extend it silently
or resurrect expired photos/jobs. A new valid test window must be explicit.
Use existing guarded host/readiness/load tooling; never stress-test DDoS on this
shared VPS. If upstream/shared changes are needed, record BLOCKED_EXTERNAL for
that boundary while finishing other authorized work. Exit: exact tested staging
revision and nonrelease TEST_ONLY receipts isolated from normal projections.

### CS14 — Final connected-device GPS and complete journey

Run only after preceding source/staging prerequisites pass. Preserve account/data;
use a synthetic test account/content and a verified fixture location. Record
private device/API/APK/backend/policy versions; no precise GPS/photo/key in Git.

1. Real PostGIS boundaries: 149.9m / 150m / 150.1m with reviewed coordinates,
   controlled age/accuracy, plus unknown/centroid locations. Freeze inclusive
   <=150m rule and existing uncertainty treatment; do not choose accuracy margins
   from test results. This proves distance math independently of phone sampling.
2. Through the phone's normal app/native adapter, enable Fake GPS as Android's
   mock provider: nearby, distant and teleporting fixes must refuse trusted
   capture. Tamper `isMock=false`, stale/future/replayed/cross-station receipts,
   denied/coarse permission and offline retry; backend authority must hold.
3. Nonrelease TEST_ONLY mode tests near/boundary/far with synthetic media and
   clear labels through the real staging HTTPS path. This proves wiring/geometry,
   not genuine location. No normal consensus/profile/location promotion.
4. Disable Fake GPS and obtain genuine fixes near/far from an actually reviewed
   station. Include poor-signal uncertainty, capture/OCR/review/subset upload,
   premium, helpfulness/change/remove, report/ops visibility, pagination and
   consent ON/OFF/delete/receipt. If genuine location or reviewed station data
   is unavailable, mark that row OWED; simulated success cannot replace it.
5. Verify signed release excludes debug controls and refuses test receipts.
   Restore the mock-location setting changed by the test; preserve unrelated
   developer settings and user data. Confirm only intended synthetic test records
   were created and their retention/deletion works.
6. Repeat the scoped offline journey against the actual staged revision: download
   a bounded city pack, airplane mode, force-stop/reopen, browse saved community
   prices, draft/review/queue, revoke training, reconnect and reconcile exact
   receipts/counts. Include 24h photo expiry, expired permit, login/key change,
   removed target and 429 recovery. Airplane mode with genuine GPS available must
   follow CS20's accepted capability contract, never an assumed online receipt.
   Run mock-location and genuine-location rows separately from network state.

Exit: independent traceable results for trusted, refused, unknown and TEST_ONLY
states. Zero false trusted capture for normal mock tests; no universal spoofing
or nationwide-coordinate accuracy claim. G09 production/pilot remains separate.

### CS15 — Offline capability inventory and acceptance freeze

Dependencies: CS00–CS12 source contracts; existing P10-T05/T08, P21-T03/T04 and
P37-PC01–PC06 obligations remain owned, not replaced. Execute before CS13/CS14.

- CS15-A: enumerate actual navigation, use cases, imported ANP tools, city/station
  prices, fuel/condition filters, search/sort/page, vehicle/calculation/history,
  account/local unlock/logout, capture/OCR/review/outbox, comments/votes/reports,
  settings/consent/privacy and any external-provider/payment surfaces that exist.
  Record code owner and scenario ID for every row; do not invent missing features.
- CS15-B: freeze B-BR-CS08–CS11/BUC-CS07–CS10, exact current cache/queue limits,
  stale/expired display, guest/account prerequisites, offline permit decision and
  expected queue/receipt conflict states. Each row is locally complete, durable
  intent pending sync, or explicitly dependent on unavailable external data/proof.
- CS15-C: define executable scenario manifests and cold/warm/partial cache seeds,
  supported devices, resource ceilings, regression budgets and review ownership.

Exit: complete, reviewable capability manifest with no undefined offline outcome.
This is coverage planning, not an assertion that all rows have passed.

### CS16 — Persistent cache-first community reads

Own domain read ports/application use cases, Room entities/DAOs/migrations,
`BackendPriceRepositories.kt`, `GetCommunityPriceGroupsUseCase.kt`,
`CityCommunityFeedHttpClient.kt` and the two community ViewModels.

- CS16-A: add only needed normalized public-cache/query-membership metadata and
  separate private overlay storage; preserve v9/current migrations and ANP data.
  Freeze IDs/revisions/times/source/unit/condition and bounded retention contracts.
- CS16-B: observable local reads render immediately; validated HTTP refresh
  commits atomically. Cancellation cannot publish old city/account/origin data.
- CS16-C: real Room upgrade/recovery tests, killed-process restart, network timeout
  and malformed page, mixed official/community products, currency precision,
  stale/expired/no-cache states and account-overlay leak negatives.

Exit: cached city and station community prices are usable after process death
without any required network round trip; last-known data is never marked current.

### CS17 — Bounded city packs and local pagination

- CS17-A: explicit city/product download with byte/row/page/time ceilings, quota
  preflight, completion manifest and atomic active-pack switch. Cap staging as
  well as completed data. Freeze guest pack count and eviction/pinning rules.
- CS17-B: indexed local keyset queries for all supported filters/orders; separate
  local continuation from expiring server cursors. No all-rows materialization,
  count-per-scroll or resetting scroll into a different downloaded generation.
- CS17-C: empty/partial/overlapping/failed/cancelled downloads, disk-full,
  changing source generation, boundary ties, capped/incomplete coverage and
  simultaneous pack/read/queue writes. Old complete pack survives failed refresh.

Exit: an independent seeded oracle matches local pages; unrequested cities/fuels
remain explicitly unavailable. Measured disk/heap stay inside frozen ceilings.

### CS18 — Revalidation, revisions and deletion propagation

- CS18-A: audit existing ETag/read versions and freeze removal/hide/expiry rules.
  Prefer bounded replacement/revalidation of retained windows. Introduce a delta
  endpoint/change log only if required and measured; a feed cursor is not a delta
  watermark. No generic speculative synchronization service.
- CS18-B: enforce atomic generation/revision acceptance and tombstones where
  needed; bind resume state to origin/query/schema/policy, not a user-controlled
  timestamp. Keep finite delta retention and explicit stale-watermark reset.
- CS18-C: out-of-order/duplicate pages, removed/edited target, clock rollback,
  reconnect after retention gap, invalid MAC/ETag, cache poison/corruption and
  interrupted replacement. Public/private cache partitioning remains enforced.

Exit: reconnect never revives known-deleted content or rolls a newer row backward;
offline stale availability and remote-deletion visibility limits are documented.

### CS19 — Durable offline social and price intents

Reuse `ContributionOutboxPorts.kt`, `RoomContributionOutboxRepository.kt`,
`ContributionOutboxDao.kt` and portable command/lease/revision patterns.

- CS19-A: bounded durable draft storage and explicit guest-to-account binding;
  atomically freeze owner/origin/target/version/command identity and payload.
- CS19-B: integrate queued price/manual contributions, eligible shared-photo
  rows, comments, helpfulness set/remove, reports and privacy requests through
  owning contracts. Unsigned local state is not a server admission credential.
- CS19-C: exact retry/partial-batch/lost-response/process-death tests, rapid
  vote flips, edit/remove after dispatch, duplicates across keys, cancelled versus
  received distinction, changed account and erased/hidden/revised target conflicts.

Exit: every accepted local intent survives ordinary restart with an honest pending
state; one matching server effect/receipt after sync, or an actionable rejection.

### CS20 — Offline capture, OCR and trustworthy deferred evidence

Backend contract/tests precede Android consumption; own existing community capture,
location verification, evidence sessions, encoder/OCR and photo-outbox ports.

- CS20-A: freeze threat/permission/age/clock model and versioned offline permit
  decision from B-BR-CS10. Test signed issuance with quotas, finite ownership/
  station/policy deadlines, one-use consumption and replay/revocation in real
  PostGIS. Rejection leaves no photo upload reservation or trusted observation.
- CS20-B: local bounded camera/review/OCR/manual draft, model availability and
  monotonic-time anchor/reboot handling. Do not persist raw GPS in normal outbox
  fields. Specify encrypted short-lived evidence only if necessary and reviewed.
- CS20-C: airplane mode with genuine native fix, mock/coarse/missing signal,
  reviewed/unknown/changed station point, absent/expired/stolen permit, changed
  account/key, future/backdated time, reboot and upload just before/at/after expiry.
  Verify delayed evidence is not classified using reconnect position/time.

Exit: named scenarios prove eligible deferred capture versus unverified local
draft. If bounded permits cannot meet the integrity contract, trusted offline
capture remains BLOCKED_DESIGN, not quietly replaced by an online-only workflow
and called complete. Manual drafts and already-authorized queues remain usable.

### CS21 — Bounded synchronization and recovery

- CS21-A: replace `listAll()` dispatch scans with indexed due/owner-scoped batches,
  fixed per-wake work budget and lease/fencing; reuse WorkManager, unique work and
  foreground status. Bound scheduled work count, not just active worker threads.
- CS21-B: fresh proof/nonce per actual send, stable command/body identity, receipt
  recovery, expected revisions, dependencies and ordered privacy revocation.
  Freeze finite dedup horizon with maximum queue age; stale jobs cannot reappear
  under new IDs. Apply account/IP/operation quotas and byte reservations normally.
- CS21-C: client/server crash at every commit boundary, duplicate workers, lease
  expiry, captive portal, bad TLS/DNS, 401/403/409/413/429/5xx, no-network startup,
  flapping connection, account suspension, retry exhaustion and poisoned queue.

Exit: at-least-once transport yields idempotent bounded effects, no request storm,
no skipped revocation and fair bounded progress when prerequisites return. Device
scheduling limits and user force-stop remain explicit, not a sync deadline promise.

### CS22 — Offline privacy, media expiry and local recovery

- CS22-A: audit storage/key custody/backup rules for cache, drafts, owner overlays,
  upload sessions, permits and image files; encrypted private fields, no auth
  secrets/signed URLs in backup, and finite retention for all queue metadata.
- CS22-B: OFF/delete/logout/account switch work locally while retaining only the
  minimal owner-bound pending request needed for server reconciliation. Unsent
  training grants cannot overtake revocation. Local cancellation cannot undo a
  remotely committed contribution or erase its acknowledged status deceptively.
- CS22-C: capture+24h purge while offline, clock rollback/reboot/long power-off,
  app update, DB migration/corruption recovery, key invalidation, backup/restore,
  queue-full and disk-full. Do not destructive-migrate or wipe ANP/user data.
  Expired photos are removed, not kept until uploads succeed; remaining text
  drafts need explicit re-review if evidence expired.

Exit: no expired media access or stale-consent replay; physical cleanup follows
the existing powered-off limitation and real-storage evidence. Unknown/corrupt
state refuses unsafe work with a recovery message, not silent data destruction.

### CS23 — Complete offline UX and accessible feedback

- CS23-A: wire every CS15 capability row: offline saved prices/coverage/time,
  local queries/tools, manual city selection, model-unavailable manual entry,
  durable draft/queue status, privacy pending and explicit server-only actions.
- CS23-B: distinct `Salvo neste aparelho`, `Aguardando sincronização`,
  `Recebido pelo servidor`, `Em validação`, `Publicado`, `Conflito`, `Expirado`
  and retry/reauth/review outcomes, matching real contracts. Never show
  `Preço enviado` on enqueue or treat HTTP receipt as already published.
- CS23-C: TalkBack/large font/rotation/back-stack/process restart, no indefinite
  spinner, offline cold start and cached local unlock without a remote request.
  Preserve successful queued work when user navigates back to Community.

Exit: each inventoried route has a useful tested outcome, including unavailable
first-use data. UI copy, offline scope and privacy terms agree with implementation.

### CS24 — Offline failure, abuse and resource qualification

- CS24-A: deterministic fault/race matrix with real Room/PostGIS/private S3 and
  HTTP, queue/cardinality/storage overflow, malicious old clients, altered local
  payloads/permits, multi-account devices and reconnect wave against local services.
  Server rejection controls remain authoritative even if local checks are bypassed.
- CS24-B: freeze/measure cold/warm offline startup, first cached render, deep local
  pages, SQL rows visited, queue depth/due-selection latency, device heap/disk/
  battery-work proxies, network bytes/wake, sync completion/429/backoff, generator
  load and server CPU/memory/locks/storage. Use declared repeated windows, exact
  workload denominators and baseline/candidate comparisons from CS07.
- CS24-C: actual 24h expiry/reconnect soak plus injected-clock boundaries,
  intermittent connectivity, process death and restored-state checks; no real
  DDoS or VPS limit escalation. Late bytes, duplicate effects, private leaks,
  lost acknowledged writes or unbounded growth fail the corresponding gate.

Exit: reproducible measured safety envelope and named residual risks, never an
absolute "all abuse prevented" or maximum-user claim.

### CS25 — Offline end-to-end acceptance and reviewer pack

- CS25-A: run all CS15 manifest rows on the supported Android devices against
  isolated real services, preserving user accounts/data. Cover empty/warm/partial/
  expired caches, guest/owner/changed account, airplane mode, backend outage,
  flapping network, force-stop/reopen/reboot and reconnect. Device lifecycle
  observations do not replace deterministic concurrency/clock tests.
- CS25-B: independent oracle reconciles each local operation with zero/one intended
  server effect and exact receipt, tally, moderation case, public projection and
  expiry outcome; distinguish refusal/conflict from lost data. Include no-network
  cold reads and no stale price as current in automated UI assertions.
- CS25-C: publish local sanitized evidence and a coverage summary with explicit
  PASS/FAIL/OWED/NOT_APPLICABLE reasons and dependencies. Any required failed or
  owed row blocks full offline acceptance. Extend the CS12 reviewer pack, refresh
  affected integration evidence, then proceed to CS13 and final CS14 staging GPS/
  offline journey; only the final authorized batch is delivered remotely.

Exit: all defined feasible offline behavior is traceable; external dependencies
are accurately classified, not counted as implemented capabilities. No release
certification follows from simulated location, a plan or a cache-only test.

## Required abuse coverage register

CS00/CS12 maintain one compact list linking threat -> control -> immediate
negative/concurrency test -> metric -> residual limitation. Cover at minimum:

- forged account/device/header/nonce, key cloning/rotation/reinstall, account
  farms, shared NAT, IPv6 churn, proxy/VPN and targeted rate-limit denial;
- self-vote, retry/flip/remove races, duplicate account votes across keys,
  stale revisions, report brigading and moderation privilege escalation;
- slow/oversized/malformed requests, cursor/header/decompression bombs,
  costly search/sort/radius, enumeration/scraping and cache-key cardinality;
- abandoned/presigned/multipart uploads, forged MIME/dimensions, decode bombs,
  object/version/quarantine/thumbnail/training/export growth and storage-full;
- DB connection/lock amplification, projection stampedes, unlimited jobs/retries,
  poison/dead-letter growth, queue lag and deleted-content cache leakage;
- consent tampering, offline stale grant, account swap, revoke versus promotion/
  export/restore races, expired media resurrection and model-use overpromises;
- mock/stale/coarse/teleported fixes, unknown station points, forged test flags,
  boundary math and release-debug leakage.
- offline key/account/origin swaps, old command/receipt replay, altered queue
  payloads, dedup-horizon gaps, expired/stolen permits and clock/reboot rollback;
- reconnect storms, queue/dependency/priority starvation, unlimited WorkManager
  jobs, cardinality/pack/download storage growth and device battery/IO exhaustion;
- stale/removed content resurrection, public/private offline overlay leakage,
  poisoned generations/tombstones, backup key loss and expired-image restoration.

No finite test list proves immunity to all abuse. Document remaining volumetric
edge dependency, coordinated genuine-account behavior and compromised-device risk.

## Test and evidence contract

Use [TEST_STRATEGY](../backend/TEST_STRATEGY.md) and [backend commands](../../backend/README.md).
Select exact owning packages in CS00. Existing command building blocks:

```sh
go -C backend test -race -count=1 ./internal/modules/community/... ./internal/modules/feedback/... ./internal/modules/moderation/...
go -C backend test -race -count=1 ./internal/modules/identity/... ./internal/modules/privacy/... ./internal/modules/evidence/... ./internal/platform/jobs/...
# Dedicated disposable DSN only; provide actual Rust preparer per backend/README.
go -C backend test -race -count=1 -p 2 -tags=integration ./internal/modules/community/... ./internal/modules/feedback/... ./internal/modules/moderation/... ./internal/modules/identity/... ./internal/modules/privacy/... ./internal/modules/evidence/... ./internal/platform/jobs/...
./gradlew :domain:test :application:test :data:testDebugUnitTest :app:testDebugUnitTest --no-daemon --max-workers=2
./gradlew :data:lintDebug :app:lintDebug :app:assembleDebug :app:assembleRelease --no-daemon --max-workers=2
bash scripts/scan-secrets.sh
git diff --check
```

These are affected-scope building blocks, not commands to rerun after every edit.
SQL changes also require sqlc generate/vet/diff, migration upgrade/recovery and
least-privilege tests; HTTP changes require OpenAPI/static checks. Keep domain
RED -> GREEN -> REFACTOR and immediate critical tests, never mirror-only tests.
No fake live traffic, user photos, identifiers or exact contributor GPS in Git.

Each phase evidence file under `docs/backend/evidence/community-safety/` records
task/rules, actual SHA or reproducible source fingerprint, exact commands and
counts, environment/resource ceilings, RED/GREEN outcomes, privacy/migration/
concurrency negatives, HTTP/pagination oracle, metrics/raw artifact hashes,
rollback, limits and next task. The path is a future deliverable, not existing
evidence. Use LOCAL_DONE / INTEGRATION_PENDING / INTEGRATED / WIKI_PENDING /
RELEASE_CERTIFIED accurately. Keep PROGRESS concise; no bookkeeping-only PRs.

Reviewer return pack: plan task list and each acceptance result, source diff,
OpenAPI/migrations/index plans, immediate test reports, HTTP benchmark manifests,
all-copy expiry/deletion/restore evidence, APK/backend hashes, private synthetic
device traces, known failures/OWED rows and safe rollback. Independent validation
is expected when the user returns; the executing AI cannot waive it.

Offline pack additions: CS15 route-to-scenario manifest, executed coverage,
Room schema/migration/recovery results, local query plans and queue ceilings,
permit/clock/age contract, lost-response receipt/effect oracle, reconnect arrival
profiles and resource metrics, media expiry under offline conditions, account/
origin/backup leak negatives and actual device/network-state evidence. Missing
first-use data and server-dependent results must be visible in the UX and report.

## External references checked 2026-10-11

Android recommends app-scoped, resettable identifiers and minimizing hardware
identifiers: [Android identifier guidance](https://developer.android.com/identity/user-data-ids).
The installation design above is our proposed reuse of Abastevo keys, not a claim
that Android supplies an infallible anonymous-person identifier.

Layered resource controls follow [OWASP DoS guidance](https://cheatsheetseries.owasp.org/cheatsheets/Denial_of_Service_Cheat_Sheet.html)
and [file-upload guidance](https://cheatsheetseries.owasp.org/cheatsheets/File_Upload_Cheat_Sheet.html).
Query validation uses [PostgreSQL18 EXPLAIN](https://www.postgresql.org/docs/18/using-explain.html).
Specific budgets, phases and acceptance tests above are proposed project choices.

Offline extension references checked 2026-10-11:
[Android offline-first architecture](https://developer.android.com/topic/architecture/data-layer/offline-first)
supports local persisted reads and explicit synchronization/conflict decisions;
[WorkManager constraints/retry](https://developer.android.com/develop/background-work/background-tasks/persistent/getting-started/define-work)
informs bounded scheduling. The permit design, horizons, quotas and acceptance
sequence above are proposed Abastevo decisions, not guarantees supplied by Android.

The notice separates optional permission and rights using
[ANPD titular guidance](https://www.gov.br/anpd/pt-br/assuntos/titular-de-dados/direito-dos-titulares)
and [LGPD, especially articles 8, 9, 15, 16 and 18](https://www.planalto.gov.br/ccivil_03/_ato2015-2018/lei/l13709compilado.htm).
Production controller/contact/operator details and legal applicability still need
actual owner facts; a professionally written draft is not runtime or legal certification.

# Security and offline reassessment — 2026-10-11

Status: REVIEW_LOCAL_DONE / REMEDIATION_NOT_STARTED for this reassessment.
Audited tracked head: `603d5ba8acd32da6b5b0de72988ac614e8c18ee6` on maintained
`dev`. Existing uncommitted planning documents were read separately. This is
an independent source review with bounded local failure experiments, not a
deployment, penetration test of production, release certification or exhaustive
proof of safety. The user requested evaluation and a complete plan before
deciding who implements corrections.

## Executive verdict

The delivered work improves the system, but neither security remediation nor
CS15–CS25 is fully accepted. The tracked history and current progress show
CS15-A inventory and CS15-B1 decisions only; CS15-B2/C and implementation of
CS16–CS25 remain pending. Existing outbox/cache improvements are useful partial
dependencies, not evidence that the offline program was completed.

Fourteen actionable review items follow. Five have HIGH severity, including two
conditional blockers for the currently inactive training pipeline; eight are
MEDIUM risks, and one is a functional integration blocker. No CRITICAL exploit
was demonstrated. Severity describes potential impact and prerequisites, not
an observed production incident or a CVSS score. Direct-storage configuration
and live edge configuration were not independently verified.

Priority: enforce actual ingress limits and non-refundable admission budgets;
make erasure recover independently of revoked credentials; fix training
revocation/commit/content guarantees before activation; then complete deadline,
Android transport/ownership, synchronization and source-parser controls. Finish
offline capabilities and their acceptance matrix only on those foundations.

## Verification and positive results

The immutable snapshot passed 71 Go packages with `-race`, eight real
PostGIS adapter/HTTP packages with `-race`, and 94 selected Android/JVM tests
across two disjoint groups. Ten added Go unit cases, three real-SQL cases and
five added Kotlin cases characterized unsafe behavior. A separate bounded
32 MiB child JVM reproduced parser heap exhaustion from a 172,161-byte
synthetic compressed XLSX. Characterization PASS means the defect exists; it
does not mean a safety gate passed. Full commands, source and output are in
the [evidence pack](evidence/reassessment-2026-10-11/README.md).

Confirmed improvements include atomic concurrent media reservation, smaller
bounded SQL dispatch reads and owner/origin/lease controls, refresh-token
issued-history checks, independent issuer JWKS expiry and redirect refusal,
privacy accounting for overdue original media, Android database backup
exclusion, and safer XLSX coordinate validation. Existing tests exercise useful
auth, SQL, moderation and helpfulness invariants. Their passing results do not
cover all cross-boundary failure paths identified below.

No runtime corrections, new dependency scans, VPS/phone tests, remote delivery
or existing soak modifications were performed. The unique local test database
was removed. SEC05-E and CS10C wall-clock proofs and SEC08-D phone checks remain
owed in recorded progress; future due dates or fake-clock tests cannot close them.

## Findings and required corrections

### REV-01 — HIGH: declared upload bytes do not bound actual direct ingress

Source: [presign.go](../../backend/internal/modules/evidence/adapters/storage/presign.go),
[SEC03-B](../backend/evidence/community-safety/SEC03B_TRANSFER_BOUNDARY.md),
[SEC03-C](../backend/evidence/community-safety/SEC03C_PROVIDER_MATRIX.md).
The direct PUT signature covers content type and host, not an enforced maximum
body. `MaxBytes` is metadata; `UNSIGNED-PAYLOAD` permits different bytes. The
provider experiment already records max+1 transport acceptance and later
worker rejection. Atomic reservation caps declared inventory, not actual
ingress or replayed PUT traffic. Proxy-mode body limits improve that mode but
do not fix a deployment exposing direct presigned storage PUT.

Impact: storage/transfer exhaustion before verification when direct PUT is
enabled. Fix the actual ingress boundary, reconcile actual bytes against durable
reservations, and prove oversize never becomes an accepted object on each
supported provider/mode. Do not assume R2 supports an S3 POST policy. Retain
the existing bounded reader and namespace breakers as additional controls.

### REV-02 — HIGH: deleting credentials refunds the hourly signup budget

Source: [accounts.sql](../../backend/db/queries/account/accounts.sql),
[account pgstore](../../backend/internal/modules/account/adapters/pgstore.go).
`CountKeyAccountsSince` counts credential rows; `DeleteAccountKeys` removes
those same rows. The advisory lock serializes admission but cannot preserve
deleted accounting. Unit and real-SQL probes created/deleted 101 accounts at
one fixed instant despite a 100/hour cap; retained account audit rows grew while
the counted budget returned to zero. This requires only creation and deletion
of the attacker's own accounts, not another user's credentials.

Fix: durable admission accounting independent of account inventory/deletion,
with bounded retention and simultaneous-create/delete failure tests. Account,
session, intent and job growth must remain constrained across repeated cycles.

### REV-03 — HIGH: erasure can become unrecoverable after committing revocation

Source: [DeleteAccount](../../backend/internal/modules/account/application/status.go),
[account HTTP](../../backend/internal/modules/account/adapters/http/http.go),
[API orchestration](../../backend/cmd/api/main.go),
[Erase](../../backend/internal/modules/privacy/application/erase.go),
[erasure job](../../backend/internal/modules/privacy/adapters/jobs/erasure.go).
Account deletion commits revocation/unlinking and contributor intents before
calling external cleanup orchestration. A callback failure leaves the next
HTTP attempt unable to pass mandatory access validation. Direct application
retry tests bypass this route boundary. No automatic consumer of account
erasure intents was found; accounts without contributor bindings also lack an
account-wide durable orchestration record for feedback cleanup.

A second failure boundary remains inside privacy jobs: a scope error marks the
request FAILED; the queue retry skips non-REQUESTED requests, including FAILED,
and returns success without resuming. Both behaviors were reproduced with
bounded seams; no real user's deletion was attempted.

Fix: atomically persist account-wide work that a worker can resume without the
deleted session. Track scope progress and distinguish failed/resumable from
completed. Verify actual HTTP deletion, crash/restart and job replay through
all cleanup scopes. Never restore deleted authentication just to retry cleanup.

### REV-04 — HIGH, conditional: training revocation does not fence promotion

Source: [consent.go](../../backend/internal/modules/privacy/application/consent.go),
[promote.go](../../backend/internal/modules/privacy/application/promote.go),
[promotion store](../../backend/internal/modules/privacy/adapters/promotion_store.go).
Revocation stores REVOKED and returns a deletion signal, but its API path does
not invoke owner inventory revocation or durable purge enqueue. Existing ACTIVE
derivatives therefore never enter `ListRevokedBefore`. Unit and real-SQL probes
left an ACTIVE derivative absent from purge inventory even at +72h.

Promotion also reads consent before sanitization and inserts inventory without
an atomic revision/state fence. A revocation committed during sanitization
still allowed an older ACTIVE promotion, reproduced with real SQL.

The current producer/source/UI path is incomplete and collection remains OFF;
these are activation blockers, not evidence of live image collection. Fix with
an atomic consent/promotion guard, durable revoke-to-purge transition and a
fence/reconciliation strategy spanning object writes. Test revoke before,
during and after inventory/object commits and restored state.

### REV-05 — MEDIUM, conditional: promotion replays can lose or orphan bytes

Source: [promotion job](../../backend/internal/modules/privacy/adapters/jobs/promote.go).
Inventory commits before object output. On output failure the next proposal
replays, and the handler returns success without retrying the missing write.
The object key uses the queued consent revision while inventory uses the
proposal's current revision; an older queued revision can put bytes under a
different key from the sweeper's inventory. Both were reproduced at job seams.

Fix: persist one authoritative object key/revision and a pending/ready storage
state. Replays must reconcile unfinished output; selection requires committed
bytes and current consent. Verify provider timeout-after-write and revocation
between DB/object steps before enabling this producer.

### REV-06 — HIGH, conditional: sanitizer PASS does not mean content minimization

Source: [promotion_sanitize.go](../../backend/internal/modules/privacy/adapters/promotion_sanitize.go),
[media.go](../../backend/internal/modules/evidence/adapters/media/media.go).
Generic decode/re-encode strips metadata but preserves the entire frame. The
adapter labels success PASS without a price-only crop or removal of identifying
scene content. A synthetic 100×50 two-region image remained 100×50 with the
non-price region intact. No face, plate or production photograph was used.

Fix: keep collection OFF until actual minimization is implemented and qualified,
or explicitly retain only structural sanitization without claiming training
privacy approval. Uncertain content must fail closed. Terms and retention rules
must match what the pipeline actually preserves. Do not activate merely by
wiring the currently missing source loader.

### REV-07 — MEDIUM: cleanup scheduling and health do not prove the 24h deadline

Source: [worker schedules](../../backend/cmd/worker/main.go),
[evidence sweep](../../backend/internal/modules/evidence/application/sweep.go),
[training sweep job](../../backend/internal/modules/privacy/adapters/jobs/sweep.go),
[promotion store](../../backend/internal/modules/privacy/adapters/promotion_store.go),
[SEC05-E](../backend/evidence/community-safety/SEC05E_EVERY_COPY_SOAK.md).
Hourly evidence cleanup selects already expired records: up to almost one hour
plus backlog after the deadline is possible. Daily training cleanup selects
revocations older than 24h: almost 48h total is possible before backlog. This is
not a scheduling margin before expiry. The new fake-clock experiment sweeps at
+25h; the wall-clock verifier observes overdue state then manually sweeps. Those
prove eventual cleanup, not automatic deletion within 24h.

The training job discards `SweepReport.Failures` and reports success, reproduced
by a probe. Evidence post-pass audit errors degrade to zero overdue rather than
unknown health. Training inventory scans have no SQL LIMIT and retain revoked
rows without a purged acknowledgement, repeatedly revisiting growing history.

Fix: deadline-aware early dispatch, bounded due scans, retryable per-copy progress,
auditable physical completion, and failure/unknown health propagation. Keep
restore-filter tombstones while excluding purged rows from active work. Prove
autonomous scheduling, backlog and provider failures over actual wall time.
Powered-off device limitations must remain explicit; they cannot be promised away.

### REV-08 — MEDIUM: local consent state is not bound to owner or server revision

Source: [TrainingConsentUseCase](../../application/src/main/kotlin/com/anpfuel/application/usecase/privacy/TrainingConsentUseCase.kt),
[TrainingConsentStateStore](../../data/src/main/kotlin/com/anpfuel/data/local/preferences/TrainingConsentStateStore.kt).
Preferences store one global toggle, revision and pending revoke, without
account/origin identity. The use case checks account presence but does not bind
the stored intent to it. A JVM probe queued owner A's OFF then synced under B's
gateway, applying it to B and clearing A's pending state. This is a seam-level
cross-account demonstration, not two live authenticated account requests.

Server revision 100 does not seed the local counter; the next command can use
revision 1 and remain stale. OFF during an online transport failure does not
durably queue the conservative local intent; pending OFF can also conflict with
a later grant. Pending sync has no complete durable background recovery path.

Fix: owner/origin-bound durable intents, stable command identity and explicit
server revision conflict handling. OFF takes local effect immediately even on
an ambiguous connection; ON requires confirmed current terms/receipt. Cancel
old-owner work on switching, without silently dropping its pending erasure.

### REV-09 — FUNCTIONAL BLOCKER: panels, signed transport and collection producer are incomplete

Source: [consent client](../../data/src/main/kotlin/com/anpfuel/data/remote/TrainingConsentHttpClient.kt),
[helpfulness client](../../data/src/main/kotlin/com/anpfuel/data/remote/CommunityHelpfulnessHttpClient.kt),
[app source](../../app/src/main/kotlin/com/anpfuel/app), [worker](../../backend/cmd/worker/main.go).
`TrainingConsentPanel`, `CommunityHelpfulnessPanel` and `TrainingPhotoOptInRow`
have definitions but no production screen invocations. DI supplies plain HTTP
clients; new calls do not carry the proof/session transport required by the
backend. The added request characterization confirms missing auth/proof fields;
real backend auth tests correctly refuse guests. A MockWebServer returning 200
without verifying proof does not establish Android-to-backend integration.

The default retry interceptor can resend these writes on 429/503 despite the
client's no-retry description; eventual signed transport needs fresh nonce per
actual attempt and stable operation identity. The training source loader always
returns “quarantine source not configured”; no complete producer was found.

Fix authenticated transport and retry ownership first, then guarded navigation
placement with actual HTTP/UI acceptance. Keep training collection OFF until
REV-04–08 are closed. The current Portuguese terms file is untracked/provisional;
controller/contact details and activation wording require explicit completion.

### REV-10 — MEDIUM: bounded outbox reads do not enforce a complete retry/storage budget

Source: [RoomContributionOutboxRepository](../../data/src/main/kotlin/com/anpfuel/data/repository/RoomContributionOutboxRepository.kt),
[DispatchContributionUseCase](../../application/src/main/kotlin/com/anpfuel/application/usecase/contribution/DispatchContributionUseCase.kt),
[outbox DAO](../../data/src/main/kotlin/com/anpfuel/data/local/dao/ContributionOutboxDao.kt).
`failDispatch` applies a finite Retry-After after the hard attempt budget has
parked a command at MAX. A JVM probe at 25 attempts makes it due again. The
domain comparison also checks the old count, allowing an extra attempt at the
boundary. Requested-ID parked FAILED work can keep returning RETRY; transient
domain failures can instead return REFUSED after saving backoff, ending the
worker without guaranteed recovery. Permanent-refusal updates lack the same
claim-nonce fencing as transient failure updates; add a concurrency regression
before declaring this source-level concern closed.

Live queue bytes/count and wake creation are not fully capped. Terminal
retention does not cover permanently parked FAILED history, which can grow
indefinitely. Counting-DAO timing is not a real SQLite deep-page benchmark.

Fix atomic admission budgets, a precise terminal state machine, claim fencing,
hard attempt limits that Retry-After cannot undo, and bounded wake/history work.
Keep existing owner/origin/revision/idempotency protections. Verify real Room
plans/migrations and worker outcomes, not only fake DAO returned-page sizes.

### REV-11 — MEDIUM, conditional input: redirect contact and XLSX aggregate memory remain unsafe

Source: [AnpFileDownloader](../../data/src/main/kotlin/com/anpfuel/data/remote/AnpFileDownloader.kt),
[StreamingXlsxParser](../../data/src/main/kotlin/com/anpfuel/data/parser/StreamingXlsxParser.kt).
The default HTTP client follows redirects before the final file URL is checked;
the listing path lacks that final check. A loopback characterization confirmed
one contact with a disallowed redirected host before refusal. Fix by validating
each hop before I/O, including listing and intermediate hops; no forbidden
request should occur. This requires a redirect from an allowed upstream, not
an arbitrary untrusted URL accepted by the initial allowlist.

Individual entry/string/count limits still permit a shared-string table larger
than the device budget. The child-heap reproduction demonstrates aggregate
retention exhaustion despite allowed individual strings and compressed size.
Fix cumulative retained/inflated-memory/time budgets or a bounded representation;
test compressible adversarial inputs and real supported-device memory. Do not
interpret a 32 MiB child limit as the actual Android heap size.

### REV-12 — MEDIUM, deployment/operation: retention and quota configuration remain incomplete

Source: [account store](../../backend/internal/modules/account/adapters/pgstore.go),
[API](../../backend/cmd/api/main.go), [worker](../../backend/cmd/worker/main.go).
Expired code/family purge methods exist but no runtime caller was found; their
DELETEs are not bounded. A live-session limit does not cap repeated issue/revoke
cycles or lifetime retained records. Admission and erasure-history growth need
bounded lifecycle handling, not just current-row caps.

Proxy identity parsing improved, but empty default trusted-proxy configuration
and commented private CIDR examples do not prove the actual deployment resolves
clients safely/fairly. Default global thresholds disable the breaker, and a
configured write-operation value is not a universally wired write gate. No live
deployment was inspected in this reassessment; these are conditional gaps, not
a claim that empty trust authorizes forged headers.

Fix bounded worker retention and independent issuance/admission accounting;
document and verify actual narrow proxy trust/network exposure and effective
operation gates inside Abastevo's authorized boundary. Shared edge changes need
their own scope, never an inferred authorization.

### REV-13 — MEDIUM residual: self-reported GPS is not independent presence proof

Source: [SEC08-A](../backend/evidence/community-safety/SEC08A_LOCATION_ASSURANCE.md),
[SEC08-B](../backend/evidence/community-safety/SEC08B_ADVERSARIAL_CLIENT.md),
[SEC08-D](../backend/evidence/community-safety/SEC08D_PHONE_MATRIX.md).
Consistently fabricated source flags/coordinates can still become VERIFIED/NEAR;
the agent's own adversarial test demonstrates this. Geometry/freshness checks
reject inconsistent submissions, but cannot authenticate an OS claim supplied
by a modified client. This does not demonstrate that one forged contribution
alone achieves HIGH consensus: other account/photo/supporter requirements remain.

Fix assurance language/algorithm boundaries or explicitly accept the versioned
residual risk. Declining attestation is not inherently wrong and attestation
does not itself prove GPS. Complete genuine/mock/offline phone cases on staging;
never describe simulated geometry as proof of production presence.

### REV-14 — MEDIUM: unknown OIDC key IDs amplify upstream refreshes

Source: [oidc.go](../../backend/internal/modules/account/adapters/oidc/oidc.go).
An unknown `kid` bypasses fresh cache TTL before signature verification. There
is no issuer refresh cooldown/negative-cache admission; a shared cache mutex
covers network fetch and can serialize unrelated issuers. Twenty invalid-token
verifications caused 21 source fetches within one fresh TTL in a local stub.
No external provider was flooded. The actual link route requires an active
free account session, not a valid provider credential; it is not fully anonymous.

Fix bounded per-issuer refresh admission/single-flight and bounded negative
caching plus route budgets, while preserving legitimate rotation recovery and
cross-issuer isolation. Test parallel fake kids, outages and valid rotated keys.

## Closure of the previous twelve findings

Assessment of [the original audit](SECURITY_AUDIT_2026-10-11.md), based on code
and tests rather than microtask completion labels:

- AUD-01: partial. Bounded/off-by-default mail path and some cardinality controls
  improve admission; REV-02/12 retain growth/bypass gaps.
- AUD-02: open in direct-upload mode; REV-01.
- AUD-03: identity code improved; actual proxy/edge/global-gate evidence still
  required; REV-12. No shared proxy changes were authorized here.
- AUD-04: original reservation race corrected at source, including concurrent
  integration coverage. This does not enforce actual ingress bytes.
- AUD-05: partial. Durable contributor intents exist, but route/job recovery
  remains incomplete; REV-03.
- AUD-06: partial. Originals now participate in audit/breaker, but autonomous
  deadline and failure reporting remain incomplete; REV-07.
- AUD-07: partial. Temporary siblings, expiry checks and backup exclusions
  improved; full clock/reboot/restore/device acceptance belongs to CS22/24.
- AUD-08: original arbitrary-token family revocation corrected using issued
  refresh history. Selected Android serialization tests pass; retain full
  affected integration when changing the lifecycle again.
- AUD-09: original shared-issuer TTL/redirect defects corrected; REV-14 is a
  separate refresh-admission gap introduced/exposed by unknown-key recovery.
- AUD-10: partial. Coordinate allocation fixed; REV-11 retains redirect and
  aggregate-memory gaps.
- AUD-11: partial. Point lookups/SQL batches improved; REV-10 retains lifecycle,
  storage and retry-budget gaps.
- AUD-12: residual risk remains; source evidence records fabricated VERIFIED/NEAR.
  Policy documentation alone is not independent GPS assurance; REV-13.

## CS15–CS25: actual implementation status

The [canonical execution plan](../planning/COMMUNITY_SAFETY_EXECUTION_PLAN.md)
itself calls these phases PLANNED. No CS16–CS25 acceptance evidence was found in
the audited tracked head.

- **CS15 — PARTIAL, planning only:** A inventory/B1 decisions recorded. B2
  capability decisions and C executable manifests remain absent.
- **CS16 — NOT IMPLEMENTED:** existing price-group reads use HTTP before Room
  fallback; city feed remains HTTP. Persistent observable cache-first community
  generations and restart acceptance are missing.
- **CS17 — NOT IMPLEMENTED:** no complete city pack manifests, atomic bounded
  download or all-order local indexed keyset pagination.
- **CS18 — NOT IMPLEMENTED:** no complete cache revision/tombstone/reset and
  atomic generation-revalidation contract.
- **CS19 — NOT IMPLEMENTED:** price outbox exists, but complete durable intents
  for eligible comments/votes/reports/privacy actions do not.
- **CS20 — NOT IMPLEMENTED:** online short-lived capture authorization remains;
  no implemented separate offline permit/consumption contract. Offline drafts
  cannot be claimed as accepted trusted evidence.
- **CS21 — PARTIAL DEPENDENCIES:** SEC06 bounds and leases are reusable;
  REV-10 and the complete cross-operation worker/failure matrix remain.
- **CS22 — PARTIAL DEPENDENCIES:** backup exclusion/cache cleanup help, but owner
  consent, restored-state, monotonic lifetime and actual deadline gates remain.
- **CS23 — NOT IMPLEMENTED:** new panels are unplaced; all-route offline states,
  coverage/freshness/accessibility and correct receipts are incomplete.
- **CS24 — NOT IMPLEMENTED:** no complete executable offline matrix, real Room
  resource campaign, wall-time and device lifecycle qualification.
- **CS25 — NOT IMPLEMENTED:** no full device journey, independent zero/one-effect
  reconciliation or accepted reviewer pack. CS13/14 staging/device work is also
  still owed and must follow the prerequisites.

## Evidence and delivery integrity gaps

Canonical execution/remediation plans, terms and the prior audit remain
untracked while delivered phase evidence links to them. A fresh clone/PR cannot
resolve those links. Preserve ownership and reconcile intended documents in the
next authorized implementation batch; do not silently publish another effort's
dirty files.

SEC09-B's “dependency/container scans” record proves Go/Rust advisory runs and
pin/diff inspection, not resolved Android transitive-dependency or container
advisory scans/SBOM. Image digests and empty dependency diffs are useful but do
not substitute for vulnerability analysis. The next validation pack must label
unrun scan categories OWED and record tool/advisory timestamps and exceptions.

Recorded PR checks/merge/wiki state was not refreshed remotely here. No release
claim follows from local tests. The [complete correction plan](../planning/SECURITY_OFFLINE_CORRECTION_PLAN_2026-10-11.md)
maps every review item and missing phase to bounded tasks and acceptance gates.

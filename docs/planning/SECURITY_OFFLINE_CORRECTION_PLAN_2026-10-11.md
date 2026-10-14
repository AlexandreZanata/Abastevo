# Independent security/offline correction plan — 2026-10-11

Current initial-policy amendment: [ADR-021](../adr/021-community-first-contribution.md) / PRM15–PRM21 supersedes mandatory 150m/GPS contribution admission and proximity-based price confidence. Identity, media, privacy and source-integrity checks remain. Historical location evidence and optional nearby geometry retain their original scope; future mandatory proximity is PRM22 DEFERRED.

Status: AUTHORIZED / EXECUTION_IN_PROGRESS (user instruction, 2026-10-11).
The original review checkpoint below remains historical; the subsequent user
request authorizes all corrections, GitHub delivery and protected merge.
Baseline: `603d5ba8acd32da6b5b0de72988ac614e8c18ee6`.
Input: [independent reassessment](../security/SECURITY_OFFLINE_REASSESSMENT_2026-10-11.md)
and [reproduction pack](../security/evidence/reassessment-2026-10-11/README.md).
The user will decide whether this agent or another implements the work. No
runtime correction, deployment, remote publication or release is part of this
planning checkpoint. Preserve existing SEC00–SEC09 and CS00–CS25 IDs/history.

## Execution contract for an economical implementation agent

Work on one bounded microtask at a time, in dependency order. At opening read
AGENTS, FAST_EXECUTION, current PROGRESS, this task and relevant contracts/code;
inspect actual head/diff before editing. The old baseline is evidence, not an
instruction to reset a newer checkout. Preserve foreign changes; isolate an
occupied checkout. Maintain dev and protected dev→main delivery per ADR-020.

For each task: document affected B-BR/BUC and failure semantics first; turn the
specific characterization into a failing safety regression; implement the
smallest correction; run meaningful targeted negative/failure/concurrency tests
and affected compilation/static checks. SQL/migrations/jobs/privacy/auth need
real integration immediately. Run diff/secret checks, record exact tested SHA,
commands and remaining obligations. Never mark a whole parent phase complete
because one subtask passed. Prefer `.A/.B/.C` commits with coherent acceptance
over loading the entire repository or rerunning aggregate suites after docs.

At every checkpoint distinguish LOCAL_DONE, INTEGRATION_PENDING, INTEGRATED,
PROOF_OWED and RELEASE_CERTIFIED. End-batch CI/merge/wiki follows the workflow;
do not wait/poll CI between construction tasks or weaken failed tests. Do not
enable training while the consent/minimization/storage prerequisites remain.

VPS/staging work is a later explicitly bounded acceptance step: only Abastevo,
never shared edge/other containers, never real DDoS, never unbounded load. Reuse
measured headroom and a unique run namespace; stop on predeclared resource/error
conditions. Lack of an authorized window means PROOF_OWED, not fabricated PASS.

## Correction batch: RVA00–RVA14

These are new review work IDs, not replacements for historical SEC completion
records. Dependencies refer to local tested checkpoints unless explicitly
requiring device/provider/wall-time proof.

### RVA00 — Reconcile baseline, acceptance and owned documents

- RVA00.A: compare current commits/diff/PROGRESS against this baseline. Preserve
  foreign files and the two existing soaks. Freeze the REV-01–14 register with
  prerequisites, controls, metrics, tests and residual risks. Label CS15 partial
  and CS16–25 not accepted; do not rewrite historical evidence as if it had passed.
- RVA00.B: reconcile ownership of untracked execution/remediation plans, audit,
  terms and modified training plan. Prepare the intended canonical documentation
  for the authorized implementation batch; verify fresh-clone links. Do not
  stage everything or publish somebody else's pending content indiscriminately.
- RVA00.C: record actual protected-delivery and scan/soak/device obligations.
  Repair misleading claims with an addendum; keep source records intact.

Exit: one source-of-truth register, exact current SHA, executable next task and
no false completion/publication claim. This gate precedes all corrections.

### RVA01 — Bound actual photo ingress and reservation reconciliation

Maps REV-01 / AUD-02; extends SEC03. HIGH, immediate backend priority.

- RVA01.A: freeze ingress mode/provider matrix and actual-byte contract. Choose
  a supported enforced transfer boundary after evaluating provider capability;
  do not assume direct PUT signing enforces a maximum. Cover repeated upload,
  unused/expired reservations, finalization and admission during degraded health.
- RVA01.B: implement the selected bound at ingress, with streaming memory cap,
  timeouts, atomic durable budget accounting and deterministic abandoned-partial
  cleanup. Count actual stored/received bytes; preserve private ownership and
  existing verification. Bound maximum reservation expiry and replay effects.
- RVA01.C: real local private S3/HTTP tests for max−1/max/max+1, wrong declared
  length, chunked/missing length, interrupted transfer, replay/concurrent PUT,
  storage failure and expiry. Run the real supported-provider matrix only in
  the authorized staging window; record transport rejection and absence of
  accepted oversize object separately from later worker rejection.

Exit: no oversized accepted object in supported modes, bounded process memory
and ingress/storage envelope, no duplicate budget consumption, honest provider
limitations. Direct-upload mode remains blocked until its row passes.

### RVA02 — Make admission accounting survive deletion

Maps REV-02 and admission part of REV-12; extends SEC01/SEC02. HIGH.

- RVA02.A: freeze the global/key-account admission event lifetime, pseudonymous
  budget subjects, NAT fairness, deletion interaction and bounded ledger retention.
  No invasive fingerprint or indefinite personal-data ledger.
- RVA02.B: use atomic durable admission events independent of credential rows.
  Applied schema changes are append-only. Bound issue/revoke cycling and
  resulting account/session/intent/job growth; distinguish successful admission
  budget from current inventory and retryable transaction failure.
- RVA02.C: real SQL 101 create/delete at one instant must refuse beyond budget;
  simultaneous create/delete, process restart, transaction rollback and expiry
  must not refund consumed capacity or permanently exhaust honest capacity.
  Prove new-migration upgrade/recovery and limits under low connection counts.

Exit: the reproduced bypass closes with bounded durable growth and truthful
retry semantics; existing account deletion privacy guarantees remain intact.

### RVA03 — Recover account and privacy erasure without live credentials

Maps REV-03 / AUD-05; extends SEC04. HIGH, prerequisite for privacy UI.

- RVA03.A: define account-wide durable erasure intent, per-scope progress,
  retryable/terminal states and a receipt that proves request acceptance rather
  than already-complete physical deletion. Include accounts without contributors.
- RVA03.B: atomically commit revocation/unlinking and recoverable work. Implement
  a bounded worker/reconciler independent of the revoked session. Make each
  scope idempotent and resumable; do not skip FAILED work as completed.
- RVA03.C: actual HTTP deletion with failure after DB commit, worker restart,
  each scope failure, feedback outage, timeout-after-effect, duplicate workers,
  no-contributor account and repeated request. Verify all expected data/media
  cleanup and preserved minimal audit; no authentication resurrection.

Exit: a client can disappear immediately after accepted deletion and all scopes
still converge. Failed scopes remain observable and retryable, with no success
receipt claiming unfinished cleanup complete.

### RVA04 — Atomically fence training consent and revocation

Maps REV-04; extends CS09/CS10. HIGH conditional activation blocker.

- RVA04.A: document consent revision, per-image eligibility, future-only grant,
  revoke cutoff, inventory/selection state and concurrent object-write behavior.
- RVA04.B: one transactional consent/inventory guard and durable owner revoke
  work. Ensure every active derivative becomes unselectable on revoke; reconcile
  late object writes without resurrecting inventory. Preserve restore tombstones.
- RVA04.C: real SQL concurrent grants/revokes/proposals; revoke before/during
  sanitizer and DB commit; lost response/replay and rollback. Pause object output,
  revoke, resume output and prove no selectable residual bytes escape cleanup.

Exit: both real-SQL review characterizations become safety regressions; no stale
promotion commits/selects after revocation, and every retained copy is tracked.

### RVA05 — Make promotion inventory and objects crash-consistent

Maps REV-05; depends RVA04; extends CS10.

- RVA05.A: define authoritative image/owner/hash/consent-revision/object-key
  identity and pending/ready/purging/purged states with bounded retries.
- RVA05.B: persist the actual key and committed revision once; reconcile output
  on replay rather than treating inventory replay as storage success. Validate
  source ownership and revision at the producer/consumer boundary.
- RVA05.C: first write failure then retry, timeout after object write, queued
  old revision, wrong hash/owner, interrupted inventory update, duplicate worker
  and revoke races. Use real private S3/SQL for cross-resource acceptance.

Exit: READY means tracked bytes exist and remain eligible; no mismatched key,
silent lost output or untracked orphan after bounded recovery.

### RVA06 — Implement and qualify real training-content minimization

Maps REV-06; HIGH conditional activation blocker; extends CS10.

- RVA06.A: freeze what approved training content may contain, uncertain-content
  refusal, allowed image crop/metadata and synthetic acceptance corpus. Structural
  JPEG sanitization must not be represented as PII/content anonymization.
- RVA06.B: implement the smallest verifiable minimization path or keep training
  collection OFF and record this feature as pending. Any dependency needs
  recorded necessity/license/security/resource review before adding it.
- RVA06.C: synthetic price/non-price regions, identifiable-content stand-ins,
  uncertain layouts, EXIF, decoder attacks, boundary sizes and resource limits.
  Demonstrate preserved price utility and excluded prohibited regions; uncertain
  results must not reach READY/selection. No real photos/PII in fixtures/logs/Git.

Exit: documented content guarantee is demonstrated, not inferred from re-encode.
No producer/source activation until RVA04–08 and privacy terms gates pass.

### RVA07 — Enforce deletion deadlines, bounded purge and truthful health

Maps REV-07 / AUD-06; depends relevant RVA03–05 paths; extends SEC05/CS10C.

- RVA07.A: define each copy's immutable deadline anchor and operational margin,
  DB/object/local/temporary/backup scope, allowed tombstone metadata, power-off
  limits, queue backlog and health UNKNOWN semantics. Do not redefine “24h” as
  “24h plus the scheduler interval.”
- RVA07.B: deadline-aware dispatch before cutoff; bounded due scans and per-copy
  acknowledgement/retry, excluding purged work from hot scans. Propagate failures
  and audit outages to jobs/health/metrics. Keep uploads gated when deletion
  capacity or observability is unsafe.
- RVA07.C: injected-clock tests at both sides of deadline, queue delay, backlog,
  provider partial outage, slow deletes, crash and restore. Add SQL LIMIT/plan
  assertions at large inventory sizes. Then actual autonomous wall-time soak
  covering originals/finals/training/temp copies and scheduler phases: no manual
  post-deadline sweep may be counted as within-deadline compliance.

Exit: receipt/bytes reconciliation and measured worst-case schedule/backlog fit
the stated deadline under the declared envelope. Existing soaks remain evidence
for what they tested; collect a new qualifying run without falsifying them.

### RVA08 — Owner-bound durable consent and signed Android transport

Maps REV-08 and transport part of REV-09; depends backend RVA03–07.

- RVA08.A: owner+origin-bound consent intents, stable command identity, revision
  conflict/current-terms protocol and conservative OFF semantics. Explicitly
  cover multiple devices, account switch, logout/delete, absent consent and
  terms rotation; never automatically grant after reading newer server state.
- RVA08.B: immediate durable local OFF on offline/ambiguous failure; bounded
  background resumption under its original owner, serialized per owner. Seed/
  reconcile revision safely and retain old-owner revocations without sending
  them under a new account. Migrate or conservatively invalidate global prefs.
- RVA08.C: share the established proof/session transport for consent/helpfulness
  with no anonymous fallback. One retry owner; fresh signed nonce per actual
  HTTP attempt, stable command/body. Test actual backend verifier, not a server
  that accepts any headers; private reads must also enforce ownership.
- RVA08.D: A→B switch before/during I/O, server revision 100, simultaneous devices,
  stale grant after revoke, OFF timeout after commit, expired auth, 429/503 and
  process death. Verify exact original owner's server effect and no silent ON.

Exit: local privacy decisions are durable/owner-safe and actual signed HTTP
interoperates with the backend. No UI activation based only on mocked 200s.

### RVA09 — Bound OIDC unknown-key recovery

Maps REV-14; extends SEC07-B; independent after RVA00, run early with availability fixes.

- RVA09.A: freeze issuer cooldown, bounded negative-key cache/admission and route
  budget; retain legitimate rotation recovery and explicit unavailable/refused.
- RVA09.B: per-issuer single-flight/bounded refresh, no shared network-held mutex
  blocking other issuers. Bound arbitrary kid cardinality and repeated invalid
  token work even for authenticated free accounts.
- RVA09.C: invalid same/unique kids, concurrent requests, fresh TTL, issuer outage,
  valid rotated key and two providers. Assert bounded provider fetches and
  independent latency, not merely eventual auth rejection.

Exit: the 20-token/21-fetch probe is bounded by policy while legitimate rotation
still succeeds. Do not reintroduce original TTL/redirect defects.

### RVA10 — Complete outbox resource, attempt and worker lifecycle bounds

Maps REV-10 / AUD-11; extends SEC06 and CS21 prerequisites.

- RVA10.A: precise pending/claimed/transient/permanent/exhausted/acknowledged
  states; maximum actual sends, Retry-After interaction, lease fencing, retained
  history lifetime, queue bytes/count and per-wake/per-owner fairness budgets.
- RVA10.B: hard stop at the intended attempt count, never overridden by server
  hints. Nonce-fence every claim completion/refusal. Return terminal versus
  retry outcomes consistent with WorkManager, and schedule bounded recovery
  whenever transient work remains. Cap atomic enqueue and work-request creation.
- RVA10.C: exact 24/25/26 boundaries, Retry-After, requested parked ID, stale
  worker refusal after lease takeover, 429/503, connection loss after receipt,
  account switching, disk-full/large history and crash at each state transition.
- RVA10.D: real Room migration/EXPLAIN and deep-history/due-selection benchmarks
  at declared cardinalities; preserve unsent work when trimming terminal history.
  Counting fakes establish query shape only, not production SQLite latency.

Exit: bounded stored/live work and CPU/network wakes, no exhausted command
reactivation, no transient work stranded or acknowledged operation duplicated.

### RVA11 — Validate redirects before contact and bound aggregate parser memory

Maps REV-11 / AUD-10; extends SEC06-A/B.

- RVA11.A: explicit redirect policy for listings/files; validate each destination
  before request, exact approved origin/path and maximum hops. Preserve capped
  streaming copy/atomic publication; block forbidden intermediate hops as well.
- RVA11.B: cumulative retained-string/inflated/time/row ceilings with a declared
  device budget, deterministic refusal and resource closure. Avoid an OOM-catching
  substitute for admission; use bounded representation/spill only if justified.
- RVA11.C: redirected foreign host sees zero requests; test intermediate/final
  redirects, listing path, HTTPS downgrade and loops. In child JVMs reproduce
  high-compression shared strings and worksheet expansion; prove bounded memory
  and honest large-source compatibility. Measure supported-device memory later.

Exit: no pre-refusal foreign contact and deterministic resource-budget refusal
before allocation exhaustion, with unchanged official-source usefulness.

### RVA12 — Operational retention, proxy identity and effective quota wiring

Maps residual REV-12; depends RVA02.

- RVA12.A: bounded expiry jobs for codes/families/admission/history, purpose-driven
  retention and progress/age metrics. Wire real runtime consumers, not just
  methods called by tests; preserve refresh-reuse/erasure tombstone requirements.
- RVA12.B: freeze and test effective per-operation/global admission configuration,
  narrow trusted peers and expected IPv4/IPv6/NAT attribution. Confirm direct
  network access cannot bypass the intended Abastevo entrypoint.
- RVA12.C: real SQL backlog/locks/restarts and lifetime create/revoke growth;
  config validation for defaults and missing required limits; spoofed forwarded
  headers/trusted versus untrusted peers. Inspect actual Abastevo deployment only
  in the permitted window; no shared Caddy/container modification inferred.

Exit: bounded operational growth and evidence of actual configured limits/client
identity. Unverified deployment entries remain OWED, not source-code PASS.

### RVA13 — Reconcile GPS assurance and guarded device acceptance

Maps REV-13 / AUD-12; extends SEC08, depends offline CS20 for its capture cases.

- RVA13.A: record the accepted assurance threat model/version. Either constrain
  self-reported presence's algorithmic benefit or explicitly accept the residual
  with accurate labels and user messaging. Document why key integrity, geometry,
  platform attestation and physical presence are different claims.
- RVA13.B: adversarial consistent/inconsistent native claims, station coordinate
  uncertainty, age/coarse/mock flags, multiple supporters and consensus paths.
  Keep helpfulness separate from price truth; don't let a vote buy trust.
- RVA13.C: final CS13/14 staging phone cases for genuine near/far, FakeGPS,
  coarse/missing signal, offline/reconnect and TEST_ONLY exclusion. Preserve real
  data and use only approved test subjects; never use fake GPS in real production
  evidence. No claims of universal GPS fraud detection.

Exit: a versioned documented residual or demonstrated improved control, with
device results and remaining obligations explicit. FakeGPS alone never certifies release.

### RVA14 — Finish functional integration, terms and independent validation

Maps remaining REV-09 and evidence gaps; depends RVA01–13 as applicable.

- RVA14.A: finalize matching Portuguese terms, actual controller/contact, rights,
  optional future-only training grant, revoke/deletion timeline, local/offline
  limitations and version/digest. Terms review must not invent legal compliance
  certification. Add screens and public terms routes only after backend contracts.
- RVA14.B: place helpfulness/settings/per-photo controls in real navigation;
  default OFF, owner/guest states, pending versus server receipt, TalkBack and
  failures. Run actual signed UI→HTTP→DB flows. Enable training producer/source
  only after every privacy/content/provider gate, otherwise retain OFF/OWED.
- RVA14.C: refresh Go/Rust advisory scans and perform resolved Android dependency
  and runtime-container advisory analysis/SBOM with timestamps and justified
  exceptions. Pins/empty diffs are not a scan. Review licenses/new dependencies.
- RVA14.D: current-head independent source/fault review and affected aggregate
  gates once, after offline construction below; record source fixes separately
  from staged/device/wall-time acceptance. No release if critical gates fail.

Exit: real routes and terms agree with enforced behavior; proof matrix and scans
are truthful. This does not skip CS15–25 or automatically authorize deployment.

## Complete offline construction batch — retain CS15–CS25

Source scope remains [the canonical plan](COMMUNITY_SAFETY_EXECUTION_PLAN.md).
Use existing B-BR-CS08–11 / BUC-CS07–10 and refine before behavior. The sequence
is correction foundations → CS15–25 → final CS13/14 device/staging journey.
RVA14 UI/terms work can accompany the relevant CS23 checkpoint; its final review
follows CS25. Do not claim offline server-only operations execute without a server.

### CS15 remaining — executable capability contract

Preserve A/B1 evidence. CS15-B2 decides every route/action's no-network behavior,
owner/auth prerequisites, data lifetime/coverage, local limits and trusted-capture
semantics. CS15-C creates executable scenario IDs/seeds: cold/warm/partial/expired,
guest/owner/switch, network/process/clock faults. Freeze specific device/backend
resource ceilings before coding and record why each is safe; never invent a
supported-user count. Exit: every row has an expected outcome and test, including
first-use data unavailable. Dependencies: RVA00 and safety-policy decisions.

### CS16 — persistent cache-first community reads

A: minimal normalized cache/query-membership schema with owner/public separation,
coverage/freshness and append-only Room upgrades. B: local observable initial
render followed by bounded authenticated/public refresh as appropriate, atomic
published generations; preserve known-good cache on network failure. C: real
Room restart/upgrade/process death, cold empty cache, timeout and account switch.
Exit: saved city/community prices render without a network request and clearly
show age/coverage. Dependencies: CS15; no stale price presented as current.

### CS17 — bounded city packs and local pagination

A: explicit pack download limits for bytes/rows/pages/time/disk, cancellation,
atomic publish and safe eviction. B: indexed local keyset orders/filters, bounded
page size and separate official/community freshness. C: deep pages and ties,
partial/overlap/download failure/disk full/low storage, real query plans and
repeatable timings. Exit: no giant query/unbounded pack and previous complete
generation remains useful after failure. Dependencies: CS16, RVA11.

### CS18 — revisions, revalidation and deletion propagation

A: version/ETag/tombstone/reset contract for edit/hide/delete/expiry and late
pages; choose minimal supported sync protocol. B: generation acceptance and
bounded refresh/revalidation, prevent old responses resurrecting removed data.
C: out-of-order/duplicate responses, old app, clock rollback and server reset.
Exit: offline data converges after reconnection without retaining hidden/private
state as current. Dependencies: CS16/17; backend deletion source remains authoritative.

### CS19 — durable offline social, price and privacy intents

A: bounded owner/origin-bound command schema and explicit guest draft→account
confirmation, no automatic cross-owner attachment. B: implement eligible price,
comment, useful/not-useful vote, report and privacy intents with stable identity,
server rules and conflict/expired outcomes; signed fresh proof on each attempt.
C: partial batches, lost reply, rapid vote changes, logout/delete/account switch
and process death. Exit: each reconnect yields zero/one intended accepted effect
or visible refusal; helpfulness never means “price verified.” Dependencies:
RVA03/08/10, CS15; actual local storage and server budgets both enforced.

### CS20 — offline capture, OCR and deferred evidence

A: accept a versioned offline permit/age/clock/location model and server consume
contract, or explicitly limit offline captures to untrusted local drafts/manual
submission. Existing online short-lived authorization is not an offline permit.
B: bounded camera/crop/OCR/manual local flow, model-unavailable fallback, media
expiry and frozen payload. C: airplane mode/native fix/mock/coarse/missing signal,
permit replay/expired/altered/foreign owner, clock/reboot and server revalidation.
Exit: offline evidence cannot bypass server admission, 150m geometry or trust
policy; no claim of independent physical presence from client flags.
Dependencies: RVA01/07/10/13 policy, CS15/19. Keep unsafe evidence unsent/expired.

### CS21 — complete bounded synchronization and recovery

A: extend RVA10's actual due/owner SQL batches across CS19 commands with fairness,
limits on per-wake bytes/requests/time and single-flight claims. B: fresh proof,
stable command/receipt, bounded exponential jitter/Retry-After and explicit
auth/conflict/expiry states. C: every commit crash boundary, duplicate workers,
lease takeover, reconnect waves, server throttling and parked history.
Exit: no infinite wake/retry loop, starvation, unbounded scan or duplicate effect.
Dependencies: CS18–20 and RVA08/10/12; real SQL/Room/HTTP faults required.

### CS22 — offline privacy, expiry and safe restore

A: audit all private/local copies, custody/backup and owner purges; minimize
retained erasure receipts. B: local OFF/delete/logout/switch with durable minimal
pending intent and server reconciliation, never silent ON after restore.
C: capture+24h, reboot/clock rollback/long power-off/process death/disk full and
restored-state rejection. Exit: expiry and access rules hold inside the stated
device-power limitations; impossible offline remote erasure remains visibly
pending. Dependencies: RVA03–08, CS19–21. Check actual files/DB, not only UI state.

### CS23 — complete offline screens and accurate feedback

A: wire all CS15 rows, cache age/coverage/manual city/model-unavailable entry,
pending privacy and server-only actions. B: distinguish saved here, queued,
received, validating, published, conflict, expired and reauth; never show
“Preço enviado” on local enqueue or “Publicado” on HTTP receipt alone.
C: accessibility/font/rotation/back/process restart/offline cold start and no
infinite spinner; return to Community after the appropriate confirmed state.
Exit: every route has a tested useful outcome and copy matches backend receipts.
Dependencies: RVA14.A/B, CS16–22; real navigation/UI tests, not isolated panels only.

### CS24 — fault, abuse and resource qualification

A: actual Room/PostGIS/private S3/HTTP matrix, altered clients, permissions,
queue/bytes/cardinality overflow, account farming and concurrent reconnect.
B: declared repeated baseline/candidate campaigns measuring cold/warm launch,
first cached render, deep-page plans/rows, queue due latency/depth, heap/disk,
wake/work proxies, network bytes, sync latency/errors and server CPU/RSS/locks/
storage. Record hardware/config, warmup, repeats, denominators and p50/p95/p99;
no API label for direct Reader calls. C: actual autonomous 24h lifecycle plus
injected boundary/restore faults. Exit: measured safe envelope and named residual
risks, no maximum-user extrapolation or “all abuse prevented.” Dependencies:
CS16–23, RVA07/11/12/14.C. Local tests precede any guarded VPS measurements.

### CS25 — independent end-to-end reconciliation and handoff

A: execute all manifest rows on supported Android devices/isolated services,
airplane/backend outage/flapping/force-stop/reboot/reconnect. B: independent
oracle matches each local command to its exact receipt and zero/one server
effect, tally/moderation/public projection/expiry; refusal is not lost success.
C: sanitized reviewer pack with PASS/FAIL/OWED/N/A plus reason and dependencies.
Exit: no required failed/owed row, no private data leakage, lost acknowledged
write or unbounded growth. Then run final CS13/14 staging GPS/offline journey
and RVA14.D. Full release still requires its separate G09/G24 acceptance scope.

## Priority and final acceptance gates

Recommended sequential start: RVA00 → RVA01 → RVA02 → RVA09 → RVA03 → RVA04 →
RVA05 → RVA06 → RVA07 → RVA08 → RVA10 → RVA11 → RVA12 → RVA13.A/B → CS15
remaining → CS16 → CS17 → CS18 → CS19 → CS20 → CS21 → CS22 → RVA14.A/B → CS23
→ RVA14.C → CS24 → CS25 → RVA13.C/CS13/14 → RVA14.D → authorized delivery.
An unavailable external proof may be recorded OWED while independent local
construction continues; it still blocks its activation/release claim.

Before final delivery require all REV IDs resolved or explicitly accepted by the
user with reason, no HIGH blocker hidden behind disabled flags, all required
privacy/provider/device/wall-time proofs, real API and SQLite resource evidence,
fresh scan scope, exact current-head/base test evidence, and no secrets/PII.
Protected required checks must pass; use the configured finish helper with
`--required "Quick verification"`, preserve commits and dev, and mirror only
owned merged documentation. No direct main push, bypass or unrelated VPS action.

Deliver a concise final register per REV/CS ID: corrected source SHA, exercised
test/scenario, measured envelope, residual limitation, acceptance state and next
obligation. This is how an independent reviewer decides completion; commit count,
all-green mocks and a plan heading are insufficient.

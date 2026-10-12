# RVA07: deletion deadlines, cleanup fencing and truthful health

Status: IN_PROGRESS. No deadline certification from this document.

## B-BR / BUC

- B-BR-RVA07-1: operational photo deadlines remain first receipt + 24h;
  training withdrawals remain the first covering OFF receipt + 24h. Retrying,
  promotion, recropping, grants and scheduler phases never extend an anchor.
- B-BR-RVA07-2: dispatch with safety margin before cutoff, bounded oldest-first
  batches, nonce-fenced writers/cleanup and per-copy terminal acknowledgements.
  Any unresolved provider effect remains inventoried and unselectable; expired
  writer leases do not erase cleanup obligations. No network-held transaction.
- B-BR-RVA07-3: query/provider failure is UNKNOWN/FAILED, never healthy zero or
  a successful queue acknowledgement. Cleanup may proceed before reporting
  errors; reports preserve partial counts. Keys/pixels/precise locations are
  not logged. Admission fails closed if deadline assurance cannot be checked.
- B-BR-RVA07-4: minimal receipt retention cannot retain photographs. Backup,
  object version, exports, temporary files and offline power-off limitations
  must be explicit. Existing historical soaks remain untouched and cannot
  certify changes made after they started.
- BUC: one deletion fails while other copies succeed -> accurate partial
  report and retry -> confirm disappearance -> acknowledge once -> exclude
  purged work from hot scans. Audit DB outage -> UNKNOWN + error; no zero-health
  success. Pause a writer across erasure -> cleanup waits/fences -> no late
  orphan or false completion. Backlog and scheduling fit measured margin.

## Subtasks

- A/B1: truthful query/storage failure propagation and partial reports.
- B2: bounded training disposal/acks, including abandoned output leases.
- B3: evidence ingress/verification/cleanup race fences and deadline dispatch.
- C: real SQL/S3 failure/concurrency/query-envelope proofs, independent new
  autonomous wall-time run and bounded provider/device acceptance.

No whole-phase completion until all applicable proofs pass. External failures
or unavailable windows are PROOF_OWED, never inferred from injected time.

## A/B1 local checkpoint — truthful health

Status: LOCAL_DONE / INTEGRATION_PENDING for A/B1 only. Validation: PASS.
Tested delta on `deee51d`. Three RED reproductions returned nil error after
health-query failure, overdue-audit failure and partial training deletion.

GREEN: independent HealthKnown/OverdueKnown flags, original query errors and
partial cleanup counters survive; storage failures fail the completed pass
without preventing unrelated deletions. Workers do not acknowledge UNKNOWN,
partial storage failure or overdue copies as successful cleanup. Reports log
bounded counts/timestamps and knowledge flags, not payload keys.

`go test -race -p 2` passed for evidence/privacy trees plus API/worker/ops;
affected vet, format, diff and staged secret review passed. New real PostGIS
`TestSweepSQLObservationFailurePreservesCleanupAndUnknown` induces actual SQL
errors independently in both observations after a committed cleanup marker;
both cases passed. These tests use an explicit synthetic storage sink; real
S3 recovery is already covered by RVA05 and more cleanup proofs remain below.

B2/B3/C remain IN_PROGRESS/PENDING: no whole RVA07 deadline acceptance, no new
soak/provider/device completion, no VPS or pre-existing soak mutation.

## B2 output disposal opening

Purge revoked training copies immediately, with no wait until the 24h cutoff;
run every minute. Claim at most 32 copies per pass, one at a time, using a
60s nonce lease. Each private transfer is limited to 5s and the pass to 45s.
Expired WRITING/PURGING output and exhausted pending attempts are cleanup work,
not silently skipped inventory. A confirmed absence atomically acknowledges
PURGED (revoked/terminal) or repairable PENDING (active incomplete output).
No storage call holds a DB transaction; a stale ack cannot affect a new lease.
A provider outage leaves work leased/inventoried for retry, never selected.
Owner-wide erasure batching and evidence races remain subsequent subtasks.

## B2.1 local checkpoint — bounded output disposal

Status: LOCAL_DONE / INTEGRATION_PENDING for global training-output cleanup.
Validation: PASS. Tested delta on `7353352`. RED: a one-minute-old revocation
received no cleanup because the old sweep waited 24h. GREEN: immediate disposal
in 32-item/45s passes, per-copy 5s transfer and 60s durable nonce claim. Schedule
is every minute. Confirmed absent bytes receive PURGED or repairable PENDING
acknowledgements; purged/healthy/unused rows leave the hot cleanup query.
Append-only 000066 adds the partial due index; no applied schema was rewritten.

Real SQL/private S3 tests passed for failed DELETE with retained bytes/lease,
restart after lease expiry, byte absence + terminal ack, no repeated hot work,
live writer exclusion, abandoned active PUT disposal/repair and stale writer
refusal. Twelve parallel cleanup claimants yield one authority; expiry creates
a distinct token and stale acknowledgement refuses. 65 revoked rows drain
32/32/1/0 after bounded tombstone reconciliation.

The actual query file was EXPLAIN ANALYZE'd against 10,000 synthetic terminal
receipts plus live cleanup: the partial due index is used with no sequential
scan on derivative inventory. 000066-only upgrade/replay preserves history.
Full privacy adapter SQL race suite passed (18.743s), affected unit/consumer
race tests, vet, sqlc vet, gofmt, diff and staged secret checks passed.
Storage tests concern transport/recovery only, never content minimization.

Remaining B2.2: owner-wide training erasure must consume bounded claimed work
rather than ListByOwner snapshots. B3 evidence writers/deadline dispatch and C
wall-time/provider/backup acceptance remain pending. This does not certify a
maximum backlog/rate or the 24h SLO; collection stays OFF.

## B2.2 owner-erasure opening

Use a durable owner/keyset cursor, at most 32 verified copies per attempt.
Never load the whole owner inventory into memory. Include historically PURGED
receipts when a new erasure/restore replay starts: restored bytes can coexist
with retained deletion receipts and must be rechecked before traffic opens.
Advance the cursor only in the same transaction as a nonce-fenced byte-absence
acknowledgement. Failure/live lease leaves the first unconfirmed key pending.
Completion removes the cursor; replay scans bounded batches and remains
idempotent. A partially processed owner returns retryable failure, never READY.
Reuse an existing OFF revision/anchor on retry; do not grow revocation receipts
or slide the deadline simply because a cleanup attempt resumed.

## B2.2 local checkpoint — bounded owner erasure

Status: LOCAL_DONE / INTEGRATION_PENDING. Validation: PASS. Tested delta on
`a7be415`. RED: one attempt loaded/deleted all 65 owned images and returned
success. GREEN: append-only 000067 durable owner/key cursor, one nonce-claimed
copy at a time, 32 per attempt, shared 5s absence-confirmation protocol and
45s pass bound. The owner cursor advances atomically with the byte receipt;
remaining/live-leased/failed work prevents a completed erasure receipt.
No ListByOwner snapshot remains in the training erasure path. Equal OFF replay
keeps the original revision/time and materializes another bounded tombstone
batch, rather than minting revocations on every retry.

A new erasure/restore replay scans prior PURGED receipts too, because retained
metadata alone cannot prove that restored storage bytes are absent. Progress
is removed only after the keyset completes. Counts describe this attempt's
covered copies/deletions, not lifetime unique events; already absent terminal
copies contribute no repeated count.

Real SQL/private S3 65-copy owner work resumes after store reconstruction as
32/32/1, with a foreign-owner byte control preserved. Restore of bytes beside
retained PURGED receipts is detected and deleted through the same bounded
replay. OFF revision/time remain unchanged. Twelve concurrent owner claimants
obtain one authority. Injected cursor-write failure rolls back byte receipt
and cursor together; lease expiry/restart resumes the same key, stale ack
refuses. 000067-only upgrade/replay preserves consent history and writable
cursor state. Full privacy SQL race suite passed (19.753s); affected units,
worker/ops, vet, sqlc vet, format/diff and staged secret review passed.

Restore must occur in a closed-traffic, separate restore window; simultaneous
out-of-band storage restore during an in-progress erasure is not certified by
this test. B3/C remain pending; existing soaks were not modified or inspected
as passing. Collection remains OFF and no deadline/release claim is made.

## B3.1 quarantine ingress/erasure opening

Serialize new ingress claims with session expiry/erasure using a session row
lock, without holding a transaction across HTTP/S3. Expiry closes admission
first. A fresh deletion check then excludes every still-live upload lease
before deleting or acknowledging a quarantine copy. An already admitted PUT
may finish; its bytes remain inventoried with a pending cleanup marker until
the lease finishes/expires and a later bounded pass deletes them. No active
writer may be acknowledged as already erased. Final-output/verify inventory
and predeadline scheduling remain B3.2/B3.3, not silently inferred here.

## B3.1 local checkpoint — quarantine ingress fence

Status: LOCAL_DONE / INTEGRATION_PENDING. Validation: PASS. Tested delta on
`db42f98`. RED: erasure during a durably admitted upload invoked deletion and
acknowledged success despite the live writer. GREEN: ingress reads/claims under
a session row lock; erasure closes admission first and checks live transfer
leases in a fresh statement before any source deletion/marker. Sweep uses the
same check, preserving a pending marker for the next bounded pass.

Real SQL tests passed for live-upload erasure refusal/no false marker, refusal
of new ingress after expiry, late completion then recoverable deletion, and a
concurrent failed-transfer retry held behind an erasure row lock. Expiry wins
without charging another transfer attempt. Ingress budget/receipt/upgrade
regressions passed. Full evidence adapter SQL race suite passed (13.008s),
affected evidence/API/worker/ops race suites, dependent privacy erasure/restore
SQL tests, vet, sqlc vet, format/diff and staged secret checks passed.

The old forward lifecycle outage assertion expected a successful partial
sweep; the full SQL run exposed it. It now requires failure plus the existing
unchanged-byte/no-marker assertions, followed by a successful retry. No test
was skipped or relaxed. This checkpoint's erasure sink is synthetic; actual
HTTP/S3 cross-resource proof for late source/final writers remains B3.2/C.
No transaction spans network I/O, no provider/24h/release claim and no change to
existing soaks. Next: inventory/lease final output before PUT and clean failed
or revoked verification output; then predeadline scheduling and capacity proof.

## B3.2a final-writer admission opening

Inventory a canonical content key, owning session and conservative deadline
before any final PUT. One content key has one writer authority; a 60s UUID lease
covers a 20s verification attempt and its bounded commit. No transaction spans
network I/O. An uncertain PUT/commit keeps its inventory and lease; retries
cannot overwrite it until cleanup confirms absence. At most five writes are
admitted per key. Reusing an existing verified key performs no PUT, cannot
extend its deadline and must match both source and sanitized hashes.

Production outcome commits require the exact live writer nonce (or explicit
read-only reuse), a still-VERIFYING session and the original output deadline.
Erasure closes session admission even when quarantine was already deleted,
and unresolved final inventory prevents a false completed erasure receipt.
First reservation is the conservative output anchor, never a later retry's
updated_at. Legacy metadata fixtures retain their explicit internal API;
production jobs use only the guarded outcome method.

This first subtask establishes inventory/admission/commit fencing. B3.2b must
consume unconfirmed final inventory with bounded private-storage cleanup,
nonce acknowledgements and restart proofs; existing object-only sweeping is
not sufficient. Neither subtask alone certifies the complete 24h SLO.

## B3.2a local checkpoint — inventory before final PUT

Status: LOCAL_DONE / INTEGRATION_PENDING for admission/commit only.
Validation: PASS. Tested delta on `9fa7ad1`. RED: a valid photo uploaded without
any inventory authority. GREEN: both verification lanes require a pre-PUT
port; read-only reuse emits no bytes. Append-only 000068 inventories known
objects and conservative legacy possible-orphan destinations. New output has
one content-key owner, bounded five attempts, a UUID/60s lease and immutable
reservation-based deadline. Production jobs have a 20s attempt and use only
RecordVerifiedGuarded. SQL rollback/expiry/erasure cannot recreate READY from
an old verifier snapshot, and unresolved final inventory blocks erasure READY.

Real PostgreSQL tests passed: 12 simultaneous claims yield one writer/attempt;
stale nonce and expired lease refuse; quarantine-already-deleted erasure still
closes verification and refuses premature completion; actual insert-trigger
failure retains inventory and VERIFYING state; caller-supplied later receipt
cannot extend retention; byte-identical reuse has no new writer/attempt/anchor;
foreign keys, sixth attempt and lease beyond deadline refuse. 000068-only
upgrade preserves known copies and inventories possible legacy orphan keys;
replay is empty. Full evidence adapter SQL race suite passed (14.604s), affected
application/adapter/worker/API/ops race suites, vet/sqlc/format/diff checks passed.
Timestamp assertions use reloaded PostgreSQL microsecond precision, not the
pre-insert nanosecond fixture clock. Explicit codec/transport-only tests use
fake inventory authority; they are not SQL or deadline evidence.

B3.2b remains required: bounded consumption of this inventory, confirmed byte
absence, terminal/repair receipts and actual S3 late-writer/restart proofs.
The historical object-only sweep cannot close these new pending obligations.
Do not overlap old unfenced workers with the migration/new worker cutover.
B3.3/C scheduling/capacity/autonomous SLO/backup/provider proofs remain pending.
No VPS/phone/old-soak mutation, no whole-phase or release certification.

## B3.2b cleanup opening

Consume final inventory in oldest-first batches of at most 32, one 60s UUID
claim at a time, a 45s pass and a 5s private deletion/absence check. A live writer
or cleaner is never bypassed. Failed/ambiguous storage keeps PURGING inventory
for lease-expiry recovery; a stale acknowledgement cannot finish another claim.
Confirmed absence atomically updates the object receipt and returns an active
incomplete attempt to PENDING, or acknowledges PURGED for terminal/exhausted/
expired work. Legacy object-only cleanup first inventories its exact existing
owner/key, then uses the same authority. Erasure shares the 32-final budget,
closes admission and cannot erase a foreign owner's duplicate reference.
Production DELETE must be followed by a successful private HEAD 404 before
any absence acknowledgement; access-denied/outage/present are failures.
Restore remains a separate closed-traffic protocol, not implied by a marker.

## B3.2b local checkpoint — bounded final cleanup/recovery

Status: LOCAL_DONE / INTEGRATION_PENDING for final cleanup/recovery.
Validation: PASS. Tested delta on `6fef50d`; completed 2026-10-12. New inventory
and legacy object paths share UUID-fenced disposal, at most 32 finals/pass,
45s reconciliation and 5s private transfer; 60s leases survive uncertain effects.
Every production evidence deletion now requires DELETE plus private HEAD 404.
An expired/stale cleanup token is checked again before any storage effect;
both verification entry points carry a 20s transport context. Confirmed absence
atomically updates object and inventory receipts; an active incomplete attempt
can repair to PENDING, terminal/exhausted/expired work becomes PURGED.

Real PostgreSQL/private S3 proofs passed: pause final PUT across owner erasure,
refuse late READY, retain inventory for its actual provider effect, then restart
and delete it; lost PUT reply, failed DELETE retaining bytes/lease, expired-lease
repair to PENDING and successful re-verification; stale cleaner performs zero
storage calls and cannot acknowledge a replacement claim; duplicate reuse
performs no PUT and a foreign owner's erasure preserves the first copy. Twelve
concurrent cleaner claims yield exactly one authority. A receipt-update trigger
fails after real byte deletion: both object/inventory markers roll back, retry
confirms absence and commits both, while the old token refuses.

65 unresolved copies drain 32/32/1/0 with explicit per-copy time bounds. The
actual cleanup query EXPLAIN ANALYZE uses the 000069 partial index with 10,000
terminal receipts and no inventory sequential scan. 000069-only upgrade/replay
preserves a live writer. Overdue audit includes unconfirmed destinations and
uses the conservative inventory anchor for known objects, without double count.
Full evidence SQL race passed (16.346s); dependent privacy erasure/restore SQL
passed (1.824s); affected units, HTTP refusal matrix, worker/API/ops, vet, sqlc,
format/diff checks passed. Secret review is performed before this checkpoint's
commit. The default CI fixture mode also passed (2.657s) with its own bounded
pinned local MinIO created and removed; an initial approval-review usage-limit
failure executed nothing and is not counted as test evidence.

The full SQL run caught the old static mutation guard's exact update count;
it now permits only the additional key-scoped, set-if-unset deletion marker and
checks that guard explicitly. The forward outage test now proves a live cleanup
lease cannot be bypassed, then explicitly expires that fixture lease for restart.
Privacy restore RED exposed a newer PURGED receipt beside a restored live object
marker: forced erasure reopens only this contradictory known object under the
same owner/key lock, closes admission and repeats confirmed deletion. The replay
regression is green. Storage-only restores beside unchanged terminal markers
still require the separate closed-traffic restore protocol/proof in RVA14.C.

No old soak was touched or treated as passing. Remaining B3.3/C obligations:
predeadline dispatch/margin, total pass/admission/capacity envelope, bounded
metadata retention, actual autonomous wall-time run, provider/version/backup/
temporary-copy and device acceptance. These tests inject lease/deadline state;
they do not certify elapsed 24h compliance or production readiness.

## B3.3a metadata-retention opening

B-BR-RVA07-5: a 90-day hash purge may remove at most 100 object rows per
invocation, oldest first with a stable ID tie-break. It must retain every
unconfirmed destination: require both the object deletion receipt and the
matching terminal PURGED output receipt. Missing inventory is uncertainty, not
proof of absence. Concurrent passes skip locked candidates; no network effect
is performed by this metadata transaction. A partial hot index excludes live
object rows. Keep the copy inventory itself for the separate receipt-retention
review, rather than erasing recovery authority in a hash cleanup.

BUC: an ancient live object survives cleanup; a deletion marker alone also
survives without matching terminal inventory. Confirmed copies drain in
100-row resumable batches across store reconstruction. An injected transaction
failure commits no deletion. Upgrade adds only the partial index and replay
is empty. EXPLAIN the actual purge over terminal/live history. This checkpoint
does not close scheduling, admission capacity or autonomous deadline proof.

## B3.3a local checkpoint — confirmed bounded metadata purge

Status: LOCAL_DONE / INTEGRATION_PENDING for metadata purge only.
Validation: PASS. Tested delta on `0bd5fc3`. RED: the old purge removed an
ancient object while its final bytes were unconfirmed. GREEN: a single
statement selects at most 100 oldest confirmed objects with a stable ID
tie-break and SKIP LOCKED, requiring both the object deletion marker and
terminal PURGED inventory receipt. Missing or pending inventory is retained.
Append-only 000070 adds the partial confirmed-object index; no applied
migration or historical evidence was rewritten. Copy receipts remain intact.

Real PostgreSQL tests passed for missing/unconfirmed receipts, 205-row
100/100/5/0 restart convergence, 12 concurrent invocations with no lost/double
count, actual delete-trigger failure with atomic rollback, and 000070-only
upgrade/replay retaining an unconfirmed copy. EXPLAIN ANALYZE of the actual
query against 10,000 live objects uses the partial purge index without an
object sequential scan (the test transaction rolls back its measured DELETE).
The prior hash test now requires confirmed cleanup before removing metadata.
Targeted SQL race passed (3.119s), full evidence SQL/private S3 race passed
(22.238s), affected evidence/API/worker/ops unit race, vet, sqlc vet, gofmt and
diff checks passed. Staged secret review runs before commit. No provider,
autonomous 24h or release claim. Next: B3.3b predeadline cadence/pass fairness
and fresh lease clocks, then B3.3c admission/capacity and C external proofs.

## B3.3b.1 actual minute-dispatch opening

B-BR-RVA07-6: recurring schedule dedupe buckets must honor the declared
interval, including minute intervals; same-bucket restart/overlap converges,
while the next minute permits a distinct job. Use UTC interval boundaries,
keep existing exact hourly/daily key formats and reject nonpositive intervals.
Evidence cleanup must request a minute cadence instead of hourly dispatch.
This makes earlier every-minute declarations executable; it does not prove
queue service capacity or wall-clock cleanup deadlines by itself.

BUC: two ticks in one minute produce one queued job; the next minute produces
one new job; overlapping ticks in the next minute still produce one; restart
in that minute converges. Exact hourly/daily buckets remain compatible. A
sub-minute or non-hour interval uses its real duration, never a coarse hour.
A composition test exercises the actual evidence schedule returned to the
worker. Other existing minute schedules consume the same corrected scheduler.

## B3.3b.1 local checkpoint — actual minute cadence

Status: LOCAL_DONE / INTEGRATION_PENDING for dispatch identity only.
Validation: PASS. Tested delta on `cc48bdd`. RED: a second minute still had
four jobs rather than eight because Period collapsed all sub-day intervals
to an hour. This affected account-erasure and training minute schedules too;
earlier declarations of a minute interval were not actual dispatch proof.
GREEN: UTC interval buckets honor their exact duration, nonpositive intervals
are refused, and exact hourly/daily key formats remain compatible. The worker
uses an enabled every-minute evidence schedule with a version-1 payload.

Real PostgreSQL race test: four safety schedules, three minute boundaries,
12 concurrent reconstructed scheduler instances per minute produce exactly
four/eight/twelve jobs, with valid queued envelopes and no duplicate identity.
Same-minute restart converges. Unit boundaries cover minute/30s/90m/weekly,
invalid intervals and host timezone independence; actual worker composition
is tested. Full platform jobs SQL race passed (6.710s), affected unit race
and vet passed, format/diff checks passed; staged secret review precedes commit.

Cutover may leave already queued legacy hourly work alongside the first new
minute job; effects are idempotent and leased, never delete queue history to
force a schedule. Minute cadence increases queue work; service/admission/queue
retention bounds and predeadline cleanup remain B3.3b.2/B3.3c/RVA12. This is
not proof of capacity, successful autonomous deletion, provider readiness or
release certification. No VPS/phone/old-soak changes.

## B3.3b.2 predeadline and bounded-pass opening

B-BR-RVA07-7: begin operational cleanup two hours before the immutable
24h deadline. This reserve is an engineering policy, not a measured capacity
guarantee. Refuse new source/final writer leases and final commits at that cleanup boundary;
no repair to PENDING after it. A queued validation job does not extend photo
retention. Expiry closes old VERIFYING admission; actual upload/final writer
leases remain protected using fresh live time independent of policy clocks.

B-BR-RVA07-8: one pass has a 45s parent context, at most 32 original and
32 final candidates, separate 20s original/final slices and 5s copy contexts.
Original expiry is DB-only, followed by a single oldest-first eligible-source
scan; source failure cannot consume the final slice. Confirmed final receipt
acknowledgement may finish within its existing 3s cancellation-independent
bound; uncertainty remains inventoried. Metadata purge remains 100 rows.
Failure/cancellation prevents successful job acknowledgement; partial effects
and unknown observations stay visible. No live lease is evaluated against an
advanced policy clock, and active writers are filtered from source scans.

BUC: photos just before the 22h boundary remain, at/after it are eligible
without moving their 24h anchors. A live source PUT is not deleted even when
policy time advances; its later completion is cleaned by a resumed pass.
Stalled/failed original deletion leaves finals serviceable within their own
slice, and a huge requested batch cannot enlarge the hard ceilings. Test real
SQL/private storage, boundary writer refusal, cancellation and query bounds.
Queue capacity/admission, autonomous/provider/backup proof remain separate.

## B3.3b.2 local checkpoint — predeadline bounded disposal

Status: LOCAL_DONE / INTEGRATION_PENDING for cleanup policy/pass bounds.
Validation: PASS. Tested delta on `e26d418`. RED reproduced physical source
deletion under a still-live upload when policy time advanced, no final cleanup
at 22h, and admission of a final writer inside the cleanup margin. GREEN:
operational cleanup starts at 22h against the immutable 24h deadline; source
and final leases must fit before that boundary, final commits/repair refuse
after it. Idle and stuck VERIFYING sessions close at the earlier cleanup
boundary even if validation work is queued. Source disposal always uses fresh
live time, and its single oldest-first query excludes actual live transfers.

The whole pass carries a 45s context, original/final work separate 20s slices,
at most 32 source plus 32 final candidates, and each copy a 5s context. Final
absence receipt retains its existing independent 3s acknowledgement bound.
Original failures still allow final cleanup/observations; cancellation and
partial/query failures propagate. Audit lists clamp at 100; metadata purge
remains 100 confirmed rows. Append-only 000071 supplies the partial original
hot index. The production queued-job lookup was removed; compatibility-only
harness ports remain and cannot extend retention.

Real PostgreSQL/private S3 proofs pass: a live source PUT survives an advanced
policy clock with no deletion marker, then completion/restart removes bytes;
a final remains immediately before 22h and is physically absent/PURGED at the
boundary with its 24h anchor unchanged. Final writer admission/commit and
source leases crossing the cleanup boundary refuse; refused ingress does not
charge another attempt; incomplete cleanup cannot reopen PENDING in the margin.
The actual source query excludes a live writer from a 32-row eligible batch
and EXPLAIN ANALYZE uses the partial hot index against 10,000 terminal receipts
without a session sequential scan. 000071-only upgrade/replay preserves a live
writer fence. Unit tests pin huge-batch ceilings, 5s copy/20s slice/45s parent
contexts, source-failure/final-service separation, cancellation and distinct
22h-cleanup/24h-overdue boundaries. The old +25h test is explicitly named as
post-deadline recovery, not within-deadline compliance.

Full evidence SQL/private S3 race passed (20.163s), dependent privacy erasure/
restore race passed (4.585s), affected unit race/vet/sqlc vet/format/diff checks
passed. Initial verification exposed unused removed-lookup imports and a
fixture assuming a ten-minute session TTL; both were corrected before green.
Staged secret review precedes commit. The two-hour reserve is not measured
capacity certification: B3.3c admission/worker backlog and C autonomous/provider/
backup/temporary/device proofs remain owed. No VPS, phone or old-soak mutation.

## B3.3c.1 safety-dispatch isolation opening

B-BR-RVA07-9: a long importer/general job must not occupy the only execution
slot for deadline cleanup or erasure recovery. In the existing worker process,
use one filtered dispatcher for each of the four minute safety kinds (evidence
sweep, account-erasure recovery, training revocation reconciliation and training
copy disposal), plus the existing general dispatcher explicitly excluding them.
Each dispatcher remains serial: fixed five loops, no per-job goroutine fan-out,
no additional service/container and no dynamically unbounded concurrency.
Missing required handlers refuse startup; one dispatcher failure cancels and
joins its siblings. Existing durable fencing/retry semantics remain intact.

BUC: leave a general handler blocked; evidence cleanup still reaches DONE.
Safety jobs cannot be claimed by the general dispatcher, and a kind-scoped
dispatcher cannot claim foreign work. Cancellation joins every loop. SQL
claims keep SKIP LOCKED/token fencing; a partial kind/created index keeps a
safety claimant out of unrelated queue history. Test actual SQL claims,
concurrency and index-only upgrade/replay. This isolates service slots; DB/CPU
headroom, heartbeat/admission capacity and bounded queue retention remain
B3.3c.2/RVA12 and require measured evidence before any readiness claim.

## B3.3c.1 local checkpoint — independent serial safety slots

Status: LOCAL_DONE / INTEGRATION_PENDING for dispatcher isolation only.
Validation: PASS. Tested delta on `dd8095e`. RED against unfiltered dispatch
stole the older safety job instead of reaching the general handler. GREEN:
the same worker process has exactly four kind-scoped serial safety loops and
one general loop excluding all four kinds. No per-job goroutine fan-out or new
service/container. Required handler absence refuses startup; cancellation joins
the fixed loops and a real dispatcher error is not hidden by sibling cancellation.
SQL keeps parameterized filters, SKIP LOCKED and existing lease-token fencing.
Append-only 000072 adds a partial live-kind/created index.

Actual PostgreSQL test leaves a general handler blocked/leased while the
evidence dispatcher claims and completes its older cleanup job; the general
claim cannot steal safety work and the safety claim cannot take another safety
kind. The general job then resumes and completes. Worker composition pins the
exact four reserved kinds/five serial loops and missing-handler refusal;
cancellation joins all loops. The actual ClaimJob EXPLAIN ANALYZE uses the new
partial index without a queue sequential scan against 10,000 unrelated terminal
and 10,000 unrelated queued jobs (measurement transaction rolls back).
000072-only upgrade/replay preserves an active lease, its acknowledgement,
dedupe identity and foreign queued work. Full jobs SQL race passed (6.281s);
affected unit race, worker/API/ops consumers, vet, sqlc vet/format/diff passed.
Staged secret review precedes commit.

Fixed source concurrency is not a measured VPS resource envelope. B3.3c.2 must
still gate intake on fresh successful cleanup observation, bounded inventory/
backlog/capacity and safe configuration; RVA12 owns queue receipt retention.
C autonomous/provider/version/backup/temp/device proof is still owed. This
source-only checkpoint changes no VPS, phone, old soak or release status.

## B3.3c.2 intake assurance and atomic capacity opening

B-BR-RVA07-10: production photo intake requires a successful worker cleanup
observation no older than three minutes, no failed/running/unknown latest
attempt, no original/final destination already at the 22h cleanup boundary,
and an explicitly configured bounded pending-photo budget. A missing budget
(default zero), missing observation, DB error, stale/future observation or
unsafe backlog refuses new photos. No supported-user/VPS throughput claim
comes from a configuration value. The source ceiling is 100 pending photo
sessions (each may have an original and a final); deployment must choose a
lower measured budget and keep it identical across API replicas. Raising the
source ceiling requires renewed capacity evidence. This conservative resource
ceiling is not a business quota or automatic production qualification.

B-BR-RVA07-11: reserve replay-first semantics remain; fresh reservations take
one global transaction advisory lock for health/capacity check plus insert,
alongside the existing per-owner quota. Pending sessions include unconfirmed
originals, final inventory and legacy final objects; a marker alone does not
refund unresolved cleanup. Each branch selects at most budget+1 distinct
sessions before the bounded cross-branch count. Start of a new byte transfer
rechecks fresh health/boundary before charging a lease; an already confirmed
byte replay performs no PUT. Production uses the gated constructor; internal
metadata fixtures explicitly retain their legacy trusted store. A static
composition regression prevents accidental production fallback.

B-BR-RVA07-12: the worker records each sweep attempt with an increasing DB
epoch; starting an attempt closes intake until its own fenced success. Only
that latest attempt may acknowledge healthy/failed. Healthy means
cleanup returned no error, observations are known, and no remaining overdue
copy/storage failure exists. A failed acknowledgement fails the job; an older
worker cannot overwrite a newer failure. Keep only singleton counts/timestamps/
state, no copy keys or contributor data. Successful admission never certifies
provider availability for the future or replaces deadline/backup acceptance.

BUC: startup/disabled/stale/failed/unknown health refuses before quota/PUT;
recovery opens after a verified pass. Many owners concurrently compete for a
four-photo test budget: exactly four fresh reservations, no extra on replay,
and no refund while any copy remains unconfirmed. Upload authorization created
before a failure cannot introduce new bytes afterward. Out-of-order health
acknowledgement refuses; DB receipt fault cannot report success. Actual SQL
upgrade/replay and large-history bounded query plans precede closure. Provider
headroom/default activation/autonomous acceptance remain PROOF_OWED.

### B3.3c.2 local checkpoint — 2026-10-12

Status: LOCAL_DONE
Validation: PASS

The first targeted SQL run reproduced the unpublished admission omission:
unknown health reserved a photo and 40 owners all won a four-slot budget.
Fresh insertion now checks health/capacity under the global transaction lock;
configured reservations/claims/checks have a three-second bound. Observation
start and finish share that lock, epoch/one-shot acknowledgements fence stale
workers, and a cancelled run cannot acknowledge healthy. SQL timeout is a
retryable enforcement failure, not an invalid user contribution. HTTP returns
503 with Retry-After 5 for unsafe admission; confirmed transfer replay remains
read-only. Default budget zero intentionally keeps production photo intake
closed until its measured explicit configuration exists.

Real PostgreSQL 18/PostGIS tests cover missing/disabled/failed/stale/future/
running health, out-of-order and duplicate acknowledgements, receipt rollback,
observation-table outage, lock timeout without a leaked reservation, exactly
four winners among 40 owners, full-budget replay, existing-reservation transfer,
all three near-deadline namespaces, unresolved and legacy final capacity,
duplicate final keys, and 000073-only upgrade/replay preserving live ingress.
Actual bounded-count EXPLAIN uses both new partial indexes across 10k purged
outputs and 10k deleted objects without scanning terminal history. Metadata
absence callbacks are synthetic store proofs, not provider deletion evidence.

Validation from backend/: full evidence adapters SQL/private-S3 race matrix
passed (29.009s store; media/storage/HTTP/jobs also green); dependent privacy
Erase/Erasure/Restore SQL/private-S3 race passed (5.243s); affected evidence,
config, API/worker/ops unit race, vet, sqlc vet and diff-check passed. Additional
observation regression covers running and duplicate receipts. Both actual
API/worker signed-process flows passed with real local SQL/S3 (20.132s): photos
refuse before worker startup, then proceed only after an autonomously recorded
successful cleanup. Fixture budget eight is explicitly synthetic. No manually
inserted healthy receipt authorizes those process flows. Staged secret review
precedes commit.

Only exclusive local project abastevo-rva-20261011-9e0a831 was used. Whole
RVA07 remains IN_PROGRESS: elapsed autonomous 24h, provider/version/backup/temp/
device proofs and measured deployment budget are owed. Old soaks, VPS and phone
were untouched. Training collection remains OFF. Continue C proof preparation,
then independent RVA08 source work with these external obligations retained;
no capacity, release, GitHub publication or merge claim belongs to this record.

### Reservation checkpoint correction

Post-commit review found that 89fe92a omitted the tested reservation body. That
commit is not a valid B3.3c.2 closure. Restore the gated body in a new commit;
repeat the full evidence SQL/S3 race and affected unit/vet matrix, and verify
the source SHA-256 before/after testing and exact Git blob at commit. Earlier
results describe the tested working tree, not that incomplete commit. No push
or merge occurred. The corrected successor supersedes that closure claim.

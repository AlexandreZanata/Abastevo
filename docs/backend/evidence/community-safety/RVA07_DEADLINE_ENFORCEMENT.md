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

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

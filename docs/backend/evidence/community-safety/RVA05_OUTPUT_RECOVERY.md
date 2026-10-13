# RVA05: crash-consistent derivative output

Status: LOCAL_DONE / INTEGRATION_PENDING for inventory/output protocol.
Validation: PASS. Collection remains OFF; content qualification and deadline
reconciliation remain RVA06/RVA07 activation gates.

## B-BR / BUC

- B-BR-RVA05-1: inventory owns the immutable object key, owner, source/output hashes and consent revision. Queue revision must still be current. Replay repairs missing output using the original key/instant, never a newly derived attempt date.
- B-BR-RVA05-2: PENDING/WRITING inventory is not selectable. READY requires verified bounded bytes plus an exact, live nonce-fenced completion under the owner consent/revocation lock. Transfers have bounded time/attempt budgets; no network operation holds a DB transaction.
- B-BR-RVA05-3: revocation during output cannot acknowledge READY. Late bytes remain inventoried and are deleted/confirmed; leases prevent purge racing admitted writes. Lost acknowledgements and provider failures retain repair work. Terminal purge acknowledgements must not erase evidence of an unresolved transfer.
- BUC: inventory commit -> first storage PUT fails -> retry uses original key -> hash verification -> READY. Pause output -> OFF -> resume output -> refuse READY -> physical delete -> unselectable tombstone. Process death after any effect retains a bounded recoverable state.

## Implementation

Append-only 000065 stores the immutable UTC-derived locator and PENDING,
WRITING, READY, PURGING and PURGED receipts. Existing active inventory becomes
PENDING, never presumed verified. Selection requires READY plus live exact
consent/revocation checks. Writes admit at most five attempts with one nonce
lease (60s); transfer plus verification is limited to 20s and 3 MiB. Cleanup
pins its nonce before deletion; a stale completion cannot delete a newer
output. Owner locks cover DB transitions only, with no S3 call in a transaction.

The job validates the queued revision and committed owner/image/key. Replays
load original committed time/key and retain minimized bytes for repair; READY
replay verifies stored bytes without rewriting. Hash mismatch, lost response,
write failure and OFF during PUT cannot acknowledge success. Erasure refuses a
live writer lease; purge and erasure use the stored, validated locator.

## Local validation

Tested delta on parent `5a286e5` (committed with this evidence), local disposable
PostGIS 18 and private MinIO only; no VPS/device/production-photo operations.

- RED `TestPromotionReplayRepairsFailedOutput`: original behavior attempted one
  write after failure/replay; corrected regression requires two.
- `go test -race -p 2 -tags=integration ./internal/modules/privacy/adapters -count=1`
  passed (17.627s), including 000065-only upgrade/replay, historical UTC key
  backfill, 12-way lease admission, five-attempt bound, queued old revision,
  immutable replay date and full prior privacy SQL suite.
- Actual private SigV4/S3 matrix: pre-effect failure, write with lost reply,
  corrupt output, restart after write/before ack, OFF while PUT is paused,
  stale nonce completion/cleanup, verified replay and exact byte absence.
  Both reused owned fixture and independently booted bounded CI fixture passed;
  the latter removed only its exact owned container. No integration skip.
- Privacy/job/worker/ops unit race suites and vet passed; sqlc vet, gofmt,
  diff and staged secret-surface checks passed. Stored-locator erasure SQL
  tests revalidated after the final cleanup-key change.

## Remaining activation gates

This validates local cross-resource protocol, not provider deadline/backup
certification or content anonymization. A provider cleanup outage leaves durable
PURGING/lease state and an error, never READY. Bounded recurring disposal,
purge acknowledgements, health and all-copy deadline acceptance remain RVA07.
Source loading/collection stays OFF. RVA04's paused physical-output race now
has real SQL/S3 evidence; parent privacy activation still depends on RVA06–08
and published terms acceptance.

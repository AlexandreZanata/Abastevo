# RVA03: durable erasure recovery

Status: LOCAL_DONE / INTEGRATION_PENDING; no external acceptance.

## B-BR / BUC contract

- B-BR-RVA03-1: deletion commits account-wide erasure intent atomically with credential revocation, including accounts without contributor bindings. An unavailable dispatch hint cannot turn that accepted intent into an unretryable HTTP error.
- B-BR-RVA03-2: bounded worker reconciliation claims one due account with an expiring nonce-fenced lease; execution is independent of deleted owner sessions. It completes only after social cleanup and all captured contributor scopes succeed. Failure retains a due intent; restart resumes.
- B-BR-RVA03-3: FAILED privacy deletion is retryable. Only READY deletion is a successful replay; exports and invalid states cannot enter erasure execution. Completion cannot downgrade a concurrent READY receipt.
- BUC-RVA03: delete -> durable acceptance -> revoke login -> dispatch outage/restart -> reconciliation -> all scopes READY -> account intent complete. A stale lease cannot acknowledge another worker.

## Validation

RED reproduced FAILED deletion silently returning success with only one ledger scope, and a queue failure returning an error after credential revocation. GREEN now acknowledges durable acceptance and resumes from the original request reference after identity unlinking. Training erasure receives that stable reference too; no new identity linkage is minted.

Append-only 000063 backfills deleted accounts (including binding-free accounts), records new intents in the revocation transaction and indexes due work. One due account/one contributor is claimed with a two-minute nonce-fenced lease. Execution has a one-minute budget; acknowledgement is bounded to three seconds and survives caller cancellation. Failed work is delayed one minute; crashes recover on lease expiry through the independent every-minute schedule. An API hint accelerates work but is not required for recovery. No connection is held during cross-module work.

Social cleanup loads at most 32 live ratings, 32 comments and 32 votes, using existing atomic aggregate removals; historical export lists are separate. Evidence cleanup now refuses storage failures (previously discarded) and incomplete batches rather than incorrectly completing its ledger scope. It loads at most 32 quarantine and 32 final objects. Remaining work is retained for another claim.

Validation on the RVA03 working tree based on `6007574`:

- Unit/domain/HTTP/job/media/storage trees for account, privacy, feedback and evidence, plus API/worker/ops consumers: `go test -race -p 2 ...` green. HTTP deletion acknowledges a failed dispatch while revoked sessions remain refused.
- Real local PostGIS: full account/privacy/feedback/evidence adapter integration suites under `-race` green. New tests cover 12 concurrent claimants -> one lease, scope failure/backoff, stale/expired nonce refusal, restart, two contributor checkpoints, binding-free accounts, migration-only backfill/replay, cancellation/ack failure and resumed physical cleanup with six ledger scopes. Production attribution resolver remains unlinked after retry.
- A 75-comment social fixture proves first batch exactly 32, restart completion, atomic rating/vote tally removal, replay and an untouched other-account control.
- `sqlc generate`, `sqlc vet`, affected `go vet`, gofmt, diff and secret checks green. No new dependency.

The owned local disposable project was used; no VPS, phone or historical soak changed. Queue/throughput/deadline operational metrics, late in-flight media output fencing and autonomous deadline proofs remain assigned to RVA07/RVA12 and final integration; this checkpoint does not certify those or production readiness. Privacy queue retries now execute FAILED requests; terminal dead-job operator recovery remains auditable, while account intents independently resume through the recurring reconciler.

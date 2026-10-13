# RVA02 — deletion-independent key-account admission

Status: LOCAL_DONE / INTEGRATION_PENDING. B-BR-A01/A05: 100 successful key-account creations in a
rolling hour globally, atomic with account/verifier creation. Deleting a key or
account never refunds admission. A minimal ledger stores only generated sequence
and receipt time, no account/credential/device/IP linkage. Bounded expired-event
pruning happens on admission under the existing global transaction lock; only
events older than the full rolling window are eligible. Failures/uniqueness
conflicts roll back the event with the account. Server clock rollback is
conservative (future events still count). Migration conservatively seeds all
most recent 100 account audit creations in the last hour, including already deleted accounts,
because legacy credentials cannot identify erased creations.

RED/GREEN acceptance: 100 create/delete cycles then refusal; budget survives
restart/credential erasure; concurrency cannot overshoot; expiry restores capacity;
conflicting/failed transactions consume no event; real upgrade/backfill/replay.
Session/intent/history lifetime retention belongs to RVA12, not this task's exit.

## Validation

Tested tree: `964d108` plus this task. RED reproduced the original 101st
create/delete admission. GREEN: all account unit/domain/application/HTTP/OIDC
packages passed under race; real SQL 115 simultaneous create/delete attempts
admit exactly 100, a new store instance observes the durable budget, and +3601s
admission prunes retained events to one. Real upgrade applies only 000062,
backfills active and deleted audit rows and replays empty. Existing aggregate
mail/session/key and frozen 000060 upgrade checks remain green. sqlc generate/vet,
affected Go vet, gofmt, diff and secret-surface review passed.

No raw credential/owner/device/address enters the new ledger. Source-level
CreateKeyAccount store compatibility methods now delegate to the same atomic
admission path. Migration seeding is limited to the newest 100 audit timestamps;
omitted older timestamps cannot survive beyond the retained newest timestamps.
All tests use the unique local PostGIS project; no production data changed.
Remaining account/session/erasure retention orchestration is tracked in RVA12.

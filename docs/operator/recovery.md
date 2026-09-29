# Recovery runbook: restore with deletion replay (P07-T04)

Restore procedure for the backend database with privacy guarantees
(BUC-007/BUC-008, B-BR-016, ADR-008). Traffic stays closed until the
deletion ledger replays: no restored erased data is served before
replay.

## Preconditions

- Off-host encrypted backup artifact with checksum manifest (backup
  pipeline in P08-T03; until then, the documented `pg_dump` custom
  format plus SHA-256 manifest).
- Snapshot timestamp of the backup (when the snapshot started).
- Operator identity (`--operator` or `ANPFUEL_OPERATOR_ID`) and the
  access model from [access.md](access.md).
- A fresh isolated database (never restore over live production).

## Procedure

1. Stop API/worker traffic (disable writes at the edge; keep the
   database reachable for the operator only).
2. Restore the snapshot into the isolated database and verify
   row counts plus the migration ledger (`migrate` reports current;
   checksum mismatch fails the drill).
3. Replay every deletion ledger row present in the restored snapshot:

   ```sh
   go run ./cmd/ops privacy replay --contributor <id> --operator op-7
   ```

   Repeat for each contributor with ledger rows (list pending rows
   with `SELECT contributor_id FROM privacy_deletion_ledger
   WHERE replayed_at IS NULL`). Scopes with no matching rows report
   zero and still stamp: replays converge.
4. Cross-check deletion receipts newer than the snapshot: any erasure
   completed after the snapshot started is absent from a stale
   backup. Re-run it explicitly:

   ```sh
   go run ./cmd/ops privacy erase --contributor <id> --reason "post-restore re-issue (backup <timestamp>)" --operator op-7
   ```

   A documented deletion request for backed-up persistent data may
   take until backup expiry to age out of immutable backups; notices
   must explain this accurately (SECURITY_PRIVACY).
5. Run retention expiry (P07-T05 sweeper) so transient rows older than
   their caps do not survive through the backup.
6. Rebuild projections that depend on unlinked rows (community
   consensus jobs drain through the worker; verify current-price
   reads before opening traffic).
7. Smoke the release endpoints, then reopen traffic.

## Guarantees and limits

- Ledger rows carry identifiers plus scope only: enough to find
  reappeared rows, never payload. The ledger itself ages out with the
  backup horizon under retention (35 days max: 7 daily + 4 weekly).
- Price facts stay for history with anonymized references (ADR-008);
  private payload, links, keys, sessions, objects, trust views and
  export bytes go.
- A failed replay blocks traffic: fix the scope error, re-run
  `privacy replay` (convergent), and only then proceed. Never bypass
  with manual SQL edits.
- Full encrypted off-host restore drills with measured RPO/RTO land
  in P08-T04; this runbook's deletion-replay path is proven by the
  P07-T04 restore simulation test.

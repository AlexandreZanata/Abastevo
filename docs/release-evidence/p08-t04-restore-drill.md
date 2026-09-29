# P08-T04 restore drill evidence (local disposable drill)

Date: 2026-09-30. Scope: `infra/scripts/restore.sh` plus
`scripts/tests/test-restore.sh` (9/9 green) against the disposable dev
PostGIS. This is a local mechanism drill, not staging certification:
the provisioned-host drill repeats the same commands per the recovery
runbook before claiming P08-T04 acceptance on staging.

## What ran

1. Seeded post-stale-restore fixtures in the source dev database: one
   active contributor (`tok-drill`) with an observation, trust
   verdict and five pending ledger rows, plus one cleanly erased
   contributor (`deleted`, no token).
2. `backup.sh run` produced an encrypted artifact with manifest.
3. Corrupted copy refused before creating anything (no drill database
   left behind).
4. `restore.sh restore` into `anpfuel_drill_<ts>`: artifact verified
   (decrypt + checksums + catalog list), schema restored (19
   migrations on the ledger, PostGIS present), per-table counts equal
   to source across 12 tables, zero orphaned evidence objects, five
   pending ledger rows reported.
5. Real `ops privacy replay` binary against the restored snapshot:
   observation and trust links removed, ledger rows stamped; second
   replay converged.
6. Post-replay assertions: no `tok-drill` rows in observations or
   trust projections, no unstamped ledger rows, erased contributor
   still `deleted`.
7. Drill database dropped; non-drill drop name refused.

## Results

- `restore-drill: 9 passed, 0 failed` (harness output preserved in CI
  logs when run remotely; rerun with `make test-restore`).
- Restored counts matched source on every checked table; evidence
  join integrity held (0 orphans).
- No restored erased data is served before replay: traffic stays
  closed until step 5 per [the recovery runbook](../operator/recovery.md).

## Limits

- Disposable single-host database, synthetic fixtures, loopback only.
- R2 bytes are not restored here (object lifecycle is independent);
  missing objects must yield deleted/missing states per the plan.
- Measured RPO/RTO come from the provisioned-host drill (P08-T04
  acceptance), not from this run.

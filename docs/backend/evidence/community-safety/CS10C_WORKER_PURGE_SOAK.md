# CS10C — Promotion worker, byte namespace, purge and 24 h soak

Date: 2026-10-11. Status: LOCAL_DONE (mechanics) / SOAK_RUNNING
(wall-clock SLO proof due 2026-10-12). Scope: job handler, object
store, purge sweeper, worker wiring, soak harness and soak start —
no erasure TRAINING scope (next), no VPS/APK. Head: maintained
`dev` + this task.
Plan: [phased plan](../../../planning/COMMUNITY_SAFETY_EXECUTION_PLAN.md)
(CS10, AT03/AT04/AT05), [CS10A contract](CS10A_PROMOTION_CONTRACT.md),
[CS10B store](CS10B_DERIVATIVE_STORE.md). Rules: B-BR-CS06, BUC-CS05.

Validation: PASS (mechanics; soak proof pending wall clock)

## Worker (`privacy/adapters/jobs/promote.go`, kind `privacy-training-promote`)

- Identifier-only payload (image/owner/captured/per-image/
  revision/source hash — never bytes). Loaded bytes hash-bind to
  the payload before any proposal: substituted objects refuse.
- `Propose` now also returns the minimized bytes just produced
  (nil on replay) so the job lands exactly the inventoried bytes
  through WriteOutput; replays converge without a duplicate write.
  Refusals and outages error for retry/park per queue convention.
- Output keys derive from the inventoried promotion instant under
  the single `training/v1/` prefix (application-owned derivation;
  traversal/zero-revision refuse).
- `privacy-training-sweep` handler runs one purge pass daily
  (`privacy-training-sweep-daily`, 24 h interval); both handlers
  registered in `cmd/worker` with real pg stores, registrar
  attribution, media sanitizer and R2/MinIO objects — unconfigured
  storage or the not-yet-built quarantine source refuse explicitly
  instead of misbehaving. The queue producer (client-review offers)
  arrives with CS11; the handler idles honestly until then.

## Purge (`privacy/application/purge.go`, AT05)

- `SweepRevoked`: revoked rows past `RevokedCopyTTL` (24 h) are
  existence-checked, deleted and confirmed gone; already-missing
  copies report as missing (observability, SLO trivially met),
  resisting copies as failures for the next pass. Rows stay
  REVOKED in all cases — receipts feed the restore filter, bytes
  are what purge owns. Deadline math uses the injected clock for
  tests; the wall-clock proof is the soak below, never asserted
  from virtual time.
- `TrainingObjects` speaks SigV4 presigned URLs against the
  private bucket (PUT/GET/DELETE): namespace-prefixed keys only,
  1-byte bounded existence reads (overflow proves presence),
  idempotent deletes.

## Validation (actual)

- Unit: 4 job tests (happy + namespace-scoped write, replay
  skips write, 7 malformed payloads, hash-bind refusal,
  consent-race/source-outage fail-closed), 2 sweep tests (fail on
  bad payload/version/outage), 4 object tests (round trip,
  idempotent delete, quiet miss, namespace refusal), 4 purge
  tests (due purge, missing/failure split, key shapes, restore
  drill over rebuilt rows). RED before GREEN throughout.
- Full `./internal/modules/privacy/...` unit + real-PostGIS
  integration suites green under `-race`; `cmd/worker` builds;
  `go vet` (plain/integration/soak tags), `gofmt -l`,
  `git diff --check` clean; `scan-secrets.sh` passed. Only
  disposable local databases and the local soak stack used.

## Soak SOAK_RUNNING (seeded 2026-10-11T16:11:11Z)

Substrate: `anpfuel-training-soak-minio` (local MinIO image,
named volume `anpfuel-training-soak-data`, restart
unless-stopped) + bucket `training-soak-private` + scratch DB
`training_soak` on the persistent dev PostGIS. Real SigV4
presigned transfers; real wall-clock revocation instants.

- `soak-overdue` (revoked 2026-10-10T15:11:11Z, due
  2026-10-11T15:11:11Z): first sweep purged it (checked=1
  purged=1, bytes confirmed gone, receipt intact) — overdue path
  proven end to end on real infrastructure.
- `soak-fresh-1/2/3` (revoked 2026-10-11T16:11:11Z, due
  2026-10-12T16:11:11Z): retained by the same sweep — the
  wall-clock SLO proof, verifiable after the deadline.
- `soak-control` (ACTIVE, granted consent): must survive every
  sweep; row + bytes confirmed present.
- Manifest: `/tmp/training-soak-manifest.json` (keys/dates
  above); verify command: `go test -count=1 -tags=soak
  ./internal/modules/privacy/adapters/ -run TestSoakVerify`.
  If the host restarts overnight, tmpfs-independent state is:
  rows in persistent dev PostGIS, bytes in the named volume;
  a vanished copy reads as Missing (failed gate → re-soak),
  never as success.

## Limits / owed

- Soak proof (run verify after 2026-10-12T16:11:11Z, record SLO
  measurements, close CS10): next turn. Do NOT claim the SLO
  before that run.
- Erasure TRAINING scope fan-out: next microtask.
- Promotion queue producer (client review opt-in): CS11.
- Restore drill against a real backup: with the soak proof
  (existing `test-restore.sh` chain + selection-filter replay).
- Restricted DB roles: P08 provisioning.

Next microtask: erasure TRAINING scope; then soak proof after the wall clock.

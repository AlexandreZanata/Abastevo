# SEC05-E — Every-copy 24 h soak

Date: 2026-10-11. Status: SOAK_RUNNING / PROOF_OWED (time-gated —
cannot complete in-turn; worker/API health is never deletion
evidence). Scope: one quarantine original + one sanitized final on
disposable local services. Head: maintained `dev` + this task.
Plan: [remediation plan](../../../planning/SECURITY_REMEDIATION_PLAN_2026-10-11.md)
(SEC05, AUD-06). Contract: B-BR-M01/M04, CopyRetention 24 h.
Constraint: synthetic bytes only; no production data; inability to
delete at due time is an explicit release failure.

Validation: PARTIAL (mechanism proven, wall-clock proof owed)

## Mechanism (proven in-turn)

- `TestSweepEveryCopyMeetsDeadline` (fake clock): one sweep at
  first-receipt + 25 h deletes and marks every category — idle
  original, terminal original, stale final, aged-out object row —
  with zero storage errors and a quiet post-pass signal.

## Live soak (seeded 2026-10-11T19:11:39Z, proof due after 2026-10-12T19:11:39Z)

- PostGIS: container `sec05e-soak-pg` (pinned `postgis/postgis:18-3.6`,
  host `127.0.0.1:32777`, volume `sec05e-pgdata`), all 59
  migrations applied; seed session `b014355b-4057-41ec-9ee4-897f29c1ba74`
  (ISSUED, `q/soak-orig-1`, created=seed time, 24 h TTL) and final
  object `b79c406e-7e39-478f-8bad-ff6cca226f42`
  (`f/soak-final-1`, received=seed time).
- S3: container `sec05e-soak-minio` (pinned MinIO, host
  `127.0.0.1:32778`, volume `sec05e-minio`), bucket
  `sec05e-everycopy` with `q/soak-orig-1` + `f/soak-final-1`
  (100 KiB synthetic each). Test-only soak credentials only.
- Pre-due audit at seed: 0 overdue originals, 0 overdue finals
  (healthy, as expected for fresh copies).
- Verify (after the wall clock, same host):
  `ANPFUEL_EVIDENCE_SOAK_DSN='postgres://anpfuel:anpfuel@127.0.0.1:32777/anpfuel?sslmode=disable' ANPFUEL_EVIDENCE_SOAK_S3='http://127.0.0.1:32778' ANPFUEL_EVIDENCE_SOAK_BUCKET='sec05e-everycopy' ANPFUEL_EVIDENCE_SOAK_S3_USER='sec05soak' ANPFUEL_EVIDENCE_SOAK_S3_PASS='sec05soakpass123' go test -count=1 -tags=integration ./internal/modules/evidence/adapters/ -run 'TestSEC05ESoakVerify' -v`
  (`TestSEC05ESoakVerify`: asserts tripped pre-sweep, runs one
  real Sweep over real PG + real MinIO, asserts zero storage
  errors and a quiet post-pass signal; skips without the env).
- Recovery: containers are `--rm` (volumes persist); if the host
  restarts, recreate both containers with the same volumes and
  re-resolve published ports before verifying.

## Limits / owed

- PROOF_OWED: the deletion receipt after 2026-10-12T19:11:39Z. Do
  NOT claim every-copy deadline compliance before that run.
- Training copies ride the separate CS10C soak; this soak covers
  quarantine originals + sanitized finals (+provider bytes via
  MinIO). Multi-replica and R2 production behavior unverified. No
  production claim.

Next microtask: SEC06-A source-origin enforcement (or the due soak proof, whichever comes first).

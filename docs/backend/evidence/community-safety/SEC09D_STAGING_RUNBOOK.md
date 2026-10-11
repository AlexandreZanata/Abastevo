# SEC09-D — Abastevo-only staging runbook and rollback

Date: 2026-10-11. Status: LOCAL_DONE. Scope: runbook document only —
no rollout, migration, or VPS mutation. Head: maintained `dev` + this
task.
Plan: [remediation plan](../../../planning/SECURITY_REMEDIATION_PLAN_2026-10-11.md)
(SEC09). Contract: staging topology (`infra/compose.staging.yml`:
db 2 cpu/6 G, api 2 cpu/1 G, worker 2 cpu/2 G, caddy 1 cpu/512 M);
existing guards (aggregate/per-key quotas, global breaker, IntakeHealth,
sweep/audit breakers, `/health/ready`). Constraint: Abastevo-only —
no shared workloads/proxy/images/volumes/data changes; historical
5 req/s windows are NOT automatic budgets; spare capacity is measured
in the window, never assumed.

Validation: PASS

## Preconditions (all required, authorized window only)

- Explicit user authorization naming the release tag
  (`ANPFUEL_RELEASE`, never `latest`) and the window.
- Staging env file operator-owned, never committed; DB password set;
  `docker compose -f infra/compose.staging.yml config` clean.
- Spare capacity measured on the host first (CPU idle, free mem,
  disk, current staging load); rollout rate fits inside measured
  headroom with 50% reserve. No number below is a substitute.

## Rollout (db → migrate → api/worker → caddy)

1. `up -d db`, wait healthy (`pg_isready`, ≤10 retries).
2. `run --rm migrate` (one-shot, lock/checksum ledger); abort on any
   migration error — never retry blindly, never skip.
3. Rebuild + recreate `api`/`worker` at the release tag; verify
   `/health/ready` 200 before traffic; smoke: anonymous read 200,
   signed write 201/409-shape, 401 without proof.
4. Switch `caddy` last; confirm TLS + front-only exposure (no
   origin-port mapping; infra gate).

## Rate bounds (measured, not historical)

- Start at 1 req/s synthetic, step only while p95 mixed-route latency,
  error rate, and backlog stay flat; stop well below any guard trip.
- The historical 5 req/s windows are prior observations, not budgets
  or maximum-user estimates — each rollout re-measures.

## Stop conditions (any one halts and rolls back)

- p95 breach vs. window baseline, any 5xx burst, growing job backlog,
  overdue-copy alarm, reserve/byte-budget trip, OOM/restart of any
  service, or operator judgment. Guards tripping is a stop, not a
  tuning prompt.

## Rollback

- Repoint `caddy` to the previous release tag (kept); recreate
  `api`/`worker`; verify `/health/ready` + smoke. Migrations are
  append-only and never rolled back — forward-fix only; data stays.
- Record the stop reason + head + timestamp in PROGRESS; failed
  rollout never retries in the same window without a new cause.

## Antiblast (Abastevo-only)

- Touch only `anpfuel-staging-*` networks/volumes/containers; host
  proxy, other workloads, shared images/volumes, and production data
  are out of scope and untouched. DB has no published ports.

## Validation (actual)

- Document-only: topology limits, guard names, and command shapes
  verified against current `compose.staging.yml`, health wiring, and
  quota/breaker code references.
- `git diff --check` clean; no code/test/infra touched, no
  secrets/PII.

## Limits / owed

- No rollout performed here; spare-capacity numbers belong to the
  authorized window, not this doc. G09/G24/live statuses distinct.

Next microtask: SEC09-E final protected delivery.

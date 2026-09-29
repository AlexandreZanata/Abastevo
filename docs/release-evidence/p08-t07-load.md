# P08-T07 bounded load evidence (local disposable drill)

Date: 2026-09-30. Scope: `infra/scripts/load/run.sh` plus
`faults.sh` via `make test-load` (bounded smoke profile: 500 seeded
stations, 20 s at concurrency 8, origin reads only). This is a local
mechanism smoke test, not staging certification: the 30-minute
acceptance matrix (100k stations, 1M rows, signed writes, cache and
fault scenarios) runs on provisioned staging infrastructure (P09).

## Observed run

- Seed: 500 deterministic stations, count-asserted before the run and
  scrubbed after (disposable database left clean for others).
- Requests: 54,951 origin reads (search/detail/nearby rotation).
- Latency: p95 8.1 ms against a 300 ms budget.
- Errors: 0.0000 unexpected-5xx rate against a 0.01 budget.
- Faults 7/7: database outage surfaces as refused/503 (never
  false-healthy), reads fail closed, database recovers with readiness
  returning; storage outage grants no upload authorization (401,
  no costly work); capped 128 MiB disk pressure keeps serving.

## Limits (explicit non-claims)

- Tiny data (500 rows, cold caches) and loopback only: passing here
  proves the harness and the budgets plumbing, not production
  capacity. Do not quote these numbers as capacity.
- Reads only: signed-write admission load needs synthetic protocol
  clients and runs in the staging matrix.
- Origin only: no shared-cache reads in this profile (edge behavior
  is covered by P08-T06 with TTL bounds).
- Disk pressure is capped and synthetic; real exhaustion drills run
  on staging with monitoring.
- Worker crash recovery is covered by lease/fencing unit and
  integration suites (P03/P06); the staging matrix adds the live
  kill-and-drain scenario.

## Budgets enforced by the harness

Reads p95 ≤300 ms, unexpected 5xx <1% (INFRASTRUCTURE_PLAN
acceptance workload). Breaches refuse with a non-zero exit; rerun
the failing scenario after tuning the measured bottleneck.

# RST-12 — Measurement harness and instrumentation qualification

Status: IMPLEMENTED — harness/tooling only; no production load, no
database change, no new dependency (Python stdlib). Date: 2026-10-09.
Scope: `RUST_STATION_BENCHMARK_PLAN.md` RST-12, dependency RST-11
(IMPLEMENTED). Language: English document; user communication in
Portuguese.

B-BR/BUC: B-BR-RST-P02 (frozen sample schema, pinned clocks/units),
B-BR-RST-P04 (isolated loopback qualification, disposable DB only),
B-BR-RST-P06 (raw samples reconcile to counters; timeouts/drops are
failures, never omitted). Serves BUC-RST-P04 (capacity) and
BUC-RST-P05 (regression detection).

## 1. What was added

- `infra/scripts/load/stats.py` (extended, legacy CLI unchanged):
  raw-sample schema v1
  (`scheduled_ms,started_ms,completed_ms,status,bytes`, monotonic
  ms, optional header); `summarize` with
  offered/started/completed/successful/dropped/cancelled/timeouts/
  wrong/error classes, err rate over offered, success-only
  p50/p90/p95/p99/max plus scheduling-delay stats and a reconcile
  flag; fixed-bucket `histogram_of`/`merge_histograms` (split runs
  merge into exactly the whole run); `compare_overhead` paired
  on/off-telemetry aid (deltas + watch beyond 10%, reporting only);
  `--samples` mode exits 1 on budget breach or mismatch and 2 with
  REFUSED (no traceback) on truncated/unreadable input. Extended
  `--self-test`: slow fixture breaches (never healthy), mixed
  error/drop/timeout/wrong/cancel accounting, histogram merge
  equality, truncated/empty input refusal.
- `infra/scripts/load/drive.py` (new, stdlib only): closed-loop
  1/8/32-client driver and open-arrival-rate driver with bounded
  slots (overload becomes counted drops = generator-limited, never
  silent demand reduction). Layers separate by construction
  (scheduled→started vs started→completed); DB statements and
  EXPLAIN stay in diagnostic runs, never in timed requests. UTC
  wall start, monotonic intervals, `--deadline-ms` right-censoring,
  `--expect-substring` semantic check (`wrong`), abandon-at-teardown
  (`cancel`), `--telemetry on|off` manifest tag for paired trials.
  `--self-test` qualifies every path against a loopback fake
  server: fast/timeout/non-2xx/wrong/cancel/drop, counter
  reconciliation on each CSV, unwritable-output refusal.
- `scripts/tests/test-load.sh`: runs `drive.py --self-test` after
  the stats self-test. `run.sh`/`faults.sh` untouched.

## 2. Evidence

- `stats.py --self-test`: ok (legacy percentiles + 7 new fixture
  groups). `drive.py --self-test`: 9/9 PASS, 0 failures
  (loopback, ~6 s). `shellcheck` on the touched shell file: clean.
  `py_compile` on both scripts: clean.
- Affected-consumer run `test-load.sh` on the disposable dev DB:
  drive self-test 0 failures; load smoke 46,546 requests, p95
  15 ms, 5xx rate 0.0000 within budgets; faults 6/7 PASS.
- Known pre-existing failure (unchanged by this task, reproduced
  on a clean tree without RST-12 changes): `faults.sh` "readiness
  returns after recovery" FAILs on this host (API readiness does
  not return promptly after DB stop/start). Outage still fails
  closed (refused/503, never false-healthy); recovery of the DB
  itself passes. Fixing readiness recovery is a separate backend
  task, not RST-12 scope.
- Exit criteria: a slow fixture reports breach (exit 1), an
  erroring fixture reports errors (exit 1), truncated samples
  refuse (exit 2), and every produced CSV reconciles
  (offered == completed + dropped + cancelled). No zero-filling:
  unavailable tools/permissions refuse explicitly.

## 3. Limits and next task

- No measured campaign latency, DB seeding, or telemetry on/off
  paired trial here; those execute under RST-13+ with this
  harness. Open-rate scheduling is thread/cadence-based (no k6;
  new load tools still need need/license/security review).
- Next (smallest, separately authorized): RST-13 Rust preparation,
  spill and resource scaling.

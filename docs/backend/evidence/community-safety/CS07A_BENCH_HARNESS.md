# CS07A — Helpfulness benchmark harness

Date: 2026-10-11. Status: LOCAL_DONE. Scope: harness + local safe-rate
runs only — no route/schema/migration change, no VPS/APK, no staging
run. Head: maintained `dev`.
Plan: [phased plan](../../../planning/COMMUNITY_SAFETY_EXECUTION_PLAN.md).
Rules: B-BR-CS07.

## Harness (`adapters/helpfulness_bench_test.go`)

Exact application paths (Cast/TallyFor) on a disposable database with
a separate 50-op warmup and fixed inner loops (200 ops, 8 workers):
tally-read, cast-fresh-voter (contended tally row), flip-same-voter.
Reports p50/p95/p99/max + wall ops/s. Tally SQL mirror with drift
test against `helpfulness.sql` (RST-07 style). Shared seeding helpers
generalized from `*testing.T` to `testing.TB` (compatible, no
behavior change).

## Local numbers (disposable PostGIS, 8 workers, 200 iters, warmed)

Two consecutive runs, stable:

- tally-read: p50 0.07ms, p95 0.28ms, wall ~34k op/s.
- cast-fresh-voter: p50 3.6ms, p95 8.9ms, wall ~1.9k op/s.
- flip-same-voter: p50 2.9ms, p95 15.2ms, wall ~1.5k op/s.

p99 spikes (4–19ms) on tiny windows are warmup/scheduling noise;
staging 120s windows smooth them. Local wall rates prove the harness,
never capacity: no max-user claim, ever.

## Frozen staging protocol (CS07B, authorized window)

5 independent 120s runs per scenario with separate warmup at safe
rates; per-run kernel/runtime evidence attached; guarded 5 req/s
ceiling frozen only when the selected tail p95 holds with ≥5%
headroom; runs stop below sampled pool/row/storage thresholds with
wall-clock throughput recorded. Ingestion stays narrow with rows/day
within twice the T-30 median; baselines on volunteers/public data.

## Checks

Harness + drift test green twice; existing helpfulness integration
re-green after the TB change; `go vet` (integration tags),
`gofmt -l`, `git diff --check` clean; `scan-secrets.sh` passed.

Next microtask: CS07B — staging guard runs (authorized window).

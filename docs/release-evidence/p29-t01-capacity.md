# P29-T01 — Capacity, freshness and review-backlog campaign

Status: LOCAL_DONE on `codex/phase-29-catalog-acceptance`. Task: P29-T01.
Binds B-BR-D02/D03/D05/D10/D12 and BUC-D06/D08. Scaled synthetic
campaign with budgets frozen before measuring; simulation only — no
live national SLA and no 30-day service claim.

## Frozen budgets and measured results (real PostGIS, disposable DBs)

| workload | budget | measured | verdict |
|---|---|---|---|
| 10,000-row stage (batch 500) | complete < 120 s, zero drops | 5.3 s @ ~1,889 rows/s, 10,000/10,000 accounted, heap +1.2 MB | PASS |
| 4-way × 1,000 burst + 64 concurrent reads | read p95 < 500 ms | p95 553 µs | PASS |
| 48 h outage → 2 backfills (500 novel each) | complete, no loss | 500 + 500 accepted | PASS |
| identical replay | converge, no duplicates | same run, counts stable | PASS |
| review backlog: batch 25 over 1,000 pending | < 5 s | 1.2 ms | PASS |
| heap growth (10k stage) | < 512 MB | ~1.2 MB | PASS |
| required indexes present | 5/5 | GIST point, municipality, active-identifier unique, run snapshot unique, assertion replay unique | PASS |

Freshness is reported per run (`LastCompleteRegistryRun`: snapshot,
finished_at, counts) — source lag and eligibility-to-publication lag
stay separately visible; the internal 24/48 h objective counts from
sufficient evidence with unresolved/quarantined rows reported apart.

## Validation

- NEW: `capacity_integration_test.go` (5 tests, `-race`): throughput/
  heap, burst-with-reads, outage catch-up, backlog batch, index
  presence. One real failure fixed honestly (identical day-2 content
  correctly deduped 0/500 — test data now uses novel days).
- `go vet` clean; `gofmt` clean.
- `git diff --check` PASS; `scan-secrets.sh` PASS (synthetic
  `[P29-TEST]` data only).

## Limits and next

- Scaled hypotheses (100k/10k/1k from the plan) are NOT proven at full
  scale here; national calibration with production-shaped hardware is
  explicitly future work — no SLA claimed.
- Next: P29-T02 source-to-app lifecycle and affected device
  reacceptance.

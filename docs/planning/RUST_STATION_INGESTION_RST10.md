# RST-10 — Experimental protocol and performance budgets

Status: PROTOCOL_FROZEN — no code, database, network or load-runner
change. Date: 2026-10-09. Scope: `RUST_STATION_INGESTION_PLAN.md`
RST-10, consuming RST-07 as the bounded pilot. The full campaign
method lives in the sibling [benchmark plan](RUST_STATION_BENCHMARK_PLAN.md);
this file freezes only the protocol artifacts below, without
duplicating it. Language: English document; user communication in
Portuguese.

## 1. Inventory (frozen context)

- Lab: Intel i7-13620H workstation, disposable PostGIS 18.6/3.6.4.
- Target class: VPS from `infra/compose.prod.yml` (db 8 GiB, api
  2 GiB, worker 4 GiB); lab numbers are not production promises.
- Queries: `SearchStations`, `NearbyStations`, `FixDistanceM` plus the
  prep pipeline (parse/join/dedup/spill/emit) at `station-prep 0.2.0`.
- Versions: migration chain through 000051, sqlc v1.31.1, Go 1.27,
  Rust 1.99, csv 1.4.0 / serde 1.0.229 / serde_json 1.0.151 /
  sha2 0.10.9 (Cargo.lock tracked).
- Data: synthetic deterministic fixtures only (seeds fixed); real
  census size unmeasured — national fetch out of scope.
- Gaps (block SLO language until closed): no `pg_stat_statements` /
  `auto_explain`, single-client benches only (8-client evidence
  missing), one workstation (no hardware matrix).

## 2. Frozen artifacts (v1)

Run-manifest schema a campaign run must fill before any verdict:

```json
{
  "id": "run-manifest-schema",
  "version": 1,
  "required": ["run_id", "workload_id", "hardware", "software", "dataset", "started_at", "metrics", "verdict"],
  "metrics": ["wall_ms", "rows_per_sec", "peak_rss_kb", "p50_ms", "p95_ms", "p99_ms", "planning_ms", "execution_ms", "table_mib", "indexes_mib", "error_count", "drop_count"],
  "verdict": ["pass", "fail", "aborted"],
  "rule": "verdict is decided from the frozen budgets below before results are seen; post-hoc thresholds are forbidden"
}
```

Metric dictionary (units fixed; percentiles over client-measured iterations):

```json
{
  "id": "metric-dictionary",
  "version": 1,
  "rows_per_sec": "accepted input rows per wall second",
  "wall_ms": "end-to-end workload wall time",
  "peak_rss_kb": "process peak RSS (/usr/bin/time -v) or HeapAlloc for Go stages",
  "p50_ms": "median client-measured iteration",
  "p95_ms": "95th percentile client-measured iteration",
  "p99_ms": "99th percentile client-measured iteration",
  "planning_ms": "PostGIS planning time (EXPLAIN ANALYZE)",
  "execution_ms": "PostGIS execution time (EXPLAIN ANALYZE)",
  "table_mib": "pg_total_relation_size of the measured table",
  "indexes_mib": "pg_indexes_size of the measured table",
  "error_count": "failed operations (any failure aborts SLO claims)",
  "drop_count": "shed load (any drop aborts SLO claims)"
}
```

Workload IDs (fixed datasets, seeds and parameters):

```json
{
  "id": "workload-ids",
  "version": 1,
  "W-parse-100k": "bench_generate 100000 7, both layouts, bench_parse + StageCSV",
  "W-parse-1M": "bench_generate 1000000 7, raised caps only, streaming follow-up owed",
  "W-city-dense": "Reader.Search SP/densest-code limit 20, first page",
  "W-city-sparse": "Reader.Search SP/sparsest-code limit 20, first page",
  "W-nearby-dense": "Reader.Nearby cluster center radius 5000 limit 20",
  "W-nearby-sparse": "Reader.Nearby empty area radius 5000 limit 20",
  "W-smoke-500": "existing infra load smoke (500 stations, 20 s, 8 clients)"
}
```

Resource ceilings and stop conditions (violations abort the run):

```json
{
  "id": "ceilings-and-stops",
  "version": 1,
  "ceilings": {
    "parser_rss": "cap-bound, ~2.2 KiB per accepted row (interim ~1.1 GiB at 500k rows)",
    "bench_scratch": "5 GiB under /tmp, never committed",
    "single_run_wall": "30 minutes",
    "target_db": "8 GiB VPS class"
  },
  "stop_conditions": [
    "RowCap/byte-cap refusal instead of unbounded growth",
    "3-strike provider retry budget then dead-letter",
    "p99 above 2x budget aborts the run",
    "scratch above ceiling aborts the run",
    "any invented, guessed or production-scraped datum aborts the campaign"
  ],
  "allowed_variants": ["baseline vs covering index", "spill caps", "1/8/32 clients (8 required for SLO language)"]
}
```

## 3. Worksheet (budgets provisional hypotheses until 8-client evidence)

| Scenario | p95 budget | p99 budget | Pilot measured |
|---|---|---|---|
| W-parse-100k | 500 ms wall | 1 s wall | 310 ms (Rust) |
| W-parse-1M | 30 s wall | 60 s wall | 3.9 s parse (raised caps) |
| W-city-dense | 100 ms | 250 ms | 49.2 ms indexed at 1M |
| W-city-sparse | 10 ms | 25 ms | 3.1 ms at 1M |
| W-nearby-dense | 150 ms | 300 ms | 44.2 ms at 1M |
| W-nearby-sparse | 5 ms | 10 ms | 0.3 ms at 1M |

Parser RSS has no SLO: streaming emission is owed first (RST-07).
Recovery target: crash mid-run replays to identical bytes with no
partial publication (proven for staging/prep/discovery in
RST-03/05/08); production recovery drills belong to the campaign.

## 4. Exit and next task

A reviewer reproduces any row above from the workload ID plus seed and
decides pass/fail from the frozen budgets without inventing
thresholds. Next (smallest, separately authorized): RST-11 datasets
and geography.

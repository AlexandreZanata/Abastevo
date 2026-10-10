# RST-07 — Benchmark and index experiment

Status: MEASURED — no production tuning beyond one adopted index, no
partitioning, no national fetch. Date: 2026-10-09. Scope:
`RUST_STATION_INGESTION_PLAN.md` RST-07. Language: English document; user
communication in Portuguese.

Hardware: Intel i7-13620H workstation, disposable PostGIS 18.6/3.6.4,
Go 1.27 toolchain, Rust 1.99 release builds. All inputs synthetic and
deterministic (seed 7); nothing below describes the real ANP census,
whose size was not measured (national fetch out of scope).

## 1. Parse: Rust vs Go on the shared 100k snapshot

Generator `tools/station-prep/src/bin/bench_generate.rs` emits raw and
normalized layouts from one RNG sequence, so both implementations must
accept the same CNPJ multiset. Driver `bench_parse.rs` prints counts,
rows/sec and the multiset checksum; `BenchmarkStageCSV100k` asserts the
same accounting and logs its own checksum.

- Rust: 100k rows in 310 ms (322k rows/s), RSS 214 MiB; 1M rows
  in 3.9 s parse (259k rows/s), RSS 2.19 GiB with raised caps
  (defaults refuse past 500k rows by design).
- Go `StageCSV`: 100k rows in ~0.7–1.0 s (124–152k rows/s), heap
  8.2 MiB, accounting 98004/1996/0 identical to Rust.
- Multiset checksum equal on both sides:
  `8e493e7d83a46e6addace8e1dc8c86f14254ad8fa0636e96aefdc2c1e048128a`.
  The checksum recipe itself is pinned by a unit golden test on both
  sides (SIMP-0001 → `6f15da0b…4e75fe23`).

Finding: the in-memory accepted-row model costs ~2.2 KiB/row, so the
provisional 256 MiB target cannot hold national snapshots without
streaming emission (no retained batch). Interim bound stands on the
default caps (~1.1 GiB at 500k rows, inside the 4 GiB worker); the
streaming follow-up is recorded, not implemented here.

## 2. Reads: 100k and 1M census-shaped stations

`BenchmarkCityReads100k/1M` seed skewed reviewed stations (70% in 5
dense cities, cluster near the dense query center) and drive the exact
app path (`Reader.Search`/`Nearby`, limit 20) for dense/sparse city and
dense/sparse nearby workloads, baseline indexes versus candidate
`(state, municipality_code, status, id)`.

100k (seed 1.6 s, table 26.4 MiB, indexes 11.4 MiB):

- Baseline p95: city-dense 8.56 ms, city-sparse 1.40 ms,
  nearby-dense 5.05 ms, nearby-sparse 0.12 ms.
- Candidate: no benefit (planner keeps the existing index; +5.6 MiB).

1M (seed 16.8 s, table 259.2 MiB, indexes 108.9 MiB):

- Baseline p95: city-dense 83.71 ms, city-sparse 3.56 ms,
  nearby-dense 44.23 ms, nearby-sparse 0.34 ms.
- Candidate p95: city-dense 49.20 ms (−41%, planner picks a parallel
  index-only scan), others within noise; index build 1.7 s, +56 MiB.

Decision: adopt the composite covering index (migration 000051) and
reject table partitioning — reads are single-digit ms at 100k and
inside targets at 1M, so partitions would buy complexity without
material benefit while threatening global CNPJ uniqueness and stable
foreign keys. Sparse workloads stay sub-2 ms at both scales.

## 3. Revised laboratory targets (synthetic + VPS headroom, db 8 GiB)

- Parser: streaming emission required before national snapshots;
  interim cap-bound RSS ~1.1 GiB at 500k rows (worker fits, lab ideal
  does not — follow-up owed, no new claim).
- City first page p95 ≤ 100 ms: holds at 1M baseline (83.7 ms), 49 ms
  indexed. Nearby p95 ≤ 150 ms: holds (44 ms at 1M). Sparse reads
  stay trivially inside both.
- Reproduce: `bench_generate 100000 7{,+normalized}` then
  `bench_parse`, `go test -tags=integration -bench 'BenchmarkStageCSV100k|BenchmarkCityReads(100k|1M)' -benchtime 1x -v`.

## 4. Limits and next task

- Numbers cover synthetic shapes on one workstation; production needs
  its own bounded smoke before any SLO language.
- Next (smallest): RST-08 incremental operations — bounded discovery
  schedule, conditional downloads, backoff/circuit stop and crash
  recovery with every failure tested now.

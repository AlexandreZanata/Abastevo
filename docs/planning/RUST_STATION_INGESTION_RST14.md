# RST-14 — Database construction and incremental load baseline

Status: MEASURED — isolated disposable PostGIS only; two minimal
contract fixes, no migration, no new dependency, no publication
change. Date: 2026-10-09. Scope:
`RUST_STATION_BENCHMARK_PLAN.md` RST-14, dependencies RST-13 and
verified RST-05. Language: English document; user communication in
Portuguese.

B-BR/BUC: B-BR-RST-P01 (oracle counts agree on every staged run),
B-BR-RST-P03 (failures stay failed and visible, never partial),
B-BR-RST-P04 (fresh disposable database per case, dropped after).
Serves BUC-RST-P01 (construction cost baseline) and BUC-RST-P05
(repeatable per-row costs for regression watch).

## 1. Contract findings (fixed, minimally)

Running the real RST-13 chain against the owned loader exposed two
mismatches; both are fixed without weakening any gate:

- Go `BatchManifest` rejected real emits (`unknown field
  "started_at"`). Added optional `started_at`/`ended_at`
  provenance (recorded, never a staging decision) so pre-existing
  manifests keep loading.
- No duplicate-bearing batch could ever pass loader accounting:
  station-prep dropped exact repeats at parse while the loader
  requires `staged rows == accepted+duplicates` to re-verify dedup
  independently. Parse now retains one byte-identical repeat per
  exact repeat (`RegistryBatch.duplicates`, `PmqcBatch.duplicates`,
  resolved post-link so twins match and the multiset stays
  order-independent) and emission carries the full multiset.
  Loader verification is unchanged and strictly stronger for it.

## 2. What was added

- Committed inputs `contracts/testdata/station-prep/datasets/`:
  `emit-tiny/` (30-row CSV → 24+2+4 registry, 24 PMQC candidates,
  fixed run UUID) and `emit-tiny-delta1/` (one changed row, new
  run UUID, edition 2). Regenerate deterministically with
  `bench_datasets` + `bench_stages --run-id <uuid>` to scratch.
- `construction_bench_test.go` (integration, `-race` clean):
  per-stage timing (create-db, migrate, `ValidateBatch`, `LoadBatch`,
  explicit `ANALYZE`) plus WAL bytes (`pg_wal_lsn_diff`),
  table/index bytes, live/dead tuples and xact commits around each
  load. Matrix: init, empty, tiny, replay, 4-way concurrent,
  edition-change/stale, 20k large (env-gated), probe index build.
- Publication/profile stages: NOT_AVAILABLE (staging never
  publishes; no tested station-prep→canonical publication exists),
  so build-to-query-ready here means build-to-staged-ready (load
  complete + live-count agreement + ANALYZE). `pg_stat_statements`
  is absent on the lab server: statement-level profiling stays
  NOT_AVAILABLE, not zero. Concurrent live-index creation is
  NOT_APPLICABLE (no shared traffic in lab).

## 3. Measurements (fresh disposable PostGIS per case, `-benchtime 1x`)

- Init: create ~17–101 ms, migrate ~356–569 ms, pool ~0 ms.
- Empty: validate 0.1 ms, load 7.5 ms, ANALYZE 1.7 ms, WAL
  4,944 B; per-row costs undefined (zero denominator, reported
  as such, never zero-filled).
- Tiny (48 accepted: 24 registry + 24 PMQC): validate 0.7 ms,
  load ~31–35 ms, ANALYZE ~2.8 ms, WAL ~68 KB, table Δ90 KB,
  index Δ41 KB → ~0.7 ms / ~1,422 WAL-bytes per accepted row.
  Replay converges in ~1.2–1.4 ms on stable run ids; 4 duplicate
  loaders converge in ~6.2 ms (owned pattern: load once, reload
  concurrently — cold-concurrent first loads can read a running
  snapshot and must re-read).
- Large 20k (24,552 staged): validate 146 ms, load 11.6 s,
  ANALYZE 232 ms, WAL 20.9 MB, table Δ12.8 MB, index Δ4.9 MB →
  ~0.5 ms / ~852 WAL-bytes per accepted row; xact Δ23,534 ≈ one
  commit per row, confirming the implemented row-at-a-time
  strategy (no COPY adapter exists; adding one is a separate
  tested task, not a measurement tweak).
- Edition-change: delta edition completes (~30 ms); the older
  edition then fails `count_mismatch` in ~4 ms with a visible
  `failed` run state, unchanged complete-run count and no pmqc
  staging (fail-fast). Global `(source, source_key, checksum)`
  dedup means incremental editions cannot restage unchanged rows:
  delta support needs a separate loader task.
- Probe index on 24.5k staged rows: fresh-lab build 30 ms,
  2.6 MB, drop 0.8 ms.
- Caveat: xact/tuple counters are sampled post-run, so short
  runs undercount stats-flush lag (tiny xact Δ1 vs 23,534 at
  20k); WAL lsn deltas are exact.

## 4. Regression checks and next task

- Existing suites green with the manifest change: Go directory
  unit tests, all 6 `TestLoadBatchIntegration*` with `-race`
  (empty/upgrade/concurrent/tampered/retained-ids/restricted-role),
  full Rust gate (12 lib + 40 integration incl. 5 stages cases
  pinning the duplicate multiset). No migration touched; no
  behavior change outside the two contract fixes.
- Reproduce: `RST14_EMIT_DIR_20K=/tmp/rst14-20k go test
  -tags=integration -run NONE -bench BenchmarkConstruction
  -benchtime 1x ./internal/modules/directory/adapters/registry/`
  (tiny runs hermetically; 20k needs a prior emit to scratch).
- Next (smallest, separately authorized): RST-15 city search,
  pagination and traffic distribution over the staged catalog.

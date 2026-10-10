# RST-11 — Representative datasets and geographic request distributions

Status: IMPLEMENTED — offline synthetic generation only; no network,
no database, no national fetch. Date: 2026-10-09. Scope:
`RUST_STATION_BENCHMARK_PLAN.md` RST-11, dependency RST-10
(PROTOCOL_FROZEN). Language: English document; user communication in
Portuguese.

B-BR/BUC: B-BR-RST-P02 (reproducible inputs: pinned seeds, byte
hashes, oracle counts), B-BR-RST-P05 (fair geographic coverage:
uniform/proportional/skew/hot traces with empty strata reported),
B-BR-RST-P06 (useful evidence: machine-readable oracle/trace JSON,
no invented rows). Serves BUC-RST-P01 (clean full import inputs) and
BUC-RST-P05 (regression oracle).

## 1. What was added

- `tools/station-prep/src/datasets.rs` (`station-datasets-v1`):
  deterministic generator over a frozen 200-city universe
  (`CIDADE 000..199`, IBGE `3550000+i`, UF SP) plus 5 zero-row
  cities (`VAZIA 000..004`, IBGE `3559900+i`). Same xorshift64* RNG
  and checksum-valid CNPJ recipe as the RST-07 pilot, with
  `[RST11-TEST]` markers so the multiset hash differs by label while
  accounting stays comparable.
- Frozen profiles: `tiny` (30 rows, seed 11, 5 cities, all knobs on),
  `representative` (20k, seed 7, RST-07 skew), `stress-100k` and
  `stress-1M` (seed 7, scratch only, never committed). Independent
  knobs: duplicate/conflict percentages, every-N quarantine
  injection (alternating invalid-CNPJ / unknown-city), every-N long
  fields, every-N missing sidecar locations, changed/no-op delta
  editions (`delta_edition`), history cardinalities 1/10/100 as
  oracle counts over the explicit unique-station denominator.
- Independent oracle per dataset: input/accepted/duplicates/
  quarantined reconciliation, unique CNPJs, per-city histogram,
  multiset checksum (bench_parse recipe), CSV SHA256, history and
  location counts. Sidecar locations are labelled `synthetic` with
  no reviewed flag by construction; missing points are explicit.
- Frozen request traces (`W-<profile>-<uniform|proportional|
  skew-80-20|hot-90>`): largest-remainder apportionment summing
  exactly to the requested total; pool shares (80/20, 90/10)
  independent of universe size; empty cities listed with zero
  stations, never invented. Strata `empty/1–10/11–100/>100`.
- `src/bin/bench_datasets.rs`: emits CSV plus `--aliases`,
  `--oracle` and `--trace` artifacts to scratch. The RST-07
  `bench_generate`/`bench_parse` pair is untouched.
- Committed correctness set
  `contracts/testdata/station-prep/datasets/`: `tiny.csv`,
  `tiny-aliases.json`, `tiny-oracle.json`,
  `trace-uniform-1000.json`.
- Tests: 5 unit + 10 integration (`tests/datasets.rs`), including a
  committed-fixture pin (regeneration reproduces committed bytes;
  real parser agrees with the on-disk oracle).

## 2. Evidence

- `cargo fmt --check`, `cargo clippy --all-targets -- -D warnings`,
  `cargo test --locked`: 12 lib + 35 integration cases pass
  (parser 9, join 9, deltas 9, datasets 10, plus unit/doc bins);
  `cargo audit`: 25 locked crates, no findings.
- Tiny oracle (frozen): input 30, accepted 24, duplicates 2,
  quarantined 4 (2 invalid-CNPJ + 2 unknown-city), unique 24,
  checksum
  `ebe47c28b9cc0d0601bc042c36beae62bb4145f1f29edd36f07220b820e24844`,
  CSV SHA256
  `507cf466ccbd6c71f5f1f743aeb91be068b662ef5c35582c095c50b6013bedd9`
  (4973 bytes). History 24/240/2400; locations 20 present + 4
  missing. Reordered input parses to identical counts/checksum.
- Representative 20k (seed 7, debug, 0.126 s):
  19552/430/18, checksum
  `b6fb02070bf1a681d76462adcbcc557fb1feb6dde09560158d52563224353f64`.
- Stress-100k (seed 7, debug, 0.695 s): 98004/1996/0 — identical
  accounting to the RST-07 pilot, confirming the shared shape
  family (multiset hash differs by `[RST11-TEST]` labelling only).
- Observed skew (reported, not tuned): 20k rows populate 27/200
  cities, densest holds ~50%; empty/unpopulated strata stay listed
  with zero stations. Uniform trace covers zero-row cities
  (500/1000 requests to empties at 10 cities); proportional keeps
  explicit zero weight for them.
- Reproduce: `cargo run --locked --bin bench_datasets --
  tiny /tmp/tiny.csv --aliases /tmp/aliases.json --oracle
  /tmp/oracle.json --trace uniform 1000 /tmp/trace.json`.

## 3. Limits and next task

- No DB seeding, load harness, or measured query latency here;
  those belong to RST-12/14 with their own evidence. The 1M profile
  is frozen but not executed in this task. PMQC assay history is
  represented as oracle cardinalities only. No contributor GPS,
  production photos, credentials or live sources anywhere.
- Next (smallest, separately authorized): RST-12 measurement
  harness and instrumentation qualification.

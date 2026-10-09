# RST-03 — Determinism, deltas and replay

Status: IMPLEMENTED — library only; no network, no database, no CLI.
Date: 2026-10-09. Scope: `RUST_STATION_INGESTION_PLAN.md` RST-03 on top
of the RST-02 parser. Language: English document; user communication in
Portuguese.

## 1. What was added (`tools/station-prep 0.2.0`)

- `src/output.rs` — deterministic emission: sorted JSONL streams
  (`assertions.jsonl`, `candidates.jsonl`, `quarantine.jsonl`) plus a
  hashed `station-batch-v1` manifest (inputs with SHA256/bytes/rows,
  parser/policy versions, alias-reference digest, per-input counts,
  completeness, per-output hashes). `write_outputs` publishes through
  temp-file + rename, so a failure leaves no partial final file.
- Spill sort: past `spill_rows` pairs per stream, length-prefixed runs
  spill to caller-provided scratch space and k-way merge back in key
  order; run files are removed on success. `usize::MAX` keeps the pure
  in-memory path. Parse batches stay bounded by `Limits::max_rows` by
  contract; the spill seam is exercised, not speculative, and RST-07
  measures when lowering the cap pays.
- `src/replay.rs` — `replay_decision` over manifests: identical bytes
  under identical parser/policy/alias versions are `Noop`; changed
  bytes at a current edition are `Publish`; changed bytes at a
  regressed `edition_seq` are `Stale`. Same bytes relabeled at another
  edition re-evaluate idempotently.
- Shared `sha256_hex` helper; alias tables carry their reference
  digest; crate version 0.2.0 with `PARSER_VERSION
  station-prep-v0.2.0`. No new dependency (lockfile unchanged apart
  from the crate bump).

## 2. Determinism contract (precise)

- Accepted/candidate streams are byte-identical across input reorder
  and across spill/no-spill paths; manifest output hashes match with
  them. The manifest binds exact input bytes, so reordered sources keep
  distinguishable input fingerprints with identical counts.
- Quarantine locators are positional by design (`file:logical-row`);
  reordered inputs therefore compare by quarantined content
  `(reason, detail, source_key)`, which is identical.
- A golden test pins one production-shaped row checksum
  (`6f15da0b…4e75fe23`): changing the canonical form changes every
  checksum and breaks the build on purpose.

## 3. Evidence (revision `e8d6905` + working tree)

- RED → GREEN: new `tests/deltas.rs` (9 cases) failed first on missing
  `output`/`replay` modules, then caught two real defects: sort keys
  containing `\x1f` colliding with the run-file separator (fixed with
  length-prefixed framing) and the positional-locator overclaim above
  (fixed by testing content equivalence). Final: `cargo test --locked`
  — 4 unit + 9 parser + 9 deltas passed.
- Cases: golden checksum, reorder equivalence, corrected bytes (same
  CNPJ, new checksum, `Publish`), unchanged replay (`Noop`), older
  edition (`Stale`), spill byte-equivalence with run cleanup,
  interrupted publish with no partial files, manifest roundtrip with
  counts matching written files, stable alias digest.
- `cargo fmt --check` clean; `cargo clippy --all-targets -- -D
  warnings` clean; `cargo audit` (1296 advisories, 25 crates): no
  findings. No network/process/`unsafe` in `src/`.

## 4. Limits and next task

- Cross-batch supersession merge stays the Go loader's job (RST-05);
  here corrections surface as new checksums plus `Publish`.
- Boundary-exact truncation remains the loader's manifest-count check.
- Next (smallest): RST-04 coordinate candidates — PMQC join/dedup
  hardening, CRS provenance and stale/conflicting/centroid cases with
  shared Go/Rust fixtures; still no automatic `reviewed` flag.

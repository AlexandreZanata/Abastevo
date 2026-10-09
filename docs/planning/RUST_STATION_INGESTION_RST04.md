# RST-04 — Coordinate-candidate join and hardening

Status: IMPLEMENTED — library only; no network, no database, no CLI.
Date: 2026-10-09. Scope: `RUST_STATION_INGESTION_PLAN.md` RST-04 on top
of the RST-03 crate. Language: English document; user communication in
Portuguese.

## 1. What was added (`tools/station-prep 0.2.0`, no version bump)

- `src/join.rs` — `join_candidates` anchors every pointed PMQC
  candidate at its exact establishment CNPJ from the accepted registry
  batch and triages it into `current`, `predates_evidence`,
  `stale_observation`, `conflicting_point` or `registry_missing`.
  Pointless candidates skip the join; unmatched CNPJs stay candidates
  without an anchor (missing rows never imply closure).
- Triage rules (documented heuristics, never verdicts): group conflict
  past 1000 m keeps every point of the group (haversine, R = 6 371 000
  m); observations older than 1095 days before the newest registry
  evidence stay stale (priority over predated); observations predating
  the anchor publication stay `predates_evidence`; coarse precision
  (fewer than 3 decimals on either axis, shortest round-trip form)
  flags without changing the verdict. Unparsable dates skip
  date-based states without failing the join.
- Emitted `station-joined-candidate-v1` rows keep the original CRS,
  `accuracy_m: null` and `review_state: pending`; joined-stream
  manifest integration belongs to the Go loader (RST-05).
- New shared fixture `contracts/testdata/station-prep/
  pmqc-join-sample.csv` (6 rows: conflicting pair, registry-missing
  CNPJ, predated evidence, coarse point, stale observation) plus its
  `manifest.json` entry. Frozen RST-01 files and hashes are untouched.

## 2. Evidence (revision `d49ac7d` + working tree)

- `cargo test --locked` — 7 unit (incl. date math, shortest-form
  decimals, haversine band) + 9 parser + 7 join + 9 deltas passed.
  Join fixture evaluates 6 = 1 current + 1 predates + 1 stale + 2
  conflicting + 1 missing; reordered join input yields identical JSONL;
  no joined row promotes location (CRS kept, accuracy null, pending).
- `cargo fmt --check` clean; `cargo clippy --all-targets -- -D
  warnings` clean; `cargo audit` (1296 advisories, 25 crates): no
  findings. No network/process/`unsafe` in `src/`.

## 3. Limits and next task

- True centroid-vs-site proof needs licensed parcel references and the
  tested CRS transform owner (RST-06); decimal heuristics stay triage.
- Cross-batch supersession merge stays the Go loader's job (RST-05).
- Next (smallest): RST-05 owned Go loader — manifest/checksum/schema
  validation, bounded staging loads and immediate real-PostGIS
  recovery/concurrency tests through existing registry ownership.

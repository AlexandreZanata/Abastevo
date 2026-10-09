# RST-02 — Pure Rust streaming parser over local files

Status: IMPLEMENTED — parser library only; no network, no database, no
CLI, no migration. Date: 2026-10-09. Scope:
`RUST_STATION_INGESTION_PLAN.md` RST-02 against the frozen RST-01
fixtures. Language: English document; user communication in Portuguese.

## 1. Crate (`tools/station-prep/`, `station-prep 0.1.0`)

- `src/lib.rs` — public surface: `parse_registry`, `parse_pmqc`,
  `AliasTable`, `Limits`, `Counts`, `RunState`, row/batch types.
- `src/cnpj.rs` — text validation byte-compatible with the Go kernel
  (`ParseCNPJ`): formatting stripped, letters uppercased, leading
  zeroes preserved, both check digits under the alphanumeric program.
- `src/municipality.rs` — bounded Latin fold (diacritics stripped,
  uppercase, whitespace collapsed) plus versioned alias table; unknown
  or ambiguous rows quarantine, never guessed.
- `src/registry.rs` — `;`-delimited streaming reader, frozen 13-column
  name matching, per-field byte cap, membership-as-evidence mapping,
  SHA256 canonical checksums, within-run dedup, succession links and
  stable `(source_key, checksum)` output order.
- `src/pmqc.rs` — sample-identity dedup, empty coords stay accepted
  without a point, zero/non-finite/out-of-bounds quarantine, mirrored
  pairs stay `suspect_swap` (never auto-swapped), original `EPSG:4674`
  with `accuracy_m: null` and `review_state: pending`.
- `src/types.rs` — bounds, reconciled counts, run states, frozen typed
  quarantine rows with `file:logical-record` locators.
- `tests/parser.rs` — 9 integration cases over the RST-01 fixtures;
  unit vectors live beside each module.

## 2. Contract deltas from RST-01 (evidenced, minimal)

1. Framing rule: a complete snapshot ends with a record terminator.
   The `csv` reader tolerates EOF inside quotes, so a cut mid-quote
   would otherwise decode as a shorter valid file; boundary-exact cuts
   stay the Go loader's manifest-count check (RST-05).
2. One reason added: `invalid_field` for malformed non-identity fields
   (empty business name, malformed UF, empty sample reference). All
   RST-01 reasons are unchanged.
3. Normalization is a bounded Latin fold, not full NFKD; it covers the
   fixture/reference sample and every IBGE Portuguese name class.
   Full NFKD awaits a measured need.
4. `staleness_note` is always empty from the parser; the pure parser
   has no clock and freshness review belongs to the Directory owner.
5. Succession: same CNPJ with several snapshots keeps every row; the
   newest publication carries `supersedes: null` and older rows link to
   its checksum. Older evidence never overwrites fresher facts.

## 3. Dependencies, MSRV, licences, audit

`Cargo.lock` is tracked. Direct pins: `csv 1.4.0`, `serde 1.0.229`
(derive), `serde_json 1.0.151`, `sha2 0.10.9`; 23 locked transitive
crates, all permissive (`MIT` / `Apache-2.0` / `Unlicense/MIT` /
`BSL-1.0` for `ryu`). `rust-version = "1.99"` (tested toolchain
`rustc/cargo 1.99.0`); no older MSRV is claimed. `cargo audit 0.22.2`
over 1296 advisories: no vulnerabilities, exit 0. No network, process
or `unsafe` code in `src/` (verified by search). No Tokio/reqwest/
parallel pools/PostgreSQL client/dataframe.

## 4. Evidence (RED → GREEN, revision `157d476` + working tree)

- RED: `cargo test` failed with no library source; then 3 implementation
  failures (borrowck in succession grouping, wrong test field paths,
  flawed reorder/truncation test framing) fixed with evidence.
- GREEN: `cargo test --locked` — 4 unit + 9 integration passed.
  Registry sample evaluates 12 = 7 accepted (one superseded history) +
  1 duplicate + 4 quarantined; PMQC 11 = 5 + 1 + 5. Reordered inputs
  yield byte-identical accepted JSONL; BOM strips; header variant
  quarantines the run; truncation fails with no partial output;
  emitted JSONL key sets equal the frozen samples.
- `cargo fmt --check` clean; `cargo clippy --all-targets -- -D
  warnings` clean. Existing Go registry contract tests re-passed in
  RST-01 and are untouched here.
- Measured peak RSS of the integration binary on synthetic fixtures:
  3.5 MiB (`/usr/bin/time -v`, max RSS 3548 kB). No national-scale or
  speedup claim; laboratory targets stay RST-07 business.

## 5. Next task: RST-03 (smallest)

Determinism and deltas on top of this parser: canonical hashing
policy shared with Go, external sorted spill/merge past the in-memory
cap, unchanged-replay no-op proofs, supersession across batches and
interrupted-output handling. Acceptance: reordered national-shaped
synthetic input yields identical output bytes, corrected bytes yield
dated corrections, older evidence never wins, and disk-spill behavior
is measured, not asserted.

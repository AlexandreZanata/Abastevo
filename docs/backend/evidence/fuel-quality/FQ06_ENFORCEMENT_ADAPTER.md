# FQ-06 — Enforcement adapter and legally accurate events

Status: LOCAL_DONE / INTEGRATION_PENDING. Date: 2026-10-14.
Base: `codex/fuel-quality-plan` atop `origin/dev 08a54dc4`. Depends:
FQ-03/04; official enforcement labels stay exact. B-BR-FQ-02/06,
BUC-FQ-05. No guilt, liability, finality or current-legal-status
semantics anywhere in this task. Publication reuses the FQ-04b event
ledger path unchanged.

## Behavior before implementation

The crate prepared coordinates and assays only: fiscal-action input had
no parser, no native-key identity and no extraction stamping.

## Delivered

- `tools/station-prep/src/enforcement.rs` (new): `parse_enforcement`
  for the frozen `enforcement-v1` input contract
  (`ACAOID;CNPJALVO;DATAOCORRENCIA;ESCOPO;SITUACAO;DATAEXTRACAO;EVIDENCIA`).
  Raw ANP enforcement columns remain unverified upstream, so this shape
  is synthetic until FQ-05 binds a verified edition.
- Label-preserving emission (`enforcement-event-v1` JSONL): scope and
  status keep source case and accents byte-identical; the parser maps no
  vocabulary and derives nothing. Required native key, CNPJ, scope,
  status, extraction date and evidence; occurred date optional.
  Malformed dates, empty labels and bad CNPJs quarantine with typed
  reasons; strict header match, BOM/encoding/limits/truncation handling
  and shared framing rules mirror the assay reader.
- Distinct native keys against one CNPJ stay distinct; same-key content
  changes (e.g. autuado→interditado) are retained as correction pairs
  for downstream revision supersession, never rewritten. Exact repeats
  collapse to counted twins with loss accounting.
- Every row carries its extraction date plus a derived `YYYY-MM` period
  label (empty for dateless rows), so content/period mismatches stay
  visible evidence instead of refused rows.
- `contracts/testdata/station-prep/enforcement-sample.csv` (12-row
  matrix: 12/6/1/5) and `tools/station-prep/tests/enforcement.rs`
  (12 cases): counts/reasons/locators, verbatim labels, correction
  pair, period mismatch + dateless rows, machine-checked absence of
  guilt semantics and frozen key set, reorder stability, BOM/encoding/
  truncation/bounds/header variants.
- Determinism fix shared with the assay reader: sort keys extended with
  content fields after the reorder test exposed order-dependent
  correction pairs; a matching assay correction-pair test now pins the
  same guarantee.

## Validation

RED→GREEN: first run 11/12 — reordered input flipped the same-key
correction pair (stable sort with equal identity keys). Fixed with
content tiebreakers in both readers; rerun green plus clippy-driven
`format!`→`to_string` cleanup in the new assay test.

- `cargo +1.99.0 test --locked --manifest-path tools/station-prep/Cargo.toml`:
  all 15 targets green — enforcement 12/12, quality 15/15, parser 15/15
  (coordinate/registry byte-compatible), remainder unchanged.
- `cargo +1.99.0 fmt --check` and `clippy --all-targets -- -D warnings` clean.
- `git diff --check` clean; new-file Markdown links resolve;
  scoped secret/PII scan clean (synthetic fixture CNPJs only).
- Go ledger untouched this round; official price/label behavior unchanged.

## Recovery

Disable the new adapter/output (`enforcement` module + event emission);
withdraw offending rows through revision supersession while retaining
the prior publication. Raw-schema drift blocks only the FQ-05 edition
binding. Next: FQ-07 sanction snapshots and extraction-time status.

## Runtime reconciliation 2026-10-17 (FQ-05W8)

This checkpoint certifies the Rust adapter contract and fixtures only: no
Go runtime stream exists yet for enforcement content. There is no
`prepare_enforcement` CLI binary in `tools/station-prep/src/bin`, no
file-import/discovery handler composed for the `enforcement` source (the
native worker registers only `pmqc`), and no verified live resource
inventory beyond the S-FQ-02 page returning 200. The Go typed publication
path is proven separately with real PostGIS (`event_publisher_integration_test.go`,
FQ-04b) but the plan's FQ-06 acceptance is not met: a live Go source
stream and complete enforcement history remain owed before any runtime or
coverage claim. No score, coverage or public enforcement history follows
from this adapter checkpoint.

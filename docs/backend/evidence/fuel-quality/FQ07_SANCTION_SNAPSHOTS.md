# FQ-07 — Sanction snapshots and extraction-time status

Status: LOCAL_DONE / INTEGRATION_PENDING. Date: 2026-10-14.
Base: `codex/fuel-quality-plan` atop `origin/dev 08a54dc4`. Depends:
FQ-04. B-BR-FQ-07, BUC-FQ-06. Status is extraction-time only and amounts
are never aggregated: no appeal, finality, guilt or current-legal-status
semantics anywhere in this task. Staging reuses the FQ-04b sanction
ledger path unchanged.

## Behavior before implementation

The crate prepared coordinates, assays and enforcement actions only:
fine/proceeding snapshots had no parser, no proceeding identity and no
extraction stamping.

## Delivered

- `tools/station-prep/src/sanction.rs` (new): `parse_sanctions` for the
  frozen `sanction-v1` input contract
  (`MULTAID;PROCESSO;CNPJPATE;VALOR;SITUACAO;DATADECISAO;DATAEXTRACAO;EVIDENCIA`).
  Raw ANP sanction columns remain unverified upstream, so this shape is
  synthetic until FQ-05 binds a verified edition.
- Snapshot emission (`sanction-event-v1` JSONL): proceeding and status
  verbatim, extraction date required, decided date optional, period
  label from the decided year falling back to the extraction year.
  Amounts keep raw text plus strictly parsed minor units for the frozen
  `D{1,3}(.DDD)*,DD` shape; empty stays null, anything else quarantines
  instead of parsing loose. No totals, conversions or math of any kind.
- One proceeding across annual snapshots stays distinct rows with their
  own extraction stamping; same-key amount/status changes are retained
  as correction pairs for downstream revision supersession; exact
  repeats collapse to counted twins. Revoked statuses store as stated
  without voiding siblings or gaining meaning. Staleness is disclosed
  through `as_of` on every row; age math belongs to readers (FQ-08).
- Shared framing rules with the sibling readers: strict header match,
  BOM/encoding/limits/truncation handling, deterministic content
  tiebreakers, manifest hash over emission plus counts.
- `contracts/testdata/station-prep/sanction-sample.csv` (12-row matrix:
  12/6/1/5) and `tools/station-prep/tests/sanction.rs` (14 cases):
  counts/reasons/locators, cross-snapshot proceeding, amount and status
  correction pairs, verbatim revocation, null amount with fallback
  period, machine-checked absence of math/appeal semantics and frozen
  key set, reorder stability, header-only empty completion, BOM/
  encoding/truncation/bounds/header variants.

## Validation

RED→GREEN: first run quarantined the whole fixture — the sample header
spelled `CnpjParte` while the frozen contract requires `CNPJPATE`. The
strict header match worked as designed; the fixture was fixed, no code
changed, rerun 14/14.

- `cargo +1.99.0 test --locked --manifest-path tools/station-prep/Cargo.toml`:
  104 passed / 0 failed across all 15 targets (sanction 14/14;
  coordinate/registry/assay/enforcement suites byte-compatible).
- `cargo +1.99.0 fmt --check` and `clippy --all-targets -- -D warnings` clean.
- `git diff --check` clean; new-file Markdown links resolve;
  scoped secret/PII scan clean (synthetic fixture CNPJs only).
- Go ledger untouched this round; official price/label behavior unchanged.

## Recovery

Disable the new adapter/output (`sanction` module + event emission);
withdraw offending rows through revision supersession and re-derive
snapshots without mutating source history. Raw-schema drift blocks only
the FQ-05 edition binding. Next: FQ-08 official read API.

## Runtime reconciliation 2026-10-17 (FQ-05W8)

As with FQ-06, this checkpoint certifies the Rust snapshot contract and
fixtures only. No `prepare_sanction` CLI binary, no Go file-import or
discovery handler for the `sanctions` source, and no verified live
resource inventory exist yet; the Go typed publication path is proven
separately with real PostGIS (FQ-04b). A live Go source stream and
complete enforcement/sanction history remain owed before any runtime or
coverage claim; the plan's FQ-07 acceptance is not met by the adapter
checkpoint alone.

# FQ-02 — Lossless PMQC quality preparation

Status: LOCAL_DONE / INTEGRATION_PENDING. Date: 2026-10-14.
Base: `codex/fuel-quality-plan` atop `origin/dev 08a54dc4`. Depends: FQ-01.
BUC-FQ-01 contracted. No change to the coordinate/location stream, loader
artifacts, migrations, API or Android.

## Behavior before implementation

The crate parsed only the five-column coordinate extract: any assay-level
input quarantined the whole run as `header_mismatch`, and no sample/assay
identity, fuel mapping or manifest hash existed on the preparation side.

## Delivered

- `tools/station-prep/src/quality.rs` (new): `parse_quality_assays`
  for the frozen `pmqc-assay-v1` input contract
  (`AMOSTRAID;CNPJPOSTO;DATACOLETA;PRODUTO;ENSAIO;RESULTADO;UNIDADE;CONFORMIDADE`).
  Raw ANP assay columns remain unverified upstream, so this preparation
  shape is recorded as synthetic until FQ-05 binds a verified edition.
- Separate `quality-assay-v1` JSONL emission: native result text
  (decimal commas, comparators, free text, quoted delimiters) and unit stay
  verbatim; fuel and conformity hold canonical values only.
- Bounded parsing/sorting: shared `Limits` (oversize/row-cap/truncation/
  encoding fail with no partial output), BOM stripped, strict header match
  (missing/unknown/repeated columns quarantine the run), per-row field-count
  and field-size guards, deterministic sort by
  station/date/sample/assay, exact-duplicate collapse with counted twins.
- Fuel map mirrors the FQ-01 kernel vocabulary (7 verified ANP labels);
  premium gasoline and any other unknown label quarantine as
  `unmapped_fuel`, never coerced. Conformity accepts case/accent variants
  of CONFORME / NÃO CONFORME / INCONCLUSIVO; anything else quarantines as
  `unknown_conformity` — two additive `reason` constants, no existing
  reason changed.
- `QualityBatch::manifest_hash` binds deterministic emission plus counts,
  so reimports and republished editions cannot silently change support.
- `contracts/testdata/station-prep/quality-assay-sample.csv` (12-row
  synthetic matrix) and `tools/station-prep/tests/quality.rs` (14 cases):
  counts reconcile (12/7/1/4 with exact reasons/locators), repeated vs
  distinct assays, verbatim results, verbatim odd unit, lowercase-accented
  conformity, same-day sample separation, reorder-identical output and
  manifest hash, hash sensitivity, BOM, bad encoding, truncation, three
  bound enforcements, three header variants, short-row quarantine,
  unmapped premium quarantine.

## Validation

RED→GREEN: first run 13/14 — the repeated-header test used a
case-mismatched column name so no replacement happened and the header
parsed clean (test bug, fixed to the uppercase contract header); rerun
14/14. Clippy then refused `assay_checksum`'s eight parameters
(`too_many_arguments`); refactored to a slice without behavior change.

- `cargo +1.99.0 test --locked --manifest-path tools/station-prep/Cargo.toml`:
  all suites green — quality 14/14, parser 15/15 (coordinate/registry
  outputs byte-compatible), lib 12, datasets 11, deltas 9, join 8,
  operational 1; zero failures.
- `cargo +1.99.0 fmt --check` and `clippy --all-targets -- -D warnings` clean.
- `git diff --check` clean; new-file Markdown links resolve;
  scoped secret/PII scan clean (synthetic fixture CNPJs already used by
  existing samples; no credentials, precise GPS or media).
- Go `quality/domain` untouched this round (no Go changes).

## Recovery

Disable the new adapter/output (`quality` module + `quality-assay-v1`
emission); the coordinate stream, registry loader artifacts and all prior
outputs are independent and unchanged. Raw-schema drift blocks only the
FQ-05 edition binding, not this contract. Next: FQ-03 identity resolution
and review policy.

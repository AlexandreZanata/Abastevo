//! FQ-02 acceptance for the lossless `pmqc-assay-v1` preparation
//! stream. Every expectation below mirrors a frozen decision in
//! `docs/backend/evidence/fuel-quality/FQ02_QUALITY_PREP.md`: the sample
//! fixture carries repeated/distinct assays, quoted fields, decimal
//! commas, comparators, free text, an odd unit kept verbatim, an
//! unmapped fuel, an unknown conformity, a bad CNPJ and a missing assay
//! reference. The coordinate and registry streams are untouched: this
//! suite must stay green alongside the existing parser suite.

use station_prep::{parse_quality_assays, Limits, RunState};
use std::path::PathBuf;

fn dir() -> PathBuf {
    PathBuf::from(env!("CARGO_MANIFEST_DIR")).join("../../contracts/testdata/station-prep")
}

fn bytes(name: &str) -> Vec<u8> {
    std::fs::read(dir().join(name)).expect("fixture must exist")
}

const HEADER: &str =
    "AMOSTRAID;CNPJPOSTO;DATACOLETA;PRODUTO;ENSAIO;RESULTADO;UNIDADE;CONFORMIDADE\n";

fn sample() -> Vec<u8> {
    bytes("quality-assay-sample.csv")
}

#[test]
fn quality_sample_counts_reconcile() {
    let batch = parse_quality_assays("quality-assay-sample.csv", sample(), &Limits::default())
        .expect("sample must parse");
    assert_eq!(batch.state, RunState::Complete);
    assert_eq!(batch.counts.input, 12);
    assert_eq!(batch.counts.accepted, 7);
    assert_eq!(batch.counts.duplicates, 1);
    assert_eq!(batch.counts.quarantined, 4);
    // One sample with several assays is one sample: S-001 keeps both
    // distinct determinations, the exact repeat collapses to a duplicate.
    assert_eq!(
        batch
            .assays
            .iter()
            .filter(|row| row.sample_ref == "S-001")
            .count(),
        2
    );
    assert_eq!(batch.duplicates.len(), 1);
    assert_eq!(batch.duplicates[0].sample_ref, "S-001");
    assert_eq!(batch.duplicates[0].assay_ref, "ASPECTO");
    let mut reasons: Vec<&str> = batch.quarantine.iter().map(|row| row.reason).collect();
    reasons.sort();
    assert_eq!(
        reasons,
        vec![
            "invalid_cnpj",
            "invalid_field",
            "unknown_conformity",
            "unmapped_fuel"
        ]
    );
    let locators: Vec<&str> = batch
        .quarantine
        .iter()
        .map(|row| row.row_locator.as_str())
        .collect();
    for locator in [
        "quality-assay-sample.csv:9",
        "quality-assay-sample.csv:10",
        "quality-assay-sample.csv:11",
        "quality-assay-sample.csv:12",
    ] {
        assert!(locators.contains(&locator), "missing {locator}");
    }
}

#[test]
fn quality_result_text_stays_verbatim() {
    let batch = parse_quality_assays("quality-assay-sample.csv", sample(), &Limits::default())
        .expect("sample must parse");
    let result = |sample: &str, assay: &str| {
        batch
            .assays
            .iter()
            .find(|row| row.sample_ref == sample && row.assay_ref == assay)
            .expect("assay must be accepted")
            .result_text
            .clone()
    };
    // Decimal comma is never normalized to a point.
    assert_eq!(result("S-001", "DESTILACAO 50%"), "181,5");
    // Comparators stay part of the native text.
    assert_eq!(result("S-003", "ENXOFRE"), "<0,1");
    // Free text and quoted embedded delimiters survive intact.
    assert_eq!(
        result("S-004", "ODORIZACAO"),
        "nota: reter; reanalisar em 7 dias"
    );
}

#[test]
fn quality_unknown_unit_is_retained_not_refused() {
    let batch = parse_quality_assays("quality-assay-sample.csv", sample(), &Limits::default())
        .expect("sample must parse");
    let row = batch
        .assays
        .iter()
        .find(|row| row.sample_ref == "S-003")
        .expect("odd unit must not refuse the row");
    assert_eq!(row.unit, "mg/kg");
    assert_eq!(row.fuel, "DIESEL_S10");
    assert_eq!(row.conformity, "CONFORMING");
}

#[test]
fn quality_lowercase_accented_conformity_maps() {
    let batch = parse_quality_assays("quality-assay-sample.csv", sample(), &Limits::default())
        .expect("sample must parse");
    let row = batch
        .assays
        .iter()
        .find(|row| row.sample_ref == "S-004")
        .expect("row must be accepted");
    assert_eq!(row.conformity, "NONCONFORMING");
}

#[test]
fn quality_same_day_separate_samples_stay_distinct() {
    let batch = parse_quality_assays("quality-assay-sample.csv", sample(), &Limits::default())
        .expect("sample must parse");
    // S-001 and S-002 share date and station but never merge.
    assert!(batch.assays.iter().any(|row| row.sample_ref == "S-001"));
    assert!(batch.assays.iter().any(|row| row.sample_ref == "S-002"));
}

#[test]
fn quality_reordered_input_keeps_identical_output() {
    let raw = String::from_utf8(sample()).expect("sample is UTF-8");
    let mut lines: Vec<&str> = raw.lines().collect();
    let header = lines.remove(0);
    lines.reverse();
    let reordered = format!("{header}\n{}\n", lines.join("\n"));
    let first = parse_quality_assays("quality-assay-sample.csv", sample(), &Limits::default())
        .expect("sample parses");
    let second = parse_quality_assays(
        "quality-assay-sample.csv",
        reordered.into_bytes(),
        &Limits::default(),
    )
    .expect("reordered parses");
    let to_jsonl = |batch: &station_prep::QualityBatch| {
        batch
            .assays
            .iter()
            .map(|row| row.to_jsonl())
            .collect::<Vec<_>>()
            .join("\n")
    };
    assert_eq!(to_jsonl(&first), to_jsonl(&second));
    assert_eq!(first.manifest_hash(), second.manifest_hash());
    assert_eq!(first.counts, second.counts);
}

#[test]
fn quality_manifest_hash_changes_with_content() {
    let first = parse_quality_assays("quality-assay-sample.csv", sample(), &Limits::default())
        .expect("sample parses");
    let altered = String::from_utf8(sample())
        .expect("sample is UTF-8")
        .replacen("181,5", "182,0", 1);
    let second = parse_quality_assays(
        "quality-assay-sample.csv",
        altered.into_bytes(),
        &Limits::default(),
    )
    .expect("altered parses");
    assert_ne!(first.manifest_hash(), second.manifest_hash());
    assert_eq!(first.counts.accepted, second.counts.accepted);
}

#[test]
fn quality_bom_prefix_is_stripped() {
    let mut raw = vec![0xEF, 0xBB, 0xBF];
    raw.extend(
        format!("{HEADER}S-001;48151623000191;2026-09-04;ETANOL;TEOR;92,6;%;CONFORME\n")
            .into_bytes(),
    );
    let batch = parse_quality_assays("bom.csv", raw, &Limits::default()).expect("BOM must strip");
    assert_eq!(batch.state, RunState::Complete);
    assert_eq!(batch.counts.accepted, 1);
}

#[test]
fn quality_bad_encoding_fails_without_partial_output() {
    let mut raw = format!("{HEADER}S-001;48151623000191;2026-09-04;ETANOL;TEOR;").into_bytes();
    raw.extend([0xFF, 0xFE]);
    raw.extend(b";%;CONFORME\n");
    let err = parse_quality_assays("bad-encoding.csv", raw, &Limits::default());
    assert!(err.is_err(), "non-UTF8 must fail, got {err:?}");
}

#[test]
fn quality_truncated_input_fails_without_partial_output() {
    let raw = format!("{HEADER}S-001;48151623000191;2026-09-04;ETANOL;TEOR;92,6;%;CONFORME");
    let err = parse_quality_assays("truncated.csv", raw.into_bytes(), &Limits::default());
    assert!(err.is_err(), "missing terminator must fail, got {err:?}");
}

#[test]
fn quality_bounds_are_enforced() {
    let tiny = Limits {
        max_bytes: 10,
        ..Limits::default()
    };
    assert!(parse_quality_assays("quality-assay-sample.csv", sample(), &tiny).is_err());
    let few = Limits {
        max_rows: 2,
        ..Limits::default()
    };
    assert!(parse_quality_assays("quality-assay-sample.csv", sample(), &few).is_err());
    let narrow = Limits {
        max_field_bytes: 10,
        ..Limits::default()
    };
    let batch = parse_quality_assays("quality-assay-sample.csv", sample(), &narrow)
        .expect("narrow fields quarantine rows");
    assert!(
        batch
            .quarantine
            .iter()
            .any(|row| row.reason == "record_too_large"),
        "long native text must quarantine at 10 bytes"
    );
}

#[test]
fn quality_header_variants_quarantine_the_run() {
    // Missing column.
    let missing = HEADER.replace(";UNIDADE;", ";").into_bytes();
    let batch = parse_quality_assays("missing-col.csv", missing, &Limits::default())
        .expect("variant must report, not fail");
    assert_eq!(batch.state, RunState::Quarantined);
    assert_eq!(batch.error_code, "header_mismatch");
    assert_eq!(batch.counts.input, 0);
    assert_eq!(batch.quarantine[0].row_locator, "missing-col.csv:header");
    // Unknown extra column.
    let mut extra = HEADER.trim_end().to_string();
    extra.push_str(";LABORATORIO\n");
    let batch = parse_quality_assays("extra-col.csv", extra.into_bytes(), &Limits::default())
        .expect("variant must report, not fail");
    assert_eq!(batch.state, RunState::Quarantined);
    // Repeated column.
    let repeated = HEADER.replace("ENSAIO;", "ENSAIO;ENSAIO;").into_bytes();
    let batch = parse_quality_assays("repeated-col.csv", repeated, &Limits::default())
        .expect("variant must report, not fail");
    assert_eq!(batch.state, RunState::Quarantined);
}

#[test]
fn quality_short_row_quarantines_without_failing_the_run() {
    let raw = format!("{HEADER}S-001;48151623000191;2026-09-04;ETANOL;TEOR;92,6;%\n");
    let batch = parse_quality_assays("short-row.csv", raw.into_bytes(), &Limits::default())
        .expect("short row must quarantine, not fail");
    assert_eq!(batch.state, RunState::Complete);
    assert_eq!(batch.counts.input, 1);
    assert_eq!(batch.counts.quarantined, 1);
    assert_eq!(batch.quarantine[0].reason, "invalid_field");
}

#[test]
fn quality_unmapped_premium_fuel_quarantines() {
    // Premium gasoline has no verified assay-side product text: the row
    // quarantines with the reason instead of guessing a mapping.
    let raw =
        format!("{HEADER}S-001;48151623000191;2026-09-04;GASOLINA PREMIUM;ASPECTO;ok;-;CONFORME\n");
    let batch = parse_quality_assays("premium.csv", raw.into_bytes(), &Limits::default())
        .expect("premium row must quarantine, not fail");
    assert_eq!(batch.counts.quarantined, 1);
    assert_eq!(batch.quarantine[0].reason, "unmapped_fuel");
}

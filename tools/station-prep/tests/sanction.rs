//! FQ-07 acceptance for the extraction-time `sanction-v1` snapshot
//! stream. Every expectation below mirrors a frozen decision in
//! `docs/backend/evidence/fuel-quality/FQ07_SANCTION_SNAPSHOTS.md`: the
//! sample fixture carries one proceeding across two annual snapshots, an
//! exact repeat, amount and status correction pairs, a revoked status kept
//! verbatim, a null amount with fallback period, a bad CNPJ and four
//! invalid-field rows. Status is extraction-time only and amounts keep
//! raw text plus strictly parsed minor units: the suite asserts no
//! amount math and no appeal inference in the emission. Sibling streams
//! are untouched.

use station_prep::{parse_sanctions, Limits, RunState};
use std::path::PathBuf;

fn dir() -> PathBuf {
    PathBuf::from(env!("CARGO_MANIFEST_DIR")).join("../../contracts/testdata/station-prep")
}

fn bytes(name: &str) -> Vec<u8> {
    std::fs::read(dir().join(name)).expect("fixture must exist")
}

const HEADER: &str =
    "MULTAID;PROCESSO;CNPJPATE;VALOR;SITUACAO;DATADECISAO;DATAEXTRACAO;EVIDENCIA\n";

fn sample() -> Vec<u8> {
    bytes("sanction-sample.csv")
}

#[test]
fn sanction_sample_counts_reconcile() {
    let batch = parse_sanctions("sanction-sample.csv", sample(), &Limits::default())
        .expect("sample must parse");
    assert_eq!(batch.state, RunState::Complete);
    assert_eq!(batch.counts.input, 12);
    assert_eq!(batch.counts.accepted, 6);
    assert_eq!(batch.counts.duplicates, 1);
    assert_eq!(batch.counts.quarantined, 5);
    assert_eq!(batch.duplicates.len(), 1);
    assert_eq!(batch.duplicates[0].native_key, "MULTA-001");
    let mut reasons: Vec<&str> = batch.quarantine.iter().map(|row| row.reason).collect();
    reasons.sort();
    assert_eq!(
        reasons,
        vec![
            "invalid_cnpj",
            "invalid_field",
            "invalid_field",
            "invalid_field",
            "invalid_field"
        ]
    );
    let locators: Vec<&str> = batch
        .quarantine
        .iter()
        .map(|row| row.row_locator.as_str())
        .collect();
    for locator in [
        "sanction-sample.csv:8",
        "sanction-sample.csv:9",
        "sanction-sample.csv:10",
        "sanction-sample.csv:11",
        "sanction-sample.csv:12",
    ] {
        assert!(locators.contains(&locator), "missing {locator}");
    }
}

#[test]
fn sanction_proceeding_spans_snapshots() {
    let batch = parse_sanctions("sanction-sample.csv", sample(), &Limits::default())
        .expect("sample must parse");
    // One proceeding across two annual snapshots: two native keys, both
    // retained with their own extraction stamping. Snapshots accumulate;
    // nothing merges years.
    let pair: Vec<_> = batch
        .snapshots
        .iter()
        .filter(|row| row.proceeding_ref == "48000.001111/2024-11")
        .collect();
    assert!(pair.iter().any(|row| row.native_key == "MULTA-001"));
    assert!(pair.iter().any(|row| row.native_key == "MULTA-002"));
    let first = pair
        .iter()
        .find(|row| row.native_key == "MULTA-001" && row.amount_text == "150.000,00")
        .expect("2024 snapshot present");
    let second = pair
        .iter()
        .find(|row| row.native_key == "MULTA-002" && row.status == "aplicada")
        .expect("2025 snapshot present");
    assert_eq!(first.as_of, "2024-12-31");
    assert_eq!(second.as_of, "2025-12-31");
}

#[test]
fn sanction_correction_pairs_are_retained() {
    let batch = parse_sanctions("sanction-sample.csv", sample(), &Limits::default())
        .expect("sample must parse");
    // Same key, new amount: replacement evidence, not a duplicate.
    let amounts: Vec<_> = batch
        .snapshots
        .iter()
        .filter(|row| row.native_key == "MULTA-001")
        .map(|row| (row.amount_text.as_str(), row.amount_minor))
        .collect();
    assert!(amounts.contains(&("150.000,00", Some(15_000_000))));
    assert!(amounts.contains(&("175.000,00", Some(17_500_000))));
    // Same key, new status: retained with explicit correction evidence.
    let statuses: Vec<&str> = batch
        .snapshots
        .iter()
        .filter(|row| row.native_key == "MULTA-002")
        .map(|row| row.status.as_str())
        .collect();
    assert!(statuses.contains(&"aplicada"));
    assert!(statuses.contains(&"suspensa"));
}

#[test]
fn sanction_revoked_status_stays_verbatim() {
    let batch = parse_sanctions("sanction-sample.csv", sample(), &Limits::default())
        .expect("sample must parse");
    // A revoked proceeding is stored as stated: revocation neither voids
    // sibling rows nor gains any inferred meaning.
    let row = batch
        .snapshots
        .iter()
        .find(|row| row.native_key == "MULTA-004")
        .expect("revoked row must be accepted");
    assert_eq!(row.status, "revogada");
    assert_eq!(row.amount_minor, Some(7_550_050));
}

#[test]
fn sanction_null_amount_keeps_fallback_period() {
    let batch = parse_sanctions("sanction-sample.csv", sample(), &Limits::default())
        .expect("sample must parse");
    let row = batch
        .snapshots
        .iter()
        .find(|row| row.native_key == "MULTA-003")
        .expect("amountless row must be accepted");
    assert_eq!(row.amount_text, "");
    assert_eq!(row.amount_minor, None);
    assert_eq!(row.status, "em análise");
    // No decided date: the extraction year labels the period instead of
    // an invented date.
    assert_eq!(row.decided_on, "");
    assert_eq!(row.period_label, "2025");
}

#[test]
fn sanction_emission_computes_no_math_or_appeals() {
    let batch = parse_sanctions("sanction-sample.csv", sample(), &Limits::default())
        .expect("sample must parse");
    for row in &batch.snapshots {
        let jsonl = row.to_jsonl();
        for forbidden in [
            "total", "sum", "appeal", "recurso", "final", "transit", "guilt", "culpa", "liab",
        ] {
            assert!(
                !jsonl.to_lowercase().contains(forbidden),
                "inferred semantics in {jsonl}"
            );
        }
        let value: serde_json::Value = serde_json::from_str(&jsonl).expect("must be JSON");
        let mut keys: Vec<&str> = value
            .as_object()
            .expect("must be an object")
            .keys()
            .map(String::as_str)
            .collect();
        keys.sort();
        assert_eq!(
            keys,
            vec![
                "amount_minor",
                "amount_text",
                "as_of",
                "decided_on",
                "evidence_ref",
                "native_key",
                "period_label",
                "proceeding_ref",
                "schema_version",
                "source",
                "source_key",
                "status"
            ]
        );
    }
}

#[test]
fn sanction_reordered_input_keeps_identical_output() {
    let raw = String::from_utf8(sample()).expect("sample is UTF-8");
    let mut lines: Vec<&str> = raw.lines().collect();
    let header = lines.remove(0);
    lines.reverse();
    let reordered = format!("{header}\n{}\n", lines.join("\n"));
    let first = parse_sanctions("sanction-sample.csv", sample(), &Limits::default())
        .expect("sample parses");
    let second = parse_sanctions(
        "sanction-sample.csv",
        reordered.into_bytes(),
        &Limits::default(),
    )
    .expect("reordered parses");
    let to_jsonl = |batch: &station_prep::SanctionBatch| {
        batch
            .snapshots
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
fn sanction_header_only_input_completes_empty() {
    // A missing annual snapshot is a discovery concern (skip_missing);
    // preparation treats header-only input as an empty complete run.
    let batch = parse_sanctions("empty.csv", HEADER.as_bytes().to_vec(), &Limits::default())
        .expect("header-only must complete");
    assert_eq!(batch.state, RunState::Complete);
    assert_eq!(batch.counts.input, 0);
    assert_eq!(batch.counts.accepted, 0);
}

#[test]
fn sanction_bom_prefix_is_stripped() {
    let mut raw = vec![0xEF, 0xBB, 0xBF];
    raw.extend(
        format!("{HEADER}MULTA-001;48000.001111/2024-11;48151623000191;150.000,00;aplicada;2024-06-15;2024-12-31;anexo-3\n")
            .into_bytes(),
    );
    let batch = parse_sanctions("bom.csv", raw, &Limits::default()).expect("BOM must strip");
    assert_eq!(batch.state, RunState::Complete);
    assert_eq!(batch.counts.accepted, 1);
    assert_eq!(batch.snapshots[0].amount_minor, Some(15_000_000));
}

#[test]
fn sanction_bad_encoding_fails_without_partial_output() {
    let mut raw = format!("{HEADER}MULTA-001;48000.001111/2024-11;48151623000191;").into_bytes();
    raw.extend([0xFF, 0xFE]);
    raw.extend(b"150.000,00;aplicada;2024-06-15;2024-12-31;anexo-3\n");
    let err = parse_sanctions("bad-encoding.csv", raw, &Limits::default());
    assert!(err.is_err(), "non-UTF8 must fail, got {err:?}");
}

#[test]
fn sanction_truncated_input_fails_without_partial_output() {
    let raw = format!(
        "{HEADER}MULTA-001;48000.001111/2024-11;48151623000191;150.000,00;aplicada;2024-06-15;2024-12-31;anexo-3"
    );
    let err = parse_sanctions("truncated.csv", raw.into_bytes(), &Limits::default());
    assert!(err.is_err(), "missing terminator must fail, got {err:?}");
}

#[test]
fn sanction_bounds_are_enforced() {
    let tiny = Limits {
        max_bytes: 10,
        ..Limits::default()
    };
    assert!(parse_sanctions("sanction-sample.csv", sample(), &tiny).is_err());
    let few = Limits {
        max_rows: 2,
        ..Limits::default()
    };
    assert!(parse_sanctions("sanction-sample.csv", sample(), &few).is_err());
    let narrow = Limits {
        max_field_bytes: 10,
        ..Limits::default()
    };
    let batch = parse_sanctions("sanction-sample.csv", sample(), &narrow)
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
fn sanction_header_variants_quarantine_the_run() {
    let missing = HEADER.replace(";VALOR;", ";").into_bytes();
    let batch = parse_sanctions("missing-col.csv", missing, &Limits::default())
        .expect("variant must report, not fail");
    assert_eq!(batch.state, RunState::Quarantined);
    assert_eq!(batch.error_code, "header_mismatch");
    assert_eq!(batch.counts.input, 0);
    assert_eq!(batch.quarantine[0].row_locator, "missing-col.csv:header");
    let mut extra = HEADER.trim_end().to_string();
    extra.push_str(";ORGAO\n");
    let batch = parse_sanctions("extra-col.csv", extra.into_bytes(), &Limits::default())
        .expect("variant must report, not fail");
    assert_eq!(batch.state, RunState::Quarantined);
    let repeated = HEADER.replace("MULTAID;", "MULTAID;MULTAID;").into_bytes();
    let batch = parse_sanctions("repeated-col.csv", repeated, &Limits::default())
        .expect("variant must report, not fail");
    assert_eq!(batch.state, RunState::Quarantined);
}

#[test]
fn sanction_short_row_quarantines_without_failing_the_run() {
    let raw = format!(
        "{HEADER}MULTA-001;48000.001111/2024-11;48151623000191;150.000,00;aplicada;2024-06-15;2024-12-31\n"
    );
    let batch = parse_sanctions("short-row.csv", raw.into_bytes(), &Limits::default())
        .expect("short row must quarantine, not fail");
    assert_eq!(batch.state, RunState::Complete);
    assert_eq!(batch.counts.input, 1);
    assert_eq!(batch.counts.quarantined, 1);
    assert_eq!(batch.quarantine[0].reason, "invalid_field");
}

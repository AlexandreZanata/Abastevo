//! FQ-06 acceptance for the label-preserving `enforcement-v1`
//! preparation stream. Every expectation below mirrors a frozen decision
//! in `docs/backend/evidence/fuel-quality/FQ06_ENFORCEMENT_ADAPTER.md`:
//! the sample fixture carries distinct actions against one CNPJ, an exact
//! repeat, a same-key status correction, verbatim mixed-case and accented
//! labels, a period mismatch, a dateless row, a bad CNPJ and four
//! invalid-field rows. Scope and status are never classified: the suite
//! asserts the absence of guilt semantics in the emission. The
//! coordinate, registry and assay streams are untouched.

use station_prep::{parse_enforcement, Limits, RunState};
use std::path::PathBuf;

fn dir() -> PathBuf {
    PathBuf::from(env!("CARGO_MANIFEST_DIR")).join("../../contracts/testdata/station-prep")
}

fn bytes(name: &str) -> Vec<u8> {
    std::fs::read(dir().join(name)).expect("fixture must exist")
}

const HEADER: &str = "ACAOID;CNPJALVO;DATAOCORRENCIA;ESCOPO;SITUACAO;DATAEXTRACAO;EVIDENCIA\n";

fn sample() -> Vec<u8> {
    bytes("enforcement-sample.csv")
}

#[test]
fn enforcement_sample_counts_reconcile() {
    let batch = parse_enforcement("enforcement-sample.csv", sample(), &Limits::default())
        .expect("sample must parse");
    assert_eq!(batch.state, RunState::Complete);
    assert_eq!(batch.counts.input, 12);
    assert_eq!(batch.counts.accepted, 6);
    assert_eq!(batch.counts.duplicates, 1);
    assert_eq!(batch.counts.quarantined, 5);
    // Distinct actions against one CNPJ stay distinct; the exact repeat
    // collapses to a counted twin of the first determination.
    assert_eq!(
        batch
            .actions
            .iter()
            .filter(|row| row.source_key == "48151623000191")
            .count(),
        4
    );
    assert_eq!(batch.duplicates.len(), 1);
    assert_eq!(batch.duplicates[0].native_key, "AUTO-001");
    assert_eq!(batch.duplicates[0].status, "autuado");
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
        "enforcement-sample.csv:8",
        "enforcement-sample.csv:9",
        "enforcement-sample.csv:10",
        "enforcement-sample.csv:11",
        "enforcement-sample.csv:12",
    ] {
        assert!(locators.contains(&locator), "missing {locator}");
    }
}

#[test]
fn enforcement_labels_stay_verbatim() {
    let batch = parse_enforcement("enforcement-sample.csv", sample(), &Limits::default())
        .expect("sample must parse");
    // Mixed case is never normalized: the source text is the record.
    let row = batch
        .actions
        .iter()
        .find(|row| row.native_key == "AUTO-005")
        .expect("mixed-case row must be accepted");
    assert_eq!(row.scope, "MUNICIPAL");
    assert_eq!(row.status, "Autuado");
    // Accents survive byte-identical.
    let row = batch
        .actions
        .iter()
        .find(|row| row.native_key == "AUTO-004")
        .expect("accented row must be accepted");
    assert_eq!(row.status, "em análise");
}

#[test]
fn enforcement_correction_pair_is_retained() {
    let batch = parse_enforcement("enforcement-sample.csv", sample(), &Limits::default())
        .expect("sample must parse");
    // Same native key, new status: a replacement correction, not a
    // duplicate. Both determinations stay for the revision supersession
    // downstream; preparation rewrites nothing.
    let pair: Vec<_> = batch
        .actions
        .iter()
        .filter(|row| row.native_key == "AUTO-001")
        .collect();
    assert_eq!(pair.len(), 2);
    let mut statuses: Vec<&str> = pair.iter().map(|row| row.status.as_str()).collect();
    statuses.sort();
    assert_eq!(statuses, vec!["autuado", "interditado"]);
}

#[test]
fn enforcement_period_mismatch_stays_visible() {
    let batch = parse_enforcement("enforcement-sample.csv", sample(), &Limits::default())
        .expect("sample must parse");
    // December action inside a September extraction: retained with its
    // own period label so edition claims can be checked, never dropped.
    let row = batch
        .actions
        .iter()
        .find(|row| row.native_key == "AUTO-003")
        .expect("mismatched row must be accepted");
    assert_eq!(row.period_label, "2026-12");
    assert_eq!(row.as_of, "2026-09-30");
    // Dateless rows stay with an empty period label instead of an
    // invented date.
    let row = batch
        .actions
        .iter()
        .find(|row| row.native_key == "AUTO-004")
        .expect("dateless row must be accepted");
    assert_eq!(row.occurred_on, "");
    assert_eq!(row.period_label, "");
}

#[test]
fn enforcement_emission_carries_no_guilt() {
    let batch = parse_enforcement("enforcement-sample.csv", sample(), &Limits::default())
        .expect("sample must parse");
    for row in &batch.actions {
        let jsonl = row.to_jsonl();
        for forbidden in ["guilt", "culpa", "final", "liab", "penalt", "conform"] {
            assert!(
                !jsonl.to_lowercase().contains(forbidden),
                "guilt semantics in {jsonl}"
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
                "as_of",
                "evidence_ref",
                "native_key",
                "occurred_on",
                "period_label",
                "schema_version",
                "scope",
                "source",
                "source_key",
                "status"
            ]
        );
    }
}

#[test]
fn enforcement_reordered_input_keeps_identical_output() {
    let raw = String::from_utf8(sample()).expect("sample is UTF-8");
    let mut lines: Vec<&str> = raw.lines().collect();
    let header = lines.remove(0);
    lines.reverse();
    let reordered = format!("{header}\n{}\n", lines.join("\n"));
    let first = parse_enforcement("enforcement-sample.csv", sample(), &Limits::default())
        .expect("sample parses");
    let second = parse_enforcement(
        "enforcement-sample.csv",
        reordered.into_bytes(),
        &Limits::default(),
    )
    .expect("reordered parses");
    let to_jsonl = |batch: &station_prep::EnforcementBatch| {
        batch
            .actions
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
fn enforcement_bom_prefix_is_stripped() {
    let mut raw = vec![0xEF, 0xBB, 0xBF];
    raw.extend(
        format!(
            "{HEADER}AUTO-001;48151623000191;2026-08-02;municipal;autuado;2026-09-30;pagina-12\n"
        )
        .into_bytes(),
    );
    let batch = parse_enforcement("bom.csv", raw, &Limits::default()).expect("BOM must strip");
    assert_eq!(batch.state, RunState::Complete);
    assert_eq!(batch.counts.accepted, 1);
}

#[test]
fn enforcement_bad_encoding_fails_without_partial_output() {
    let mut raw = format!("{HEADER}AUTO-001;48151623000191;2026-08-02;municipal;").into_bytes();
    raw.extend([0xFF, 0xFE]);
    raw.extend(b"autuado;2026-09-30;pagina-12\n");
    let err = parse_enforcement("bad-encoding.csv", raw, &Limits::default());
    assert!(err.is_err(), "non-UTF8 must fail, got {err:?}");
}

#[test]
fn enforcement_truncated_input_fails_without_partial_output() {
    let raw = format!(
        "{HEADER}AUTO-001;48151623000191;2026-08-02;municipal;autuado;2026-09-30;pagina-12"
    );
    let err = parse_enforcement("truncated.csv", raw.into_bytes(), &Limits::default());
    assert!(err.is_err(), "missing terminator must fail, got {err:?}");
}

#[test]
fn enforcement_bounds_are_enforced() {
    let tiny = Limits {
        max_bytes: 10,
        ..Limits::default()
    };
    assert!(parse_enforcement("enforcement-sample.csv", sample(), &tiny).is_err());
    let few = Limits {
        max_rows: 2,
        ..Limits::default()
    };
    assert!(parse_enforcement("enforcement-sample.csv", sample(), &few).is_err());
    let narrow = Limits {
        max_field_bytes: 10,
        ..Limits::default()
    };
    let batch = parse_enforcement("enforcement-sample.csv", sample(), &narrow)
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
fn enforcement_header_variants_quarantine_the_run() {
    let missing = HEADER.replace(";ESCOPO;", ";").into_bytes();
    let batch = parse_enforcement("missing-col.csv", missing, &Limits::default())
        .expect("variant must report, not fail");
    assert_eq!(batch.state, RunState::Quarantined);
    assert_eq!(batch.error_code, "header_mismatch");
    assert_eq!(batch.counts.input, 0);
    assert_eq!(batch.quarantine[0].row_locator, "missing-col.csv:header");
    let mut extra = HEADER.trim_end().to_string();
    extra.push_str(";ORGAO\n");
    let batch = parse_enforcement("extra-col.csv", extra.into_bytes(), &Limits::default())
        .expect("variant must report, not fail");
    assert_eq!(batch.state, RunState::Quarantined);
    let repeated = HEADER.replace("ACAOID;", "ACAOID;ACAOID;").into_bytes();
    let batch = parse_enforcement("repeated-col.csv", repeated, &Limits::default())
        .expect("variant must report, not fail");
    assert_eq!(batch.state, RunState::Quarantined);
}

#[test]
fn enforcement_short_row_quarantines_without_failing_the_run() {
    let raw = format!("{HEADER}AUTO-001;48151623000191;2026-08-02;municipal;autuado;2026-09-30\n");
    let batch = parse_enforcement("short-row.csv", raw.into_bytes(), &Limits::default())
        .expect("short row must quarantine, not fail");
    assert_eq!(batch.state, RunState::Complete);
    assert_eq!(batch.counts.input, 1);
    assert_eq!(batch.counts.quarantined, 1);
    assert_eq!(batch.quarantine[0].reason, "invalid_field");
}

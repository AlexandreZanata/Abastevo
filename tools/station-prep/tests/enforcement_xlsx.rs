//! Acceptance of the pinned enforcement workbook reader.
//!
//! The synthetic cases live with the reader. This file holds the real
//! source acceptance: the native reader and the independent Python census
//! (`scripts/audit-anp-source-census.py`) must agree on every aggregate of
//! the pinned workbook. The private workbook is never part of the
//! repository; the run is enabled by
//! `ANPFUEL_TEST_ENFORCEMENT_XLSX_PATH` and
//! `ANPFUEL_TEST_ENFORCEMENT_CENSUS_PATH`.

use serde_json::Value;
use station_prep::enforcement_xlsx::{parse_enforcement_xlsx, EnforcementSourceRecord};
use station_prep::Limits;

fn read_records(
    path: &str,
) -> (
    Vec<EnforcementSourceRecord>,
    station_prep::enforcement_xlsx::EnforcementCensus,
) {
    let raw = std::fs::read(path).expect("private enforcement workbook");
    let mut records = Vec::new();
    let run = parse_enforcement_xlsx("enforcement.xlsx", raw, &Limits::default(), |record| {
        records.push(record.clone());
        Ok(())
    })
    .expect("enforcement workbook read");
    (records, run.census)
}

#[test]
fn native_reader_matches_the_independent_source_census() {
    let Ok(workbook) = std::env::var("ANPFUEL_TEST_ENFORCEMENT_XLSX_PATH") else {
        eprintln!("skipped: private enforcement workbook not available");
        return;
    };
    let Ok(census_path) = std::env::var("ANPFUEL_TEST_ENFORCEMENT_CENSUS_PATH") else {
        eprintln!("skipped: independent census not available");
        return;
    };
    let oracle: Value =
        serde_json::from_str(&std::fs::read_to_string(&census_path).expect("independent census"))
            .expect("independent census json");
    let expected = &oracle["enforcement"];
    let (records, census) = read_records(&workbook);

    assert_eq!(census.rows, expected["rows"].as_u64().unwrap() as usize);
    assert_eq!(
        census.distinct_documents,
        expected["distinct_documents"].as_u64().unwrap() as usize
    );
    assert_eq!(
        census.distinct_subject_date_document_procedure,
        expected["distinct_subject_date_document_procedure"]
            .as_u64()
            .unwrap() as usize
    );
    assert_eq!(
        census.result_vocabulary_size,
        expected["result_vocabulary_size"].as_u64().unwrap() as usize
    );
    assert_eq!(
        census.result_max_chars,
        expected["result_max_chars"].as_u64().unwrap() as usize
    );
    assert_eq!(
        census.results_over_64_chars,
        expected["results_over_64_chars"].as_u64().unwrap() as usize
    );
    assert_eq!(
        census.nonnumeric_dates,
        expected["nonnumeric_dates"].as_u64().unwrap() as usize
    );
    assert_eq!(
        census.members,
        expected["members"].as_u64().unwrap() as usize
    );
    assert_eq!(
        census.expanded_bytes,
        expected["expanded_bytes"].as_u64().unwrap()
    );
    assert_eq!(
        census.date_system,
        if expected["date1904"].as_bool().unwrap_or(false) {
            "EXCEL_1904"
        } else {
            "EXCEL_1900"
        }
    );

    let oracle_segments = expected["segments"].as_object().expect("oracle segments");
    let oracle_shapes = expected["subject_shapes"]
        .as_object()
        .expect("oracle shapes");
    assert_eq!(census.segments.len(), oracle_segments.len());
    for (label, count) in oracle_segments {
        assert_eq!(
            census.segments.get(label).copied(),
            Some(count.as_u64().unwrap() as usize),
            "segment {label}"
        );
    }
    for (shape, count) in oracle_shapes {
        assert_eq!(
            census.subject_shapes.get(shape).copied(),
            Some(count.as_u64().unwrap() as usize),
            "subject shape {shape}"
        );
    }

    assert_eq!(records.len(), census.rows);
    for record in &records {
        if record.subject_kind != "company-shaped" {
            assert!(
                record.subject_cnpj.is_none(),
                "a non company subject must not carry an identifier"
            );
        }
        assert!(record.document_date.len() == 10, "exact day precision");
        assert!(!record.result.is_empty(), "source wording is kept");
    }
}

//! RST-04 acceptance: PMQC-to-registry join triage over the frozen
//! fixtures. Rust emits joined candidates only; `review_state` stays
//! `pending` everywhere and nothing promotes a point to `reviewed`.

use station_prep::{
    join_candidates, parse_pmqc, parse_registry, AliasTable, JoinedBatch, Limits, MatchState,
};
use std::path::PathBuf;

const REGISTRY: &str = "registry-13col-sample.csv";
const JOIN: &str = "pmqc-join-sample.csv";

fn dir() -> PathBuf {
    PathBuf::from(env!("CARGO_MANIFEST_DIR")).join("../../contracts/testdata/station-prep")
}

fn bytes(name: &str) -> Vec<u8> {
    std::fs::read(dir().join(name)).expect("fixture must exist")
}

fn text(name: &str) -> String {
    String::from_utf8(bytes(name)).expect("fixture must be UTF-8")
}

fn joined() -> JoinedBatch {
    let table =
        AliasTable::from_json(&text("ibge-mapping-sample.json")).expect("alias sample must load");
    let registry = parse_registry(REGISTRY, bytes(REGISTRY), &Limits::default(), &table)
        .expect("registry parses");
    let pmqc = parse_pmqc(JOIN, bytes(JOIN), &Limits::default()).expect("join sample parses");
    assert_eq!(
        (pmqc.counts.input, pmqc.counts.accepted),
        (6, 6),
        "join fixture rows must all validate"
    );
    join_candidates(&registry.accepted, &pmqc.candidates)
}

fn by_sample<'a>(batch: &'a JoinedBatch, sample: &str) -> &'a station_prep::JoinedCandidate {
    batch
        .candidates
        .iter()
        .find(|row| row.sample_ref == sample)
        .unwrap_or_else(|| panic!("{sample} must join"))
}

#[test]
fn join_counts_reconcile_by_verdict() {
    let batch = joined();
    assert_eq!(batch.counts.input, 6);
    assert_eq!(batch.counts.conflicting, 2);
    assert_eq!(batch.counts.missing, 1);
    assert_eq!(batch.counts.predates, 1);
    assert_eq!(batch.counts.stale, 1);
    assert_eq!(batch.counts.current, 1);
    // Stable output order independent of input order.
    let keys: Vec<_> = batch
        .candidates
        .iter()
        .map(|row| {
            (
                row.source_key.clone(),
                row.observed_at.clone(),
                row.sample_ref.clone(),
            )
        })
        .collect();
    let mut sorted = keys.clone();
    sorted.sort();
    assert_eq!(keys, sorted);
}

#[test]
fn far_apart_pair_stays_conflicting_for_review() {
    let batch = joined();
    for sample in ["J-001", "J-002"] {
        let row = by_sample(&batch, sample);
        assert_eq!(row.match_state, MatchState::ConflictingPoint);
        assert_eq!(row.group_size, 2);
        let diameter = row.max_group_distance_m.expect("group diameter");
        assert!(
            (diameter - 23950.5).abs() < 1.0,
            "unexpected conflict span {diameter}"
        );
        assert_eq!(row.review_state, "pending");
        assert!(row.anchor.is_some());
    }
}

#[test]
fn unmatched_cnpj_stays_candidate_without_anchor() {
    let batch = joined();
    let row = by_sample(&batch, "J-003");
    assert_eq!(row.match_state, MatchState::RegistryMissing);
    assert_eq!(row.anchor, None);
    assert_eq!(row.review_state, "pending");
}

#[test]
fn predated_and_stale_observations_triage() {
    let batch = joined();
    let predated = by_sample(&batch, "J-004");
    assert_eq!(predated.match_state, MatchState::PredatesEvidence);
    assert_eq!(
        predated.anchor.as_ref().expect("anchor").published_at,
        "2024-03-02"
    );
    let stale = by_sample(&batch, "J-006");
    assert_eq!(stale.match_state, MatchState::StaleObservation);
    assert_eq!(stale.review_state, "pending");
}

#[test]
fn coarse_points_flag_without_verdict_change() {
    let batch = joined();
    let row = by_sample(&batch, "J-005");
    assert_eq!(row.match_state, MatchState::Current);
    assert!(row.coarse_precision, "2-decimal latitude must flag");
    assert_eq!(row.review_state, "pending");
    // Three-decimal survey-grade points stay unflagged.
    assert!(!by_sample(&batch, "J-001").coarse_precision);
}

#[test]
fn reordered_join_input_keeps_identical_output() {
    let raw = text(JOIN);
    let (header, body) = raw.split_once('\n').expect("header line");
    let mut rows: Vec<&str> = body.lines().collect();
    rows.reverse();
    let reordered = format!("{header}\n{}\n", rows.join("\n"));
    let table =
        AliasTable::from_json(&text("ibge-mapping-sample.json")).expect("alias sample must load");
    let registry = parse_registry(REGISTRY, bytes(REGISTRY), &Limits::default(), &table)
        .expect("registry parses");
    let pmqc = parse_pmqc(JOIN, reordered.into_bytes(), &Limits::default()).expect("parses");
    let second = join_candidates(&registry.accepted, &pmqc.candidates);
    let first = joined();
    let to_jsonl = |batch: &JoinedBatch| {
        batch
            .candidates
            .iter()
            .map(|row| row.to_jsonl())
            .collect::<Vec<_>>()
            .join("\n")
    };
    assert_eq!(to_jsonl(&first), to_jsonl(&second));
}

#[test]
fn no_joined_row_promotes_location() {
    let batch = joined();
    assert!(!batch.candidates.is_empty());
    for row in &batch.candidates {
        assert_eq!(row.review_state, "pending");
        assert_eq!(row.accuracy_m, None);
        assert_eq!(row.original_crs, "EPSG:4674");
    }
}

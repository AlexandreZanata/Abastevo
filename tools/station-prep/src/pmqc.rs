//! PMQC sample reader: official coordinate candidates with unknown
//! accuracy. Points stay in their original CRS (`EPSG:4674`) with
//! `accuracy_m: null` and `review_state: pending`; nothing here promotes
//! a candidate to `reviewed` or feeds the 150m capture rule.

use crate::cnpj::normalize_cnpj;
use crate::types::{reason, sha256_hex, Counts, Limits, QuarantineRow, RunState};
use serde::Serialize;
use std::collections::HashSet;

/// Row source label for `station-coordinate-candidate-v1` output.
pub const PMQC_SOURCE: &str = "pmqc";

/// Contract header for the PMQC extract consumed here.
const PMQC_HEADER: [&str; 5] = [
    "AMOSTRAID",
    "CNPJPOSTO",
    "DATACOLETA",
    "LATITUDE",
    "LONGITUDE",
];

/// Coarse national bounds. A point outside is quarantined, never moved:
/// swapped pairs stay `suspect_swap` for review, never auto-swapped.
const LAT_MIN: f64 = -34.0;
const LAT_MAX: f64 = 6.0;
const LON_MIN: f64 = -74.0;
const LON_MAX: f64 = -28.0;

/// Transport-level failures. `Err` carries no partial output.
#[derive(Debug)]
pub enum PmqcError {
    Oversize,
    RowCap,
    Truncated(String),
    Encoding,
}

/// One accepted coordinate candidate (`station-coordinate-candidate-v1`).
/// Freshness review belongs to the Directory owner: the parser records
/// the observation date and leaves `staleness_note` empty.
#[derive(Debug, Clone, Serialize)]
pub struct PmqcCandidate {
    pub schema_version: &'static str,
    pub source: &'static str,
    pub source_key: String,
    pub sample_ref: String,
    pub observed_at: String,
    pub latitude: Option<f64>,
    pub longitude: Option<f64>,
    pub original_crs: String,
    pub accuracy_m: Option<f64>,
    pub evidence_ref: String,
    pub review_state: String,
    pub staleness_note: String,
}

impl PmqcCandidate {
    pub fn to_jsonl(&self) -> String {
        serde_json::to_string(self).expect("candidate must serialize")
    }
}

/// Parsed PMQC batch with reconciled counts.
#[derive(Debug)]
pub struct PmqcBatch {
    pub state: RunState,
    pub error_code: String,
    pub counts: Counts,
    pub candidates: Vec<PmqcCandidate>,
    pub quarantine: Vec<QuarantineRow>,
}

fn in_brazil(latitude: f64, longitude: f64) -> bool {
    (LAT_MIN..=LAT_MAX).contains(&latitude) && (LON_MIN..=LON_MAX).contains(&longitude)
}

fn candidate_checksum(sample: &str, cnpj: &str, observed: &str, point: &str) -> String {
    sha256_hex([sample, cnpj, observed, point].join("\x1f").as_bytes())
}

/// Parse one PMQC extract. `file` names the input for row locators and
/// evidence references only; bytes are never read from disk here.
pub fn parse_pmqc(file: &str, raw: Vec<u8>, limits: &Limits) -> Result<PmqcBatch, PmqcError> {
    if raw.len() as u64 > limits.max_bytes {
        return Err(PmqcError::Oversize);
    }
    let text = std::str::from_utf8(&raw).map_err(|_| PmqcError::Encoding)?;
    let text = text.strip_prefix('\u{FEFF}').unwrap_or(text);
    // Framing rule, same as the registry reader: a complete snapshot ends
    // with a record terminator; anything else fails as truncated.
    if !text.ends_with('\n') {
        return Err(PmqcError::Truncated(
            "missing final record terminator; truncated snapshot".to_string(),
        ));
    }

    let mut reader = csv::ReaderBuilder::new()
        .delimiter(b';')
        .flexible(true)
        .from_reader(text.as_bytes());
    let header = reader
        .headers()
        .map_err(|err| PmqcError::Truncated(err.to_string()))?
        .clone();
    let names: Vec<String> = header
        .iter()
        .map(|name| name.trim().to_uppercase())
        .collect();
    let position = |wanted: &str| names.iter().position(|name| name == wanted);
    let missing: Vec<&str> = PMQC_HEADER
        .iter()
        .filter(|wanted| position(wanted).is_none())
        .copied()
        .collect();
    let unknown: Vec<String> = names
        .iter()
        .filter(|name| !PMQC_HEADER.contains(&name.as_str()))
        .cloned()
        .collect();
    if !missing.is_empty() || !unknown.is_empty() {
        return Ok(PmqcBatch {
            state: RunState::Quarantined,
            error_code: "header_mismatch".to_string(),
            counts: Counts::default(),
            candidates: Vec::new(),
            quarantine: vec![QuarantineRow {
                schema_version: "station-quarantine-v1",
                source: PMQC_SOURCE,
                row_locator: format!("{file}:header"),
                reason: reason::HEADER_MISMATCH,
                detail: format!("missing {missing:?}; unknown {unknown:?}; run quarantined"),
                source_key: None,
            }],
        });
    }
    let at = |record: &csv::StringRecord, wanted: &str| -> String {
        record
            .get(position(wanted).expect("checked"))
            .unwrap_or("")
            .trim()
            .to_string()
    };
    let quarantine_row =
        |logical_row: usize, reason: &'static str, detail: String, source_key: Option<String>| {
            QuarantineRow {
                schema_version: "station-quarantine-v1",
                source: PMQC_SOURCE,
                row_locator: format!("{file}:{logical_row}"),
                reason,
                detail,
                source_key,
            }
        };

    let mut counts = Counts::default();
    let mut candidates = Vec::new();
    let mut quarantine = Vec::new();
    let mut seen: HashSet<String> = HashSet::new();

    for (offset, next) in reader.records().enumerate() {
        let logical_row = offset + 2;
        if counts.input >= limits.max_rows {
            return Err(PmqcError::RowCap);
        }
        counts.input += 1;
        let record = next.map_err(|err| PmqcError::Truncated(err.to_string()))?;
        if record
            .iter()
            .any(|field| field.len() > limits.max_field_bytes)
        {
            counts.quarantined += 1;
            quarantine.push(quarantine_row(
                logical_row,
                reason::RECORD_TOO_LARGE,
                format!("field exceeds {} bytes", limits.max_field_bytes),
                None,
            ));
            continue;
        }
        let sample = at(&record, "AMOSTRAID");
        let cnpj_raw = at(&record, "CNPJPOSTO");
        let observed = at(&record, "DATACOLETA");
        let latitude_raw = at(&record, "LATITUDE");
        let longitude_raw = at(&record, "LONGITUDE");

        let cnpj = match normalize_cnpj(&cnpj_raw) {
            Ok(cnpj) => cnpj,
            Err(_) => {
                counts.quarantined += 1;
                quarantine.push(quarantine_row(
                    logical_row,
                    reason::INVALID_CNPJ,
                    "identifier fails checksum vector".to_string(),
                    None,
                ));
                continue;
            }
        };
        if sample.is_empty() || observed.is_empty() {
            counts.quarantined += 1;
            quarantine.push(quarantine_row(
                logical_row,
                reason::INVALID_FIELD,
                "empty sample reference or collection date".to_string(),
                Some(cnpj),
            ));
            continue;
        }
        // Empty coordinates stay an accepted candidate without a point:
        // unknown stays unknown, never a fabricated centroid.
        let (latitude, longitude) = match (latitude_raw.as_str(), longitude_raw.as_str()) {
            ("", "") => (None, None),
            (latitude_raw, longitude_raw) => {
                let parsed = || -> Option<(f64, f64)> {
                    let latitude: f64 = latitude_raw.parse().ok()?;
                    let longitude: f64 = longitude_raw.parse().ok()?;
                    if !latitude.is_finite() || !longitude.is_finite() {
                        return None;
                    }
                    Some((latitude, longitude))
                };
                let Some((latitude, longitude)) = parsed() else {
                    counts.quarantined += 1;
                    quarantine.push(quarantine_row(
                        logical_row,
                        reason::INVALID_POINT,
                        "non-finite coordinates".to_string(),
                        Some(cnpj),
                    ));
                    continue;
                };
                if latitude == 0.0 && longitude == 0.0 {
                    counts.quarantined += 1;
                    quarantine.push(quarantine_row(
                        logical_row,
                        reason::INVALID_POINT,
                        "zero coordinates are placeholders, not a site".to_string(),
                        Some(cnpj),
                    ));
                    continue;
                }
                if !in_brazil(latitude, longitude) {
                    let reason = if in_brazil(longitude, latitude) {
                        reason::SUSPECT_SWAP
                    } else {
                        reason::OUT_OF_BRAZIL
                    };
                    let detail = if reason == reason::SUSPECT_SWAP {
                        "latitude outside Brazil with mirrored values; never auto-swap".to_string()
                    } else {
                        "point outside national bounds".to_string()
                    };
                    counts.quarantined += 1;
                    quarantine.push(quarantine_row(logical_row, reason, detail, Some(cnpj)));
                    continue;
                }
                (Some(latitude), Some(longitude))
            }
        };
        let point = match (latitude, longitude) {
            (Some(latitude), Some(longitude)) => format!("{latitude:.6},{longitude:.6}"),
            _ => String::new(),
        };
        let checksum = candidate_checksum(&sample, &cnpj, &observed, &point);
        if !seen.insert(checksum) {
            counts.duplicates += 1;
            continue;
        }
        counts.accepted += 1;
        candidates.push(PmqcCandidate {
            schema_version: "station-coordinate-candidate-v1",
            source: PMQC_SOURCE,
            source_key: cnpj,
            sample_ref: sample.clone(),
            observed_at: observed,
            latitude,
            longitude,
            original_crs: "EPSG:4674".to_string(),
            accuracy_m: None,
            evidence_ref: format!("{file}#{sample}"),
            review_state: "pending".to_string(),
            staleness_note: String::new(),
        });
    }

    candidates.sort_by(|a, b| {
        a.source_key
            .cmp(&b.source_key)
            .then_with(|| a.observed_at.cmp(&b.observed_at))
            .then_with(|| a.sample_ref.cmp(&b.sample_ref))
    });

    Ok(PmqcBatch {
        state: RunState::Complete,
        error_code: String::new(),
        counts,
        candidates,
        quarantine,
    })
}

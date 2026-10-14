//! Sanction snapshots: extraction-time fine/proceeding records for the
//! `sanction-v1` stream. Status stays source-verbatim and extraction-time
//! only: this parser infers no appeal outcome, no finality and no
//! current-legal-status meaning. Amounts keep their raw text verbatim
//! plus strictly parsed minor units; no totals, conversions or math are
//! computed anywhere here. Same proceedings across annual snapshots stay
//! distinct rows; same-key content changes are retained as correction
//! pairs for downstream revision supersession, never rewritten.
//!
//! FQ-07 input contract (frozen here; raw ANP sanction columns remain
//! unverified upstream, so this preparation shape is synthetic until FQ-05
//! binds a verified edition):
//! `MULTAID;PROCESSO;CNPJPATE;VALOR;SITUACAO;DATADECISAO;DATAEXTRACAO;EVIDENCIA`

use crate::cnpj::normalize_cnpj;
use crate::types::{reason, sha256_hex, Counts, Limits, QuarantineRow, RunState};
use serde::Serialize;
use std::collections::{HashMap, HashSet};

/// Row source label for `sanction-event-v1` output.
pub const SANCTION_SOURCE: &str = "sanctions";

/// Frozen contract header for the sanction extract consumed here.
const SANCTION_HEADER: [&str; 8] = [
    "MULTAID",
    "PROCESSO",
    "CNPJPATE",
    "VALOR",
    "SITUACAO",
    "DATADECISAO",
    "DATAEXTRACAO",
    "EVIDENCIA",
];

/// Transport-level failures. `Err` carries no partial output.
#[derive(Debug)]
pub enum SanctionError {
    Oversize,
    RowCap,
    Truncated(String),
    Encoding,
}

/// One accepted sanction snapshot (`sanction-event-v1`). `amount_text`
/// keeps the source text verbatim; `amount_minor` holds strictly parsed
/// minor units or `None` when the source states no amount. Staleness is
/// disclosed through `as_of` on every row: readers compute age, this
/// parser stamps and retains.
#[derive(Debug, Clone, Serialize)]
pub struct SanctionSnapshot {
    pub schema_version: &'static str,
    pub source: &'static str,
    pub source_key: String,
    pub native_key: String,
    pub proceeding_ref: String,
    pub amount_text: String,
    pub amount_minor: Option<u64>,
    pub status: String,
    pub decided_on: String,
    pub as_of: String,
    pub period_label: String,
    pub evidence_ref: String,
}

impl SanctionSnapshot {
    pub fn to_jsonl(&self) -> String {
        serde_json::to_string(self).expect("snapshot must serialize")
    }
}

/// Parsed sanction batch with reconciled counts.
#[derive(Debug)]
pub struct SanctionBatch {
    pub state: RunState,
    pub error_code: String,
    pub counts: Counts,
    pub snapshots: Vec<SanctionSnapshot>,
    /// One retained repeat per within-run exact repeat (repeat order),
    /// so emission carries the full multiset the manifest accounts for.
    pub duplicates: Vec<SanctionSnapshot>,
    pub quarantine: Vec<QuarantineRow>,
}

impl SanctionBatch {
    /// Manifest hash over the deterministic snapshot emission plus counts.
    pub fn manifest_hash(&self) -> String {
        let mut parts: Vec<String> = self.snapshots.iter().map(|row| row.to_jsonl()).collect();
        parts.push(format!(
            "counts:{}:{}:{}:{}",
            self.counts.input,
            self.counts.accepted,
            self.counts.duplicates,
            self.counts.quarantined
        ));
        sha256_hex(parts.join("\n").as_bytes())
    }
}

/// Date-only `YYYY-MM-DD` shape check. No calendar validation and no
/// midnight time is invented; malformed dates quarantine the row.
fn valid_day(text: &str) -> bool {
    let bytes = text.as_bytes();
    bytes.len() == 10
        && bytes[4] == b'-'
        && bytes[7] == b'-'
        && bytes
            .iter()
            .enumerate()
            .filter(|(i, _)| *i != 4 && *i != 7)
            .all(|(_, b)| b.is_ascii_digit())
}

/// Strict minor-unit parsing for `D{1,3}(.DDD)*,DD` source text. Anything
/// else is `None` so the caller quarantines instead of guessing a value.
/// No totals, conversions or rounding are performed here.
fn parse_amount_minor(text: &str) -> Option<u64> {
    let (int_part, frac_part) = text.split_once(',')?;
    if frac_part.len() != 2 || !frac_part.bytes().all(|b| b.is_ascii_digit()) {
        return None;
    }
    let groups: Vec<&str> = int_part.split('.').collect();
    if groups.is_empty() || groups[0].len() > 3 || groups[0].is_empty() {
        return None;
    }
    if groups[0].starts_with('0') && groups[0].len() > 1 {
        return None;
    }
    for group in groups.iter().skip(1) {
        if group.len() != 3 || !group.bytes().all(|b| b.is_ascii_digit()) {
            return None;
        }
    }
    if !groups[0].bytes().all(|b| b.is_ascii_digit()) {
        return None;
    }
    let units: u64 = groups.join("").parse().ok()?;
    let cents: u64 = frac_part.parse().ok()?;
    units.checked_mul(100)?.checked_add(cents)
}

fn snapshot_checksum(parts: &[&str]) -> String {
    sha256_hex(parts.join("\x1f").as_bytes())
}

/// Parse one sanction extract. `file` names the input for row locators
/// and evidence references only; bytes are never read from disk here.
pub fn parse_sanctions(
    file: &str,
    raw: Vec<u8>,
    limits: &Limits,
) -> Result<SanctionBatch, SanctionError> {
    if raw.len() as u64 > limits.max_bytes {
        return Err(SanctionError::Oversize);
    }
    let text = std::str::from_utf8(&raw).map_err(|_| SanctionError::Encoding)?;
    let text = text.strip_prefix('\u{FEFF}').unwrap_or(text);
    // Framing rule, same as the sibling readers: a complete snapshot ends
    // with a record terminator; anything else fails as truncated.
    if !text.ends_with('\n') {
        return Err(SanctionError::Truncated(
            "missing final record terminator; truncated snapshot".to_string(),
        ));
    }

    let mut reader = csv::ReaderBuilder::new()
        .delimiter(b';')
        .flexible(true)
        .from_reader(text.as_bytes());
    let header = reader
        .headers()
        .map_err(|err| SanctionError::Truncated(err.to_string()))?
        .clone();
    let names: Vec<String> = header
        .iter()
        .map(|name| name.trim().to_uppercase())
        .collect();
    let position = |wanted: &str| names.iter().position(|name| name == wanted);
    let missing: Vec<&str> = SANCTION_HEADER
        .iter()
        .filter(|wanted| position(wanted).is_none())
        .copied()
        .collect();
    let unknown: Vec<String> = names
        .iter()
        .filter(|name| !SANCTION_HEADER.contains(&name.as_str()))
        .cloned()
        .collect();
    let unique_names: HashSet<&str> = names.iter().map(String::as_str).collect();
    if !missing.is_empty() || !unknown.is_empty() || unique_names.len() != names.len() {
        return Ok(SanctionBatch {
            state: RunState::Quarantined,
            error_code: "header_mismatch".to_string(),
            counts: Counts::default(),
            snapshots: Vec::new(),
            duplicates: Vec::new(),
            quarantine: vec![QuarantineRow {
                schema_version: "station-quarantine-v1",
                source: SANCTION_SOURCE,
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
                source: SANCTION_SOURCE,
                row_locator: format!("{file}:{logical_row}"),
                reason,
                detail,
                source_key,
            }
        };

    let mut counts = Counts::default();
    let mut snapshots = Vec::new();
    let mut quarantine = Vec::new();
    let mut seen: HashSet<String> = HashSet::new();
    let mut dup_checksums: Vec<String> = Vec::new();

    for (offset, next) in reader.records().enumerate() {
        let logical_row = offset + 2;
        if counts.input >= limits.max_rows {
            return Err(SanctionError::RowCap);
        }
        counts.input += 1;
        let record = next.map_err(|err| SanctionError::Truncated(err.to_string()))?;
        if record.len() != header.len() {
            counts.quarantined += 1;
            quarantine.push(quarantine_row(
                logical_row,
                reason::INVALID_FIELD,
                format!(
                    "expected {} fields, received {}",
                    header.len(),
                    record.len()
                ),
                None,
            ));
            continue;
        }
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
        let native_key = at(&record, "MULTAID");
        let proceeding = at(&record, "PROCESSO");
        let cnpj_raw = at(&record, "CNPJPATE");
        let amount_text = at(&record, "VALOR");
        let status = at(&record, "SITUACAO");
        let decided = at(&record, "DATADECISAO");
        let as_of = at(&record, "DATAEXTRACAO");
        let evidence = at(&record, "EVIDENCIA");

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
        // Identity, proceeding, status and extraction stamping are all
        // required verbatim: nothing here is invented or defaulted.
        if native_key.is_empty()
            || proceeding.is_empty()
            || status.is_empty()
            || evidence.is_empty()
        {
            counts.quarantined += 1;
            quarantine.push(quarantine_row(
                logical_row,
                reason::INVALID_FIELD,
                "empty snapshot key, proceeding, status or evidence reference".to_string(),
                Some(cnpj),
            ));
            continue;
        }
        if !valid_day(&as_of) || (!decided.is_empty() && !valid_day(&decided)) {
            counts.quarantined += 1;
            quarantine.push(quarantine_row(
                logical_row,
                reason::INVALID_FIELD,
                "malformed decided or extraction date".to_string(),
                Some(cnpj),
            ));
            continue;
        }
        // Amounts are optional but strict: empty stays null, anything
        // outside the frozen shape quarantines instead of parsing loose.
        let amount_minor = if amount_text.is_empty() {
            None
        } else {
            match parse_amount_minor(&amount_text) {
                Some(minor) => Some(minor),
                None => {
                    counts.quarantined += 1;
                    quarantine.push(quarantine_row(
                        logical_row,
                        reason::INVALID_FIELD,
                        format!("unparseable amount {amount_text:?}; never guessed"),
                        Some(cnpj),
                    ));
                    continue;
                }
            }
        };
        // Period evidence prefers the decided year and falls back to the
        // extraction year; dateless decisions stay visible, never dated
        // by invention.
        let period_label = if !decided.is_empty() {
            decided[..4].to_string()
        } else {
            as_of[..4].to_string()
        };
        let evidence_ref = format!("{file}#{native_key}");
        let amount_part = amount_minor
            .map(|minor| minor.to_string())
            .unwrap_or_default();
        let checksum = snapshot_checksum(&[
            &native_key,
            &cnpj,
            &proceeding,
            &amount_part,
            &status,
            &decided,
            &as_of,
            &evidence_ref,
        ]);
        if !seen.insert(checksum.clone()) {
            counts.duplicates += 1;
            dup_checksums.push(checksum);
            continue;
        }
        counts.accepted += 1;
        snapshots.push(SanctionSnapshot {
            schema_version: "sanction-event-v1",
            source: SANCTION_SOURCE,
            source_key: cnpj,
            native_key: native_key.clone(),
            proceeding_ref: proceeding,
            amount_text,
            amount_minor,
            status,
            decided_on: decided,
            as_of,
            period_label,
            evidence_ref,
        });
    }

    // Content fields break sort ties so reordered input, including
    // same-key amount/status corrections, emits byte-identical output.
    snapshots.sort_by(|a, b| {
        a.source_key
            .cmp(&b.source_key)
            .then_with(|| a.decided_on.cmp(&b.decided_on))
            .then_with(|| a.native_key.cmp(&b.native_key))
            .then_with(|| a.amount_text.cmp(&b.amount_text))
            .then_with(|| a.status.cmp(&b.status))
            .then_with(|| a.as_of.cmp(&b.as_of))
            .then_with(|| a.evidence_ref.cmp(&b.evidence_ref))
    });

    // Resolve repeats to byte-identical snapshot twins so the emitted
    // multiset stays order-independent.
    let mut by_checksum: HashMap<String, &SanctionSnapshot> = HashMap::new();
    for row in &snapshots {
        let amount_part = row
            .amount_minor
            .map(|minor| minor.to_string())
            .unwrap_or_default();
        by_checksum
            .entry(snapshot_checksum(&[
                &row.native_key,
                &row.source_key,
                &row.proceeding_ref,
                &amount_part,
                &row.status,
                &row.decided_on,
                &row.as_of,
                &row.evidence_ref,
            ]))
            .or_insert(row);
    }
    let mut duplicates = Vec::with_capacity(dup_checksums.len());
    for checksum in &dup_checksums {
        duplicates.push(
            (*by_checksum
                .get(checksum)
                .expect("duplicate checksum was accepted"))
            .clone(),
        );
    }

    Ok(SanctionBatch {
        state: RunState::Complete,
        error_code: String::new(),
        counts,
        snapshots,
        duplicates,
        quarantine,
    })
}

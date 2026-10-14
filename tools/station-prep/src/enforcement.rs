//! Enforcement preparation: label-preserving fiscal-action records for
//! the `enforcement-v1` stream. Scope and procedural status stay
//! source-verbatim: this parser classifies nothing, derives no guilt,
//! liability or finality, and invents no current-legal-status semantics.
//! Distinct native keys against one CNPJ stay distinct rows; exact
//! within-run repeats collapse to counted duplicates. Rows carry their
//! extraction date and a derived period label so downstream edition
//! claims can be checked for content/period mismatches without dropping
//! evidence.
//!
//! FQ-06 input contract (frozen here; raw ANP enforcement columns remain
//! unverified upstream, so this preparation shape is synthetic until FQ-05
//! binds a verified edition):
//! `ACAOID;CNPJALVO;DATAOCORRENCIA;ESCOPO;SITUACAO;DATAEXTRACAO;EVIDENCIA`

use crate::cnpj::normalize_cnpj;
use crate::types::{reason, sha256_hex, Counts, Limits, QuarantineRow, RunState};
use serde::Serialize;
use std::collections::{HashMap, HashSet};

/// Row source label for `enforcement-event-v1` output.
pub const ENFORCEMENT_SOURCE: &str = "enforcement";

/// Frozen contract header for the enforcement extract consumed here.
const ENFORCEMENT_HEADER: [&str; 7] = [
    "ACAOID",
    "CNPJALVO",
    "DATAOCORRENCIA",
    "ESCOPO",
    "SITUACAO",
    "DATAEXTRACAO",
    "EVIDENCIA",
];

/// Transport-level failures. `Err` carries no partial output.
#[derive(Debug)]
pub enum EnforcementError {
    Oversize,
    RowCap,
    Truncated(String),
    Encoding,
}

/// One accepted fiscal-action record (`enforcement-event-v1`). Scope and
/// status keep the source text verbatim, including case and accents;
/// `period_label` derives from the occurred date (`YYYY-MM`) or stays
/// empty when the source gives no occurred date, so content/period
/// mismatches stay visible instead of refused.
#[derive(Debug, Clone, Serialize)]
pub struct EnforcementAction {
    pub schema_version: &'static str,
    pub source: &'static str,
    pub source_key: String,
    pub native_key: String,
    pub scope: String,
    pub status: String,
    pub occurred_on: String,
    pub as_of: String,
    pub period_label: String,
    pub evidence_ref: String,
}

impl EnforcementAction {
    pub fn to_jsonl(&self) -> String {
        serde_json::to_string(self).expect("action must serialize")
    }
}

/// Parsed enforcement batch with reconciled counts.
#[derive(Debug)]
pub struct EnforcementBatch {
    pub state: RunState,
    pub error_code: String,
    pub counts: Counts,
    pub actions: Vec<EnforcementAction>,
    /// One retained repeat per within-run exact repeat (repeat order),
    /// so emission carries the full multiset the manifest accounts for.
    pub duplicates: Vec<EnforcementAction>,
    pub quarantine: Vec<QuarantineRow>,
}

impl EnforcementBatch {
    /// Manifest hash over the deterministic action emission plus counts.
    pub fn manifest_hash(&self) -> String {
        let mut parts: Vec<String> = self
            .actions
            .iter()
            .map(|action| action.to_jsonl())
            .collect();
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

fn action_checksum(parts: &[&str]) -> String {
    sha256_hex(parts.join("\x1f").as_bytes())
}

/// Parse one enforcement extract. `file` names the input for row locators
/// and evidence references only; bytes are never read from disk here.
pub fn parse_enforcement(
    file: &str,
    raw: Vec<u8>,
    limits: &Limits,
) -> Result<EnforcementBatch, EnforcementError> {
    if raw.len() as u64 > limits.max_bytes {
        return Err(EnforcementError::Oversize);
    }
    let text = std::str::from_utf8(&raw).map_err(|_| EnforcementError::Encoding)?;
    let text = text.strip_prefix('\u{FEFF}').unwrap_or(text);
    // Framing rule, same as the registry/PMQC/assay readers: a complete
    // snapshot ends with a record terminator; anything else fails as
    // truncated.
    if !text.ends_with('\n') {
        return Err(EnforcementError::Truncated(
            "missing final record terminator; truncated snapshot".to_string(),
        ));
    }

    let mut reader = csv::ReaderBuilder::new()
        .delimiter(b';')
        .flexible(true)
        .from_reader(text.as_bytes());
    let header = reader
        .headers()
        .map_err(|err| EnforcementError::Truncated(err.to_string()))?
        .clone();
    let names: Vec<String> = header
        .iter()
        .map(|name| name.trim().to_uppercase())
        .collect();
    let position = |wanted: &str| names.iter().position(|name| name == wanted);
    let missing: Vec<&str> = ENFORCEMENT_HEADER
        .iter()
        .filter(|wanted| position(wanted).is_none())
        .copied()
        .collect();
    let unknown: Vec<String> = names
        .iter()
        .filter(|name| !ENFORCEMENT_HEADER.contains(&name.as_str()))
        .cloned()
        .collect();
    let unique_names: HashSet<&str> = names.iter().map(String::as_str).collect();
    if !missing.is_empty() || !unknown.is_empty() || unique_names.len() != names.len() {
        return Ok(EnforcementBatch {
            state: RunState::Quarantined,
            error_code: "header_mismatch".to_string(),
            counts: Counts::default(),
            actions: Vec::new(),
            duplicates: Vec::new(),
            quarantine: vec![QuarantineRow {
                schema_version: "station-quarantine-v1",
                source: ENFORCEMENT_SOURCE,
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
                source: ENFORCEMENT_SOURCE,
                row_locator: format!("{file}:{logical_row}"),
                reason,
                detail,
                source_key,
            }
        };

    let mut counts = Counts::default();
    let mut actions = Vec::new();
    let mut quarantine = Vec::new();
    let mut seen: HashSet<String> = HashSet::new();
    let mut dup_checksums: Vec<String> = Vec::new();

    for (offset, next) in reader.records().enumerate() {
        let logical_row = offset + 2;
        if counts.input >= limits.max_rows {
            return Err(EnforcementError::RowCap);
        }
        counts.input += 1;
        let record = next.map_err(|err| EnforcementError::Truncated(err.to_string()))?;
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
        let native_key = at(&record, "ACAOID");
        let cnpj_raw = at(&record, "CNPJALVO");
        let occurred = at(&record, "DATAOCORRENCIA");
        let scope = at(&record, "ESCOPO");
        let status = at(&record, "SITUACAO");
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
        // Identity, procedural labels and extraction stamping are all
        // required verbatim: nothing here is invented or defaulted.
        if native_key.is_empty() || scope.is_empty() || status.is_empty() || evidence.is_empty() {
            counts.quarantined += 1;
            quarantine.push(quarantine_row(
                logical_row,
                reason::INVALID_FIELD,
                "empty action key, scope, status or evidence reference".to_string(),
                Some(cnpj),
            ));
            continue;
        }
        if !valid_day(&as_of) || (!occurred.is_empty() && !valid_day(&occurred)) {
            counts.quarantined += 1;
            quarantine.push(quarantine_row(
                logical_row,
                reason::INVALID_FIELD,
                "malformed occurred or extraction date".to_string(),
                Some(cnpj),
            ));
            continue;
        }
        let period_label = if occurred.is_empty() {
            String::new()
        } else {
            occurred[..7].to_string()
        };
        // Identity covers the stored evidence reference, not the raw
        // locator text: locator-only differences collapse, while any key,
        // party, date, scope or status change is a distinct determination.
        let evidence_ref = format!("{file}#{native_key}");
        let checksum = action_checksum(&[
            &native_key,
            &cnpj,
            &occurred,
            &scope,
            &status,
            &as_of,
            &evidence_ref,
        ]);
        if !seen.insert(checksum.clone()) {
            counts.duplicates += 1;
            dup_checksums.push(checksum);
            continue;
        }
        counts.accepted += 1;
        actions.push(EnforcementAction {
            schema_version: "enforcement-event-v1",
            source: ENFORCEMENT_SOURCE,
            source_key: cnpj,
            native_key: native_key.clone(),
            scope,
            status,
            occurred_on: occurred,
            as_of,
            period_label,
            evidence_ref,
        });
    }

    // Correction pairs share station, date and native key: content
    // fields break the tie so reordered input emits byte-identical output.
    actions.sort_by(|a, b| {
        a.source_key
            .cmp(&b.source_key)
            .then_with(|| a.occurred_on.cmp(&b.occurred_on))
            .then_with(|| a.native_key.cmp(&b.native_key))
            .then_with(|| a.status.cmp(&b.status))
            .then_with(|| a.scope.cmp(&b.scope))
            .then_with(|| a.as_of.cmp(&b.as_of))
            .then_with(|| a.evidence_ref.cmp(&b.evidence_ref))
    });

    // Resolve repeats to byte-identical action twins so the emitted
    // multiset stays order-independent.
    let mut by_checksum: HashMap<String, &EnforcementAction> = HashMap::new();
    for action in &actions {
        by_checksum
            .entry(action_checksum(&[
                &action.native_key,
                &action.source_key,
                &action.occurred_on,
                &action.scope,
                &action.status,
                &action.as_of,
                &action.evidence_ref,
            ]))
            .or_insert(action);
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

    Ok(EnforcementBatch {
        state: RunState::Complete,
        error_code: String::new(),
        counts,
        actions,
        duplicates,
        quarantine,
    })
}

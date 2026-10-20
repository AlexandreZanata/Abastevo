//! PMQC assay preparation: lossless sample/assay records for the
//! `pmqc-assay-v1` stream. This is a distinct versioned stream from the
//! five-column coordinate extract in [`crate::pmqc`]: every assay row keeps
//! its native result text, unit and conformity verbatim, and repeated
//! assays under one sample are each retained. Exact within-run repeats
//! collapse to counted duplicates so the manifest accounts the full
//! multiset. Unknown fuels or conformity quarantine the row for review;
//! nothing is coerced into a false canonical value.
//!
//! FQ-02 prepared input contract (kept for compatibility):
//! `AMOSTRAID;CNPJPOSTO;DATACOLETA;PRODUTO;ENSAIO;RESULTADO;UNIDADE;CONFORMIDADE`
//! FQ-02B also accepts the verified native 19-column October-2026 layout.
//! Raw source results are preserved; assay status never judges a sample.

use crate::cnpj::normalize_cnpj;
use crate::types::{reason, sha256_hex, Counts, Limits, QuarantineRow, RunState};
use serde::Serialize;
use std::collections::{HashMap, HashSet};

/// Row source label for `quality-assay-v1` output.
pub const QUALITY_SOURCE: &str = "pmqc-assay";

/// Frozen contract header for the assay extract consumed here.
const QUALITY_HEADER: [&str; 8] = [
    "AMOSTRAID",
    "CNPJPOSTO",
    "DATACOLETA",
    "PRODUTO",
    "ENSAIO",
    "RESULTADO",
    "UNIDADE",
    "CONFORMIDADE",
];

const NATIVE_HEADER: [&str; 19] = [
    "DATACOLETA",
    "IDNUMERIC",
    "GRUPOPRODUTO",
    "PRODUTO",
    "RAZAOSOCIALPOSTO",
    "CNPJPOSTO",
    "DISTRIBUIDORA",
    "ENDEREÇO",
    "COMPLEMENTO",
    "BAIRRO",
    "MUNICÍPIO",
    "LATITUDE",
    "LONGITUDE",
    "UF",
    "REGIAOPOLITICA",
    "ENSAIO",
    "RESULTADO",
    "UNIDADEENSAIO",
    "CONFORME",
];

/// Transport-level failures. `Err` carries no partial output.
#[derive(Debug)]
pub enum QualityError {
    Oversize,
    RowCap,
    Truncated(String),
    Encoding,
}

/// One accepted assay determination (`quality-assay-v1`). Native values
/// stay verbatim: `result_text` keeps decimal commas, comparators and free
/// text; `unit` keeps any source text including empty. `fuel` and
/// `conformity` hold canonical values only — unknown source text never
/// reaches this struct (the row quarantines instead).
#[derive(Debug, Clone, Serialize)]
pub struct QualityAssay {
    pub schema_version: &'static str,
    pub source: &'static str,
    pub source_key: String,
    pub sample_ref: String,
    pub observed_at: String,
    pub fuel: String,
    pub assay_ref: String,
    pub result_text: String,
    pub unit: String,
    pub conformity: String,
    pub evidence_ref: String,
}

impl QualityAssay {
    pub fn to_jsonl(&self) -> String {
        serde_json::to_string(self).expect("assay must serialize")
    }
}

/// Parsed assay batch with reconciled counts.
#[derive(Debug)]
pub struct QualityBatch {
    pub state: RunState,
    pub error_code: String,
    pub counts: Counts,
    pub assays: Vec<QualityAssay>,
    /// One retained repeat per within-run exact repeat (repeat order),
    /// so emission carries the full multiset the manifest accounts for.
    pub duplicates: Vec<QualityAssay>,
    pub quarantine: Vec<QuarantineRow>,
}

impl QualityBatch {
    /// Manifest hash over the deterministic assay emission plus counts, so
    /// a reimported or republished edition cannot silently change support.
    pub fn manifest_hash(&self) -> String {
        let mut parts: Vec<String> = self.assays.iter().map(|assay| assay.to_jsonl()).collect();
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

/// ANP source fuel label to wire product. Mirrors the kernel vocabulary
/// pinned by FQ-01, extended only with the verified FQ-02B native labels.
fn map_fuel(label: &str) -> Option<&'static str> {
    match fold_label(label).as_str() {
        "ETANOL" | "ETANOL HIDRATADO COMUM" | "ETANOL HIDRATADO ADITIVADO" => Some("ETHANOL"),
        "GASOLINA COMUM" | "GASOLINA C COMUM" => Some("GASOLINE_REGULAR"),
        "GASOLINA ADITIVADA" | "GASOLINA C COMUM ADITIVADA" => Some("GASOLINE_ADDITIVED"),
        "GASOLINA C PREMIUM" => Some("GASOLINE_PREMIUM_GRADE"),
        "OLEO DIESEL S500" | "OLEO DIESEL B S500 - COMUM" | "OLEO DIESEL B S500 - ADITIVADO" => {
            Some("DIESEL_S500")
        }
        "OLEO DIESEL S10" | "OLEO DIESEL B S10 - COMUM" | "OLEO DIESEL B S10 - ADITIVADO" => {
            Some("DIESEL_S10")
        }
        "GNV" => Some("CNG"),
        "GLP P13" => Some("LPG_P13"),
        _ => None,
    }
}

/// Source conformity text to closed vocabulary. Unknown or empty text is
/// `None` so the caller quarantines with the reason instead of guessing.
fn map_conformity(text: &str) -> Option<&'static str> {
    match fold_label(text).as_str() {
        "CONFORME" | "SIM" => Some("CONFORMING"),
        "NAO CONFORME" | "NAO" => Some("NONCONFORMING"),
        "INCONCLUSIVO" => Some("INCONCLUSIVE"),
        _ => None,
    }
}

/// Uppercase-first accent fold over the Latin-1 vowels/caps found in ANP
/// source labels. Uppertasing first matters: lowercased accented input
/// (e.g. `não conforme`) uppercases to `NÃO`, which still needs folding.
fn fold_label(label: &str) -> String {
    let upper = label.to_uppercase();
    let mut out = String::with_capacity(upper.len());
    for c in upper.chars() {
        match c {
            'À' | 'Á' | 'Â' | 'Ã' | 'Ä' | 'Å' => out.push('A'),
            'È' | 'É' | 'Ê' | 'Ë' => out.push('E'),
            'Ì' | 'Í' | 'Î' | 'Ï' => out.push('I'),
            'Ò' | 'Ó' | 'Ô' | 'Õ' | 'Ö' => out.push('O'),
            'Ù' | 'Ú' | 'Û' | 'Ü' => out.push('U'),
            'Ç' => out.push('C'),
            'Ñ' => out.push('N'),
            _ => out.push(c),
        }
    }
    out.split_whitespace().collect::<Vec<_>>().join(" ")
}

fn assay_checksum(parts: &[&str]) -> String {
    sha256_hex(parts.join("\x1f").as_bytes())
}

/// Parse one assay extract. `file` names the input for row locators and
/// evidence references only; bytes are never read from disk here.
pub fn parse_quality_assays(
    file: &str,
    raw: Vec<u8>,
    limits: &Limits,
) -> Result<QualityBatch, QualityError> {
    if raw.len() as u64 > limits.max_bytes {
        return Err(QualityError::Oversize);
    }
    let text = std::str::from_utf8(&raw).map_err(|_| QualityError::Encoding)?;
    let text = text.strip_prefix('\u{FEFF}').unwrap_or(text);
    // Framing rule, same as the registry/PMQC readers: a complete snapshot
    // ends with a record terminator; anything else fails as truncated.
    if !text.ends_with('\n') {
        return Err(QualityError::Truncated(
            "missing final record terminator; truncated snapshot".to_string(),
        ));
    }

    let mut reader = csv::ReaderBuilder::new()
        .delimiter(b';')
        .flexible(true)
        .from_reader(text.as_bytes());
    let header = reader
        .headers()
        .map_err(|err| QualityError::Truncated(err.to_string()))?
        .clone();
    let names: Vec<String> = header
        .iter()
        .map(|name| name.trim().to_uppercase())
        .collect();
    let native = names.iter().any(|name| name == "IDNUMERIC");
    let expected: &[&str] = if native {
        &NATIVE_HEADER
    } else {
        &QUALITY_HEADER
    };
    let position = |wanted: &str| names.iter().position(|name| name == wanted);
    let missing: Vec<&str> = expected
        .iter()
        .filter(|wanted| position(wanted).is_none())
        .copied()
        .collect();
    let unknown: Vec<String> = names
        .iter()
        .filter(|name| !expected.contains(&name.as_str()))
        .cloned()
        .collect();
    let unique_names: HashSet<&str> = names.iter().map(String::as_str).collect();
    if !missing.is_empty() || !unknown.is_empty() || unique_names.len() != names.len() {
        return Ok(QualityBatch {
            state: RunState::Quarantined,
            error_code: "header_mismatch".to_string(),
            counts: Counts::default(),
            assays: Vec::new(),
            duplicates: Vec::new(),
            quarantine: vec![QuarantineRow {
                schema_version: "station-quarantine-v1",
                source: QUALITY_SOURCE,
                row_locator: format!("{file}:header"),
                reason: reason::HEADER_MISMATCH,
                detail: format!("missing {missing:?}; unknown {unknown:?}; run quarantined"),
                source_key: None,
            }],
        });
    }
    let mapped = |wanted: &str| {
        match (native, wanted) {
            (true, "AMOSTRAID") => "IDNUMERIC",
            (true, "UNIDADE") => "UNIDADEENSAIO",
            (true, "CONFORMIDADE") => "CONFORME",
            _ => wanted,
        }
        .to_string()
    };
    let at = |record: &csv::StringRecord, wanted: &str| -> String {
        record
            .get(position(&mapped(wanted)).expect("checked"))
            .unwrap_or("")
            .trim()
            .to_string()
    };
    let quarantine_row =
        |logical_row: usize, reason: &'static str, detail: String, source_key: Option<String>| {
            QuarantineRow {
                schema_version: "station-quarantine-v1",
                source: QUALITY_SOURCE,
                row_locator: format!("{file}:{logical_row}"),
                reason,
                detail,
                source_key,
            }
        };

    let mut counts = Counts::default();
    let mut assays = Vec::new();
    let mut quarantine = Vec::new();
    let mut seen: HashSet<String> = HashSet::new();
    let mut dup_checksums: Vec<String> = Vec::new();

    for (offset, next) in reader.records().enumerate() {
        let logical_row = offset + 2;
        if counts.input >= limits.max_rows {
            return Err(QualityError::RowCap);
        }
        counts.input += 1;
        let record = next.map_err(|err| QualityError::Truncated(err.to_string()))?;
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
        let mut sample = at(&record, "AMOSTRAID");
        let cnpj_raw = at(&record, "CNPJPOSTO");
        let observed = at(&record, "DATACOLETA");
        let fuel_raw = at(&record, "PRODUTO");
        let assay_ref = at(&record, "ENSAIO");
        let result = if native {
            record
                .get(position("RESULTADO").expect("checked"))
                .unwrap_or("")
                .to_string()
        } else {
            at(&record, "RESULTADO")
        };
        let unit = if native {
            record
                .get(position("UNIDADEENSAIO").expect("checked"))
                .unwrap_or("")
                .to_string()
        } else {
            at(&record, "UNIDADE")
        };
        let conformity_raw = at(&record, "CONFORMIDADE");

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
        if sample.is_empty()
            || observed.is_empty()
            || (native
                && (!sample.bytes().all(|b| b.is_ascii_digit())
                    || !valid_collection_clock(&observed)))
        {
            counts.quarantined += 1;
            quarantine.push(quarantine_row(
                logical_row,
                reason::INVALID_FIELD,
                "empty sample reference or collection date".to_string(),
                Some(cnpj),
            ));
            continue;
        }
        if assay_ref.is_empty() || (!native && result.is_empty()) {
            counts.quarantined += 1;
            quarantine.push(quarantine_row(
                logical_row,
                reason::INVALID_FIELD,
                "empty assay reference or result text".to_string(),
                Some(cnpj),
            ));
            continue;
        }
        let fuel = match map_fuel(&fuel_raw) {
            Some(fuel) => fuel,
            None => {
                counts.quarantined += 1;
                quarantine.push(quarantine_row(
                    logical_row,
                    reason::UNMAPPED_FUEL,
                    format!("unmapped source fuel {fuel_raw:?}; never coerced"),
                    Some(cnpj),
                ));
                continue;
            }
        };
        let conformity = match map_conformity(&conformity_raw) {
            Some(conformity) => conformity,
            None => {
                counts.quarantined += 1;
                quarantine.push(quarantine_row(
                    logical_row,
                    reason::UNKNOWN_CONFORMITY,
                    format!("unrecognized conformity {conformity_raw:?}; stays explicit"),
                    Some(cnpj),
                ));
                continue;
            }
        };
        if native {
            sample = format!("{sample}|{fuel_raw}");
        }
        let checksum = assay_checksum(&[
            &sample, &cnpj, &observed, fuel, &assay_ref, &result, &unit, conformity,
        ]);
        if !seen.insert(checksum.clone()) {
            counts.duplicates += 1;
            dup_checksums.push(checksum);
            continue;
        }
        counts.accepted += 1;
        assays.push(QualityAssay {
            schema_version: "quality-assay-v1",
            source: QUALITY_SOURCE,
            source_key: cnpj,
            sample_ref: sample.clone(),
            observed_at: observed,
            fuel: fuel.to_string(),
            assay_ref: assay_ref.clone(),
            result_text: result,
            unit,
            conformity: conformity.to_string(),
            evidence_ref: format!("{file}#{sample}#{assay_ref}"),
        });
    }

    // Same-key content corrections share every identity field: content
    // fields break the tie so reordered input emits byte-identical output.
    assays.sort_by(|a, b| {
        a.source_key
            .cmp(&b.source_key)
            .then_with(|| a.observed_at.cmp(&b.observed_at))
            .then_with(|| a.sample_ref.cmp(&b.sample_ref))
            .then_with(|| a.assay_ref.cmp(&b.assay_ref))
            .then_with(|| a.result_text.cmp(&b.result_text))
            .then_with(|| a.unit.cmp(&b.unit))
            .then_with(|| a.conformity.cmp(&b.conformity))
    });

    // Resolve repeats to byte-identical assay twins so the emitted
    // multiset stays order-independent.
    let mut by_checksum: HashMap<String, &QualityAssay> = HashMap::new();
    for assay in &assays {
        by_checksum
            .entry(assay_checksum(&[
                &assay.sample_ref,
                &assay.source_key,
                &assay.observed_at,
                &assay.fuel,
                &assay.assay_ref,
                &assay.result_text,
                &assay.unit,
                &assay.conformity,
            ]))
            .or_insert(assay);
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

    Ok(QualityBatch {
        state: RunState::Complete,
        error_code: String::new(),
        counts,
        assays,
        duplicates,
        quarantine,
    })
}

fn valid_collection_clock(s: &str) -> bool {
    if s.len() == 10 {
        return valid_collection_date(s);
    }
    let b = s.as_bytes();
    if b.len() != 19
        || b[10] != b' '
        || b[13] != b':'
        || b[16] != b':'
        || ![11, 12, 14, 15, 17, 18]
            .iter()
            .all(|i| b[*i].is_ascii_digit())
    {
        return false;
    }
    // Validate ASCII date bytes before slicing; never panic on UTF-8 input.
    if !b[..10].iter().enumerate().all(|(i, c)| {
        if i == 4 || i == 7 {
            *c == b'-'
        } else {
            c.is_ascii_digit()
        }
    }) {
        return false;
    }
    valid_collection_date(&s[..10])
        && &s[11..13] <= "23"
        && &s[14..16] <= "59"
        && &s[17..19] <= "59"
}

fn valid_collection_date(s: &str) -> bool {
    let b = s.as_bytes();
    if b.len() != 10
        || b[4] != b'-'
        || b[7] != b'-'
        || !b
            .iter()
            .enumerate()
            .all(|(i, c)| i == 4 || i == 7 || c.is_ascii_digit())
    {
        return false;
    }
    let y = s[..4].parse::<u32>().unwrap_or(0);
    let m = s[5..7].parse::<u32>().unwrap_or(0);
    let d = s[8..].parse::<u32>().unwrap_or(0);
    let max = match m {
        1 | 3 | 5 | 7 | 8 | 10 | 12 => 31,
        4 | 6 | 9 | 11 => 30,
        2 => {
            if y % 4 == 0 && (y % 100 != 0 || y % 400 == 0) {
                29
            } else {
                28
            }
        }
        _ => 0,
    };
    y > 0 && d > 0 && d <= max
}

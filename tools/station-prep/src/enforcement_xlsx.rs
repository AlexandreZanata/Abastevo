//! Actual ANP enforcement workbook reader (MEX-20a).
//!
//! The pinned profile is the official enforcement workbook published from
//! 2019 on: one worksheet, the source wording of every column and the
//! workbook date system as the only clock. A row keeps its own source
//! record: distinct results that share a document stay distinct rows,
//! nothing is merged and no fine, guilt, payment or finality is derived.
//!
//! The reader minimizes what it emits. A company identifier is kept only
//! when it is a valid CNPJ; personal identifiers, names, neighbourhoods
//! and addresses never leave the reader. The subject shape is counted so
//! the exclusion stays visible, and the census keeps aggregates only.

use crate::cnpj::normalize_cnpj;
use crate::types::{reason, sha256_hex, Counts, Limits, QuarantineRow, RunState};
use crate::xlsx::{read_shared_strings, read_workbook, SheetRows, XlsxError};
use crate::zip::{ZipArchive, ZipLimits};
use serde::Serialize;
use std::collections::{BTreeMap, HashSet};

/// Source label of the enforcement workbook stream.
pub const ENFORCEMENT_XLSX_SOURCE: &str = "anp-enforcement-xlsx";
/// Pinned source profile of the enforcement workbook. The typed boundary
/// (MEX-20b) requires this exact profile; a workbook revision that changes
/// the columns needs a new profile and a new acceptance, never an
/// automatic adaptation.
pub const ENFORCEMENT_XLSX_PROFILE: &str = "anp-enforcement-xlsx-2019-v1";
/// Frozen column order of the pinned enforcement profile.
pub const ENFORCEMENT_XLSX_HEADERS: [&str; 11] = [
    "UF",
    "Município",
    "Bairro",
    "ENDEREÇO",
    "CNPJ/CPF",
    "Agente Econômico",
    "Segmento Fiscalizado",
    "DATA DO DF",
    "Número do Documento",
    "Procedimento de Fiscalização",
    "Resultado",
];
/// Column count of the pinned enforcement profile.
pub const ENFORCEMENT_XLSX_WIDTH: usize = 11;

/// Refusal reasons of the enforcement workbook reader.
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum EnforcementXlsxError {
    Oversize,
    RowCap,
    Truncated(String),
    Archive(String),
    Xml(String),
    Profile(String),
    Contract(String),
}

impl std::fmt::Display for EnforcementXlsxError {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            EnforcementXlsxError::Oversize => write!(formatter, "input exceeds the budget"),
            EnforcementXlsxError::RowCap => write!(formatter, "row cap exceeded"),
            EnforcementXlsxError::Truncated(detail) => write!(formatter, "truncated: {detail}"),
            EnforcementXlsxError::Archive(detail) => write!(formatter, "archive refused: {detail}"),
            EnforcementXlsxError::Xml(detail) => write!(formatter, "xml refused: {detail}"),
            EnforcementXlsxError::Profile(detail) => write!(formatter, "profile refused: {detail}"),
            EnforcementXlsxError::Contract(detail) => {
                write!(formatter, "contract refused: {detail}")
            }
        }
    }
}

impl std::error::Error for EnforcementXlsxError {}

/// One minimized enforcement source record.
#[derive(Debug, Clone, Serialize)]
pub struct EnforcementSourceRecord {
    pub schema_version: &'static str,
    pub source: &'static str,
    /// Stable position of the row inside the pinned workbook revision.
    pub row_locator: String,
    /// Content identity of the minimized record.
    pub record_hash: String,
    /// Census key of the source tuple (subject, date, document, procedure).
    /// The raw subject is hashed so the typed boundary can verify the
    /// distinct-tuple census without ever receiving a personal identifier.
    pub tuple_hash: String,
    /// Company identifier, present only for a valid CNPJ.
    pub subject_cnpj: Option<String>,
    /// Shape of the source subject: company-shaped, personal-shaped or other.
    pub subject_kind: &'static str,
    pub uf: String,
    pub municipality: String,
    pub segment: String,
    /// Source document date, converted with the workbook date system.
    pub document_date: String,
    pub document_number: String,
    pub procedure: String,
    pub result: String,
    /// `true` for an exact repeat of an already emitted record.
    pub exact_repeat: bool,
}

/// Aggregates of the read workbook. Counts only, never source content.
#[derive(Debug, Clone, Default, Serialize)]
pub struct EnforcementCensus {
    pub rows: usize,
    pub distinct_documents: usize,
    pub distinct_subject_date_document_procedure: usize,
    pub result_vocabulary_size: usize,
    pub result_max_chars: usize,
    pub results_over_64_chars: usize,
    pub segments: BTreeMap<String, usize>,
    pub subject_shapes: BTreeMap<String, usize>,
    pub subjects_admitted: usize,
    pub nonnumeric_dates: usize,
    pub date_system: String,
    pub date_precision: String,
    pub first_date: Option<String>,
    pub last_date: Option<String>,
    pub members: usize,
    pub expanded_bytes: u64,
}

/// Result of one enforcement workbook read.
#[derive(Debug)]
pub struct EnforcementXlsxRun {
    pub state: RunState,
    pub error_code: String,
    pub counts: Counts,
    pub census: EnforcementCensus,
    pub quarantine: Vec<QuarantineRow>,
}

impl EnforcementXlsxRun {
    /// Manifest hash over the reconciled counts and the census
    /// aggregates. The record content is bound separately by the artifact
    /// hash of the emitted file.
    pub fn manifest_hash(&self) -> String {
        sha256_hex(
            serde_json::to_vec(&(
                "enforcement-xlsx-v1",
                self.counts.input,
                self.counts.accepted,
                self.counts.duplicates,
                self.counts.quarantined,
                &self.census,
            ))
            .expect("serializable")
            .as_slice(),
        )
    }
}

/// Convert one Excel serial date into a calendar day.
///
/// The 1900 system keeps the historical phantom leap day out of the
/// result: serial 60 never existed and is refused, and every serial
/// from 61 on is read as the real calendar day. The 1904 system counts
/// from 1904-01-01.
pub fn serial_to_day(serial: i64, date1904: bool) -> Option<(i64, u32, u32)> {
    let days = if date1904 {
        serial.checked_sub(24_107)?
    } else if serial == 60 {
        return None;
    } else if serial >= 61 {
        serial.checked_sub(25_569)?
    } else if serial >= 1 {
        serial.checked_sub(25_568)?
    } else {
        return None;
    };
    Some(civil_from_days(days))
}

fn civil_from_days(days: i64) -> (i64, u32, u32) {
    let z = days + 719_468;
    let era = if z >= 0 { z } else { z - 146_096 } / 146_097;
    let day_of_era = z - era * 146_097;
    let year_of_era =
        (day_of_era - day_of_era / 1460 + day_of_era / 36_524 - day_of_era / 146_096) / 365;
    let year = year_of_era + era * 400;
    let day_of_year = day_of_era - (365 * year_of_era + year_of_era / 4 - year_of_era / 100);
    let month_index = (5 * day_of_year + 2) / 153;
    let day = day_of_year - (153 * month_index + 2) / 5 + 1;
    let month = if month_index < 10 {
        month_index + 3
    } else {
        month_index - 9
    };
    let year = if month <= 2 { year + 1 } else { year };
    (year, month as u32, day as u32)
}

/// Shape of the source subject identifier.
fn subject_shape(raw: &str) -> &'static str {
    let digits: String = raw
        .chars()
        .filter(|character| !matches!(character, '.' | '/' | '-' | ' ' | '\t' | '\n' | '\r'))
        .collect();
    if digits.len() == 14 && digits.bytes().all(|byte| byte.is_ascii_digit()) {
        "company-shaped"
    } else if digits.len() == 11 && digits.bytes().all(|byte| byte.is_ascii_digit()) {
        "personal-shaped"
    } else {
        "other"
    }
}

/// Read the pinned enforcement workbook and emit minimized records.
///
/// `emit` receives every accepted record and every exact repeat, in
/// worksheet order. A refused workbook produces no partial output: the
/// error carries the reason and no record is emitted.
pub fn parse_enforcement_xlsx<F>(
    file: &str,
    raw: Vec<u8>,
    limits: &Limits,
    mut emit: F,
) -> Result<EnforcementXlsxRun, EnforcementXlsxError>
where
    F: FnMut(&EnforcementSourceRecord) -> Result<(), EnforcementXlsxError>,
{
    if raw.len() as u64 > limits.max_bytes {
        return Err(EnforcementXlsxError::Oversize);
    }
    let archive = ZipArchive::open(raw, ZipLimits::default())
        .map_err(|error| EnforcementXlsxError::Archive(format!("{error:?}")))?;
    let workbook = read_workbook(&archive).map_err(map_xlsx_error)?;
    let strings = read_shared_strings(&archive).map_err(map_xlsx_error)?;

    let mut census = EnforcementCensus {
        members: archive.entries().len(),
        expanded_bytes: archive.expanded_bytes(),
        date_system: if workbook.date1904 {
            "EXCEL_1904".to_string()
        } else {
            "EXCEL_1900".to_string()
        },
        date_precision: "DAY".to_string(),
        ..EnforcementCensus::default()
    };
    let mut counts = Counts::default();
    let mut quarantine: Vec<QuarantineRow> = Vec::new();
    let mut documents: HashSet<String> = HashSet::new();
    let mut tuples: HashSet<[u8; 16]> = HashSet::new();
    let mut results: HashSet<String> = HashSet::new();
    let mut seen: HashSet<[u8; 32]> = HashSet::new();

    let rows = SheetRows::new(
        &archive,
        &workbook.sheet_path,
        &strings,
        ENFORCEMENT_XLSX_WIDTH,
    )
    .map_err(map_xlsx_error)?;
    let mut header: Option<Vec<String>> = None;
    for row in rows {
        let row = row.map_err(map_xlsx_error)?;
        if header.is_none() {
            let cells = pad(&row.cells);
            if cells != ENFORCEMENT_XLSX_HEADERS {
                return Ok(EnforcementXlsxRun {
                    state: RunState::Quarantined,
                    error_code: "header_mismatch".to_string(),
                    counts: Counts::default(),
                    census,
                    quarantine: vec![QuarantineRow {
                        schema_version: "station-quarantine-v1",
                        source: ENFORCEMENT_XLSX_SOURCE,
                        row_locator: format!("{file}:header"),
                        reason: reason::HEADER_MISMATCH,
                        detail: format!(
                            "expected {ENFORCEMENT_XLSX_HEADERS:?}, received {cells:?}"
                        ),
                        source_key: None,
                    }],
                });
            }
            header = Some(cells);
            continue;
        }
        if counts.input >= limits.max_rows {
            return Err(EnforcementXlsxError::RowCap);
        }
        counts.input += 1;
        let cells = pad(&row.cells);
        let subject_raw = cells[4].as_str();
        let shape = subject_shape(subject_raw);
        *census.subject_shapes.entry(shape.to_string()).or_insert(0) += 1;
        let subject_cnpj = match shape {
            "company-shaped" => match normalize_cnpj(subject_raw) {
                Ok(cnpj) => {
                    census.subjects_admitted += 1;
                    Some(cnpj)
                }
                Err(_) => None,
            },
            _ => None,
        };

        let date_text = cells[7].as_str();
        let document_date = match numeric_date(date_text) {
            Some(serial) => match serial_to_day(serial, workbook.date1904) {
                Some((year, month, day)) => format!("{year:04}-{month:02}-{day:02}"),
                None => {
                    counts.quarantined += 1;
                    quarantine.push(QuarantineRow {
                        schema_version: "station-quarantine-v1",
                        source: ENFORCEMENT_XLSX_SOURCE,
                        row_locator: format!("{file}#row:{}", row.number),
                        reason: reason::INVALID_FIELD,
                        detail: "source date serial has no calendar day".to_string(),
                        source_key: subject_cnpj.clone(),
                    });
                    continue;
                }
            },
            None => {
                census.nonnumeric_dates += 1;
                counts.quarantined += 1;
                quarantine.push(QuarantineRow {
                    schema_version: "station-quarantine-v1",
                    source: ENFORCEMENT_XLSX_SOURCE,
                    row_locator: format!("{file}#row:{}", row.number),
                    reason: reason::INVALID_FIELD,
                    detail: "source date is not an exact day serial".to_string(),
                    source_key: subject_cnpj.clone(),
                });
                continue;
            }
        };

        let uf = cells[0].clone();
        let municipality = cells[1].clone();
        let segment = cells[6].clone();
        let document_number = cells[8].clone();
        let procedure = cells[9].clone();
        let result = cells[10].clone();

        documents.insert(document_number.clone());
        let tuple = digest16(subject_raw, date_text, &document_number, &procedure);
        tuples.insert(tuple);
        let tuple_hash = bytes_to_hex(&tuple);

        results.insert(result.clone());
        let result_chars = result.chars().count();
        if result_chars > census.result_max_chars {
            census.result_max_chars = result_chars;
        }
        if result_chars > 64 {
            census.results_over_64_chars += 1;
        }
        *census.segments.entry(segment.clone()).or_insert(0) += 1;
        match census.first_date.as_ref() {
            Some(first) if first.as_str() > document_date.as_str() => {
                census.first_date = Some(document_date.clone());
            }
            None => census.first_date = Some(document_date.clone()),
            _ => {}
        }
        match census.last_date.as_ref() {
            Some(last) if last.as_str() < document_date.as_str() => {
                census.last_date = Some(document_date.clone());
            }
            None => census.last_date = Some(document_date.clone()),
            _ => {}
        }

        let record_hash = sha256_hex(
            [
                subject_cnpj.as_deref().unwrap_or_default(),
                uf.as_str(),
                municipality.as_str(),
                segment.as_str(),
                document_date.as_str(),
                document_number.as_str(),
                procedure.as_str(),
                result.as_str(),
            ]
            .join("\u{1f}")
            .as_bytes(),
        );
        let mut digest = [0u8; 32];
        digest.copy_from_slice(&hex_to_bytes(&record_hash));
        let exact_repeat = !seen.insert(digest);
        if exact_repeat {
            counts.duplicates += 1;
        } else {
            counts.accepted += 1;
        }
        let record = EnforcementSourceRecord {
            schema_version: "enforcement-source-record-v1",
            source: ENFORCEMENT_XLSX_SOURCE,
            row_locator: format!("{file}#row:{}", row.number),
            record_hash,
            tuple_hash,
            subject_cnpj,
            subject_kind: shape,
            uf,
            municipality,
            segment,
            document_date,
            document_number,
            procedure,
            result,
            exact_repeat,
        };
        emit(&record)?;
    }

    if header.is_none() {
        return Err(EnforcementXlsxError::Contract(
            "the worksheet has no header row".to_string(),
        ));
    }
    census.rows = counts.input;
    census.distinct_documents = documents.len();
    census.distinct_subject_date_document_procedure = tuples.len();
    census.result_vocabulary_size = results.len();
    Ok(EnforcementXlsxRun {
        state: RunState::Complete,
        error_code: String::new(),
        counts,
        census,
        quarantine,
    })
}

fn map_xlsx_error(error: XlsxError) -> EnforcementXlsxError {
    match error {
        XlsxError::Archive(detail) => EnforcementXlsxError::Archive(format!("{detail:?}")),
        XlsxError::Xml(detail) => EnforcementXlsxError::Xml(format!("{detail:?}")),
        XlsxError::MissingPart => EnforcementXlsxError::Profile("missing part".to_string()),
        XlsxError::ProfileRefused => EnforcementXlsxError::Profile("profile refused".to_string()),
        XlsxError::CellRefused => EnforcementXlsxError::Contract("cell refused".to_string()),
        XlsxError::SharedStringRefused => {
            EnforcementXlsxError::Contract("shared string refused".to_string())
        }
    }
}

fn pad(cells: &[String]) -> Vec<String> {
    let mut padded = cells.to_vec();
    while padded.len() < ENFORCEMENT_XLSX_WIDTH {
        padded.push(String::new());
    }
    padded
}

/// Exact day serial of a source date value. A value with a fractional
/// part carries a time the source does not state as a day, so it is
/// refused instead of being truncated.
fn numeric_date(text: &str) -> Option<i64> {
    if text.is_empty() || !text.bytes().all(|byte| byte.is_ascii_digit()) {
        return None;
    }
    text.parse::<i64>().ok()
}

fn digest16(subject: &str, date: &str, document: &str, procedure: &str) -> [u8; 16] {
    let digest = sha256_hex(
        [subject, date, document, procedure]
            .join("\u{1f}")
            .as_bytes(),
    );
    let mut out = [0u8; 16];
    out.copy_from_slice(&hex_to_bytes(&digest)[..16]);
    out
}

fn hex_to_bytes(hex: &str) -> Vec<u8> {
    let bytes = hex.as_bytes();
    let mut out = Vec::with_capacity(bytes.len() / 2);
    let mut index = 0;
    while index + 1 < bytes.len() {
        let high = (bytes[index] as char).to_digit(16).unwrap_or(0) as u8;
        let low = (bytes[index + 1] as char).to_digit(16).unwrap_or(0) as u8;
        out.push((high << 4) | low);
        index += 2;
    }
    out
}

fn bytes_to_hex(bytes: &[u8]) -> String {
    const TABLE: &[u8; 16] = b"0123456789abcdef";
    let mut out = String::with_capacity(bytes.len() * 2);
    for byte in bytes {
        out.push(TABLE[(byte >> 4) as usize] as char);
        out.push(TABLE[(byte & 0x0F) as usize] as char);
    }
    out
}

#[cfg(test)]
mod tests {
    use super::{numeric_date, parse_enforcement_xlsx, serial_to_day, subject_shape};
    use crate::types::RunState;
    use crate::zip::tests::stored_archive;
    use crate::Limits;

    const WORKBOOK: &str = r#"<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><workbookPr defaultThemeVersion="202300"/><sheets><sheet name="v_dados_abertos_dfs" sheetId="1" r:id="rId1"/></sheets></workbook>"#;
    const RELS: &str = r#"<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/></Relationships>"#;

    /// Synthetic enforcement workbook: the pinned column order, one
    /// company subject, one personal subject and one exact repeat.
    fn workbook_bytes(rows: &[Vec<String>]) -> Vec<u8> {
        let mut strings: Vec<String> = Vec::new();
        let mut shared = String::from(
            "<sst xmlns=\"http://schemas.openxmlformats.org/spreadsheetml/2006/main\" count=\"0\" uniqueCount=\"0\">",
        );
        let mut sheet = String::from(
            "<worksheet xmlns=\"http://schemas.openxmlformats.org/spreadsheetml/2006/main\"><sheetData>",
        );
        for (index, row) in rows.iter().enumerate() {
            let number = index + 1;
            sheet.push_str(&format!("<row r=\"{number}\">"));
            for (column, value) in row.iter().enumerate() {
                let letter = column_letter(column);
                if index > 0 && column == 7 {
                    sheet.push_str(&format!("<c r=\"{letter}{number}\"><v>{value}</v></c>"));
                    continue;
                }
                let position = match strings.iter().position(|existing| existing == value) {
                    Some(position) => position,
                    None => {
                        strings.push((*value).to_string());
                        shared.push_str(&format!("<si><t>{}</t></si>", escape(value)));
                        strings.len() - 1
                    }
                };
                sheet.push_str(&format!(
                    "<c r=\"{letter}{number}\" t=\"s\"><v>{position}</v></c>"
                ));
            }
            sheet.push_str("</row>");
        }
        sheet.push_str("</sheetData></worksheet>");
        shared.push_str("</sst>");
        let workbook = WORKBOOK.as_bytes().to_vec();
        let rels = RELS.as_bytes().to_vec();
        let shared = shared.into_bytes();
        let sheet = sheet.into_bytes();
        let mut members: Vec<(&str, &[u8])> = vec![
            ("xl/workbook.xml", workbook.as_slice()),
            ("xl/_rels/workbook.xml.rels", rels.as_slice()),
            ("xl/sharedStrings.xml", shared.as_slice()),
            ("xl/worksheets/sheet1.xml", sheet.as_slice()),
        ];
        members.sort_by(|left, right| left.0.cmp(right.0));
        stored_archive(&members)
    }

    fn column_letter(mut index: usize) -> String {
        let mut letter = String::new();
        loop {
            letter.insert(0, (b'A' + (index % 26) as u8) as char);
            if index < 26 {
                break;
            }
            index = index / 26 - 1;
        }
        letter
    }

    fn escape(value: &str) -> String {
        value
            .replace('&', "&amp;")
            .replace('<', "&lt;")
            .replace('>', "&gt;")
    }

    const HEADER: [&str; 11] = [
        "UF",
        "Município",
        "Bairro",
        "ENDEREÇO",
        "CNPJ/CPF",
        "Agente Econômico",
        "Segmento Fiscalizado",
        "DATA DO DF",
        "Número do Documento",
        "Procedimento de Fiscalização",
        "Resultado",
    ];

    fn header_row() -> Vec<String> {
        HEADER.iter().map(|value| (*value).to_string()).collect()
    }

    #[allow(clippy::too_many_arguments)]
    fn row(
        uf: &str,
        municipality: &str,
        bairro: &str,
        address: &str,
        subject: &str,
        name: &str,
        segment: &str,
        date: &str,
        document: &str,
        procedure: &str,
        result: &str,
    ) -> Vec<String> {
        [
            uf,
            municipality,
            bairro,
            address,
            subject,
            name,
            segment,
            date,
            document,
            procedure,
            result,
        ]
        .iter()
        .map(|value| (*value).to_string())
        .collect()
    }

    #[test]
    fn reads_the_pinned_profile_and_minimizes_every_row() {
        let long_result = "Resultado procedural sintético com rótulo longo demais para sessenta e quatro caracteres";
        let rows = vec![
            header_row(),
            row(
                "SP",
                "São Paulo",
                "Centro Sintético",
                "Rua Sintética 1",
                "36754752000195",
                "Empresa Sintética Ltda",
                "Revenda de Combustíveis",
                "43467",
                "712508",
                "Procedimento sintético",
                long_result,
            ),
            row(
                "SP",
                "Campinas",
                "Centro Sintético",
                "Rua Sintética 2",
                "12345678901",
                "Pessoa Sintética",
                "Revenda de Combustíveis",
                "43468",
                "712509",
                "Procedimento sintético",
                "Resultado curto",
            ),
        ];
        let bytes = workbook_bytes(&rows);
        let mut records = Vec::new();
        let run = parse_enforcement_xlsx("synthetic.xlsx", bytes, &Limits::default(), |record| {
            records.push(record.clone());
            Ok(())
        })
        .unwrap();
        assert_eq!(run.state, RunState::Complete);
        assert_eq!(run.counts.input, 2);
        assert_eq!(run.counts.accepted, 2);
        assert_eq!(run.counts.duplicates, 0);
        assert_eq!(run.counts.quarantined, 0);
        assert_eq!(records.len(), 2);

        assert_eq!(records[0].subject_cnpj.as_deref(), Some("36754752000195"));
        assert_eq!(records[0].subject_kind, "company-shaped");
        assert_eq!(records[0].document_date, "2019-01-02");
        assert_eq!(records[0].row_locator, "synthetic.xlsx#row:2");
        assert_eq!(records[1].subject_cnpj, None);
        assert_eq!(records[1].subject_kind, "personal-shaped");
        assert_eq!(records[1].document_date, "2019-01-03");
        for record in &records {
            assert_eq!(record.tuple_hash.len(), 32);
            assert!(record
                .tuple_hash
                .bytes()
                .all(|byte| byte.is_ascii_hexdigit()));
        }
        assert_ne!(records[0].tuple_hash, records[1].tuple_hash);

        let emitted = serde_json::to_string(&records).unwrap();
        for excluded in [
            "Centro Sintético",
            "Rua Sintética",
            "Empresa Sintética",
            "Pessoa Sintética",
            "12345678901",
        ] {
            assert!(
                !emitted.contains(excluded),
                "{excluded} must not be emitted"
            );
        }

        assert_eq!(run.census.rows, 2);
        assert_eq!(run.census.distinct_documents, 2);
        assert_eq!(run.census.result_vocabulary_size, 2);
        assert_eq!(run.census.result_max_chars, long_result.chars().count());
        assert_eq!(run.census.results_over_64_chars, 1);
        assert_eq!(run.census.subjects_admitted, 1);
        assert_eq!(run.census.subject_shapes.get("company-shaped"), Some(&1));
        assert_eq!(run.census.subject_shapes.get("personal-shaped"), Some(&1));
        assert_eq!(run.census.segments.get("Revenda de Combustíveis"), Some(&2));
        assert_eq!(run.census.date_system, "EXCEL_1900");
        assert_eq!(run.census.date_precision, "DAY");
        assert_eq!(run.census.first_date.as_deref(), Some("2019-01-02"));
        assert_eq!(run.census.last_date.as_deref(), Some("2019-01-03"));
    }

    #[test]
    fn keeps_an_exact_repeat_visible_without_losing_the_first_record() {
        let rows = vec![
            header_row(),
            row(
                "SP",
                "São Paulo",
                "Centro Sintético",
                "Rua Sintética 1",
                "36754752000195",
                "Empresa Sintética Ltda",
                "Revenda de Combustíveis",
                "43467",
                "712508",
                "Procedimento sintético",
                "Resultado sintético",
            ),
            row(
                "SP",
                "São Paulo",
                "Centro Sintético",
                "Rua Sintética 1",
                "36754752000195",
                "Empresa Sintética Ltda",
                "Revenda de Combustíveis",
                "43467",
                "712508",
                "Procedimento sintético",
                "Resultado sintético",
            ),
        ];
        let bytes = workbook_bytes(&rows);
        let mut repeats = 0usize;
        let run = parse_enforcement_xlsx("synthetic.xlsx", bytes, &Limits::default(), |record| {
            if record.exact_repeat {
                repeats += 1;
            }
            Ok(())
        })
        .unwrap();
        assert_eq!(run.counts.input, 2);
        assert_eq!(run.counts.accepted, 1);
        assert_eq!(run.counts.duplicates, 1);
        assert_eq!(repeats, 1);
    }

    #[test]
    fn quarantines_a_workbook_whose_header_does_not_match_the_profile() {
        let rows = vec![
            vec![
                "UF".to_string(),
                "Município".to_string(),
                "Bairro".to_string(),
                "ENDEREÇO".to_string(),
                "CNPJ/CPF".to_string(),
                "Agente Econômico".to_string(),
                "Segmento Fiscalizado".to_string(),
                "DATA DO DF".to_string(),
                "Número do Documento".to_string(),
                "Procedimento".to_string(),
                "Resultado".to_string(),
            ],
            row(
                "SP",
                "São Paulo",
                "Centro Sintético",
                "Rua Sintética 1",
                "36754752000195",
                "Empresa Sintética Ltda",
                "Revenda de Combustíveis",
                "43467",
                "712508",
                "Procedimento sintético",
                "Resultado sintético",
            ),
        ];
        let bytes = workbook_bytes(&rows);
        let mut emitted = 0usize;
        let run = parse_enforcement_xlsx("synthetic.xlsx", bytes, &Limits::default(), |_record| {
            emitted += 1;
            Ok(())
        })
        .unwrap();
        assert_eq!(run.state, RunState::Quarantined);
        assert_eq!(run.error_code, "header_mismatch");
        assert_eq!(emitted, 0);
        assert_eq!(run.quarantine.len(), 1);
    }

    #[test]
    fn refuses_a_workbook_that_is_not_an_archive() {
        let outcome = parse_enforcement_xlsx(
            "broken.xlsx",
            b"not an archive".to_vec(),
            &Limits::default(),
            |_record| Ok(()),
        );
        assert!(outcome.is_err());
    }

    #[test]
    fn converts_the_1900_date_system_without_the_phantom_leap_day() {
        assert_eq!(serial_to_day(1, false), Some((1900, 1, 1)));
        assert_eq!(serial_to_day(59, false), Some((1900, 2, 28)));
        assert_eq!(serial_to_day(60, false), None);
        assert_eq!(serial_to_day(61, false), Some((1900, 3, 1)));
        assert_eq!(serial_to_day(43_467, false), Some((2019, 1, 2)));
        assert_eq!(serial_to_day(46_299, false), Some((2026, 10, 4)));
        assert_eq!(serial_to_day(0, false), None);
    }

    #[test]
    fn converts_the_1904_date_system() {
        assert_eq!(serial_to_day(0, true), Some((1904, 1, 1)));
        assert_eq!(serial_to_day(42_004, true), Some((2019, 1, 1)));
    }

    #[test]
    fn classifies_the_subject_shape_without_keeping_the_value() {
        assert_eq!(subject_shape("36754752000195"), "company-shaped");
        assert_eq!(subject_shape("36.754.752/0001-95"), "company-shaped");
        assert_eq!(subject_shape("12345678901"), "personal-shaped");
        assert_eq!(subject_shape("7"), "other");
        assert_eq!(subject_shape(""), "other");
    }

    #[test]
    fn refuses_a_date_that_carries_more_than_a_day() {
        assert_eq!(numeric_date("43467"), Some(43_467));
        assert_eq!(numeric_date("43467.5"), None);
        assert_eq!(numeric_date(""), None);
        assert_eq!(numeric_date("01/01/2019"), None);
    }
}

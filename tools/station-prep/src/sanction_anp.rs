//! Native ANP SICOM CSV observations. No source-issued fine ID, decision day,
//! exact extraction day, balance, finality or station identity is invented.
use crate::cnpj::normalize_cnpj;
use crate::types::{reason, sha256_hex, Counts, Limits, QuarantineRow};
use serde::Serialize;
use std::collections::HashSet;

const CP850: &str = "\u{c7}\u{fc}\u{e9}\u{e2}\u{e4}\u{e0}\u{e5}\u{e7}\u{ea}\u{eb}\u{e8}\u{ef}\u{ee}\u{ec}\u{c4}\u{c5}\u{c9}\u{e6}\u{c6}\u{f4}\u{f6}\u{f2}\u{fb}\u{f9}\u{ff}\u{d6}\u{dc}\u{f8}\u{a3}\u{d8}\u{d7}\u{192}\u{e1}\u{ed}\u{f3}\u{fa}\u{f1}\u{d1}\u{aa}\u{ba}\u{bf}\u{ae}\u{ac}\u{bd}\u{bc}\u{a1}\u{ab}\u{bb}\u{2591}\u{2592}\u{2593}\u{2502}\u{2524}\u{c1}\u{c2}\u{c0}\u{a9}\u{2563}\u{2551}\u{2557}\u{255d}\u{a2}\u{a5}\u{2510}\u{2514}\u{2534}\u{252c}\u{251c}\u{2500}\u{253c}\u{e3}\u{c3}\u{255a}\u{2554}\u{2569}\u{2566}\u{2560}\u{2550}\u{256c}\u{a4}\u{f0}\u{d0}\u{ca}\u{cb}\u{c8}\u{131}\u{cd}\u{ce}\u{cf}\u{2518}\u{250c}\u{2588}\u{2584}\u{a6}\u{cc}\u{2580}\u{d3}\u{df}\u{d4}\u{d2}\u{f5}\u{d5}\u{b5}\u{fe}\u{de}\u{da}\u{db}\u{d9}\u{fd}\u{dd}\u{af}\u{b4}\u{ad}\u{b1}\u{2017}\u{be}\u{b6}\u{a7}\u{f7}\u{b8}\u{b0}\u{a8}\u{b7}\u{b9}\u{b3}\u{b2}\u{25a0}\u{a0}";
const HEADER: [&str; 9] = [
    "Status Processo",
    "Superintendência",
    "Número do Processo",
    "Número do DUF",
    "CNPJ/CPF",
    "Razão Social",
    "Vencimento GRM",
    "Valor da Multa",
    "Valor Total Pago",
];

#[derive(Debug, PartialEq, Eq)]
pub enum FineError {
    Budget,
    Contract,
    Framing,
}

#[derive(Debug, Clone, Serialize)]
pub struct FineObservation {
    pub schema_version: &'static str,
    pub source: &'static str,
    /// Content identity of one minimized observation; NOT an ANP fine number.
    pub observation_key: String,
    pub source_key: String,
    pub proceeding_ref: String,
    pub duf_ref: String,
    pub department: String,
    pub status: String,
    pub due_on: String,
    pub amount_text: String,
    pub amount_minor: i64,
    pub paid_text: String,
    pub paid_minor: i64,
    pub report_year: u16,
    pub extraction_month: String,
    pub evidence_ref: String,
}
#[derive(Debug)]
pub struct FineBatch {
    pub report_year: u16,
    pub extraction_month: String,
    pub counts: Counts,
    pub snapshots: Vec<FineObservation>,
    pub duplicates: Vec<FineObservation>,
    pub quarantine: Vec<QuarantineRow>,
}
impl FineBatch {
    pub fn manifest_hash(&self) -> String {
        // JSON boundaries prevent ambiguous field concatenation. Row evidence is
        // retained in artifacts; the manifest is bound to the original byte hash.
        sha256_hex(
            &serde_json::to_vec(&(
                self.report_year,
                &self.extraction_month,
                &self.snapshots,
                &self.duplicates,
                &self.quarantine,
                self.counts.input,
                self.counts.accepted,
                self.counts.duplicates,
                self.counts.quarantined,
            ))
            .expect("serializable"),
        )
    }
}
fn month(label: &str) -> Option<u8> {
    [
        "janeiro",
        "fevereiro",
        "março",
        "abril",
        "maio",
        "junho",
        "julho",
        "agosto",
        "setembro",
        "outubro",
        "novembro",
        "dezembro",
    ]
    .iter()
    .position(|v| *v == label)
    .map(|n| n as u8 + 1)
}
fn extraction(preamble: &str, report_year: u16) -> Option<String> {
    let prefix = format!("RELAÇÃO DE MULTAS APLICADAS EM {report_year}  (Relatório extraído em ");
    let text = preamble.strip_prefix(&prefix)?.strip_suffix(')')?;
    let (m, y) = text.split_once('/')?;
    if y.len() != 4 || !y.bytes().all(|b| b.is_ascii_digit()) {
        return None;
    }
    let year = y.parse::<u16>().ok()?;
    if year < report_year {
        return None;
    }
    Some(format!("{year:04}-{:02}", month(m)?))
}
fn money(value: &str) -> Option<i64> {
    let (units, cents) = value.split_once(',')?;
    if cents.len() != 2 || !cents.bytes().all(|b| b.is_ascii_digit()) {
        return None;
    }
    let groups: Vec<_> = units.split('.').collect();
    if groups[0].is_empty()
        || (groups[0].len() > 1 && groups[0].starts_with('0'))
        || !groups[0].bytes().all(|b| b.is_ascii_digit())
        || (groups.len() > 1
            && (groups[0].len() > 3
                || groups
                    .iter()
                    .skip(1)
                    .any(|g| g.len() != 3 || !g.bytes().all(|b| b.is_ascii_digit()))))
    {
        return None;
    }
    groups
        .join("")
        .parse::<i64>()
        .ok()?
        .checked_mul(100)?
        .checked_add(cents.parse::<i64>().ok()?)
}
fn due(value: &str) -> Option<String> {
    let b = value.as_bytes();
    if b.len() != 10
        || b[2] != b'/'
        || b[5] != b'/'
        || !b
            .iter()
            .enumerate()
            .all(|(i, c)| i == 2 || i == 5 || c.is_ascii_digit())
    {
        return None;
    }
    let day = value[..2].parse::<u32>().ok()?;
    let month = value[3..5].parse::<u32>().ok()?;
    let year = value[6..].parse::<u32>().ok()?;
    let max = match month {
        1 | 3 | 5 | 7 | 8 | 10 | 12 => 31,
        4 | 6 | 9 | 11 => 30,
        2 => {
            if year % 4 == 0 && (year % 100 != 0 || year % 400 == 0) {
                29
            } else {
                28
            }
        }
        _ => 0,
    };
    if year == 0 || day == 0 || day > max {
        return None;
    }
    Some(format!("{year:04}-{month:02}-{day:02}"))
}
fn quarantine(file: &str, row: usize, reason: &'static str) -> QuarantineRow {
    QuarantineRow {
        schema_version: "station-quarantine-v1",
        source: "sanctions",
        row_locator: format!("{file}:{row}"),
        reason,
        detail: "official fine row refused; raw fields redacted".to_string(),
        source_key: None,
    }
}
/// Exact CP850 input contract. UTF-8 is not silently interpreted as a second
/// profile: its accented headers fail this contract. No partial batch on error.
pub fn parse_anp_fines(
    file: &str,
    raw: Vec<u8>,
    report_year: u16,
    limits: &Limits,
) -> Result<FineBatch, FineError> {
    if raw.len() as u64 > limits.max_bytes {
        return Err(FineError::Budget);
    }
    if !(2016..=9999).contains(&report_year)
        || file.is_empty()
        || file.len() > 128
        || !file
            .bytes()
            .all(|b| b.is_ascii_alphanumeric() || b"._-".contains(&b))
        || file == "."
        || file == ".."
    {
        return Err(FineError::Contract);
    }
    if !raw.ends_with(b"\n") {
        return Err(FineError::Framing);
    }
    let table: Vec<char> = CP850.chars().collect();
    let text: String = raw
        .iter()
        .map(|b| {
            if *b < 128 {
                *b as char
            } else {
                table[(*b - 128) as usize]
            }
        })
        .collect();
    let mut reader = csv::ReaderBuilder::new()
        .delimiter(b';')
        .has_headers(false)
        .flexible(true)
        .from_reader(text.as_bytes());
    let mut records = reader.records();
    let mut extraction_month = String::new();
    for index in 0..7 {
        let row = records
            .next()
            .ok_or(FineError::Contract)?
            .map_err(|_| FineError::Framing)?;
        if row.len() != 9 {
            return Err(FineError::Contract);
        }
        if index == 6 {
            if !row.iter().eq(HEADER.iter().copied()) {
                return Err(FineError::Contract);
            }
        } else {
            if row.iter().skip(1).any(|v| !v.is_empty()) {
                return Err(FineError::Contract);
            }
            match index {
                0 => {
                    if &row[0]
                        != "ANP - Agência Nacional do Petroleo, Gás Natural e Biocombustíveis"
                    {
                        return Err(FineError::Contract);
                    }
                }
                2 => {
                    extraction_month =
                        extraction(&row[0], report_year).ok_or(FineError::Contract)?
                }
                3 => {
                    if &row[0] != "Fonte: Sistema de Controle de Multas - SICOM" {
                        return Err(FineError::Contract);
                    }
                }
                _ => {
                    if !row[0].is_empty() {
                        return Err(FineError::Contract);
                    }
                }
            }
        }
    }
    let mut batch = FineBatch {
        report_year,
        extraction_month: extraction_month.clone(),
        counts: Counts::default(),
        snapshots: vec![],
        duplicates: vec![],
        quarantine: vec![],
    };
    let mut seen = HashSet::new();
    for (offset, next) in records.enumerate() {
        if batch.counts.input >= limits.max_rows {
            return Err(FineError::Budget);
        }
        batch.counts.input += 1;
        let locator = offset + 8;
        let row = next.map_err(|_| FineError::Framing)?;
        let refusal = if row.len() != 9 {
            Some(reason::INVALID_FIELD)
        } else if row.iter().any(|v| v.len() > limits.max_field_bytes) {
            Some(reason::RECORD_TOO_LARGE)
        } else if normalize_cnpj(&row[4]).is_err() {
            Some(reason::INVALID_CNPJ)
        } else if [0, 1, 2, 3].iter().any(|i| row[*i].trim().is_empty())
            || [0, 1, 2, 3, 6, 7, 8]
                .iter()
                .any(|i| row[*i].chars().any(char::is_control))
            || due(&row[6]).is_none()
            || money(&row[7]).is_none()
            || money(&row[8]).is_none()
        {
            Some(reason::INVALID_FIELD)
        } else {
            None
        };
        if let Some(reason) = refusal {
            batch.counts.quarantined += 1;
            batch.quarantine.push(quarantine(file, locator, reason));
            continue;
        }
        let mut snapshot = FineObservation {
            schema_version: "anp-fine-observation-v1",
            source: "sanctions",
            observation_key: String::new(),
            source_key: normalize_cnpj(&row[4]).expect("checked"),
            proceeding_ref: row[2].to_string(),
            duf_ref: row[3].to_string(),
            department: row[1].to_string(),
            status: row[0].to_string(),
            due_on: due(&row[6]).expect("checked"),
            amount_text: row[7].to_string(),
            amount_minor: money(&row[7]).expect("checked"),
            paid_text: row[8].to_string(),
            paid_minor: money(&row[8]).expect("checked"),
            report_year,
            extraction_month: extraction_month.clone(),
            evidence_ref: String::new(),
        };
        snapshot.observation_key = format!(
            "observation:{}",
            sha256_hex(&serde_json::to_vec(&snapshot).expect("serializable"))
        );
        snapshot.evidence_ref = format!("{file}:{locator}");
        if seen.insert(snapshot.observation_key.clone()) {
            batch.counts.accepted += 1;
            batch.snapshots.push(snapshot);
        } else {
            batch.counts.duplicates += 1;
            batch.duplicates.push(snapshot);
        }
    }
    batch
        .snapshots
        .sort_by(|a, b| a.observation_key.cmp(&b.observation_key));
    Ok(batch)
}

//! Exact historical SICOM profiles. Raw source facts and uncertainty survive;
//! no reference/date/payment label is silently converted to another meaning.
use crate::cnpj::normalize_cnpj;
use crate::sanction_anp::{due, extraction, money, FineError, CP850};
use crate::types::{reason, sha256_hex, Counts, Limits, QuarantineRow};
use serde::Serialize;
use std::collections::HashSet;

const AGENCY: &str = "ANP - Agência Nacional do Petroleo, Gás Natural e Biocombustíveis";
const SOURCE: &str = "Fonte: Sistema de Controle de Multas - SICOM";
const NOTE_PAYMENT: &str = "(*) Para verificação do valor total pago dos processos com status parcelamento em andamento é preciso acessar o link: https://app.anp.gov.br/anp-gpc-consulta-web/public/emissao-gru/list.xhtml";
const NOTE_2020: &str = "(**) Parcelamentos feitos via SAPIENS são controlados pela Procuradoria Federal. Para obter os valores já pagos entrar em contato com a AGU.";
const NOTE_2021: &str = "(**) Parcelamentos SAPIENS são controlados pela Procuradoria Federal. Para obter o s valores já pagos entrar em contato com a AGU.";

#[derive(Debug, Clone, Serialize, PartialEq, Eq)]
pub struct HistoryClock {
    pub precision: &'static str,
    pub value: String,
}
#[derive(Debug, Clone, Serialize)]
pub struct HistoryObservation {
    pub schema_version: &'static str,
    pub source: &'static str,
    pub observation_key: String,
    pub source_key: String,
    pub proceeding_ref: String,
    pub reference_label: &'static str,
    pub reference_ref: String,
    pub department: String,
    pub status: String,
    pub source_transit_on: Option<String>,
    pub due_on: String,
    pub amount_text: String,
    pub amount_minor: i64,
    pub paid_text: String,
    pub paid_minor: Option<i64>,
    /// Deliberately no semantic label: the source header is empty.
    pub unlabelled_field: String,
    pub source_period: String,
    pub clock: HistoryClock,
    pub input_contract: &'static str,
    pub evidence_ref: String,
}
#[derive(Debug, Serialize)]
pub struct HistoryMetadata {
    pub schema_version: &'static str,
    pub kind: &'static str,
    pub text: String,
    pub evidence_ref: String,
}
#[derive(Debug)]
pub struct HistoryBatch {
    pub input_contract: &'static str,
    pub source_period: String,
    pub clock: HistoryClock,
    pub header_record: usize,
    pub counts: Counts,
    pub observations: Vec<HistoryObservation>,
    pub duplicates: Vec<HistoryObservation>,
    pub quarantine: Vec<QuarantineRow>,
    /// Public note annotations refer only to source_metadata quarantine rows.
    /// They are never additional fine/event observations.
    pub metadata: Vec<HistoryMetadata>,
}
impl HistoryBatch {
    pub fn manifest_hash(&self) -> String {
        sha256_hex(
            &serde_json::to_vec(&(
                self.input_contract,
                &self.source_period,
                &self.clock,
                self.header_record,
                &self.observations,
                &self.duplicates,
                &self.quarantine,
                &self.metadata,
                self.counts.input,
                self.counts.accepted,
                self.counts.duplicates,
                self.counts.quarantined,
            ))
            .expect("serializable"),
        )
    }
}
struct Profile {
    contract: &'static str,
    period: &'static str,
    name: &'static str,
    header_record: usize,
    reference_label: &'static str,
    transit: bool,
    utf8: bool,
    header: Vec<&'static str>,
}
fn profile(period: &str) -> Option<Profile> {
    let common = vec!["Status Processo", "Superintendência", "Número do Processo"];
    let (contract, period, name, header_record, reference_label, transit, utf8, tail) = match period
    {
        "2016-2019" => (
            "anp-sicom-utf8bom-group-2016-2019-v1",
            "2016-2019",
            "multas-aplicadas-2016a2019.csv",
            5,
            "Auto de Infração",
            true,
            true,
            vec![
                "Data Transito Julgado",
                "Vencimento ",
                "Valor da Multa Aplicada",
                "Valor Total Pago (*) (**)",
            ],
        ),
        "2020" => (
            "anp-sicom-utf8bom-2020-v1",
            "2020",
            "multas-aplicadas-2020.csv",
            7,
            "Auto de Infração",
            true,
            true,
            vec![
                "Data Transito Julgado",
                "Vencimento",
                " Valor  da Multa Aplicada ",
                " Valor Total Pago (*) (**)  ",
            ],
        ),
        "2021" => (
            "anp-sicom-utf8bom-csv-named-xlsx-2021-v1",
            "2021",
            "multas-2021-junho22.xlsx",
            7,
            "Auto de Infração",
            true,
            true,
            vec![
                "Data Transito Julgado",
                "Vencimento",
                "Valor  da Multa Aplicada",
                "Valor Total Pago (*) (**) ",
            ],
        ),
        "2022" => (
            "anp-sicom-utf8bom-2022-v1",
            "2022",
            "multas-aplicadas-2022.csv",
            7,
            "Número do DUF",
            true,
            true,
            vec![
                "Data Transito Julgado",
                "Vencimento",
                "Valor da Multa",
                "Valor Total Pago",
            ],
        ),
        "2023" => (
            "anp-sicom-cp850-2023-v1",
            "2023",
            "multas-aplicadas-2023.csv",
            6,
            "Número do DUF",
            false,
            false,
            vec![
                "Vencimento da Multa",
                " Valor da Multa ",
                " Valor Total Pago ",
            ],
        ),
        "2024" => (
            "anp-sicom-utf8bom-2024-v1",
            "2024",
            "multas-aplicadas-2024.csv",
            7,
            "Número do DUF",
            false,
            true,
            vec!["Vencimento", "Valor da Multa", "Valor Total Pago", ""],
        ),
        _ => return None,
    };
    let mut header = common;
    header.extend([reference_label, "CNPJ/CPF", "Razão Social"]);
    header.extend(tail);
    Some(Profile {
        contract,
        period,
        name,
        header_record,
        reference_label,
        transit,
        utf8,
        header,
    })
}
fn value_money(value: &str) -> Option<i64> {
    let s = value.trim();
    money(s.strip_prefix("R$").map(str::trim).unwrap_or(s))
}
fn unknown_paid(value: &str) -> bool {
    value
        .trim()
        .strip_prefix("R$")
        .is_some_and(|s| s.trim() == "-")
}
fn quarantine(file: &str, number: usize, why: &'static str) -> QuarantineRow {
    QuarantineRow {
        schema_version: "station-quarantine-v1",
        source: "sanctions",
        row_locator: format!("{file}:{number}"),
        reason: why,
        detail: "historical fine row classified; raw fields redacted".into(),
        source_key: None,
    }
}

// The CSV crate deliberately accepts an unfinished quoted field at EOF. A
// snapshot must instead end outside quotes, even when the last byte is LF.
// Scan without allocations; quoted multiline cells and doubled quotes are legal.
fn complete_framing(text: &str) -> bool {
    let bytes = text.as_bytes();
    let (mut index, mut quoted, mut field_start, mut closed) = (0, false, true, false);
    while index < bytes.len() {
        let b = bytes[index];
        if quoted {
            if b == b'"' {
                if bytes.get(index + 1) == Some(&b'"') {
                    index += 2;
                    continue;
                }
                quoted = false;
                closed = true;
            }
        } else if closed {
            match b {
                b';' | b'\n' => {
                    closed = false;
                    field_start = true;
                }
                b'\r' if bytes.get(index + 1) == Some(&b'\n') => {}
                _ => return false,
            }
        } else {
            match b {
                b'"' if field_start => {
                    quoted = true;
                    field_start = false;
                }
                b';' | b'\n' => field_start = true,
                b'\r' if bytes.get(index + 1) == Some(&b'\n') => {}
                _ => field_start = false,
            }
        }
        index += 1;
    }
    !quoted
}
/// File/profile/encoding/preamble mismatches refuse the whole batch. Record-level
/// refusals have redacted locators; every logical post-header record reconciles.
pub fn parse_fine_history(
    file: &str,
    raw: Vec<u8>,
    period: &str,
    limits: &Limits,
) -> Result<HistoryBatch, FineError> {
    if raw.len() as u64 > limits.max_bytes {
        return Err(FineError::Budget);
    }
    let p = profile(period).ok_or(FineError::Contract)?;
    if file != p.name {
        return Err(FineError::Contract);
    }
    if !raw.ends_with(b"\n") {
        return Err(FineError::Framing);
    }
    let text = if p.utf8 {
        let bytes = raw
            .strip_prefix(&[0xef, 0xbb, 0xbf])
            .ok_or(FineError::Contract)?;
        std::str::from_utf8(bytes)
            .map_err(|_| FineError::Contract)?
            .to_owned()
    } else {
        let table: Vec<char> = CP850.chars().collect();
        raw.iter()
            .map(|b| {
                if *b < 128 {
                    *b as char
                } else {
                    table[(*b - 128) as usize]
                }
            })
            .collect()
    };
    if !complete_framing(&text) {
        return Err(FineError::Framing);
    }
    let mut reader = csv::ReaderBuilder::new()
        .delimiter(b';')
        .has_headers(false)
        .flexible(true)
        .from_reader(text.as_bytes());
    let mut records = reader.records();
    let mut clock = None;
    for index in 0..p.header_record {
        let row = records
            .next()
            .ok_or(FineError::Contract)?
            .map_err(|_| FineError::Framing)?;
        if row.len() != p.header.len() {
            return Err(FineError::Contract);
        }
        if index + 1 == p.header_record {
            if !row.iter().eq(p.header.iter().copied()) {
                return Err(FineError::Contract);
            }
            continue;
        }
        if p.period == "2016-2019" {
            if row
                .iter()
                .enumerate()
                .any(|(i, v)| i != 0 && !(index == 0 && i == 4) && !v.is_empty())
            {
                return Err(FineError::Contract);
            }
            match index {
                0 => {
                    if &row[0] != AGENCY {
                        return Err(FineError::Contract);
                    }
                    let day = row[4]
                        .strip_prefix("Data: ")
                        .and_then(due)
                        .ok_or(FineError::Contract)?;
                    if day.as_str() < "2020-01-01" {
                        return Err(FineError::Contract);
                    }
                    clock = Some(HistoryClock {
                        precision: "DAY",
                        value: day,
                    });
                }
                1 => {
                    if &row[0] != "SICOM" {
                        return Err(FineError::Contract);
                    }
                }
                2 => {
                    if &row[0] != "Financeiro (GRM)" {
                        return Err(FineError::Contract);
                    }
                }
                _ => {
                    if !row[0].is_empty() {
                        return Err(FineError::Contract);
                    }
                }
            }
        } else {
            if row.iter().skip(1).any(|v| !v.is_empty()) {
                return Err(FineError::Contract);
            }
            match index {
                0 => {
                    if &row[0] != AGENCY {
                        return Err(FineError::Contract);
                    }
                }
                2 => {
                    clock = Some(HistoryClock {
                        precision: "MONTH",
                        value: extraction(
                            &row[0],
                            p.period.parse().map_err(|_| FineError::Contract)?,
                        )
                        .ok_or(FineError::Contract)?,
                    });
                }
                3 => {
                    if &row[0] != SOURCE {
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
    let clock = clock.ok_or(FineError::Contract)?;
    let mut batch = HistoryBatch {
        input_contract: p.contract,
        source_period: p.period.into(),
        clock: clock.clone(),
        header_record: p.header_record,
        counts: Counts::default(),
        observations: vec![],
        duplicates: vec![],
        quarantine: vec![],
        metadata: vec![],
    };
    let mut seen = HashSet::new();
    let mut footer = false;
    for (offset, next) in records.enumerate() {
        if batch.counts.input >= limits.max_rows {
            return Err(FineError::Budget);
        }
        batch.counts.input += 1;
        let number = offset + p.header_record + 1;
        let row = next.map_err(|_| FineError::Framing)?;
        if row.iter().any(|v| v.len() > limits.max_field_bytes) {
            batch.counts.quarantined += 1;
            batch
                .quarantine
                .push(quarantine(file, number, reason::RECORD_TOO_LARGE));
            continue;
        }
        let due_index = if p.transit { 7 } else { 6 };
        let amount_index = due_index + 1;
        let paid_index = due_index + 2;
        // Only the exact independently inspected public notes, with every other
        // cell empty, can become metadata. Unknown footer content stays redacted.
        let metadata = (p.period == "2020" || p.period == "2021")
            && row.len() == p.header.len()
            && row.iter().skip(1).all(str::is_empty)
            && (row[0].is_empty()
                || &row[0] == NOTE_PAYMENT
                || &row[0]
                    == if p.period == "2020" {
                        NOTE_2020
                    } else {
                        NOTE_2021
                    });
        if metadata {
            footer = true;
            batch.counts.quarantined += 1;
            batch
                .quarantine
                .push(quarantine(file, number, "source_metadata"));
            batch.metadata.push(HistoryMetadata {
                schema_version: "anp-fine-source-note-v1",
                kind: if row[0].is_empty() {
                    "BLANK_SEPARATOR"
                } else {
                    "PUBLIC_PAYMENT_NOTE"
                },
                text: row[0].into(),
                evidence_ref: format!("{file}:{number}"),
            });
            continue;
        }
        let refusal = if row.len() != p.header.len() {
            Some(reason::INVALID_FIELD)
        } else if normalize_cnpj(&row[4]).is_err() {
            Some(reason::INVALID_CNPJ)
        } else if footer {
            return Err(FineError::Contract);
        } else if row.iter().enumerate().any(|(i, v)| {
            i != 5
                && v.chars()
                    .any(|c| c.is_control() || matches!(c, '\u{2028}' | '\u{2029}'))
        }) || [0, 2, 3].iter().any(|i| row[*i].trim().is_empty())
            || due(&row[due_index]).is_none()
            || value_money(&row[amount_index]).is_none()
            || (value_money(&row[paid_index]).is_none()
                && !(p.period == "2023" && unknown_paid(&row[paid_index])))
            || (p.transit && !row[6].is_empty() && due(&row[6]).is_none())
        {
            Some(reason::INVALID_FIELD)
        } else {
            None
        };
        if let Some(why) = refusal {
            batch.counts.quarantined += 1;
            batch.quarantine.push(quarantine(file, number, why));
            continue;
        }
        let mut observation = HistoryObservation {
            schema_version: "anp-fine-history-observation-v1",
            source: "sanctions",
            observation_key: String::new(),
            source_key: normalize_cnpj(&row[4]).expect("checked"),
            proceeding_ref: row[2].into(),
            reference_label: p.reference_label,
            reference_ref: row[3].into(),
            department: row[1].into(),
            status: row[0].into(),
            source_transit_on: if p.transit && !row[6].is_empty() {
                Some(due(&row[6]).expect("checked"))
            } else {
                None
            },
            due_on: due(&row[due_index]).expect("checked"),
            amount_text: row[amount_index].into(),
            amount_minor: value_money(&row[amount_index]).expect("checked"),
            paid_text: row[paid_index].into(),
            paid_minor: value_money(&row[paid_index]),
            unlabelled_field: if p.period == "2024" {
                row[9].into()
            } else {
                String::new()
            },
            source_period: p.period.into(),
            clock: clock.clone(),
            input_contract: p.contract,
            evidence_ref: String::new(),
        };
        observation.observation_key = format!(
            "observation:{}",
            sha256_hex(&serde_json::to_vec(&observation).expect("serializable"))
        );
        observation.evidence_ref = format!("{file}:{number}");
        if seen.insert(observation.observation_key.clone()) {
            batch.counts.accepted += 1;
            batch.observations.push(observation);
        } else {
            batch.counts.duplicates += 1;
            batch.duplicates.push(observation);
        }
    }
    batch
        .observations
        .sort_by(|a, b| a.observation_key.cmp(&b.observation_key));
    Ok(batch)
}

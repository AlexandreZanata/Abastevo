//! Exact per-station ANP survey facts.
//!
//! Parses the local `POSTOS REVENDEDORES` XLSX sheet without network or
//! database. Municipal/state averages are never accepted as station facts.
//! Prices stay exact integer milli-BRL; unknown/ambiguous rows quarantine.
//! This does not replace the existing Android importer.

use crate::cnpj::normalize_cnpj;
use crate::types::{reason, sha256_hex, Counts, Limits, QuarantineRow, RunState};
use crate::xlsx::{read_shared_strings, read_workbook, SheetRows, XlsxError};
use crate::zip::{ZipArchive, ZipLimits};
use serde::Serialize;
use std::collections::HashMap;
use std::fs::{self, File};
use std::io::{BufWriter, Write};
use std::path::Path;

pub const PRICE_FORMAT: &str = "abastevo.anp-station-prices-v1";
pub const PARSER_VERSION: &str = "anp-station-prices-v1";
pub const SOURCE: &str = "anp-survey";
pub const SHEET_NAME: &str = "POSTOS REVENDEDORES";
pub const HEADER_ROW: u64 = 10;

#[derive(Debug)]
pub enum AnpPriceError {
    Oversize,
    RowCap,
    Truncated(String),
    Profile(String),
    Cancelled,
}

impl std::fmt::Display for AnpPriceError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            AnpPriceError::Oversize => f.write_str("input exceeds the byte budget"),
            AnpPriceError::RowCap => f.write_str("row cap exceeded"),
            AnpPriceError::Truncated(detail) => write!(f, "truncated: {detail}"),
            AnpPriceError::Profile(detail) => write!(f, "profile refused: {detail}"),
            AnpPriceError::Cancelled => f.write_str("cancelled"),
        }
    }
}

impl std::error::Error for AnpPriceError {}

fn map_xlsx(error: XlsxError) -> AnpPriceError {
    AnpPriceError::Truncated(format!("{error:?}"))
}

#[derive(Debug, Clone, Serialize)]
pub struct AnpPriceFact {
    pub schema_version: &'static str,
    pub source: &'static str,
    pub source_key: String,
    pub cnpj: String,
    pub station_name: Option<String>,
    pub trade_name: Option<String>,
    pub municipality: String,
    pub state: String,
    pub brand: Option<String>,
    pub product: String,
    pub unit: String,
    pub amount_milli_brl: i64,
    pub raw_price_text: String,
    pub observed_at: String,
    pub period_start: String,
    pub period_end: String,
    pub source_revision: String,
    pub source_row: u64,
}

#[derive(Debug)]
pub struct AnpPriceRun {
    pub state: RunState,
    pub counts: Counts,
    pub facts: Vec<AnpPriceFact>,
    pub duplicates: Vec<AnpPriceFact>,
    pub quarantine: Vec<QuarantineRow>,
    pub source_sha256: String,
    pub source_revision: String,
    pub period_start: String,
    pub period_end: String,
}

/// Parses one admitted local ANP station-detail workbook. `period_start` and
/// `period_end` are the survey's explicit `YYYY-MM-DD` bounds.
pub fn parse_station_prices_xlsx(
    file_name: &str,
    raw: &[u8],
    limits: &Limits,
    period_start: &str,
    period_end: &str,
    source_revision: &str,
    cancel: &dyn Fn() -> bool,
) -> Result<AnpPriceRun, AnpPriceError> {
    if raw.len() as u64 > limits.max_bytes {
        return Err(AnpPriceError::Oversize);
    }
    if !valid_day(period_start) || !valid_day(period_end) || period_start > period_end {
        return Err(AnpPriceError::Profile("invalid survey period".into()));
    }
    let archive = ZipArchive::open(raw.to_vec(), ZipLimits::default())
        .map_err(|error| AnpPriceError::Truncated(format!("{error:?}")))?;
    let workbook = read_workbook(&archive).map_err(map_xlsx)?;
    if workbook.sheet_name != SHEET_NAME {
        return Err(AnpPriceError::Profile(format!(
            "expected sheet {SHEET_NAME:?}, found {:?}",
            workbook.sheet_name
        )));
    }
    let strings = read_shared_strings(&archive).map_err(map_xlsx)?;
    let rows = SheetRows::new(&archive, &workbook.sheet_path, &strings, 15).map_err(map_xlsx)?;
    let mut counts = Counts::default();
    let mut facts: Vec<AnpPriceFact> = Vec::new();
    let mut duplicates: Vec<AnpPriceFact> = Vec::new();
    let mut quarantine: Vec<QuarantineRow> = Vec::new();
    let mut seen: HashMap<String, AnpPriceFact> = HashMap::new();
    let mut saw_header = false;
    for row in rows {
        if cancel() {
            return Err(AnpPriceError::Cancelled);
        }
        let row = row.map_err(map_xlsx)?;
        if row.number < HEADER_ROW {
            continue;
        }
        let cells = pad(&row.cells);
        if row.number == HEADER_ROW {
            validate_header(&cells)?;
            saw_header = true;
            continue;
        }
        if cells.iter().all(|cell| cell.trim().is_empty()) {
            continue;
        }
        counts.input += 1;
        if counts.input > limits.max_rows {
            return Err(AnpPriceError::RowCap);
        }
        let locator = format!("{file_name}:{}", row.number);
        let Some(cnpj) = normalize_cnpj(cells[0].trim()).ok() else {
            quarantine.push(q(locator, reason::INVALID_CNPJ, "CNPJ is not valid", None));
            counts.quarantined += 1;
            continue;
        };
        let municipality = cells[8].trim().to_string();
        let state = cells[9].trim().to_uppercase();
        if municipality.is_empty() || state.len() != 2 {
            quarantine.push(q(
                locator,
                reason::MISSING_STATION_CONTEXT,
                "municipality/state required",
                Some(cnpj.clone()),
            ));
            counts.quarantined += 1;
            continue;
        }
        let Some((product, expected_unit)) = product(&cells[11]) else {
            quarantine.push(q(
                locator,
                reason::UNMAPPED_PRODUCT,
                "unsupported product label",
                Some(cnpj.clone()),
            ));
            counts.quarantined += 1;
            continue;
        };
        let unit = cells[12].trim().to_uppercase();
        if unit != expected_unit {
            quarantine.push(q(
                locator,
                reason::PRODUCT_UNIT_MISMATCH,
                "unit does not match product",
                Some(cnpj.clone()),
            ));
            counts.quarantined += 1;
            continue;
        }
        let raw_price_text = cells[13].trim().to_string();
        let Some(amount_milli_brl) = parse_milli_brl(&raw_price_text) else {
            quarantine.push(q(
                locator,
                reason::PRICE_OUT_OF_RANGE,
                "price is not exact milli-BRL",
                Some(cnpj.clone()),
            ));
            counts.quarantined += 1;
            continue;
        };
        let Some(observed_at) = parse_date_cell(&cells[14], workbook.date1904) else {
            quarantine.push(q(
                locator,
                reason::MISSING_OBSERVED_AT,
                "collected date is missing or invalid",
                Some(cnpj.clone()),
            ));
            counts.quarantined += 1;
            continue;
        };
        if observed_at.as_str() < period_start || observed_at.as_str() > period_end {
            quarantine.push(q(
                locator,
                reason::OUTSIDE_PERIOD,
                "collected date is outside the survey period",
                Some(cnpj.clone()),
            ));
            counts.quarantined += 1;
            continue;
        }
        let source_key = sha256_hex(
            format!("{SOURCE}|{cnpj}|{product}|{unit}|{observed_at}|{period_start}|{period_end}")
                .as_bytes(),
        );
        let fact = AnpPriceFact {
            schema_version: "anp-station-price-v1",
            source: SOURCE,
            source_key: source_key.clone(),
            cnpj: cnpj.clone(),
            station_name: nonempty(&cells[1]),
            trade_name: nonempty(&cells[2]),
            municipality,
            state,
            brand: nonempty(&cells[10]),
            product: product.to_string(),
            unit: unit.clone(),
            amount_milli_brl,
            raw_price_text,
            observed_at,
            period_start: period_start.to_string(),
            period_end: period_end.to_string(),
            source_revision: source_revision.to_string(),
            source_row: row.number,
        };
        if let Some(previous) = seen.get(&source_key) {
            if previous.amount_milli_brl == fact.amount_milli_brl {
                duplicates.push(fact);
                counts.duplicates += 1;
            } else {
                quarantine.push(q(
                    locator,
                    reason::CONFLICTING_STATION_FACT,
                    "same station/product/date with a different price",
                    Some(cnpj),
                ));
                counts.quarantined += 1;
            }
            continue;
        }
        seen.insert(source_key, fact.clone());
        facts.push(fact);
        counts.accepted += 1;
    }
    if !saw_header {
        return Err(AnpPriceError::Profile("header row missing".into()));
    }
    debug_assert_eq!(
        counts.input,
        counts.accepted + counts.duplicates + counts.quarantined
    );
    facts.sort_by(|a, b| a.source_key.cmp(&b.source_key));
    duplicates.sort_by(|a, b| a.source_key.cmp(&b.source_key));
    quarantine.sort_by(|a, b| a.row_locator.cmp(&b.row_locator));
    Ok(AnpPriceRun {
        state: if counts.quarantined == 0 {
            RunState::Complete
        } else {
            RunState::Quarantined
        },
        counts,
        facts,
        duplicates,
        quarantine,
        source_sha256: sha256_hex(raw),
        source_revision: source_revision.to_string(),
        period_start: period_start.to_string(),
        period_end: period_end.to_string(),
    })
}

fn q(locator: String, reason: &'static str, detail: &str, key: Option<String>) -> QuarantineRow {
    QuarantineRow {
        schema_version: "station-quarantine-v1",
        source: SOURCE,
        row_locator: locator,
        reason,
        detail: detail.to_string(),
        source_key: key,
    }
}

fn pad(cells: &[String]) -> Vec<String> {
    let mut out = cells.to_vec();
    out.resize(15, String::new());
    out
}

fn validate_header(cells: &[String]) -> Result<(), AnpPriceError> {
    let checks = [
        (0, "CNPJ"),
        (8, "MUNIC"),
        (9, "ESTADO"),
        (11, "PRODUTO"),
        (12, "UNIDADE"),
        (13, "PRE"),
        (14, "DATA"),
    ];
    for (index, needle) in checks {
        let value = fold(&cells[index]);
        if !value.contains(needle) {
            return Err(AnpPriceError::Profile(format!(
                "header mismatch at column {index}: expected {needle:?}, found {:?}",
                cells[index]
            )));
        }
    }
    Ok(())
}

fn fold(value: &str) -> String {
    value
        .trim()
        .chars()
        .map(|character| match character {
            'Á' | 'À' | 'Â' | 'Ã' | 'Ä' | 'á' | 'à' | 'â' | 'ã' | 'ä' => 'A',
            'É' | 'È' | 'Ê' | 'Ë' | 'é' | 'è' | 'ê' | 'ë' => 'E',
            'Í' | 'Ì' | 'Î' | 'Ï' | 'í' | 'ì' | 'î' | 'ï' => 'I',
            'Ó' | 'Ò' | 'Ô' | 'Õ' | 'Ö' | 'ó' | 'ò' | 'ô' | 'õ' | 'ö' => 'O',
            'Ú' | 'Ù' | 'Û' | 'Ü' | 'ú' | 'ù' | 'û' | 'ü' => 'U',
            'Ç' | 'ç' => 'C',
            other => other,
        })
        .collect::<String>()
        .to_uppercase()
}

fn product(label: &str) -> Option<(&'static str, &'static str)> {
    match fold(label).as_str() {
        "ETANOL HIDRATADO" | "ETANOL" => Some(("ETHANOL_HYDRATED", "L")),
        "GASOLINA COMUM" => Some(("GASOLINE_REGULAR", "L")),
        "GASOLINA ADITIVADA" => Some(("GASOLINE_ADDITIVED", "L")),
        "GASOLINA PREMIUM" | "GASOLINA PREMIUM GRADE" => Some(("GASOLINE_PREMIUM_GRADE", "L")),
        "OLEO DIESEL" | "DIESEL S500" => Some(("DIESEL_S500", "L")),
        "OLEO DIESEL S10" | "DIESEL S10" => Some(("DIESEL_S10", "L")),
        "GNV" => Some(("CNG", "M3")),
        "GLP" | "GLP P13" => Some(("LPG_P13", "KG_13")),
        _ => None,
    }
}

fn nonempty(value: &str) -> Option<String> {
    let trimmed = value.trim();
    if trimmed.is_empty() {
        None
    } else {
        Some(trimmed.to_string())
    }
}

/// Exact `amount_milli_brl` from a Brazilian/Excel decimal lexeme.
/// Accepts at most three decimal places; never rounds.
pub fn parse_milli_brl(raw: &str) -> Option<i64> {
    let mut text = raw.trim().trim_start_matches("R$").trim().replace(' ', "");
    if text.is_empty() || text.starts_with('-') || text.starts_with('+') {
        return None;
    }
    if text.contains(',') {
        if text.matches(',').count() != 1 {
            return None;
        }
        text = text.replace('.', "");
        text = text.replace(',', ".");
    }
    let mut parts = text.split('.');
    let integer = parts.next()?;
    let fraction = parts.next().unwrap_or("");
    if parts.next().is_some() || integer.is_empty() || !integer.chars().all(|c| c.is_ascii_digit())
    {
        return None;
    }
    if fraction.len() > 3 || !fraction.chars().all(|c| c.is_ascii_digit()) {
        return None;
    }
    let integer: i64 = integer.parse().ok()?;
    let fraction: i64 = if fraction.is_empty() {
        0
    } else {
        format!("{fraction:0<3}").parse().ok()?
    };
    let amount = integer.checked_mul(1000)?.checked_add(fraction)?;
    if (1..=1_000_000).contains(&amount) {
        Some(amount)
    } else {
        None
    }
}

fn valid_day(value: &str) -> bool {
    if value.len() != 10 {
        return false;
    }
    let bytes = value.as_bytes();
    if bytes[4] != b'-' || bytes[7] != b'-' {
        return false;
    }
    let (year, month, day) = match (
        value[0..4].parse::<i32>(),
        value[5..7].parse::<u32>(),
        value[8..10].parse::<u32>(),
    ) {
        (Ok(year), Ok(month), Ok(day)) => (year, month, day),
        _ => return false,
    };
    (1900..=2100).contains(&year) && (1..=12).contains(&month) && (1..=31).contains(&day)
}

fn parse_date_cell(cell: &str, date1904: bool) -> Option<String> {
    let value = cell.trim();
    if value.is_empty() {
        return None;
    }
    if value.contains('-') {
        return valid_day(value).then(|| value.to_string());
    }
    let serial = value.parse::<f64>().ok()?;
    if !serial.is_finite() || serial < 1.0 {
        return None;
    }
    let days = serial.floor() as i64 - if date1904 { 24_107 } else { 25_569 };
    let (year, month, day) = civil_from_days(days)?;
    if !(1900..=2100).contains(&year) {
        return None;
    }
    Some(format!("{year:04}-{month:02}-{day:02}"))
}

/// Howard Hinnant's civil-from-days, Unix epoch based.
fn civil_from_days(days: i64) -> Option<(i32, u32, u32)> {
    let z = days.checked_add(719_468)?;
    let era = if z >= 0 { z } else { z - 146_096 } / 146_097;
    let doe = (z - era * 146_097) as u64;
    let yoe = (doe - doe / 1_460 + doe / 36_524 - doe / 146_096) / 365;
    let year = yoe as i64 + era * 400;
    let doy = doe - (365 * yoe + yoe / 4 - yoe / 100);
    let mp = (5 * doy + 2) / 153;
    let day = doy - (153 * mp + 2) / 5 + 1;
    let month = if mp < 10 { mp + 3 } else { mp - 9 };
    let year = if month <= 2 { year + 1 } else { year };
    Some((year as i32, month as u32, day as u32))
}

#[derive(Debug, Serialize)]
struct FileDigest {
    bytes: u64,
    sha256: String,
}

#[derive(Debug, Serialize)]
struct OutputManifest {
    format: String,
    parser_version: String,
    source: String,
    source_sha256: String,
    source_revision: String,
    period_start: String,
    period_end: String,
    counts: Counts,
    quarantine: usize,
    files: std::collections::BTreeMap<String, FileDigest>,
}

/// Writes facts, duplicates, quarantine and a manifest atomically.
pub fn write_run(dir: &Path, run: &AnpPriceRun) -> Result<(), AnpPriceError> {
    let parent = dir.parent().unwrap_or_else(|| Path::new("."));
    fs::create_dir_all(parent).map_err(|error| AnpPriceError::Truncated(error.to_string()))?;
    let name = dir
        .file_name()
        .and_then(|value| value.to_str())
        .unwrap_or("anp-prices");
    let temp = parent.join(format!(".{name}.tmp-{}", std::process::id()));
    let backup = parent.join(format!(".{name}.old-{}", std::process::id()));
    let _ = fs::remove_dir_all(&temp);
    let _ = fs::remove_dir_all(&backup);
    fs::create_dir(&temp).map_err(|error| AnpPriceError::Truncated(error.to_string()))?;
    let mut files = std::collections::BTreeMap::new();
    for (file_name, bytes) in [
        ("prices.ndjson", ndjson(&run.facts)?),
        ("duplicates.ndjson", ndjson(&run.duplicates)?),
        ("quarantine.ndjson", ndjson(&run.quarantine)?),
    ] {
        write_file(&temp.join(file_name), &bytes)?;
        files.insert(
            file_name.to_string(),
            FileDigest {
                bytes: bytes.len() as u64,
                sha256: sha256_hex(&bytes),
            },
        );
    }
    let manifest = OutputManifest {
        format: PRICE_FORMAT.into(),
        parser_version: PARSER_VERSION.into(),
        source: SOURCE.into(),
        source_sha256: run.source_sha256.clone(),
        source_revision: run.source_revision.clone(),
        period_start: run.period_start.clone(),
        period_end: run.period_end.clone(),
        counts: run.counts,
        quarantine: run.quarantine.len(),
        files,
    };
    let manifest = serde_json::to_vec_pretty(&manifest)
        .map_err(|error| AnpPriceError::Truncated(error.to_string()))?;
    write_file(&temp.join("manifest.json"), &manifest)?;
    if dir.exists() {
        fs::rename(dir, &backup).map_err(|error| AnpPriceError::Truncated(error.to_string()))?;
        if let Err(error) = fs::rename(&temp, dir) {
            let _ = fs::rename(&backup, dir);
            return Err(AnpPriceError::Truncated(error.to_string()));
        }
        let _ = fs::remove_dir_all(&backup);
    } else if let Err(error) = fs::rename(&temp, dir) {
        let _ = fs::remove_dir_all(&temp);
        return Err(AnpPriceError::Truncated(error.to_string()));
    }
    Ok(())
}

fn ndjson<T: Serialize>(values: &[T]) -> Result<Vec<u8>, AnpPriceError> {
    let mut out = Vec::new();
    for value in values {
        serde_json::to_writer(&mut out, value)
            .map_err(|error| AnpPriceError::Truncated(error.to_string()))?;
        out.push(b'\n');
    }
    Ok(out)
}

fn write_file(path: &Path, bytes: &[u8]) -> Result<(), AnpPriceError> {
    let file = File::create(path).map_err(|error| AnpPriceError::Truncated(error.to_string()))?;
    let mut writer = BufWriter::new(file);
    writer
        .write_all(bytes)
        .map_err(|error| AnpPriceError::Truncated(error.to_string()))?;
    writer
        .flush()
        .map_err(|error| AnpPriceError::Truncated(error.to_string()))?;
    writer
        .get_ref()
        .sync_all()
        .map_err(|error| AnpPriceError::Truncated(error.to_string()))?;
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;

    fn fixture() -> Vec<u8> {
        std::fs::read(
            std::path::Path::new(env!("CARGO_MANIFEST_DIR"))
                .join("../../contracts/testdata/map/anp-station-prices-v1.xlsx"),
        )
        .expect("fixture")
    }

    #[test]
    fn parses_exact_station_facts_and_reconciles_census() {
        let run = parse_station_prices_xlsx(
            "anp.xlsx",
            &fixture(),
            &Limits::default(),
            "2026-10-18",
            "2026-10-24",
            "survey-1",
            &|| false,
        )
        .expect("parse");
        assert_eq!(run.counts.input, 15);
        assert_eq!(run.counts.accepted, 7);
        assert_eq!(run.counts.duplicates, 1);
        assert_eq!(run.counts.quarantined, 7);
        let gasoline = run
            .facts
            .iter()
            .find(|fact| fact.product == "GASOLINE_REGULAR")
            .unwrap();
        assert_eq!(gasoline.amount_milli_brl, 5999);
        assert_eq!(gasoline.unit, "L");
        assert_eq!(gasoline.cnpj, "01122233300000");
        let cng = run.facts.iter().find(|fact| fact.product == "CNG").unwrap();
        assert_eq!(cng.amount_milli_brl, 4499);
        assert_eq!(cng.unit, "M3");
        let lpg = run
            .facts
            .iter()
            .find(|fact| fact.product == "LPG_P13" && fact.cnpj == "00112223330019")
            .unwrap();
        assert_eq!(lpg.amount_milli_brl, 109990);
        assert_eq!(lpg.unit, "KG_13");
    }

    #[test]
    fn price_and_date_parsing_are_exact() {
        assert_eq!(parse_milli_brl("5,999"), Some(5999));
        assert_eq!(parse_milli_brl("5.999"), Some(5999));
        assert_eq!(parse_milli_brl("999,999"), Some(999_999));
        assert_eq!(parse_milli_brl("6.1234"), None);
        assert_eq!(parse_milli_brl("0,000"), None);
        assert_eq!(parse_milli_brl("-1,000"), None);
        assert_eq!(
            parse_date_cell("46315", false).as_deref(),
            Some("2026-10-20")
        );
        assert_eq!(
            parse_date_cell("2026-10-20", false).as_deref(),
            Some("2026-10-20")
        );
        assert_eq!(parse_date_cell("", false), None);
    }

    #[test]
    fn truncated_input_and_invalid_period_are_refused() {
        let wrong = fixture();
        assert!(parse_station_prices_xlsx(
            "x",
            &wrong[..100],
            &Limits::default(),
            "2026-10-18",
            "2026-10-24",
            "s",
            &|| false
        )
        .is_err());
        assert!(parse_station_prices_xlsx(
            "x",
            &wrong,
            &Limits::default(),
            "2026-10-24",
            "2026-10-18",
            "s",
            &|| false
        )
        .is_err());
        assert!(matches!(
            parse_station_prices_xlsx(
                "x",
                &fixture(),
                &Limits::default(),
                "2026-10-18",
                "2026-10-24",
                "s",
                &|| true
            ),
            Err(AnpPriceError::Cancelled)
        ));
    }

    #[test]
    fn atomic_output_keeps_only_the_last_generation() {
        let dir = std::env::temp_dir().join(format!("abastevo-anp-prices-{}", std::process::id()));
        let _ = fs::remove_dir_all(&dir);
        fs::create_dir_all(&dir).unwrap();
        let mut run = parse_station_prices_xlsx(
            "anp.xlsx",
            &fixture(),
            &Limits::default(),
            "2026-10-18",
            "2026-10-24",
            "survey-1",
            &|| false,
        )
        .expect("parse");
        let out = dir.join("prices");
        write_run(&out, &run).expect("write");
        let before = fs::read(out.join("prices.ndjson")).unwrap();
        run.facts.clear();
        write_run(&out, &run).expect("rewrite");
        assert_ne!(before, fs::read(out.join("prices.ndjson")).unwrap());
        assert!(out.join("manifest.json").exists());
    }
}

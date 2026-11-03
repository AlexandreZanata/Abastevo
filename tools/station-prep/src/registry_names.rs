//! Pinned, minimized offline name enrichment. This module never fetches sources.
use crate::cnpj::normalize_cnpj;
use crate::registry::{normalize_calendar_date, RegistryBatch};
use crate::types::sha256_hex;
use serde::{Deserialize, Serialize};
use std::collections::{BTreeMap, HashMap, HashSet};
use std::io::Read;
use std::path::{Component, Path};

pub const NAMED_PARSER_VERSION: &str = "station-prep-v0.3.1+names-v1";
const MAX_BYTES: u64 = 64 << 20;

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct NameInputPin {
    pub source: String,
    pub profile: String,
    pub reference: String,
    pub edition: String,
    pub sha256: String,
    pub bytes: u64,
    pub rows: usize,
}

#[derive(Debug, Clone, Serialize)]
pub struct NameProvenance {
    pub source: String,
    pub profile: String,
    pub reference: String,
    pub edition: String,
    pub sha256: String,
    pub observed_at: String,
}

#[derive(Deserialize)]
#[serde(deny_unknown_fields)]
struct Descriptor {
    format_version: String,
    inputs: Vec<serde_json::Value>,
}

#[derive(Deserialize)]
#[serde(deny_unknown_fields)]
struct RfbRow {
    schema_version: String,
    cnpj: String,
    trade_name: String,
    active: bool,
    observed_at: String,
}

#[derive(Deserialize)]
struct SurveyRow {
    schema_version: String,
    cnpj: String,
    trade_name: Option<String>,
    observed_at: String,
    #[serde(flatten)]
    other: BTreeMap<String, serde_json::Value>,
}

#[derive(Debug, Clone, PartialEq, Eq)]
struct NameRow {
    name: String,
    active: bool,
}

/// Metadata and bounded lookup tables; raw source files never enter output.
pub struct NameInputs {
    pins: Vec<NameInputPin>,
    values: HashMap<String, HashMap<String, BTreeMap<String, NameRow>>>,
}

fn bounded_file(path: &Path, cap: u64) -> Result<Vec<u8>, String> {
    let file = std::fs::File::open(path).map_err(|_| "name input unavailable")?;
    if !file
        .metadata()
        .map_err(|_| "name input metadata")?
        .is_file()
    {
        return Err("name input must be a regular file".into());
    }
    let mut raw = Vec::new();
    file.take(cap + 1)
        .read_to_end(&mut raw)
        .map_err(|_| "name input read")?;
    if raw.len() as u64 > cap {
        return Err("name input byte cap exceeded".into());
    }
    Ok(raw)
}

/// Verify local bytes, rows, profiles and conflicting observations before emission.
pub fn load_name_inputs(path: &Path) -> Result<NameInputs, String> {
    let raw = bounded_file(path, 128 << 10)?;
    let descriptor: Descriptor =
        serde_json::from_slice(&raw).map_err(|_| "invalid name descriptor")?;
    if descriptor.format_version != "registry-name-inputs-v1" || descriptor.inputs.len() > 2 {
        return Err("unsupported name descriptor".into());
    }
    let parent = path.parent().unwrap_or_else(|| Path::new("."));
    let parent = std::fs::canonicalize(if parent.as_os_str().is_empty() {
        Path::new(".")
    } else {
        parent
    })
    .map_err(|_| "name descriptor directory")?;
    let mut result = NameInputs {
        pins: Vec::new(),
        values: HashMap::new(),
    };
    let mut sources = HashSet::new();
    for mut value in descriptor.inputs {
        let file = value
            .as_object_mut()
            .and_then(|v| v.remove("file"))
            .and_then(|v| v.as_str().map(String::from))
            .ok_or("name input file missing")?;
        let components: Vec<_> = Path::new(&file).components().collect();
        if components.len() != 1
            || !matches!(components[0], Component::Normal(_))
            || file.contains(['/', '\\'])
        {
            return Err("name input file must be a basename".into());
        }
        let pin: NameInputPin =
            serde_json::from_value(value).map_err(|_| "invalid name input pin")?;
        let profile = match pin.source.as_str() {
            "rfb" => "rfb-station-names-v1",
            "anp-survey" => "anp-station-price-v1",
            _ => return Err("unadmitted name source profile".into()),
        };
        if pin.profile != profile
            || !sources.insert(pin.source.clone())
            || pin.reference.trim().is_empty()
            || pin.reference.len() > 2048
            || pin.edition.trim().is_empty()
            || pin.edition.len() > 256
            || pin.bytes > MAX_BYTES
            || pin.rows > 500_000
            || pin.sha256.len() != 64
            || !pin.sha256.bytes().all(|b| b.is_ascii_hexdigit())
        {
            return Err("invalid name input bounds or profile".into());
        }
        let input_path =
            std::fs::canonicalize(parent.join(&file)).map_err(|_| "name input unavailable")?;
        if input_path.parent() != Some(parent.as_path()) {
            return Err("name input escapes descriptor directory".into());
        }
        let raw = bounded_file(&input_path, MAX_BYTES)?;
        if pin.bytes != raw.len() as u64 || pin.sha256 != sha256_hex(&raw) {
            return Err("name input fingerprint mismatch".into());
        }
        let text = std::str::from_utf8(&raw).map_err(|_| "name input encoding")?;
        if !text.is_empty() && !text.ends_with('\n') {
            return Err("truncated name JSONL".into());
        }
        let lines: Vec<_> = text.lines().collect();
        if lines.len() != pin.rows {
            return Err("name input row count mismatch".into());
        }
        let by_cnpj = result.values.entry(pin.source.clone()).or_default();
        for line in lines {
            if line.len() > 256 << 10 {
                return Err("name input line cap exceeded".into());
            }
            let (cnpj, name, active, date) = if pin.source == "rfb" {
                let row: RfbRow =
                    serde_json::from_str(line).map_err(|_| "invalid minimized RFB name row")?;
                if row.schema_version != profile {
                    return Err("RFB name row schema mismatch".into());
                }
                (row.cnpj, row.trade_name, row.active, row.observed_at)
            } else {
                let row: SurveyRow =
                    serde_json::from_str(line).map_err(|_| "invalid minimized ANP survey row")?;
                let allowed = [
                    "source",
                    "source_key",
                    "station_name",
                    "municipality",
                    "state",
                    "brand",
                    "product",
                    "unit",
                    "amount_milli_brl",
                    "raw_price_text",
                    "period_start",
                    "period_end",
                    "source_revision",
                    "source_row",
                ];
                if row.schema_version != profile
                    || row.other.keys().any(|k| !allowed.contains(&k.as_str()))
                {
                    return Err("ANP name row schema mismatch".into());
                }
                (
                    row.cnpj,
                    row.trade_name.unwrap_or_default(),
                    true,
                    row.observed_at,
                )
            };
            let cnpj = normalize_cnpj(&cnpj).map_err(|_| "invalid name input CNPJ")?;
            if date.is_empty()
                || normalize_calendar_date(&date).map_err(|_| "invalid name observation date")?
                    != date
            {
                return Err("name observation requires ISO date".into());
            }
            let record = NameRow { name, active };
            let observations = by_cnpj.entry(cnpj).or_default();
            if observations.get(&date).is_some_and(|old| old != &record) {
                return Err("conflicting name observations at the same clock".into());
            }
            observations.insert(date, record);
        }
        result.pins.push(pin);
    }
    result.pins.sort_by(|a, b| a.source.cmp(&b.source));
    Ok(result)
}

fn clean(raw: &str) -> String {
    raw.split_whitespace().collect::<Vec<_>>().join(" ")
}

fn usable(trade: &str, legal: &str, cnpj: &str) -> bool {
    let trade = clean(trade);
    let identity: String = trade
        .chars()
        .filter(|c| c.is_alphanumeric())
        .flat_map(char::to_uppercase)
        .collect();
    if identity.is_empty()
        || trade.to_lowercase() == clean(legal).to_lowercase()
        || identity == cnpj
    {
        return false;
    }
    let placeholder: String = trade
        .chars()
        .filter(|c| !('\u{0300}'..='\u{036f}').contains(c))
        .flat_map(char::to_uppercase)
        .map(|c| match c {
            'À' | 'Á' | 'Â' | 'Ã' | 'Ä' | 'Å' => 'A',
            'È' | 'É' | 'Ê' | 'Ë' => 'E',
            'Ì' | 'Í' | 'Î' | 'Ï' => 'I',
            'Ò' | 'Ó' | 'Ô' | 'Õ' | 'Ö' => 'O',
            'Ù' | 'Ú' | 'Û' | 'Ü' => 'U',
            other => other,
        })
        .collect();
    !matches!(
        placeholder.as_str(),
        "NAO POSSUI" | "SEM NOME" | "NAO INFORMADO"
    )
}

/// Linear keyed join, followed by stable checksum/link rebuilding. No location,
/// authorization, station membership or row accounting changes are made here.
pub fn enrich_registry_names(batch: &mut RegistryBatch, inputs: &NameInputs) -> Result<(), String> {
    if !batch.name_profile
        || batch
            .accepted
            .iter()
            .chain(&batch.duplicates)
            .any(|r| r.legal_name.is_none())
    {
        return Err("name enrichment requires v3 parsing".into());
    }
    let mut remap = HashMap::new();
    for row in batch.accepted.iter_mut().chain(&mut batch.duplicates) {
        let legal = row.legal_name.as_deref().expect("checked v3 legal name");
        let selected = ["rfb", "anp-survey"].into_iter().find_map(|source| {
            let observations = inputs.values.get(source)?.get(&row.source_key)?;
            let chosen = if source == "rfb" {
                observations
                    .last_key_value()
                    .filter(|(_, r)| r.active && usable(&r.name, legal, &row.source_key))
            } else {
                observations
                    .iter()
                    .rev()
                    .find(|(_, r)| usable(&r.name, legal, &row.source_key))
            }?;
            let pin = inputs.pins.iter().find(|p| p.source == source)?;
            Some((
                chosen.1.name.clone(),
                NameProvenance {
                    source: pin.source.clone(),
                    profile: pin.profile.clone(),
                    reference: pin.reference.clone(),
                    edition: pin.edition.clone(),
                    sha256: pin.sha256.clone(),
                    observed_at: chosen.0.clone(),
                },
            ))
        });
        row.trade_name = Some(
            selected
                .as_ref()
                .map_or_else(String::new, |(name, _)| name.clone()),
        );
        row.trade_name_provenance = selected.map(|(_, provenance)| provenance);
        let old = row.checksum.clone();
        let mut canonical = row.clone();
        canonical.checksum.clear();
        canonical.supersedes = None;
        row.checksum = sha256_hex(canonical.to_jsonl().as_bytes());
        remap.insert(old, row.checksum.clone());
    }
    for row in batch.accepted.iter_mut().chain(&mut batch.duplicates) {
        if let Some(previous) = &row.supersedes {
            row.supersedes = Some(
                remap
                    .get(previous)
                    .ok_or("missing v3 succession target")?
                    .clone(),
            );
        }
    }
    batch
        .accepted
        .sort_by(|a, b| (&a.source_key, &a.checksum).cmp(&(&b.source_key, &b.checksum)));
    batch.name_inputs = inputs.pins.clone();
    Ok(())
}

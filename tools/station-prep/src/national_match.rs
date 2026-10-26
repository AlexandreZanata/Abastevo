//! National ANP registry-API x OSM fuel-station matching.
//!
//! Local files only: no network, database, native engine or clock. The ANP
//! Revendedores API pages and the OSM reference prepared by `prepare_osm` are
//! joined deterministically. A station is never located by proximity alone and
//! an ambiguous OSM tie is never resolved by picking one candidate.
//!
//! Quality ladder used here:
//! - L1: ANP API point plus an OSM fuel POI with the same CNPJ tag, or the
//!   MAP-20 exact normalized street+number, unique, within the configured
//!   distance. L1 rows are emitted for `station-review`.
//! - L2: OSM fuel POI with the station CNPJ tag, or a unique OSM fuel POI with
//!   equal normalized street+number, equal municipality and compatible
//!   name/brand. L2 rows are emitted as `osm-mapped` map assertions.
//! - L3: a valid ANP API point without OSM confirmation. It is recorded as a
//!   decision only while owner decision D1 is open.
//! - L5: no proven point. No pin is emitted.

use crate::cnpj::normalize_cnpj;
use crate::location_review::{
    brand_match_text, haversine_m, normalize_number, normalize_street, split_address, AnpEvidence,
    Counts as ReviewCounts, OsmEvidence, RejectRow, ReviewRow,
};
use crate::map_contract::{self, ASSERTION_FORMAT, VERSION as MAP_VERSION};
use crate::municipality::normalize_name;
use crate::types::sha256_hex;
use serde::{Deserialize, Serialize};
use std::collections::{BTreeMap, BTreeSet, HashMap, HashSet};
use std::fs::{self, File};
use std::io::{BufWriter, Write};
use std::path::{Path, PathBuf};
use std::time::Instant;

pub const SUMMARY_FORMAT: &str = "abastevo.national-match-summary-v1";
pub const DECISION_FORMAT: &str = "abastevo.national-match-decision-v1";
pub const REVIEW_FORMAT: &str = "abastevo.station-location-review-v1";
pub const REVIEW_PARSER_VERSION: &str = "station-location-review-v1";
pub const REVIEW_SCHEMA_VERSION: &str = "station-location-review-v1";
pub const DEFAULT_MAX_DISTANCE_M: f64 = 50.0;
pub const DEFAULT_MAX_CNPJ_DISTANCE_M: f64 = 200.0;
pub const MAX_REVIEW_DISTANCE_M: f64 = 200.0;
pub const GRID_CELL_DEG: f64 = 0.0025;
pub const MAX_API_RECORDS: usize = 200_000;
pub const MAX_OSM_REFERENCES: usize = 500_000;
pub const MAX_REGISTRY_ROWS: usize = 200_000;
pub const MAX_MAP_ASSERTIONS: usize = 10_000;
pub const MAX_REVIEW_ROWS: usize = 10_000;
pub const MAX_REVIEW_BYTES: usize = 4 << 20;
pub const MAX_API_FILE_BYTES: u64 = 64 << 20;
pub const MAX_OSM_FILE_BYTES: u64 = 1 << 30;
pub const MAX_REGISTRY_FILE_BYTES: u64 = 1 << 30;

const OSM_ATTRIBUTION: &str = "© OpenStreetMap contributors — ODbL 1.0";
const UUID_SEED: &str = "abastevo:map-osm-fuel:v1";

#[derive(Debug)]
pub enum MatchError {
    Io(String),
    Json(String),
    Profile(String),
    Limit(String),
}

impl std::fmt::Display for MatchError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            MatchError::Io(message) => write!(f, "{message}"),
            MatchError::Json(message) => write!(f, "{message}"),
            MatchError::Profile(message) => write!(f, "{message}"),
            MatchError::Limit(message) => write!(f, "{message}"),
        }
    }
}

impl std::error::Error for MatchError {}

fn io_error(error: std::io::Error) -> MatchError {
    MatchError::Io(error.to_string())
}

fn json_error(error: serde_json::Error) -> MatchError {
    MatchError::Json(error.to_string())
}

fn profile<T>(message: impl Into<String>) -> Result<T, MatchError> {
    Err(MatchError::Profile(message.into()))
}

fn limit<T>(message: impl Into<String>) -> Result<T, MatchError> {
    Err(MatchError::Limit(message.into()))
}

#[derive(Debug, Clone, Deserialize)]
pub struct ApiEnvelope {
    #[serde(default)]
    pub data: Vec<ApiRecord>,
}

#[derive(Debug, Clone, Deserialize)]
pub struct ApiRecord {
    #[serde(default)]
    pub cnpj: String,
    #[serde(default, rename = "razaoSocial")]
    pub business: String,
    #[serde(default)]
    pub endereco: String,
    #[serde(default)]
    pub complemento: String,
    #[serde(default)]
    pub bairro: String,
    #[serde(default)]
    pub cep: String,
    #[serde(default)]
    pub municipio: String,
    #[serde(default)]
    pub uf: String,
    #[serde(default)]
    pub latitude: Option<String>,
    #[serde(default)]
    pub longitude: Option<String>,
    #[serde(default)]
    pub srid: String,
    #[serde(default)]
    pub src: String,
    #[serde(default)]
    pub validacao: String,
    #[serde(default, rename = "estimativaAcuracia")]
    pub accuracy: String,
    #[serde(default, rename = "dataObtencao")]
    pub obtained: String,
    #[serde(default, rename = "origemInformacao")]
    pub origin: String,
    #[serde(default)]
    pub distribuidora: Option<String>,
}

#[derive(Debug, Clone, Default, Deserialize)]
pub struct RegistryAddress {
    #[serde(default)]
    pub street: String,
    #[serde(default)]
    pub number: String,
}

#[derive(Debug, Clone, Deserialize)]
pub struct RegistryRecord {
    #[serde(default)]
    pub source_key: String,
    #[serde(default)]
    pub business_name_raw: String,
    #[serde(default)]
    pub address_raw: String,
    #[serde(default)]
    pub address_normalized: RegistryAddress,
    #[serde(default)]
    pub uf: String,
    #[serde(default)]
    pub municipality_name_raw: String,
    #[serde(default)]
    pub municipality_ibge: String,
    #[serde(default)]
    pub brand_raw: String,
}

#[derive(Debug, Clone, Deserialize)]
pub struct OsmPlace {
    #[serde(default)]
    pub key: String,
    #[serde(default)]
    pub osm_type: String,
    #[serde(default)]
    pub osm_id: i64,
    #[serde(default)]
    pub kind: String,
    #[serde(default)]
    pub name: Option<String>,
    #[serde(default)]
    pub latitude_deg: Option<String>,
    #[serde(default)]
    pub longitude_deg: Option<String>,
    #[serde(default)]
    pub geometry: String,
    #[serde(default)]
    pub display_point: Option<String>,
    #[serde(default)]
    pub cnpj: Option<String>,
    #[serde(default)]
    pub brand: Option<String>,
    #[serde(default)]
    pub operator: Option<String>,
    #[serde(default)]
    pub tags: BTreeMap<String, String>,
    #[serde(default)]
    pub license: String,
    #[serde(default)]
    pub attribution: String,
}

#[derive(Debug, Clone, Serialize)]
pub struct InputFile {
    pub path: String,
    pub bytes: u64,
    pub sha256: String,
}

#[derive(Debug, Clone)]
pub struct InputSet {
    pub api_records: Vec<ApiRecord>,
    pub api_files: Vec<InputFile>,
    pub api_sha256: String,
    pub osm: Vec<OsmPlace>,
    pub osm_sha256: String,
    pub osm_bytes: u64,
    pub registry: HashMap<String, RegistryRecord>,
    pub registry_sha256: String,
    pub registry_bytes: u64,
    pub registry_rows: usize,
    pub registry_duplicates: usize,
    pub invalid_registry_cnpj: usize,
}

#[derive(Debug, Clone, Serialize)]
pub struct MapIdentities {
    pub station_uuid: Option<String>,
    pub cnpj: Option<String>,
    pub place_id: Option<String>,
    pub equipment_id: Option<String>,
    pub connector_id: Option<String>,
    pub osm_type: Option<String>,
    pub osm_id: Option<i64>,
}

#[derive(Debug, Clone, Serialize)]
pub struct MapLocation {
    pub latitude_deg: Option<String>,
    pub longitude_deg: Option<String>,
    pub quality: String,
    pub display_point: Option<String>,
    pub entrance_latitude_deg: Option<String>,
    pub entrance_longitude_deg: Option<String>,
}

#[derive(Debug, Clone, Serialize)]
pub struct MapClocks {
    pub observed_at: String,
    pub published_at: String,
    pub fetched_at: String,
}

#[derive(Debug, Clone, Serialize)]
pub struct MapRights {
    pub source: String,
    pub license: String,
    pub attribution: String,
    pub source_sha256: String,
}

#[derive(Debug, Clone, Serialize)]
pub struct MapCapabilities {
    pub price_known: bool,
    pub pin_eligible: bool,
    pub entrance_known: bool,
    pub live_status: bool,
}

#[derive(Debug, Clone, Serialize)]
pub struct MapAssertion {
    pub id: String,
    pub source: String,
    pub kind: String,
    pub name: String,
    pub identities: MapIdentities,
    pub location: MapLocation,
    pub services: Vec<String>,
    pub prices: Vec<serde_json::Value>,
    pub charging: Option<serde_json::Value>,
    pub clocks: MapClocks,
    pub rights: MapRights,
    pub capabilities: MapCapabilities,
}

#[derive(Debug, Clone, Serialize)]
pub struct DecisionRow {
    pub schema_version: &'static str,
    pub cnpj: String,
    pub uf: String,
    pub municipality: String,
    pub tier: &'static str,
    pub reason: &'static str,
    pub publish: bool,
    pub candidate_count: u32,
    pub osm_type: Option<String>,
    pub osm_id: Option<i64>,
    pub distance_m: Option<f64>,
    pub api_point: bool,
    pub api_srid: String,
    pub api_crs: String,
    pub registry_address: String,
    pub osm_address: String,
}

#[derive(Debug, Clone, Default, Serialize)]
pub struct TierCounts {
    pub l1_cnpj: u64,
    pub l1_address: u64,
    pub l2_cnpj: u64,
    pub l2_address: u64,
    pub l3_anp: u64,
    pub l5: u64,
    pub api_only: u64,
}

impl TierCounts {
    fn add(&mut self, tier: &str) {
        match tier {
            "L1_CNPJ" => self.l1_cnpj += 1,
            "L1_ADDRESS" => self.l1_address += 1,
            "L2_CNPJ" => self.l2_cnpj += 1,
            "L2_ADDRESS" => self.l2_address += 1,
            "L3_ANP" => self.l3_anp += 1,
            "L5" => self.l5 += 1,
            "API_ONLY" => self.api_only += 1,
            _ => {}
        }
    }
}

#[derive(Debug, Clone, Default, Serialize)]
pub struct MatchCounts {
    pub registry_rows: u64,
    pub api_records: u64,
    pub api_duplicates: u64,
    pub api_invalid_cnpj: u64,
    pub api_only: u64,
    pub map_assertions: u64,
    pub tiers: TierCounts,
    pub reasons: BTreeMap<String, u64>,
}

#[derive(Debug, Clone, Default, Serialize)]
pub struct UfCounts {
    pub registry_rows: u64,
    pub tiers: TierCounts,
    pub reasons: BTreeMap<String, u64>,
}

#[derive(Debug, Clone, Default)]
pub struct MatchResult {
    pub reviews: Vec<ReviewRow>,
    pub rejects: Vec<RejectRow>,
    pub decisions: Vec<DecisionRow>,
    pub map_assertions: Vec<MapAssertion>,
    pub counts: MatchCounts,
    pub per_uf: BTreeMap<String, UfCounts>,
    pub review_counts: ReviewCounts,
}

#[derive(Debug, Clone, Default, Serialize)]
pub struct Timings {
    pub load_ms: f64,
    pub index_ms: f64,
    pub match_ms: f64,
    pub write_ms: f64,
    pub total_ms: f64,
}

#[derive(Debug, Clone)]
pub struct RunOptions {
    pub max_distance_m: f64,
    pub max_cnpj_distance_m: f64,
    pub max_map_assertions: usize,
    pub generation: String,
    pub reviewer: String,
    pub clocks: MapClocks,
}

#[derive(Debug, Clone)]
pub struct RunOutput {
    pub inputs: InputSet,
    pub result: MatchResult,
    pub timings: Timings,
    pub generation: String,
}

#[derive(Debug, Clone, Copy, PartialEq)]
enum PointStatus {
    Valid(f64, f64),
    MissingCoordinates,
    SridInvalid,
    OutOfBrazil,
}

impl PointStatus {
    fn reason(self) -> &'static str {
        match self {
            PointStatus::Valid(_, _) => "anp_point",
            PointStatus::MissingCoordinates => "missing_coordinates",
            PointStatus::SridInvalid => "srid_invalid",
            PointStatus::OutOfBrazil => "out_of_brazil",
        }
    }
}

struct OsmIndex {
    grid: HashMap<(i32, i32), Vec<usize>>,
    cnpj: HashMap<String, Vec<usize>>,
    address: HashMap<(String, String), Vec<usize>>,
    points: Vec<Option<(f64, f64)>>,
}

#[derive(Debug, Clone, Serialize)]
struct ManifestFile {
    bytes: u64,
    sha256: String,
}

#[derive(Debug, Clone, Serialize)]
struct ReviewManifest {
    format: &'static str,
    parser_version: &'static str,
    reviewer: String,
    max_distance_m: f64,
    source_sha256: String,
    osm_sha256: String,
    registry_sha256: String,
    counts: ReviewCounts,
    files: BTreeMap<&'static str, ManifestFile>,
}

#[derive(Debug, Clone, Serialize)]
struct SummaryInputs {
    api_sha256: String,
    api_records: u64,
    api_files: Vec<InputFile>,
    osm_sha256: String,
    osm_bytes: u64,
    osm_references: u64,
    registry_sha256: String,
    registry_bytes: u64,
    registry_rows: u64,
    registry_duplicates: u64,
    invalid_registry_cnpj: u64,
}

#[derive(Debug, Clone, Serialize)]
struct SummaryLimits {
    max_distance_m: f64,
    max_cnpj_distance_m: f64,
    grid_cell_deg: f64,
    max_api_records: usize,
    max_osm_references: usize,
    max_registry_rows: usize,
    max_map_assertions: usize,
    max_review_rows: usize,
    max_review_bytes: usize,
}

#[derive(Debug, Clone, Serialize)]
struct SummaryOutputs {
    reviews_ndjson: ManifestFile,
    rejects_ndjson: ManifestFile,
    review_manifest: ManifestFile,
    decisions_ndjson: ManifestFile,
    map_assertions_ndjson: ManifestFile,
    map_assertions_json: Option<ManifestFile>,
}

#[derive(Debug, Clone, Serialize)]
struct MatchSummary {
    format: &'static str,
    generation: String,
    inputs: SummaryInputs,
    limits: SummaryLimits,
    counts: MatchCounts,
    per_uf: BTreeMap<String, UfCounts>,
    outputs: SummaryOutputs,
    map_assertions_contract_valid: bool,
    timings_file: &'static str,
}

fn read_bounded(path: &Path, max: u64) -> Result<Vec<u8>, MatchError> {
    let metadata = fs::metadata(path).map_err(io_error)?;
    if metadata.len() > max {
        return limit(format!("{} exceeds {} bytes", path.display(), max));
    }
    fs::read(path).map_err(io_error)
}

fn json_files(dir: &Path) -> Result<Vec<PathBuf>, MatchError> {
    let mut files = Vec::new();
    let mut stack = vec![dir.to_path_buf()];
    while let Some(current) = stack.pop() {
        for entry in fs::read_dir(&current).map_err(io_error)? {
            let entry = entry.map_err(io_error)?;
            let path = entry.path();
            if path.is_dir() {
                stack.push(path);
            } else if path.extension().and_then(|value| value.to_str()) == Some("json") {
                files.push(path);
            }
        }
    }
    files.sort();
    Ok(files)
}

fn relative_name(root: &Path, path: &Path) -> String {
    path.strip_prefix(root)
        .unwrap_or(path)
        .to_string_lossy()
        .replace('\\', "/")
}

pub fn load_inputs(
    api_dir: &Path,
    osm_path: &Path,
    registry_path: &Path,
) -> Result<InputSet, MatchError> {
    if !api_dir.is_dir() {
        return profile(format!("API directory missing: {}", api_dir.display()));
    }
    let mut api_records = Vec::new();
    let mut api_files = Vec::new();
    let mut api_seed = Vec::new();
    for path in json_files(api_dir)? {
        let raw = read_bounded(&path, MAX_API_FILE_BYTES)?;
        let envelope: ApiEnvelope = serde_json::from_slice(&raw).map_err(json_error)?;
        if api_records.len() + envelope.data.len() > MAX_API_RECORDS {
            return limit(format!("API records exceed {}", MAX_API_RECORDS));
        }
        let digest = sha256_hex(&raw);
        let name = relative_name(api_dir, &path);
        api_seed.extend_from_slice(name.as_bytes());
        api_seed.push(0);
        api_seed.extend_from_slice(digest.as_bytes());
        api_seed.push(b'\n');
        api_files.push(InputFile {
            path: name,
            bytes: raw.len() as u64,
            sha256: digest,
        });
        api_records.extend(envelope.data);
    }
    if api_files.is_empty() {
        return profile("API directory has no JSON files");
    }

    let osm_raw = read_bounded(osm_path, MAX_OSM_FILE_BYTES)?;
    let osm_sha256 = sha256_hex(&osm_raw);
    let mut osm = Vec::new();
    for line in osm_raw.split(|byte| *byte == b'\n') {
        if line.iter().all(|byte| byte.is_ascii_whitespace()) {
            continue;
        }
        if osm.len() >= MAX_OSM_REFERENCES {
            return limit(format!("OSM references exceed {}", MAX_OSM_REFERENCES));
        }
        osm.push(serde_json::from_slice::<OsmPlace>(line).map_err(json_error)?);
    }

    let registry_raw = read_bounded(registry_path, MAX_REGISTRY_FILE_BYTES)?;
    let registry_sha256 = sha256_hex(&registry_raw);
    let mut registry = HashMap::new();
    let mut registry_rows = 0usize;
    let mut registry_duplicates = 0usize;
    let mut invalid_registry_cnpj = 0usize;
    for line in registry_raw.split(|byte| *byte == b'\n') {
        if line.iter().all(|byte| byte.is_ascii_whitespace()) {
            continue;
        }
        if registry_rows >= MAX_REGISTRY_ROWS {
            return limit(format!("registry rows exceed {}", MAX_REGISTRY_ROWS));
        }
        registry_rows += 1;
        let record = serde_json::from_slice::<RegistryRecord>(line).map_err(json_error)?;
        let Ok(cnpj) = normalize_cnpj(&record.source_key) else {
            invalid_registry_cnpj += 1;
            continue;
        };
        if registry.insert(cnpj, record).is_some() {
            registry_duplicates += 1;
        }
    }

    Ok(InputSet {
        api_records,
        api_files,
        api_sha256: sha256_hex(&api_seed),
        osm,
        osm_sha256,
        osm_bytes: osm_raw.len() as u64,
        registry,
        registry_sha256,
        registry_bytes: registry_raw.len() as u64,
        registry_rows,
        registry_duplicates,
        invalid_registry_cnpj,
    })
}

fn osm_point(place: &OsmPlace) -> Option<(f64, f64)> {
    let lat = place
        .latitude_deg
        .as_deref()
        .map(str::trim)
        .filter(|value| !value.is_empty())?
        .parse::<f64>()
        .ok()?;
    let lon = place
        .longitude_deg
        .as_deref()
        .map(str::trim)
        .filter(|value| !value.is_empty())?
        .parse::<f64>()
        .ok()?;
    if !(-90.0..=90.0).contains(&lat) || !(-180.0..=180.0).contains(&lon) {
        return None;
    }
    Some((lat, lon))
}

fn cell(lat: f64, lon: f64) -> (i32, i32) {
    (
        (lat / GRID_CELL_DEG).floor() as i32,
        (lon / GRID_CELL_DEG).floor() as i32,
    )
}

fn normalize_osm_cnpj(raw: &str) -> Option<String> {
    let first = raw.split(';').next().unwrap_or(raw).trim();
    let without_country = first
        .strip_prefix("BR")
        .or_else(|| first.strip_prefix("br"))
        .unwrap_or(first);
    normalize_cnpj(without_country).ok()
}

fn osm_address(place: &OsmPlace) -> Option<(String, String)> {
    let street = place.tags.get("addr:street")?;
    let number = place.tags.get("addr:housenumber")?;
    let street = normalize_street(street);
    let number = normalize_number(number);
    if street.is_empty() || number.is_empty() {
        return None;
    }
    Some((street, number))
}

fn osm_city(place: &OsmPlace) -> Option<String> {
    place
        .tags
        .get("addr:city")
        .map(|value| normalize_name(value))
        .filter(|value| !value.is_empty())
}

fn registry_address(record: &RegistryRecord) -> Option<(String, String)> {
    let street = normalize_street(&record.address_normalized.street);
    let number = normalize_number(&record.address_normalized.number);
    if street.is_empty() || number.is_empty() {
        return None;
    }
    Some((street, number))
}

fn address_label(address: Option<&(String, String)>) -> String {
    match address {
        Some((street, number)) => format!("{street}, {number}"),
        None => String::new(),
    }
}

fn api_point(record: &ApiRecord) -> PointStatus {
    let latitude = record
        .latitude
        .as_deref()
        .map(str::trim)
        .filter(|value| !value.is_empty());
    let longitude = record
        .longitude
        .as_deref()
        .map(str::trim)
        .filter(|value| !value.is_empty());
    let (Some(latitude), Some(longitude)) = (latitude, longitude) else {
        return PointStatus::MissingCoordinates;
    };
    let Ok(latitude) = latitude.parse::<f64>() else {
        return PointStatus::MissingCoordinates;
    };
    let Ok(longitude) = longitude.parse::<f64>() else {
        return PointStatus::MissingCoordinates;
    };
    if !(-34.0..=6.0).contains(&latitude) || !(-74.0..=-28.0).contains(&longitude) {
        return PointStatus::OutOfBrazil;
    }
    let srid = record.srid.trim();
    let crs = record.src.trim();
    let valid = (srid == "4674" && crs.eq_ignore_ascii_case("SIRGAS2000"))
        || (srid == "4326" && crs.eq_ignore_ascii_case("WGS84"));
    if !valid {
        return PointStatus::SridInvalid;
    }
    PointStatus::Valid(latitude, longitude)
}

fn api_context_matches(registry: &RegistryRecord, api: &ApiRecord) -> bool {
    normalize_name(&registry.uf) == normalize_name(&api.uf)
        && normalize_name(&registry.municipality_name_raw) == normalize_name(&api.municipio)
}

fn api_address(api: &ApiRecord) -> Option<(String, String)> {
    split_address(&api.endereco)
}

fn name_tokens(value: &str) -> BTreeSet<String> {
    const STOP: &[&str] = &[
        "AUTO",
        "COMERCIO",
        "COMBUSTIVEIS",
        "COMBUSTIVEL",
        "DA",
        "DAS",
        "DE",
        "DIESEL",
        "DO",
        "DOS",
        "E",
        "ENERGIA",
        "EPP",
        "EIRELI",
        "GASOLINA",
        "LTDA",
        "ME",
        "POSTO",
        "SA",
        "SERVICOS",
    ];
    normalize_name(value)
        .split(|character: char| !character.is_alphanumeric())
        .filter(|token| token.chars().count() >= 4)
        .filter(|token| !STOP.contains(token))
        .map(str::to_string)
        .collect()
}

fn compatible_name_or_brand(registry: &RegistryRecord, place: &OsmPlace) -> bool {
    let registry_brand = registry.brand_raw.trim();
    let osm_brand = place
        .tags
        .get("brand")
        .or_else(|| place.tags.get("operator"))
        .map(|value| value.trim())
        .filter(|value| !value.is_empty());
    if !registry_brand.is_empty() && osm_brand.is_some() {
        return brand_match_text(Some(registry_brand), osm_brand, None);
    }
    let Some(osm_name) = place
        .name
        .as_deref()
        .filter(|value| !value.trim().is_empty())
    else {
        return false;
    };
    let left = name_tokens(&registry.business_name_raw);
    let right = name_tokens(osm_name);
    !left.is_disjoint(&right)
}

fn deterministic_uuid(seed: &str) -> String {
    use sha2::{Digest, Sha256};
    let digest = Sha256::digest(format!("{UUID_SEED}|{seed}").as_bytes());
    let mut bytes = [0u8; 16];
    bytes.copy_from_slice(&digest[..16]);
    bytes[6] = (bytes[6] & 0x0f) | 0x50;
    bytes[8] = (bytes[8] & 0x3f) | 0x80;
    let hex: String = bytes.iter().map(|byte| format!("{byte:02x}")).collect();
    format!(
        "{}-{}-{}-{}-{}",
        &hex[0..8],
        &hex[8..12],
        &hex[12..16],
        &hex[16..20],
        &hex[20..32]
    )
}

fn build_index(osm: &[OsmPlace]) -> OsmIndex {
    let mut grid: HashMap<(i32, i32), Vec<usize>> = HashMap::new();
    let mut cnpj: HashMap<String, Vec<usize>> = HashMap::new();
    let mut address: HashMap<(String, String), Vec<usize>> = HashMap::new();
    let mut points = vec![None; osm.len()];
    for (index, place) in osm.iter().enumerate() {
        if !matches!(place.kind.as_str(), "FUEL_STATION" | "COMBINED_SITE") {
            continue;
        }
        let Some(point) = osm_point(place) else {
            continue;
        };
        points[index] = Some(point);
        grid.entry(cell(point.0, point.1)).or_default().push(index);
        let mut tags = HashSet::new();
        if let Some(raw) = place
            .tags
            .get("ref:vatin")
            .or_else(|| place.tags.get("ref:CNPJ"))
        {
            if let Some(normalized) = normalize_osm_cnpj(raw) {
                tags.insert(normalized);
            }
        }
        if let Some(raw) = &place.cnpj {
            if let Ok(normalized) = normalize_cnpj(raw) {
                tags.insert(normalized);
            }
        }
        for normalized in tags {
            cnpj.entry(normalized).or_default().push(index);
        }
        if let Some((street, number)) = osm_address(place) {
            address.entry((street, number)).or_default().push(index);
        }
    }
    OsmIndex {
        grid,
        cnpj,
        address,
        points,
    }
}

fn grid_candidates(index: &OsmIndex, point: (f64, f64)) -> Vec<usize> {
    let (row, column) = cell(point.0, point.1);
    let mut candidates = Vec::new();
    for row_offset in -1..=1 {
        for column_offset in -1..=1 {
            if let Some(bucket) = index.grid.get(&(row + row_offset, column + column_offset)) {
                candidates.extend(bucket.iter().copied());
            }
        }
    }
    candidates.sort_unstable();
    candidates.dedup();
    candidates
}

fn cnpj_candidates(index: &OsmIndex, cnpj: &str) -> Vec<usize> {
    let mut candidates = index.cnpj.get(cnpj).cloned().unwrap_or_default();
    candidates.sort_unstable();
    candidates.dedup();
    candidates
}

fn address_candidates(index: &OsmIndex, address: &(String, String)) -> Vec<usize> {
    let mut candidates = index.address.get(address).cloned().unwrap_or_default();
    candidates.sort_unstable();
    candidates.dedup();
    candidates
}

fn round_distance(value: f64) -> f64 {
    (value * 10.0).round() / 10.0
}

#[allow(clippy::too_many_arguments)]
fn decision(
    registry: &RegistryRecord,
    tier: &'static str,
    reason: &'static str,
    publish: bool,
    candidate_count: u32,
    candidate: Option<&OsmPlace>,
    distance: Option<f64>,
    api: Option<&ApiRecord>,
    point_status: Option<PointStatus>,
    registry_address: Option<&(String, String)>,
    osm_address: Option<&(String, String)>,
) -> DecisionRow {
    DecisionRow {
        schema_version: DECISION_FORMAT,
        cnpj: registry.source_key.clone(),
        uf: registry.uf.clone(),
        municipality: registry.municipality_name_raw.clone(),
        tier,
        reason,
        publish,
        candidate_count,
        osm_type: candidate.map(|place| place.osm_type.clone()),
        osm_id: candidate.map(|place| place.osm_id),
        distance_m: distance.map(round_distance),
        api_point: matches!(point_status, Some(PointStatus::Valid(_, _))),
        api_srid: api.map(|record| record.srid.clone()).unwrap_or_default(),
        api_crs: api.map(|record| record.src.clone()).unwrap_or_default(),
        registry_address: address_label(registry_address),
        osm_address: address_label(osm_address),
    }
}

fn review_row(
    registry: &RegistryRecord,
    api: &ApiRecord,
    place: &OsmPlace,
    point: (f64, f64),
    distance: f64,
    method: &'static str,
) -> ReviewRow {
    ReviewRow {
        schema_version: REVIEW_SCHEMA_VERSION,
        cnpj: registry.source_key.clone(),
        station_name: api.business.clone(),
        municipality: api.municipio.clone(),
        state: api.uf.to_uppercase(),
        latitude: point.0,
        longitude: point.1,
        method,
        match_distance_m: round_distance(distance),
        brand_match: brand_match_text(
            api.distribuidora.as_deref(),
            place.tags.get("brand").map(String::as_str),
            place.tags.get("operator").map(String::as_str),
        ),
        reviewer: String::new(),
        evidence: format!(
            "anp-registry-api:{};{}:{};{}",
            api.obtained,
            place.osm_type,
            place.osm_id,
            if method.ends_with("CNPJ") {
                "ref-cnpj"
            } else {
                "addr"
            }
        ),
        anp: AnpEvidence {
            address: api.endereco.clone(),
            latitude: api.latitude.clone().unwrap_or_default(),
            longitude: api.longitude.clone().unwrap_or_default(),
            observed_at: api.obtained.clone(),
            origin: api.origin.clone(),
            validation: api.validacao.clone(),
        },
        osm: OsmEvidence {
            osm_type: place.osm_type.clone(),
            osm_id: place.osm_id,
            name: place.name.clone(),
            brand: place
                .tags
                .get("brand")
                .or_else(|| place.tags.get("operator"))
                .cloned(),
            street: place.tags.get("addr:street").cloned().unwrap_or_default(),
            housenumber: place
                .tags
                .get("addr:housenumber")
                .cloned()
                .unwrap_or_default(),
            latitude_deg: place.latitude_deg.clone().unwrap_or_default(),
            longitude_deg: place.longitude_deg.clone().unwrap_or_default(),
            license: place.license.clone(),
            attribution: place.attribution.clone(),
        },
    }
}

fn map_assertion(
    registry: &RegistryRecord,
    place: &OsmPlace,
    point: (f64, f64),
    osm_sha256: &str,
    clocks: &MapClocks,
) -> Result<MapAssertion, &'static str> {
    let display_point = place
        .display_point
        .clone()
        .or_else(|| match place.geometry.as_str() {
            "node" => Some("osm_node".to_string()),
            "way_centroid" | "relation_centroid" => Some("osm_centroid".to_string()),
            _ => None,
        })
        .ok_or("missing_display_point")?;
    if !matches!(display_point.as_str(), "osm_node" | "osm_centroid") {
        return Err("missing_display_point");
    }
    let name = place
        .name
        .as_deref()
        .filter(|value| !value.trim().is_empty())
        .or(place.brand.as_deref())
        .or(place.operator.as_deref())
        .unwrap_or(&registry.business_name_raw)
        .trim()
        .to_string();
    if name.is_empty() {
        return Err("missing_name");
    }
    if name.chars().count() > 200 {
        return Err("name_out_of_bounds");
    }
    let attribution = if place.attribution.trim().is_empty() {
        OSM_ATTRIBUTION.to_string()
    } else {
        place.attribution.clone()
    };
    let id = deterministic_uuid(&format!(
        "{}:{}:{}",
        place.osm_type, place.osm_id, registry.source_key
    ));
    Ok(MapAssertion {
        id,
        source: "OSM".to_string(),
        kind: "FUEL_STATION".to_string(),
        name,
        identities: MapIdentities {
            station_uuid: None,
            cnpj: Some(registry.source_key.clone()),
            place_id: None,
            equipment_id: None,
            connector_id: None,
            osm_type: Some(place.osm_type.clone()),
            osm_id: Some(place.osm_id),
        },
        location: MapLocation {
            latitude_deg: Some(format!("{:.7}", point.0)),
            longitude_deg: Some(format!("{:.7}", point.1)),
            quality: "osm-mapped".to_string(),
            display_point: Some(display_point),
            entrance_latitude_deg: None,
            entrance_longitude_deg: None,
        },
        services: vec!["FUEL".to_string()],
        prices: Vec::new(),
        charging: None,
        clocks: clocks.clone(),
        rights: MapRights {
            source: "OSM".to_string(),
            license: "ADMITTED".to_string(),
            attribution,
            source_sha256: osm_sha256.to_string(),
        },
        capabilities: MapCapabilities {
            price_known: false,
            pin_eligible: true,
            entrance_known: false,
            live_status: false,
        },
    })
}

fn record_tier(
    counts: &mut MatchCounts,
    per_uf: &mut BTreeMap<String, UfCounts>,
    uf: &str,
    tier: &str,
    reason: &str,
) {
    counts.tiers.add(tier);
    *counts.reasons.entry(reason.to_string()).or_default() += 1;
    let bucket = per_uf.entry(uf.to_string()).or_default();
    bucket.registry_rows += 1;
    bucket.tiers.add(tier);
    *bucket.reasons.entry(reason.to_string()).or_default() += 1;
}

fn match_stations(
    inputs: &InputSet,
    index: &OsmIndex,
    options: &RunOptions,
) -> Result<MatchResult, MatchError> {
    let mut api_by_cnpj: HashMap<String, &ApiRecord> = HashMap::new();
    let mut counts = MatchCounts {
        registry_rows: inputs.registry.len() as u64,
        api_records: inputs.api_records.len() as u64,
        ..MatchCounts::default()
    };
    for record in &inputs.api_records {
        match normalize_cnpj(&record.cnpj) {
            Ok(cnpj) => match api_by_cnpj.entry(cnpj) {
                std::collections::hash_map::Entry::Vacant(slot) => {
                    slot.insert(record);
                }
                std::collections::hash_map::Entry::Occupied(_) => {
                    counts.api_duplicates += 1;
                }
            },
            Err(_) => counts.api_invalid_cnpj += 1,
        }
    }

    let mut result = MatchResult::default();
    let mut per_uf: BTreeMap<String, UfCounts> = BTreeMap::new();
    let mut registry_cnpjs: Vec<&String> = inputs.registry.keys().collect();
    registry_cnpjs.sort();

    for cnpj in registry_cnpjs {
        let registry = &inputs.registry[cnpj];
        let api = api_by_cnpj.get(cnpj).copied();
        let point_status = api.map(api_point);
        let point = match point_status {
            Some(PointStatus::Valid(lat, lon)) => Some((lat, lon)),
            _ => None,
        };
        let registry_address = registry_address(registry);
        let mut reasons: BTreeSet<&'static str> = BTreeSet::new();
        let mut emitted = false;

        let context_ok = match (api, point) {
            (Some(api), Some(_)) => {
                if api_context_matches(registry, api) {
                    true
                } else {
                    reasons.insert("registry_context_mismatch");
                    false
                }
            }
            _ => false,
        };

        if !emitted {
            if let (Some(api), Some(point), true) = (api, point, context_ok) {
                let candidates = cnpj_candidates(index, cnpj);
                if candidates.len() == 1 {
                    let place_index = candidates[0];
                    let place = &inputs.osm[place_index];
                    let osm_point = index.points[place_index].expect("indexed point");
                    let distance = haversine_m(point.0, point.1, osm_point.0, osm_point.1);
                    if distance <= options.max_cnpj_distance_m {
                        let mut row = review_row(
                            registry,
                            api,
                            place,
                            point,
                            distance,
                            "ANP_REGISTRY_API+OSM_CNPJ",
                        );
                        row.reviewer = options.reviewer.clone();
                        result.reviews.push(row);
                        let osm_addr = osm_address(place);
                        result.decisions.push(decision(
                            registry,
                            "L1_CNPJ",
                            "cnpj_tag",
                            true,
                            1,
                            Some(place),
                            Some(distance),
                            Some(api),
                            point_status,
                            registry_address.as_ref(),
                            osm_addr.as_ref(),
                        ));
                        record_tier(
                            &mut counts,
                            &mut per_uf,
                            &registry.uf,
                            "L1_CNPJ",
                            "cnpj_tag",
                        );
                        emitted = true;
                    } else {
                        reasons.insert("distance_exceeded");
                    }
                } else if candidates.len() > 1 {
                    reasons.insert("ambiguous_osm");
                }
            }
        }

        if !emitted {
            if let (Some(api), Some(point), true) = (api, point, context_ok) {
                let api_addr = api_address(api);
                let address_ok = match (&api_addr, &registry_address) {
                    (Some(left), Some(right)) if left == right => true,
                    (Some(_), Some(_)) => {
                        reasons.insert("registry_address_mismatch");
                        false
                    }
                    (None, _) => {
                        reasons.insert("missing_number");
                        false
                    }
                    (_, None) => false,
                };
                if address_ok {
                    let wanted = registry_address.as_ref().expect("address checked");
                    let mut candidates: Vec<usize> = grid_candidates(index, point)
                        .into_iter()
                        .filter(|candidate| {
                            osm_address(&inputs.osm[*candidate]).as_ref() == Some(wanted)
                        })
                        .collect();
                    candidates.sort_unstable();
                    candidates.dedup();
                    if candidates.len() == 1 {
                        let place_index = candidates[0];
                        let place = &inputs.osm[place_index];
                        let osm_point = index.points[place_index].expect("indexed point");
                        let distance = haversine_m(point.0, point.1, osm_point.0, osm_point.1);
                        if distance <= options.max_distance_m {
                            let mut row = review_row(
                                registry,
                                api,
                                place,
                                point,
                                distance,
                                "ANP_REGISTRY_API+OSM_ADDRESS_NAME",
                            );
                            row.reviewer = options.reviewer.clone();
                            result.reviews.push(row);
                            let osm_addr = osm_address(place);
                            result.decisions.push(decision(
                                registry,
                                "L1_ADDRESS",
                                "address_50m",
                                true,
                                1,
                                Some(place),
                                Some(distance),
                                Some(api),
                                point_status,
                                registry_address.as_ref(),
                                osm_addr.as_ref(),
                            ));
                            record_tier(
                                &mut counts,
                                &mut per_uf,
                                &registry.uf,
                                "L1_ADDRESS",
                                "address_50m",
                            );
                            emitted = true;
                        } else {
                            reasons.insert("distance_exceeded");
                        }
                    } else if candidates.len() > 1 {
                        reasons.insert("ambiguous_osm");
                    }
                }
            }
        }

        if !emitted {
            let candidates = cnpj_candidates(index, cnpj);
            if candidates.len() == 1 {
                let place_index = candidates[0];
                let place = &inputs.osm[place_index];
                let osm_point = index.points[place_index].expect("indexed point");
                let distance =
                    point.map(|point| haversine_m(point.0, point.1, osm_point.0, osm_point.1));
                match map_assertion(
                    registry,
                    place,
                    osm_point,
                    &inputs.osm_sha256,
                    &options.clocks,
                ) {
                    Ok(assertion) => {
                        let reason = if reasons.contains("distance_exceeded") {
                            "distance_exceeded"
                        } else if reasons.contains("registry_context_mismatch") {
                            "registry_context_mismatch"
                        } else {
                            "cnpj_tag"
                        };
                        let osm_addr = osm_address(place);
                        result.map_assertions.push(assertion);
                        result.decisions.push(decision(
                            registry,
                            "L2_CNPJ",
                            reason,
                            true,
                            1,
                            Some(place),
                            distance,
                            api,
                            point_status,
                            registry_address.as_ref(),
                            osm_addr.as_ref(),
                        ));
                        record_tier(&mut counts, &mut per_uf, &registry.uf, "L2_CNPJ", reason);
                        emitted = true;
                    }
                    Err(reason) => {
                        reasons.insert(reason);
                    }
                }
            } else if candidates.len() > 1 {
                reasons.insert("ambiguous_osm");
            }
        }

        if !emitted {
            if let Some(wanted) = registry_address.as_ref() {
                let mut candidates = address_candidates(index, wanted);
                candidates.retain(|candidate| {
                    let place = &inputs.osm[*candidate];
                    match osm_city(place) {
                        Some(city) if city == normalize_name(&registry.municipality_name_raw) => {
                            true
                        }
                        Some(_) => {
                            reasons.insert("municipality_mismatch");
                            false
                        }
                        None => {
                            reasons.insert("missing_municipality");
                            false
                        }
                    }
                });
                candidates.retain(|candidate| {
                    let place = &inputs.osm[*candidate];
                    if compatible_name_or_brand(registry, place) {
                        true
                    } else {
                        reasons.insert("name_mismatch");
                        false
                    }
                });
                if candidates.len() == 1 {
                    let place_index = candidates[0];
                    let place = &inputs.osm[place_index];
                    let osm_point = index.points[place_index].expect("indexed point");
                    let distance =
                        point.map(|point| haversine_m(point.0, point.1, osm_point.0, osm_point.1));
                    if distance.is_some_and(|value| value > options.max_distance_m) {
                        reasons.insert("distance_exceeded");
                    } else {
                        match map_assertion(
                            registry,
                            place,
                            osm_point,
                            &inputs.osm_sha256,
                            &options.clocks,
                        ) {
                            Ok(assertion) => {
                                let osm_addr = osm_address(place);
                                result.map_assertions.push(assertion);
                                result.decisions.push(decision(
                                    registry,
                                    "L2_ADDRESS",
                                    "address_municipality",
                                    true,
                                    1,
                                    Some(place),
                                    distance,
                                    api,
                                    point_status,
                                    registry_address.as_ref(),
                                    osm_addr.as_ref(),
                                ));
                                record_tier(
                                    &mut counts,
                                    &mut per_uf,
                                    &registry.uf,
                                    "L2_ADDRESS",
                                    "address_municipality",
                                );
                                emitted = true;
                            }
                            Err(reason) => {
                                reasons.insert(reason);
                            }
                        }
                    }
                } else if candidates.len() > 1 {
                    reasons.insert("ambiguous_osm");
                }
            }
        }

        if !emitted {
            let reason = reasons.iter().next().copied().unwrap_or_else(|| {
                if point.is_some() {
                    "missing_osm"
                } else {
                    match point_status {
                        Some(status) => status.reason(),
                        None => "no_api_point",
                    }
                }
            });
            let tier = if point.is_some() { "L3_ANP" } else { "L5" };
            result.decisions.push(decision(
                registry,
                tier,
                reason,
                false,
                0,
                None,
                None,
                api,
                point_status,
                registry_address.as_ref(),
                None,
            ));
            record_tier(&mut counts, &mut per_uf, &registry.uf, tier, reason);
            if api.is_some() {
                result.rejects.push(RejectRow {
                    schema_version: "station-location-reject-v1",
                    cnpj: Some(registry.source_key.clone()),
                    reason,
                    detail: format!("national matcher: {reason}"),
                });
            }
        }
    }

    let mut api_only: Vec<&String> = api_by_cnpj
        .keys()
        .filter(|cnpj| !inputs.registry.contains_key(*cnpj))
        .collect();
    api_only.sort();
    for cnpj in api_only {
        let record = api_by_cnpj[cnpj];
        let uf = if record.uf.trim().is_empty() {
            "??".to_string()
        } else {
            record.uf.to_uppercase()
        };
        result.decisions.push(DecisionRow {
            schema_version: DECISION_FORMAT,
            cnpj: cnpj.clone(),
            uf: uf.clone(),
            municipality: record.municipio.clone(),
            tier: "API_ONLY",
            reason: "missing_registry",
            publish: false,
            candidate_count: 0,
            osm_type: None,
            osm_id: None,
            distance_m: None,
            api_point: matches!(api_point(record), PointStatus::Valid(_, _)),
            api_srid: record.srid.clone(),
            api_crs: record.src.clone(),
            registry_address: String::new(),
            osm_address: String::new(),
        });
        counts.api_only += 1;
        counts.tiers.add("API_ONLY");
        *counts
            .reasons
            .entry("missing_registry".to_string())
            .or_default() += 1;
        let bucket = per_uf.entry(uf).or_default();
        bucket.tiers.add("API_ONLY");
        *bucket
            .reasons
            .entry("missing_registry".to_string())
            .or_default() += 1;
    }

    result
        .reviews
        .sort_by(|left, right| left.cnpj.cmp(&right.cnpj));
    result.rejects.sort_by(|left, right| {
        left.cnpj
            .as_deref()
            .unwrap_or("")
            .cmp(right.cnpj.as_deref().unwrap_or(""))
            .then(left.reason.cmp(right.reason))
    });
    result.decisions.sort_by(|left, right| {
        left.cnpj
            .cmp(&right.cnpj)
            .then(left.tier.cmp(right.tier))
            .then(left.reason.cmp(right.reason))
    });
    result.map_assertions.sort_by(|left, right| {
        left.identities
            .cnpj
            .cmp(&right.identities.cnpj)
            .then(left.identities.osm_type.cmp(&right.identities.osm_type))
            .then(left.identities.osm_id.cmp(&right.identities.osm_id))
    });
    result.review_counts = ReviewCounts {
        input: result.reviews.len() as u64 + result.rejects.len() as u64 + counts.api_duplicates,
        reviewed: result.reviews.len() as u64,
        rejected: result.rejects.len() as u64,
        duplicates: counts.api_duplicates,
    };
    result.counts = counts;
    result.counts.map_assertions = result.map_assertions.len() as u64;
    result.per_uf = per_uf;
    Ok(result)
}

fn ndjson<T: Serialize>(values: &[T]) -> Result<Vec<u8>, MatchError> {
    let mut out = Vec::new();
    for value in values {
        serde_json::to_writer(&mut out, value).map_err(json_error)?;
        out.push(b'\n');
    }
    Ok(out)
}

fn write_file(path: &Path, bytes: &[u8]) -> Result<ManifestFile, MatchError> {
    let file = File::create(path).map_err(io_error)?;
    let mut writer = BufWriter::new(file);
    writer.write_all(bytes).map_err(io_error)?;
    writer.flush().map_err(io_error)?;
    writer.get_ref().sync_all().map_err(io_error)?;
    Ok(ManifestFile {
        bytes: bytes.len() as u64,
        sha256: sha256_hex(bytes),
    })
}

fn elapsed_ms(start: Instant) -> f64 {
    start.elapsed().as_secs_f64() * 1000.0
}

fn default_generation(inputs: &InputSet) -> String {
    let mut seed = Vec::new();
    seed.extend_from_slice(inputs.api_sha256.as_bytes());
    seed.extend_from_slice(inputs.osm_sha256.as_bytes());
    seed.extend_from_slice(inputs.registry_sha256.as_bytes());
    format!("national-{}", &sha256_hex(&seed)[..16])
}

fn summary_bytes(
    inputs: &InputSet,
    result: &MatchResult,
    options: &RunOptions,
    generation: &str,
    outputs: SummaryOutputs,
    map_assertions_contract_valid: bool,
) -> Result<Vec<u8>, MatchError> {
    let summary = MatchSummary {
        format: SUMMARY_FORMAT,
        generation: generation.to_string(),
        inputs: SummaryInputs {
            api_sha256: inputs.api_sha256.clone(),
            api_records: inputs.api_records.len() as u64,
            api_files: inputs.api_files.clone(),
            osm_sha256: inputs.osm_sha256.clone(),
            osm_bytes: inputs.osm_bytes,
            osm_references: inputs.osm.len() as u64,
            registry_sha256: inputs.registry_sha256.clone(),
            registry_bytes: inputs.registry_bytes,
            registry_rows: inputs.registry_rows as u64,
            registry_duplicates: inputs.registry_duplicates as u64,
            invalid_registry_cnpj: inputs.invalid_registry_cnpj as u64,
        },
        limits: SummaryLimits {
            max_distance_m: options.max_distance_m,
            max_cnpj_distance_m: options.max_cnpj_distance_m,
            grid_cell_deg: GRID_CELL_DEG,
            max_api_records: MAX_API_RECORDS,
            max_osm_references: MAX_OSM_REFERENCES,
            max_registry_rows: MAX_REGISTRY_ROWS,
            max_map_assertions: options.max_map_assertions,
            max_review_rows: MAX_REVIEW_ROWS,
            max_review_bytes: MAX_REVIEW_BYTES,
        },
        counts: result.counts.clone(),
        per_uf: result.per_uf.clone(),
        outputs,
        map_assertions_contract_valid,
        timings_file: "timings.json",
    };
    let mut bytes = serde_json::to_vec_pretty(&summary).map_err(json_error)?;
    bytes.push(b'\n');
    Ok(bytes)
}

fn write_run(
    output: &Path,
    inputs: &InputSet,
    result: &MatchResult,
    options: &RunOptions,
    generation: &str,
    timings: &mut Timings,
) -> Result<(), MatchError> {
    if result.map_assertions.len() > options.max_map_assertions {
        return limit(format!(
            "map assertions {} exceed configured {}",
            result.map_assertions.len(),
            options.max_map_assertions
        ));
    }
    let write_start = Instant::now();
    let parent = output.parent().unwrap_or_else(|| Path::new("."));
    fs::create_dir_all(parent).map_err(io_error)?;
    let temp = output.with_extension("tmp-national-match");
    if temp.exists() {
        fs::remove_dir_all(&temp).map_err(io_error)?;
    }
    fs::create_dir(&temp).map_err(io_error)?;

    if result.reviews.len() > MAX_REVIEW_ROWS {
        return limit(format!(
            "review rows {} exceed station-review cap {}",
            result.reviews.len(),
            MAX_REVIEW_ROWS
        ));
    }
    let reviews = ndjson(&result.reviews)?;
    if reviews.len() > MAX_REVIEW_BYTES {
        return limit(format!(
            "reviews.ndjson {} bytes exceed station-review cap {}",
            reviews.len(),
            MAX_REVIEW_BYTES
        ));
    }
    let rejects = ndjson(&result.rejects)?;
    let decisions = ndjson(&result.decisions)?;
    let assertions_ndjson = ndjson(&result.map_assertions)?;
    let mut files = BTreeMap::new();
    let reviews_file = write_file(&temp.join("reviews.ndjson"), &reviews)?;
    let rejects_file = write_file(&temp.join("rejects.ndjson"), &rejects)?;
    files.insert("reviews.ndjson", reviews_file.clone());
    files.insert("rejects.ndjson", rejects_file.clone());
    let manifest = ReviewManifest {
        format: REVIEW_FORMAT,
        parser_version: REVIEW_PARSER_VERSION,
        reviewer: options.reviewer.clone(),
        max_distance_m: options.max_distance_m.max(options.max_cnpj_distance_m),
        source_sha256: inputs.api_sha256.clone(),
        osm_sha256: inputs.osm_sha256.clone(),
        registry_sha256: inputs.registry_sha256.clone(),
        counts: result.review_counts.clone(),
        files,
    };
    let manifest_bytes = {
        let mut bytes = serde_json::to_vec_pretty(&manifest).map_err(json_error)?;
        bytes.push(b'\n');
        bytes
    };
    let manifest_file = write_file(&temp.join("manifest.json"), &manifest_bytes)?;
    let decisions_file = write_file(&temp.join("decisions.ndjson"), &decisions)?;
    let assertions_ndjson_file =
        write_file(&temp.join("map-assertions.ndjson"), &assertions_ndjson)?;

    let corpus = serde_json::json!({
        "format": ASSERTION_FORMAT,
        "version": MAP_VERSION,
        "generation": generation,
        "assertions": &result.map_assertions,
    });
    let corpus_bytes = {
        let mut bytes = serde_json::to_vec_pretty(&corpus).map_err(json_error)?;
        bytes.push(b'\n');
        bytes
    };
    let corpus_valid = if result.map_assertions.is_empty() {
        false
    } else {
        map_contract::parse_assertions(&corpus_bytes).map_err(|error| {
            MatchError::Profile(format!("generated map assertions invalid: {error}"))
        })?;
        true
    };
    let corpus_file = if corpus_valid {
        Some(write_file(
            &temp.join("map-assertions.json"),
            &corpus_bytes,
        )?)
    } else {
        None
    };

    let outputs = SummaryOutputs {
        reviews_ndjson: reviews_file,
        rejects_ndjson: rejects_file,
        review_manifest: manifest_file,
        decisions_ndjson: decisions_file,
        map_assertions_ndjson: assertions_ndjson_file,
        map_assertions_json: corpus_file,
    };
    let summary = summary_bytes(inputs, result, options, generation, outputs, corpus_valid)?;
    write_file(&temp.join("summary.json"), &summary)?;

    timings.write_ms = elapsed_ms(write_start);
    timings.total_ms = timings.load_ms + timings.index_ms + timings.match_ms + timings.write_ms;
    let timings_bytes = {
        let mut bytes = serde_json::to_vec_pretty(timings).map_err(json_error)?;
        bytes.push(b'\n');
        bytes
    };
    write_file(&temp.join("timings.json"), &timings_bytes)?;

    if output.exists() {
        let backup = output.with_extension("bak-national-match");
        if backup.exists() {
            fs::remove_dir_all(&backup).map_err(io_error)?;
        }
        fs::rename(output, &backup).map_err(io_error)?;
        if let Err(error) = fs::rename(&temp, output) {
            let _ = fs::rename(&backup, output);
            return Err(io_error(error));
        }
        let _ = fs::remove_dir_all(&backup);
    } else {
        fs::rename(&temp, output).map_err(io_error)?;
    }
    Ok(())
}

pub fn run(
    api_dir: &Path,
    osm_path: &Path,
    registry_path: &Path,
    output: &Path,
    options: &RunOptions,
) -> Result<RunOutput, MatchError> {
    if !(1.0..=MAX_REVIEW_DISTANCE_M).contains(&options.max_distance_m) {
        return profile("max distance must be 1..200 m");
    }
    if !(1.0..=MAX_REVIEW_DISTANCE_M).contains(&options.max_cnpj_distance_m) {
        return profile("max CNPJ distance must be 1..200 m");
    }
    if options.max_map_assertions == 0 || options.max_map_assertions > MAX_MAP_ASSERTIONS {
        return profile(format!(
            "max map assertions must be 1..{}",
            MAX_MAP_ASSERTIONS
        ));
    }
    if options.reviewer.trim().is_empty() {
        return profile("reviewer is required");
    }
    if options.clocks.observed_at.len() < 20
        || options.clocks.published_at.len() < 20
        || options.clocks.fetched_at.len() < 20
    {
        return profile("RFC3339 clocks are required");
    }

    let total_start = Instant::now();
    let load_start = Instant::now();
    let inputs = load_inputs(api_dir, osm_path, registry_path)?;
    let load_ms = elapsed_ms(load_start);
    let index_start = Instant::now();
    let index = build_index(&inputs.osm);
    let index_ms = elapsed_ms(index_start);
    let match_start = Instant::now();
    let result = match_stations(&inputs, &index, options)?;
    let match_ms = elapsed_ms(match_start);
    let generation = if options.generation.trim().is_empty() {
        default_generation(&inputs)
    } else {
        options.generation.clone()
    };
    let mut timings = Timings {
        load_ms,
        index_ms,
        match_ms,
        ..Timings::default()
    };
    write_run(output, &inputs, &result, options, &generation, &mut timings)?;
    timings.total_ms = elapsed_ms(total_start);
    Ok(RunOutput {
        inputs,
        result,
        timings,
        generation,
    })
}

#[cfg(test)]
mod tests {
    use super::*;

    fn clocks() -> MapClocks {
        MapClocks {
            observed_at: "2026-10-25T21:21:21Z".to_string(),
            published_at: "2026-10-25T21:21:21Z".to_string(),
            fetched_at: "2026-10-25T21:21:21Z".to_string(),
        }
    }

    fn options() -> RunOptions {
        RunOptions {
            max_distance_m: 50.0,
            max_cnpj_distance_m: 200.0,
            max_map_assertions: MAX_MAP_ASSERTIONS,
            generation: "test-generation".to_string(),
            reviewer: "tester".to_string(),
            clocks: clocks(),
        }
    }

    fn api(cnpj: &str, address: &str, lat: &str, lon: &str, srid: &str, src: &str) -> ApiRecord {
        ApiRecord {
            cnpj: cnpj.to_string(),
            business: "POSTO TESTE LTDA".to_string(),
            endereco: address.to_string(),
            complemento: String::new(),
            bairro: String::new(),
            cep: String::new(),
            municipio: "CURITIBA".to_string(),
            uf: "PR".to_string(),
            latitude: Some(lat.to_string()),
            longitude: Some(lon.to_string()),
            srid: srid.to_string(),
            src: src.to_string(),
            validacao: "Posição Geográfica não validada".to_string(),
            accuracy: String::new(),
            obtained: "22/08/2024".to_string(),
            origin: "Preposto da empresa".to_string(),
            distribuidora: Some("IPIRANGA".to_string()),
        }
    }

    fn registry(cnpj: &str, street: &str, number: &str, city: &str) -> RegistryRecord {
        RegistryRecord {
            source_key: cnpj.to_string(),
            business_name_raw: "POSTO TESTE LTDA".to_string(),
            address_raw: format!("{street}, {number}"),
            address_normalized: RegistryAddress {
                street: street.to_string(),
                number: number.to_string(),
            },
            uf: "PR".to_string(),
            municipality_name_raw: city.to_string(),
            municipality_ibge: "4106902".to_string(),
            brand_raw: "IPIRANGA".to_string(),
        }
    }

    #[allow(clippy::too_many_arguments)]
    fn osm(
        osm_id: i64,
        cnpj_tag: Option<&str>,
        street: &str,
        number: &str,
        city: &str,
        lat: &str,
        lon: &str,
        name: &str,
    ) -> OsmPlace {
        let mut tags = BTreeMap::new();
        tags.insert("addr:street".to_string(), street.to_string());
        tags.insert("addr:housenumber".to_string(), number.to_string());
        tags.insert("addr:city".to_string(), city.to_string());
        tags.insert("brand".to_string(), "Ipiranga".to_string());
        if let Some(cnpj) = cnpj_tag {
            tags.insert("ref:vatin".to_string(), cnpj.to_string());
        }
        OsmPlace {
            key: format!("osm:node:{osm_id}"),
            osm_type: "node".to_string(),
            osm_id,
            kind: "FUEL_STATION".to_string(),
            name: Some(name.to_string()),
            latitude_deg: Some(lat.to_string()),
            longitude_deg: Some(lon.to_string()),
            geometry: "node".to_string(),
            display_point: Some("osm_node".to_string()),
            cnpj: None,
            brand: Some("Ipiranga".to_string()),
            operator: None,
            tags,
            license: "ODbL-1.0".to_string(),
            attribution: "© OpenStreetMap contributors — ODbL 1.0".to_string(),
        }
    }

    fn bare_osm(osm_id: i64, lat: f64, lon: f64) -> OsmPlace {
        OsmPlace {
            key: format!("osm:node:{osm_id}"),
            osm_type: "node".to_string(),
            osm_id,
            kind: "FUEL_STATION".to_string(),
            name: None,
            latitude_deg: Some(format!("{lat:.7}")),
            longitude_deg: Some(format!("{lon:.7}")),
            geometry: "node".to_string(),
            display_point: Some("osm_node".to_string()),
            cnpj: None,
            brand: None,
            operator: None,
            tags: BTreeMap::new(),
            license: String::new(),
            attribution: String::new(),
        }
    }

    fn inputs(
        api_records: Vec<ApiRecord>,
        osm: Vec<OsmPlace>,
        registry_rows: Vec<RegistryRecord>,
    ) -> InputSet {
        let registry: HashMap<String, RegistryRecord> = registry_rows
            .into_iter()
            .map(|row| (row.source_key.clone(), row))
            .collect();
        InputSet {
            api_records,
            api_files: Vec::new(),
            api_sha256: "a".repeat(64),
            osm,
            osm_sha256: "b".repeat(64),
            osm_bytes: 0,
            registry_rows: registry.len(),
            registry,
            registry_sha256: "c".repeat(64),
            registry_bytes: 0,
            registry_duplicates: 0,
            invalid_registry_cnpj: 0,
        }
    }

    fn run_match(inputs: &InputSet) -> MatchResult {
        let index = build_index(&inputs.osm);
        match_stations(inputs, &index, &options()).expect("match")
    }

    #[test]
    fn cnpj_tag_match_is_l1_and_emits_review() {
        let cnpj = "01122233300000";
        let inputs = inputs(
            vec![api(
                cnpj,
                "RUA TESTE, 100",
                "-25.4",
                "-49.2",
                "4674",
                "SIRGAS2000",
            )],
            vec![osm(
                1,
                Some(cnpj),
                "RUA OUTRA",
                "999",
                "CURITIBA",
                "-25.40001",
                "-49.20001",
                "Posto Teste",
            )],
            vec![registry(cnpj, "RUA TESTE", "100", "CURITIBA")],
        );
        let result = run_match(&inputs);
        assert_eq!(result.reviews.len(), 1);
        assert_eq!(result.reviews[0].method, "ANP_REGISTRY_API+OSM_CNPJ");
        assert!(result.map_assertions.is_empty());
        assert_eq!(result.decisions[0].tier, "L1_CNPJ");
        assert!(result.decisions[0].publish);
        assert_eq!(result.reviews[0].reviewer, "tester");
    }

    #[test]
    fn address_match_within_fifty_meters_is_l1() {
        let cnpj = "01122233300000";
        let inputs = inputs(
            vec![api(
                cnpj,
                "RUA TESTE, 100",
                "-25.4",
                "-49.2",
                "4674",
                "SIRGAS2000",
            )],
            vec![osm(
                1,
                None,
                "RUA TESTE",
                "100",
                "CURITIBA",
                "-25.40001",
                "-49.20001",
                "Posto Teste",
            )],
            vec![registry(cnpj, "RUA TESTE", "100", "CURITIBA")],
        );
        let result = run_match(&inputs);
        assert_eq!(result.reviews.len(), 1);
        assert_eq!(
            result.reviews[0].method,
            "ANP_REGISTRY_API+OSM_ADDRESS_NAME"
        );
        assert_eq!(result.decisions[0].tier, "L1_ADDRESS");
    }

    #[test]
    fn ambiguous_osm_is_not_picked() {
        let cnpj = "01122233300000";
        let inputs = inputs(
            vec![api(
                cnpj,
                "RUA TESTE, 100",
                "-25.4",
                "-49.2",
                "4674",
                "SIRGAS2000",
            )],
            vec![
                osm(
                    1,
                    Some(cnpj),
                    "RUA TESTE",
                    "100",
                    "CURITIBA",
                    "-25.40001",
                    "-49.20001",
                    "Posto Teste",
                ),
                osm(
                    2,
                    Some(cnpj),
                    "RUA TESTE",
                    "100",
                    "CURITIBA",
                    "-25.40002",
                    "-49.20002",
                    "Posto Teste",
                ),
            ],
            vec![registry(cnpj, "RUA TESTE", "100", "CURITIBA")],
        );
        let result = run_match(&inputs);
        assert!(result.reviews.is_empty());
        assert!(result.map_assertions.is_empty());
        assert_eq!(result.decisions[0].tier, "L3_ANP");
        assert_eq!(result.decisions[0].reason, "ambiguous_osm");
        assert!(!result.decisions[0].publish);
    }

    #[test]
    fn fifty_one_meters_is_refused() {
        let cnpj = "01122233300000";
        let inputs = inputs(
            vec![api(
                cnpj,
                "RUA TESTE, 100",
                "-25.4",
                "-49.2",
                "4674",
                "SIRGAS2000",
            )],
            vec![osm(
                1,
                None,
                "RUA TESTE",
                "100",
                "CURITIBA",
                "-25.40046",
                "-49.2",
                "Posto Teste",
            )],
            vec![registry(cnpj, "RUA TESTE", "100", "CURITIBA")],
        );
        let result = run_match(&inputs);
        assert!(result.reviews.is_empty());
        assert!(result.map_assertions.is_empty());
        assert_eq!(result.decisions[0].tier, "L3_ANP");
        assert_eq!(result.decisions[0].reason, "distance_exceeded");
    }

    #[test]
    fn wrong_municipality_is_refused() {
        let cnpj = "01122233300000";
        let inputs = inputs(
            Vec::new(),
            vec![osm(
                1,
                None,
                "RUA TESTE",
                "100",
                "OUTRA CIDADE",
                "-25.40001",
                "-49.20001",
                "Posto Teste",
            )],
            vec![registry(cnpj, "RUA TESTE", "100", "CURITIBA")],
        );
        let result = run_match(&inputs);
        assert!(result.map_assertions.is_empty());
        assert_eq!(result.decisions[0].tier, "L5");
        assert_eq!(result.decisions[0].reason, "municipality_mismatch");
    }

    #[test]
    fn srid_4674_is_accepted_for_l3() {
        let cnpj = "01122233300000";
        let inputs = inputs(
            vec![api(
                cnpj,
                "RUA TESTE, 100",
                "-25.4",
                "-49.2",
                "4674",
                "SIRGAS2000",
            )],
            Vec::new(),
            vec![registry(cnpj, "RUA TESTE", "100", "CURITIBA")],
        );
        let result = run_match(&inputs);
        assert!(result.reviews.is_empty());
        assert_eq!(result.decisions[0].tier, "L3_ANP");
        assert!(result.decisions[0].api_point);
        assert_eq!(result.decisions[0].reason, "missing_osm");
    }

    #[test]
    fn srid_invalid_is_refused() {
        let cnpj = "01122233300000";
        let inputs = inputs(
            vec![api(
                cnpj,
                "RUA TESTE, 100",
                "-25.4",
                "-49.2",
                "9999",
                "UNKNOWN",
            )],
            Vec::new(),
            vec![registry(cnpj, "RUA TESTE", "100", "CURITIBA")],
        );
        let result = run_match(&inputs);
        assert_eq!(result.decisions[0].tier, "L5");
        assert_eq!(result.decisions[0].reason, "srid_invalid");
    }

    #[test]
    fn point_outside_brazil_is_refused() {
        let cnpj = "01122233300000";
        let inputs = inputs(
            vec![api(
                cnpj,
                "RUA TESTE, 100",
                "10.0",
                "-49.2",
                "4674",
                "SIRGAS2000",
            )],
            Vec::new(),
            vec![registry(cnpj, "RUA TESTE", "100", "CURITIBA")],
        );
        let result = run_match(&inputs);
        assert_eq!(result.decisions[0].tier, "L5");
        assert_eq!(result.decisions[0].reason, "out_of_brazil");
        assert!(!result.decisions[0].api_point);
    }

    #[test]
    fn l2_cnpj_without_anp_point_emits_valid_map_assertion() {
        let cnpj = "01122233300000";
        let inputs = inputs(
            Vec::new(),
            vec![osm(
                1,
                Some(cnpj),
                "RUA TESTE",
                "100",
                "CURITIBA",
                "-25.40001",
                "-49.20001",
                "Posto Teste",
            )],
            vec![registry(cnpj, "RUA TESTE", "100", "CURITIBA")],
        );
        let result = run_match(&inputs);
        assert_eq!(result.map_assertions.len(), 1);
        assert_eq!(result.decisions[0].tier, "L2_CNPJ");
        let corpus = serde_json::json!({
            "format": ASSERTION_FORMAT,
            "version": MAP_VERSION,
            "generation": "test-generation",
            "assertions": &result.map_assertions,
        });
        let parsed = map_contract::parse_assertions(&serde_json::to_vec(&corpus).unwrap())
            .expect("valid map assertions");
        assert_eq!(parsed.assertions.len(), 1);
        assert_eq!(parsed.assertions[0].location.quality, "osm-mapped");
        assert!(parsed.assertions[0].capabilities.pin_eligible);
    }

    #[test]
    fn osm_cnpj_prefix_and_suffix_are_normalized() {
        assert_eq!(
            normalize_osm_cnpj("BR01122233300000").as_deref(),
            Some("01122233300000")
        );
        assert_eq!(
            normalize_osm_cnpj("BR01122233300000;BRRS1090259503").as_deref(),
            Some("01122233300000")
        );
        assert_eq!(normalize_osm_cnpj("not-a-cnpj"), None);
    }

    #[test]
    fn outputs_are_deterministic_across_two_runs() {
        let cnpj = "01122233300000";
        let inputs = inputs(
            vec![
                api(
                    cnpj,
                    "RUA TESTE, 100",
                    "-25.4",
                    "-49.2",
                    "4674",
                    "SIRGAS2000",
                ),
                api(
                    "05938540000134",
                    "RUA OUTRA, 200",
                    "-25.41",
                    "-49.21",
                    "4326",
                    "WGS84",
                ),
            ],
            vec![
                osm(
                    1,
                    Some(cnpj),
                    "RUA TESTE",
                    "100",
                    "CURITIBA",
                    "-25.40001",
                    "-49.20001",
                    "Posto Teste",
                ),
                osm(
                    2,
                    None,
                    "RUA OUTRA",
                    "200",
                    "CURITIBA",
                    "-25.41001",
                    "-49.21001",
                    "Posto Outro",
                ),
            ],
            vec![
                registry(cnpj, "RUA TESTE", "100", "CURITIBA"),
                registry("05938540000134", "RUA OUTRA", "200", "CURITIBA"),
            ],
        );
        let first = run_match(&inputs);
        let second = run_match(&inputs);
        assert_eq!(
            serde_json::to_vec(&first.reviews).unwrap(),
            serde_json::to_vec(&second.reviews).unwrap()
        );
        assert_eq!(
            serde_json::to_vec(&first.decisions).unwrap(),
            serde_json::to_vec(&second.decisions).unwrap()
        );
        assert_eq!(
            serde_json::to_vec(&first.map_assertions).unwrap(),
            serde_json::to_vec(&second.map_assertions).unwrap()
        );
    }

    #[test]
    fn api_record_without_registry_is_api_only() {
        let inputs = inputs(
            vec![api(
                "01122233300000",
                "RUA TESTE, 100",
                "-25.4",
                "-49.2",
                "4674",
                "SIRGAS2000",
            )],
            Vec::new(),
            Vec::new(),
        );
        let result = run_match(&inputs);
        assert_eq!(result.decisions[0].tier, "API_ONLY");
        assert_eq!(result.decisions[0].reason, "missing_registry");
    }

    #[test]
    #[ignore = "1M-point capacity check; run explicitly with --release --ignored"]
    fn million_osm_points_index_within_budget() {
        let start = Instant::now();
        let osm: Vec<OsmPlace> = (0..1_000_000)
            .map(|index| {
                let lat = -33.0 + (index % 1000) as f64 * 0.0001;
                let lon = -73.0 + (index / 1000) as f64 * 0.0001;
                bare_osm(index as i64, lat, lon)
            })
            .collect();
        let index = build_index(&osm);
        assert!(!index.grid.is_empty());
        let candidates = grid_candidates(&index, (-32.95, -72.95));
        assert!(!candidates.is_empty());
        assert!(
            start.elapsed().as_secs() < 30,
            "1M index took {:?}",
            start.elapsed()
        );
    }
}

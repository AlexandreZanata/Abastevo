//! Open-data geocoding for registry stations without an ANP point.
//!
//! Local files only: no network, database, native engine or clock. The input
//! decisions are the A2 `decisions.ndjson`; only `L5` rows are processed. The
//! matcher tries, in order, an OSM CNPJ tag, a unique OSM normalized
//! street+number+name, and an IBGE CNEFE establishment point with the exact
//! street+number in the station municipality. RFB CNPJ data may replace a stale
//! registry address only for the same CNPJ. A centroid, a nearest street or a
//! street-only/CEP-only match is never used.

use crate::location_review::{haversine_m, normalize_number, normalize_street};
use crate::map_contract::{self, ASSERTION_FORMAT, VERSION as MAP_VERSION};
use crate::municipality::normalize_name;
use crate::national_match::{
    self, address_candidates, build_index, cnpj_candidates, compatible_name_or_brand,
    map_assertion, osm_city, registry_address, MapAssertion, MapClocks, MatchError, OsmIndex,
    OsmPlace, RegistryRecord,
};
use crate::types::sha256_hex;
use serde::{Deserialize, Serialize};
use std::collections::{BTreeMap, BTreeSet, HashMap, HashSet};
use std::fs::{self, File};
use std::io::{BufWriter, Write};
use std::path::Path;
use std::time::Instant;

pub const DECISION_FORMAT: &str = "abastevo.open-geocode-decision-v1";
pub const CANDIDATE_FORMAT: &str = "abastevo.open-geocode-candidate-v1";
pub const SUMMARY_FORMAT: &str = "abastevo.open-geocode-summary-v1";
pub const DEFAULT_AMBIGUITY_RADIUS_M: f64 = 100.0;
pub const MAX_CNEFE_POINTS: usize = 5_000_000;
pub const MAX_RFB_ROWS: usize = 500_000;
pub const MAX_DECISIONS: usize = 200_000;
pub const MAX_MAP_ASSERTIONS: usize = 10_000;
pub const MAX_CNEFE_FILE_BYTES: u64 = 2 << 30;
pub const MAX_RFB_FILE_BYTES: u64 = 256 << 20;
pub const MAX_DECISIONS_FILE_BYTES: u64 = 256 << 20;
pub const MAX_OSM_FILE_BYTES: u64 = 1 << 30;
pub const MAX_REGISTRY_FILE_BYTES: u64 = 1 << 30;

const CNEFE_SOURCE: &str = "IBGE-CNEFE-2022";
const REASON_PRIORITY: &[&str] = &[
    "missing_number",
    "missing_address",
    "ambiguous_osm",
    "municipality_mismatch",
    "missing_municipality",
    "name_mismatch",
    "ambiguous_cnefe",
    "missing_establishment_evidence",
    "missing_cnefe",
    "missing_osm",
];

#[derive(Debug, Clone, Deserialize)]
pub struct InputDecision {
    #[serde(default)]
    pub cnpj: String,
    #[serde(default)]
    pub tier: String,
    #[serde(default)]
    pub publish: bool,
}

#[derive(Debug, Clone, Deserialize)]
pub struct CnefePoint {
    #[serde(default)]
    pub municipality_ibge: String,
    #[serde(default)]
    pub street: String,
    #[serde(default)]
    pub number: String,
    #[serde(default)]
    pub cep: String,
    #[serde(default)]
    pub latitude_deg: String,
    #[serde(default)]
    pub longitude_deg: String,
    #[serde(default)]
    pub establishment_name: String,
    #[serde(default)]
    pub establishment: bool,
}

#[derive(Debug, Clone, Deserialize)]
pub struct RfbAddress {
    #[serde(default)]
    pub cnpj: String,
    #[serde(default)]
    pub street: String,
    #[serde(default)]
    pub number: String,
    #[serde(default)]
    pub cep: String,
    #[serde(default)]
    pub district: String,
}

#[derive(Debug, Clone, Serialize)]
pub struct GeocodeDecision {
    pub schema_version: &'static str,
    pub cnpj: String,
    pub uf: String,
    pub municipality: String,
    pub municipality_ibge: String,
    pub tier: &'static str,
    pub reason: &'static str,
    pub publish: bool,
    pub address_source: &'static str,
    pub address: String,
    pub osm_type: Option<String>,
    pub osm_id: Option<i64>,
    pub cnefe_latitude_deg: Option<String>,
    pub cnefe_longitude_deg: Option<String>,
    pub cnefe_cep: Option<String>,
    pub cnefe_establishment: Option<String>,
    pub candidate_count: u32,
    pub spread_m: Option<f64>,
}

#[derive(Debug, Clone, Serialize)]
pub struct GeocodeCandidate {
    pub schema_version: &'static str,
    pub cnpj: String,
    pub uf: String,
    pub municipality: String,
    pub municipality_ibge: String,
    pub tier: &'static str,
    pub reason: &'static str,
    pub publish: bool,
    pub latitude_deg: String,
    pub longitude_deg: String,
    pub address_source: &'static str,
    pub address: String,
    pub cep: Option<String>,
    pub establishment_name: Option<String>,
    pub candidate_count: u32,
    pub spread_m: f64,
    pub source: &'static str,
    pub source_sha256: String,
}

#[derive(Debug, Clone, Default, Serialize)]
pub struct GeocodeCounts {
    pub processed: u64,
    pub l2_cnpj: u64,
    pub l2_address: u64,
    pub l4_address: u64,
    pub l5: u64,
    pub reasons: BTreeMap<String, u64>,
}

#[derive(Debug, Clone, Default, Serialize)]
pub struct UfCounts {
    pub processed: u64,
    pub l2_cnpj: u64,
    pub l2_address: u64,
    pub l4_address: u64,
    pub l5: u64,
    pub reasons: BTreeMap<String, u64>,
}

#[derive(Debug, Clone, Default)]
pub struct GeocodeResult {
    pub decisions: Vec<GeocodeDecision>,
    pub candidates: Vec<GeocodeCandidate>,
    pub map_assertions: Vec<MapAssertion>,
    pub counts: GeocodeCounts,
    pub per_uf: BTreeMap<String, UfCounts>,
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
pub struct GeocodeOptions {
    pub ambiguity_radius_m: f64,
    pub max_map_assertions: usize,
    pub generation: String,
    pub clocks: MapClocks,
}

#[derive(Debug, Clone)]
pub struct GeocodeInputs {
    pub registry: HashMap<String, RegistryRecord>,
    pub registry_sha256: String,
    pub registry_bytes: u64,
    pub registry_rows: usize,
    pub decisions_sha256: String,
    pub decisions_bytes: u64,
    pub decisions_rows: usize,
    pub l5: HashSet<String>,
    pub osm: Vec<OsmPlace>,
    pub osm_sha256: String,
    pub osm_bytes: u64,
    pub cnefe: Vec<CnefePoint>,
    pub cnefe_sha256: String,
    pub cnefe_bytes: u64,
    pub rfb: HashMap<String, RfbAddress>,
    pub rfb_sha256: String,
    pub rfb_bytes: u64,
    pub rfb_rows: usize,
}

#[derive(Debug, Clone)]
pub struct RunOutput {
    pub inputs: GeocodeInputs,
    pub result: GeocodeResult,
    pub timings: Timings,
    pub generation: String,
}

struct CnefeIndex {
    points: HashMap<(String, String, String), Vec<usize>>,
}

impl CnefeIndex {
    fn candidates(&self, key: &(String, String, String)) -> Vec<usize> {
        let mut candidates = self.points.get(key).cloned().unwrap_or_default();
        candidates.sort_unstable();
        candidates
    }
}

#[derive(Debug, Clone, Serialize)]
struct ManifestFile {
    bytes: u64,
    sha256: String,
}

#[derive(Debug, Clone, Serialize)]
struct SummaryInputs {
    registry_sha256: String,
    registry_bytes: u64,
    registry_rows: u64,
    decisions_sha256: String,
    decisions_bytes: u64,
    decisions_rows: u64,
    l5_rows: u64,
    osm_sha256: String,
    osm_bytes: u64,
    osm_references: u64,
    cnefe_sha256: String,
    cnefe_bytes: u64,
    cnefe_points: u64,
    rfb_sha256: String,
    rfb_bytes: u64,
    rfb_rows: u64,
}

#[derive(Debug, Clone, Serialize)]
struct SummaryLimits {
    ambiguity_radius_m: f64,
    max_cnefe_points: usize,
    max_rfb_rows: usize,
    max_decisions: usize,
    max_map_assertions: usize,
}

#[derive(Debug, Clone, Serialize)]
struct SummaryOutputs {
    decisions_ndjson: ManifestFile,
    candidates_ndjson: ManifestFile,
    map_assertions_ndjson: ManifestFile,
    map_assertions_json: Option<ManifestFile>,
}

#[derive(Debug, Clone, Serialize)]
struct GeocodeSummary {
    format: &'static str,
    generation: String,
    inputs: SummaryInputs,
    limits: SummaryLimits,
    counts: GeocodeCounts,
    per_uf: BTreeMap<String, UfCounts>,
    outputs: SummaryOutputs,
    map_assertions_contract_valid: bool,
    timings_file: &'static str,
}

fn read_bounded(path: &Path, max: u64) -> Result<Vec<u8>, MatchError> {
    let metadata = fs::metadata(path).map_err(|error| MatchError::Io(error.to_string()))?;
    if metadata.len() > max {
        return Err(MatchError::Limit(format!(
            "{} exceeds {} bytes",
            path.display(),
            max
        )));
    }
    fs::read(path).map_err(|error| MatchError::Io(error.to_string()))
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

pub fn load_decisions(path: &Path) -> Result<(HashSet<String>, String, u64, usize), MatchError> {
    let raw = read_bounded(path, MAX_DECISIONS_FILE_BYTES)?;
    let sha256 = sha256_hex(&raw);
    let mut l5 = HashSet::new();
    let mut rows = 0usize;
    for line in raw.split(|byte| *byte == b'\n') {
        if line.iter().all(|byte| byte.is_ascii_whitespace()) {
            continue;
        }
        if rows >= MAX_DECISIONS {
            return limit(format!("decisions exceed {}", MAX_DECISIONS));
        }
        rows += 1;
        let decision: InputDecision = serde_json::from_slice(line).map_err(json_error)?;
        if decision.tier != "L5" || decision.publish {
            continue;
        }
        if let Ok(cnpj) = crate::cnpj::normalize_cnpj(&decision.cnpj) {
            l5.insert(cnpj);
        }
    }
    Ok((l5, sha256, raw.len() as u64, rows))
}

pub fn load_cnefe(path: &Path) -> Result<(Vec<CnefePoint>, String, u64), MatchError> {
    let raw = read_bounded(path, MAX_CNEFE_FILE_BYTES)?;
    let sha256 = sha256_hex(&raw);
    let mut points = Vec::new();
    for line in raw.split(|byte| *byte == b'\n') {
        if line.iter().all(|byte| byte.is_ascii_whitespace()) {
            continue;
        }
        if points.len() >= MAX_CNEFE_POINTS {
            return limit(format!("CNEFE points exceed {}", MAX_CNEFE_POINTS));
        }
        points.push(serde_json::from_slice::<CnefePoint>(line).map_err(json_error)?);
    }
    Ok((points, sha256, raw.len() as u64))
}

pub fn load_rfb(
    path: &Path,
) -> Result<(HashMap<String, RfbAddress>, String, u64, usize), MatchError> {
    let raw = read_bounded(path, MAX_RFB_FILE_BYTES)?;
    let sha256 = sha256_hex(&raw);
    let mut rows = HashMap::new();
    let mut parsed = 0usize;
    for line in raw.split(|byte| *byte == b'\n') {
        if line.iter().all(|byte| byte.is_ascii_whitespace()) {
            continue;
        }
        if parsed >= MAX_RFB_ROWS {
            return limit(format!("RFB rows exceed {}", MAX_RFB_ROWS));
        }
        parsed += 1;
        let row = serde_json::from_slice::<RfbAddress>(line).map_err(json_error)?;
        let Ok(cnpj) = crate::cnpj::normalize_cnpj(&row.cnpj) else {
            continue;
        };
        rows.entry(cnpj).or_insert(row);
    }
    Ok((rows, sha256, raw.len() as u64, parsed))
}

fn cnefe_point(point: &CnefePoint) -> Option<(f64, f64)> {
    let latitude = point.latitude_deg.trim().parse::<f64>().ok()?;
    let longitude = point.longitude_deg.trim().parse::<f64>().ok()?;
    if !(-34.0..=6.0).contains(&latitude) || !(-74.0..=-28.0).contains(&longitude) {
        return None;
    }
    Some((latitude, longitude))
}

fn cnefe_establishment(point: &CnefePoint) -> bool {
    point.establishment || !point.establishment_name.trim().is_empty()
}

fn build_cnefe_index(points: &[CnefePoint]) -> CnefeIndex {
    let mut index: HashMap<(String, String, String), Vec<usize>> = HashMap::new();
    for (position, point) in points.iter().enumerate() {
        let municipality = point.municipality_ibge.trim();
        if municipality.is_empty() {
            continue;
        }
        let street = normalize_street(&point.street);
        let number = normalize_number(&point.number);
        if street.is_empty() || number.is_empty() || number == "SN" {
            continue;
        }
        index
            .entry((municipality.to_string(), street, number))
            .or_default()
            .push(position);
    }
    CnefeIndex { points: index }
}

fn rfb_address(row: &RfbAddress) -> Option<(String, String)> {
    let street = normalize_street(&row.street);
    let number = normalize_number(&row.number);
    if street.is_empty() || number.is_empty() || number == "SN" {
        return None;
    }
    Some((street, number))
}

fn address_label(address: &(String, String)) -> String {
    format!("{}, {}", address.0, address.1)
}

fn pick_reason(reasons: &BTreeSet<&'static str>) -> &'static str {
    for reason in REASON_PRIORITY {
        if reasons.contains(reason) {
            return reason;
        }
    }
    "no_match"
}

fn round_distance(value: f64) -> f64 {
    (value * 10.0).round() / 10.0
}

fn max_spread(points: &[(f64, f64)]) -> f64 {
    let mut maximum = 0.0f64;
    for (index, left) in points.iter().enumerate() {
        for right in points.iter().skip(index + 1) {
            let distance = haversine_m(left.0, left.1, right.0, right.1);
            if distance > maximum {
                maximum = distance;
            }
        }
    }
    maximum
}

fn l5_decision(
    registry: &RegistryRecord,
    cnpj: &str,
    reason: &'static str,
    address_source: &'static str,
    address: &(String, String),
    candidate_count: u32,
) -> GeocodeDecision {
    GeocodeDecision {
        schema_version: DECISION_FORMAT,
        cnpj: cnpj.to_string(),
        uf: registry.uf.clone(),
        municipality: registry.municipality_name_raw.clone(),
        municipality_ibge: registry.municipality_ibge.clone(),
        tier: "L5",
        reason,
        publish: false,
        address_source,
        address: address_label(address),
        osm_type: None,
        osm_id: None,
        cnefe_latitude_deg: None,
        cnefe_longitude_deg: None,
        cnefe_cep: None,
        cnefe_establishment: None,
        candidate_count,
        spread_m: None,
    }
}

fn l2_decision(
    registry: &RegistryRecord,
    cnpj: &str,
    tier: &'static str,
    reason: &'static str,
    address_source: &'static str,
    address: &(String, String),
    place: &OsmPlace,
) -> GeocodeDecision {
    GeocodeDecision {
        schema_version: DECISION_FORMAT,
        cnpj: cnpj.to_string(),
        uf: registry.uf.clone(),
        municipality: registry.municipality_name_raw.clone(),
        municipality_ibge: registry.municipality_ibge.clone(),
        tier,
        reason,
        publish: true,
        address_source,
        address: address_label(address),
        osm_type: Some(place.osm_type.clone()),
        osm_id: Some(place.osm_id),
        cnefe_latitude_deg: None,
        cnefe_longitude_deg: None,
        cnefe_cep: None,
        cnefe_establishment: None,
        candidate_count: 1,
        spread_m: None,
    }
}

#[allow(clippy::too_many_arguments)]
fn geocode_station(
    registry: &RegistryRecord,
    cnpj: &str,
    osm_index: &OsmIndex,
    osm: &[OsmPlace],
    osm_sha256: &str,
    cnefe_index: &CnefeIndex,
    cnefe: &[CnefePoint],
    cnefe_sha256: &str,
    rfb: &HashMap<String, RfbAddress>,
    options: &GeocodeOptions,
) -> (
    Option<MapAssertion>,
    Option<GeocodeCandidate>,
    GeocodeDecision,
) {
    let rfb_address = rfb.get(cnpj).and_then(rfb_address);
    let registry_address = registry_address(registry);
    let (address, address_source) = match (&rfb_address, &registry_address) {
        (Some(value), _) => (value.clone(), "rfb"),
        (None, Some(value)) => (value.clone(), "registry"),
        (None, None) => {
            return (
                None,
                None,
                l5_decision(
                    registry,
                    cnpj,
                    "missing_address",
                    "registry",
                    &(String::new(), String::new()),
                    0,
                ),
            );
        }
    };
    if address.1.is_empty() || address.1 == "SN" {
        return (
            None,
            None,
            l5_decision(
                registry,
                cnpj,
                "missing_number",
                address_source,
                &address,
                0,
            ),
        );
    }

    let mut reasons: BTreeSet<&'static str> = BTreeSet::new();

    let cnpj_matches = cnpj_candidates(osm_index, cnpj);
    if cnpj_matches.len() == 1 {
        let place = &osm[cnpj_matches[0]];
        if let Some(point) = osm_index.point(cnpj_matches[0]) {
            match map_assertion(registry, place, point, osm_sha256, &options.clocks) {
                Ok(assertion) => {
                    let decision = l2_decision(
                        registry,
                        cnpj,
                        "L2_CNPJ",
                        "cnpj_tag",
                        address_source,
                        &address,
                        place,
                    );
                    return (Some(assertion), None, decision);
                }
                Err(reason) => {
                    reasons.insert(reason);
                }
            }
        } else {
            reasons.insert("missing_osm_point");
        }
    } else if cnpj_matches.len() > 1 {
        reasons.insert("ambiguous_osm");
    }

    let mut address_matches = address_candidates(osm_index, &address);
    address_matches.retain(|candidate| {
        let place = &osm[*candidate];
        match osm_city(place) {
            Some(city) if city == normalize_name(&registry.municipality_name_raw) => true,
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
    address_matches.retain(|candidate| {
        if compatible_name_or_brand(registry, &osm[*candidate]) {
            true
        } else {
            reasons.insert("name_mismatch");
            false
        }
    });
    if address_matches.len() == 1 {
        let place = &osm[address_matches[0]];
        if let Some(point) = osm_index.point(address_matches[0]) {
            match map_assertion(registry, place, point, osm_sha256, &options.clocks) {
                Ok(assertion) => {
                    let decision = l2_decision(
                        registry,
                        cnpj,
                        "L2_ADDRESS",
                        "address_municipality",
                        address_source,
                        &address,
                        place,
                    );
                    return (Some(assertion), None, decision);
                }
                Err(reason) => {
                    reasons.insert(reason);
                }
            }
        } else {
            reasons.insert("missing_osm_point");
        }
    } else if address_matches.len() > 1 {
        reasons.insert("ambiguous_osm");
    }

    let municipality_ibge = registry.municipality_ibge.trim();
    if municipality_ibge.is_empty() {
        reasons.insert("missing_municipality");
    } else {
        let key = (
            municipality_ibge.to_string(),
            address.0.clone(),
            address.1.clone(),
        );
        let candidates = cnefe_index.candidates(&key);
        if candidates.is_empty() {
            reasons.insert("missing_cnefe");
        } else {
            let mut points: Vec<(usize, (f64, f64))> = candidates
                .into_iter()
                .filter(|candidate| cnefe_establishment(&cnefe[*candidate]))
                .filter_map(|candidate| {
                    cnefe_point(&cnefe[candidate]).map(|point| (candidate, point))
                })
                .collect();
            if points.is_empty() {
                reasons.insert("missing_establishment_evidence");
            } else {
                points.sort_by(|left, right| {
                    left.1
                         .0
                        .partial_cmp(&right.1 .0)
                        .unwrap_or(std::cmp::Ordering::Equal)
                        .then(
                            left.1
                                 .1
                                .partial_cmp(&right.1 .1)
                                .unwrap_or(std::cmp::Ordering::Equal),
                        )
                        .then(left.0.cmp(&right.0))
                });
                let coordinates: Vec<(f64, f64)> = points.iter().map(|(_, point)| *point).collect();
                let spread = max_spread(&coordinates);
                if spread > options.ambiguity_radius_m {
                    reasons.insert("ambiguous_cnefe");
                } else {
                    let (chosen, (latitude, longitude)) = points[0];
                    let point = &cnefe[chosen];
                    let cep = if point.cep.trim().is_empty() {
                        None
                    } else {
                        Some(point.cep.clone())
                    };
                    let establishment_name = if point.establishment_name.trim().is_empty() {
                        None
                    } else {
                        Some(point.establishment_name.clone())
                    };
                    let candidate = GeocodeCandidate {
                        schema_version: CANDIDATE_FORMAT,
                        cnpj: cnpj.to_string(),
                        uf: registry.uf.clone(),
                        municipality: registry.municipality_name_raw.clone(),
                        municipality_ibge: registry.municipality_ibge.clone(),
                        tier: "L4_ADDRESS",
                        reason: "cnefe_establishment",
                        publish: false,
                        latitude_deg: format!("{latitude:.7}"),
                        longitude_deg: format!("{longitude:.7}"),
                        address_source,
                        address: address_label(&address),
                        cep: cep.clone(),
                        establishment_name: establishment_name.clone(),
                        candidate_count: points.len() as u32,
                        spread_m: round_distance(spread),
                        source: CNEFE_SOURCE,
                        source_sha256: cnefe_sha256.to_string(),
                    };
                    let decision = GeocodeDecision {
                        schema_version: DECISION_FORMAT,
                        cnpj: cnpj.to_string(),
                        uf: registry.uf.clone(),
                        municipality: registry.municipality_name_raw.clone(),
                        municipality_ibge: registry.municipality_ibge.clone(),
                        tier: "L4_ADDRESS",
                        reason: "cnefe_establishment",
                        publish: false,
                        address_source,
                        address: address_label(&address),
                        osm_type: None,
                        osm_id: None,
                        cnefe_latitude_deg: Some(format!("{latitude:.7}")),
                        cnefe_longitude_deg: Some(format!("{longitude:.7}")),
                        cnefe_cep: cep,
                        cnefe_establishment: establishment_name,
                        candidate_count: points.len() as u32,
                        spread_m: Some(round_distance(spread)),
                    };
                    return (None, Some(candidate), decision);
                }
            }
        }
    }

    let reason = pick_reason(&reasons);
    (
        None,
        None,
        l5_decision(registry, cnpj, reason, address_source, &address, 0),
    )
}

fn record(
    counts: &mut GeocodeCounts,
    per_uf: &mut BTreeMap<String, UfCounts>,
    decision: &GeocodeDecision,
) {
    counts.processed += 1;
    *counts
        .reasons
        .entry(decision.reason.to_string())
        .or_default() += 1;
    match decision.tier {
        "L2_CNPJ" => counts.l2_cnpj += 1,
        "L2_ADDRESS" => counts.l2_address += 1,
        "L4_ADDRESS" => counts.l4_address += 1,
        _ => counts.l5 += 1,
    }
    let bucket = per_uf.entry(decision.uf.clone()).or_default();
    bucket.processed += 1;
    *bucket
        .reasons
        .entry(decision.reason.to_string())
        .or_default() += 1;
    match decision.tier {
        "L2_CNPJ" => bucket.l2_cnpj += 1,
        "L2_ADDRESS" => bucket.l2_address += 1,
        "L4_ADDRESS" => bucket.l4_address += 1,
        _ => bucket.l5 += 1,
    }
}

fn geocode(
    inputs: &GeocodeInputs,
    osm_index: &OsmIndex,
    cnefe_index: &CnefeIndex,
    options: &GeocodeOptions,
) -> GeocodeResult {
    let mut result = GeocodeResult::default();
    let mut counts = GeocodeCounts::default();
    let mut per_uf: BTreeMap<String, UfCounts> = BTreeMap::new();
    let mut cnpjs: Vec<&String> = inputs.l5.iter().collect();
    cnpjs.sort();
    for cnpj in cnpjs {
        let Some(registry) = inputs.registry.get(cnpj) else {
            let decision = GeocodeDecision {
                schema_version: DECISION_FORMAT,
                cnpj: cnpj.clone(),
                uf: String::new(),
                municipality: String::new(),
                municipality_ibge: String::new(),
                tier: "L5",
                reason: "missing_registry",
                publish: false,
                address_source: "registry",
                address: String::new(),
                osm_type: None,
                osm_id: None,
                cnefe_latitude_deg: None,
                cnefe_longitude_deg: None,
                cnefe_cep: None,
                cnefe_establishment: None,
                candidate_count: 0,
                spread_m: None,
            };
            record(&mut counts, &mut per_uf, &decision);
            result.decisions.push(decision);
            continue;
        };
        let (assertion, candidate, decision) = geocode_station(
            registry,
            cnpj,
            osm_index,
            &inputs.osm,
            &inputs.osm_sha256,
            cnefe_index,
            &inputs.cnefe,
            &inputs.cnefe_sha256,
            &inputs.rfb,
            options,
        );
        if let Some(assertion) = assertion {
            result.map_assertions.push(assertion);
        }
        if let Some(candidate) = candidate {
            result.candidates.push(candidate);
        }
        record(&mut counts, &mut per_uf, &decision);
        result.decisions.push(decision);
    }
    result
        .decisions
        .sort_by(|left, right| left.cnpj.cmp(&right.cnpj));
    result
        .candidates
        .sort_by(|left, right| left.cnpj.cmp(&right.cnpj));
    result.map_assertions.sort_by(|left, right| {
        left.identities
            .cnpj
            .cmp(&right.identities.cnpj)
            .then(left.identities.osm_type.cmp(&right.identities.osm_type))
            .then(left.identities.osm_id.cmp(&right.identities.osm_id))
    });
    result.counts = counts;
    result.per_uf = per_uf;
    result
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
    let file = File::create(path).map_err(|error| MatchError::Io(error.to_string()))?;
    let mut writer = BufWriter::new(file);
    writer
        .write_all(bytes)
        .map_err(|error| MatchError::Io(error.to_string()))?;
    writer
        .flush()
        .map_err(|error| MatchError::Io(error.to_string()))?;
    writer
        .get_ref()
        .sync_all()
        .map_err(|error| MatchError::Io(error.to_string()))?;
    Ok(ManifestFile {
        bytes: bytes.len() as u64,
        sha256: sha256_hex(bytes),
    })
}

fn elapsed_ms(start: Instant) -> f64 {
    start.elapsed().as_secs_f64() * 1000.0
}

fn default_generation(inputs: &GeocodeInputs) -> String {
    let mut seed = Vec::new();
    seed.extend_from_slice(inputs.registry_sha256.as_bytes());
    seed.extend_from_slice(inputs.decisions_sha256.as_bytes());
    seed.extend_from_slice(inputs.osm_sha256.as_bytes());
    seed.extend_from_slice(inputs.cnefe_sha256.as_bytes());
    seed.extend_from_slice(inputs.rfb_sha256.as_bytes());
    format!("open-geocode-{}", &sha256_hex(&seed)[..16])
}

fn summary_bytes(
    inputs: &GeocodeInputs,
    result: &GeocodeResult,
    options: &GeocodeOptions,
    generation: &str,
    outputs: SummaryOutputs,
    map_assertions_contract_valid: bool,
) -> Result<Vec<u8>, MatchError> {
    let summary = GeocodeSummary {
        format: SUMMARY_FORMAT,
        generation: generation.to_string(),
        inputs: SummaryInputs {
            registry_sha256: inputs.registry_sha256.clone(),
            registry_bytes: inputs.registry_bytes,
            registry_rows: inputs.registry_rows as u64,
            decisions_sha256: inputs.decisions_sha256.clone(),
            decisions_bytes: inputs.decisions_bytes,
            decisions_rows: inputs.decisions_rows as u64,
            l5_rows: inputs.l5.len() as u64,
            osm_sha256: inputs.osm_sha256.clone(),
            osm_bytes: inputs.osm_bytes,
            osm_references: inputs.osm.len() as u64,
            cnefe_sha256: inputs.cnefe_sha256.clone(),
            cnefe_bytes: inputs.cnefe_bytes,
            cnefe_points: inputs.cnefe.len() as u64,
            rfb_sha256: inputs.rfb_sha256.clone(),
            rfb_bytes: inputs.rfb_bytes,
            rfb_rows: inputs.rfb_rows as u64,
        },
        limits: SummaryLimits {
            ambiguity_radius_m: options.ambiguity_radius_m,
            max_cnefe_points: MAX_CNEFE_POINTS,
            max_rfb_rows: MAX_RFB_ROWS,
            max_decisions: MAX_DECISIONS,
            max_map_assertions: options.max_map_assertions,
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
    inputs: &GeocodeInputs,
    result: &GeocodeResult,
    options: &GeocodeOptions,
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
    fs::create_dir_all(parent).map_err(|error| MatchError::Io(error.to_string()))?;
    let temp = output.with_extension("tmp-open-geocode");
    if temp.exists() {
        fs::remove_dir_all(&temp).map_err(|error| MatchError::Io(error.to_string()))?;
    }
    fs::create_dir(&temp).map_err(|error| MatchError::Io(error.to_string()))?;

    let decisions = ndjson(&result.decisions)?;
    let candidates = ndjson(&result.candidates)?;
    let assertions_ndjson = ndjson(&result.map_assertions)?;
    let decisions_file = write_file(&temp.join("decisions.ndjson"), &decisions)?;
    let candidates_file = write_file(&temp.join("candidates.ndjson"), &candidates)?;
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
        decisions_ndjson: decisions_file,
        candidates_ndjson: candidates_file,
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
        let backup = output.with_extension("bak-open-geocode");
        if backup.exists() {
            fs::remove_dir_all(&backup).map_err(|error| MatchError::Io(error.to_string()))?;
        }
        fs::rename(output, &backup).map_err(|error| MatchError::Io(error.to_string()))?;
        if let Err(error) = fs::rename(&temp, output) {
            let _ = fs::rename(&backup, output);
            return Err(MatchError::Io(error.to_string()));
        }
        let _ = fs::remove_dir_all(&backup);
    } else {
        fs::rename(&temp, output).map_err(|error| MatchError::Io(error.to_string()))?;
    }
    Ok(())
}

pub fn load_inputs(
    registry_path: &Path,
    decisions_path: &Path,
    osm_path: &Path,
    cnefe_path: &Path,
    rfb_path: &Path,
) -> Result<GeocodeInputs, MatchError> {
    let registry = national_match::load_registry(registry_path)?;
    let (l5, decisions_sha256, decisions_bytes, decisions_rows) = load_decisions(decisions_path)?;
    let osm = national_match::load_osm(osm_path)?;
    let (cnefe, cnefe_sha256, cnefe_bytes) = load_cnefe(cnefe_path)?;
    let (rfb, rfb_sha256, rfb_bytes, rfb_rows) = load_rfb(rfb_path)?;
    Ok(GeocodeInputs {
        registry: registry.rows,
        registry_sha256: registry.sha256,
        registry_bytes: registry.bytes,
        registry_rows: registry.parsed_rows,
        decisions_sha256,
        decisions_bytes,
        decisions_rows,
        l5,
        osm: osm.places,
        osm_sha256: osm.sha256,
        osm_bytes: osm.bytes,
        cnefe,
        cnefe_sha256,
        cnefe_bytes,
        rfb,
        rfb_sha256,
        rfb_bytes,
        rfb_rows,
    })
}

pub fn run(
    registry_path: &Path,
    decisions_path: &Path,
    osm_path: &Path,
    cnefe_path: &Path,
    rfb_path: &Path,
    output: &Path,
    options: &GeocodeOptions,
) -> Result<RunOutput, MatchError> {
    if !(1.0..=1000.0).contains(&options.ambiguity_radius_m) {
        return profile("ambiguity radius must be 1..1000 m");
    }
    if options.max_map_assertions == 0 || options.max_map_assertions > MAX_MAP_ASSERTIONS {
        return profile(format!(
            "max map assertions must be 1..{}",
            MAX_MAP_ASSERTIONS
        ));
    }
    if options.clocks.observed_at.len() < 20
        || options.clocks.published_at.len() < 20
        || options.clocks.fetched_at.len() < 20
    {
        return profile("RFC3339 clocks are required");
    }

    let total_start = Instant::now();
    let load_start = Instant::now();
    let inputs = load_inputs(
        registry_path,
        decisions_path,
        osm_path,
        cnefe_path,
        rfb_path,
    )?;
    let load_ms = elapsed_ms(load_start);
    let index_start = Instant::now();
    let osm_index = build_index(&inputs.osm);
    let cnefe_index = build_cnefe_index(&inputs.cnefe);
    let index_ms = elapsed_ms(index_start);
    let match_start = Instant::now();
    let result = geocode(&inputs, &osm_index, &cnefe_index, options);
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

    fn options() -> GeocodeOptions {
        GeocodeOptions {
            ambiguity_radius_m: 100.0,
            max_map_assertions: MAX_MAP_ASSERTIONS,
            generation: "test-generation".to_string(),
            clocks: clocks(),
        }
    }

    fn registry(cnpj: &str, street: &str, number: &str, city: &str, ibge: &str) -> RegistryRecord {
        RegistryRecord {
            source_key: cnpj.to_string(),
            business_name_raw: "POSTO TESTE LTDA".to_string(),
            address_raw: format!("{street}, {number}"),
            address_normalized: crate::national_match::RegistryAddress {
                street: street.to_string(),
                number: number.to_string(),
            },
            uf: "PR".to_string(),
            municipality_name_raw: city.to_string(),
            municipality_ibge: ibge.to_string(),
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

    fn cnefe(
        ibge: &str,
        street: &str,
        number: &str,
        cep: &str,
        lat: f64,
        lon: f64,
        establishment: &str,
    ) -> CnefePoint {
        CnefePoint {
            municipality_ibge: ibge.to_string(),
            street: street.to_string(),
            number: number.to_string(),
            cep: cep.to_string(),
            latitude_deg: format!("{lat:.7}"),
            longitude_deg: format!("{lon:.7}"),
            establishment_name: establishment.to_string(),
            establishment: !establishment.is_empty(),
        }
    }

    fn rfb(cnpj: &str, street: &str, number: &str, cep: &str) -> RfbAddress {
        RfbAddress {
            cnpj: cnpj.to_string(),
            street: street.to_string(),
            number: number.to_string(),
            cep: cep.to_string(),
            district: String::new(),
        }
    }

    fn inputs(
        registry_rows: Vec<RegistryRecord>,
        l5: &[&str],
        osm_rows: Vec<OsmPlace>,
        cnefe_rows: Vec<CnefePoint>,
        rfb_rows: Vec<RfbAddress>,
    ) -> GeocodeInputs {
        let registry: HashMap<String, RegistryRecord> = registry_rows
            .into_iter()
            .map(|row| (row.source_key.clone(), row))
            .collect();
        let rfb: HashMap<String, RfbAddress> = rfb_rows
            .into_iter()
            .map(|row| (row.cnpj.clone(), row))
            .collect();
        GeocodeInputs {
            registry_sha256: "a".repeat(64),
            registry_bytes: 0,
            registry_rows: registry.len(),
            registry,
            decisions_sha256: "b".repeat(64),
            decisions_bytes: 0,
            decisions_rows: l5.len(),
            l5: l5.iter().map(|cnpj| cnpj.to_string()).collect(),
            osm: osm_rows,
            osm_sha256: "c".repeat(64),
            osm_bytes: 0,
            cnefe: cnefe_rows,
            cnefe_sha256: "d".repeat(64),
            cnefe_bytes: 0,
            rfb,
            rfb_sha256: "e".repeat(64),
            rfb_bytes: 0,
            rfb_rows: 0,
        }
    }

    fn run_geocode(inputs: &GeocodeInputs) -> GeocodeResult {
        let osm_index = build_index(&inputs.osm);
        let cnefe_index = build_cnefe_index(&inputs.cnefe);
        geocode(inputs, &osm_index, &cnefe_index, &options())
    }

    #[test]
    fn cnefe_exact_number_match_is_l4() {
        let cnpj = "01122233300000";
        let inputs = inputs(
            vec![registry(
                cnpj,
                "AVENIDA TESTE",
                "100",
                "CURITIBA",
                "4106902",
            )],
            &[cnpj],
            Vec::new(),
            vec![cnefe(
                "4106902",
                "AVENIDA TESTE",
                "100",
                "80000000",
                -25.4,
                -49.2,
                "POSTO TESTE",
            )],
            Vec::new(),
        );
        let result = run_geocode(&inputs);
        assert_eq!(result.decisions.len(), 1);
        assert_eq!(result.decisions[0].tier, "L4_ADDRESS");
        assert!(!result.decisions[0].publish);
        assert_eq!(result.decisions[0].reason, "cnefe_establishment");
        assert_eq!(result.candidates.len(), 1);
        assert!(!result.candidates[0].publish);
        assert!(result.map_assertions.is_empty());
    }

    #[test]
    fn missing_number_is_l5() {
        let cnpj = "01122233300000";
        let inputs = inputs(
            vec![registry(cnpj, "AVENIDA TESTE", "SN", "CURITIBA", "4106902")],
            &[cnpj],
            Vec::new(),
            vec![cnefe(
                "4106902",
                "AVENIDA TESTE",
                "100",
                "80000000",
                -25.4,
                -49.2,
                "POSTO TESTE",
            )],
            Vec::new(),
        );
        let result = run_geocode(&inputs);
        assert_eq!(result.decisions[0].tier, "L5");
        assert_eq!(result.decisions[0].reason, "missing_number");
        assert!(result.candidates.is_empty());
    }

    #[test]
    fn two_cnefe_points_over_100m_are_ambiguous() {
        let cnpj = "01122233300000";
        let inputs = inputs(
            vec![registry(
                cnpj,
                "AVENIDA TESTE",
                "100",
                "CURITIBA",
                "4106902",
            )],
            &[cnpj],
            Vec::new(),
            vec![
                cnefe(
                    "4106902",
                    "AVENIDA TESTE",
                    "100",
                    "80000000",
                    -25.4,
                    -49.2,
                    "POSTO TESTE A",
                ),
                cnefe(
                    "4106902",
                    "AVENIDA TESTE",
                    "100",
                    "80000000",
                    -25.4018,
                    -49.2,
                    "POSTO TESTE B",
                ),
            ],
            Vec::new(),
        );
        let result = run_geocode(&inputs);
        assert_eq!(result.decisions[0].tier, "L5");
        assert_eq!(result.decisions[0].reason, "ambiguous_cnefe");
        assert!(result.candidates.is_empty());
    }

    #[test]
    fn accent_and_abbreviation_normalization_matches() {
        let cnpj = "01122233300000";
        let inputs = inputs(
            vec![registry(cnpj, "AV. TESTE", "100", "CURITIBA", "4106902")],
            &[cnpj],
            Vec::new(),
            vec![cnefe(
                "4106902",
                "AVENIDA TESTE",
                "100",
                "80000000",
                -25.4,
                -49.2,
                "POSTO TESTE",
            )],
            Vec::new(),
        );
        let result = run_geocode(&inputs);
        assert_eq!(result.decisions[0].tier, "L4_ADDRESS");
    }

    #[test]
    fn rfb_address_overrides_stale_registry_address() {
        let cnpj = "01122233300000";
        let inputs = inputs(
            vec![registry(cnpj, "RUA ANTIGA", "10", "CURITIBA", "4106902")],
            &[cnpj],
            Vec::new(),
            vec![cnefe(
                "4106902",
                "AVENIDA NOVA",
                "200",
                "80000000",
                -25.4,
                -49.2,
                "POSTO TESTE",
            )],
            vec![rfb(cnpj, "AVENIDA NOVA", "200", "80000000")],
        );
        let result = run_geocode(&inputs);
        assert_eq!(result.decisions[0].tier, "L4_ADDRESS");
        assert_eq!(result.decisions[0].address_source, "rfb");
        assert_eq!(result.decisions[0].address, "NOVA, 200");
        assert_eq!(result.candidates[0].address_source, "rfb");
    }

    #[test]
    fn rfb_cnpj_mismatch_does_not_override() {
        let cnpj = "01122233300000";
        let inputs = inputs(
            vec![registry(cnpj, "RUA ANTIGA", "10", "CURITIBA", "4106902")],
            &[cnpj],
            Vec::new(),
            vec![cnefe(
                "4106902",
                "AVENIDA NOVA",
                "200",
                "80000000",
                -25.4,
                -49.2,
                "POSTO TESTE",
            )],
            vec![rfb("05938540000134", "AVENIDA NOVA", "200", "80000000")],
        );
        let result = run_geocode(&inputs);
        assert_eq!(result.decisions[0].tier, "L5");
        assert_eq!(result.decisions[0].reason, "missing_cnefe");
        assert_eq!(result.decisions[0].address_source, "registry");
    }

    #[test]
    fn osm_cnpj_tag_is_l2() {
        let cnpj = "01122233300000";
        let inputs = inputs(
            vec![registry(
                cnpj,
                "AVENIDA TESTE",
                "100",
                "CURITIBA",
                "4106902",
            )],
            &[cnpj],
            vec![osm(
                1,
                Some(cnpj),
                "RUA OUTRA",
                "999",
                "CURITIBA",
                "-25.4",
                "-49.2",
                "Posto Teste",
            )],
            Vec::new(),
            Vec::new(),
        );
        let result = run_geocode(&inputs);
        assert_eq!(result.decisions[0].tier, "L2_CNPJ");
        assert!(result.decisions[0].publish);
        assert_eq!(result.map_assertions.len(), 1);
        assert!(result.candidates.is_empty());
    }

    #[test]
    fn osm_address_name_is_l2() {
        let cnpj = "01122233300000";
        let inputs = inputs(
            vec![registry(
                cnpj,
                "AVENIDA TESTE",
                "100",
                "CURITIBA",
                "4106902",
            )],
            &[cnpj],
            vec![osm(
                1,
                None,
                "AVENIDA TESTE",
                "100",
                "CURITIBA",
                "-25.4",
                "-49.2",
                "Posto Teste",
            )],
            Vec::new(),
            Vec::new(),
        );
        let result = run_geocode(&inputs);
        assert_eq!(result.decisions[0].tier, "L2_ADDRESS");
        assert!(result.decisions[0].publish);
        assert_eq!(result.map_assertions.len(), 1);
    }

    #[test]
    fn duplicate_cnefe_points_within_100m_are_deterministic() {
        let cnpj = "01122233300000";
        let inputs = inputs(
            vec![registry(
                cnpj,
                "AVENIDA TESTE",
                "100",
                "CURITIBA",
                "4106902",
            )],
            &[cnpj],
            Vec::new(),
            vec![
                cnefe(
                    "4106902",
                    "AVENIDA TESTE",
                    "100",
                    "80000000",
                    -25.40005,
                    -49.2,
                    "POSTO TESTE B",
                ),
                cnefe(
                    "4106902",
                    "AVENIDA TESTE",
                    "100",
                    "80000000",
                    -25.4,
                    -49.2,
                    "POSTO TESTE A",
                ),
            ],
            Vec::new(),
        );
        let first = run_geocode(&inputs);
        let second = run_geocode(&inputs);
        assert_eq!(first.decisions[0].tier, "L4_ADDRESS");
        assert_eq!(first.decisions[0].candidate_count, 2);
        assert!(first.decisions[0].spread_m.unwrap() < 100.0);
        assert_eq!(
            serde_json::to_vec(&first.candidates).unwrap(),
            serde_json::to_vec(&second.candidates).unwrap()
        );
    }

    #[test]
    fn no_evidence_is_l5() {
        let cnpj = "01122233300000";
        let inputs = inputs(
            vec![registry(
                cnpj,
                "AVENIDA TESTE",
                "100",
                "CURITIBA",
                "4106902",
            )],
            &[cnpj],
            Vec::new(),
            Vec::new(),
            Vec::new(),
        );
        let result = run_geocode(&inputs);
        assert_eq!(result.decisions[0].tier, "L5");
        assert_eq!(result.decisions[0].reason, "missing_cnefe");
        assert!(!result.decisions[0].publish);
    }

    #[test]
    fn outputs_are_deterministic_across_two_runs() {
        let cnpj = "01122233300000";
        let inputs = inputs(
            vec![registry(
                cnpj,
                "AVENIDA TESTE",
                "100",
                "CURITIBA",
                "4106902",
            )],
            &[cnpj],
            vec![osm(
                1,
                None,
                "AVENIDA TESTE",
                "100",
                "CURITIBA",
                "-25.4",
                "-49.2",
                "Posto Teste",
            )],
            vec![cnefe(
                "4106902",
                "AVENIDA TESTE",
                "100",
                "80000000",
                -25.4,
                -49.2,
                "POSTO TESTE",
            )],
            Vec::new(),
        );
        let first = run_geocode(&inputs);
        let second = run_geocode(&inputs);
        assert_eq!(
            serde_json::to_vec(&first.decisions).unwrap(),
            serde_json::to_vec(&second.decisions).unwrap()
        );
        assert_eq!(
            serde_json::to_vec(&first.candidates).unwrap(),
            serde_json::to_vec(&second.candidates).unwrap()
        );
        assert_eq!(
            serde_json::to_vec(&first.map_assertions).unwrap(),
            serde_json::to_vec(&second.map_assertions).unwrap()
        );
    }
}

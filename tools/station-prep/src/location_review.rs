//! Cross-source review of station coordinates for the public map overlay.
//!
//! Local files only: no network, database or clock. An official ANP registry
//! API record supplies station identity and a reported point; an independently
//! prepared OSM reference must carry the exact same street and house number and
//! lie within the configured distance. Proximity alone never matches. A brand
//! difference is retained as evidence, not used as the identity key.

use crate::cnpj::normalize_cnpj;
use crate::types::sha256_hex;
use serde::{Deserialize, Serialize};
use std::collections::{BTreeMap, HashMap};
use std::fs::{self, File};
use std::io::{BufWriter, Write};
use std::path::Path;

pub const REVIEW_FORMAT: &str = "abastevo.station-location-review-v1";
pub const REVIEW_PARSER_VERSION: &str = "station-location-review-v1";
pub const REVIEW_SCHEMA_VERSION: &str = "station-location-review-v1";
pub const DEFAULT_MAX_DISTANCE_M: f64 = 50.0;
const EARTH_RADIUS_M: f64 = 6_371_000.0;

#[derive(Debug)]
pub enum ReviewError {
    Io(String),
    Json(String),
    Profile(String),
}

impl std::fmt::Display for ReviewError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            ReviewError::Io(message) => write!(f, "{message}"),
            ReviewError::Json(message) => write!(f, "{message}"),
            ReviewError::Profile(message) => write!(f, "{message}"),
        }
    }
}

impl std::error::Error for ReviewError {}

fn io_error(error: std::io::Error) -> ReviewError {
    ReviewError::Io(error.to_string())
}

fn json_error(error: serde_json::Error) -> ReviewError {
    ReviewError::Json(error.to_string())
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
    pub municipio: String,
    #[serde(default)]
    pub uf: String,
    #[serde(default)]
    pub latitude: Option<String>,
    #[serde(default)]
    pub longitude: Option<String>,
    #[serde(default)]
    pub validacao: String,
    #[serde(default, rename = "origemInformacao")]
    pub origin: String,
    #[serde(default, rename = "dataObtencao")]
    pub obtained: String,
    #[serde(default)]
    pub distribuidora: Option<String>,
}

#[derive(Debug, Clone, Deserialize)]
pub struct OsmReference {
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
    pub quality: String,
    #[serde(default)]
    pub tags: BTreeMap<String, String>,
    #[serde(default)]
    pub license: String,
    #[serde(default)]
    pub attribution: String,
}

#[derive(Debug, Clone, Deserialize)]
pub struct RegistryRow {
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
}

#[derive(Debug, Clone, Default, Deserialize)]
pub struct RegistryAddress {
    #[serde(default)]
    pub street: String,
    #[serde(default)]
    pub number: String,
}

#[derive(Debug, Clone, Serialize)]
pub struct ReviewRow {
    pub schema_version: &'static str,
    pub cnpj: String,
    pub station_name: String,
    pub municipality: String,
    pub state: String,
    pub latitude: f64,
    pub longitude: f64,
    pub method: &'static str,
    pub match_distance_m: f64,
    pub brand_match: bool,
    pub reviewer: String,
    pub evidence: String,
    pub anp: AnpEvidence,
    pub osm: OsmEvidence,
}

#[derive(Debug, Clone, Serialize)]
pub struct AnpEvidence {
    pub address: String,
    pub latitude: String,
    pub longitude: String,
    pub observed_at: String,
    pub origin: String,
    pub validation: String,
}

#[derive(Debug, Clone, Serialize)]
pub struct OsmEvidence {
    pub osm_type: String,
    pub osm_id: i64,
    pub name: Option<String>,
    pub brand: Option<String>,
    pub street: String,
    pub housenumber: String,
    pub latitude_deg: String,
    pub longitude_deg: String,
    pub license: String,
    pub attribution: String,
}

#[derive(Debug, Clone, Serialize)]
pub struct RejectRow {
    pub schema_version: &'static str,
    pub cnpj: Option<String>,
    pub reason: &'static str,
    pub detail: String,
}

#[derive(Debug, Clone, Default, Serialize)]
pub struct Counts {
    pub input: u64,
    pub reviewed: u64,
    pub rejected: u64,
    pub duplicates: u64,
}

#[derive(Debug)]
pub struct ReviewRun {
    pub counts: Counts,
    pub reviews: Vec<ReviewRow>,
    pub rejects: Vec<RejectRow>,
    pub source_sha256: String,
    pub osm_sha256: String,
    pub registry_sha256: String,
}

#[derive(Debug, Serialize)]
struct ManifestFile {
    bytes: u64,
    sha256: String,
}

#[derive(Debug, Serialize)]
struct Manifest {
    format: &'static str,
    parser_version: &'static str,
    reviewer: String,
    max_distance_m: f64,
    source_sha256: String,
    osm_sha256: String,
    registry_sha256: String,
    counts: Counts,
    files: BTreeMap<&'static str, ManifestFile>,
}

fn fold(value: &str) -> String {
    value
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

/// Normalize a street label for exact comparison: fold accents, expand only
/// unambiguous road-type abbreviations and drop punctuation. The raw source
/// text is preserved in evidence.
pub(crate) fn normalize_street(value: &str) -> String {
    let mut folded = fold(value);
    for (from, to) in [
        ("AVENIDA", " "),
        ("AV.", " "),
        ("AV ", " "),
        ("RUA", " "),
        ("R.", " "),
        ("RODOVIA", " "),
        ("ROD.", " "),
        ("ROD ", " "),
        ("ALAMEDA", " "),
        ("AL.", " "),
        ("TRAVESSA", " "),
        ("TV.", " "),
        ("ESTRADA", " "),
        ("EST.", " "),
    ] {
        folded = folded.replace(from, to);
    }
    let cleaned: String = folded
        .chars()
        .map(|character| {
            if character.is_ascii_alphanumeric() {
                character
            } else {
                ' '
            }
        })
        .collect();
    cleaned.split_whitespace().collect::<Vec<_>>().join(" ")
}

pub(crate) fn normalize_number(value: &str) -> String {
    let trimmed = value.trim().to_uppercase();
    if trimmed == "SN" || trimmed == "S/N" {
        return "SN".to_string();
    }
    let digits: String = trimmed
        .chars()
        .take_while(|character| character.is_ascii_digit() || *character == '-')
        .collect();
    if digits.is_empty() {
        trimmed
    } else {
        digits
    }
}

pub(crate) fn split_address(raw: &str) -> Option<(String, String)> {
    let trimmed = raw.trim();
    let (street, number) = trimmed.rsplit_once(',')?;
    let number = number.trim();
    if street.trim().is_empty() || number.is_empty() {
        return None;
    }
    Some((normalize_street(street), normalize_number(number)))
}

fn brand_match(anp: Option<&str>, osm: &OsmReference) -> bool {
    brand_match_text(
        anp,
        osm.tags.get("brand").map(String::as_str),
        osm.tags.get("operator").map(String::as_str),
    )
}

/// Shared brand comparison for callers that carry OSM tags in another shape.
pub(crate) fn brand_match_text(
    anp: Option<&str>,
    osm_brand: Option<&str>,
    osm_operator: Option<&str>,
) -> bool {
    let left = fold(anp.unwrap_or_default());
    let right = fold(osm_brand.or(osm_operator).unwrap_or_default());
    if left.is_empty() || right.is_empty() {
        return true;
    }
    left.contains(&right) || right.contains(&left) || (left == "BR" && right == "VIBRA")
}

pub(crate) fn haversine_m(lat_a: f64, lon_a: f64, lat_b: f64, lon_b: f64) -> f64 {
    let phi_a = lat_a.to_radians();
    let phi_b = lat_b.to_radians();
    let delta_phi = (lat_b - lat_a).to_radians();
    let delta_lambda = (lon_b - lon_a).to_radians();
    let a = (delta_phi / 2.0).sin().powi(2)
        + phi_a.cos() * phi_b.cos() * (delta_lambda / 2.0).sin().powi(2);
    2.0 * EARTH_RADIUS_M * a.sqrt().asin()
}

fn parse_osm(raw: &[u8]) -> Result<Vec<OsmReference>, ReviewError> {
    let mut rows = Vec::new();
    for line in raw.split(|byte| *byte == b'\n') {
        if line.iter().all(|byte| byte.is_ascii_whitespace()) {
            continue;
        }
        rows.push(serde_json::from_slice::<OsmReference>(line).map_err(json_error)?);
    }
    Ok(rows)
}

fn parse_registry(raw: &[u8]) -> Result<HashMap<String, RegistryRow>, ReviewError> {
    let mut rows = HashMap::new();
    for line in raw.split(|byte| *byte == b'\n') {
        if line.iter().all(|byte| byte.is_ascii_whitespace()) {
            continue;
        }
        let row = serde_json::from_slice::<RegistryRow>(line).map_err(json_error)?;
        if let Ok(cnpj) = normalize_cnpj(&row.source_key) {
            rows.insert(cnpj, row);
        }
    }
    Ok(rows)
}

/// Match official ANP records to independently prepared OSM nodes.
pub fn review(
    api: &[ApiRecord],
    osm: &[OsmReference],
    registry: &HashMap<String, RegistryRow>,
    max_distance_m: f64,
    reviewer: &str,
) -> (Vec<ReviewRow>, Vec<RejectRow>, Counts) {
    let mut index: HashMap<(String, String), Vec<&OsmReference>> = HashMap::new();
    for reference in osm {
        if reference.kind != "FUEL_STATION" {
            continue;
        }
        let (Some(lat), Some(lon)) = (&reference.latitude_deg, &reference.longitude_deg) else {
            continue;
        };
        if lat.parse::<f64>().is_err() || lon.parse::<f64>().is_err() {
            continue;
        }
        let (Some(street), Some(number)) = (
            reference.tags.get("addr:street"),
            reference.tags.get("addr:housenumber"),
        ) else {
            continue;
        };
        index
            .entry((normalize_street(street), normalize_number(number)))
            .or_default()
            .push(reference);
    }

    let mut counts = Counts::default();
    let mut reviews = Vec::new();
    let mut rejects = Vec::new();
    let mut seen = std::collections::HashSet::new();
    for record in api {
        counts.input += 1;
        let Ok(cnpj) = normalize_cnpj(&record.cnpj) else {
            rejects.push(reject(None, "invalid_cnpj", "CNPJ is not valid"));
            continue;
        };
        if !seen.insert(cnpj.clone()) {
            counts.duplicates += 1;
            continue;
        }
        let Some(registry_row) = registry.get(&cnpj) else {
            rejects.push(reject(
                Some(cnpj),
                "missing_registry",
                "no prepared registry row",
            ));
            continue;
        };
        if !record.uf.trim().eq_ignore_ascii_case(&registry_row.uf)
            || fold(&record.municipio) != fold(&registry_row.municipality_name_raw)
        {
            rejects.push(reject(
                Some(cnpj),
                "registry_context_mismatch",
                "municipality/UF differ",
            ));
            continue;
        }
        let (Some(latitude), Some(longitude)) = (
            record
                .latitude
                .as_deref()
                .and_then(|raw| raw.trim().parse::<f64>().ok()),
            record
                .longitude
                .as_deref()
                .and_then(|raw| raw.trim().parse::<f64>().ok()),
        ) else {
            rejects.push(reject(
                Some(cnpj),
                "missing_coordinates",
                "ANP record has no point",
            ));
            continue;
        };
        if !(-90.0..=90.0).contains(&latitude) || !(-180.0..=180.0).contains(&longitude) {
            rejects.push(reject(
                Some(cnpj),
                "invalid_coordinates",
                "ANP point out of range",
            ));
            continue;
        }
        let Some((street, number)) = split_address(&record.endereco) else {
            rejects.push(reject(
                Some(cnpj),
                "missing_number",
                "ANP address has no house number",
            ));
            continue;
        };
        if street != normalize_street(&registry_row.address_normalized.street)
            || number != normalize_number(&registry_row.address_normalized.number)
        {
            rejects.push(reject(
                Some(cnpj),
                "registry_address_mismatch",
                "ANP API and prepared registry addresses differ",
            ));
            continue;
        }
        let Some(candidates) = index.get(&(street.clone(), number.clone())) else {
            rejects.push(reject(
                Some(cnpj),
                "missing_osm",
                "no OSM fuel station at this address",
            ));
            continue;
        };
        if candidates.len() != 1 {
            rejects.push(reject(
                Some(cnpj),
                "ambiguous_osm",
                "more than one OSM fuel station at this address",
            ));
            continue;
        }
        let candidate = candidates[0];
        let osm_lat = candidate
            .latitude_deg
            .as_deref()
            .and_then(|raw| raw.trim().parse::<f64>().ok())
            .expect("indexed coordinate");
        let osm_lon = candidate
            .longitude_deg
            .as_deref()
            .and_then(|raw| raw.trim().parse::<f64>().ok())
            .expect("indexed coordinate");
        let distance = haversine_m(latitude, longitude, osm_lat, osm_lon);
        if distance > max_distance_m {
            rejects.push(reject(
                Some(cnpj),
                "distance_exceeded",
                "OSM point is farther than the configured evidence radius",
            ));
            continue;
        }
        let anp_brand = record.distribuidora.as_deref();
        let brand_match = brand_match(anp_brand, candidate);
        let evidence = format!(
            "anp-registry-api:{};{}:{}",
            record.obtained, candidate.osm_type, candidate.osm_id
        );
        reviews.push(ReviewRow {
            schema_version: REVIEW_SCHEMA_VERSION,
            cnpj,
            station_name: record.business.clone(),
            municipality: record.municipio.clone(),
            state: record.uf.to_uppercase(),
            latitude,
            longitude,
            method: "ANP_REGISTRY_API+OSM_ADDRESS_NAME",
            match_distance_m: (distance * 10.0).round() / 10.0,
            brand_match,
            reviewer: reviewer.to_string(),
            evidence,
            anp: AnpEvidence {
                address: record.endereco.clone(),
                latitude: record.latitude.clone().unwrap_or_default(),
                longitude: record.longitude.clone().unwrap_or_default(),
                observed_at: record.obtained.clone(),
                origin: record.origin.clone(),
                validation: record.validacao.clone(),
            },
            osm: OsmEvidence {
                osm_type: candidate.osm_type.clone(),
                osm_id: candidate.osm_id,
                name: candidate.name.clone(),
                brand: candidate
                    .tags
                    .get("brand")
                    .or_else(|| candidate.tags.get("operator"))
                    .cloned(),
                street: candidate
                    .tags
                    .get("addr:street")
                    .cloned()
                    .unwrap_or_default(),
                housenumber: candidate
                    .tags
                    .get("addr:housenumber")
                    .cloned()
                    .unwrap_or_default(),
                latitude_deg: candidate.latitude_deg.clone().unwrap_or_default(),
                longitude_deg: candidate.longitude_deg.clone().unwrap_or_default(),
                license: candidate.license.clone(),
                attribution: candidate.attribution.clone(),
            },
        });
    }
    counts.reviewed = reviews.len() as u64;
    counts.rejected = rejects.len() as u64;
    (reviews, rejects, counts)
}

fn reject(cnpj: Option<String>, reason: &'static str, detail: &str) -> RejectRow {
    RejectRow {
        schema_version: "station-location-reject-v1",
        cnpj,
        reason,
        detail: detail.to_string(),
    }
}

pub fn run_review(
    api_raw: &[u8],
    osm_raw: &[u8],
    registry_raw: &[u8],
    max_distance_m: f64,
    reviewer: &str,
) -> Result<ReviewRun, ReviewError> {
    if !(1.0..=200.0).contains(&max_distance_m) {
        return Err(ReviewError::Profile("max distance must be 1..200 m".into()));
    }
    if reviewer.trim().is_empty() {
        return Err(ReviewError::Profile("reviewer is required".into()));
    }
    let envelope: ApiEnvelope = serde_json::from_slice(api_raw).map_err(json_error)?;
    let osm = parse_osm(osm_raw)?;
    let registry = parse_registry(registry_raw)?;
    let (reviews, rejects, counts) =
        review(&envelope.data, &osm, &registry, max_distance_m, reviewer);
    Ok(ReviewRun {
        counts,
        reviews,
        rejects,
        source_sha256: sha256_hex(api_raw),
        osm_sha256: sha256_hex(osm_raw),
        registry_sha256: sha256_hex(registry_raw),
    })
}

fn ndjson<T: Serialize>(values: &[T]) -> Result<Vec<u8>, ReviewError> {
    let mut out = Vec::new();
    for value in values {
        serde_json::to_writer(&mut out, value).map_err(json_error)?;
        out.push(b'\n');
    }
    Ok(out)
}

fn write_file(path: &Path, bytes: &[u8]) -> Result<ManifestFile, ReviewError> {
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

pub fn write_run(
    output: &Path,
    run: &ReviewRun,
    max_distance_m: f64,
    reviewer: &str,
) -> Result<(), ReviewError> {
    let parent = output.parent().unwrap_or_else(|| Path::new("."));
    fs::create_dir_all(parent).map_err(io_error)?;
    let temp = output.with_extension("tmp-review");
    if temp.exists() {
        fs::remove_dir_all(&temp).map_err(io_error)?;
    }
    fs::create_dir(&temp).map_err(io_error)?;
    let reviews = ndjson(&run.reviews)?;
    let rejects = ndjson(&run.rejects)?;
    let mut files = BTreeMap::new();
    files.insert(
        "reviews.ndjson",
        write_file(&temp.join("reviews.ndjson"), &reviews)?,
    );
    files.insert(
        "rejects.ndjson",
        write_file(&temp.join("rejects.ndjson"), &rejects)?,
    );
    let manifest = Manifest {
        format: REVIEW_FORMAT,
        parser_version: REVIEW_PARSER_VERSION,
        reviewer: reviewer.to_string(),
        max_distance_m,
        source_sha256: run.source_sha256.clone(),
        osm_sha256: run.osm_sha256.clone(),
        registry_sha256: run.registry_sha256.clone(),
        counts: run.counts.clone(),
        files,
    };
    let manifest = serde_json::to_vec_pretty(&manifest).map_err(json_error)?;
    write_file(&temp.join("manifest.json"), &manifest)?;
    if output.exists() {
        let backup = output.with_extension("bak-review");
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

#[cfg(test)]
mod tests {
    use super::*;

    fn api(cnpj: &str, address: &str, lat: &str, lon: &str) -> ApiRecord {
        ApiRecord {
            cnpj: cnpj.into(),
            business: "POSTO TESTE LTDA".into(),
            endereco: address.into(),
            municipio: "CURITIBA".into(),
            uf: "PR".into(),
            latitude: Some(lat.into()),
            longitude: Some(lon.into()),
            validacao: "Posição Geográfica não validada".into(),
            origin: "Preposto da empresa".into(),
            obtained: "22/08/2024".into(),
            distribuidora: Some("IPIRANGA".into()),
        }
    }

    fn osm(street: &str, number: &str, lat: &str, lon: &str) -> OsmReference {
        OsmReference {
            key: "osm:node:1".into(),
            osm_type: "node".into(),
            osm_id: 1,
            kind: "FUEL_STATION".into(),
            name: Some("Posto Teste".into()),
            latitude_deg: Some(lat.into()),
            longitude_deg: Some(lon.into()),
            quality: "osm-mapped".into(),
            tags: BTreeMap::from([
                ("addr:street".into(), street.into()),
                ("addr:housenumber".into(), number.into()),
                ("brand".into(), "Ipiranga".into()),
            ]),
            license: "ODbL-1.0".into(),
            attribution: "© OpenStreetMap contributors — ODbL 1.0".into(),
        }
    }

    fn registry(cnpj: &str, street: &str, number: &str) -> HashMap<String, RegistryRow> {
        HashMap::from([(
            cnpj.into(),
            RegistryRow {
                source_key: cnpj.into(),
                business_name_raw: "POSTO TESTE LTDA".into(),
                address_raw: format!("{street}, {number}"),
                address_normalized: RegistryAddress {
                    street: street.into(),
                    number: number.into(),
                },
                uf: "PR".into(),
                municipality_name_raw: "CURITIBA".into(),
            },
        )])
    }

    #[test]
    fn matches_exact_address_and_retains_independent_evidence() {
        let records = vec![api("01122233300000", "RUA TESTE, 100", "-25.4", "-49.2")];
        let references = vec![osm("RUA TESTE", "100", "-25.40001", "-49.20001")];
        let (reviews, rejects, counts) = review(
            &records,
            &references,
            &registry("01122233300000", "RUA TESTE", "100"),
            50.0,
            "tester",
        );
        assert_eq!(counts.input, 1);
        assert_eq!(counts.reviewed, 1);
        assert!(rejects.is_empty());
        assert_eq!(reviews[0].method, "ANP_REGISTRY_API+OSM_ADDRESS_NAME");
        assert!(reviews[0].brand_match);
        assert!(reviews[0].match_distance_m < 5.0);
    }

    #[test]
    fn refuses_proximity_without_the_same_address() {
        let records = vec![api("01122233300000", "RUA TESTE, 100", "-25.4", "-49.2")];
        let references = vec![osm("RUA OUTRA", "100", "-25.40001", "-49.20001")];
        let (reviews, rejects, _) = review(
            &records,
            &references,
            &registry("01122233300000", "RUA TESTE", "100"),
            50.0,
            "tester",
        );
        assert!(reviews.is_empty());
        assert_eq!(rejects[0].reason, "missing_osm");
    }

    #[test]
    fn refuses_ambiguous_or_distant_osm_matches() {
        let records = vec![api("01122233300000", "RUA TESTE, 100", "-25.4", "-49.2")];
        let duplicates = vec![
            osm("RUA TESTE", "100", "-25.40001", "-49.20001"),
            osm("RUA TESTE", "100", "-25.40002", "-49.20002"),
        ];
        let (reviews, rejects, _) = review(
            &records,
            &duplicates,
            &registry("01122233300000", "RUA TESTE", "100"),
            50.0,
            "tester",
        );
        assert!(reviews.is_empty());
        assert_eq!(rejects[0].reason, "ambiguous_osm");

        let far = vec![osm("RUA TESTE", "100", "-25.401", "-49.201")];
        let (reviews, rejects, _) = review(
            &records,
            &far,
            &registry("01122233300000", "RUA TESTE", "100"),
            5.0,
            "tester",
        );
        assert!(reviews.is_empty());
        assert_eq!(rejects[0].reason, "distance_exceeded");
    }

    #[test]
    fn registry_mismatch_is_not_reviewed() {
        let records = vec![api("01122233300000", "RUA TESTE, 100", "-25.4", "-49.2")];
        let references = vec![osm("RUA TESTE", "100", "-25.40001", "-49.20001")];
        let (reviews, rejects, _) = review(
            &records,
            &references,
            &registry("01122233300000", "RUA OUTRA", "100"),
            50.0,
            "tester",
        );
        assert!(reviews.is_empty());
        assert_eq!(rejects[0].reason, "registry_address_mismatch");
    }

    #[test]
    fn short_numeric_cnpj_is_not_reviewed() {
        let records = vec![api("2759173000113", "RUA TESTE, 100", "-25.4", "-49.2")];
        let (reviews, rejects, _) = review(
            &records,
            &[osm("RUA TESTE", "100", "-25.40001", "-49.20001")],
            &registry("02759173000113", "RUA TESTE", "100"),
            50.0,
            "tester",
        );
        assert!(reviews.is_empty());
        assert_eq!(rejects[0].reason, "invalid_cnpj");
    }
}

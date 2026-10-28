//! MC-A3 official point validation (B-BR-MAP-15). Pure local computation:
//! no network, database or clock. Rejections remove a point; they never
//! upgrade a tier or invent a replacement.

use serde::{Deserialize, Serialize};
use sha2::{Digest, Sha256};
use std::collections::HashMap;
use std::fs;
use std::io::Write;
use std::path::{Path, PathBuf};

pub const VALIDATOR_VERSION: &str = "anp-point-validation-v1";
pub const BUFFER_M: f64 = 500.0;
pub const CANDIDATE_MAX_M: f64 = 2000.0;
pub const SHARED_CLUSTER_MIN: usize = 5;

#[derive(Debug, Deserialize)]
struct RegistryRow {
    cnpj: String,
    state: String,
    municipality: String,
    latitude: Option<f64>,
    longitude: Option<f64>,
    srid: String,
}

#[derive(Debug, Deserialize)]
struct MunicipalityAsset {
    n: String,
    u: String,
}

#[derive(Debug, Deserialize)]
struct GeoFeature {
    properties: GeoProperties,
    geometry: GeoGeometry,
}

#[derive(Debug, Deserialize)]
struct GeoProperties {
    codarea: String,
}

#[derive(Debug, Deserialize)]
struct GeoGeometry {
    #[serde(rename = "type")]
    kind: String,
    coordinates: serde_json::Value,
}

#[derive(Debug, Deserialize)]
struct GeoCollection {
    features: Vec<GeoFeature>,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
pub struct Validation {
    pub schema_version: String,
    pub cnpj: String,
    pub state: String,
    pub municipality: String,
    pub municipality_code: String,
    pub latitude: Option<f64>,
    pub longitude: Option<f64>,
    pub status: String,
    pub reason: String,
    pub candidate_distance_m: Option<f64>,
}

#[derive(Debug, Clone, Serialize, PartialEq)]
pub struct ReasonCount {
    pub reason: String,
    pub count: usize,
}

#[derive(Debug, Clone, Serialize, PartialEq)]
pub struct Manifest {
    pub schema_version: String,
    pub validator_version: String,
    pub registry_sha256: String,
    pub malha_sha256: String,
    pub municipalities_sha256: String,
    pub counts: Counts,
    pub reasons: Vec<ReasonCount>,
    pub output_file: String,
    pub output_sha256: String,
}

#[derive(Debug, Clone, Serialize, PartialEq)]
pub struct Counts {
    pub input: usize,
    pub valid: usize,
    pub no_point: usize,
    pub rejected: usize,
}

pub struct Run {
    pub manifest: Manifest,
    pub output_path: PathBuf,
}

fn sha256_hex(raw: &[u8]) -> String {
    let mut hasher = Sha256::new();
    hasher.update(raw);
    hasher
        .finalize()
        .iter()
        .map(|b| format!("{b:02x}"))
        .collect()
}

fn fold(value: &str) -> String {
    value
        .trim()
        .to_uppercase()
        .chars()
        .map(|c| match c {
            'Á' | 'À' | 'Â' | 'Ã' | 'Ä' => 'A',
            'É' | 'È' | 'Ê' | 'Ë' => 'E',
            'Í' | 'Ì' | 'Î' | 'Ï' => 'I',
            'Ó' | 'Ò' | 'Ô' | 'Õ' | 'Ö' => 'O',
            'Ú' | 'Ù' | 'Û' | 'Ü' => 'U',
            'Ç' => 'C',
            other => other,
        })
        .collect()
}

type Ring = Vec<(f64, f64)>;

fn parse_rings(value: &serde_json::Value) -> Vec<Ring> {
    let mut rings = Vec::new();
    let Some(outer) = value.as_array() else {
        return rings;
    };
    for ring in outer {
        let Some(points) = ring.as_array() else {
            continue;
        };
        let mut parsed = Vec::with_capacity(points.len());
        for point in points {
            let Some(pair) = point.as_array() else {
                continue;
            };
            if pair.len() >= 2 {
                if let (Some(lon), Some(lat)) = (pair[0].as_f64(), pair[1].as_f64()) {
                    parsed.push((lon, lat));
                }
            }
        }
        if parsed.len() >= 3 {
            rings.push(parsed);
        }
    }
    rings
}

fn rings_for(geometry: &GeoGeometry) -> Vec<Ring> {
    match geometry.kind.as_str() {
        "Polygon" => parse_rings(&geometry.coordinates),
        "MultiPolygon" => {
            let mut out = Vec::new();
            if let Some(polys) = geometry.coordinates.as_array() {
                for poly in polys {
                    out.extend(parse_rings(poly));
                }
            }
            out
        }
        _ => Vec::new(),
    }
}

fn point_in_ring(lon: f64, lat: f64, ring: &Ring) -> bool {
    let mut inside = false;
    let mut j = ring.len() - 1;
    for i in 0..ring.len() {
        let (xi, yi) = ring[i];
        let (xj, yj) = ring[j];
        if ((yi > lat) != (yj > lat)) && (lon < (xj - xi) * (lat - yi) / (yj - yi) + xi) {
            inside = !inside;
        }
        j = i;
    }
    inside
}

fn point_in_polygon(lon: f64, lat: f64, rings: &[Ring]) -> bool {
    if rings.is_empty() {
        return false;
    }
    if !point_in_ring(lon, lat, &rings[0]) {
        return false;
    }
    for hole in rings.iter().skip(1) {
        if point_in_ring(lon, lat, hole) {
            return false;
        }
    }
    true
}

fn meters_per_degree(lat: f64) -> (f64, f64) {
    let lat_m = 110_540.0;
    let lon_m = 111_320.0 * lat.to_radians().cos().abs().max(0.01);
    (lat_m, lon_m)
}

fn point_segment_distance_m(lon: f64, lat: f64, a: (f64, f64), b: (f64, f64)) -> f64 {
    let (lat_m, lon_m) = meters_per_degree(lat);
    let px = (lon - a.0) * lon_m;
    let py = (lat - a.1) * lat_m;
    let bx = (b.0 - a.0) * lon_m;
    let by = (b.1 - a.1) * lat_m;
    let len2 = bx * bx + by * by;
    let t = if len2 == 0.0 {
        0.0
    } else {
        ((px * bx + py * by) / len2).clamp(0.0, 1.0)
    };
    let dx = px - t * bx;
    let dy = py - t * by;
    (dx * dx + dy * dy).sqrt()
}

fn distance_to_polygon_m(lon: f64, lat: f64, rings: &[Ring]) -> f64 {
    if point_in_polygon(lon, lat, rings) {
        return 0.0;
    }
    let mut best = f64::INFINITY;
    for ring in rings {
        for i in 0..ring.len() {
            let a = ring[i];
            let b = ring[(i + 1) % ring.len()];
            best = best.min(point_segment_distance_m(lon, lat, a, b));
        }
    }
    best
}

fn haversine_m(lat1: f64, lon1: f64, lat2: f64, lon2: f64) -> f64 {
    const R: f64 = 6_371_000.0;
    let dlat = (lat2 - lat1).to_radians();
    let dlon = (lon2 - lon1).to_radians();
    let a = (dlat / 2.0).sin().powi(2)
        + lat1.to_radians().cos() * lat2.to_radians().cos() * (dlon / 2.0).sin().powi(2);
    2.0 * R * a.sqrt().asin()
}

fn in_brazil(lat: f64, lon: f64) -> bool {
    (-33.75..=5.27).contains(&lat) && (-73.99..=-34.79).contains(&lon)
}

fn sentinel(lat: f64, lon: f64) -> bool {
    (lat == 0.0 && lon == 0.0) || !lat.is_finite() || !lon.is_finite()
}

fn srid_ok(srid: &str) -> bool {
    matches!(srid.trim(), "" | "4674" | "4326")
}

/// Validate one registry NDJSON file against the official malha and the
/// municipality name table. The optional candidates file maps CNPJ to a
/// geocoded point for the distance check.
pub fn run(
    registry: &Path,
    malha: &Path,
    municipalities: &Path,
    candidates: Option<&Path>,
    output: &Path,
) -> Result<Run, String> {
    let registry_raw = fs::read(registry).map_err(|e| format!("read registry: {e}"))?;
    let malha_raw = fs::read(malha).map_err(|e| format!("read malha: {e}"))?;
    let municipalities_raw =
        fs::read(municipalities).map_err(|e| format!("read municipalities: {e}"))?;
    let assets: HashMap<String, MunicipalityAsset> = serde_json::from_slice(&municipalities_raw)
        .map_err(|e| format!("parse municipalities: {e}"))?;
    let mut by_name: HashMap<(String, String), String> = HashMap::new();
    for (code, item) in &assets {
        by_name.insert((item.u.clone(), fold(&item.n)), code.clone());
    }
    let collection: GeoCollection =
        serde_json::from_slice(&malha_raw).map_err(|e| format!("parse malha: {e}"))?;
    let mut polygons: HashMap<String, Vec<Ring>> = HashMap::new();
    for feature in collection.features {
        polygons.insert(feature.properties.codarea, rings_for(&feature.geometry));
    }
    let mut candidate_points: HashMap<String, (f64, f64)> = HashMap::new();
    if let Some(path) = candidates {
        let raw = fs::read_to_string(path).map_err(|e| format!("read candidates: {e}"))?;
        for line in raw.lines().filter(|line| !line.trim().is_empty()) {
            let value: serde_json::Value =
                serde_json::from_str(line).map_err(|e| format!("parse candidate: {e}"))?;
            let cnpj = value
                .get("cnpj")
                .and_then(|v| v.as_str())
                .unwrap_or("")
                .to_string();
            let lat = value.get("latitude").and_then(|v| v.as_f64());
            let lon = value.get("longitude").and_then(|v| v.as_f64());
            if !cnpj.is_empty() {
                if let (Some(lat), Some(lon)) = (lat, lon) {
                    candidate_points.insert(cnpj, (lat, lon));
                }
            }
        }
    }
    let mut rows = Vec::new();
    for line in String::from_utf8_lossy(&registry_raw)
        .lines()
        .filter(|l| !l.trim().is_empty())
    {
        let row: RegistryRow = serde_json::from_str(line).map_err(|e| format!("parse row: {e}"))?;
        rows.push(row);
    }
    let mut shared: HashMap<(i64, i64), usize> = HashMap::new();
    for row in &rows {
        if let (Some(lat), Some(lon)) = (row.latitude, row.longitude) {
            if lat.is_finite() && lon.is_finite() {
                *shared
                    .entry(((lat * 1e6).round() as i64, (lon * 1e6).round() as i64))
                    .or_insert(0) += 1;
            }
        }
    }
    let mut validations = Vec::with_capacity(rows.len());
    for row in rows {
        let mut municipality_code = String::new();
        let key = (row.state.clone(), fold(&row.municipality));
        if let Some(code) = by_name.get(&key) {
            municipality_code = code.clone();
        }
        let mut status = "valid".to_string();
        let mut reason = String::new();
        let mut candidate_distance = None;
        match (row.latitude, row.longitude) {
            (None, _) | (_, None) => {
                status = "no_point".to_string();
                reason = "missing_point".to_string();
            }
            (Some(lat), Some(lon)) => {
                let shared_count = shared
                    .get(&((lat * 1e6).round() as i64, (lon * 1e6).round() as i64))
                    .copied()
                    .unwrap_or(0);
                if sentinel(lat, lon) {
                    status = "rejected".to_string();
                    reason = "zero_or_sentinel".to_string();
                } else if !srid_ok(&row.srid) {
                    status = "rejected".to_string();
                    reason = "srid_invalid".to_string();
                } else if !in_brazil(lat, lon) {
                    if in_brazil(lon, lat) {
                        status = "rejected".to_string();
                        reason = "lat_lon_swapped".to_string();
                    } else {
                        status = "rejected".to_string();
                        reason = "outside_brazil".to_string();
                    }
                } else if municipality_code.is_empty() {
                    status = "rejected".to_string();
                    reason = "municipality_unknown".to_string();
                } else if shared_count >= SHARED_CLUSTER_MIN {
                    status = "rejected".to_string();
                    reason = "shared_coordinate_cluster".to_string();
                } else {
                    let rings = polygons.get(&municipality_code);
                    match rings {
                        None => {
                            status = "rejected".to_string();
                            reason = "municipality_polygon_missing".to_string();
                        }
                        Some(rings) => {
                            let distance = distance_to_polygon_m(lon, lat, rings);
                            if distance > BUFFER_M {
                                status = "rejected".to_string();
                                reason = "outside_municipality".to_string();
                            }
                        }
                    }
                    if status == "valid" {
                        if let Some((clat, clon)) = candidate_points.get(&row.cnpj) {
                            let distance = haversine_m(lat, lon, *clat, *clon);
                            candidate_distance = Some(distance);
                            if distance > CANDIDATE_MAX_M {
                                status = "rejected".to_string();
                                reason = "address_candidate_too_far".to_string();
                            }
                        }
                    }
                }
            }
        }
        validations.push(Validation {
            schema_version: VALIDATOR_VERSION.to_string(),
            cnpj: row.cnpj,
            state: row.state,
            municipality: row.municipality,
            municipality_code,
            latitude: row.latitude,
            longitude: row.longitude,
            status,
            reason,
            candidate_distance_m: candidate_distance,
        });
    }
    validations.sort_by(|a, b| {
        (&a.state, &a.cnpj, &a.municipality).cmp(&(&b.state, &b.cnpj, &b.municipality))
    });
    let mut counts = Counts {
        input: validations.len(),
        valid: 0,
        no_point: 0,
        rejected: 0,
    };
    let mut reasons: HashMap<String, usize> = HashMap::new();
    for row in &validations {
        match row.status.as_str() {
            "valid" => counts.valid += 1,
            "no_point" => counts.no_point += 1,
            _ => counts.rejected += 1,
        }
        if !row.reason.is_empty() {
            *reasons.entry(row.reason.clone()).or_insert(0) += 1;
        }
    }
    if counts.input != counts.valid + counts.no_point + counts.rejected {
        return Err("counts do not reconcile".to_string());
    }
    let mut reasons: Vec<ReasonCount> = reasons
        .into_iter()
        .map(|(reason, count)| ReasonCount { reason, count })
        .collect();
    reasons.sort_by(|a, b| a.reason.cmp(&b.reason));
    let mut out = Vec::new();
    for row in &validations {
        serde_json::to_writer(&mut out, row).map_err(|e| format!("serialize: {e}"))?;
        out.push(b'\n');
    }
    fs::create_dir_all(output).map_err(|e| format!("create output: {e}"))?;
    let output_path = output.join("anp-validation.ndjson");
    write_atomic(&output_path, &out)?;
    let manifest = Manifest {
        schema_version: VALIDATOR_VERSION.to_string(),
        validator_version: VALIDATOR_VERSION.to_string(),
        registry_sha256: sha256_hex(&registry_raw),
        malha_sha256: sha256_hex(&malha_raw),
        municipalities_sha256: sha256_hex(&municipalities_raw),
        counts,
        reasons,
        output_file: "anp-validation.ndjson".to_string(),
        output_sha256: sha256_hex(&out),
    };
    let manifest_raw =
        serde_json::to_vec_pretty(&manifest).map_err(|e| format!("manifest: {e}"))?;
    write_atomic(&output.join("manifest.json"), &manifest_raw)?;
    Ok(Run {
        manifest,
        output_path,
    })
}

fn write_atomic(path: &Path, raw: &[u8]) -> Result<(), String> {
    let tmp = path.with_extension("tmp");
    let mut file = fs::File::create(&tmp).map_err(|e| format!("create {}: {e}", tmp.display()))?;
    file.write_all(raw)
        .map_err(|e| format!("write {}: {e}", tmp.display()))?;
    file.sync_all()
        .map_err(|e| format!("sync {}: {e}", tmp.display()))?;
    fs::rename(&tmp, path).map_err(|e| format!("rename {}: {e}", path.display()))
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::time::{SystemTime, UNIX_EPOCH};

    fn temp_dir(tag: &str) -> PathBuf {
        let nanos = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .unwrap()
            .as_nanos();
        let dir = std::env::temp_dir().join(format!("anp-validation-{tag}-{nanos}"));
        fs::create_dir_all(&dir).unwrap();
        dir
    }

    fn write_inputs(dir: &Path) -> (PathBuf, PathBuf, PathBuf, PathBuf) {
        let malha = dir.join("malha.json");
        fs::write(&malha, r#"{"type":"FeatureCollection","features":[
          {"type":"Feature","properties":{"codarea":"5103403"},"geometry":{"type":"Polygon","coordinates":[[[-56.2,-12.7],[-55.9,-12.7],[-55.9,-12.4],[-56.2,-12.4],[-56.2,-12.7]]]}},
          {"type":"Feature","properties":{"codarea":"3550308"},"geometry":{"type":"Polygon","coordinates":[[[-46.9,-23.7],[-46.5,-23.7],[-46.5,-23.4],[-46.9,-23.4],[-46.9,-23.7]]]}}
        ]}"#).unwrap();
        let municipalities = dir.join("municipalities.json");
        fs::write(
            &municipalities,
            r#"{"5103403":{"n":"CUIABÁ","u":"MT"},"3550308":{"n":"SÃO PAULO","u":"SP"},"9999999":{"n":"SEM POLIGONO","u":"MT"}}"#,
        )
        .unwrap();
        let registry = dir.join("registry.ndjson");
        let mut lines = Vec::new();
        let mut push = |cnpj: &str,
                        uf: &str,
                        muni: &str,
                        lat: Option<f64>,
                        lon: Option<f64>,
                        srid: &str| {
            lines.push(serde_json::json!({"cnpj":cnpj,"state":uf,"municipality":muni,"latitude":lat,"longitude":lon,"srid":srid}).to_string());
        };
        push("1", "MT", "CUIABÁ", Some(-12.60), Some(-56.10), "4674");
        push("2", "MT", "CUIABÁ", Some(10.0), Some(-50.0), "4674");
        push("8", "MT", "CUIABÁ", Some(-12.8), Some(-56.3), "4674");
        push("3", "MT", "CUIABÁ", Some(-56.05), Some(-12.55), "4674");
        push("4", "MT", "CUIABÁ", Some(0.0), Some(0.0), "4674");
        push(
            "5",
            "MT",
            "DESCONHECIDO",
            Some(-12.55),
            Some(-56.05),
            "4674",
        );
        for i in 0..5 {
            push(
                &format!("6{i}"),
                "MT",
                "CUIABÁ",
                Some(-12.50),
                Some(-56.00),
                "4674",
            );
        }
        push("7", "MT", "CUIABÁ", None, None, "4674");
        push("9", "MT", "CUIABÁ", Some(-12.55), Some(-56.05), "4674");
        push("10", "MT", "CUIABÁ", Some(-12.55), Some(-56.05), "9999");
        push(
            "11",
            "MT",
            "SEM POLIGONO",
            Some(-12.55),
            Some(-56.05),
            "4674",
        );
        fs::write(&registry, lines.join("\n") + "\n").unwrap();
        let candidates = dir.join("candidates.ndjson");
        fs::write(
            &candidates,
            "{\"cnpj\":\"1\",\"latitude\":-12.601,\"longitude\":-56.101}\n{\"cnpj\":\"9\",\"latitude\":-22.0,\"longitude\":-43.0}\n",
        )
        .unwrap();
        (registry, malha, municipalities, candidates)
    }

    #[test]
    fn validates_and_reports_each_reason() {
        let dir = temp_dir("reasons");
        let (registry, malha, municipalities, candidates) = write_inputs(&dir);
        let run = run(
            &registry,
            &malha,
            &municipalities,
            Some(&candidates),
            &dir.join("out"),
        )
        .unwrap();
        assert_eq!(run.manifest.counts.input, 15);
        assert_eq!(run.manifest.counts.valid, 1);
        assert_eq!(run.manifest.counts.no_point, 1);
        let reasons: HashMap<_, _> = run
            .manifest
            .reasons
            .iter()
            .map(|r| (r.reason.as_str(), r.count))
            .collect();
        assert_eq!(reasons.get("outside_brazil"), Some(&1));
        assert_eq!(reasons.get("outside_municipality"), Some(&1));
        assert_eq!(reasons.get("lat_lon_swapped"), Some(&1));
        assert_eq!(reasons.get("zero_or_sentinel"), Some(&1));
        assert_eq!(reasons.get("municipality_unknown"), Some(&1));
        assert_eq!(reasons.get("shared_coordinate_cluster"), Some(&5));
        assert_eq!(reasons.get("srid_invalid"), Some(&1));
        assert_eq!(reasons.get("municipality_polygon_missing"), Some(&1));
        assert_eq!(reasons.get("address_candidate_too_far"), Some(&1));
    }

    #[test]
    fn rerun_is_byte_identical_and_never_emits_a_tier() {
        let dir = temp_dir("rerun");
        let (registry, malha, municipalities, candidates) = write_inputs(&dir);
        let first = run(
            &registry,
            &malha,
            &municipalities,
            Some(&candidates),
            &dir.join("a"),
        )
        .unwrap();
        let second = run(
            &registry,
            &malha,
            &municipalities,
            Some(&candidates),
            &dir.join("b"),
        )
        .unwrap();
        assert_eq!(
            fs::read(&first.output_path).unwrap(),
            fs::read(&second.output_path).unwrap()
        );
        assert_eq!(first.manifest.output_sha256, second.manifest.output_sha256);
        let text = fs::read_to_string(&first.output_path).unwrap();
        assert!(!text.contains("\"quality\""));
        assert!(!text.contains("\"tier\""));
        fs::remove_dir_all(dir).unwrap();
    }
}

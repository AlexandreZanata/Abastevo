//! File-only Phase 3 coverage and apply preparation.
//!
//! Reads the A2/B2 outputs and the prepared registry, reconciles every station
//! into one final tier, writes a per-UF coverage report, merges the publishable
//! L2 map assertions and builds a `map-load` prepared envelope. It also writes
//! `locations.ndjson`, one best-tier pin per station for `station-locate`
//! (B-BR-MAP-02); L3/L4 rows are emitted only when the owner decisions D1/D2
//! are recorded as enabled. It never writes to the database or deploys.

use crate::map_contract::{self, ASSERTION_FORMAT, VERSION as MAP_VERSION};
use crate::national_match::{load_registry, MatchError, RegistryInput};
use crate::types::sha256_hex;
use serde::{Deserialize, Serialize};
use std::collections::{BTreeMap, HashSet};
use std::fs::{self, File};
use std::io::{BufWriter, Write};
use std::path::{Path, PathBuf};
use std::time::Instant;

pub const COVERAGE_FORMAT: &str = "abastevo.national-coverage-v1";
pub const APPLY_PLAN_FORMAT: &str = "abastevo.national-apply-plan-v1";
pub const ENVELOPE_FORMAT: &str = "abastevo.map-prepared-v1";
pub const ENVELOPE_VERSION: u32 = 1;
pub const MAX_DECISIONS: usize = 200_000;
pub const MAX_ASSERTIONS: usize = 10_000;
pub const MAX_FILE_BYTES: u64 = 256 << 20;
pub const PROFILE_ID: &str = "national-open-map-v1";
pub const LOCATIONS_FORMAT: &str = "abastevo.station-location-tiers-v1";
pub const LOCATION_SCHEMA: &str = "station-location-tier-v1";

const A2_SUMMARY_FORMAT: &str = "abastevo.national-match-summary-v1";
const B2_SUMMARY_FORMAT: &str = "abastevo.open-geocode-summary-v1";

#[derive(Debug, Clone, Default, Deserialize)]
struct TierDecision {
    #[serde(default)]
    cnpj: String,
    #[serde(default)]
    tier: String,
    #[serde(default)]
    publish: bool,
    #[serde(default)]
    reason: String,
    #[serde(default)]
    osm_type: Option<String>,
    #[serde(default)]
    osm_id: Option<i64>,
    #[serde(default)]
    api_srid: String,
    #[serde(default)]
    api_latitude_deg: Option<String>,
    #[serde(default)]
    api_longitude_deg: Option<String>,
}

#[derive(Debug, Clone, Deserialize)]
struct SummaryDoc {
    format: String,
    generation: String,
    inputs: SummaryHashes,
}

#[derive(Debug, Clone, Deserialize)]
struct SummaryHashes {
    registry_sha256: String,
    #[serde(default)]
    decisions_sha256: String,
}

#[derive(Debug, Clone, Deserialize, Serialize)]
pub struct FileDigest {
    pub bytes: u64,
    pub sha256: String,
}

#[derive(Debug, Clone, Deserialize)]
struct ReviewManifest {
    files: BTreeMap<String, FileDigest>,
}

#[derive(Debug, Clone, Deserialize, Serialize)]
struct AssertionCorpus {
    format: String,
    version: u32,
    generation: String,
    assertions: Vec<serde_json::Value>,
}

#[derive(Debug, Clone, Default, PartialEq, Eq, Serialize)]
pub struct CoverageCounts {
    pub stations: u64,
    pub l1: u64,
    pub l2: u64,
    pub l3: u64,
    pub l4: u64,
    pub l5: u64,
    pub published: u64,
    pub candidates: u64,
    pub no_point: u64,
}

#[derive(Debug, Clone, Serialize)]
pub struct Scope {
    pub a1a_status: &'static str,
    pub b2_scope: &'static str,
    pub national_api_complete: bool,
    pub national_cnefe_complete: bool,
    pub note: &'static str,
}

impl Scope {
    fn from_options(options: &CoverageOptions) -> Self {
        Scope {
            a1a_status: if options.national_api {
                "complete_by_uf"
            } else {
                "blocked_repeated_429"
            },
            b2_scope: if options.national_cnefe {
                "national_cnefe_rfb_extracts"
            } else {
                "pilot_municipalities_from_cnefe_rfb_extracts"
            },
            national_api_complete: options.national_api,
            national_cnefe_complete: options.national_cnefe,
            note: if options.national_api && options.national_cnefe {
                "National run: every UF of the ANP API and IBGE CNEFE."
            } else {
                "Pipeline validation, not a national coverage claim."
            },
        }
    }
}

/// Pin counts written to `locations.ndjson`, by stored quality.
#[derive(Debug, Clone, Default, PartialEq, Eq, Serialize)]
pub struct LocationCounts {
    pub reviewed: u64,
    pub osm_mapped: u64,
    pub anp_reported: u64,
    pub address_point: u64,
    pub total: u64,
}

#[derive(Debug, Clone, Serialize)]
struct LocationRow {
    schema_version: &'static str,
    cnpj: String,
    tier: String,
    quality: &'static str,
    latitude_deg: String,
    longitude_deg: String,
    provider: &'static str,
    source_reference: String,
}

#[derive(Debug, Clone, Serialize)]
pub struct InputHashes {
    pub registry_sha256: String,
    pub a2_generation: String,
    pub b2_generation: String,
    pub a2_decisions_sha256: String,
    pub a2_reviews_sha256: String,
    pub a2_manifest_sha256: String,
    pub a2_assertions_sha256: Option<String>,
    pub b2_decisions_sha256: String,
    pub b2_candidates_sha256: String,
    pub b2_assertions_sha256: Option<String>,
}

#[derive(Debug, Clone, Serialize)]
pub struct OutputHashes {
    pub coverage_md: FileDigest,
    pub combined_assertions: Option<FileDigest>,
    pub envelope: Option<FileDigest>,
    pub locations: Option<FileDigest>,
    pub locations_manifest: Option<FileDigest>,
}

#[derive(Debug, Clone, Serialize)]
pub struct CoverageReport {
    pub format: &'static str,
    pub generation: String,
    pub scope: Scope,
    pub api_only: u64,
    pub inputs: InputHashes,
    pub counts: CoverageCounts,
    pub per_uf: BTreeMap<String, CoverageCounts>,
    pub reasons: BTreeMap<String, u64>,
    pub locations: LocationCounts,
    pub outputs: OutputHashes,
}

#[derive(Debug, Clone, Serialize)]
pub struct ApplyStep {
    pub ready: bool,
    pub reason: String,
    pub count: u64,
    pub files: BTreeMap<String, String>,
    pub command: String,
}

#[derive(Debug, Clone, Serialize)]
pub struct Blocked {
    pub l3_anp: u64,
    pub l4_address: u64,
    pub owner_decisions: Vec<&'static str>,
    pub a1a_status: &'static str,
}

#[derive(Debug, Clone, Serialize)]
pub struct ApplyPlan {
    pub format: &'static str,
    pub generation: String,
    pub no_database_write_performed: bool,
    pub station_review: ApplyStep,
    pub map_load: ApplyStep,
    pub station_locate: ApplyStep,
    pub blocked: Blocked,
    pub coverage: CoverageCounts,
}

#[derive(Debug, Clone, Default, Serialize)]
pub struct Timings {
    pub load_ms: f64,
    pub combine_ms: f64,
    pub write_ms: f64,
    pub total_ms: f64,
}

#[derive(Debug, Clone, Default)]
pub struct CoverageOptions {
    pub generation: String,
    /// Owner decision D1: an ANP API point without OSM confirmation is a pin.
    pub publish_anp_reported: bool,
    /// Owner decision D2: an exact-number CNEFE establishment point is a pin.
    pub publish_address_point: bool,
    /// A1.a covered every UF of the ANP API.
    pub national_api: bool,
    /// B2 used CNEFE/RFB extracts for every UF.
    pub national_cnefe: bool,
}

#[derive(Debug, Clone)]
struct A2Input {
    dir: PathBuf,
    summary: SummaryDoc,
    decisions: Vec<TierDecision>,
    decisions_sha256: String,
    reviews: Vec<u8>,
    reviews_sha256: String,
    manifest_sha256: String,
    assertions: Option<AssertionCorpus>,
    assertions_sha256: Option<String>,
    assertions_path: Option<PathBuf>,
}

#[derive(Debug, Clone)]
struct B2Input {
    summary: SummaryDoc,
    decisions: Vec<TierDecision>,
    decisions_sha256: String,
    candidates: Vec<u8>,
    candidates_sha256: String,
    assertions: Option<AssertionCorpus>,
    assertions_sha256: Option<String>,
    assertions_path: Option<PathBuf>,
}

#[derive(Debug, Clone)]
pub struct RunOutput {
    pub coverage: CoverageReport,
    pub apply_plan: ApplyPlan,
    pub generation: String,
    pub timings: Timings,
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

fn load_summary(path: &Path, expected: &str) -> Result<SummaryDoc, MatchError> {
    let raw = read_bounded(path, MAX_FILE_BYTES)?;
    let summary: SummaryDoc = serde_json::from_slice(&raw).map_err(json_error)?;
    if summary.format != expected {
        return profile(format!(
            "{} has unexpected format {}",
            path.display(),
            summary.format
        ));
    }
    Ok(summary)
}

fn load_decisions(path: &Path) -> Result<(Vec<TierDecision>, String), MatchError> {
    let raw = read_bounded(path, MAX_FILE_BYTES)?;
    let sha256 = sha256_hex(&raw);
    let mut decisions = Vec::new();
    for line in raw.split(|byte| *byte == b'\n') {
        if line.iter().all(|byte| byte.is_ascii_whitespace()) {
            continue;
        }
        if decisions.len() >= MAX_DECISIONS {
            return limit(format!("decisions exceed {}", MAX_DECISIONS));
        }
        decisions.push(serde_json::from_slice::<TierDecision>(line).map_err(json_error)?);
    }
    Ok((decisions, sha256))
}

fn load_assertions(path: &Path) -> Result<Option<(AssertionCorpus, String)>, MatchError> {
    if !path.exists() {
        return Ok(None);
    }
    let raw = read_bounded(path, MAX_FILE_BYTES)?;
    let corpus: AssertionCorpus = serde_json::from_slice(&raw).map_err(json_error)?;
    if corpus.format != ASSERTION_FORMAT || corpus.version != MAP_VERSION {
        return profile(format!(
            "{} has unsupported assertion corpus",
            path.display()
        ));
    }
    if corpus.assertions.len() > MAX_ASSERTIONS {
        return limit(format!(
            "{} assertions exceed {}",
            path.display(),
            MAX_ASSERTIONS
        ));
    }
    Ok(Some((corpus, sha256_hex(&raw))))
}

fn load_assertions_ndjson(path: &Path) -> Result<Option<(AssertionCorpus, String)>, MatchError> {
    if !path.exists() {
        return Ok(None);
    }
    let raw = read_bounded(path, MAX_FILE_BYTES)?;
    if raw.iter().all(|byte| byte.is_ascii_whitespace()) {
        return Ok(None);
    }
    let mut assertions = Vec::new();
    for line in raw.split(|byte| *byte == b'\n') {
        if line.iter().all(|byte| byte.is_ascii_whitespace()) {
            continue;
        }
        if assertions.len() >= MAX_ASSERTIONS {
            return limit(format!(
                "{} assertions exceed {}",
                path.display(),
                MAX_ASSERTIONS
            ));
        }
        assertions.push(serde_json::from_slice::<serde_json::Value>(line).map_err(json_error)?);
    }
    Ok(Some((
        AssertionCorpus {
            format: ASSERTION_FORMAT.to_string(),
            version: MAP_VERSION,
            generation: String::new(),
            assertions,
        },
        sha256_hex(&raw),
    )))
}

fn load_reviews(
    reviews_path: &Path,
    manifest_path: &Path,
) -> Result<(Vec<u8>, String, ReviewManifest, String), MatchError> {
    let reviews = read_bounded(reviews_path, MAX_FILE_BYTES)?;
    let manifest_raw = read_bounded(manifest_path, MAX_FILE_BYTES)?;
    let manifest: ReviewManifest = serde_json::from_slice(&manifest_raw).map_err(json_error)?;
    let descriptor = manifest
        .files
        .get("reviews.ndjson")
        .ok_or_else(|| MatchError::Profile("review manifest has no reviews.ndjson".into()))?;
    if descriptor.bytes != reviews.len() as u64 {
        return profile("reviews.ndjson bytes do not match manifest");
    }
    let reviews_sha256 = sha256_hex(&reviews);
    if descriptor.sha256 != reviews_sha256 {
        return profile("reviews.ndjson hash does not match manifest");
    }
    Ok((reviews, reviews_sha256, manifest, sha256_hex(&manifest_raw)))
}

fn load_a2(dir: &Path) -> Result<A2Input, MatchError> {
    let summary = load_summary(&dir.join("summary.json"), A2_SUMMARY_FORMAT)?;
    let (decisions, decisions_sha256) = load_decisions(&dir.join("decisions.ndjson"))?;
    let (reviews, reviews_sha256, _manifest, manifest_sha256) =
        load_reviews(&dir.join("reviews.ndjson"), &dir.join("manifest.json"))?;
    let assertions_path = dir.join("map-assertions.json");
    let (assertions, assertions_sha256, assertions_path) = match load_assertions(&assertions_path)?
    {
        Some((corpus, sha256)) => (Some(corpus), Some(sha256), Some(assertions_path)),
        None => (None, None, None),
    };
    Ok(A2Input {
        dir: dir.to_path_buf(),
        summary,
        decisions,
        decisions_sha256,
        reviews,
        reviews_sha256,
        manifest_sha256,
        assertions,
        assertions_sha256,
        assertions_path,
    })
}

fn load_b2(dir: &Path) -> Result<B2Input, MatchError> {
    let summary = load_summary(&dir.join("summary.json"), B2_SUMMARY_FORMAT)?;
    let (decisions, decisions_sha256) = load_decisions(&dir.join("decisions.ndjson"))?;
    let candidates = read_bounded(&dir.join("candidates.ndjson"), MAX_FILE_BYTES)?;
    let candidates_sha256 = sha256_hex(&candidates);
    let json_path = dir.join("map-assertions.json");
    let ndjson_path = dir.join("map-assertions.ndjson");
    let (assertions, assertions_sha256, assertions_path) = match load_assertions(&json_path)? {
        Some((corpus, sha256)) => (Some(corpus), Some(sha256), Some(json_path)),
        None => match load_assertions_ndjson(&ndjson_path)? {
            Some((corpus, sha256)) => (Some(corpus), Some(sha256), Some(ndjson_path)),
            None => (None, None, None),
        },
    };
    Ok(B2Input {
        summary,
        decisions,
        decisions_sha256,
        candidates,
        candidates_sha256,
        assertions,
        assertions_sha256,
        assertions_path,
    })
}

fn count_reviews(raw: &[u8]) -> u64 {
    raw.split(|byte| *byte == b'\n')
        .filter(|line| !line.iter().all(|byte| byte.is_ascii_whitespace()))
        .count() as u64
}

fn default_generation(registry_sha256: &str, a2_sha256: &str, b2_sha256: &str) -> String {
    let mut seed = Vec::new();
    seed.extend_from_slice(registry_sha256.as_bytes());
    seed.extend_from_slice(a2_sha256.as_bytes());
    seed.extend_from_slice(b2_sha256.as_bytes());
    format!("national-{}", &sha256_hex(&seed)[..16])
}

fn add_tier(counts: &mut CoverageCounts, tier: &str) {
    match tier {
        "L1_CNPJ" | "L1_ADDRESS" => counts.l1 += 1,
        "L2_CNPJ" | "L2_ADDRESS" => counts.l2 += 1,
        "L3_ANP" => counts.l3 += 1,
        "L4_ADDRESS" => counts.l4 += 1,
        "L5" => counts.l5 += 1,
        _ => {}
    }
}

fn finalize_counts(counts: &mut CoverageCounts) {
    counts.published = counts.l1 + counts.l2;
    counts.candidates = counts.l3 + counts.l4;
    counts.no_point = counts.l5;
}

type CoverageOutput = (
    CoverageCounts,
    BTreeMap<String, CoverageCounts>,
    BTreeMap<String, u64>,
    u64,
    BTreeMap<String, TierDecision>,
);

fn compute_coverage(
    registry: &RegistryInput,
    a2: &[TierDecision],
    b2: &[TierDecision],
) -> Result<CoverageOutput, MatchError> {
    let mut a2_by_cnpj: BTreeMap<String, &TierDecision> = BTreeMap::new();
    let mut api_only = 0u64;
    for decision in a2 {
        if decision.tier == "API_ONLY" {
            api_only += 1;
            continue;
        }
        let Ok(cnpj) = crate::cnpj::normalize_cnpj(&decision.cnpj) else {
            return profile(format!("invalid A2 CNPJ {}", decision.cnpj));
        };
        if a2_by_cnpj.insert(cnpj.clone(), decision).is_some() {
            return profile(format!("duplicate A2 decision for {cnpj}"));
        }
        let published = decision.tier.starts_with("L1") || decision.tier.starts_with("L2");
        if decision.publish != published {
            return profile(format!(
                "A2 publish flag disagrees with tier {} for {}",
                decision.tier, cnpj
            ));
        }
    }
    if a2_by_cnpj.len() != registry.rows.len() {
        return profile(format!(
            "A2 decisions cover {} of {} registry stations",
            a2_by_cnpj.len(),
            registry.rows.len()
        ));
    }
    let mut b2_by_cnpj: BTreeMap<String, &TierDecision> = BTreeMap::new();
    for decision in b2 {
        let Ok(cnpj) = crate::cnpj::normalize_cnpj(&decision.cnpj) else {
            return profile(format!("invalid B2 CNPJ {}", decision.cnpj));
        };
        if b2_by_cnpj.insert(cnpj.clone(), decision).is_some() {
            return profile(format!("duplicate B2 decision for {cnpj}"));
        }
        let published = decision.tier.starts_with("L2");
        if decision.publish != published {
            return profile(format!(
                "B2 publish flag disagrees with tier {} for {}",
                decision.tier, cnpj
            ));
        }
    }
    let a2_l5: HashSet<&String> = a2_by_cnpj
        .iter()
        .filter(|(_, decision)| decision.tier == "L5")
        .map(|(cnpj, _)| cnpj)
        .collect();
    let b2_keys: HashSet<&String> = b2_by_cnpj.keys().collect();
    if a2_l5 != b2_keys {
        return profile(format!(
            "B2 covers {} A2 L5 stations, expected {}",
            b2_keys.len(),
            a2_l5.len()
        ));
    }

    let mut national = CoverageCounts::default();
    let mut per_uf: BTreeMap<String, CoverageCounts> = BTreeMap::new();
    let mut reasons: BTreeMap<String, u64> = BTreeMap::new();
    let mut finals: BTreeMap<String, TierDecision> = BTreeMap::new();
    for (cnpj, a2_decision) in &a2_by_cnpj {
        let Some(registry_row) = registry.rows.get(cnpj) else {
            return profile(format!("A2 decision without registry row: {cnpj}"));
        };
        let final_decision: &TierDecision = if a2_decision.tier == "L5" {
            b2_by_cnpj
                .get(cnpj)
                .ok_or_else(|| MatchError::Profile(format!("B2 missing {cnpj}")))?
        } else {
            a2_decision
        };
        finals.insert(cnpj.clone(), final_decision.clone());
        let (tier, reason) = (final_decision.tier.as_str(), final_decision.reason.as_str());
        let uf = registry_row.uf.clone();
        let bucket = per_uf.entry(uf).or_default();
        bucket.stations += 1;
        add_tier(bucket, tier);
        *reasons.entry(reason.to_string()).or_default() += 1;
        national.stations += 1;
        add_tier(&mut national, tier);
    }
    finalize_counts(&mut national);
    for bucket in per_uf.values_mut() {
        finalize_counts(bucket);
    }
    if national.published + national.candidates + national.no_point != national.stations
        || national.l1 + national.l2 + national.l3 + national.l4 + national.l5 != national.stations
    {
        return profile("coverage tiers do not reconcile");
    }
    Ok((national, per_uf, reasons, api_only, finals))
}

fn brazil_point(cnpj: &str, latitude: &str, longitude: &str) -> Result<(), MatchError> {
    let parsed = latitude
        .trim()
        .parse::<f64>()
        .ok()
        .zip(longitude.trim().parse::<f64>().ok());
    match parsed {
        Some((lat, lon))
            if lat.is_finite()
                && lon.is_finite()
                && (-34.0..=6.0).contains(&lat)
                && (-74.0..=-28.0).contains(&lon) =>
        {
            Ok(())
        }
        _ => profile(format!("location for {cnpj} is not a point inside Brazil")),
    }
}

fn json_text(value: &serde_json::Value) -> Option<String> {
    match value {
        serde_json::Value::String(text) => Some(text.trim().to_string()),
        serde_json::Value::Number(number) => Some(number.to_string()),
        _ => None,
    }
}

type PointRef = (String, String, String);

/// `locations.ndjson` bytes, counts and its hash-binding manifest.
struct LocationsOutput {
    bytes: Vec<u8>,
    counts: LocationCounts,
    manifest: Vec<u8>,
}

impl LocationsOutput {
    fn new(
        bytes: Vec<u8>,
        counts: LocationCounts,
        generation: &str,
        registry_sha256: &str,
        a2_decisions_sha256: &str,
        b2_decisions_sha256: &str,
        options: &CoverageOptions,
    ) -> Result<Self, MatchError> {
        let manifest = serde_json::json!({
            "format": LOCATIONS_FORMAT,
            "schema_version": LOCATION_SCHEMA,
            "generation": generation,
            "owner_decisions": {
                "D1_anp_reported": options.publish_anp_reported,
                "D2_address_point": options.publish_address_point,
            },
            "inputs": {
                "registry_sha256": registry_sha256,
                "a2_decisions_sha256": a2_decisions_sha256,
                "b2_decisions_sha256": b2_decisions_sha256,
            },
            "counts": counts,
            "files": {
                "locations.ndjson": {"bytes": bytes.len(), "sha256": sha256_hex(&bytes)},
            },
        });
        let mut manifest = serde_json::to_vec_pretty(&manifest).map_err(json_error)?;
        manifest.push(b'\n');
        Ok(LocationsOutput {
            bytes,
            counts,
            manifest,
        })
    }
}

fn ndjson_objects(raw: &[u8]) -> Result<Vec<serde_json::Value>, MatchError> {
    let mut out = Vec::new();
    for line in raw.split(|byte| *byte == b'\n') {
        if line.iter().all(|byte| byte.is_ascii_whitespace()) {
            continue;
        }
        out.push(serde_json::from_slice(line).map_err(json_error)?);
    }
    Ok(out)
}

/// Builds one best-tier pin per station (B-BR-MAP-02) from the final
/// decisions. Every published tier must resolve to exactly the point its
/// decision names; L3/L4 rows appear only when D1/D2 are enabled.
fn build_locations(
    finals: &BTreeMap<String, TierDecision>,
    reviews: &[u8],
    l2_assertions: &[serde_json::Value],
    b2_candidates: &[u8],
    options: &CoverageOptions,
) -> Result<(Vec<u8>, LocationCounts), MatchError> {
    let mut l1: BTreeMap<String, PointRef> = BTreeMap::new();
    for review in ndjson_objects(reviews)? {
        let cnpj = review["cnpj"].as_str().unwrap_or_default();
        let Ok(cnpj) = crate::cnpj::normalize_cnpj(cnpj) else {
            return profile(format!("review with invalid CNPJ {cnpj}"));
        };
        let (Some(lat), Some(lon)) = (
            json_text(&review["latitude"]),
            json_text(&review["longitude"]),
        ) else {
            return profile(format!("review without point for {cnpj}"));
        };
        let reference = format!(
            "{}|reviewer={}|{}",
            review["method"].as_str().unwrap_or_default(),
            review["reviewer"].as_str().unwrap_or_default(),
            review["evidence"].as_str().unwrap_or_default()
        );
        if l1.insert(cnpj.clone(), (lat, lon, reference)).is_some() {
            return profile(format!("duplicate review for {cnpj}"));
        }
    }
    let mut l2: BTreeMap<(String, String, i64), PointRef> = BTreeMap::new();
    for assertion in l2_assertions {
        let identities = &assertion["identities"];
        let (Some(cnpj), Some(osm_type), Some(osm_id)) = (
            identities["cnpj"].as_str(),
            identities["osm_type"].as_str(),
            identities["osm_id"].as_i64(),
        ) else {
            continue;
        };
        let location = &assertion["location"];
        let (Some(lat), Some(lon)) = (
            json_text(&location["latitude_deg"]),
            json_text(&location["longitude_deg"]),
        ) else {
            continue;
        };
        let reference = format!("osm:{osm_type}/{osm_id}|ODbL-1.0");
        l2.insert(
            (cnpj.to_string(), osm_type.to_string(), osm_id),
            (lat, lon, reference),
        );
    }
    let mut l4: BTreeMap<String, PointRef> = BTreeMap::new();
    for candidate in ndjson_objects(b2_candidates)? {
        if candidate["tier"].as_str() != Some("L4_ADDRESS") {
            continue;
        }
        let cnpj = candidate["cnpj"].as_str().unwrap_or_default().to_string();
        let (Some(lat), Some(lon)) = (
            json_text(&candidate["latitude_deg"]),
            json_text(&candidate["longitude_deg"]),
        ) else {
            return profile(format!("L4 candidate without point for {cnpj}"));
        };
        let source_sha = candidate["source_sha256"].as_str().unwrap_or_default();
        let reference = format!(
            "ibge-cnefe-2022|sha256={}|address={}|spread_m={}",
            &source_sha[..source_sha.len().min(16)],
            candidate["address_source"].as_str().unwrap_or_default(),
            candidate["spread_m"]
        );
        if l4.insert(cnpj.clone(), (lat, lon, reference)).is_some() {
            return profile(format!("duplicate L4 candidate for {cnpj}"));
        }
    }

    let mut counts = LocationCounts::default();
    let mut out = Vec::new();
    for (cnpj, decision) in finals {
        let (quality, provider, point) = match decision.tier.as_str() {
            "L1_CNPJ" | "L1_ADDRESS" => ("reviewed", "registry-api", l1.get(cnpj).cloned()),
            "L2_CNPJ" | "L2_ADDRESS" => {
                let key = (
                    cnpj.clone(),
                    decision.osm_type.clone().unwrap_or_default(),
                    decision.osm_id.unwrap_or_default(),
                );
                ("osm-mapped", "openstreetmap", l2.get(&key).cloned())
            }
            "L3_ANP" if options.publish_anp_reported => {
                let point = decision
                    .api_latitude_deg
                    .clone()
                    .zip(decision.api_longitude_deg.clone())
                    .map(|(lat, lon)| (lat, lon, format!("anp-api|srid={}", decision.api_srid)));
                ("anp-reported", "anp-revendedores-api", point)
            }
            "L4_ADDRESS" if options.publish_address_point => {
                ("address-point", "ibge-cnefe-2022", l4.get(cnpj).cloned())
            }
            _ => continue,
        };
        let Some((latitude, longitude, reference)) = point else {
            return profile(format!(
                "{} decision for {cnpj} has no matching point",
                decision.tier
            ));
        };
        brazil_point(cnpj, &latitude, &longitude)?;
        match quality {
            "reviewed" => counts.reviewed += 1,
            "osm-mapped" => counts.osm_mapped += 1,
            "anp-reported" => counts.anp_reported += 1,
            _ => counts.address_point += 1,
        }
        counts.total += 1;
        let row = LocationRow {
            schema_version: LOCATION_SCHEMA,
            cnpj: cnpj.clone(),
            tier: decision.tier.clone(),
            quality,
            latitude_deg: latitude,
            longitude_deg: longitude,
            provider,
            source_reference: reference,
        };
        out.extend(serde_json::to_vec(&row).map_err(json_error)?);
        out.push(b'\n');
    }
    Ok((out, counts))
}

fn combine_assertions(
    a2: Option<&AssertionCorpus>,
    b2: Option<&AssertionCorpus>,
) -> Result<(Vec<serde_json::Value>, usize), MatchError> {
    let mut by_id: BTreeMap<String, serde_json::Value> = BTreeMap::new();
    let mut duplicates = 0usize;
    for corpus in [a2, b2].into_iter().flatten() {
        for assertion in &corpus.assertions {
            let Some(id) = assertion.get("id").and_then(|value| value.as_str()) else {
                return profile("map assertion without id");
            };
            if let Some(existing) = by_id.get(id) {
                if existing != assertion {
                    return profile(format!("conflicting map assertion id {id}"));
                }
                duplicates += 1;
            } else {
                by_id.insert(id.to_string(), assertion.clone());
            }
        }
    }
    if by_id.len() > MAX_ASSERTIONS {
        return limit(format!(
            "combined assertions {} exceed {}",
            by_id.len(),
            MAX_ASSERTIONS
        ));
    }
    Ok((by_id.into_values().collect(), duplicates))
}

fn combined_corpus_bytes(
    generation: &str,
    assertions: &[serde_json::Value],
) -> Result<Vec<u8>, MatchError> {
    let corpus = serde_json::json!({
        "format": ASSERTION_FORMAT,
        "version": MAP_VERSION,
        "generation": generation,
        "assertions": assertions,
    });
    let mut bytes = serde_json::to_vec_pretty(&corpus).map_err(json_error)?;
    bytes.push(b'\n');
    Ok(bytes)
}

fn object(
    entries: impl IntoIterator<Item = (&'static str, serde_json::Value)>,
) -> serde_json::Value {
    let mut map = BTreeMap::new();
    for (key, value) in entries {
        map.insert(key.to_string(), value);
    }
    serde_json::Value::Object(map.into_iter().collect())
}

fn build_envelope(
    combined_bytes: &[u8],
    records: usize,
    duplicates: usize,
    generation: &str,
    inputs: Vec<serde_json::Value>,
) -> Result<(Vec<u8>, String), MatchError> {
    let assertions_sha256 = sha256_hex(combined_bytes);
    let profile_sha256 = sha256_hex(format!("{PROFILE_ID}|{generation}").as_bytes());
    let rights_sha256 = sha256_hex(b"ODbL-1.0|ANP|OSM");
    let mut root = BTreeMap::new();
    root.insert(
        "format".to_string(),
        serde_json::Value::String(ENVELOPE_FORMAT.to_string()),
    );
    root.insert(
        "version".to_string(),
        serde_json::Value::Number(ENVELOPE_VERSION.into()),
    );
    root.insert(
        "product_class".to_string(),
        serde_json::Value::String("PUBLIC_OVERLAY".to_string()),
    );
    root.insert(
        "visibility".to_string(),
        serde_json::Value::String("PUBLIC".to_string()),
    );
    root.insert(
        "profile_id".to_string(),
        serde_json::Value::String(PROFILE_ID.to_string()),
    );
    root.insert(
        "profile_sha256".to_string(),
        serde_json::Value::String(profile_sha256),
    );
    root.insert(
        "rights_sha256".to_string(),
        serde_json::Value::String(rights_sha256),
    );
    root.insert(
        "source_revision".to_string(),
        serde_json::Value::String(generation.to_string()),
    );
    root.insert("inputs".to_string(), serde_json::Value::Array(inputs));
    root.insert(
        "artifacts".to_string(),
        object([(
            "assertions",
            object([
                (
                    "bytes",
                    serde_json::Value::Number((combined_bytes.len() as u64).into()),
                ),
                (
                    "records",
                    serde_json::Value::Number((records as u64).into()),
                ),
                ("sha256", serde_json::Value::String(assertions_sha256)),
            ]),
        )]),
    );
    root.insert(
        "counts".to_string(),
        object([
            (
                "input",
                serde_json::Value::Number(((records + duplicates) as u64).into()),
            ),
            (
                "accepted",
                serde_json::Value::Number((records as u64).into()),
            ),
            (
                "duplicates",
                serde_json::Value::Number((duplicates as u64).into()),
            ),
            ("quarantined", serde_json::Value::Number(0.into())),
        ]),
    );
    let canonical = serde_json::to_string(&root).map_err(json_error)?;
    let content_sha256 = sha256_hex(canonical.as_bytes());
    root.insert(
        "content_sha256".to_string(),
        serde_json::Value::String(content_sha256.clone()),
    );
    let mut bytes = serde_json::to_vec_pretty(&root).map_err(json_error)?;
    bytes.push(b'\n');
    Ok((bytes, content_sha256))
}

fn source_input(file: &str, bytes: u64, sha256: &str, roots: u64) -> serde_json::Value {
    object([
        ("file", serde_json::Value::String(file.to_string())),
        ("bytes", serde_json::Value::Number(bytes.into())),
        ("sha256", serde_json::Value::String(sha256.to_string())),
        ("roots", serde_json::Value::Number(roots.into())),
    ])
}

fn coverage_markdown(report: &CoverageReport) -> String {
    let mut out = String::new();
    out.push_str("# National station coverage\n\n");
    out.push_str(&format!("Generation: `{}`\n\n", report.generation));
    out.push_str(&format!(
        "Scope: A1.a is `{}`; B2 covers `{}`. This is pipeline coverage, not a national coverage claim.\n\n",
        report.scope.a1a_status, report.scope.b2_scope
    ));
    out.push_str("| UF | Stations | L1 | L2 | Published | L3 | L4 | Candidates | L5 | No point | Published % |\n");
    out.push_str("| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |\n");
    for (uf, counts) in &report.per_uf {
        let pct = if counts.stations == 0 {
            0.0
        } else {
            counts.published as f64 * 100.0 / counts.stations as f64
        };
        out.push_str(&format!(
            "| {} | {} | {} | {} | {} | {} | {} | {} | {} | {} | {:.2}% |\n",
            uf,
            counts.stations,
            counts.l1,
            counts.l2,
            counts.published,
            counts.l3,
            counts.l4,
            counts.candidates,
            counts.l5,
            counts.no_point,
            pct
        ));
    }
    let counts = &report.counts;
    let pct = if counts.stations == 0 {
        0.0
    } else {
        counts.published as f64 * 100.0 / counts.stations as f64
    };
    out.push_str(&format!(
        "| **Total** | **{}** | **{}** | **{}** | **{}** | **{}** | **{}** | **{}** | **{}** | **{}** | **{:.2}%** |\n",
        counts.stations,
        counts.l1,
        counts.l2,
        counts.published,
        counts.l3,
        counts.l4,
        counts.candidates,
        counts.l5,
        counts.no_point,
        pct
    ));
    out.push_str("\n## Reasons\n\n");
    let mut reasons: Vec<(&String, &u64)> = report.reasons.iter().collect();
    reasons.sort_by(|left, right| right.1.cmp(left.1).then(left.0.cmp(right.0)));
    for (reason, count) in reasons {
        out.push_str(&format!("- `{reason}`: {count}\n"));
    }
    out
}

fn write_file(path: &Path, bytes: &[u8]) -> Result<FileDigest, MatchError> {
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
    Ok(FileDigest {
        bytes: bytes.len() as u64,
        sha256: sha256_hex(bytes),
    })
}

fn elapsed_ms(start: Instant) -> f64 {
    start.elapsed().as_secs_f64() * 1000.0
}

fn path_string(path: &Path) -> String {
    path.to_string_lossy().to_string()
}

#[allow(clippy::too_many_arguments)]
fn write_run(
    output: &Path,
    generation: &str,
    coverage: CoverageCounts,
    per_uf: BTreeMap<String, CoverageCounts>,
    reasons: BTreeMap<String, u64>,
    api_only: u64,
    inputs: InputHashes,
    a2: &A2Input,
    combined: &[serde_json::Value],
    combined_bytes: Option<&[u8]>,
    envelope_bytes: Option<&[u8]>,
    locations: &LocationsOutput,
    options: &CoverageOptions,
    timings: &mut Timings,
) -> Result<RunOutput, MatchError> {
    let write_start = Instant::now();
    let parent = output.parent().unwrap_or_else(|| Path::new("."));
    fs::create_dir_all(parent).map_err(|error| MatchError::Io(error.to_string()))?;
    let temp = output.with_extension("tmp-national-coverage");
    if temp.exists() {
        fs::remove_dir_all(&temp).map_err(|error| MatchError::Io(error.to_string()))?;
    }
    fs::create_dir(&temp).map_err(|error| MatchError::Io(error.to_string()))?;

    let combined_path = temp.join("map-assertions-combined.json");
    let envelope_path = temp.join("map-envelope.json");
    let combined_digest = match combined_bytes {
        Some(bytes) => Some(write_file(&combined_path, bytes)?),
        None => None,
    };
    let envelope_digest = match envelope_bytes {
        Some(bytes) => Some(write_file(&envelope_path, bytes)?),
        None => None,
    };
    let (locations_digest, locations_manifest_digest) = if locations.counts.total > 0 {
        (
            Some(write_file(
                &temp.join("locations.ndjson"),
                &locations.bytes,
            )?),
            Some(write_file(
                &temp.join("locations-manifest.json"),
                &locations.manifest,
            )?),
        )
    } else {
        (None, None)
    };
    let station_locate = ApplyStep {
        ready: locations.counts.total > 0,
        reason: if locations.counts.total > 0 {
            "ready".to_string()
        } else {
            "no_locations".to_string()
        },
        count: locations.counts.total,
        files: BTreeMap::from([
            (
                "locations".to_string(),
                path_string(&output.join("locations.ndjson")),
            ),
            (
                "manifest".to_string(),
                path_string(&output.join("locations-manifest.json")),
            ),
        ]),
        command: format!(
            "station-locate --locations {} --manifest {} --dsn <STAGING_DSN>",
            path_string(&output.join("locations.ndjson")),
            path_string(&output.join("locations-manifest.json"))
        ),
    };
    let mut owner_decisions = Vec::new();
    if !options.publish_anp_reported {
        owner_decisions.push("D1");
    }
    if !options.publish_address_point {
        owner_decisions.push("D2");
    }
    let scope = Scope::from_options(options);

    let a2_reviews_count = count_reviews(&a2.reviews);
    let a2_reviews_path = a2.dir.join("reviews.ndjson");
    let a2_manifest_path = a2.dir.join("manifest.json");
    let station_review = ApplyStep {
        ready: a2_reviews_count > 0,
        reason: if a2_reviews_count > 0 {
            "ready".to_string()
        } else {
            "no_reviews".to_string()
        },
        count: a2_reviews_count,
        files: BTreeMap::from([
            ("reviews".to_string(), path_string(&a2_reviews_path)),
            ("manifest".to_string(), path_string(&a2_manifest_path)),
        ]),
        command: format!(
            "station-review --reviews {} --manifest {} --dsn <STAGING_DSN>",
            path_string(&a2_reviews_path),
            path_string(&a2_manifest_path)
        ),
    };
    let map_load = ApplyStep {
        ready: !combined.is_empty() && envelope_digest.is_some(),
        reason: if !combined.is_empty() && envelope_digest.is_some() {
            "ready".to_string()
        } else {
            "no_publishable_map_assertions".to_string()
        },
        count: combined.len() as u64,
        files: BTreeMap::from([
            (
                "assertions".to_string(),
                path_string(&output.join("map-assertions-combined.json")),
            ),
            (
                "envelope".to_string(),
                path_string(&output.join("map-envelope.json")),
            ),
        ]),
        command: format!(
            "map-load --envelope {} --assertions {} --dsn <STAGING_DSN>",
            path_string(&output.join("map-envelope.json")),
            path_string(&output.join("map-assertions-combined.json"))
        ),
    };
    let apply_plan = ApplyPlan {
        format: APPLY_PLAN_FORMAT,
        generation: generation.to_string(),
        no_database_write_performed: true,
        station_review,
        map_load,
        station_locate,
        blocked: Blocked {
            l3_anp: if options.publish_anp_reported {
                0
            } else {
                coverage.l3
            },
            l4_address: if options.publish_address_point {
                0
            } else {
                coverage.l4
            },
            owner_decisions,
            a1a_status: scope.a1a_status,
        },
        coverage: coverage.clone(),
    };
    let coverage_md = coverage_markdown(&CoverageReport {
        format: COVERAGE_FORMAT,
        generation: generation.to_string(),
        scope: scope.clone(),
        api_only,
        inputs: inputs.clone(),
        counts: coverage.clone(),
        per_uf: per_uf.clone(),
        reasons: reasons.clone(),
        locations: locations.counts.clone(),
        outputs: OutputHashes {
            coverage_md: FileDigest {
                bytes: 0,
                sha256: String::new(),
            },
            combined_assertions: combined_digest.clone(),
            envelope: envelope_digest.clone(),
            locations: locations_digest.clone(),
            locations_manifest: locations_manifest_digest.clone(),
        },
    });
    let coverage_md_digest = write_file(&temp.join("coverage.md"), coverage_md.as_bytes())?;
    let report = CoverageReport {
        format: COVERAGE_FORMAT,
        generation: generation.to_string(),
        scope: scope.clone(),
        api_only,
        inputs,
        counts: coverage,
        per_uf,
        reasons,
        locations: locations.counts.clone(),
        outputs: OutputHashes {
            coverage_md: coverage_md_digest,
            combined_assertions: combined_digest,
            envelope: envelope_digest,
            locations: locations_digest,
            locations_manifest: locations_manifest_digest,
        },
    };
    let coverage_bytes = {
        let mut bytes = serde_json::to_vec_pretty(&report).map_err(json_error)?;
        bytes.push(b'\n');
        bytes
    };
    write_file(&temp.join("coverage.json"), &coverage_bytes)?;
    let plan_bytes = {
        let mut bytes = serde_json::to_vec_pretty(&apply_plan).map_err(json_error)?;
        bytes.push(b'\n');
        bytes
    };
    write_file(&temp.join("apply-plan.json"), &plan_bytes)?;

    timings.write_ms = elapsed_ms(write_start);
    timings.total_ms = timings.load_ms + timings.combine_ms + timings.write_ms;
    let timings_bytes = {
        let mut bytes = serde_json::to_vec_pretty(timings).map_err(json_error)?;
        bytes.push(b'\n');
        bytes
    };
    write_file(&temp.join("timings.json"), &timings_bytes)?;

    if output.exists() {
        let backup = output.with_extension("bak-national-coverage");
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
    Ok(RunOutput {
        coverage: report,
        apply_plan,
        generation: generation.to_string(),
        timings: timings.clone(),
    })
}

pub fn run(
    a2_dir: &Path,
    b2_dir: &Path,
    registry_path: &Path,
    output: &Path,
    options: &CoverageOptions,
) -> Result<RunOutput, MatchError> {
    let total_start = Instant::now();
    let load_start = Instant::now();
    let registry = load_registry(registry_path)?;
    let a2 = load_a2(a2_dir)?;
    let b2 = load_b2(b2_dir)?;
    if a2.summary.inputs.registry_sha256 != registry.sha256 {
        return profile("A2 summary registry hash does not match the registry file");
    }
    if b2.summary.inputs.registry_sha256 != registry.sha256 {
        return profile("B2 summary registry hash does not match the registry file");
    }
    if b2.summary.inputs.decisions_sha256 != a2.decisions_sha256 {
        return profile("B2 decisions were not produced from the supplied A2 decisions");
    }
    let load_ms = elapsed_ms(load_start);

    let combine_start = Instant::now();
    let (national, per_uf, reasons, api_only, finals) =
        compute_coverage(&registry, &a2.decisions, &b2.decisions)?;
    let (combined, duplicates) =
        combine_assertions(a2.assertions.as_ref(), b2.assertions.as_ref())?;
    let generation = if options.generation.trim().is_empty() {
        default_generation(&registry.sha256, &a2.decisions_sha256, &b2.decisions_sha256)
    } else {
        options.generation.clone()
    };
    let combined_bytes = if combined.is_empty() {
        None
    } else {
        let bytes = combined_corpus_bytes(&generation, &combined)?;
        map_contract::parse_assertions(&bytes).map_err(|error| {
            MatchError::Profile(format!("combined assertions invalid: {error}"))
        })?;
        Some(bytes)
    };
    let envelope_bytes = match &combined_bytes {
        Some(bytes) => {
            let mut inputs = Vec::new();
            if let (Some(path), Some(sha), Some(corpus)) =
                (&a2.assertions_path, &a2.assertions_sha256, &a2.assertions)
            {
                let bytes = fs::metadata(path)
                    .map_err(|error| MatchError::Io(error.to_string()))?
                    .len();
                inputs.push(source_input(
                    "a2-map-assertions.json",
                    bytes,
                    sha,
                    corpus.assertions.len() as u64,
                ));
            }
            if let (Some(path), Some(sha), Some(corpus)) =
                (&b2.assertions_path, &b2.assertions_sha256, &b2.assertions)
            {
                let bytes = fs::metadata(path)
                    .map_err(|error| MatchError::Io(error.to_string()))?
                    .len();
                inputs.push(source_input(
                    "b2-map-assertions.json",
                    bytes,
                    sha,
                    corpus.assertions.len() as u64,
                ));
            }
            let (envelope, _) =
                build_envelope(bytes, combined.len(), duplicates, &generation, inputs)?;
            Some(envelope)
        }
        None => None,
    };
    let (location_bytes, location_counts) =
        build_locations(&finals, &a2.reviews, &combined, &b2.candidates, options)?;
    let locations = LocationsOutput::new(
        location_bytes,
        location_counts,
        &generation,
        &registry.sha256,
        &a2.decisions_sha256,
        &b2.decisions_sha256,
        options,
    )?;
    let combine_ms = elapsed_ms(combine_start);

    let inputs = InputHashes {
        registry_sha256: registry.sha256.clone(),
        a2_generation: a2.summary.generation.clone(),
        b2_generation: b2.summary.generation.clone(),
        a2_decisions_sha256: a2.decisions_sha256.clone(),
        a2_reviews_sha256: a2.reviews_sha256.clone(),
        a2_manifest_sha256: a2.manifest_sha256.clone(),
        a2_assertions_sha256: a2.assertions_sha256.clone(),
        b2_decisions_sha256: b2.decisions_sha256.clone(),
        b2_candidates_sha256: b2.candidates_sha256.clone(),
        b2_assertions_sha256: b2.assertions_sha256.clone(),
    };
    let mut timings = Timings {
        load_ms,
        combine_ms,
        ..Timings::default()
    };
    let mut output_run = write_run(
        output,
        &generation,
        national,
        per_uf,
        reasons,
        api_only,
        inputs,
        &a2,
        &combined,
        combined_bytes.as_deref(),
        envelope_bytes.as_deref(),
        &locations,
        options,
        &mut timings,
    )?;
    output_run.timings.total_ms = elapsed_ms(total_start);
    Ok(output_run)
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::national_match::{RegistryAddress, RegistryRecord};
    use serde_json::json;
    use std::collections::HashMap;
    use std::sync::atomic::{AtomicUsize, Ordering};

    static COUNTER: AtomicUsize = AtomicUsize::new(0);

    fn temp_dir(name: &str) -> PathBuf {
        let counter = COUNTER.fetch_add(1, Ordering::SeqCst);
        let dir = std::env::temp_dir().join(format!(
            "national-coverage-{}-{}-{}",
            std::process::id(),
            name,
            counter
        ));
        let _ = fs::remove_dir_all(&dir);
        fs::create_dir_all(&dir).unwrap();
        dir
    }

    fn registry_record(cnpj: &str, uf: &str, city: &str, ibge: &str) -> RegistryRecord {
        RegistryRecord {
            source_key: cnpj.to_string(),
            business_name_raw: "POSTO TESTE LTDA".to_string(),
            address_raw: "RUA TESTE, 100".to_string(),
            address_normalized: RegistryAddress {
                street: "RUA TESTE".to_string(),
                number: "100".to_string(),
            },
            uf: uf.to_string(),
            municipality_name_raw: city.to_string(),
            municipality_ibge: ibge.to_string(),
            brand_raw: "IPIRANGA".to_string(),
        }
    }

    fn registry_input(rows: Vec<RegistryRecord>) -> RegistryInput {
        let rows: HashMap<String, RegistryRecord> = rows
            .into_iter()
            .map(|row| (row.source_key.clone(), row))
            .collect();
        RegistryInput {
            rows,
            sha256: "a".repeat(64),
            bytes: 0,
            parsed_rows: 0,
            duplicates: 0,
            invalid_cnpj: 0,
        }
    }

    fn decision(cnpj: &str, tier: &str, publish: bool, reason: &str) -> TierDecision {
        TierDecision {
            cnpj: cnpj.to_string(),
            tier: tier.to_string(),
            publish,
            reason: reason.to_string(),
            ..TierDecision::default()
        }
    }

    fn corpus(assertions: Vec<serde_json::Value>) -> AssertionCorpus {
        AssertionCorpus {
            format: ASSERTION_FORMAT.to_string(),
            version: MAP_VERSION,
            generation: "test".to_string(),
            assertions,
        }
    }

    fn assertion(id: &str, cnpj: &str, osm_id: i64, name: &str) -> serde_json::Value {
        json!({
            "id": id,
            "source": "OSM",
            "kind": "FUEL_STATION",
            "name": name,
            "identities": {
                "station_uuid": null,
                "cnpj": cnpj,
                "place_id": null,
                "equipment_id": null,
                "connector_id": null,
                "osm_type": "node",
                "osm_id": osm_id
            },
            "location": {
                "latitude_deg": "-25.4",
                "longitude_deg": "-49.2",
                "quality": "osm-mapped",
                "display_point": "osm_node",
                "entrance_latitude_deg": null,
                "entrance_longitude_deg": null
            },
            "services": ["FUEL"],
            "prices": [],
            "charging": null,
            "clocks": {
                "observed_at": "2026-10-25T21:21:21Z",
                "published_at": "2026-10-25T21:21:21Z",
                "fetched_at": "2026-10-25T21:21:21Z"
            },
            "rights": {
                "source": "OSM",
                "license": "ADMITTED",
                "attribution": "© OpenStreetMap contributors — ODbL 1.0",
                "source_sha256": "b".repeat(64)
            },
            "capabilities": {
                "price_known": false,
                "pin_eligible": true,
                "entrance_known": false,
                "live_status": false
            }
        })
    }

    #[test]
    fn coverage_replaces_a2_l5_and_reconciles() {
        let registry = registry_input(vec![
            registry_record("01122233300000", "PR", "CURITIBA", "4106902"),
            registry_record("05938540000134", "PR", "LONDRINA", "4113700"),
            registry_record("08065008000148", "SP", "SAO PAULO", "3550308"),
        ]);
        let a2 = vec![
            decision("01122233300000", "L1_CNPJ", true, "cnpj_tag"),
            decision("05938540000134", "L5", false, "missing_osm"),
            decision("08065008000148", "L3_ANP", false, "missing_osm"),
        ];
        let b2 = vec![decision(
            "05938540000134",
            "L4_ADDRESS",
            false,
            "cnefe_establishment",
        )];
        let (national, per_uf, reasons, api_only, finals) =
            compute_coverage(&registry, &a2, &b2).expect("coverage");
        assert_eq!(finals["05938540000134"].tier, "L4_ADDRESS");
        assert_eq!(finals["08065008000148"].tier, "L3_ANP");
        assert_eq!(national.stations, 3);
        assert_eq!(national.l1, 1);
        assert_eq!(national.l3, 1);
        assert_eq!(national.l4, 1);
        assert_eq!(national.l5, 0);
        assert_eq!(national.published, 1);
        assert_eq!(national.candidates, 2);
        assert_eq!(national.no_point, 0);
        assert_eq!(per_uf["PR"].stations, 2);
        assert_eq!(per_uf["PR"].l1, 1);
        assert_eq!(per_uf["PR"].l4, 1);
        assert_eq!(per_uf["SP"].l3, 1);
        assert_eq!(reasons["cnefe_establishment"], 1);
        assert_eq!(api_only, 0);
    }

    #[test]
    fn b2_must_cover_every_a2_l5() {
        let registry = registry_input(vec![registry_record(
            "01122233300000",
            "PR",
            "CURITIBA",
            "4106902",
        )]);
        let a2 = vec![decision("01122233300000", "L5", false, "missing_osm")];
        assert!(compute_coverage(&registry, &a2, &[]).is_err());
    }

    #[test]
    fn b2_cannot_publish_an_a2_non_l5_station() {
        let registry = registry_input(vec![registry_record(
            "01122233300000",
            "PR",
            "CURITIBA",
            "4106902",
        )]);
        let a2 = vec![decision("01122233300000", "L1_CNPJ", true, "cnpj_tag")];
        let b2 = vec![decision(
            "01122233300000",
            "L2_ADDRESS",
            true,
            "address_municipality",
        )];
        assert!(compute_coverage(&registry, &a2, &b2).is_err());
    }

    #[test]
    fn combine_dedupes_identical_ids_and_rejects_conflicts() {
        let a2 = corpus(vec![json!({"id": "x", "value": 1})]);
        let b2 = corpus(vec![json!({"id": "x", "value": 1})]);
        let (combined, duplicates) = combine_assertions(Some(&a2), Some(&b2)).expect("combine");
        assert_eq!(combined.len(), 1);
        assert_eq!(duplicates, 1);
        let conflicting = corpus(vec![json!({"id": "x", "value": 2})]);
        assert!(combine_assertions(Some(&a2), Some(&conflicting)).is_err());
    }

    #[test]
    fn envelope_is_deterministic_and_self_consistent() {
        let bytes = b"{\"format\":\"abastevo.map-assertions-v1\",\"version\":1,\"generation\":\"g\",\"assertions\":[]}";
        let inputs = vec![source_input("a2.json", 10, &"a".repeat(64), 1)];
        let (first, hash) = build_envelope(bytes, 1, 0, "g", inputs.clone()).expect("envelope");
        let (second, hash2) = build_envelope(bytes, 1, 0, "g", inputs).expect("envelope");
        assert_eq!(first, second);
        assert_eq!(hash, hash2);
        let mut doc: serde_json::Value = serde_json::from_slice(&first).expect("json");
        assert_eq!(doc["format"], ENVELOPE_FORMAT);
        assert_eq!(doc["source_revision"], "g");
        assert_eq!(doc["counts"]["accepted"], 1);
        let expected = doc["content_sha256"].as_str().expect("hash").to_string();
        doc.as_object_mut().unwrap().remove("content_sha256");
        let canonical = serde_json::to_string(&doc).expect("canonical");
        assert_eq!(sha256_hex(canonical.as_bytes()), expected);
    }

    #[test]
    fn coverage_markdown_has_total_row() {
        let report = CoverageReport {
            format: COVERAGE_FORMAT,
            generation: "g".to_string(),
            scope: Scope {
                a1a_status: "blocked_repeated_429",
                b2_scope: "pilot",
                national_api_complete: false,
                national_cnefe_complete: false,
                note: "test",
            },
            api_only: 0,
            inputs: InputHashes {
                registry_sha256: "a".repeat(64),
                a2_generation: "a2".to_string(),
                b2_generation: "b2".to_string(),
                a2_decisions_sha256: "b".repeat(64),
                a2_reviews_sha256: "c".repeat(64),
                a2_manifest_sha256: "d".repeat(64),
                a2_assertions_sha256: None,
                b2_decisions_sha256: "e".repeat(64),
                b2_candidates_sha256: "f".repeat(64),
                b2_assertions_sha256: None,
            },
            counts: CoverageCounts {
                stations: 1,
                l1: 1,
                published: 1,
                ..CoverageCounts::default()
            },
            per_uf: BTreeMap::from([(
                "PR".to_string(),
                CoverageCounts {
                    stations: 1,
                    l1: 1,
                    published: 1,
                    ..CoverageCounts::default()
                },
            )]),
            reasons: BTreeMap::from([("cnpj_tag".to_string(), 1)]),
            locations: LocationCounts::default(),
            outputs: OutputHashes {
                coverage_md: FileDigest {
                    bytes: 0,
                    sha256: String::new(),
                },
                combined_assertions: None,
                envelope: None,
                locations: None,
                locations_manifest: None,
            },
        };
        let markdown = coverage_markdown(&report);
        assert!(markdown.contains("| **Total** |"));
        assert!(markdown.contains("| PR | 1 | 1 | 0 | 1 |"));
    }

    fn write_registry(root: &Path) -> (PathBuf, String) {
        let bytes = concat!(
            "{\"source_key\":\"01122233300000\",\"business_name_raw\":\"POSTO TESTE LTDA\",",
            "\"address_raw\":\"RUA TESTE, 100\",\"address_normalized\":{\"street\":\"RUA TESTE\",\"number\":\"100\"},",
            "\"uf\":\"PR\",\"municipality_name_raw\":\"CURITIBA\",\"municipality_ibge\":\"4106902\",",
            "\"brand_raw\":\"IPIRANGA\"}\n"
        );
        let path = root.join("registry.jsonl");
        fs::write(&path, bytes).unwrap();
        (path, sha256_hex(bytes.as_bytes()))
    }

    fn write_a2(root: &Path, registry_sha256: &str, with_assertions: bool) -> PathBuf {
        let dir = root.join("a2");
        fs::create_dir_all(&dir).unwrap();
        let decisions =
            "{\"cnpj\":\"01122233300000\",\"uf\":\"PR\",\"tier\":\"L1_CNPJ\",\"publish\":true,\"reason\":\"cnpj_tag\"}\n";
        fs::write(dir.join("decisions.ndjson"), decisions).unwrap();
        let decisions_sha256 = sha256_hex(decisions.as_bytes());
        let reviews = concat!(
            "{\"schema_version\":\"station-location-review-v1\",\"cnpj\":\"01122233300000\",",
            "\"station_name\":\"POSTO TESTE LTDA\",\"municipality\":\"CURITIBA\",\"state\":\"PR\",",
            "\"latitude\":-25.4,\"longitude\":-49.2,\"method\":\"ANP_REGISTRY_API+OSM_CNPJ\",",
            "\"match_distance_m\":1.0,\"reviewer\":\"tester\",\"evidence\":\"anp:2026-10-10;osm:node:1;ref-cnpj\"}\n"
        );
        fs::write(dir.join("reviews.ndjson"), reviews).unwrap();
        let manifest = json!({
            "format": "abastevo.station-location-review-v1",
            "parser_version": "station-location-review-v1",
            "reviewer": "tester",
            "max_distance_m": 200.0,
            "source_sha256": "a".repeat(64),
            "osm_sha256": "b".repeat(64),
            "registry_sha256": registry_sha256,
            "counts": {"input": 1, "reviewed": 1, "rejected": 0, "duplicates": 0},
            "files": {"reviews.ndjson": {"bytes": reviews.len(), "sha256": sha256_hex(reviews.as_bytes())}}
        });
        fs::write(
            dir.join("manifest.json"),
            serde_json::to_vec_pretty(&manifest).unwrap(),
        )
        .unwrap();
        let summary = json!({
            "format": A2_SUMMARY_FORMAT,
            "generation": "a2-test",
            "inputs": {"registry_sha256": registry_sha256}
        });
        fs::write(
            dir.join("summary.json"),
            serde_json::to_vec_pretty(&summary).unwrap(),
        )
        .unwrap();
        if with_assertions {
            let corpus = json!({
                "format": ASSERTION_FORMAT,
                "version": MAP_VERSION,
                "generation": "a2-test",
                "assertions": [assertion(
                    "11111111-1111-4111-8111-111111111111",
                    "01122233300000",
                    1,
                    "Posto Teste"
                )]
            });
            fs::write(
                dir.join("map-assertions.json"),
                serde_json::to_vec_pretty(&corpus).unwrap(),
            )
            .unwrap();
        }
        let _ = decisions_sha256;
        dir
    }

    fn write_b2(root: &Path, registry_sha256: &str, a2_dir: &Path) -> PathBuf {
        let dir = root.join("b2");
        fs::create_dir_all(&dir).unwrap();
        let a2_decisions = fs::read(a2_dir.join("decisions.ndjson")).unwrap();
        let summary = json!({
            "format": B2_SUMMARY_FORMAT,
            "generation": "b2-test",
            "inputs": {
                "registry_sha256": registry_sha256,
                "decisions_sha256": sha256_hex(&a2_decisions)
            }
        });
        fs::write(
            dir.join("summary.json"),
            serde_json::to_vec_pretty(&summary).unwrap(),
        )
        .unwrap();
        fs::write(dir.join("decisions.ndjson"), b"").unwrap();
        fs::write(dir.join("candidates.ndjson"), b"").unwrap();
        fs::write(dir.join("map-assertions.ndjson"), b"").unwrap();
        dir
    }

    #[test]
    fn end_to_end_builds_coverage_plan_and_envelope() {
        let root = temp_dir("e2e");
        let (registry, registry_sha256) = write_registry(&root);
        let a2 = write_a2(&root, &registry_sha256, true);
        let b2 = write_b2(&root, &registry_sha256, &a2);
        let output = root.join("out");
        let result = run(
            &a2,
            &b2,
            &registry,
            &output,
            &CoverageOptions {
                generation: "test-generation".to_string(),
                ..CoverageOptions::default()
            },
        )
        .expect("run");
        assert_eq!(result.coverage.counts.stations, 1);
        assert_eq!(result.coverage.counts.l1, 1);
        assert_eq!(result.coverage.counts.published, 1);
        assert!(result.apply_plan.station_review.ready);
        assert_eq!(result.apply_plan.station_review.count, 1);
        assert!(result.apply_plan.map_load.ready);
        assert_eq!(result.apply_plan.map_load.count, 1);
        assert!(output.join("coverage.json").exists());
        assert!(output.join("coverage.md").exists());
        assert!(output.join("apply-plan.json").exists());
        assert!(output.join("map-assertions-combined.json").exists());
        assert!(output.join("map-envelope.json").exists());
        let envelope: serde_json::Value =
            serde_json::from_slice(&fs::read(output.join("map-envelope.json")).unwrap()).unwrap();
        assert_eq!(envelope["format"], ENVELOPE_FORMAT);
        assert_eq!(envelope["source_revision"], "test-generation");
        assert_eq!(envelope["counts"]["accepted"], 1);
        assert_eq!(envelope["content_sha256"].as_str().unwrap().len(), 64);
        let combined: serde_json::Value =
            serde_json::from_slice(&fs::read(output.join("map-assertions-combined.json")).unwrap())
                .unwrap();
        assert_eq!(combined["assertions"].as_array().unwrap().len(), 1);
        let plan: serde_json::Value =
            serde_json::from_slice(&fs::read(output.join("apply-plan.json")).unwrap()).unwrap();
        assert_eq!(plan["no_database_write_performed"], true);
        assert_eq!(plan["blocked"]["owner_decisions"][0], "D1");
        assert!(result.apply_plan.station_locate.ready);
        assert_eq!(result.apply_plan.station_locate.count, 1);
        let locations = fs::read(output.join("locations.ndjson")).unwrap();
        let manifest: serde_json::Value =
            serde_json::from_slice(&fs::read(output.join("locations-manifest.json")).unwrap())
                .unwrap();
        assert_eq!(manifest["format"], LOCATIONS_FORMAT);
        assert_eq!(
            manifest["files"]["locations.ndjson"]["sha256"],
            sha256_hex(&locations)
        );
        let first_envelope = fs::read(output.join("map-envelope.json")).unwrap();
        let first_coverage = fs::read(output.join("coverage.json")).unwrap();
        let _ = fs::remove_dir_all(&root);
        let _ = first_envelope;
        let _ = first_coverage;
    }

    #[test]
    fn empty_assertions_leave_map_load_not_ready() {
        let root = temp_dir("empty");
        let (registry, registry_sha256) = write_registry(&root);
        let a2 = write_a2(&root, &registry_sha256, false);
        let b2 = write_b2(&root, &registry_sha256, &a2);
        let output = root.join("out");
        let result = run(
            &a2,
            &b2,
            &registry,
            &output,
            &CoverageOptions {
                generation: "test-generation".to_string(),
                ..CoverageOptions::default()
            },
        )
        .expect("run");
        assert!(result.apply_plan.station_review.ready);
        assert!(!result.apply_plan.map_load.ready);
        assert!(!output.join("map-envelope.json").exists());
        assert!(!output.join("map-assertions-combined.json").exists());
        let _ = fs::remove_dir_all(&root);
    }

    fn located(cnpj: &str, tier: &str) -> TierDecision {
        let mut decision = decision(
            cnpj,
            tier,
            tier.starts_with("L1") || tier.starts_with("L2"),
            "test",
        );
        if tier.starts_with("L2") {
            decision.osm_type = Some("node".to_string());
            decision.osm_id = Some(7);
        }
        if tier == "L3_ANP" {
            decision.api_srid = "4674".to_string();
            decision.api_latitude_deg = Some("-23.55".to_string());
            decision.api_longitude_deg = Some("-46.63".to_string());
        }
        decision
    }

    fn ladder_inputs() -> (
        BTreeMap<String, TierDecision>,
        Vec<u8>,
        Vec<serde_json::Value>,
        Vec<u8>,
    ) {
        let finals: BTreeMap<String, TierDecision> = [
            ("01122233300000", "L1_ADDRESS"),
            ("05938540000134", "L2_CNPJ"),
            ("08065008000148", "L3_ANP"),
            ("11222333000181", "L4_ADDRESS"),
            ("33000167000101", "L5"),
        ]
        .into_iter()
        .map(|(cnpj, tier)| (cnpj.to_string(), located(cnpj, tier)))
        .collect();
        let reviews = concat!(
            "{\"cnpj\":\"01122233300000\",\"latitude\":-25.4,\"longitude\":-49.2,",
            "\"method\":\"ANP_REGISTRY_API+OSM_ADDRESS_NAME\",\"reviewer\":\"r\",\"evidence\":\"e\"}\n"
        )
        .as_bytes()
        .to_vec();
        let l2 = vec![assertion(
            "11111111-1111-4111-8111-111111111111",
            "05938540000134",
            7,
            "Posto",
        )];
        let candidates = concat!(
            "{\"cnpj\":\"11222333000181\",\"tier\":\"L4_ADDRESS\",\"latitude_deg\":\"-15.6\",",
            "\"longitude_deg\":\"-56.1\",\"address_source\":\"rfb\",\"spread_m\":0.0,",
            "\"source_sha256\":\"0123456789abcdef0123\"}\n"
        )
        .as_bytes()
        .to_vec();
        (finals, reviews, l2, candidates)
    }

    fn rows(bytes: &[u8]) -> Vec<serde_json::Value> {
        ndjson_objects(bytes).unwrap()
    }

    #[test]
    fn locations_emit_one_best_tier_pin_per_station() {
        let (finals, reviews, l2, candidates) = ladder_inputs();
        let options = CoverageOptions {
            publish_anp_reported: true,
            publish_address_point: true,
            ..CoverageOptions::default()
        };
        let (bytes, counts) =
            build_locations(&finals, &reviews, &l2, &candidates, &options).unwrap();
        let rows = rows(&bytes);
        let qualities: Vec<&str> = rows
            .iter()
            .map(|row| row["quality"].as_str().unwrap())
            .collect();
        assert_eq!(
            qualities,
            ["reviewed", "osm-mapped", "anp-reported", "address-point"]
        );
        assert_eq!(counts.total, 4);
        assert_eq!(rows[0]["latitude_deg"], "-25.4");
        assert_eq!(rows[1]["source_reference"], "osm:node/7|ODbL-1.0");
        assert_eq!(rows[2]["latitude_deg"], "-23.55");
        assert_eq!(rows[2]["source_reference"], "anp-api|srid=4674");
        assert_eq!(rows[3]["provider"], "ibge-cnefe-2022");
        // No L5 row, never a centroid.
        assert!(rows.iter().all(|row| row["cnpj"] != "33000167000101"));
        let (again, _) = build_locations(&finals, &reviews, &l2, &candidates, &options).unwrap();
        assert_eq!(bytes, again, "deterministic");
    }

    #[test]
    fn locations_withhold_l3_l4_without_owner_decisions() {
        let (finals, reviews, l2, candidates) = ladder_inputs();
        let (bytes, counts) = build_locations(
            &finals,
            &reviews,
            &l2,
            &candidates,
            &CoverageOptions::default(),
        )
        .unwrap();
        assert_eq!(counts.total, 2);
        assert_eq!(counts.anp_reported + counts.address_point, 0);
        let only_d1 = CoverageOptions {
            publish_anp_reported: true,
            ..CoverageOptions::default()
        };
        let (_, counts) = build_locations(&finals, &reviews, &l2, &candidates, &only_d1).unwrap();
        assert_eq!((counts.anp_reported, counts.address_point), (1, 0));
        assert_eq!(rows(&bytes).len(), 2);
    }

    #[test]
    fn locations_refuse_a_tier_without_its_point_or_outside_brazil() {
        let (mut finals, reviews, l2, candidates) = ladder_inputs();
        let options = CoverageOptions {
            publish_anp_reported: true,
            publish_address_point: true,
            ..CoverageOptions::default()
        };
        finals.get_mut("08065008000148").unwrap().api_latitude_deg = None;
        assert!(build_locations(&finals, &reviews, &l2, &candidates, &options).is_err());
        let (mut finals, ..) = ladder_inputs();
        finals.get_mut("08065008000148").unwrap().api_latitude_deg = Some("48.85".to_string());
        assert!(build_locations(&finals, &reviews, &l2, &candidates, &options).is_err());
        let (mut finals, ..) = ladder_inputs();
        finals.get_mut("05938540000134").unwrap().osm_id = Some(8);
        assert!(build_locations(&finals, &reviews, &l2, &candidates, &options).is_err());
    }
}

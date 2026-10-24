//! Frozen map assertion and native-manifest validation.
//!
//! Local files only: no network, database, engine or clock. Any invalid
//! assertion rejects the whole generation so a partial overlay is never
//! published.

use serde::Deserialize;

pub const ASSERTION_FORMAT: &str = "abastevo.map-assertions-v1";
pub const MANIFEST_FORMAT: &str = "abastevo.native-map-manifest-v1";
pub const VERSION: u32 = 1;

#[derive(Debug)]
pub enum MapContractError {
    Invalid(String),
}

impl std::fmt::Display for MapContractError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            MapContractError::Invalid(message) => f.write_str(message),
        }
    }
}

impl std::error::Error for MapContractError {}

type Result<T> = std::result::Result<T, MapContractError>;

fn invalid<T>(message: impl Into<String>) -> Result<T> {
    Err(MapContractError::Invalid(message.into()))
}

#[derive(Debug, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct Corpus {
    pub format: String,
    pub version: u32,
    pub generation: String,
    pub assertions: Vec<Assertion>,
}

#[derive(Debug, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct Assertion {
    pub id: String,
    pub source: String,
    pub kind: String,
    pub name: String,
    pub identities: Identities,
    pub location: Location,
    pub services: Vec<String>,
    pub prices: Vec<Price>,
    pub charging: Option<Charging>,
    pub clocks: Clocks,
    pub rights: Rights,
    pub capabilities: Capabilities,
}

#[derive(Debug, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct Identities {
    pub station_uuid: Option<String>,
    pub cnpj: Option<String>,
    pub place_id: Option<String>,
    pub equipment_id: Option<String>,
    pub connector_id: Option<String>,
    pub osm_type: Option<String>,
    pub osm_id: Option<i64>,
}

#[derive(Debug, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct Location {
    pub latitude_deg: Option<String>,
    pub longitude_deg: Option<String>,
    pub quality: String,
    pub display_point: Option<String>,
    pub entrance_latitude_deg: Option<String>,
    pub entrance_longitude_deg: Option<String>,
}

#[derive(Debug, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct Price {
    pub source: String,
    pub station_uuid: String,
    pub product: String,
    pub unit: String,
    pub amount_milli_brl: i64,
    pub observed_at: String,
    pub period_start: Option<String>,
    pub period_end: Option<String>,
}

#[derive(Debug, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct Charging {
    pub place_id: String,
    pub equipment_id: String,
    pub connector_id: String,
    pub connector_code: String,
    pub power_w: Option<i64>,
    pub access: String,
    pub status: String,
    pub tariff_milli_brl_per_kwh: Option<i64>,
    pub tariff_condition: Option<String>,
    pub observed_at: String,
}

#[derive(Debug, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct Clocks {
    pub observed_at: String,
    pub published_at: String,
    pub fetched_at: String,
}

#[derive(Debug, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct Rights {
    pub source: String,
    pub license: String,
    pub attribution: String,
    pub source_sha256: String,
}

#[derive(Debug, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct Capabilities {
    pub price_known: bool,
    pub pin_eligible: bool,
    pub entrance_known: bool,
    pub live_status: bool,
}

#[derive(Debug, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct NativeManifest {
    pub format: String,
    pub version: u32,
    pub generation: String,
    pub engine: Engine,
    pub region: Region,
    pub packages: Vec<Package>,
    pub dependencies: Vec<String>,
    pub sources: Vec<Source>,
    pub rights: ManifestRights,
    pub generated_at: String,
}

#[derive(Debug, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct Engine {
    pub name: String,
    pub upstream_sha: String,
    pub generator_sha: String,
    pub format_version: String,
}

#[derive(Debug, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct Region {
    pub id: String,
    pub name: String,
    pub bbox: BBox,
    pub overview: String,
}

#[derive(Debug, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct BBox {
    pub min_lon_deg: f64,
    pub min_lat_deg: f64,
    pub max_lon_deg: f64,
    pub max_lat_deg: f64,
}

#[derive(Debug, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct Package {
    pub id: String,
    pub role: String,
    pub path: String,
    pub bytes: i64,
    pub sha256: String,
}

#[derive(Debug, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct Source {
    pub id: String,
    pub license: String,
    pub attribution: String,
    pub sha256: String,
}

#[derive(Debug, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct ManifestRights {
    pub license: String,
    pub attribution: String,
    pub commercial: bool,
    pub offline: bool,
    pub redistribution: bool,
}

pub fn parse_assertions(raw: &[u8]) -> Result<Corpus> {
    let corpus: Corpus = serde_json::from_slice(raw)
        .map_err(|error| MapContractError::Invalid(format!("invalid json: {error}")))?;
    if corpus.format != ASSERTION_FORMAT {
        return invalid("unsupported format");
    }
    if corpus.version != VERSION {
        return invalid("unsupported version");
    }
    if corpus.generation.trim().is_empty() {
        return invalid("generation required");
    }
    if corpus.assertions.is_empty() || corpus.assertions.len() > 10_000 {
        return invalid("assertion count out of bounds");
    }
    let mut seen = std::collections::HashSet::new();
    for assertion in &corpus.assertions {
        validate_assertion(assertion)?;
        if !seen.insert(assertion.id.as_str()) {
            return invalid("duplicate assertion id");
        }
    }
    Ok(corpus)
}

fn validate_assertion(a: &Assertion) -> Result<()> {
    if !uuid(&a.id) {
        return invalid("canonical uuid required");
    }
    if !matches!(a.source.as_str(), "ANP" | "COMMUNITY" | "OSM" | "OCM") {
        return invalid("unknown source");
    }
    if !matches!(
        a.kind.as_str(),
        "FUEL_STATION" | "CHARGING_SITE" | "COMBINED_SITE" | "REFERENCE_POI"
    ) {
        return invalid("unknown kind");
    }
    if a.name.trim().is_empty() || a.name.chars().count() > 200 {
        return invalid("name out of bounds");
    }
    if a.source == "ANP" || a.source == "COMMUNITY" {
        match &a.identities.station_uuid {
            Some(id) if uuid(id) => {}
            _ => return invalid("station uuid required"),
        }
    }
    if let Some(cnpj) = &a.identities.cnpj {
        if !cnpj_ok(cnpj) {
            return invalid("cnpj must be 14 alphanumeric characters");
        }
    }
    for id in [
        &a.identities.station_uuid,
        &a.identities.place_id,
        &a.identities.equipment_id,
        &a.identities.connector_id,
    ]
    .into_iter()
    .flatten()
    {
        if !uuid(id) {
            return invalid("canonical uuid required");
        }
    }
    if a.identities.osm_type.is_some() != a.identities.osm_id.is_some() {
        return invalid("osm type and id travel together");
    }
    if let Some(kind) = &a.identities.osm_type {
        if !matches!(kind.as_str(), "node" | "way" | "relation") {
            return invalid("unknown osm type");
        }
    }
    if !matches!(
        a.location.quality.as_str(),
        "reviewed" | "osm-mapped" | "unknown" | "city-centroid"
    ) {
        return invalid("unknown quality");
    }
    if a.location.latitude_deg.is_some() != a.location.longitude_deg.is_some() {
        return invalid("latitude and longitude travel together");
    }
    if let (Some(lat), Some(lon)) = (&a.location.latitude_deg, &a.location.longitude_deg) {
        let lat = lat
            .parse::<f64>()
            .map_err(|_| MapContractError::Invalid("bad latitude".into()))?;
        let lon = lon
            .parse::<f64>()
            .map_err(|_| MapContractError::Invalid("bad longitude".into()))?;
        if !(-90.0..=90.0).contains(&lat) || !(-180.0..=180.0).contains(&lon) {
            return invalid("coordinate out of range");
        }
    }
    match a.location.quality.as_str() {
        "reviewed" => {
            let point = a.location.display_point.as_deref();
            if a.location.latitude_deg.is_none() || !matches!(point, Some("station" | "entrance")) {
                return invalid("reviewed geometry requires station/entrance display point");
            }
        }
        "osm-mapped" => {
            if !matches!(
                a.location.display_point.as_deref(),
                Some("osm_node" | "osm_centroid")
            ) {
                return invalid("osm geometry requires an osm display point");
            }
        }
        "unknown" => {
            if a.location.latitude_deg.is_some() || a.location.display_point.is_some() {
                return invalid("unknown geometry cannot carry a display point");
            }
        }
        _ => {
            if a.location.display_point.as_deref() != Some("city_centroid") {
                return invalid("city centroid is not an exact pin");
            }
        }
    }
    if a.location.entrance_latitude_deg.is_some() != a.location.entrance_longitude_deg.is_some() {
        return invalid("entrance coordinates travel together");
    }
    if a.location.entrance_latitude_deg.is_some() && a.location.quality != "reviewed" {
        return invalid("entrance requires reviewed geometry");
    }
    if a.services.is_empty() || a.services.len() > 2 {
        return invalid("service count out of bounds");
    }
    let mut services = std::collections::HashSet::new();
    for service in &a.services {
        if !matches!(service.as_str(), "FUEL" | "CHARGING") || !services.insert(service.as_str()) {
            return invalid("unknown or duplicate service");
        }
    }
    if a.prices.len() > 20 {
        return invalid("too many prices");
    }
    let mut price_keys = std::collections::HashSet::new();
    for price in &a.prices {
        if price.source != a.source || !matches!(price.source.as_str(), "ANP" | "COMMUNITY") {
            return invalid("price source must match assertion source");
        }
        if a.identities.station_uuid.as_deref() != Some(price.station_uuid.as_str()) {
            return invalid("price station identity mismatch");
        }
        if !product_unit(&price.product, &price.unit) {
            return invalid("incompatible product/unit");
        }
        if !(1..=1_000_000).contains(&price.amount_milli_brl) {
            return invalid("amount out of range");
        }
        if price.period_start.is_some() != price.period_end.is_some() {
            return invalid("period bounds travel together");
        }
        if price.source == "ANP" && price.period_start.is_none() {
            return invalid("ANP survey period required");
        }
        if price.source == "COMMUNITY" && price.period_start.is_some() {
            return invalid("community projection has no survey period");
        }
        let key = format!("{}|{}|{}", price.source, price.product, price.unit);
        if !price_keys.insert(key) {
            return invalid("duplicate source/product/unit");
        }
    }
    if let Some(charging) = &a.charging {
        if !matches!(a.source.as_str(), "OCM" | "COMMUNITY") {
            return invalid("source cannot publish charging facts");
        }
        for id in [
            &charging.place_id,
            &charging.equipment_id,
            &charging.connector_id,
        ] {
            if !uuid(id) {
                return invalid("canonical charging uuid required");
            }
        }
        if let Some(identity) = &a.identities.place_id {
            if identity != &charging.place_id {
                return invalid("charging place identity mismatch");
            }
        }
        if charging.connector_code.trim().is_empty() {
            return invalid("connector code required");
        }
        if let Some(power) = charging.power_w {
            if !(0..=1_000_000_000).contains(&power) {
                return invalid("power out of range");
            }
        }
        if !matches!(charging.access.as_str(), "public" | "private" | "unknown") {
            return invalid("unknown access");
        }
        if !matches!(
            charging.status.as_str(),
            "unknown" | "available" | "unavailable"
        ) {
            return invalid("unknown status");
        }
        if let Some(tariff) = charging.tariff_milli_brl_per_kwh {
            if !(1..=1_000_000_000).contains(&tariff) {
                return invalid("tariff out of range");
            }
        }
        if charging.tariff_milli_brl_per_kwh.is_none() && charging.tariff_condition.is_some() {
            return invalid("condition without tariff");
        }
    } else if a.source == "OCM" && a.services.iter().any(|s| s == "CHARGING") {
        return invalid("OCM charging site requires connector facts");
    }
    if a.rights.source != a.source || a.rights.license != "ADMITTED" {
        return invalid("rights not admitted for source");
    }
    if a.rights.attribution.trim().is_empty() || !hex64(&a.rights.source_sha256) {
        return invalid("attribution/hash required");
    }
    if a.capabilities.price_known != !a.prices.is_empty() {
        return invalid("price_known must match prices");
    }
    if a.capabilities.live_status {
        return invalid("no admitted live feed");
    }
    let pin = a.location.latitude_deg.is_some()
        && (matches!(
            (
                a.location.quality.as_str(),
                a.location.display_point.as_deref()
            ),
            ("reviewed", Some("station" | "entrance"))
        ) || matches!(
            (
                a.location.quality.as_str(),
                a.location.display_point.as_deref()
            ),
            ("osm-mapped", Some("osm_node" | "osm_centroid"))
        ));
    if a.capabilities.pin_eligible != pin {
        return invalid("pin_eligible must match admitted geometry");
    }
    if a.capabilities.entrance_known
        != (a.location.entrance_latitude_deg.is_some()
            && a.location.entrance_longitude_deg.is_some())
    {
        return invalid("entrance_known must match entrance geometry");
    }
    if [
        &a.clocks.observed_at,
        &a.clocks.published_at,
        &a.clocks.fetched_at,
    ]
    .into_iter()
    .any(|clock| clock.len() < 20)
    {
        return invalid("rfc3339 clocks required");
    }
    Ok(())
}

pub fn parse_manifest(raw: &[u8]) -> Result<NativeManifest> {
    let manifest: NativeManifest = serde_json::from_slice(raw)
        .map_err(|error| MapContractError::Invalid(format!("invalid json: {error}")))?;
    if manifest.format != MANIFEST_FORMAT || manifest.version != VERSION {
        return invalid("unsupported manifest format/version");
    }
    if manifest.engine.name != "comaps"
        || !hex40(&manifest.engine.upstream_sha)
        || !hex40(&manifest.engine.generator_sha)
    {
        return invalid("pinned engine metadata required");
    }
    if manifest.region.bbox.min_lon_deg >= manifest.region.bbox.max_lon_deg
        || manifest.region.bbox.min_lat_deg >= manifest.region.bbox.max_lat_deg
    {
        return invalid("invalid bbox");
    }
    if manifest.packages.is_empty() {
        return invalid("packages required");
    }
    let ids: std::collections::HashSet<&str> =
        manifest.packages.iter().map(|p| p.id.as_str()).collect();
    if ids.len() != manifest.packages.len() {
        return invalid("duplicate package id");
    }
    for package in &manifest.packages {
        if package.path.contains("..") || package.path.starts_with('/') || !hex64(&package.sha256) {
            return invalid("unsafe package path or digest");
        }
        if !matches!(
            package.role.as_str(),
            "REGION" | "OVERVIEW" | "SEARCH" | "ROUTING" | "STYLE" | "FONT"
        ) {
            return invalid("unknown package role");
        }
    }
    for dependency in &manifest.dependencies {
        if !ids.contains(dependency.as_str()) {
            return invalid("dependency is not a listed package");
        }
    }
    if manifest.rights.license != "ODbL-1.0"
        || !manifest.rights.commercial
        || !manifest.rights.offline
        || !manifest.rights.redistribution
        || !manifest
            .sources
            .iter()
            .any(|s| s.license == manifest.rights.license)
    {
        return invalid("commercial offline redistribution rights required");
    }
    Ok(manifest)
}

fn product_unit(product: &str, unit: &str) -> bool {
    match product {
        "CNG" => unit == "M3",
        "LPG_P13" => unit == "KG_13",
        "ETHANOL_HYDRATED"
        | "GASOLINE_REGULAR"
        | "GASOLINE_ADDITIVED"
        | "GASOLINE_PREMIUM_GRADE"
        | "DIESEL_S500"
        | "DIESEL_S10" => unit == "L",
        _ => false,
    }
}

fn uuid(value: &str) -> bool {
    value.len() == 36
        && value.chars().enumerate().all(|(index, ch)| match index {
            8 | 13 | 18 | 23 => ch == '-',
            _ => ch.is_ascii_hexdigit() && !ch.is_ascii_uppercase(),
        })
}

fn cnpj_ok(value: &str) -> bool {
    value.len() == 14 && value.chars().all(|ch| ch.is_ascii_alphanumeric())
}

fn hex40(value: &str) -> bool {
    value.len() == 40
        && value
            .chars()
            .all(|ch| ch.is_ascii_hexdigit() && !ch.is_ascii_uppercase())
}

fn hex64(value: &str) -> bool {
    value.len() == 64
        && value
            .chars()
            .all(|ch| ch.is_ascii_hexdigit() && !ch.is_ascii_uppercase())
}

#[cfg(test)]
mod tests {
    use super::*;

    fn fixture(name: &str) -> Vec<u8> {
        let path = std::path::Path::new(env!("CARGO_MANIFEST_DIR"))
            .join("../../contracts/testdata/map")
            .join(name);
        std::fs::read(path).expect("fixture")
    }

    #[test]
    fn valid_corpus_preserves_ids_units_and_unknowns() {
        let corpus = parse_assertions(&fixture("poi-assertions-v1.json")).expect("valid");
        assert_eq!(corpus.assertions.len(), 5);
        let anp = &corpus.assertions[0];
        assert_eq!(anp.prices[0].amount_milli_brl, 5890);
        assert_eq!(anp.prices[0].unit, "L");
        assert_eq!(anp.identities.cnpj.as_deref(), Some("01122233300018"));
        let ocm = &corpus.assertions[2];
        let charging = ocm.charging.as_ref().expect("charging");
        assert!(charging.power_w.is_none());
        assert!(charging.tariff_milli_brl_per_kwh.is_none());
        assert!(!ocm.capabilities.price_known);
    }

    #[test]
    fn rejects_wrong_unit_overflow_false_pin_and_rights() {
        let raw = fixture("poi-assertions-v1.json");
        let mut doc: serde_json::Value = serde_json::from_slice(&raw).expect("json");
        doc["assertions"][0]["prices"][0]["product"] = serde_json::json!("LPG_P13");
        assert!(parse_assertions(&serde_json::to_vec(&doc).unwrap()).is_err());

        let mut doc: serde_json::Value = serde_json::from_slice(&raw).expect("json");
        doc["assertions"][0]["prices"][0]["amount_milli_brl"] = serde_json::json!(1_000_001);
        assert!(parse_assertions(&serde_json::to_vec(&doc).unwrap()).is_err());

        let mut doc: serde_json::Value = serde_json::from_slice(&raw).expect("json");
        doc["assertions"][3]["location"]["latitude_deg"] = serde_json::json!("-15.79");
        doc["assertions"][3]["location"]["longitude_deg"] = serde_json::json!("-47.88");
        doc["assertions"][3]["location"]["quality"] = serde_json::json!("city-centroid");
        doc["assertions"][3]["location"]["display_point"] = serde_json::json!("station");
        doc["assertions"][3]["capabilities"]["pin_eligible"] = serde_json::json!(true);
        assert!(parse_assertions(&serde_json::to_vec(&doc).unwrap()).is_err());

        let mut doc: serde_json::Value = serde_json::from_slice(&raw).expect("json");
        doc["assertions"][0]["rights"]["source"] = serde_json::json!("COMMUNITY");
        assert!(parse_assertions(&serde_json::to_vec(&doc).unwrap()).is_err());
    }

    #[test]
    fn rejects_unknown_fields_and_duplicate_ids() {
        let raw = fixture("poi-assertions-v1.json");
        let mut doc: serde_json::Value = serde_json::from_slice(&raw).expect("json");
        doc["assertions"][0]["invented"] = serde_json::json!(true);
        assert!(parse_assertions(&serde_json::to_vec(&doc).unwrap()).is_err());

        let mut doc: serde_json::Value = serde_json::from_slice(&raw).expect("json");
        let duplicate = doc["assertions"][0]["id"].clone();
        doc["assertions"][1]["id"] = duplicate;
        assert!(parse_assertions(&serde_json::to_vec(&doc).unwrap()).is_err());
    }

    #[test]
    fn valid_manifest_and_rejections() {
        let raw = fixture("native-map-manifest-v1.json");
        let manifest = parse_manifest(&raw).expect("valid manifest");
        assert_eq!(
            manifest.engine.upstream_sha,
            "4587b8ad4d12c9f59982709fdaf03cc925948827"
        );
        let mut doc: serde_json::Value = serde_json::from_slice(&raw).expect("json");
        doc["dependencies"] = serde_json::json!(["world", "br-sp"]);
        assert!(parse_manifest(&serde_json::to_vec(&doc).unwrap()).is_err());
        let mut doc: serde_json::Value = serde_json::from_slice(&raw).expect("json");
        doc["rights"]["commercial"] = serde_json::json!(false);
        assert!(parse_manifest(&serde_json::to_vec(&doc).unwrap()).is_err());
        let mut doc: serde_json::Value = serde_json::from_slice(&raw).expect("json");
        doc["packages"][0]["path"] = serde_json::json!("../br-df.mwm");
        assert!(parse_manifest(&serde_json::to_vec(&doc).unwrap()).is_err());
    }
}

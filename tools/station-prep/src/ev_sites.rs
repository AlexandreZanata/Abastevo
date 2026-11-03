//! Minimized common source-site model. No cross-source merge, network or database.
use crate::ev_sockets::{current_family, family_for_ocm, power_kw_to_watts};
use crate::types::sha256_hex;
use serde::{Deserialize, Serialize};
use serde_json::{value::RawValue, Value};
use std::collections::{BTreeMap, BTreeSet};

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct SitePoint {
    pub latitude_decimal: String,
    pub longitude_decimal: String,
    pub provenance: String,
}
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct SourceLineage {
    pub source: String,
    pub external_id: String,
    pub revision: String,
    pub record_sha256: String,
    pub object_version: Option<i32>,
    pub clocks: BTreeMap<String, Option<String>>,
    pub provider_id: Option<i64>,
    pub license: String,
    pub attribution: String,
}
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct ConnectorGroup {
    pub key: String,
    pub family: String,
    pub raw_socket: String,
    pub power_w: Option<u64>,
    pub reported_quantity: Option<u64>,
    pub current: String,
    pub raw_status: Option<i64>,
}
/// Closed source-way rings, longitude/latitude decimal strings, never guessed closure.
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct FootprintPolygon {
    pub outer: Vec<[String; 2]>,
    pub holes: Vec<Vec<[String; 2]>>,
}
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct SourceFootprint {
    pub polygons: Vec<FootprintPolygon>,
}
impl SourceFootprint {
    pub fn valid(&self) -> bool {
        if self.polygons.is_empty() || self.polygons.len() > 32 {
            return false;
        }
        let mut points = 0;
        for polygon in &self.polygons {
            for ring in std::iter::once(&polygon.outer).chain(&polygon.holes) {
                points += ring.len();
                if ring.len() < 4 || points > 4096 || ring.first() != ring.last() {
                    return false;
                }
                if ring
                    .iter()
                    .any(|p| point(&p[1], &p[0], "source_ring").is_none())
                {
                    return false;
                }
                let unique: BTreeSet<_> = ring.iter().collect();
                if unique.len() < 3 {
                    return false;
                }
            }
        }
        true
    }
}

#[derive(Debug, Clone, Copy, Serialize, Deserialize, PartialEq, Eq)]
#[serde(rename_all = "SCREAMING_SNAKE_CASE")]
pub enum SiteKind {
    ChargingSite,
    CombinedSite,
    ChargePoint,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct SourceSite {
    pub key: String,
    pub kind: SiteKind,
    pub site_marker: bool,
    pub equipment_marker: bool,
    pub footprint: Option<SourceFootprint>,
    pub source: SourceLineage,
    pub point: SitePoint,
    pub name: Option<String>,
    pub operator: Option<String>,
    pub network: Option<String>,
    #[serde(default)]
    pub brand: Option<String>,
    pub address: BTreeMap<String, String>,
    pub access: String,
    pub raw_access: Option<String>,
    pub fee: Option<bool>,
    pub opening_hours: Option<String>,
    pub status: String,
    pub raw_status: Option<String>,
    pub groups: Vec<ConnectorGroup>,
    /// Explicit source membership only; never assigned by nearest-point guessing.
    pub parent_keys: Vec<String>,
}
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct NormalizationIssue {
    pub key: String,
    pub reason: String,
}
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct NormalizationDecision {
    pub key: Option<String>,
    pub state: String,
    pub reason: Option<String>,
}
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct Normalization {
    pub site: Option<SourceSite>,
    pub decision: NormalizationDecision,
    pub issues: Vec<NormalizationIssue>,
}
impl Normalization {
    pub(crate) fn refused(key: Option<String>, reason: &str) -> Self {
        Self {
            site: None,
            decision: NormalizationDecision {
                key,
                state: "QUARANTINED".into(),
                reason: Some(reason.into()),
            },
            issues: vec![],
        }
    }
    pub(crate) fn accepted(site: SourceSite, mut issues: Vec<NormalizationIssue>) -> Self {
        issues.sort_by(|a, b| a.key.cmp(&b.key).then(a.reason.cmp(&b.reason)));
        issues.dedup();
        Self {
            decision: NormalizationDecision {
                key: Some(site.key.clone()),
                state: "ACCEPTED".into(),
                reason: None,
            },
            site: Some(site),
            issues,
        }
    }
}

#[derive(Debug, Clone)]
struct ProviderRights {
    license: String,
    attribution: String,
}
#[derive(Debug)]
pub struct OcmContext {
    revision: String,
    providers: BTreeMap<i64, ProviderRights>,
    operators: BTreeMap<i64, String>,
}
impl OcmContext {
    /// The caller supplies the separately admitted reference hash/revision.
    /// CLI admission binds those arguments to the frozen national source profile.
    pub fn from_reference(
        raw: &[u8],
        expected_sha256: &str,
        revision: &str,
    ) -> Result<Self, &'static str> {
        if raw.len() > 8 << 20
            || sha256_hex(raw) != expected_sha256
            || revision.is_empty()
            || revision.len() > 128
            || revision.chars().any(char::is_control)
        {
            return Err("reference pin refused");
        }
        #[derive(Deserialize)]
        struct Provider {
            #[serde(rename = "ID")]
            id: i64,
            #[serde(rename = "License")]
            license: Option<String>,
        }
        #[derive(Deserialize)]
        struct Operator {
            #[serde(rename = "ID")]
            id: i64,
            #[serde(rename = "Title")]
            title: Option<String>,
        }
        #[derive(Deserialize)]
        struct Reference {
            #[serde(rename = "DataProviders")]
            providers: Vec<Provider>,
            #[serde(rename = "Operators", default)]
            operators: Vec<Operator>,
        }
        let reference: Reference =
            serde_json::from_slice(raw).map_err(|_| "invalid reference shape")?;
        if reference.providers.len() > 1000 || reference.operators.len() > 10000 {
            return Err("reference census refused");
        }
        let mut reported = BTreeMap::new();
        for p in reference.providers {
            if reported.insert(p.id, p.license).is_some() {
                return Err("duplicate provider identity");
            }
        }
        let profile: Value = serde_json::from_str(include_str!(
            "../../../contracts/ev/national-source-pins-20261103-v1.json"
        ))
        .expect("compiled source admission");
        let mut providers = BTreeMap::new();
        for p in profile["sources"]["OCM"]["providers"]
            .as_array()
            .ok_or("compiled provider shape")?
        {
            let id = p["id"].as_i64().ok_or("compiled provider identity")?;
            let license = p["license"].as_str().ok_or("compiled provider license")?;
            if reported.get(&id).and_then(|v| v.as_deref()) != Some(license)
                || sha256_hex(license.as_bytes())
                    != p["license_sha256"]
                        .as_str()
                        .ok_or("compiled license digest")?
            {
                return Err("provider license changed");
            }
            providers.insert(
                id,
                ProviderRights {
                    license: license.into(),
                    attribution: p["attribution"]
                        .as_str()
                        .ok_or("compiled attribution")?
                        .into(),
                },
            );
        }
        let mut operators = BTreeMap::new();
        let mut seen = BTreeSet::new();
        for operator in reference.operators {
            if !seen.insert(operator.id) {
                return Err("duplicate operator identity");
            }
            if let Some(title) = text(operator.title.as_deref(), 160).filter(|_| operator.id > 0) {
                operators.insert(operator.id, title);
            }
        }
        Ok(Self {
            revision: revision.into(),
            providers,
            operators,
        })
    }
}

pub(crate) fn text(raw: Option<&str>, max: usize) -> Option<String> {
    raw.map(str::trim)
        .filter(|v| !v.is_empty() && v.len() <= max && !v.chars().any(char::is_control))
        .map(String::from)
}
/// Validate a source timestamp without assigning a missing timezone.
pub(crate) fn source_clock(raw: &str) -> Option<String> {
    if raw.len() < 19 || raw.len() > 40 || !raw.is_ascii() {
        return None;
    }
    for (i, b) in raw.as_bytes()[..19].iter().enumerate() {
        let expected = match i {
            4 | 7 => Some(b'-'),
            10 => Some(b'T'),
            13 | 16 => Some(b':'),
            _ => None,
        };
        if expected.map_or_else(|| !b.is_ascii_digit(), |e| *b != e) {
            return None;
        }
    }
    let n = |a, b| raw[a..b].parse::<u32>().unwrap_or(u32::MAX);
    let (year, month, day) = (n(0, 4), n(5, 7), n(8, 10));
    let leap = year.is_multiple_of(4) && (!year.is_multiple_of(100) || year.is_multiple_of(400));
    let days = match month {
        1 | 3 | 5 | 7 | 8 | 10 | 12 => 31,
        4 | 6 | 9 | 11 => 30,
        2 => {
            if leap {
                29
            } else {
                28
            }
        }
        _ => return None,
    };
    if year == 0 || day == 0 || day > days || n(11, 13) > 23 || n(14, 16) > 59 || n(17, 19) > 60 {
        return None;
    }
    let mut suffix = &raw[19..];
    if let Some(fraction) = suffix.strip_prefix('.') {
        let digits = fraction.bytes().take_while(u8::is_ascii_digit).count();
        if !(1..=9).contains(&digits) {
            return None;
        }
        suffix = &fraction[digits..];
    }
    let zone_ok = suffix.is_empty()
        || suffix == "Z"
        || (suffix.len() == 6
            && suffix.starts_with(['+', '-'])
            && suffix.as_bytes()[3] == b':'
            && suffix.as_bytes()[1..3]
                .iter()
                .chain(&suffix.as_bytes()[4..6])
                .all(u8::is_ascii_digit)
            && suffix[1..3].parse::<u8>().is_ok_and(|v| v <= 23)
            && suffix[4..6].parse::<u8>().is_ok_and(|v| v <= 59));
    if !zone_ok {
        return None;
    }
    Some(raw.into())
}

pub(crate) fn point(lat: &str, lon: &str, provenance: &str) -> Option<SitePoint> {
    if lat.len() > 64 || lon.len() > 64 {
        return None;
    }
    let a = lat.parse::<f64>().ok()?;
    let b = lon.parse::<f64>().ok()?;
    if !a.is_finite()
        || !b.is_finite()
        || !(-90.0..=90.0).contains(&a)
        || !(-180.0..=180.0).contains(&b)
    {
        return None;
    }
    Some(SitePoint {
        latitude_decimal: lat.into(),
        longitude_decimal: lon.into(),
        provenance: provenance.into(),
    })
}
fn issue(issues: &mut Vec<NormalizationIssue>, key: &str, reason: &str) {
    issues.push(NormalizationIssue {
        key: key.into(),
        reason: reason.into(),
    });
}

#[derive(Deserialize)]
#[serde(rename_all = "PascalCase")]
struct OcmAddress<'a> {
    #[serde(rename = "CountryID")]
    country_id: Option<i64>,
    #[serde(borrow)]
    latitude: Option<&'a RawValue>,
    #[serde(borrow)]
    longitude: Option<&'a RawValue>,
    title: Option<String>,
    address_line1: Option<String>,
    address_line2: Option<String>,
    town: Option<String>,
    state_or_province: Option<String>,
    postcode: Option<String>,
}
#[derive(Deserialize)]
#[serde(rename_all = "PascalCase")]
struct OcmGroup<'a> {
    #[serde(rename = "ID")]
    id: Option<i64>,
    #[serde(rename = "ConnectionTypeID")]
    connector: Option<i64>,
    #[serde(rename = "CurrentTypeID")]
    current: Option<i64>,
    #[serde(rename = "StatusTypeID")]
    status: Option<i64>,
    #[serde(borrow, rename = "PowerKW")]
    power: Option<&'a RawValue>,
    quantity: Option<Value>,
}
#[derive(Deserialize)]
#[serde(rename_all = "PascalCase")]
struct OcmPoi<'a> {
    #[serde(rename = "ID")]
    id: Option<i64>,
    #[serde(rename = "DataProviderID")]
    provider: Option<i64>,
    #[serde(rename = "UsageTypeID")]
    usage: Option<i64>,
    #[serde(rename = "StatusTypeID")]
    status: Option<i64>,
    #[serde(rename = "OperatorID")]
    operator: Option<i64>,
    #[serde(borrow)]
    address_info: Option<OcmAddress<'a>>,
    #[serde(borrow, default)]
    connections: Option<Vec<OcmGroup<'a>>>,
    date_created: Option<String>,
    date_last_verified: Option<String>,
    date_last_status_update: Option<String>,
}

pub fn normalize_ocm(raw: &[u8], context: &OcmContext) -> Normalization {
    if raw.len() > 1 << 20 {
        return Normalization::refused(None, "source_record_budget");
    }
    let row: OcmPoi<'_> = match serde_json::from_slice(raw) {
        Ok(v) => v,
        Err(_) => return Normalization::refused(None, "invalid_source_shape"),
    };
    let id = match row.id.filter(|v| *v > 0) {
        Some(v) => v,
        None => return Normalization::refused(None, "missing_source_identity"),
    };
    let key = format!("ocm:poi:{id}");
    let refuse = |reason| Normalization::refused(Some(key.clone()), reason);
    let rights = match row.provider.and_then(|p| context.providers.get(&p)) {
        Some(v) => v,
        None => return refuse("unlicensed_provider"),
    };
    if matches!(row.usage, Some(2 | 3 | 6)) {
        return refuse("private_access");
    }
    if matches!(row.status, Some(200 | 210)) {
        return refuse("removed_site");
    }
    if row.status == Some(150) {
        return refuse("planned_site");
    }
    let address = match row.address_info {
        Some(v) => v,
        None => return refuse("missing_address_record"),
    };
    if address.country_id != Some(36) {
        return refuse("foreign_country");
    }
    let coords = address
        .latitude
        .zip(address.longitude)
        .and_then(|(a, b)| point(a.get(), b.get(), "ocm_source_point"));
    let point = match coords {
        Some(v) => v,
        None => return refuse("invalid_geometry"),
    };
    let connections = row.connections.unwrap_or_default();
    if connections.len() > 2000 {
        return refuse("connection_group_budget");
    }
    let mut issues = Vec::new();
    let access = match row.usage {
        Some(1 | 5) => "PUBLIC",
        Some(4) => "MEMBERSHIP",
        Some(7) => "NOTICE_REQUIRED",
        _ => {
            issue(&mut issues, &key, "unknown_access");
            "UNKNOWN"
        }
    };
    let status = match row.status {
        Some(10 | 20 | 50) => "STATIC_REPORTED_OPERATIONAL",
        Some(75) => "STATIC_REPORTED_PARTIAL",
        Some(30) => "STATIC_REPORTED_TEMPORARILY_UNAVAILABLE",
        Some(100) => "STATIC_REPORTED_NOT_OPERATIONAL",
        _ => "UNKNOWN",
    };
    let mut address_fields = BTreeMap::new();
    for (name, raw) in [
        ("street", address.address_line1),
        ("complement", address.address_line2),
        ("municipio", address.town),
        ("postal_code", address.postcode),
    ] {
        if let Some(value) = text(raw.as_deref(), 256) {
            address_fields.insert(name.into(), value);
        }
    }
    if let Some(uf) = text(address.state_or_province.as_deref(), 2)
        .filter(|v| crate::ev_geography::STATES.contains(&v.as_str()))
    {
        address_fields.insert("uf".into(), uf);
    }
    let mut identities = BTreeMap::new();
    for group in &connections {
        if let Some(id) = group.id {
            *identities.entry(id).or_insert(0usize) += 1;
        }
    }
    let mut groups = Vec::new();
    for group in connections {
        let group_id = match group.id.filter(|v| *v > 0) {
            Some(v) => v,
            None => {
                issue(&mut issues, &key, "missing_group_identity");
                continue;
            }
        };
        let group_key = format!("ocm:group:{id}:{group_id}");
        if identities[&group_id] != 1 {
            issue(&mut issues, &group_key, "duplicate_group_identity");
            continue;
        }
        if matches!(group.status, Some(200 | 210)) {
            issue(&mut issues, &group_key, "removed_group");
            continue;
        }
        if group.status == Some(150) {
            issue(&mut issues, &group_key, "planned_group");
            continue;
        }
        let family = family_for_ocm(group.connector.unwrap_or(0));
        if family == "UNKNOWN" {
            issue(&mut issues, &group_key, "unknown_socket");
        }
        let power = group.power.and_then(|v| power_kw_to_watts(v.get()));
        if group.power.is_some() && power.is_none() {
            issue(&mut issues, &group_key, "invalid_power");
        }
        let quantity = group
            .quantity
            .as_ref()
            .and_then(Value::as_u64)
            .filter(|v| (1..=500000).contains(v));
        if group.quantity.is_some() && quantity.is_none() {
            issue(&mut issues, &group_key, "invalid_quantity");
        }
        groups.push(ConnectorGroup {
            key: group_key,
            family: family.into(),
            raw_socket: group.connector.map(|v| v.to_string()).unwrap_or_default(),
            power_w: power,
            reported_quantity: quantity,
            current: current_family(group.current.unwrap_or(0)).into(),
            raw_status: group.status,
        });
    }
    groups.sort_by(|a, b| a.key.cmp(&b.key));
    let mut clocks = BTreeMap::new();
    for (field, value) in [
        ("created_at", row.date_created),
        ("verified_at", row.date_last_verified),
        ("status_at", row.date_last_status_update),
    ] {
        let clock = value.as_deref().and_then(source_clock);
        if value.is_some() && clock.is_none() {
            issue(
                &mut issues,
                &format!("{key}:clock:{field}"),
                "invalid_source_clock",
            );
        }
        clocks.insert(field.into(), clock);
    }
    let source = SourceLineage {
        source: "OCM".into(),
        external_id: id.to_string(),
        revision: context.revision.clone(),
        record_sha256: sha256_hex(raw),
        object_version: None,
        clocks,
        provider_id: row.provider,
        license: rights.license.clone(),
        attribution: rights.attribution.clone(),
    };
    Normalization::accepted(
        SourceSite {
            key,
            kind: SiteKind::ChargingSite,
            site_marker: true,
            equipment_marker: false,
            footprint: None,
            source,
            point,
            name: text(address.title.as_deref(), 160),
            operator: row
                .operator
                .and_then(|id| context.operators.get(&id).cloned()),
            network: None,
            brand: None,
            address: address_fields,
            access: access.into(),
            raw_access: row.usage.map(|v| v.to_string()),
            fee: if row.usage == Some(5) {
                Some(true)
            } else {
                None
            },
            opening_hours: None,
            status: status.into(),
            raw_status: row.status.map(|v| v.to_string()),
            groups,
            parent_keys: vec![],
        },
        issues,
    )
}

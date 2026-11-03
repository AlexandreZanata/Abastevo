//! Pure OSM normalization into the common source model; equipment is not a site.
use crate::ev_sites::{
    point, source_clock, text, ConnectorGroup, Normalization, NormalizationDecision,
    NormalizationIssue, SiteKind, SitePoint, SourceFootprint, SourceLineage, SourceSite,
};
use crate::ev_sockets::{family_for_osm, power_osm_to_watts};
use crate::types::sha256_hex;
use serde::{Deserialize, Serialize};
use std::collections::{BTreeMap, BTreeSet};

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct OsmEvidence {
    pub osm_type: String,
    pub id: i64,
    pub version: Option<i32>,
    pub timestamp: Option<String>,
    pub visible: bool,
    pub tags: BTreeMap<String, String>,
    pub point: Option<SitePoint>,
    pub footprint: Option<SourceFootprint>,
    pub parent_keys: Vec<String>,
}
#[derive(Debug)]
pub struct OsmContext {
    revision: String,
}
impl OsmContext {
    pub(crate) fn revision(&self) -> &str {
        &self.revision
    }

    pub fn new(revision: &str, source_sha256: &str) -> Result<Self, &'static str> {
        if revision.is_empty()
            || revision.len() > 128
            || revision.chars().any(char::is_control)
            || source_sha256.len() != 64
            || !source_sha256.bytes().all(|v| v.is_ascii_hexdigit())
        {
            return Err("OSM source pin refused");
        }
        Ok(Self {
            revision: format!("{revision}@sha256:{source_sha256}"),
        })
    }
}
fn positive_socket(raw: &str) -> bool {
    raw == "yes" || raw.parse::<u64>().is_ok_and(|v| (1..=500000).contains(&v))
}
fn fuel_charging(tags: &BTreeMap<String, String>) -> bool {
    tags.get("fuel:electricity").is_some_and(|v| v == "yes")
        || tags.get("charging_station").is_some_and(|v| v == "yes")
        || tags.iter().any(|(key, value)| {
            key.strip_prefix("socket:").is_some_and(|key| {
                !key.contains(':') && family_for_osm(key) != "UNKNOWN" && positive_socket(value)
            })
        })
}
/// Cheap first-pass filter; lifecycle candidates remain visible to decision counts.
pub fn is_osm_ev_candidate(tags: &BTreeMap<String, String>) -> bool {
    tags.get("amenity")
        .is_some_and(|v| v == "charging_station" || (v == "fuel" && fuel_charging(tags)))
        || tags.get("man_made").is_some_and(|v| v == "charge_point")
        || tags
            .get("construction")
            .is_some_and(|v| ["charging_station", "charge_point"].contains(&v.as_str()))
        || tags.iter().any(|(key, value)| {
            ["disused:", "abandoned:", "construction:"]
                .into_iter()
                .any(|prefix| {
                    key.strip_prefix(prefix).is_some_and(|rest| {
                        (rest == "amenity" && value == "charging_station")
                            || (rest == "man_made" && value == "charge_point")
                    })
                })
        })
}

fn valid_parent(key: &str) -> bool {
    let parts: Vec<_> = key.split(':').collect();
    parts.len() == 3
        && parts[0] == "osm"
        && ["node", "way", "relation"].contains(&parts[1])
        && parts[2].parse::<i64>().is_ok_and(|v| v > 0)
}
fn issue(issues: &mut Vec<NormalizationIssue>, key: &str, reason: &str) {
    issues.push(NormalizationIssue {
        key: key.into(),
        reason: reason.into(),
    });
}

pub fn normalize_osm(row: &OsmEvidence, context: &OsmContext) -> Normalization {
    if row.id <= 0 || !["node", "way", "relation"].contains(&row.osm_type.as_str()) {
        return Normalization::refused(None, "invalid_source_identity");
    }
    let key = format!("osm:{}:{}", row.osm_type, row.id);
    let refuse = |reason| Normalization::refused(Some(key.clone()), reason);
    if !row.visible {
        return refuse("deleted_object");
    }
    if row.tags.len() > 64
        || row.tags.iter().any(|(k, v)| {
            k.is_empty() || k.len() > 256 || v.len() > 256 || k.contains('\0') || v.contains('\0')
        })
    {
        return refuse("tag_budget");
    }
    let tag = |k: &str| row.tags.get(k).map(String::as_str);
    if tag("access")
        .is_some_and(|v| ["private", "no"].contains(&v.trim().to_ascii_lowercase().as_str()))
    {
        return refuse("private_access");
    }
    if row.tags.iter().any(|(k, v)| {
        (k.starts_with("disused:") || k.starts_with("abandoned:"))
            && !["no", "false", "0"].contains(&v.as_str())
    }) || ["disused", "abandoned"]
        .into_iter()
        .any(|k| tag(k) == Some("yes"))
    {
        return refuse("inactive_lifecycle");
    }
    if row.tags.iter().any(|(k, v)| {
        (k == "construction" || k.starts_with("construction:"))
            && !["no", "false", "0"].contains(&v.as_str())
    }) {
        return refuse("construction_site");
    }
    let site_marker = tag("amenity") == Some("charging_station")
        || (tag("amenity") == Some("fuel") && fuel_charging(&row.tags));
    let equipment_marker = tag("man_made") == Some("charge_point");
    let kind = if equipment_marker && (!site_marker || !row.parent_keys.is_empty()) {
        SiteKind::ChargePoint
    } else if tag("amenity") == Some("charging_station") {
        if tag("fuel") == Some("yes") {
            SiteKind::CombinedSite
        } else {
            SiteKind::ChargingSite
        }
    } else if tag("amenity") == Some("fuel") && fuel_charging(&row.tags) {
        SiteKind::CombinedSite
    } else {
        return Normalization {
            site: None,
            decision: NormalizationDecision {
                key: Some(key),
                state: "IGNORED".into(),
                reason: Some("not_charging_site".into()),
            },
            issues: vec![],
        };
    };
    if tag("addr:country").is_some_and(|v| {
        !["BR", "BRASIL", "BRAZIL"].contains(&v.trim().to_ascii_uppercase().as_str())
    }) {
        return refuse("foreign_country");
    }
    let coords = match &row.point {
        None => return refuse("missing_geometry"),
        Some(p) => match point(&p.latitude_decimal, &p.longitude_decimal, &p.provenance) {
            Some(v) => v,
            None => return refuse("invalid_geometry"),
        },
    };
    let provenance_ok = match row.osm_type.as_str() {
        "node" => ["node", "osm_node"].contains(&coords.provenance.as_str()),
        "way" => ["way_centroid", "osm_centroid"].contains(&coords.provenance.as_str()),
        "relation" => ["relation_centroid", "osm_centroid"].contains(&coords.provenance.as_str()),
        _ => false,
    };
    if !provenance_ok {
        return refuse("invalid_geometry_provenance");
    }
    if row.parent_keys.len() > 64
        || row
            .parent_keys
            .iter()
            .any(|p| !valid_parent(p) || p == &key)
    {
        return refuse("invalid_parent_identity");
    }
    if row.footprint.as_ref().is_some_and(|f| !f.valid()) {
        return refuse("invalid_source_footprint");
    }
    let mut issues = vec![];
    if row.osm_type != "node" && row.footprint.is_none() {
        issue(&mut issues, &key, "footprint_unavailable");
    }
    let version = row.version.filter(|v| *v > 0);
    if version.is_none() {
        issue(&mut issues, &key, "missing_object_version");
    }
    let timestamp = row.timestamp.as_deref().and_then(source_clock);
    if row.timestamp.is_some() && timestamp.is_none() {
        issue(&mut issues, &key, "invalid_source_clock");
    }
    let access = match tag("access") {
        Some("yes" | "public") => "PUBLIC",
        Some("permissive") => "PERMISSIVE",
        Some("customers") => "CUSTOMERS",
        Some("destination") => "DESTINATION",
        Some(_) => {
            issue(&mut issues, &key, "unknown_access");
            "UNKNOWN"
        }
        None => "UNKNOWN",
    };
    let fee = match tag("fee") {
        Some("yes") => Some(true),
        Some("no") => Some(false),
        Some(_) => {
            issue(&mut issues, &key, "unknown_fee");
            None
        }
        None => None,
    };
    let mut address = BTreeMap::new();
    for (out, input) in [
        ("street", "addr:street"),
        ("number", "addr:housenumber"),
        ("complement", "addr:unit"),
        ("district", "addr:suburb"),
        ("municipio", "addr:city"),
        ("postal_code", "addr:postcode"),
    ] {
        if let Some(value) = text(tag(input), 256) {
            address.insert(out.into(), value);
        }
    }
    if let Some(uf) = tag("addr:state")
        .map(str::to_ascii_uppercase)
        .filter(|v| crate::ev_geography::STATES.contains(&v.as_str()))
    {
        address.insert("uf".into(), uf);
    }
    let mut groups = vec![];
    for (key_name, value) in &row.tags {
        let Some(socket) = key_name
            .strip_prefix("socket:")
            .filter(|v| !v.contains(':'))
        else {
            continue;
        };
        let group_key = format!("{key}:socket:{socket}");
        if socket.is_empty()
            || socket.len() > 64
            || !socket
                .bytes()
                .all(|v| v.is_ascii_alphanumeric() || v == b'_')
        {
            issue(&mut issues, &key, "invalid_socket_key");
            continue;
        }
        if ["no", "0"].contains(&value.as_str()) {
            continue;
        }
        if !positive_socket(value) {
            issue(&mut issues, &group_key, "invalid_socket_presence");
            continue;
        }
        let family = family_for_osm(socket);
        if family == "UNKNOWN" {
            issue(&mut issues, &group_key, "unknown_socket");
        }
        let raw_power = row.tags.get(&format!("socket:{socket}:output"));
        let power = raw_power.and_then(|v| power_osm_to_watts(v));
        if raw_power.is_some() && power.is_none() {
            issue(&mut issues, &group_key, "invalid_power");
        }
        groups.push(ConnectorGroup {
            key: group_key,
            family: family.into(),
            raw_socket: socket.into(),
            power_w: power,
            reported_quantity: value.parse::<u64>().ok(),
            current: "UNKNOWN".into(),
            raw_status: None,
        });
    }
    groups.sort_by(|a, b| a.key.cmp(&b.key));
    let mut canonical_evidence = row.clone();
    canonical_evidence.parent_keys.sort();
    canonical_evidence.parent_keys.dedup();
    let source_bytes = match serde_json::to_vec(&canonical_evidence) {
        Ok(v) => v,
        Err(_) => return refuse("invalid_source_shape"),
    };
    let source = SourceLineage {
        source: "OSM".into(),
        external_id: format!("{}:{}", row.osm_type, row.id),
        revision: context.revision().into(),
        record_sha256: sha256_hex(&source_bytes),
        object_version: version,
        clocks: BTreeMap::from([("object_at".into(), timestamp)]),
        provider_id: None,
        license: "ODbL-1.0".into(),
        attribution: "© OpenStreetMap contributors (ODbL)".into(),
    };
    let parents: BTreeSet<_> = row.parent_keys.iter().cloned().collect();
    Normalization::accepted(
        SourceSite {
            key,
            kind,
            site_marker,
            equipment_marker,
            footprint: row.footprint.clone(),
            source,
            point: coords,
            name: text(tag("name"), 160),
            operator: text(tag("operator"), 160),
            network: text(tag("network"), 160),
            brand: text(tag("brand"), 160),
            address,
            access: access.into(),
            raw_access: text(tag("access"), 64),
            fee,
            opening_hours: text(tag("opening_hours"), 256),
            status: "UNKNOWN".into(),
            raw_status: None,
            groups,
            parent_keys: parents.into_iter().collect(),
        },
        issues,
    )
}

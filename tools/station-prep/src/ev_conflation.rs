//! Pure bounded one-to-one source conflation; no inferred connector totals.
use crate::ev_hierarchy::{
    resolve_hierarchy, EquipmentAssignment, HierarchyReview, SiteWithEquipment,
};
use crate::ev_sites::{SiteKind, SitePoint, SourceSite};
use crate::location_review::{haversine_m, EARTH_RADIUS_M};
use crate::municipality::normalize_name;
use crate::types::sha256_hex;
use serde::{Deserialize, Serialize};
use std::collections::{BTreeMap, BTreeSet};
use std::sync::OnceLock;
const CELL_M: f64 = 150.0;
const EDGE_EPS_M: f64 = 1e-7; // Numerical boundary tolerance, never a rounded-distance band.
const MAX_COMPARISONS: usize = 2_000_000;
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct FieldSource {
    pub source_key: String,
    pub field: String,
}
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct MatchEvidence {
    pub distance_mm: u64,
    pub shared_families: Vec<String>,
}
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct CanonicalSite {
    pub key: String,
    pub kind: SiteKind,
    pub point: SitePoint,
    pub name: Option<String>,
    pub operator: Option<String>,
    pub network: Option<String>,
    pub brand: Option<String>,
    pub address: BTreeMap<String, String>,
    pub field_sources: BTreeMap<String, FieldSource>,
    pub sources: Vec<SourceSite>,
    pub equipment: Vec<SourceSite>,
    pub confidence: String,
    pub evidence: Option<MatchEvidence>,
}
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct PairReview {
    pub osm_key: String,
    pub ocm_key: String,
    pub distance_mm: u64,
    pub reason: String,
}
#[derive(Debug, Clone, Default, Serialize, Deserialize, PartialEq, Eq)]
pub struct Stats {
    pub input_records: usize,
    pub source_roots: usize,
    pub grouped_equipment: usize,
    pub unresolved_records: usize,
    pub confirmed_pairs: usize,
    pub osm_only: usize,
    pub ocm_only: usize,
    pub probable_pairs: usize,
    pub conflict_pairs: usize,
    pub candidate_comparisons: usize,
}
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct Conflation {
    pub sites: Vec<CanonicalSite>,
    pub review: Vec<PairReview>,
    pub conflicts: Vec<PairReview>,
    pub hierarchy_review: Vec<HierarchyReview>,
    pub assignments: Vec<EquipmentAssignment>,
    pub stats: Stats,
}
#[derive(Debug)]
struct Edge {
    osm: usize,
    ocm: usize,
    distance: f64,
    shared: Vec<String>,
}
fn position(s: &SourceSite) -> (f64, f64) {
    (
        s.point.latitude_decimal.parse().expect("validated point"),
        s.point.longitude_decimal.parse().expect("validated point"),
    )
}
fn cell(s: &SourceSite) -> (i32, i32, i32) {
    let (a, b) = position(s);
    let (lat, lon) = (a.to_radians(), b.to_radians());
    (
        (EARTH_RADIUS_M * lat.cos() * lon.cos() / CELL_M).floor() as i32,
        (EARTH_RADIUS_M * lat.cos() * lon.sin() / CELL_M).floor() as i32,
        (EARTH_RADIUS_M * lat.sin() / CELL_M).floor() as i32,
    )
}
fn known_family(value: &str) -> bool {
    static KNOWN: OnceLock<BTreeSet<String>> = OnceLock::new();
    KNOWN
        .get_or_init(|| {
            let profile: serde_json::Value = serde_json::from_str(include_str!(
                "../../../contracts/ev/socket-families-v1.json"
            ))
            .expect("compiled socket profile");
            profile["families"]
                .as_array()
                .expect("compiled families")
                .iter()
                .map(|v| {
                    v["family"]
                        .as_str()
                        .expect("compiled family name")
                        .to_string()
                })
                .collect()
        })
        .contains(value)
}
fn families(s: &SiteWithEquipment) -> BTreeSet<String> {
    std::iter::once(&s.site)
        .chain(&s.equipment)
        .flat_map(|r| r.groups.iter())
        .filter(|g| known_family(&g.family))
        .map(|g| g.family.clone())
        .collect()
}
fn labels(s: &SourceSite) -> BTreeSet<String> {
    [
        s.operator.as_deref(),
        s.network.as_deref(),
        s.name.as_deref(),
    ]
    .into_iter()
    .flatten()
    .map(|v| {
        normalize_name(v)
            .chars()
            .map(|c| if c.is_alphanumeric() { c } else { ' ' })
            .collect::<String>()
            .split_whitespace()
            .collect::<Vec<_>>()
            .join(" ")
    })
    .filter(|v| !v.is_empty())
    .collect()
}
fn text_field(s: &SourceSite, field: &str) -> Option<String> {
    match field {
        "name" => s.name.clone(),
        "operator" => s.operator.clone(),
        "network" => s.network.clone(),
        "brand" => s.brand.clone(),
        _ => None,
    }
    .filter(|v| !v.trim().is_empty())
}
fn selected(
    candidates: &[(&SourceSite, &str)],
    target: &str,
    lineage: &mut BTreeMap<String, FieldSource>,
) -> Option<String> {
    for (source, field) in candidates {
        if let Some(value) = text_field(source, field) {
            lineage.insert(
                target.into(),
                FieldSource {
                    source_key: source.key.clone(),
                    field: (*field).into(),
                },
            );
            return Some(value);
        }
    }
    None
}
fn canonical(
    primary: &SiteWithEquipment,
    secondary: Option<&SiteWithEquipment>,
    evidence: Option<MatchEvidence>,
) -> CanonicalSite {
    let a = &primary.site;
    let b = secondary.map(|s| &s.site);
    let mut lineage = BTreeMap::from([(
        "point".into(),
        FieldSource {
            source_key: a.key.clone(),
            field: "point".into(),
        },
    )]);
    let basic = |field: &'static str| {
        std::iter::once((a, field))
            .chain(b.map(|s| (s, field)))
            .collect::<Vec<_>>()
    };
    let name = selected(&basic("name"), "name", &mut lineage);
    let operator = selected(
        &std::iter::once((a, "operator"))
            .chain(if a.source.source == "OSM" {
                Some((a, "network"))
            } else {
                None
            })
            .chain(b.map(|s| (s, "operator")))
            .collect::<Vec<_>>(),
        "operator",
        &mut lineage,
    );
    let network = selected(&basic("network"), "network", &mut lineage);
    let brand = selected(&basic("brand"), "brand", &mut lineage);
    let candidates: Vec<_> = std::iter::once(a).chain(b).collect();
    let address_source = candidates
        .iter()
        .find(|s| {
            s.address
                .get("street")
                .is_some_and(|v| !v.trim().is_empty())
        })
        .or_else(|| candidates.iter().find(|s| !s.address.is_empty()));
    let address = if let Some(source) = address_source {
        lineage.insert(
            "address".into(),
            FieldSource {
                source_key: source.key.clone(),
                field: "address".into(),
            },
        );
        source.address.clone()
    } else {
        BTreeMap::new()
    };
    let mut sources = vec![a.clone()];
    if let Some(s) = b {
        sources.push(s.clone());
    }
    sources.sort_by(|a, b| a.key.cmp(&b.key));
    let mut equipment = primary.equipment.clone();
    if let Some(s) = secondary {
        equipment.extend(s.equipment.iter().cloned());
    }
    equipment.sort_by(|a, b| a.key.cmp(&b.key));
    CanonicalSite {
        key: format!(
            "ev-site-v1:{}",
            sha256_hex(format!("abastevo:ev-site:v1\0{}", a.key).as_bytes())
        ),
        kind: a.kind,
        point: a.point.clone(),
        name,
        operator,
        network,
        brand,
        address,
        field_sources: lineage,
        sources,
        equipment,
        confidence: if evidence.is_some() {
            "two-source"
        } else {
            "single-source"
        }
        .into(),
        evidence,
    }
}
fn pair(
    edge: &Edge,
    osm: &[&SiteWithEquipment],
    ocm: &[&SiteWithEquipment],
    reason: &str,
) -> PairReview {
    PairReview {
        osm_key: osm[edge.osm].site.key.clone(),
        ocm_key: ocm[edge.ocm].site.key.clone(),
        distance_mm: (edge.distance * 1000.0).round() as u64,
        reason: reason.into(),
    }
}

pub fn conflate(input: &[SourceSite]) -> Result<Conflation, &'static str> {
    for source in input {
        if !["OSM", "OCM"].contains(&source.source.source.as_str()) {
            return Err("unknown source namespace");
        }
        if ["PRIVATE", "NO"].contains(&source.access.to_ascii_uppercase().as_str())
            || source.source.source == "OCM"
                && source
                    .raw_access
                    .as_deref()
                    .is_some_and(|v| ["2", "3", "6"].contains(&v))
        {
            return Err("private source input refused");
        }
        if source
            .raw_status
            .as_deref()
            .is_some_and(|v| ["150", "200", "210"].contains(&v))
            || ["REMOVED", "DECOMMISSIONED", "PLANNED"].contains(&source.status.as_str())
        {
            return Err("inactive source input refused");
        }
        if source.kind == SiteKind::ChargePoint && !source.equipment_marker
            || source.kind != SiteKind::ChargePoint && !source.site_marker
        {
            return Err("inconsistent source role");
        }
    }
    let hierarchy = resolve_hierarchy(input)?;
    let osm: Vec<_> = hierarchy
        .sites
        .iter()
        .filter(|s| s.site.source.source == "OSM")
        .collect();
    let ocm: Vec<_> = hierarchy
        .sites
        .iter()
        .filter(|s| s.site.source.source == "OCM")
        .collect();
    let mut grid: BTreeMap<(i32, i32, i32), Vec<usize>> = BTreeMap::new();
    for (i, s) in ocm.iter().enumerate() {
        grid.entry(cell(&s.site)).or_default().push(i);
    }
    let of: Vec<_> = osm.iter().map(|s| families(s)).collect();
    let cf: Vec<_> = ocm.iter().map(|s| families(s)).collect();
    let ol: Vec<_> = osm.iter().map(|s| labels(&s.site)).collect();
    let cl: Vec<_> = ocm.iter().map(|s| labels(&s.site)).collect();
    let mut confirmed = Vec::new();
    let mut probable = Vec::new();
    let mut conflicts = Vec::new();
    let mut comparisons = 0;
    for (i, s) in osm.iter().enumerate() {
        let (x, y, z) = cell(&s.site);
        let (lat, lon) = position(&s.site);
        for dx in -1..=1 {
            for dy in -1..=1 {
                for dz in -1..=1 {
                    for j in grid.get(&(x + dx, y + dy, z + dz)).into_iter().flatten() {
                        comparisons += 1;
                        if comparisons > MAX_COMPARISONS {
                            return Err("cross-source candidate budget exceeded");
                        }
                        let (a, b) = position(&ocm[*j].site);
                        let distance = haversine_m(lat, lon, a, b);
                        if !distance.is_finite() {
                            return Err("invalid source distance");
                        }
                        if distance > 150.0 + EDGE_EPS_M {
                            continue;
                        }
                        let shared: Vec<_> = of[i].intersection(&cf[*j]).cloned().collect();
                        let edge = Edge {
                            osm: i,
                            ocm: *j,
                            distance,
                            shared,
                        };
                        if distance <= 50.0 + EDGE_EPS_M {
                            if edge.shared.is_empty() {
                                conflicts.push(edge);
                            } else {
                                confirmed.push(edge);
                            }
                        } else if !ol[i].is_disjoint(&cl[*j]) {
                            probable.push(edge);
                        }
                    }
                }
            }
        }
    }
    confirmed.sort_by(|a, b| {
        a.distance
            .total_cmp(&b.distance)
            .then(osm[a.osm].site.key.cmp(&osm[b.osm].site.key))
            .then(ocm[a.ocm].site.key.cmp(&ocm[b.ocm].site.key))
    });
    let mut used_osm = BTreeSet::new();
    let mut used_ocm = BTreeSet::new();
    let mut sites = Vec::new();
    let mut review = Vec::new();
    for edge in confirmed {
        if used_osm.contains(&edge.osm) || used_ocm.contains(&edge.ocm) {
            review.push(pair(&edge, &osm, &ocm, "one_to_one_candidate_consumed"));
            continue;
        }
        used_osm.insert(edge.osm);
        used_ocm.insert(edge.ocm);
        sites.push(canonical(
            osm[edge.osm],
            Some(ocm[edge.ocm]),
            Some(MatchEvidence {
                distance_mm: (edge.distance * 1000.0).round() as u64,
                shared_families: edge.shared,
            }),
        ));
    }
    let pairs = sites.len();
    let osm_only = osm.len() - used_osm.len();
    let ocm_only = ocm.len() - used_ocm.len();
    for (i, s) in osm.iter().enumerate() {
        if !used_osm.contains(&i) {
            sites.push(canonical(s, None, None));
        }
    }
    for (i, s) in ocm.iter().enumerate() {
        if !used_ocm.contains(&i) {
            sites.push(canonical(s, None, None));
        }
    }
    let probable_count = probable.len();
    for edge in probable {
        review.push(pair(&edge, &osm, &ocm, "probable_same_label"));
    }
    let mut conflict_rows: Vec<_> = conflicts
        .iter()
        .map(|e| pair(e, &osm, &ocm, "socket_conflict"))
        .collect();
    let sort = |a: &PairReview, b: &PairReview| {
        a.osm_key
            .cmp(&b.osm_key)
            .then(a.ocm_key.cmp(&b.ocm_key))
            .then(a.reason.cmp(&b.reason))
    };
    review.sort_by(sort);
    conflict_rows.sort_by(sort);
    sites.sort_by(|a, b| a.key.cmp(&b.key));
    let stats = Stats {
        input_records: input.len(),
        source_roots: hierarchy.sites.len(),
        grouped_equipment: hierarchy.assignments.len(),
        unresolved_records: input.len() - hierarchy.sites.len() - hierarchy.assignments.len(),
        confirmed_pairs: pairs,
        osm_only,
        ocm_only,
        probable_pairs: probable_count,
        conflict_pairs: conflict_rows.len(),
        candidate_comparisons: comparisons,
    };
    Ok(Conflation {
        sites,
        review,
        conflicts: conflict_rows,
        hierarchy_review: hierarchy.review,
        assignments: hierarchy.assignments,
        stats,
    })
}

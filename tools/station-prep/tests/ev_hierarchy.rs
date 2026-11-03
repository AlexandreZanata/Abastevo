use station_prep::ev_hierarchy::resolve_hierarchy;
use station_prep::ev_sites::{
    FootprintPolygon, SiteKind, SitePoint, SourceFootprint, SourceLineage, SourceSite,
};
use std::collections::BTreeMap;
fn site(id: i64, kind: SiteKind) -> SourceSite {
    SourceSite {
        key: format!("osm:node:{id}"),
        kind,
        site_marker: kind != SiteKind::ChargePoint,
        equipment_marker: kind == SiteKind::ChargePoint,
        footprint: None,
        source: SourceLineage {
            source: "OSM".into(),
            external_id: format!("node:{id}"),
            revision: "synthetic-revision".into(),
            record_sha256: "a".repeat(64),
            object_version: Some(1),
            clocks: BTreeMap::new(),
            provider_id: None,
            license: "ODbL-1.0".into(),
            attribution: "© OpenStreetMap contributors (ODbL)".into(),
        },
        point: SitePoint {
            latitude_decimal: "-20".into(),
            longitude_decimal: "-50".into(),
            provenance: "osm_node".into(),
        },
        name: Some("[P25-TEST] synthetic site".into()),
        operator: None,
        network: None,
        brand: None,
        address: BTreeMap::new(),
        access: "PUBLIC".into(),
        raw_access: Some("yes".into()),
        fee: None,
        opening_hours: None,
        status: "UNKNOWN".into(),
        raw_status: None,
        groups: vec![],
        parent_keys: vec![],
    }
}
fn ring(x: f64, y: f64, d: f64) -> Vec<[String; 2]> {
    [(x, y), (x + d, y), (x + d, y + d), (x, y + d), (x, y)]
        .into_iter()
        .map(|(x, y)| [x.to_string(), y.to_string()])
        .collect()
}
fn area(id: i64) -> SourceSite {
    let mut s = site(id, SiteKind::ChargingSite);
    s.key = format!("osm:way:{id}");
    s.source.external_id = format!("way:{id}");
    s.point.provenance = "way_centroid".into();
    s.footprint = Some(SourceFootprint {
        polygons: vec![FootprintPolygon {
            outer: ring(-50.001, -20.001, 0.002),
            holes: vec![],
        }],
    });
    s
}
#[test]
fn explicit_parent_and_containment_keep_equipment_out_of_site_count() {
    let parent = area(90001);
    let mut a = site(90002, SiteKind::ChargePoint);
    a.parent_keys = vec![parent.key.clone()];
    let b = site(90003, SiteKind::ChargePoint);
    let out = resolve_hierarchy(&[parent, a, b]).unwrap();
    assert_eq!(out.sites.len(), 1);
    assert_eq!(out.sites[0].equipment.len(), 2);
    assert!(out.review.is_empty());
}
#[test]
fn holes_boundaries_ambiguity_and_unparented_equipment_require_review() {
    let mut parent = area(90001);
    parent.footprint.as_mut().unwrap().polygons[0].holes = vec![ring(-50.0005, -20.0005, 0.001)];
    let child = site(90002, SiteKind::ChargePoint);
    let out = resolve_hierarchy(&[parent, child]).unwrap();
    assert_eq!(out.sites.len(), 1);
    assert_eq!(out.review[0].reason, "unparented_equipment");
    let parent = area(90001);
    let mut edge = site(90002, SiteKind::ChargePoint);
    edge.point.longitude_decimal = "-50.001".into();
    let out = resolve_hierarchy(&[parent.clone(), edge]).unwrap();
    assert_eq!(out.review[0].reason, "parent_boundary_ambiguous");
    let other = area(90003);
    let child = site(90002, SiteKind::ChargePoint);
    let out = resolve_hierarchy(&[parent, other, child]).unwrap();
    assert_eq!(out.review[0].reason, "ambiguous_parent");
    assert_eq!(out.sites.len(), 2);
}
#[test]
fn chains_cycles_and_dual_markers_are_deterministic() {
    let root = area(90001);
    let mut intermediate = site(90002, SiteKind::ChargePoint);
    intermediate.site_marker = true;
    intermediate.parent_keys = vec![root.key.clone()];
    let mut child = site(90003, SiteKind::ChargePoint);
    child.parent_keys = vec![intermediate.key.clone()];
    let input = vec![root, intermediate.clone(), child.clone()];
    let first = resolve_hierarchy(&input).unwrap();
    assert_eq!(first.sites.len(), 1);
    assert_eq!(first.sites[0].equipment.len(), 2);
    let mut reversed = input;
    reversed.reverse();
    assert_eq!(first, resolve_hierarchy(&reversed).unwrap());
    intermediate.parent_keys = vec![child.key.clone()];
    child.site_marker = true;
    let out = resolve_hierarchy(&[intermediate, child]).unwrap();
    assert!(out.sites.is_empty());
    assert!(out.review.iter().all(|r| r.reason == "parent_cycle"));
    let mut dual = site(90004, SiteKind::ChargingSite);
    dual.equipment_marker = true;
    let out = resolve_hierarchy(&[dual]).unwrap();
    assert_eq!(out.sites.len(), 1);
}
#[test]
fn invalid_points_and_missing_declared_parents_never_get_nearest_fallback() {
    let parent = area(90001);
    let mut child = site(90002, SiteKind::ChargePoint);
    child.parent_keys = vec!["osm:node:99999".into()];
    let out = resolve_hierarchy(&[parent, child]).unwrap();
    assert_eq!(out.review[0].reason, "missing_declared_parent");
    assert!(out.sites[0].equipment.is_empty());
    let mut bad = site(90003, SiteKind::ChargingSite);
    bad.point.latitude_decimal = "NaN".into();
    assert!(resolve_hierarchy(&[bad]).is_err());
}

#[test]
fn derived_parent_basis_is_separate_from_unchanged_source_assertions() {
    let parent = area(90001);
    let mut explicit = site(90002, SiteKind::ChargePoint);
    explicit.parent_keys = vec![parent.key.clone()];
    let contained = site(90003, SiteKind::ChargePoint);
    let out = resolve_hierarchy(&[parent, explicit, contained]).unwrap();
    assert_eq!(out.assignments.len(), 2);
    assert_eq!(out.assignments[0].basis, "source_membership");
    assert_eq!(out.assignments[1].basis, "source_footprint");
    assert!(out.sites[0].equipment[1].parent_keys.is_empty());
}
#[test]
fn invalid_parent_topology_and_partial_child_footprints_cannot_claim_containment() {
    let mut parent = area(90001);
    parent.footprint.as_mut().unwrap().polygons[0].outer = vec![
        [-50.001f64, -20.001f64],
        [-49.999, -19.999],
        [-49.999, -20.001],
        [-50.001, -19.999],
        [-50.001, -20.001],
    ]
    .into_iter()
    .map(|p| [p[0].to_string(), p[1].to_string()])
    .collect();
    let out = resolve_hierarchy(&[parent, site(90002, SiteKind::ChargePoint)]).unwrap();
    assert!(out
        .review
        .iter()
        .any(|r| r.reason == "invalid_parent_geometry"));
    assert!(out.sites[0].equipment.is_empty());
    let parent = area(90001);
    let mut child = site(90002, SiteKind::ChargePoint);
    child.footprint = Some(SourceFootprint {
        polygons: vec![FootprintPolygon {
            outer: ring(-50.0005, -20.0005, 0.002),
            holes: vec![],
        }],
    });
    let out = resolve_hierarchy(&[parent, child]).unwrap();
    assert!(out.sites[0].equipment.is_empty());
    assert_eq!(out.review[0].reason, "unparented_equipment");
}

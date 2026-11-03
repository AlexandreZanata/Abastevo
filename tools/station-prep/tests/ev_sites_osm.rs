use station_prep::ev_osm_sites::{normalize_osm, OsmContext, OsmEvidence};
use station_prep::ev_sites::{SiteKind, SitePoint};
use std::collections::BTreeMap;
fn context() -> OsmContext {
    OsmContext::new("synthetic-pbf-revision", &"a".repeat(64)).unwrap()
}
fn evidence(pairs: &[(&str, &str)]) -> OsmEvidence {
    OsmEvidence {
        osm_type: "node".into(),
        id: 90001,
        version: Some(3),
        timestamp: Some("2026-01-01T00:00:00Z".into()),
        visible: true,
        tags: pairs
            .iter()
            .map(|(k, v)| (k.to_string(), v.to_string()))
            .collect(),
        point: Some(SitePoint {
            latitude_decimal: "-20.125".into(),
            longitude_decimal: "-50.25".into(),
            provenance: "osm_node".into(),
        }),
        footprint: None,
        parent_keys: vec![],
    }
}
#[test]
fn osm_uses_common_model_without_name_address_or_quantity_fabrication() {
    let row = evidence(&[
        ("amenity", "charging_station"),
        ("socket:type2", "yes"),
        ("socket:type2:output", "22 kW"),
        ("socket:type2_combo", "2"),
        ("socket:type2_combo:output", "50;150 kW"),
        ("socket:unknown", "1"),
        ("socket:chademo", "no"),
        ("operator", "[P25-TEST] operator"),
        ("contact:email", "synthetic-private-email@invalid.test"),
        ("phone", "synthetic-private-phone"),
    ]);
    let out = normalize_osm(&row, &context());
    let site = out.site.unwrap();
    assert_eq!(site.kind, SiteKind::ChargingSite);
    assert_eq!(site.source.object_version, Some(3));
    assert_eq!(site.source.license, "ODbL-1.0");
    assert!(site.name.is_none());
    assert!(site.address.is_empty());
    let type2 = site.groups.iter().find(|g| g.family == "TYPE2").unwrap();
    assert!(type2.reported_quantity.is_none());
    assert_eq!(type2.power_w, Some(22000));
    let ccs = site.groups.iter().find(|g| g.family == "CCS2").unwrap();
    assert_eq!(ccs.reported_quantity, Some(2));
    assert!(ccs.power_w.is_none());
    assert_eq!(site.groups.len(), 3);
    let serialized = serde_json::to_string(&site).unwrap();
    assert!(!serialized.contains("synthetic-private"));
    assert!(out.issues.iter().any(|x| x.reason == "invalid_power"));
}
#[test]
fn private_deleted_lifecycle_and_missing_geometry_never_produce_a_site() {
    for (tags, reason) in [
        (vec![("access", "private")], "private_access"),
        (vec![("access", "no")], "private_access"),
        (
            vec![("disused:amenity", "charging_station")],
            "inactive_lifecycle",
        ),
        (
            vec![("abandoned:amenity", "charging_station")],
            "inactive_lifecycle",
        ),
        (
            vec![("construction", "charging_station")],
            "construction_site",
        ),
    ] {
        let mut row = evidence(&[("amenity", "charging_station")]);
        row.tags
            .extend(tags.into_iter().map(|(k, v)| (k.into(), v.into())));
        let out = normalize_osm(&row, &context());
        assert!(out.site.is_none());
        assert_eq!(out.decision.reason.as_deref(), Some(reason));
        assert!(!serde_json::to_string(&out).unwrap().contains("latitude"));
    }
    let mut row = evidence(&[("amenity", "charging_station")]);
    row.point = None;
    assert_eq!(
        normalize_osm(&row, &context()).decision.reason.as_deref(),
        Some("missing_geometry")
    );
    row.visible = false;
    assert_eq!(
        normalize_osm(&row, &context()).decision.reason.as_deref(),
        Some("deleted_object")
    );
}
#[test]
fn fuel_evidence_and_child_equipment_remain_distinct_units() {
    let fuel = evidence(&[("amenity", "fuel")]);
    let out = normalize_osm(&fuel, &context());
    assert!(out.site.is_none());
    assert_eq!(out.decision.state, "IGNORED");
    let combined = evidence(&[("amenity", "fuel"), ("fuel:electricity", "yes")]);
    assert_eq!(
        normalize_osm(&combined, &context()).site.unwrap().kind,
        SiteKind::CombinedSite
    );
    let mut child = evidence(&[("man_made", "charge_point"), ("socket:type2", "2")]);
    child.parent_keys = vec!["osm:way:90002".into()];
    let site = normalize_osm(&child, &context()).site.unwrap();
    assert_eq!(site.kind, SiteKind::ChargePoint);
    assert_eq!(site.parent_keys, vec!["osm:way:90002"]);
    child.parent_keys = vec!["ocm:poi:90002".into()];
    assert_eq!(
        normalize_osm(&child, &context()).decision.reason.as_deref(),
        Some("invalid_parent_identity")
    );
}
#[test]
fn osm_names_addresses_unknowns_and_order_are_source_bound() {
    let mut row = evidence(&[
        ("amenity", "charging_station"),
        ("name", "[P25-TEST] source name"),
        ("addr:street", "[P25-TEST] source street"),
        ("addr:housenumber", "90001"),
        ("addr:city", "[P25-TEST] source city"),
        ("addr:state", "SP"),
        ("fee", "no"),
        ("socket:type2", "unknown"),
        ("socket:future", "yes"),
        ("opening_hours", "24/7"),
    ]);
    row.version = None;
    row.timestamp = Some("synthetic-private-clock@invalid.test".into());
    let out = normalize_osm(&row, &context());
    let site = out.site.unwrap();
    assert_eq!(site.name.as_deref(), Some("[P25-TEST] source name"));
    assert_eq!(site.address["street"], "[P25-TEST] source street");
    assert_eq!(site.address["uf"], "SP");
    assert_eq!(site.fee, Some(false));
    assert_eq!(site.opening_hours.as_deref(), Some("24/7"));
    assert_eq!(site.groups.len(), 1);
    assert_eq!(site.groups[0].family, "UNKNOWN");
    assert!(site.source.object_version.is_none());
    assert!(site.source.clocks["object_at"].is_none());
    let reasons: Vec<_> = out.issues.iter().map(|v| v.reason.as_str()).collect();
    for reason in [
        "missing_object_version",
        "invalid_source_clock",
        "invalid_socket_presence",
        "unknown_socket",
    ] {
        assert!(reasons.contains(&reason), "{reason}");
    }
    row.tags = row.tags.into_iter().rev().collect::<BTreeMap<_, _>>();
    assert_eq!(site, normalize_osm(&row, &context()).site.unwrap());
}

#[test]
fn geometry_provenance_and_parent_sets_are_validated_deterministically() {
    let mut row = evidence(&[("amenity", "charging_station")]);
    row.osm_type = "way".into();
    assert_eq!(
        normalize_osm(&row, &context()).decision.reason.as_deref(),
        Some("invalid_geometry_provenance")
    );
    row.point.as_mut().unwrap().provenance = "way_centroid".into();
    row.parent_keys = vec!["osm:relation:90003".into(), "osm:relation:90002".into()];
    let first = normalize_osm(&row, &context()).site.unwrap();
    row.parent_keys.reverse();
    assert_eq!(first, normalize_osm(&row, &context()).site.unwrap());
    assert!(first.source.revision.contains(&"a".repeat(64)));
    assert!(OsmContext::new("synthetic", "invalid-hash").is_err());
}

#[test]
fn candidate_selection_counts_inactive_equipment_and_never_invents_fuel_charging() {
    use station_prep::ev_osm_sites::is_osm_ev_candidate;
    for pairs in [
        vec![("disused:man_made", "charge_point")],
        vec![("abandoned:man_made", "charge_point")],
        vec![("construction", "charging_station")],
        vec![("amenity", "fuel"), ("socket:type2", "yes")],
    ] {
        assert!(is_osm_ev_candidate(&evidence(&pairs).tags), "{pairs:?}");
    }
    for pairs in [
        vec![("amenity", "fuel")],
        vec![("amenity", "fuel"), ("socket:domestic", "yes")],
        vec![("amenity", "fuel"), ("socket:type2", "no")],
    ] {
        assert!(!is_osm_ev_candidate(&evidence(&pairs).tags), "{pairs:?}");
    }
}

#[test]
fn dual_site_equipment_markers_do_not_lose_explicit_unparented_sites() {
    let mut row = evidence(&[
        ("amenity", "charging_station"),
        ("man_made", "charge_point"),
    ]);
    let site = normalize_osm(&row, &context()).site.unwrap();
    assert_eq!(site.kind, SiteKind::ChargingSite);
    assert!(site.site_marker);
    assert!(site.equipment_marker);
    row.parent_keys = vec!["osm:way:90003".into()];
    let site = normalize_osm(&row, &context()).site.unwrap();
    assert_eq!(site.kind, SiteKind::ChargePoint);
    assert!(site.site_marker);
    assert!(site.equipment_marker);
}

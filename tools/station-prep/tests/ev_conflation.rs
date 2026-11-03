use station_prep::ev_conflation::conflate;
use station_prep::ev_sites::{ConnectorGroup, SiteKind, SitePoint, SourceLineage, SourceSite};
use std::collections::{BTreeMap, BTreeSet};
fn source(ns: &str, id: i64, meters: f64, family: &str) -> SourceSite {
    let key = if ns == "OSM" {
        format!("osm:node:{id}")
    } else {
        format!("ocm:poi:{id}")
    };
    SourceSite {
        key: key.clone(),
        kind: SiteKind::ChargingSite,
        site_marker: true,
        equipment_marker: false,
        footprint: None,
        source: SourceLineage {
            source: ns.into(),
            external_id: if ns == "OSM" {
                format!("node:{id}")
            } else {
                id.to_string()
            },
            revision: "[P25-TEST] synthetic revision".into(),
            record_sha256: "a".repeat(64),
            object_version: if ns == "OSM" { Some(1) } else { None },
            clocks: BTreeMap::new(),
            provider_id: if ns == "OCM" { Some(1) } else { None },
            license: if ns == "OSM" { "ODbL-1.0" } else { "CC-BY-4.0" }.into(),
            attribution: format!("[P25-TEST] synthetic {ns} attribution"),
        },
        point: SitePoint {
            latitude_decimal: "0".into(),
            longitude_decimal: (meters / 6371000.0_f64).to_degrees().to_string(),
            provenance: if ns == "OSM" {
                "osm_node"
            } else {
                "ocm_source_point"
            }
            .into(),
        },
        name: None,
        operator: None,
        network: None,
        brand: None,
        address: BTreeMap::new(),
        access: "PUBLIC".into(),
        raw_access: Some(if ns == "OCM" { "1" } else { "yes" }.into()),
        fee: None,
        opening_hours: None,
        status: "UNKNOWN".into(),
        raw_status: None,
        groups: vec![ConnectorGroup {
            key: format!("{key}:group:90001"),
            family: family.into(),
            raw_socket: "synthetic-reported-socket".into(),
            power_w: Some(22000),
            reported_quantity: Some(2),
            current: "AC".into(),
            raw_status: None,
        }],
        parent_keys: vec![],
    }
}
#[test]
fn every_distance_band_is_truthful_and_probable_never_merges() {
    for (meters, confirmed, probable) in [
        (0.0, 1, 0),
        (49.999, 1, 0),
        (50.0, 1, 0),
        (50.001, 0, 1),
        (100.0, 0, 1),
        (150.0, 0, 1),
        (150.001, 0, 0),
    ] {
        let mut a = source("OSM", 90001, 0.0, "TYPE2");
        let mut b = source("OCM", 90002, meters, "TYPE2");
        a.operator = Some("[P25-TEST] São Charge".into());
        b.operator = Some("[P25-TEST] SAO CHARGE".into());
        let out = conflate(&[a, b]).unwrap();
        assert_eq!(out.stats.confirmed_pairs, confirmed, "{meters}");
        assert_eq!(
            out.review
                .iter()
                .filter(|r| r.reason == "probable_same_label")
                .count(),
            probable,
            "{meters}"
        );
        assert_eq!(out.sites.len(), if confirmed == 1 { 1 } else { 2 });
        for site in &out.sites {
            assert_eq!(
                site.confidence,
                if confirmed == 1 {
                    "two-source"
                } else {
                    "single-source"
                }
            );
        }
    }
}
#[test]
fn conflicting_or_unknown_sockets_never_confirm_even_at_the_same_point() {
    for families in [
        ("TYPE2", "CCS2"),
        ("UNKNOWN", "UNKNOWN"),
        ("UNREVIEWED_FAMILY", "UNREVIEWED_FAMILY"),
    ] {
        let out = conflate(&[
            source("OSM", 90001, 0.0, families.0),
            source("OCM", 90002, 0.0, families.1),
        ])
        .unwrap();
        assert_eq!(out.stats.confirmed_pairs, 0);
        assert_eq!(out.sites.len(), 2);
        assert_eq!(out.conflicts.len(), 1);
    }
}
#[test]
fn ties_are_one_to_one_and_order_independent() {
    let rows = vec![
        source("OSM", 90002, 0.0, "TYPE2"),
        source("OCM", 90004, 0.0, "TYPE2"),
        source("OSM", 90001, 0.0, "TYPE2"),
        source("OCM", 90003, 0.0, "TYPE2"),
    ];
    let first = conflate(&rows).unwrap();
    let mut reversed = rows;
    reversed.reverse();
    assert_eq!(first, conflate(&reversed).unwrap());
    assert_eq!(first.stats.confirmed_pairs, 2);
    let pairs: BTreeSet<_> = first
        .sites
        .iter()
        .map(|s| s.sources.iter().map(|r| r.key.clone()).collect::<Vec<_>>())
        .collect();
    assert!(pairs.contains(&vec!["ocm:poi:90003".into(), "osm:node:90001".into()]));
    assert!(pairs.contains(&vec!["ocm:poi:90004".into(), "osm:node:90002".into()]));
}
#[test]
fn fields_and_quantities_keep_their_source_without_address_or_power_blending() {
    let mut a = source("OSM", 90001, 0.0, "TYPE2");
    let mut b = source("OCM", 90002, 10.0, "TYPE2");
    a.name = Some("[P25-TEST] OSM source name".into());
    b.name = Some("[P25-TEST] OCM source name".into());
    a.network = Some("[P25-TEST] source network".into());
    b.operator = Some("[P25-TEST] other operator".into());
    a.address = BTreeMap::from([("street".into(), "[P25-TEST] OSM street".into())]);
    b.address = BTreeMap::from([
        ("street".into(), "[P25-TEST] OCM street".into()),
        ("number".into(), "90002".into()),
    ]);
    b.groups[0].reported_quantity = Some(4);
    b.groups[0].power_w = Some(11000);
    let out = conflate(&[a.clone(), b]).unwrap();
    let site = &out.sites[0];
    assert_eq!(site.point, a.point);
    assert_eq!(site.name, a.name);
    assert_eq!(site.operator, a.network);
    assert_eq!(site.address, a.address);
    assert!(!site.address.contains_key("number"));
    assert_eq!(site.field_sources["operator"].field, "network");
    let by_source: BTreeMap<_, _> = site
        .sources
        .iter()
        .map(|s| (s.source.source.as_str(), &s.groups[0]))
        .collect();
    assert_eq!(by_source["OSM"].reported_quantity, Some(2));
    assert_eq!(by_source["OCM"].reported_quantity, Some(4));
    assert_eq!(by_source["OSM"].power_w, Some(22000));
    assert_eq!(by_source["OCM"].power_w, Some(11000));
}
#[test]
fn assigned_equipment_can_supply_reported_family_without_creating_an_extra_site() {
    let mut parent = source("OSM", 90001, 0.0, "UNKNOWN");
    parent.groups.clear();
    let mut child = source("OSM", 90003, 20.0, "TYPE2");
    child.kind = SiteKind::ChargePoint;
    child.site_marker = false;
    child.equipment_marker = true;
    child.parent_keys = vec![parent.key.clone()];
    let out = conflate(&[parent, child, source("OCM", 90002, 1.0, "TYPE2")]).unwrap();
    assert_eq!(out.sites.len(), 1);
    assert_eq!(out.sites[0].equipment.len(), 1);
    assert_eq!(out.stats.confirmed_pairs, 1);
    assert_eq!(out.assignments.len(), 1);
}
#[test]
fn sparse_grid_avoids_source_wide_pairs_and_unsafe_records_refuse() {
    let mut rows = Vec::new();
    for i in 0..300 {
        rows.push(source("OSM", 90000 + i, i as f64 * 1000.0, "TYPE2"));
        rows.push(source("OCM", 91000 + i, i as f64 * 1000.0, "TYPE2"));
    }
    let out = conflate(&rows).unwrap();
    assert_eq!(out.stats.confirmed_pairs, 300);
    assert!(out.stats.candidate_comparisons < 3000);
    for field in ["access", "removed", "planned"] {
        let mut bad = source("OCM", 90002, 0.0, "TYPE2");
        if field == "access" {
            bad.access = "PRIVATE".into();
        } else {
            bad.raw_status = Some(if field == "removed" { "200" } else { "150" }.into());
        }
        assert!(conflate(&[bad]).is_err());
    }
}

#[test]
fn reviewed_golden_outputs_preserve_identity_confidence_and_source_lineage() {
    let expected: serde_json::Value = serde_json::from_str(include_str!(
        "../../../contracts/testdata/ev/conflation-v1/expected-basic.json"
    ))
    .unwrap();
    let mut actual = Vec::new();
    for scenario in ["confirmed", "conflict", "probable"] {
        let mut a = source("OSM", 90001, 0.0, "TYPE2");
        let mut b = source(
            "OCM",
            90002,
            if scenario == "confirmed" {
                10.0
            } else if scenario == "probable" {
                100.0
            } else {
                0.0
            },
            if scenario == "conflict" {
                "CCS2"
            } else {
                "TYPE2"
            },
        );
        if scenario == "probable" {
            a.operator = Some("[P25-TEST] same operator".into());
            b.operator = a.operator.clone();
        }
        let out = conflate(&[a, b]).unwrap();
        let sites:Vec<_>=out.sites.iter().map(|s|serde_json::json!({"key":s.key,"source_keys":s.sources.iter().map(|v|v.key.clone()).collect::<Vec<_>>(),"confidence":s.confidence,"point_source":s.field_sources["point"].source_key,"distance_mm":s.evidence.as_ref().map(|e|e.distance_mm),"shared_families":s.evidence.as_ref().map(|e|e.shared_families.clone()).unwrap_or_default()})).collect();
        actual.push(serde_json::json!({"scenario":scenario,"sites":sites,"review_reasons":out.review.iter().map(|r|r.reason.clone()).collect::<Vec<_>>(),"conflict_reasons":out.conflicts.iter().map(|r|r.reason.clone()).collect::<Vec<_>>() }));
    }
    assert_eq!(serde_json::Value::Array(actual), expected);
}

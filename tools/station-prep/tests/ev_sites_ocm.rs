use serde_json::{json, Value};
use sha2::{Digest, Sha256};
use station_prep::ev_sites::{normalize_ocm, OcmContext};
fn hash(raw: &[u8]) -> String {
    format!("{:x}", Sha256::digest(raw))
}
fn reference() -> Vec<u8> {
    serde_json::to_vec(&json!({"DataProviders":[
 {"ID":1,"Title":"[P25-TEST] provider one","License":"Licensed under Creative Commons Attribution 4.0 International (CC BY 4.0)"},
 {"ID":40,"Title":"[P25-TEST] provider forty","License":"CC-0"},
 {"ID":57,"Title":"[P25-TEST] provider fifty-seven","License":"CC0"}],
 "Operators":[{"ID":90001,"Title":"[P25-TEST] synthetic operator"}]})).unwrap()
}
fn context() -> OcmContext {
    let raw = reference();
    OcmContext::from_reference(&raw, &hash(&raw), "synthetic-revision").unwrap()
}
fn poi() -> Value {
    json!({"ID":90001,"DataProviderID":1,"UsageTypeID":1,"StatusTypeID":50,"OperatorID":90001,
 "DateLastVerified":"2026-01-01T00:00:00Z","AddressInfo":{"Title":"[P25-TEST] synthetic site","CountryID":36,"Latitude":-20.125,"Longitude":-50.25,"AddressLine1":"[P25-TEST] synthetic street","Town":"[P25-TEST] city","ContactEmail":"synthetic-contact@invalid.test","ContactTelephone1":"synthetic-private-phone"},
 "Connections":[{"ID":90002,"ConnectionTypeID":1036,"CurrentTypeID":20,"PowerKW":22,"Quantity":2}],
 "UserComments":[{"Comment":"synthetic-private-comment"}],"MediaItems":[{"ItemURL":"synthetic-private-photo"}]})
}
#[test]
fn common_model_keeps_source_fields_and_excludes_private_surface() {
    let raw = serde_json::to_vec(&poi()).unwrap();
    let out = normalize_ocm(&raw, &context());
    let site = out.site.unwrap();
    assert_eq!(site.key, "ocm:poi:90001");
    assert_eq!(site.source.record_sha256, hash(&raw));
    assert_eq!(site.point.latitude_decimal, "-20.125");
    assert_eq!(
        site.operator.as_deref(),
        Some("[P25-TEST] synthetic operator")
    );
    assert_eq!(site.groups[0].family, "TYPE2");
    assert_eq!(site.groups[0].power_w, Some(22000));
    assert_eq!(site.groups[0].reported_quantity, Some(2));
    assert_eq!(site.groups[0].current, "AC");
    assert_eq!(site.address["street"], "[P25-TEST] synthetic street");
    assert_eq!(site.status, "STATIC_REPORTED_OPERATIONAL");
    let bytes = serde_json::to_string(&site).unwrap();
    for forbidden in [
        "ContactEmail",
        "synthetic-contact",
        "synthetic-private-phone",
        "synthetic-private-comment",
        "synthetic-private-photo",
    ] {
        assert!(!bytes.contains(forbidden));
    }
}
#[test]
fn private_foreign_unlicensed_removed_planned_and_bad_points_never_emit_coordinates() {
    for (field, value, reason) in [
        ("UsageTypeID", json!(2), "private_access"),
        ("UsageTypeID", json!(3), "private_access"),
        ("UsageTypeID", json!(6), "private_access"),
        ("StatusTypeID", json!(200), "removed_site"),
        ("StatusTypeID", json!(210), "removed_site"),
        ("StatusTypeID", json!(150), "planned_site"),
        ("DataProviderID", json!(999), "unlicensed_provider"),
        ("CountryID", json!(1), "foreign_country"),
        ("Latitude", json!(91), "invalid_geometry"),
    ] {
        let mut row = poi();
        if ["CountryID", "Latitude"].contains(&field) {
            row["AddressInfo"][field] = value;
        } else {
            row[field] = value;
        }
        let out = normalize_ocm(&serde_json::to_vec(&row).unwrap(), &context());
        assert!(out.site.is_none(), "{field}");
        assert_eq!(out.decision.reason.as_deref(), Some(reason));
        let bytes = serde_json::to_string(&out).unwrap();
        assert!(!bytes.contains("latitude"));
        assert!(!bytes.contains("synthetic street"));
    }
}
#[test]
fn unknowns_and_partial_groups_stay_honest_and_stably_ordered() {
    let mut row = poi();
    row["UsageTypeID"] = json!(999);
    row["StatusTypeID"] = json!(10);
    row["AddressInfo"]["Title"] = json!("");
    row["Connections"] = json!([{"ID":90004,"ConnectionTypeID":9999,"PowerKW":1.2345,"Quantity":0},{"ID":90003,"ConnectionTypeID":33,"PowerKW":50,"Quantity":1},{"ID":90005,"ConnectionTypeID":1,"StatusTypeID":200}]);
    let out = normalize_ocm(&serde_json::to_vec(&row).unwrap(), &context());
    let site = out.site.unwrap();
    assert!(site.name.is_none());
    assert_eq!(site.access, "UNKNOWN");
    assert_eq!(site.status, "STATIC_REPORTED_OPERATIONAL");
    assert_eq!(site.groups.len(), 2);
    assert_eq!(site.groups[0].key, "ocm:group:90001:90003");
    assert_eq!(site.groups[1].family, "UNKNOWN");
    assert!(site.groups[1].power_w.is_none());
    assert!(site.groups[1].reported_quantity.is_none());
    let reasons: Vec<_> = out.issues.iter().map(|x| x.reason.as_str()).collect();
    for reason in [
        "unknown_access",
        "unknown_socket",
        "invalid_power",
        "invalid_quantity",
        "removed_group",
    ] {
        assert!(reasons.contains(&reason), "{reason}");
    }
}
#[test]
fn duplicate_group_conflicts_are_order_independent_and_numeric_paths_are_exact() {
    let raw=br#"{"ID":90001,"DataProviderID":1,"UsageTypeID":1,"Latitude":1,"AddressInfo":{"CountryID":36,"Latitude":-2.0125e1,"Longitude":-50.25},"Connections":[{"ID":90002,"PowerKW":7.4e0},{"ID":90003,"PowerKW":1},{"ID":90003,"PowerKW":2}]}"#;
    let out = normalize_ocm(raw, &context());
    let site = out.site.unwrap();
    assert_eq!(site.point.latitude_decimal, "-2.0125e1");
    assert_eq!(site.groups.len(), 1);
    assert_eq!(site.groups[0].power_w, Some(7400));
    assert!(out
        .issues
        .iter()
        .any(|x| x.reason == "duplicate_group_identity"));
    let mut row: Value = serde_json::from_slice(raw).unwrap();
    row["Connections"].as_array_mut().unwrap().reverse();
    let reordered = normalize_ocm(&serde_json::to_vec(&row).unwrap(), &context())
        .site
        .unwrap();
    assert_eq!(site.groups, reordered.groups);
    let duplicated = String::from_utf8(raw.to_vec())
        .unwrap()
        .replace("\"PowerKW\":7.4e0", "\"PowerKW\":7.4e0,\"PowerKW\":8");
    assert!(normalize_ocm(duplicated.as_bytes(), &context())
        .site
        .is_none());
}
#[test]
fn reference_hash_rights_and_malformed_source_refuse() {
    let raw = reference();
    assert!(OcmContext::from_reference(&raw, &"0".repeat(64), "synthetic").is_err());
    let mut changed: Value = serde_json::from_slice(&raw).unwrap();
    changed["DataProviders"][0]["License"] = json!("restricted");
    let changed = serde_json::to_vec(&changed).unwrap();
    assert!(OcmContext::from_reference(&changed, &hash(&changed), "synthetic").is_err());
    for raw in [b"{".as_slice(), b"{}", b"{\"ID\":0}"] {
        let out = normalize_ocm(raw, &context());
        assert!(out.site.is_none());
    }
}

#[test]
fn public_conditions_and_missing_groups_do_not_fabricate_fees_or_sockets() {
    for (usage, access, fee) in [
        (1, "PUBLIC", None),
        (4, "MEMBERSHIP", None),
        (5, "PUBLIC", Some(true)),
        (7, "NOTICE_REQUIRED", None),
        (0, "UNKNOWN", None),
    ] {
        let mut row = poi();
        row["UsageTypeID"] = json!(usage);
        row["Connections"] = Value::Null;
        row["OperatorID"] = json!(0);
        let out = normalize_ocm(&serde_json::to_vec(&row).unwrap(), &context());
        let site = out.site.expect("missing groups are unknown, not malformed");
        assert_eq!(site.access, access);
        assert_eq!(site.fee, fee);
        assert!(site.groups.is_empty());
        assert!(site.operator.is_none());
    }
}

#[test]
fn source_clocks_are_validated_without_inventing_timezone_or_leaking_free_text() {
    for (raw, valid) in [
        ("2024-02-29T23:59:59.123Z", true),
        ("2026-01-01T12:00:00", true),
        ("2026-01-01T12:00:00-04:00", true),
        ("2026-02-29T12:00:00Z", false),
        ("synthetic-private-clock@invalid.test", false),
        ("2026-01-01T25:00:00Z", false),
    ] {
        let mut row = poi();
        row["DateCreated"] = json!(raw);
        let out = normalize_ocm(&serde_json::to_vec(&row).unwrap(), &context());
        let site = out.site.unwrap();
        assert_eq!(
            site.source.clocks["created_at"].as_deref(),
            if valid { Some(raw) } else { None },
            "{raw}"
        );
        if !valid {
            assert!(out
                .issues
                .iter()
                .any(|x| x.reason == "invalid_source_clock"));
        }
        assert!(!serde_json::to_string(&site)
            .unwrap()
            .contains("synthetic-private-clock"));
    }
}

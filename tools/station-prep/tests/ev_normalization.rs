use station_prep::ev_sockets::{
    current_family, family_for_ocm, family_for_osm, power_kw_to_watts, power_osm_to_watts,
};

#[test]
fn exact_power_refuses_rounding_unknowns_overflow_and_bad_units() {
    for (raw, expected) in [
        ("7.4", Some(7400)),
        ("1.234", Some(1234)),
        ("1.2340", Some(1234)),
        ("7.4e0", Some(7400)),
        ("1e-3", Some(1)),
        ("0", None),
        ("-1", None),
        ("NaN", None),
        ("1.2345", None),
        ("1000000.001", None),
        ("1e100", None),
        ("1e-100", None),
    ] {
        assert_eq!(power_kw_to_watts(raw), expected, "{raw}");
    }
    for (raw, expected) in [
        ("22 kW", Some(22000)),
        ("22000 W", Some(22000)),
        ("0.15 MW", Some(150000)),
        ("22", Some(22000)),
        ("22kW", Some(22000)),
        ("50;100 kW", None),
        ("16 A", None),
    ] {
        assert_eq!(power_osm_to_watts(raw), expected, "{raw}");
    }
}

#[test]
fn explicit_socket_contract_never_guesses_compatibility() {
    for (osm, ocm, family) in [
        ("type1", 1, "TYPE1"),
        ("type2", 25, "TYPE2"),
        ("type2_cable", 1036, "TYPE2"),
        ("type1_combo", 32, "CCS1"),
        ("type2_combo", 33, "CCS2"),
        ("chademo", 2, "CHADEMO"),
        ("gb_ac", 1038, "GBT_AC"),
        ("gb_dc", 1040, "GBT_DC"),
        ("nacs", 27, "NACS"),
    ] {
        assert_eq!(family_for_osm(osm), family);
        assert_eq!(family_for_ocm(ocm), family);
    }
    assert_eq!(family_for_osm("nbr14136"), "NBR14136");
    for key in [
        "domestic",
        "tesla_supercharger",
        "tesla_destination",
        "future_socket",
    ] {
        assert_eq!(family_for_osm(key), "UNKNOWN");
    }
    assert_eq!(family_for_ocm(99999), "UNKNOWN");
    assert_eq!(current_family(10), "AC");
    assert_eq!(current_family(20), "AC");
    assert_eq!(current_family(30), "DC");
    assert_eq!(current_family(99), "UNKNOWN");
}

#[test]
fn socket_contract_has_unique_source_codes_and_every_entry_is_used() {
    let contract: serde_json::Value = serde_json::from_str(include_str!(
        "../../../contracts/ev/socket-families-v1.json"
    ))
    .unwrap();
    let mut osm = std::collections::BTreeSet::new();
    let mut ocm = std::collections::BTreeSet::new();
    for family in contract["families"].as_array().unwrap() {
        let label = family["family"].as_str().unwrap();
        assert_ne!(label, "UNKNOWN");
        for key in family["osm"].as_array().unwrap() {
            let key = key.as_str().unwrap();
            assert!(osm.insert(key));
            assert_eq!(family_for_osm(key), label);
        }
        for code in family["ocm"].as_array().unwrap() {
            let code = code.as_i64().unwrap();
            assert!(ocm.insert(code));
            assert_eq!(family_for_ocm(code), label);
        }
    }
    for key in contract["ambiguous_osm_keys"].as_array().unwrap() {
        assert_eq!(family_for_osm(key.as_str().unwrap()), "UNKNOWN");
    }
}

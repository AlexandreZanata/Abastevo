use station_prep::ev_ocm::parse_ocm_poi;
use std::collections::HashSet;

#[test]
fn ocm_reader_corrects_three_phase_dc_and_preserves_fractional_unknown() {
    let raw=br#"{"ID":90001,"DataProviderID":1,"UsageTypeID":1,"AddressInfo":{"Title":"[P25-TEST] synthetic site","CountryID":36,"Latitude":-20.0,"Longitude":-50.0},"Connections":[{"ID":90002,"ConnectionTypeID":1036,"CurrentTypeID":20,"PowerKW":22.000},{"ID":90003,"ConnectionTypeID":33,"CurrentTypeID":30,"PowerKW":50},{"ID":90004,"PowerKW":1.2345}]}"#;
    let run = parse_ocm_poi(
        "synthetic.json",
        raw,
        "synthetic-rights",
        &mut HashSet::new(),
        &mut HashSet::new(),
        &mut |_| Ok(()),
    )
    .unwrap();
    assert_eq!(run.entities[1].payload["current"], "AC");
    assert_eq!(run.entities[2].payload["current"], "DC");
    assert_eq!(run.entities[1].payload["power_w"], 22000);
    assert!(run.entities[3].payload["power_w"].is_null());
}

#[test]
fn numeric_tokens_are_bound_to_their_connection_and_duplicate_fields_refuse() {
    let raw=br#"{"ID":90001,"DataProviderID":1,"UsageTypeID":1,"PowerKW":999,"AddressInfo":{"Title":"[P25-TEST] synthetic site","CountryID":36,"Latitude":-20.0,"Longitude":-50.0},"Connections":[{"ID":90002,"PowerKW":7.4e0},{"ID":90003,"PowerKW":22.0000}]}"#;
    let parse = |bytes: &[u8]| {
        parse_ocm_poi(
            "synthetic.json",
            bytes,
            "synthetic-rights",
            &mut HashSet::new(),
            &mut HashSet::new(),
            &mut |_| Ok(()),
        )
    };
    let run = parse(raw).unwrap();
    assert_eq!(run.entities[1].payload["power_w"], 7400);
    assert_eq!(run.entities[2].payload["power_w"], 22000);
    let duplicate = String::from_utf8(raw.to_vec())
        .unwrap()
        .replace("\"PowerKW\":7.4e0", "\"PowerKW\":7.4e0,\"PowerKW\":8");
    assert!(parse(duplicate.as_bytes()).is_err());
}

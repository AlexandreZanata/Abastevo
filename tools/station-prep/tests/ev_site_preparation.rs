use pbf_craft::models::{Element, Node, Tag, Way, WayNode};
use pbf_craft::writers::PbfWriter;
use serde_json::{json, Value};
use sha2::{Digest, Sha256};
use station_prep::ev_site_preparation::{prepare, Inputs, SourcePins};
use station_prep::osm_national::NationalProfile;
use std::path::{Path, PathBuf};
use std::sync::atomic::{AtomicU64, Ordering};
fn hash(raw: &[u8]) -> String {
    format!("{:x}", Sha256::digest(raw))
}
struct Fixture {
    root: PathBuf,
    pins: SourcePins,
}
impl Fixture {
    fn new() -> Self {
        static NEXT: AtomicU64 = AtomicU64::new(0);
        let root = std::env::temp_dir().join(format!(
            "evc-normalized-{}-{}",
            std::process::id(),
            NEXT.fetch_add(1, Ordering::Relaxed)
        ));
        std::fs::create_dir(&root).unwrap();
        let mut writer = PbfWriter::from_path(root.join("synthetic.pbf"), true).unwrap();
        for id in [90001, 90002, 90003] {
            writer
                .write(Element::Node(Node {
                    id,
                    version: 3,
                    timestamp: None,
                    user: None,
                    changeset_id: -1,
                    latitude: if id == 90001 {
                        -20_000_000_000
                    } else {
                        -20_001_000_000
                    },
                    longitude: match id {
                        90002 => -50_001_000_000,
                        90003 => -49_999_000_000,
                        _ => -50_000_000_000,
                    },
                    visible: true,
                    tags: if id == 90001 {
                        vec![
                            Tag {
                                key: "man_made".into(),
                                value: "charge_point".into(),
                            },
                            Tag {
                                key: "socket:type2".into(),
                                value: "yes".into(),
                            },
                            Tag {
                                key: "contact:email".into(),
                                value: "synthetic-private-email@invalid.test".into(),
                            },
                        ]
                    } else {
                        vec![]
                    },
                }))
                .unwrap();
        }
        writer
            .write(Element::Way(Way {
                id: 90003,
                version: 2,
                timestamp: None,
                user: None,
                changeset_id: -1,
                visible: true,
                tags: vec![
                    Tag {
                        key: "amenity".into(),
                        value: "charging_station".into(),
                    },
                    Tag {
                        key: "name".into(),
                        value: "[P25-TEST] synthetic source site".into(),
                    },
                    Tag {
                        key: "socket:type2".into(),
                        value: "1".into(),
                    },
                ],
                way_nodes: vec![
                    WayNode::new_without_coords(90001),
                    WayNode::new_without_coords(90002),
                    WayNode::new_without_coords(90003),
                    WayNode::new_without_coords(90001),
                ],
            }))
            .unwrap();
        writer.finish().unwrap();
        std::fs::create_dir(root.join("ocm")).unwrap();
        std::fs::create_dir(root.join("ocm/pois")).unwrap();
        let poi=serde_json::to_vec(&json!({"ID":90004,"DataProviderID":1,"UsageTypeID":1,"AddressInfo":{"CountryID":36,"Title":"[P25-TEST] synthetic OCM site","Latitude":-20.0,"Longitude":-50.0,"ContactTelephone1":"synthetic-private-phone"},"Connections":[{"ID":90005,"ConnectionTypeID":1036,"Quantity":1}]})).unwrap();
        std::fs::write(root.join("ocm/pois/OCM-90004.json"), &poi).unwrap();
        let inventory=serde_json::to_vec(&json!([{"file":"OCM-90004.json","git_blob_sha":"synthetic-not-used-for-admission","bytes":poi.len(),"sha256":hash(&poi),"provider_id":1,"connection_groups":1}])).unwrap();
        std::fs::write(root.join("ocm/source-inventory.json"), &inventory).unwrap();
        let reference=serde_json::to_vec(&json!({"DataProviders":[{"ID":1,"License":"Licensed under Creative Commons Attribution 4.0 International (CC BY 4.0)"},{"ID":40,"License":"CC-0"},{"ID":57,"License":"CC0"}]})).unwrap();
        std::fs::write(root.join("ocm/referencedata.json"), &reference).unwrap();
        let boundary=serde_json::to_vec(&json!({"type":"FeatureCollection","features":[{"type":"Feature","properties":{"codarea":"3500000","fixture":"[P25-TEST] synthetic municipality"},"geometry":{"type":"Polygon","coordinates":[[[-51,-21],[-49,-21],[-49,-19],[-51,-19],[-51,-21]]]}}]})).unwrap();
        std::fs::write(root.join("boundaries.json"), &boundary).unwrap();
        let pbf = std::fs::read(root.join("synthetic.pbf")).unwrap();
        let pins = SourcePins {
            profile_id: "synthetic-ev-source-fixture-v1".into(),
            profile_sha256: hash(b"[P25-TEST] synthetic policy"),
            osm_revision: "synthetic-pbf".into(),
            osm_sha256: hash(&pbf),
            osm_bytes: pbf.len() as u64,
            ocm_revision: "synthetic-ocm".into(),
            ocm_reference_sha256: hash(&reference),
            ocm_inventory_sha256: hash(&inventory),
            ocm_rows: 1,
            boundary_sha256: hash(&boundary),
        };
        Self { root, pins }
    }
    fn inputs(&self, out: &str) -> Inputs {
        Inputs {
            osm: self.root.join("synthetic.pbf"),
            ocm: self.root.join("ocm"),
            boundaries: self.root.join("boundaries.json"),
            output: self.root.join(out),
        }
    }
}
impl Drop for Fixture {
    fn drop(&mut self) {
        let _ = std::fs::remove_dir_all(&self.root);
    }
}
fn load(root: &Path, name: &str) -> Value {
    serde_json::from_slice(&std::fs::read(root.join(name)).unwrap()).unwrap()
}
#[test]
fn private_generation_is_repeatable_complete_and_keeps_equipment_separate() {
    let f = Fixture::new();
    let limits = NationalProfile::frozen().limits;
    for name in ["one", "two"] {
        prepare(&f.inputs(name), &f.pins, &limits, &|| false).unwrap();
    }
    for name in [
        "sites.ndjson",
        "decisions.ndjson",
        "issues.ndjson",
        "uf-census.json",
        "manifest.json",
    ] {
        assert_eq!(
            std::fs::read(f.root.join("one").join(name)).unwrap(),
            std::fs::read(f.root.join("two").join(name)).unwrap(),
            "{name}"
        );
    }
    let rows: Vec<Value> = std::fs::read_to_string(f.root.join("one/sites.ndjson"))
        .unwrap()
        .lines()
        .map(|s| serde_json::from_str(s).unwrap())
        .collect();
    assert_eq!(rows.len(), 3);
    let parent = rows.iter().find(|r| r["key"] == "osm:way:90003").unwrap();
    assert_eq!(
        parent["footprint"]["polygons"][0]["outer"]
            .as_array()
            .unwrap()
            .len(),
        4
    );
    let child = rows.iter().find(|r| r["kind"] == "CHARGE_POINT").unwrap();
    assert_eq!(child["parent_keys"], json!(["osm:way:90003"]));
    let c = load(&f.root.join("one"), "uf-census.json");
    assert_eq!(c["by_source"]["OSM"]["SP"]["CHARGING_SITE"], 1);
    assert_eq!(c["by_source"]["OSM"]["SP"]["CHARGE_POINT"], 1);
    assert_eq!(c["by_source"]["OCM"]["SP"]["CHARGING_SITE"], 1);
    let m = load(&f.root.join("one"), "manifest.json");
    assert_eq!(m["format"], "abastevo.ev-source-sites-v1");
    assert_eq!(m["source_profile"]["profile_id"], f.pins.profile_id);
    assert_eq!(m["source_profile"]["exposure"], "PRIVATE_PREPARATION");
    for name in ["sites.ndjson", "decisions.ndjson", "issues.ndjson"] {
        let raw = std::fs::read_to_string(f.root.join("one").join(name)).unwrap();
        assert!(!raw.contains("synthetic-private"));
    }
}
#[test]
fn changed_missing_and_unexpected_inputs_refuse_without_generation() {
    for fault in ["pbf", "poi", "reference", "boundary", "missing", "extra"] {
        let f = Fixture::new();
        let input = f.inputs("rejected");
        let target = match fault {
            "pbf" => Some(f.root.join("synthetic.pbf")),
            "poi" => Some(f.root.join("ocm/pois/OCM-90004.json")),
            "reference" => Some(f.root.join("ocm/referencedata.json")),
            "boundary" => Some(f.root.join("boundaries.json")),
            _ => None,
        };
        if let Some(p) = target {
            let mut raw = std::fs::read(&p).unwrap();
            raw.push(b' ');
            std::fs::write(p, raw).unwrap();
        }
        if fault == "missing" {
            std::fs::remove_file(f.root.join("ocm/pois/OCM-90004.json")).unwrap();
        }
        if fault == "extra" {
            std::fs::write(f.root.join("ocm/pois/OCM-99999.json"), b"{}").unwrap();
        }
        assert!(
            prepare(&input, &f.pins, &NationalProfile::frozen().limits, &|| {
                false
            })
            .is_err(),
            "{fault}"
        );
        assert!(!input.output.exists());
    }
}
#[test]
fn cancellation_caps_and_existing_output_preserve_last_good() {
    let f = Fixture::new();
    let input = f.inputs("rejected");
    let mut limits = NationalProfile::frozen().limits;
    assert!(prepare(&input, &f.pins, &limits, &|| true).is_err());
    assert!(!input.output.exists());
    limits.max_input_elements = 1;
    assert!(prepare(&input, &f.pins, &limits, &|| false).is_err());
    assert!(!input.output.exists());
    std::fs::create_dir(&input.output).unwrap();
    std::fs::write(input.output.join("last-good"), b"[P25-TEST] last good").unwrap();
    assert!(
        prepare(&input, &f.pins, &NationalProfile::frozen().limits, &|| {
            false
        })
        .is_err()
    );
    assert_eq!(
        std::fs::read(input.output.join("last-good")).unwrap(),
        b"[P25-TEST] last good"
    );
}

#[test]
fn cancellation_at_output_start_removes_only_unpublished_generation() {
    let f = Fixture::new();
    let input = f.inputs("cancelled-output");
    let result = prepare(&input, &f.pins, &NationalProfile::frozen().limits, &|| {
        input.output.exists()
    });
    assert!(result.is_err());
    assert!(!input.output.exists());
    assert!(input.osm.exists());
}
#[test]
fn untrusted_inventory_byte_totals_refuse_without_overflow() {
    let mut f = Fixture::new();
    let raw = serde_json::to_vec(&json!([
 {"file":"OCM-90004.json","bytes":u64::MAX,"sha256":"synthetic"},
 {"file":"OCM-90006.json","bytes":u64::MAX,"sha256":"synthetic"}]))
    .unwrap();
    std::fs::write(f.root.join("ocm/source-inventory.json"), &raw).unwrap();
    f.pins.ocm_inventory_sha256 = hash(&raw);
    f.pins.ocm_rows = 2;
    assert!(prepare(
        &f.inputs("invalid-inventory"),
        &f.pins,
        &NationalProfile::frozen().limits,
        &|| false
    )
    .is_err());
}

#[test]
fn native_cli_refuses_runtime_and_profile_overrides_before_source_io() {
    use std::process::Command;
    let binary = env!("CARGO_BIN_EXE_prepare_ev_sites");
    let args = [
        "--osm",
        "synthetic-missing-pbf",
        "--ocm",
        "synthetic-missing-ocm",
        "--boundaries",
        "synthetic-missing-boundaries",
        "--output",
        "synthetic-missing-output",
    ];
    let out = Command::new(binary)
        .args(args)
        .env("ANPFUEL_ENV", "staging")
        .output()
        .unwrap();
    assert!(!out.status.success());
    assert!(String::from_utf8_lossy(&out.stderr).contains("local-only"));
    let out = Command::new(binary)
        .args(args)
        .args(["--max-matched", "999999"])
        .output()
        .unwrap();
    assert!(!out.status.success());
    assert!(String::from_utf8_lossy(&out.stderr).contains("overrides refused"));
}

#[test]
fn declared_country_does_not_publish_a_point_outside_admitted_country_extent() {
    let mut f = Fixture::new();
    let path = f.root.join("ocm/pois/OCM-90004.json");
    let mut row: Value = serde_json::from_slice(&std::fs::read(&path).unwrap()).unwrap();
    row["AddressInfo"]["Latitude"] = json!(60);
    row["AddressInfo"]["Longitude"] = json!(120);
    let raw = serde_json::to_vec(&row).unwrap();
    std::fs::write(&path, &raw).unwrap();
    let inv = f.root.join("ocm/source-inventory.json");
    let mut inventory: Value = serde_json::from_slice(&std::fs::read(&inv).unwrap()).unwrap();
    inventory[0]["bytes"] = json!(raw.len());
    inventory[0]["sha256"] = json!(hash(&raw));
    let inventory = serde_json::to_vec(&inventory).unwrap();
    std::fs::write(inv, &inventory).unwrap();
    f.pins.ocm_inventory_sha256 = hash(&inventory);
    let input = f.inputs("country-checked");
    let summary = prepare(&input, &f.pins, &NationalProfile::frozen().limits, &|| {
        false
    })
    .unwrap();
    assert_eq!(summary.quarantined, 1);
    assert_eq!(summary.accepted_source_sites, 1);
    let decisions: Vec<Value> = std::fs::read_to_string(input.output.join("decisions.ndjson"))
        .unwrap()
        .lines()
        .map(|s| serde_json::from_str(s).unwrap())
        .collect();
    let rejected = decisions
        .iter()
        .find(|r| r["key"] == "ocm:poi:90004")
        .unwrap();
    assert_eq!(rejected["reason"], "outside_country_extent");
    assert!(!rejected.to_string().contains("latitude"));
}

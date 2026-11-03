use pbf_craft::models::{Element, Node, Tag};
use pbf_craft::writers::PbfWriter;
use serde_json::json;
use station_prep::osm_national::NationalProfile;
use std::path::{Path, PathBuf};
use std::process::Command;

struct Fixture(PathBuf);
impl Fixture {
    fn new() -> Self {
        let root = std::env::temp_dir().join(format!("evc-national-{}", std::process::id()));
        std::fs::create_dir(&root).unwrap();
        Self(root)
    }
}
impl Drop for Fixture {
    fn drop(&mut self) {
        let _ = std::fs::remove_dir_all(&self.0);
    }
}
fn synthetic_pbf(path: &Path) {
    let mut writer = PbfWriter::from_path(path, true).unwrap();
    for (id, amenity) in [
        (90001, "fuel"),
        (90002, "charging_station"),
        (90003, "charging_station"),
    ] {
        writer
            .write(Element::Node(Node {
                id,
                version: 1,
                timestamp: None,
                user: None,
                changeset_id: -1,
                latitude: if id == 90003 { 0 } else { -20_000_000_000 },
                longitude: if id == 90003 { 0 } else { -50_000_000_000 },
                visible: true,
                tags: {
                    let mut tags = vec![
                        Tag {
                            key: "amenity".into(),
                            value: amenity.into(),
                        },
                        Tag {
                            key: "name".into(),
                            value: "[P25-TEST] synthetic site".into(),
                        },
                    ];
                    if id == 90003 {
                        tags.push(Tag {
                            key: "fuel".into(),
                            value: "yes".into(),
                        });
                    }
                    tags
                },
            }))
            .unwrap();
    }
    writer.finish().unwrap();
}
#[test]
fn national_profile_is_frozen_and_cli_refuses_overrides_before_publication() {
    let profile = NationalProfile::frozen();
    assert_eq!(profile.profile_id, "brazil-ev-national-v1");
    assert_eq!(profile.limits.max_input_bytes, 3 << 30);
    assert_eq!(profile.resources.max_wall_seconds, 1800);
    for args in [
        vec!["--bbox", "-51,-21,-49,-19"],
        vec!["--max-matched", "999999"],
        vec!["--max-input-bytes", "99999999999"],
    ] {
        let output = Command::new(env!("CARGO_BIN_EXE_prepare_osm"))
            .args([
                "--profile",
                "brazil-ev-national-v1",
                "--input",
                "synthetic-missing.pbf",
                "--output",
                "synthetic-missing-output",
            ])
            .args(args)
            .output()
            .unwrap();
        assert!(!output.status.success());
        assert!(String::from_utf8_lossy(&output.stderr).contains("profile refuses"));
    }
}
#[test]
fn national_cli_emits_repeatable_hash_bound_uf_census() {
    let f = Fixture::new();
    let pbf = f.0.join("synthetic.pbf");
    synthetic_pbf(&pbf);
    let malha = f.0.join("synthetic-ibge.json");
    std::fs::write(&malha,json!({"type":"FeatureCollection","features":[{"type":"Feature","properties":{"codarea":"3500000","fixture":"[P25-TEST] synthetic shape"},"geometry":{"type":"Polygon","coordinates":[[[-51,-21],[-49,-21],[-49,-19],[-51,-19],[-51,-21]]]}}]}).to_string()).unwrap();
    for out in ["one", "two"] {
        let output = Command::new(env!("CARGO_BIN_EXE_prepare_osm"))
            .args(["--profile", "brazil-ev-national-v1", "--input"])
            .arg(&pbf)
            .arg("--boundaries")
            .arg(&malha)
            .arg("--output")
            .arg(f.0.join(out))
            .output()
            .unwrap();
        assert!(
            output.status.success(),
            "{}",
            String::from_utf8_lossy(&output.stderr)
        );
    }
    for file in [
        "references.ndjson",
        "decisions.ndjson",
        "uf-census.json",
        "manifest.json",
    ] {
        assert_eq!(
            std::fs::read(f.0.join("one").join(file)).unwrap(),
            std::fs::read(f.0.join("two").join(file)).unwrap(),
            "{file}"
        );
    }
    let census: serde_json::Value =
        serde_json::from_slice(&std::fs::read(f.0.join("one/uf-census.json")).unwrap()).unwrap();
    assert_eq!(census["by_uf"]["SP"]["FUEL_STATION"], 1);
    assert_eq!(census["by_uf"]["SP"]["CHARGING_SITE"], 1);
    assert_eq!(census["totals"]["COMBINED_SITE"], 1);
    assert_eq!(census["by_uf"]["UNASSIGNED"]["COMBINED_SITE"], 1);
    assert_eq!(census["classification_reasons"]["no_boundary_match"], 1);
    assert_eq!(census["by_uf"].as_object().unwrap().len(), 28);
    let manifest: serde_json::Value =
        serde_json::from_slice(&std::fs::read(f.0.join("one/manifest.json")).unwrap()).unwrap();
    assert_eq!(
        manifest["preparation_profile"]["profile_id"],
        "brazil-ev-national-v1"
    );
    assert!(manifest["files"]["uf-census.json"]["sha256"]
        .as_str()
        .is_some_and(|v| v.len() == 64));
}

#[test]
fn national_cli_preserves_existing_generation_and_refuses_unknown_profile() {
    let root = std::env::temp_dir().join(format!("evc-national-preserve-{}", std::process::id()));
    std::fs::create_dir(&root).unwrap();
    let _fixture = Fixture(root.clone());
    std::fs::write(root.join("last-good"), b"[P25-TEST] last good").unwrap();
    for profile in ["brazil-ev-national-v1", "unknown-profile"] {
        let output = Command::new(env!("CARGO_BIN_EXE_prepare_osm"))
            .args([
                "--profile",
                profile,
                "--input",
                "synthetic-missing.pbf",
                "--output",
            ])
            .arg(&root)
            .output()
            .unwrap();
        assert!(!output.status.success());
        assert_eq!(
            std::fs::read(root.join("last-good")).unwrap(),
            b"[P25-TEST] last good"
        );
        let error = String::from_utf8_lossy(&output.stderr);
        assert!(error.contains(if profile == "unknown-profile" {
            "unknown national profile"
        } else {
            "national output already exists"
        }));
    }
}

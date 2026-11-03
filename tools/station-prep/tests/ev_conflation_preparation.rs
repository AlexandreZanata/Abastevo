use serde_json::{json, Value};
use sha2::{Digest, Sha256};
use station_prep::ev_conflation_preparation::{prepare, Inputs};
use station_prep::ev_site_preparation::SourcePins;
use std::path::PathBuf;
use std::sync::atomic::{AtomicU64, Ordering};
fn hash(raw: &[u8]) -> String {
    format!("{:x}", Sha256::digest(raw))
}
struct Fixture {
    root: PathBuf,
    pins: SourcePins,
    manifest_sha: String,
}
impl Fixture {
    fn new() -> Self {
        static NEXT: AtomicU64 = AtomicU64::new(0);
        let root = std::env::temp_dir().join(format!(
            "evc-conflation-{}-{}",
            std::process::id(),
            NEXT.fetch_add(1, Ordering::Relaxed)
        ));
        std::fs::create_dir(&root).unwrap();
        std::fs::create_dir(root.join("input")).unwrap();
        let boundary=serde_json::to_vec(&json!({"type":"FeatureCollection","features":[{"type":"Feature","properties":{"codarea":"3500000","fixture":"[P25-TEST] synthetic boundary"},"geometry":{"type":"Polygon","coordinates":[[[-51,-21],[-49,-21],[-49,-19],[-51,-19],[-51,-21]]]}}]})).unwrap();
        std::fs::write(root.join("boundary.json"), &boundary).unwrap();
        let pins = SourcePins {
            profile_id: "synthetic-ev-source-v1".into(),
            profile_sha256: hash(b"[P25-TEST] policy"),
            osm_revision: "synthetic-osm".into(),
            osm_sha256: "a".repeat(64),
            osm_bytes: 1,
            ocm_revision: "synthetic-ocm".into(),
            ocm_reference_sha256: "b".repeat(64),
            ocm_inventory_sha256: "c".repeat(64),
            ocm_rows: 1,
            boundary_sha256: hash(&boundary),
        };
        let mut sites = Vec::new();
        let mut decisions = Vec::new();
        for (ns, key, external) in [
            ("OSM", "osm:node:90001", "node:90001"),
            ("OCM", "ocm:poi:90002", "90002"),
        ] {
            let row = json!({"key":key,"kind":"CHARGING_SITE","site_marker":true,"equipment_marker":false,"footprint":null,"source":{"source":ns,"external_id":external,"revision":"[P25-TEST] synthetic","record_sha256":"a".repeat(64),"object_version":null,"clocks":{},"provider_id":if ns=="OCM"{Some(1)}else{None},"license":if ns=="OSM"{"ODbL-1.0"}else{"CC-BY-4.0"},"attribution":"[P25-TEST] synthetic attribution"},"point":{"latitude_decimal":"-20","longitude_decimal":"-50","provenance":"synthetic_source_point"},"name":"[P25-TEST] synthetic site","operator":null,"network":null,"brand":null,"address":{},"access":"PUBLIC","raw_access":null,"fee":null,"opening_hours":null,"status":"UNKNOWN","raw_status":null,"groups":[{"key":format!("{key}:group"),"family":"TYPE2","raw_socket":"synthetic","power_w":22000,"reported_quantity":2,"current":"AC","raw_status":null}],"parent_keys":[]});
            sites.extend(serde_json::to_vec(&row).unwrap());
            sites.push(b'\n');
            decisions.extend(
                serde_json::to_vec(&json!({"key":key,"state":"ACCEPTED","reason":null})).unwrap(),
            );
            decisions.push(b'\n');
        }
        let mut files = serde_json::Map::new();
        for (name, raw, records) in [
            ("sites.ndjson", sites, Some(2)),
            ("decisions.ndjson", decisions, Some(2)),
            ("issues.ndjson", vec![], Some(0)),
            ("uf-census.json", b"{}".to_vec(), None),
        ] {
            std::fs::write(root.join("input").join(name), &raw).unwrap();
            let mut info = json!({"bytes":raw.len(),"sha256":hash(&raw)});
            if let Some(n) = records {
                info["records"] = json!(n);
            }
            files.insert(name.into(), info);
        }
        let manifest = json!({"format":"abastevo.ev-source-sites-v1","parser_version":"ev-normalization-v2","normalization_profile_sha256":hash(include_bytes!("../../../contracts/ev/normalization-profile-v2.json")),"source_profile":{"profile_id":pins.profile_id,"sha256":pins.profile_sha256,"exposure":"PRIVATE_PREPARATION"},"source_pins":pins,"summary":{"accepted_source_sites":2,"source_equipment":0,"quarantined":0,"ignored":0,"osm_candidates":1},"files":files});
        let raw = serde_json::to_vec(&manifest).unwrap();
        let manifest_sha = hash(&raw);
        std::fs::write(root.join("input/manifest.json"), raw).unwrap();
        Self {
            root,
            pins,
            manifest_sha,
        }
    }
    fn inputs(&self, name: &str) -> Inputs {
        Inputs {
            normalized: self.root.join("input"),
            manifest_sha256: self.manifest_sha.clone(),
            boundaries: self.root.join("boundary.json"),
            output: self.root.join(name),
        }
    }
    fn alter_manifest(&mut self, edit: impl FnOnce(&mut Value)) {
        let p = self.root.join("input/manifest.json");
        let mut m: Value = serde_json::from_slice(&std::fs::read(&p).unwrap()).unwrap();
        edit(&mut m);
        let raw = serde_json::to_vec(&m).unwrap();
        self.manifest_sha = hash(&raw);
        std::fs::write(p, raw).unwrap();
    }
}
impl Drop for Fixture {
    fn drop(&mut self) {
        let _ = std::fs::remove_dir_all(&self.root);
    }
}
#[test]
fn private_catalog_and_queues_are_repeatable_and_reconciled() {
    use std::os::unix::fs::PermissionsExt;
    let f = Fixture::new();
    for out in ["one", "two"] {
        let stats = prepare(&f.inputs(out), &f.pins, &|| false).unwrap();
        assert_eq!(stats.confirmed_pairs, 1);
    }
    for name in [
        "catalog.ndjson",
        "pair-review.ndjson",
        "conflicts.ndjson",
        "hierarchy-review.ndjson",
        "equipment-assignments.ndjson",
        "uf-census.json",
        "owner-review-sample.json",
        "manifest.json",
    ] {
        assert_eq!(
            std::fs::read(f.root.join("one").join(name)).unwrap(),
            std::fs::read(f.root.join("two").join(name)).unwrap(),
            "{name}"
        );
        assert_eq!(
            std::fs::metadata(f.root.join("one").join(name))
                .unwrap()
                .permissions()
                .mode()
                & 0o777,
            0o600
        );
    }
    let census: Value =
        serde_json::from_slice(&std::fs::read(f.root.join("one/uf-census.json")).unwrap()).unwrap();
    assert_eq!(census["by_uf"]["SP"]["two_source_sites"], 1);
    assert_eq!(census["by_uf"].as_object().unwrap().len(), 28);
    let sample: Value = serde_json::from_slice(
        &std::fs::read(f.root.join("one/owner-review-sample.json")).unwrap(),
    )
    .unwrap();
    assert_eq!(sample["status"], "OWNER_IMAGERY_REVIEW_PENDING");
    assert_eq!(sample["samples"].as_array().unwrap().len(), 1);
}
#[test]
fn changed_stale_incomplete_and_inconsistent_inputs_refuse() {
    for fault in ["hash", "file", "profile", "missing", "decision"] {
        let mut f = Fixture::new();
        if fault == "hash" {
            f.manifest_sha = "0".repeat(64);
        }
        if fault == "file" {
            std::fs::write(f.root.join("input/sites.ndjson"), b"changed").unwrap();
        }
        if fault == "profile" {
            f.alter_manifest(|m| m["parser_version"] = json!("ev-normalization-v1"));
        }
        if fault == "missing" {
            f.alter_manifest(|m| {
                m["files"].as_object_mut().unwrap().remove("issues.ndjson");
            });
        }
        if fault == "decision" {
            let raw = b"{\"key\":\"ocm:poi:99999\",\"state\":\"ACCEPTED\",\"reason\":null}\n";
            std::fs::write(f.root.join("input/decisions.ndjson"), raw).unwrap();
            f.alter_manifest(|m| {
                m["files"]["decisions.ndjson"] =
                    json!({"bytes":raw.len(),"sha256":hash(raw),"records":1})
            });
        }
        let input = f.inputs("rejected");
        assert!(prepare(&input, &f.pins, &|| false).is_err(), "{fault}");
        assert!(!input.output.exists());
    }
}
#[test]
fn cancellation_and_existing_output_cannot_replace_last_good() {
    let f = Fixture::new();
    let input = f.inputs("cancelled");
    assert!(prepare(&input, &f.pins, &|| true).is_err());
    assert!(!input.output.exists());
    assert!(prepare(&input, &f.pins, &|| input.output.exists()).is_err());
    assert!(!input.output.exists());
    std::fs::create_dir(&input.output).unwrap();
    std::fs::write(input.output.join("last-good"), b"[P25-TEST] retained").unwrap();
    assert!(prepare(&input, &f.pins, &|| false).is_err());
    assert_eq!(
        std::fs::read(input.output.join("last-good")).unwrap(),
        b"[P25-TEST] retained"
    );
}

#[test]
fn native_cli_refuses_staging_and_rule_overrides_before_io() {
    use std::process::Command;
    let args = [
        "--normalized",
        "synthetic-missing",
        "--manifest-sha256",
        "synthetic",
        "--boundaries",
        "synthetic-missing",
        "--output",
        "synthetic-missing",
    ];
    let out = Command::new(env!("CARGO_BIN_EXE_prepare_ev_catalog"))
        .args(args)
        .env("DEPLOY_ENV", "staging")
        .output()
        .unwrap();
    assert!(!out.status.success());
    assert!(String::from_utf8_lossy(&out.stderr).contains("local-only"));
    let out = Command::new(env!("CARGO_BIN_EXE_prepare_ev_catalog"))
        .args(args)
        .args(["--confirmed-max-m", "999"])
        .output()
        .unwrap();
    assert!(!out.status.success());
    assert!(String::from_utf8_lossy(&out.stderr).contains("overrides refused"));
}

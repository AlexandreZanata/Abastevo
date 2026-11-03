use serde_json::json;
use sha2::{Digest, Sha256};
use station_prep::{parse_registry, registry_names, AliasTable, Limits};
use std::path::{Path, PathBuf};
use std::sync::atomic::{AtomicUsize, Ordering};

struct Fixture(PathBuf);
impl Fixture {
    fn new() -> Self {
        static NEXT: AtomicUsize = AtomicUsize::new(0);
        let path = std::env::temp_dir().join(format!(
            "sna-names-{}-{}",
            std::process::id(),
            NEXT.fetch_add(1, Ordering::Relaxed)
        ));
        std::fs::create_dir(&path).unwrap();
        Self(path)
    }
    fn path(&self) -> &Path {
        &self.0
    }
}
impl Drop for Fixture {
    fn drop(&mut self) {
        let _ = std::fs::remove_dir_all(&self.0);
    }
}

fn input() -> (Vec<u8>, AliasTable) {
    let root = PathBuf::from(env!("CARGO_MANIFEST_DIR"))
        .join("../../contracts/testdata/station-prep/datasets");
    (
        std::fs::read(root.join("tiny.csv")).unwrap(),
        AliasTable::from_json(&std::fs::read_to_string(root.join("tiny-aliases.json")).unwrap())
            .unwrap(),
    )
}

fn pin(dir: &Path, source: &str, profile: &str, rows: Vec<serde_json::Value>) -> serde_json::Value {
    let file = format!("{source}.jsonl");
    let raw = rows
        .iter()
        .map(|r| r.to_string() + "\n")
        .collect::<String>();
    std::fs::write(dir.join(&file), &raw).unwrap();
    json!({"source":source,"profile":profile,"file":file,"reference":format!("https://example.invalid/{source}"),"edition":"synthetic-2026-11","sha256":format!("{:x}",Sha256::digest(raw.as_bytes())),"bytes":raw.len(),"rows":rows.len()})
}

#[test]
fn active_rfb_wins_and_inactive_or_placeholder_falls_back_to_latest_survey() {
    let (csv, aliases) = input();
    let legacy =
        parse_registry("synthetic.csv", csv.clone(), &Limits::default(), &aliases).unwrap();
    let ids: Vec<_> = legacy
        .accepted
        .iter()
        .take(3)
        .map(|r| r.source_key.clone())
        .collect();
    let f = Fixture::new();
    let rfb = pin(
        f.path(),
        "rfb",
        "rfb-station-names-v1",
        vec![
            json!({"schema_version":"rfb-station-names-v1","cnpj":ids[0],"trade_name":"[P25-TEST] RFB Trade","active":true,"observed_at":"2026-10-01"}),
            json!({"schema_version":"rfb-station-names-v1","cnpj":ids[1],"trade_name":"[P25-TEST] Old RFB","active":true,"observed_at":"2026-09-01"}),
            json!({"schema_version":"rfb-station-names-v1","cnpj":ids[1],"trade_name":"[P25-TEST] Inactive","active":false,"observed_at":"2026-10-01"}),
            json!({"schema_version":"rfb-station-names-v1","cnpj":ids[2],"trade_name":"NÃO POSSUI","active":true,"observed_at":"2026-10-01"}),
        ],
    );
    let survey=pin(f.path(),"anp-survey","anp-station-price-v1",ids.iter().flat_map(|id|vec![
        json!({"schema_version":"anp-station-price-v1","cnpj":id,"trade_name":"[P25-TEST] Old Survey","observed_at":"2026-09-01"}),
        json!({"schema_version":"anp-station-price-v1","cnpj":id,"trade_name":"[P25-TEST] Latest Survey","observed_at":"2026-10-02"})]).collect());
    let descriptor = f.path().join("names.json");
    std::fs::write(
        &descriptor,
        json!({"format_version":"registry-name-inputs-v1","inputs":[rfb,survey]}).to_string(),
    )
    .unwrap();
    let names = registry_names::load_name_inputs(&descriptor).unwrap();
    let mut batch = station_prep::registry::parse_registry_named(
        "synthetic.csv",
        csv,
        &Limits::default(),
        &aliases,
    )
    .unwrap();
    registry_names::enrich_registry_names(&mut batch, &names).unwrap();
    assert_eq!(batch.counts, legacy.counts);
    for (index, id) in ids.iter().enumerate() {
        let row = batch.accepted.iter().find(|r| &r.source_key == id).unwrap();
        assert_eq!(row.schema_version, "station-assertion-v3");
        assert_eq!(
            row.trade_name.as_deref(),
            Some(if index == 0 {
                "[P25-TEST] RFB Trade"
            } else {
                "[P25-TEST] Latest Survey"
            })
        );
        assert_eq!(
            row.trade_name_provenance.as_ref().unwrap().source,
            if index == 0 { "rfb" } else { "anp-survey" }
        );
    }
    let once: Vec<_> = batch.accepted.iter().map(|r| r.to_jsonl()).collect();
    let (csv, aliases) = input();
    let mut again = station_prep::registry::parse_registry_named(
        "synthetic.csv",
        csv,
        &Limits::default(),
        &aliases,
    )
    .unwrap();
    registry_names::enrich_registry_names(&mut again, &names).unwrap();
    assert_eq!(
        once,
        again
            .accepted
            .iter()
            .map(|r| r.to_jsonl())
            .collect::<Vec<_>>()
    );
    let hashes: std::collections::HashSet<_> =
        batch.accepted.iter().map(|r| r.checksum.as_str()).collect();
    assert!(batch
        .accepted
        .iter()
        .all(|r| r.supersedes.as_deref().is_none_or(|s| hashes.contains(s))));
}

#[test]
fn pinned_inputs_refuse_tamper_unknown_profile_and_conflicting_clock() {
    let (csv, aliases) = input();
    let batch = parse_registry("synthetic.csv", csv, &Limits::default(), &aliases).unwrap();
    let id = &batch.accepted[0].source_key;
    for fault in ["hash", "profile", "conflict", "privacy", "traversal"] {
        let f = Fixture::new();
        let mut rows = vec![
            json!({"schema_version":"rfb-station-names-v1","cnpj":id,"trade_name":"[P25-TEST] One","active":true,"observed_at":"2026-10-01"}),
        ];
        if fault == "conflict" {
            let mut other = rows[0].clone();
            other["trade_name"] = json!("[P25-TEST] Two");
            rows.push(other);
        }
        if fault == "privacy" {
            rows[0]["owner_contact"] = json!("synthetic forbidden field");
        }
        let mut p = pin(f.path(), "rfb", "rfb-station-names-v1", rows);
        if fault == "hash" {
            p["sha256"] = json!("0".repeat(64));
        }
        if fault == "profile" {
            p["profile"] = json!("unknown");
        }
        if fault == "traversal" {
            p["file"] = json!("../rfb.jsonl");
        }
        let descriptor = f.path().join("names.json");
        std::fs::write(
            &descriptor,
            json!({"format_version":"registry-name-inputs-v1","inputs":[p]}).to_string(),
        )
        .unwrap();
        assert!(
            registry_names::load_name_inputs(&descriptor).is_err(),
            "{fault}"
        );
    }
}

#[test]
fn names_cli_verifies_before_publication_and_preserves_untrimmed_legal() {
    let f = Fixture::new();
    let (csv, _) = input();
    let fixtures = PathBuf::from(env!("CARGO_MANIFEST_DIR"))
        .join("../../contracts/testdata/station-prep/datasets");
    std::fs::write(f.path().join("registry.csv"), &csv).unwrap();
    std::fs::write(
        f.path().join("names.json"),
        json!({"format_version":"registry-name-inputs-v1","inputs":[]}).to_string(),
    )
    .unwrap();
    let run = |descriptor: &str, output: &str| {
        std::process::Command::new(env!("CARGO_BIN_EXE_prepare_registry"))
            .args([
                f.path().join("registry.csv"),
                fixtures.join("tiny-aliases.json"),
                f.path().join(output),
            ])
            .args([
                "https://example.invalid/registry",
                "synthetic",
                "11111111-1111-4111-8111-111111111111",
                "2026-11-03T00:00:00Z",
                "--name-inputs",
            ])
            .arg(f.path().join(descriptor))
            .output()
            .unwrap()
    };
    assert!(run("names.json", "out").status.success());
    let manifest: serde_json::Value =
        serde_json::from_slice(&std::fs::read(f.path().join("out/manifest.json")).unwrap())
            .unwrap();
    assert_eq!(
        manifest["parser_version"],
        registry_names::NAMED_PARSER_VERSION
    );
    let lines = std::fs::read_to_string(f.path().join("out/assertions.jsonl")).unwrap();
    for line in lines.lines() {
        let row: serde_json::Value = serde_json::from_str(line).unwrap();
        assert_eq!(row["schema_version"], "station-assertion-v3");
        assert!(row["legal_name"].is_string());
        assert_eq!(row["trade_name"], "");
        assert!(row.get("trade_name_provenance").is_none());
    }
    // Wrong declared bytes must fail before any output directory is published.
    let mut wrong = pin(f.path(), "rfb", "rfb-station-names-v1", vec![]);
    wrong["bytes"] = json!(1);
    std::fs::write(
        f.path().join("bad.json"),
        json!({"format_version":"registry-name-inputs-v1","inputs":[wrong]}).to_string(),
    )
    .unwrap();
    assert!(!run("bad.json", "refused").status.success());
    assert!(!f.path().join("refused").exists());
}

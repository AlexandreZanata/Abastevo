use serde_json::{json, Value};
use sha2::{Digest, Sha256};
use std::{
    path::PathBuf,
    process::Command,
    sync::atomic::{AtomicU64, Ordering},
};
fn sha(raw: &[u8]) -> String {
    format!("{:x}", Sha256::digest(raw))
}
struct Fixture {
    dir: PathBuf,
    pin: Value,
}
impl Fixture {
    fn new() -> Self {
        static NEXT: AtomicU64 = AtomicU64::new(0);
        let dir = std::env::temp_dir().join(format!(
            "sna04-addresses-{}-{}",
            std::process::id(),
            NEXT.fetch_add(1, Ordering::Relaxed)
        ));
        std::fs::create_dir(&dir).unwrap();
        std::fs::write(dir.join("registry.csv"),b"CODIGOISIMP;AUTORIZACAO;DATAPUBLICACAO;RAZAOSOCIAL;CNPJ;ENDERECO;COMPLEMENTO;BAIRRO;CEP;UF;MUNICIPIO;BANDEIRA;DATAVINCULACAO\nSNA-TEST;SYNTHETIC;01/01/2024;[P25-TEST] Legal LTDA;91000000000066;;;;;SP;TEST CITY;BRANCA;01/01/2023\n").unwrap();
        std::fs::write(
            dir.join("aliases.json"),
            b"{\"entries\":[{\"uf\":\"SP\",\"ibge\":\"3550308\",\"aliases\":[\"TEST CITY\"]}]}",
        )
        .unwrap();
        let raw=b"{\"schema_version\":\"rfb-station-addresses-v1\",\"cnpj\":\"91000000000066\",\"active\":true,\"address\":{\"street\":\"[P25-TEST] RFB Street\",\"number\":\"12A\"},\"observed_at\":\"2026-10-01\"}\n";
        std::fs::write(dir.join("rfb.jsonl"), raw).unwrap();
        let pin = json!({"source":"rfb","profile":"rfb-station-addresses-v1","file":"rfb.jsonl","reference":"https://example.invalid/rfb","edition":"[P25-TEST] synthetic","sha256":sha(raw),"bytes":raw.len(),"rows":1,"license":"Creative Commons Attribution","attribution":"Receita Federal do Brasil — Cadastro Nacional da Pessoa Jurídica (CNPJ)"});
        let f = Self { dir, pin };
        f.descriptor(f.pin.clone());
        f
    }
    fn descriptor(&self, pin: Value) {
        std::fs::write(
            self.dir.join("addresses.json"),
            json!({"format_version":"registry-address-inputs-v1","inputs":[pin]}).to_string(),
        )
        .unwrap();
    }
    fn command(&self, out: &str) -> Command {
        let mut c = Command::new(env!("CARGO_BIN_EXE_prepare_registry"));
        c.args([
            self.dir.join("registry.csv"),
            self.dir.join("aliases.json"),
            self.dir.join(out),
        ])
        .args([
            "https://example.invalid/registry",
            "synthetic",
            "55555555-1111-4111-8111-111111111111",
            "2026-11-03T00:00:00Z",
            "--address-inputs",
        ])
        .arg(self.dir.join("addresses.json"));
        c
    }
    fn read(&self, out: &str, name: &str) -> Value {
        serde_json::from_slice(&std::fs::read(self.dir.join(out).join(name)).unwrap()).unwrap()
    }
}
impl Drop for Fixture {
    fn drop(&mut self) {
        let _ = std::fs::remove_dir_all(&self.dir);
    }
}
#[test]
fn real_cli_emits_v4_bound_address_facts_and_reconciled_coverage() {
    let f = Fixture::new();
    for out in ["one", "two"] {
        let r = f.command(out).output().unwrap();
        assert!(r.status.success(), "{}", String::from_utf8_lossy(&r.stderr));
    }
    for name in [
        "assertions.jsonl",
        "candidates.jsonl",
        "quarantine.jsonl",
        "manifest.json",
    ] {
        assert_eq!(
            std::fs::read(f.dir.join("one").join(name)).unwrap(),
            std::fs::read(f.dir.join("two").join(name)).unwrap()
        );
    }
    let row = f.read("one", "assertions.jsonl");
    assert_eq!(row["schema_version"], "station-assertion-v4");
    assert_eq!(row["address_raw"], "");
    assert_eq!(row["address_normalized"]["street"], "");
    assert_eq!(row["address_facts"]["source"], "rfb");
    assert_eq!(row["address_facts"]["components"]["number"], "12A");
    assert_eq!(
        row["address_facts"]["provenance"]["input_sha256"],
        f.pin["sha256"]
    );
    assert_eq!(row["latitude"], Value::Null);
    assert_eq!(row["location_quality"], "unknown");
    let m = f.read("one", "manifest.json");
    assert_eq!(m["parser_version"], "station-prep-v0.3.1+addresses-v1");
    assert_eq!(m["address_inputs"][0]["sha256"], f.pin["sha256"]);
    assert_eq!(
        m["address_coverage"]["by_uf"]["SP"]["before"]["municipality"],
        1
    );
    assert_eq!(m["address_coverage"]["by_uf"]["SP"]["after"]["rfb"], 1);
}
#[test]
fn unadmitted_cnefe_is_skipped_and_does_not_require_reading_its_rows() {
    let f = Fixture::new();
    let mut p = f.pin.clone();
    p["source"] = json!("cnefe");
    p["profile"] = json!("cnefe-station-addresses-v1");
    p["file"] = json!("not-present.jsonl");
    p["license"] = json!("CC-BY-4.0");
    f.descriptor(p);
    let r = f.command("out").output().unwrap();
    assert!(r.status.success(), "{}", String::from_utf8_lossy(&r.stderr));
    let row = f.read("out", "assertions.jsonl");
    assert_eq!(row["address_facts"]["source"], "municipality");
    let m = f.read("out", "manifest.json");
    assert_eq!(m["address_skips"][0]["source"], "cnefe");
    assert_eq!(m["address_inputs"], json!([]));
}
#[test]
fn changed_bytes_paths_profiles_counts_rights_and_extra_contacts_refuse_before_output() {
    for fault in [
        "hash",
        "path",
        "symlink",
        "profile",
        "rows",
        "bytes",
        "license",
        "attribution",
        "contact",
        "duplicate",
        "truncated",
    ] {
        let f = Fixture::new();
        let mut p = f.pin.clone();
        match fault {
            "hash" => p["sha256"] = json!("b".repeat(64)),
            "path" => p["file"] = json!("../rfb.jsonl"),
            "symlink" => {
                std::os::unix::fs::symlink(f.dir.join("rfb.jsonl"), f.dir.join("link.jsonl"))
                    .unwrap();
                p["file"] = json!("link.jsonl")
            }
            "profile" => p["profile"] = json!("unknown"),
            "rows" => p["rows"] = json!(2),
            "bytes" => p["bytes"] = json!(1),
            "license" => p["license"] = json!("invented"),
            "attribution" => p["attribution"] = json!(""),
            "contact" | "truncated" => {
                let path = f.dir.join("rfb.jsonl");
                let mut v: Value = serde_json::from_slice(&std::fs::read(&path).unwrap()).unwrap();
                if fault == "contact" {
                    v["email"] = json!("synthetic@example.invalid");
                }
                let raw = v.to_string() + if fault == "truncated" { "" } else { "\n" };
                std::fs::write(&path, &raw).unwrap();
                p["bytes"] = json!(raw.len());
                p["sha256"] = json!(sha(raw.as_bytes()));
            }
            "duplicate" => {}
            _ => unreachable!(),
        }
        f.descriptor(p.clone());
        if fault == "duplicate" {
            std::fs::write(
                f.dir.join("addresses.json"),
                json!({"format_version":"registry-address-inputs-v1","inputs":[p.clone(),p]})
                    .to_string(),
            )
            .unwrap();
        }
        let r = f.command("out").output().unwrap();
        assert!(!r.status.success(), "{fault}");
        assert!(!f.dir.join("out").exists(), "{fault}");
    }
}

#[test]
fn matched_osm_and_named_inputs_keep_unique_current_station_denominator() {
    let f = Fixture::new();
    let raw = b"{\"schema_version\":\"osm-station-addresses-v1\",\"cnpj\":\"91000000000066\",\"object_type\":\"node\",\"object_id\":90001,\"match_reference\":\"[P25-TEST] existing exact match\",\"address\":{\"street\":\"[P25-TEST] OSM Street\",\"number\":\"12\"},\"observed_at\":\"2026-10-01\"}\n";
    std::fs::write(f.dir.join("osm.jsonl"), raw).unwrap();
    f.descriptor(json!({"source":"osm","profile":"osm-station-addresses-v1","file":"osm.jsonl","reference":"https://example.invalid/osm","edition":"[P25-TEST] synthetic","sha256":sha(raw),"bytes":raw.len(),"rows":1,"license":"ODbL-1.0","attribution":"© OpenStreetMap contributors"}));
    let name = b"{\"schema_version\":\"rfb-station-names-v1\",\"cnpj\":\"91000000000066\",\"trade_name\":\"[P25-TEST] Trade\",\"active\":true,\"observed_at\":\"2026-10-01\"}\n";
    std::fs::write(f.dir.join("name.jsonl"), name).unwrap();
    std::fs::write(f.dir.join("names.json"),json!({"format_version":"registry-name-inputs-v1","inputs":[{"source":"rfb","profile":"rfb-station-names-v1","file":"name.jsonl","reference":"https://example.invalid/rfb","edition":"[P25-TEST] synthetic","sha256":sha(name),"bytes":name.len(),"rows":1}]}).to_string()).unwrap();
    let csv = std::fs::read_to_string(f.dir.join("registry.csv")).unwrap();
    let current = csv.lines().nth(1).unwrap();
    let historical = current
        .replace("01/01/2024", "01/01/2023")
        .replace(";;;;;SP", ";[P25-TEST] Historical Street 12;;;;SP");
    std::fs::write(
        f.dir.join("registry.csv"),
        format!("{csv}{current}\n{historical}\n"),
    )
    .unwrap();
    let r = f
        .command("out")
        .arg("--name-inputs")
        .arg(f.dir.join("names.json"))
        .output()
        .unwrap();
    assert!(r.status.success(), "{}", String::from_utf8_lossy(&r.stderr));
    let m = f.read("out", "manifest.json");
    assert_eq!(m["counts"]["registry-13col"]["accepted"], 2);
    assert_eq!(m["counts"]["registry-13col"]["duplicates"], 1);
    assert_eq!(m["address_coverage"]["station_count"], 1);
    assert_eq!(m["address_coverage"]["national"]["after"]["osm"], 1);
    let assertions = std::fs::read_to_string(f.dir.join("out/assertions.jsonl")).unwrap();
    let rows: Vec<Value> = assertions
        .lines()
        .map(|line| serde_json::from_str(line).unwrap())
        .collect();
    let current: Vec<_> = rows
        .iter()
        .filter(|row| row["supersedes"].is_null())
        .collect();
    assert_eq!(current.len(), 2);
    assert_eq!(current[0], current[1]);
    assert_eq!(current[0]["trade_name"], "[P25-TEST] Trade");
    assert_eq!(
        current[0]["address_facts"]["provenance"]["proof"]["object_id"],
        90001
    );
    let historical = rows
        .iter()
        .find(|row| !row["supersedes"].is_null())
        .unwrap();
    assert_eq!(historical["address_facts"]["source"], "anp");
    assert_eq!(historical["supersedes"], current[0]["checksum"]);
}

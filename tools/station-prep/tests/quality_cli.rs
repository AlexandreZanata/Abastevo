use sha2::{Digest, Sha256};
use std::path::{Path, PathBuf};
use std::process::Command;
struct Scratch(PathBuf);
impl Scratch {
    fn new() -> Self {
        let p = std::env::temp_dir().join(format!(
            "quality-cli-{}-{}",
            std::process::id(),
            std::time::SystemTime::now()
                .duration_since(std::time::UNIX_EPOCH)
                .unwrap()
                .as_nanos()
        ));
        std::fs::create_dir(&p).unwrap();
        Self(p)
    }
}
impl Drop for Scratch {
    fn drop(&mut self) {
        std::fs::remove_dir_all(&self.0).unwrap();
    }
}
fn cli(input: &Path, out: &Path) -> std::process::Output {
    Command::new(env!("CARGO_BIN_EXE_prepare_quality"))
        .arg(input)
        .arg(out)
        .output()
        .unwrap()
}

#[test]
fn quality_cli_artifacts_are_hashed_balanced_and_refuse_overwrite() {
    let s = Scratch::new();
    let input = PathBuf::from(env!("CARGO_MANIFEST_DIR"))
        .join("../../contracts/testdata/station-prep/quality-assay-sample.csv");
    let out = s.0.join("prepared");
    let res = cli(&input, &out);
    assert!(
        res.status.success(),
        "{}",
        String::from_utf8_lossy(&res.stderr)
    );
    let manifest: serde_json::Value =
        serde_json::from_slice(&std::fs::read(out.join("manifest.json")).unwrap()).unwrap();
    assert_eq!(manifest["counts"]["input"], 12);
    assert_eq!(manifest["counts"]["accepted"], 7);
    for artifact in manifest["outputs"].as_array().unwrap() {
        let b = std::fs::read(out.join(artifact["file"].as_str().unwrap())).unwrap();
        assert_eq!(artifact["bytes"].as_u64().unwrap(), b.len() as u64);
        assert_eq!(
            artifact["sha256"].as_str().unwrap(),
            format!("{:x}", Sha256::digest(&b))
        );
    }
    let before = std::fs::read(out.join("manifest.json")).unwrap();
    assert!(!cli(&input, &out).status.success());
    assert_eq!(before, std::fs::read(out.join("manifest.json")).unwrap());
    assert!(
        !String::from_utf8(res.stdout)
            .unwrap()
            .contains("48151623000191"),
        "stdout carries counters only"
    );
}
#[test]
fn quality_cli_quarantined_and_oversize_sources_create_no_publishable_manifest() {
    let s = Scratch::new();
    let input = s.0.join("bad.csv");
    std::fs::write(&input, "Unknown;Header\na;b\n").unwrap();
    let out = s.0.join("bad-out");
    assert!(!cli(&input, &out).status.success());
    assert!(!out.exists());
    let f = std::fs::File::create(s.0.join("huge.csv")).unwrap();
    f.set_len((100 << 20) + 1).unwrap();
    assert!(!cli(&s.0.join("huge.csv"), &out).status.success());
    assert!(!out.exists());
}

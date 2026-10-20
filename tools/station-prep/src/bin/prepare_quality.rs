//! Bounded local PMQC preparation. No network, database or publication.
use sha2::{Digest, Sha256};
use station_prep::{parse_quality_assays, Limits, RunState};
use std::io::{BufWriter, Read, Write};
use std::path::Path;

fn emit<T: serde::Serialize>(
    path: &Path,
    rows: &[T],
) -> Result<(u64, String), Box<dyn std::error::Error>> {
    let mut file = BufWriter::new(std::fs::File::create_new(path)?);
    let mut hash = Sha256::new();
    let mut bytes = 0u64;
    for row in rows {
        let mut line = serde_json::to_vec(row)?;
        line.push(b'\n');
        file.write_all(&line)?;
        hash.update(&line);
        bytes += line.len() as u64;
    }
    file.flush()?;
    file.get_ref().sync_all()?;
    Ok((bytes, format!("{:x}", hash.finalize())))
}

fn run() -> Result<(), Box<dyn std::error::Error>> {
    let args: Vec<String> = std::env::args().collect();
    if args.len() != 3 {
        return Err("usage: prepare_quality CSV NEW_OUTPUT_DIRECTORY".into());
    }
    let limits = Limits::default();
    let mut raw = Vec::new();
    std::fs::File::open(&args[1])?
        .take(limits.max_bytes + 1)
        .read_to_end(&mut raw)?;
    if raw.len() as u64 > limits.max_bytes {
        return Err("quality input exceeds byte limit".into());
    }
    let input_bytes = raw.len();
    let input_hash = format!("{:x}", Sha256::digest(&raw));
    let native = std::str::from_utf8(&raw)?
        .lines()
        .next()
        .unwrap_or("")
        .to_lowercase()
        .contains("idnumeric");
    let name = Path::new(&args[1])
        .file_name()
        .and_then(|n| n.to_str())
        .ok_or("input name must be UTF-8")?;
    let batch = parse_quality_assays(name, raw, &limits)
        .map_err(|e| format!("quality preparation refused: {e:?}"))?;
    if batch.state != RunState::Complete {
        return Err("quality source/header quarantined: no output directory created".into());
    }
    let out = Path::new(&args[2]);
    std::fs::create_dir(out)?;
    let assays = emit(&out.join("assays.jsonl"), &batch.assays)?;
    let duplicates = emit(&out.join("duplicates.jsonl"), &batch.duplicates)?;
    let quarantine = emit(&out.join("quarantine.jsonl"), &batch.quarantine)?;
    let manifest = serde_json::json!({"schema_version":"quality-prepared-v1","input_contract":if native{"pmqc-native-19-v2026-10b"}else{"pmqc-prepared-8-v1"},"source_file":name,"input_bytes":input_bytes,"input_sha256":input_hash,"manifest_hash":batch.manifest_hash(),"counts":{"input":batch.counts.input,"accepted":batch.counts.accepted,"duplicates":batch.counts.duplicates,"quarantined":batch.counts.quarantined},"outputs":[{"file":"assays.jsonl","bytes":assays.0,"sha256":assays.1},{"file":"duplicates.jsonl","bytes":duplicates.0,"sha256":duplicates.1},{"file":"quarantine.jsonl","bytes":quarantine.0,"sha256":quarantine.1}]});
    let mut f = std::fs::File::create_new(out.join("manifest.partial"))?;
    serde_json::to_writer_pretty(&mut f, &manifest)?;
    f.write_all(b"\n")?;
    f.sync_all()?;
    std::fs::rename(out.join("manifest.partial"), out.join("manifest.json"))?;
    println!(
        "input={} accepted={} duplicates={} quarantined={} artifacts=file-backed",
        batch.counts.input,
        batch.counts.accepted,
        batch.counts.duplicates,
        batch.counts.quarantined
    );
    Ok(())
}
fn main() {
    if let Err(e) = run() {
        eprintln!("prepare_quality: {e}");
        std::process::exit(1);
    }
}

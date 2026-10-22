//! Bounded native preparation of the official enforcement workbook.
//! Private artifacts; counts-only stdout.
use sha2::{Digest, Sha256};
use station_prep::enforcement_xlsx::{
    parse_enforcement_xlsx, EnforcementXlsxError, ENFORCEMENT_XLSX_PROFILE,
};
use station_prep::Limits;
use std::io::{BufWriter, Read, Write};
use std::os::unix::fs::{DirBuilderExt, OpenOptionsExt};
use std::path::Path;

fn run() -> Result<(), Box<dyn std::error::Error>> {
    let args: Vec<String> = std::env::args().collect();
    if args.len() != 3 {
        return Err("usage: prepare_enforcement XLSX NEW_OUTPUT_DIRECTORY".into());
    }
    let limits = Limits::default();
    let mut raw = Vec::new();
    std::fs::File::open(&args[1])?
        .take(limits.max_bytes + 1)
        .read_to_end(&mut raw)?;
    if raw.len() as u64 > limits.max_bytes {
        return Err("input exceeds the preparation budget".into());
    }
    let input_bytes = raw.len();
    let input_hash = format!("{:x}", Sha256::digest(&raw));
    let name = Path::new(&args[1])
        .file_name()
        .and_then(|value| value.to_str())
        .ok_or("name refused")?
        .to_string();
    let out = Path::new(&args[2]);
    std::fs::DirBuilder::new().mode(0o700).create(out)?;
    let mut summary = (0usize, 0usize, 0usize, 0usize);

    let result = (|| -> Result<(), Box<dyn std::error::Error>> {
        let file = std::fs::OpenOptions::new()
            .write(true)
            .create_new(true)
            .mode(0o600)
            .open(out.join("enforcement-source-records.jsonl"))?;
        let mut writer = BufWriter::new(file);
        let mut hash = Sha256::new();
        let mut written = 0u64;
        let run = parse_enforcement_xlsx(&name, raw, &limits, |record| {
            let mut line = serde_json::to_vec(record).map_err(|error| {
                EnforcementXlsxError::Contract(format!("record encoding refused: {error}"))
            })?;
            line.push(b'\n');
            writer
                .write_all(&line)
                .map_err(|error| EnforcementXlsxError::Truncated(format!("write: {error}")))?;
            hash.update(&line);
            written += line.len() as u64;
            Ok(())
        })?;
        summary = (
            run.counts.input,
            run.counts.accepted,
            run.counts.duplicates,
            run.counts.quarantined,
        );
        writer.flush()?;
        writer.get_ref().sync_all()?;
        let records = serde_json::json!({
            "file": "enforcement-source-records.jsonl",
            "bytes": written,
            "sha256": format!("{:x}", hash.finalize()),
        });
        let manifest = serde_json::json!({
            "schema_version": "enforcement-native-manifest-v1",
            "source_profile": ENFORCEMENT_XLSX_PROFILE,
            "source_file": name,
            "input_bytes": input_bytes,
            "input_sha256": input_hash,
            "manifest_hash": run.manifest_hash(),
            "counts": {
                "input": run.counts.input,
                "accepted": run.counts.accepted,
                "duplicates": run.counts.duplicates,
                "quarantined": run.counts.quarantined,
            },
            "census": run.census,
            "outputs": [records],
        });
        let mut manifest_file = std::fs::OpenOptions::new()
            .write(true)
            .create_new(true)
            .mode(0o600)
            .open(out.join("manifest.partial"))?;
        serde_json::to_writer_pretty(&mut manifest_file, &manifest)?;
        manifest_file.write_all(b"\n")?;
        manifest_file.sync_all()?;
        std::fs::rename(out.join("manifest.partial"), out.join("manifest.json"))?;
        std::fs::File::open(out)?.sync_all()?;
        Ok(())
    })();
    if result.is_err() {
        std::fs::remove_dir_all(out)?;
        return result;
    }
    println!(
        "input={} accepted={} duplicates={} quarantined={} artifacts=file-backed",
        summary.0, summary.1, summary.2, summary.3
    );
    Ok(())
}

fn main() {
    if run().is_err() {
        // Source paths, rows and records never enter process logs.
        eprintln!("prepare_enforcement: source contract or artifact operation refused");
        std::process::exit(1);
    }
}

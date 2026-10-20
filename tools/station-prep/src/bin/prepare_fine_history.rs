//! Bounded historical SICOM profile preparation. Private artifacts; counts-only stdout.
use sha2::{Digest, Sha256};
use station_prep::{sanction_history::parse_fine_history, Limits};
use std::io::{BufWriter, Read, Write};
use std::os::unix::fs::{DirBuilderExt, OpenOptionsExt};
use std::path::Path;

fn emit<T: serde::Serialize>(
    path: &Path,
    rows: &[T],
    budget: &mut usize,
) -> Result<serde_json::Value, Box<dyn std::error::Error>> {
    let file = std::fs::OpenOptions::new()
        .write(true)
        .create_new(true)
        .mode(0o600)
        .open(path)?;
    let mut output = BufWriter::new(file);
    let mut hash = Sha256::new();
    let mut bytes = 0;
    for row in rows {
        let mut line = serde_json::to_vec(row)?;
        line.push(b'\n');
        *budget = budget.checked_sub(line.len()).ok_or("output budget")?;
        output.write_all(&line)?;
        hash.update(&line);
        bytes += line.len();
    }
    output.flush()?;
    output.get_ref().sync_all()?;
    Ok(
        serde_json::json!({"file":path.file_name().and_then(|s|s.to_str()).ok_or("name")?,"bytes":bytes,"sha256":format!("{:x}",hash.finalize())}),
    )
}

fn run() -> Result<(), Box<dyn std::error::Error>> {
    let args: Vec<String> = std::env::args().collect();
    if args.len() != 4 {
        return Err("usage: prepare_fine_history FILE SOURCE_PERIOD NEW_OUTPUT_DIRECTORY".into());
    }
    let limits = Limits::default();
    let mut raw = Vec::new();
    std::fs::File::open(&args[1])?
        .take(limits.max_bytes + 1)
        .read_to_end(&mut raw)?;
    let input_bytes = raw.len();
    let input_hash = format!("{:x}", Sha256::digest(&raw));
    let name = Path::new(&args[1])
        .file_name()
        .and_then(|s| s.to_str())
        .ok_or("name refused")?;
    let batch = parse_fine_history(name, raw, &args[2], &limits)
        .map_err(|_| "official fine contract refused")?;
    let out = Path::new(&args[3]);
    std::fs::DirBuilder::new().mode(0o700).create(out)?;
    let result = (|| -> Result<(), Box<dyn std::error::Error>> {
        let mut budget = limits.max_bytes as usize;
        let outputs = vec![
            emit(&out.join("history.jsonl"), &batch.observations, &mut budget)?,
            emit(
                &out.join("duplicates.jsonl"),
                &batch.duplicates,
                &mut budget,
            )?,
            emit(
                &out.join("quarantine.jsonl"),
                &batch.quarantine,
                &mut budget,
            )?,
            emit(
                &out.join("source_notes.jsonl"),
                &batch.metadata,
                &mut budget,
            )?,
        ];
        let manifest = serde_json::json!({"schema_version":"fine-history-prepared-v1","input_contract":batch.input_contract,"source_file":name,"input_bytes":input_bytes,"input_sha256":input_hash,"source_period":batch.source_period,"source_clock":batch.clock,"header_record":batch.header_record,"metadata_records":batch.metadata.len(),"manifest_hash":batch.manifest_hash(),"counts":{"input":batch.counts.input,"accepted":batch.counts.accepted,"duplicates":batch.counts.duplicates,"quarantined":batch.counts.quarantined},"outputs":outputs});
        let mut f = std::fs::OpenOptions::new()
            .write(true)
            .create_new(true)
            .mode(0o600)
            .open(out.join("manifest.partial"))?;
        serde_json::to_writer_pretty(&mut f, &manifest)?;
        f.write_all(b"\n")?;
        f.sync_all()?;
        std::fs::rename(out.join("manifest.partial"), out.join("manifest.json"))?;
        std::fs::File::open(out)?.sync_all()?;
        Ok(())
    })();
    if result.is_err() {
        std::fs::remove_dir_all(out)?;
    }
    result?;
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
    if run().is_err() {
        // Source/paths/records never enter process logs, even on filesystem errors.
        eprintln!("prepare_fine_history: source contract or artifact operation refused");
        std::process::exit(1);
    }
}

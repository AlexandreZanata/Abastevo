//! Bounded native SICOM CSV preparation. Private artifacts; counts-only stdout.
use sha2::{Digest, Sha256};
use station_prep::{sanction_anp::parse_anp_fines, Limits};
use std::io::{BufWriter, Read, Write};
use std::os::unix::fs::{DirBuilderExt, OpenOptionsExt};
use std::path::Path;

fn emit<T: serde::Serialize>(
    path: &Path,
    rows: &[T],
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
        return Err("usage: prepare_sanctions CSV YEAR NEW_OUTPUT_DIRECTORY".into());
    }
    let year = args[2].parse::<u16>().map_err(|_| "year refused")?;
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
    let batch =
        parse_anp_fines(name, raw, year, &limits).map_err(|_| "official fine contract refused")?;
    let out = Path::new(&args[3]);
    std::fs::DirBuilder::new().mode(0o700).create(out)?;
    let result = (|| -> Result<(), Box<dyn std::error::Error>> {
        let outputs = vec![
            emit(&out.join("sanctions.jsonl"), &batch.snapshots)?,
            emit(&out.join("duplicates.jsonl"), &batch.duplicates)?,
            emit(&out.join("quarantine.jsonl"), &batch.quarantine)?,
        ];
        let manifest = serde_json::json!({"schema_version":"sanctions-prepared-v1","input_contract":"anp-sicom-cp850-9-v1","source_file":name,"input_bytes":input_bytes,"input_sha256":input_hash,"report_year":batch.report_year,"extraction_month":batch.extraction_month,"manifest_hash":batch.manifest_hash(),"counts":{"input":batch.counts.input,"accepted":batch.counts.accepted,"duplicates":batch.counts.duplicates,"quarantined":batch.counts.quarantined},"outputs":outputs});
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
        eprintln!("prepare_sanctions: source contract or artifact operation refused");
        std::process::exit(1);
    }
}

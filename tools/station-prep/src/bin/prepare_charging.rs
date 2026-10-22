//! Bounded native preparation of the admitted OCM snapshot.
//! Private artifacts; counts-only stdout.
use sha2::{Digest, Sha256};
use station_prep::ev_contract::canonical_bytes;
use station_prep::ev_ocm::{parse_ocm_poi, OCM_PROFILE, OCM_SOURCE};
use std::collections::HashSet;
use std::io::{BufWriter, Write};
use std::os::unix::fs::{DirBuilderExt, OpenOptionsExt};
use std::path::{Path, PathBuf};

fn run() -> Result<(), Box<dyn std::error::Error>> {
    let args: Vec<String> = std::env::args().collect();
    if args.len() != 3 {
        return Err("usage: prepare_charging SNAPSHOT_DIR NEW_OUTPUT_DIRECTORY".into());
    }
    let snapshot = Path::new(&args[1]);
    let out = Path::new(&args[2]);
    let mut files: Vec<PathBuf> = std::fs::read_dir(snapshot)?
        .filter_map(|entry| entry.ok().map(|e| e.path()))
        .filter(|path| path.extension().is_some_and(|ext| ext == "json"))
        .collect();
    files.sort();
    if files.is_empty() || files.len() > 500_000 {
        return Err("snapshot file census refused".into());
    }
    std::fs::DirBuilder::new().mode(0o700).create(out)?;
    let result = (|| -> Result<(), Box<dyn std::error::Error>> {
        let rights = Sha256::digest(b"open-licenses-cc-by-40-cc-0-cc0-20261022");
        let rights_sha256 = format!("{rights:x}");
        let mut seen: HashSet<String> = HashSet::new();
        let mut providers: HashSet<i64> = HashSet::new();
        let mut entities_file = BufWriter::new(
            std::fs::OpenOptions::new()
                .write(true)
                .create_new(true)
                .mode(0o600)
                .open(out.join("entities.ndjson"))?,
        );
        let mut decisions_file = BufWriter::new(
            std::fs::OpenOptions::new()
                .write(true)
                .create_new(true)
                .mode(0o600)
                .open(out.join("decisions.ndjson"))?,
        );
        let mut entity_bytes = 0u64;
        let mut decision_bytes = 0u64;
        let mut accepted = 0usize;
        let mut duplicates = 0usize;
        let mut quarantined = 0usize;
        let mut quarantine_rows: Vec<serde_json::Value> = Vec::new();
        let mut input_inventory: Vec<serde_json::Value> = Vec::new();
        for path in &files {
            let name = path
                .file_name()
                .and_then(|n| n.to_str())
                .ok_or("name refused")?
                .to_string();
            let raw = std::fs::read(path)?;
            if raw.len() as u64 > 1_000_000 {
                return Err("source file exceeds record budget".into());
            }
            input_inventory.push(serde_json::json!({
                "filename": name,
                "bytes": raw.len(),
                "sha256": format!("{:x}", Sha256::digest(&raw)),
            }));
            let file_run = parse_ocm_poi(
                &name,
                &raw,
                &rights_sha256,
                &mut seen,
                &mut providers,
                &mut |entity| {
                    let record = serde_json::json!({
                        "kind": entity.kind,
                        "key": entity.key,
                        "parent_key": entity.parent_key,
                        "external_id": entity.external_id,
                        "unit": format!("{}:{}", entity.kind, entity.external_id),
                        "payload": entity.payload,
                        "field_provenance": {"source": OCM_SOURCE, "profile": OCM_PROFILE},
                        "rights_sha256": rights_sha256,
                        "source_clocks": entity.clocks,
                        "assertion_sha256": entity.assertion,
                    });
                    let mut line = canonical_bytes(&record).map_err(|e| {
                        station_prep::ev_ocm::OcmError::Contract(format!("entity encoding: {e}"))
                    })?;
                    line.push(b'\n');
                    entity_bytes += line.len() as u64;
                    if entity_bytes > 100_000_000 {
                        return Err(station_prep::ev_ocm::OcmError::Contract(
                            "artifact budget exceeded".to_string(),
                        ));
                    }
                    entities_file.write_all(&line).map_err(|e| {
                        station_prep::ev_ocm::OcmError::Truncated(format!("write: {e}"))
                    })?;
                    Ok(())
                },
            )?;
            accepted += file_run.accepted;
            duplicates += file_run.duplicates;
            quarantined += file_run.quarantined;
            for row in file_run.quarantine {
                quarantine_rows.push(serde_json::json!({
                    "schema_version": row.schema_version,
                    "source": row.source,
                    "row_locator": row.row_locator,
                    "reason": row.reason,
                    "detail": row.detail,
                }));
            }
            for entity in file_run.entities {
                let decision = serde_json::json!({
                    "unit": format!("{}:{}", entity.kind, entity.external_id),
                    "kind": entity.kind,
                    "source_file": name,
                    "root": entity.kind == "PLACE",
                    "pointer": entity.external_id,
                    "parent_unit": entity.parent_key,
                    "outcome": "accepted",
                    "key": entity.key,
                    "external_id": entity.external_id,
                    "assertion_sha256": entity.assertion,
                    "duplicate_of": Value::Null,
                    "reason": "",
                    "raw_sha256": format!("{:x}", Sha256::digest(&raw)),
                });
                let mut line = canonical_bytes(&decision).map_err(|e| {
                    station_prep::ev_ocm::OcmError::Contract(format!("decision encoding: {e}"))
                })?;
                line.push(b'\n');
                decision_bytes += line.len() as u64;
                decisions_file.write_all(&line).map_err(|e| {
                    station_prep::ev_ocm::OcmError::Truncated(format!("write: {e}"))
                })?;
            }
        }
        entities_file.flush()?;
        decisions_file.flush()?;
        let mut providers_sorted: Vec<i64> = providers.into_iter().collect();
        providers_sorted.sort_unstable();
        let manifest = serde_json::json!({
            "schema_version": "charging-prepared-v1",
            "source_profile": OCM_PROFILE,
            "source_revision": "8e3bedca48ca94807d96b2f5e7ee02cffe63ae54",
            "counts": {"input": accepted + duplicates + quarantined, "accepted": accepted, "duplicates": duplicates, "quarantined": quarantined},
            "census": {"places": accepted, "providers": providers_sorted},
            "inputs": input_inventory,
            "outputs": [
                {"file": "entities.ndjson", "bytes": entity_bytes},
                {"file": "decisions.ndjson", "bytes": decision_bytes},
                {"file": "aliases.ndjson", "bytes": 0},
            ],
        });
        let mut manifest_bytes =
            canonical_bytes(&manifest).map_err(|e| format!("manifest: {e}"))?;
        manifest_bytes.push(b'\n');
        std::fs::OpenOptions::new()
            .write(true)
            .create_new(true)
            .mode(0o600)
            .open(out.join("aliases.ndjson"))?;
        let mut manifest_file = std::fs::OpenOptions::new()
            .write(true)
            .create_new(true)
            .mode(0o600)
            .open(out.join("manifest.partial"))?;
        manifest_file.write_all(&manifest_bytes)?;
        manifest_file.sync_all()?;
        std::fs::rename(out.join("manifest.partial"), out.join("manifest.json"))?;
        std::fs::File::open(out)?.sync_all()?;
        println!(
            "input={} accepted={} duplicates={} quarantined={} artifacts=file-backed",
            accepted + duplicates + quarantined,
            accepted,
            duplicates,
            quarantined
        );
        Ok(())
    })();
    if result.is_err() {
        std::fs::remove_dir_all(out)?;
        return result;
    }
    Ok(())
}

fn main() {
    if run().is_err() {
        eprintln!("prepare_charging: source contract or artifact operation refused");
        std::process::exit(1);
    }
}

use serde_json::Value;

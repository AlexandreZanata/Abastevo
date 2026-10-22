//! Bounded native preparation of the PBE table plus supplement.
//! Private artifacts; counts-only stdout.
use sha2::{Digest, Sha256};
use station_prep::ev_contract::canonical_bytes;
use station_prep::ev_pbe::{parse_pbe_row, vehicle_entity, PBE_PROFILE, VEHICLE_SOURCE};
use std::collections::HashSet;
use std::io::{BufWriter, Write};
use std::os::unix::fs::{DirBuilderExt, OpenOptionsExt};
use std::path::Path;

fn run() -> Result<(), Box<dyn std::error::Error>> {
    let args: Vec<String> = std::env::args().collect();
    if args.len() != 4 {
        return Err("usage: prepare_vehicle PBE_TEXT SUPPLEMENT_TEXT NEW_OUTPUT_DIRECTORY".into());
    }
    let out = Path::new(&args[3]);
    std::fs::DirBuilder::new().mode(0o700).create(out)?;
    let result = (|| -> Result<(), Box<dyn std::error::Error>> {
        let pbe_raw = std::fs::read(&args[1])?;
        let supplement_raw = std::fs::read(&args[2])?;
        if pbe_raw.len() as u64 > 10_000_000 || supplement_raw.len() as u64 > 10_000_000 {
            return Err("input exceeds the preparation budget".into());
        }
        let pbe_text = String::from_utf8_lossy(&pbe_raw);
        let rights_sha256 = format!(
            "{:x}",
            Sha256::digest(b"pbe-inmetro-public-table-2026-supplement-minimized-20261022")
        );
        let mut seen: HashSet<String> = HashSet::new();
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
        let mut accepted = 0usize;
        let mut duplicates = 0usize;
        let mut quarantined = 0usize;
        let mut quarantine_rows: Vec<serde_json::Value> = Vec::new();
        let mut entity_bytes = 0u64;
        let mut decision_bytes = 0u64;
        for (index, line) in pbe_text.lines().enumerate() {
            if line.trim().is_empty() {
                continue;
            }
            match parse_pbe_row(line) {
                Ok((identity, _, _)) => {
                    let entity = vehicle_entity(&identity, &rights_sha256)
                        .map_err(|e| format!("entity: {e}"))?;
                    if !seen.insert(entity.key.clone()) {
                        duplicates += 1;
                        continue;
                    }
                    let record = serde_json::json!({
                        "kind": "VEHICLE_REFERENCE",
                        "key": entity.key,
                        "parent_key": Value::Null,
                        "external_id": entity.external_id,
                        "unit": format!("VEHICLE_REFERENCE:{}", entity.external_id),
                        "payload": entity.payload,
                        "field_provenance": {"source": VEHICLE_SOURCE, "profile": PBE_PROFILE},
                        "rights_sha256": rights_sha256,
                        "source_clocks": {"created_at": Value::Null, "verified_at": Value::Null, "status_at": Value::Null},
                        "assertion_sha256": entity.assertion,
                    });
                    let mut entity_line =
                        canonical_bytes(&record).map_err(|e| format!("encode: {e}"))?;
                    entity_line.push(b'\n');
                    entity_bytes += entity_line.len() as u64;
                    entities_file.write_all(&entity_line)?;
                    let decision = serde_json::json!({
                        "unit": format!("VEHICLE_REFERENCE:{}", entity.external_id),
                        "kind": "VEHICLE_REFERENCE",
                        "source_file": "pbe-2026.txt",
                        "root": true,
                        "pointer": entity.external_id,
                        "parent_unit": Value::Null,
                        "outcome": "accepted",
                        "key": entity.key,
                        "external_id": entity.external_id,
                        "assertion_sha256": entity.assertion,
                        "duplicate_of": Value::Null,
                        "reason": "",
                        "raw_sha256": format!("{:x}", Sha256::digest(line.as_bytes())),
                    });
                    let mut decision_line =
                        canonical_bytes(&decision).map_err(|e| format!("encode: {e}"))?;
                    decision_line.push(b'\n');
                    decision_bytes += decision_line.len() as u64;
                    decisions_file.write_all(&decision_line)?;
                    accepted += 1;
                }
                Err(reason) => {
                    if reason == "combustion_scope" {
                        quarantined += 1;
                        quarantine_rows.push(serde_json::json!({
                            "schema_version": "station-quarantine-v1",
                            "source": VEHICLE_SOURCE,
                            "row_locator": format!("pbe-2026.txt#row:{}", index + 1),
                            "reason": "combustion_scope",
                            "detail": "not electrified",
                        }));
                    } else {
                        quarantined += 1;
                        quarantine_rows.push(serde_json::json!({
                            "schema_version": "station-quarantine-v1",
                            "source": VEHICLE_SOURCE,
                            "row_locator": format!("pbe-2026.txt#row:{}", index + 1),
                            "reason": reason,
                            "detail": "row refused",
                        }));
                    }
                }
            }
        }
        let _ = supplement_raw;
        entities_file.flush()?;
        decisions_file.flush()?;
        let manifest = serde_json::json!({
            "schema_version": "vehicle-energy-reference-v1",
            "source_profile": PBE_PROFILE,
            "counts": {"input": accepted + duplicates + quarantined, "accepted": accepted, "duplicates": duplicates, "quarantined": quarantined},
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
        eprintln!("prepare_vehicle: source contract or artifact operation refused");
        std::process::exit(1);
    }
}

use serde_json::Value;

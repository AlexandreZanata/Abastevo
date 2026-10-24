//! Bounded native preparation of the PBE table plus supplement.
//! Private artifacts; counts-only stdout.
use sha2::{Digest, Sha256};
use station_prep::ev_contract::canonical_bytes;
use station_prep::ev_output::{
    decision_record, entity_record, hex_sha256, manifest_record, zero_census,
};
use station_prep::ev_pbe::{parse_pbe_row, vehicle_entity, PBE_PROFILE};
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
        let mut entity_lines: Vec<(String, Vec<u8>)> = Vec::new();
        let mut decision_lines: Vec<(String, Vec<u8>)> = Vec::new();
        let mut accepted = 0usize;
        let mut duplicates = 0usize;
        let mut quarantined = 0usize;
        let mut entity_bytes = 0u64;
        let mut decision_bytes = 0u64;
        let mut entity_records = 0u64;
        let mut decision_records = 0u64;
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
                    let record = entity_record(
                        "VEHICLE_REFERENCE",
                        &entity.key,
                        None,
                        &entity.external_id,
                        &format!("VEHICLE_REFERENCE:{}", entity.external_id),
                        entity.payload.clone(),
                        &["model_code"],
                        &rights_sha256,
                        serde_json::json!([
                            {"field": "created_at", "value": Value::Null},
                            {"field": "verified_at", "value": Value::Null},
                            {"field": "status_at", "value": Value::Null},
                        ]),
                        &entity.assertion,
                    );
                    let mut entity_line =
                        canonical_bytes(&record).map_err(|e| format!("encode: {e}"))?;
                    entity_line.push(b'\n');
                    entity_bytes += entity_line.len() as u64;
                    entity_records += 1;
                    entity_lines.push((entity.key.clone(), entity_line));
                    let decision = decision_record(
                        &format!("VEHICLE_REFERENCE:{}", entity.external_id),
                        "VEHICLE_REFERENCE",
                        "pbe-2026.txt",
                        0,
                        &entity.external_id,
                        None,
                        "ACCEPTED",
                        Some(entity.key.clone()),
                        &entity.external_id,
                        Some(entity.assertion.clone()),
                        None,
                        None,
                        &hex_sha256(line.as_bytes()),
                    );
                    let mut decision_line =
                        canonical_bytes(&decision).map_err(|e| format!("encode: {e}"))?;
                    decision_line.push(b'\n');
                    decision_bytes += decision_line.len() as u64;
                    decision_records += 1;
                    decision_lines.push((
                        format!("VEHICLE_REFERENCE:{}", entity.external_id),
                        decision_line,
                    ));
                    accepted += 1;
                }
                Err(reason) => {
                    quarantined += 1;
                    let unit = format!("pbe-2026.txt#row:{}", index + 1);
                    let decision = decision_record(
                        &unit,
                        "VEHICLE_REFERENCE",
                        "pbe-2026.txt",
                        0,
                        &unit,
                        None,
                        "QUARANTINED",
                        None,
                        "",
                        None,
                        None,
                        Some(reason.to_string()),
                        &hex_sha256(line.as_bytes()),
                    );
                    let mut decision_line =
                        canonical_bytes(&decision).map_err(|e| format!("encode: {e}"))?;
                    decision_line.push(b'\n');
                    decision_bytes += decision_line.len() as u64;
                    decision_records += 1;
                    decision_lines.push((unit.clone(), decision_line));
                }
            }
        }
        let _ = supplement_raw;
        entity_lines.sort_by(|a, b| a.0.cmp(&b.0));
        decision_lines.sort_by(|a, b| a.0.cmp(&b.0));
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
        for (_, line) in &entity_lines {
            entities_file.write_all(line)?;
        }
        for (_, line) in &decision_lines {
            decisions_file.write_all(line)?;
        }
        entities_file.flush()?;
        decisions_file.flush()?;
        entities_file.get_ref().sync_all()?;
        decisions_file.get_ref().sync_all()?;
        let mut counts = zero_census();
        counts["VEHICLE_REFERENCE"] = serde_json::json!({"input": accepted + duplicates + quarantined, "accepted": accepted, "duplicates": duplicates, "quarantined": quarantined});
        let manifest = manifest_record("vehicle-energy-reference-v1", PBE_PROFILE,
            &hex_sha256(PBE_PROFILE.as_bytes()), &rights_sha256,
            "pbe-2026-09-25", None,
            vec![serde_json::json!({"file": "pbe-2026.txt", "bytes": pbe_raw.len(), "sha256": hex_sha256(&pbe_raw), "roots": 1})],
            serde_json::json!({
                "entities.ndjson": {"bytes": entity_bytes, "records": entity_records, "sha256": hex_sha256(&std::fs::read(out.join("entities.ndjson"))?)},
                "decisions.ndjson": {"bytes": decision_bytes, "records": decision_records, "sha256": hex_sha256(&std::fs::read(out.join("decisions.ndjson"))?)},
                "aliases.ndjson": {"bytes": 0, "records": 0, "sha256": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"},
            }),
            counts,
        )
        .map_err(|e| format!("manifest: {e}"))?;
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

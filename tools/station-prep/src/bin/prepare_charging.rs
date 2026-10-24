//! Bounded native preparation of the admitted OCM snapshot.
//! Private artifacts; counts-only stdout. Frozen envelope shapes.
use station_prep::ev_contract::canonical_bytes;
use station_prep::ev_ocm::{parse_ocm_poi, OCM_PROFILE};
use station_prep::ev_output::{
    decision_record, entity_record, hex_sha256, manifest_record, zero_census,
};
use std::collections::{BTreeMap, HashSet};
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
        let rights_sha256 = hex_sha256(b"open-licenses-cc-by-40-cc-0-cc0-20261022");
        let profile_sha256 = hex_sha256(OCM_PROFILE.as_bytes());
        let mut seen: HashSet<String> = HashSet::new();
        let mut providers: HashSet<i64> = HashSet::new();
        let mut entity_lines: Vec<(String, Vec<u8>)> = Vec::new();
        let mut decision_lines: Vec<(String, Vec<u8>)> = Vec::new();
        let mut entity_bytes = 0u64;
        let mut decision_bytes = 0u64;
        let mut kind_counts: BTreeMap<&str, [usize; 4]> = BTreeMap::new();
        let mut input_inventory: Vec<serde_json::Value> = Vec::new();
        for (root_index, path) in files.iter().enumerate() {
            let name = path
                .file_name()
                .and_then(|n| n.to_str())
                .ok_or("name refused")?
                .to_string();
            let raw = std::fs::read(path)?;
            if raw.len() as u64 > 1_000_000 {
                return Err("source file exceeds record budget".into());
            }
            let raw_sha256 = hex_sha256(&raw);
            input_inventory.push(serde_json::json!({
                "file": name, "bytes": raw.len(), "sha256": raw_sha256, "roots": 1,
            }));
            let file_run = parse_ocm_poi(
                &name,
                &raw,
                &rights_sha256,
                &mut seen,
                &mut providers,
                &mut |_| Ok(()),
            )?;
            let place_unit = file_run
                .entities
                .first()
                .map(|e| format!("PLACE:{}", e.external_id));
            for entity in &file_run.entities {
                let provenance: &[&str] = if entity.kind == "PLACE" {
                    &[
                        "public_access",
                        "name",
                        "country",
                        "latitude_decimal",
                        "longitude_decimal",
                        "reported_point_count",
                    ]
                } else {
                    &[
                        "connector_code",
                        "raw_connector_code",
                        "current",
                        "power_w",
                        "reported_quantity",
                    ]
                };
                let parent_unit = if entity.kind == "PLACE" {
                    None
                } else {
                    place_unit.clone()
                };
                let record = entity_record(
                    entity.kind,
                    &entity.key,
                    entity.parent_key.as_deref(),
                    &entity.external_id,
                    &format!("{}:{}", entity.kind, entity.external_id),
                    entity.payload.clone(),
                    provenance,
                    &rights_sha256,
                    entity.clocks.clone(),
                    &entity.assertion,
                );
                let mut line = canonical_bytes(&record).map_err(|e| format!("entity: {e}"))?;
                line.push(b'\n');
                entity_bytes += line.len() as u64;
                if entity_bytes > 100_000_000 {
                    return Err("artifact budget exceeded".into());
                }
                entity_lines.push((entity.key.clone(), line));
                let decision = decision_record(
                    &format!("{name}#{}", entity.external_id),
                    entity.kind,
                    &name,
                    root_index,
                    &entity.external_id,
                    parent_unit.clone(),
                    "ACCEPTED",
                    Some(entity.key.clone()),
                    &entity.external_id,
                    Some(entity.assertion.clone()),
                    None,
                    None,
                    &raw_sha256,
                );
                let mut decision_line =
                    canonical_bytes(&decision).map_err(|e| format!("decision: {e}"))?;
                decision_line.push(b'\n');
                decision_bytes += decision_line.len() as u64;
                decision_lines.push((format!("{name}#{}", entity.external_id), decision_line));
                let entry = kind_counts.entry(entity.kind).or_insert([0, 0, 0, 0]);
                entry[0] += 1;
                entry[1] += 1;
            }
            for row in &file_run.quarantine {
                let kind = if row.row_locator.contains("#connection:") {
                    "CONNECTION_GROUP"
                } else {
                    "PLACE"
                };
                let decision = decision_record(
                    &row.row_locator,
                    kind,
                    &name,
                    root_index,
                    &row.row_locator,
                    None,
                    "QUARANTINED",
                    None,
                    "",
                    None,
                    None,
                    Some(row.reason.to_string()),
                    &raw_sha256,
                );
                let mut decision_line =
                    canonical_bytes(&decision).map_err(|e| format!("decision: {e}"))?;
                decision_line.push(b'\n');
                decision_bytes += decision_line.len() as u64;
                decision_lines.push((row.row_locator.clone(), decision_line));
                let entry = kind_counts.entry(kind).or_insert([0, 0, 0, 0]);
                entry[0] += 1;
                entry[3] += 1;
            }
            for (unit, key) in &file_run.duplicate_units {
                let kind = if unit.contains("#connection:") {
                    "CONNECTION_GROUP"
                } else {
                    "PLACE"
                };
                let decision = decision_record(
                    unit,
                    kind,
                    &name,
                    root_index,
                    unit,
                    None,
                    "DUPLICATE",
                    Some(key.clone()),
                    "",
                    None,
                    Some(unit.clone()),
                    None,
                    &raw_sha256,
                );
                let mut decision_line =
                    canonical_bytes(&decision).map_err(|e| format!("decision: {e}"))?;
                decision_line.push(b'\n');
                decision_bytes += decision_line.len() as u64;
                decision_lines.push((unit.clone(), decision_line));
                let entry = kind_counts.entry(kind).or_insert([0, 0, 0, 0]);
                entry[0] += 1;
                entry[2] += 1;
            }
        }
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
        let entity_records = entity_lines.len() as u64;
        let decision_records = decision_lines.len() as u64;
        entities_file.flush()?;
        decisions_file.flush()?;
        entities_file.get_ref().sync_all()?;
        decisions_file.get_ref().sync_all()?;
        let mut counts = zero_census();
        for (kind, tallies) in &kind_counts {
            counts[*kind] = serde_json::json!({"input": tallies[0], "accepted": tallies[1], "duplicates": tallies[2], "quarantined": tallies[3]});
        }
        let entity_hash = hex_sha256(&std::fs::read(out.join("entities.ndjson"))?);
        let decision_hash = hex_sha256(&std::fs::read(out.join("decisions.ndjson"))?);
        let manifest = manifest_record("charging-prepared-v1", OCM_PROFILE, &profile_sha256, &rights_sha256,
            "8e3bedca48ca94807d96b2f5e7ee02cffe63ae54", None, input_inventory,
            serde_json::json!({
                "entities.ndjson": {"bytes": entity_bytes, "records": entity_records, "sha256": entity_hash},
                "decisions.ndjson": {"bytes": decision_bytes, "records": decision_records, "sha256": decision_hash},
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
        std::fs::File::open(out)?.sync_all()?;
        let total: usize = kind_counts.values().map(|c| c[0]).sum();
        let accepted: usize = kind_counts.values().map(|c| c[1]).sum();
        println!("input={total} accepted={accepted} artifacts=file-backed");
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

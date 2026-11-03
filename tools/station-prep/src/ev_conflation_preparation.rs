//! Private file boundary for reviewed normalized-v2 source conflation.
use crate::ev_conflation::{conflate, Conflation, Stats};
use crate::ev_geography::{StateIndex, STATES};
use crate::ev_site_preparation::{checked_raw, write_bytes, write_lines, PrepError, SourcePins};
use crate::ev_sites::{NormalizationDecision, SiteKind, SourceSite};
use crate::types::sha256_hex;
use serde::de::DeserializeOwned;
use serde_json::{json, Value};
use std::collections::{BTreeMap, BTreeSet};
use std::fs;
use std::os::unix::fs::DirBuilderExt;
use std::path::PathBuf;
type Result<T> = std::result::Result<T, PrepError>;
pub const FORMAT: &str = "abastevo.ev-conflated-v1";
pub const PROFILE: &[u8] = include_bytes!("../../../contracts/ev/conflation-profile-v1.json");
#[derive(Debug)]
pub struct Inputs {
    pub normalized: PathBuf,
    pub manifest_sha256: String,
    pub boundaries: PathBuf,
    pub output: PathBuf,
}
fn decode<T: DeserializeOwned>(raw: &[u8], maximum: usize) -> Result<Vec<T>> {
    let mut rows = Vec::new();
    for line in raw.split(|b| *b == b'\n').filter(|l| !l.is_empty()) {
        if line.len() > 1 << 20 || rows.len() >= maximum {
            return Err(PrepError("input record budget exceeded"));
        }
        rows.push(
            serde_json::from_slice(line)
                .map_err(|_| PrepError("normalized record shape refused"))?,
        );
    }
    Ok(rows)
}
fn admitted(
    inputs: &Inputs,
    pins: &SourcePins,
    cancel: &dyn Fn() -> bool,
) -> Result<(Value, Vec<SourceSite>)> {
    let raw = checked_raw(
        &inputs.normalized.join("manifest.json"),
        8 << 20,
        &inputs.manifest_sha256,
    )?;
    let m: Value =
        serde_json::from_slice(&raw).map_err(|_| PrepError("normalized manifest shape refused"))?;
    let expected = json!({"profile_id":pins.profile_id,"sha256":pins.profile_sha256,"exposure":"PRIVATE_PREPARATION"});
    if m["format"] != "abastevo.ev-source-sites-v1"
        || m["parser_version"] != "ev-normalization-v2"
        || m["normalization_profile_sha256"]
            != sha256_hex(include_bytes!(
                "../../../contracts/ev/normalization-profile-v2.json"
            ))
        || m["source_profile"] != expected
        || m["source_pins"]
            != serde_json::to_value(pins).map_err(|_| PrepError("source pins refused"))?
    {
        return Err(PrepError("normalized source/profile admission refused"));
    }
    let files = m["files"]
        .as_object()
        .ok_or(PrepError("input file set missing"))?;
    let required: BTreeSet<_> = [
        "sites.ndjson",
        "decisions.ndjson",
        "issues.ndjson",
        "uf-census.json",
    ]
    .into_iter()
    .collect();
    if files.keys().map(String::as_str).collect::<BTreeSet<_>>() != required {
        return Err(PrepError("incomplete normalized file set"));
    }
    let mut source_raw = None;
    let mut decision_raw = None;
    for (name, descriptor) in files {
        if cancel() {
            return Err(PrepError("cancelled"));
        }
        let hash = descriptor["sha256"]
            .as_str()
            .ok_or(PrepError("input file hash missing"))?;
        let raw = checked_raw(&inputs.normalized.join(name), 512 << 20, hash)?;
        if descriptor["bytes"].as_u64() != Some(raw.len() as u64) {
            return Err(PrepError("input byte count mismatch"));
        }
        if name == "sites.ndjson" {
            source_raw = Some(raw);
        } else if name == "decisions.ndjson" {
            decision_raw = Some(raw);
        }
    }
    let sites: Vec<SourceSite> = decode(&source_raw.unwrap(), 205000)?;
    let decisions: Vec<NormalizationDecision> = decode(&decision_raw.unwrap(), 205000)?;
    if files["sites.ndjson"]["records"].as_u64() != Some(sites.len() as u64)
        || files["decisions.ndjson"]["records"].as_u64() != Some(decisions.len() as u64)
    {
        return Err(PrepError("input record count mismatch"));
    }
    let keys: BTreeSet<_> = sites.iter().map(|s| s.key.clone()).collect();
    if keys.len() != sites.len() {
        return Err(PrepError("duplicate source record"));
    }
    let mut seen = BTreeSet::new();
    let mut accepted = BTreeSet::new();
    let mut quarantined = 0;
    let mut ignored = 0;
    let mut ocm = 0;
    let mut osm = 0;
    for decision in decisions {
        let key = decision.key.ok_or(PrepError("decision identity missing"))?;
        if !seen.insert(key.clone()) {
            return Err(PrepError("duplicate source decision"));
        }
        if key.starts_with("ocm:") {
            ocm += 1;
        } else if key.starts_with("osm:") {
            osm += 1;
        } else {
            return Err(PrepError("unknown decision namespace"));
        }
        match decision.state.as_str() {
            "ACCEPTED" => {
                accepted.insert(key);
            }
            "QUARANTINED" => quarantined += 1,
            "IGNORED" => ignored += 1,
            _ => return Err(PrepError("unknown decision state")),
        }
    }
    let equipment = sites
        .iter()
        .filter(|s| s.kind == SiteKind::ChargePoint)
        .count();
    if keys != accepted
        || ocm != pins.ocm_rows
        || m["summary"]["osm_candidates"].as_u64() != Some(osm)
        || m["summary"]["accepted_source_sites"].as_u64() != Some((sites.len() - equipment) as u64)
        || m["summary"]["source_equipment"].as_u64() != Some(equipment as u64)
        || m["summary"]["quarantined"].as_u64() != Some(quarantined)
        || m["summary"]["ignored"].as_u64() != Some(ignored)
    {
        return Err(PrepError("source decisions/summary do not reconcile"));
    }
    Ok((m, sites))
}
fn state(index: &StateIndex, point: &crate::ev_sites::SitePoint) -> String {
    index
        .classify(
            point.latitude_decimal.parse().unwrap_or(f64::NAN),
            point.longitude_decimal.parse().unwrap_or(f64::NAN),
        )
        .uf
        .unwrap_or_else(|| "UNASSIGNED".into())
}
fn zero() -> BTreeMap<String, u64> {
    [
        "catalog_sites",
        "two_source_sites",
        "osm_only_sites",
        "ocm_only_sites",
        "combined_sites",
        "attached_equipment",
        "probable_pairs",
        "conflict_pairs",
        "consumed_candidates",
        "hierarchy_review_records",
    ]
    .into_iter()
    .map(|s| (s.into(), 0))
    .collect()
}
fn census(result: &Conflation, records: &[SourceSite], index: &StateIndex) -> Value {
    let mut by_uf: BTreeMap<String, BTreeMap<String, u64>> = STATES
        .into_iter()
        .chain(["UNASSIGNED"])
        .map(|s| (s.into(), zero()))
        .collect();
    for site in &result.sites {
        let row = by_uf.get_mut(&state(index, &site.point)).unwrap();
        *row.get_mut("catalog_sites").unwrap() += 1;
        let category = if site.confidence == "two-source" {
            "two_source_sites"
        } else if site.sources[0].source.source == "OSM" {
            "osm_only_sites"
        } else {
            "ocm_only_sites"
        };
        *row.get_mut(category).unwrap() += 1;
        if site.kind == SiteKind::CombinedSite {
            *row.get_mut("combined_sites").unwrap() += 1;
        }
        *row.get_mut("attached_equipment").unwrap() += site.equipment.len() as u64;
    }
    let lookup: BTreeMap<_, _> = records.iter().map(|s| (s.key.as_str(), s)).collect();
    for pair in &result.review {
        let row = by_uf
            .get_mut(&state(index, &lookup[pair.osm_key.as_str()].point))
            .unwrap();
        let category = if pair.reason == "probable_same_label" {
            "probable_pairs"
        } else {
            "consumed_candidates"
        };
        *row.get_mut(category).unwrap() += 1;
    }
    for pair in &result.conflicts {
        *by_uf
            .get_mut(&state(index, &lookup[pair.osm_key.as_str()].point))
            .unwrap()
            .get_mut("conflict_pairs")
            .unwrap() += 1;
    }
    for r in &result.hierarchy_review {
        *by_uf
            .get_mut(&state(index, &lookup[r.key.as_str()].point))
            .unwrap()
            .get_mut("hierarchy_review_records")
            .unwrap() += 1;
    }
    json!({"format":"abastevo.ev-conflation-census-v1","units":"catalog sites after source conflation; pair queues and source equipment are separate units","coverage":"pinned open-source catalog; not complete physical Brazil coverage","boundary_sha256":index.source_sha256,"boundary_attribution":"Fonte: IBGE (CC BY 4.0)","by_uf":by_uf,"stats":result.stats})
}
fn owner_sample(result: &Conflation, index: &StateIndex) -> Value {
    let mut groups: BTreeMap<String, Vec<_>> = BTreeMap::new();
    for site in &result.sites {
        groups
            .entry(state(index, &site.point))
            .or_default()
            .push(site);
    }
    for group in groups.values_mut() {
        group.sort_by(|a, b| {
            b.sources
                .len()
                .cmp(&a.sources.len())
                .then(a.key.cmp(&b.key))
        });
    }
    let mut selected = Vec::new();
    let mut offset = 0;
    while selected.len() < 30 {
        let before = selected.len();
        for sites in groups.values() {
            if let Some(site) = sites.get(offset) {
                selected.push(*site);
                if selected.len() == 30 {
                    break;
                }
            }
        }
        if selected.len() == before {
            break;
        }
        offset += 1;
    }
    let samples:Vec<_>=selected.into_iter().map(|s|{
  let urls:Vec<_>=s.sources.iter().filter_map(|source|{if source.source.source=="OCM"{Some(format!("https://www.openchargemap.org/site/poi/details/{}",source.source.external_id))}else{source.source.external_id.split_once(':').map(|(kind,id)|format!("https://www.openstreetmap.org/{kind}/{id}"))}}).collect();
  json!({"catalog_key":s.key,"uf":state(index,&s.point),"confidence":s.confidence,"source_name":s.name,"source_operator":s.operator,"reported_point":s.point,"source_urls":urls,"source_ids":s.sources.iter().map(|v|v.key.clone()).collect::<Vec<_>>(),"imagery_result":null,"reviewer":null,"reviewed_at":null})
 }).collect();
    json!({"status":"OWNER_IMAGERY_REVIEW_PENDING","method":"round-robin UF; two-source first within each UF; stable catalog key","required_samples":30,"available_samples":samples.len(),"claim":"generated source-linked candidates, not evidence of completed imagery review","samples":samples})
}
pub fn prepare(inputs: &Inputs, pins: &SourcePins, cancel: &dyn Fn() -> bool) -> Result<Stats> {
    if fs::symlink_metadata(&inputs.output).is_ok() {
        return Err(PrepError("output already exists"));
    }
    if cancel() {
        return Err(PrepError("cancelled"));
    }
    let (manifest, records) = admitted(inputs, pins, cancel)?;
    let boundary = checked_raw(&inputs.boundaries, 8 << 20, &pins.boundary_sha256)?;
    let index =
        StateIndex::from_bytes(&boundary).map_err(|_| PrepError("boundary shape refused"))?;
    let result = conflate(&records).map_err(PrepError)?;
    if result.sites.len()
        != result.stats.confirmed_pairs + result.stats.osm_only + result.stats.ocm_only
        || result.stats.input_records
            != result.stats.source_roots
                + result.stats.grouped_equipment
                + result.stats.unresolved_records
    {
        return Err(PrepError("conflation counters do not reconcile"));
    }
    if cancel() {
        return Err(PrepError("cancelled"));
    }
    fs::DirBuilder::new()
        .mode(0o700)
        .create(&inputs.output)
        .map_err(|_| PrepError("private output creation refused"))?;
    let write = (|| {
        let mut files = BTreeMap::new();
        files.insert(
            "catalog.ndjson",
            write_lines(&inputs.output.join("catalog.ndjson"), &result.sites, cancel)?,
        );
        files.insert(
            "pair-review.ndjson",
            write_lines(
                &inputs.output.join("pair-review.ndjson"),
                &result.review,
                cancel,
            )?,
        );
        files.insert(
            "conflicts.ndjson",
            write_lines(
                &inputs.output.join("conflicts.ndjson"),
                &result.conflicts,
                cancel,
            )?,
        );
        files.insert(
            "hierarchy-review.ndjson",
            write_lines(
                &inputs.output.join("hierarchy-review.ndjson"),
                &result.hierarchy_review,
                cancel,
            )?,
        );
        files.insert(
            "equipment-assignments.ndjson",
            write_lines(
                &inputs.output.join("equipment-assignments.ndjson"),
                &result.assignments,
                cancel,
            )?,
        );
        for (name, value) in [
            ("uf-census.json", census(&result, &records, &index)),
            ("owner-review-sample.json", owner_sample(&result, &index)),
        ] {
            let bytes = serde_json::to_vec_pretty(&value)
                .map_err(|_| PrepError("private JSON output refused"))?;
            files.insert(name, write_bytes(&inputs.output.join(name), &bytes)?);
        }
        let out = json!({"format":FORMAT,"conflation_profile":{"id":"ev-conflation-v1","sha256":sha256_hex(PROFILE),"exposure":"PRIVATE_PREPARATION"},"input_manifest_sha256":inputs.manifest_sha256,"normalization_profile_sha256":manifest["normalization_profile_sha256"],"source_profile":manifest["source_profile"],"source_pins":pins,"source_input_summary":manifest["summary"],"stats":result.stats,"owner_imagery_acceptance":"PENDING","files":files});
        let bytes = serde_json::to_vec_pretty(&out)
            .map_err(|_| PrepError("conflation manifest refused"))?;
        if cancel() {
            return Err(PrepError("cancelled"));
        }
        write_bytes(&inputs.output.join("manifest.json"), &bytes)?;
        fs::File::open(&inputs.output)
            .and_then(|f| f.sync_all())
            .map_err(|_| PrepError("output sync refused"))?;
        Ok(())
    })();
    if write.is_err() {
        let _ = fs::remove_dir_all(&inputs.output);
    }
    write?;
    Ok(result.stats)
}

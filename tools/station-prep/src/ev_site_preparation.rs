//! Local file adapter for hash-bound private source-site normalization.
use crate::ev_geography::{StateIndex, STATES};
use crate::ev_osm_sites::{is_osm_ev_candidate, normalize_osm, OsmContext, OsmEvidence};
use crate::ev_sites::{
    normalize_ocm, FootprintPolygon, Normalization, OcmContext, SiteKind, SitePoint,
    SourceFootprint, SourceSite,
};
use crate::osm_national::NationalProfile;
use crate::osm_reference::{coordinates, Limits};
use crate::types::sha256_hex;
use pbf_craft::models::{Element, ElementType, Tag};
use pbf_craft::readers::{IndexedReader, PbfReader};
use serde::{Deserialize, Serialize};
use serde_json::{json, Value};
use sha2::{Digest, Sha256};
use std::collections::{BTreeMap, BTreeSet};
use std::fs::{self, File, OpenOptions};
use std::io::{BufWriter, Read, Write};
use std::os::unix::fs::{DirBuilderExt, OpenOptionsExt};
use std::path::{Path, PathBuf};

type Result<T> = std::result::Result<T, PrepError>;
#[derive(Debug)]
pub struct PrepError(pub &'static str);
impl std::fmt::Display for PrepError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.write_str(self.0)
    }
}
impl std::error::Error for PrepError {}
fn io<T>(r: std::io::Result<T>) -> Result<T> {
    r.map_err(|_| PrepError("private source/artifact operation refused"))
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct SourcePins {
    pub profile_id: String,
    pub profile_sha256: String,
    pub osm_revision: String,
    pub osm_sha256: String,
    pub osm_bytes: u64,
    pub ocm_revision: String,
    pub ocm_reference_sha256: String,
    pub ocm_inventory_sha256: String,
    pub ocm_rows: usize,
    pub boundary_sha256: String,
}
impl SourcePins {
    pub fn frozen() -> Self {
        let raw = include_bytes!("../../../contracts/ev/national-source-pins-20261103-v1.json");
        let v: Value = serde_json::from_slice(raw).expect("compiled national source pins");
        let s = &v["sources"];
        Self {
            profile_id: v["profile_id"].as_str().unwrap().into(),
            profile_sha256: sha256_hex(raw),
            osm_revision: s["OSM"]["source_revision"].as_str().unwrap().into(),
            osm_sha256: s["OSM"]["sha256"].as_str().unwrap().into(),
            osm_bytes: s["OSM"]["bytes"].as_u64().unwrap(),
            ocm_revision: s["OCM"]["source_revision"].as_str().unwrap().into(),
            ocm_reference_sha256: s["OCM"]["reference_sha256"].as_str().unwrap().into(),
            ocm_inventory_sha256: s["OCM"]["inventory_sha256"].as_str().unwrap().into(),
            ocm_rows: s["OCM"]["rows"].as_u64().unwrap() as usize,
            boundary_sha256: s["IBGE"]["sha256"].as_str().unwrap().into(),
        }
    }
}
#[derive(Debug)]
pub struct Inputs {
    pub osm: PathBuf,
    pub ocm: PathBuf,
    pub boundaries: PathBuf,
    pub output: PathBuf,
}
#[derive(Debug, Default, Serialize)]
pub struct Summary {
    pub osm_input_elements: u64,
    pub osm_candidates: u64,
    pub accepted_source_sites: u64,
    pub source_equipment: u64,
    pub quarantined: u64,
    pub ignored: u64,
}
#[derive(Deserialize)]
struct Inventory {
    file: String,
    bytes: u64,
    sha256: String,
}

pub(crate) fn read_bounded(path: &Path, maximum: u64) -> Result<Vec<u8>> {
    let metadata = io(fs::symlink_metadata(path))?;
    if !metadata.is_file() || metadata.file_type().is_symlink() || metadata.len() > maximum {
        return Err(PrepError("regular bounded source file required"));
    }
    let mut raw = Vec::new();
    io(io(File::open(path))?
        .take(maximum + 1)
        .read_to_end(&mut raw))?;
    if raw.len() as u64 > maximum {
        return Err(PrepError("source byte budget exceeded"));
    }
    Ok(raw)
}
fn hash_file(path: &Path, maximum: u64, cancel: &dyn Fn() -> bool) -> Result<(String, u64)> {
    let meta = io(fs::symlink_metadata(path))?;
    if !meta.is_file() || meta.file_type().is_symlink() || meta.len() > maximum {
        return Err(PrepError("regular bounded PBF required"));
    }
    let mut file = io(File::open(path))?;
    let mut digest = Sha256::new();
    let mut bytes = 0;
    let mut buffer = vec![0; 1 << 20];
    loop {
        if cancel() {
            return Err(PrepError("cancelled"));
        }
        let n = io(file.read(&mut buffer))?;
        if n == 0 {
            break;
        }
        bytes += n as u64;
        if bytes > maximum {
            return Err(PrepError("PBF byte budget exceeded"));
        }
        digest.update(&buffer[..n]);
    }
    Ok((format!("{:x}", digest.finalize()), bytes))
}
pub(crate) fn checked_raw(path: &Path, maximum: u64, expected: &str) -> Result<Vec<u8>> {
    let raw = read_bounded(path, maximum)?;
    if sha256_hex(&raw) != expected {
        return Err(PrepError("source hash mismatch"));
    }
    Ok(raw)
}
fn parts(element: &Element) -> (&'static str, i64, i32, bool, &Vec<Tag>, Option<String>) {
    match element {
        Element::Node(n) => (
            "node",
            n.id,
            n.version,
            n.visible,
            &n.tags,
            n.timestamp.as_ref().map(|v| v.to_rfc3339()),
        ),
        Element::Way(w) => (
            "way",
            w.id,
            w.version,
            w.visible,
            &w.tags,
            w.timestamp.as_ref().map(|v| v.to_rfc3339()),
        ),
        Element::Relation(r) => (
            "relation",
            r.id,
            r.version,
            r.visible,
            &r.tags,
            r.timestamp.as_ref().map(|v| v.to_rfc3339()),
        ),
    }
}
fn broad_candidate(tags: &[Tag]) -> bool {
    tags.iter().any(|t| {
        (t.key == "amenity" && ["fuel", "charging_station"].contains(&t.value.as_str()))
            || (t.key == "man_made" && t.value == "charge_point")
            || (["charging_station", "charge_point"].contains(&t.value.as_str())
                && (t.key == "construction"
                    || ["disused:", "abandoned:", "construction:"]
                        .iter()
                        .any(|p| t.key.starts_with(p))))
    })
}
fn type_name(t: &ElementType) -> &'static str {
    match t {
        ElementType::Node => "node",
        ElementType::Way => "way",
        ElementType::Relation => "relation",
    }
}

fn osm(
    inputs: &Inputs,
    pins: &SourcePins,
    limits: &Limits,
    cancel: &dyn Fn() -> bool,
    summary: &mut Summary,
) -> Result<Vec<Normalization>> {
    let (digest, bytes) = hash_file(&inputs.osm, limits.max_input_bytes, cancel)?;
    if digest != pins.osm_sha256 || bytes != pins.osm_bytes {
        return Err(PrepError("PBF source pin mismatch"));
    }
    let context = OsmContext::new(&pins.osm_revision, &pins.osm_sha256).map_err(PrepError)?;
    let mut reader =
        PbfReader::from_path(&inputs.osm).map_err(|_| PrepError("PBF reader refused"))?;
    let mut selected = Vec::new();
    let mut outcomes = Vec::new();
    let mut identities = BTreeSet::new();
    loop {
        if cancel() {
            return Err(PrepError("cancelled"));
        }
        let Some(blob) = reader
            .read_next_blob()
            .map_err(|_| PrepError("PBF decode refused"))?
        else {
            break;
        };
        summary.osm_input_elements +=
            (blob.nodes.len() + blob.ways.len() + blob.relations.len()) as u64;
        if summary.osm_input_elements > limits.max_input_elements {
            return Err(PrepError("PBF element budget exceeded"));
        }
        for element in blob
            .nodes
            .into_iter()
            .map(Element::Node)
            .chain(blob.ways.into_iter().map(Element::Way))
            .chain(blob.relations.into_iter().map(Element::Relation))
        {
            let (kind, id, _, _, tags, _) = parts(&element);
            if !broad_candidate(tags) {
                continue;
            }
            let mut mapped = BTreeMap::new();
            let mut duplicate = false;
            for t in tags {
                if mapped.insert(t.key.clone(), t.value.clone()).is_some() {
                    duplicate = true;
                }
            }
            if !is_osm_ev_candidate(&mapped) {
                continue;
            }
            summary.osm_candidates += 1;
            if summary.osm_candidates > limits.max_matched as u64 {
                return Err(PrepError("EV candidate budget exceeded"));
            }
            let key = format!("osm:{kind}:{id}");
            if !identities.insert(key.clone()) {
                return Err(PrepError("duplicate OSM source identity"));
            }
            if duplicate {
                outcomes.push(Normalization::refused(Some(key), "duplicate_source_tag"));
                continue;
            }
            selected.push((element, mapped));
        }
    }
    drop(reader);
    let mut parents: BTreeMap<String, BTreeSet<String>> = BTreeMap::new();
    for (element, tags) in &selected {
        if tags.get("man_made").is_some_and(|v| v == "charge_point") {
            continue;
        }
        let (kind, id, _, _, _, _) = parts(element);
        let key = format!("osm:{kind}:{id}");
        match element {
            Element::Way(w) => {
                for n in &w.way_nodes {
                    parents
                        .entry(format!("osm:node:{}", n.id))
                        .or_default()
                        .insert(key.clone());
                }
            }
            Element::Relation(r) => {
                for m in &r.members {
                    parents
                        .entry(format!("osm:{}:{}", type_name(&m.member_type), m.member_id))
                        .or_default()
                        .insert(key.clone());
                }
            }
            Element::Node(_) => {}
        }
    }
    let mut index = IndexedReader::from_path_with_cache(&inputs.osm.to_string_lossy(), 64)
        .map_err(|_| PrepError("PBF index refused"))?;
    for (element, tags) in selected {
        if cancel() {
            return Err(PrepError("cancelled"));
        }
        let (kind, id, version, visible, _, timestamp) = parts(&element);
        let key = format!("osm:{kind}:{id}");
        let mut evidence = OsmEvidence {
            osm_type: kind.into(),
            id,
            version: Some(version),
            timestamp,
            visible,
            tags,
            point: None,
            footprint: None,
            parent_keys: parents
                .get(&key)
                .map(|v| v.iter().cloned().collect())
                .unwrap_or_default(),
        };
        // Excluded records must never need geometry resolution or leak their point.
        let preliminary = normalize_osm(&evidence, &context);
        if preliminary.decision.reason.as_deref() != Some("missing_geometry") {
            outcomes.push(preliminary);
            continue;
        }
        let geometry = coordinates(&mut index, &element, cancel)
            .map_err(|_| PrepError("PBF geometry/cancellation refused"))?;
        if let Some((lat, lon, provenance)) = geometry {
            evidence.point = Some(SitePoint {
                latitude_decimal: format!("{lat:.9}"),
                longitude_decimal: format!("{lon:.9}"),
                provenance,
            });
        }
        if let Element::Way(way) = &element {
            if way.way_nodes.len() >= 4
                && way.way_nodes.len() <= 4096
                && way.way_nodes.first().map(|n| n.id) == way.way_nodes.last().map(|n| n.id)
            {
                let deps = index
                    .get_with_deps(&ElementType::Way, way.id)
                    .map_err(|_| PrepError("source footprint dependencies refused"))?;
                let nodes: BTreeMap<_, _> = deps
                    .into_iter()
                    .filter_map(|e| {
                        if let Element::Node(n) = e {
                            Some((
                                n.id,
                                [
                                    format!("{:.9}", n.longitude as f64 / 1e9),
                                    format!("{:.9}", n.latitude as f64 / 1e9),
                                ],
                            ))
                        } else {
                            None
                        }
                    })
                    .collect();
                let ring: Option<Vec<_>> = way
                    .way_nodes
                    .iter()
                    .map(|n| nodes.get(&n.id).cloned())
                    .collect();
                if let Some(outer) = ring {
                    let footprint = SourceFootprint {
                        polygons: vec![FootprintPolygon {
                            outer,
                            holes: vec![],
                        }],
                    };
                    if footprint.valid() {
                        evidence.footprint = Some(footprint);
                    }
                }
            }
        }
        outcomes.push(normalize_osm(&evidence, &context));
    }
    let after = hash_file(&inputs.osm, limits.max_input_bytes, cancel)?;
    if after != (digest, bytes) {
        return Err(PrepError("PBF changed during preparation"));
    }
    Ok(outcomes)
}

fn ocm(
    inputs: &Inputs,
    pins: &SourcePins,
    cancel: &dyn Fn() -> bool,
) -> Result<Vec<Normalization>> {
    let inventory_raw = checked_raw(
        &inputs.ocm.join("source-inventory.json"),
        8 << 20,
        &pins.ocm_inventory_sha256,
    )?;
    let inventory: Vec<Inventory> = serde_json::from_slice(&inventory_raw)
        .map_err(|_| PrepError("OCM inventory shape refused"))?;
    let total_bytes = inventory
        .iter()
        .try_fold(0u64, |total, v| total.checked_add(v.bytes))
        .ok_or(PrepError("OCM inventory byte total overflow"))?;
    if inventory.iter().any(|v| v.bytes > 1 << 20) {
        return Err(PrepError("OCM source record byte budget exceeded"));
    }
    if inventory.is_empty()
        || inventory.len() != pins.ocm_rows
        || inventory.len() > 5000
        || total_bytes > 128 << 20
    {
        return Err(PrepError("OCM inventory census refused"));
    }
    let expected: BTreeSet<_> = inventory.iter().map(|v| v.file.clone()).collect();
    if expected.len() != inventory.len() {
        return Err(PrepError("duplicate OCM inventory path"));
    }
    let mut actual = BTreeSet::new();
    for entry in io(fs::read_dir(inputs.ocm.join("pois")))? {
        let entry = io(entry)?;
        let name = entry
            .file_name()
            .into_string()
            .map_err(|_| PrepError("OCM filename refused"))?;
        if !io(entry.file_type())?.is_file() || !actual.insert(name) {
            return Err(PrepError("OCM directory entry refused"));
        }
    }
    if actual != expected {
        return Err(PrepError("incomplete or unexpected OCM source file"));
    }
    let reference = checked_raw(
        &inputs.ocm.join("referencedata.json"),
        8 << 20,
        &pins.ocm_reference_sha256,
    )?;
    let context =
        OcmContext::from_reference(&reference, &pins.ocm_reference_sha256, &pins.ocm_revision)
            .map_err(PrepError)?;
    let mut outcomes = Vec::new();
    for entry in inventory {
        if cancel() {
            return Err(PrepError("cancelled"));
        }
        let id = entry
            .file
            .strip_prefix("OCM-")
            .and_then(|s| s.strip_suffix(".json"))
            .filter(|v| !v.is_empty() && v.bytes().all(|b| b.is_ascii_digit()))
            .and_then(|v| v.parse::<i64>().ok())
            .filter(|v| *v > 0)
            .ok_or(PrepError("OCM filename identity refused"))?;
        let raw = checked_raw(
            &inputs.ocm.join("pois").join(&entry.file),
            1 << 20,
            &entry.sha256,
        )?;
        if raw.len() as u64 != entry.bytes {
            return Err(PrepError("OCM file size mismatch"));
        }
        let mut outcome = normalize_ocm(&raw, &context);
        let key = format!("ocm:poi:{id}");
        if outcome.decision.key.as_ref().is_some_and(|v| v != &key) {
            return Err(PrepError("OCM file/source identity mismatch"));
        }
        outcome.decision.key = Some(key);
        outcomes.push(outcome);
    }
    Ok(outcomes)
}

fn units() -> BTreeMap<String, u64> {
    ["CHARGING_SITE", "COMBINED_SITE", "CHARGE_POINT"]
        .into_iter()
        .map(|v| (v.into(), 0))
        .collect()
}
fn unit_name(kind: SiteKind) -> &'static str {
    match kind {
        SiteKind::ChargingSite => "CHARGING_SITE",
        SiteKind::CombinedSite => "COMBINED_SITE",
        SiteKind::ChargePoint => "CHARGE_POINT",
    }
}
fn census(sites: &[SourceSite], index: &StateIndex) -> Value {
    let mut counts: BTreeMap<String, BTreeMap<String, BTreeMap<String, u64>>> = ["OSM", "OCM"]
        .into_iter()
        .map(|s| {
            (
                s.into(),
                STATES
                    .into_iter()
                    .chain(["UNASSIGNED"])
                    .map(|uf| (uf.into(), units()))
                    .collect(),
            )
        })
        .collect();
    let mut reasons: BTreeMap<String, u64> = BTreeMap::new();
    for site in sites {
        let decision = index.classify(
            site.point.latitude_decimal.parse().unwrap_or(f64::NAN),
            site.point.longitude_decimal.parse().unwrap_or(f64::NAN),
        );
        let uf = decision.uf.as_deref().unwrap_or("UNASSIGNED");
        let kind = unit_name(site.kind);
        *counts
            .get_mut(&site.source.source)
            .expect("internal source")
            .get_mut(uf)
            .expect("internal UF")
            .get_mut(kind)
            .unwrap() += 1;
        *reasons
            .entry(format!("{}:{}", site.source.source, decision.reason))
            .or_default() += 1;
    }
    json!({"format":"abastevo.ev-source-census-v1","units":"source objects; CHARGE_POINT equipment is separate, not sites or summed connectors","eligibility":"normalized source assertions before cross-source conflation; not complete Brazil coverage","boundary_sha256":index.source_sha256,"boundary_license":"CC-BY-4.0","boundary_attribution":"Fonte: IBGE","by_source":counts,"classification_reasons":reasons})
}
pub(crate) fn write_bytes(path: &Path, raw: &[u8]) -> Result<Value> {
    let mut f = io(OpenOptions::new()
        .write(true)
        .create_new(true)
        .mode(0o600)
        .open(path))?;
    io(f.write_all(raw))?;
    io(f.sync_all())?;
    Ok(json!({"bytes":raw.len(),"sha256":sha256_hex(raw)}))
}
pub(crate) fn write_lines<T: Serialize>(
    path: &Path,
    rows: &[T],
    cancel: &dyn Fn() -> bool,
) -> Result<Value> {
    let mut out = BufWriter::new(io(OpenOptions::new()
        .write(true)
        .create_new(true)
        .mode(0o600)
        .open(path))?);
    let mut hash = Sha256::new();
    let mut bytes = 0u64;
    for row in rows {
        if cancel() {
            return Err(PrepError("cancelled"));
        }
        let mut line = serde_json::to_vec(row).map_err(|_| PrepError("artifact JSON refused"))?;
        line.push(b'\n');
        if line.len() > 1 << 20 {
            return Err(PrepError("artifact record budget exceeded"));
        }
        bytes += line.len() as u64;
        if bytes > 512 << 20 {
            return Err(PrepError("artifact file budget exceeded"));
        }
        hash.update(&line);
        io(out.write_all(&line))?;
    }
    io(out.flush())?;
    io(out.get_ref().sync_all())?;
    Ok(json!({"bytes":bytes,"records":rows.len(),"sha256":format!("{:x}",hash.finalize())}))
}

pub fn prepare(
    inputs: &Inputs,
    pins: &SourcePins,
    limits: &Limits,
    cancel: &dyn Fn() -> bool,
) -> Result<Summary> {
    if fs::symlink_metadata(&inputs.output).is_ok() {
        return Err(PrepError("output already exists"));
    }
    if cancel() {
        return Err(PrepError("cancelled"));
    }
    let boundary = checked_raw(&inputs.boundaries, 8 << 20, &pins.boundary_sha256)?;
    let index =
        StateIndex::from_bytes(&boundary).map_err(|_| PrepError("boundary model refused"))?;
    let mut summary = Summary::default();
    let mut outcomes = ocm(inputs, pins, cancel)?;
    outcomes.extend(osm(inputs, pins, limits, cancel, &mut summary)?);
    outcomes.sort_by(|a, b| a.decision.key.cmp(&b.decision.key));
    let mut seen = BTreeSet::new();
    let mut sites = Vec::new();
    let mut decisions = Vec::new();
    let mut issues = Vec::new();
    let mut reasons: BTreeMap<String, u64> = BTreeMap::new();
    for mut outcome in outcomes {
        if let Some(site) = &outcome.site {
            let lat = site.point.latitude_decimal.parse().unwrap_or(f64::NAN);
            let lon = site.point.longitude_decimal.parse().unwrap_or(f64::NAN);
            if !index.within_country_extent(lat, lon) {
                outcome = Normalization::refused(Some(site.key.clone()), "outside_country_extent");
            }
        }
        if let Some(key) = &outcome.decision.key {
            if !seen.insert(key.clone()) {
                return Err(PrepError("duplicate source decision identity"));
            }
        }
        if let Some(site) = outcome.site {
            match site.kind {
                SiteKind::ChargePoint => summary.source_equipment += 1,
                SiteKind::ChargingSite | SiteKind::CombinedSite => {
                    summary.accepted_source_sites += 1
                }
            }
            sites.push(site);
        }
        if outcome.decision.state == "QUARANTINED" {
            summary.quarantined += 1;
        }
        if outcome.decision.state == "IGNORED" {
            summary.ignored += 1;
        }
        if let Some(reason) = &outcome.decision.reason {
            *reasons.entry(reason.clone()).or_default() += 1;
        }
        issues.extend(outcome.issues);
        decisions.push(outcome.decision);
    }
    sites.sort_by(|a, b| a.key.cmp(&b.key));
    issues.sort_by(|a, b| a.key.cmp(&b.key).then(a.reason.cmp(&b.reason)));
    if cancel() {
        return Err(PrepError("cancelled"));
    }
    io(fs::DirBuilder::new().mode(0o700).create(&inputs.output))?;
    let result = (|| {
        let mut files = BTreeMap::new();
        files.insert(
            "sites.ndjson",
            write_lines(&inputs.output.join("sites.ndjson"), &sites, cancel)?,
        );
        files.insert(
            "decisions.ndjson",
            write_lines(&inputs.output.join("decisions.ndjson"), &decisions, cancel)?,
        );
        files.insert(
            "issues.ndjson",
            write_lines(&inputs.output.join("issues.ndjson"), &issues, cancel)?,
        );
        let census = serde_json::to_vec_pretty(&census(&sites, &index))
            .map_err(|_| PrepError("census JSON refused"))?;
        files.insert(
            "uf-census.json",
            write_bytes(&inputs.output.join("uf-census.json"), &census)?,
        );
        let manifest = json!({"format":"abastevo.ev-source-sites-v1","parser_version":"ev-normalization-v2","normalization_profile_sha256":sha256_hex(include_bytes!("../../../contracts/ev/normalization-profile-v2.json")),"source_profile":{"profile_id":pins.profile_id,"sha256":pins.profile_sha256,"exposure":"PRIVATE_PREPARATION"},"source_pins":pins,"osm_limits":limits,"national_profile_sha256":NationalProfile::sha256(),"summary":summary,"quarantine_reasons":reasons,"files":files});
        let raw =
            serde_json::to_vec_pretty(&manifest).map_err(|_| PrepError("manifest JSON refused"))?;
        if cancel() {
            return Err(PrepError("cancelled"));
        }
        write_bytes(&inputs.output.join("manifest.json"), &raw)?;
        io(File::open(&inputs.output).and_then(|f| f.sync_all()))?;
        Ok(())
    })();
    if result.is_err() {
        let _ = fs::remove_dir_all(&inputs.output);
    }
    result?;
    Ok(summary)
}

//! Bounded OSM reference preparation.
//!
//! Local PBF files only: no network, database, native engine or original
//! source scraping. Fuel/charging candidates keep their OSM identity, geometry
//! provenance and unknown tags. A city centroid is never fabricated from a
//! missing station geometry. Output is deterministic and replaced atomically.

use pbf_craft::models::{Element, ElementType, Tag};
#[cfg(test)]
use pbf_craft::models::{Node, Relation, RelationMember, Way, WayNode};
use pbf_craft::readers::{IndexedReader, PbfReader};
use serde::Serialize;
use sha2::{Digest, Sha256};
use std::collections::{BTreeMap, HashSet};
use std::fs::{self, File};
use std::io::{BufWriter, Write};
use std::path::Path;

pub const REFERENCE_FORMAT: &str = "abastevo.osm-reference-v1";
pub const PARSER_VERSION: &str = "osm-reference-v1";
pub const SOURCE_LICENSE: &str = "ODbL-1.0";
pub const SOURCE_ATTRIBUTION: &str = "© OpenStreetMap contributors — ODbL 1.0";

#[derive(Debug, Clone)]
pub struct Limits {
    pub max_input_bytes: u64,
    pub max_input_elements: u64,
    pub max_matched: usize,
    pub max_tags: usize,
    pub max_tag_bytes: usize,
}

impl Default for Limits {
    fn default() -> Self {
        Self {
            max_input_bytes: 8 << 30,
            max_input_elements: 400_000_000,
            max_matched: 200_000,
            max_tags: 64,
            max_tag_bytes: 256,
        }
    }
}

#[derive(Debug, Clone, Copy)]
pub struct BBox {
    pub min_lon_deg: f64,
    pub min_lat_deg: f64,
    pub max_lon_deg: f64,
    pub max_lat_deg: f64,
}

impl BBox {
    fn contains(self, lon: f64, lat: f64) -> bool {
        lon >= self.min_lon_deg
            && lon <= self.max_lon_deg
            && lat >= self.min_lat_deg
            && lat <= self.max_lat_deg
    }
}

#[derive(Debug)]
pub enum OsmReferenceError {
    Invalid(String),
    Cancelled,
}

impl std::fmt::Display for OsmReferenceError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            OsmReferenceError::Invalid(message) => f.write_str(message),
            OsmReferenceError::Cancelled => f.write_str("cancelled"),
        }
    }
}

impl std::error::Error for OsmReferenceError {}

type Result<T> = std::result::Result<T, OsmReferenceError>;

fn invalid<T>(message: impl Into<String>) -> Result<T> {
    Err(OsmReferenceError::Invalid(message.into()))
}

#[derive(Debug, Clone, Serialize)]
pub struct ReferencePlace {
    pub key: String,
    pub osm_type: String,
    pub osm_id: i64,
    pub kind: String,
    pub name: Option<String>,
    pub latitude_deg: Option<String>,
    pub longitude_deg: Option<String>,
    pub geometry: String,
    pub quality: String,
    pub display_point: Option<String>,
    pub cnpj: Option<String>,
    pub access: Option<String>,
    pub opening_hours: Option<String>,
    pub operator: Option<String>,
    pub brand: Option<String>,
    pub tags: BTreeMap<String, String>,
    pub source: String,
    pub license: String,
    pub attribution: String,
}

#[derive(Debug, Clone, Serialize)]
pub struct Decision {
    pub key: String,
    pub osm_type: String,
    pub osm_id: i64,
    pub state: String,
    pub reason: Option<String>,
}

#[derive(Debug, Clone, Default, Serialize)]
pub struct Census {
    pub input_elements: u64,
    pub matched: u64,
    pub accepted: u64,
    pub duplicates: u64,
    pub quarantined: u64,
    pub missing_geometry: u64,
    pub fuel: u64,
    pub charging: u64,
}

#[derive(Debug, Serialize)]
pub struct ReferenceRun {
    pub places: Vec<ReferencePlace>,
    pub decisions: Vec<Decision>,
    pub census: Census,
    pub source_sha256: String,
    pub input_bytes: u64,
}

fn is_candidate(element: &Element) -> bool {
    let tags = match element {
        Element::Node(node) => &node.tags,
        Element::Way(way) => &way.tags,
        Element::Relation(relation) => &relation.tags,
    };
    tags.iter().any(|tag| {
        (tag.key == "amenity" && (tag.value == "fuel" || tag.value == "charging_station"))
            || (tag.key == "fuel" && tag.value == "yes")
    })
}

fn visible(element: &Element) -> bool {
    match element {
        Element::Node(node) => node.visible,
        Element::Way(way) => way.visible,
        Element::Relation(relation) => relation.visible,
    }
}

fn element_type(element: &Element) -> (&'static str, i64, &Vec<Tag>) {
    match element {
        Element::Node(node) => ("node", node.id, &node.tags),
        Element::Way(way) => ("way", way.id, &way.tags),
        Element::Relation(relation) => ("relation", relation.id, &relation.tags),
    }
}

fn kind(tags: &BTreeMap<String, String>) -> &'static str {
    let fuel = tags.get("amenity").is_some_and(|v| v == "fuel")
        || tags.get("fuel").is_some_and(|v| v == "yes");
    let charging = tags.get("amenity").is_some_and(|v| v == "charging_station");
    match (fuel, charging) {
        (true, true) => "COMBINED_SITE",
        (true, false) => "FUEL_STATION",
        (false, true) => "CHARGING_SITE",
        (false, false) => "REFERENCE_POI",
    }
}

fn tag_map(tags: &[Tag], limits: &Limits) -> Result<BTreeMap<String, String>> {
    if tags.len() > limits.max_tags {
        return invalid("too many tags");
    }
    let mut map = BTreeMap::new();
    for tag in tags {
        if tag.key.is_empty()
            || tag.key.len() > limits.max_tag_bytes
            || tag.value.len() > limits.max_tag_bytes
        {
            return invalid("tag out of bounds");
        }
        if tag.key.contains('\0') || tag.value.contains('\0') {
            return invalid("NUL in tag");
        }
        if map.insert(tag.key.clone(), tag.value.clone()).is_some() {
            return invalid("duplicate tag key");
        }
    }
    Ok(map)
}

fn input_digest(path: &Path, max_bytes: u64) -> Result<(String, u64)> {
    let metadata =
        fs::metadata(path).map_err(|error| OsmReferenceError::Invalid(error.to_string()))?;
    if metadata.len() > max_bytes {
        return invalid("input exceeds byte budget");
    }
    let mut file =
        File::open(path).map_err(|error| OsmReferenceError::Invalid(error.to_string()))?;
    let mut hasher = Sha256::new();
    let mut bytes = 0u64;
    let mut buffer = [0u8; 64 * 1024];
    loop {
        let read = std::io::Read::read(&mut file, &mut buffer)
            .map_err(|error| OsmReferenceError::Invalid(error.to_string()))?;
        if read == 0 {
            break;
        }
        bytes += read as u64;
        if bytes > max_bytes {
            return invalid("input exceeds byte budget");
        }
        hasher.update(&buffer[..read]);
    }
    Ok((format!("{:x}", hasher.finalize()), bytes))
}

/// Parses one admitted local PBF. `cancel` is checked per element; a cancelled
/// call returns no partial run.
pub fn parse_file(
    path: &Path,
    limits: &Limits,
    bbox: Option<BBox>,
    cancel: &dyn Fn() -> bool,
) -> Result<ReferenceRun> {
    let (source_sha256, input_bytes) = input_digest(path, limits.max_input_bytes)?;
    let mut reader = PbfReader::from_path(path)
        .map_err(|error| OsmReferenceError::Invalid(error.to_string()))?;
    let mut matched: Vec<Element> = Vec::new();
    let mut input_elements = 0u64;
    loop {
        if cancel() {
            return Err(OsmReferenceError::Cancelled);
        }
        let Some(blob) = reader
            .read_next_blob()
            .map_err(|error| OsmReferenceError::Invalid(error.to_string()))?
        else {
            break;
        };
        for node in blob.nodes {
            input_elements += 1;
            if input_elements > limits.max_input_elements {
                return invalid("input exceeds element budget");
            }
            let element = Element::Node(node);
            if visible(&element) && is_candidate(&element) {
                matched.push(element);
            }
        }
        for way in blob.ways {
            input_elements += 1;
            if input_elements > limits.max_input_elements {
                return invalid("input exceeds element budget");
            }
            let element = Element::Way(way);
            if visible(&element) && is_candidate(&element) {
                matched.push(element);
            }
        }
        for relation in blob.relations {
            input_elements += 1;
            if input_elements > limits.max_input_elements {
                return invalid("input exceeds element budget");
            }
            let element = Element::Relation(relation);
            if visible(&element) && is_candidate(&element) {
                matched.push(element);
            }
        }
        if matched.len() > limits.max_matched {
            return invalid("matched candidates exceed budget");
        }
    }

    let mut index = IndexedReader::from_path_with_cache(&path.to_string_lossy(), 64)
        .map_err(|error| OsmReferenceError::Invalid(error.to_string()))?;
    let mut places = Vec::with_capacity(matched.len());
    let mut decisions = Vec::with_capacity(matched.len());
    let mut seen = HashSet::new();
    let mut census = Census {
        input_elements,
        matched: matched.len() as u64,
        ..Census::default()
    };

    for element in matched {
        if cancel() {
            return Err(OsmReferenceError::Cancelled);
        }
        let (osm_type, osm_id, raw_tags) = element_type(&element);
        let key = format!("osm:{osm_type}:{osm_id}");
        if !seen.insert(key.clone()) {
            census.duplicates += 1;
            decisions.push(Decision {
                key,
                osm_type: osm_type.into(),
                osm_id,
                state: "DUPLICATE".into(),
                reason: Some("duplicate source identity".into()),
            });
            continue;
        }
        let tags = match tag_map(raw_tags, limits) {
            Ok(tags) => tags,
            Err(OsmReferenceError::Invalid(reason)) => {
                census.quarantined += 1;
                decisions.push(Decision {
                    key,
                    osm_type: osm_type.into(),
                    osm_id,
                    state: "QUARANTINED".into(),
                    reason: Some(reason),
                });
                continue;
            }
            Err(other) => return Err(other),
        };
        let (coords, geometry) = match coordinates(&mut index, &element, cancel)? {
            Some((lat, lon, geometry)) => {
                if bbox.is_some_and(|b| !b.contains(lon, lat)) {
                    census.quarantined += 1;
                    decisions.push(Decision {
                        key,
                        osm_type: osm_type.into(),
                        osm_id,
                        state: "QUARANTINED".into(),
                        reason: Some("outside admitted region bbox".into()),
                    });
                    continue;
                }
                (Some((lat, lon)), geometry)
            }
            None => (None, "unknown".to_string()),
        };
        let site_kind = kind(&tags);
        if site_kind == "FUEL_STATION" || site_kind == "COMBINED_SITE" {
            census.fuel += 1;
        }
        if site_kind == "CHARGING_SITE" || site_kind == "COMBINED_SITE" {
            census.charging += 1;
        }
        if coords.is_none() {
            census.missing_geometry += 1;
        }
        let display_point = match geometry.as_str() {
            "node" => Some("osm_node".to_string()),
            "way_centroid" | "relation_centroid" => Some("osm_centroid".to_string()),
            _ => None,
        };
        places.push(ReferencePlace {
            key: key.clone(),
            osm_type: osm_type.into(),
            osm_id,
            kind: site_kind.into(),
            name: tags
                .get("name")
                .cloned()
                .or_else(|| tags.get("operator").cloned()),
            latitude_deg: coords.map(|(lat, _)| format!("{lat:.7}")),
            longitude_deg: coords.map(|(_, lon)| format!("{lon:.7}")),
            geometry,
            quality: if coords.is_some() {
                "osm-mapped".into()
            } else {
                "unknown".into()
            },
            display_point,
            cnpj: tags
                .get("ref:vatin")
                .or_else(|| tags.get("ref:CNPJ"))
                .and_then(|raw| crate::cnpj::normalize_cnpj(raw).ok()),
            access: tags.get("access").cloned(),
            opening_hours: tags.get("opening_hours").cloned(),
            operator: tags.get("operator").cloned(),
            brand: tags.get("brand").cloned(),
            tags,
            source: "OSM".into(),
            license: SOURCE_LICENSE.into(),
            attribution: SOURCE_ATTRIBUTION.into(),
        });
        census.accepted += 1;
        decisions.push(Decision {
            key,
            osm_type: osm_type.into(),
            osm_id,
            state: "ACCEPTED".into(),
            reason: None,
        });
    }
    places.sort_by(|a, b| a.key.cmp(&b.key));
    decisions.sort_by(|a, b| a.key.cmp(&b.key).then(a.state.cmp(&b.state)));
    Ok(ReferenceRun {
        places,
        decisions,
        census,
        source_sha256,
        input_bytes,
    })
}

fn coordinates(
    index: &mut IndexedReader<pbf_craft::readers::CachedReader>,
    element: &Element,
    cancel: &dyn Fn() -> bool,
) -> Result<Option<(f64, f64, String)>> {
    match element {
        Element::Node(node) => Ok(Some((
            nanodeg(node.latitude),
            nanodeg(node.longitude),
            "node".into(),
        ))),
        Element::Way(way) => {
            let deps = index
                .get_with_deps(&ElementType::Way, way.id)
                .map_err(|error| OsmReferenceError::Invalid(error.to_string()))?;
            let (lat, lon) = centroid(&deps, cancel)?;
            Ok(lat
                .zip(lon)
                .map(|(lat, lon)| (lat, lon, "way_centroid".into())))
        }
        Element::Relation(relation) => {
            let deps = index
                .get_with_deps(&ElementType::Relation, relation.id)
                .map_err(|error| OsmReferenceError::Invalid(error.to_string()))?;
            let (lat, lon) = centroid(&deps, cancel)?;
            Ok(lat
                .zip(lon)
                .map(|(lat, lon)| (lat, lon, "relation_centroid".into())))
        }
    }
}

fn centroid(elements: &[Element], cancel: &dyn Fn() -> bool) -> Result<(Option<f64>, Option<f64>)> {
    let mut nodes = BTreeMap::new();
    for element in elements {
        if cancel() {
            return Err(OsmReferenceError::Cancelled);
        }
        if let Element::Node(node) = element {
            nodes.insert(node.id, (nanodeg(node.latitude), nanodeg(node.longitude)));
        }
    }
    if nodes.is_empty() {
        return Ok((None, None));
    }
    let count = nodes.len() as f64;
    let (lat, lon) = nodes
        .values()
        .fold((0.0, 0.0), |(lat, lon), (node_lat, node_lon)| {
            (lat + node_lat, lon + node_lon)
        });
    Ok((Some(lat / count), Some(lon / count)))
}

fn nanodeg(value: i64) -> f64 {
    value as f64 / 1_000_000_000.0
}

#[derive(Debug, Serialize)]
struct FileDigest {
    bytes: u64,
    sha256: String,
}

#[derive(Debug, Serialize)]
struct OutputManifest {
    format: String,
    parser_version: String,
    source: String,
    license: String,
    attribution: String,
    source_sha256: String,
    input_bytes: u64,
    census: Census,
    files: BTreeMap<String, FileDigest>,
}

/// Writes a complete generation and swaps it into place. The previous
/// directory remains untouched on any failure before the final rename.
pub fn write_run(dir: &Path, run: &ReferenceRun) -> Result<()> {
    let parent = dir.parent().unwrap_or_else(|| Path::new("."));
    fs::create_dir_all(parent).map_err(|error| OsmReferenceError::Invalid(error.to_string()))?;
    let name = dir
        .file_name()
        .and_then(|value| value.to_str())
        .unwrap_or("reference");
    let temp = parent.join(format!(".{name}.tmp-{}", std::process::id()));
    let backup = parent.join(format!(".{name}.old-{}", std::process::id()));
    let _ = fs::remove_dir_all(&temp);
    let _ = fs::remove_dir_all(&backup);
    fs::create_dir(&temp).map_err(|error| OsmReferenceError::Invalid(error.to_string()))?;
    let mut files = BTreeMap::new();
    for (file_name, bytes) in [
        ("references.ndjson", ndjson(&run.places)?),
        ("decisions.ndjson", ndjson(&run.decisions)?),
    ] {
        write_file(&temp.join(file_name), &bytes)?;
        files.insert(
            file_name.to_string(),
            FileDigest {
                bytes: bytes.len() as u64,
                sha256: sha256_hex(&bytes),
            },
        );
    }
    let manifest = OutputManifest {
        format: REFERENCE_FORMAT.into(),
        parser_version: PARSER_VERSION.into(),
        source: "OSM".into(),
        license: SOURCE_LICENSE.into(),
        attribution: SOURCE_ATTRIBUTION.into(),
        source_sha256: run.source_sha256.clone(),
        input_bytes: run.input_bytes,
        census: run.census.clone(),
        files,
    };
    let manifest_bytes = serde_json::to_vec_pretty(&manifest)
        .map_err(|error| OsmReferenceError::Invalid(error.to_string()))?;
    write_file(&temp.join("manifest.json"), &manifest_bytes)?;
    if dir.exists() {
        fs::rename(dir, &backup).map_err(|error| OsmReferenceError::Invalid(error.to_string()))?;
        if let Err(error) = fs::rename(&temp, dir) {
            let _ = fs::rename(&backup, dir);
            return invalid(error.to_string());
        }
        let _ = fs::remove_dir_all(&backup);
    } else if let Err(error) = fs::rename(&temp, dir) {
        let _ = fs::remove_dir_all(&temp);
        return invalid(error.to_string());
    }
    Ok(())
}

fn ndjson<T: Serialize>(values: &[T]) -> Result<Vec<u8>> {
    let mut out = Vec::new();
    for value in values {
        serde_json::to_writer(&mut out, value)
            .map_err(|error| OsmReferenceError::Invalid(error.to_string()))?;
        out.push(b'\n');
    }
    Ok(out)
}

fn write_file(path: &Path, bytes: &[u8]) -> Result<()> {
    let file = File::create(path).map_err(|error| OsmReferenceError::Invalid(error.to_string()))?;
    let mut writer = BufWriter::new(file);
    writer
        .write_all(bytes)
        .map_err(|error| OsmReferenceError::Invalid(error.to_string()))?;
    writer
        .flush()
        .map_err(|error| OsmReferenceError::Invalid(error.to_string()))?;
    writer
        .get_ref()
        .sync_all()
        .map_err(|error| OsmReferenceError::Invalid(error.to_string()))?;
    Ok(())
}

fn sha256_hex(bytes: &[u8]) -> String {
    let mut hasher = Sha256::new();
    hasher.update(bytes);
    format!("{:x}", hasher.finalize())
}

/// Test fixture writer. Builds a small valid PBF from typed elements.
#[cfg(test)]
fn write_fixture(path: &Path, elements: Vec<Element>) {
    use pbf_craft::writers::PbfWriter;
    let mut writer = PbfWriter::from_path(path, true).expect("writer");
    for element in elements {
        writer.write(element).expect("write element");
    }
    writer.finish().expect("finish");
}

#[cfg(test)]
fn node(id: i64, lat: f64, lon: f64, tags: &[(&str, &str)]) -> Element {
    Element::Node(Node {
        id,
        version: -1,
        timestamp: None,
        user: None,
        changeset_id: -1,
        latitude: (lat * 1_000_000_000.0) as i64,
        longitude: (lon * 1_000_000_000.0) as i64,
        visible: true,
        tags: tags
            .iter()
            .map(|(key, value)| Tag {
                key: (*key).into(),
                value: (*value).into(),
            })
            .collect(),
    })
}

#[cfg(test)]
fn way(id: i64, node_ids: &[i64], tags: &[(&str, &str)]) -> Element {
    Element::Way(Way {
        id,
        version: -1,
        timestamp: None,
        user: None,
        changeset_id: -1,
        visible: true,
        tags: tags
            .iter()
            .map(|(key, value)| Tag {
                key: (*key).into(),
                value: (*value).into(),
            })
            .collect(),
        way_nodes: node_ids
            .iter()
            .map(|id| WayNode::new_without_coords(*id))
            .collect(),
    })
}

#[cfg(test)]
fn relation(id: i64, members: &[(ElementType, i64)], tags: &[(&str, &str)]) -> Element {
    Element::Relation(Relation {
        id,
        version: -1,
        timestamp: None,
        user: None,
        changeset_id: -1,
        visible: true,
        tags: tags
            .iter()
            .map(|(key, value)| Tag {
                key: (*key).into(),
                value: (*value).into(),
            })
            .collect(),
        members: members
            .iter()
            .map(|(member_type, member_id)| RelationMember {
                member_id: *member_id,
                member_type: (*member_type).clone(),
                role: "".into(),
            })
            .collect(),
    })
}

#[cfg(test)]
mod tests {
    use super::*;

    fn temp_dir(name: &str) -> std::path::PathBuf {
        let dir = std::env::temp_dir().join(format!("abastevo-osm-{name}-{}", std::process::id()));
        let _ = fs::remove_dir_all(&dir);
        fs::create_dir_all(&dir).unwrap();
        dir
    }

    fn fixture(dir: &Path) -> std::path::PathBuf {
        let path = dir.join("fixture.osm.pbf");
        write_fixture(
            &path,
            vec![
                node(
                    1,
                    -15.7939,
                    -47.8828,
                    &[
                        ("amenity", "fuel"),
                        ("name", "Posto Águas Claras"),
                        ("ref:vatin", "01122233300000"),
                    ],
                ),
                node(2, -15.7940, -47.8829, &[]),
                node(3, -15.7950, -47.8830, &[]),
                node(
                    4,
                    -23.5505,
                    -46.6333,
                    &[("amenity", "fuel"), ("name", "Fora da região")],
                ),
                way(
                    10,
                    &[2, 3],
                    &[
                        ("amenity", "charging_station"),
                        ("name", "Recarga"),
                        ("socket:type2", "2"),
                        ("opening_hours", "24/7"),
                    ],
                ),
                way(11, &[], &[("amenity", "fuel"), ("name", "Sem geometria")]),
                relation(
                    20,
                    &[(ElementType::Node, 1)],
                    &[("amenity", "fuel"), ("name", "Relação Posto")],
                ),
            ],
        );
        path
    }

    #[test]
    fn parses_nodes_ways_relations_and_preserves_unknown_tags() {
        let dir = temp_dir("parse");
        let path = fixture(&dir);
        let run = parse_file(
            &path,
            &Limits::default(),
            Some(BBox {
                min_lon_deg: -48.5,
                min_lat_deg: -16.5,
                max_lon_deg: -47.0,
                max_lat_deg: -15.0,
            }),
            &|| false,
        )
        .expect("parse");
        assert_eq!(run.census.accepted, 4);
        assert_eq!(run.census.missing_geometry, 1);
        assert_eq!(run.census.quarantined, 1);
        let fuel = run.places.iter().find(|place| place.osm_id == 1).unwrap();
        assert_eq!(fuel.quality, "osm-mapped");
        assert_eq!(fuel.display_point.as_deref(), Some("osm_node"));
        assert_eq!(fuel.cnpj.as_deref(), Some("01122233300000"));
        assert_eq!(fuel.tags.get("name").unwrap(), "Posto Águas Claras");
        let charging = run.places.iter().find(|place| place.osm_id == 10).unwrap();
        assert_eq!(charging.geometry, "way_centroid");
        assert_eq!(charging.display_point.as_deref(), Some("osm_centroid"));
        assert_eq!(charging.tags.get("socket:type2").unwrap(), "2");
        let missing = run.places.iter().find(|place| place.osm_id == 11).unwrap();
        assert_eq!(missing.quality, "unknown");
        assert!(missing.latitude_deg.is_none());
        let relation = run.places.iter().find(|place| place.osm_id == 20).unwrap();
        assert_eq!(relation.geometry, "relation_centroid");
        let outside = run
            .decisions
            .iter()
            .find(|decision| decision.osm_id == 4)
            .unwrap();
        assert_eq!(outside.state, "QUARANTINED");
    }

    #[test]
    fn rejects_truncated_input_and_honours_cancellation() {
        let dir = temp_dir("invalid");
        let path = fixture(&dir);
        let raw = fs::read(&path).unwrap();
        let truncated = dir.join("truncated.osm.pbf");
        fs::write(&truncated, &raw[..raw.len() / 2]).unwrap();
        assert!(parse_file(&truncated, &Limits::default(), None, &|| false).is_err());

        let cancel = parse_file(&path, &Limits::default(), None, &|| true);
        assert!(matches!(cancel, Err(OsmReferenceError::Cancelled)));
    }

    #[test]
    fn duplicate_ids_and_oversized_tags_are_accounted_for() {
        let dir = temp_dir("duplicates");
        let path = dir.join("duplicate.osm.pbf");
        let mut many = vec![("amenity", "fuel")];
        for _ in 0..70 {
            many.push(("note", "tag"));
        }
        write_fixture(
            &path,
            vec![
                node(1, -15.79, -47.88, &[("amenity", "fuel")]),
                node(1, -15.79, -47.88, &[("amenity", "fuel")]),
                node(2, -15.80, -47.89, &many),
            ],
        );
        let run = parse_file(&path, &Limits::default(), None, &|| false).expect("parse");
        assert_eq!(run.census.duplicates, 1);
        assert_eq!(run.census.quarantined, 1);
        assert_eq!(run.census.accepted, 1);
    }

    #[test]
    fn atomic_output_replaces_the_previous_generation() {
        let dir = temp_dir("output");
        let path = fixture(&dir);
        let run = parse_file(&path, &Limits::default(), None, &|| false).expect("parse");
        let out = dir.join("reference");
        write_run(&out, &run).expect("first write");
        assert!(out.join("manifest.json").exists());
        let manifest: serde_json::Value =
            serde_json::from_slice(&fs::read(out.join("manifest.json")).unwrap()).unwrap();
        assert_eq!(
            manifest["files"]["references.ndjson"]["sha256"]
                .as_str()
                .unwrap()
                .len(),
            64
        );
        assert!(fs::metadata(out.join("manifest.json")).unwrap().len() < 4096);
        let first = fs::read(out.join("references.ndjson")).unwrap();
        let mut second = run;
        second.places.clear();
        second.census.accepted = 0;
        write_run(&out, &second).expect("second write");
        assert_ne!(first, fs::read(out.join("references.ndjson")).unwrap());
        assert!(out.join("manifest.json").exists());
    }
}

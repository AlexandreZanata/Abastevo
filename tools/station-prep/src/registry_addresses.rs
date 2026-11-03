//! Hash-bound minimized station-address inputs. File preparation only; no fetch/import.
use crate::{
    cnpj::normalize_cnpj,
    registry::{RegistryBatch, RegistryRow},
    station_addresses::{
        self, AddressCandidate, AddressSource, MatchProof, Provenance, ReuseDecision, ReuseStatus,
    },
    types::sha256_hex,
};
use serde::{Deserialize, Serialize};
use serde_json::Value;
use std::{
    collections::{BTreeMap, BTreeSet, HashMap},
    io::Read,
    path::{Component, Path},
};
pub const PARSER_VERSION: &str = "station-prep-v0.3.1+addresses-v1";
pub const SOURCE_POLICY: &[u8] =
    include_bytes!("../../../contracts/station-prep/address-source-policy-v1.json");
const MAX_BYTES: u64 = 64 << 20;
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct AddressInputPin {
    pub source: String,
    pub profile: String,
    pub reference: String,
    pub edition: String,
    pub sha256: String,
    pub bytes: u64,
    pub rows: usize,
    pub license: String,
    pub attribution: String,
}
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct AddressSkip {
    pub source: String,
    pub reason: String,
}
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct AddressProvenance {
    pub profile: String,
    pub reference: String,
    pub edition: String,
    pub input_sha256: String,
    pub observed_at: String,
    pub license: String,
    pub attribution: String,
    pub rights_reference: String,
    pub rights_policy_sha256: String,
    pub proof: MatchProof,
}
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct AddressFacts {
    pub source: Option<AddressSource>,
    pub components: BTreeMap<String, String>,
    pub provenance: Option<AddressProvenance>,
}
#[derive(Debug, Clone, Default, Serialize, Deserialize)]
pub struct Transition {
    pub before: BTreeMap<String, u64>,
    pub after: BTreeMap<String, u64>,
}
#[derive(Debug, Clone, Default, Serialize, Deserialize)]
pub struct AddressCoverage {
    pub station_count: u64,
    pub national: Transition,
    pub by_uf: BTreeMap<String, Transition>,
    pub review_reasons: BTreeMap<String, u64>,
}
#[derive(Debug, Clone)]
pub struct AddressMetadata {
    pub inputs: Vec<AddressInputPin>,
    pub skips: Vec<AddressSkip>,
    pub coverage: AddressCoverage,
}
pub struct AddressInputs {
    pins: Vec<AddressInputPin>,
    skips: Vec<AddressSkip>,
    by_cnpj: HashMap<String, Vec<AddressCandidate>>,
}
#[derive(Deserialize)]
#[serde(deny_unknown_fields)]
struct Descriptor {
    format_version: String,
    inputs: Vec<Value>,
}
#[derive(Deserialize)]
#[serde(deny_unknown_fields)]
struct RfbRow {
    schema_version: String,
    cnpj: String,
    active: bool,
    address: BTreeMap<String, String>,
    observed_at: String,
}
#[derive(Deserialize)]
#[serde(deny_unknown_fields)]
struct OsmRow {
    schema_version: String,
    cnpj: String,
    object_type: String,
    object_id: i64,
    match_reference: String,
    address: BTreeMap<String, String>,
    observed_at: String,
}
fn bounded(path: &Path, cap: u64) -> Result<Vec<u8>, String> {
    let m = std::fs::symlink_metadata(path).map_err(|_| "address input unavailable")?;
    if !m.is_file() || m.file_type().is_symlink() || m.len() > cap {
        return Err("regular bounded address input required".into());
    }
    let mut raw = Vec::new();
    std::fs::File::open(path)
        .map_err(|_| "address input unavailable")?
        .take(cap + 1)
        .read_to_end(&mut raw)
        .map_err(|_| "address input read")?;
    if raw.len() as u64 > cap {
        return Err("address byte budget exceeded".into());
    }
    Ok(raw)
}
fn text(v: &str, cap: usize) -> bool {
    !v.trim().is_empty() && v.len() <= cap && !v.chars().any(char::is_control)
}
pub fn load_address_inputs(path: &Path) -> Result<AddressInputs, String> {
    let raw = bounded(path, 128 << 10)?;
    let descriptor: Descriptor =
        serde_json::from_slice(&raw).map_err(|_| "address descriptor shape")?;
    if descriptor.format_version != "registry-address-inputs-v1" || descriptor.inputs.len() > 3 {
        return Err("unsupported address input set".into());
    }
    let policy: Value = serde_json::from_slice(SOURCE_POLICY).expect("compiled address policy");
    let parent = std::fs::canonicalize(
        path.parent()
            .filter(|p| !p.as_os_str().is_empty())
            .unwrap_or(Path::new(".")),
    )
    .map_err(|_| "address descriptor directory")?;
    let mut result = AddressInputs {
        pins: Vec::new(),
        skips: Vec::new(),
        by_cnpj: HashMap::new(),
    };
    let mut seen = BTreeSet::new();
    for mut value in descriptor.inputs {
        let file = value
            .as_object_mut()
            .and_then(|v| v.remove("file"))
            .and_then(|v| v.as_str().map(String::from))
            .ok_or("address input basename missing")?;
        let parts: Vec<_> = Path::new(&file).components().collect();
        if parts.len() != 1
            || !matches!(parts[0], Component::Normal(_))
            || file.contains(['/', '\\'])
        {
            return Err("address input escapes descriptor directory".into());
        }
        let pin: AddressInputPin =
            serde_json::from_value(value).map_err(|_| "address input pin shape")?;
        let rule = &policy["sources"][&pin.source];
        if !seen.insert(pin.source.clone())
            || rule.is_null()
            || rule["profile"] != pin.profile
            || !text(&pin.reference, 2048)
            || !text(&pin.edition, 256)
            || pin.bytes > MAX_BYTES
            || pin.rows > 500_000
            || pin.sha256.len() != 64
            || !pin.sha256.bytes().all(|b| b.is_ascii_hexdigit())
        {
            return Err("address source profile/bounds refused".into());
        }
        if rule["status"] != "ADMITTED" {
            result.skips.push(AddressSkip {
                source: pin.source,
                reason: "source_reuse_not_admitted".into(),
            });
            continue;
        }
        if rule["license"] != pin.license || rule["attribution"] != pin.attribution {
            return Err("address source rights differ from reviewed policy".into());
        }
        let input = parent.join(file);
        let raw = bounded(&input, MAX_BYTES)?;
        if pin.bytes != raw.len() as u64 || pin.sha256 != sha256_hex(&raw) {
            return Err("address input fingerprint mismatch".into());
        }
        let raw = std::str::from_utf8(&raw).map_err(|_| "address input encoding")?;
        if !raw.is_empty() && !raw.ends_with('\n') {
            return Err("truncated address JSONL".into());
        }
        if raw.lines().count() != pin.rows {
            return Err("address row count mismatch".into());
        }
        for line in raw.lines() {
            if line.len() > 64 << 10 {
                return Err("address record budget exceeded".into());
            }
            let (cnpj, address, observed_at, proof) = match pin.source.as_str() {
                "rfb" => {
                    let r: RfbRow = serde_json::from_str(line)
                        .map_err(|_| "minimized RFB address row shape")?;
                    if r.schema_version != pin.profile {
                        return Err("RFB address row profile".into());
                    }
                    let proof = MatchProof::Rfb {
                        cnpj: r.cnpj.clone(),
                        active: r.active,
                    };
                    (r.cnpj, r.address, r.observed_at, proof)
                }
                "osm" => {
                    let r: OsmRow = serde_json::from_str(line)
                        .map_err(|_| "minimized OSM address row shape")?;
                    if r.schema_version != pin.profile {
                        return Err("OSM address row profile".into());
                    }
                    let proof = MatchProof::Osm {
                        cnpj: r.cnpj.clone(),
                        object_type: r.object_type,
                        object_id: r.object_id,
                        link_reference: r.match_reference,
                    };
                    (r.cnpj, r.address, r.observed_at, proof)
                }
                _ => return Err("unimplemented address source cannot be admitted".into()),
            };
            let normalized = normalize_cnpj(&cnpj).map_err(|_| "address CNPJ")?;
            if normalized != cnpj
                || crate::registry::normalize_calendar_date(&observed_at)
                    .map_err(|_| "address observation date")?
                    != observed_at
                || address
                    .keys()
                    .any(|k| !station_addresses::COMPONENTS.contains(&k.as_str()))
                || address
                    .values()
                    .any(|v| v.len() > 2048 || v.chars().any(char::is_control))
            {
                return Err("address minimized fields/date/identity refused".into());
            }
            let source = if pin.source == "rfb" {
                AddressSource::Rfb
            } else {
                AddressSource::Osm
            };
            let row = AddressCandidate {
                source,
                components: address,
                provenance: Provenance {
                    reference: pin.reference.clone(),
                    edition: pin.edition.clone(),
                    input_sha256: pin.sha256.clone(),
                    observed_at,
                },
                proof,
                reuse: ReuseDecision {
                    status: ReuseStatus::Admitted,
                    reference: rule["rights_reference"]
                        .as_str()
                        .expect("compiled rights reference")
                        .into(),
                },
            };
            let rows = result.by_cnpj.entry(cnpj).or_default();
            if rows.len() >= 256 {
                return Err("station address observation budget exceeded".into());
            }
            rows.push(row);
        }
        result.pins.push(pin);
    }
    result.pins.sort_by(|a, b| a.source.cmp(&b.source));
    result.skips.sort_by(|a, b| a.source.cmp(&b.source));
    Ok(result)
}
fn input(row: &RegistryRow) -> station_addresses::StationAddressInput {
    let address = &row.address_normalized;
    let official_address = BTreeMap::from([
        ("raw".into(), row.address_raw.clone()),
        ("street".into(), address.street.clone()),
        ("number".into(), address.number.clone()),
        ("complement".into(), address.complement.clone()),
        ("district".into(), address.district.clone()),
        ("postal_code".into(), address.postal_code.clone()),
        ("municipio".into(), row.municipality_name_raw.clone()),
        ("municipio_ibge".into(), row.municipality_ibge.clone()),
        ("uf".into(), row.uf.clone()),
    ]);
    station_addresses::StationAddressInput {
        cnpj: row.source_key.clone(),
        municipality_ibge: row.municipality_ibge.clone(),
        municipality: row.municipality_name_raw.clone(),
        uf: row.uf.clone(),
        official_address,
    }
}
fn category(source: Option<AddressSource>) -> &'static str {
    match source {
        Some(AddressSource::Anp) => "anp",
        Some(AddressSource::Rfb) => "rfb",
        Some(AddressSource::Osm) => "osm",
        Some(AddressSource::Cnefe) => "cnefe",
        Some(AddressSource::Municipality) => "municipality",
        None => "unknown",
    }
}
fn add(transition: &mut Transition, before: &str, after: &str) {
    *transition.before.entry(before.into()).or_default() += 1;
    *transition.after.entry(after.into()).or_default() += 1;
}
pub fn enrich_registry_addresses(
    batch: &mut RegistryBatch,
    inputs: &AddressInputs,
) -> Result<(), String> {
    if !batch.name_profile
        || batch
            .accepted
            .iter()
            .chain(&batch.duplicates)
            .any(|r| r.legal_name.is_none())
    {
        return Err("address enrichment requires named parsing".into());
    }
    let mut coverage = AddressCoverage::default();
    let rights_policy_sha256 = sha256_hex(SOURCE_POLICY);
    let mut mapping = HashMap::new();
    let mut current = BTreeSet::new();
    for row in batch.accepted.iter_mut().chain(&mut batch.duplicates) {
        let station = input(row);
        let before = station_addresses::select_address(&station, &[])?;
        let candidates = inputs
            .by_cnpj
            .get(&row.source_key)
            .map(Vec::as_slice)
            .unwrap_or(&[]);
        let selected = station_addresses::select_address(&station, candidates)?;
        let provenance = if let Some(p) = &selected.provenance {
            let c = candidates
                .iter()
                .find(|c| {
                    c.source == selected.source.expect("source provenance") && c.provenance == *p
                })
                .ok_or("address choice has no source input")?;
            let pin = inputs
                .pins
                .iter()
                .find(|pin| pin.sha256 == p.input_sha256 && pin.source == category(selected.source))
                .ok_or("address choice has no admitted pin")?;
            Some(AddressProvenance {
                profile: pin.profile.clone(),
                reference: p.reference.clone(),
                edition: p.edition.clone(),
                input_sha256: p.input_sha256.clone(),
                observed_at: p.observed_at.clone(),
                license: pin.license.clone(),
                attribution: pin.attribution.clone(),
                rights_reference: c.reuse.reference.clone(),
                rights_policy_sha256: rights_policy_sha256.clone(),
                proof: c.proof.clone(),
            })
        } else {
            None
        };
        // Only current unique CNPJ contributes to the coverage denominator.
        if row.supersedes.is_none() && current.insert(row.source_key.clone()) {
            coverage.station_count += 1;
            add(
                &mut coverage.national,
                category(before.source),
                category(selected.source),
            );
            add(
                coverage.by_uf.entry(row.uf.clone()).or_default(),
                category(before.source),
                category(selected.source),
            );
            for review in &selected.reviews {
                *coverage
                    .review_reasons
                    .entry(review.reason.clone())
                    .or_default() += 1;
            }
        }
        let old = row.checksum.clone();
        row.schema_version = "station-assertion-v4";
        row.address_facts = Some(AddressFacts {
            source: selected.source,
            components: selected.components,
            provenance,
        });
        let mut canonical = row.clone();
        canonical.checksum.clear();
        canonical.supersedes = None;
        row.checksum = sha256_hex(canonical.to_jsonl().as_bytes());
        mapping.insert(old, row.checksum.clone());
    }
    for row in batch.accepted.iter_mut().chain(&mut batch.duplicates) {
        if let Some(old) = &row.supersedes {
            row.supersedes = Some(
                mapping
                    .get(old)
                    .ok_or("missing v4 succession target")?
                    .clone(),
            );
        }
    }
    batch
        .accepted
        .sort_by(|a, b| (&a.source_key, &a.checksum).cmp(&(&b.source_key, &b.checksum)));
    batch.address_metadata = Some(AddressMetadata {
        inputs: inputs.pins.clone(),
        skips: inputs.skips.clone(),
        coverage,
    });
    Ok(())
}
pub fn apply_manifest(manifest: &mut crate::output::Manifest, batch: &RegistryBatch) {
    if let Some(meta) = &batch.address_metadata {
        manifest.parser_version = PARSER_VERSION.into();
        manifest.address_policy_sha256 = Some(sha256_hex(SOURCE_POLICY));
        manifest.address_inputs = Some(meta.inputs.clone());
        manifest.address_skips = Some(meta.skips.clone());
        manifest.address_coverage = Some(meta.coverage.clone());
    }
}

//! Local rights bundle and publication admission.
use crate::ev_conflation::CanonicalSite;
use crate::ev_conflation_preparation::{decode, FORMAT, PROFILE};
use crate::ev_rights::generation_rights;
use crate::ev_site_preparation::{
    checked_raw, read_bounded, write_bytes, write_lines, PrepError, SourcePins,
};
use crate::types::sha256_hex;
use serde::Serialize;
use serde_json::{json, Value};
use std::collections::BTreeMap;
use std::fs;
use std::os::unix::fs::DirBuilderExt;
use std::path::Path;

const CATALOG: &str = "catalog.ndjson";
const DATA: &str = "derived-database.ndjson";
const OFFER: &str = "derived-database-offer.json";
const RIGHTS: &str = "rights.json";
const MAX_DATA: u64 = 512 << 20;
const MAX_RECEIPT: u64 = 8 << 20;

type Result<T> = std::result::Result<T, PrepError>;

struct Admission {
    sites: Vec<CanonicalSite>,
    rights: Value,
    rights_bytes: Vec<u8>,
    offer_bytes: Option<Vec<u8>>,
}

fn admit(catalog: &Path, manifest_sha256: &str) -> Result<Admission> {
    let manifest_bytes = checked_raw(&catalog.join("manifest.json"), MAX_RECEIPT, manifest_sha256)?;
    let manifest: Value = serde_json::from_slice(&manifest_bytes)
        .map_err(|_| PrepError("conflation manifest shape refused"))?;
    let pins = SourcePins::frozen();
    let expected = json!({"id":"ev-conflation-v1","sha256":sha256_hex(PROFILE),"exposure":"PRIVATE_PREPARATION"});
    if manifest["format"] != FORMAT
        || manifest["conflation_profile"] != expected
        || manifest["source_pins"]
            != serde_json::to_value(&pins).map_err(|_| PrepError("compiled source pins refused"))?
    {
        return Err(PrepError("conflation manifest admission refused"));
    }
    let descriptor = &manifest["files"][CATALOG];
    let digest = descriptor["sha256"]
        .as_str()
        .ok_or(PrepError("catalog hash missing"))?;
    let raw = checked_raw(&catalog.join(CATALOG), MAX_DATA, digest)?;
    let sites = decode(&raw, 205_000)?;
    if descriptor["bytes"].as_u64() != Some(raw.len() as u64)
        || descriptor["records"].as_u64() != Some(sites.len() as u64)
    {
        return Err(PrepError("catalog size or record count mismatch"));
    }
    let mut lines = raw.split_inclusive(|byte| *byte == b'\n');
    for site in &sites {
        let canonical =
            serde_json::to_vec(site).map_err(|_| PrepError("catalog record encoding refused"))?;
        if lines.next().and_then(|line| line.strip_suffix(b"\n")) != Some(canonical.as_slice()) {
            return Err(PrepError("noncanonical catalog record refused"));
        }
    }
    if lines.next().is_some() {
        return Err(PrepError("trailing catalog record refused"));
    }
    let mut rights = generation_rights(digest, &sites).map_err(PrepError)?;
    rights["catalog_manifest_sha256"] = json!(manifest_sha256);
    let offer_bytes = if rights["requires_derived_database_offer"] == true {
        let data = json!({"path":DATA,"sha256":digest,"bytes":raw.len(),"records":sites.len()});
        let offer = json!({
            "format":"abastevo.ev-derived-database-offer-v1",
            "scope":"derived national charging catalog",
            "data":data,
            "license":"ODbL-1.0",
            "license_url":"https://opendatacommons.org/licenses/odbl/1-0/",
            "osm_attribution":rights["osm"]["attribution"],
            "osm_attribution_url":"https://www.openstreetmap.org/copyright",
            "osm_extract":rights["osm"],
            "ocm_provider_terms":rights["ocm"]["providers"],
        });
        let bytes =
            serde_json::to_vec_pretty(&offer).map_err(|_| PrepError("offer encoding refused"))?;
        rights["offer"] = json!({
            "data":data,
            "notice":{"path":OFFER,"bytes":bytes.len(),"sha256":sha256_hex(&bytes)}
        });
        Some(bytes)
    } else {
        rights["offer"] = Value::Null;
        None
    };
    let rights_bytes =
        serde_json::to_vec_pretty(&rights).map_err(|_| PrepError("rights encoding refused"))?;
    Ok(Admission {
        sites,
        rights,
        rights_bytes,
        offer_bytes,
    })
}

fn validate_files(output: &Path, admission: &Admission) -> Result<()> {
    let mut files = 0;
    for entry in fs::read_dir(output).map_err(|_| PrepError("rights bundle missing"))? {
        let entry = entry.map_err(|_| PrepError("rights bundle entry refused"))?;
        let name = entry.file_name();
        if name != RIGHTS && (admission.offer_bytes.is_none() || (name != OFFER && name != DATA)) {
            return Err(PrepError("unexpected rights bundle file"));
        }
        files += 1;
    }
    if files
        != if admission.offer_bytes.is_some() {
            3
        } else {
            1
        }
    {
        return Err(PrepError("rights or offer file missing"));
    }
    if read_bounded(&output.join(RIGHTS), MAX_RECEIPT)? != admission.rights_bytes {
        return Err(PrepError("generation rights bytes mismatch"));
    }
    if let Some(notice) = &admission.offer_bytes {
        if read_bounded(&output.join(OFFER), MAX_RECEIPT)? != *notice {
            return Err(PrepError("derived database offer terms mismatch"));
        }
        let digest = admission.rights["catalog_sha256"]
            .as_str()
            .expect("admitted catalog digest");
        checked_raw(&output.join(DATA), MAX_DATA, digest)?;
    }
    Ok(())
}

/// Stage an exact, local offer. The offer reserializes only the admitted
/// catalog schema and requires byte parity before marking the bundle valid.
pub fn prepare_bundle(
    catalog: &Path,
    manifest_sha256: &str,
    output: &Path,
    cancel: &dyn Fn() -> bool,
) -> Result<Value> {
    if cancel() {
        return Err(PrepError("cancelled"));
    }
    let admission = admit(catalog, manifest_sha256)?;
    fs::DirBuilder::new()
        .mode(0o700)
        .create(output)
        .map_err(|_| PrepError("private rights output creation refused"))?;
    let result = (|| {
        if let Some(notice) = &admission.offer_bytes {
            let data = write_lines(&output.join(DATA), &admission.sites, cancel)?;
            if data["sha256"] != admission.rights["catalog_sha256"] {
                return Err(PrepError("catalog contains noncanonical offer records"));
            }
            write_bytes(&output.join(OFFER), notice)?;
        }
        if cancel() {
            return Err(PrepError("cancelled"));
        }
        write_bytes(&output.join(RIGHTS), &admission.rights_bytes)?;
        validate_files(output, &admission)?;
        fs::File::open(output)
            .and_then(|f| f.sync_all())
            .map_err(|_| PrepError("rights bundle sync refused"))?;
        Ok(())
    })();
    if result.is_err() {
        let _ = fs::remove_dir_all(output);
    }
    result?;
    Ok(admission.rights)
}

/// Fail closed if the catalog, rights or required offer changed after
/// preparation. This verifies the local candidate, without publishing it.
pub fn validate_bundle(catalog: &Path, manifest_sha256: &str, bundle: &Path) -> Result<Value> {
    let admission = admit(catalog, manifest_sha256)?;
    validate_files(bundle, &admission)?;
    Ok(admission.rights)
}

pub(crate) fn catalog_geography(
    index: &crate::ev_geography::StateIndex,
    catalog_digest: &str,
    sites: &[CanonicalSite],
) -> Value {
    let mut records: Vec<_> = sites.iter().collect();
    records.sort_by(|a, b| a.key.cmp(&b.key));
    let decisions: Vec<_> = records
        .into_iter()
        .map(|site| {
            let result = index.classify(
                site.point.latitude_decimal.parse().unwrap_or(f64::NAN),
                site.point.longitude_decimal.parse().unwrap_or(f64::NAN),
            );
            json!({"key":site.key,"uf":result.uf.as_deref().unwrap_or("UNASSIGNED"),"reason":result.reason})
        })
        .collect();
    json!({
        "format":"abastevo.ev-catalog-geography-v1",
        "catalog_sha256":catalog_digest,
        "boundary_sha256":index.source_sha256,
        "sites":decisions
    })
}

/// Produce the geography sidecar separately from the three-file rights offer.
pub fn prepare_geography(
    catalog: &Path,
    manifest_sha256: &str,
    boundaries: &Path,
    output: &Path,
) -> Result<Value> {
    let admission = admit(catalog, manifest_sha256)?;
    let boundary_digest = SourcePins::frozen().boundary_sha256;
    let raw = checked_raw(boundaries, 8 << 20, &boundary_digest)?;
    let index = crate::ev_geography::StateIndex::from_bytes(&raw)
        .map_err(|_| PrepError("boundary model refused"))?;
    let catalog_digest = admission.rights["catalog_sha256"]
        .as_str()
        .expect("admitted digest");
    let geography = catalog_geography(&index, catalog_digest, &admission.sites);
    let bytes =
        serde_json::to_vec(&geography).map_err(|_| PrepError("geography encoding refused"))?;
    if bytes.len() > 64 << 20 {
        return Err(PrepError("geography file budget exceeded"));
    }
    write_bytes(output, &bytes)
}

#[derive(Default, Serialize)]
struct UFCoverage {
    sites: usize,
    municipalities: usize,
    municipalities_with_sites: usize,
}

#[derive(Serialize)]
struct MunicipalityCoverage {
    uf: &'static str,
    sites: usize,
}

pub(crate) fn catalog_coverage(
    index: &crate::ev_geography::StateIndex,
    catalog_digest: &str,
    sites: &[CanonicalSite],
) -> Value {
    let mut by_uf: BTreeMap<_, _> = crate::ev_geography::STATES
        .into_iter()
        .map(|uf| (uf, UFCoverage::default()))
        .collect();
    let mut by_municipality: BTreeMap<_, _> = index
        .municipalities()
        .map(|(code, uf)| {
            by_uf.get_mut(uf).expect("known UF").municipalities += 1;
            (code, MunicipalityCoverage { uf, sites: 0 })
        })
        .collect();
    let mut unassigned_uf = 0;
    let mut unassigned_municipality = 0;
    for site in sites {
        let point = index.classify(
            site.point.latitude_decimal.parse().unwrap_or(f64::NAN),
            site.point.longitude_decimal.parse().unwrap_or(f64::NAN),
        );
        if let Some(uf) = point.uf {
            by_uf.get_mut(uf.as_str()).expect("known UF").sites += 1;
        } else {
            unassigned_uf += 1;
        }
        if let Some(code) = point.municipality_code {
            let municipality = by_municipality
                .get_mut(code.as_str())
                .expect("known municipality");
            if municipality.sites == 0 {
                by_uf
                    .get_mut(municipality.uf)
                    .expect("known UF")
                    .municipalities_with_sites += 1;
            }
            municipality.sites += 1;
        } else {
            unassigned_municipality += 1;
        }
    }
    json!({
        "format": "abastevo.ev-geographic-coverage-v1",
        "unit": "canonical_site",
        "catalog_sha256": catalog_digest,
        "boundary_sha256": index.source_sha256,
        "boundary_attribution": "Fonte: IBGE",
        "sites": sites.len(),
        "unassigned_uf": unassigned_uf,
        "unassigned_municipality": unassigned_municipality,
        "by_uf": by_uf,
        "by_municipality": by_municipality
    })
}

pub fn prepare_coverage(
    catalog: &Path,
    manifest_sha256: &str,
    boundaries: &Path,
    output: &Path,
) -> Result<Value> {
    let admission = admit(catalog, manifest_sha256)?;
    let raw = checked_raw(boundaries, 8 << 20, &SourcePins::frozen().boundary_sha256)?;
    let index = crate::ev_geography::StateIndex::from_bytes(&raw)
        .map_err(|_| PrepError("boundary model refused"))?;
    let catalog_digest = admission.rights["catalog_sha256"]
        .as_str()
        .expect("admitted digest");
    let coverage = catalog_coverage(&index, catalog_digest, &admission.sites);
    let bytes =
        serde_json::to_vec(&coverage).map_err(|_| PrepError("coverage encoding refused"))?;
    write_bytes(output, &bytes)
}

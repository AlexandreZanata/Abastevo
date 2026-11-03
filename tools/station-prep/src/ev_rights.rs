//! Generation rights admission for the pinned national charging catalog.
use crate::ev_conflation::CanonicalSite;
use crate::ev_osm_sites::OsmContext;
use crate::ev_site_preparation::SourcePins;
use crate::types::sha256_hex;
use serde_json::{json, Value};
use std::collections::{BTreeMap, BTreeSet};

const PROFILE: &[u8] =
    include_bytes!("../../../contracts/ev/national-source-pins-20261103-v1.json");
pub const FORMAT: &str = "abastevo.ev-generation-rights-v1";

fn sha256(value: &str) -> bool {
    value.len() == 64
        && value
            .bytes()
            .all(|b| b.is_ascii_digit() || (b'a'..=b'f').contains(&b))
}

/// The caller must hash-check and decode the catalog bytes before calling.
/// Every retained root and equipment assertion must be covered by this
/// exact policy. A single mismatch prevents a manifest for the generation.
pub fn generation_rights(
    catalog_sha256: &str,
    sites: &[CanonicalSite],
) -> Result<Value, &'static str> {
    if !sha256(catalog_sha256) || sites.is_empty() || sites.len() > 205_000 {
        return Err("catalog rights input refused");
    }
    let pins = SourcePins::frozen();
    let osm_context = OsmContext::new(&pins.osm_revision, &pins.osm_sha256)?;
    let profile: Value = serde_json::from_slice(PROFILE).expect("compiled EV source policy");
    let osm = &profile["sources"]["OSM"];
    let ocm = &profile["sources"]["OCM"];
    let providers: BTreeMap<_, _> = ocm["providers"]
        .as_array()
        .expect("compiled OCM providers")
        .iter()
        .map(|provider| {
            (
                provider["id"].as_i64().expect("compiled provider ID"),
                provider,
            )
        })
        .collect();
    let mut catalog_keys = BTreeSet::new();
    let mut source_keys = BTreeSet::new();
    let mut osm_count = 0u64;
    let mut counts = BTreeMap::<i64, u64>::new();
    for site in sites {
        if !catalog_keys.insert(&site.key) || site.sources.is_empty() || site.sources.len() > 2 {
            return Err("incomplete or duplicate site lineage");
        }
        let mut root_sources = BTreeSet::new();
        for row in &site.sources {
            if !root_sources.insert(&row.source.source) {
                return Err("duplicate root source");
            }
        }
        for row in site.sources.iter().chain(&site.equipment) {
            let source = &row.source;
            let namespace = match source.source.as_str() {
                "OSM" => "osm",
                "OCM" => "ocm:poi",
                _ => return Err("unknown rights source"),
            };
            if source_keys.len() >= 205_000
                || source.external_id.is_empty()
                || source.external_id.len() > 128
                || source.external_id.chars().any(char::is_control)
                || row.key != format!("{namespace}:{}", source.external_id)
                || !source_keys.insert(&row.key)
                || !sha256(&source.record_sha256)
            {
                return Err("invalid or duplicate source lineage");
            }
            if namespace == "osm" {
                if source.revision != osm_context.revision()
                    || source.provider_id.is_some()
                    || osm["license"].as_str() != Some(source.license.as_str())
                    || osm["attribution"].as_str() != Some(source.attribution.as_str())
                {
                    return Err("OSM source rights mismatch");
                }
                osm_count += 1;
            } else {
                let id = source.provider_id.ok_or("OCM provider rights missing")?;
                let policy = providers.get(&id).ok_or("unknown OCM provider")?;
                if source.revision != pins.ocm_revision
                    || policy["license"].as_str() != Some(source.license.as_str())
                    || policy["attribution"].as_str() != Some(source.attribution.as_str())
                    || policy["license_sha256"].as_str()
                        != Some(&sha256_hex(source.license.as_bytes()))
                {
                    return Err("OCM source rights mismatch");
                }
                *counts.entry(id).or_default() += 1;
            }
        }
    }
    let admitted_providers: Vec<_> = counts
        .iter()
        .map(|(id, count)| {
            let mut provider = providers[id].clone();
            provider["assertions"] = json!(count);
            provider
        })
        .collect();
    Ok(json!({
        "format": FORMAT,
        "catalog_sha256": catalog_sha256,
        "catalog_sites": sites.len(),
        "source_profile": {"id": pins.profile_id, "sha256": pins.profile_sha256},
        "osm": {
            "revision": pins.osm_revision, "sha256": pins.osm_sha256, "bytes": pins.osm_bytes,
            "url": osm["url"], "license": osm["license"], "attribution": osm["attribution"],
            "assertions": osm_count,
        },
        "ocm": {
            "revision": pins.ocm_revision, "reference_sha256": pins.ocm_reference_sha256,
            "inventory_sha256": pins.ocm_inventory_sha256,
            "source_date": ocm["source_date"], "freshness": ocm["freshness"],
            "assertions": counts.values().sum::<u64>(), "providers": admitted_providers,
        },
        "requires_derived_database_offer": osm_count != 0,
    }))
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::ev_conflation::conflate;
    use crate::ev_site_preparation::SourcePins;
    use crate::ev_sites::SourceSite;
    use serde_json::json;

    fn source(osm: bool) -> SourceSite {
        let pins = SourcePins::frozen();
        let (key, external, source, revision, provider, license, attribution) = if osm {
            (
                "osm:node:101",
                "node:101",
                "OSM",
                format!("{}@sha256:{}", pins.osm_revision, pins.osm_sha256),
                None,
                "ODbL-1.0",
                "© OpenStreetMap contributors (ODbL)",
            )
        } else {
            (
                "ocm:poi:101",
                "101",
                "OCM",
                pins.ocm_revision,
                Some(40),
                "CC-0",
                "Easy Volt",
            )
        };
        serde_json::from_value(json!({
            "key":key,"kind":"CHARGING_SITE","site_marker":true,"equipment_marker":false,
            "footprint":null,"source":{"source":source,"external_id":external,"revision":revision,
            "record_sha256":"a".repeat(64),"object_version":null,"clocks":{},"provider_id":provider,
            "license":license,"attribution":attribution},"point":{"latitude_decimal":"-23.5",
            "longitude_decimal":"-46.6","provenance":"source"},"name":"Synthetic Charging",
            "operator":null,"network":null,"brand":null,"address":{},"access":"PUBLIC",
            "raw_access":null,"fee":null,"opening_hours":null,"status":"UNKNOWN","raw_status":null,
            "groups":[],"parent_keys":[]
        }))
        .unwrap()
    }

    #[test]
    fn geography_binds_each_canonical_site_without_inventing_state() {
        let boundary = br#"{"type":"FeatureCollection","features":[{"type":"Feature","properties":{"codarea":"3550308"},"geometry":{"type":"Polygon","coordinates":[[[-47,-24],[-46,-24],[-46,-23],[-47,-23],[-47,-24]]]}}]}"#;
        let index = crate::ev_geography::StateIndex::from_bytes(boundary).unwrap();
        let mut outside = source(false);
        outside.point.latitude_decimal = "-10".into();
        outside.point.longitude_decimal = "-50".into();
        let sites = conflate(&[source(true), outside]).unwrap().sites;
        let hash = "a".repeat(64);
        let receipt = crate::ev_rights_bundle::catalog_geography(&index, &hash, &sites);
        let coverage = crate::ev_rights_bundle::catalog_coverage(&index, &hash, &sites);
        assert_eq!(coverage["sites"], 2);
        assert_eq!(coverage["by_municipality"]["3550308"]["sites"], 1);
        assert_eq!(coverage["by_uf"]["SP"]["sites"], 1);
        assert_eq!(coverage["by_uf"]["AC"]["sites"], 0);
        assert_eq!(coverage["unassigned_municipality"], 1);
        assert_eq!(receipt["catalog_sha256"], hash);
        assert_eq!(receipt["boundary_sha256"], sha256_hex(boundary));
        for site in sites {
            let uf = if site.point.latitude_decimal == "-23.5" {
                "SP"
            } else {
                "UNASSIGNED"
            };
            let row = receipt["sites"]
                .as_array()
                .unwrap()
                .iter()
                .find(|row| row["key"] == site.key)
                .unwrap();
            assert_eq!(row["uf"], uf);
        }
    }

    #[test]
    fn overlapping_municipalities_preserve_only_unambiguous_uf() {
        let feature = json!({"type":"Feature","properties":{"codarea":"3550308"},
            "geometry":{"type":"Polygon","coordinates":[[[-47,-24],[-46,-24],[-46,-23],[-47,-23],[-47,-24]]]}});
        let mut overlapping = feature.clone();
        overlapping["properties"]["codarea"] = json!("3548708");
        let raw = serde_json::to_vec(
            &json!({"type":"FeatureCollection","features":[feature,overlapping]}),
        )
        .unwrap();
        let index = crate::ev_geography::StateIndex::from_bytes(&raw).unwrap();
        let inside = index.classify(-23.5, -46.5);
        assert_eq!(inside.uf.as_deref(), Some("SP"));
        assert_eq!(inside.municipality_code, None);
        let boundary = index.classify(-23.5, -46.0);
        assert_eq!(boundary.uf, None);
        assert_eq!(boundary.municipality_code, None);
    }

    #[test]
    fn rights_bundle_requires_exact_offer_bytes() {
        use crate::ev_conflation_preparation;
        use crate::ev_rights_bundle::{prepare_bundle, validate_bundle};
        use crate::ev_site_preparation::{write_bytes, write_lines};
        let root = std::env::temp_dir().join(format!(
            "abastevo-ev-rights-{}-{}",
            std::process::id(),
            std::time::SystemTime::now()
                .duration_since(std::time::UNIX_EPOCH)
                .unwrap()
                .as_nanos()
        ));
        let catalog_dir = root.join("catalog");
        let output = root.join("bundle");
        std::fs::create_dir_all(&catalog_dir).unwrap();
        let sites = conflate(&[source(true), source(false)]).unwrap().sites;
        let descriptor =
            write_lines(&catalog_dir.join("catalog.ndjson"), &sites, &|| false).unwrap();
        let pins = SourcePins::frozen();
        let manifest = json!({
            "format":ev_conflation_preparation::FORMAT,
            "conflation_profile":{"id":"ev-conflation-v1","sha256":sha256_hex(ev_conflation_preparation::PROFILE),"exposure":"PRIVATE_PREPARATION"},
            "source_pins":pins,"files":{"catalog.ndjson":descriptor}
        });
        let manifest_bytes = serde_json::to_vec_pretty(&manifest).unwrap();
        write_bytes(&catalog_dir.join("manifest.json"), &manifest_bytes).unwrap();
        let manifest_hash = sha256_hex(&manifest_bytes);
        let prepared = prepare_bundle(&catalog_dir, &manifest_hash, &output, &|| false).unwrap();
        assert_eq!(
            prepared,
            validate_bundle(&catalog_dir, &manifest_hash, &output).unwrap()
        );
        assert_eq!(prepared["offer"]["data"]["sha256"], descriptor["sha256"]);
        let offer = output.join("derived-database.ndjson");
        let data = std::fs::read(&offer).unwrap();
        std::fs::remove_file(&offer).unwrap();
        assert!(validate_bundle(&catalog_dir, &manifest_hash, &output).is_err());
        std::fs::write(&offer, b"altered").unwrap();
        assert!(validate_bundle(&catalog_dir, &manifest_hash, &output).is_err());
        std::fs::write(&offer, &data).unwrap();
        let terms = output.join("derived-database-offer.json");
        let original_terms = std::fs::read(&terms).unwrap();
        std::fs::write(&terms, b"changed terms").unwrap();
        assert!(validate_bundle(&catalog_dir, &manifest_hash, &output).is_err());
        std::fs::write(&terms, original_terms).unwrap();
        std::fs::remove_file(output.join("rights.json")).unwrap();
        assert!(validate_bundle(&catalog_dir, &manifest_hash, &output).is_err());
        // Even a rehashed manifest may not admit fields outside the catalog schema.
        let text = String::from_utf8(data).unwrap();
        let changed = text.replacen('{', "{\"unknown_field\":\"synthetic\",", 1);
        std::fs::write(catalog_dir.join("catalog.ndjson"), changed.as_bytes()).unwrap();
        let mut changed_manifest = manifest;
        changed_manifest["files"]["catalog.ndjson"]["sha256"] =
            json!(sha256_hex(changed.as_bytes()));
        changed_manifest["files"]["catalog.ndjson"]["bytes"] = json!(changed.len());
        let changed_bytes = serde_json::to_vec_pretty(&changed_manifest).unwrap();
        std::fs::write(catalog_dir.join("manifest.json"), &changed_bytes).unwrap();
        let refused = root.join("refused");
        assert!(
            prepare_bundle(&catalog_dir, &sha256_hex(&changed_bytes), &refused, &|| {
                false
            })
            .is_err()
        );
        assert!(!refused.exists());
        std::fs::remove_dir_all(root).unwrap();
    }

    #[test]
    fn preserves_extract_and_provider_rights() {
        let catalog = conflate(&[source(true), source(false)]).unwrap().sites;
        let digest = "c".repeat(64);
        let rights = generation_rights(&digest, &catalog).unwrap();
        assert_eq!(rights["catalog_sha256"], digest);
        assert_eq!(rights["osm"]["sha256"], SourcePins::frozen().osm_sha256);
        assert_eq!(rights["osm"]["assertions"], 1);
        assert_eq!(rights["ocm"]["providers"][0]["id"], 40);
        assert_eq!(rights["ocm"]["providers"][0]["attribution"], "Easy Volt");
        assert_eq!(rights["ocm"]["providers"][0]["license"], "CC-0");
        assert_eq!(rights["ocm"]["providers"][0]["assertions"], 1);
        assert_eq!(rights["requires_derived_database_offer"], true);
    }

    #[test]
    fn checks_attached_equipment_and_keeps_provider_licences_separate() {
        let mut yellot = source(false);
        yellot.key = "ocm:poi:102".into();
        yellot.source.external_id = "102".into();
        yellot.source.provider_id = Some(57);
        yellot.source.license = "CC0".into();
        yellot.source.attribution = "YellotMob".into();
        let mut catalog = conflate(&[source(false), yellot]).unwrap().sites;
        let mut equipment = source(true);
        equipment.kind = crate::ev_sites::SiteKind::ChargePoint;
        catalog[0].equipment.push(equipment);
        let digest = "c".repeat(64);
        let rights = generation_rights(&digest, &catalog).unwrap();
        assert_eq!(rights["osm"]["assertions"], 1);
        assert_eq!(rights["ocm"]["providers"][0]["license"], "CC-0");
        assert_eq!(rights["ocm"]["providers"][1]["license"], "CC0");
        catalog[0].equipment[0].source.attribution.clear();
        assert_eq!(
            generation_rights(&digest, &catalog),
            Err("OSM source rights mismatch")
        );
    }

    #[test]
    fn rejects_osm_filename_without_the_extract_digest() {
        let mut site = source(true);
        site.source.revision = SourcePins::frozen().osm_revision;
        let catalog = conflate(&[site]).unwrap().sites;
        assert_eq!(
            generation_rights(&"a".repeat(64), &catalog),
            Err("OSM source rights mismatch")
        );
    }

    #[test]
    fn rejects_missing_changed_and_duplicate_rights() {
        let catalog = conflate(&[source(false)]).unwrap().sites;
        let digest = "c".repeat(64);
        assert!(generation_rights(&digest, &catalog).is_ok());
        for mutate in [
            |s: &mut SourceSite| s.source.provider_id = None,
            |s: &mut SourceSite| s.source.provider_id = Some(999),
            |s: &mut SourceSite| s.source.attribution.clear(),
            |s: &mut SourceSite| s.source.license = "CC BY-SA".into(),
            |s: &mut SourceSite| s.source.revision = "another-edition".into(),
        ] {
            let mut changed = catalog.clone();
            mutate(&mut changed[0].sources[0]);
            assert!(generation_rights(&digest, &changed).is_err());
        }
        let mut duplicate = catalog.clone();
        duplicate.push(catalog[0].clone());
        assert!(generation_rights(&digest, &duplicate).is_err());
        assert!(generation_rights("", &catalog).is_err());
    }
}

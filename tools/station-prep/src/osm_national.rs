//! Frozen national preparation profile and raw object census; no public eligibility claim.
use crate::ev_geography::{StateIndex, STATES};
use crate::osm_reference::{Limits, ReferenceRun};
use crate::types::sha256_hex;
use serde::{Deserialize, Serialize};
use std::collections::BTreeMap;

const PROFILE: &str = include_str!("../../../contracts/ev/osm-national-limits-v1.json");

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct Resources {
    pub max_wall_seconds: u64,
    pub max_cpu_seconds: u64,
    pub max_memory_bytes: u64,
    pub max_output_file_bytes: u64,
    pub max_working_disk_bytes: u64,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct NationalProfile {
    pub format: String,
    pub profile_id: String,
    pub parser_version: String,
    pub exposure: String,
    pub limits: Limits,
    pub resources: Resources,
    pub census: serde_json::Value,
}

impl NationalProfile {
    pub fn frozen() -> Self {
        serde_json::from_str(PROFILE).expect("compiled national profile must be valid")
    }
    pub fn sha256() -> String {
        sha256_hex(PROFILE.as_bytes())
    }
    pub fn manifest(&self) -> serde_json::Value {
        serde_json::json!({"profile_id": self.profile_id, "sha256": Self::sha256(),
            "exposure": self.exposure, "limits": self.limits, "resources": self.resources})
    }
}

#[derive(Debug, Default, Serialize)]
pub struct SiteCounts {
    #[serde(rename = "FUEL_STATION")]
    fuel: u64,
    #[serde(rename = "CHARGING_SITE")]
    charging: u64,
    #[serde(rename = "COMBINED_SITE")]
    combined: u64,
}
impl SiteCounts {
    fn add(&mut self, kind: &str) -> Result<(), String> {
        match kind {
            "FUEL_STATION" => self.fuel += 1,
            "CHARGING_SITE" => self.charging += 1,
            "COMBINED_SITE" => self.combined += 1,
            _ => return Err("unknown site kind in national census".into()),
        }
        Ok(())
    }
}

#[derive(Debug, Serialize)]
pub struct NationalCensus {
    format: &'static str,
    eligibility: &'static str,
    units: &'static str,
    source_sha256: String,
    boundary_sha256: String,
    boundary_license: &'static str,
    boundary_attribution: &'static str,
    pub totals: SiteCounts,
    pub by_uf: BTreeMap<String, SiteCounts>,
    classification_reasons: BTreeMap<String, u64>,
}

pub fn census(run: &ReferenceRun, index: &StateIndex) -> Result<NationalCensus, String> {
    let mut result = NationalCensus {
        format: "abastevo.osm-national-census-v1",
        eligibility: "raw candidates; not an eligible/public catalog or complete coverage",
        units: "OSM objects; three site kinds kept separate; not connectors",
        source_sha256: run.source_sha256.clone(),
        boundary_sha256: index.source_sha256.clone(),
        boundary_license: "CC-BY-4.0",
        boundary_attribution: "Fonte: IBGE",
        totals: SiteCounts::default(),
        by_uf: STATES
            .into_iter()
            .chain(["UNASSIGNED"])
            .map(|uf| (uf.into(), SiteCounts::default()))
            .collect(),
        classification_reasons: BTreeMap::new(),
    };
    for place in &run.places {
        let point = place
            .latitude_deg
            .as_deref()
            .and_then(|v| v.parse::<f64>().ok())
            .zip(
                place
                    .longitude_deg
                    .as_deref()
                    .and_then(|v| v.parse::<f64>().ok()),
            );
        let decision = point.map(|(lat, lon)| index.classify(lat, lon));
        let uf = decision
            .as_ref()
            .and_then(|v| v.uf.as_deref())
            .unwrap_or("UNASSIGNED");
        let reason = decision
            .as_ref()
            .map(|v| v.reason)
            .unwrap_or("missing_geometry");
        *result
            .classification_reasons
            .entry(reason.into())
            .or_default() += 1;
        result
            .by_uf
            .get_mut(uf)
            .ok_or("unknown boundary UF")?
            .add(&place.kind)?;
        result.totals.add(&place.kind)?;
    }
    Ok(result)
}

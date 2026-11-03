//! Pure source-backed address selection. No coordinates, network, files or DB.
use crate::cnpj::normalize_cnpj;
use crate::location_review::normalize_street;
use crate::municipality::normalize_name;
use crate::registry::normalize_calendar_date;
use serde::{Deserialize, Serialize};
use std::collections::BTreeMap;

#[derive(Debug, Clone, Copy, PartialEq, Eq, PartialOrd, Ord, Serialize, Deserialize)]
#[serde(rename_all = "lowercase")]
pub enum AddressSource {
    Anp,
    Rfb,
    Osm,
    Cnefe,
    Municipality,
}
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct Provenance {
    pub reference: String,
    pub edition: String,
    pub input_sha256: String,
    pub observed_at: String,
}
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum ReuseStatus {
    Admitted,
    Restricted,
    Pending,
}
/// The admission port supplies this decision; a download approval is not one.
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct ReuseDecision {
    pub status: ReuseStatus,
    pub reference: String,
}
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
#[serde(tag = "kind", rename_all = "snake_case", deny_unknown_fields)]
pub enum MatchProof {
    Rfb {
        cnpj: String,
        active: bool,
    },
    Osm {
        cnpj: String,
        object_type: String,
        object_id: i64,
        link_reference: String,
    },
    Cnefe {
        cnpj: String,
        municipality_ibge: String,
        street: String,
        number: String,
        species: u8,
        geocode_level: u8,
        match_reference: String,
    },
}
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct AddressCandidate {
    pub source: AddressSource,
    pub components: BTreeMap<String, String>,
    pub provenance: Provenance,
    pub proof: MatchProof,
    pub reuse: ReuseDecision,
}
#[derive(Debug, Clone)]
pub struct StationAddressInput {
    pub cnpj: String,
    pub municipality_ibge: String,
    pub municipality: String,
    pub uf: String,
    pub official_address: BTreeMap<String, String>,
}
#[derive(Debug, Clone, PartialEq, Eq, PartialOrd, Ord, Serialize)]
pub struct Review {
    pub source: AddressSource,
    pub reference: String,
    pub reason: String,
}
#[derive(Debug, Clone, PartialEq, Eq, Serialize)]
pub struct Selection {
    pub source: Option<AddressSource>,
    pub components: BTreeMap<String, String>,
    pub provenance: Option<Provenance>,
    pub reviews: Vec<Review>,
}
const COMPONENTS: &[&str] = &[
    "street",
    "number",
    "complement",
    "district",
    "postal_code",
    "municipio",
    "municipio_ibge",
    "uf",
    "raw",
];
const STATES: &str =
    " AC AL AM AP BA CE DF ES GO MA MG MS MT PA PB PE PI PR RJ RN RO RR RS SC SE SP TO ";
fn bounded_text(v: &str, cap: usize) -> bool {
    !v.trim().is_empty() && v.len() <= cap && !v.chars().any(char::is_control)
}
fn readable(v: &str) -> bool {
    bounded_text(v, 2048)
        && v.chars().any(char::is_alphabetic)
        && !matches!(
            normalize_name(v).as_str(),
            "NAO INFORMADO" | "SEM ENDERECO" | "NAO POSSUI" | "N/A"
        )
}
fn address_valid(address: &BTreeMap<String, String>) -> bool {
    address.len() <= COMPONENTS.len()
        && address.iter().all(|(k, v)| {
            COMPONENTS.contains(&k.as_str()) && v.len() <= 2048 && !v.chars().any(char::is_control)
        })
        && ["street", "raw"]
            .iter()
            .any(|k| address.get(*k).is_some_and(|v| readable(v)))
}
fn provenance_valid(p: &Provenance) -> bool {
    bounded_text(&p.reference, 2048)
        && bounded_text(&p.edition, 256)
        && p.input_sha256.len() == 64
        && p.input_sha256.bytes().all(|b| b.is_ascii_hexdigit())
        && normalize_calendar_date(&p.observed_at).is_ok_and(|v| v == p.observed_at)
}
fn proof_reason(s: &StationAddressInput, c: &AddressCandidate) -> Option<&'static str> {
    let bound = match (&c.source, &c.proof) {
        (AddressSource::Rfb, MatchProof::Rfb { cnpj, .. }) => cnpj,
        (
            AddressSource::Osm,
            MatchProof::Osm {
                cnpj,
                object_type,
                object_id,
                link_reference,
            },
        ) => {
            if !["node", "way", "relation"].contains(&object_type.as_str())
                || *object_id <= 0
                || !bounded_text(link_reference, 2048)
            {
                return Some("osm_match_proof_missing");
            }
            cnpj
        }
        (
            AddressSource::Cnefe,
            MatchProof::Cnefe {
                cnpj,
                municipality_ibge,
                street,
                number,
                species,
                geocode_level,
                match_reference,
            },
        ) => {
            // A prior exact match is bound to this station; it is not inferred from proximity.
            let current_street = c.components.get("street").map(String::as_str).unwrap_or("");
            let current_number = c.components.get("number").map(String::as_str).unwrap_or("");
            if municipality_ibge != &s.municipality_ibge
                || municipality_ibge.len() != 7
                || !municipality_ibge.bytes().all(|b| b.is_ascii_digit())
                || !bounded_text(match_reference, 2048)
                || *species != 6
                || !(1..=3).contains(geocode_level)
                || !readable(street)
                || normalize_street(street) != normalize_street(current_street)
                || number.trim().is_empty()
                || !number.trim().chars().any(|c| c.is_ascii_digit())
                || normalize_name(number) != normalize_name(current_number)
            {
                return Some("cnefe_exact_establishment_proof_missing");
            }
            cnpj
        }
        _ => return Some("source_proof_mismatch"),
    };
    if normalize_cnpj(bound).is_err() || bound != &s.cnpj {
        Some("station_identity_mismatch")
    } else {
        None
    }
}
fn eligibility(c: &AddressCandidate) -> Option<&'static str> {
    if c.reuse.status != ReuseStatus::Admitted || !bounded_text(&c.reuse.reference, 2048) {
        return Some("source_reuse_not_admitted");
    }
    if matches!(c.proof, MatchProof::Rfb { active: false, .. }) {
        return Some("rfb_inactive");
    }
    if !address_valid(&c.components) {
        return Some("unusable_address");
    }
    None
}
fn review(c: &AddressCandidate, reason: &str) -> Review {
    Review {
        source: c.source,
        reference: c.provenance.reference.clone(),
        reason: reason.into(),
    }
}
/// Selects one unchanged source object. The file adapter owns proof/rights
/// admission against pinned inputs; this domain never infers those from GPS.
pub fn select_address(
    s: &StationAddressInput,
    candidates: &[AddressCandidate],
) -> Result<Selection, &'static str> {
    if candidates.len() > 256 {
        return Err("address candidate budget exceeded");
    }
    if !normalize_cnpj(&s.cnpj).is_ok_and(|v| v == s.cnpj) {
        return Err("invalid station identity");
    }
    let mut result = Selection {
        source: None,
        components: BTreeMap::new(),
        provenance: None,
        reviews: Vec::new(),
    };
    if address_valid(&s.official_address) {
        result.source = Some(AddressSource::Anp);
        result.components = s.official_address.clone();
        return Ok(result);
    }
    for source in [AddressSource::Rfb, AddressSource::Osm, AddressSource::Cnefe] {
        let mut valid = Vec::new();
        let mut uncertain_clock = false;
        for c in candidates.iter().filter(|c| c.source == source) {
            let reason = if !provenance_valid(&c.provenance) {
                uncertain_clock = true;
                Some("invalid_address_provenance")
            } else {
                proof_reason(s, c)
            };
            if let Some(reason) = reason {
                result.reviews.push(review(c, reason));
            } else {
                valid.push(c);
            }
        }
        // Without a trustworthy clock/pin we cannot prove which observation is
        // latest. In particular, do not revive an older active RFB address.
        if uncertain_clock {
            continue;
        }
        if let Some(newest) = valid
            .iter()
            .map(|c| c.provenance.observed_at.as_str())
            .max()
        {
            let latest: Vec<_> = valid
                .iter()
                .copied()
                .filter(|c| c.provenance.observed_at == newest)
                .collect();
            let bad: Vec<_> = latest
                .iter()
                .filter_map(|c| eligibility(c).map(|r| review(c, r)))
                .collect();
            if !bad.is_empty() {
                result.reviews.extend(bad);
                continue;
            }
            if latest.iter().any(|c| *c != latest[0]) {
                result
                    .reviews
                    .extend(latest.iter().map(|c| review(c, "ambiguous_latest_address")));
                continue;
            }
            result.source = Some(source);
            result.components = latest[0].components.clone();
            result.provenance = Some(latest[0].provenance.clone());
            break;
        }
    }
    if result.source.is_none()
        && readable(&s.municipality)
        && s.uf.len() == 2
        && STATES.contains(&format!(" {} ", s.uf.to_uppercase()))
    {
        result.source = Some(AddressSource::Municipality);
        result.components = BTreeMap::from([
            ("municipio".into(), s.municipality.clone()),
            ("uf".into(), s.uf.clone()),
        ]);
    }
    result.reviews.sort();
    result.reviews.dedup();
    Ok(result)
}

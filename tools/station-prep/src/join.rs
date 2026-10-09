//! PMQC-to-registry join: anchor every pointed candidate at its exact
//! establishment CNPJ, then triage chronology, staleness and spatial
//! conflict. Rust emits joined candidates only; `review_state` stays
//! `pending` on every row and nothing here promotes a point to
//! `reviewed` or feeds the 150m capture rule.
//!
//! Pointless candidates (unknown location) skip the join: there is no
//! geometry to anchor. Unmatched CNPJs stay candidates with
//! `registry_missing`; missing rows never imply closure.

use crate::pmqc::PmqcCandidate;
use crate::registry::RegistryRow;
use serde::Serialize;
use std::collections::HashMap;

/// Same-establishment points farther apart than this stay
/// `conflicting_point` for review; the join never picks a winner.
pub const CONFLICT_RADIUS_M: f64 = 1000.0;
/// Observations older than this many days before the newest registry
/// evidence stay `stale_observation` (triage flag, not a verdict).
pub const STALE_AFTER_DAYS: i64 = 1095;
/// Coarse-precision triage: fewer decimals than this on either axis,
/// counted on the shortest round-trip form, so genuinely round
/// coordinates (city-centroid grade) stand out. Heuristic flag only.
pub const COARSE_DECIMALS: u32 = 3;

/// Great-circle distance in metres (spherical earth, R = 6 371 000 m).
/// Adequate for kilometre-scale conflict triage, not a survey tool.
pub fn haversine_m(latitude_a: f64, longitude_a: f64, latitude_b: f64, longitude_b: f64) -> f64 {
    const RADIUS_M: f64 = 6_371_000.0;
    let lat_a = latitude_a.to_radians();
    let lat_b = latitude_b.to_radians();
    let delta_lat = (latitude_b - latitude_a).to_radians();
    let delta_lon = (longitude_b - longitude_a).to_radians();
    let half = (delta_lat / 2.0).sin().powi(2)
        + lat_a.cos() * lat_b.cos() * (delta_lon / 2.0).sin().powi(2);
    2.0 * RADIUS_M * half.sqrt().asin()
}

/// Days since the Unix epoch for `YYYY-MM-DD`; `None` when unparsable.
/// Unparsable dates skip date-based states without failing the join.
fn days_since_epoch(date: &str) -> Option<i64> {
    let (year, rest) = date.split_once('-')?;
    let (month, day) = rest.split_once('-')?;
    if year.len() != 4 || month.len() != 2 || day.len() != 2 {
        return None;
    }
    let (year, month, day): (i64, i64, i64) =
        (year.parse().ok()?, month.parse().ok()?, day.parse().ok()?);
    if !(1..=12).contains(&month) || !(1..=31).contains(&day) {
        return None;
    }
    let shifted = if month <= 2 { year - 1 } else { year };
    let era = if shifted >= 0 { shifted } else { shifted - 399 } / 400;
    let year_of_era = shifted - era * 400;
    let month_prime = (month + 9) % 12;
    let day_of_year = (153 * month_prime + 2) / 5 + day - 1;
    let day_of_era = year_of_era * 365 + year_of_era / 4 - year_of_era / 100 + day_of_year;
    Some(era * 146097 + day_of_era - 719468)
}

fn decimals(value: f64) -> u32 {
    match format!("{value}").split_once('.') {
        Some((_, fraction)) => fraction.len() as u32,
        None => 0,
    }
}

/// Join verdict for one pointed candidate.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize)]
#[serde(rename_all = "snake_case")]
pub enum MatchState {
    Current,
    PredatesEvidence,
    StaleObservation,
    ConflictingPoint,
    RegistryMissing,
}

/// Registry anchor of a joined candidate: the newest publication for the
/// exact establishment CNPJ.
#[derive(Debug, Clone, PartialEq, Eq, Serialize)]
pub struct RegistryAnchor {
    pub simp_ref: String,
    pub municipality_ibge: String,
    pub published_at: String,
    pub effective_at: String,
}

/// One joined candidate (`station-joined-candidate-v1`).
#[derive(Debug, Clone, Serialize)]
pub struct JoinedCandidate {
    pub schema_version: &'static str,
    pub source: &'static str,
    pub source_key: String,
    pub sample_ref: String,
    pub observed_at: String,
    pub latitude: f64,
    pub longitude: f64,
    pub original_crs: String,
    pub accuracy_m: Option<f64>,
    pub evidence_ref: String,
    pub review_state: String,
    pub anchor: Option<RegistryAnchor>,
    pub match_state: MatchState,
    pub coarse_precision: bool,
    pub group_size: usize,
    pub max_group_distance_m: Option<f64>,
    pub note: String,
}

impl JoinedCandidate {
    pub fn to_jsonl(&self) -> String {
        serde_json::to_string(self).expect("joined candidate must serialize")
    }
}

/// Row accounting over pointed join inputs.
#[derive(Debug, Clone, Copy, Default, PartialEq, Eq)]
pub struct JoinedCounts {
    pub input: usize,
    pub current: usize,
    pub predates: usize,
    pub stale: usize,
    pub conflicting: usize,
    pub missing: usize,
}

/// Joined batch in stable `(source_key, observed_at, sample_ref)` order.
#[derive(Debug)]
pub struct JoinedBatch {
    pub candidates: Vec<JoinedCandidate>,
    pub counts: JoinedCounts,
}

/// Anchor pointed PMQC candidates at exact registry CNPJs and triage
/// them. Candidates without a point skip the join; every emitted row
/// keeps `review_state: pending`.
pub fn join_candidates(registry: &[RegistryRow], candidates: &[PmqcCandidate]) -> JoinedBatch {
    let mut anchors: HashMap<&str, &RegistryRow> = HashMap::new();
    for row in registry {
        anchors
            .entry(row.source_key.as_str())
            .and_modify(|anchor| {
                if (row.published_at.as_str(), row.checksum.as_str())
                    > (anchor.published_at.as_str(), anchor.checksum.as_str())
                {
                    *anchor = row;
                }
            })
            .or_insert(row);
    }
    let reference_days = registry
        .iter()
        .filter_map(|row| days_since_epoch(&row.effective_at))
        .max();

    let pointed: Vec<&PmqcCandidate> = candidates
        .iter()
        .filter(|candidate| candidate.latitude.is_some() && candidate.longitude.is_some())
        .collect();
    let mut groups: HashMap<&str, Vec<&PmqcCandidate>> = HashMap::new();
    for candidate in &pointed {
        groups
            .entry(candidate.source_key.as_str())
            .or_default()
            .push(candidate);
    }
    // Max pairwise distance per establishment group.
    let mut diameters: HashMap<&str, f64> = HashMap::new();
    for (key, group) in &groups {
        let mut diameter: f64 = 0.0;
        for pair in group.windows(2) {
            let (a, b) = (pair[0], pair[1]);
            diameter = diameter.max(haversine_m(
                a.latitude.expect("pointed"),
                a.longitude.expect("pointed"),
                b.latitude.expect("pointed"),
                b.longitude.expect("pointed"),
            ));
        }
        // `windows(2)` misses non-adjacent pairs past size 2; close the
        // loop explicitly so trios cannot hide a conflict.
        if group.len() > 2 {
            for left in group {
                for right in group {
                    diameter = diameter.max(haversine_m(
                        left.latitude.expect("pointed"),
                        left.longitude.expect("pointed"),
                        right.latitude.expect("pointed"),
                        right.longitude.expect("pointed"),
                    ));
                }
            }
        }
        diameters.insert(key, diameter);
    }

    let mut batch = JoinedBatch {
        candidates: Vec::with_capacity(pointed.len()),
        counts: JoinedCounts {
            input: pointed.len(),
            ..JoinedCounts::default()
        },
    };
    for candidate in pointed {
        let latitude = candidate.latitude.expect("pointed");
        let longitude = candidate.longitude.expect("pointed");
        let group = &groups[candidate.source_key.as_str()];
        let diameter = diameters[candidate.source_key.as_str()];
        let observed_days = days_since_epoch(&candidate.observed_at);
        let anchor = anchors.get(candidate.source_key.as_str()).copied();

        let (state, note) = if group.len() >= 2 && diameter > CONFLICT_RADIUS_M {
            (
                MatchState::ConflictingPoint,
                format!(
                    "group of {} points spans {diameter:.0} m; review owns the site",
                    group.len()
                ),
            )
        } else if let (Some(observed), Some(reference)) = (observed_days, reference_days) {
            if observed < reference - STALE_AFTER_DAYS {
                (
                    MatchState::StaleObservation,
                    "observation predates newest registry evidence by over 3 years".to_string(),
                )
            } else {
                anchor_state(candidate, anchor)
            }
        } else {
            anchor_state(candidate, anchor)
        };
        match state {
            MatchState::Current => batch.counts.current += 1,
            MatchState::PredatesEvidence => batch.counts.predates += 1,
            MatchState::StaleObservation => batch.counts.stale += 1,
            MatchState::ConflictingPoint => batch.counts.conflicting += 1,
            MatchState::RegistryMissing => batch.counts.missing += 1,
        }
        batch.candidates.push(JoinedCandidate {
            schema_version: "station-joined-candidate-v1",
            source: crate::pmqc::PMQC_SOURCE,
            source_key: candidate.source_key.clone(),
            sample_ref: candidate.sample_ref.clone(),
            observed_at: candidate.observed_at.clone(),
            latitude,
            longitude,
            original_crs: candidate.original_crs.clone(),
            accuracy_m: None,
            evidence_ref: candidate.evidence_ref.clone(),
            review_state: "pending".to_string(),
            anchor: anchor.map(|row| RegistryAnchor {
                simp_ref: row.simp_ref.clone(),
                municipality_ibge: row.municipality_ibge.clone(),
                published_at: row.published_at.clone(),
                effective_at: row.effective_at.clone(),
            }),
            match_state: state,
            coarse_precision: decimals(latitude).min(decimals(longitude)) < COARSE_DECIMALS,
            group_size: group.len(),
            max_group_distance_m: (group.len() >= 2).then_some(diameter),
            note,
        });
    }
    batch.candidates.sort_by(|a, b| {
        a.source_key
            .cmp(&b.source_key)
            .then_with(|| a.observed_at.cmp(&b.observed_at))
            .then_with(|| a.sample_ref.cmp(&b.sample_ref))
    });
    batch
}

/// Anchor verdict once group conflict and staleness are excluded.
fn anchor_state(candidate: &PmqcCandidate, anchor: Option<&RegistryRow>) -> (MatchState, String) {
    match anchor {
        None => (
            MatchState::RegistryMissing,
            "sampled CNPJ has no registry assertion; missing rows never imply closure".to_string(),
        ),
        Some(row) => match (
            days_since_epoch(&candidate.observed_at),
            days_since_epoch(&row.published_at),
        ) {
            (Some(observed), Some(published)) if observed < published => (
                MatchState::PredatesEvidence,
                format!(
                    "observation {} predates authorization record {}; site may have changed",
                    candidate.observed_at, row.published_at
                ),
            ),
            _ => (
                MatchState::Current,
                "exact-CNPJ anchor with compatible chronology".to_string(),
            ),
        },
    }
}

#[cfg(test)]
mod tests {
    use super::{days_since_epoch, decimals, haversine_m};

    #[test]
    fn date_math_matches_known_days() {
        assert_eq!(days_since_epoch("1970-01-01"), Some(0));
        assert_eq!(days_since_epoch("2024-03-18"), Some(19800));
        assert_eq!(days_since_epoch("not-a-date"), None);
        assert_eq!(days_since_epoch("2024-13-01"), None);
    }

    #[test]
    fn decimals_count_shortest_round_trip_form() {
        assert_eq!(decimals(-23.55), 2);
        assert_eq!(decimals(-46.656), 3);
        assert_eq!(decimals(0.0), 0);
    }

    #[test]
    fn haversine_stays_in_conflict_scale() {
        let meters = haversine_m(-23.561, -46.656, -23.400, -46.500);
        assert!(
            (meters - 23950.5).abs() < 1.0,
            "unexpected conflict distance {meters}"
        );
        assert!(haversine_m(-23.561, -46.656, -23.561, -46.656) < 0.01);
    }
}

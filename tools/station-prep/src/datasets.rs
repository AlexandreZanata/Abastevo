//! RST-11 representative datasets and geographic request distributions.
//!
//! Deterministic synthetic fixtures for the RST-10–21 performance
//! campaign. Every byte derives from a pinned seed: the same profile
//! always yields identical CSV bytes, oracle counts and request
//! traces. Generated establishments carry `[RST11-TEST]` markers and
//! synthetic `355xxxx` IBGE codes; sidecar locations are labelled
//! `synthetic` and never reviewed. No national fetch, no database and
//! no network.
//!
//! Profiles: `tiny` (committed correctness set), `representative`,
//! `stress-100k` and `stress-1M` (same skewed shape as the RST-07
//! pilot, seed 7). Knobs vary independently: duplicate/conflict
//! ratios, quarantine injection, long fields, missing sidecar
//! locations and changed/no-op delta editions. History cardinalities
//! 1/10/100 per station are oracle counts with an explicit
//! unique-station denominator for later DB seeding.

use crate::types::sha256_hex;
use std::collections::{BTreeMap, HashSet};

/// Version stamp for every dataset artifact of this module.
pub const DATASETS_VERSION: &str = "station-datasets-v1";
/// Populated synthetic cities (`CIDADE 000..199`, IBGE `3550000+i`).
pub const UNIVERSE_CITIES: usize = 200;
/// Zero-row cities (`VAZIA 000..004`, IBGE `3559900+i`).
pub const EMPTY_CITIES: usize = 5;
/// Frozen raw header, identical to the registry reader contract.
const HEADER: &str = "CODIGOISIMP;AUTORIZACAO;DATAPUBLICACAO;RAZAOSOCIAL;CNPJ;ENDERECO;COMPLEMENTO;BAIRRO;CEP;UF;MUNICIPIO;BANDEIRA;DATAVINCULACAO";

/// Deterministic generator profile. All percentages are 0–100.
#[derive(Debug, Clone, Copy)]
pub struct DatasetProfile {
    pub id: &'static str,
    pub rows: usize,
    pub seed: u64,
    /// Populated city count (<= [`UNIVERSE_CITIES`]).
    pub cities: usize,
    pub duplicate_pct: u64,
    pub conflict_pct: u64,
    /// Inject one poisoned row every N data rows (0 disables).
    pub quarantine_every: usize,
    /// Widen razao/endereco every N data rows (0 disables).
    pub long_field_every: usize,
    /// Sidecar location missing every N accepted stations (0 disables).
    pub missing_location_every: usize,
    /// Starting establishment serial (0 default). Spacing offsets beyond
    /// rows yields disjoint identity sets across editions.
    pub serial_offset: u64,
}

/// Committed correctness set: 30 rows over 5 cities with at least one
/// duplicate, one invalid-CNPJ quarantine, one unknown-city
/// quarantine and one older-dated conflict row.
pub fn tiny_profile() -> DatasetProfile {
    DatasetProfile {
        id: "tiny",
        rows: 30,
        seed: 11,
        cities: 5,
        duplicate_pct: 7,
        conflict_pct: 4,
        quarantine_every: 9,
        long_field_every: 15,
        missing_location_every: 7,
        serial_offset: 0,
    }
}

/// Representative lab set: 20k skewed rows, same shape family as RST-07.
pub fn representative_profile() -> DatasetProfile {
    DatasetProfile {
        id: "representative",
        rows: 20_000,
        seed: 7,
        cities: UNIVERSE_CITIES,
        duplicate_pct: 2,
        conflict_pct: 1,
        quarantine_every: 1000,
        long_field_every: 5000,
        missing_location_every: 20,
        serial_offset: 0,
    }
}

/// 100k stress set, same skewed shape as the RST-07 pilot.
pub fn stress_100k_profile() -> DatasetProfile {
    DatasetProfile {
        id: "stress-100k",
        rows: 100_000,
        seed: 7,
        cities: UNIVERSE_CITIES,
        duplicate_pct: 2,
        conflict_pct: 1,
        quarantine_every: 0,
        long_field_every: 5000,
        missing_location_every: 20,
        serial_offset: 0,
    }
}

/// 1M stress set (not a claim about real station counts). Generate to
/// scratch through `bench_datasets`; never commit the output.
pub fn stress_1m_profile() -> DatasetProfile {
    DatasetProfile {
        id: "stress-1M",
        rows: 1_000_000,
        seed: 7,
        cities: UNIVERSE_CITIES,
        duplicate_pct: 2,
        conflict_pct: 1,
        quarantine_every: 0,
        long_field_every: 5000,
        missing_location_every: 20,
        serial_offset: 0,
    }
}

/// Independent correctness oracle for one generated dataset.
#[derive(Debug, Clone)]
pub struct DatasetOracle {
    pub profile_id: &'static str,
    pub seed: u64,
    pub input: usize,
    pub accepted: usize,
    pub duplicates: usize,
    pub quarantined: usize,
    pub unique_cnpjs: usize,
    /// Accepted rows per IBGE code (empty cities absent).
    pub per_city: BTreeMap<String, usize>,
    /// Multiset checksum over sorted accepted CNPJs (bench_parse recipe).
    pub checksum: String,
    pub csv_sha256: String,
    pub csv_bytes: usize,
    pub history_per_1: usize,
    pub history_per_10: usize,
    pub history_per_100: usize,
    pub with_location: usize,
    pub without_location: usize,
    pub long_field_rows: usize,
}

/// One synthetic sidecar location. `synthetic` is always true and
/// there is no reviewed flag by construction: generated points never
/// become reviewed evidence.
#[derive(Debug, Clone)]
pub struct SyntheticLocation {
    pub cnpj: String,
    pub ibge: String,
    pub latitude: Option<f64>,
    pub longitude: Option<f64>,
    pub synthetic: bool,
}

/// Generated bytes plus oracle, alias table and sidecar locations.
#[derive(Debug, Clone)]
pub struct GeneratedDataset {
    pub csv: Vec<u8>,
    pub oracle: DatasetOracle,
    pub aliases_json: String,
    pub locations: Vec<SyntheticLocation>,
}

/// Deterministic xorshift64* (same family as the RST-07 generator).
struct Rng(u64);

impl Rng {
    fn next(&mut self) -> u64 {
        let mut x = self.0;
        x ^= x >> 12;
        x ^= x << 25;
        x ^= x >> 27;
        self.0 = x;
        x.wrapping_mul(0x2545F4914F6CDD1D)
    }

    fn below(&mut self, n: u64) -> u64 {
        self.next() % n
    }
}

fn digit_value(byte: u8) -> u32 {
    if byte <= b'9' {
        (byte - b'0') as u32
    } else {
        (byte - b'A' + 17) as u32
    }
}

/// Valid-checksum CNPJ from a 12-digit prefix (leading zeroes kept).
fn complete_cnpj(prefix: &[u8; 12]) -> String {
    const W1: [u32; 12] = [5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2];
    const W2: [u32; 13] = [6, 5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2];
    let sum: u32 = prefix
        .iter()
        .zip(W1)
        .map(|(b, w)| digit_value(*b) * w)
        .sum();
    let first = if sum % 11 >= 2 { 11 - sum % 11 } else { 0 };
    let sum: u32 = prefix
        .iter()
        .zip(W2)
        .map(|(b, w)| digit_value(*b) * w)
        .sum::<u32>()
        + first * W2[12];
    let second = if sum % 11 >= 2 { 11 - sum % 11 } else { 0 };
    format!(
        "{}{first}{second}",
        prefix.iter().map(|b| *b as char).collect::<String>()
    )
}

fn prefix_for(serial: u64) -> [u8; 12] {
    let mut prefix = [b'0'; 12];
    let mut base = serial;
    for slot in prefix.iter_mut() {
        *slot = b'0' + (base % 10) as u8;
        base /= 10;
    }
    prefix
}

/// IBGE code for a populated city index.
pub fn ibge_for(city: usize) -> String {
    (3_550_000 + city).to_string()
}

/// IBGE code for a zero-row city index.
pub fn empty_ibge_for(index: usize) -> String {
    (3_559_900 + index).to_string()
}

/// Display name for a populated city index.
pub fn city_name_for(city: usize) -> String {
    format!("CIDADE {city:03}")
}

/// Alias table covering `cities` populated cities plus all zero-row
/// cities. Minimal `{"entries": [...]}` shape accepted by the reader.
pub fn alias_table_json(cities: usize) -> String {
    let mut out = String::from("{\"entries\": [");
    let mut first = true;
    for city in 0..cities {
        if !first {
            out.push(',');
        }
        first = false;
        out.push_str(&format!(
            "{{\"uf\": \"SP\", \"ibge\": \"{}\", \"aliases\": [\"{}\"]}}",
            ibge_for(city),
            city_name_for(city)
        ));
    }
    for empty in 0..EMPTY_CITIES {
        out.push(',');
        out.push_str(&format!(
            "{{\"uf\": \"SP\", \"ibge\": \"{}\", \"aliases\": [\"VAZIA {empty:03}\"]}}",
            empty_ibge_for(empty)
        ));
    }
    out.push_str("]}");
    out
}

fn multiset_checksum(keys: &[String]) -> String {
    let mut sorted = keys.to_vec();
    sorted.sort();
    let mut joined = String::new();
    for key in &sorted {
        joined.push_str(key);
        joined.push('\n');
    }
    sha256_hex(joined.as_bytes())
}

/// Generate a deterministic dataset and its oracle.
pub fn generate(profile: &DatasetProfile) -> GeneratedDataset {
    let mut rng = Rng(profile.seed.max(1));
    let mut text = String::with_capacity(profile.rows * 128 + 256);
    text.push_str(HEADER);
    text.push('\n');
    let mut emitted: Vec<String> = Vec::new();
    let mut accepted_keys: Vec<String> = Vec::new();
    let mut accepted_ibge: Vec<String> = Vec::new();
    let mut long_field_rows = 0usize;
    let mut duplicates = 0usize;
    let mut quarantined = 0usize;
    let mut quarantine_injections = 0usize;
    let mut serial: u64 = profile.serial_offset;
    // Round-robin cities stay even for tiny correctness sets; larger
    // universes reuse the RST-07 Zipf-ish skew as aggregate shape.
    let skewed = profile.cities > 8;

    for index in 0..profile.rows {
        let pick = rng.below(100);
        if pick < profile.duplicate_pct && !emitted.is_empty() {
            let line = emitted[(rng.next() % emitted.len() as u64) as usize].clone();
            text.push_str(&line);
            text.push('\n');
            duplicates += 1;
            continue;
        }
        serial += 1;
        let city = if skewed {
            (profile.cities as u64 / (1 + rng.below(profile.cities as u64)))
                .min(profile.cities as u64 - 1) as usize
        } else {
            ((serial - 1) % profile.cities as u64) as usize
        };
        let conflict = profile.conflict_pct > 0 && pick >= 100 - profile.conflict_pct.clamp(1, 99);
        let (published, effective, address) = if conflict {
            (
                "2023-01-01".to_string(),
                "2023-01-10".to_string(),
                format!("AV ANTIGA {}", serial % 900 + 100),
            )
        } else {
            (
                "2024-03-01".to_string(),
                "2024-03-10".to_string(),
                format!("RUA GERADA {}", serial % 900 + 100),
            )
        };
        let poisoned =
            profile.quarantine_every > 0 && index.is_multiple_of(profile.quarantine_every);
        let long_field =
            profile.long_field_every > 0 && index.is_multiple_of(profile.long_field_every);
        let (cnpj, municipio) = if poisoned {
            quarantine_injections += 1;
            if quarantine_injections % 2 == 1 {
                ("00INVALID000000".to_string(), city_name_for(city))
            } else {
                (complete_cnpj(&prefix_for(serial)), "NOWHERE XV".to_string())
            }
        } else {
            (complete_cnpj(&prefix_for(serial)), city_name_for(city))
        };
        let razao = if long_field {
            format!("[RST11-TEST] POSTO {serial:08} LTDA {}", "X".repeat(160))
        } else {
            format!("[RST11-TEST] POSTO {serial:08} LTDA")
        };
        let endereco = if long_field {
            format!("{address} {}", "Y".repeat(96))
        } else {
            address
        };
        if long_field && !poisoned {
            long_field_rows += 1;
        }
        let line = format!(
            "SIMP-{serial:08};PRC-2024-{serial:06};{published};{razao};{cnpj};{endereco};;CENTRO;01310900;SP;{municipio};BRANCA;{effective}"
        );
        text.push_str(&line);
        text.push('\n');
        emitted.push(line);
        if emitted.len() > 4096 {
            emitted.remove(0);
        }
        if poisoned {
            quarantined += 1;
        } else {
            accepted_keys.push(cnpj);
            accepted_ibge.push(ibge_for(city));
        }
    }

    let accepted = accepted_keys.len();
    let mut per_city: BTreeMap<String, usize> = BTreeMap::new();
    for ibge in &accepted_ibge {
        *per_city.entry(ibge.clone()).or_default() += 1;
    }
    let unique: HashSet<&String> = accepted_keys.iter().collect();
    let unique_cnpjs = unique.len();
    let checksum = multiset_checksum(&accepted_keys);
    let csv = text.into_bytes();
    let csv_sha256 = sha256_hex(&csv);
    let csv_bytes = csv.len();

    // Deterministic sidecar locations for accepted stations in
    // first-seen order; every Nth station is missing its point.
    let mut locations = Vec::with_capacity(unique_cnpjs);
    let mut seen: HashSet<String> = HashSet::new();
    let mut station_index = 0usize;
    for (key, ibge) in accepted_keys.iter().zip(accepted_ibge.iter()) {
        if !seen.insert(key.clone()) {
            continue;
        }
        let missing = profile.missing_location_every > 0
            && station_index.is_multiple_of(profile.missing_location_every);
        station_index += 1;
        let (latitude, longitude) = if missing {
            (None, None)
        } else {
            let lat = -23.55 + (rng.below(300) as f64 - 150.0) / 1000.0;
            let lon = -46.63 + (rng.below(500) as f64 - 250.0) / 1000.0;
            (Some(lat), Some(lon))
        };
        locations.push(SyntheticLocation {
            cnpj: key.clone(),
            ibge: ibge.clone(),
            latitude,
            longitude,
            synthetic: true,
        });
    }
    let without_location = locations
        .iter()
        .filter(|location| location.latitude.is_none())
        .count();

    GeneratedDataset {
        csv,
        oracle: DatasetOracle {
            profile_id: profile.id,
            seed: profile.seed,
            input: profile.rows,
            accepted,
            duplicates,
            quarantined,
            unique_cnpjs,
            per_city,
            checksum,
            csv_sha256,
            csv_bytes,
            history_per_1: unique_cnpjs,
            history_per_10: unique_cnpjs * 10,
            history_per_100: unique_cnpjs * 100,
            with_location: unique_cnpjs - without_location,
            without_location,
            long_field_rows,
        },
        aliases_json: alias_table_json(profile.cities),
        locations,
    }
}

/// Cardinality stratum for one city station count.
pub fn stratum(stations: usize) -> &'static str {
    if stations == 0 {
        "empty"
    } else if stations <= 10 {
        "tiny-1-10"
    } else if stations <= 100 {
        "small-11-100"
    } else {
        "large-over-100"
    }
}

/// Frozen request-trace distributions over the dataset city universe.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum TraceDistribution {
    /// Equal weight to every city, including zero-row cities.
    Uniform,
    /// Weight proportional to fixture station count (hypothesis, not
    /// user traffic); zero-row cities keep explicit zero weight.
    Proportional,
    /// 80% of requests to the top 20% cities by station count.
    Skew80_20,
    /// 90% to one hot city (rotatable), 10% uniform over the rest.
    Hot90,
}

impl TraceDistribution {
    pub fn id(self) -> &'static str {
        match self {
            TraceDistribution::Uniform => "uniform",
            TraceDistribution::Proportional => "proportional",
            TraceDistribution::Skew80_20 => "skew-80-20",
            TraceDistribution::Hot90 => "hot-90",
        }
    }
}

/// One city entry of a frozen request trace.
#[derive(Debug, Clone)]
pub struct CityWeight {
    pub ibge: String,
    pub name: String,
    pub stations: usize,
    pub stratum: &'static str,
    pub weight: f64,
    pub requests: usize,
}

/// Per-stratum city/request rollup of a trace.
#[derive(Debug, Clone, Default)]
pub struct StrataSummary {
    pub empty_cities: usize,
    pub empty_requests: usize,
    pub tiny_cities: usize,
    pub tiny_requests: usize,
    pub small_cities: usize,
    pub small_requests: usize,
    pub large_cities: usize,
    pub large_requests: usize,
}

/// Deterministic request trace over the dataset universe.
#[derive(Debug, Clone)]
pub struct RequestTrace {
    pub workload_id: String,
    pub profile_id: &'static str,
    pub distribution: &'static str,
    pub total_requests: usize,
    pub seed: u64,
    pub hot_ibge: Option<String>,
    pub cities: Vec<CityWeight>,
    pub strata: StrataSummary,
}

/// Build a deterministic trace with largest-remainder apportionment so
/// requests always sum to exactly `total_requests`.
pub fn request_trace(
    oracle: &DatasetOracle,
    cities: usize,
    distribution: TraceDistribution,
    total_requests: usize,
    hot_ibge: Option<&str>,
) -> RequestTrace {
    let mut universe: Vec<(String, String, usize)> = Vec::new();
    for city in 0..cities {
        let ibge = ibge_for(city);
        let stations = oracle.per_city.get(&ibge).copied().unwrap_or(0);
        universe.push((ibge, city_name_for(city), stations));
    }
    for empty in 0..EMPTY_CITIES {
        universe.push((empty_ibge_for(empty), format!("VAZIA {empty:03}"), 0));
    }

    let hot: String = match distribution {
        TraceDistribution::Hot90 => hot_ibge
            .map(str::to_string)
            .or_else(|| {
                universe
                    .iter()
                    .max_by_key(|(_, _, stations)| *stations)
                    .map(|(ibge, _, _)| ibge.clone())
            })
            .expect("universe is never empty"),
        _ => String::new(),
    };

    let mut ranked: Vec<usize> = (0..universe.len()).collect();
    ranked.sort_by_key(|&index| std::cmp::Reverse(universe[index].2));
    // Pool shares, not per-city weights: the top pool owns 80% of
    // skew requests and the hot city 90%, regardless of universe size.
    let top = (cities * 20 / 100).max(1);
    let skew_rest = universe
        .iter()
        .enumerate()
        .filter(|(index, (_, _, stations))| {
            ranked
                .iter()
                .position(|&slot| slot == *index)
                .unwrap_or(usize::MAX)
                >= top
                && *stations > 0
        })
        .count()
        .max(1);
    let hot_rest = (universe.len() - 1).max(1) as f64;

    let weights: Vec<f64> = universe
        .iter()
        .enumerate()
        .map(|(index, (ibge, _, stations))| match distribution {
            TraceDistribution::Uniform => 1.0,
            TraceDistribution::Proportional => *stations as f64,
            TraceDistribution::Skew80_20 => {
                let rank = ranked
                    .iter()
                    .position(|&slot| slot == index)
                    .unwrap_or(usize::MAX);
                if rank < top {
                    80.0 / top as f64
                } else if *stations > 0 {
                    20.0 / skew_rest as f64
                } else {
                    0.0
                }
            }
            TraceDistribution::Hot90 => {
                if *ibge == hot {
                    90.0
                } else {
                    10.0 / hot_rest
                }
            }
        })
        .collect();

    let total_weight: f64 = weights.iter().sum();
    let mut apportioned = vec![0usize; universe.len()];
    if total_weight > 0.0 {
        let mut fractions: Vec<(usize, f64)> = Vec::with_capacity(universe.len());
        let mut assigned = 0usize;
        for (index, weight) in weights.iter().enumerate() {
            let exact = *weight / total_weight * total_requests as f64;
            let base = exact.floor() as usize;
            apportioned[index] = base;
            assigned += base;
            fractions.push((index, exact - base as f64));
        }
        fractions.sort_by(|a, b| b.1.partial_cmp(&a.1).unwrap_or(std::cmp::Ordering::Equal));
        let mut remainder = total_requests - assigned;
        for (index, _) in fractions {
            if remainder == 0 {
                break;
            }
            if weights[index] > 0.0 {
                apportioned[index] += 1;
                remainder -= 1;
            }
        }
    }

    let mut city_weights = Vec::with_capacity(universe.len());
    let mut strata = StrataSummary::default();
    for (index, (ibge, name, stations)) in universe.into_iter().enumerate() {
        let stratum = stratum(stations);
        let requests = apportioned[index];
        match stratum {
            "empty" => {
                strata.empty_cities += 1;
                strata.empty_requests += requests;
            }
            "tiny-1-10" => {
                strata.tiny_cities += 1;
                strata.tiny_requests += requests;
            }
            "small-11-100" => {
                strata.small_cities += 1;
                strata.small_requests += requests;
            }
            _ => {
                strata.large_cities += 1;
                strata.large_requests += requests;
            }
        }
        city_weights.push(CityWeight {
            ibge,
            name,
            stations,
            stratum,
            weight: weights[index],
            requests,
        });
    }
    city_weights.sort_by(|a, b| a.ibge.cmp(&b.ibge));

    RequestTrace {
        workload_id: format!("W-{}-{}", oracle.profile_id, distribution.id()),
        profile_id: oracle.profile_id,
        distribution: distribution.id(),
        total_requests,
        seed: oracle.seed,
        hot_ibge: match distribution {
            TraceDistribution::Hot90 => Some(hot),
            _ => None,
        },
        cities: city_weights,
        strata,
    }
}

/// Second-edition delta: every `changed_every`-th data row (1-indexed)
/// carries a newer address/publication pair under the same identity;
/// `changed_every == 0` returns byte-identical input (no-op replay).
#[derive(Debug, Clone)]
pub struct DeltaEdition {
    pub csv: Vec<u8>,
    pub changed: usize,
    pub unchanged: usize,
}

pub fn delta_edition(csv: &[u8], changed_every: usize) -> DeltaEdition {
    let text = String::from_utf8(csv.to_vec()).expect("dataset csv is UTF-8");
    let mut lines: Vec<&str> = text.lines().collect();
    let header = lines.remove(0);
    let mut out = String::with_capacity(csv.len() + 64);
    out.push_str(header);
    out.push('\n');
    let mut changed = 0usize;
    for (position, line) in lines.iter().enumerate() {
        let data_row = position + 1;
        if changed_every > 0 && data_row.is_multiple_of(changed_every) {
            let mut fields: Vec<&str> = line.split(';').collect();
            if fields.len() == 13 {
                fields[2] = "2024-06-01";
                fields[5] = "RUA ALTERADA 999";
                fields[12] = "2024-06-10";
                out.push_str(&fields.join(";"));
                out.push('\n');
                changed += 1;
                continue;
            }
        }
        out.push_str(line);
        out.push('\n');
    }
    let unchanged = lines.len() - changed;
    DeltaEdition {
        csv: out.into_bytes(),
        changed,
        unchanged,
    }
}

#[cfg(test)]
mod tests {
    use super::{
        alias_table_json, delta_edition, generate, ibge_for, request_trace, stratum, tiny_profile,
        TraceDistribution, UNIVERSE_CITIES,
    };

    #[test]
    fn alias_table_covers_populated_and_empty_cities() {
        let table = alias_table_json(5);
        assert!(table.contains("\"ibge\": \"3550000\""));
        assert!(table.contains("\"ibge\": \"3550004\""));
        assert!(table.contains("\"ibge\": \"3559900\""));
        assert!(table.contains("\"ibge\": \"3559904\""));
        assert!(!table.contains("\"ibge\": \"3550005\""));
        assert_eq!(super::EMPTY_CITIES, 5);
        assert_eq!(UNIVERSE_CITIES, 200);
    }

    #[test]
    fn strata_boundaries_match_campaign_definition() {
        assert_eq!(stratum(0), "empty");
        assert_eq!(stratum(1), "tiny-1-10");
        assert_eq!(stratum(10), "tiny-1-10");
        assert_eq!(stratum(11), "small-11-100");
        assert_eq!(stratum(100), "small-11-100");
        assert_eq!(stratum(101), "large-over-100");
    }

    #[test]
    fn tiny_oracle_reconciles_and_cities_cover_universe() {
        let generated = generate(&tiny_profile());
        let oracle = &generated.oracle;
        assert_eq!(oracle.input, 30);
        assert_eq!(
            oracle.accepted + oracle.duplicates + oracle.quarantined,
            oracle.input
        );
        assert!(oracle.duplicates >= 1);
        assert!(oracle.quarantined >= 2);
        assert_eq!(oracle.unique_cnpjs, oracle.accepted);
        assert_eq!(oracle.history_per_1, oracle.unique_cnpjs);
        assert_eq!(oracle.history_per_10, oracle.unique_cnpjs * 10);
        assert_eq!(oracle.history_per_100, oracle.unique_cnpjs * 100);
        let per_city_total: usize = oracle.per_city.values().sum();
        assert_eq!(per_city_total, oracle.accepted);
        for ibge in oracle.per_city.keys() {
            assert!(ibge.starts_with("3550"), "synthetic IBGE range: {ibge}");
        }
        assert!(generated.csv.starts_with(b"CODIGOISIMP;"));
        assert_eq!(ibge_for(0), "3550000");
    }

    #[test]
    fn traces_apportion_exactly_and_cover_empty_strata() {
        let generated = generate(&tiny_profile());
        for distribution in [
            TraceDistribution::Uniform,
            TraceDistribution::Proportional,
            TraceDistribution::Skew80_20,
            TraceDistribution::Hot90,
        ] {
            let trace = request_trace(&generated.oracle, 5, distribution, 1000, None);
            let total: usize = trace.cities.iter().map(|city| city.requests).sum();
            assert_eq!(total, 1000, "distribution {}", trace.distribution);
            assert_eq!(trace.cities.len(), 5 + 5);
            assert_eq!(trace.strata.empty_cities, 5);
        }
        let uniform = request_trace(&generated.oracle, 5, TraceDistribution::Uniform, 1000, None);
        assert!(uniform.strata.empty_requests > 0);
        let hot = request_trace(&generated.oracle, 5, TraceDistribution::Hot90, 1000, None);
        let hot_ibge = hot.hot_ibge.clone().expect("hot city pinned");
        let hot_entry = hot
            .cities
            .iter()
            .find(|city| city.ibge == hot_ibge)
            .expect("hot city listed");
        assert_eq!(hot_entry.requests, 900);
    }

    #[test]
    fn delta_noop_is_byte_identical_and_delta_counts_change() {
        let generated = generate(&tiny_profile());
        let noop = delta_edition(&generated.csv, 0);
        assert_eq!(noop.csv, generated.csv);
        assert_eq!(noop.changed, 0);
        let delta = delta_edition(&generated.csv, 7);
        assert!(delta.changed > 0);
        assert_eq!(delta.changed + delta.unchanged, 30);
        assert_ne!(delta.csv, generated.csv);
    }
}

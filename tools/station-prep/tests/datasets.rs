//! RST-11 acceptance: generated datasets reconcile with the real
//! registry parser, stay deterministic, keep semantics under input
//! reorder and freeze geographic request distributions.

use station_prep::{
    alias_table_json, delta_edition, generate, representative_profile, request_trace, tiny_profile,
    AliasTable, DatasetProfile, Limits, TraceDistribution,
};
use std::collections::{HashMap, HashSet};

fn parse_generated(csv: &[u8], aliases_json: &str) -> station_prep::RegistryBatch {
    let table = AliasTable::from_json(aliases_json).expect("generated aliases load");
    station_prep::parse_registry(
        "datasets-tiny.csv",
        csv.to_vec(),
        &Limits::default(),
        &table,
    )
    .expect("generated csv parses")
}

fn parser_checksum(batch: &station_prep::RegistryBatch) -> String {
    let mut keys: Vec<&str> = batch
        .accepted
        .iter()
        .map(|row| row.source_key.as_str())
        .collect();
    keys.sort();
    let mut joined = String::new();
    for key in &keys {
        joined.push_str(key);
        joined.push('\n');
    }
    use sha2::Digest;
    let digest = sha2::Sha256::digest(joined.as_bytes());
    const TABLE: &[u8; 16] = b"0123456789abcdef";
    let mut out = String::with_capacity(digest.len() * 2);
    for byte in digest {
        out.push(TABLE[(byte >> 4) as usize] as char);
        out.push(TABLE[(byte & 0x0F) as usize] as char);
    }
    out
}

#[test]
fn tiny_oracle_matches_real_parser_counts_and_checksum() {
    let generated = generate(&tiny_profile());
    let batch = parse_generated(&generated.csv, &generated.aliases_json);
    let oracle = &generated.oracle;
    assert_eq!(batch.counts.input, oracle.input);
    assert_eq!(batch.counts.accepted, oracle.accepted);
    assert_eq!(batch.counts.duplicates, oracle.duplicates);
    assert_eq!(batch.counts.quarantined, oracle.quarantined);
    assert_eq!(
        batch.counts.accepted + batch.counts.duplicates + batch.counts.quarantined,
        batch.counts.input
    );
    assert_eq!(parser_checksum(&batch), oracle.checksum);
    // Per-city histogram agrees with accepted rows resolved by code.
    let mut per_city: HashMap<String, usize> = HashMap::new();
    for row in &batch.accepted {
        *per_city.entry(row.municipality_ibge.clone()).or_default() += 1;
    }
    assert_eq!(per_city.len(), oracle.per_city.len());
    for (ibge, count) in &oracle.per_city {
        assert_eq!(per_city.get(ibge).copied().unwrap_or(0), *count);
    }
    // Quarantine covers both injected reasons.
    let reasons: HashSet<&str> = batch.quarantine.iter().map(|row| row.reason).collect();
    assert!(reasons.contains("invalid_cnpj"));
    assert!(reasons.contains("unknown_city"));
}

#[test]
fn generation_is_deterministic_down_to_bytes() {
    let first = generate(&tiny_profile());
    let second = generate(&tiny_profile());
    assert_eq!(first.csv, second.csv);
    assert_eq!(first.aliases_json, second.aliases_json);
    assert_eq!(first.oracle.checksum, second.oracle.checksum);
    assert_eq!(first.oracle.csv_sha256, second.oracle.csv_sha256);
}

#[test]
fn reordered_inputs_keep_oracle_semantics() {
    let generated = generate(&tiny_profile());
    let text = String::from_utf8(generated.csv.clone()).expect("csv utf-8");
    let mut lines: Vec<&str> = text.lines().collect();
    let header = lines.remove(0);
    // Deterministic Fisher-Yates shuffle with a fixed seed.
    let mut state: u64 = 0x9E3779B97F4A7C15;
    let mut next = move || {
        state ^= state >> 12;
        state ^= state << 25;
        state ^= state >> 27;
        state = state.wrapping_mul(0x2545F4914F6CDD1D);
        state
    };
    for index in (1..lines.len()).rev() {
        let other = (next() % (index as u64 + 1)) as usize;
        lines.swap(index, other);
    }
    let mut reordered = String::from(header);
    reordered.push('\n');
    for line in &lines {
        reordered.push_str(line);
        reordered.push('\n');
    }
    let batch = parse_generated(reordered.as_bytes(), &generated.aliases_json);
    assert_eq!(batch.counts.input, generated.oracle.input);
    assert_eq!(batch.counts.accepted, generated.oracle.accepted);
    assert_eq!(batch.counts.duplicates, generated.oracle.duplicates);
    assert_eq!(batch.counts.quarantined, generated.oracle.quarantined);
    assert_eq!(parser_checksum(&batch), generated.oracle.checksum);
}

#[test]
fn knobs_vary_independently() {
    let base = DatasetProfile {
        id: "probe",
        rows: 300,
        seed: 21,
        cities: 5,
        duplicate_pct: 0,
        conflict_pct: 0,
        quarantine_every: 0,
        long_field_every: 0,
        missing_location_every: 0,
    };
    let clean = generate(&base);
    assert_eq!(clean.oracle.duplicates, 0);
    assert_eq!(clean.oracle.quarantined, 0);
    assert_eq!(clean.oracle.without_location, 0);
    assert_eq!(clean.oracle.long_field_rows, 0);

    let duplicates = generate(&DatasetProfile {
        duplicate_pct: 10,
        ..base
    });
    assert!(duplicates.oracle.duplicates > 0);
    assert_eq!(duplicates.oracle.quarantined, 0);

    let quarantined = generate(&DatasetProfile {
        quarantine_every: 30,
        ..base
    });
    assert!(quarantined.oracle.quarantined > 0);
    assert_eq!(quarantined.oracle.duplicates, 0);

    let missing = generate(&DatasetProfile {
        missing_location_every: 10,
        ..base
    });
    assert!(missing.oracle.without_location > 0);
    assert_eq!(
        missing.oracle.with_location + missing.oracle.without_location,
        missing.oracle.unique_cnpjs
    );

    let long = generate(&DatasetProfile {
        long_field_every: 25,
        ..base
    });
    assert!(long.oracle.long_field_rows > 0);
    // Long fields stay accepted: same accounting as the clean probe.
    assert_eq!(long.oracle.accepted, clean.oracle.accepted);
}

#[test]
fn synthetic_locations_never_carry_trust() {
    let generated = generate(&tiny_profile());
    assert_eq!(generated.locations.len(), generated.oracle.unique_cnpjs);
    assert!(generated.locations.iter().all(|point| point.synthetic));
    let missing = generated
        .locations
        .iter()
        .filter(|point| point.latitude.is_none())
        .count();
    assert_eq!(missing, generated.oracle.without_location);
    for point in &generated.locations {
        assert_eq!(point.latitude.is_none(), point.longitude.is_none());
        if let (Some(lat), Some(lon)) = (point.latitude, point.longitude) {
            assert!((-24.0..-23.0).contains(&lat), "SP cluster lat: {lat}");
            assert!((-47.5..-46.0).contains(&lon), "SP cluster lon: {lon}");
        }
    }
}

#[test]
fn no_identical_cnpj_scaling_and_history_denominators_hold() {
    let generated = generate(&tiny_profile());
    let oracle = &generated.oracle;
    let batch = parse_generated(&generated.csv, &generated.aliases_json);
    let mut seen: HashSet<&str> = HashSet::new();
    let mut exact_repeats = 0usize;
    for row in &batch.accepted {
        if !seen.insert(row.source_key.as_str()) {
            exact_repeats += 1;
        }
    }
    // Accepted rows are unique identities; repeats land in duplicates.
    assert_eq!(exact_repeats, 0);
    assert_eq!(oracle.unique_cnpjs, oracle.accepted);
    assert_eq!(oracle.history_per_1, oracle.unique_cnpjs);
    assert_eq!(oracle.history_per_10, oracle.unique_cnpjs * 10);
    assert_eq!(oracle.history_per_100, oracle.unique_cnpjs * 100);
}

#[test]
fn representative_shape_stays_skewed_and_parseable() {
    let profile = DatasetProfile {
        rows: 2000,
        ..representative_profile()
    };
    let generated = generate(&profile);
    let batch = parse_generated(&generated.csv, &generated.aliases_json);
    assert_eq!(batch.counts.input, 2000);
    assert_eq!(batch.counts.accepted, generated.oracle.accepted);
    assert_eq!(batch.counts.duplicates, generated.oracle.duplicates);
    assert_eq!(batch.counts.quarantined, generated.oracle.quarantined);
    assert_eq!(parser_checksum(&batch), generated.oracle.checksum);
    // Skew: the densest city holds strictly more than the mean share.
    let mean = generated.oracle.accepted as f64 / generated.oracle.per_city.len() as f64;
    let densest = generated
        .oracle
        .per_city
        .values()
        .max()
        .copied()
        .unwrap_or(0) as f64;
    assert!(densest > mean, "skewed aggregate shape");
    // The 2k probe covers a subset of the 200-city universe; the full
    // 20k representative widens coverage under the same skew family.
    assert!(generated.oracle.per_city.len() > 20);
}

#[test]
fn traces_follow_station_counts_and_hot_rotation() {
    let generated = generate(&tiny_profile());
    let proportional = request_trace(
        &generated.oracle,
        5,
        TraceDistribution::Proportional,
        1000,
        None,
    );
    // Highest-station city gets at least as many requests as the lowest.
    let mut weighted: Vec<(&str, usize, usize)> = proportional
        .cities
        .iter()
        .map(|city| (city.ibge.as_str(), city.stations, city.requests))
        .collect();
    weighted.sort_by_key(|(_, stations, _)| *stations);
    assert!(
        weighted.first().unwrap().2 <= weighted.last().unwrap().2,
        "proportional follows station counts"
    );
    // Hot rotation pins a different city on request.
    let first_city = generated.oracle.per_city.keys().next().unwrap().clone();
    let rotated = request_trace(
        &generated.oracle,
        5,
        TraceDistribution::Hot90,
        1000,
        Some(&first_city),
    );
    assert_eq!(rotated.hot_ibge.as_deref(), Some(first_city.as_str()));
    let hot_requests = rotated
        .cities
        .iter()
        .find(|city| city.ibge == first_city)
        .unwrap()
        .requests;
    assert_eq!(hot_requests, 900);
    // Skew 80/20 concentrates on the top 20% of populated cities.
    let skew = request_trace(
        &generated.oracle,
        5,
        TraceDistribution::Skew80_20,
        1000,
        None,
    );
    let top: usize = skew
        .cities
        .iter()
        .filter(|city| city.weight == 80.0)
        .map(|city| city.requests)
        .sum();
    assert!((780..=820).contains(&top), "80/20 concentration: {top}");
    // Delta editions: second edition changes rows, identical bytes noop.
    let noop = delta_edition(&generated.csv, 0);
    assert_eq!(noop.csv, generated.csv);
    let delta = delta_edition(&generated.csv, 7);
    assert_eq!(delta.changed + delta.unchanged, 30);
    assert!(delta.changed > 0);
}

#[test]
fn alias_table_shape_matches_reader_contract() {
    let aliases = alias_table_json(5);
    let table = AliasTable::from_json(&aliases).expect("shape loads");
    assert!(table.resolve("CIDADE 000", "SP").is_ok());
    assert!(table.resolve("VAZIA 000", "SP").is_ok());
    assert!(table.resolve("NOWHERE XV", "SP").is_err());
}

#[test]
fn committed_tiny_fixtures_match_frozen_oracle() {
    let dir = std::path::PathBuf::from(env!("CARGO_MANIFEST_DIR"))
        .join("../../contracts/testdata/station-prep/datasets");
    let csv = std::fs::read(dir.join("tiny.csv")).expect("tiny fixture committed");
    let aliases =
        String::from_utf8(std::fs::read(dir.join("tiny-aliases.json")).expect("aliases committed"))
            .expect("aliases utf-8");
    let oracle_text =
        String::from_utf8(std::fs::read(dir.join("tiny-oracle.json")).expect("oracle committed"))
            .expect("oracle utf-8");
    let oracle: serde_json::Value = serde_json::from_str(&oracle_text).expect("oracle parses");
    // Regeneration from the frozen profile reproduces committed bytes.
    let generated = generate(&tiny_profile());
    assert_eq!(generated.csv, csv, "tiny fixture is reproducible");
    assert_eq!(generated.aliases_json, aliases);
    // The real parser agrees with the frozen on-disk oracle.
    let batch = parse_generated(&csv, &aliases);
    assert_eq!(
        batch.counts.input,
        oracle["input"].as_u64().unwrap() as usize
    );
    assert_eq!(
        batch.counts.accepted,
        oracle["accepted"].as_u64().unwrap() as usize
    );
    assert_eq!(
        batch.counts.duplicates,
        oracle["duplicates"].as_u64().unwrap() as usize
    );
    assert_eq!(
        batch.counts.quarantined,
        oracle["quarantined"].as_u64().unwrap() as usize
    );
    assert_eq!(
        parser_checksum(&batch),
        oracle["checksum"].as_str().unwrap()
    );
    let trace_text = String::from_utf8(
        std::fs::read(dir.join("trace-uniform-1000.json")).expect("trace committed"),
    )
    .expect("trace utf-8");
    let trace: serde_json::Value = serde_json::from_str(&trace_text).expect("trace parses");
    let total: u64 = trace["cities"]
        .as_array()
        .unwrap()
        .iter()
        .map(|city| city["requests"].as_u64().unwrap())
        .sum();
    assert_eq!(total, 1000);
    assert_eq!(trace["strata"]["empty_requests"].as_u64().unwrap(), 500);
}

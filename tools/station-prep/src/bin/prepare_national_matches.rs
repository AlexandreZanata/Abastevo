//! Local-only national ANP API x OSM matcher. No network, database or clock.
//!
//! Usage:
//!   prepare_national_matches --api DIR --osm references.ndjson \
//!     --registry assertions.jsonl --output DIR --observed-at RFC3339 \
//!     [--published-at RFC3339] [--fetched-at RFC3339] \
//!     [--max-distance-m 50] [--max-cnpj-distance-m 200] \
//!     [--generation ID] [--reviewer NAME] [--max-map-assertions 10000]

use station_prep::national_match::{
    self, MapClocks, RunOptions, DEFAULT_MAX_CNPJ_DISTANCE_M, DEFAULT_MAX_DISTANCE_M,
    MAX_MAP_ASSERTIONS,
};
use std::path::Path;

fn value(args: &[String], name: &str) -> Option<String> {
    args.iter()
        .position(|arg| arg == name)
        .and_then(|index| args.get(index + 1))
        .cloned()
}

fn fail(message: &str) -> ! {
    eprintln!("prepare_national_matches: {message}");
    std::process::exit(2);
}

fn main() {
    let args: Vec<String> = std::env::args().collect();
    let api = value(&args, "--api").unwrap_or_else(|| fail("--api is required"));
    let osm = value(&args, "--osm").unwrap_or_else(|| fail("--osm is required"));
    let registry = value(&args, "--registry").unwrap_or_else(|| fail("--registry is required"));
    let output = value(&args, "--output").unwrap_or_else(|| fail("--output is required"));
    let observed_at =
        value(&args, "--observed-at").unwrap_or_else(|| fail("--observed-at is required"));
    let published_at = value(&args, "--published-at").unwrap_or_else(|| observed_at.clone());
    let fetched_at = value(&args, "--fetched-at").unwrap_or_else(|| observed_at.clone());
    let max_distance_m = value(&args, "--max-distance-m")
        .map(|raw| {
            raw.parse::<f64>()
                .unwrap_or_else(|_| fail("invalid --max-distance-m"))
        })
        .unwrap_or(DEFAULT_MAX_DISTANCE_M);
    let max_cnpj_distance_m = value(&args, "--max-cnpj-distance-m")
        .map(|raw| {
            raw.parse::<f64>()
                .unwrap_or_else(|_| fail("invalid --max-cnpj-distance-m"))
        })
        .unwrap_or(DEFAULT_MAX_CNPJ_DISTANCE_M);
    let max_map_assertions = value(&args, "--max-map-assertions")
        .map(|raw| {
            raw.parse::<usize>()
                .unwrap_or_else(|_| fail("invalid --max-map-assertions"))
        })
        .unwrap_or(MAX_MAP_ASSERTIONS);
    let generation = value(&args, "--generation").unwrap_or_default();
    let reviewer = value(&args, "--reviewer").unwrap_or_else(|| "map-national-phase2".to_string());

    let options = RunOptions {
        max_distance_m,
        max_cnpj_distance_m,
        max_map_assertions,
        generation,
        reviewer,
        clocks: MapClocks {
            observed_at,
            published_at,
            fetched_at,
        },
    };
    let result = national_match::run(
        Path::new(&api),
        Path::new(&osm),
        Path::new(&registry),
        Path::new(&output),
        &options,
    )
    .unwrap_or_else(|error| fail(&format!("{error}")));
    let summary = serde_json::json!({
        "generation": result.generation,
        "api_records": result.inputs.api_records.len(),
        "registry_rows": result.inputs.registry.len(),
        "reviews": result.result.reviews.len(),
        "map_assertions": result.result.map_assertions.len(),
        "decisions": result.result.decisions.len(),
        "tiers": result.result.counts.tiers,
        "timings": result.timings,
    });
    println!(
        "{}",
        serde_json::to_string(&summary).unwrap_or_else(|_| "{}".to_string())
    );
}

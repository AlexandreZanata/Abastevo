//! Local-only open-data geocoder for registry stations without an ANP point.
//! No network, database or clock.
//!
//! Usage:
//!   prepare_open_geocode --registry assertions.jsonl \
//!     --decisions decisions.ndjson --osm references.ndjson \
//!     --cnefe cnefe.ndjson --rfb rfb.ndjson --output DIR \
//!     --observed-at RFC3339 [--published-at RFC3339] [--fetched-at RFC3339] \
//!     [--generation ID] [--ambiguity-radius-m 100] \
//!     [--max-map-assertions 10000]

use station_prep::national_match::MapClocks;
use station_prep::open_geocode::{
    self, GeocodeOptions, DEFAULT_AMBIGUITY_RADIUS_M, MAX_MAP_ASSERTIONS,
};
use std::path::Path;

fn value(args: &[String], name: &str) -> Option<String> {
    args.iter()
        .position(|arg| arg == name)
        .and_then(|index| args.get(index + 1))
        .cloned()
}

fn fail(message: &str) -> ! {
    eprintln!("prepare_open_geocode: {message}");
    std::process::exit(2);
}

fn main() {
    let args: Vec<String> = std::env::args().collect();
    let registry = value(&args, "--registry").unwrap_or_else(|| fail("--registry is required"));
    let decisions = value(&args, "--decisions").unwrap_or_else(|| fail("--decisions is required"));
    let osm = value(&args, "--osm").unwrap_or_else(|| fail("--osm is required"));
    let cnefe = value(&args, "--cnefe").unwrap_or_else(|| fail("--cnefe is required"));
    let rfb = value(&args, "--rfb").unwrap_or_else(|| fail("--rfb is required"));
    let output = value(&args, "--output").unwrap_or_else(|| fail("--output is required"));
    let observed_at =
        value(&args, "--observed-at").unwrap_or_else(|| fail("--observed-at is required"));
    let published_at = value(&args, "--published-at").unwrap_or_else(|| observed_at.clone());
    let fetched_at = value(&args, "--fetched-at").unwrap_or_else(|| observed_at.clone());
    let ambiguity_radius_m = value(&args, "--ambiguity-radius-m")
        .map(|raw| {
            raw.parse::<f64>()
                .unwrap_or_else(|_| fail("invalid --ambiguity-radius-m"))
        })
        .unwrap_or(DEFAULT_AMBIGUITY_RADIUS_M);
    let max_map_assertions = value(&args, "--max-map-assertions")
        .map(|raw| {
            raw.parse::<usize>()
                .unwrap_or_else(|_| fail("invalid --max-map-assertions"))
        })
        .unwrap_or(MAX_MAP_ASSERTIONS);
    let generation = value(&args, "--generation").unwrap_or_default();
    let options = GeocodeOptions {
        ambiguity_radius_m,
        max_map_assertions,
        generation,
        clocks: MapClocks {
            observed_at,
            published_at,
            fetched_at,
        },
    };
    let result = open_geocode::run(
        Path::new(&registry),
        Path::new(&decisions),
        Path::new(&osm),
        Path::new(&cnefe),
        Path::new(&rfb),
        Path::new(&output),
        &options,
    )
    .unwrap_or_else(|error| fail(&format!("{error}")));
    let summary = serde_json::json!({
        "generation": result.generation,
        "l5_rows": result.inputs.l5.len(),
        "decisions": result.result.decisions.len(),
        "candidates": result.result.candidates.len(),
        "map_assertions": result.result.map_assertions.len(),
        "counts": result.result.counts,
        "timings": result.timings,
    });
    println!(
        "{}",
        serde_json::to_string(&summary).unwrap_or_else(|_| "{}".to_string())
    );
}

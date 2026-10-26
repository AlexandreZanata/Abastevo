//! File-only Phase 3 coverage and apply preparation. No database, network or
//! deployment.
//!
//! Usage:
//!   prepare_national_coverage --a2 DIR --b2 DIR --registry assertions.jsonl \
//!     --output DIR [--generation ID]

use station_prep::national_coverage::{self, CoverageOptions};
use std::path::Path;

fn value(args: &[String], name: &str) -> Option<String> {
    args.iter()
        .position(|arg| arg == name)
        .and_then(|index| args.get(index + 1))
        .cloned()
}

fn fail(message: &str) -> ! {
    eprintln!("prepare_national_coverage: {message}");
    std::process::exit(2);
}

fn main() {
    let args: Vec<String> = std::env::args().collect();
    let a2 = value(&args, "--a2").unwrap_or_else(|| fail("--a2 is required"));
    let b2 = value(&args, "--b2").unwrap_or_else(|| fail("--b2 is required"));
    let registry = value(&args, "--registry").unwrap_or_else(|| fail("--registry is required"));
    let output = value(&args, "--output").unwrap_or_else(|| fail("--output is required"));
    let generation = value(&args, "--generation").unwrap_or_default();
    let options = CoverageOptions { generation };
    let result = national_coverage::run(
        Path::new(&a2),
        Path::new(&b2),
        Path::new(&registry),
        Path::new(&output),
        &options,
    )
    .unwrap_or_else(|error| fail(&format!("{error}")));
    let summary = serde_json::json!({
        "generation": result.generation,
        "coverage": result.coverage.counts,
        "station_review_ready": result.apply_plan.station_review.ready,
        "map_load_ready": result.apply_plan.map_load.ready,
        "station_review_count": result.apply_plan.station_review.count,
        "map_load_count": result.apply_plan.map_load.count,
        "timings": result.timings,
    });
    println!(
        "{}",
        serde_json::to_string(&summary).unwrap_or_else(|_| "{}".to_string())
    );
}

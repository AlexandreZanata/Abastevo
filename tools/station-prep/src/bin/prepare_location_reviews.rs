//! Local-only cross-source location review CLI. No network, database or clock.
//!
//! Usage:
//!   prepare_location_reviews --api anp-curitiba.json --osm references.ndjson \
//!     --registry assertions.jsonl --output out/reviews \
//!     [--max-distance-m 50] [--reviewer map-minimum-gate-20261025]

use station_prep::location_review::{run_review, write_run, DEFAULT_MAX_DISTANCE_M};

fn value(args: &[String], name: &str) -> Option<String> {
    args.iter()
        .position(|arg| arg == name)
        .and_then(|index| args.get(index + 1))
        .cloned()
}

fn fail(message: &str) -> ! {
    eprintln!("prepare_location_reviews: {message}");
    std::process::exit(2);
}

fn main() {
    let args: Vec<String> = std::env::args().collect();
    let api_path = value(&args, "--api").unwrap_or_else(|| fail("--api is required"));
    let osm_path = value(&args, "--osm").unwrap_or_else(|| fail("--osm is required"));
    let registry_path =
        value(&args, "--registry").unwrap_or_else(|| fail("--registry is required"));
    let output = value(&args, "--output").unwrap_or_else(|| fail("--output is required"));
    let reviewer = value(&args, "--reviewer").unwrap_or_else(|| "map-minimum-gate".into());
    let max_distance_m = value(&args, "--max-distance-m")
        .map(|raw| {
            raw.parse::<f64>()
                .unwrap_or_else(|_| fail("invalid --max-distance-m"))
        })
        .unwrap_or(DEFAULT_MAX_DISTANCE_M);
    let api = std::fs::read(&api_path).unwrap_or_else(|error| fail(&format!("read api: {error}")));
    let osm = std::fs::read(&osm_path).unwrap_or_else(|error| fail(&format!("read osm: {error}")));
    let registry = std::fs::read(&registry_path)
        .unwrap_or_else(|error| fail(&format!("read registry: {error}")));
    let run = run_review(&api, &osm, &registry, max_distance_m, &reviewer)
        .unwrap_or_else(|error| fail(&format!("{error}")));
    write_run(
        std::path::Path::new(&output),
        &run,
        max_distance_m,
        &reviewer,
    )
    .unwrap_or_else(|error| fail(&format!("{error}")));
    println!(
        "{{\"input\":{},\"reviewed\":{},\"rejected\":{},\"duplicates\":{}}}",
        run.counts.input, run.counts.reviewed, run.counts.rejected, run.counts.duplicates
    );
}

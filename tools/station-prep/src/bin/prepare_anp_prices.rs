//! Local-only ANP station-price CLI. No network, database or native engine.
//!
//! Usage:
//!   prepare_anp_prices --input stations.xlsx --output out/prices \
//!     --period-start 2026-10-18 --period-end 2026-10-24 \
//!     [--source-revision surveys/2026-10-24]

use station_prep::anp_prices::{parse_station_prices_xlsx, write_run};
use station_prep::Limits;

fn value(args: &[String], name: &str) -> Option<String> {
    args.iter()
        .position(|arg| arg == name)
        .and_then(|index| args.get(index + 1))
        .cloned()
}

fn fail(message: &str) -> ! {
    eprintln!("prepare_anp_prices: {message}");
    std::process::exit(2);
}

fn main() {
    let args: Vec<String> = std::env::args().collect();
    let input = value(&args, "--input").unwrap_or_else(|| fail("--input is required"));
    let output = value(&args, "--output").unwrap_or_else(|| fail("--output is required"));
    let period_start =
        value(&args, "--period-start").unwrap_or_else(|| fail("--period-start is required"));
    let period_end =
        value(&args, "--period-end").unwrap_or_else(|| fail("--period-end is required"));
    let source_revision = value(&args, "--source-revision").unwrap_or_else(|| input.clone());
    let raw = std::fs::read(&input).unwrap_or_else(|error| fail(&format!("read failed: {error}")));
    let file_name = std::path::Path::new(&input)
        .file_name()
        .and_then(|name| name.to_str())
        .unwrap_or("stations.xlsx");
    let run = parse_station_prices_xlsx(
        file_name,
        &raw,
        &Limits::default(),
        &period_start,
        &period_end,
        &source_revision,
        &|| false,
    )
    .unwrap_or_else(|error| fail(&format!("{error}")));
    write_run(std::path::Path::new(&output), &run)
        .unwrap_or_else(|error| fail(&format!("{error}")));
    println!(
        "{{\"accepted\":{},\"duplicates\":{},\"quarantined\":{},\"input\":{}}}",
        run.counts.accepted, run.counts.duplicates, run.counts.quarantined, run.counts.input
    );
}

//! Normalize raw ANP Revendedores API pages (MC-A2). No network or clock.
use station_prep::anp_registry;
use std::path::Path;

fn value(args: &[String], name: &str) -> Option<String> {
    args.iter()
        .position(|arg| arg == name)
        .and_then(|i| args.get(i + 1))
        .cloned()
}

fn fail(message: &str) -> ! {
    eprintln!("prepare_anp_registry: {message}");
    std::process::exit(2);
}

fn main() {
    let args: Vec<String> = std::env::args().collect();
    let input = value(&args, "--input").unwrap_or_else(|| fail("--input is required"));
    let output = value(&args, "--output").unwrap_or_else(|| fail("--output is required"));
    let fetched_at =
        value(&args, "--fetched-at").unwrap_or_else(|| fail("--fetched-at is required"));
    let kind = value(&args, "--kind").unwrap_or_else(|| "automotive-fuel".to_string());
    let source = value(&args, "--source").unwrap_or_else(|| "anp-revendedores-api".to_string());
    let run = anp_registry::run(
        Path::new(&input),
        Path::new(&output),
        &kind,
        &fetched_at,
        &source,
    )
    .unwrap_or_else(|e| fail(&e));
    println!(
        "{{\"accepted\":{},\"quarantined\":{},\"input\":{},\"output_sha256\":\"{}\"}}",
        run.manifest.counts.accepted,
        run.manifest.counts.quarantined,
        run.manifest.counts.input,
        run.manifest.output_sha256
    );
}

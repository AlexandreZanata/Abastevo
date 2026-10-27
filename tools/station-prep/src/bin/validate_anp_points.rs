//! Validate official ANP points against the IBGE malha (MC-A3, B-BR-MAP-15).
use station_prep::anp_validation;
use std::path::Path;

fn value(args: &[String], name: &str) -> Option<String> {
    args.iter()
        .position(|arg| arg == name)
        .and_then(|i| args.get(i + 1))
        .cloned()
}

fn fail(message: &str) -> ! {
    eprintln!("validate_anp_points: {message}");
    std::process::exit(2);
}

fn main() {
    let args: Vec<String> = std::env::args().collect();
    let registry = value(&args, "--registry").unwrap_or_else(|| fail("--registry is required"));
    let malha = value(&args, "--malha").unwrap_or_else(|| fail("--malha is required"));
    let municipalities =
        value(&args, "--municipalities").unwrap_or_else(|| fail("--municipalities is required"));
    let output = value(&args, "--output").unwrap_or_else(|| fail("--output is required"));
    let candidates = value(&args, "--candidates");
    let run = anp_validation::run(
        Path::new(&registry),
        Path::new(&malha),
        Path::new(&municipalities),
        candidates.as_deref().map(Path::new),
        Path::new(&output),
    )
    .unwrap_or_else(|e| fail(&e));
    println!(
        "{{\"input\":{},\"valid\":{},\"no_point\":{},\"rejected\":{},\"output_sha256\":\"{}\"}}",
        run.manifest.counts.input,
        run.manifest.counts.valid,
        run.manifest.counts.no_point,
        run.manifest.counts.rejected,
        run.manifest.output_sha256
    );
}

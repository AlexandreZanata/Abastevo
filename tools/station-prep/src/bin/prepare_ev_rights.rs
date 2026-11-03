//! Prepare or verify a local charging rights and derived-database bundle.
use station_prep::ev_rights_bundle::{
    prepare_bundle, prepare_coverage, prepare_geography, validate_bundle,
};
use std::path::Path;
use std::time::Instant;

fn run() -> Result<(), Box<dyn std::error::Error>> {
    let args: Vec<_> = std::env::args().skip(1).collect();
    if std::env::var_os("KUBERNETES_SERVICE_HOST").is_some()
        || ["APP_ENV", "ANPFUEL_ENV", "ANPFUEL_RUN_ENV", "DEPLOY_ENV"]
            .iter()
            .any(|key| {
                std::env::var(key).is_ok_and(|value| {
                    matches!(
                        value.to_ascii_lowercase().as_str(),
                        "prod" | "production" | "stage" | "staging"
                    )
                })
            })
    {
        return Err("local-only rights preparation".into());
    }
    let started = Instant::now();
    let receipt = match args.as_slice() {
        [mode, catalog, digest, bundle] if mode == "prepare" => prepare_bundle(
            Path::new(catalog), digest, Path::new(bundle), &|| started.elapsed().as_secs() >= 120,
        )?,
        [mode, catalog, digest, bundle] if mode == "verify" => {
            validate_bundle(Path::new(catalog), digest, Path::new(bundle))?
        }
        [mode, catalog, digest, boundaries, output] if mode == "geography" => {
            prepare_geography(Path::new(catalog), digest, Path::new(boundaries), Path::new(output))?
        }
        [mode, catalog, digest, boundaries, output] if mode == "coverage" => {
            prepare_coverage(Path::new(catalog), digest, Path::new(boundaries), Path::new(output))?
        }
        _ => return Err("usage: prepare_ev_rights prepare|verify CATALOG_DIR MANIFEST_SHA256 BUNDLE_DIR; or geography|coverage CATALOG_DIR MANIFEST_SHA256 IBGE_FILE OUTPUT_FILE".into()),
    };
    println!("{}", serde_json::to_string(&receipt)?);
    Ok(())
}

fn main() {
    if let Err(error) = run() {
        eprintln!("prepare_ev_rights: {error}");
        std::process::exit(2);
    }
}

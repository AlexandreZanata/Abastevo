//! Prepare or verify a local charging rights and derived-database bundle.
use station_prep::ev_rights_bundle::{prepare_bundle, validate_bundle};
use std::path::Path;
use std::time::Instant;

fn run() -> Result<(), Box<dyn std::error::Error>> {
    let args: Vec<_> = std::env::args().skip(1).collect();
    let [mode, catalog, digest, bundle] = args.as_slice() else {
        return Err(
            "usage: prepare_ev_rights prepare|verify CATALOG_DIR MANIFEST_SHA256 BUNDLE_DIR".into(),
        );
    };
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
    let catalog = Path::new(catalog);
    let bundle = Path::new(bundle);
    let started = Instant::now();
    let rights = match mode.as_str() {
        "prepare" => prepare_bundle(catalog, digest, bundle, &|| {
            started.elapsed().as_secs() >= 120
        })?,
        "verify" => validate_bundle(catalog, digest, bundle)?,
        _ => return Err("expected prepare or verify".into()),
    };
    println!("{}", serde_json::to_string(&rights)?);
    Ok(())
}

fn main() {
    if let Err(error) = run() {
        eprintln!("prepare_ev_rights: {error}");
        std::process::exit(2);
    }
}

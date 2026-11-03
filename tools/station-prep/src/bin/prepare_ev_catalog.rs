//! Local-only private EV catalog preparation from admitted normalized v2 artifacts.
use station_prep::ev_conflation_preparation::{prepare, Inputs};
use station_prep::ev_site_preparation::SourcePins;
use station_prep::osm_national::NationalProfile;
use std::collections::BTreeMap;
use std::path::PathBuf;
use std::time::Instant;
fn run() -> Result<(), Box<dyn std::error::Error>> {
    let args: Vec<_> = std::env::args().skip(1).collect();
    let mut values = BTreeMap::new();
    for pair in args.chunks(2) {
        if pair.len() != 2
            || ![
                "--normalized",
                "--manifest-sha256",
                "--boundaries",
                "--output",
            ]
            .contains(&pair[0].as_str())
            || values.insert(pair[0].as_str(), pair[1].as_str()).is_some()
        {
            return Err("usage: prepare_ev_catalog --normalized DIR --manifest-sha256 SHA --boundaries IBGE --output NEW_DIR; overrides refused".into());
        }
    }
    let value = |key| values.get(key).copied().ok_or("required input missing");
    let inputs = Inputs {
        normalized: PathBuf::from(value("--normalized")?),
        manifest_sha256: value("--manifest-sha256")?.into(),
        boundaries: PathBuf::from(value("--boundaries")?),
        output: PathBuf::from(value("--output")?),
    };
    for key in ["APP_ENV", "ANPFUEL_ENV", "ANPFUEL_RUN_ENV", "DEPLOY_ENV"] {
        if std::env::var(key).ok().is_some_and(|v| {
            ["prod", "production", "stage", "staging"].contains(&v.to_ascii_lowercase().as_str())
        }) {
            return Err("local-only conflation".into());
        }
    }
    if std::env::var_os("KUBERNETES_SERVICE_HOST").is_some() {
        return Err("local-only conflation".into());
    }
    let started = Instant::now();
    let profile = NationalProfile::frozen();
    let stats = prepare(&inputs, &SourcePins::frozen(), &|| {
        started.elapsed().as_secs() >= profile.resources.max_wall_seconds
    })?;
    println!("{}", serde_json::to_string(&stats)?);
    Ok(())
}
fn main() {
    if let Err(error) = run() {
        eprintln!("prepare_ev_catalog: {error}");
        std::process::exit(2);
    }
}

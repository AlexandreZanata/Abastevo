//! Local-only preparation of the frozen national EV source profile.
use station_prep::ev_site_preparation::{prepare, Inputs, SourcePins};
use station_prep::osm_national::NationalProfile;
use std::collections::BTreeMap;
use std::path::PathBuf;
use std::time::Instant;
fn run() -> Result<(), Box<dyn std::error::Error>> {
    let args: Vec<_> = std::env::args().skip(1).collect();
    let mut values = BTreeMap::new();
    for pair in args.chunks(2) {
        if pair.len() != 2
            || !["--osm", "--ocm", "--boundaries", "--output"].contains(&pair[0].as_str())
            || values.insert(pair[0].as_str(), pair[1].as_str()).is_some()
        {
            return Err("usage: prepare_ev_sites --osm PBF --ocm SNAPSHOT --boundaries IBGE --output NEW_DIRECTORY; overrides refused".into());
        }
    }
    let path = |key| {
        values
            .get(key)
            .map(PathBuf::from)
            .ok_or("required input missing")
    };
    let inputs = Inputs {
        osm: path("--osm")?,
        ocm: path("--ocm")?,
        boundaries: path("--boundaries")?,
        output: path("--output")?,
    };
    for key in ["APP_ENV", "ANPFUEL_ENV", "ANPFUEL_RUN_ENV", "DEPLOY_ENV"] {
        if std::env::var(key).ok().is_some_and(|v| {
            ["prod", "production", "stage", "staging"].contains(&v.to_ascii_lowercase().as_str())
        }) {
            return Err("local-only source preparation".into());
        }
    }
    if std::env::var_os("KUBERNETES_SERVICE_HOST").is_some() {
        return Err("local-only source preparation".into());
    }
    let profile = NationalProfile::frozen();
    let start = Instant::now();
    let summary = prepare(&inputs, &SourcePins::frozen(), &profile.limits, &|| {
        start.elapsed().as_secs() >= profile.resources.max_wall_seconds
    })?;
    println!("{}", serde_json::to_string(&summary)?);
    Ok(())
}
fn main() {
    if let Err(error) = run() {
        eprintln!("prepare_ev_sites: {error}");
        std::process::exit(2);
    }
}

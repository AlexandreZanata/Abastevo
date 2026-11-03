//! Local-only OSM reference CLI. No network, database or native engine.
//!
//! Usage:
//!   prepare_osm --input region.osm.pbf --output out/reference \
//!     [--bbox min_lon,min_lat,max_lon,max_lat] \
//!     [--max-input-bytes N] [--max-matched N]

use station_prep::osm_reference::{parse_file, write_run, BBox, Limits};

fn value(args: &[String], name: &str) -> Option<String> {
    args.iter()
        .position(|arg| arg == name)
        .and_then(|index| args.get(index + 1))
        .cloned()
}

fn fail(message: &str) -> ! {
    eprintln!("prepare_osm: {message}");
    std::process::exit(2);
}

fn national(args: &[String], input: &str, output: &str) {
    use station_prep::osm_reference::write_national_run;
    use station_prep::{
        ev_geography::StateIndex,
        osm_national::{census, NationalProfile},
    };
    use std::{path::Path, time::Instant};
    let profile = NationalProfile::frozen();
    if value(args, "--profile").as_deref() != Some(profile.profile_id.as_str()) {
        fail("unknown national profile");
    }
    let mut seen = std::collections::BTreeSet::new();
    for pair in args[1..].chunks(2) {
        if pair.len() != 2
            || !["--profile", "--input", "--output", "--boundaries"].contains(&pair[0].as_str())
            || !seen.insert(&pair[0])
        {
            fail("profile refuses overrides, unknown or repeated arguments");
        }
    }
    if Path::new(output).exists() {
        fail("national output already exists");
    }
    let boundaries = value(args, "--boundaries")
        .unwrap_or_else(|| fail("--boundaries required for national census"));
    let file = std::fs::File::open(boundaries).unwrap_or_else(|_| fail("cannot open boundaries"));
    use std::io::Read;
    let mut bytes = Vec::new();
    file.take((station_prep::ev_geography::MAX_BOUNDARY_BYTES + 1) as u64)
        .read_to_end(&mut bytes)
        .unwrap_or_else(|_| fail("cannot read boundaries"));
    let index = StateIndex::from_bytes(&bytes).unwrap_or_else(|e| fail(&e));
    let started = Instant::now();
    let run = parse_file(Path::new(input), &profile.limits, None, &|| {
        started.elapsed().as_secs() >= profile.resources.max_wall_seconds
    })
    .unwrap_or_else(|error| fail(&error.to_string()));
    let counts = census(&run, &index).unwrap_or_else(|e| fail(&e));
    write_national_run(Path::new(output), &run, &counts, &profile)
        .unwrap_or_else(|error| fail(&error.to_string()));
    println!(
        "{}",
        serde_json::json!({"places": run.places.len(), "quarantined": run.census.quarantined,
        "missing_geometry": run.census.missing_geometry, "profile": profile.profile_id})
    );
}

fn main() {
    let args: Vec<String> = std::env::args().collect();
    let input = value(&args, "--input").unwrap_or_else(|| fail("--input is required"));
    let output = value(&args, "--output").unwrap_or_else(|| fail("--output is required"));
    if args.iter().any(|arg| arg == "--profile") {
        national(&args, &input, &output);
        return;
    }
    let mut limits = Limits::default();
    if let Some(raw) = value(&args, "--max-input-bytes") {
        limits.max_input_bytes = raw
            .parse()
            .unwrap_or_else(|_| fail("invalid --max-input-bytes"));
    }
    if let Some(raw) = value(&args, "--max-matched") {
        limits.max_matched = raw
            .parse()
            .unwrap_or_else(|_| fail("invalid --max-matched"));
    }
    let bbox = value(&args, "--bbox").map(|raw| {
        let parts: Vec<f64> = raw
            .split(',')
            .filter_map(|part| part.parse().ok())
            .collect();
        if parts.len() != 4 {
            fail("--bbox requires min_lon,min_lat,max_lon,max_lat");
        }
        BBox {
            min_lon_deg: parts[0],
            min_lat_deg: parts[1],
            max_lon_deg: parts[2],
            max_lat_deg: parts[3],
        }
    });
    let run = parse_file(std::path::Path::new(&input), &limits, bbox, &|| false)
        .unwrap_or_else(|error| fail(&format!("{error}")));
    write_run(std::path::Path::new(&output), &run)
        .unwrap_or_else(|error| fail(&format!("{error}")));
    println!(
        "{{\"places\":{},\"decisions\":{},\"accepted\":{},\"quarantined\":{},\"duplicates\":{},\"missing_geometry\":{}}}",
        run.places.len(),
        run.decisions.len(),
        run.census.accepted,
        run.census.quarantined,
        run.census.duplicates,
        run.census.missing_geometry
    );
}

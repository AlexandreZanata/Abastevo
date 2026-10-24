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

fn main() {
    let args: Vec<String> = std::env::args().collect();
    let input = value(&args, "--input").unwrap_or_else(|| fail("--input is required"));
    let output = value(&args, "--output").unwrap_or_else(|| fail("--output is required"));
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

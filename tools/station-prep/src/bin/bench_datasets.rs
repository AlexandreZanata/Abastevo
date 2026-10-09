//! RST-11 dataset emitter: deterministic synthetic registry CSV plus
//! machine-readable oracle, alias table and request traces.
//!
//! Output goes to scratch files, never Git, except the tiny
//! correctness set committed under `contracts/testdata/station-prep/`.
//! Usage: bench_datasets <tiny|representative|stress-100k|stress-1M>
//!   <out.csv> [--aliases out.json] [--oracle out.json]
//!   [--trace <uniform|proportional|skew-80-20|hot-90> <total> <out.json>]
//!   [--serial-offset N]

use station_prep::{
    alias_table_json, generate, representative_profile, request_trace, stress_100k_profile,
    stress_1m_profile, tiny_profile, TraceDistribution,
};
use std::io::Write;

fn usage() -> ! {
    eprintln!(
        "usage: bench_datasets <tiny|representative|stress-100k|stress-1M> <out.csv> [--aliases out.json] [--oracle out.json] [--trace <uniform|proportional|skew-80-20|hot-90> <total> <out.json>] [--serial-offset N]"
    );
    std::process::exit(2);
}

fn distribution_for(raw: &str) -> TraceDistribution {
    match raw {
        "uniform" => TraceDistribution::Uniform,
        "proportional" => TraceDistribution::Proportional,
        "skew-80-20" => TraceDistribution::Skew80_20,
        "hot-90" => TraceDistribution::Hot90,
        _ => usage(),
    }
}

fn main() {
    let args: Vec<String> = std::env::args().collect();
    if args.len() < 3 {
        usage();
    }
    let mut profile = match args[1].as_str() {
        "tiny" => tiny_profile(),
        "representative" => representative_profile(),
        "stress-100k" => stress_100k_profile(),
        "stress-1M" => stress_1m_profile(),
        _ => usage(),
    };
    // Pre-pass: serial offsets shift establishment identities before
    // generation so editions stay disjoint.
    let mut scan = 3;
    while scan < args.len() {
        if args[scan] == "--serial-offset" {
            if scan + 1 >= args.len() {
                usage();
            }
            profile.serial_offset = args[scan + 1].parse().unwrap_or_else(|_| usage());
        }
        scan += 1;
    }
    let generated = generate(&profile);
    std::fs::write(&args[2], &generated.csv).expect("csv");
    let mut position = 3;
    let mut trace_spec: Option<(TraceDistribution, usize, String)> = None;
    while position < args.len() {
        match args[position].as_str() {
            "--aliases" => {
                position += 1;
                if position >= args.len() {
                    usage();
                }
                std::fs::write(&args[position], alias_table_json(profile.cities)).expect("aliases");
            }
            "--oracle" => {
                position += 1;
                if position >= args.len() {
                    usage();
                }
                let oracle = &generated.oracle;
                let mut per_city: Vec<String> = oracle
                    .per_city
                    .iter()
                    .map(|(ibge, count)| format!("\"{ibge}\": {count}"))
                    .collect();
                per_city.sort();
                let body = format!(
                    "{{\n  \"datasets_version\": \"{}\",\n  \"profile\": \"{}\",\n  \"seed\": {},\n  \"input\": {},\n  \"accepted\": {},\n  \"duplicates\": {},\n  \"quarantined\": {},\n  \"unique_cnpjs\": {},\n  \"checksum\": \"{}\",\n  \"csv_sha256\": \"{}\",\n  \"csv_bytes\": {},\n  \"history_per_1\": {},\n  \"history_per_10\": {},\n  \"history_per_100\": {},\n  \"with_location\": {},\n  \"without_location\": {},\n  \"long_field_rows\": {},\n  \"per_city\": {{{}}}\n}}\n",
                    station_prep::DATASETS_VERSION,
                    oracle.profile_id,
                    oracle.seed,
                    oracle.input,
                    oracle.accepted,
                    oracle.duplicates,
                    oracle.quarantined,
                    oracle.unique_cnpjs,
                    oracle.checksum,
                    oracle.csv_sha256,
                    oracle.csv_bytes,
                    oracle.history_per_1,
                    oracle.history_per_10,
                    oracle.history_per_100,
                    oracle.with_location,
                    oracle.without_location,
                    oracle.long_field_rows,
                    per_city.join(", ")
                );
                std::fs::write(&args[position], body).expect("oracle");
            }
            "--trace" => {
                if position + 3 >= args.len() {
                    usage();
                }
                let distribution = distribution_for(&args[position + 1]);
                let total: usize = args[position + 2].parse().expect("total");
                trace_spec = Some((distribution, total, args[position + 3].clone()));
                position += 3;
            }
            "--serial-offset" => {
                // Applied in the pre-pass above; skipped here.
                position += 1;
                if position >= args.len() {
                    usage();
                }
            }
            _ => usage(),
        }
        position += 1;
    }
    if let Some((distribution, total, path)) = trace_spec {
        let trace = request_trace(&generated.oracle, profile.cities, distribution, total, None);
        let mut entries: Vec<String> = trace
            .cities
            .iter()
            .map(|city| {
                format!(
                    "    {{\"ibge\": \"{}\", \"stations\": {}, \"stratum\": \"{}\", \"requests\": {}}}",
                    city.ibge, city.stations, city.stratum, city.requests
                )
            })
            .collect();
        entries.sort();
        let body = format!(
            "{{\n  \"workload_id\": \"{}\",\n  \"profile\": \"{}\",\n  \"distribution\": \"{}\",\n  \"total_requests\": {},\n  \"seed\": {},\n  \"hot_ibge\": {},\n  \"strata\": {{\"empty_cities\": {}, \"empty_requests\": {}, \"tiny_cities\": {}, \"tiny_requests\": {}, \"small_cities\": {}, \"small_requests\": {}, \"large_cities\": {}, \"large_requests\": {}}},\n  \"cities\": [\n{}\n  ]\n}}\n",
            trace.workload_id,
            trace.profile_id,
            trace.distribution,
            trace.total_requests,
            trace.seed,
            trace.hot_ibge.as_deref().map(|hot| format!("\"{hot}\"")).unwrap_or_else(|| "null".to_string()),
            trace.strata.empty_cities,
            trace.strata.empty_requests,
            trace.strata.tiny_cities,
            trace.strata.tiny_requests,
            trace.strata.small_cities,
            trace.strata.small_requests,
            trace.strata.large_cities,
            trace.strata.large_requests,
            entries.join(",\n")
        );
        std::fs::write(&path, body).expect("trace");
    }
    let oracle = &generated.oracle;
    let out = std::io::stdout();
    let mut out = std::io::BufWriter::new(out.lock());
    writeln!(
        out,
        "profile={} input={} accepted={} duplicates={} quarantined={} unique={} checksum={} csv_sha256={}",
        oracle.profile_id,
        oracle.input,
        oracle.accepted,
        oracle.duplicates,
        oracle.quarantined,
        oracle.unique_cnpjs,
        oracle.checksum,
        oracle.csv_sha256
    )
    .expect("stdout");
}

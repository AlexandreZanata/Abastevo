//! RST-13 preparation scaling driver: the complete offline pipeline
//! with per-stage wall timing. Stages mirror the campaign layers:
//! read, parse (normalize/municipality/dedup fused in the streaming
//! reader), PMQC parse, join triage, emit (serialize/sort/spill),
//! hash, manifest already inside emit, publish. Memory is measured
//! externally (`/usr/bin/time -v`); compile/setup time is excluded
//! and reported separately.
//!
//! Usage: bench_stages <registry.csv> <aliases.json> [--pmqc-n N]
//!   [--spill-rows N] [--max-rows N] [--max-bytes-mb N]
//!   [--scratch DIR] [--out-dir DIR] [--run-id ID]

use station_prep::{
    emit_run, join_candidates, parse_pmqc, parse_registry, write_outputs, AliasTable, EmitOptions,
    Limits, SourceMeta,
};
use std::time::Instant;

fn usage() -> ! {
    eprintln!(
        "usage: bench_stages <registry.csv> <aliases.json> [--pmqc-n N] [--spill-rows N] [--max-rows N] [--max-bytes-mb N] [--scratch DIR] [--out-dir DIR] [--run-id ID]"
    );
    std::process::exit(2);
}

fn hex_encode(bytes: &[u8]) -> String {
    const TABLE: &[u8; 16] = b"0123456789abcdef";
    let mut out = String::with_capacity(bytes.len() * 2);
    for byte in bytes {
        out.push(TABLE[(byte >> 4) as usize] as char);
        out.push(TABLE[(byte & 0x0F) as usize] as char);
    }
    out
}

fn main() {
    let args: Vec<String> = std::env::args().collect();
    if args.len() < 3 {
        usage();
    }
    let mut pmqc_n: usize = 5000;
    let mut spill_rows: usize = usize::MAX;
    let mut limits = Limits::default();
    let mut scratch = std::env::temp_dir().join(format!("bench-stages-{}", std::process::id()));
    let mut out_dir: Option<String> = None;
    let mut run_id = "bench-stages-run".to_string();
    let mut position = 3;
    while position < args.len() {
        let flag = args[position].as_str();
        let value = |offset: usize| args.get(offset).unwrap_or_else(|| usage()).to_string();
        match flag {
            "--pmqc-n" => pmqc_n = value(position + 1).parse().unwrap_or_else(|_| usage()),
            "--spill-rows" => spill_rows = value(position + 1).parse().unwrap_or_else(|_| usage()),
            "--max-rows" => {
                limits.max_rows = value(position + 1).parse().unwrap_or_else(|_| usage())
            }
            "--max-bytes-mb" => {
                limits.max_bytes = value(position + 1)
                    .parse::<u64>()
                    .unwrap_or_else(|_| usage())
                    << 20
            }
            "--scratch" => scratch = value(position + 1).into(),
            "--out-dir" => out_dir = Some(value(position + 1)),
            "--run-id" => run_id = value(position + 1),
            _ => usage(),
        }
        position += 2;
    }
    std::fs::create_dir_all(&scratch).expect("scratch");
    let out_path: std::path::PathBuf = match out_dir {
        Some(dir) => dir.into(),
        None => scratch.join("out"),
    };
    std::fs::create_dir_all(&out_path).expect("out dir");

    let started = Instant::now();
    let raw = std::fs::read(&args[1]).expect("registry csv");
    let read_ms = started.elapsed().as_secs_f64() * 1000.0;
    let input_bytes = raw.len();
    let alias_text =
        String::from_utf8(std::fs::read(&args[2]).expect("aliases")).expect("aliases utf-8");
    let table = AliasTable::from_json(&alias_text).expect("aliases");

    let stage = Instant::now();
    let batch = parse_registry(&args[1], raw.clone(), &limits, &table).expect("parse");
    let parse_ms = stage.elapsed().as_secs_f64() * 1000.0;

    // Deterministic synthetic PMQC extract anchored at accepted CNPJs.
    let mut pmqc_text = String::from("AmostraId;CnpjPosto;DataColeta;Latitude;Longitude\n");
    let take = pmqc_n.min(batch.accepted.len());
    for (index, row) in batch.accepted.iter().take(take).enumerate() {
        let lat = -23.55 + ((index % 300) as f64 - 150.0) / 1000.0;
        let lon = -46.63 + ((index % 500) as f64 - 250.0) / 1000.0;
        pmqc_text.push_str(&format!(
            "A-{index:06};{};2024-04-01;{lat:.6};{lon:.6}\n",
            row.source_key
        ));
    }
    let stage = Instant::now();
    let pmqc_raw = pmqc_text.into_bytes();
    let pmqc = parse_pmqc("bench-pmqc.csv", pmqc_raw.clone(), &limits).expect("pmqc");
    let pmqc_ms = stage.elapsed().as_secs_f64() * 1000.0;

    let stage = Instant::now();
    let joined = join_candidates(&batch.accepted, &pmqc.candidates);
    let join_ms = stage.elapsed().as_secs_f64() * 1000.0;

    let meta = SourceMeta::fingerprint(
        "registry-13col",
        &args[1],
        "bench-edition",
        1,
        &raw,
        batch.counts.input,
    );
    let options = EmitOptions {
        run_id,
        started_at: "2026-10-09T00:00:00Z".to_string(),
        ended_at: "2026-10-09T00:00:00Z".to_string(),
        spill_rows,
    };
    let pmqc_meta = SourceMeta::fingerprint(
        "pmqc",
        "bench-pmqc.csv",
        "bench-edition",
        1,
        &pmqc_raw,
        pmqc.counts.input,
    );
    let stage = Instant::now();
    let emitted = emit_run(
        Some((&meta, &batch)),
        Some((&pmqc_meta, &pmqc)),
        "bench-aliases",
        table.digest(),
        &options,
        &scratch,
    )
    .expect("emit");
    let emit_ms = stage.elapsed().as_secs_f64() * 1000.0;

    // Hash stage: the same SHA256-over-streams work the manifest
    // performs, timed separately for the campaign ledger.
    let stage = Instant::now();
    let mut hasher = sha2::Sha256::new();
    use sha2::Digest;
    hasher.update(&emitted.assertions);
    hasher.update(&emitted.candidates);
    hasher.update(&emitted.quarantine);
    let streams_hash = hex_encode(&hasher.finalize());
    let hash_ms = stage.elapsed().as_secs_f64() * 1000.0;

    let stage = Instant::now();
    write_outputs(&out_path, &emitted).expect("publish");
    let publish_ms = stage.elapsed().as_secs_f64() * 1000.0;
    let total_ms = started.elapsed().as_secs_f64() * 1000.0;

    let rows = batch.counts.input as f64;
    let mib = input_bytes as f64 / 1048576.0;
    let rate = |ms: f64| rows / (ms / 1000.0).max(1e-9);
    println!(
        "input_rows={} input_mib={:.2} accepted={} duplicates={} quarantined={} pmqc={} joined={}",
        batch.counts.input,
        mib,
        batch.counts.accepted,
        batch.counts.duplicates,
        batch.counts.quarantined,
        pmqc.counts.input,
        joined.candidates.len()
    );
    println!(
        "read_ms={:.1} parse_ms={:.1} pmqc_ms={:.1} join_ms={:.1} emit_ms={:.1} hash_ms={:.1} publish_ms={:.1} total_ms={:.1}",
        read_ms, parse_ms, pmqc_ms, join_ms, emit_ms, hash_ms, publish_ms, total_ms
    );
    println!(
        "parse_rows_per_sec={:.0} emit_rows_per_sec={:.0} read_mib_per_sec={:.1}",
        rate(parse_ms),
        rate(emit_ms),
        mib / (read_ms / 1000.0).max(1e-9)
    );
    println!(
        "assertions_bytes={} candidates_bytes={} quarantine_bytes={} streams_hash={} spill_runs={} spill_bytes={}",
        emitted.assertions.len(),
        emitted.candidates.len(),
        emitted.quarantine.len(),
        streams_hash,
        emitted.spill_runs,
        emitted.spill_bytes
    );
}

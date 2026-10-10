//! RST-07 parse benchmark driver: streaming parse of a generated CSV
//! plus alias table, printing counts, wall time and the overall checksum
//! (SHA256 over sorted accepted source keys) for cross-implementation
//! comparison. Memory is measured externally (/usr/bin/time -v).
//!
//! Usage: bench_parse <csv> <aliases.json> [max_rows] [max_bytes_mb]

use station_prep::{parse_registry, AliasTable, Limits};

fn main() {
    let args: Vec<String> = std::env::args().collect();
    if args.len() < 3 || args.len() > 5 {
        eprintln!("usage: bench_parse <csv> <aliases.json> [max_rows] [max_bytes_mb]");
        std::process::exit(2);
    }
    let mut limits = Limits::default();
    if args.len() >= 4 {
        limits.max_rows = args[3].parse().expect("max_rows");
    }
    if args.len() >= 5 {
        let megabytes: u64 = args[4].parse().expect("max_bytes_mb");
        limits.max_bytes = megabytes << 20;
    }
    let raw = std::fs::read(&args[1]).expect("csv");
    let alias_text =
        String::from_utf8(std::fs::read(&args[2]).expect("aliases")).expect("aliases utf-8");
    let table = AliasTable::from_json(&alias_text).expect("aliases");
    let started = std::time::Instant::now();
    let batch = parse_registry(&args[1], raw, &limits, &table).expect("parse");
    let elapsed = started.elapsed();
    let mut keys: Vec<&str> = batch
        .accepted
        .iter()
        .map(|row| row.source_key.as_str())
        .collect();
    keys.sort();
    let mut hasher = sha2::Sha256::new();
    use sha2::Digest;
    for key in &keys {
        hasher.update(key.as_bytes());
        hasher.update(b"\n");
    }
    let overall = hex_encode(&hasher.finalize());
    let rows = batch.counts.input as f64;
    println!(
        "input={} accepted={} duplicates={} quarantined={} elapsed_ms={} rows_per_sec={:.0} overall={}",
        batch.counts.input,
        batch.counts.accepted,
        batch.counts.duplicates,
        batch.counts.quarantined,
        elapsed.as_millis(),
        rows / elapsed.as_secs_f64().max(1e-9),
        overall
    );
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

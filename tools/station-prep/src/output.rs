//! Deterministic output: sorted JSONL streams, a hashed batch manifest
//! and atomic publication to a directory. Identical logical content
//! always yields identical bytes, whether rows sort in memory or spill
//! through temp runs first.
//!
//! Parse batches stay bounded by [`Limits::max_rows`](crate::Limits), so
//! the in-memory path is the default. The spill path exists for larger
//! measured snapshots (RST-07 decides when it pays): past `spill_rows`
//! per stream, sorted runs spill to caller-provided scratch space and
//! merge back in key order. Both paths are covered by the same
//! byte-equivalence tests.

use crate::pmqc::PmqcBatch;
use crate::registry::RegistryBatch;
use crate::types::{sha256_hex, Counts};
use serde::Serialize;
use std::collections::BTreeMap;
use std::io::{BufRead, BufReader, Write};
use std::path::{Path, PathBuf};

/// Parser and policy versions stamped on every manifest.
pub const PARSER_VERSION: &str = "station-prep-v0.2.0";
pub const POLICY_VERSION: &str = "station-policy-v1";

/// Published stream file names.
pub const ASSERTIONS_FILE: &str = "assertions.jsonl";
pub const CANDIDATES_FILE: &str = "candidates.jsonl";
pub const QUARANTINE_FILE: &str = "quarantine.jsonl";
pub const MANIFEST_FILE: &str = "manifest.json";

/// One consumed input for manifest provenance.
#[derive(Debug, Clone, Serialize, serde::Deserialize)]
pub struct SourceMeta {
    pub key: String,
    pub reference: String,
    pub edition: String,
    pub edition_seq: u64,
    pub sha256: String,
    pub bytes: u64,
    pub rows: usize,
    pub encoding: String,
}

impl SourceMeta {
    /// Fingerprint `raw` bytes as a manifest input entry.
    pub fn fingerprint(
        key: &str,
        reference: &str,
        edition: &str,
        edition_seq: u64,
        raw: &[u8],
        rows: usize,
    ) -> Self {
        Self {
            key: key.to_string(),
            reference: reference.to_string(),
            edition: edition.to_string(),
            edition_seq,
            sha256: sha256_hex(raw),
            bytes: raw.len() as u64,
            rows,
            encoding: "UTF-8 without BOM".to_string(),
        }
    }
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, serde::Deserialize)]
pub struct ManifestCounts {
    pub input: usize,
    pub accepted: usize,
    pub duplicates: usize,
    pub quarantined: usize,
}

impl From<Counts> for ManifestCounts {
    fn from(counts: Counts) -> Self {
        Self {
            input: counts.input,
            accepted: counts.accepted,
            duplicates: counts.duplicates,
            quarantined: counts.quarantined,
        }
    }
}

#[derive(Debug, Clone, Serialize, serde::Deserialize)]
pub struct MunicipalityReference {
    pub reference: String,
    pub reference_hash: String,
}

#[derive(Debug, Clone, Serialize, serde::Deserialize)]
pub struct Completeness {
    pub eof_validated: bool,
    pub expected_manifest: bool,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, serde::Deserialize)]
pub struct OutputEntry {
    pub path: String,
    pub sha256: String,
    pub bytes: u64,
    pub rows: usize,
}

/// Versioned batch manifest (`station-batch-v1` lineage). `inputs` keeps
/// insertion order; `counts` sorts by key for deterministic bytes.
#[derive(Debug, Clone, Serialize, serde::Deserialize)]
pub struct Manifest {
    pub format_version: String,
    pub run_id: String,
    pub parser_version: String,
    pub policy_version: String,
    pub municipality_reference: MunicipalityReference,
    pub started_at: String,
    pub ended_at: String,
    pub inputs: Vec<SourceMeta>,
    pub counts: BTreeMap<String, ManifestCounts>,
    pub completeness: Completeness,
    pub outputs: Vec<OutputEntry>,
}

impl Manifest {
    pub fn to_json(&self) -> String {
        serde_json::to_string_pretty(self).expect("manifest must serialize")
    }

    pub fn from_json(raw: &str) -> Result<Self, serde_json::Error> {
        serde_json::from_str(raw)
    }
}

/// Emission options. `spill_rows` bounds one in-memory sort run;
/// `usize::MAX` keeps the pure in-memory path.
#[derive(Debug, Clone)]
pub struct EmitOptions {
    pub run_id: String,
    pub started_at: String,
    pub ended_at: String,
    pub spill_rows: usize,
}

/// One emitted run: stream bytes plus the manifest describing them.
#[derive(Debug, Clone)]
pub struct EmittedRun {
    pub assertions: Vec<u8>,
    pub candidates: Vec<u8>,
    pub quarantine: Vec<u8>,
    pub manifest: Manifest,
}

impl EmittedRun {
    pub fn manifest_json(&self) -> String {
        self.manifest.to_json()
    }
}

/// Write `(sort key, JSONL line)` pairs in key order. Past `spill_rows`
/// pairs, sorted runs spill to `scratch` and merge back; otherwise the
/// whole stream sorts in memory with no files touched.
fn write_sorted(
    pairs: Vec<(String, String)>,
    writer: &mut dyn Write,
    spill_rows: usize,
    scratch: &Path,
    stream: &str,
) -> std::io::Result<usize> {
    if pairs.len() <= spill_rows {
        let mut pairs = pairs;
        pairs.sort();
        for (_, line) in &pairs {
            writer.write_all(line.as_bytes())?;
            writer.write_all(b"\n")?;
        }
        return Ok(pairs.len());
    }
    std::fs::create_dir_all(scratch)?;
    let mut runs: Vec<PathBuf> = Vec::new();
    for (index, chunk) in pairs.chunks(spill_rows).enumerate() {
        let mut run: Vec<(String, String)> = chunk.to_vec();
        run.sort();
        let path = scratch.join(format!("{stream}-run-{index:06}.tmp"));
        let mut file = std::fs::File::create(&path)?;
        // Length-prefixed framing: sort keys may hold any byte except
        // `\n`, so the key length rides up front instead of a separator.
        for (key, line) in &run {
            file.write_all(format!("{:08x}", key.len()).as_bytes())?;
            file.write_all(key.as_bytes())?;
            file.write_all(line.as_bytes())?;
            file.write_all(b"\n")?;
        }
        file.flush()?;
        runs.push(path);
    }
    let result = merge_runs(&runs, writer);
    for path in &runs {
        let _ = std::fs::remove_file(path);
    }
    result
}

/// K-way merge of spilled length-prefixed runs in key order.
fn merge_runs(runs: &[PathBuf], writer: &mut dyn Write) -> std::io::Result<usize> {
    /// Split one framed run line into its sort key and JSONL payload.
    fn split_framed(raw: &str) -> Option<(String, String)> {
        let (hex, rest) = raw.split_at_checked(8)?;
        let len = usize::from_str_radix(hex, 16).ok()?;
        let (key, line) = rest.split_at_checked(len)?;
        Some((key.to_string(), line.trim_end_matches('\n').to_string()))
    }

    let mut readers: Vec<(BufReader<std::fs::File>, String, String, bool)> = Vec::new();
    for path in runs {
        let mut reader = BufReader::new(std::fs::File::open(path)?);
        let mut raw = String::new();
        let (key, line, live) = if reader.read_line(&mut raw)? > 0 {
            split_framed(&raw)
                .map(|(key, line)| (key, line, true))
                .unwrap_or_else(|| (String::new(), String::new(), false))
        } else {
            (String::new(), String::new(), false)
        };
        readers.push((reader, key, line, live));
    }
    let mut written = 0;
    loop {
        let mut best: Option<usize> = None;
        for (position, (_, key, _, live)) in readers.iter().enumerate() {
            if !live {
                continue;
            }
            if best.is_none_or(|current| *key < readers[current].1) {
                best = Some(position);
            }
        }
        let Some(position) = best else {
            break;
        };
        let (reader, _, line, live) = &mut readers[position];
        writer.write_all(line.as_bytes())?;
        writer.write_all(b"\n")?;
        written += 1;
        let mut raw = String::new();
        if reader.read_line(&mut raw)? == 0 {
            *live = false;
        } else if let Some((key, next)) = split_framed(&raw) {
            readers[position].1 = key;
            readers[position].2 = next;
        } else {
            return Err(std::io::Error::new(
                std::io::ErrorKind::InvalidData,
                "corrupt spill run",
            ));
        }
    }
    Ok(written)
}

/// Emit one run combining an optional registry batch and an optional
/// PMQC batch. Quarantine rows merge into a single stream ordered by
/// `(source, locator)`. Accepted rows keep their parse-time stable
/// order and are re-sorted here only to prove spill equivalence.
#[allow(clippy::too_many_arguments)]
pub fn emit_run(
    registry: Option<(&SourceMeta, &RegistryBatch)>,
    pmqc: Option<(&SourceMeta, &PmqcBatch)>,
    alias_reference: &str,
    alias_digest: &str,
    options: &EmitOptions,
    scratch: &Path,
) -> std::io::Result<EmittedRun> {
    let mut manifest = Manifest {
        format_version: "station-batch-v1".to_string(),
        run_id: options.run_id.clone(),
        parser_version: PARSER_VERSION.to_string(),
        policy_version: POLICY_VERSION.to_string(),
        municipality_reference: MunicipalityReference {
            reference: alias_reference.to_string(),
            reference_hash: alias_digest.to_string(),
        },
        started_at: options.started_at.clone(),
        ended_at: options.ended_at.clone(),
        inputs: Vec::new(),
        counts: BTreeMap::new(),
        completeness: Completeness {
            eof_validated: true,
            expected_manifest: true,
        },
        outputs: Vec::new(),
    };

    let mut assertions: Vec<u8> = Vec::new();
    let mut candidates: Vec<u8> = Vec::new();
    let mut quarantine_pairs: Vec<(String, String)> = Vec::new();

    if let Some((meta, batch)) = registry {
        manifest.inputs.push(meta.clone());
        manifest
            .counts
            .insert(meta.key.clone(), batch.counts.into());
        let pairs: Vec<(String, String)> = batch
            .accepted
            .iter()
            .map(|row| {
                (
                    format!("{}\x1f{}", row.source_key, row.checksum),
                    row.to_jsonl(),
                )
            })
            .collect();
        write_sorted(
            pairs,
            &mut assertions,
            options.spill_rows,
            scratch,
            "assertions",
        )?;
        for row in &batch.quarantine {
            quarantine_pairs.push((
                format!("{}\x1f{}", row.source, row.row_locator),
                row.to_jsonl(),
            ));
        }
    }
    if let Some((meta, batch)) = pmqc {
        manifest.inputs.push(meta.clone());
        manifest
            .counts
            .insert(meta.key.clone(), batch.counts.into());
        let pairs: Vec<(String, String)> = batch
            .candidates
            .iter()
            .map(|row| {
                (
                    format!(
                        "{}\x1f{}\x1f{}",
                        row.source_key, row.observed_at, row.sample_ref
                    ),
                    row.to_jsonl(),
                )
            })
            .collect();
        write_sorted(
            pairs,
            &mut candidates,
            options.spill_rows,
            scratch,
            "candidates",
        )?;
        for row in &batch.quarantine {
            quarantine_pairs.push((
                format!("{}\x1f{}", row.source, row.row_locator),
                row.to_jsonl(),
            ));
        }
    }
    let mut quarantine: Vec<u8> = Vec::new();
    write_sorted(
        quarantine_pairs,
        &mut quarantine,
        options.spill_rows,
        scratch,
        "quarantine",
    )?;

    for (path, bytes) in [
        (ASSERTIONS_FILE, &assertions),
        (CANDIDATES_FILE, &candidates),
        (QUARANTINE_FILE, &quarantine),
    ] {
        manifest.outputs.push(OutputEntry {
            path: path.to_string(),
            sha256: sha256_hex(bytes),
            bytes: bytes.len() as u64,
            rows: bytes.iter().filter(|byte| **byte == b'\n').count(),
        });
    }
    Ok(EmittedRun {
        assertions,
        candidates,
        quarantine,
        manifest,
    })
}

/// Publish emitted bytes to `dir` atomically: every stream lands as a
/// temp file first and renames into place only after a full flush. Any
/// failure leaves no partial final file behind.
pub fn write_outputs(dir: &Path, emitted: &EmittedRun) -> std::io::Result<()> {
    for (name, bytes) in [
        (ASSERTIONS_FILE, emitted.assertions.as_slice()),
        (CANDIDATES_FILE, emitted.candidates.as_slice()),
        (QUARANTINE_FILE, emitted.quarantine.as_slice()),
    ] {
        let tmp = dir.join(format!("{name}.tmp"));
        std::fs::write(&tmp, bytes)?;
        std::fs::rename(&tmp, dir.join(name))?;
    }
    let tmp = dir.join(format!("{MANIFEST_FILE}.tmp"));
    std::fs::write(&tmp, emitted.manifest_json())?;
    std::fs::rename(&tmp, dir.join(MANIFEST_FILE))?;
    Ok(())
}

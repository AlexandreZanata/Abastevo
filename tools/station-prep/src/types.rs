//! Shared batch types: bounds, row accounting, run states and the frozen
//! typed quarantine row. Row accounting always reconciles:
//! `accepted + duplicates + quarantined == input`.

use serde::Serialize;

/// Bounds for one local-file parse. Past the caps the run is refused so a
/// corrupt snapshot cannot exhaust memory. The full spill/merge strategy
/// for larger snapshots belongs to RST-03.
#[derive(Debug, Clone, Copy)]
pub struct Limits {
    /// Refuse inputs larger than this many bytes.
    pub max_bytes: u64,
    /// Refuse inputs with more data rows than this.
    pub max_rows: usize,
    /// Quarantine rows holding a single field larger than this many bytes.
    pub max_field_bytes: usize,
}

impl Default for Limits {
    fn default() -> Self {
        Self {
            max_bytes: 100 << 20,
            max_rows: 500_000,
            max_field_bytes: 64 << 10,
        }
    }
}

/// Per-input row accounting. Every input row lands in exactly one bucket.
#[derive(Debug, Clone, Copy, Default, PartialEq, Eq)]
pub struct Counts {
    pub input: usize,
    pub accepted: usize,
    pub duplicates: usize,
    pub quarantined: usize,
}

/// Terminal state of a parsed batch. `parse_*` returns `Ok` for both:
/// only transport-level failures (oversize, row cap, truncation, bad
/// encoding) are `Err`, so a failure never yields partial output.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum RunState {
    Complete,
    Quarantined,
}

/// Frozen quarantine reasons (`station-quarantine-v1`). `invalid_field`
/// covers malformed non-identity fields (empty business name, malformed
/// UF, empty sample reference); every other reason matches RST-01.
pub mod reason {
    pub const INVALID_CNPJ: &str = "invalid_cnpj";
    pub const UNKNOWN_CITY: &str = "unknown_city";
    pub const AMBIGUOUS_MUNICIPALITY: &str = "ambiguous_municipality";
    pub const INVALID_POINT: &str = "invalid_point";
    pub const SUSPECT_SWAP: &str = "suspect_swap";
    pub const OUT_OF_BRAZIL: &str = "out_of_brazil";
    pub const DATE_REVERSAL: &str = "date_reversal";
    pub const HEADER_MISMATCH: &str = "header_mismatch";
    pub const RECORD_TOO_LARGE: &str = "record_too_large";
    pub const OLDER_CONFLICTING_EVIDENCE: &str = "older_conflicting_evidence";
    pub const INVALID_FIELD: &str = "invalid_field";
}

/// One quarantined row with a stable locator. `row_locator` is
/// `file:logical-record` where the header is record 1, or `file:header`
/// for run-level header quarantines.
#[derive(Debug, Clone, Serialize)]
pub struct QuarantineRow {
    pub schema_version: &'static str,
    pub source: &'static str,
    pub row_locator: String,
    pub reason: &'static str,
    pub detail: String,
    pub source_key: Option<String>,
}

impl QuarantineRow {
    pub fn to_jsonl(&self) -> String {
        serde_json::to_string(self).expect("quarantine row must serialize")
    }
}

//! `station-prep`: offline preparation of official fuel-station records
//! into validated, deterministic batches for the Go-owned staging loader.
//!
//! RST-02 implements the pure local-file parser only: `;`-delimited
//! streaming CSV to typed rows with bounded memory, UTF-8/BOM handling,
//! frozen header matching, full CNPJ text validation (numeric and
//! alphanumeric vectors, leading zeroes preserved) and municipality
//! resolution through a versioned alias table. Every input row is
//! accepted, counted as a within-run duplicate, or quarantined with a
//! typed reason and a stable row locator; the counts always reconcile.
//! Transport failures (oversize, row cap, truncation, bad encoding)
//! return `Err` with no partial output. There is no network, no database
//! and no automatic promotion of coordinate candidates.

pub mod cnpj;
pub mod municipality;
pub mod pmqc;
pub mod registry;
pub mod types;

pub use cnpj::{normalize_cnpj, InvalidCnpj};
pub use municipality::{AliasError, AliasTable, MunicipalityError};
pub use pmqc::{parse_pmqc, PmqcBatch, PmqcCandidate, PmqcError};
pub use registry::{parse_registry, RegistryBatch, RegistryError, RegistryRow};
pub use types::{Counts, Limits, QuarantineRow, RunState};

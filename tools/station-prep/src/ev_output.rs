//! Frozen prepared-envelope serialization (EE-03 shapes, EE-06 verified).
//!
//! Entities, decisions and manifests use the exact frozen field shapes:
//! field provenance as arrays, uppercase outcomes, integer root indices
//! and null (never empty-string) optionals. Canonical bytes follow
//! ev-json-v1; content_sha256 binds the manifest without itself.
use crate::ev_contract::canonical_bytes;
use serde_json::{json, Value};
use sha2::{Digest, Sha256};

/// Build one entity record in the frozen shape.
#[allow(clippy::too_many_arguments)]
pub fn entity_record(
    kind: &str,
    key: &str,
    parent_key: Option<&str>,
    external_id: &str,
    unit: &str,
    payload: Value,
    provenance_fields: &[&str],
    rights_sha256: &str,
    clocks: Value,
    assertion: &str,
) -> Value {
    let provenance: Vec<Value> = provenance_fields
        .iter()
        .map(|field| json!({"field": field, "path": format!("/{field}")}))
        .collect();
    json!({
        "kind": kind,
        "key": key,
        "parent_key": parent_key,
        "external_id": external_id,
        "unit": unit,
        "payload": payload,
        "field_provenance": provenance,
        "rights_sha256": rights_sha256,
        "source_clocks": clocks,
        "assertion_sha256": assertion,
    })
}

/// Build one decision record in the frozen shape. Outcome is uppercase;
#[allow(clippy::too_many_arguments)]
/// root is the input index; reason and duplicate_of are null unless set.
pub fn decision_record(
    unit: &str,
    kind: &str,
    source_file: &str,
    root: usize,
    pointer: &str,
    parent_unit: Option<String>,
    outcome: &str,
    key: Option<String>,
    external_id: &str,
    assertion: Option<String>,
    duplicate_of: Option<String>,
    reason: Option<String>,
    raw_sha256: &str,
) -> Value {
    json!({
        "unit": unit,
        "kind": kind,
        "source_file": source_file,
        "root": root,
        "pointer": pointer,
        "parent_unit": parent_unit,
        "outcome": outcome,
        "key": key,
        "external_id": external_id,
        "assertion_sha256": assertion,
        "duplicate_of": duplicate_of,
        "reason": reason,
        "raw_sha256": raw_sha256,
    })
}

/// Hash bytes as lowercase hex SHA-256.
pub fn hex_sha256(bytes: &[u8]) -> String {
    format!("{:x}", Sha256::digest(bytes))
}

/// Build the frozen manifest with content_sha256 bound over the canonical
#[allow(clippy::too_many_arguments)]
/// manifest minus itself. Counts cover all five kinds (zero where absent).
pub fn manifest_record(
    format: &str,
    profile_id: &str,
    profile_sha256: &str,
    rights_sha256: &str,
    source_revision: &str,
    reference_sha256: Option<String>,
    inputs: Vec<Value>,
    artifacts: Value,
    counts: Value,
) -> Result<Value, String> {
    let mut manifest = json!({
        "format": format,
        "byte_profile": "ev-json-v1",
        "identity_profile": "ev-source-key-v1",
        "privacy_profile": "ev-minimal-v1",
        "profile_id": profile_id,
        "profile_sha256": profile_sha256,
        "rights_sha256": rights_sha256,
        "source_revision": source_revision,
        "reference_sha256": reference_sha256,
        "inputs": inputs,
        "artifacts": artifacts,
        "counts": counts,
    });
    let content = canonical_bytes(&manifest).map_err(|e| e.to_string())?;
    let digest = hex_sha256(&content);
    manifest["content_sha256"] = Value::String(digest);
    Ok(manifest)
}

/// Zeroed per-kind census covering all five entity kinds.
pub fn zero_census() -> Value {
    let zero = json!({"input": 0, "accepted": 0, "duplicates": 0, "quarantined": 0});
    json!({
        "PLACE": zero,
        "EVSE": zero.clone(),
        "CONNECTOR": zero.clone(),
        "CONNECTION_GROUP": zero.clone(),
        "VEHICLE_REFERENCE": zero,
    })
}

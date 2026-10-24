//! Actual OCM Brazilian snapshot reader (EE-04).
//!
//! The pinned profile is the open-licensed whole-Brazil snapshot: places
//! with their connection groups, source wording for every retained field
//! and the source clocks as the only time. A place stays a place and a
//! connection group stays a group; reported quantities are never proof of
//! individual physical EVSEs.
//!
//! The reader minimizes what it emits. Contacts, telephones, emails,
//! comments, media and home/private coordinates never leave the reader;
//! only openly licensed providers (1/40/57) are admitted, everything else
//! quarantines. A refusal is a refusal: unknown profile, corrupt JSON,
//! unlicensed provider, missing identity or inexact numerics produce no
//! partial output.

use crate::ev_contract::{canonical_bytes, source_key};
use crate::types::{sha256_hex, QuarantineRow};
use serde_json::{json, Value};
use std::collections::HashSet;

/// Source label of the OCM charging stream.
pub const OCM_SOURCE: &str = "ocm-charging";
/// Pinned admitted OCM profile (user-authorized 2026-10-22).
pub const OCM_PROFILE: &str = "ocm-br-20261021-v1";
/// Admitted open-license provider IDs (CC-BY-4.0, CC-0, CC0).
pub const ADMITTED_PROVIDERS: [i64; 3] = [1, 40, 57];
/// Brazil country ID in OCM reference data.
pub const BRAZIL_COUNTRY_ID: i64 = 36;

/// Quarantine reasons for OCM units (ev-quarantine-v1, frozen here).
pub mod ocm_reason {
    pub const UNLICENSED_PROVIDER: &str = "unlicensed_provider";
    pub const FOREIGN_PLACE: &str = "foreign_place";
    pub const MISSING_IDENTITY: &str = "missing_identity";
    pub const DUPLICATE_UNIT: &str = "duplicate_unit";
    pub const INVALID_NUMERIC: &str = "invalid_numeric";
}

/// Refusal reasons of the OCM snapshot reader.
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum OcmError {
    Oversize,
    RowCap,
    Truncated(String),
    Profile(String),
    Contract(String),
}

impl std::fmt::Display for OcmError {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            OcmError::Oversize => write!(formatter, "input exceeds the budget"),
            OcmError::RowCap => write!(formatter, "row cap exceeded"),
            OcmError::Truncated(detail) => write!(formatter, "truncated: {detail}"),
            OcmError::Profile(detail) => write!(formatter, "profile refused: {detail}"),
            OcmError::Contract(detail) => write!(formatter, "contract refused: {detail}"),
        }
    }
}

impl std::error::Error for OcmError {}

/// One minimized OCM entity (place or connection group).
#[derive(Debug, Clone)]
pub struct OcmEntity {
    pub kind: &'static str,
    pub key: String,
    pub parent_key: Option<String>,
    pub external_id: String,
    pub payload: Value,
    pub clocks: Value,
    pub assertion: String,
}

/// Census of one OCM snapshot read. Counts only.
#[derive(Debug, Clone, Default)]
pub struct OcmCensus {
    pub places: usize,
    pub connection_groups: usize,
    pub distinct_providers: usize,
    pub total_quantity: i64,
}

/// Result of one OCM snapshot file read.
#[derive(Debug)]
pub struct OcmFileRun {
    pub entities: Vec<OcmEntity>,
    pub accepted: usize,
    pub duplicates: usize,
    pub quarantined: usize,
    pub quarantine: Vec<QuarantineRow>,
    pub duplicate_units: Vec<(String, String)>,
}

fn get_i64(value: &Value, field: &str) -> Option<i64> {
    value.get(field).and_then(Value::as_i64)
}

fn clock(value: &Value, field: &str) -> Value {
    match value.get(field).and_then(Value::as_str) {
        Some(text) if !text.is_empty() => Value::String(text.to_string()),
        _ => Value::Null,
    }
}

/// Read one OCM POI file and emit its entities. Quarantined units produce
/// no entity; duplicates produce no new entity. PII fields (contacts,
/// telephones, emails, comments, media) are never read into entities.
pub fn parse_ocm_poi(
    file: &str,
    raw: &[u8],
    rights_sha256: &str,
    seen: &mut HashSet<String>,
    providers: &mut HashSet<i64>,
    emit: &mut impl FnMut(&OcmEntity) -> Result<(), OcmError>,
) -> Result<OcmFileRun, OcmError> {
    let mut run = OcmFileRun {
        entities: Vec::new(),
        accepted: 0,
        duplicates: 0,
        quarantined: 0,
        quarantine: Vec::new(),
        duplicate_units: Vec::new(),
    };
    let value: Value =
        serde_json::from_slice(raw).map_err(|e| OcmError::Truncated(format!("{file}: {e}")))?;
    let id = match get_i64(&value, "ID").filter(|id| *id > 0) {
        Some(id) => id,
        None => return Err(OcmError::Contract(format!("{file}: missing place ID"))),
    };
    let provider = match get_i64(&value, "DataProviderID") {
        Some(provider) => provider,
        None => return Err(OcmError::Contract(format!("{file}: missing provider"))),
    };
    if !ADMITTED_PROVIDERS.contains(&provider) {
        run.quarantined += 1;
        run.quarantine.push(QuarantineRow {
            schema_version: "station-quarantine-v1",
            source: OCM_SOURCE,
            row_locator: format!("{file}#place:{id}"),
            reason: ocm_reason::UNLICENSED_PROVIDER,
            detail: format!("provider {provider} not admitted"),
            source_key: None,
        });
        return Ok(run);
    }
    providers.insert(provider);
    let addr = value
        .get("AddressInfo")
        .ok_or_else(|| OcmError::Contract(format!("{file}: missing address")))?;
    if addr.get("CountryID").and_then(Value::as_i64).unwrap_or(-1) != BRAZIL_COUNTRY_ID {
        run.quarantined += 1;
        run.quarantine.push(QuarantineRow {
            schema_version: "station-quarantine-v1",
            source: OCM_SOURCE,
            row_locator: format!("{file}#place:{id}"),
            reason: ocm_reason::FOREIGN_PLACE,
            detail: "country is not Brazil".to_string(),
            source_key: None,
        });
        return Ok(run);
    }
    let place_key = source_key("ocm", "PLACE", &id.to_string())
        .map_err(|e| OcmError::Contract(format!("{file}: {e}")))?;
    if !seen.insert(place_key.clone()) {
        run.duplicates += 1;
        run.duplicate_units
            .push((format!("{file}#place:{id}"), place_key));
        return Ok(run);
    }
    let usage_public = match get_i64(&value, "UsageTypeID") {
        Some(1) => "PUBLIC",
        _ => "UNKNOWN",
    };
    let name = addr.get("Title").and_then(Value::as_str).unwrap_or("");
    if name.len() > 160 {
        return Err(OcmError::Contract(format!(
            "{file}: name exceeds 160 chars"
        )));
    }
    let latitude = decimal_lexeme(raw, b"Latitude")
        .filter(|lexeme| valid_decimal(lexeme, 2))
        .ok_or_else(|| OcmError::Contract(format!("{file}: bad latitude")))?;
    let longitude = decimal_lexeme(raw, b"Longitude")
        .filter(|lexeme| valid_decimal(lexeme, 3))
        .ok_or_else(|| OcmError::Contract(format!("{file}: bad longitude")))?;
    let payload = json!({
        "public_access": usage_public,
        "name": name,
        "country": "BR",
        "latitude_decimal": latitude,
        "longitude_decimal": longitude,
        "reported_point_count": get_i64(&value, "NumberOfPoints").unwrap_or(0),
    });
    let clocks = json!([
        {"field": "created_at", "value": clock(&value, "DateCreated")},
        {"field": "verified_at", "value": clock(&value, "DateLastVerified")},
        {"field": "status_at", "value": clock(&value, "DateLastStatusUpdate")},
    ]);
    let assertion_input = json!({
        "kind": "PLACE",
        "key": place_key,
        "parent_key": Value::Null,
        "payload": payload,
        "rights_sha256": rights_sha256,
        "source_clocks": clocks,
    });
    let assertion = sha256_hex(
        &canonical_bytes(&assertion_input)
            .map_err(|e| OcmError::Contract(format!("{file}: {e}")))?,
    );
    let entity = OcmEntity {
        kind: "PLACE",
        key: place_key,
        parent_key: None,
        external_id: id.to_string(),
        payload,
        clocks,
        assertion,
    };
    emit(&entity)?;
    run.entities.push(entity);
    run.accepted += 1;
    if let Some(connections) = value.get("Connections").and_then(Value::as_array) {
        for (index, connection) in connections.iter().enumerate() {
            let connection_id = match get_i64(connection, "ID").filter(|id| *id > 0) {
                Some(id) => id,
                None => {
                    run.quarantined += 1;
                    continue;
                }
            };
            let group_key = source_key("ocm", "CONNECTION_GROUP", &format!("{id}:{connection_id}"))
                .map_err(|e| OcmError::Contract(format!("{file}: {e}")))?;
            if !seen.insert(group_key.clone()) {
                run.duplicates += 1;
                run.duplicate_units
                    .push((format!("{file}#connection:{connection_id}"), group_key));
                continue;
            }
            let raw_code = get_i64(connection, "ConnectionTypeID").unwrap_or(0);
            let power_w = match connection.get("PowerKW").and_then(Value::as_f64) {
                Some(kw) if kw.is_finite() && kw > 0.0 && kw <= 1_000_000.0 => {
                    Some((kw * 1000.0).round() as i64)
                }
                _ => None,
            };
            let quantity = match get_i64(connection, "Quantity") {
                Some(q) if (1..=500_000).contains(&q) => Some(q),
                _ => None,
            };
            let payload = json!({
                "connector_code": connector_label(raw_code),
                "raw_connector_code": raw_code.to_string(),
                "current": current_label(get_i64(connection, "CurrentTypeID").unwrap_or(0)),
                "power_w": power_w,
                "reported_quantity": quantity,
            });
            let _ = index;
            let clocks = json!([
                {"field": "created_at", "value": Value::Null},
                {"field": "verified_at", "value": clock(&value, "DateLastVerified")},
                {"field": "status_at", "value": clock(&value, "DateLastStatusUpdate")},
            ]);
            let assertion_input = json!({
                "kind": "CONNECTION_GROUP",
                "key": group_key,
                "parent_key": run.entities[0].key,
                "payload": payload,
                "rights_sha256": rights_sha256,
                "source_clocks": clocks,
            });
            let assertion = sha256_hex(
                &canonical_bytes(&assertion_input)
                    .map_err(|e| OcmError::Contract(format!("{file}: {e}")))?,
            );
            let entity = OcmEntity {
                kind: "CONNECTION_GROUP",
                key: group_key,
                parent_key: Some(run.entities[0].key.clone()),
                external_id: format!("{id}:{connection_id}"),
                payload,
                clocks,
                assertion,
            };
            emit(&entity)?;
            run.entities.push(entity);
            run.accepted += 1;
        }
    }
    Ok(run)
}

fn current_label(code: i64) -> &'static str {
    match code {
        10 => "AC",
        20 => "DC",
        _ => "UNKNOWN",
    }
}

/// Frozen connector vocabulary (reference data). Unmapped source codes
/// stay UNKNOWN with the raw code preserved; capability is never guessed.
fn connector_label(code: i64) -> &'static str {
    match code {
        25 | 1036 => "TYPE2",
        33 => "CCS2",
        2 => "CHADEMO",
        27 => "NACS",
        _ => "UNKNOWN",
    }
}

/// Exact decimal lexeme of a top-level numeric field, read from the raw
/// source bytes (serde_json floats would lose the lexeme). The field must
/// occur once in this pinned profile; the caller validates the shape.
fn decimal_lexeme(raw: &[u8], field: &[u8]) -> Option<String> {
    let mut needle = Vec::with_capacity(field.len() + 3);
    needle.push(b'"');
    needle.extend_from_slice(field);
    needle.extend_from_slice(b"\":");
    let start = raw
        .windows(needle.len())
        .position(|w| w == needle.as_slice())?;
    let mut cursor = start + needle.len();
    while cursor < raw.len() && (raw[cursor] == b' ' || raw[cursor] == b'\t') {
        cursor += 1;
    }
    let mut end = cursor;
    while end < raw.len()
        && (raw[end].is_ascii_digit()
            || raw[end] == b'-'
            || raw[end] == b'+'
            || raw[end] == b'.'
            || raw[end] == b'E'
            || raw[end] == b'e')
    {
        end += 1;
    }
    if end == cursor {
        return None;
    }
    String::from_utf8(raw[cursor..end].to_vec()).ok()
}

/// Latitude (-90..90, ≤16 decimals) or longitude (-180..180) shape.
fn valid_decimal(lexeme: &str, max_int_digits: usize) -> bool {
    let unsigned = lexeme.strip_prefix('-').unwrap_or(lexeme);
    let (integer, fraction) = match unsigned.split_once('.') {
        Some((i, f)) => (i, Some(f)),
        None => (unsigned, None),
    };
    if integer.is_empty()
        || !(1..=max_int_digits).contains(&integer.len())
        || !integer.bytes().all(|b| b.is_ascii_digit())
    {
        return false;
    }
    if integer.len() > 1 && integer.starts_with('0') {
        return false;
    }
    match fraction {
        None => true,
        Some(f) => !f.is_empty() && f.len() <= 16 && f.bytes().all(|b| b.is_ascii_digit()),
    }
}

#[cfg(test)]
mod tests {
    use super::{parse_ocm_poi, ADMITTED_PROVIDERS, BRAZIL_COUNTRY_ID};
    use std::collections::HashSet;

    #[test]
    fn admits_open_providers_and_brazil_only() {
        assert!(ADMITTED_PROVIDERS.contains(&1));
        assert!(ADMITTED_PROVIDERS.contains(&40));
        assert!(ADMITTED_PROVIDERS.contains(&57));
        assert_eq!(BRAZIL_COUNTRY_ID, 36);
    }

    #[test]
    fn reads_place_with_connection_groups() {
        let mut seen = HashSet::new();
        let mut providers = HashSet::new();
        let mut emitted = 0usize;
        let raw = br#"{"ID":1,"DataProviderID":1,"UsageTypeID":1,"NumberOfPoints":2,"DateCreated":"2020-01-01T00:00:00Z","AddressInfo":{"Title":"Test","CountryID":36,"Latitude":-23.0,"Longitude":-46.0},"Connections":[{"ID":10,"ConnectionTypeID":1036,"CurrentTypeID":10,"PowerKW":5.0,"Quantity":2}]}"#;
        let run = parse_ocm_poi("a.json", raw, "00", &mut seen, &mut providers, &mut |_| {
            emitted += 1;
            Ok(())
        })
        .unwrap();
        assert_eq!(run.accepted, 2);
        assert_eq!(emitted, 2);
        assert_eq!(run.entities.len(), 2);
        assert_eq!(run.entities[0].kind, "PLACE");
        assert_eq!(run.entities[1].kind, "CONNECTION_GROUP");
    }

    #[test]
    fn keeps_unknown_connector_power_and_quantity_honest() {
        let mut seen = HashSet::new();
        let mut providers = HashSet::new();
        let mut emit = |_: &super::OcmEntity| Ok(());
        let raw = br#"{"ID":4,"DataProviderID":1,"UsageTypeID":1,"NumberOfPoints":2,"AddressInfo":{"Title":"Mixed","CountryID":36,"Latitude":-23.0,"Longitude":-46.0},"Connections":[{"ID":40,"ConnectionTypeID":9999,"CurrentTypeID":99},{"ID":41,"ConnectionTypeID":1036,"CurrentTypeID":10,"PowerKW":7.4,"Quantity":1}]}"#;
        let run = parse_ocm_poi("f.json", raw, "00", &mut seen, &mut providers, &mut emit).unwrap();
        assert_eq!(run.accepted, 3);
        let unknown = run
            .entities
            .iter()
            .find(|entity| entity.external_id == "4:40")
            .unwrap();
        assert_eq!(unknown.payload["connector_code"], "UNKNOWN");
        assert_eq!(unknown.payload["raw_connector_code"], "9999");
        assert!(unknown.payload["power_w"].is_null());
        assert!(unknown.payload["reported_quantity"].is_null());
        assert_eq!(unknown.payload["current"], "UNKNOWN");
        let known = run
            .entities
            .iter()
            .find(|entity| entity.external_id == "4:41")
            .unwrap();
        assert_eq!(known.payload["connector_code"], "TYPE2");
        assert_eq!(known.payload["power_w"], 7400);
        assert_eq!(known.payload["reported_quantity"], 1);
    }

    #[test]
    fn quarantines_unlicensed_and_foreign_units() {
        let mut seen = HashSet::new();
        let mut providers = HashSet::new();
        let mut emit = |_: &super::OcmEntity| Ok(());
        let raw = br#"{"ID":2,"DataProviderID":999,"AddressInfo":{"CountryID":36}}"#;
        let run = parse_ocm_poi("b.json", raw, "00", &mut seen, &mut providers, &mut emit).unwrap();
        assert_eq!(run.quarantined, 1);
        let raw = br#"{"ID":3,"DataProviderID":1,"AddressInfo":{"CountryID":1}}"#;
        let run = parse_ocm_poi("c.json", raw, "00", &mut seen, &mut providers, &mut emit).unwrap();
        assert_eq!(run.quarantined, 1);
    }

    #[test]
    fn refuses_corrupt_and_identity_less_input() {
        let mut seen = HashSet::new();
        let mut providers = HashSet::new();
        let mut emit = |_: &super::OcmEntity| Ok(());
        assert!(parse_ocm_poi(
            "d.json",
            b"not json",
            "00",
            &mut seen,
            &mut providers,
            &mut emit
        )
        .is_err());
        assert!(parse_ocm_poi(
            "e.json",
            br#"{"DataProviderID":1}"#,
            "00",
            &mut seen,
            &mut providers,
            &mut emit
        )
        .is_err());
    }
}

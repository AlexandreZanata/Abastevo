//! Actual PBE vehicle-reference reader (EE-05).
//!
//! The pinned profile is the official PBE table plus an approved
//! manufacturer supplement: model/version identities with source
//! wording, exact energy lexemes and source clocks. A PBE row stays a
//! reference; supplement facts enrich exact matches only, never guessed
//! trims. Combustion rows quarantine as out of electric scope with full
//! loss accounting.
//!
//! The reader minimizes what it emits. Only model identities and
//! validated energy lexemes leave the reader; supplement marketing,
//! prices, colors and non-technical content never enter entities.

use crate::ev_contract::{canonical_bytes, source_key};
use crate::types::sha256_hex;
use serde_json::{json, Value};

/// Source label of the vehicle-reference stream.
pub const VEHICLE_SOURCE: &str = "pbe-vehicle";
/// Pinned PBE profile (2026 table, user-authorized 2026-10-22).
pub const PBE_PROFILE: &str = "pbe-2026-pdf-v1";

/// Refusal reasons of the PBE reader.
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum PbeError {
    Oversize,
    RowCap,
    Truncated(String),
    Profile(String),
    Contract(String),
}

impl std::fmt::Display for PbeError {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            PbeError::Oversize => write!(formatter, "input exceeds the budget"),
            PbeError::RowCap => write!(formatter, "row cap exceeded"),
            PbeError::Truncated(detail) => write!(formatter, "truncated: {detail}"),
            PbeError::Profile(detail) => write!(formatter, "profile refused: {detail}"),
            PbeError::Contract(detail) => write!(formatter, "contract refused: {detail}"),
        }
    }
}

impl std::error::Error for PbeError {}

/// One minimized vehicle reference.
#[derive(Debug, Clone)]
pub struct VehicleEntity {
    pub key: String,
    pub external_id: String,
    pub payload: Value,
    pub assertion: String,
}

/// Split a row on runs of 3+ spaces (Excel column gaps). Category is
/// field 0, brand 1, model 2, version 3, motor 4, combustion 5. Widths
/// vary by section; gaps do not.
fn split_fields(line: &str) -> Vec<String> {
    let mut fields: Vec<String> = Vec::new();
    let mut current = String::new();
    let mut spaces = 0usize;
    for character in line.chars() {
        if character == ' ' {
            spaces += 1;
            if spaces >= 3 && !current.trim().is_empty() {
                fields.push(current.trim().to_string());
                current = String::new();
            } else if spaces < 3 {
                current.push(character);
            }
            continue;
        }
        if spaces >= 3 && !current.trim().is_empty() {
            fields.push(current.trim().to_string());
            current = String::new();
        } else if spaces > 0 && spaces < 3 {
            for _ in 0..spaces {
                current.push(' ');
            }
        }
        spaces = 0;
        current.push(character);
    }
    if !current.trim().is_empty() {
        fields.push(current.trim().to_string());
    }
    fields
}

/// Parse one PBE text row. Returns the model identity with energy lexemes,
/// or a quarantine reason (combustion scope, malformed, missing identity).
/// MJ/km uses comma decimals in source (e.g. "0,41"); range is integer km.
/// Energy is the rightmost decimal+integer token pair (robust across the
/// nine section layouts); brand/model/version are gap-split fields.
pub fn parse_pbe_row(line: &str) -> Result<(String, String, String), &'static str> {
    let fields = split_fields(line);
    if fields.len() < 6 {
        return Err("short_row");
    }
    let electric = fields.iter().take(8).any(|field| {
        let lower = field.to_lowercase();
        lower.contains("elétrico") || lower.contains("eletrico")
    });
    if !electric {
        return Err("combustion_scope");
    }
    let brand = fields[1].clone();
    let model = fields[2].clone();
    let version = fields[3].clone();
    if brand.is_empty() || model.is_empty() || version.is_empty() {
        return Err("missing_identity");
    }
    let identity = format!("{brand} {model} {version}")
        .split_whitespace()
        .collect::<Vec<_>>()
        .join(" ")
        .to_uppercase();
    if identity.len() > 256 || identity.is_empty() {
        return Err("missing_identity");
    }
    let (mjkm, range) = energy_from_right(line).ok_or("invalid_energy")?;
    Ok((identity, mjkm, range))
}

/// Rightmost decimal+integer token pair (MJ/km and range km). Earlier
/// decimals (consumption equivalents) lack a bare-integer follower.
fn energy_from_right(line: &str) -> Option<(String, String)> {
    let tokens: Vec<&str> = line.split_whitespace().collect();
    let mut index = tokens.len();
    while index >= 2 {
        index -= 1;
        if valid_integer(tokens[index]) && valid_comma_decimal(tokens[index - 1]) {
            return Some((tokens[index - 1].to_string(), tokens[index].to_string()));
        }
    }
    None
}

fn valid_comma_decimal(text: &str) -> bool {
    match text.split_once(',') {
        Some((integer, fraction)) => {
            !integer.is_empty()
                && integer.bytes().all(|b| b.is_ascii_digit())
                && !fraction.is_empty()
                && fraction.len() <= 4
                && fraction.bytes().all(|b| b.is_ascii_digit())
        }
        None => false,
    }
}

fn valid_integer(text: &str) -> bool {
    !text.is_empty() && text.len() <= 6 && text.bytes().all(|b| b.is_ascii_digit())
}

/// Build a vehicle-reference entity with the frozen minimal payload.
pub fn vehicle_entity(identity: &str, rights_sha256: &str) -> Result<VehicleEntity, PbeError> {
    let key = source_key("pbe", "VEHICLE_REFERENCE", identity)
        .map_err(|e| PbeError::Contract(e.to_string()))?;
    let payload = json!({"model_code": identity});
    let assertion_input = json!({
        "kind": "VEHICLE_REFERENCE",
        "key": key,
        "parent_key": Value::Null,
        "payload": payload,
        "rights_sha256": rights_sha256,
        "source_clocks": {"created_at": Value::Null, "verified_at": Value::Null, "status_at": Value::Null},
    });
    let assertion = sha256_hex(
        &canonical_bytes(&assertion_input).map_err(|e| PbeError::Contract(e.to_string()))?,
    );
    Ok(VehicleEntity {
        key,
        external_id: identity.to_string(),
        payload,
        assertion,
    })
}

#[cfg(test)]
mod tests {
    use super::{parse_pbe_row, vehicle_entity};

    fn ev_row() -> String {
        let mut row = vec![' '; 573];
        let place = |row: &mut Vec<char>, start: usize, text: &str| {
            for (i, c) in text.chars().enumerate() {
                row[start + i] = c;
            }
        };
        place(&mut row, 93, "Sub Compacto");
        place(&mut row, 115, "BYD");
        place(&mut row, 128, "DOLPHIN MINI");
        place(&mut row, 162, "GS EV");
        place(&mut row, 182, "Eletrico       Eletrico");
        place(&mut row, 494, "0,41");
        place(&mut row, 509, "280");
        row.into_iter().collect()
    }

    #[test]
    fn reads_electric_rows_with_energy_lexemes() {
        let (identity, mjkm, range) = parse_pbe_row(&ev_row()).unwrap();
        assert_eq!(identity, "BYD DOLPHIN MINI GS EV");
        assert_eq!(mjkm, "0,41");
        assert_eq!(range, "280");
    }

    #[test]
    fn quarantines_combustion_and_malformed_rows() {
        let combustion = ev_row().replace("Eletrico       Eletrico", "1.0-6V       Combustao ");
        assert_eq!(parse_pbe_row(&combustion), Err("combustion_scope"));
        assert_eq!(parse_pbe_row("short"), Err("short_row"));
    }

    #[test]
    fn builds_minimal_frozen_entities() {
        let entity = vehicle_entity("BYD DOLPHIN MINI GS EV", "00").unwrap();
        assert_eq!(entity.payload["model_code"], "BYD DOLPHIN MINI GS EV");
        assert!(!entity.key.is_empty() && !entity.assertion.is_empty());
    }
}

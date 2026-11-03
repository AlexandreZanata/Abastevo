//! Source-specific socket lookup and exact decimal power. No inferred capability.
use serde::Deserialize;
use std::collections::BTreeMap;
use std::sync::OnceLock;

#[derive(Deserialize)]
struct Family {
    family: String,
    osm: Vec<String>,
    ocm: Vec<i64>,
}
#[derive(Deserialize)]
struct Mapping {
    families: Vec<Family>,
    current: BTreeMap<String, String>,
}
fn mapping() -> &'static Mapping {
    static MAPPING: OnceLock<Mapping> = OnceLock::new();
    MAPPING.get_or_init(|| {
        serde_json::from_str(include_str!(
            "../../../contracts/ev/socket-families-v1.json"
        ))
        .expect("compiled socket mapping")
    })
}

pub fn family_for_osm(raw: &str) -> &'static str {
    mapping()
        .families
        .iter()
        .find(|f| f.osm.iter().any(|v| v == raw))
        .map(|f| f.family.as_str())
        .unwrap_or("UNKNOWN")
}
pub fn family_for_ocm(raw: i64) -> &'static str {
    mapping()
        .families
        .iter()
        .find(|f| f.ocm.contains(&raw))
        .map(|f| f.family.as_str())
        .unwrap_or("UNKNOWN")
}
pub fn current_family(raw: i64) -> &'static str {
    mapping()
        .current
        .get(&raw.to_string())
        .map(String::as_str)
        .unwrap_or("UNKNOWN")
}

fn scaled_decimal(raw: &str, scale: i32) -> Option<u64> {
    if raw.is_empty() || raw.len() > 64 {
        return None;
    }
    let pieces: Vec<_> = raw.split(['e', 'E']).collect();
    if pieces.len() > 2 {
        return None;
    }
    let exponent = if pieces.len() == 2 {
        pieces[1].parse::<i32>().ok()?
    } else {
        0
    };
    if !(-18..=18).contains(&exponent) {
        return None;
    }
    let (whole, fraction) = pieces[0].split_once('.').unwrap_or((pieces[0], ""));
    if whole.is_empty()
        || !whole.bytes().all(|v| v.is_ascii_digit())
        || !fraction.bytes().all(|v| v.is_ascii_digit())
        || (pieces[0].contains('.') && fraction.is_empty())
    {
        return None;
    }
    let digits = format!("{whole}{fraction}").parse::<u128>().ok()?;
    let shift = scale
        .checked_add(exponent)?
        .checked_sub(fraction.len() as i32)?;
    let value = if shift >= 0 {
        digits.checked_mul(10u128.checked_pow(shift as u32)?)?
    } else {
        let divisor = 10u128.checked_pow((-shift) as u32)?;
        if digits % divisor != 0 {
            return None;
        }
        digits / divisor
    };
    (value > 0 && value <= 1_000_000_000).then_some(value as u64)
}
pub fn power_kw_to_watts(raw: &str) -> Option<u64> {
    scaled_decimal(raw, 3)
}
pub fn power_osm_to_watts(raw: &str) -> Option<u64> {
    let raw = raw.trim();
    for (unit, scale) in [("kW", 3), ("MW", 6), ("W", 0)] {
        if let Some(number) = raw.strip_suffix(unit) {
            return scaled_decimal(number.trim(), scale);
        }
    }
    scaled_decimal(raw, 3)
}

/// RawValue preserves each connection's numeric token before any f64 conversion.
/// Derived visitors reject duplicate Connections/PowerKW fields rather than
/// binding a lexeme from an unrelated nested field.
pub(crate) fn ocm_connection_powers(raw: &[u8]) -> Result<Vec<Option<String>>, serde_json::Error> {
    #[derive(Deserialize)]
    struct Connection<'a> {
        #[serde(borrow, rename = "PowerKW")]
        power: Option<&'a serde_json::value::RawValue>,
    }
    #[derive(Deserialize)]
    struct Document<'a> {
        #[serde(borrow, rename = "Connections")]
        connections: Option<Vec<Connection<'a>>>,
    }
    let document: Document<'_> = serde_json::from_slice(raw)?;
    Ok(document
        .connections
        .unwrap_or_default()
        .into_iter()
        .map(|v| v.power.map(|p| p.get().to_string()))
        .collect())
}

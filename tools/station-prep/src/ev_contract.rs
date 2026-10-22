//! EE-03 byte and identity primitives. No source parser or admission authority.
use crate::types::sha256_hex;
use serde_json::Value;

const MAX_INTEGER: u64 = 9_007_199_254_740_991;

/// Compact UTF-8 JSON with sorted ASCII keys and exact safe integers only.
/// Arrays retain order; callers own protocol-specific stream sorting/framing.
pub fn canonical_bytes(value: &Value) -> Result<Vec<u8>, &'static str> {
    fn write(v: &Value, out: &mut Vec<u8>, depth: usize) -> Result<(), &'static str> {
        if depth > 32 || out.len() > 1 << 20 {
            return Err("canonical bounds exceeded");
        }
        match v {
            Value::Null => out.extend_from_slice(b"null"),
            Value::Bool(b) => out.extend_from_slice(if *b { b"true" } else { b"false" }),
            Value::Number(n) => {
                let valid = n.as_u64().is_some_and(|n| n <= MAX_INTEGER)
                    || n.as_i64().is_some_and(|n| n.unsigned_abs() <= MAX_INTEGER);
                if !valid {
                    return Err("exact safe integer required");
                }
                out.extend_from_slice(n.to_string().as_bytes());
            }
            Value::String(s) => {
                if s.len() > 64 << 10 {
                    return Err("string too large");
                }
                out.extend_from_slice(serde_json::to_string(s).map_err(|_| "string")?.as_bytes());
            }
            Value::Array(a) => {
                out.push(b'[');
                for (i, x) in a.iter().enumerate() {
                    if i != 0 {
                        out.push(b',');
                    }
                    write(x, out, depth + 1)?;
                }
                out.push(b']');
            }
            Value::Object(m) => {
                out.push(b'{');
                let mut keys: Vec<_> = m.keys().collect();
                keys.sort_unstable();
                for (i, k) in keys.iter().enumerate() {
                    if !k.is_ascii() {
                        return Err("ASCII object key required");
                    }
                    if i != 0 {
                        out.push(b',');
                    }
                    write(&Value::String((*k).clone()), out, depth + 1)?;
                    out.push(b':');
                    write(&m[*k], out, depth + 1)?;
                }
                out.push(b'}');
            }
        }
        if out.len() > 1 << 20 {
            return Err("canonical record too large");
        }
        Ok(())
    }
    let mut out = Vec::new();
    write(value, &mut out, 0)?;
    Ok(out)
}

/// Stable source identity, independent of revision, coordinates and array order.
/// Each UTF-8 field (including the domain) has a big-endian u32 byte length.
pub fn source_key(namespace: &str, kind: &str, external_id: &str) -> Result<String, &'static str> {
    if namespace.is_empty()
        || namespace.len() > 64
        || !namespace
            .bytes()
            .all(|b| b.is_ascii_lowercase() || b.is_ascii_digit() || b == b'-')
        || !matches!(
            kind,
            "PLACE" | "EVSE" | "CONNECTOR" | "CONNECTION_GROUP" | "VEHICLE_REFERENCE"
        )
        || external_id.is_empty()
        || external_id.len() > 256
        || external_id.chars().any(char::is_control)
    {
        return Err("invalid source identity");
    }
    let mut bytes = Vec::new();
    for field in ["ev-source-key-v1", namespace, kind, external_id] {
        bytes.extend_from_slice(&(field.len() as u32).to_be_bytes());
        bytes.extend_from_slice(field.as_bytes());
    }
    Ok(sha256_hex(&bytes))
}

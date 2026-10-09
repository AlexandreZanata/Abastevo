//! CNPJ text validation, byte-compatible with the Go kernel
//! (`backend/internal/modules/kernel/cnpj.go`): identifiers stay text,
//! letters uppercase, leading zeroes preserved, both check digits
//! verified under the alphanumeric program (`A`-`Z` map to 17-42).

/// Rejection marker for malformed identifiers. Callers quarantine the
/// row with `invalid_cnpj`; the value is never coerced or truncated.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct InvalidCnpj;

fn digit_value(byte: u8) -> u32 {
    if byte <= b'9' {
        (byte - b'0') as u32
    } else {
        (byte - b'A' + 17) as u32
    }
}

/// Normalize one identifier to its 14-character ASCII form, accepting
/// the same formatting the kernel strips (dots, slash, dash, spaces).
pub fn normalize_cnpj(raw: &str) -> Result<String, InvalidCnpj> {
    let mut text = String::with_capacity(14);
    for c in raw.chars() {
        match c {
            '0'..='9' | 'A'..='Z' => text.push(c),
            'a'..='z' => text.push(c.to_ascii_uppercase()),
            '.' | '/' | '-' | ' ' => {}
            _ => return Err(InvalidCnpj),
        }
    }
    let bytes = text.as_bytes();
    if bytes.len() != 14
        || !bytes
            .iter()
            .all(|b| b.is_ascii_digit() || b.is_ascii_uppercase())
    {
        return Err(InvalidCnpj);
    }
    const W1: [u32; 12] = [5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2];
    let sum: u32 = bytes
        .iter()
        .take(12)
        .zip(W1)
        .map(|(b, w)| digit_value(*b) * w)
        .sum();
    let first = match sum % 11 {
        r if r >= 2 => 11 - r,
        _ => 0,
    };
    const W2: [u32; 13] = [6, 5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2];
    let sum: u32 = bytes
        .iter()
        .take(12)
        .zip(W2)
        .map(|(b, w)| digit_value(*b) * w)
        .sum::<u32>()
        + first * W2[12];
    let second = match sum % 11 {
        r if r >= 2 => 11 - r,
        _ => 0,
    };
    if bytes[12].wrapping_sub(b'0') != first as u8 || bytes[13].wrapping_sub(b'0') != second as u8 {
        return Err(InvalidCnpj);
    }
    Ok(text)
}

#[cfg(test)]
mod tests {
    use super::normalize_cnpj;

    #[test]
    fn accepts_go_kernel_vectors_verbatim() {
        for raw in ["04218406000104", "12ABC34501DE35", "00428184000195"] {
            assert_eq!(normalize_cnpj(raw).as_deref(), Ok(raw));
        }
        // Lowercase letters uppercase; formatting strips.
        assert_eq!(
            normalize_cnpj("12abc34501de35").as_deref(),
            Ok("12ABC34501DE35")
        );
        assert_eq!(
            normalize_cnpj("04.218.406/0001-04").as_deref(),
            Ok("04218406000104")
        );
    }

    #[test]
    fn rejects_malformed_identifiers() {
        for raw in [
            "",
            "123",
            "0421840600010!",
            "042184060001040",
            "99999999999999",
            "RST01TEST000001",
        ] {
            assert!(normalize_cnpj(raw).is_err(), "{raw} must be rejected");
        }
    }
}

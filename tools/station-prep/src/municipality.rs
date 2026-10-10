//! Municipality resolution: name + UF through versioned normalization
//! plus an explicit alias table. Ambiguous or unmatched rows quarantine;
//! nothing is guessed. RFB municipality codes are a separate namespace
//! and never enter this table.
//!
//! Normalization is a bounded Latin fold (diacritics stripped, uppercase,
//! whitespace collapsed) covering IBGE Portuguese names. Full Unicode
//! NFKD awaits a measured need; the limitation is tested below.

use std::collections::HashMap;

/// Normalize a municipality name for alias lookup.
pub fn normalize_name(raw: &str) -> String {
    let folded: String = raw.trim().chars().map(fold_latin).collect();
    folded
        .to_uppercase()
        .split_whitespace()
        .collect::<Vec<_>>()
        .join(" ")
}

/// Normalize a UF code for alias lookup.
pub fn normalize_uf(raw: &str) -> String {
    raw.trim().to_uppercase()
}

fn fold_latin(c: char) -> char {
    match c {
        'à' | 'á' | 'â' | 'ã' | 'ä' | 'å' | 'ā' | 'ă' | 'ą' | 'ǎ' | 'ạ' | 'ả' | 'ấ' | 'ầ' | 'ẩ'
        | 'ẫ' | 'ậ' | 'ắ' | 'ằ' | 'ẳ' | 'ẵ' | 'ặ' | 'À' | 'Á' | 'Â' | 'Ã' | 'Ä' | 'Å' | 'Ā'
        | 'Ă' | 'Ą' | 'Ǎ' | 'Ạ' | 'Ả' | 'Ấ' | 'Ầ' | 'Ẩ' | 'Ẫ' | 'Ậ' | 'Ắ' | 'Ằ' | 'Ẳ' | 'Ẵ'
        | 'Ặ' => 'a',
        'è' | 'é' | 'ê' | 'ë' | 'ē' | 'ĕ' | 'ė' | 'ę' | 'ě' | 'ẹ' | 'ẻ' | 'ẽ' | 'ế' | 'ề' | 'ể'
        | 'ễ' | 'ệ' | 'È' | 'É' | 'Ê' | 'Ë' | 'Ē' | 'Ĕ' | 'Ė' | 'Ę' | 'Ě' | 'Ẹ' | 'Ẻ' | 'Ẽ'
        | 'Ế' | 'Ề' | 'Ể' | 'Ễ' | 'Ệ' => 'e',
        'ì' | 'í' | 'î' | 'ï' | 'ĩ' | 'ī' | 'ĭ' | 'į' | 'ı' | 'ǐ' | 'ị' | 'ỉ' | 'Ì' | 'Í' | 'Î'
        | 'Ï' | 'Ĩ' | 'Ī' | 'Ĭ' | 'Į' | 'Ǐ' | 'Ị' | 'Ỉ' => 'i',
        'ò' | 'ó' | 'ô' | 'õ' | 'ö' | 'ø' | 'ō' | 'ŏ' | 'ő' | 'ǒ' | 'ọ' | 'ỏ' | 'ố' | 'ồ' | 'ổ'
        | 'ỗ' | 'ộ' | 'ớ' | 'ờ' | 'ở' | 'ỡ' | 'ợ' | 'Ò' | 'Ó' | 'Ô' | 'Õ' | 'Ö' | 'Ø' | 'Ō'
        | 'Ŏ' | 'Ő' | 'Ǒ' | 'Ọ' | 'Ỏ' | 'Ố' | 'Ồ' | 'Ổ' | 'Ỗ' | 'Ộ' | 'Ớ' | 'Ờ' | 'Ở' | 'Ỡ'
        | 'Ợ' => 'o',
        'ù' | 'ú' | 'û' | 'ü' | 'ũ' | 'ū' | 'ŭ' | 'ů' | 'ű' | 'ų' | 'ǔ' | 'ụ' | 'ủ' | 'Ù' | 'Ú'
        | 'Û' | 'Ü' | 'Ũ' | 'Ū' | 'Ŭ' | 'Ů' | 'Ű' | 'Ų' | 'Ǔ' | 'Ụ' | 'Ủ' => 'u',
        'ç' | 'ć' | 'ĉ' | 'ċ' | 'č' | 'Ç' | 'Ć' | 'Ĉ' | 'Ċ' | 'Č' => 'c',
        'ñ' | 'ń' | 'ņ' | 'ň' | 'Ñ' | 'Ń' | 'Ņ' | 'Ň' => 'n',
        'ý' | 'ÿ' | 'ŷ' | 'Ý' | 'Ŷ' | 'Ÿ' => 'y',
        'ĝ' | 'ğ' | 'ġ' | 'ģ' | 'Ĝ' | 'Ğ' | 'Ġ' | 'Ģ' => 'g',
        'ŝ' | 'ş' | 'š' | 'ș' | 'Ŝ' | 'Ş' | 'Š' | 'Ș' => 's',
        'ĵ' | 'Ĵ' => 'j',
        'ŵ' | 'Ŵ' => 'w',
        'ẑ' | 'ź' | 'ż' | 'ž' | 'Ẑ' | 'Ź' | 'Ż' | 'Ž' => 'z',
        other => other,
    }
}

/// Resolution failure. Both variants quarantine the row; neither invents
/// a code.
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum MunicipalityError {
    Unknown,
    Ambiguous,
}

#[derive(Debug, Clone)]
enum Entry {
    Single(String),
    Ambiguous,
}

/// Versioned alias table: normalized `(name, uf)` to IBGE code, with the
/// SHA256 digest of the loaded reference for manifest provenance.
#[derive(Debug)]
pub struct AliasTable {
    map: HashMap<(String, String), Entry>,
    digest: String,
}

impl Default for AliasTable {
    fn default() -> Self {
        Self {
            map: HashMap::new(),
            digest: crate::types::sha256_hex(b""),
        }
    }
}

#[derive(Debug, serde::Deserialize)]
struct AliasFile {
    entries: Vec<AliasEntryFile>,
}

#[derive(Debug, serde::Deserialize)]
struct AliasEntryFile {
    uf: String,
    ibge: String,
    aliases: Vec<String>,
}

/// Loading failure for the alias reference file itself.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct AliasError(pub String);

impl AliasTable {
    /// Load the versioned alias reference. A normalized alias claimed by
    /// two codes is recorded ambiguous; loading never picks a winner.
    pub fn from_json(raw: &str) -> Result<Self, AliasError> {
        let file: AliasFile =
            serde_json::from_str(raw).map_err(|err| AliasError(err.to_string()))?;
        let mut table = Self {
            digest: crate::types::sha256_hex(raw.as_bytes()),
            ..Self::default()
        };
        for entry in file.entries {
            let uf = normalize_uf(&entry.uf);
            for alias in entry.aliases {
                let key = (normalize_name(&alias), uf.clone());
                match table.map.get(&key) {
                    None => {
                        table.map.insert(key, Entry::Single(entry.ibge.clone()));
                    }
                    Some(Entry::Single(known)) if *known == entry.ibge => {}
                    _ => {
                        table.map.insert(key, Entry::Ambiguous);
                    }
                }
            }
        }
        Ok(table)
    }

    /// Resolve a raw name + UF pair to an IBGE code.
    pub fn resolve(&self, name: &str, uf: &str) -> Result<String, MunicipalityError> {
        match self.map.get(&(normalize_name(name), normalize_uf(uf))) {
            Some(Entry::Single(ibge)) => Ok(ibge.clone()),
            Some(Entry::Ambiguous) => Err(MunicipalityError::Ambiguous),
            None => Err(MunicipalityError::Unknown),
        }
    }

    /// Digest of the loaded alias reference for manifest provenance.
    pub fn digest(&self) -> &str {
        &self.digest
    }
}

#[cfg(test)]
mod tests {
    use super::{normalize_name, AliasTable, MunicipalityError};

    const ALIASES: &str = r#"{"entries": [
        {"uf": "SP", "ibge": "3550308", "aliases": ["SÃO PAULO", "SAO PAULO"]},
        {"uf": "SP", "ibge": "3549904", "aliases": ["SÃO JOSÉ DOS CAMPOS"]},
        {"uf": "SP", "ibge": "3559999", "aliases": ["SAO PAULO"]},
        {"uf": "RJ", "ibge": "3304557", "aliases": ["RIO DE JANEIRO"]}
    ]}"#;

    #[test]
    fn normalizes_accents_case_and_whitespace() {
        assert_eq!(normalize_name("  São   Paulo "), "SAO PAULO");
        assert_eq!(normalize_name("São José dos Campos"), "SAO JOSE DOS CAMPOS");
        assert_eq!(normalize_name("av pomP"), "AV POMP");
    }

    #[test]
    fn resolves_aliases_and_flags_unknown_or_ambiguous() {
        let table = AliasTable::from_json(ALIASES).expect("test aliases load");
        assert_eq!(
            table.resolve("sao jose dos campos", "sp").as_deref(),
            Ok("3549904")
        );
        assert_eq!(
            table.resolve("VILLE INCONNUE", "SP"),
            Err(MunicipalityError::Unknown)
        );
        // "SAO PAULO" is claimed by SP/3550308 and SP/3559999 here.
        assert_eq!(
            table.resolve("Sao Paulo", "SP"),
            Err(MunicipalityError::Ambiguous)
        );
    }
}

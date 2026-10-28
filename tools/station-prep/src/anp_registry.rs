//! MC-A2: normalize raw ANP Revendedores API pages into deterministic NDJSON.
//! No network, database or clock: the caller supplies the fetch timestamp.

use serde::{Deserialize, Serialize};
use sha2::{Digest, Sha256};
use std::fs;
use std::io::Write;
use std::path::{Path, PathBuf};

use crate::cnpj::normalize_cnpj;

pub const PARSER_VERSION: &str = "anp-registry-v1";

#[derive(Debug, Deserialize)]
struct ApiPage {
    #[serde(default)]
    data: Vec<RawRow>,
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct RawRow {
    #[serde(default)]
    codigo_simp: String,
    #[serde(default)]
    autorizacao: String,
    #[serde(default)]
    data_publicacao: String,
    #[serde(default)]
    razao_social: String,
    #[serde(default)]
    cnpj: String,
    #[serde(default)]
    endereco: String,
    #[serde(default)]
    complemento: String,
    #[serde(default)]
    bairro: String,
    #[serde(default)]
    cep: String,
    #[serde(default)]
    uf: String,
    #[serde(default)]
    municipio: String,
    #[serde(default)]
    distribuidora: String,
    #[serde(default)]
    classe: String,
    #[serde(default)]
    produtos: serde_json::Value,
    #[serde(default)]
    latitude: String,
    #[serde(default)]
    longitude: String,
    #[serde(default)]
    latitude_anp4_c: String,
    #[serde(default)]
    longitude_anp4_c: String,
    #[serde(default)]
    validacao: String,
    #[serde(default)]
    estimativa_acuracia: String,
    #[serde(default)]
    srid: String,
    #[serde(default)]
    src: String,
    #[serde(default)]
    data_obtencao: String,
    #[serde(default)]
    origem_informacao: String,
    #[serde(default)]
    situacao_constatada: String,
    #[serde(default)]
    observacao: String,
    #[serde(default)]
    status_sigaf: String,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
pub struct Row {
    pub schema_version: String,
    pub kind: String,
    pub source: String,
    pub source_file: String,
    pub source_row: usize,
    pub cnpj: String,
    pub codigo_simp: String,
    pub authorization: String,
    pub published_date: String,
    pub legal_name: String,
    pub address: String,
    pub complement: String,
    pub district: String,
    pub postal_code: String,
    pub state: String,
    pub municipality: String,
    pub distributor: String,
    pub class: String,
    pub products: serde_json::Value,
    pub products_raw: String,
    pub latitude: Option<f64>,
    pub longitude: Option<f64>,
    pub latitude_anp4c: String,
    pub longitude_anp4c: String,
    pub validation: String,
    pub accuracy: String,
    pub srid: String,
    pub datum: String,
    pub obtained_at: String,
    pub origin: String,
    pub observed_situation: String,
    pub note: String,
    pub sigaf_status: String,
}

#[derive(Debug, Clone, Serialize, PartialEq)]
pub struct Quarantine {
    pub schema_version: String,
    pub source_file: String,
    pub source_row: usize,
    pub cnpj_raw: String,
    pub reason: String,
}

#[derive(Debug, Clone, Serialize, PartialEq)]
pub struct FileMeta {
    pub file: String,
    pub bytes: u64,
    pub sha256: String,
    pub rows: usize,
}

#[derive(Debug, Clone, Serialize, PartialEq)]
pub struct Counts {
    pub input: usize,
    pub accepted: usize,
    pub quarantined: usize,
}

#[derive(Debug, Clone, Serialize, PartialEq)]
pub struct Manifest {
    pub schema_version: String,
    pub parser_version: String,
    pub kind: String,
    pub source: String,
    pub fetched_at: String,
    pub files: Vec<FileMeta>,
    pub counts: Counts,
    pub output_file: String,
    pub output_sha256: String,
    pub quarantine_file: String,
    pub quarantine_sha256: String,
}

pub struct Run {
    pub manifest: Manifest,
    pub output_path: PathBuf,
    pub quarantine_path: PathBuf,
}

fn sha256_hex(raw: &[u8]) -> String {
    let mut hasher = Sha256::new();
    hasher.update(raw);
    let digest = hasher.finalize();
    digest.iter().map(|b| format!("{b:02x}")).collect()
}

fn parse_products(raw: &serde_json::Value) -> (serde_json::Value, String) {
    match raw {
        serde_json::Value::String(text) => {
            if text.trim().is_empty() {
                return (serde_json::Value::Array(Vec::new()), String::new());
            }
            let parsed = serde_json::from_str(text).unwrap_or(serde_json::Value::Null);
            (parsed, text.clone())
        }
        serde_json::Value::Array(_) => {
            (raw.clone(), serde_json::to_string(raw).unwrap_or_default())
        }
        serde_json::Value::Null => (serde_json::Value::Array(Vec::new()), String::new()),
        other => (
            other.clone(),
            serde_json::to_string(other).unwrap_or_default(),
        ),
    }
}

fn parse_coord(raw: &str) -> Option<f64> {
    let value: f64 = raw.trim().parse().ok()?;
    if !value.is_finite() {
        return None;
    }
    Some(value)
}

fn normalize_row(
    kind: &str,
    source: &str,
    file: &str,
    row_index: usize,
    raw: RawRow,
) -> Result<Row, Quarantine> {
    let cnpj = normalize_cnpj(&raw.cnpj).map_err(|_| Quarantine {
        schema_version: PARSER_VERSION.to_string(),
        source_file: file.to_string(),
        source_row: row_index,
        cnpj_raw: raw.cnpj.clone(),
        reason: "invalid_cnpj".to_string(),
    })?;
    let state = raw.uf.trim().to_ascii_uppercase();
    if state.len() != 2 || !state.bytes().all(|b| b.is_ascii_uppercase()) {
        return Err(Quarantine {
            schema_version: PARSER_VERSION.to_string(),
            source_file: file.to_string(),
            source_row: row_index,
            cnpj_raw: raw.cnpj.clone(),
            reason: "missing_or_invalid_uf".to_string(),
        });
    }
    let (products, products_raw) = parse_products(&raw.produtos);
    Ok(Row {
        schema_version: PARSER_VERSION.to_string(),
        kind: kind.to_string(),
        source: source.to_string(),
        source_file: file.to_string(),
        source_row: row_index,
        cnpj,
        codigo_simp: raw.codigo_simp.trim().to_string(),
        authorization: raw.autorizacao.trim().to_string(),
        published_date: raw.data_publicacao.trim().to_string(),
        legal_name: raw.razao_social.trim().to_string(),
        address: raw.endereco.trim().to_string(),
        complement: raw.complemento.trim().to_string(),
        district: raw.bairro.trim().to_string(),
        postal_code: raw.cep.trim().to_string(),
        state,
        municipality: raw.municipio.trim().to_string(),
        distributor: raw.distribuidora.trim().to_string(),
        class: raw.classe.trim().to_string(),
        products,
        products_raw,
        latitude: parse_coord(&raw.latitude),
        longitude: parse_coord(&raw.longitude),
        latitude_anp4c: raw.latitude_anp4_c.trim().to_string(),
        longitude_anp4c: raw.longitude_anp4_c.trim().to_string(),
        validation: raw.validacao.trim().to_string(),
        accuracy: raw.estimativa_acuracia.trim().to_string(),
        srid: raw.srid.trim().to_string(),
        datum: raw.src.trim().to_string(),
        obtained_at: raw.data_obtencao.trim().to_string(),
        origin: raw.origem_informacao.trim().to_string(),
        observed_situation: raw.situacao_constatada.trim().to_string(),
        note: raw.observacao.trim().to_string(),
        sigaf_status: raw.status_sigaf.trim().to_string(),
    })
}

fn collect_files(input: &Path) -> Result<Vec<PathBuf>, String> {
    if input.is_file() {
        return Ok(vec![input.to_path_buf()]);
    }
    let entries = fs::read_dir(input).map_err(|e| format!("read input: {e}"))?;
    let mut files = Vec::new();
    for entry in entries {
        let path = entry.map_err(|e| format!("read input entry: {e}"))?.path();
        if path.is_file() && path.extension().and_then(|x| x.to_str()) == Some("json") {
            files.push(path);
        }
    }
    files.sort();
    if files.is_empty() {
        return Err("input has no JSON pages".to_string());
    }
    Ok(files)
}

/// Parse every page under `input` and write `anp-registry.ndjson`,
/// `anp-registry-quarantine.ndjson` and `manifest.json` atomically.
pub fn run(
    input: &Path,
    output: &Path,
    kind: &str,
    fetched_at: &str,
    source: &str,
) -> Result<Run, String> {
    if fetched_at.len() < 20 {
        return Err("fetched_at must be an RFC3339 timestamp".to_string());
    }
    let files = collect_files(input)?;
    let mut rows = Vec::new();
    let mut quarantine = Vec::new();
    let mut metas = Vec::new();
    for path in files {
        let raw = fs::read(&path).map_err(|e| format!("read {}: {e}", path.display()))?;
        let name = path
            .file_name()
            .and_then(|x| x.to_str())
            .unwrap_or("page.json")
            .to_string();
        let page: ApiPage =
            serde_json::from_slice(&raw).map_err(|e| format!("parse {}: {e}", path.display()))?;
        metas.push(FileMeta {
            file: name.clone(),
            bytes: raw.len() as u64,
            sha256: sha256_hex(&raw),
            rows: page.data.len(),
        });
        for (index, raw_row) in page.data.into_iter().enumerate() {
            match normalize_row(kind, source, &name, index + 1, raw_row) {
                Ok(row) => rows.push(row),
                Err(q) => quarantine.push(q),
            }
        }
    }
    rows.sort_by(|a, b| {
        (
            &a.state,
            &a.cnpj,
            &a.authorization,
            &a.source_file,
            a.source_row,
        )
            .cmp(&(
                &b.state,
                &b.cnpj,
                &b.authorization,
                &b.source_file,
                b.source_row,
            ))
    });
    quarantine.sort_by(|a, b| {
        (&a.source_file, a.source_row, &a.reason).cmp(&(&b.source_file, b.source_row, &b.reason))
    });
    let mut out = Vec::new();
    for row in &rows {
        serde_json::to_writer(&mut out, row).map_err(|e| format!("serialize row: {e}"))?;
        out.push(b'\n');
    }
    let mut quar = Vec::new();
    for row in &quarantine {
        serde_json::to_writer(&mut quar, row).map_err(|e| format!("serialize quarantine: {e}"))?;
        quar.push(b'\n');
    }
    let counts = Counts {
        input: metas.iter().map(|m| m.rows).sum(),
        accepted: rows.len(),
        quarantined: quarantine.len(),
    };
    if counts.input != counts.accepted + counts.quarantined {
        return Err("counts do not reconcile".to_string());
    }
    fs::create_dir_all(output).map_err(|e| format!("create output: {e}"))?;
    let output_path = output.join("anp-registry.ndjson");
    let quarantine_path = output.join("anp-registry-quarantine.ndjson");
    let manifest_path = output.join("manifest.json");
    write_atomic(&output_path, &out)?;
    write_atomic(&quarantine_path, &quar)?;
    let manifest = Manifest {
        schema_version: PARSER_VERSION.to_string(),
        parser_version: PARSER_VERSION.to_string(),
        kind: kind.to_string(),
        source: source.to_string(),
        fetched_at: fetched_at.to_string(),
        files: metas,
        counts,
        output_file: "anp-registry.ndjson".to_string(),
        output_sha256: sha256_hex(&out),
        quarantine_file: "anp-registry-quarantine.ndjson".to_string(),
        quarantine_sha256: sha256_hex(&quar),
    };
    let manifest_raw =
        serde_json::to_vec_pretty(&manifest).map_err(|e| format!("serialize manifest: {e}"))?;
    write_atomic(&manifest_path, &manifest_raw)?;
    Ok(Run {
        manifest,
        output_path,
        quarantine_path,
    })
}

fn write_atomic(path: &Path, raw: &[u8]) -> Result<(), String> {
    let tmp = path.with_extension("tmp");
    let mut file = fs::File::create(&tmp).map_err(|e| format!("create {}: {e}", tmp.display()))?;
    file.write_all(raw)
        .map_err(|e| format!("write {}: {e}", tmp.display()))?;
    file.sync_all()
        .map_err(|e| format!("sync {}: {e}", tmp.display()))?;
    fs::rename(&tmp, path).map_err(|e| format!("rename {}: {e}", path.display()))
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::time::{SystemTime, UNIX_EPOCH};

    fn temp_dir(tag: &str) -> PathBuf {
        let nanos = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .unwrap()
            .as_nanos();
        let dir = std::env::temp_dir().join(format!("anp-registry-{tag}-{nanos}"));
        fs::create_dir_all(&dir).unwrap();
        dir
    }

    fn page() -> &'static str {
        r#"{"status":200,"data":[
          {"codigoSIMP":"1","autorizacao":"PR/MT1","dataPublicacao":"16/09/2010","razaoSocial":"POSTO A","cnpj":"04.218.406/0001-04","endereco":"RUA A, 10","bairro":"CENTRO","cep":"78000000","uf":"MT","municipio":"CUIABA","produtos":"[{\"produto\":\"GASOLINA\"}]","latitude":"-12.5","longitude":"-56.1","srid":"4674","src":"SIRGAS2000","validacao":"validada","estimativaAcuracia":"5"},
          {"codigoSIMP":"2","autorizacao":"PR/MT2","razaoSocial":"POSTO B","cnpj":"123","uf":"MT"},
          {"codigoSIMP":"3","autorizacao":"PR/MT3","razaoSocial":"POSTO C","cnpj":"04.218.406/0001-04","uf":""}
        ]}"#
    }

    #[test]
    fn parses_and_counts_reconcile() {
        let dir = temp_dir("parse");
        let input = dir.join("MT-p1.json");
        fs::write(&input, page()).unwrap();
        let out = dir.join("out");
        let run = run(
            &input,
            &out,
            "automotive-fuel",
            "2026-10-27T00:00:00Z",
            "anp-revendedores-api",
        )
        .unwrap();
        assert_eq!(run.manifest.counts.input, 3);
        assert_eq!(run.manifest.counts.accepted, 1);
        assert_eq!(run.manifest.counts.quarantined, 2);
        let rows: Vec<Row> = fs::read_to_string(&run.output_path)
            .unwrap()
            .lines()
            .map(|line| serde_json::from_str(line).unwrap())
            .collect();
        assert_eq!(rows[0].cnpj, "04218406000104");
        assert_eq!(rows[0].state, "MT");
        assert_eq!(rows[0].srid, "4674");
        assert_eq!(rows[0].latitude, Some(-12.5));
        assert!(rows[0].products.is_array());
        fs::remove_dir_all(dir).unwrap();
    }

    #[test]
    fn reruns_are_byte_identical() {
        let dir = temp_dir("rerun");
        let input = dir.join("MT-p1.json");
        fs::write(&input, page()).unwrap();
        let first = run(
            &input,
            &dir.join("a"),
            "automotive-fuel",
            "2026-10-27T00:00:00Z",
            "anp-revendedores-api",
        )
        .unwrap();
        let second = run(
            &input,
            &dir.join("b"),
            "automotive-fuel",
            "2026-10-27T00:00:00Z",
            "anp-revendedores-api",
        )
        .unwrap();
        assert_eq!(
            fs::read(&first.output_path).unwrap(),
            fs::read(&second.output_path).unwrap()
        );
        assert_eq!(first.manifest.output_sha256, second.manifest.output_sha256);
        assert_eq!(
            first.manifest.quarantine_sha256,
            second.manifest.quarantine_sha256
        );
        assert_eq!(first.manifest.files, second.manifest.files);
        fs::remove_dir_all(dir).unwrap();
    }
}

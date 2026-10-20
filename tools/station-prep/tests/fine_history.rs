use station_prep::sanction_history::parse_fine_history;
use station_prep::Limits;

fn source_2024(rows: &[&str]) -> Vec<u8> {
    let mut text = "\u{feff}ANP - Agência Nacional do Petroleo, Gás Natural e Biocombustíveis;;;;;;;;;\n;;;;;;;;;\nRELAÇÃO DE MULTAS APLICADAS EM 2024  (Relatório extraído em março/2025);;;;;;;;;\nFonte: Sistema de Controle de Multas - SICOM;;;;;;;;;\n;;;;;;;;;\n;;;;;;;;;\nStatus Processo;Superintendência;Número do Processo;Número do DUF;CNPJ/CPF;Razão Social;Vencimento;Valor da Multa;Valor Total Pago;\n".to_string();
    for row in rows {
        text.push_str(row);
        text.push('\n');
    }
    text.into_bytes()
}
const ROW:&str="Synthetic status;SYNTHETIC;000000000000001;SYNTHETIC/1;11.222.333/0001-81;Discarded synthetic name;28/02/2024; R$ 1.234,56 ; R$ 0,00 ;SIM";

#[test]
fn native_history_retains_unlabelled_raw_facts_and_private_minimization() {
    let batch = parse_fine_history(
        "multas-aplicadas-2024.csv",
        source_2024(&[ROW, ROW]),
        "2024",
        &Limits::default(),
    )
    .unwrap();
    assert_eq!(
        (
            batch.counts.input,
            batch.counts.accepted,
            batch.counts.duplicates,
            batch.counts.quarantined
        ),
        (2, 1, 1, 0)
    );
    let row = &batch.observations[0];
    assert_eq!(row.reference_label, "Número do DUF");
    assert_eq!(row.reference_ref, "SYNTHETIC/1");
    assert_eq!(row.unlabelled_field, "SIM");
    assert_eq!(row.amount_minor, 123456);
    assert_eq!(row.paid_minor, Some(0));
    assert_eq!(row.clock.precision, "MONTH");
    assert_eq!(row.clock.value, "2025-03");
    assert_eq!(row.source_transit_on, None);
    assert_eq!(row.amount_text, " R$ 1.234,56 ");
    let json = serde_json::to_string(row).unwrap();
    for value in ["Discarded synthetic name", "decided_on", "balance", "guilt"] {
        assert!(!json.contains(value));
    }
}

#[test]
fn missing_source_department_is_unknown_without_losing_a_valid_observation() {
    let row = ROW.replacen(";SYNTHETIC;", ";;", 1);
    let batch = parse_fine_history(
        "multas-aplicadas-2024.csv",
        source_2024(&[&row]),
        "2024",
        &Limits::default(),
    )
    .unwrap();
    assert_eq!(batch.counts.accepted, 1);
    assert_eq!(batch.observations[0].department, "");
}

fn annual(period: &str, header: &str, rows: &[&str], cp850: bool) -> Vec<u8> {
    let width = header.split(';').count();
    let empty = ";".repeat(width - 1);
    let mut text = if cp850 {
        String::new()
    } else {
        "\u{feff}".into()
    };
    text.push_str(&format!("ANP - Agência Nacional do Petroleo, Gás Natural e Biocombustíveis{empty}\n{empty}\nRELAÇÃO DE MULTAS APLICADAS EM {period}  (Relatório extraído em junho/2024){empty}\nFonte: Sistema de Controle de Multas - SICOM{empty}\n{empty}\n"));
    if period != "2023" {
        text.push_str(&format!("{empty}\n"));
    }
    text.push_str(header);
    text.push('\n');
    for row in rows {
        text.push_str(row);
        text.push('\n');
    }
    if cp850 {
        text.chars()
            .map(|c| {
                if c.is_ascii() {
                    c as u8
                } else {
                    match c {
                        'ê' => 0x88,
                        'á' => 0xa0,
                        'í' => 0xa1,
                        'Ç' => 0x80,
                        'Ã' => 0xc7,
                        'ó' => 0xa2,
                        'ú' => 0xa3,
                        'ã' => 0xc6,
                        'ç' => 0x87,
                        _ => panic!("synthetic encoding mapping"),
                    }
                }
            })
            .collect()
    } else {
        text.into_bytes()
    }
}
const HEADER_2023:&str="Status Processo;Superintendência;Número do Processo;Número do DUF;CNPJ/CPF;Razão Social;Vencimento da Multa; Valor da Multa ; Valor Total Pago ";
const HEADER_2022:&str="Status Processo;Superintendência;Número do Processo;Número do DUF;CNPJ/CPF;Razão Social;Data Transito Julgado;Vencimento;Valor da Multa;Valor Total Pago";
const HEADER_2021:&str="Status Processo;Superintendência;Número do Processo;Auto de Infração;CNPJ/CPF;Razão Social;Data Transito Julgado;Vencimento;Valor  da Multa Aplicada;Valor Total Pago (*) (**) ";
const HEADER_2020:&str="Status Processo;Superintendência;Número do Processo;Auto de Infração;CNPJ/CPF;Razão Social;Data Transito Julgado;Vencimento; Valor  da Multa Aplicada ; Valor Total Pago (*) (**)  ";
const OLD_ROW:&str="Synthetic status;SYNTHETIC;000000000000001;SYNTHETIC/1;11.222.333/0001-81;Discarded synthetic name;20/02/2020;28/02/2020; R$ 1.234,56 ; R$ 0,00 ";

#[test]
fn unknown_paid_is_null_and_never_zero_or_an_unknown_fine_amount() {
    let row = ROW
        .trim_end_matches(";SIM")
        .replace(" R$ 0,00 ", " R$ -   ");
    let batch = parse_fine_history(
        "multas-aplicadas-2023.csv",
        annual("2023", HEADER_2023, &[&row], true),
        "2023",
        &Limits::default(),
    )
    .unwrap();
    assert_eq!(batch.counts.accepted, 1);
    assert_eq!(batch.observations[0].paid_minor, None);
    assert_eq!(batch.observations[0].paid_text, " R$ -   ");
    assert_eq!(batch.header_record, 6);
    let invalid = row.replace(" R$ 1.234,56 ", " R$ - ");
    let batch = parse_fine_history(
        "multas-aplicadas-2023.csv",
        annual("2023", HEADER_2023, &[&invalid], true),
        "2023",
        &Limits::default(),
    )
    .unwrap();
    assert_eq!(batch.counts.quarantined, 1);
    let different = row.replace(" R$ -   ", " R$ 0,00 ");
    let batch = parse_fine_history(
        "multas-aplicadas-2023.csv",
        annual("2023", HEADER_2023, &[&row, &different], true),
        "2023",
        &Limits::default(),
    )
    .unwrap();
    assert_eq!(batch.counts.accepted, 2);
    assert_ne!(
        batch.observations[0].observation_key,
        batch.observations[1].observation_key
    );
}

#[test]
fn historical_reference_labels_and_source_transit_never_become_decision_dates() {
    for (year, name, header, label) in [
        (
            "2020",
            "multas-aplicadas-2020.csv",
            HEADER_2020,
            "Auto de Infração",
        ),
        (
            "2021",
            "multas-2021-junho22.xlsx",
            HEADER_2021,
            "Auto de Infração",
        ),
        (
            "2022",
            "multas-aplicadas-2022.csv",
            HEADER_2022,
            "Número do DUF",
        ),
    ] {
        let empty = OLD_ROW.replace("20/02/2020", "");
        let batch = parse_fine_history(
            name,
            annual(year, header, &[OLD_ROW, &empty], false),
            year,
            &Limits::default(),
        )
        .unwrap();
        assert_eq!(batch.counts.accepted, 2);
        assert!(batch
            .observations
            .iter()
            .any(|r| r.source_transit_on.as_deref() == Some("2020-02-20")));
        assert!(batch
            .observations
            .iter()
            .any(|r| r.source_transit_on.is_none()));
        assert!(batch
            .observations
            .iter()
            .all(|r| r.reference_label == label && r.due_on == "2020-02-28"));
        assert!(!serde_json::to_string(&batch.observations)
            .unwrap()
            .contains("decided_on"));
    }
}

#[test]
fn grouped_period_is_one_snapshot_with_an_actual_extraction_day() {
    let header="Status Processo;Superintendência;Número do Processo;Auto de Infração;CNPJ/CPF;Razão Social;Data Transito Julgado;Vencimento ;Valor da Multa Aplicada;Valor Total Pago (*) (**)";
    let raw=format!("\u{feff}ANP - Agência Nacional do Petroleo, Gás Natural e Biocombustíveis;;;;Data: 06/02/2020;;;;;\nSICOM;;;;;;;;;\nFinanceiro (GRM);;;;;;;;;\n;;;;;;;;;\n{header}\n{OLD_ROW}\n").into_bytes();
    let batch = parse_fine_history(
        "multas-aplicadas-2016a2019.csv",
        raw,
        "2016-2019",
        &Limits::default(),
    )
    .unwrap();
    assert_eq!(batch.source_period, "2016-2019");
    assert_eq!(batch.clock.precision, "DAY");
    assert_eq!(batch.clock.value, "2020-02-06");
    assert_eq!(batch.counts.accepted, 1);
    assert_eq!(
        batch.observations[0].evidence_ref,
        "multas-aplicadas-2016a2019.csv:6"
    );
}

#[test]
fn exact_public_notes_are_non_event_annotations_and_unknown_footer_is_redacted() {
    let note="(*) Para verificação do valor total pago dos processos com status parcelamento em andamento é preciso acessar o link: https://app.anp.gov.br/anp-gpc-consulta-web/public/emissao-gru/list.xhtml;;;;;;;;;";
    let unknown = "(*) Discarded private synthetic source text;;;;;;;;;";
    let batch = parse_fine_history(
        "multas-aplicadas-2020.csv",
        annual(
            "2020",
            HEADER_2020,
            &[OLD_ROW, note, ";;;;;;;;;", unknown],
            false,
        ),
        "2020",
        &Limits::default(),
    )
    .unwrap();
    assert_eq!(
        (
            batch.counts.input,
            batch.counts.accepted,
            batch.counts.quarantined
        ),
        (4, 1, 3)
    );
    assert_eq!(batch.metadata.len(), 2);
    assert_eq!(
        batch
            .quarantine
            .iter()
            .filter(|r| r.reason == "source_metadata")
            .count(),
        2
    );
    assert_eq!(
        batch.metadata[0].evidence_ref,
        "multas-aplicadas-2020.csv:9"
    );
    assert!(batch.quarantine.iter().all(|r| r.source_key.is_none()));
    assert!(!serde_json::to_string(&batch.metadata)
        .unwrap()
        .contains("Discarded private"));
    assert!(!serde_json::to_string(&batch.quarantine)
        .unwrap()
        .contains("Discarded private"));
    let bounded = parse_fine_history(
        "multas-aplicadas-2020.csv",
        annual("2020", HEADER_2020, &[OLD_ROW, note], false),
        "2020",
        &Limits {
            max_field_bytes: 100,
            ..Limits::default()
        },
    )
    .unwrap();
    assert_eq!(bounded.counts.accepted, 1);
    assert_eq!(bounded.quarantine[0].reason, "record_too_large");
    assert!(bounded.metadata.is_empty());
    // A valid subject record after a footer violates the frozen framing contract.
    assert!(parse_fine_history(
        "multas-aplicadas-2020.csv",
        annual("2020", HEADER_2020, &[OLD_ROW, note, OLD_ROW], false),
        "2020",
        &Limits::default()
    )
    .is_err());
}

#[test]
fn malformed_identity_calendar_amount_and_retained_controls_are_accounted_without_pii() {
    let cpf = ROW.replace("11.222.333/0001-81", "000.000.000-00");
    let invalid = ROW.replace("11.222.333/0001-81", "11.222.333/0001-82");
    let date = ROW.replace("28/02/2024", "30/02/2024");
    let money = ROW.replace("1.234,56", "1.2345,56");
    let control = ROW.replace("Synthetic status", "Synthetic\tstatus");
    let discarded = ROW.replace("Discarded synthetic name", "Discarded\tname");
    let batch = parse_fine_history(
        "multas-aplicadas-2024.csv",
        source_2024(&[ROW, &cpf, &invalid, &date, &money, &control, &discarded]),
        "2024",
        &Limits::default(),
    )
    .unwrap();
    assert_eq!(
        (
            batch.counts.input,
            batch.counts.accepted,
            batch.counts.duplicates,
            batch.counts.quarantined
        ),
        (7, 1, 1, 5)
    );
    let text = serde_json::to_string(&batch.quarantine).unwrap();
    for private in [
        "000.000.000-00",
        "11.222.333/0001-82",
        "Synthetic\\tstatus",
        "Discarded",
    ] {
        assert!(!text.contains(private));
    }
    let mut numbers = batch
        .observations
        .iter()
        .chain(batch.duplicates.iter())
        .map(|r| r.evidence_ref.clone())
        .chain(batch.quarantine.iter().map(|r| r.row_locator.clone()))
        .collect::<Vec<_>>();
    numbers.sort();
    numbers.dedup();
    assert_eq!(numbers.len(), 7);
}

#[test]
fn exact_profile_encoding_name_header_framing_and_budgets_fail_without_partial_batch() {
    let raw = source_2024(&[ROW]);
    for (name, period) in [
        ("invented.csv", "2024"),
        ("multas-aplicadas-2024.csv", "2023"),
        ("multas-aplicadas-2024.csv", "2025"),
        ("multas-aplicadas-2024.csv", "02024"),
    ] {
        assert!(parse_fine_history(name, raw.clone(), period, &Limits::default()).is_err());
    }
    assert!(parse_fine_history(
        "multas-aplicadas-2024.csv",
        raw[3..].to_vec(),
        "2024",
        &Limits::default()
    )
    .is_err());
    assert!(parse_fine_history(
        "multas-aplicadas-2024.csv",
        raw[..raw.len() - 1].to_vec(),
        "2024",
        &Limits::default()
    )
    .is_err());
    assert!(parse_fine_history(
        "multas-aplicadas-2024.csv",
        String::from_utf8(raw.clone())
            .unwrap()
            .replace("Número do DUF", "Auto de Infração")
            .into_bytes(),
        "2024",
        &Limits::default()
    )
    .is_err());
    for limits in [
        Limits {
            max_bytes: 10,
            ..Limits::default()
        },
        Limits {
            max_rows: 0,
            ..Limits::default()
        },
    ] {
        assert!(
            parse_fine_history("multas-aplicadas-2024.csv", raw.clone(), "2024", &limits).is_err()
        );
    }
    let batch = parse_fine_history(
        "multas-aplicadas-2024.csv",
        raw,
        "2024",
        &Limits {
            max_field_bytes: 10,
            ..Limits::default()
        },
    )
    .unwrap();
    assert_eq!(batch.counts.quarantined, 1);
    let empty = parse_fine_history(
        "multas-aplicadas-2024.csv",
        source_2024(&[]),
        "2024",
        &Limits::default(),
    )
    .unwrap();
    assert_eq!(empty.counts.input, 0);
}

#[test]
fn historical_cli_private_integrity_no_output_on_refusal_and_existing_output_preserved() {
    use sha2::{Digest, Sha256};
    use std::os::unix::fs::PermissionsExt;
    use std::{fs, process::Command};
    let root = std::env::temp_dir().join(format!(
        "fine-history-cli-{}-{}",
        std::process::id(),
        std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .unwrap()
            .as_nanos()
    ));
    fs::create_dir(&root).unwrap();
    struct Cleanup(std::path::PathBuf);
    impl Drop for Cleanup {
        fn drop(&mut self) {
            let _ = fs::remove_dir_all(&self.0);
        }
    }
    let _cleanup = Cleanup(root.clone());
    let source = root.join("multas-aplicadas-2024.csv");
    let out = root.join("prepared");
    let raw = source_2024(&[ROW]);
    fs::write(&source, &raw).unwrap();
    let run = || {
        Command::new(env!("CARGO_BIN_EXE_prepare_fine_history"))
            .arg(&source)
            .arg("2024")
            .arg(&out)
            .output()
            .unwrap()
    };
    let result = run();
    assert!(result.status.success());
    assert_eq!(
        fs::metadata(&out).unwrap().permissions().mode() & 0o777,
        0o700
    );
    let m: serde_json::Value =
        serde_json::from_slice(&fs::read(out.join("manifest.json")).unwrap()).unwrap();
    assert_eq!(m["input_sha256"], format!("{:x}", Sha256::digest(&raw)));
    assert_eq!(m["counts"]["accepted"], 1);
    for output in m["outputs"].as_array().unwrap() {
        let name = output["file"].as_str().unwrap();
        let path = out.join(name);
        let bytes = fs::read(&path).unwrap();
        assert_eq!(
            fs::metadata(path).unwrap().permissions().mode() & 0o777,
            0o600
        );
        assert_eq!(output["bytes"], bytes.len());
        assert_eq!(output["sha256"], format!("{:x}", Sha256::digest(bytes)));
    }
    assert!(!String::from_utf8(result.stdout)
        .unwrap()
        .contains("Discarded"));
    fs::write(out.join("owned-marker"), b"preserve").unwrap();
    assert!(!run().status.success());
    assert!(out.join("owned-marker").exists());
    fs::remove_dir_all(&out).unwrap();
    fs::write(&source, b"private synthetic malformed source").unwrap();
    let denied = run();
    assert!(!denied.status.success());
    assert!(!out.exists());
    assert_eq!(
        String::from_utf8(denied.stderr).unwrap(),
        "prepare_fine_history: source contract or artifact operation refused\n"
    );
}

#[test]
fn unfinished_quoted_record_with_a_final_newline_is_not_a_complete_snapshot() {
    let mut raw = source_2024(&[ROW]);
    raw.extend_from_slice(b"\"discarded private unfinished record\n");
    assert!(
        parse_fine_history("multas-aplicadas-2024.csv", raw, "2024", &Limits::default()).is_err()
    );
    // Proper quoted multiline discarded names remain one logical observation.
    let quoted = ROW.replace("Discarded synthetic name", "\"Discarded\nname\"");
    assert_eq!(
        parse_fine_history(
            "multas-aplicadas-2024.csv",
            source_2024(&[&quoted]),
            "2024",
            &Limits::default()
        )
        .unwrap()
        .counts
        .accepted,
        1
    );
}

#[test]
fn unicode_line_separators_are_quarantined_for_the_canonical_rust_go_contract() {
    for separator in ['\u{2028}', '\u{2029}'] {
        let row = ROW.replace("Synthetic status", &format!("Synthetic{separator}status"));
        let batch = parse_fine_history(
            "multas-aplicadas-2024.csv",
            source_2024(&[&row]),
            "2024",
            &Limits::default(),
        )
        .unwrap();
        assert_eq!(batch.counts.quarantined, 1);
    }
}

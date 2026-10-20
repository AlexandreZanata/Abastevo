use station_prep::sanction_anp::{parse_anp_fines, FineError};
use station_prep::Limits;

fn source(rows: &[&str]) -> Vec<u8> {
    // Synthetic ASCII is also exact CP850. Accented source metadata is encoded
    // explicitly below; no production person or proceeding is a test fixture.
    let mut raw = b"ANP - Ag\x88ncia Nacional do Petroleo, G\xa0s Natural e Biocombust\xa1veis;;;;;;;;\n;;;;;;;;\nRELA\x80\xc7O DE MULTAS APLICADAS EM 2025  (Relat\xa2rio extra\xa1do em janeiro/2026);;;;;;;;\nFonte: Sistema de Controle de Multas - SICOM;;;;;;;;\n;;;;;;;;\n;;;;;;;;\nStatus Processo;Superintend\x88ncia;N\xa3mero do Processo;N\xa3mero do DUF;CNPJ/CPF;Raz\xc6o Social;Vencimento GRM;Valor da Multa;Valor Total Pago\n".to_vec();
    for row in rows {
        raw.extend_from_slice(row.as_bytes());
        raw.push(b'\n');
    }
    raw
}
const ROW:&str="Processo Pago;SYNTHETIC;000000000000001;SYNTHETIC/1;11.222.333/0001-81;Synthetic company;28/02/2025;150000,00;180000,01";
fn parse(raw: Vec<u8>) -> station_prep::sanction_anp::FineBatch {
    parse_anp_fines("synthetic.csv", raw, 2025, &Limits::default()).unwrap()
}

#[test]
fn native_dates_money_and_extraction_precision_are_not_relabelled() {
    let batch = parse(source(&[ROW]));
    assert_eq!(
        (
            batch.counts.input,
            batch.counts.accepted,
            batch.counts.duplicates,
            batch.counts.quarantined
        ),
        (1, 1, 0, 0)
    );
    let row = &batch.snapshots[0];
    assert_eq!(row.due_on, "2025-02-28");
    assert_eq!(row.extraction_month, "2026-01");
    assert_eq!(row.amount_minor, 15000000);
    assert_eq!(row.paid_minor, 18000001);
    assert_eq!(row.amount_text, "150000,00");
    assert_eq!(row.status, "Processo Pago");
    assert_eq!(row.source_key, "11222333000181");
    assert_eq!(row.proceeding_ref, "000000000000001");
    assert_eq!(row.duf_ref, "SYNTHETIC/1");
    let text = serde_json::to_string(row).unwrap();
    for forbidden in [
        "decided_on",
        "as_of",
        "balance",
        "Synthetic company",
        "guilt",
        "finality",
    ] {
        assert!(!text.contains(forbidden));
    }
}

#[test]
fn repeated_proceedings_and_dufs_preserve_distinct_observations_without_summing() {
    let changed = ROW.replace("150000,00", "150001,00");
    let batch = parse(source(&[ROW, ROW, &changed]));
    assert_eq!(
        (
            batch.counts.input,
            batch.counts.accepted,
            batch.counts.duplicates,
            batch.counts.quarantined
        ),
        (3, 2, 1, 0)
    );
    assert_eq!(
        batch.snapshots[0].proceeding_ref,
        batch.snapshots[1].proceeding_ref
    );
    assert_ne!(
        batch.snapshots[0].observation_key,
        batch.snapshots[1].observation_key
    );
    let reversed = parse(source(&[&changed, ROW, ROW]));
    assert_eq!(
        batch
            .snapshots
            .iter()
            .map(|r| &r.observation_key)
            .collect::<Vec<_>>(),
        reversed
            .snapshots
            .iter()
            .map(|r| &r.observation_key)
            .collect::<Vec<_>>()
    );
    // Discarded company names do not manufacture additional observations.
    assert_eq!(
        parse(source(&[
            ROW,
            &ROW.replace("Synthetic company", "Other synthetic label")
        ]))
        .counts
        .duplicates,
        1
    );
}

#[test]
fn discarded_name_controls_do_not_quarantine_valid_minimized_observations() {
    let name = ROW.replace("Synthetic company", "Discarded\tname");
    assert_eq!(parse(source(&[&name])).counts.accepted, 1);
    let status = ROW.replace("Processo Pago", "Status\tcontrol");
    assert_eq!(parse(source(&[&status])).counts.quarantined, 1);
}

#[test]
fn cpf_invalid_company_and_bad_fields_are_redacted_and_accounted() {
    let cpf = ROW.replace("11.222.333/0001-81", "123.456.789-00");
    let invalid = ROW.replace("11.222.333/0001-81", "11.222.333/0001-82");
    let date = ROW.replace("28/02/2025", "29/02/2025");
    let money = ROW.replace("150000,00", "1.50000,00");
    let batch = parse(source(&[ROW, &cpf, &invalid, &date, &money]));
    assert_eq!(
        (
            batch.counts.input,
            batch.counts.accepted,
            batch.counts.quarantined
        ),
        (5, 1, 4)
    );
    let json = serde_json::to_string(&batch.quarantine).unwrap();
    for forbidden in [
        "123.456.789-00",
        "11.222.333/0001-82",
        "Synthetic company",
        "000000000000001",
    ] {
        assert!(!json.contains(forbidden));
    }
    assert!(batch.quarantine.iter().all(|r| r.source_key.is_none()));
}

#[test]
fn exact_money_calendar_and_unknown_status_remain_conservative() {
    for good in [
        "0,00",
        "1,01",
        "1234,56",
        "1.234,56",
        "92233720368547758,07",
    ] {
        assert_eq!(
            parse(source(&[&ROW.replace("150000,00", good)]))
                .counts
                .accepted,
            1,
            "{good}"
        );
    }
    for bad in [
        "",
        "01,00",
        "-1,00",
        "1.2,00",
        "1,1",
        "1.2345,00",
        "1e3",
        "92233720368547758,08",
        "+1,00",
        "1 000,00",
    ] {
        assert_eq!(
            parse(source(&[&ROW.replace("150000,00", bad)]))
                .counts
                .quarantined,
            1,
            "{bad}"
        );
    }
    assert_eq!(
        parse(source(&[&ROW.replace("28/02/2025", "29/02/2024")]))
            .counts
            .accepted,
        1
    );
    assert_eq!(
        parse(source(&[
            &ROW.replace("Processo Pago", "UNKNOWN SOURCE LABEL")
        ]))
        .snapshots[0]
            .status,
        "UNKNOWN SOURCE LABEL"
    );
}

#[test]
fn source_header_preamble_year_encoding_and_framing_fail_closed() {
    assert!(matches!(
        parse_anp_fines("x", source(&[ROW]), 2024, &Limits::default()),
        Err(FineError::Contract)
    ));
    let raw = source(&[ROW]);
    assert!(parse_anp_fines("x", raw[..raw.len() - 1].to_vec(), 2025, &Limits::default()).is_err());
    let broken = source(&[ROW])
        .into_iter()
        .map(|b| if b == 0x88 { b'x' } else { b })
        .collect();
    assert!(parse_anp_fines("x", broken, 2025, &Limits::default()).is_err());
    let extra = ROW.to_string() + ";extra";
    assert_eq!(parse(source(&[&extra])).counts.quarantined, 1);
    assert_eq!(parse(source(&[])).counts.input, 0);
}

#[test]
fn budgets_have_no_partial_batch_or_raw_error() {
    assert!(matches!(
        parse_anp_fines(
            "x",
            source(&[ROW]),
            2025,
            &Limits {
                max_bytes: 10,
                ..Limits::default()
            }
        ),
        Err(FineError::Budget)
    ));
    assert!(matches!(
        parse_anp_fines(
            "x",
            source(&[ROW, ROW]),
            2025,
            &Limits {
                max_rows: 1,
                ..Limits::default()
            }
        ),
        Err(FineError::Budget)
    ));
    let limits = Limits {
        max_field_bytes: 5,
        ..Limits::default()
    };
    assert_eq!(
        parse_anp_fines("x", source(&[ROW]), 2025, &limits)
            .unwrap()
            .counts
            .quarantined,
        1
    );
    assert!(parse_anp_fines(
        "../../private.csv",
        source(&[ROW]),
        2025,
        &Limits::default()
    )
    .is_err());
}

#[test]
fn private_cli_artifacts_have_exact_hashes_and_failure_cannot_replace_owned_files() {
    use sha2::{Digest, Sha256};
    use std::os::unix::fs::PermissionsExt;
    use std::process::Command;
    let dir = std::env::temp_dir().join(format!(
        "fine-cli-{}-{}",
        std::process::id(),
        std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .unwrap()
            .as_nanos()
    ));
    std::fs::create_dir(&dir).unwrap();
    let input = dir.join("synthetic.csv");
    let out = dir.join("prepared");
    let raw = source(&[ROW, &ROW.replace("11.222.333/0001-81", "123.456.789-00")]);
    std::fs::write(&input, &raw).unwrap();
    let run = Command::new(env!("CARGO_BIN_EXE_prepare_sanctions"))
        .arg(&input)
        .arg("2025")
        .arg(&out)
        .output()
        .unwrap();
    assert!(run.status.success(), "counts-only CLI must succeed");
    assert!(run.stderr.is_empty());
    assert_eq!(
        String::from_utf8(run.stdout).unwrap(),
        "input=2 accepted=1 duplicates=0 quarantined=1 artifacts=file-backed\n"
    );
    let manifest: serde_json::Value =
        serde_json::from_slice(&std::fs::read(out.join("manifest.json")).unwrap()).unwrap();
    assert_eq!(
        manifest["input_sha256"],
        format!("{:x}", Sha256::digest(&raw))
    );
    assert_eq!(manifest["extraction_month"], "2026-01");
    assert_eq!(
        std::fs::metadata(&out).unwrap().permissions().mode() & 0o777,
        0o700
    );
    for output in manifest["outputs"].as_array().unwrap() {
        let path = out.join(output["file"].as_str().unwrap());
        let bytes = std::fs::read(&path).unwrap();
        assert_eq!(
            std::fs::metadata(&path).unwrap().permissions().mode() & 0o777,
            0o600
        );
        assert_eq!(output["sha256"], format!("{:x}", Sha256::digest(&bytes)));
        assert_eq!(output["bytes"].as_u64().unwrap(), bytes.len() as u64);
        let text = String::from_utf8(bytes).unwrap();
        assert!(!text.contains("123.456.789-00"));
        assert!(!text.contains("Synthetic company"));
    }
    let preserved = std::fs::read(out.join("manifest.json")).unwrap();
    let refused = Command::new(env!("CARGO_BIN_EXE_prepare_sanctions"))
        .arg(&input)
        .arg("2025")
        .arg(&out)
        .output()
        .unwrap();
    assert!(!refused.status.success());
    assert!(refused.stdout.is_empty());
    assert_eq!(preserved, std::fs::read(out.join("manifest.json")).unwrap());
    std::fs::write(&input, b"private source body invalid\n").unwrap();
    let absent = dir.join("refused");
    let refused = Command::new(env!("CARGO_BIN_EXE_prepare_sanctions"))
        .arg(&input)
        .arg("2025")
        .arg(&absent)
        .output()
        .unwrap();
    assert!(!refused.status.success());
    assert!(!absent.exists());
    assert!(refused.stdout.is_empty());
    assert!(!String::from_utf8(refused.stderr)
        .unwrap()
        .contains("private source body"));
    std::fs::remove_dir_all(dir).unwrap();
}

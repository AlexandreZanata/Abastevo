//! Synthetic native-layout fixtures: no production station or coordinates.
use station_prep::{parse_quality_assays, Limits, RunState};
const HEADER:&str="DataColeta;IdNumeric;GrupoProduto;Produto;RazaoSocialPosto;CnpjPosto;Distribuidora;Endereço;Complemento;Bairro;Município;Latitude;Longitude;Uf;RegiaoPolitica;Ensaio;Resultado;UnidadeEnsaio;Conforme\n";
fn row(product: &str, assay: &str, result: &str, unit: &str, status: &str) -> String {
    format!("2026-09-04;501;synthetic;{product};Synthetic station;04.218.406/0001-04;Synthetic;Synthetic street;;Synthetic;Synthetic;;;MT;CENTRO-OESTE;{assay};{result};{unit};{status}\n")
}
#[test]
fn native_header_and_closed_vocabulary_are_accepted_losslessly() {
    let mut raw = HEADER.to_string();
    for product in [
        "GASOLINA C COMUM",
        "GASOLINA C COMUM ADITIVADA",
        "GASOLINA C PREMIUM",
        "ETANOL HIDRATADO COMUM",
        "ETANOL HIDRATADO ADITIVADO",
        "ÓLEO DIESEL B S10 - COMUM",
        "ÓLEO DIESEL B S10 - ADITIVADO",
        "ÓLEO DIESEL B S500 - COMUM",
        "ÓLEO DIESEL B S500 - ADITIVADO",
    ] {
        raw += &row(
            product,
            "Aspecto",
            "Límpido e isento de impurezas (LII)",
            "",
            "Sim",
        );
    }
    let b = parse_quality_assays("native.csv", raw.into_bytes(), &Limits::default()).unwrap();
    assert_eq!(b.state, RunState::Complete);
    assert_eq!(b.counts.input, 9);
    assert_eq!(b.counts.accepted, 9);
    assert_eq!(b.counts.quarantined, 0);
    let samples: std::collections::HashSet<_> = b.assays.iter().map(|x| &x.sample_ref).collect();
    assert_eq!(
        samples.len(),
        9,
        "native product disambiguates a repeated sample ID"
    );
    assert!(b.assays.iter().any(|x| x.fuel == "GASOLINE_PREMIUM_GRADE"));
    assert!(b
        .assays
        .iter()
        .all(|x| x.unit.is_empty() && x.conformity == "CONFORMING"));
}
#[test]
fn native_empty_and_comparator_results_stay_verbatim() {
    let raw = format!(
        "{HEADER}{}{}{}",
        row(
            "ETANOL HIDRATADO COMUM",
            "Material Não Volátil",
            "<5",
            "mg/100 mL",
            "Sim"
        ),
        row(
            "ETANOL HIDRATADO COMUM",
            "Aspecto",
            "  original text  ",
            "",
            "Não"
        ),
        row("ETANOL HIDRATADO COMUM", "Cor", "", "", "Sim")
    );
    let b = parse_quality_assays("native.csv", raw.into_bytes(), &Limits::default()).unwrap();
    assert_eq!(b.counts.accepted, 3);
    assert!(b.assays.iter().any(|x| x.result_text.is_empty()));
    assert!(b
        .assays
        .iter()
        .any(|x| x.result_text == "  original text  " && x.conformity == "NONCONFORMING"));
    assert!(b.assays.iter().any(|x| x.result_text == "<5"));
}
#[test]
fn native_schema_drift_stays_quarantined() {
    let raw = format!(
        "{HEADER}{}",
        row("GASOLINA C COMUM", "Aspecto", "LII", "", "Sim")
    )
    .replace("UnidadeEnsaio", "UnrecognizedUnit");
    let b = parse_quality_assays("drift.csv", raw.into_bytes(), &Limits::default()).unwrap();
    assert_eq!(b.state, RunState::Quarantined);
    assert!(b.assays.is_empty());
}

#[test]
fn native_invalid_date_and_limits_fail_safely() {
    let raw = format!(
        "{HEADER}{}",
        row("GASOLINA C COMUM", "Aspecto", "LII", "", "Sim")
    );
    let invalid = raw.replace("2026-09-04", "2026-02-30");
    let b = parse_quality_assays("invalid.csv", invalid.into_bytes(), &Limits::default()).unwrap();
    assert_eq!(b.counts.quarantined, 1);
    assert!(b.assays.is_empty());
    let capped = Limits {
        max_rows: 0,
        ..Limits::default()
    };
    assert!(parse_quality_assays("cap.csv", raw.into_bytes(), &capped).is_err());
}

#[test]
fn native_reorder_and_exact_replay_preserve_sample_support() {
    let a = row("GASOLINA C COMUM", "Aspecto", "LII", "", "Sim");
    let b = row(
        "GASOLINA C COMUM",
        "Teor de Etanol",
        "30.0",
        "% volume",
        "Não",
    );
    let x = parse_quality_assays(
        "native.csv",
        format!("{HEADER}{a}{b}{a}").into_bytes(),
        &Limits::default(),
    )
    .unwrap();
    let y = parse_quality_assays(
        "native.csv",
        format!("{HEADER}{b}{a}{a}").into_bytes(),
        &Limits::default(),
    )
    .unwrap();
    assert_eq!(x.counts.accepted, 2);
    assert_eq!(x.counts.duplicates, 1);
    assert_eq!(x.manifest_hash(), y.manifest_hash());
    assert_eq!(x.assays[0].sample_ref, x.assays[1].sample_ref);
}

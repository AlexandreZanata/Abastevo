//! EE-11 shared tariff vectors, executed against the Rust parity oracle.
//! The same reviewed rows run in Go and Kotlin; they are synthetic contract
//! examples, never provider billing terms or customer results.

use station_prep::ev_tariff::{
    calculate, parse_instant, Condition, Dimension, Estimate, Rate, Scenario, Tariff,
    TariffComponent, TariffError, UnknownReason,
};
use std::collections::HashSet;

const CSV: &str = include_str!("../../../contracts/testdata/ev/tariff-v1.csv");
const COLUMNS: usize = 22;

#[test]
fn shared_tariff_vectors() {
    let mut lines = CSV.lines();
    assert_eq!(
        lines.next().map(|header| header.split(',').count()),
        Some(COLUMNS)
    );
    let mut seen = HashSet::new();
    let mut count = 0;
    for line in lines.filter(|line| !line.is_empty()) {
        let row: Vec<&str> = line.split(',').collect();
        assert_eq!(row.len(), COLUMNS);
        assert!(seen.insert(row[0].to_string()), "duplicate {}", row[0]);
        count += 1;
        let (tariff, scenario) = decode(&row);
        let expected = row[21];
        if let Some(code) = expected.strip_prefix("ERROR:") {
            assert_eq!(
                calculate(&tariff, &scenario),
                Err(error(code)),
                "{} expected {expected}",
                row[0]
            );
        } else if let Some(reason) = expected.strip_prefix("UNKNOWN:") {
            assert_eq!(
                calculate(&tariff, &scenario),
                Ok(Estimate::Unknown(reason_enum(reason))),
                "{} expected {expected}",
                row[0]
            );
        } else {
            let cents: i128 = expected
                .strip_prefix("KNOWN:")
                .expect("known expectation")
                .parse()
                .expect("reviewed centavos");
            assert_eq!(
                calculate(&tariff, &scenario),
                Ok(Estimate::Known {
                    numerator: cents,
                    denominator: 1
                }),
                "{} expected {expected}",
                row[0]
            );
        }
    }
    assert_eq!(count, 42);
}

fn decode(row: &[&str]) -> (Tariff, Scenario) {
    let components = if row[20] == "-" {
        Vec::new()
    } else {
        row[20]
            .split('|')
            .map(|part| {
                let fields: Vec<&str> = part.split(':').collect();
                assert_eq!(fields.len(), 4, "{part}");
                let rate: Vec<&str> = fields[1].split('/').collect();
                assert_eq!(rate.len(), 2, "{}", fields[1]);
                TariffComponent {
                    dimension: Dimension::parse(fields[0]).expect("dimension"),
                    // Numeric construction keeps structurally invalid rates
                    // (e.g. 2/4) reachable so the engine must refuse them.
                    rate: Rate {
                        numerator: rate[0].parse().expect("rate numerator"),
                        denominator: rate[1].parse().expect("rate denominator"),
                    },
                    billing_step: fields[2].parse().expect("step"),
                    grace_seconds: fields[3].parse().expect("grace"),
                }
            })
            .collect()
    };
    let tariff = Tariff {
        condition: Condition::parse(row[1]).expect("tariff condition"),
        qualifier: nullable(row[2]),
        currency: row[5].to_string(),
        valid_from: parse_instant(row[11]).expect("valid_from"),
        valid_until: nullable(row[12]).map(|value| parse_instant(&value).expect("valid_until")),
        tax_basis: row[6].to_string(),
        mandatory_components_complete: row[7] == "1",
        components,
        minimum_centavos: optional(row[9]),
        maximum_centavos: optional(row[10]),
        billing_rounding: row[8].to_string(),
    };
    let scenario = Scenario {
        condition: Condition::parse(row[3]).expect("scenario condition"),
        qualifier: nullable(row[4]),
        window_start: parse_instant(row[13]).expect("window_start"),
        window_end: parse_instant(row[14]).expect("window_end"),
        energy_wh: optional(row[15]),
        charging_seconds: optional(row[16]),
        idle_seconds: optional(row[17]),
        session_count: optional(row[18]),
        parking_seconds: optional(row[19]),
    };
    (tariff, scenario)
}

fn nullable(value: &str) -> Option<String> {
    if value == "-" || value.is_empty() {
        None
    } else {
        Some(value.to_string())
    }
}

fn optional(value: &str) -> Option<i64> {
    nullable(value).map(|value| value.parse().expect("reviewed integer"))
}

fn error(code: &str) -> TariffError {
    match code {
        "invalid-tariff" => TariffError::InvalidTariff,
        "invalid-scenario" => TariffError::InvalidScenario,
        "ineligible-condition" => TariffError::IneligibleCondition,
        "missing-scenario-quantity" => TariffError::MissingScenarioQuantity,
        other => panic!("unknown error code {other}"),
    }
}

fn reason_enum(reason: &str) -> UnknownReason {
    match reason {
        "UNSUPPORTED_CURRENCY" => UnknownReason::UnsupportedCurrency,
        "UNKNOWN_FEES" => UnknownReason::UnknownFees,
        "MISMATCHED_WINDOW" => UnknownReason::MismatchedWindow,
        "OVERFLOW" => UnknownReason::Overflow,
        other => panic!("unknown reason {other}"),
    }
}

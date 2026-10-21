//! EE-11 shared tariff wire/arithmetic parity.
//!
//! This module is a reviewed parity oracle for the shared Go/Kotlin vectors,
//! not the production billing service. It uses bounded checked `i128`
//! arithmetic sized for the reviewed input envelope; product intermediates
//! outside that envelope are refused as overflow instead of wrapping.

const MAX_ENERGY_WH: i64 = 1_000_000_000;
const MAX_SECONDS: i64 = 31_622_400;
const MAX_CENTAVOS: i128 = 1_000_000_000_000;
const MAX_BILLING_STEP: i64 = 1_000_000_000;
const MAX_COMPONENTS: usize = 32;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct Rate {
    pub numerator: i128,
    pub denominator: i128,
}

impl Rate {
    /// Parses the frozen canonical decimal strings (no sign, exponent or
    /// leading zero except the explicit zero numerator) and validates bounds
    /// and reduction, matching the Go/Kotlin wire rules.
    pub fn parse(numerator: &str, denominator: &str) -> Option<Self> {
        fn canonical(value: &str, max_digits: usize, max: i128) -> Option<i128> {
            if value.is_empty()
                || value.len() > max_digits
                || (value.len() > 1 && value.starts_with('0'))
                || !value.bytes().all(|b| b.is_ascii_digit())
            {
                return None;
            }
            let parsed: i128 = value.parse().ok()?;
            (parsed <= max).then_some(parsed)
        }
        let rate = Self {
            numerator: canonical(numerator, 13, 1_000_000_000_000)?,
            denominator: canonical(denominator, 10, 1_000_000_000)?,
        };
        validate_rate(rate).ok()?;
        Some(rate)
    }
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Dimension {
    EnergyWh,
    ChargingSecond,
    IdleSecond,
    Session,
    ParkingSecond,
}

impl Dimension {
    pub fn parse(value: &str) -> Option<Self> {
        match value {
            "ENERGY_WH" => Some(Self::EnergyWh),
            "CHARGING_SECOND" => Some(Self::ChargingSecond),
            "IDLE_SECOND" => Some(Self::IdleSecond),
            "SESSION" => Some(Self::Session),
            "PARKING_SECOND" => Some(Self::ParkingSecond),
            _ => None,
        }
    }

    fn is_time(self) -> bool {
        matches!(
            self,
            Self::ChargingSecond | Self::IdleSecond | Self::ParkingSecond
        )
    }
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct TariffComponent {
    pub dimension: Dimension,
    pub rate: Rate,
    pub billing_step: i64,
    pub grace_seconds: i64,
}

#[derive(Debug, Clone)]
pub struct Tariff {
    pub condition: Condition,
    pub qualifier: Option<String>,
    pub currency: String,
    pub valid_from: i64,
    pub valid_until: Option<i64>,
    pub tax_basis: String,
    pub mandatory_components_complete: bool,
    pub components: Vec<TariffComponent>,
    pub minimum_centavos: Option<i64>,
    pub maximum_centavos: Option<i64>,
    pub billing_rounding: String,
}

#[derive(Debug, Clone)]
pub struct Scenario {
    pub condition: Condition,
    pub qualifier: Option<String>,
    pub window_start: i64,
    pub window_end: i64,
    pub energy_wh: Option<i64>,
    pub charging_seconds: Option<i64>,
    pub idle_seconds: Option<i64>,
    pub session_count: Option<i64>,
    pub parking_seconds: Option<i64>,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Condition {
    Standard,
    App,
    Member,
    Employer,
    Unknown,
}

impl Condition {
    pub fn parse(value: &str) -> Option<Self> {
        match value {
            "STANDARD" => Some(Self::Standard),
            "APP" => Some(Self::App),
            "MEMBER" => Some(Self::Member),
            "EMPLOYER" => Some(Self::Employer),
            "UNKNOWN" => Some(Self::Unknown),
            _ => None,
        }
    }
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum TariffError {
    InvalidTariff,
    InvalidScenario,
    IneligibleCondition,
    MissingScenarioQuantity,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum UnknownReason {
    UnsupportedCurrency,
    UnknownFees,
    MismatchedWindow,
    Overflow,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub enum Estimate {
    Known { numerator: i128, denominator: i128 },
    Unknown(UnknownReason),
}

pub fn calculate(tariff: &Tariff, scenario: &Scenario) -> Result<Estimate, TariffError> {
    validate_tariff(tariff)?;
    validate_scenario(scenario)?;
    if tariff.condition == Condition::Unknown
        || scenario.condition == Condition::Unknown
        || tariff.condition != scenario.condition
        || tariff.qualifier != scenario.qualifier
    {
        return Err(TariffError::IneligibleCondition);
    }
    if tariff.currency != "BRL" {
        return Ok(Estimate::Unknown(UnknownReason::UnsupportedCurrency));
    }
    if tariff.tax_basis != "INCLUDED"
        || !tariff.mandatory_components_complete
        || tariff.minimum_centavos.is_some()
        || tariff.maximum_centavos.is_some()
        || tariff.billing_rounding != "FINAL_HALF_UP_CENTAVO"
    {
        return Ok(Estimate::Unknown(UnknownReason::UnknownFees));
    }
    if scenario.window_start < tariff.valid_from
        || tariff
            .valid_until
            .is_some_and(|until| scenario.window_end > until)
    {
        return Ok(Estimate::Unknown(UnknownReason::MismatchedWindow));
    }

    let mut quantities = Vec::with_capacity(tariff.components.len());
    let mut has_energy = false;
    for component in &tariff.components {
        quantities.push(charged_quantity(component, scenario)?);
        if component.dimension == Dimension::EnergyWh {
            has_energy = true;
        }
    }
    if !has_energy {
        return Ok(Estimate::Unknown(UnknownReason::UnknownFees));
    }

    let mut sum = (0i128, 1i128);
    for (index, component) in tariff.components.iter().enumerate() {
        let quantity = i128::from(quantities[index]);
        let numerator = component
            .rate
            .numerator
            .checked_mul(quantity)
            .ok_or(TariffError::InvalidTariff)?;
        let denominator = component.rate.denominator;
        let Some(next) = add(sum, (numerator, denominator)) else {
            return Ok(Estimate::Unknown(UnknownReason::Overflow));
        };
        sum = next;
    }

    let Some(rounded) = half_up(sum) else {
        return Ok(Estimate::Unknown(UnknownReason::Overflow));
    };
    if rounded > MAX_CENTAVOS {
        return Ok(Estimate::Unknown(UnknownReason::Overflow));
    }
    Ok(Estimate::Known {
        numerator: rounded,
        denominator: 1,
    })
}

fn validate_tariff(tariff: &Tariff) -> Result<(), TariffError> {
    if tariff.currency.len() != 3 || !tariff.currency.bytes().all(|b| b.is_ascii_uppercase()) {
        return Err(TariffError::InvalidTariff);
    }
    if tariff
        .qualifier
        .as_ref()
        .is_some_and(|q| q.is_empty() || q.len() > 128)
    {
        return Err(TariffError::InvalidTariff);
    }
    match tariff.condition {
        Condition::Standard | Condition::Unknown => {
            if tariff.qualifier.is_some() {
                return Err(TariffError::InvalidTariff);
            }
        }
        Condition::App | Condition::Member | Condition::Employer => {
            if tariff.qualifier.is_none() {
                return Err(TariffError::InvalidTariff);
            }
        }
    }
    if tariff
        .valid_until
        .is_some_and(|until| until < tariff.valid_from)
    {
        return Err(TariffError::InvalidTariff);
    }
    if !matches!(
        tariff.tax_basis.as_str(),
        "INCLUDED" | "EXCLUDED" | "UNKNOWN"
    ) {
        return Err(TariffError::InvalidTariff);
    }
    if !matches!(
        tariff.billing_rounding.as_str(),
        "FINAL_HALF_UP_CENTAVO" | "SOURCE_SPECIFIC_UNSUPPORTED"
    ) {
        return Err(TariffError::InvalidTariff);
    }
    if tariff.components.is_empty() || tariff.components.len() > MAX_COMPONENTS {
        return Err(TariffError::InvalidTariff);
    }
    let mut seen = [false; 5];
    for component in &tariff.components {
        let slot = match component.dimension {
            Dimension::EnergyWh => 0,
            Dimension::ChargingSecond => 1,
            Dimension::IdleSecond => 2,
            Dimension::Session => 3,
            Dimension::ParkingSecond => 4,
        };
        if seen[slot] {
            return Err(TariffError::InvalidTariff);
        }
        seen[slot] = true;
        validate_rate(component.rate)?;
        if component.billing_step < 1
            || component.billing_step > MAX_BILLING_STEP
            || component.grace_seconds < 0
            || component.grace_seconds > MAX_SECONDS
        {
            return Err(TariffError::InvalidTariff);
        }
        if component.grace_seconds > 0 && !component.dimension.is_time() {
            return Err(TariffError::InvalidTariff);
        }
        if component.dimension == Dimension::Session && component.billing_step != 1 {
            return Err(TariffError::InvalidTariff);
        }
    }
    for value in [tariff.minimum_centavos, tariff.maximum_centavos]
        .into_iter()
        .flatten()
    {
        if value < 0 || i128::from(value) > MAX_CENTAVOS {
            return Err(TariffError::InvalidTariff);
        }
    }
    if let (Some(minimum), Some(maximum)) = (tariff.minimum_centavos, tariff.maximum_centavos) {
        if minimum > maximum {
            return Err(TariffError::InvalidTariff);
        }
    }
    Ok(())
}

fn validate_scenario(scenario: &Scenario) -> Result<(), TariffError> {
    if scenario.window_end <= scenario.window_start {
        return Err(TariffError::InvalidScenario);
    }
    if scenario
        .qualifier
        .as_ref()
        .is_some_and(|q| q.is_empty() || q.len() > 128)
    {
        return Err(TariffError::InvalidScenario);
    }
    match scenario.condition {
        Condition::Standard | Condition::Unknown => {
            if scenario.qualifier.is_some() {
                return Err(TariffError::InvalidScenario);
            }
        }
        Condition::App | Condition::Member | Condition::Employer => {
            if scenario.qualifier.is_none() {
                return Err(TariffError::InvalidScenario);
            }
        }
    }
    Ok(())
}

fn validate_rate(rate: Rate) -> Result<(), TariffError> {
    if rate.numerator < 0
        || rate.numerator > 1_000_000_000_000
        || rate.denominator <= 0
        || rate.denominator > 1_000_000_000
    {
        return Err(TariffError::InvalidTariff);
    }
    if gcd(rate.numerator, rate.denominator) != 1 {
        return Err(TariffError::InvalidTariff);
    }
    Ok(())
}

fn gcd(mut a: i128, mut b: i128) -> i128 {
    a = a.abs();
    while b != 0 {
        (a, b) = (b, a % b);
    }
    a
}

fn charged_quantity(component: &TariffComponent, scenario: &Scenario) -> Result<i64, TariffError> {
    let quantity = match component.dimension {
        Dimension::EnergyWh => scenario.energy_wh,
        Dimension::ChargingSecond => scenario.charging_seconds,
        Dimension::IdleSecond => scenario.idle_seconds,
        Dimension::Session => scenario.session_count,
        Dimension::ParkingSecond => scenario.parking_seconds,
    }
    .ok_or(TariffError::MissingScenarioQuantity)?;
    match component.dimension {
        Dimension::EnergyWh => {
            if !(0..=MAX_ENERGY_WH).contains(&quantity) {
                return Err(TariffError::InvalidScenario);
            }
        }
        Dimension::Session => {
            if quantity != 1 {
                return Err(TariffError::InvalidScenario);
            }
            return Ok(1);
        }
        _ => {
            if !(0..=MAX_SECONDS).contains(&quantity) {
                return Err(TariffError::InvalidScenario);
            }
        }
    }
    let mut value = quantity;
    if component.dimension.is_time() && component.grace_seconds > 0 {
        if value <= component.grace_seconds {
            return Ok(0);
        }
        value -= component.grace_seconds;
    }
    if value == 0 {
        return Ok(0);
    }
    if value % component.billing_step != 0 {
        value = (value / component.billing_step + 1) * component.billing_step;
    }
    Ok(value)
}

fn add(a: (i128, i128), b: (i128, i128)) -> Option<(i128, i128)> {
    let numerator = a.0.checked_mul(b.1)?.checked_add(b.0.checked_mul(a.1)?)?;
    let denominator = a.1.checked_mul(b.1)?;
    Some((numerator, denominator))
}

/// floor(sum + 1/2) for a nonnegative exact sum, refusing nonpositive den.
fn half_up(sum: (i128, i128)) -> Option<i128> {
    if sum.1 <= 0 {
        return None;
    }
    let numerator = sum.0.checked_mul(2)?.checked_add(sum.1)?;
    let denominator = sum.1.checked_mul(2)?;
    Some(numerator / denominator)
}

/// Minimal RFC3339 instant parser for the reviewed vectors (Z or ±HH:MM).
pub fn parse_instant(value: &str) -> Option<i64> {
    let bytes = value.as_bytes();
    if bytes.len() < 20
        || bytes[4] != b'-'
        || bytes[7] != b'-'
        || bytes[10] != b'T'
        || bytes[13] != b':'
        || bytes[16] != b':'
    {
        return None;
    }
    let number = |range: std::ops::Range<usize>| -> Option<i64> {
        let text = value.get(range)?;
        if !text.bytes().all(|b| b.is_ascii_digit()) {
            return None;
        }
        text.parse().ok()
    };
    let (year, month, day) = (number(0..4)?, number(5..7)?, number(8..10)?);
    let (hour, minute, second) = (number(11..13)?, number(14..16)?, number(17..19)?);
    if !(1..=12).contains(&month)
        || !(1..=31).contains(&day)
        || hour > 23
        || minute > 59
        || second > 59
    {
        return None;
    }
    let (offset, rest) = match bytes.get(19)? {
        b'Z' => (0, 20),
        b'+' | b'-' => {
            let sign = if bytes[19] == b'-' { -1 } else { 1 };
            let hours = number(20..22)?;
            let minutes = number(23..25)?;
            if hours > 23 || minutes > 59 || bytes.get(22) != Some(&b':') {
                return None;
            }
            (sign * (hours * 3600 + minutes * 60), 25)
        }
        _ => return None,
    };
    if rest != bytes.len() {
        return None;
    }
    let days = days_from_civil(year, month, day);
    Some(days * 86_400 + hour * 3_600 + minute * 60 + second - offset)
}

/// Howard Hinnant's exact days-from-civil algorithm.
fn days_from_civil(mut year: i64, month: i64, day: i64) -> i64 {
    if month <= 2 {
        year -= 1;
    }
    let era = if year >= 0 { year } else { year - 399 } / 400;
    let year_of_era = year - era * 400;
    let day_of_year = (153 * (if month > 2 { month - 3 } else { month + 9 }) + 2) / 5 + day - 1;
    let day_of_era = year_of_era * 365 + year_of_era / 4 - year_of_era / 100 + day_of_year;
    era * 146_097 + day_of_era - 719_468
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn rate_parser_requires_canonical_reduced_values() {
        assert_eq!(
            Rate::parse("9", "50"),
            Some(Rate {
                numerator: 9,
                denominator: 50
            })
        );
        assert_eq!(
            Rate::parse("0", "1"),
            Some(Rate {
                numerator: 0,
                denominator: 1
            })
        );
        assert_eq!(Rate::parse("2", "4"), None);
        assert_eq!(Rate::parse("01", "1"), None);
        assert_eq!(Rate::parse("1", "0"), None);
        assert_eq!(Rate::parse("1000000000001", "1"), None);
        assert_eq!(Rate::parse("1", "1000000001"), None);
    }

    #[test]
    fn parsed_instants_compare_across_offsets() {
        assert_eq!(
            parse_instant("2026-10-21T10:00:00Z"),
            parse_instant("2026-10-21T07:00:00-03:00")
        );
        assert_eq!(parse_instant("2026-10-21T10:00:00Z"), Some(1_792_576_800));
        assert_eq!(parse_instant("2026-10-21T10:00:00"), None);
        assert_eq!(parse_instant("2026-13-21T10:00:00Z"), None);
    }
}

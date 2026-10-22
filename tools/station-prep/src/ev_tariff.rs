//! EE-11 shared tariff wire/arithmetic parity.
//!
//! This module is a reviewed parity oracle for the shared Go/Kotlin vectors,
//! not the production billing service. Checked 256-bit positive intermediates
//! cover all five distinct dimensions under the frozen bounds, even when
//! denominators are coprime; a large intermediate is not a monetary overflow.

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
    pub valid_from: i128,
    pub valid_until: Option<i128>,
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
    pub window_start: i128,
    pub window_end: i128,
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

    let mut sum = (Wide::from(0), Wide::from(1));
    for (index, component) in tariff.components.iter().enumerate() {
        let quantity = quantities[index] as u64;
        let denominator = component.rate.denominator as u64;
        let next = sum
            .0
            .checked_mul(denominator)
            .and_then(|left| {
                sum.1
                    .checked_mul(component.rate.numerator as u64)?
                    .checked_mul(quantity)?
                    .checked_add(left)
            })
            .zip(sum.1.checked_mul(denominator));
        let Some(next) = next else {
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
        .is_some_and(|q| q.is_empty() || q.chars().count() > 128)
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
    for (dimension, quantity) in [
        (Dimension::EnergyWh, scenario.energy_wh),
        (Dimension::ChargingSecond, scenario.charging_seconds),
        (Dimension::IdleSecond, scenario.idle_seconds),
        (Dimension::Session, scenario.session_count),
        (Dimension::ParkingSecond, scenario.parking_seconds),
    ] {
        if let Some(quantity) = quantity {
            validate_quantity(dimension, quantity)?;
        }
    }
    if scenario
        .qualifier
        .as_ref()
        .is_some_and(|q| q.is_empty() || q.chars().count() > 128)
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
    validate_quantity(component.dimension, quantity)?;
    if component.dimension == Dimension::Session {
        return Ok(1);
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

fn validate_quantity(dimension: Dimension, quantity: i64) -> Result<(), TariffError> {
    match dimension {
        Dimension::EnergyWh => {
            if !(0..=MAX_ENERGY_WH).contains(&quantity) {
                return Err(TariffError::InvalidScenario);
            }
        }
        Dimension::Session => {
            if quantity != 1 {
                return Err(TariffError::InvalidScenario);
            }
        }
        _ => {
            if !(0..=MAX_SECONDS).contains(&quantity) {
                return Err(TariffError::InvalidScenario);
            }
        }
    }
    Ok(())
}

// At most five unique dimensions, each den<=1e9: common denominator <2^150.
// num<=1e12 and rounded quantity<2e9 yield total numerator <2^193.
// Four limbs cover the entire accepted envelope without a dependency.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
struct Wide([u64; 4]);

impl Wide {
    fn from(value: u64) -> Self {
        Self([value, 0, 0, 0])
    }
    fn checked_add(self, other: Self) -> Option<Self> {
        let mut words = [0; 4];
        let mut carry = 0u128;
        for (index, word) in words.iter_mut().enumerate() {
            let value = u128::from(self.0[index]) + u128::from(other.0[index]) + carry;
            *word = value as u64;
            carry = value >> 64;
        }
        (carry == 0).then_some(Self(words))
    }
    fn checked_mul(self, factor: u64) -> Option<Self> {
        let mut words = [0; 4];
        let mut carry = 0u128;
        for (index, word) in words.iter_mut().enumerate() {
            let value = u128::from(self.0[index]) * u128::from(factor) + carry;
            *word = value as u64;
            carry = value >> 64;
        }
        (carry == 0).then_some(Self(words))
    }
    fn at_least(self, other: Self) -> bool {
        for index in (0..4).rev() {
            if self.0[index] != other.0[index] {
                return self.0[index] > other.0[index];
            }
        }
        true
    }
}

/// Exact half-up using monotone integer comparisons; never round intermediates.
fn half_up(sum: (Wide, Wide)) -> Option<i128> {
    if sum.1 == Wide::from(0) {
        return None;
    }
    let twice = sum.0.checked_mul(2)?;
    let maximum = MAX_CENTAVOS as u64;
    if twice.at_least(sum.1.checked_mul(2 * maximum + 1)?) {
        return Some(MAX_CENTAVOS + 1);
    }
    let (mut low, mut high) = (0, maximum);
    while low < high {
        let middle = low + (high - low).div_ceil(2);
        if twice.at_least(sum.1.checked_mul(2 * middle - 1)?) {
            low = middle;
        } else {
            high = middle - 1;
        }
    }
    Some(i128::from(low))
}

/// RFC3339 instants at nanosecond precision, including calendar validation.
/// Only the typed shared decoder uses this helper; it never normalizes bad dates.
pub fn parse_instant(value: &str) -> Option<i128> {
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
        if text.is_empty() || !text.bytes().all(|b| b.is_ascii_digit()) {
            return None;
        }
        text.parse().ok()
    };
    let (year, month, day) = (number(0..4)?, number(5..7)?, number(8..10)?);
    let (hour, minute, second) = (number(11..13)?, number(14..16)?, number(17..19)?);
    let leap = year % 4 == 0 && (year % 100 != 0 || year % 400 == 0);
    let last_day = match month {
        1 | 3 | 5 | 7 | 8 | 10 | 12 => 31,
        4 | 6 | 9 | 11 => 30,
        2 => {
            if leap {
                29
            } else {
                28
            }
        }
        _ => return None,
    };
    if !(1..=last_day).contains(&day) || hour > 23 || minute > 59 || second > 59 {
        return None;
    }
    let mut index = 19;
    let mut nanos = 0;
    if bytes.get(index) == Some(&b'.') {
        index += 1;
        let start = index;
        while bytes.get(index).is_some_and(u8::is_ascii_digit) {
            index += 1;
        }
        let length = index - start;
        if length == 0 || length > 9 {
            return None;
        }
        nanos = number(start..index)? * 10i64.pow((9 - length) as u32);
    }
    let (offset, rest) = match bytes.get(index)? {
        b'Z' => (0, index + 1),
        b'+' | b'-' => {
            let sign = if bytes[index] == b'-' { -1 } else { 1 };
            let hours = number(index + 1..index + 3)?;
            let minutes = number(index + 4..index + 6)?;
            if hours > 23 || minutes > 59 || bytes.get(index + 3) != Some(&b':') {
                return None;
            }
            (sign * (hours * 3600 + minutes * 60), index + 6)
        }
        _ => return None,
    };
    if rest != bytes.len() {
        return None;
    }
    let days = days_from_civil(year, month, day);
    Some(
        i128::from(days * 86_400 + hour * 3_600 + minute * 60 + second - offset) * 1_000_000_000
            + i128::from(nanos),
    )
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
    fn wide_money_boundary_and_carries() {
        assert_eq!(
            Wide::from(u64::MAX).checked_mul(2),
            Some(Wide([u64::MAX - 1, 1, 0, 0]))
        );
        assert_eq!(Wide([u64::MAX; 4]).checked_add(Wide::from(1)), None);
        assert_eq!(Wide([u64::MAX; 4]).checked_mul(2), None);
        assert_eq!(half_up((Wide::from(3), Wide::from(2))), Some(2));
        assert_eq!(half_up((Wide::from(1), Wide::from(0))), None);
        let denominator = Wide([0, 0, 1, 0]);
        assert_eq!(
            half_up((
                denominator.checked_mul(MAX_CENTAVOS as u64).unwrap(),
                denominator
            )),
            Some(MAX_CENTAVOS)
        );
        assert_eq!(
            half_up((
                denominator.checked_mul(MAX_CENTAVOS as u64 + 1).unwrap(),
                denominator
            )),
            Some(MAX_CENTAVOS + 1)
        );
    }

    #[test]
    fn instant_decoder_preserves_nanos_and_rejects_impossible_dates() {
        for invalid in [
            "2026-02-31T10:00:00Z",
            "2026-04-31T10:00:00Z",
            "1900-02-29T10:00:00Z",
            "2026-02-29T10:00:00Z",
            "2026-10-21T10:00:00.Z",
            "2026-10-21T10:00:00.0000000001Z",
        ] {
            assert_eq!(parse_instant(invalid), None, "{invalid}");
        }
        assert!(parse_instant("2000-02-29T10:00:00Z").is_some());
        let a = parse_instant("2026-10-21T10:00:00.000000001Z").unwrap();
        let b = parse_instant("2026-10-21T07:00:00.000000001-03:00").unwrap();
        assert_eq!(a, b);
        assert_eq!(
            a + 1,
            parse_instant("2026-10-21T10:00:00.000000002Z").unwrap()
        );
    }

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
        assert_eq!(
            parse_instant("2026-10-21T10:00:00Z"),
            Some(1_792_576_800_000_000_000)
        );
        assert_eq!(parse_instant("2026-10-21T10:00:00"), None);
        assert_eq!(parse_instant("2026-13-21T10:00:00Z"), None);
    }
}

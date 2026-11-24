import Foundation

/// Exact full-to-full consumption, cost and energy rules (B-BR-PR16/PR17, EE-23).
///
/// Swift port of Android `domain/premium/PremiumConsumption.kt`, `PremiumEnergy.kt`
/// and `PremiumBounds.kt`, which mirror `backend/internal/modules/metrics`.
/// Integer math only; intermediates that exceed Int64 use 128-bit magnitudes.
/// Both sides execute `contracts/testdata/premium/financial-execution-v1.csv`
/// and `energy-execution-v1.csv`.
/// Zero postfix unwrap operators anywhere in this file.

/// Unsigned 128-bit magnitude for exact rationals (iOS 17 has no `UInt128`).
public struct LedgerWide: Equatable, Comparable, Sendable {
    public let hi: UInt64
    public let lo: UInt64

    public init(hi: UInt64, lo: UInt64) {
        self.hi = hi
        self.lo = lo
    }

    public init(_ value: UInt64) {
        self.hi = 0
        self.lo = value
    }

    public static let zero = LedgerWide(0)
    public static let one = LedgerWide(1)

    public var isZero: Bool { hi == 0 && lo == 0 }

    public static func < (lhs: LedgerWide, rhs: LedgerWide) -> Bool {
        if lhs.hi != rhs.hi { return lhs.hi < rhs.hi }
        return lhs.lo < rhs.lo
    }

    public static func product(_ lhs: UInt64, _ rhs: UInt64) -> LedgerWide {
        let (high, low) = lhs.multipliedFullWidth(by: rhs)
        return LedgerWide(hi: high, lo: low)
    }

    /// Nil on 128-bit overflow.
    public func multiplied(by factor: UInt64) -> LedgerWide? {
        let (lowHigh, lowLow) = lo.multipliedFullWidth(by: factor)
        let highProduct = hi.multipliedReportingOverflow(by: factor)
        if highProduct.overflow { return nil }
        let high = highProduct.partialValue.addingReportingOverflow(lowHigh)
        if high.overflow { return nil }
        return LedgerWide(hi: high.partialValue, lo: lowLow)
    }

    /// Wrapping subtraction; exact whenever `self >= other`.
    public func subtracting(_ other: LedgerWide) -> LedgerWide {
        let low = lo.subtractingReportingOverflow(other.lo)
        let borrow: UInt64 = low.overflow ? 1 : 0
        return LedgerWide(hi: hi &- other.hi &- borrow, lo: low.partialValue)
    }

    private func bit(at index: Int) -> UInt64 {
        if index >= 64 {
            return (hi >> UInt64(index - 64)) & 1
        }
        return (lo >> UInt64(index)) & 1
    }

    private func settingBit(at index: Int) -> LedgerWide {
        if index >= 64 {
            return LedgerWide(hi: hi | (UInt64(1) << UInt64(index - 64)), lo: lo)
        }
        return LedgerWide(hi: hi, lo: lo | (UInt64(1) << UInt64(index)))
    }

    /// Restoring binary long division; `divisor` must be nonzero.
    public func quotientAndRemainder(dividingBy divisor: LedgerWide) -> (quotient: LedgerWide, remainder: LedgerWide) {
        if divisor.isZero { return (LedgerWide.zero, self) }
        if hi == 0 && divisor.hi == 0 {
            return (LedgerWide(lo / divisor.lo), LedgerWide(lo % divisor.lo))
        }
        var quotient = LedgerWide.zero
        var remainder = LedgerWide.zero
        var index = 127
        while index >= 0 {
            let carry = (remainder.hi >> 63) == 1
            remainder = LedgerWide(hi: (remainder.hi << 1) | (remainder.lo >> 63), lo: (remainder.lo << 1) | bit(at: index))
            if carry || !(remainder < divisor) {
                remainder = remainder.subtracting(divisor)
                quotient = quotient.settingBit(at: index)
            }
            index -= 1
        }
        return (quotient, remainder)
    }

    public static func gcd(_ lhs: LedgerWide, _ rhs: LedgerWide) -> LedgerWide {
        var x = lhs
        var y = rhs
        while !y.isZero {
            let remainder = x.quotientAndRemainder(dividingBy: y).remainder
            x = y
            y = remainder
        }
        return x
    }

    public var decimalString: String {
        if hi == 0 { return String(lo) }
        var text = ""
        var current = self
        let ten = LedgerWide(10)
        while !current.isZero {
            let step = current.quotientAndRemainder(dividingBy: ten)
            text = String(step.remainder.lo) + text
            current = step.quotient
        }
        return text
    }
}

/// Reduced rational strings `n` or `n/d`, identical to Kotlin `PremiumBounds.scaledRational`.
public enum LedgerRational {
    public static func fraction(negative: Bool, numerator: LedgerWide, denominator: LedgerWide) -> String {
        guard !denominator.isZero else { return "" }
        let divisor = LedgerWide.gcd(numerator, denominator)
        let reducedNumerator = numerator.quotientAndRemainder(dividingBy: divisor).quotient
        let reducedDenominator = denominator.quotientAndRemainder(dividingBy: divisor).quotient
        let sign = negative && !reducedNumerator.isZero ? "-" : ""
        if reducedDenominator == LedgerWide.one {
            return sign + reducedNumerator.decimalString
        }
        return sign + reducedNumerator.decimalString + "/" + reducedDenominator.decimalString
    }

    /// `num * scale / den`; empty when `den <= 0`.
    public static func scaled(_ num: Int64, scale: Int64, den: Int64) -> String {
        if den <= 0 { return "" }
        return fraction(
            negative: (num < 0) != (scale < 0),
            numerator: LedgerWide.product(num.magnitude, scale.magnitude),
            denominator: LedgerWide(den.magnitude)
        )
    }

    public static func ratio(_ num: Int64, _ den: Int64) -> String {
        return scaled(num, scale: 1, den: den)
    }

    /// `(cd * bq - bd * cq) * 100 / (bd * cq)` for positive inputs; nil on 128-bit overflow.
    public static func changePercent(
        baseDistance: Int64,
        baseQuantity: Int64,
        currentDistance: Int64,
        currentQuantity: Int64
    ) -> String? {
        guard baseDistance > 0, baseQuantity > 0, currentDistance > 0, currentQuantity > 0 else { return nil }
        let current = LedgerWide.product(currentDistance.magnitude, baseQuantity.magnitude)
        let base = LedgerWide.product(baseDistance.magnitude, currentQuantity.magnitude)
        let negative = current < base
        let difference = negative ? base.subtracting(current) : current.subtracting(base)
        guard let scaled = difference.multiplied(by: 100) else { return nil }
        return fraction(negative: negative, numerator: scaled, denominator: base)
    }

    /// Display-only half-up rounding of an exact rational such as `800/3`; nil when malformed.
    public static func decimal(_ rational: String, fractionDigits: Int) -> Decimal? {
        guard !rational.isEmpty, fractionDigits >= 0, fractionDigits <= 6 else { return nil }
        let parts = rational.split(separator: "/", omittingEmptySubsequences: false).map(String.init)
        guard parts.count == 1 || parts.count == 2,
              let numerator = Decimal(string: parts[0], locale: Locale(identifier: "en_US_POSIX")) else {
            return nil
        }
        var value = numerator
        if parts.count == 2 {
            guard let denominator = Decimal(string: parts[1], locale: Locale(identifier: "en_US_POSIX")),
                  denominator != 0 else {
                return nil
            }
            value = numerator / denominator
        }
        var rounded = Decimal()
        NSDecimalRound(&rounded, &value, fractionDigits, .plain)
        return rounded
    }


    /// Round qualified ratios for display without changing the exact stored value.
    public static func displayText(
        _ rational: String, fractionDigits: Int, divideBy: Decimal = 1, locale: Locale = Locale(identifier: "pt_BR")
    ) -> String? {
        guard fractionDigits >= 0, var value = decimal(rational, fractionDigits: 6), divideBy != 0 else { return nil }
        value = value / divideBy
        var rounded = Decimal()
        NSDecimalRound(&rounded, &value, fractionDigits, .plain)
        return rounded.formatted(.number.precision(.fractionLength(0...fractionDigits)).locale(locale))
    }
}

/// Non-negative checked addition (`PremiumBounds.addNonNegative`).
public enum LedgerChecked {
    public static func add(_ lhs: Int64, _ rhs: Int64) -> Int64? {
        if lhs < 0 || rhs < 0 { return nil }
        let sum = lhs.addingReportingOverflow(rhs)
        return sum.overflow ? nil : sum.partialValue
    }
}

public struct LedgerFillInput: Equatable, Sendable {
    public let quantityMilli: Int64
    public let totalCentavos: Int64
    public let full: Bool
    public let fuel: LedgerFuel
    public let unit: LedgerUnit

    public init(quantityMilli: Int64, totalCentavos: Int64, full: Bool, fuel: LedgerFuel, unit: LedgerUnit) {
        self.quantityMilli = quantityMilli
        self.totalCentavos = totalCentavos
        self.full = full
        self.fuel = fuel
        self.unit = unit
    }
}

public struct LedgerIntervalInput: Equatable, Sendable {
    public let startingFillExcluded: LedgerFillInput
    public let fills: [LedgerFillInput]
    public let distanceMetres: Int64
    public let hasGap: Bool
    public let crossesSegment: Bool
    public let odometerMissing: Bool

    public init(
        startingFillExcluded: LedgerFillInput,
        fills: [LedgerFillInput],
        distanceMetres: Int64,
        hasGap: Bool = false,
        crossesSegment: Bool = false,
        odometerMissing: Bool = false
    ) {
        self.startingFillExcluded = startingFillExcluded
        self.fills = fills
        self.distanceMetres = distanceMetres
        self.hasGap = hasGap
        self.crossesSegment = crossesSegment
        self.odometerMissing = odometerMissing
    }
}

public struct LedgerConsumptionResult: Equatable, Sendable {
    public let eligibleQuantityMilli: Int64
    public let eligibleCostCentavos: Int64
    public let distanceMetres: Int64
    public let kmPerUnit: String
    public let centavosPerKm: String
    public let unknown: String

    public init(
        eligibleQuantityMilli: Int64 = 0,
        eligibleCostCentavos: Int64 = 0,
        distanceMetres: Int64 = 0,
        kmPerUnit: String = "",
        centavosPerKm: String = "",
        unknown: String = ""
    ) {
        self.eligibleQuantityMilli = eligibleQuantityMilli
        self.eligibleCostCentavos = eligibleCostCentavos
        self.distanceMetres = distanceMetres
        self.kmPerUnit = kmPerUnit
        self.centavosPerKm = centavosPerKm
        self.unknown = unknown
    }

    public static func unknown(_ reason: String) -> LedgerConsumptionResult {
        return LedgerConsumptionResult(unknown: reason)
    }
}

public struct LedgerCostResult: Equatable, Sendable {
    public let version: Int
    public let totalCentavos: Int64
    public let distanceMetres: Int64
    public let centavosPerKm: String
    public let method: String
    public let unknown: String

    public init(
        version: Int = LedgerBounds.calculationVersion,
        totalCentavos: Int64 = 0,
        distanceMetres: Int64 = 0,
        centavosPerKm: String = "",
        method: String = "",
        unknown: String = ""
    ) {
        self.version = version
        self.totalCentavos = totalCentavos
        self.distanceMetres = distanceMetres
        self.centavosPerKm = centavosPerKm
        self.method = method
        self.unknown = unknown
    }

    public static func unknown(_ reason: String) -> LedgerCostResult {
        return LedgerCostResult(unknown: reason)
    }
}

public struct LedgerWindowInput: Equatable, Sendable {
    public let startOdometerMetres: Int64
    public let endOdometerMetres: Int64
    public let startMillis: Int64
    public let endMillis: Int64
    public let hasGap: Bool
    public let crossesSegment: Bool

    public init(
        startOdometerMetres: Int64,
        endOdometerMetres: Int64,
        startMillis: Int64,
        endMillis: Int64,
        hasGap: Bool = false,
        crossesSegment: Bool = false
    ) {
        self.startOdometerMetres = startOdometerMetres
        self.endOdometerMetres = endOdometerMetres
        self.startMillis = startMillis
        self.endMillis = endMillis
        self.hasGap = hasGap
        self.crossesSegment = crossesSegment
    }
}

public struct LedgerExpenseInput: Equatable, Sendable {
    public let id: String
    public let amountCentavos: Int64
    public let occurredAtMillis: Int64
    public let odometerMetres: Int64?

    public init(id: String, amountCentavos: Int64, occurredAtMillis: Int64, odometerMetres: Int64? = nil) {
        self.id = id
        self.amountCentavos = amountCentavos
        self.occurredAtMillis = occurredAtMillis
        self.odometerMetres = odometerMetres
    }
}

public struct LedgerAggregateTotals: Equatable, Sendable {
    public let distanceMetres: Int64
    public let eligibleQuantityMilli: Int64
    public let eligibleCostCentavos: Int64
    public let kmPerUnit: String
    public let centavosPerKm: String
    public let unknown: String

    public init(
        distanceMetres: Int64 = 0,
        eligibleQuantityMilli: Int64 = 0,
        eligibleCostCentavos: Int64 = 0,
        kmPerUnit: String = "",
        centavosPerKm: String = "",
        unknown: String = ""
    ) {
        self.distanceMetres = distanceMetres
        self.eligibleQuantityMilli = eligibleQuantityMilli
        self.eligibleCostCentavos = eligibleCostCentavos
        self.kmPerUnit = kmPerUnit
        self.centavosPerKm = centavosPerKm
        self.unknown = unknown
    }
}

public struct LedgerComparisonResult: Equatable, Sendable {
    public let changePercent: String
    public let unknown: String

    public init(changePercent: String = "", unknown: String = "") {
        self.changePercent = changePercent
        self.unknown = unknown
    }
}

/// Port of Android `PremiumConsumption` (frozen UNKNOWN vocabulary and precedence).
public enum LedgerConsumption {
    public static func compute(_ input: LedgerIntervalInput) -> LedgerConsumptionResult {
        let all = [input.startingFillExcluded] + input.fills
        for fill in all {
            if fill.quantityMilli < 0 || fill.quantityMilli > LedgerBounds.maxQuantityMilli
                || fill.totalCentavos < 0 || fill.totalCentavos > LedgerBounds.maxTotalCentavos {
                return .unknown("input-out-of-bounds")
            }
        }
        if input.distanceMetres < 0 || input.distanceMetres > LedgerBounds.maxDistanceMetres {
            return .unknown("input-out-of-bounds")
        }
        if input.odometerMissing { return .unknown("odometer-missing") }
        if input.hasGap { return .unknown("declared-fill-gap") }
        if input.crossesSegment { return .unknown("odometer-segment-boundary") }
        if input.distanceMetres <= 0 { return .unknown("distance-nonpositive") }
        for fill in all where fill.unit != fill.fuel.unit {
            return .unknown("incompatible-unit")
        }
        if Set(all.map { $0.unit }).count > 1 { return .unknown("incompatible-unit") }
        let fuels = Set(all.map { $0.fuel })
        if fuels.contains(.cng) { return .unknown("unsupported-gnv-method") }
        if fuels.count > 1 { return .unknown("unsupported-mixed-method") }
        guard input.startingFillExcluded.full, let last = input.fills.last, last.full else {
            return .unknown("full-anchors-required")
        }
        var quantity: Int64 = 0
        var cost: Int64 = 0
        for fill in input.fills {
            guard let nextQuantity = LedgerChecked.add(quantity, fill.quantityMilli),
                  let nextCost = LedgerChecked.add(cost, fill.totalCentavos) else {
                return .unknown("input-out-of-bounds")
            }
            quantity = nextQuantity
            cost = nextCost
        }
        if quantity <= 0 { return .unknown("full-anchors-required") }
        return LedgerConsumptionResult(
            eligibleQuantityMilli: quantity,
            eligibleCostCentavos: cost,
            distanceMetres: input.distanceMetres,
            kmPerUnit: LedgerRational.ratio(input.distanceMetres, quantity),
            centavosPerKm: LedgerRational.scaled(cost, scale: 1000, den: input.distanceMetres)
        )
    }

    public static func fuelPurchasePerKm(eligibleCostCentavos: Int64, distanceMetres: Int64) -> LedgerCostResult {
        if eligibleCostCentavos < 0 || eligibleCostCentavos > LedgerBounds.maxTotalCentavos
            || distanceMetres < 0 || distanceMetres > LedgerBounds.maxDistanceMetres {
            return .unknown("input-out-of-bounds")
        }
        if distanceMetres <= 0 { return .unknown("distance-nonpositive") }
        return LedgerCostResult(
            totalCentavos: eligibleCostCentavos,
            distanceMetres: distanceMetres,
            centavosPerKm: LedgerRational.scaled(eligibleCostCentavos, scale: 1000, den: distanceMetres),
            method: "fuel-purchase-interval"
        )
    }

    public static func matchedWindowCost(
        fuelCostCentavos: Int64,
        fuelDistanceMetres: Int64,
        expenses: [LedgerExpenseInput],
        window: LedgerWindowInput
    ) -> LedgerCostResult {
        let money = 0...LedgerBounds.maxTotalCentavos
        let distanceRange = 0...LedgerBounds.maxDistanceMetres
        guard money.contains(fuelCostCentavos), distanceRange.contains(fuelDistanceMetres),
              distanceRange.contains(window.startOdometerMetres), distanceRange.contains(window.endOdometerMetres) else {
            return .unknown("input-out-of-bounds")
        }
        if window.endMillis <= window.startMillis { return .unknown("unmatched-period-window") }
        if window.hasGap { return .unknown("declared-fill-gap") }
        if window.crossesSegment { return .unknown("odometer-segment-boundary") }
        let distance = window.endOdometerMetres - window.startOdometerMetres
        if distance <= 0 { return .unknown("distance-nonpositive") }
        if fuelDistanceMetres != distance { return .unknown("unmatched-period-window") }
        var seen = Set<String>()
        var total = fuelCostCentavos
        for expense in expenses {
            guard money.contains(expense.amountCentavos) else { return .unknown("input-out-of-bounds") }
            if !seen.insert(expense.id).inserted { continue }
            if expense.occurredAtMillis < window.startMillis || expense.occurredAtMillis >= window.endMillis { continue }
            if let odometer = expense.odometerMetres,
               odometer < window.startOdometerMetres || odometer > window.endOdometerMetres {
                continue
            }
            guard let next = LedgerChecked.add(total, expense.amountCentavos) else {
                return .unknown("input-out-of-bounds")
            }
            total = next
        }
        return LedgerCostResult(
            totalCentavos: total,
            distanceMetres: distance,
            centavosPerKm: LedgerRational.scaled(total, scale: 1000, den: distance),
            method: "matched-window"
        )
    }

    /// Weighted totals of known intervals: sums first, one division (never averages of ratios).
    public static func sumIntervals(_ results: [LedgerConsumptionResult]) -> LedgerAggregateTotals {
        var distance: Int64 = 0
        var quantity: Int64 = 0
        var cost: Int64 = 0
        for result in results where result.unknown.isEmpty {
            guard let nextDistance = LedgerChecked.add(distance, result.distanceMetres),
                  let nextQuantity = LedgerChecked.add(quantity, result.eligibleQuantityMilli),
                  let nextCost = LedgerChecked.add(cost, result.eligibleCostCentavos) else {
                return LedgerAggregateTotals(unknown: "input-out-of-bounds")
            }
            distance = nextDistance
            quantity = nextQuantity
            cost = nextCost
        }
        return LedgerAggregateTotals(
            distanceMetres: distance,
            eligibleQuantityMilli: quantity,
            eligibleCostCentavos: cost,
            kmPerUnit: distance > 0 && quantity > 0 ? LedgerRational.ratio(distance, quantity) : "",
            centavosPerKm: distance > 0 ? LedgerRational.scaled(cost, scale: 1000, den: distance) : ""
        )
    }

    public static func compare(base: LedgerConsumptionResult, current: LedgerConsumptionResult) -> LedgerComparisonResult {
        if !base.unknown.isEmpty { return LedgerComparisonResult(unknown: "baseline-incomplete: " + base.unknown) }
        if !current.unknown.isEmpty { return LedgerComparisonResult(unknown: "current-incomplete: " + current.unknown) }
        if base.eligibleCostCentavos == 0 { return LedgerComparisonResult(unknown: "comparison-baseline-zero") }
        if base.distanceMetres <= 0 || base.eligibleQuantityMilli <= 0
            || current.distanceMetres <= 0 || current.eligibleQuantityMilli <= 0
            || base.eligibleCostCentavos < 0 || current.eligibleCostCentavos < 0 {
            return LedgerComparisonResult(unknown: "input-out-of-bounds")
        }
        guard let change = LedgerRational.changePercent(
            baseDistance: base.distanceMetres,
            baseQuantity: base.eligibleQuantityMilli,
            currentDistance: current.distanceMetres,
            currentQuantity: current.eligibleQuantityMilli
        ) else {
            return LedgerComparisonResult(unknown: "input-out-of-bounds")
        }
        return LedgerComparisonResult(changePercent: change)
    }
}

public struct LedgerEnergyComparison: Equatable, Sendable {
    public let changePercent: String
    public let unknown: String

    public init(changePercent: String = "", unknown: String = "") {
        self.changePercent = changePercent
        self.unknown = unknown
    }
}

/// EE-23 comparison and combined fuel-plus-charge cost (`PremiumEnergy.compare`, `combinedPurchasePerKm`).
/// Interval energy itself stays in `PremiumEnergyCalculator.compute`.
public enum LedgerEnergy {
    public static func compare(
        base: PremiumEnergyCalculator.EnergyResult,
        current: PremiumEnergyCalculator.EnergyResult
    ) -> LedgerEnergyComparison {
        if !base.unknown.isEmpty { return LedgerEnergyComparison(unknown: "baseline-incomplete: " + base.unknown) }
        if !current.unknown.isEmpty { return LedgerEnergyComparison(unknown: "current-incomplete: " + current.unknown) }
        if base.purchasedWh <= 0 || current.purchasedWh <= 0 || base.distanceMetres <= 0 || current.distanceMetres <= 0 {
            return LedgerEnergyComparison(unknown: "input-out-of-bounds")
        }
        guard let change = LedgerRational.changePercent(
            baseDistance: base.distanceMetres,
            baseQuantity: base.purchasedWh,
            currentDistance: current.distanceMetres,
            currentQuantity: current.purchasedWh
        ) else {
            return LedgerEnergyComparison(unknown: "input-out-of-bounds")
        }
        return LedgerEnergyComparison(changePercent: change)
    }

    public static func combinedPurchasePerKm(
        fuelCostCentavos: Int64,
        chargeCostCentavos: Int64,
        distanceMetres: Int64
    ) -> LedgerCostResult {
        let money = 0...LedgerBounds.maxTotalCentavos
        guard money.contains(fuelCostCentavos), money.contains(chargeCostCentavos),
              let total = LedgerChecked.add(fuelCostCentavos, chargeCostCentavos) else {
            return .unknown("input-out-of-bounds")
        }
        if distanceMetres < 0 || distanceMetres > LedgerBounds.maxDistanceMetres { return .unknown("input-out-of-bounds") }
        if distanceMetres <= 0 { return .unknown("distance-nonpositive") }
        return LedgerCostResult(
            totalCentavos: total,
            distanceMetres: distanceMetres,
            centavosPerKm: LedgerRational.scaled(total, scale: 1000, den: distanceMetres),
            method: "combined-purchase-interval"
        )
    }
}

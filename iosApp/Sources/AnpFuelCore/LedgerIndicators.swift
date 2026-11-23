import Foundation

/// Monthly ledger indicators over local records (B-BR-PR16/PR17/PR18, BUC-PR05).
///
/// Port of Android `GetMonthlyIndicatorsUseCase` (`MonthlyIndicatorsUseCases.kt`):
/// month bounds follow the chosen time zone (half-open); measured consumption uses
/// the full-to-full engine on in-month fills; a declared gap touching the month
/// forces `declared-fill-gap`; PHEV months share one distance; the comparison runs
/// against the previous month; migrated records with an unreviewed odometer give
/// `odometer-unit-unverified`. iOS also applies its local odometer segment marker
/// (`odometer-segment-boundary`) and applies gaps and segments to the baseline month.
/// Nothing is cached: every call recomputes from the current records.
/// Zero postfix unwrap operators anywhere in this file.

public struct LedgerWindowNote: Equatable, Sendable {
    public let recordCount: Int
    public let anchorFills: Int
    public let hasMissingOdometer: Bool
    public let distanceMetres: Int64
    public let hasMixedCarrier: Bool
    public let hasUnverifiedOdometer: Bool
    public let crossesSegment: Bool
    public let hasDeclaredGap: Bool
}

public struct LedgerMonthlyIndicators: Equatable {
    public let vehicleId: String
    public let vehicleName: String
    public let year: Int
    public let month: Int
    public let timezone: String
    public let purchaseCentavos: Int64
    public let expenseCentavos: Int64
    public let chargeCostCentavos: Int64
    public let chargeEnergyWh: Int64
    public let chargeFeesCentavos: Int64
    public let byCategory: [LedgerExpenseCategory: Int64]
    public let recordCount: Int
    public let measured: LedgerConsumptionResult
    public let cost: LedgerCostResult
    public let combined: LedgerCostResult
    public let comparison: LedgerComparisonResult
    public let energyMeasured: PremiumEnergyCalculator.EnergyResult
    public let energyReference: PremiumEnergyCalculator.EnergyResult
    public let energyComparison: LedgerEnergyComparison
    public let windowNote: LedgerWindowNote
}

public enum LedgerIndicatorError: String, Error, Equatable, Sendable {
    case invalidTimezone = "invalid-timezone"
    case invalidMonth = "invalid-month"
    case inputOutOfBounds = "input-out-of-bounds"
}

public enum LedgerIndicators {
    /// Half-open `[start, end)` epoch milliseconds of a calendar month in `timeZone`.
    public static func monthBounds(year: Int, month: Int, timeZone: TimeZone) -> (start: Int64, end: Int64)? {
        guard (2000...2100).contains(year), (1...12).contains(month) else { return nil }
        var calendar = Calendar(identifier: .gregorian)
        calendar.timeZone = timeZone
        var startComponents = DateComponents()
        startComponents.year = year
        startComponents.month = month
        startComponents.day = 1
        var endComponents = DateComponents()
        endComponents.year = month == 12 ? year + 1 : year
        endComponents.month = month == 12 ? 1 : month + 1
        endComponents.day = 1
        guard let start = calendar.date(from: startComponents), let end = calendar.date(from: endComponents) else {
            return nil
        }
        return (Int64((start.timeIntervalSince1970 * 1000).rounded()), Int64((end.timeIntervalSince1970 * 1000).rounded()))
    }

    public static func monthly(
        vehicle: VehicleRecord,
        records: [LedgerEntryRecord],
        year: Int,
        month: Int,
        timezone: String
    ) throws -> LedgerMonthlyIndicators {
        guard LedgerText.isValidTimezone(timezone), let zone = TimeZone(identifier: timezone) else {
            throw LedgerIndicatorError.invalidTimezone
        }
        guard let bounds = monthBounds(year: year, month: month, timeZone: zone) else {
            throw LedgerIndicatorError.invalidMonth
        }
        let history = ordered(records.filter { $0.vehicleId == vehicle.id && $0.dataset != nil })
        let rows = history.filter { $0.occurredAtMillis >= bounds.start && $0.occurredAtMillis < bounds.end }

        var purchase: Int64 = 0
        var expense: Int64 = 0
        var chargeCost: Int64 = 0
        var chargeEnergy: Int64 = 0
        var chargeFees: Int64 = 0
        var byCategory: [LedgerExpenseCategory: Int64] = [:]
        var fills: [LedgerEntryRecord] = []
        var chargeOdometers: [(Int64, Int64)] = []
        var energyStated = 0
        var chargeSessions = 0
        var socInconsistent = false
        var missingOdometer = false
        var energyOdometerMissing = false

        func add(_ lhs: Int64, _ rhs: Int64) throws -> Int64 {
            guard let sum = LedgerChecked.add(lhs, rhs) else { throw LedgerIndicatorError.inputOutOfBounds }
            return sum
        }

        for row in rows {
            switch row.kind {
            case .refuel:
                purchase = try add(purchase, row.totalCentavos ?? 0)
                fills.append(row)
                if row.odometerMetres == nil { missingOdometer = true }
            case .expense:
                let amount = row.totalCentavos ?? 0
                expense = try add(expense, amount)
                let category = row.category ?? .other
                byCategory[category] = try add(byCategory[category] ?? 0, amount)
            case .charge:
                chargeSessions += 1
                if let energy = row.energyWh {
                    energyStated += 1
                    chargeEnergy = try add(chargeEnergy, energy)
                    if row.odometerMetres == nil {
                        missingOdometer = true
                        energyOdometerMissing = true
                    }
                }
                if let start = row.socStartPercent, let end = row.socEndPercent, end < start {
                    socInconsistent = true
                }
                if let odometer = row.odometerMetres {
                    chargeOdometers.append((row.occurredAtMillis, odometer))
                }
                if let total = row.totalCentavos {
                    chargeCost = try add(chargeCost, total)
                }
                if let fees = row.feesCentavos {
                    chargeFees = try add(chargeFees, fees)
                }
            case .odometer:
                break
            }
        }

        let hasMixedCarrier = !fills.isEmpty && energyStated > 0
        let gapHit = rows.contains { $0.missedFillBefore }
        let segmentHit = crossesSegment(rows)
        let fillOdometers = fills.compactMap { $0.odometerMetres }
        var timeline: [(at: Int64, odometer: Int64?)] = fills.map { (at: $0.occurredAtMillis, odometer: $0.odometerMetres) }
        for item in chargeOdometers {
            timeline.append((at: item.0, odometer: item.1))
        }
        let sortedTimeline = timeline.enumerated().sorted { lhs, rhs in
            lhs.element.at != rhs.element.at ? lhs.element.at < rhs.element.at : lhs.offset < rhs.offset
        }
        let shared: [Int64] = sortedTimeline.compactMap { $0.element.odometer }
        let unverified = hasUnverifiedOdometer(rows)
        let distance: Int64 = unverified ? 0 : windowDistance(shared, crossesSegment: segmentHit)

        let measured: LedgerConsumptionResult
        if unverified {
            measured = .unknown("odometer-unit-unverified")
        } else if fills.isEmpty {
            measured = .unknown("no-records")
        } else if hasMixedCarrier {
            measured = .unknown("phev-shared-distance")
        } else {
            measured = interval(
                fills: fills,
                distance: distance,
                hasGap: gapHit,
                crossesSegment: segmentHit,
                odometerMissing: missingOdometer || fillOdometers.count < fills.count
            )
        }

        let cost: LedgerCostResult
        if measured.unknown.isEmpty {
            cost = LedgerConsumption.fuelPurchasePerKm(
                eligibleCostCentavos: measured.eligibleCostCentavos,
                distanceMetres: measured.distanceMetres
            )
        } else {
            cost = .unknown(measured.unknown)
        }

        let combined: LedgerCostResult
        if unverified {
            combined = .unknown("odometer-unit-unverified")
        } else if fills.isEmpty && chargeSessions == 0 {
            combined = .unknown("no-records")
        } else if missingOdometer {
            combined = .unknown("odometer-missing")
        } else if gapHit {
            combined = .unknown("declared-fill-gap")
        } else if segmentHit {
            combined = .unknown("odometer-segment-boundary")
        } else {
            combined = LedgerEnergy.combinedPurchasePerKm(
                fuelCostCentavos: purchase,
                chargeCostCentavos: chargeCost,
                distanceMetres: distance
            )
        }

        let energyMeasured: PremiumEnergyCalculator.EnergyResult
        if unverified {
            energyMeasured = PremiumEnergyCalculator.EnergyResult(unknown: "odometer-unit-unverified")
        } else if chargeSessions == 0 && fills.isEmpty {
            energyMeasured = PremiumEnergyCalculator.EnergyResult(unknown: "no-records")
        } else {
            energyMeasured = PremiumEnergyCalculator.compute(
                PremiumEnergyCalculator.IntervalInput(
                    purchasedWh: chargeEnergy,
                    costCentavos: chargeCost,
                    distanceMetres: windowDistance(shared, crossesSegment: segmentHit),
                    hasEnergy: energyStated > 0,
                    complete: energyStated == chargeSessions,
                    hasGap: gapHit,
                    crossesSegment: segmentHit,
                    odometerMissing: energyOdometerMissing,
                    socInconsistent: socInconsistent,
                    mixedCarrier: hasMixedCarrier
                )
            )
        }
        let energyReference = reference(history)
        let energyComparison = LedgerEnergy.compare(base: energyReference, current: energyMeasured)
        let previous = previousMonth(history: history, year: year, month: month, timeZone: zone)
        let comparison = LedgerConsumption.compare(base: previous, current: measured)

        return LedgerMonthlyIndicators(
            vehicleId: vehicle.id,
            vehicleName: vehicle.name,
            year: year,
            month: month,
            timezone: timezone,
            purchaseCentavos: purchase,
            expenseCentavos: expense,
            chargeCostCentavos: chargeCost,
            chargeEnergyWh: chargeEnergy,
            chargeFeesCentavos: chargeFees,
            byCategory: byCategory,
            recordCount: rows.count,
            measured: measured,
            cost: cost,
            combined: combined,
            comparison: comparison,
            energyMeasured: energyMeasured,
            energyReference: energyReference,
            energyComparison: energyComparison,
            windowNote: LedgerWindowNote(
                recordCount: rows.count,
                anchorFills: fills.count,
                hasMissingOdometer: missingOdometer,
                distanceMetres: distance,
                hasMixedCarrier: hasMixedCarrier,
                hasUnverifiedOdometer: unverified,
                crossesSegment: segmentHit,
                hasDeclaredGap: gapHit
            )
        )
    }

    /// Chronological order with a stable identifier tie-break.
    static func ordered(_ records: [LedgerEntryRecord]) -> [LedgerEntryRecord] {
        return records.sorted { lhs, rhs in
            lhs.occurredAtMillis != rhs.occurredAtMillis ? lhs.occurredAtMillis < rhs.occurredAtMillis : lhs.id < rhs.id
        }
    }

    /// A reset or replacement on any record after the window's first record splits the distance.
    static func crossesSegment(_ rows: [LedgerEntryRecord]) -> Bool {
        return rows.dropFirst().contains { $0.opensOdometerSegment }
    }

    /// Migrated readings keep an unverified odometer until the owner reviews them (expenses excluded, as on Android).
    static func hasUnverifiedOdometer(_ rows: [LedgerEntryRecord]) -> Bool {
        return rows.contains { $0.kind != .expense && $0.odometerMetres != nil && $0.reviewRequired }
    }

    /// Last minus first reading; a declared reset or replacement makes the distance unknown (zero
    /// with the segment flag) instead of a negative or invented value.
    static func windowDistance(_ odometers: [Int64], crossesSegment: Bool) -> Int64 {
        guard !crossesSegment, odometers.count >= 2, let first = odometers.first, let last = odometers.last else { return 0 }
        let difference = last.subtractingReportingOverflow(first)
        return difference.overflow ? 0 : difference.partialValue
    }

    static func fillInput(_ record: LedgerEntryRecord) -> LedgerFillInput? {
        guard let fuel = record.fuel else { return nil }
        return LedgerFillInput(
            quantityMilli: record.quantityMilli,
            totalCentavos: record.totalCentavos ?? 0,
            full: record.fill == .full,
            fuel: fuel,
            unit: fuel.unit
        )
    }

    private static func interval(
        fills: [LedgerEntryRecord],
        distance: Int64,
        hasGap: Bool,
        crossesSegment: Bool,
        odometerMissing: Bool
    ) -> LedgerConsumptionResult {
        let inputs = fills.compactMap(fillInput)
        guard inputs.count == fills.count, let first = inputs.first else {
            return .unknown("full-anchors-required")
        }
        return LedgerConsumption.compute(
            LedgerIntervalInput(
                startingFillExcluded: first,
                fills: Array(inputs.dropFirst()),
                distanceMetres: distance,
                hasGap: hasGap,
                crossesSegment: crossesSegment,
                odometerMissing: odometerMissing
            )
        )
    }

    private static func previousMonth(
        history: [LedgerEntryRecord],
        year: Int,
        month: Int,
        timeZone: TimeZone
    ) -> LedgerConsumptionResult {
        let previousYear = month == 1 ? year - 1 : year
        let previous = month == 1 ? 12 : month - 1
        guard previousYear >= 2000, let bounds = monthBounds(year: previousYear, month: previous, timeZone: timeZone) else {
            return .unknown("no-records")
        }
        let rows = history.filter { $0.occurredAtMillis >= bounds.start && $0.occurredAtMillis < bounds.end }
        let fills = rows.filter { $0.kind == .refuel }
        if fills.isEmpty { return .unknown("no-records") }
        if hasUnverifiedOdometer(rows) { return .unknown("odometer-unit-unverified") }
        let odometers = fills.compactMap { $0.odometerMetres }
        let segment = crossesSegment(rows)
        return interval(
            fills: fills,
            distance: windowDistance(odometers, crossesSegment: segment),
            hasGap: rows.contains { $0.missedFillBefore },
            crossesSegment: segment,
            odometerMissing: odometers.count < fills.count
        )
    }

    /// Historical Wh/km reference over the vehicle's whole held history; never a guarantee.
    private static func reference(_ history: [LedgerEntryRecord]) -> PremiumEnergyCalculator.EnergyResult {
        if history.isEmpty { return PremiumEnergyCalculator.EnergyResult(unknown: "no-records") }
        if hasUnverifiedOdometer(history) { return PremiumEnergyCalculator.EnergyResult(unknown: "odometer-unit-unverified") }
        var purchased: Int64 = 0
        var cost: Int64 = 0
        var stated = 0
        var sessions = 0
        var socInconsistent = false
        var odometerMissing = false
        var fuelEver = false
        var odometers: [Int64] = []
        for row in history {
            switch row.kind {
            case .refuel:
                fuelEver = true
            case .charge:
                sessions += 1
                if let energy = row.energyWh {
                    stated += 1
                    guard let next = LedgerChecked.add(purchased, energy) else {
                        return PremiumEnergyCalculator.EnergyResult(unknown: "input-out-of-bounds")
                    }
                    purchased = next
                    if row.odometerMetres == nil { odometerMissing = true }
                }
                if let total = row.totalCentavos {
                    guard let next = LedgerChecked.add(cost, total) else {
                        return PremiumEnergyCalculator.EnergyResult(unknown: "input-out-of-bounds")
                    }
                    cost = next
                }
                if let start = row.socStartPercent, let end = row.socEndPercent, end < start {
                    socInconsistent = true
                }
                if let odometer = row.odometerMetres {
                    odometers.append(odometer)
                }
            case .expense, .odometer:
                break
            }
        }
        let segment = crossesSegment(history.filter { $0.kind == .charge })
        return PremiumEnergyCalculator.compute(
            PremiumEnergyCalculator.IntervalInput(
                purchasedWh: purchased,
                costCentavos: cost,
                distanceMetres: windowDistance(odometers, crossesSegment: segment),
                hasEnergy: stated > 0,
                complete: stated == sessions,
                hasGap: history.contains { $0.missedFillBefore },
                crossesSegment: segment,
                odometerMissing: odometerMissing,
                socInconsistent: socInconsistent,
                mixedCarrier: fuelEver && stated > 0
            )
        )
    }
}

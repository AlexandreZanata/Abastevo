import Foundation
import XCTest
@testable import AnpFuelCore

/// Exact consumption, cost, energy, monthly indicator and vehicle admission rules,
/// replaying the shared Go/Kotlin execution corpora in `LedgerFixtures`.
final class LedgerMathTests: XCTestCase {
    private static func cells(_ line: String) -> [String] {
        return line.split(separator: ",", omittingEmptySubsequences: false).map(String.init)
    }

    private static func number(_ text: String) throws -> Int64 {
        if text.isEmpty { return 0 }
        return try XCTUnwrap(Int64(text), text)
    }

    private static func items(_ text: String) -> [String] {
        if text.isEmpty { return [] }
        var result: [String] = []
        for item in text.split(separator: ";").map(String.init) {
            let parts = item.split(separator: "*", maxSplits: 1).map(String.init)
            let count = parts.count == 2 ? (Int(parts[0]) ?? 1) : 1
            let body = parts.count == 2 ? parts[1] : parts[0]
            for index in 0..<count {
                result.append(body.replacingOccurrences(of: "#", with: String(index)))
            }
        }
        return result
    }

    private static func fill(_ text: String) throws -> LedgerFillInput {
        let parts = text.split(separator: ":", omittingEmptySubsequences: false).map(String.init)
        XCTAssertEqual(parts.count, 5, text)
        return LedgerFillInput(
            quantityMilli: try number(parts[0]),
            totalCentavos: try number(parts[1]),
            full: parts[2] == "1",
            fuel: try XCTUnwrap(LedgerFuel(rawValue: parts[3]), text),
            unit: try XCTUnwrap(LedgerUnit(rawValue: parts[4]), text)
        )
    }

    func testFinancialExecutionVectorsMatchGoAndKotlin() throws {
        let lines = LedgerFixtures.financialExecution.split(separator: "\n").map(String.init)
        let header = Self.cells(try XCTUnwrap(lines.first))
        XCTAssertEqual(header.joined(separator: ","), "id,operation,anchor,fills,distance,fuel_cost,expenses,window_start_odo,window_end_odo,window_start_ms,window_end_ms,has_gap,crosses_segment,odometer_missing,expected_quantity,expected_cost,expected_distance,expected_efficiency,expected_cost_per_km,expected_change,expected_unknown")
        XCTAssertEqual(lines.count, 37)
        var ids = Set<String>()
        for line in lines.dropFirst() {
            let row = Self.cells(line)
            XCTAssertEqual(row.count, header.count, line)
            var values: [String: String] = [:]
            for (index, name) in header.enumerated() where index < row.count {
                values[name] = row[index]
            }
            func value(_ name: String) -> String { values[name] ?? "" }
            func number(_ name: String) throws -> Int64 { try Self.number(value(name)) }
            let id = value("id")
            XCTAssertTrue(ids.insert(id).inserted, id)
            var change = ""
            let got: LedgerConsumptionResult
            switch value("operation") {
            case "INTERVAL":
                got = LedgerConsumption.compute(
                    LedgerIntervalInput(
                        startingFillExcluded: try Self.fill(value("anchor")),
                        fills: try Self.items(value("fills")).map { try Self.fill($0) },
                        distanceMetres: try number("distance"),
                        hasGap: value("has_gap") == "1",
                        crossesSegment: value("crosses_segment") == "1",
                        odometerMissing: value("odometer_missing") == "1"
                    )
                )
            case "SUM", "COMPARE":
                var results: [LedgerConsumptionResult] = []
                for item in Self.items(value("fills")) {
                    let parts = item.split(separator: ":", omittingEmptySubsequences: false).map(String.init)
                    XCTAssertEqual(parts.count, 4, item)
                    results.append(
                        LedgerConsumptionResult(
                            eligibleQuantityMilli: try Self.number(parts[1]),
                            eligibleCostCentavos: try Self.number(parts[2]),
                            distanceMetres: try Self.number(parts[0]),
                            unknown: parts[3].replacingOccurrences(of: "_", with: "")
                        )
                    )
                }
                if value("operation") == "SUM" {
                    let totals = LedgerConsumption.sumIntervals(results)
                    got = LedgerConsumptionResult(
                        eligibleQuantityMilli: totals.eligibleQuantityMilli,
                        eligibleCostCentavos: totals.eligibleCostCentavos,
                        distanceMetres: totals.distanceMetres,
                        kmPerUnit: totals.kmPerUnit,
                        centavosPerKm: totals.centavosPerKm,
                        unknown: totals.unknown
                    )
                } else {
                    XCTAssertEqual(results.count, 2, id)
                    let comparison = LedgerConsumption.compare(base: results[0], current: results[1])
                    change = comparison.changePercent
                    got = LedgerConsumptionResult(unknown: comparison.unknown)
                }
            case "MATCHED", "COST":
                let cost: LedgerCostResult
                if value("operation") == "COST" {
                    cost = LedgerConsumption.fuelPurchasePerKm(
                        eligibleCostCentavos: try number("fuel_cost"),
                        distanceMetres: try number("distance")
                    )
                } else {
                    var expenses: [LedgerExpenseInput] = []
                    for item in Self.items(value("expenses")) {
                        let parts = item.split(separator: ":", omittingEmptySubsequences: false).map(String.init)
                        XCTAssertEqual(parts.count, 4, item)
                        let odometer: Int64? = parts[3] == "_" ? nil : Int64(parts[3])
                        expenses.append(
                            LedgerExpenseInput(
                                id: parts[0],
                                amountCentavos: try Self.number(parts[1]),
                                occurredAtMillis: try Self.number(parts[2]),
                                odometerMetres: odometer
                            )
                        )
                    }
                    cost = LedgerConsumption.matchedWindowCost(
                        fuelCostCentavos: try number("fuel_cost"),
                        fuelDistanceMetres: try number("distance"),
                        expenses: expenses,
                        window: LedgerWindowInput(
                            startOdometerMetres: try number("window_start_odo"),
                            endOdometerMetres: try number("window_end_odo"),
                            startMillis: try number("window_start_ms"),
                            endMillis: try number("window_end_ms"),
                            hasGap: value("has_gap") == "1",
                            crossesSegment: value("crosses_segment") == "1"
                        )
                    )
                }
                got = LedgerConsumptionResult(
                    eligibleCostCentavos: cost.totalCentavos,
                    distanceMetres: cost.distanceMetres,
                    centavosPerKm: cost.centavosPerKm,
                    unknown: cost.unknown
                )
            default:
                XCTFail("Unknown fixture operation \(value("operation"))")
                continue
            }
            let expected = LedgerConsumptionResult(
                eligibleQuantityMilli: try number("expected_quantity"),
                eligibleCostCentavos: try number("expected_cost"),
                distanceMetres: try number("expected_distance"),
                kmPerUnit: value("expected_efficiency"),
                centavosPerKm: value("expected_cost_per_km"),
                unknown: value("expected_unknown")
            )
            XCTAssertEqual(got, expected, id)
            XCTAssertEqual(change, value("expected_change"), id)
        }
    }

    func testEnergyExecutionVectorsIncludingCombinedCost() throws {
        let lines = LedgerFixtures.energyExecution.split(separator: "\n").map(String.init)
        let header = Self.cells(try XCTUnwrap(lines.first))
        XCTAssertEqual(lines.count - 1, 11)
        var combinedRows = 0
        for line in lines.dropFirst() {
            let row = Self.cells(line)
            XCTAssertEqual(row.count, header.count, line)
            var values: [String: String] = [:]
            for (index, name) in header.enumerated() where index < row.count {
                values[name] = row[index]
            }
            func value(_ name: String) -> String { values[name] ?? "" }
            let id = value("id")
            if !value("expected_combined_centavos_per_km").isEmpty || !value("expected_combined_unknown").isEmpty {
                combinedRows += 1
                let got = LedgerEnergy.combinedPurchasePerKm(
                    fuelCostCentavos: try Self.number(value("fuel_cost_centavos")),
                    chargeCostCentavos: try Self.number(value("charge_cost_centavos")),
                    distanceMetres: try Self.number(value("distance_metres"))
                )
                XCTAssertEqual(got.centavosPerKm, value("expected_combined_centavos_per_km"), id)
                XCTAssertEqual(got.unknown, value("expected_combined_unknown"), id)
                XCTAssertEqual(got.method, got.unknown.isEmpty ? "combined-purchase-interval" : "", id)
                continue
            }
            let got = PremiumEnergyCalculator.compute(
                PremiumEnergyCalculator.IntervalInput(
                    purchasedWh: try Self.number(value("purchased_wh")),
                    costCentavos: try Self.number(value("cost_centavos")),
                    distanceMetres: try Self.number(value("distance_metres")),
                    hasEnergy: value("has_energy") == "true",
                    complete: value("complete") == "true",
                    hasGap: value("has_gap") == "true",
                    crossesSegment: value("crosses_segment") == "true",
                    odometerMissing: value("odometer_missing") == "true",
                    socInconsistent: value("soc_inconsistent") == "true",
                    mixedCarrier: value("mixed_carrier") == "true"
                )
            )
            XCTAssertEqual(got.whPerKm, value("expected_wh_per_km"), id)
            XCTAssertEqual(got.centavosPerKm, value("expected_centavos_per_km"), id)
            XCTAssertEqual(got.unknown, value("expected_unknown"), id)
        }
        XCTAssertEqual(combinedRows, 2)
    }

    func testEnergyComparisonIsWeightedAndRefusesUnknownBaselines() {
        let base = PremiumEnergyCalculator.EnergyResult(purchasedWh: 40000, costCentavos: 4000, distanceMetres: 150000)
        let better = PremiumEnergyCalculator.EnergyResult(purchasedWh: 20000, costCentavos: 2000, distanceMetres: 150000)
        let worse = PremiumEnergyCalculator.EnergyResult(purchasedWh: 80000, costCentavos: 8000, distanceMetres: 150000)
        XCTAssertEqual(LedgerEnergy.compare(base: base, current: better).changePercent, "100")
        XCTAssertEqual(LedgerEnergy.compare(base: base, current: worse).changePercent, "-50")
        let unknown = PremiumEnergyCalculator.EnergyResult(unknown: "energy-unmeasured")
        XCTAssertEqual(LedgerEnergy.compare(base: unknown, current: better).unknown, "baseline-incomplete: energy-unmeasured")
        XCTAssertEqual(LedgerEnergy.compare(base: better, current: unknown).unknown, "current-incomplete: energy-unmeasured")
    }

    func testWideArithmeticBeyondInt64() {
        let product = LedgerWide.product(10_000_000_000_000_000, 1000)
        XCTAssertEqual(product.decimalString, "10000000000000000000")
        XCTAssertEqual(LedgerRational.scaled(10_000_000_000_000_000, scale: 1000, den: 1), "10000000000000000000")
        XCTAssertEqual(LedgerRational.scaled(24400, scale: 1000, den: 400_000), "61")
        XCTAssertEqual(LedgerRational.scaled(0, scale: 1000, den: 7), "0")
        XCTAssertEqual(LedgerRational.scaled(5, scale: 1, den: 0), "")
        XCTAssertEqual(LedgerRational.ratio(1, 10000), "1/10000")
        let big = LedgerWide.product(UInt64.max, UInt64.max)
        let division = big.quotientAndRemainder(dividingBy: LedgerWide(UInt64.max))
        XCTAssertEqual(division.quotient, LedgerWide(UInt64.max))
        XCTAssertTrue(division.remainder.isZero)
        XCTAssertEqual(LedgerWide.gcd(LedgerWide.product(1_000_000_000_000, 1_000_000_000), LedgerWide(4_000)), LedgerWide(4_000))
        XCTAssertEqual(
            LedgerRational.changePercent(baseDistance: 1_000_000_000_000, baseQuantity: 1_000_000_000, currentDistance: 999_999_999_999, currentQuantity: 1_000_000_000),
            "-1/10000000000"
        )
        XCTAssertNil(LedgerRational.changePercent(baseDistance: 1, baseQuantity: Int64.max, currentDistance: Int64.max, currentQuantity: 1))
    }

    func testRationalDisplayRoundsHalfUpOnlyAtTheBoundary() {
        XCTAssertEqual(LedgerRational.decimal("800/3", fractionDigits: 2), Decimal(string: "266.67"))
        XCTAssertEqual(LedgerRational.decimal("115/2", fractionDigits: 0), Decimal(58))
        XCTAssertEqual(LedgerRational.decimal("-100/3", fractionDigits: 1), Decimal(string: "-33.3"))
        XCTAssertNil(LedgerRational.decimal("1/0", fractionDigits: 2))
        XCTAssertNil(LedgerRational.decimal("", fractionDigits: 2))
        XCTAssertEqual(LedgerRational.displayText("61", fractionDigits: 3, divideBy: 100), "0,61")
    }

    // MARK: Monthly indicators (Android `GetMonthlyIndicatorsUseCase`)

    private let zone = "America/Sao_Paulo"
    private let vehicle = VehicleRecord(
        id: "10000000-0000-4000-8000-000000000001",
        name: "Carro",
        propulsion: .flex,
        primaryFuel: .ethanol,
        tankCapacityMilliLiters: 50_000
    )
    private let phev = VehicleRecord(
        id: "10000000-0000-4000-8000-000000000002",
        name: "PHEV",
        propulsion: .plugInHybrid,
        primaryFuel: .gasolineRegular,
        tankCapacityMilliLiters: 40_000,
        batteryCapacityWh: 18_000
    )
    /// 2026-03-10 12:00 in America/Sao_Paulo.
    private let march10: Int64 = 1_773_154_800_000
    private let day: Int64 = 86_400_000

    private func refuel(
        _ index: Int,
        at millis: Int64,
        quantity: Int64,
        total: Int64,
        odometer: Int64?,
        fill: LedgerFill = .full,
        fuel: LedgerFuel = .ethanol,
        vehicleId: String? = nil,
        segment: Bool = false,
        gap: Bool = false,
        review: Bool = false
    ) -> LedgerEntryRecord {
        return LedgerEntryRecord(
            id: String(format: "20000000-0000-4000-8000-%012ld", index),
            vehicleId: vehicleId ?? vehicle.id,
            kind: .refuel,
            occurredAtMillis: millis,
            timezone: zone,
            fuel: fuel,
            quantityMilli: quantity,
            totalCentavos: total,
            fill: fill,
            odometerMetres: odometer,
            opensOdometerSegment: segment,
            missedFillBefore: gap,
            reviewRequired: review
        )
    }

    func testMonthBoundsAreHalfOpenInTheChosenZone() throws {
        let zoneValue = try XCTUnwrap(TimeZone(identifier: zone))
        let bounds = try XCTUnwrap(LedgerIndicators.monthBounds(year: 2026, month: 3, timeZone: zoneValue))
        XCTAssertEqual(bounds.start, 1_772_334_000_000)
        XCTAssertEqual(bounds.end, 1_775_012_400_000)
        XCTAssertNil(LedgerIndicators.monthBounds(year: 2026, month: 13, timeZone: zoneValue))
        XCTAssertThrowsError(try LedgerIndicators.monthly(vehicle: vehicle, records: [], year: 2026, month: 3, timezone: "Mars/Base"))
    }

    func testFullToFullMonthComputesExactConsumptionAndCost() throws {
        let records = [
            refuel(1, at: march10, quantity: 50_000, total: 30_000, odometer: 10_000_000),
            refuel(2, at: march10 + day, quantity: 20_000, total: 12_000, odometer: 10_200_000, fill: .partial),
            refuel(3, at: march10 + 2 * day, quantity: 20_000, total: 12_400, odometer: 10_400_000),
            LedgerEntryRecord(
                id: "30000000-0000-4000-8000-000000000001", vehicleId: vehicle.id, kind: .expense,
                occurredAtMillis: march10 + 3 * day, timezone: zone, totalCentavos: 5_000, category: .parking
            ),
        ]
        let result = try LedgerIndicators.monthly(vehicle: vehicle, records: records, year: 2026, month: 3, timezone: zone)
        XCTAssertEqual(result.purchaseCentavos, 54_400)
        XCTAssertEqual(result.expenseCentavos, 5_000)
        XCTAssertEqual(result.byCategory[.parking], 5_000)
        XCTAssertEqual(result.recordCount, 4)
        XCTAssertEqual(result.measured.eligibleQuantityMilli, 40_000)
        XCTAssertEqual(result.measured.eligibleCostCentavos, 24_400)
        XCTAssertEqual(result.measured.kmPerUnit, "10")
        XCTAssertEqual(result.measured.centavosPerKm, "61")
        XCTAssertEqual(result.cost.method, "fuel-purchase-interval")
        XCTAssertEqual(result.combined.method, "combined-purchase-interval")
        XCTAssertEqual(result.combined.centavosPerKm, "136")
        XCTAssertEqual(result.comparison.unknown, "baseline-incomplete: no-records")
        XCTAssertEqual(result.windowNote.distanceMetres, 400_000)
    }

    func testIncompleteMonthsStayUnknownWithTheirReason() throws {
        let partialOnly = [
            refuel(1, at: march10, quantity: 50_000, total: 30_000, odometer: 10_000_000, fill: .partial),
            refuel(2, at: march10 + day, quantity: 20_000, total: 12_000, odometer: 10_400_000, fill: .partial),
        ]
        XCTAssertEqual(try LedgerIndicators.monthly(vehicle: vehicle, records: partialOnly, year: 2026, month: 3, timezone: zone).measured.unknown, "full-anchors-required")

        let missing = [
            refuel(1, at: march10, quantity: 50_000, total: 30_000, odometer: 10_000_000),
            refuel(2, at: march10 + day, quantity: 20_000, total: 12_000, odometer: nil),
        ]
        let missingResult = try LedgerIndicators.monthly(vehicle: vehicle, records: missing, year: 2026, month: 3, timezone: zone)
        XCTAssertEqual(missingResult.measured.unknown, "odometer-missing")
        XCTAssertEqual(missingResult.combined.unknown, "odometer-missing")

        let gap = [
            refuel(1, at: march10, quantity: 50_000, total: 30_000, odometer: 10_000_000),
            refuel(2, at: march10 + day, quantity: 20_000, total: 12_000, odometer: 10_400_000, gap: true),
        ]
        XCTAssertEqual(try LedgerIndicators.monthly(vehicle: vehicle, records: gap, year: 2026, month: 3, timezone: zone).measured.unknown, "declared-fill-gap")

        let reset = [
            refuel(1, at: march10, quantity: 50_000, total: 30_000, odometer: 10_000_000),
            refuel(2, at: march10 + day, quantity: 20_000, total: 12_000, odometer: 400_000, segment: true),
        ]
        let resetResult = try LedgerIndicators.monthly(vehicle: vehicle, records: reset, year: 2026, month: 3, timezone: zone)
        XCTAssertEqual(resetResult.measured.unknown, "odometer-segment-boundary")
        XCTAssertEqual(resetResult.combined.unknown, "odometer-segment-boundary")

        let legacy = [
            refuel(1, at: march10, quantity: 50_000, total: 30_000, odometer: 10_000_000, review: true),
            refuel(2, at: march10 + day, quantity: 20_000, total: 12_000, odometer: 10_400_000),
        ]
        let legacyResult = try LedgerIndicators.monthly(vehicle: vehicle, records: legacy, year: 2026, month: 3, timezone: zone)
        XCTAssertEqual(legacyResult.measured.unknown, "odometer-unit-unverified")
        XCTAssertEqual(legacyResult.purchaseCentavos, 42_000, "exact totals stay available")

        let mixed = [
            refuel(1, at: march10, quantity: 50_000, total: 30_000, odometer: 10_000_000),
            refuel(2, at: march10 + day, quantity: 20_000, total: 12_000, odometer: 10_400_000, fuel: .gasolineRegular),
        ]
        XCTAssertEqual(try LedgerIndicators.monthly(vehicle: vehicle, records: mixed, year: 2026, month: 3, timezone: zone).measured.unknown, "unsupported-mixed-method")

        let empty = try LedgerIndicators.monthly(vehicle: vehicle, records: [], year: 2026, month: 3, timezone: zone)
        XCTAssertEqual(empty.measured.unknown, "no-records")
        XCTAssertEqual(empty.combined.unknown, "no-records")
        XCTAssertEqual(empty.energyMeasured.unknown, "no-records")
    }

    func testMonthComparisonUsesThePreviousMonthAsBaseline() throws {
        let february = march10 - 28 * day
        let records = [
            refuel(1, at: february, quantity: 1, total: 1, odometer: 1_000_000),
            refuel(2, at: february + day, quantity: 10_000, total: 6_000, odometer: 1_100_000),
            refuel(3, at: march10, quantity: 1, total: 1, odometer: 2_000_000),
            refuel(4, at: march10 + day, quantity: 60_000, total: 36_000, odometer: 2_900_000),
        ]
        let result = try LedgerIndicators.monthly(vehicle: vehicle, records: records, year: 2026, month: 3, timezone: zone)
        XCTAssertEqual(result.measured.kmPerUnit, "15")
        XCTAssertEqual(result.comparison.changePercent, "50")
        XCTAssertEqual(result.comparison.unknown, "")
    }

    func testPluginHybridSharesDistanceAndRefusesPerCarrierRatios() throws {
        let records = [
            refuel(1, at: march10, quantity: 30_000, total: 18_000, odometer: 5_000_000, fuel: .gasolineRegular, vehicleId: phev.id),
            LedgerEntryRecord(
                id: "40000000-0000-4000-8000-000000000001", vehicleId: phev.id, kind: .charge,
                occurredAtMillis: march10 + day, timezone: zone, totalCentavos: 1_275, energyWh: 15_000,
                odometerMetres: 5_100_000
            ),
        ]
        let result = try LedgerIndicators.monthly(vehicle: phev, records: records, year: 2026, month: 3, timezone: zone)
        XCTAssertEqual(result.measured.unknown, "phev-shared-distance")
        XCTAssertEqual(result.energyMeasured.unknown, "phev-shared-distance")
        XCTAssertEqual(result.combined.centavosPerKm, "771/4")
        XCTAssertEqual(result.chargeEnergyWh, 15_000)
    }

    // MARK: Vehicle admission (`vehicle-registration-v2.json`)

    func testVehicleRegistrationVectors() throws {
        XCTAssertEqual(LedgerFixtures.vehicleRegistration.count, 23)
        for vector in LedgerFixtures.vehicleRegistration {
            let scope = try XCTUnwrap(VehicleAdmissionScope(rawValue: vector.scope), vector.id)
            let authority = try XCTUnwrap(VehicleAdmissionAuthority(rawValue: vector.authority), vector.id)
            let outcome = VehicleAdmissionRule.decide(
                scope: scope,
                authority: authority,
                registeredBefore: vector.registeredBefore,
                newDistinctIds: vector.newDistinctIds
            )
            XCTAssertEqual(outcome?.rawValue, vector.outcome, vector.id)
            let after = outcome == .accept ? vector.registeredBefore + vector.newDistinctIds : vector.registeredBefore
            if scope == .local {
                XCTAssertEqual(after, vector.registeredAfter, vector.id)
            }
        }
        XCTAssertNil(VehicleAdmissionRule.decide(scope: .local, authority: .absent, registeredBefore: -1, newDistinctIds: 1))
    }

    func testCapabilityLeaseAndServerProofFreshness() {
        let account = "a0000000-0000-4000-8000-000000000001"
        let origin = "https://abastevo.com.br"
        let verified: Int64 = 1_000_000_000_000
        let lease = VehicleCapabilityLease(accountId: account, origin: origin, verifiedAtMillis: verified, paidThroughMillis: verified + 10 * day)
        XCTAssertEqual(VehicleAdmissionRule.resolveLocalAuthority(lease: nil, accountId: account, origin: origin, nowMillis: verified), .absent)
        XCTAssertEqual(VehicleAdmissionRule.resolveLocalAuthority(lease: lease, accountId: account, origin: origin, nowMillis: verified + 1000), .localValidLease)
        XCTAssertEqual(VehicleAdmissionRule.resolveLocalAuthority(lease: lease, accountId: account, origin: origin, nowMillis: verified + day), .expired)
        XCTAssertEqual(VehicleAdmissionRule.resolveLocalAuthority(lease: lease, accountId: account, origin: origin, nowMillis: verified - 1), .clockRollback)
        XCTAssertEqual(VehicleAdmissionRule.resolveLocalAuthority(lease: lease, accountId: "other", origin: origin, nowMillis: verified + 1000), .absent)
        XCTAssertEqual(VehicleAdmissionRule.resolveLocalAuthority(lease: lease, accountId: account, origin: "https://other.example", nowMillis: verified + 1000), .absent)
        let revoked = VehicleCapabilityLease(accountId: account, origin: origin, verifiedAtMillis: verified, paidThroughMillis: verified + day, revoked: true)
        XCTAssertEqual(VehicleAdmissionRule.resolveLocalAuthority(lease: revoked, accountId: account, origin: origin, nowMillis: verified + 1), .expired)
        let shortPaid = VehicleCapabilityLease(accountId: account, origin: origin, verifiedAtMillis: verified, paidThroughMillis: verified + 1000)
        XCTAssertEqual(VehicleAdmissionRule.resolveLocalAuthority(lease: shortPaid, accountId: account, origin: origin, nowMillis: verified + 1000), .expired)
        let malformed = VehicleCapabilityLease(accountId: account, origin: origin, verifiedAtMillis: verified, paidThroughMillis: verified)
        XCTAssertEqual(VehicleAdmissionRule.resolveLocalAuthority(lease: malformed, accountId: account, origin: origin, nowMillis: verified), .unknownPolicy)
        let otherPolicy = VehicleCapabilityLease(version: 1, accountId: account, origin: origin, verifiedAtMillis: verified, paidThroughMillis: verified + day)
        XCTAssertEqual(VehicleAdmissionRule.resolveLocalAuthority(lease: otherPolicy, accountId: account, origin: origin, nowMillis: verified), .unknownPolicy)

        XCTAssertEqual(VehicleAdmissionRule.resolveServerAuthority(verifiedAtMillis: verified, nowMillis: verified + 900_000), .serverVerified)
        XCTAssertEqual(VehicleAdmissionRule.resolveServerAuthority(verifiedAtMillis: verified, nowMillis: verified + 900_001), .proofStale)
        XCTAssertEqual(VehicleAdmissionRule.resolveServerAuthority(verifiedAtMillis: verified + 1, nowMillis: verified), .proofStale)
        XCTAssertEqual(VehicleAdmissionRule.resolveServerAuthority(verifiedAtMillis: nil, nowMillis: verified), .proofStale)
    }

    func testAdmissionUsesOnlyTheServerStatusOfTheSameOwnerAndOrigin() {
        let account = "a0000000-0000-4000-8000-000000000001"
        let origin = "https://abastevo.com.br"
        let now: Int64 = 1_000_000_000_000
        let premium = PremiumCapabilityObservation(
            accountId: account, origin: origin, observedAtMillis: now - 1000, active: true,
            capabilities: ["cloud.sync", "professional.write", "vehicles.multiple"], validUntilMillis: now + 30 * day
        )
        XCTAssertEqual(VehicleAdmission.outcomeForNewVehicle(registered: 0, accountId: nil, origin: origin, observation: nil, lease: nil, nowMillis: now), .accept)
        XCTAssertEqual(VehicleAdmission.outcomeForNewVehicle(registered: 1, accountId: nil, origin: origin, observation: premium, lease: nil, nowMillis: now), .premiumRequired)
        XCTAssertEqual(VehicleAdmission.outcomeForNewVehicle(registered: 1, accountId: account, origin: origin, observation: premium, lease: nil, nowMillis: now), .accept)
        XCTAssertEqual(VehicleAdmission.outcomeForNewVehicle(registered: 3, accountId: account, origin: origin, observation: premium, lease: nil, nowMillis: now), .capacityExceeded)
        XCTAssertEqual(VehicleAdmission.outcomeForNewVehicle(registered: 1, accountId: "other", origin: origin, observation: premium, lease: nil, nowMillis: now), .premiumRequired)
        let lapsed = PremiumCapabilityObservation(
            accountId: account, origin: origin, observedAtMillis: now - 1000, active: false,
            capabilities: ["vehicles.multiple"], validUntilMillis: now - day
        )
        XCTAssertEqual(VehicleAdmission.outcomeForNewVehicle(registered: 1, accountId: account, origin: origin, observation: lapsed, lease: lapsed.vehicleLease(), nowMillis: now), .premiumRequired)
        XCTAssertEqual(lapsed.vehicleLease()?.revoked, true)
        let stale = PremiumCapabilityObservation(
            accountId: account, origin: origin, observedAtMillis: now - 2 * 3_600_000, active: true,
            capabilities: ["vehicles.multiple"], validUntilMillis: now + day
        )
        XCTAssertEqual(VehicleAdmission.outcomeForNewVehicle(registered: 1, accountId: account, origin: origin, observation: stale, lease: stale.vehicleLease(), nowMillis: now), .accept, "offline lease inside 24 h")
        XCTAssertEqual(VehicleAdmission.outcomeForNewVehicle(registered: 1, accountId: account, origin: origin, observation: stale, lease: nil, nowMillis: now), .premiumRequired)
    }
}

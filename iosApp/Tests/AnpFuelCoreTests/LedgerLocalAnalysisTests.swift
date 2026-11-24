import Foundation
import XCTest
@testable import AnpFuelCore

final class LedgerLocalAnalysisTests: XCTestCase {
    private let origin = "https://example.test"
    private let accountA = "00000000-0000-4000-8000-000000000001"
    private let accountB = "00000000-0000-4000-8000-000000000002"
    private let vehicleId = "00000000-0000-4000-8000-000000000003"
    private let secondVehicleId = "00000000-0000-4000-8000-000000000004"

    func testEstimatedEfficiencyUsesAndroidDecimalScaleAndBounds() {
        for value in [" 12,50 ", "12.5", "+12.5", "125e-1"] {
            XCTAssertEqual(LedgerEstimatedEfficiency(value)?.canonical, "12.5")
        }
        XCTAssertEqual(LedgerEstimatedEfficiency("1e+3")?.canonical, "1000")
        XCTAssertEqual(LedgerEstimatedEfficiency(",01")?.hundredths, 1)
        for value in ["0", "-3", "1000.01", "abc", "12.345", "12.500", "1.25e-2", "1,000.00", "2e100", ".", "+"] {
            XCTAssertNil(LedgerEstimatedEfficiency(value), value)
        }
    }

    func testPrivatePreferenceIsScopedAndClearedOnVehicleOrAccountRemoval() throws {
        let directory = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString, isDirectory: true)
        defer { try? FileManager.default.removeItem(at: directory) }
        let account = try XCTUnwrap(LedgerEstimateScope(accountId: accountA, origin: origin))
        let other = try XCTUnwrap(LedgerEstimateScope(accountId: accountB, origin: origin))
        let guest = try XCTUnwrap(LedgerEstimateScope(accountId: nil, origin: origin))
        let differentOrigin = try XCTUnwrap(LedgerEstimateScope(accountId: accountA, origin: "https://other.test"))
        let estimate = try XCTUnwrap(LedgerEstimatedEfficiency("12,5"))
        let alternative = try XCTUnwrap(LedgerEstimatedEfficiency("9"))
        let first = LedgerEfficiencyPreferenceStore(directory: directory)
        try first.set(estimate, for: vehicleId, in: account)
        try first.set(alternative, for: secondVehicleId, in: account)
        try first.set(alternative, for: vehicleId, in: other)

        let restarted = LedgerEfficiencyPreferenceStore(directory: directory)
        XCTAssertEqual(restarted.estimate(for: vehicleId, in: account), estimate)
        XCTAssertEqual(restarted.estimate(for: vehicleId, in: other), alternative)
        XCTAssertNil(restarted.estimate(for: vehicleId, in: guest))
        XCTAssertNil(restarted.estimate(for: vehicleId, in: differentOrigin))
        try restarted.set(estimate, for: vehicleId, in: differentOrigin)
        try restarted.set(alternative, for: vehicleId, in: guest)
        try restarted.removeVehicles([vehicleId], in: account)
        XCTAssertNil(restarted.estimate(for: vehicleId, in: account))
        XCTAssertEqual(restarted.estimate(for: secondVehicleId, in: account), alternative)
        try restarted.set(nil, for: secondVehicleId, in: account)
        XCTAssertNil(restarted.estimate(for: secondVehicleId, in: account))
        try restarted.set(estimate, for: vehicleId, in: account)
        try restarted.erase(account)
        XCTAssertNil(restarted.estimate(for: vehicleId, in: account))
        XCTAssertEqual(restarted.estimate(for: vehicleId, in: other), alternative)
        XCTAssertEqual(restarted.estimate(for: vehicleId, in: differentOrigin), estimate)
        XCTAssertEqual(restarted.estimate(for: vehicleId, in: guest), alternative)
        try restarted.erase(guest)
        XCTAssertNil(restarted.estimate(for: vehicleId, in: guest))
        XCTAssertEqual(restarted.estimate(for: vehicleId, in: other), alternative)
    }

    func testFallbackPreservesMeasuredCostReasonsAndNeverEntersCSV() throws {
        let configured = try XCTUnwrap(LedgerEstimatedEfficiency("11,65"))
        let unknown = try monthly([])
        XCTAssertEqual(configured.fallback(for: unknown), configured)
        XCTAssertEqual(unknown.measured.unknown, "no-records")
        XCTAssertTrue(unknown.cost.centavosPerKm.isEmpty)
        XCTAssertFalse(LedgerMonthlyCSV.build(unknown).contains("11.65"))

        let baseline = refuel(day: 3, odometer: 1_000_000, litersMilli: 50_000, cents: 30_000)
        let final = refuel(day: 12, odometer: 1_400_000, litersMilli: 40_000, cents: 24_400)
        let measured = try monthly([baseline, final])
        XCTAssertEqual(measured.measured.kmPerUnit, "10")
        XCTAssertNil(configured.fallback(for: measured))
        XCTAssertTrue(LedgerMonthlyCSV.build(measured).contains(",km_per_unit,10\n"))
        XCTAssertTrue(LedgerMonthlyCSV.build(measured).contains(",centavos_per_km,61\n"))

        let review = try monthly([refuel(day: 3, odometer: 1_000_000, litersMilli: 50_000, cents: 30_000, review: true)])
        XCTAssertEqual(review.measured.unknown, "odometer-unit-unverified")
        XCTAssertNil(configured.fallback(for: review))
    }

    func testMonthlyCSVHasExactAndroidRowsForSelectedWindowAndEscapesVehicleNames() throws {
        let expense = LedgerEntryRecord(vehicleId: vehicleId, kind: .expense, occurredAtMillis: at(day: 6),
                                        timezone: "UTC", totalCentavos: 2701, category: .maintenance)
        let other = LedgerEntryRecord(vehicleId: secondVehicleId, kind: .expense, occurredAtMillis: at(day: 6),
                                      timezone: "UTC", totalCentavos: 4001, category: .maintenance)
        let later = LedgerEntryRecord(vehicleId: vehicleId, kind: .expense, occurredAtMillis: at(day: 12, month: 4),
                                      timezone: "UTC", totalCentavos: 5001, category: .maintenance)
        let report = try monthly([expense, other, later], vehicleName: " \t=1+1")
        let csv = LedgerMonthlyCSV.build(report)
        XCTAssertTrue(csv.hasPrefix("vehicle,metric,value\n\"' \t=1+1\",purchase_centavos,0\n"))
        XCTAssertTrue(csv.contains(",expense_centavos,2701\n"))
        XCTAssertTrue(csv.contains(",expense:maintenance,2701\n"))
        XCTAssertTrue(csv.contains(",energy_unknown,no-records\n"))
        XCTAssertTrue(csv.hasSuffix(",charge_fees_centavos,0\n"))
        XCTAssertFalse(csv.contains("4001"))
        XCTAssertFalse(csv.contains("5001"))
        XCTAssertFalse(csv.contains("km_per_unit"))
        XCTAssertEqual(LedgerCsvExport.escape("\"A,B\""), "\"'\"\"A,B\"\"\"")
    }

    private func monthly(_ entries: [LedgerEntryRecord], vehicleName: String = "Example") throws -> LedgerMonthlyIndicators {
        let vehicle = VehicleRecord(id: vehicleId, name: vehicleName, propulsion: .combustion,
                                    primaryFuel: .gasolineRegular, tankCapacityMilliLiters: 50_000)
        return try LedgerIndicators.monthly(vehicle: vehicle, records: entries, year: 2026, month: 3, timezone: "UTC")
    }

    private func at(day: Int, month: Int = 3) -> Int64 {
        let calendar = Calendar(identifier: .gregorian)
        let components = DateComponents(timeZone: .gmt, year: 2026, month: month, day: day, hour: 12)
        return Int64((calendar.date(from: components)?.timeIntervalSince1970 ?? 0) * 1_000)
    }

    private func refuel(day: Int, odometer: Int64, litersMilli: Int64, cents: Int64, review: Bool = false) -> LedgerEntryRecord {
        LedgerEntryRecord(vehicleId: vehicleId, kind: .refuel, occurredAtMillis: at(day: day), timezone: "UTC",
                          fuel: .gasolineRegular, quantityMilli: litersMilli, totalCentavos: cents, fill: .full,
                          odometerMetres: odometer, reviewRequired: review)
    }
}

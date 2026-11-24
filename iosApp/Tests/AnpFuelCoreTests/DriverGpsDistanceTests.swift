import Foundation
import XCTest
@testable import AnpFuelCore

final class DriverGpsDistanceTests: XCTestCase {
    private let scope = DriverGpsScope(accountId: "scope-owner", origin: "https://abastevo.com.br")

    func testEstimateSkipsUnknownPoorStaleSimulatedAndOutOfOrderFixes() {
        var gps = DriverGpsAccumulator()
        XCTAssertNil(gps.distanceMetres)
        XCTAssertEqual(gps.append(accuracyMetres: 12, sampledAt: 2, receivedAt: 18, simulated: false, segmentMetres: nil), .rejected)
        XCTAssertEqual(gps.append(accuracyMetres: 12, sampledAt: 3, receivedAt: 3, simulated: true, segmentMetres: nil), .rejected)
        XCTAssertEqual(gps.append(accuracyMetres: -1, sampledAt: 4, receivedAt: 4, simulated: false, segmentMetres: nil), .rejected)
        XCTAssertEqual(gps.append(accuracyMetres: 52, sampledAt: 5, receivedAt: 6, simulated: false, segmentMetres: nil), .rejected)
        XCTAssertEqual(gps.append(accuracyMetres: .nan, sampledAt: 6, receivedAt: 7, simulated: false, segmentMetres: nil), .rejected)
        XCTAssertEqual(gps.append(accuracyMetres: 50, sampledAt: 7, receivedAt: 7, simulated: false, segmentMetres: nil), .anchor)
        XCTAssertNil(gps.distanceMetres)
        XCTAssertEqual(gps.append(accuracyMetres: 5, sampledAt: 7, receivedAt: 8, simulated: false, segmentMetres: 8), .rejected)
        XCTAssertEqual(gps.append(accuracyMetres: 5, sampledAt: 9, receivedAt: 8, simulated: false, segmentMetres: 8), .rejected)
        XCTAssertEqual(gps.append(accuracyMetres: 5, sampledAt: 8, receivedAt: 8, simulated: false, segmentMetres: 0), .segment)
        XCTAssertEqual(gps.distanceMetres, 0)
        XCTAssertEqual(gps.append(accuracyMetres: 5, sampledAt: 9, receivedAt: 9, simulated: false, segmentMetres: 47.8), .segment)
        XCTAssertEqual(gps.append(accuracyMetres: 5, sampledAt: 10, receivedAt: 10, simulated: false, segmentMetres: 32.8), .segment)
        XCTAssertEqual(gps.distanceMetres, 80)
        XCTAssertTrue(gps.hasGap)
    }

    func testLongGapAndProviderLossRestartAnchorWithoutFillingMissingMetres() {
        var gps = DriverGpsAccumulator()
        XCTAssertEqual(gps.append(accuracyMetres: 5, sampledAt: 1, receivedAt: 1, simulated: false, segmentMetres: nil), .anchor)
        XCTAssertEqual(gps.append(accuracyMetres: 5, sampledAt: 2, receivedAt: 2, simulated: false, segmentMetres: 10), .segment)
        XCTAssertEqual(gps.append(accuracyMetres: 5, sampledAt: 20, receivedAt: 20, simulated: false, segmentMetres: 140), .anchor)
        XCTAssertEqual(gps.distanceMetres, 10)
        gps.resetAnchor()
        XCTAssertEqual(gps.append(accuracyMetres: 5, sampledAt: 19, receivedAt: 21, simulated: false, segmentMetres: nil), .rejected)
        XCTAssertEqual(gps.append(accuracyMetres: 5, sampledAt: 21, receivedAt: 21, simulated: false, segmentMetres: nil), .anchor)
        XCTAssertEqual(gps.append(accuracyMetres: 5, sampledAt: 22, receivedAt: 22, simulated: false, segmentMetres: -4), .rejected)
        XCTAssertEqual(gps.append(accuracyMetres: 5, sampledAt: 22, receivedAt: 22, simulated: false, segmentMetres: .infinity), .rejected)
        XCTAssertEqual(gps.append(accuracyMetres: 5, sampledAt: 22, receivedAt: 22, simulated: false, segmentMetres: Double.greatestFiniteMagnitude), .rejected)
        XCTAssertEqual(gps.distanceMetres, 10)
    }

    func testCheckpointSurvivesRestartWithoutDistanceAndOnlyUserStopCanCompleteItsOwnEpoch() throws {
        let directory = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
        defer { try? FileManager.default.removeItem(at: directory) }
        let first = DriverGpsCheckpointStore(directory: directory)
        let started = UUID()
        try first.begin(scope: scope, shiftId: "shift", recordingId: started)

        let restarted = DriverGpsCheckpointStore(directory: directory)
        let checkpoint = try XCTUnwrap(try restarted.load(scope: scope, shiftId: "shift"))
        XCTAssertEqual(checkpoint.recordingId, started)
        XCTAssertFalse(checkpoint.stopped)
        XCTAssertTrue(checkpoint.hasGap)
        XCTAssertNil(checkpoint.acknowledgedMetres)
        XCTAssertNil(try restarted.load(scope: scope, shiftId: "different"))
        XCTAssertNil(try restarted.load(scope: DriverGpsScope(accountId: nil, origin: scope.origin), shiftId: "shift"))
        XCTAssertNil(try restarted.load(scope: DriverGpsScope(accountId: "other", origin: scope.origin), shiftId: "shift"))
        XCTAssertNil(try restarted.load(scope: DriverGpsScope(accountId: scope.accountId, origin: "https://other.example"), shiftId: "shift"))

        XCTAssertThrowsError(try restarted.finish(scope: scope, shiftId: "shift", recordingId: UUID(), metres: 920)) {
            XCTAssertEqual($0 as? DriverGpsStoreError, .wrongRecording)
        }
        XCTAssertThrowsError(try restarted.finish(scope: scope, shiftId: "shift", recordingId: started, metres: -1))
        let stopped = try restarted.finish(scope: scope, shiftId: "shift", recordingId: started, metres: 920)
        XCTAssertEqual(stopped.acknowledgedMetres, 920)
        XCTAssertTrue(stopped.hasGap)
        XCTAssertEqual(try restarted.finish(scope: scope, shiftId: "shift", recordingId: started, metres: 999), stopped)

        let serialized = try JSONEncoder().encode(stopped)
        let object = try XCTUnwrap(JSONSerialization.jsonObject(with: serialized) as? [String: Any])
        XCTAssertEqual(Set(object.keys), ["recordingId", "stopped", "metres"])
        try restarted.remove(scope: scope, shiftId: "shift")
        XCTAssertNil(try first.load(scope: scope, shiftId: "shift"))
        try first.begin(scope: scope, shiftId: "next", recordingId: UUID())
        try first.purge()
        XCTAssertNil(try restarted.load(scope: scope, shiftId: "next"))
    }

    func testRecordedDistanceSourcesRemainDistinctAndLegacyShiftsAreGuestOnly() throws {
        #if SWIFT_PACKAGE
        let repo = (0..<4).reduce(URL(fileURLWithPath: #filePath)) { url, _ in url.deletingLastPathComponent() }
        let url = repo.appendingPathComponent("contracts/testdata/driver-suite/planned-financial-v1.json")
        #else
        let url = try XCTUnwrap(Bundle(for: Self.self).url(forResource: "planned-financial-v1", withExtension: "json"))
        #endif
        let document = try XCTUnwrap(JSONSerialization.jsonObject(with: Data(contentsOf: url)) as? [String: Any])
        let cases = try XCTUnwrap(document["vectors"] as? [[String: Any]])
        let vector = try XCTUnwrap(cases.first { $0["id"] as? String == "DRV-V13" })
        let input = try XCTUnwrap(vector["inputs"] as? [String: Any])
        let expected = try XCTUnwrap(vector["expected"] as? [String: Any])
        let odometer = try XCTUnwrap(input["odometer_distance_metres"] as? Int64)
        let gps = try XCTUnwrap(input["gps_distance_metres"] as? Int64)
        let legacy = DriverShiftRecord(
            vehicleId: "vehicle", startedAtLabel: "", startOdometerKm: 90, endOdometerKm: 90 + odometer / 1000, isActive: false
        )
        let reloaded = try JSONDecoder().decode(DriverShiftRecord.self, from: JSONEncoder().encode(legacy))
        XCTAssertEqual(reloaded.distanceMetres, expected["selected_distance_metres"] as? Int64)
        XCTAssertEqual(reloaded.distanceBasis, .odometer)
        XCTAssertFalse(reloaded.belongs(to: scope))
        XCTAssertTrue(reloaded.belongs(to: DriverGpsScope(accountId: nil, origin: scope.origin)))

        let estimated = DriverShiftRecord(
            vehicleId: "vehicle", startedAtLabel: "", startOdometerKm: 90, isActive: false,
            gpsDistanceMetres: gps, ownerAccountId: scope.accountId, ownerApiOrigin: scope.origin
        )
        XCTAssertEqual(estimated.distanceMetres, gps)
        XCTAssertNotEqual(estimated.distanceMetres, expected["not_distance_metres"] as? Int64)
        XCTAssertEqual(estimated.distanceBasis, .gpsEstimate)
        XCTAssertTrue(estimated.belongs(to: scope))
        XCTAssertFalse(estimated.belongs(to: DriverGpsScope(accountId: scope.accountId, origin: "https://other.example")))
        XCTAssertNil(estimated.endOdometerKm)
    }
}

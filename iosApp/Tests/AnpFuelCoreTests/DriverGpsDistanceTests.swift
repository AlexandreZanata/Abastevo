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
        // Refused fixes between accepted ones drop no distance.
        XCTAssertFalse(gps.hasGap)
    }

    func testLongGapAndProviderLossRestartAnchorWithoutFillingMissingMetres() {
        var gps = DriverGpsAccumulator()
        gps.resetAnchor()
        XCTAssertFalse(gps.hasGap)
        XCTAssertEqual(gps.append(accuracyMetres: 5, sampledAt: 1, receivedAt: 1, simulated: false, segmentMetres: nil), .anchor)
        XCTAssertEqual(gps.append(accuracyMetres: 5, sampledAt: 2, receivedAt: 2, simulated: false, segmentMetres: 10), .segment)
        XCTAssertFalse(gps.hasGap)
        XCTAssertEqual(gps.append(accuracyMetres: 5, sampledAt: 20, receivedAt: 20, simulated: false, segmentMetres: 140), .anchor)
        XCTAssertEqual(gps.distanceMetres, 10)
        XCTAssertTrue(gps.hasGap)

        var lost = DriverGpsAccumulator()
        XCTAssertEqual(lost.append(accuracyMetres: 5, sampledAt: 1, receivedAt: 1, simulated: false, segmentMetres: nil), .anchor)
        lost.resetAnchor()
        XCTAssertTrue(lost.hasGap)
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
        XCTAssertNil(checkpoint.acknowledgedMetres)
        XCTAssertNil(checkpoint.shiftEstimate)
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
        XCTAssertEqual(stopped.shiftEstimate, DriverGpsShiftEstimate(metres: 920, incomplete: false))
        XCTAssertEqual(try restarted.finish(scope: scope, shiftId: "shift", recordingId: started, metres: 999), stopped)

        let serialized = try JSONEncoder().encode(stopped)
        let object = try XCTUnwrap(JSONSerialization.jsonObject(with: serialized) as? [String: Any])
        XCTAssertEqual(Set(object.keys), ["recordingId", "stopped", "metres", "priorRecordings", "gap"])
        try restarted.remove(scope: scope, shiftId: "shift")
        XCTAssertNil(try first.load(scope: scope, shiftId: "shift"))
        try first.begin(scope: scope, shiftId: "next", recordingId: UUID())
        try first.purge(scope: scope)
        XCTAssertNil(try restarted.load(scope: scope, shiftId: "next"))
    }

    func testRestartKeepsStoppedMetresAndMarksTheShiftEstimateIncomplete() throws {
        let directory = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
        defer { try? FileManager.default.removeItem(at: directory) }
        let store = DriverGpsCheckpointStore(directory: directory)
        let morning = UUID()
        try store.begin(scope: scope, shiftId: "shift", recordingId: morning)
        let first = try store.finish(scope: scope, shiftId: "shift", recordingId: morning, metres: 1_000)
        XCTAssertEqual(first.shiftEstimate, DriverGpsShiftEstimate(metres: 1_000, incomplete: false))

        let afternoon = UUID()
        let restarted = try store.begin(scope: scope, shiftId: "shift", recordingId: afternoon)
        XCTAssertEqual(restarted.priorMetres, 1_000)
        XCTAssertEqual(restarted.priorRecordings, 1)
        XCTAssertNil(restarted.acknowledgedMetres)
        XCTAssertEqual(try store.begin(scope: scope, shiftId: "shift", recordingId: afternoon), restarted)
        let reopened = DriverGpsCheckpointStore(directory: directory)
        XCTAssertEqual(try reopened.load(scope: scope, shiftId: "shift"), restarted)
        let second = try reopened.finish(scope: scope, shiftId: "shift", recordingId: afternoon, metres: 500)
        XCTAssertEqual(second.shiftEstimate, DriverGpsShiftEstimate(metres: 1_500, incomplete: true))

        // An interrupted recording adds nothing; the stopped metres before it survive.
        try reopened.begin(scope: scope, shiftId: "shift", recordingId: UUID())
        let evening = UUID()
        let afterInterruption = try reopened.begin(scope: scope, shiftId: "shift", recordingId: evening)
        XCTAssertEqual(afterInterruption.priorMetres, 1_500)
        XCTAssertEqual(afterInterruption.priorRecordings, 3)
        XCTAssertEqual(afterInterruption.gap, true)
        let withoutFix = try reopened.finish(scope: scope, shiftId: "shift", recordingId: evening, metres: nil)
        XCTAssertEqual(withoutFix.shiftEstimate, DriverGpsShiftEstimate(metres: 1_500, incomplete: true))

        // A single recording with a dropped gap is incomplete; one without fixes has no estimate.
        let gapped = UUID()
        try store.begin(scope: scope, shiftId: "gapped", recordingId: gapped)
        let dropped = try store.finish(scope: scope, shiftId: "gapped", recordingId: gapped, metres: 300, droppedGap: true)
        XCTAssertEqual(dropped.shiftEstimate, DriverGpsShiftEstimate(metres: 300, incomplete: true))
        let empty = UUID()
        try store.begin(scope: scope, shiftId: "empty", recordingId: empty)
        XCTAssertNil(try store.finish(scope: scope, shiftId: "empty", recordingId: empty, metres: nil).shiftEstimate)

        XCTAssertNil(DriverGpsCheckpoint.combined(DriverGpsAccumulator.maximumDistanceMetres, 1))
        XCTAssertNil(DriverGpsCheckpoint.combined(nil, nil))
        XCTAssertEqual(DriverGpsCheckpoint.combined(nil, 7), 7)
    }

    func testFilesWithoutCoverageFieldsStillDecodeAsIncompleteEstimates() throws {
        let id = UUID()
        let legacy = Data(#"{"recordingId":"\#(id.uuidString)","stopped":true,"metres":700}"#.utf8)
        let checkpoint = try JSONDecoder().decode(DriverGpsCheckpoint.self, from: legacy)
        XCTAssertEqual(checkpoint.shiftEstimate, DriverGpsShiftEstimate(metres: 700, incomplete: true))

        let estimated = DriverShiftRecord(
            vehicleId: "vehicle", startedAtLabel: "", startOdometerKm: 90, isActive: false, gpsDistanceMetres: 700,
            ownerAccountId: scope.accountId, ownerApiOrigin: scope.origin, gpsDistanceIncomplete: false
        )
        XCTAssertFalse(estimated.isGpsEstimateIncomplete)
        var object = try XCTUnwrap(JSONSerialization.jsonObject(with: JSONEncoder().encode(estimated)) as? [String: Any])
        XCTAssertEqual(object["gpsDistanceIncomplete"] as? Bool, false)
        object["gpsDistanceIncomplete"] = nil
        let older = try JSONDecoder().decode(DriverShiftRecord.self, from: JSONSerialization.data(withJSONObject: object))
        XCTAssertEqual(older.gpsDistanceMetres, 700)
        XCTAssertTrue(older.isGpsEstimateIncomplete)
        let odometer = DriverShiftRecord(
            vehicleId: "vehicle", startedAtLabel: "", startOdometerKm: 90, endOdometerKm: 91, isActive: false,
            gpsDistanceIncomplete: true
        )
        XCTAssertNil(odometer.gpsDistanceIncomplete)
        XCTAssertFalse(odometer.isGpsEstimateIncomplete)
    }

    func testLogoutPurgesOnlyTheDepartingScopeAndAnOriginSwitchEveryOwnerThere() throws {
        let directory = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
        defer { try? FileManager.default.removeItem(at: directory) }
        let store = DriverGpsCheckpointStore(directory: directory)
        let guest = DriverGpsScope(accountId: nil, origin: scope.origin)
        let other = DriverGpsScope(accountId: "other-owner", origin: scope.origin)
        let elsewhere = DriverGpsScope(accountId: scope.accountId, origin: "https://other.example")
        let guestElsewhere = DriverGpsScope(accountId: nil, origin: "https://other.example")
        for owner in [scope, guest, other, elsewhere, guestElsewhere] {
            try store.begin(scope: owner, shiftId: "shift", recordingId: UUID())
        }

        try store.purge(scope: scope)
        XCTAssertNil(try store.load(scope: scope, shiftId: "shift"))
        for kept in [guest, other, elsewhere, guestElsewhere] {
            XCTAssertNotNil(try store.load(scope: kept, shiftId: "shift"))
        }

        try store.purge(origin: scope.origin)
        XCTAssertNil(try store.load(scope: guest, shiftId: "shift"))
        XCTAssertNil(try store.load(scope: other, shiftId: "shift"))
        XCTAssertNotNil(try store.load(scope: elsewhere, shiftId: "shift"))
        XCTAssertNotNil(try store.load(scope: guestElsewhere, shiftId: "shift"))
        try store.purge(origin: "https://never-used.example")
        XCTAssertNotNil(try store.load(scope: elsewhere, shiftId: "shift"))
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

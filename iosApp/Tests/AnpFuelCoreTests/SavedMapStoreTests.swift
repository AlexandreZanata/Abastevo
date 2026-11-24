import Foundation
import XCTest
@testable import AnpFuelCore

final class SavedMapStoreTests: XCTestCase {
    private let origin = "https://configured.example"
    private let accountA = "00000000-0000-4000-8000-000000000001"
    private let accountB = "00000000-0000-4000-8000-000000000002"
    private let stationId = "00000000-0000-4000-8000-000000000003"

    func testScopeSurvivesRestartButNeverCrossesOriginOwnerOrGuest() throws {
        try withDirectory { directory in
            let owner = try scope(accountA)
            let storage = SavedMapStore(directory: directory)
            let point = try SavedGeoPoint(latitudeMicro: 10, longitudeMicro: 20)
            let inventory = try storage.toggleStation(
                id: stationId, name: "Example station", point: point, source: .anp, note: nil, nowMillis: 1, in: owner
            )
            XCTAssertTrue(inventory.hasStation(stationId))
            let restarted = SavedMapStore(directory: directory)
            XCTAssertTrue(try restarted.inventory(in: owner).hasStation(stationId))
            for other in [try scope(accountB), try scope(SavedMapScope.guest),
                          try scope(accountA, origin: "https://different.example")] {
                XCTAssertTrue(try restarted.inventory(in: other).places.isEmpty)
                let id = try XCTUnwrap(inventory.places.first?.id)
                XCTAssertTrue(try restarted.delete(.place(id), in: other).places.isEmpty)
                XCTAssertThrowsError(try restarted.exportGPX(.everything, unnamedPlace: "Unnamed", in: other))
            }
            XCTAssertTrue(try restarted.inventory(in: owner).hasStation(stationId))
            try restarted.erase(owner)
            XCTAssertTrue(try storage.inventory(in: owner).places.isEmpty)
        }
    }

    func testPurgingAGuestOriginKeepsOtherOwnedPartitions() throws {
        try withDirectory { directory in
            let old = try scope(SavedMapScope.guest)
            let other = try scope(SavedMapScope.guest, origin: "https://other.example")
            let account = try scope(accountA)
            let store = SavedMapStore(directory: directory)
            let content = try gpx(#"<gpx><wpt lat="1" lon="2"><name>Saved</name></wpt></gpx>"#)
            for scope in [old, other, account] {
                _ = try store.importGPX(content, restoringDeleted: false, in: scope)
            }
            try store.erase(old)
            XCTAssertTrue(try store.inventory(in: old).places.isEmpty)
            XCTAssertEqual(try store.inventory(in: other).places.count, 1)
            XCTAssertEqual(try store.inventory(in: account).places.count, 1)
        }
    }

    func testRepeatImportAndTombstoneRequireConfirmationBeforeAnyWrite() throws {
        try withDirectory { directory in
            let owner = try scope(accountA)
            let other = try scope(accountB)
            let storage = SavedMapStore(directory: directory)
            let content = try gpx(#"<gpx><wpt lat="1" lon="2"><name>One</name></wpt><wpt lat="3" lon="4"><name>Two</name></wpt></gpx>"#)
            let first = try storage.importGPX(content, restoringDeleted: false, in: owner)
            XCTAssertEqual(first.result.importedPlaces, 2)
            XCTAssertTrue(try storage.importGPX(content, restoringDeleted: false, in: owner).result.alreadyImported)
            let id = try XCTUnwrap(first.inventory.places.first?.id)
            _ = try storage.delete(.place(id), in: owner)
            XCTAssertThrowsError(try storage.importGPX(content, restoringDeleted: false, in: owner)) {
                XCTAssertEqual($0 as? SavedMapError, .restoreConfirmationRequired)
            }
            XCTAssertEqual(try storage.inventory(in: owner).places.map(\.name), ["Two"])
            XCTAssertEqual(try storage.importGPX(content, restoringDeleted: false, in: other).result.importedPlaces, 2)
            XCTAssertEqual(try storage.importGPX(content, restoringDeleted: true, in: owner).result.importedPlaces, 1)
            XCTAssertTrue(try storage.importGPX(content, restoringDeleted: false, in: owner).result.alreadyImported)
        }
    }

    func testLegacyMigrationRunsOnceAndKeepsSummariesWithoutSyntheticTripGeometry() throws {
        try withDirectory { directory in
            let guest = try scope(SavedMapScope.guest)
            let owner = try scope(accountA)
            let storage = SavedMapStore(directory: directory)
            let place = SavedPlaceRecord(id: "older-place", title: "Legacy title", subtitle: "Legacy subtitle",
                                         latitude: -25, longitude: -48, stationId: "old-station-id")
            let trip = SavedTripRecord(id: "older-trip", title: "Summary", originLabel: "A", destinationLabel: "B", distanceKm: 3)
            let inventory = try storage.migrateLegacy([place], trips: [trip], inGuestScope: guest)
            XCTAssertEqual(inventory.legacyPlaces, [place])
            XCTAssertEqual(inventory.legacyTrips, [trip])
            XCTAssertTrue(inventory.hasStation("old-station-id"))
            XCTAssertTrue(try storage.inventory(in: owner).legacyPlaces.isEmpty)
            XCTAssertThrowsError(try storage.migrateLegacy([place], trips: [trip], inGuestScope: owner))
            let file = try storage.exportGPX(.everything, unnamedPlace: "Imported place", in: guest)
            let exported = try gpx(String(decoding: file, as: UTF8.self))
            XCTAssertEqual(exported.places.first?.name, place.title)
            XCTAssertEqual(exported.places.first?.sourceId, place.stationId)
            XCTAssertTrue(exported.trips.isEmpty)
            _ = try storage.delete(.legacyPlace(place.id), in: guest)
            let retry = try storage.migrateLegacy([place], trips: [trip], inGuestScope: guest)
            XCTAssertTrue(retry.legacyPlaces.isEmpty)
            XCTAssertEqual(retry.legacyTrips, [trip])
            XCTAssertThrowsError(try storage.importGPX(exported, restoringDeleted: false, in: guest)) {
                XCTAssertEqual($0 as? SavedMapError, .restoreConfirmationRequired)
            }
        }
    }

    func testQuotaFailureLeavesPreviousInventoryAndImportDigestUntouched() throws {
        try withDirectory { directory in
            let owner = try scope(accountA)
            let storage = SavedMapStore(directory: directory)
            let full = try gpx("<gpx>" + String(repeating: #"<wpt lat="1" lon="2"/>"#, count: SavedMapBounds.places) + "</gpx>")
            let extra = try gpx(#"<gpx><wpt lat="3" lon="4"><name>Extra</name></wpt></gpx>"#)
            let ready = try storage.importGPX(full, restoringDeleted: false, in: owner)
            XCTAssertEqual(ready.inventory.places.count, SavedMapBounds.places)
            XCTAssertThrowsError(try storage.importGPX(extra, restoringDeleted: false, in: owner)) {
                XCTAssertEqual($0 as? SavedMapError, .placeQuota)
            }
            let saved = try storage.inventory(in: owner)
            XCTAssertEqual(saved, ready.inventory)
            let first = try XCTUnwrap(saved.places.first?.id)
            _ = try storage.delete(.place(first), in: owner)
            XCTAssertEqual(try storage.importGPX(extra, restoringDeleted: false, in: owner).result.importedPlaces, 1)
        }
    }

    private func withDirectory(_ body: (URL) throws -> Void) throws {
        let root = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString, isDirectory: true)
        defer { try? FileManager.default.removeItem(at: root) }
        try body(root)
    }

    private func scope(_ user: String, origin: String? = nil) throws -> SavedMapScope {
        try XCTUnwrap(SavedMapScope(owner: user, origin: origin ?? self.origin))
    }

    private func gpx(_ value: String) throws -> SavedGPXContent {
        try SavedGPX.parse(Data(value.utf8), nowMillis: 1000, unnamedPlace: "Imported place", unnamedTrip: "Imported trip")
    }
}

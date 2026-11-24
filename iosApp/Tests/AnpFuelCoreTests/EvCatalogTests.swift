import XCTest
@testable import AnpFuelCore

/// Wire shapes mirror the Go handler `placeFacts`/`connectionGroupFacts` and its national catalog test.
final class EvCatalogTests: XCTestCase {
    static let placeId = "00000000-0000-4000-8000-000000000001"
    static let secondId = "00000000-0000-4000-8000-000000000002"
    static let groupId = "00000000-0000-4000-8000-0000000000a1"
    static let entity = String(repeating: "0", count: 63) + "1"
    static let groupEntity = String(repeating: "a", count: 64)
    static let revision = String(repeating: "1", count: 64)

    static func nationalPage(nextCursor: String? = "opaque-next", extraPlaceField: String = "", groupSource: String = "osm:node:9") -> Data {
        let cursor = nextCursor.map { "\"\($0)\"" } ?? "null"
        return Data("""
        {"items":[
          {"id":"\(placeId)","entity_key":"\(entity)","external_id":"0","name":"Eletroposto Teste","country":"BR",
           "latitude":"-23.5","longitude":"-46.6","public_access":"PUBLIC","reported_points":2,
           "sources":[{"source":"OSM","external_id":"node:9","revision":"synthetic","license":"ODbL-1.0","attribution":"Synthetic attribution"}],
           "confidence":"single-source","uf":"SP","operator":"Synthetic operator",
           "address":{"street":"Synthetic street","number":"7","municipio":"São Paulo"},
           "access_class":"public","linked_station_id":"00000000-0000-4000-8000-0000000000ff",\(extraPlaceField)
           "groups":[{"id":"\(groupId)","entity_key":"\(groupEntity)","external_id":"osm:node:9:socket:ccs2",
             "connector_code":"CCS2","raw_connector_code":"type2_combo","current":"DC","power_w":150000,"source_key":"\(groupSource)"}]},
          {"id":"\(secondId)","entity_key":"\(String(repeating: "2", count: 64))","external_id":"1","name":"","country":"BR",
           "latitude":"-23.6","longitude":"-46.7","public_access":"UNKNOWN","reported_points":0,
           "sources":[{"source":"OCM","external_id":"55","revision":"synthetic","provider_id":3,"license":"CC-BY-4.0","attribution":"Open Charge Map"}],
           "confidence":"single-source","uf":"SP","groups":[]}
        ],"next_cursor":\(cursor),"generated_at":"2026-11-23T10:00:00Z"}
        """.utf8)
    }

    func testNationalPageDecodesPublicFactsAndAttribution() throws {
        let page = try ChargingWireDecoder.decodePage(Self.nationalPage())
        XCTAssertEqual(page.nextCursor, "opaque-next")
        XCTAssertEqual(page.items.count, 2)
        let place = try XCTUnwrap(page.items.first)
        XCTAssertEqual(place.coordinate, EvCoordinate(latitude: -23.5, longitude: -46.6))
        XCTAssertEqual(place.groups.first?.connector, .ccs2)
        XCTAssertEqual(place.groups.first?.powerW, 150_000)
        XCTAssertEqual(place.sources.first?.sourceKey, "osm:node:9")
        XCTAssertEqual(place.operatorName, "Synthetic operator")
        XCTAssertEqual(place.addressLine, "Synthetic street, 7 – São Paulo")
        XCTAssertEqual(place.publicAccess, .publicAccess)
        XCTAssertEqual(place.maxPowerW, 150_000)
        let unnamed = try XCTUnwrap(page.items.last)
        let label = "android.charging_place_unnamed"
        let resources = CoreLocalization.bundle
        XCTAssertEqual(unnamed.displayName, resources.localizedString(forKey: label, value: nil, table: nil))
        for (locale, expected) in [("en", "Unnamed charging site"), ("pt-BR", "Eletroposto sem nome")] {
            let url = try XCTUnwrap(resources.url(forResource: locale, withExtension: "lproj"))
            let localization = try XCTUnwrap(Bundle(url: url))
            XCTAssertEqual(localization.localizedString(forKey: label, value: nil, table: nil), expected)
        }
        XCTAssertEqual(unnamed.publicAccess, .unknown)
        XCTAssertTrue(ChargingRules.isInCity(place, city: Municipality(ibgeCode: "3550308", name: "SÃO PAULO", state: .SP)))
    }

    func testPrivateFieldsForeignOwnersAndMalformedRowsFailClosed() {
        XCTAssertThrowsError(try ChargingWireDecoder.decodePage(Self.nationalPage(extraPlaceField: #""price":1,"#)))
        XCTAssertThrowsError(try ChargingWireDecoder.decodePage(Self.nationalPage(groupSource: "osm:node:10")))
        XCTAssertThrowsError(try ChargingWireDecoder.decodePage(Self.nationalPage(nextCursor: "")))
        let badCountry = String(decoding: Self.nationalPage(), as: UTF8.self).replacingOccurrences(of: "\"country\":\"BR\"", with: "\"country\":\"AR\"")
        XCTAssertThrowsError(try ChargingWireDecoder.decodePage(Data(badCountry.utf8)))
        let badLatitude = String(decoding: Self.nationalPage(), as: UTF8.self).replacingOccurrences(of: "\"-23.5\"", with: "\"-123.5\"")
        XCTAssertThrowsError(try ChargingWireDecoder.decodePage(Data(badLatitude.utf8)))
        let twoSourceWithOne = String(decoding: Self.nationalPage(), as: UTF8.self).replacingOccurrences(of: "\"confidence\":\"single-source\",\"uf\":\"SP\",\"operator\"", with: "\"confidence\":\"two-source\",\"uf\":\"SP\",\"operator\"")
        XCTAssertThrowsError(try ChargingWireDecoder.decodePage(Data(twoSourceWithOne.utf8)))
        XCTAssertTrue(ChargingWireRules.isCanonicalId(Self.placeId))
        XCTAssertFalse(ChargingWireRules.isCanonicalId("00000000-0000-4000-8000-00000000000A"))
        XCTAssertNil(ChargingWireRules.decimal("-23.12345678901234567", minimum: -90, maximum: 90, maxScale: 16))
        XCTAssertEqual(ChargingWireRules.decimal("-23.5000", minimum: -90, maximum: 90, maxScale: 1), -23.5)
    }

    func testCoverageAndOfferDecoding() throws {
        let body = Data("""
        {"profile":"charging-br-national-v1","source_revision":"\(Self.revision)","places":2,"groups":1,"vehicles":0,
         "offer":{"notice":"/v1/charging/catalog/\(Self.revision)/offer/derived-database-offer.json","data":"/v1/charging/catalog/\(Self.revision)/offer/derived-database.ndjson"},
         "generated_at":"2026-11-23T10:00:00Z"}
        """.utf8)
        let coverage = try ChargingWireDecoder.decodeCoverage(body)
        let offer = try XCTUnwrap(EvCatalogService.expectedOffer(revision: Self.revision))
        XCTAssertEqual(coverage.offerNotice, offer.notice)
        XCTAssertEqual(coverage.offerData, offer.data)
        XCTAssertEqual(coverage.places, 2)
        XCTAssertNil(EvCatalogService.expectedOffer(revision: "not-a-revision"))
        XCTAssertThrowsError(try ChargingWireDecoder.decodeCoverage(Data(#"{"profile":"","source_revision":"x","places":1,"groups":0,"vehicles":0,"generated_at":"2026-11-23T10:00:00Z"}"#.utf8)))
    }

    func testFiltersCompatibilityAndFreshness() throws {
        let page = try ChargingWireDecoder.decodePage(Self.nationalPage())
        let place = try XCTUnwrap(page.items.first)
        let unnamed = try XCTUnwrap(page.items.last)
        XCTAssertTrue(ChargingRules.matches(place, filter: ChargingFilter(connector: .ccs2, current: .dc, publicOnly: true, minPowerKw: 50)))
        XCTAssertFalse(ChargingRules.matches(place, filter: ChargingFilter(connector: .type2)))
        XCTAssertFalse(ChargingRules.matches(place, filter: ChargingFilter(minPowerKw: 200)))
        XCTAssertFalse(ChargingRules.matches(unnamed, filter: ChargingFilter(publicOnly: true)), "unknown access is not public")
        XCTAssertFalse(ChargingRules.matches(unnamed, filter: ChargingFilter(minPowerKw: 7)), "national rows need a known power")
        XCTAssertTrue(ChargingRules.matches(place, filter: ChargingFilter(query: "synthetic operator")))

        XCTAssertEqual(ChargingRules.assess(vehicleConnector: .ccs2, groups: place.groups), .compatible)
        XCTAssertEqual(ChargingRules.assess(vehicleConnector: .type2, groups: place.groups), .incompatible)
        XCTAssertEqual(ChargingRules.assess(vehicleConnector: nil, groups: place.groups), .unknown)
        XCTAssertEqual(ChargingRules.assess(vehicleConnector: .ccs2, groups: []), .unknown)

        XCTAssertEqual(ChargingRules.age(nowEpochSeconds: 100, savedAtEpochSeconds: nil), .unknown)
        XCTAssertEqual(ChargingRules.age(nowEpochSeconds: 100, savedAtEpochSeconds: 200), .unknown)
        XCTAssertEqual(ChargingRules.age(nowEpochSeconds: 100 + 86_399, savedAtEpochSeconds: 100), .fresh)
        XCTAssertEqual(ChargingRules.age(nowEpochSeconds: 100 + 86_400, savedAtEpochSeconds: 100), .stale)
    }

    func testMapPinsUseAdmittedCoordinatesInsideTheVisibleRegion() throws {
        let page = try ChargingWireDecoder.decodePage(Self.nationalPage())
        let withoutCoordinate = ChargingPlace(
            id: "00000000-0000-4000-8000-000000000003", entityKey: String(repeating: "3", count: 64), externalId: "3",
            name: "Sem coordenada", latitude: nil, longitude: nil, publicAccess: .publicAccess, reportedPoints: 1,
            groups: [], sources: [], confidence: nil, uf: "SP", linkedStationId: nil, operatorName: nil, addressLine: nil, municipality: nil
        )
        let points = ChargingRules.mapPoints(page.items + [withoutCoordinate])
        XCTAssertEqual(points.map(\.id), [Self.placeId, Self.secondId])
        let near = ChargingRules.pins(points, center: EvCoordinate(latitude: -23.5, longitude: -46.6), latitudeDelta: 0.1, longitudeDelta: 0.1, limit: 10)
        XCTAssertEqual(near.map(\.id), [Self.placeId])
        let wide = ChargingRules.pins(points, center: EvCoordinate(latitude: -23.6, longitude: -46.7), latitudeDelta: 1, longitudeDelta: 1, limit: 10)
        XCTAssertEqual(wide.map(\.id), [Self.secondId, Self.placeId])
        XCTAssertEqual(ChargingRules.pins(points, center: EvCoordinate(latitude: -23.6, longitude: -46.7), latitudeDelta: 1, longitudeDelta: 1, limit: 1).count, 1)
    }

    func testSnapshotStoreRoundTrip() throws {
        let directory = FileManager.default.temporaryDirectory.appendingPathComponent("ev-store-\(UUID().uuidString)")
        defer { try? FileManager.default.removeItem(at: directory) }
        let store = EvCatalogStore(directory: directory)
        XCTAssertNil(store.load())
        let page = try ChargingWireDecoder.decodePage(Self.nationalPage(nextCursor: nil))
        let snapshot = EvCatalogSnapshot(
            profile: EvCatalogService.nationalProfile, sourceRevision: Self.revision, origin: "https://abastevo.com.br",
            license: EvCatalogService.nationalLicense, generatedAt: page.generatedAt, savedAtEpochSeconds: 1_800_000_000, places: page.items
        )
        try store.save(snapshot)
        XCTAssertEqual(store.load(), snapshot)
        XCTAssertEqual(store.load()?.groupCount, 1)
        store.clear()
        XCTAssertNil(store.load())
    }
}

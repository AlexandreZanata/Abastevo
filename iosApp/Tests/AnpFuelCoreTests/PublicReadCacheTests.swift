import XCTest
@testable import AnpFuelCore

final class PublicReadCacheTests: XCTestCase {
    private let origin = "https://abastevo.com.br"
    private let stationId = "00000000-0000-4000-8000-000000000001"

    private func makeCache() -> (PublicReadCache, URL) {
        let directory = FileManager.default.temporaryDirectory.appendingPathComponent("public-cache-\(UUID().uuidString)")
        return (PublicReadCache(directory: directory), directory)
    }

    private func observation(_ id: String, amount: Int64) -> CommunityPriceObservation {
        CommunityPriceObservation(
            id: id, stationId: id, stationName: "Posto", municipalityCode: "5107925", product: .gasolineRegular,
            amountMilli: amount, condition: .standard, confirmations: 1, disputes: 0, helpfulCount: 0,
            notHelpfulCount: 0, observedAtLabel: "2026-11-23T10:00:00Z", observedEpochSeconds: 1_800_000_000,
            hasPhotoEvidence: false
        )
    }

    func testCommunityFeedItemsKeepServerExpiryAndScope() throws {
        let body = Data("""
        {"items":[
          {"station_id":"\(stationId)","station_name":"Posto A","fuel_product":"GASOLINE_REGULAR","amount_milli_brl":5999,
           "unit":"L","currency":"BRL","source":"COMMUNITY","condition":"STANDARD",
           "updated_at":"2026-11-23T10:00:00Z","expires_at":"2026-11-24T10:00:00Z","confirmations":2},
          {"station_id":"00000000-0000-4000-8000-000000000002","station_name":"Posto B","fuel_product":"GASOLINE_REGULAR","amount_milli_brl":6099,
           "unit":"L","currency":"BRL","source":"COMMUNITY","condition":"PIX",
           "updated_at":"2026-11-20T10:00:00Z","expires_at":"2026-11-21T10:00:00Z","confirmations":0}
        ],"state":"MT","municipality_code":"5107925","fuel_product":"GASOLINE_REGULAR"}
        """.utf8)
        let items = try AbastevoHttpClient.decodeCommunityFeedItems(body, state: .MT, municipalityCode: "5107925", fuel: .gasolineRegular)
        XCTAssertEqual(items.count, 2)
        let now = try XCTUnwrap(AbastevoHttpClient.readIsoDate("2026-11-23T12:00:00Z"))
        let nowSeconds = Int64(now.timeIntervalSince1970)
        XCTAssertEqual(items.filter { $0.isLive(nowEpochSeconds: nowSeconds) }.map(\.observation.amountMilli), [5_999])
        XCTAssertEqual(items.last?.observation.condition, .pix)
        let live = try AbastevoHttpClient.decodeCommunityFeed(body, state: .MT, municipalityCode: "5107925", fuel: .gasolineRegular, now: now)
        XCTAssertEqual(live.map(\.amountMilli), [5_999], "same validation as the live feed decoder")
        XCTAssertThrowsError(try AbastevoHttpClient.decodeCommunityFeedItems(body, state: .MT, municipalityCode: "5103403", fuel: .gasolineRegular))
        XCTAssertThrowsError(try AbastevoHttpClient.decodeCommunityFeedItems(body, state: .MT, municipalityCode: "5107925", fuel: .ethanol))
    }

    func testCachedCommunityPricesExpireAndAreNeverPresentedAsCurrent() {
        let (cache, directory) = makeCache()
        defer { try? FileManager.default.removeItem(at: directory) }
        let saveTime = Date(timeIntervalSince1970: 1_000)
        let items = [
            CachedCommunityItem(observation: observation("a", amount: 5_999), expiresAtEpochSeconds: 2_000),
            CachedCommunityItem(observation: observation("b", amount: 6_099), expiresAtEpochSeconds: 3_000),
            CachedCommunityItem(observation: observation("c", amount: 6_199), expiresAtEpochSeconds: 900),
        ]
        cache.saveCommunityFeed(origin: origin, municipalityCode: "5107925", fuel: .gasolineRegular, items: items, now: saveTime)
        let cached = cache.communityFeed(origin: origin, municipalityCode: "5107925", fuel: .gasolineRegular)
        XCTAssertEqual(cached?.savedAtEpochSeconds, 1_000)
        XCTAssertEqual(cached?.items.count, 2, "already expired rows are never stored")
        XCTAssertEqual(cached?.liveObservations(nowEpochSeconds: 1_500).map(\.id), ["a", "b"])
        XCTAssertEqual(cached?.liveObservations(nowEpochSeconds: 2_500).map(\.id), ["b"])
        XCTAssertEqual(cached?.expiredCount(nowEpochSeconds: 2_500), 1)
        XCTAssertEqual(cached?.liveObservations(nowEpochSeconds: 3_000).count, 0)
        XCTAssertNil(cache.communityFeed(origin: "https://other.example", municipalityCode: "5107925", fuel: .gasolineRegular))
        XCTAssertNil(cache.communityFeed(origin: origin, municipalityCode: "5107925", fuel: .ethanol))
    }

    func testDirectoryAndOfficialPricesAreScopedAndBounded() {
        let (cache, directory) = makeCache()
        defer { try? FileManager.default.removeItem(at: directory) }
        let station = StationRecord(
            id: stationId, cnpj: "04218406000104", name: "Posto", brand: "", address: "Rua 1", neighborhood: "Centro",
            municipalityCode: "5107925", latitude: -12.5, longitude: -55.7, pinTier: .osmMapped,
            officialPricesMilli: [:], officialDateLabel: "", communityObservations: []
        )
        cache.saveDirectory(origin: origin, municipalityCode: "5107925", stations: [station], now: Date(timeIntervalSince1970: 50))
        XCTAssertEqual(cache.directory(origin: origin, municipalityCode: "5107925")?.stations, [station])
        XCTAssertEqual(cache.directory(origin: origin, municipalityCode: "5107925")?.savedAtEpochSeconds, 50)
        XCTAssertNil(cache.directory(origin: origin, municipalityCode: "5103403"))
        for index in 0..<(PublicReadCache.maxDirectories + 2) {
            cache.saveDirectory(origin: origin, municipalityCode: "code-\(index)", stations: [])
        }
        XCTAssertNil(cache.directory(origin: origin, municipalityCode: "5107925"), "oldest cities are evicted")

        let price = OfficialStationPrice(product: .ethanol, amountMilli: 4_190, collectedOn: "2026-11-10", surveyStart: "2026-11-09", surveyEnd: "2026-11-15")
        cache.saveStationPrices(origin: origin, stationId: stationId, municipalityCode: "5107925", prices: [price])
        let stored = cache.stationPrices(origin: origin, stationId: stationId)
        XCTAssertEqual(stored?.prices.map(\.officialPrice.amountMilli), [4_190])
        XCTAssertEqual(stored?.prices.first?.officialPrice.surveyEnd, "2026-11-15")
        XCTAssertNil(cache.stationPrices(origin: "https://other.example", stationId: stationId))
        cache.clear()
        XCTAssertNil(cache.stationPrices(origin: origin, stationId: stationId))
    }

    func testCorruptCacheFileReadsAsEmpty() throws {
        let (cache, directory) = makeCache()
        defer { try? FileManager.default.removeItem(at: directory) }
        try Data("{not json".utf8).write(to: directory.appendingPathComponent("public-read-cache-v1.json"))
        XCTAssertNil(cache.directory(origin: origin, municipalityCode: "5107925"))
        try Data(#"{"directories":[]}"#.utf8).write(to: directory.appendingPathComponent("public-read-cache-v1.json"))
        XCTAssertNil(cache.communityFeed(origin: origin, municipalityCode: "5107925", fuel: .ethanol))
    }
}

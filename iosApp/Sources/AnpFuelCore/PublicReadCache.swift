import Foundation
#if canImport(FoundationNetworking)
import FoundationNetworking
#endif

/// Last successful public reads, scoped by API origin and city, for offline display.
/// Cached rows always carry their save time and source; a community price is
/// shown only until its server `expires_at`, so a stale price is never current.
/// Excluded from backup; private owner data never enters this cache.
/// Zero postfix unwrap operators anywhere in this file.

public struct CachedCommunityItem: Codable, Equatable, Sendable {
    public let observation: CommunityPriceObservation
    public let expiresAtEpochSeconds: Int64

    public init(observation: CommunityPriceObservation, expiresAtEpochSeconds: Int64) {
        self.observation = observation
        self.expiresAtEpochSeconds = expiresAtEpochSeconds
    }

    public func isLive(nowEpochSeconds: Int64) -> Bool {
        expiresAtEpochSeconds > nowEpochSeconds
    }
}

public struct CachedCommunityFeed: Codable, Equatable, Sendable {
    public let origin: String
    public let municipalityCode: String
    public let fuel: FuelProduct
    public let sort: FeedSortOrder?
    public let savedAtEpochSeconds: Int64
    public let items: [CachedCommunityItem]

    /// Items whose server expiry has not passed; expired prices are dropped, never relabelled.
    public func liveObservations(nowEpochSeconds: Int64) -> [CommunityPriceObservation] {
        items.filter { $0.isLive(nowEpochSeconds: nowEpochSeconds) }.map(\.observation)
    }

    public func expiredCount(nowEpochSeconds: Int64) -> Int {
        items.filter { !$0.isLive(nowEpochSeconds: nowEpochSeconds) }.count
    }
}

public struct CachedDirectory: Codable, Equatable, Sendable {
    public let origin: String
    public let municipalityCode: String
    public let savedAtEpochSeconds: Int64
    public let stations: [StationRecord]
}

public struct CachedOfficialPrice: Codable, Equatable, Sendable {
    public let product: FuelProduct
    public let amountMilli: Int64
    public let collectedOn: String
    public let surveyStart: String
    public let surveyEnd: String

    public init(_ price: OfficialStationPrice) {
        product = price.product
        amountMilli = price.amountMilli
        collectedOn = price.collectedOn
        surveyStart = price.surveyStart
        surveyEnd = price.surveyEnd
    }

    public var officialPrice: OfficialStationPrice {
        OfficialStationPrice(
            product: product, amountMilli: amountMilli, collectedOn: collectedOn,
            surveyStart: surveyStart, surveyEnd: surveyEnd
        )
    }
}

public struct CachedStationPrices: Codable, Equatable, Sendable {
    public let origin: String
    public let stationId: String
    public let municipalityCode: String
    public let savedAtEpochSeconds: Int64
    public let prices: [CachedOfficialPrice]
}

public struct PublicReadCacheState: Codable, Equatable, Sendable {
    public var directories: [CachedDirectory]
    public var communityFeeds: [CachedCommunityFeed]
    public var stationPrices: [CachedStationPrices]

    public init(directories: [CachedDirectory] = [], communityFeeds: [CachedCommunityFeed] = [], stationPrices: [CachedStationPrices] = []) {
        self.directories = directories
        self.communityFeeds = communityFeeds
        self.stationPrices = stationPrices
    }

    private enum CodingKeys: String, CodingKey {
        case directories
        case communityFeeds
        case stationPrices
    }

    public init(from decoder: Decoder) throws {
        let values = try decoder.container(keyedBy: CodingKeys.self)
        directories = (try? values.decodeIfPresent([CachedDirectory].self, forKey: .directories)) ?? []
        communityFeeds = (try? values.decodeIfPresent([CachedCommunityFeed].self, forKey: .communityFeeds)) ?? []
        stationPrices = (try? values.decodeIfPresent([CachedStationPrices].self, forKey: .stationPrices)) ?? []
    }
}

public final class PublicReadCache: @unchecked Sendable {
    public static let maxDirectories = 6
    public static let maxCommunityFeeds = 24
    public static let maxStationPrices = 300

    private let fileUrl: URL
    private let lock = NSLock()

    public init(directory: URL = IosLocalStore.defaultDirectory()) {
        try? FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
        fileUrl = directory.appendingPathComponent("public-read-cache-v1.json")
    }

    public func directory(origin: String, municipalityCode: String) -> CachedDirectory? {
        read().directories.first { $0.origin == origin && $0.municipalityCode == municipalityCode }
    }

    public func saveDirectory(origin: String, municipalityCode: String, stations: [StationRecord], now: Date = Date()) {
        mutate { state in
            state.directories.removeAll { $0.origin == origin && $0.municipalityCode == municipalityCode }
            state.directories.insert(CachedDirectory(
                origin: origin, municipalityCode: municipalityCode,
                savedAtEpochSeconds: Int64(now.timeIntervalSince1970), stations: stations
            ), at: 0)
            state.directories = Array(state.directories.prefix(Self.maxDirectories))
        }
    }

    public func communityFeed(
        origin: String, municipalityCode: String, fuel: FuelProduct, sort: FeedSortOrder = .recent
    ) -> CachedCommunityFeed? {
        read().communityFeeds.first {
            $0.origin == origin && $0.municipalityCode == municipalityCode && $0.fuel == fuel && ($0.sort ?? .recent) == sort
        }
    }

    public func saveCommunityFeed(
        origin: String, municipalityCode: String, fuel: FuelProduct, sort: FeedSortOrder = .recent,
        items: [CachedCommunityItem], now: Date = Date()
    ) {
        let nowSeconds = Int64(now.timeIntervalSince1970)
        mutate { state in
            state.communityFeeds.removeAll {
                $0.origin == origin && $0.municipalityCode == municipalityCode && $0.fuel == fuel && ($0.sort ?? .recent) == sort
            }
            state.communityFeeds.insert(CachedCommunityFeed(
                origin: origin, municipalityCode: municipalityCode, fuel: fuel, sort: sort,
                savedAtEpochSeconds: nowSeconds, items: items.filter { $0.isLive(nowEpochSeconds: nowSeconds) }
            ), at: 0)
            state.communityFeeds = Array(state.communityFeeds.prefix(Self.maxCommunityFeeds))
        }
    }

    public func stationPrices(origin: String, stationId: String) -> CachedStationPrices? {
        read().stationPrices.first { $0.origin == origin && $0.stationId == stationId }
    }

    public func saveStationPrices(origin: String, stationId: String, municipalityCode: String, prices: [OfficialStationPrice], now: Date = Date()) {
        mutate { state in
            state.stationPrices.removeAll { $0.origin == origin && $0.stationId == stationId }
            state.stationPrices.insert(CachedStationPrices(
                origin: origin, stationId: stationId, municipalityCode: municipalityCode,
                savedAtEpochSeconds: Int64(now.timeIntervalSince1970), prices: prices.map { CachedOfficialPrice($0) }
            ), at: 0)
            state.stationPrices = Array(state.stationPrices.prefix(Self.maxStationPrices))
        }
    }

    public func clear() {
        lock.lock()
        defer { lock.unlock() }
        try? FileManager.default.removeItem(at: fileUrl)
    }

    public func storedBytes() -> Int64 {
        let attributes = try? FileManager.default.attributesOfItem(atPath: fileUrl.path)
        return (attributes?[.size] as? NSNumber)?.int64Value ?? 0
    }

    private func read() -> PublicReadCacheState {
        lock.lock()
        defer { lock.unlock() }
        return readLocked()
    }

    private func readLocked() -> PublicReadCacheState {
        guard let data = try? Data(contentsOf: fileUrl),
              let state = try? JSONDecoder().decode(PublicReadCacheState.self, from: data) else {
            return PublicReadCacheState()
        }
        return state
    }

    private func mutate(_ change: (inout PublicReadCacheState) -> Void) {
        lock.lock()
        defer { lock.unlock() }
        var state = readLocked()
        change(&state)
        guard let data = try? JSONEncoder().encode(state) else { return }
        try? data.write(to: fileUrl, options: .atomic)
        var url = fileUrl
        var values = URLResourceValues()
        values.isExcludedFromBackup = true
        try? url.setResourceValues(values)
    }
}

private struct CommunityFeedItemsWire: Decodable {
    struct Item: Decodable {
        let stationId: String
        let stationName: String
        let fuelProduct: String
        let amountMilliBrl: Int64
        let unit: String
        let currency: String
        let source: String
        let condition: String
        let updatedAt: String
        let expiresAt: String
        let confirmations: Int
        let contribution: CommunityContributionReference?
    }

    let items: [Item]
    let state: String
    let municipalityCode: String
    let fuelProduct: String
    let sort: String?
    let generatedAt: String?
    let nextCursor: String?
}

public struct CommunityFeedPage: Equatable, Sendable {
    public let items: [CachedCommunityItem]
    public let nextCursor: String?
}

public extension AbastevoHttpClient {
    /// The scoped community feed with each row's server expiry and its public, opaque action reference.
    func fetchCommunityFeedItems(
        state: BrazilianState,
        municipalityCode: String,
        fuel: FuelProduct,
        sort: FeedSortOrder
    ) async throws -> [CachedCommunityItem] {
        try await fetchCommunityFeedPage(state: state, municipalityCode: municipalityCode, fuel: fuel, sort: sort).items
    }

    func fetchCommunityFeedPage(
        state: BrazilianState,
        municipalityCode: String,
        fuel: FuelProduct,
        sort: FeedSortOrder,
        after cursor: String? = nil
    ) async throws -> CommunityFeedPage {
        let sortWire = sort == .cheapest ? "cheapest" : "recent"
        var query = [
            URLQueryItem(name: "state", value: state.rawValue),
            URLQueryItem(name: "municipality_code", value: municipalityCode),
            URLQueryItem(name: "fuel_product", value: fuel.wireCode),
            URLQueryItem(name: "sort", value: sortWire),
            URLQueryItem(name: "limit", value: "20"),
        ]
        if let cursor {
            guard Self.isCommunityFeedCursor(cursor) else { throw HttpClientError.invalidRequest }
            query.append(URLQueryItem(name: "cursor", value: cursor))
        }
        let data = try await publicRead(path: Self.communityFeedPath, query: query)
        return try Self.decodeCommunityFeedPage(data, state: state, municipalityCode: municipalityCode, fuel: fuel, sort: sortWire)
    }

    static func decodeCommunityFeedItems(
        _ data: Data,
        state: BrazilianState,
        municipalityCode: String,
        fuel: FuelProduct
    ) throws -> [CachedCommunityItem] {
        try decodeCommunityFeedPage(data, state: state, municipalityCode: municipalityCode, fuel: fuel).items
    }

    static func decodeCommunityFeedPage(
        _ data: Data,
        state: BrazilianState,
        municipalityCode: String,
        fuel: FuelProduct,
        sort: String? = nil
    ) throws -> CommunityFeedPage {
        let page = try publicDecoder().decode(CommunityFeedItemsWire.self, from: data)
        let generatedAt = page.generatedAt.flatMap(readIsoDate)
        guard page.state == state.rawValue, page.municipalityCode == municipalityCode, page.fuelProduct == fuel.wireCode,
              page.items.count <= 20, page.nextCursor.map(isCommunityFeedCursor) ?? true,
              (sort == nil || page.sort == sort && generatedAt != nil),
              Set(page.items.map(\.stationId)).count == page.items.count else {
            throw HttpClientError.invalidJson
        }
        let items = try page.items.map { row in
            guard UUID(uuidString: row.stationId) != nil, row.source == "COMMUNITY", row.currency == "BRL",
                  row.fuelProduct == fuel.wireCode, row.unit == fuel.unitWire, row.amountMilliBrl > 0,
                  row.confirmations >= 0, let condition = PriceConditionKind(rawValue: row.condition),
                  row.contribution.map({ $0.isValid && condition == .standard }) ?? true,
                  let updatedAt = readIsoDate(row.updatedAt), let expiresAt = readIsoDate(row.expiresAt),
                  generatedAt.map({ updatedAt <= $0 }) ?? true else {
                throw HttpClientError.invalidJson
            }
            let observation = CommunityPriceObservation(
                id: row.stationId,
                stationId: row.stationId,
                stationName: row.stationName,
                municipalityCode: municipalityCode,
                product: fuel,
                amountMilli: row.amountMilliBrl,
                condition: condition,
                confirmations: row.confirmations,
                disputes: 0,
                helpfulCount: 0,
                notHelpfulCount: 0,
                observedAtLabel: row.updatedAt,
                observedEpochSeconds: Int64(updatedAt.timeIntervalSince1970),
                hasPhotoEvidence: false,
                contribution: row.contribution
            )
            return CachedCommunityItem(observation: observation, expiresAtEpochSeconds: Int64(expiresAt.timeIntervalSince1970))
        }
        return CommunityFeedPage(items: items, nextCursor: page.nextCursor)
    }

    private static func isCommunityFeedCursor(_ cursor: String) -> Bool {
        !cursor.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty && cursor.utf8.count <= 4_096
    }
}

import Foundation
#if canImport(FoundationNetworking)
import FoundationNetworking
#endif

/// Bounded gov.br transport and week synchronization.
/// Swift port of Android `AnpListingScraper` (4 MiB listing cap), `AnpFileDownloader`
/// (64 MiB cap enforced while reading, approved final URL after redirects, `.xlsx`
/// name, accepted content types, SHA-256), `DiscoverSurveyWeekCatalogUseCase`,
/// `DownloadSurveyWeekUseCase` and `SyncPriceTablesUseCase` outcomes.
/// Automatic refreshes never use expensive or constrained networks (Android
/// `AutoSyncOnWifiRule` default).
/// Zero postfix unwrap operators anywhere in this file.

public enum AnpSurveyClientError: Error, Equatable {
    case refusedUrl
    case httpStatus(Int)
    case tooLarge
    case empty
    case unsupportedContentType
    case notSpreadsheet
    case transport
    case emptyCatalog
}

public struct AnpDownloadedFile: Sendable {
    public let data: Data
    public let finalUrl: String
    public let contentType: String?
    public let sha256: String

    public var digest: AnpSourceFileDigest {
        AnpSourceFileDigest(url: finalUrl, sha256: sha256, bytes: Int64(data.count))
    }
}

public final class AnpSurveyClient: @unchecked Sendable {
    public static let maxListingBytes = 4 * 1024 * 1024
    public static let maxDownloadBytes = 64 * 1024 * 1024
    public static let acceptedContentTypes: Set<String> = [
        "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
        "application/octet-stream",
        "application/vnd.ms-excel",
        "binary/octet-stream",
    ]

    private let manualSession: URLSession
    private let automaticSession: URLSession

    public init() {
        manualSession = URLSession(configuration: Self.configuration(automatic: false))
        automaticSession = URLSession(configuration: Self.configuration(automatic: true))
    }

    public static func isAcceptedContentType(_ contentType: String?) -> Bool {
        guard let raw = contentType?.trimmingCharacters(in: .whitespacesAndNewlines), !raw.isEmpty else { return true }
        let mime = raw.split(separator: ";", maxSplits: 1, omittingEmptySubsequences: false)
            .first.map { $0.trimmingCharacters(in: .whitespaces).lowercased() } ?? ""
        return acceptedContentTypes.contains(mime)
    }

    /// True when the bytes start with a ZIP local file header (every XLSX does).
    public static func looksLikeSpreadsheet(_ data: Data) -> Bool {
        data.starts(with: [0x50, 0x4B, 0x03, 0x04])
    }

    public func fetchListingHtml(automatic: Bool) async throws -> String {
        guard AnpEndpoints.isApprovedUrl(AnpEndpoints.listingPageUrl), let url = URL(string: AnpEndpoints.listingPageUrl) else {
            throw AnpSurveyClientError.refusedUrl
        }
        let fetched = try await fetch(url, cap: Self.maxListingBytes, automatic: automatic) { _, _ in }
        return String(decoding: fetched.data, as: UTF8.self)
    }

    public func downloadTable(
        _ rawUrl: String,
        automatic: Bool,
        progress: @escaping @Sendable (Int64, Int64?) -> Void
    ) async throws -> AnpDownloadedFile {
        let trimmed = rawUrl.trimmingCharacters(in: .whitespacesAndNewlines)
        guard AnpPriceTableUrl.isPriceTableUrl(trimmed), AnpEndpoints.isApprovedUrl(trimmed),
              let name = AnpPriceTableUrl.safeFileName(trimmed), name.lowercased().hasSuffix(".xlsx"),
              let url = URL(string: trimmed) else {
            throw AnpSurveyClientError.refusedUrl
        }
        let fetched = try await fetch(url, cap: Self.maxDownloadBytes, automatic: automatic, progress: progress)
        guard !fetched.data.isEmpty else { throw AnpSurveyClientError.empty }
        guard Self.isAcceptedContentType(fetched.contentType) else { throw AnpSurveyClientError.unsupportedContentType }
        guard Self.looksLikeSpreadsheet(fetched.data) else { throw AnpSurveyClientError.notSpreadsheet }
        return AnpDownloadedFile(
            data: fetched.data,
            finalUrl: fetched.finalUrl,
            contentType: fetched.contentType,
            sha256: AnpDigest.sha256Hex(fetched.data)
        )
    }

    private struct Fetched {
        let data: Data
        let finalUrl: String
        let contentType: String?
    }

    private func fetch(
        _ url: URL,
        cap: Int,
        automatic: Bool,
        progress: @escaping @Sendable (Int64, Int64?) -> Void
    ) async throws -> Fetched {
        var request = URLRequest(url: url)
        request.httpMethod = "GET"
        let session = automatic ? automaticSession : manualSession
        let opened: (URLSession.AsyncBytes, URLResponse)
        do {
            opened = try await session.bytes(for: request)
        } catch {
            if Task.isCancelled { throw CancellationError() }
            throw AnpSurveyClientError.transport
        }
        let (bytes, response) = opened
        guard let http = response as? HTTPURLResponse else { throw AnpSurveyClientError.transport }
        guard (200..<300).contains(http.statusCode) else { throw AnpSurveyClientError.httpStatus(http.statusCode) }
        guard let finalUrl = http.url?.absoluteString, AnpEndpoints.isApprovedUrl(finalUrl) else {
            throw AnpSurveyClientError.refusedUrl
        }
        let declared = http.expectedContentLength
        if declared > Int64(cap) { throw AnpSurveyClientError.tooLarge }
        let expected: Int64? = declared >= 0 ? declared : nil
        var data = Data()
        if let expected, expected > 0 { data.reserveCapacity(Int(expected)) }
        let chunkSize = 64 * 1024
        var chunk: [UInt8] = []
        chunk.reserveCapacity(chunkSize)
        do {
            for try await byte in bytes {
                chunk.append(byte)
                if chunk.count == chunkSize {
                    data.append(contentsOf: chunk)
                    chunk.removeAll(keepingCapacity: true)
                    if data.count > cap { throw AnpSurveyClientError.tooLarge }
                    progress(Int64(data.count), expected)
                }
            }
        } catch let error as AnpSurveyClientError {
            throw error
        } catch {
            if Task.isCancelled { throw CancellationError() }
            throw AnpSurveyClientError.transport
        }
        data.append(contentsOf: chunk)
        if data.count > cap { throw AnpSurveyClientError.tooLarge }
        progress(Int64(data.count), expected)
        return Fetched(data: data, finalUrl: finalUrl, contentType: http.value(forHTTPHeaderField: "Content-Type"))
    }

    private static func configuration(automatic: Bool) -> URLSessionConfiguration {
        let configuration = URLSessionConfiguration.ephemeral
        configuration.timeoutIntervalForRequest = 30
        configuration.timeoutIntervalForResource = 600
        configuration.httpCookieStorage = nil
        configuration.httpShouldSetCookies = false
        configuration.urlCache = nil
        configuration.allowsExpensiveNetworkAccess = !automatic
        configuration.allowsConstrainedNetworkAccess = !automatic
        return configuration
    }
}

public struct AnpDownloadProgress: Equatable, Sendable {
    public enum Stage: String, Equatable, Sendable {
        case summary
        case stations
        case importing
        case saving
    }

    public let stage: Stage
    public let receivedBytes: Int64
    public let expectedBytes: Int64?

    public init(stage: Stage, receivedBytes: Int64 = 0, expectedBytes: Int64? = nil) {
        self.stage = stage
        self.receivedBytes = receivedBytes
        self.expectedBytes = expectedBytes
    }

    public var fraction: Double? {
        guard let expectedBytes, expectedBytes > 0 else { return nil }
        return min(1.0, max(0.0, Double(receivedBytes) / Double(expectedBytes)))
    }
}

public enum AnpSyncOutcome: Equatable, Sendable {
    /// The week was downloaded and imported; station detail may have failed (Android PARTIAL).
    case downloaded(AnpWeekManifest, stationsFailed: Bool)
    /// The week was already stored for this scope (Android NO_NEW_DATA).
    case alreadyDownloaded(AnpWeekManifest)

    public var manifest: AnpWeekManifest {
        switch self {
        case .downloaded(let manifest, _): return manifest
        case .alreadyDownloaded(let manifest): return manifest
        }
    }
}

public final class AnpSurveyService: @unchecked Sendable {
    public let store: AnpDownloadStore
    private let client: AnpSurveyClient

    public init(store: AnpDownloadStore = AnpDownloadStore(), client: AnpSurveyClient = AnpSurveyClient()) {
        self.store = store
        self.client = client
    }

    /// UC-009: newest-first listing weeks that expose both downloads.
    public func discoverCatalog(automatic: Bool, now: Date = Date()) async throws -> [AnpSurveyCatalogEntry] {
        let html = try await client.fetchListingHtml(automatic: automatic)
        let catalog = AnpListingParser.parseCatalog(html: html)
        store.mutateState { $0.lastCatalogCheckEpochSeconds = Int64(now.timeIntervalSince1970) }
        guard !catalog.isEmpty else { throw AnpSurveyClientError.emptyCatalog }
        return catalog
    }

    /// True when `entry` is already stored with station detail for `scope`.
    public func isCurrent(_ entry: AnpSurveyCatalogEntry, scope: BrazilianState?) -> AnpWeekManifest? {
        guard let existing = store.manifest(weekId: entry.week.id) else { return nil }
        guard let scope else { return existing }
        return existing.hasStationData && existing.stationScope == scope ? existing : nil
    }

    /// Downloads, verifies and imports one week. A station-detail failure keeps the
    /// imported summary (Android PARTIAL); a summary failure stores nothing.
    public func sync(
        entry: AnpSurveyCatalogEntry,
        scope: BrazilianState?,
        automatic: Bool,
        progress: @escaping @Sendable (AnpDownloadProgress) -> Void
    ) async throws -> AnpSyncOutcome {
        if let existing = isCurrent(entry, scope: scope) { return .alreadyDownloaded(existing) }
        progress(AnpDownloadProgress(stage: .summary))
        let summaryFile = try await client.downloadTable(entry.summaryUrl, automatic: automatic) { received, expected in
            progress(AnpDownloadProgress(stage: .summary, receivedBytes: received, expectedBytes: expected))
        }
        try Task.checkCancellation()
        progress(AnpDownloadProgress(stage: .importing, receivedBytes: Int64(summaryFile.data.count)))
        let week = entry.week
        let summaryData = summaryFile.data
        let summary = try await Task.detached(priority: .utility) {
            try AnpWeekImporter.importSummary(xlsx: summaryData, expectedWeek: week)
        }.value
        var stationImport: AnpStationImport?
        var stationDigest: AnpSourceFileDigest?
        var stationsFailed = false
        if let scope {
            do {
                let stationFile = try await client.downloadTable(entry.stationUrl, automatic: automatic) { received, expected in
                    progress(AnpDownloadProgress(stage: .stations, receivedBytes: received, expectedBytes: expected))
                }
                try Task.checkCancellation()
                progress(AnpDownloadProgress(stage: .importing, receivedBytes: Int64(stationFile.data.count)))
                let stationData = stationFile.data
                stationImport = try await Task.detached(priority: .utility) {
                    try AnpWeekImporter.importStations(xlsx: stationData, scope: scope)
                }.value
                stationDigest = stationFile.digest
            } catch {
                if Task.isCancelled { throw CancellationError() }
                stationsFailed = true
            }
        }
        try Task.checkCancellation()
        progress(AnpDownloadProgress(stage: .saving))
        let manifest = try store.save(
            entry: entry,
            summary: summary,
            summarySource: summaryFile.digest,
            stations: stationImport,
            stationSource: stationDigest,
            scope: stationImport == nil ? nil : scope
        )
        return .downloaded(manifest, stationsFailed: stationsFailed)
    }
}

/// One weekly ANP observation of a directory station (backend `/v1/stations/{id}/official-prices`).
public struct OfficialPriceHistoryEntry: Equatable, Identifiable, Sendable {
    public let revisionId: String
    public let product: FuelProduct
    public let amountMilli: Int64
    public let collectedOn: String
    public let surveyWeek: AnpSurveyWeekRef
    public let sourceUrl: String

    public var id: String { "\(revisionId)-\(product.rawValue)-\(surveyWeek.id)" }
}

private struct OfficialHistoryWire: Decodable {
    struct Entry: Decodable {
        struct Week: Decodable {
            let start: String
            let end: String
        }

        struct Source: Decodable {
            let kind: String
            let url: String
        }

        let revisionId: String
        let surveyWeek: Week
        let collectedOn: String
        let fuelProduct: String
        let unit: String
        let amountMilliBrl: Int64
        let currency: String
        let source: Source
    }

    let items: [Entry]?
}

public extension AbastevoHttpClient {
    /// Newest-first official weekly prices of one station and fuel.
    func fetchOfficialPriceHistory(stationId: String, fuel: FuelProduct, limit: Int = 12) async throws -> [OfficialPriceHistoryEntry] {
        guard UUID(uuidString: stationId) != nil, (1...100).contains(limit) else { throw HttpClientError.invalidRequest }
        let query = [
            URLQueryItem(name: "fuel_product", value: fuel.wireCode),
            URLQueryItem(name: "limit", value: String(limit)),
        ]
        let data = try await publicRead(path: "/v1/stations/\(stationId)/official-prices", query: query)
        return try Self.decodeOfficialHistory(data, fuel: fuel)
    }

    static func decodeOfficialHistory(_ data: Data, fuel: FuelProduct) throws -> [OfficialPriceHistoryEntry] {
        let page = try publicDecoder().decode(OfficialHistoryWire.self, from: data)
        let entries = try (page.items ?? []).map { row -> OfficialPriceHistoryEntry in
            guard UUID(uuidString: row.revisionId) != nil, row.fuelProduct == fuel.wireCode, row.unit == fuel.unitWire,
                  row.currency == "BRL", row.source.kind == "ANP", row.amountMilliBrl > 0,
                  let week = AnpSurveyWeekRef(startIso: row.surveyWeek.start, endIso: row.surveyWeek.end),
                  AnpCalendar.parseIsoDay(row.collectedOn) != nil,
                  week.startIso <= row.collectedOn, row.collectedOn <= week.endIso else {
                throw HttpClientError.invalidJson
            }
            return OfficialPriceHistoryEntry(
                revisionId: row.revisionId, product: fuel, amountMilli: row.amountMilliBrl,
                collectedOn: row.collectedOn, surveyWeek: week, sourceUrl: row.source.url
            )
        }
        return entries.sorted { $0.surveyWeek > $1.surveyWeek }
    }
}

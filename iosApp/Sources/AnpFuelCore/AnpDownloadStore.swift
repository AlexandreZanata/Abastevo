import Foundation
import CryptoKit

/// Downloaded ANP weeks in Application Support, excluded from backup.
/// Each week keeps a manifest with the SHA-256 and size of the two source
/// spreadsheets and of the two stored row files. A stored file whose size or
/// digest no longer matches its manifest is deleted and reported as missing.
/// Zero postfix unwrap operators anywhere in this file.

public enum AnpDigest {
    public static func sha256Hex(_ data: Data) -> String {
        SHA256.hash(data: data).map { byte in
            let text = String(byte, radix: 16)
            return text.count == 1 ? "0" + text : text
        }.joined()
    }
}

/// Size and digest of one downloaded source file.
public struct AnpSourceFileDigest: Codable, Equatable, Sendable {
    public let url: String
    public let sha256: String
    public let bytes: Int64

    public init(url: String, sha256: String, bytes: Int64) {
        self.url = url
        self.sha256 = sha256
        self.bytes = bytes
    }
}

public struct AnpWeekManifest: Codable, Equatable, Identifiable, Sendable {
    public let week: AnpSurveyWeekRef
    public let summarySource: AnpSourceFileDigest
    public let stationSource: AnpSourceFileDigest?
    public let stationScope: BrazilianState?
    public let averageRowCount: Int
    public let stationRowCount: Int
    public let quarantinedRowCount: Int
    public let averagesFileSha256: String
    public let averagesFileBytes: Int64
    public let stationsFileSha256: String?
    public let stationsFileBytes: Int64?
    public let downloadedAtEpochSeconds: Int64
    public let publishedOnIso: String?
    public let operationalNote: String?

    public var id: String { week.id }

    public var hasStationData: Bool { stationScope != nil && stationsFileSha256 != nil }

    /// Bytes transferred from gov.br for this week.
    public var downloadedBytes: Int64 { summarySource.bytes + (stationSource?.bytes ?? 0) }

    /// Bytes kept on the device for this week.
    public var storedBytes: Int64 { averagesFileBytes + (stationsFileBytes ?? 0) }
}

public struct AnpManualVerification: Codable, Equatable, Sendable {
    public let weekId: String
    public let verifiedAtEpochSeconds: Int64

    public init(weekId: String, verifiedAtEpochSeconds: Int64) {
        self.weekId = weekId
        self.verifiedAtEpochSeconds = verifiedAtEpochSeconds
    }
}

public struct AnpStoreState: Codable, Equatable, Sendable {
    public var manifests: [AnpWeekManifest]
    public var lastCatalogCheckEpochSeconds: Int64?
    public var verification: AnpManualVerification?
    /// Price-drop alert key -> survey week id already notified (P23-T01 duplicate suppression).
    public var notifiedWeekByAlert: [String: String]

    public init(
        manifests: [AnpWeekManifest] = [],
        lastCatalogCheckEpochSeconds: Int64? = nil,
        verification: AnpManualVerification? = nil,
        notifiedWeekByAlert: [String: String] = [:]
    ) {
        self.manifests = manifests
        self.lastCatalogCheckEpochSeconds = lastCatalogCheckEpochSeconds
        self.verification = verification
        self.notifiedWeekByAlert = notifiedWeekByAlert
    }

    private enum CodingKeys: String, CodingKey {
        case manifests
        case lastCatalogCheckEpochSeconds
        case verification
        case notifiedWeekByAlert
    }

    public init(from decoder: Decoder) throws {
        let values = try decoder.container(keyedBy: CodingKeys.self)
        manifests = try values.decodeIfPresent([AnpWeekManifest].self, forKey: .manifests) ?? []
        lastCatalogCheckEpochSeconds = try values.decodeIfPresent(Int64.self, forKey: .lastCatalogCheckEpochSeconds)
        verification = try values.decodeIfPresent(AnpManualVerification.self, forKey: .verification)
        notifiedWeekByAlert = try values.decodeIfPresent([String: String].self, forKey: .notifiedWeekByAlert) ?? [:]
    }
}

public enum AnpStoreError: Error, Equatable {
    case writeFailed
}

public final class AnpDownloadStore: @unchecked Sendable {
    public let directory: URL
    private let lock = NSLock()
    private let stateFileName = "anp-state-v1.json"

    public init(directory: URL = AnpDownloadStore.defaultDirectory()) {
        self.directory = directory
        try? FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
        Self.excludeFromBackup(directory)
    }

    public static func defaultDirectory() -> URL {
        IosLocalStore.defaultDirectory().appendingPathComponent("anp-weeks", isDirectory: true)
    }

    public func loadState() -> AnpStoreState {
        lock.lock()
        defer { lock.unlock() }
        return readStateLocked()
    }

    @discardableResult
    public func mutateState(_ change: (inout AnpStoreState) -> Void) -> AnpStoreState {
        lock.lock()
        defer { lock.unlock() }
        var state = readStateLocked()
        change(&state)
        writeStateLocked(state)
        return state
    }

    /// Downloaded weeks, newest first.
    public func manifests() -> [AnpWeekManifest] {
        loadState().manifests.sorted { $0.week > $1.week }
    }

    public func manifest(weekId: String) -> AnpWeekManifest? {
        loadState().manifests.first { $0.week.id == weekId }
    }

    /// Persists one imported week, replacing any previous download of the same week.
    public func save(
        entry: AnpSurveyCatalogEntry,
        summary: AnpSummaryImport,
        summarySource: AnpSourceFileDigest,
        stations: AnpStationImport?,
        stationSource: AnpSourceFileDigest?,
        scope: BrazilianState?,
        now: Date = Date()
    ) throws -> AnpWeekManifest {
        let encoder = JSONEncoder()
        guard let averagesData = try? encoder.encode(summary.averages) else { throw AnpStoreError.writeFailed }
        let stationsData: Data?
        if let stations, stationSource != nil, scope != nil {
            guard let encoded = try? encoder.encode(stations.stations) else { throw AnpStoreError.writeFailed }
            stationsData = encoded
        } else {
            stationsData = nil
        }
        lock.lock()
        defer { lock.unlock() }
        let weekId = entry.week.id
        removeFilesLocked(weekId: weekId)
        do {
            try write(averagesData, to: averagesUrl(weekId))
            if let stationsData { try write(stationsData, to: stationsUrl(weekId)) }
        } catch {
            removeFilesLocked(weekId: weekId)
            throw AnpStoreError.writeFailed
        }
        let manifest = AnpWeekManifest(
            week: entry.week,
            summarySource: summarySource,
            stationSource: stationsData == nil ? nil : stationSource,
            stationScope: stationsData == nil ? nil : scope,
            averageRowCount: summary.averages.count,
            stationRowCount: stationsData == nil ? 0 : (stations?.stations.count ?? 0),
            quarantinedRowCount: summary.quarantinedRows + (stationsData == nil ? 0 : (stations?.quarantinedRows ?? 0)),
            averagesFileSha256: AnpDigest.sha256Hex(averagesData),
            averagesFileBytes: Int64(averagesData.count),
            stationsFileSha256: stationsData.map { AnpDigest.sha256Hex($0) },
            stationsFileBytes: stationsData.map { Int64($0.count) },
            downloadedAtEpochSeconds: Int64(now.timeIntervalSince1970),
            publishedOnIso: entry.publishedOnIso,
            operationalNote: entry.operationalNote
        )
        var state = readStateLocked()
        state.manifests.removeAll { $0.week.id == weekId }
        state.manifests.append(manifest)
        writeStateLocked(state)
        return manifest
    }

    /// Verified municipality averages of a week, or nil when missing or corrupt.
    public func averages(weekId: String) -> [AnpMunicipalityAverage]? {
        lock.lock()
        defer { lock.unlock() }
        guard let manifest = readStateLocked().manifests.first(where: { $0.week.id == weekId }) else { return nil }
        guard let data = verifiedLocked(averagesUrl(weekId), sha256: manifest.averagesFileSha256, bytes: manifest.averagesFileBytes),
              let rows = try? JSONDecoder().decode([AnpMunicipalityAverage].self, from: data) else {
            dropLocked(weekId: weekId)
            return nil
        }
        return rows
    }

    /// Verified station prices of a week (its downloaded state only), or nil.
    public func stations(weekId: String) -> [AnpStationWeekPrice]? {
        lock.lock()
        defer { lock.unlock() }
        guard let manifest = readStateLocked().manifests.first(where: { $0.week.id == weekId }),
              let sha = manifest.stationsFileSha256, let bytes = manifest.stationsFileBytes else { return nil }
        guard let data = verifiedLocked(stationsUrl(weekId), sha256: sha, bytes: bytes),
              let rows = try? JSONDecoder().decode([AnpStationWeekPrice].self, from: data) else {
            dropLocked(weekId: weekId)
            return nil
        }
        return rows
    }

    public func delete(weekId: String) {
        lock.lock()
        defer { lock.unlock() }
        dropLocked(weekId: weekId)
    }

    public func deleteAllWeeks() {
        lock.lock()
        defer { lock.unlock() }
        var state = readStateLocked()
        for manifest in state.manifests { removeFilesLocked(weekId: manifest.week.id) }
        state.manifests.removeAll()
        state.verification = nil
        writeStateLocked(state)
    }

    public func storedBytes() -> Int64 {
        manifests().reduce(0) { $0 + $1.storedBytes }
    }

    // MARK: - Locked helpers

    private func readStateLocked() -> AnpStoreState {
        guard let data = try? Data(contentsOf: stateUrl()),
              let state = try? JSONDecoder().decode(AnpStoreState.self, from: data) else {
            return AnpStoreState()
        }
        return state
    }

    private func writeStateLocked(_ state: AnpStoreState) {
        guard let data = try? JSONEncoder().encode(state) else { return }
        try? write(data, to: stateUrl())
    }

    private func verifiedLocked(_ url: URL, sha256: String, bytes: Int64) -> Data? {
        guard let data = try? Data(contentsOf: url), Int64(data.count) == bytes,
              AnpDigest.sha256Hex(data) == sha256 else {
            return nil
        }
        return data
    }

    private func dropLocked(weekId: String) {
        removeFilesLocked(weekId: weekId)
        var state = readStateLocked()
        state.manifests.removeAll { $0.week.id == weekId }
        if state.verification?.weekId == weekId { state.verification = nil }
        writeStateLocked(state)
    }

    private func removeFilesLocked(weekId: String) {
        try? FileManager.default.removeItem(at: averagesUrl(weekId))
        try? FileManager.default.removeItem(at: stationsUrl(weekId))
    }

    private func write(_ data: Data, to url: URL) throws {
        try data.write(to: url, options: .atomic)
        Self.excludeFromBackup(url)
    }

    private func stateUrl() -> URL { directory.appendingPathComponent(stateFileName) }
    private func averagesUrl(_ weekId: String) -> URL { directory.appendingPathComponent("\(weekId).averages.json") }
    private func stationsUrl(_ weekId: String) -> URL { directory.appendingPathComponent("\(weekId).stations.json") }

    private static func excludeFromBackup(_ url: URL) {
        var mutableUrl = url
        var values = URLResourceValues()
        values.isExcludedFromBackup = true
        try? mutableUrl.setResourceValues(values)
    }
}

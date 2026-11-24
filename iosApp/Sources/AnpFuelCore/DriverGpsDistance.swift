import CryptoKit
import Foundation

public enum DriverShiftDistanceBasis: String, CaseIterable, Identifiable, Sendable {
    case odometer = "ODOMETER"
    case gpsEstimate = "GPS_ESTIMATE"

    public var id: String { rawValue }
}

public struct DriverGpsScope: Equatable, Sendable {
    public let accountId: String?
    public let origin: String

    public init(accountId: String?, origin: String) {
        self.accountId = accountId
        self.origin = origin
    }

    fileprivate var storageKey: String {
        let owner = accountId.map { "account:" + $0 } ?? "local"
        return Self.digest(owner + "\u{0}" + origin)
    }

    fileprivate static func digest(_ value: String) -> String {
        SHA256.hash(data: Data(value.utf8)).map { String(format: "%02x", $0) }.joined()
    }
}

public enum DriverGpsSampleResult: Equatable, Sendable {
    case rejected
    case anchor
    case segment
}

/// Accumulates accepted segments. It consumes no coordinates and cannot request a location fix.
public struct DriverGpsAccumulator: Equatable, Sendable {
    public static let maximumDistanceMetres = PremiumEnergyCalculator.maxDistanceMetres
    private static let maximumAge = Double(NavigationGuidancePolicy.maxFixAgeNanos) / 1_000_000_000
    private var lastAccepted: TimeInterval?
    private var metres: Double?
    private var minimumTimestamp: TimeInterval?

    public init() {}

    public var distanceMetres: Int64? { metres.map { Int64($0.rounded(.down)) } }

    /// Android's runtime distance remains an incomplete estimate even after an orderly stop.
    public var hasGap: Bool { true }

    public mutating func resetAnchor() {
        lastAccepted = nil
    }

    public mutating func append(
        accuracyMetres: Double, sampledAt: TimeInterval, receivedAt: TimeInterval,
        simulated: Bool, segmentMetres: Double?
    ) -> DriverGpsSampleResult {
        let age = receivedAt - sampledAt
        guard !simulated, (0...NavigationGuidancePolicy.maxAccuracyMeters).contains(accuracyMetres),
              (0...Self.maximumAge).contains(age),
              minimumTimestamp.map({ sampledAt > $0 }) ?? true else { return .rejected }
        if let lastAccepted, sampledAt - lastAccepted <= Self.maximumAge {
            guard let segmentMetres, segmentMetres.isFinite, segmentMetres >= 0 else { return .rejected }
            let total = (metres ?? 0) + segmentMetres
            guard total <= Double(Self.maximumDistanceMetres) else { return .rejected }
            metres = total
            self.lastAccepted = sampledAt
            minimumTimestamp = sampledAt
            return .segment
        }
        lastAccepted = sampledAt
        minimumTimestamp = sampledAt
        return .anchor
    }
}

public struct DriverGpsCheckpoint: Codable, Equatable, Sendable {
    public let recordingId: UUID
    public let stopped: Bool
    public let metres: Int64?

    /// A locally interrupted checkpoint never supplies a distance or restarts collection.
    public var acknowledgedMetres: Int64? { stopped ? metres : nil }
    public var hasGap: Bool { true }
}

public enum DriverGpsStoreError: Error, Equatable, Sendable {
    case unavailable
    case wrongRecording
}

/// Stores only an incomplete marker or a user-stopped integer metre estimate.
public final class DriverGpsCheckpointStore: @unchecked Sendable {
    private let directory: URL
    private let lock = NSLock()

    public init(directory: URL? = nil) {
        self.directory = (directory ?? IosLocalStore.defaultDirectory())
            .appendingPathComponent("driver-gps-v1", isDirectory: true)
    }

    public func load(scope: DriverGpsScope, shiftId: String) throws -> DriverGpsCheckpoint? {
        lock.lock()
        defer { lock.unlock() }
        return try readUnlocked(path(scope: scope, shiftId: shiftId))
    }

    public func begin(scope: DriverGpsScope, shiftId: String, recordingId: UUID) throws {
        lock.lock()
        defer { lock.unlock() }
        let file = path(scope: scope, shiftId: shiftId)
        if try readUnlocked(file)?.recordingId == recordingId { return }
        try writeUnlocked(DriverGpsCheckpoint(recordingId: recordingId, stopped: false, metres: nil), to: file)
    }

    @discardableResult
    public func finish(scope: DriverGpsScope, shiftId: String, recordingId: UUID, metres: Int64?) throws -> DriverGpsCheckpoint {
        lock.lock()
        defer { lock.unlock() }
        let file = path(scope: scope, shiftId: shiftId)
        guard let existing = try readUnlocked(file), existing.recordingId == recordingId else {
            throw DriverGpsStoreError.wrongRecording
        }
        if existing.stopped { return existing }
        guard metres.map({ (0...DriverGpsAccumulator.maximumDistanceMetres).contains($0) }) ?? true else {
            throw DriverGpsStoreError.unavailable
        }
        let stopped = DriverGpsCheckpoint(recordingId: recordingId, stopped: true, metres: metres)
        try writeUnlocked(stopped, to: file)
        return stopped
    }

    public func remove(scope: DriverGpsScope, shiftId: String) throws {
        lock.lock()
        defer { lock.unlock() }
        try removeUnlocked(path(scope: scope, shiftId: shiftId))
    }

    public func purge() throws {
        lock.lock()
        defer { lock.unlock() }
        try removeUnlocked(directory)
    }

    private func path(scope: DriverGpsScope, shiftId: String) -> URL {
        directory.appendingPathComponent(scope.storageKey, isDirectory: true)
            .appendingPathComponent(DriverGpsScope.digest(shiftId) + ".json")
    }

    private func readUnlocked(_ file: URL) throws -> DriverGpsCheckpoint? {
        do {
            guard let size = try file.resourceValues(forKeys: [.fileSizeKey]).fileSize, size <= 1024 else {
                throw DriverGpsStoreError.unavailable
            }
            let result = try JSONDecoder().decode(DriverGpsCheckpoint.self, from: Data(contentsOf: file))
            guard result.stopped || result.metres == nil,
                  result.metres.map({ (0...DriverGpsAccumulator.maximumDistanceMetres).contains($0) }) ?? true else {
                throw DriverGpsStoreError.unavailable
            }
            return result
        } catch let error as CocoaError where error.code == .fileNoSuchFile || error.code == .fileReadNoSuchFile {
            return nil
        } catch {
            throw DriverGpsStoreError.unavailable
        }
    }

    private func writeUnlocked(_ state: DriverGpsCheckpoint, to file: URL) throws {
        do {
            try FileManager.default.createDirectory(at: file.deletingLastPathComponent(), withIntermediateDirectories: true)
            var folder = directory
            var exclusion = URLResourceValues()
            exclusion.isExcludedFromBackup = true
            try folder.setResourceValues(exclusion)
            let bytes = try JSONEncoder().encode(state)
            #if os(iOS)
            try bytes.write(to: file, options: [.atomic, .completeFileProtection])
            #else
            try bytes.write(to: file, options: .atomic)
            #endif
        } catch {
            throw DriverGpsStoreError.unavailable
        }
    }

    private func removeUnlocked(_ url: URL) throws {
        do {
            try FileManager.default.removeItem(at: url)
        } catch let error as CocoaError where error.code == .fileNoSuchFile {
            return
        } catch {
            throw DriverGpsStoreError.unavailable
        }
    }
}

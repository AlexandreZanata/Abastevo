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

    /// True once the recording dropped the distance between two fixes: a gap longer than the
    /// fix age limit or a provider loss after an accepted fix. The missing metres are never filled.
    public private(set) var hasGap = false

    public mutating func resetAnchor() {
        if lastAccepted != nil { hasGap = true }
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
        if let lastAccepted {
            if sampledAt - lastAccepted <= Self.maximumAge {
                guard let segmentMetres, segmentMetres.isFinite, segmentMetres >= 0 else { return .rejected }
                let total = (metres ?? 0) + segmentMetres
                guard total <= Double(Self.maximumDistanceMetres) else { return .rejected }
                metres = total
                self.lastAccepted = sampledAt
                minimumTimestamp = sampledAt
                return .segment
            }
            hasGap = true
        }
        lastAccepted = sampledAt
        minimumTimestamp = sampledAt
        return .anchor
    }
}

/// A shift's GPS estimate in whole metres and whether it is known to be incomplete.
public struct DriverGpsShiftEstimate: Equatable, Sendable {
    public let metres: Int64
    public let incomplete: Bool

    public init(metres: Int64, incomplete: Bool) {
        self.metres = metres
        self.incomplete = incomplete
    }
}

/// One shift's GPS state: integer metres and coverage flags only, never coordinates or a trace.
public struct DriverGpsCheckpoint: Codable, Equatable, Sendable {
    public static let maximumRecordings = 1_000

    public let recordingId: UUID
    public let stopped: Bool
    /// Metres of the current recording, present only after the person stops it.
    public let metres: Int64?
    /// Sum of the earlier stopped recordings of the same shift, when any produced metres.
    public let priorMetres: Int64?
    /// Earlier recordings of the same shift, stopped or interrupted. Absent in older files.
    public let priorRecordings: Int?
    /// A dropped gap, an interrupted recording or a recording without metres. Older files
    /// lack the field, which counts as incomplete.
    public let gap: Bool?

    public init(
        recordingId: UUID, stopped: Bool, metres: Int64?, priorMetres: Int64? = nil,
        priorRecordings: Int = 0, gap: Bool = false
    ) {
        self.recordingId = recordingId
        self.stopped = stopped
        self.metres = metres
        self.priorMetres = priorMetres
        self.priorRecordings = priorRecordings
        self.gap = gap
    }

    /// Stopped recordings of the shift, summed exactly. A recording that is still running or was
    /// interrupted supplies nothing, so an interrupted checkpoint never yields a distance.
    public var acknowledgedMetres: Int64? {
        guard stopped else { return nil }
        return Self.combined(priorMetres, metres)
    }

    /// A stop and restart, a dropped gap or a recording without metres leaves part of the shift uncovered.
    public var isIncomplete: Bool {
        (gap ?? true) || (priorRecordings ?? 0) > 0 || (stopped && metres == nil)
    }

    public var shiftEstimate: DriverGpsShiftEstimate? {
        acknowledgedMetres.map { DriverGpsShiftEstimate(metres: $0, incomplete: isIncomplete) }
    }

    /// Exact sum within the distance bound. A missing part adds nothing; both missing stays unknown.
    public static func combined(_ first: Int64?, _ second: Int64?) -> Int64? {
        guard let first else { return second }
        guard let second else { return first }
        let (total, overflow) = first.addingReportingOverflow(second)
        return overflow || total > DriverGpsAccumulator.maximumDistanceMetres ? nil : total
    }

    fileprivate var isValid: Bool {
        let bounds = 0...DriverGpsAccumulator.maximumDistanceMetres
        return (stopped || metres == nil)
            && (metres.map { bounds.contains($0) } ?? true)
            && (priorMetres.map { bounds.contains($0) } ?? true)
            && (priorRecordings.map { (0...Self.maximumRecordings).contains($0) } ?? true)
    }
}

public enum DriverGpsStoreError: Error, Equatable, Sendable {
    case unavailable
    case wrongRecording
}

/// Stores only the running marker, stopped integer metres and coverage flags of each shift.
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

    /// Starts a recording. An earlier recording of the same shift is kept: its stopped metres
    /// carry forward and the restart marks the shift's estimate incomplete.
    @discardableResult
    public func begin(scope: DriverGpsScope, shiftId: String, recordingId: UUID) throws -> DriverGpsCheckpoint {
        lock.lock()
        defer { lock.unlock() }
        let file = path(scope: scope, shiftId: shiftId)
        let existing = try readUnlocked(file)
        if let existing, existing.recordingId == recordingId { return existing }
        let next: DriverGpsCheckpoint
        if let existing {
            let earlier = existing.priorRecordings ?? 0
            guard earlier < DriverGpsCheckpoint.maximumRecordings else { throw DriverGpsStoreError.unavailable }
            // An interrupted recording contributes no metres; only its stopped predecessors carry
            // forward. A sum beyond the distance bound is dropped rather than clamped.
            let carried = existing.stopped ? existing.acknowledgedMetres : existing.priorMetres
            let lost = !existing.stopped || existing.metres == nil || (carried == nil && existing.priorMetres != nil)
            next = DriverGpsCheckpoint(
                recordingId: recordingId, stopped: false, metres: nil, priorMetres: carried,
                priorRecordings: earlier + 1, gap: (existing.gap ?? true) || lost
            )
        } else {
            next = DriverGpsCheckpoint(recordingId: recordingId, stopped: false, metres: nil)
        }
        try writeUnlocked(next, to: file, origin: scope.origin)
        return next
    }

    /// Stops the person's own running recording. `droppedGap` reports the accumulator's gaps.
    @discardableResult
    public func finish(
        scope: DriverGpsScope, shiftId: String, recordingId: UUID, metres: Int64?, droppedGap: Bool = false
    ) throws -> DriverGpsCheckpoint {
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
        let stopped = DriverGpsCheckpoint(
            recordingId: recordingId, stopped: true, metres: metres, priorMetres: existing.priorMetres,
            priorRecordings: existing.priorRecordings ?? 0, gap: (existing.gap ?? true) || droppedGap
        )
        try writeUnlocked(stopped, to: file, origin: scope.origin)
        return stopped
    }

    public func remove(scope: DriverGpsScope, shiftId: String) throws {
        lock.lock()
        defer { lock.unlock() }
        try removeUnlocked(path(scope: scope, shiftId: shiftId))
    }

    /// Removes one owner's checkpoints at one origin (logout or account deletion). Other owners,
    /// including the device's guest, keep theirs.
    public func purge(scope: DriverGpsScope) throws {
        lock.lock()
        defer { lock.unlock() }
        try removeUnlocked(scopeDirectory(scope))
    }

    /// Removes every owner's checkpoints at one origin, the guest's included (origin switch).
    public func purge(origin: String) throws {
        lock.lock()
        defer { lock.unlock() }
        do {
            try LocalScopeOriginTag.purge(
                origin: origin, in: directory,
                alsoRemoving: [scopeDirectory(DriverGpsScope(accountId: nil, origin: origin))]
            )
        } catch {
            throw DriverGpsStoreError.unavailable
        }
    }

    private func scopeDirectory(_ scope: DriverGpsScope) -> URL {
        directory.appendingPathComponent(scope.storageKey, isDirectory: true)
    }

    private func path(scope: DriverGpsScope, shiftId: String) -> URL {
        scopeDirectory(scope).appendingPathComponent(DriverGpsScope.digest(shiftId) + ".json")
    }

    private func readUnlocked(_ file: URL) throws -> DriverGpsCheckpoint? {
        do {
            guard let size = try file.resourceValues(forKeys: [.fileSizeKey]).fileSize, size <= 1024 else {
                throw DriverGpsStoreError.unavailable
            }
            let result = try JSONDecoder().decode(DriverGpsCheckpoint.self, from: Data(contentsOf: file))
            guard result.isValid else { throw DriverGpsStoreError.unavailable }
            return result
        } catch let error as CocoaError where error.code == .fileNoSuchFile || error.code == .fileReadNoSuchFile {
            return nil
        } catch {
            throw DriverGpsStoreError.unavailable
        }
    }

    private func writeUnlocked(_ state: DriverGpsCheckpoint, to file: URL, origin: String) throws {
        do {
            try FileManager.default.createDirectory(at: file.deletingLastPathComponent(), withIntermediateDirectories: true)
            var folder = directory
            var exclusion = URLResourceValues()
            exclusion.isExcludedFromBackup = true
            try folder.setResourceValues(exclusion)
            try LocalScopeOriginTag.write(origin: origin, in: file.deletingLastPathComponent())
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

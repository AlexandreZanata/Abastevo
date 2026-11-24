import Foundation

public struct DriverOutboxScope: Codable, Hashable, Sendable {
    public let accountId: String
    public let origin: String

    public init?(accountId: String, origin: String) {
        guard DriverAccountingWire.validID(accountId), !origin.isEmpty,
              origin.utf8.count <= 256, !origin.contains("\n") else { return nil }
        self.accountId = accountId
        self.origin = origin
    }

    var filename: String {
        DriverAccountingWire.hash(Data("\(accountId)\n\(origin)".utf8)) + ".json"
    }
}

public enum DriverOutboxState: String, Codable, Sendable {
    case pending = "PENDING"
    case synced = "SYNCED"
    case conflict = "CONFLICT"
}

public enum DriverOutboxIssue: String, Codable, Sendable {
    case offline
    case timeout
    case rateLimited = "rate-limited"
    case unavailable
    case signInRequired = "sign-in-required"
    case capabilityRequired = "capability-required"
    case disabled
    case conflict
    case invalid
    case invalidReceipt = "invalid-receipt"
    case retryLimit = "retry-limit"
}

public enum DriverOutboxOutcome: Equatable, Sendable {
    case synced(DriverCommandReceipt)
    case retry(DriverOutboxIssue, retryAfterSeconds: Int64?)
    case waiting(DriverOutboxIssue)
    case conflict(DriverOutboxIssue)
    case interrupted

    public static func forFailure(_ error: Error) -> Self {
        if error is CancellationError { return .interrupted }
        if let transport = error as? SignedTransportError {
            switch transport {
            case .offline: return .retry(.offline, retryAfterSeconds: nil)
            case .sessionRequired, .identityUnavailable, .signingUnavailable:
                return .waiting(.signInRequired)
            case .originUnavailable: return .waiting(.unavailable)
            case .refused(let status, let code, let retryAfter):
                switch status {
                case 401: return .waiting(.signInRequired)
                case 403: return .waiting(.capabilityRequired)
                case 404 where code == "driver.disabled": return .waiting(.disabled)
                case 408: return .retry(.timeout, retryAfterSeconds: retryAfter)
                case 409: return .conflict(.conflict)
                case 429: return .retry(.rateLimited, retryAfterSeconds: retryAfter)
                case 500...599: return .retry(.unavailable, retryAfterSeconds: retryAfter)
                default: return .conflict(.invalid)
                }
            case .invalidResponse, .oversizedResponse: return .conflict(.invalidReceipt)
            case .invalidRequest: return .conflict(.invalid)
            }
        }
        if let driver = error as? DriverAccountingError {
            switch driver {
            case .scopeChanged, .staleClaim: return .interrupted
            case .invalidReceipt: return .conflict(.invalidReceipt)
            case .invalidCommand: return .conflict(.invalid)
            case .localStorage, .existingCommand, .queueFull: return .interrupted
            }
        }
        if (error as? CommunityWriteError) == .signInRequired { return .waiting(.signInRequired) }
        return .conflict(.invalid)
    }
}

public struct DriverOutboxEntry: Codable, Equatable, Identifiable, Sendable {
    public let command: DriverAccountingCommand
    public let createdAtMillis: Int64
    public fileprivate(set) var state: DriverOutboxState = .pending
    public fileprivate(set) var attempts: Int = 0
    public fileprivate(set) var nextEligibleMillis: Int64 = 0
    public fileprivate(set) var issue: DriverOutboxIssue?
    public fileprivate(set) var receipt: DriverCommandReceipt?
    fileprivate var lease: Lease?

    public var id: String { command.operationId }

    fileprivate struct Lease: Codable, Equatable, Sendable {
        let token: UUID
        let untilMillis: Int64
    }
}

public struct DriverAccountingClaim: Equatable, Sendable {
    public let scope: DriverOutboxScope
    public let command: DriverAccountingCommand
    fileprivate let lease: DriverOutboxEntry.Lease
}

/// Durable, private driver outbox. A local lock makes lease acquisition and
/// settlement atomic across coordinators; a unique lease token fences a worker
/// that finishes after expiration or after another worker reclaims the command.
public final class DriverAccountingOutbox: @unchecked Sendable {
    public static let maxEntries = 1_000
    public static let leaseMillis: Int64 = 60_000
    public static let baseRetryMillis: Int64 = 5_000
    public static let maxRetryMillis: Int64 = 15 * 60_000
    public static let maxAttempts = 6

    private struct Stored: Codable, Equatable {
        let scope: DriverOutboxScope
        var entries: [DriverOutboxEntry] = []
    }

    private static let lock = NSLock()
    private let directory: URL

    public init(directory: URL? = nil) {
        self.directory = directory ?? IosLocalStore.defaultDirectory().appendingPathComponent("driver-outbox", isDirectory: true)
    }

    public func entries(in scope: DriverOutboxScope) throws -> [DriverOutboxEntry] {
        Self.lock.lock()
        defer { Self.lock.unlock() }
        return try read(scope).entries
    }

    public func knownEpoch(in scope: DriverOutboxScope) throws -> Int64? {
        try entries(in: scope).compactMap { $0.receipt?.epoch }.max()
    }

    @discardableResult
    public func enqueue(
        _ command: DriverAccountingCommand, in scope: DriverOutboxScope, nowMillis: Int64
    ) throws -> DriverOutboxEntry {
        guard command.isValid, nowMillis >= 0 else { throw DriverAccountingError.invalidCommand }
        return try update(scope) { state in
            if let previous = state.entries.first(where: { $0.id == command.id }) {
                guard previous.command == command else { throw DriverAccountingError.existingCommand }
                return previous
            }
            for previous in state.entries where previous.command.recordId == command.recordId {
                guard previous.state == .synced, let receipt = previous.receipt,
                      receipt.id == command.recordId, receipt.epoch == command.expectedEpoch,
                      receipt.revision <= command.expectedRevision else {
                    throw DriverAccountingError.existingCommand
                }
            }
            guard state.entries.count < Self.maxEntries else { throw DriverAccountingError.queueFull }
            let entry = DriverOutboxEntry(command: command, createdAtMillis: nowMillis, nextEligibleMillis: nowMillis)
            state.entries.append(entry)
            return entry
        }
    }

    public func claim(in scope: DriverOutboxScope, nowMillis: Int64) throws -> DriverAccountingClaim? {
        guard nowMillis >= 0, nowMillis <= Int64.max - Self.leaseMillis else { return nil }
        return try update(scope) { state in
            guard let index = state.entries.firstIndex(where: {
                $0.state == .pending && $0.nextEligibleMillis <= nowMillis && ($0.lease?.untilMillis ?? 0) <= nowMillis
            }) else { return nil }
            let lease = DriverOutboxEntry.Lease(token: UUID(), untilMillis: nowMillis + Self.leaseMillis)
            state.entries[index].lease = lease
            return DriverAccountingClaim(scope: scope, command: state.entries[index].command, lease: lease)
        }
    }

    public func settle(
        _ claim: DriverAccountingClaim, outcome: DriverOutboxOutcome, nowMillis: Int64
    ) throws {
        guard nowMillis >= 0, nowMillis < claim.lease.untilMillis else { throw DriverAccountingError.staleClaim }
        try update(claim.scope) { state in
            guard let index = state.entries.firstIndex(where: {
                $0.id == claim.command.id && $0.state == .pending && $0.lease == claim.lease
            }) else { throw DriverAccountingError.staleClaim }
            var entry = state.entries[index]
            entry.lease = nil
            switch outcome {
            case .synced(let receipt):
                guard entry.command.accepts(receipt) else { throw DriverAccountingError.invalidReceipt }
                entry.state = .synced
                entry.issue = nil
                entry.receipt = receipt
            case .retry(let issue, let retryAfter):
                entry.attempts += 1
                if entry.attempts >= Self.maxAttempts {
                    entry.state = .conflict
                    entry.issue = .retryLimit
                } else {
                    let header = max(0, min(retryAfter ?? 0, Self.maxRetryMillis / 1_000)) * 1_000
                    let exponential = min(Self.maxRetryMillis, Self.baseRetryMillis << (entry.attempts - 1))
                    let delay = max(header, exponential)
                    entry.nextEligibleMillis = nowMillis + min(delay, Int64.max - nowMillis)
                    entry.issue = issue
                }
            case .waiting(let issue):
                entry.nextEligibleMillis = Int64.max
                entry.issue = issue
            case .conflict(let issue):
                entry.state = .conflict
                entry.issue = issue
            case .interrupted:
                break
            }
            state.entries[index] = entry
        }
    }

    /// Explicit owner retry after session, capability or service becomes available.
    /// Parked conflicts must be reconciled or discarded, never auto-replayed.
    public func resumeWaiting(in scope: DriverOutboxScope, nowMillis: Int64) throws {
        guard nowMillis >= 0, nowMillis < Int64.max else { throw DriverAccountingError.invalidCommand }
        try update(scope) { state in
            for index in state.entries.indices where state.entries[index].state == .pending
                && state.entries[index].nextEligibleMillis == Int64.max {
                state.entries[index].nextEligibleMillis = nowMillis
            }
        }
    }

    public func discardConflict(operationId: String, in scope: DriverOutboxScope) throws {
        try update(scope) { state in
            state.entries.removeAll { $0.id == operationId && $0.state == .conflict }
        }
    }

    public func nextEligibleMillis(in scope: DriverOutboxScope) throws -> Int64? {
        let pending = try entries(in: scope).filter { $0.state == .pending && $0.nextEligibleMillis < Int64.max }
        return pending.map { max($0.nextEligibleMillis, $0.lease?.untilMillis ?? 0) }.min()
    }

    /// Exact scope purge. A late response cannot recreate a file after this call.
    public func erase(_ scope: DriverOutboxScope) throws {
        Self.lock.lock()
        defer { Self.lock.unlock() }
        let url = file(scope)
        guard FileManager.default.fileExists(atPath: url.path) else { return }
        do {
            try FileManager.default.removeItem(at: url)
        } catch {
            throw DriverAccountingError.localStorage
        }
    }

    private func file(_ scope: DriverOutboxScope) -> URL { directory.appendingPathComponent(scope.filename) }

    private func update<T>(_ scope: DriverOutboxScope, change: (inout Stored) throws -> T) throws -> T {
        Self.lock.lock()
        defer { Self.lock.unlock() }
        var stored = try read(scope)
        let original = stored
        let result = try change(&stored)
        if original != stored { try write(stored) }
        return result
    }

    private func read(_ scope: DriverOutboxScope) throws -> Stored {
        let url = file(scope)
        guard FileManager.default.fileExists(atPath: url.path) else { return Stored(scope: scope) }
        do {
            let state = try JSONDecoder().decode(Stored.self, from: Data(contentsOf: url))
            guard state.scope == scope, state.entries.count <= Self.maxEntries,
                  Set(state.entries.map(\.id)).count == state.entries.count,
                  state.entries.allSatisfy({ (0...Self.maxAttempts).contains($0.attempts) && $0.command.isValid }) else {
                throw DriverAccountingError.localStorage
            }
            return state
        } catch {
            throw DriverAccountingError.localStorage
        }
    }

    private func write(_ stored: Stored) throws {
        do {
            var folder = directory
            try FileManager.default.createDirectory(at: folder, withIntermediateDirectories: true)
            var settings = URLResourceValues()
            settings.isExcludedFromBackup = true
            try folder.setResourceValues(settings)
            let data = try JSONEncoder().encode(stored)
            #if os(iOS)
            try data.write(to: file(stored.scope), options: [.atomic, .completeFileProtection])
            #else
            try data.write(to: file(stored.scope), options: .atomic)
            #endif
        } catch {
            throw DriverAccountingError.localStorage
        }
    }
}

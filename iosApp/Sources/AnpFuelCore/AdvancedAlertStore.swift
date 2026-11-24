import Foundation

/// Protected owner data needed to retry a confirmed change and deduplicate optional local notices.
/// Live rule lists, inbox payloads and session credentials do not enter this store.
public final class AdvancedAlertStore: @unchecked Sendable {
    private static let retentionMillis: Int64 = 7 * 24 * 60 * 60 * 1_000
    private static let claimMillis: Int64 = 120_000
    private static let maxNotifiedItems = 400
    public static let notificationPrefix = "abastevo-hosted-alert-"
    private let directory: URL
    private let lock = NSLock()
    private var generation = UUID()

    private struct State: Codable {
        var pending: Pending?
        var notificationsEnabled = false
        var baselineRecorded = false
        var acknowledged: [String] = []
    }

    private struct Pending: Codable {
        let command: AdvancedAlertCommand
        let createdAt: Int64
        var attempts = 0
        var eligibleAt: Int64
        var claimId: UUID?
        var claimUntil: Int64?
    }

    struct Claim {
        let generation: UUID
        let id: UUID
        let command: AdvancedAlertCommand
    }

    public init(directory: URL = IosLocalStore.defaultDirectory()) {
        self.directory = directory.appendingPathComponent("advanced-alerts", isDirectory: true)
    }

    public func freeze(_ command: AdvancedAlertCommand, scope: AccountDataExportScope, now: Date = Date()) throws {
        try command.validate()
        try lock.withLock {
            let tick = try millis(now)
            var state = try load(scope, tick: tick)
            if let previous = state.pending {
                guard previous.command == command else { throw AdvancedAlertError.requestInProgress }
                return
            }
            state.pending = Pending(command: command, createdAt: tick, eligibleAt: tick)
            try save(state, scope: scope)
        }
    }

    public func pending(scope: AccountDataExportScope, now: Date = Date()) throws -> AdvancedAlertCommand? {
        try lock.withLock { try load(scope, tick: millis(now)).pending?.command }
    }

    public func retryAvailableAt(scope: AccountDataExportScope, now: Date = Date()) throws -> Date? {
        try lock.withLock {
            guard let item = try load(scope, tick: millis(now)).pending, item.attempts < PortableOutbox.maxDispatchAttempts,
                  item.eligibleAt < item.createdAt + Self.retentionMillis else { return nil }
            return Date(timeIntervalSince1970: Double(max(item.eligibleAt, item.claimUntil ?? 0)) / 1_000)
        }
    }

    public func discardPending(scope: AccountDataExportScope, now: Date = Date()) throws {
        try lock.withLock {
            let tick = try millis(now)
            var state = try load(scope, tick: tick)
            guard let pending = state.pending else { return }
            guard (pending.claimUntil ?? 0) <= tick else { throw AdvancedAlertError.requestInProgress }
            state.pending = nil
            try save(state, scope: scope)
        }
    }

    func claim(scope: AccountDataExportScope, now: Date) throws -> Claim? {
        try lock.withLock {
            let tick = try millis(now)
            var state = try load(scope, tick: tick)
            guard var item = state.pending, item.attempts < PortableOutbox.maxDispatchAttempts,
                  item.eligibleAt <= tick, (item.claimUntil ?? 0) <= tick else { return nil }
            let id = UUID()
            item.claimId = id
            item.claimUntil = min(tick + Self.claimMillis, item.createdAt + Self.retentionMillis)
            item.attempts += 1
            state.pending = item
            try save(state, scope: scope)
            return Claim(generation: generation, id: id, command: item.command)
        }
    }

    func finish(_ claim: Claim, scope: AccountDataExportScope, refusal: AdvancedAlertError?, now: Date) throws {
        try lock.withLock {
            guard claim.generation == generation else { throw AdvancedAlertError.ownerChanged }
            let tick = try millis(now)
            var state = try load(scope, tick: tick)
            guard var item = state.pending,
                  item.claimId == claim.id, item.command == claim.command else { throw AdvancedAlertError.ownerChanged }
            switch refusal {
            case .rateLimited(let seconds)?, .unavailable(let seconds)?:
                let backoff = PortableOutbox.markFailed(
                    command: OutboxCommand(commandId: item.command.operationId, kind: "advanced-alert", payload: "",
                                           attempts: item.attempts - 1), tickMillis: tick
                )
                let retryAfterMillis = max(0, min(seconds ?? 0, PortableOutbox.maxDelayTicks / 1_000)) * 1_000
                item.eligibleAt = max(tick + retryAfterMillis, backoff.nextEligibleTick)
                item.claimId = nil
                item.claimUntil = nil
                state.pending = item
            default:
                state.pending = nil
            }
            try save(state, scope: scope)
        }
    }

    public func dispatch(scope: AccountDataExportScope, client: CommunityWriteClient) async throws -> AdvancedAlertReceipt? {
        guard client.transport.origin == scope.origin, client.transport.ownerScope() == scope.ownerScope else {
            throw AdvancedAlertError.ownerChanged
        }
        guard let claim = try claim(scope: scope, now: Date()) else { return nil }
        let receipt: AdvancedAlertReceipt
        do {
            receipt = try await client.executeAdvancedAlert(claim.command, scope: scope)
        } catch is CancellationError {
            // Keep the immutable command and lease. The server may have received it.
            throw CancellationError()
        } catch {
            let refusal = error as? AdvancedAlertError ?? .unavailable(retryAfterSeconds: nil)
            try finish(claim, scope: scope, refusal: refusal, now: Date())
            throw refusal
        }
        guard client.transport.origin == scope.origin, client.transport.ownerScope() == scope.ownerScope else {
            throw AdvancedAlertError.ownerChanged
        }
        try finish(claim, scope: scope, refusal: nil, now: Date())
        return receipt
    }

    public func notificationsEnabled(scope: AccountDataExportScope, now: Date = Date()) throws -> Bool {
        try lock.withLock { try load(scope, tick: millis(now)).notificationsEnabled }
    }

    public func setNotificationsEnabled(_ enabled: Bool, scope: AccountDataExportScope, now: Date = Date()) throws {
        try lock.withLock {
            var state = try load(scope, tick: millis(now))
            guard state.notificationsEnabled != enabled else { return }
            state.notificationsEnabled = enabled
            state.baselineRecorded = false
            state.acknowledged = []
            try save(state, scope: scope)
        }
    }

    public func notificationCandidates(
        for inbox: AdvancedAlertInbox, scope: AccountDataExportScope, now: Date = Date()
    ) throws -> [AdvancedAlertInboxItem] {
        try lock.withLock {
            var state = try load(scope, tick: millis(now))
            guard state.notificationsEnabled else { return [] }
            let eligible = inbox.items.filter(\.canNotify)
            if !state.baselineRecorded {
                state.acknowledged = eligible.map(\.id)
                state.baselineRecorded = true
                try save(state, scope: scope)
                return []
            }
            let seen = Set(state.acknowledged)
            return eligible.filter { !seen.contains($0.id) }
        }
    }

    public func recordNotification(_ inboxID: String, scope: AccountDataExportScope, now: Date = Date()) throws {
        guard AdvancedAlertWire.validID(inboxID) else { throw AdvancedAlertError.invalid }
        try lock.withLock {
            var state = try load(scope, tick: millis(now))
            guard state.notificationsEnabled, state.baselineRecorded, !state.acknowledged.contains(inboxID) else { return }
            state.acknowledged = Array((state.acknowledged + [inboxID]).suffix(Self.maxNotifiedItems))
            try save(state, scope: scope)
        }
    }

    public func notificationIDPrefix(scope: AccountDataExportScope) -> String {
        Self.notificationPrefix + scopeKey(scope) + "-"
    }

    public func notificationID(_ inboxID: String, scope: AccountDataExportScope) -> String {
        notificationIDPrefix(scope: scope) + inboxID
    }

    public func purge() {
        lock.withLock {
            generation = UUID()
            try? FileManager.default.removeItem(at: directory)
        }
    }

    public func purge(scope: AccountDataExportScope) throws {
        try lock.withLock {
            generation = UUID()
            let path = url(scope)
            if FileManager.default.fileExists(atPath: path.path) { try FileManager.default.removeItem(at: path) }
        }
    }

    private func millis(_ date: Date) throws -> Int64 {
        let seconds = date.timeIntervalSince1970
        guard seconds.isFinite, (0..<8_000_000_000_000).contains(seconds) else { throw AdvancedAlertError.invalid }
        return Int64(seconds * 1_000)
    }

    private func scopeKey(_ scope: AccountDataExportScope) -> String {
        let fields = [scope.origin, scope.ownerScope, scope.sessionFamilyId]
        return IosDeviceProofSigner.sha256Hex(Data(fields.joined(separator: "\u{0}").utf8))
    }

    private func url(_ scope: AccountDataExportScope) -> URL { directory.appendingPathComponent(scopeKey(scope)) }

    private func load(_ scope: AccountDataExportScope, tick: Int64) throws -> State {
        let path = url(scope)
        guard FileManager.default.fileExists(atPath: path.path) else { return State() }
        guard let size = try path.resourceValues(forKeys: [.fileSizeKey]).fileSize, size <= 48 * 1024 else {
            throw AdvancedAlertError.unavailable(retryAfterSeconds: nil)
        }
        var state = try JSONDecoder().decode(State.self, from: Data(contentsOf: path))
        if let pending = state.pending,
           pending.createdAt < 0 || pending.createdAt > tick || tick - pending.createdAt >= Self.retentionMillis {
            state.pending = nil
            try save(state, scope: scope)
        }
        return state
    }

    private func save(_ state: State, scope: AccountDataExportScope) throws {
        try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
        var folder = directory
        var values = URLResourceValues()
        values.isExcludedFromBackup = true
        try folder.setResourceValues(values)
        var path = url(scope)
        let data = try JSONEncoder().encode(state)
        #if os(iOS)
        try data.write(to: path, options: [.atomic, .completeFileProtection])
        #else
        try data.write(to: path, options: .atomic)
        #endif
        try path.setResourceValues(values)
    }
}

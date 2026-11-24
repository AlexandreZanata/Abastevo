import CryptoKit
import Foundation
#if canImport(Security)
import Security
#endif

/// The confirmed command only. Private report history and session credentials never enter this journal.
public final class QualityReportJournal: @unchecked Sendable {
    private static let retentionMillis: Int64 = 7 * 24 * 60 * 60 * 1_000
    private static let claimMillis: Int64 = 120_000
    private let directory: URL
    private let injectedKey: SymmetricKey?
    private let lock = NSLock()
    private var generation = UUID()

    struct Record: Codable {
        let command: QualityReportCommand
        let createdAt: Int64
        let expiresAt: Int64
        var attempts: Int
        var eligibleAt: Int64
        var claimId: UUID?
        var claimUntil: Int64?
    }

    struct Claim {
        let generation: UUID
        let id: UUID
        let command: QualityReportCommand
    }

    public init(directory: URL = IosLocalStore.defaultDirectory(), testKey: SymmetricKey? = nil) {
        self.directory = directory.appendingPathComponent("pending-quality-reports", isDirectory: true)
        injectedKey = testKey
    }

    public func pending(scope: QualityReportScope, now: Date = Date()) throws -> QualityReportCommand? {
        try lock.withLock { try read(scope: scope, tick: millis(now))?.command }
    }

    /// Every pending command offers an explicit retry (Android `QualityReportsViewModel.retry`).
    /// Only a server delay, backoff or a live lease postpones it.
    public func retryAvailableAt(scope: QualityReportScope, now: Date = Date()) throws -> Date? {
        try lock.withLock {
            guard let item = try read(scope: scope, tick: millis(now)) else { return nil }
            let at = max(Self.eligibleAt(item), item.claimUntil ?? 0)
            return Date(timeIntervalSince1970: TimeInterval(at) / 1_000)
        }
    }

    public func freeze(_ command: QualityReportCommand, scope: QualityReportScope, now: Date = Date()) throws {
        try command.validate(now: now)
        try lock.withLock {
            let tick = try millis(now)
            if let previous = try read(scope: scope, tick: tick) {
                guard previous.command == command else { throw QualityReportError.conflict }
                return
            }
            let item = Record(command: command, createdAt: tick, expiresAt: tick + Self.retentionMillis,
                              attempts: 0, eligibleAt: tick, claimId: nil, claimUntil: nil)
            try save(item, scope: scope)
        }
    }

    func claim(scope: QualityReportScope, now: Date) throws -> Claim? {
        try lock.withLock {
            let tick = try millis(now)
            guard var item = try read(scope: scope, tick: tick),
                  Self.eligibleAt(item) <= tick, (item.claimUntil ?? 0) <= tick else { return nil }
            let claim = UUID()
            item.claimId = claim
            item.claimUntil = min(tick + Self.claimMillis, item.expiresAt)
            item.attempts += 1
            try save(item, scope: scope)
            return Claim(generation: generation, id: claim, command: item.command)
        }
    }

    func finish(_ claim: Claim, scope: QualityReportScope, refusal: QualityReportError?, now: Date) throws {
        try lock.withLock {
            let tick = try millis(now)
            guard generation == claim.generation, var item = try read(scope: scope, tick: tick),
                  item.claimId == claim.id, item.command == claim.command else {
                throw QualityReportError.ownerChanged
            }
            switch refusal {
            case .none, .invalid?, .conflict?, .notFound?:
                try FileManager.default.removeItem(at: path(scope))
            case .authRequired?, .ownerChanged?:
                // Keep the frozen command and its id. Only the same account and session family can retry it.
                item.eligibleAt = tick
                item.claimId = nil
                item.claimUntil = nil
                try save(item, scope: scope)
            case .quota(let seconds)?, .unavailable(let seconds)?:
                let failed = PortableOutbox.markFailed(
                    command: OutboxCommand(commandId: item.command.commandId, kind: "quality-report", payload: "",
                                           attempts: min(max(0, item.attempts - 1), PortableOutbox.maxDispatchAttempts - 1)),
                    tickMillis: tick
                )
                let hint = max(0, min(seconds ?? 0, PortableOutbox.maxDelayTicks / 1_000)) * 1_000
                item.eligibleAt = max(failed.nextEligibleTick, tick + hint)
                item.claimId = nil
                item.claimUntil = nil
                try save(item, scope: scope)
            }
        }
    }

    public func purge() {
        lock.withLock {
            generation = UUID()
            try? FileManager.default.removeItem(at: directory)
            #if canImport(Security)
            if injectedKey == nil { SecItemDelete(keyQuery as CFDictionary) }
            #endif
        }
    }

    /// Dispatches one already-confirmed command under a single lease. A lost reply can be retried with the same ID.
    public func dispatch(
        scope: QualityReportScope, client: CommunityWriteClient, now: Date = Date()
    ) async throws -> QualityReportReceipt? {
        guard transportMatches(client, scope: scope) else { throw QualityReportError.ownerChanged }
        guard let claim = try claim(scope: scope, now: now) else { return nil }
        let receipt: QualityReportReceipt
        do {
            receipt = try await client.executeQualityReport(claim.command, scope: scope)
        } catch is CancellationError {
            // Leave the claim to expire. The server may already have accepted the command.
            throw CancellationError()
        } catch {
            let refusal = error as? QualityReportError ?? .unavailable(retryAfterSeconds: nil)
            try finish(claim, scope: scope, refusal: refusal, now: Date())
            throw refusal
        }
        guard transportMatches(client, scope: scope) else { throw QualityReportError.ownerChanged }
        try finish(claim, scope: scope, refusal: nil, now: Date())
        return receipt
    }

    /// Records parked by an earlier build carry no automatic eligibility; an explicit retry may claim them.
    private static func eligibleAt(_ item: Record) -> Int64 {
        item.eligibleAt >= item.expiresAt ? item.createdAt : item.eligibleAt
    }

    private func transportMatches(_ client: CommunityWriteClient, scope: QualityReportScope) -> Bool {
        client.transport.origin == scope.origin && client.transport.ownerScope() == scope.ownerScope
    }

    private func millis(_ value: Date) throws -> Int64 {
        let millis = value.timeIntervalSince1970 * 1_000
        guard millis.isFinite, millis >= 0, millis < 253_402_300_799_999 else { throw QualityReportError.invalid }
        return Int64(millis)
    }

    private func path(_ scope: QualityReportScope) -> URL {
        directory.appendingPathComponent(scope.key)
    }

    private func read(scope: QualityReportScope, tick: Int64) throws -> Record? {
        let file = path(scope)
        guard FileManager.default.fileExists(atPath: file.path) else { return nil }
        guard let key = try key(create: false) else {
            try? FileManager.default.removeItem(at: file)
            return nil
        }
        let size = try file.resourceValues(forKeys: [.fileSizeKey]).fileSize
        guard let size, size <= 16 * 1024 else { throw QualityReportError.unavailable(retryAfterSeconds: nil) }
        let box = try AES.GCM.SealedBox(combined: Data(contentsOf: file))
        let bytes = try AES.GCM.open(box, using: key, authenticating: Data(scope.key.utf8))
        let item = try JSONDecoder().decode(Record.self, from: bytes)
        guard item.createdAt <= tick, item.expiresAt > tick, item.expiresAt - item.createdAt <= Self.retentionMillis else {
            try FileManager.default.removeItem(at: file)
            return nil
        }
        return item
    }

    private func save(_ item: Record, scope: QualityReportScope) throws {
        guard let key = try key(create: true) else { throw QualityReportError.unavailable(retryAfterSeconds: nil) }
        let box = try AES.GCM.seal(JSONEncoder().encode(item), using: key, authenticating: Data(scope.key.utf8))
        guard let data = box.combined else { throw QualityReportError.unavailable(retryAfterSeconds: nil) }
        try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
        var folder = directory
        var values = URLResourceValues()
        values.isExcludedFromBackup = true
        try folder.setResourceValues(values)
        var file = path(scope)
        #if os(iOS)
        try data.write(to: file, options: [.atomic, .completeFileProtection])
        #else
        try data.write(to: file, options: .atomic)
        #endif
        try file.setResourceValues(values)
    }

    #if canImport(Security)
    private var keyQuery: [String: Any] {
        [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: "com.anpfuel.private-quality-report-journal",
            kSecAttrAccount as String: "encryption-key",
            kSecAttrSynchronizable as String: false,
        ]
    }
    #endif

    private func key(create: Bool) throws -> SymmetricKey? {
        if let injectedKey { return injectedKey }
        #if canImport(Security)
        var read = keyQuery
        read[kSecMatchLimit as String] = kSecMatchLimitOne
        read[kSecReturnData as String] = true
        var result: CFTypeRef?
        let status = SecItemCopyMatching(read as CFDictionary, &result)
        if status == errSecSuccess, let data = result as? Data, data.count == 32 {
            return SymmetricKey(data: data)
        }
        guard status == errSecItemNotFound else { throw QualityReportError.unavailable(retryAfterSeconds: nil) }
        guard create else { return nil }
        let key = SymmetricKey(size: .bits256)
        var add = keyQuery
        add[kSecAttrAccessible as String] = kSecAttrAccessibleWhenUnlockedThisDeviceOnly
        add[kSecValueData as String] = key.withUnsafeBytes { Data($0) }
        let saved = SecItemAdd(add as CFDictionary, nil)
        if saved == errSecDuplicateItem { return try self.key(create: false) }
        guard saved == errSecSuccess else { throw QualityReportError.unavailable(retryAfterSeconds: nil) }
        return key
        #else
        return nil
        #endif
    }
}

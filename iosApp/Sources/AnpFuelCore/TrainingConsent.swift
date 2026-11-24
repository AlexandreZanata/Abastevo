import CryptoKit
import Foundation

public enum TrainingConsentError: Error, Equatable, Sendable {
    case signInRequired
    case termsOutdated
    case staleRevision
    case conflict
    case invalid
    case offline
    case unavailable(retryAfterSeconds: Int64?)
    case localStorage

    public static func fromTransport(_ error: Error) -> TrainingConsentError {
        if let error = error as? TrainingConsentError { return error }
        if let community = error as? CommunityWriteError, case .signInRequired = community {
            return .signInRequired
        }
        guard let transport = error as? SignedTransportError else {
            return .unavailable(retryAfterSeconds: nil)
        }
        switch transport {
        case .sessionRequired, .identityUnavailable, .signingUnavailable:
            return .signInRequired
        case .offline:
            return .offline
        case .refused(let status, let code, let retryAfter):
            switch (status, code) {
            case (401, _), (403, _): return .signInRequired
            case (400, "privacy.terms-outdated"): return .termsOutdated
            case (409, "privacy.stale-revision"): return .staleRevision
            case (409, "privacy.conflict"): return .conflict
            case (400, _): return .invalid
            default: return .unavailable(retryAfterSeconds: retryAfter)
            }
        default:
            return .unavailable(retryAfterSeconds: nil)
        }
    }

    public var canRebindRevocation: Bool { self == .staleRevision || self == .conflict }
}

public struct TrainingConsentScope: Equatable, Sendable {
    public let ownerScope: String
    public let origin: String

    public init?(ownerScope: String, origin: String) {
        let parts = ownerScope.split(separator: ":", maxSplits: 2, omittingEmptySubsequences: false)
        guard parts.count == 3, parts[0] == "account", !parts[1].isEmpty,
              PortableAnonymousProof.isFingerprintShape(String(parts[2])),
              SignedRequestProfile.authority(origin: origin) != nil else { return nil }
        self.ownerScope = ownerScope
        self.origin = origin
    }

    fileprivate var storageKey: String {
        SHA256.hash(data: Data("\(origin)|\(ownerScope)".utf8))
            .map { String(format: "%02x", $0) }.joined()
    }
}

public struct TrainingConsentReview: Equatable, Sendable {
    public let scope: TrainingConsentScope
    public private(set) var read = false
    public private(set) var checked = false

    public init(scope: TrainingConsentScope) { self.scope = scope }

    public var canConfirm: Bool { read && checked && TrainingTermsContract.verifiedDigestMatches() }

    public mutating func finishReading() {
        read = true
        checked = false
    }

    public mutating func check(_ value: Bool) {
        checked = value && read
    }
}

public enum TrainingConsentDecision: String, Codable, Sendable {
    case granted = "GRANTED"
    case revoked = "REVOKED"
}

public struct TrainingConsentStatus: Equatable, Sendable {
    public let granted: Bool
    public let state: TrainingConsentDecision?
    public let termsVersion: String?
    public let termsDigest: String?
    public let revision: Int64
    public let needsRegrant: Bool
    public let decidedAt: Date?
}

public struct TrainingConsentReceipt: Equatable, Sendable {
    public let state: TrainingConsentDecision
    public let revision: Int64
    public let termsVersion: String?
    public let decidedAt: Date
    public let replayed: Bool
    public let deleteOwnedTrainingAssets: Bool
}

public enum TrainingConsentWire {
    public static let statusPath = "/v1/privacy/training-consent"
    static let grantPath = statusPath + "/grant"
    static let revokePath = statusPath + "/revoke"

    public static func body(for command: TrainingConsentCommand) throws -> Data {
        guard let revision = command.revision, revision > 0 else { throw TrainingConsentError.invalid }
        let encoder = JSONEncoder()
        encoder.outputFormatting = [.sortedKeys]
        if command.decision == .granted {
            guard let version = command.termsVersion, let digest = command.termsDigest else {
                throw TrainingConsentError.invalid
            }
            return try encoder.encode(GrantBody(termsVersion: version, termsDigest: digest, revision: revision))
        }
        return try encoder.encode(RevokeBody(revision: revision))
    }

    public static func status(from data: Data) throws -> TrainingConsentStatus {
        guard let body = try? JSONDecoder().decode(StatusBody.self, from: data) else {
            throw TrainingConsentError.unavailable(retryAfterSeconds: nil)
        }
        let revision = body.revision ?? 0
        let needsRegrant = body.needsRegrant ?? false
        let validState = body.state == nil || body.state == "" || body.state.flatMap(TrainingConsentDecision.init(rawValue:)) != nil
        let state = body.state.flatMap(TrainingConsentDecision.init(rawValue:))
        let version = body.termsVersion.flatMap { $0.isEmpty ? nil : $0 }
        let digest = body.termsDigest.flatMap { $0.isEmpty ? nil : $0 }
        let decidedAt = body.decidedAt.flatMap(parseDate)
        guard validState, revision >= 0, (state == nil) == (revision == 0),
              (version?.utf8.count ?? 0) <= 128,
              digest == nil || digest?.range(of: "^[a-f0-9]{64}$", options: .regularExpression) != nil,
              body.decidedAt == nil || decidedAt != nil,
              !body.granted || (state == .granted && version != nil && digest != nil && !needsRegrant),
              !needsRegrant || (!body.granted && state == .granted) else {
            throw TrainingConsentError.unavailable(retryAfterSeconds: nil)
        }
        return TrainingConsentStatus(
            granted: body.granted, state: state, termsVersion: version, termsDigest: digest,
            revision: revision, needsRegrant: needsRegrant, decidedAt: decidedAt
        )
    }

    public static func receipt(from data: Data, command: TrainingConsentCommand) throws -> TrainingConsentReceipt {
        guard let body = try? JSONDecoder().decode(ReceiptBody.self, from: data),
              body.state == command.decision, body.revision == command.revision,
              let decidedAt = parseDate(body.decidedAt),
              (body.termsVersion?.utf8.count ?? 0) <= 128,
              body.state != .granted || body.termsVersion == command.termsVersion else {
            throw TrainingConsentError.unavailable(retryAfterSeconds: nil)
        }
        return TrainingConsentReceipt(
            state: body.state, revision: body.revision, termsVersion: body.termsVersion, decidedAt: decidedAt,
            replayed: body.replayed, deleteOwnedTrainingAssets: body.deleteOwnedTrainingAssets ?? false
        )
    }

    private static func parseDate(_ value: String) -> Date? {
        let formatter = ISO8601DateFormatter()
        formatter.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        if let date = formatter.date(from: value) { return date }
        formatter.formatOptions = [.withInternetDateTime]
        return formatter.date(from: value)
    }

    private struct GrantBody: Encodable {
        let termsVersion: String
        let termsDigest: String
        let revision: Int64

        enum CodingKeys: String, CodingKey {
            case termsVersion = "terms_version"
            case termsDigest = "terms_digest"
            case revision
        }
    }

    private struct RevokeBody: Encodable {
        let revision: Int64
    }

    private struct StatusBody: Decodable {
        let granted: Bool
        let state: String?
        let termsVersion: String?
        let termsDigest: String?
        let revision: Int64?
        let needsRegrant: Bool?
        let decidedAt: String?

        enum CodingKeys: String, CodingKey {
            case granted, state, revision
            case termsVersion = "terms_version"
            case termsDigest = "terms_digest"
            case needsRegrant = "needs_regrant"
            case decidedAt = "decided_at"
        }
    }

    private struct ReceiptBody: Decodable {
        let state: TrainingConsentDecision
        let revision: Int64
        let termsVersion: String?
        let decidedAt: String
        let replayed: Bool
        let deleteOwnedTrainingAssets: Bool?

        enum CodingKeys: String, CodingKey {
            case state, revision, replayed
            case termsVersion = "terms_version"
            case decidedAt = "decided_at"
            case deleteOwnedTrainingAssets = "delete_owned_training_assets"
        }
    }
}

public struct TrainingConsentCommand: Codable, Equatable, Sendable {
    public let id: UUID
    public let decision: TrainingConsentDecision
    public let revision: Int64?
    public let termsVersion: String?
    public let termsDigest: String?

    fileprivate init(
        id: UUID, decision: TrainingConsentDecision, revision: Int64?,
        termsVersion: String? = nil, termsDigest: String? = nil
    ) {
        self.id = id
        self.decision = decision
        self.revision = revision
        self.termsVersion = termsVersion
        self.termsDigest = termsDigest
    }

    public var idempotencyKey: String { id.uuidString.lowercased() }
}

/// A grant retries only after another explicit confirmation. A local OFF supersedes any grant immediately.
public struct TrainingConsentJournalState: Codable, Equatable, Sendable {
    public private(set) var revisionFloor: Int64 = 0
    public private(set) var pending: TrainingConsentCommand?
    public private(set) var retryAt: Int64 = 0
    public private(set) var attempts: Int = 0

    public init() {}

    fileprivate var valid: Bool {
        guard revisionFloor >= 0, retryAt >= 0, (0...16).contains(attempts) else { return false }
        guard let pending else { return retryAt == 0 && attempts == 0 }
        if pending.decision == .granted {
            guard let version = pending.termsVersion, !version.isEmpty, version.utf8.count <= 128,
                  pending.termsDigest?.range(of: "^[a-f0-9]{64}$", options: .regularExpression) != nil,
                  let revision = pending.revision else { return false }
            return revision > 0 && revision == revisionFloor
        }
        guard pending.termsVersion == nil, pending.termsDigest == nil else { return false }
        return pending.revision == nil || pending.revision == revisionFloor && revisionFloor > 0
    }

    public mutating func requestRevocation(id: UUID) {
        guard pending?.decision != .revoked else { return }
        pending = TrainingConsentCommand(id: id, decision: .revoked, revision: nil)
        attempts = 0
        retryAt = 0
    }

    public mutating func requestGrant(id: UUID, serverRevision: Int64) throws {
        switch pending?.decision {
        case .granted:
            guard pending?.termsVersion == TrainingTermsContract.version,
                  pending?.termsDigest == TrainingTermsContract.digestSha256 else {
                throw TrainingConsentError.conflict
            }
            return
        case .revoked: throw TrainingConsentError.conflict
        case nil: break
        }
        guard serverRevision >= revisionFloor else { throw TrainingConsentError.staleRevision }
        revisionFloor = try nextRevision(after: serverRevision)
        pending = TrainingConsentCommand(
            id: id, decision: .granted, revision: revisionFloor, termsVersion: TrainingTermsContract.version,
            termsDigest: TrainingTermsContract.digestSha256
        )
    }

    public mutating func bindRevocation(
        _ command: TrainingConsentCommand, serverRevision: Int64, confirmedConflict: Bool = false
    ) throws {
        guard pending == command, command.decision == .revoked,
              command.revision == nil || confirmedConflict else { return }
        revisionFloor = try nextRevision(after: serverRevision)
        pending = TrainingConsentCommand(id: command.id, decision: .revoked, revision: revisionFloor)
    }

    public mutating func acknowledge(_ command: TrainingConsentCommand, receipt: TrainingConsentReceipt) {
        guard pending == command, receipt.revision == command.revision, receipt.state == command.decision else { return }
        clearPending()
    }

    public mutating func discardRefusedGrant(_ command: TrainingConsentCommand) {
        guard pending == command, command.decision == .granted else { return }
        // The refusal confirms this grant performed no mutation.
        revisionFloor = max(0, revisionFloor - 1)
        clearPending()
    }

    public mutating func failed(_ command: TrainingConsentCommand, now: Int64, retryAfterSeconds: Int64?) {
        guard pending == command, now >= 0 else { return }
        let delay = max(30 * (Int64(1) << min(attempts, 6)), retryAfterSeconds ?? 0)
        retryAt = min(delay, Int64.max - now) + now
        attempts = min(attempts + 1, 16)
    }

    private mutating func clearPending() {
        pending = nil
        retryAt = 0
        attempts = 0
    }

    private func nextRevision(after serverRevision: Int64) throws -> Int64 {
        let floor = max(serverRevision, revisionFloor)
        guard serverRevision >= 0, floor < Int64.max else { throw TrainingConsentError.invalid }
        return floor + 1
    }
}

/// A protected, atomically updated journal. A failed disk write must never report that OFF has been saved.
public final class TrainingConsentJournal: @unchecked Sendable {
    private let directory: URL
    private let lock = NSLock()

    public init(directory: URL? = nil) {
        self.directory = (directory ?? IosLocalStore.defaultDirectory())
            .appendingPathComponent("training-consent-v1", isDirectory: true)
    }

    public func load(_ scope: TrainingConsentScope) throws -> TrainingConsentJournalState {
        lock.lock()
        defer { lock.unlock() }
        return try loadUnlocked(scope)
    }

    @discardableResult
    public func update(
        _ scope: TrainingConsentScope, change: (inout TrainingConsentJournalState) throws -> Void
    ) throws -> TrainingConsentJournalState {
        lock.lock()
        defer { lock.unlock() }
        var state = try loadUnlocked(scope)
        try change(&state)
        guard state.valid else { throw TrainingConsentError.localStorage }
        do {
            let file = path(scope)
            try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
            var excluded = URLResourceValues()
            excluded.isExcludedFromBackup = true
            var protectedFolder = directory
            try protectedFolder.setResourceValues(excluded)
            let data = try JSONEncoder().encode(state)
            #if os(iOS)
            try data.write(to: file, options: [.atomic, .completeFileProtection])
            #else
            try data.write(to: file, options: .atomic)
            #endif
        } catch {
            throw TrainingConsentError.localStorage
        }
        return state
    }

    public func purge() throws {
        lock.lock()
        defer { lock.unlock() }
        do {
            try FileManager.default.removeItem(at: directory)
        } catch let error as CocoaError where error.code == .fileNoSuchFile {
            return
        } catch {
            throw TrainingConsentError.localStorage
        }
    }

    /// Logout keeps only the journals that still owe the server an OFF, so the revocation resumes
    /// when the same owner signs in again (Android keeps its journal and retries it in a worker).
    /// Every other journal, and any unreadable file, is removed.
    public func purgeKeepingPendingRevocations() throws {
        lock.lock()
        defer { lock.unlock() }
        let files: [URL]
        do {
            files = try FileManager.default.contentsOfDirectory(at: directory, includingPropertiesForKeys: [.fileSizeKey])
        } catch let error as CocoaError where error.code == .fileNoSuchFile || error.code == .fileReadNoSuchFile {
            return
        } catch {
            throw TrainingConsentError.localStorage
        }
        do {
            for file in files {
                if let state = try? Self.decode(file), state.pending?.decision == .revoked { continue }
                try FileManager.default.removeItem(at: file)
            }
        } catch {
            throw TrainingConsentError.localStorage
        }
    }

    private func path(_ scope: TrainingConsentScope) -> URL {
        directory.appendingPathComponent(scope.storageKey + ".json")
    }

    private func loadUnlocked(_ scope: TrainingConsentScope) throws -> TrainingConsentJournalState {
        do {
            return try Self.decode(path(scope))
        } catch let error as CocoaError where error.code == .fileNoSuchFile || error.code == .fileReadNoSuchFile {
            return TrainingConsentJournalState()
        } catch {
            throw TrainingConsentError.localStorage
        }
    }

    private static func decode(_ url: URL) throws -> TrainingConsentJournalState {
        let size = try url.resourceValues(forKeys: [.fileSizeKey]).fileSize
        guard let size, size <= 4096 else { throw TrainingConsentError.localStorage }
        let state = try JSONDecoder().decode(TrainingConsentJournalState.self, from: Data(contentsOf: url))
        guard state.valid else { throw TrainingConsentError.localStorage }
        return state
    }
}

public extension CommunityWriteClient {
    func trainingConsentStatus(
        expectedScope: String, expectedFamilyId: String? = nil
    ) async throws -> TrainingConsentStatus {
        do {
            let response = try await signedOwnerGet(
                path: TrainingConsentWire.statusPath, expectedScope: expectedScope, expectedFamilyId: expectedFamilyId
            )
            return try TrainingConsentWire.status(from: response.body)
        } catch {
            try Task.checkCancellation()
            throw TrainingConsentError.fromTransport(error)
        }
    }

    func writeTrainingConsent(
        _ command: TrainingConsentCommand, expectedScope: String, expectedFamilyId: String? = nil
    ) async throws -> TrainingConsentReceipt {
        let body = try TrainingConsentWire.body(for: command)
        let path = command.decision == .granted ? TrainingConsentWire.grantPath : TrainingConsentWire.revokePath
        do {
            _ = try await ownerSession(expectedScope: expectedScope, expectedFamilyId: expectedFamilyId)
            let response = try await ownerPost(
                path: path, expectedScope: expectedScope, requiresDeviceProof: true,
                idempotencyKey: command.idempotencyKey, requiresSameSessionFamily: true
            ) { session in
                guard expectedFamilyId == nil || session.familyId == expectedFamilyId else { return nil }
                return body
            }
            return try TrainingConsentWire.receipt(from: response.body, command: command)
        } catch {
            try Task.checkCancellation()
            throw TrainingConsentError.fromTransport(error)
        }
    }
}

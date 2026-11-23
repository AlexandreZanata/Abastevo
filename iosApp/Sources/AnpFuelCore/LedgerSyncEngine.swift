import Foundation

/// Private ledger cloud sync state and transitions (B-BR-PR04/PR05/PR07–PR09, BUC-PR02/PR04/PR07).
///
/// Port of Android `DispatchLedgerCommandUseCase`, `ResolveLedgerConflictUseCase`,
/// `PreviewLedgerRestoreUseCase` and `LedgerCommandFactory`, against the backend
/// `/v1/ledger/*` routes. Every state belongs to one account and one origin; nothing
/// crosses scopes. Uploads need explicit opt-in and server consent; a 403 pauses paid
/// uploads while local records stay readable; deletes and erasure stay free; a 409
/// keeps base, current and proposed versions until the owner decides.
/// Zero postfix unwrap operators anywhere in this file.

public struct LedgerSyncScope: Codable, Equatable, Hashable, Sendable {
    public let accountId: String
    public let origin: String

    public init?(accountId: String?, origin: String) {
        guard let accountId, !accountId.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty,
              !origin.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty else {
            return nil
        }
        self.accountId = accountId
        self.origin = origin
    }

    public var key: String { accountId + "|" + origin }
}

public enum LedgerCommandStatus: String, Codable, Sendable {
    case pending
    case refused
}

/// Durable command intent saved before any upload attempt.
public struct LedgerQueuedCommand: Codable, Equatable, Identifiable, Sendable {
    public let operationId: String
    public let dataset: LedgerDataset
    public let recordId: String
    public let expectedRevision: Int64
    public let isDelete: Bool
    /// Exact payload body; empty for a delete.
    public let payloadJson: String
    public let createdAtMillis: Int64
    public var status: LedgerCommandStatus
    public var attempts: Int
    public var nextAttemptAtMillis: Int64
    public var lastCode: String?

    public init(
        operationId: String,
        dataset: LedgerDataset,
        recordId: String,
        expectedRevision: Int64,
        isDelete: Bool,
        payloadJson: String,
        createdAtMillis: Int64,
        status: LedgerCommandStatus = .pending,
        attempts: Int = 0,
        nextAttemptAtMillis: Int64 = 0,
        lastCode: String? = nil
    ) {
        self.operationId = operationId
        self.dataset = dataset
        self.recordId = recordId
        self.expectedRevision = expectedRevision
        self.isDelete = isDelete
        self.payloadJson = payloadJson
        self.createdAtMillis = createdAtMillis
        self.status = status
        self.attempts = attempts
        self.nextAttemptAtMillis = nextAttemptAtMillis
        self.lastCode = lastCode
    }

    public var id: String { operationId }
    public var key: String { LedgerSyncKeys.key(dataset, recordId) }
}

/// Last server acknowledgement for one record: its revision and the acknowledged payload (the conflict base).
public struct LedgerAck: Codable, Equatable, Sendable {
    public let revision: Int64
    public let payloadJson: String
    public let atMillis: Int64

    public init(revision: Int64, payloadJson: String, atMillis: Int64) {
        self.revision = revision
        self.payloadJson = payloadJson
        self.atMillis = atMillis
    }
}

/// What the server holds now for a conflicting record.
public struct LedgerCurrentVersion: Codable, Equatable, Sendable {
    public enum State: String, Codable, Sendable {
        case unknown
        case missing
        case present
    }

    public let state: State
    public let revision: Int64?
    /// Canonical payload; nil when the server copy could not be decoded.
    public let payloadJson: String?
    public let voided: Bool

    public init(state: State, revision: Int64? = nil, payloadJson: String? = nil, voided: Bool = false) {
        self.state = state
        self.revision = revision
        self.payloadJson = payloadJson
        self.voided = voided
    }

    public static let unknown = LedgerCurrentVersion(state: .unknown)
    public static let missing = LedgerCurrentVersion(state: .missing)
}

/// An explicit conflict: never resolved by arrival order.
public struct LedgerConflict: Codable, Equatable, Identifiable, Sendable {
    public let dataset: LedgerDataset
    public let recordId: String
    public let operationId: String
    public let expectedRevision: Int64
    public var basePayloadJson: String?
    public var proposedPayloadJson: String
    public var current: LedgerCurrentVersion
    public let detectedAtMillis: Int64

    public init(
        dataset: LedgerDataset,
        recordId: String,
        operationId: String,
        expectedRevision: Int64,
        basePayloadJson: String?,
        proposedPayloadJson: String,
        current: LedgerCurrentVersion,
        detectedAtMillis: Int64
    ) {
        self.dataset = dataset
        self.recordId = recordId
        self.operationId = operationId
        self.expectedRevision = expectedRevision
        self.basePayloadJson = basePayloadJson
        self.proposedPayloadJson = proposedPayloadJson
        self.current = current
        self.detectedAtMillis = detectedAtMillis
    }

    public var key: String { LedgerSyncKeys.key(dataset, recordId) }
    public var id: String { key }
}

public struct LedgerPullCursor: Codable, Equatable, Sendable {
    public let cursor: String
    public let savedAtMillis: Int64

    public init(cursor: String, savedAtMillis: Int64) {
        self.cursor = cursor
        self.savedAtMillis = savedAtMillis
    }
}

/// Everything the device holds about one account's private ledger on one origin.
public struct LedgerSyncState: Codable, Equatable, Sendable {
    public let scope: LedgerSyncScope
    /// Explicit owner opt-in on this device (B-BR-PR07).
    public var enabled: Bool
    /// Datasets the server confirmed in its consent mask.
    public var consentedDatasets: [LedgerDataset]
    public var consentRevision: Int64?
    public var commands: [LedgerQueuedCommand]
    public var conflicts: [LedgerConflict]
    public var acked: [String: LedgerAck]
    /// Locally deleted records: a pull never brings them back.
    public var deletedKeys: [String]
    /// Records whose cloud copy the owner erased; they stay on the device and are not uploaded again.
    public var localOnlyKeys: [String]
    public var pullCursors: [String: LedgerPullCursor]
    public var lastPullAtMillis: Int64?
    /// Set by a 403 on a paid write; deletes keep flowing.
    public var paidUploadsPaused: Bool
    public var pauseCode: String?
    public var lastErrorCode: String?
    public var lastSuccessAtMillis: Int64?
    public var vehicleLease: VehicleCapabilityLease?
    public var premiumObservation: PremiumCapabilityObservation?

    public init(scope: LedgerSyncScope) {
        self.scope = scope
        self.enabled = false
        self.consentedDatasets = []
        self.consentRevision = nil
        self.commands = []
        self.conflicts = []
        self.acked = [:]
        self.deletedKeys = []
        self.localOnlyKeys = []
        self.pullCursors = [:]
        self.lastPullAtMillis = nil
        self.paidUploadsPaused = false
        self.pauseCode = nil
        self.lastErrorCode = nil
        self.lastSuccessAtMillis = nil
        self.vehicleLease = nil
        self.premiumObservation = nil
    }

    private enum CodingKeys: String, CodingKey {
        case scope
        case enabled
        case consentedDatasets
        case consentRevision
        case commands
        case conflicts
        case acked
        case deletedKeys
        case localOnlyKeys
        case pullCursors
        case lastPullAtMillis
        case paidUploadsPaused
        case pauseCode
        case lastErrorCode
        case lastSuccessAtMillis
        case vehicleLease
        case premiumObservation
    }

    public init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        self.init(scope: try container.decode(LedgerSyncScope.self, forKey: .scope))
        enabled = try container.decodeIfPresent(Bool.self, forKey: .enabled) ?? false
        consentedDatasets = try container.decodeIfPresent([LedgerDataset].self, forKey: .consentedDatasets) ?? []
        consentRevision = try container.decodeIfPresent(Int64.self, forKey: .consentRevision)
        commands = try container.decodeIfPresent([LedgerQueuedCommand].self, forKey: .commands) ?? []
        conflicts = try container.decodeIfPresent([LedgerConflict].self, forKey: .conflicts) ?? []
        acked = try container.decodeIfPresent([String: LedgerAck].self, forKey: .acked) ?? [:]
        deletedKeys = try container.decodeIfPresent([String].self, forKey: .deletedKeys) ?? []
        localOnlyKeys = try container.decodeIfPresent([String].self, forKey: .localOnlyKeys) ?? []
        pullCursors = try container.decodeIfPresent([String: LedgerPullCursor].self, forKey: .pullCursors) ?? [:]
        lastPullAtMillis = try container.decodeIfPresent(Int64.self, forKey: .lastPullAtMillis)
        paidUploadsPaused = try container.decodeIfPresent(Bool.self, forKey: .paidUploadsPaused) ?? false
        pauseCode = try container.decodeIfPresent(String.self, forKey: .pauseCode)
        lastErrorCode = try container.decodeIfPresent(String.self, forKey: .lastErrorCode)
        lastSuccessAtMillis = try container.decodeIfPresent(Int64.self, forKey: .lastSuccessAtMillis)
        vehicleLease = try container.decodeIfPresent(VehicleCapabilityLease.self, forKey: .vehicleLease)
        premiumObservation = try container.decodeIfPresent(PremiumCapabilityObservation.self, forKey: .premiumObservation)
    }

    public func encode(to encoder: Encoder) throws {
        var container = encoder.container(keyedBy: CodingKeys.self)
        try container.encode(scope, forKey: .scope)
        try container.encode(enabled, forKey: .enabled)
        try container.encode(consentedDatasets, forKey: .consentedDatasets)
        try container.encodeIfPresent(consentRevision, forKey: .consentRevision)
        try container.encode(commands, forKey: .commands)
        try container.encode(conflicts, forKey: .conflicts)
        try container.encode(acked, forKey: .acked)
        try container.encode(deletedKeys, forKey: .deletedKeys)
        try container.encode(localOnlyKeys, forKey: .localOnlyKeys)
        try container.encode(pullCursors, forKey: .pullCursors)
        try container.encodeIfPresent(lastPullAtMillis, forKey: .lastPullAtMillis)
        try container.encode(paidUploadsPaused, forKey: .paidUploadsPaused)
        try container.encodeIfPresent(pauseCode, forKey: .pauseCode)
        try container.encodeIfPresent(lastErrorCode, forKey: .lastErrorCode)
        try container.encodeIfPresent(lastSuccessAtMillis, forKey: .lastSuccessAtMillis)
        try container.encodeIfPresent(vehicleLease, forKey: .vehicleLease)
        try container.encodeIfPresent(premiumObservation, forKey: .premiumObservation)
    }

    public var pendingCount: Int { commands.filter { $0.status == .pending }.count }
    public var refusedCount: Int { commands.filter { $0.status == .refused }.count }
}

/// Per-record sync label for the ledger list.
public enum LedgerRecordSyncStatus: Equatable, Sendable {
    case localOnly
    case reviewRequired
    case pending
    case synced(revision: Int64)
    case conflict
    case refused(code: String?)
    case paused
    case erasedFromCloud
}

/// One server acknowledgement of a ledger command.
public struct LedgerCommandReceipt: Equatable, Sendable {
    public let operationId: String
    public let dataset: LedgerDataset
    public let recordId: String
    public let revision: Int64
    public let replayed: Bool

    public init(operationId: String, dataset: LedgerDataset, recordId: String, revision: Int64, replayed: Bool) {
        self.operationId = operationId
        self.dataset = dataset
        self.recordId = recordId
        self.revision = revision
        self.replayed = replayed
    }
}

/// One server record from `POST /v1/ledger/records`.
public struct LedgerServerRecord: Equatable, Sendable {
    public let dataset: LedgerDataset
    public let id: String
    public let vehicleId: String
    public let occurredAtMillis: Int64
    public let revision: Int64
    public let voided: Bool
    /// Decoded payload; nil when the server copy cannot be represented safely.
    public let record: LedgerEntryRecord?

    public init(
        dataset: LedgerDataset,
        id: String,
        vehicleId: String,
        occurredAtMillis: Int64,
        revision: Int64,
        voided: Bool,
        record: LedgerEntryRecord?
    ) {
        self.dataset = dataset
        self.id = id
        self.vehicleId = vehicleId
        self.occurredAtMillis = occurredAtMillis
        self.revision = revision
        self.voided = voided
        self.record = record
    }

    public var canonicalPayload: String? {
        record.flatMap { LedgerWire.payloadJson($0) }
    }
}

public struct LedgerRecordsPage: Equatable, Sendable {
    public let items: [LedgerServerRecord]
    /// Opaque; returned verbatim. Nil when the listing is complete.
    public let nextCursor: String?

    public init(items: [LedgerServerRecord], nextCursor: String?) {
        self.items = items
        self.nextCursor = nextCursor
    }
}

public struct LedgerConsentStatus: Equatable, Sendable {
    public let revision: Int64
    public let datasets: [LedgerDataset]

    public init(revision: Int64, datasets: [LedgerDataset]) {
        self.revision = revision
        self.datasets = datasets
    }
}

public enum LedgerSyncError: Error, Equatable, Sendable {
    case signInRequired
    case ownerMismatch
    case invalidCommand
    case invalidReceipt
    case invalidPage
}

/// Network port of the dispatcher, faked in XCTest.
public protocol LedgerSyncGateway: Sendable {
    func submit(_ command: LedgerQueuedCommand, accountId: String) async throws -> LedgerCommandReceipt
    func listRecords(
        accountId: String,
        dataset: LedgerDataset,
        vehicleId: String?,
        limit: Int,
        cursor: String?
    ) async throws -> LedgerRecordsPage
    func setConsent(accountId: String, datasets: [LedgerDataset], enabled: Bool) async throws -> LedgerConsentStatus
    func eraseAll(accountId: String) async throws
}

public enum LedgerFailure: Equatable, Sendable {
    case conflict
    case capabilityRequired(code: String)
    case retry(code: String, notBeforeMillis: Int64?)
    case refused(code: String)
    case signInRequired
}

public struct LedgerPullReport: Equatable, Sendable {
    public var added: Int = 0
    public var updated: Int = 0
    public var unchanged: Int = 0
    public var skippedDeleted: Int = 0
    public var skippedVoided: Int = 0
    public var skippedHeld: Int = 0
    public var undecodable: Int = 0
    public var pages: Int = 0
    /// More pages remain; the saved cursor continues the pull.
    public var truncated: Bool = false

    public init() {}
}

public enum LedgerConflictChoice: String, Sendable {
    case keepMine
    case takeServer
}

public enum LedgerResolution: Equatable, Sendable {
    /// The local version is re-queued against the current server revision.
    case requeued(LedgerSyncState)
    /// The server version replaces the local record.
    case adoptServer(LedgerSyncState, LedgerEntryRecord)
    /// The server no longer holds the record; the local copy is removed.
    case removeLocal(LedgerSyncState)
    case unavailable(String)
}

public enum LedgerSyncEngine {
    public static let maxDeletedKeys = 5000
    public static let baseBackoffMillis: Int64 = 1000
    public static let maxBackoffMillis: Int64 = 3_600_000
    public static let pageLimit = 100
    public static let cursorReuseMillis: Int64 = 14 * 60 * 1000

    // MARK: Operation IDs (Android `LedgerCommandFactory`)

    public static func operationId(
        scope: LedgerSyncScope,
        dataset: LedgerDataset,
        recordId: String,
        expectedRevision: Int64,
        payloadJson: String
    ) -> String? {
        let seed = "ledger-command|\(scope.accountId)|\(scope.origin)|\(dataset.rawValue)|\(recordId)|\(expectedRevision)|\(payloadJson)"
        return LedgerIds.nameBased(seed)
    }

    public static func deleteOperationId(scope: LedgerSyncScope, dataset: LedgerDataset, recordId: String) -> String? {
        return LedgerIds.nameBased("ledger-delete|\(scope.accountId)|\(scope.origin)|\(dataset.rawValue)|\(recordId)")
    }

    // MARK: Opt-in and consent

    /// Applies the server consent answer; disabling also stops this device's uploads.
    public static func applyConsent(_ state: LedgerSyncState, status: LedgerConsentStatus, enabled: Bool) -> LedgerSyncState {
        var next = state
        next.enabled = enabled && !status.datasets.isEmpty
        next.consentedDatasets = LedgerDataset.allCases.filter { status.datasets.contains($0) }
        next.consentRevision = status.revision
        next.lastErrorCode = nil
        return next
    }

    /// Local opt-out without network: queued work stays held, nothing is sent.
    public static func disableLocally(_ state: LedgerSyncState) -> LedgerSyncState {
        var next = state
        next.enabled = false
        return next
    }

    // MARK: Queue

    /// Brings the queue in line with the local records: one upsert per changed, syncable, consented
    /// record at its acknowledged revision. Unchanged commands keep their retry state; superseded
    /// ones are replaced; conflicts only update their proposed version.
    public static func reconcile(_ state: LedgerSyncState, records: [LedgerEntryRecord], nowMillis: Int64) -> LedgerSyncState {
        guard state.enabled else { return state }
        var next = state
        let consented = Set(state.consentedDatasets)
        let localOnly = Set(state.localOnlyKeys)
        let deleted = Set(state.deletedKeys)
        var desired: [String: LedgerQueuedCommand] = [:]
        var desiredOrder: [String] = []
        let chronological = records.sorted { lhs, rhs in
            lhs.occurredAtMillis != rhs.occurredAtMillis ? lhs.occurredAtMillis < rhs.occurredAtMillis : lhs.id < rhs.id
        }
        for record in chronological {
            guard let dataset = record.dataset, consented.contains(dataset), LedgerRecordRules.isSyncable(record),
                  let payload = LedgerWire.payloadJson(record) else {
                continue
            }
            let key = LedgerSyncKeys.key(dataset, record.id)
            if localOnly.contains(key) || deleted.contains(key) { continue }
            if let index = next.conflicts.firstIndex(where: { $0.key == key }) {
                next.conflicts[index].proposedPayloadJson = payload
                continue
            }
            let ack = next.acked[key]
            if ack?.payloadJson == payload { continue }
            let expected = ack?.revision ?? 0
            guard let operation = operationId(
                scope: state.scope,
                dataset: dataset,
                recordId: record.id,
                expectedRevision: expected,
                payloadJson: payload
            ) else {
                continue
            }
            if desired[key] == nil { desiredOrder.append(key) }
            desired[key] = LedgerQueuedCommand(
                operationId: operation,
                dataset: dataset,
                recordId: record.id,
                expectedRevision: expected,
                isDelete: false,
                payloadJson: payload,
                createdAtMillis: nowMillis
            )
        }
        var rebuilt: [LedgerQueuedCommand] = []
        var placed = Set<String>()
        for command in next.commands {
            if command.isDelete {
                rebuilt.append(command)
                continue
            }
            if let wanted = desired[command.key], wanted.operationId == command.operationId, !placed.contains(command.key) {
                rebuilt.append(command)
                placed.insert(command.key)
            }
        }
        for key in desiredOrder where !placed.contains(key) {
            if let wanted = desired[key] {
                rebuilt.append(wanted)
                placed.insert(key)
            }
        }
        next.commands = rebuilt
        return next
    }

    /// A local delete: tombstone fence, no further upserts, and a free delete command whenever the
    /// server may hold the record. A delete of a record the server never saw answers 409 and is dropped.
    public static func markDeleted(_ state: LedgerSyncState, dataset: LedgerDataset, recordId: String, nowMillis: Int64) -> LedgerSyncState {
        let key = LedgerSyncKeys.key(dataset, recordId)
        var next = state
        let hadConflict = next.conflicts.contains { $0.key == key }
        let hadCommand = next.commands.contains { $0.key == key }
        next.commands.removeAll { $0.key == key && !$0.isDelete }
        next.conflicts.removeAll { $0.key == key }
        appendDeletedKey(&next, key)
        let serverMayHold = next.acked[key] != nil || hadConflict || hadCommand
        let erasedFromCloud = next.localOnlyKeys.contains(key)
        if serverMayHold && !erasedFromCloud {
            enqueueDelete(&next, dataset: dataset, recordId: recordId, nowMillis: nowMillis)
        }
        return next
    }

    private static func enqueueDelete(_ state: inout LedgerSyncState, dataset: LedgerDataset, recordId: String, nowMillis: Int64) {
        let key = LedgerSyncKeys.key(dataset, recordId)
        guard !state.commands.contains(where: { $0.key == key && $0.isDelete }),
              let operation = deleteOperationId(scope: state.scope, dataset: dataset, recordId: recordId) else {
            return
        }
        state.commands.append(
            LedgerQueuedCommand(
                operationId: operation,
                dataset: dataset,
                recordId: recordId,
                expectedRevision: 0,
                isDelete: true,
                payloadJson: "",
                createdAtMillis: nowMillis
            )
        )
    }

    private static func appendDeletedKey(_ state: inout LedgerSyncState, _ key: String) {
        state.deletedKeys.removeAll { $0 == key }
        state.deletedKeys.append(key)
        if state.deletedKeys.count > maxDeletedKeys {
            state.deletedKeys.removeFirst(state.deletedKeys.count - maxDeletedKeys)
        }
    }

    /// Next command to send. Paid upserts need opt-in, consent and no capability pause; deletes are free.
    public static func nextDispatchable(_ state: LedgerSyncState, nowMillis: Int64) -> LedgerQueuedCommand? {
        guard state.enabled else { return nil }
        let consented = Set(state.consentedDatasets)
        return state.commands.first { command in
            guard command.status == .pending, command.nextAttemptAtMillis <= nowMillis else { return false }
            if command.isDelete { return true }
            return consented.contains(command.dataset) && !state.paidUploadsPaused
        }
    }

    public static func nextWakeMillis(_ state: LedgerSyncState) -> Int64? {
        guard state.enabled else { return nil }
        return state.commands
            .filter { $0.status == .pending && (!state.paidUploadsPaused || $0.isDelete) }
            .map { $0.nextAttemptAtMillis }
            .min()
    }

    public static func backoffMillis(attempts: Int) -> Int64 {
        var delay = baseBackoffMillis
        var step = 1
        while step < attempts && delay < maxBackoffMillis {
            delay = min(delay * 2, maxBackoffMillis)
            step += 1
        }
        return delay
    }

    // MARK: Results

    public static func applyReceipt(
        _ state: LedgerSyncState,
        command: LedgerQueuedCommand,
        receipt: LedgerCommandReceipt,
        nowMillis: Int64
    ) -> LedgerSyncState {
        var next = state
        let key = command.key
        next.commands.removeAll { $0.operationId == command.operationId }
        if command.isDelete {
            next.acked[key] = nil
        } else {
            next.acked[key] = LedgerAck(revision: receipt.revision, payloadJson: command.payloadJson, atMillis: nowMillis)
            next.paidUploadsPaused = false
            next.pauseCode = nil
            if next.deletedKeys.contains(key) {
                enqueueDelete(&next, dataset: command.dataset, recordId: command.recordId, nowMillis: nowMillis)
            }
        }
        next.lastSuccessAtMillis = nowMillis
        next.lastErrorCode = nil
        return next
    }

    public static func applyFailure(
        _ state: LedgerSyncState,
        command: LedgerQueuedCommand,
        failure: LedgerFailure,
        current: LedgerCurrentVersion,
        nowMillis: Int64
    ) -> LedgerSyncState {
        var next = state
        let key = command.key
        guard let index = next.commands.firstIndex(where: { $0.operationId == command.operationId }) else {
            return next
        }
        switch failure {
        case .conflict:
            next.commands.remove(at: index)
            if command.isDelete {
                next.acked[key] = nil
                return next
            }
            next.conflicts.removeAll { $0.key == key }
            next.conflicts.append(
                LedgerConflict(
                    dataset: command.dataset,
                    recordId: command.recordId,
                    operationId: command.operationId,
                    expectedRevision: command.expectedRevision,
                    basePayloadJson: next.acked[key]?.payloadJson,
                    proposedPayloadJson: command.payloadJson,
                    current: current,
                    detectedAtMillis: nowMillis
                )
            )
            next.lastErrorCode = "ledger.conflict"
        case .capabilityRequired(let code):
            if command.isDelete {
                next.commands[index].status = .refused
                next.commands[index].lastCode = code
            } else {
                next.paidUploadsPaused = true
                next.pauseCode = code
            }
            next.lastErrorCode = code
        case .retry(let code, let notBefore):
            let attempts = next.commands[index].attempts + 1
            next.commands[index].attempts = attempts
            next.commands[index].nextAttemptAtMillis = max(nowMillis + backoffMillis(attempts: attempts), notBefore ?? 0)
            next.commands[index].lastCode = code
            next.lastErrorCode = code
        case .refused(let code):
            next.commands[index].status = .refused
            next.commands[index].lastCode = code
            next.lastErrorCode = code
        case .signInRequired:
            next.lastErrorCode = "ledger.sign-in-required"
        }
        return next
    }

    /// Maps transport and wire failures; 409 conflicts, 403 lapses, 400/404 refusals and the rest retry.
    public static func classify(_ error: Error, nowMillis: Int64) -> LedgerFailure {
        if let sync = error as? LedgerSyncError {
            switch sync {
            case .signInRequired, .ownerMismatch: return .signInRequired
            case .invalidCommand: return .refused(code: "ledger.invalid-command")
            case .invalidReceipt: return .retry(code: "ledger.invalid-receipt", notBeforeMillis: nil)
            case .invalidPage: return .retry(code: "ledger.invalid-records", notBeforeMillis: nil)
            }
        }
        if let transport = error as? SignedTransportError {
            switch transport {
            case .offline: return .retry(code: "ledger.offline", notBeforeMillis: nil)
            case .oversizedResponse, .invalidResponse: return .retry(code: "ledger.invalid-response", notBeforeMillis: nil)
            case .identityUnavailable, .signingUnavailable: return .retry(code: "ledger.identity-unavailable", notBeforeMillis: nil)
            case .originUnavailable: return .refused(code: "ledger.origin-unavailable")
            case .invalidRequest: return .refused(code: "ledger.invalid-request")
            case .sessionRequired: return .signInRequired
            case .refused(let status, let code, let retryAfter):
                let notBefore = retryAfter.map { nowMillis + min($0, 86_400) * 1000 }
                switch status {
                case 409: return .conflict
                case 403: return .capabilityRequired(code: code ?? "ledger.capability-required")
                case 401, 408, 425, 429: return .retry(code: code ?? "http-\(status)", notBeforeMillis: notBefore)
                case 500...599: return .retry(code: code ?? "http-\(status)", notBeforeMillis: notBefore)
                default: return .refused(code: code ?? "http-\(status)")
                }
            }
        }
        return .retry(code: "ledger.unavailable", notBeforeMillis: nil)
    }

    // MARK: Conflict resolution (Android `ResolveLedgerConflictUseCase`)

    public static func resolve(
        _ state: LedgerSyncState,
        conflictKey: String,
        choice: LedgerConflictChoice,
        localRecord: LedgerEntryRecord?,
        nowMillis: Int64
    ) -> LedgerResolution {
        guard let conflict = state.conflicts.first(where: { $0.key == conflictKey }) else {
            return .unavailable("ledger.no-conflict")
        }
        var next = state
        let current = conflict.current
        switch choice {
        case .keepMine:
            guard current.state == .present, !current.voided, let revision = current.revision else {
                return .unavailable(current.state == .unknown ? "ledger.current-unknown" : "ledger.current-missing")
            }
            guard let local = localRecord, LedgerRecordRules.isSyncable(local), LedgerWire.payloadJson(local) != nil else {
                return .unavailable("ledger.local-invalid")
            }
            next.conflicts.removeAll { $0.key == conflictKey }
            next.commands.removeAll { $0.key == conflictKey && !$0.isDelete }
            next.acked[conflictKey] = LedgerAck(revision: revision, payloadJson: current.payloadJson ?? "", atMillis: nowMillis)
            return .requeued(next)
        case .takeServer:
            if current.state == .unknown { return .unavailable("ledger.current-unknown") }
            next.conflicts.removeAll { $0.key == conflictKey }
            next.commands.removeAll { $0.key == conflictKey }
            if current.state == .missing || current.voided {
                next.acked[conflictKey] = nil
                appendDeletedKey(&next, conflictKey)
                return .removeLocal(next)
            }
            guard let revision = current.revision, let payload = current.payloadJson,
                  let server = LedgerWire.decodePayload(json: payload, dataset: conflict.dataset),
                  server.id == conflict.recordId else {
                return .unavailable("ledger.current-undecodable")
            }
            next.acked[conflictKey] = LedgerAck(revision: revision, payloadJson: payload, atMillis: nowMillis)
            return .adoptServer(next, server.keepingLocalMarkers(of: localRecord))
        }
    }

    public static func updateConflictCurrent(_ state: LedgerSyncState, conflictKey: String, current: LedgerCurrentVersion) -> LedgerSyncState {
        var next = state
        if let index = next.conflicts.firstIndex(where: { $0.key == conflictKey }) {
            next.conflicts[index].current = current
        }
        return next
    }

    /// Owner retry of refused commands and the capability pause.
    public static func retryNow(_ state: LedgerSyncState) -> LedgerSyncState {
        var next = state
        next.paidUploadsPaused = false
        next.pauseCode = nil
        for index in next.commands.indices {
            next.commands[index].status = .pending
            next.commands[index].nextAttemptAtMillis = 0
        }
        return next
    }

    /// Owner cancellation of queued uploads (B-BR-PR04); local records are untouched.
    public static func discardQueued(_ state: LedgerSyncState) -> LedgerSyncState {
        var next = state
        next.commands.removeAll { !$0.isDelete }
        return next
    }

    /// After a free cloud erasure every known record stays only on this device.
    public static func applyErasure(_ state: LedgerSyncState, records: [LedgerEntryRecord]) -> LedgerSyncState {
        var next = state
        var keys = Set(next.localOnlyKeys)
        for record in records {
            if let key = record.syncKey { keys.insert(key) }
        }
        for key in next.acked.keys { keys.insert(key) }
        next.localOnlyKeys = keys.sorted()
        next.acked = [:]
        next.commands = []
        next.conflicts = []
        next.pullCursors = [:]
        next.enabled = false
        next.consentedDatasets = []
        next.paidUploadsPaused = false
        next.pauseCode = nil
        return next
    }

    // MARK: Pull (paged restore, Android `PreviewLedgerRestoreUseCase` plus newer-revision adoption)

    /// Merges one page: adds missing records, adopts newer server revisions of clean local records,
    /// and never removes, overwrites held work or resurrects a local delete.
    public static func mergePage(
        _ state: LedgerSyncState,
        records: [LedgerEntryRecord],
        page: [LedgerServerRecord],
        report: LedgerPullReport,
        nowMillis: Int64
    ) -> (LedgerSyncState, [LedgerEntryRecord], LedgerPullReport) {
        var next = state
        var local = records
        var summary = report
        let deleted = Set(state.deletedKeys)
        let held = Set(state.commands.map { $0.key } + state.conflicts.map { $0.key })
        for row in page {
            let key = LedgerSyncKeys.key(row.dataset, row.id)
            if deleted.contains(key) {
                summary.skippedDeleted += 1
                continue
            }
            if row.voided {
                summary.skippedVoided += 1
                continue
            }
            guard let server = row.record, server.id == row.id, server.vehicleId == row.vehicleId,
                  server.dataset == row.dataset, let canonical = row.canonicalPayload else {
                summary.undecodable += 1
                continue
            }
            if held.contains(key) {
                summary.skippedHeld += 1
                continue
            }
            if let index = local.firstIndex(where: { $0.id == row.id && $0.dataset == row.dataset }) {
                let existing = local[index]
                let existingPayload = LedgerWire.payloadJson(existing)
                let ack = next.acked[key]
                if let ack, row.revision <= ack.revision {
                    summary.unchanged += 1
                    continue
                }
                if existingPayload == canonical {
                    next.acked[key] = LedgerAck(revision: row.revision, payloadJson: canonical, atMillis: nowMillis)
                    summary.unchanged += 1
                } else if let ack, existingPayload == ack.payloadJson {
                    local[index] = server.keepingLocalMarkers(of: existing)
                    next.acked[key] = LedgerAck(revision: row.revision, payloadJson: canonical, atMillis: nowMillis)
                    summary.updated += 1
                } else {
                    summary.skippedHeld += 1
                }
            } else {
                local.append(server)
                next.acked[key] = LedgerAck(revision: row.revision, payloadJson: canonical, atMillis: nowMillis)
                summary.added += 1
            }
        }
        summary.pages += 1
        return (next, local, summary)
    }

    /// The saved cursor of `dataset` while it is still inside the server's 15-minute window.
    public static func resumableCursor(_ state: LedgerSyncState, dataset: LedgerDataset, nowMillis: Int64) -> String? {
        guard let saved = state.pullCursors[dataset.rawValue] else { return nil }
        let age = nowMillis - saved.savedAtMillis
        return age >= 0 && age <= cursorReuseMillis ? saved.cursor : nil
    }

    public static func status(
        of record: LedgerEntryRecord,
        in state: LedgerSyncState?
    ) -> LedgerRecordSyncStatus {
        if record.reviewRequired { return .reviewRequired }
        guard let key = record.syncKey, let state else { return .localOnly }
        if state.localOnlyKeys.contains(key) { return .erasedFromCloud }
        if state.conflicts.contains(where: { $0.key == key }) { return .conflict }
        if let command = state.commands.first(where: { $0.key == key && !$0.isDelete }) {
            if command.status == .refused { return .refused(code: command.lastCode) }
            return state.paidUploadsPaused ? .paused : .pending
        }
        if let ack = state.acked[key], ack.payloadJson == LedgerWire.payloadJson(record) {
            return .synced(revision: ack.revision)
        }
        return .localOnly
    }
}

/// Dispatch steps with an injected gateway; the caller applies each result to its freshest state.
public struct LedgerDispatchResult: Equatable, Sendable {
    public let command: LedgerQueuedCommand
    public let receipt: LedgerCommandReceipt?
    public let failure: LedgerFailure?
    public let current: LedgerCurrentVersion
}

public enum LedgerSyncDispatcher {
    public static func dispatch(
        _ command: LedgerQueuedCommand,
        scope: LedgerSyncScope,
        gateway: LedgerSyncGateway,
        nowMillis: Int64
    ) async -> LedgerDispatchResult {
        do {
            let receipt = try await gateway.submit(command, accountId: scope.accountId)
            guard receipt.operationId == command.operationId, receipt.recordId == command.recordId,
                  receipt.dataset == command.dataset else {
                return LedgerDispatchResult(
                    command: command,
                    receipt: nil,
                    failure: .retry(code: "ledger.invalid-receipt", notBeforeMillis: nil),
                    current: .unknown
                )
            }
            return LedgerDispatchResult(command: command, receipt: receipt, failure: nil, current: .unknown)
        } catch {
            let failure = LedgerSyncEngine.classify(error, nowMillis: nowMillis)
            var current = LedgerCurrentVersion.unknown
            if case .conflict = failure, !command.isDelete {
                let vehicleId = LedgerWire.decodePayload(json: command.payloadJson, dataset: command.dataset)?.vehicleId
                current = await lookupCurrent(
                    dataset: command.dataset,
                    recordId: command.recordId,
                    vehicleId: vehicleId,
                    scope: scope,
                    gateway: gateway
                )
            }
            return LedgerDispatchResult(command: command, receipt: nil, failure: failure, current: current)
        }
    }

    public static func apply(
        _ result: LedgerDispatchResult,
        to state: LedgerSyncState,
        records: [LedgerEntryRecord],
        nowMillis: Int64
    ) -> LedgerSyncState {
        if let receipt = result.receipt {
            let acknowledged = LedgerSyncEngine.applyReceipt(state, command: result.command, receipt: receipt, nowMillis: nowMillis)
            return LedgerSyncEngine.reconcile(acknowledged, records: records, nowMillis: nowMillis)
        }
        if let failure = result.failure {
            return LedgerSyncEngine.applyFailure(
                state,
                command: result.command,
                failure: failure,
                current: result.current,
                nowMillis: nowMillis
            )
        }
        return state
    }

    /// Reads the server's current version through bounded pages (at most 5 x 100 records).
    public static func lookupCurrent(
        dataset: LedgerDataset,
        recordId: String,
        vehicleId: String?,
        scope: LedgerSyncScope,
        gateway: LedgerSyncGateway
    ) async -> LedgerCurrentVersion {
        var cursor: String?
        var seen = Set<String>()
        for _ in 0..<5 {
            guard let page = try? await gateway.listRecords(
                accountId: scope.accountId,
                dataset: dataset,
                vehicleId: vehicleId,
                limit: LedgerSyncEngine.pageLimit,
                cursor: cursor
            ) else {
                return .unknown
            }
            if let row = page.items.first(where: { $0.id == recordId }) {
                return LedgerCurrentVersion(
                    state: .present,
                    revision: row.revision,
                    payloadJson: row.canonicalPayload,
                    voided: row.voided
                )
            }
            guard let next = page.nextCursor else { return .missing }
            if !seen.insert(next).inserted { return .unknown }
            cursor = next
        }
        return .unknown
    }

    /// Sequential drain used by tests and simple callers: sends up to `limit` commands.
    public static func drain(
        _ state: LedgerSyncState,
        records: [LedgerEntryRecord],
        gateway: LedgerSyncGateway,
        nowMillis: Int64,
        limit: Int = 20
    ) async -> LedgerSyncState {
        var current = state
        for _ in 0..<max(0, limit) {
            guard let command = LedgerSyncEngine.nextDispatchable(current, nowMillis: nowMillis) else { break }
            let result = await dispatch(command, scope: current.scope, gateway: gateway, nowMillis: nowMillis)
            current = apply(result, to: current, records: records, nowMillis: nowMillis)
            if case .some(.signInRequired) = result.failure { break }
            if case .some(.retry) = result.failure { break }
        }
        return current
    }
}

/// File persistence of every account and origin state, excluded from device backup (SEC05-D).
public final class LedgerSyncStore: @unchecked Sendable {
    private let fileUrl: URL
    private let lock = NSLock()

    public init(directory: URL? = nil) {
        let baseDir = directory ?? IosLocalStore.defaultDirectory()
        try? FileManager.default.createDirectory(at: baseDir, withIntermediateDirectories: true)
        self.fileUrl = baseDir.appendingPathComponent("ledger-sync-v1.json")
    }

    public func load(scope: LedgerSyncScope) -> LedgerSyncState {
        lock.lock()
        defer { lock.unlock() }
        guard let state = readAll()[scope.key], state.scope == scope else {
            return LedgerSyncState(scope: scope)
        }
        return state
    }

    public func save(_ state: LedgerSyncState) {
        lock.lock()
        defer { lock.unlock() }
        var all = readAll()
        all[state.scope.key] = state
        writeAll(all)
    }

    /// Logout and deletion drop the owner's queue, acknowledgements and cursors on every origin.
    public func purge(accountId: String) {
        lock.lock()
        defer { lock.unlock() }
        let kept = readAll().filter { $0.value.scope.accountId != accountId }
        writeAll(kept)
    }

    public func purgeAll() {
        lock.lock()
        defer { lock.unlock() }
        try? FileManager.default.removeItem(at: fileUrl)
    }

    private func readAll() -> [String: LedgerSyncState] {
        guard let data = try? Data(contentsOf: fileUrl),
              let decoded = try? JSONDecoder().decode([String: LedgerSyncState].self, from: data) else {
            return [:]
        }
        return decoded
    }

    private func writeAll(_ states: [String: LedgerSyncState]) {
        guard let data = try? JSONEncoder().encode(states) else { return }
        try? data.write(to: fileUrl, options: .atomic)
        var url = fileUrl
        var values = URLResourceValues()
        values.isExcludedFromBackup = true
        try? url.setResourceValues(values)
    }
}

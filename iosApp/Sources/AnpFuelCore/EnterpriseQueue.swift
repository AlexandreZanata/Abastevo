import Foundation

/// Delivery state of one protected company command. No send attempt becomes acceptance.
public enum EnterpriseQueueStatus: String, Codable, Sendable {
    /// Saved on this device; waiting for (re)delivery.
    case pending = "PENDING"
    /// ACCEPTED by the server with an entity revision and receipt time.
    case accepted = "ACCEPTED"
    /// ACCEPTED_FOR_REVIEW: received, a company reviewer decides.
    case inReview = "IN_REVIEW"
    /// REJECTED: a final refusal with its server code.
    case rejected = "REJECTED"
    /// The bounded automatic retries were used; held until the person retries.
    case exhausted = "EXHAUSTED"

    public var isFinal: Bool {
        return self == .accepted || self == .inReview || self == .rejected
    }

    /// The server received the command (accepted or kept for review).
    public var isReceived: Bool {
        return self == .accepted || self == .inReview
    }
}

public enum EnterpriseCommandKind: String, Codable, Sendable {
    case tripStart = "TRIP_START"
    case missionResponse = "MISSION_RESPONSE"
    case tripLifecycle = "TRIP_LIFECYCLE"
    case tripFinish = "TRIP_FINISH"
    case expense = "EXPENSE"
    case fueling = "FUELING"
    case checklist = "CHECKLIST"
}

public struct EnterpriseQueueItem: Codable, Equatable, Sendable, Identifiable {
    public let operation: EnterpriseSealedOperation
    public let kind: EnterpriseCommandKind
    /// The person's own local description of the command, shown only on this device.
    public let summary: String
    public let createdAtMillis: Int64
    public var status: EnterpriseQueueStatus
    public var attempts: Int
    public var nextAttemptAtMillis: Int64
    public var lastCode: String?
    public var receivedAt: String?
    public var entityRevision: Int64?
    public var dismissedFromQueue: Bool?

    public var id: String { operation.operationId }

    public init(operation: EnterpriseSealedOperation, kind: EnterpriseCommandKind, summary: String, createdAtMillis: Int64) {
        self.operation = operation
        self.kind = kind
        self.summary = summary
        self.createdAtMillis = createdAtMillis
        self.status = .pending
        self.attempts = 0
        self.nextAttemptAtMillis = 0
        self.lastCode = nil
        self.receivedAt = nil
        self.entityRevision = nil
        self.dismissedFromQueue = nil
    }
}

public enum EnterpriseAttachmentStatus: String, Codable, Sendable {
    /// Waiting until the owning command is received by the server.
    case waitingOwner = "WAITING_OWNER"
    case pending = "PENDING"
    /// The server holds the content (RECEIVED or STORED); the local copy is gone.
    case delivered = "DELIVERED"
    /// A final refusal or a quarantined, failed, replaced or deleted document.
    case refused = "REFUSED"
    case exhausted = "EXHAUSTED"
}

/// One receipt photo declared for an expense; bytes live in a protected file until delivered.
public struct EnterpriseAttachmentItem: Codable, Equatable, Sendable, Identifiable {
    public let documentId: String
    public let declarationId: String
    public let linkId: String
    public let sha256: String
    public let bytes: Int64
    public let targetKind: String
    public let targetId: String
    public let ownerOperationId: String
    public var status: EnterpriseAttachmentStatus
    public var attempts: Int
    public var nextAttemptAtMillis: Int64
    public var lastCode: String?

    public var id: String { documentId }

    public init(documentId: String, declarationId: String, linkId: String, sha256: String, bytes: Int64,
                targetKind: String, targetId: String, ownerOperationId: String) {
        self.documentId = documentId
        self.declarationId = declarationId
        self.linkId = linkId
        self.sha256 = sha256
        self.bytes = bytes
        self.targetKind = targetKind
        self.targetId = targetId
        self.ownerOperationId = ownerOperationId
        self.status = .waitingOwner
        self.attempts = 0
        self.nextAttemptAtMillis = 0
        self.lastCode = nil
    }
}

/// A trip started on this device. The server decides its state; this is the local logbook copy.
public struct EnterpriseTripRecord: Codable, Equatable, Sendable, Identifiable {
    public let id: String
    public let assetId: String
    public let assetLabel: String
    public let startOperationId: String
    public let startedAtMillis: Int64
    public let startMeterId: String?
    public let missionId: String?
    /// The departure checklist run the start names; absent in snapshots written before it was kept.
    public let departureRunId: String?
    public var lifecycle: [EnterpriseTripLifecycleFact]?
    public var finishOperationId: String?
    public var finishedAtMillis: Int64?

    public var isOpen: Bool { localState == .running || localState == .paused }

    public init(id: String, assetId: String, assetLabel: String, startOperationId: String, startedAtMillis: Int64, startMeterId: String?,
                missionId: String? = nil, departureRunId: String? = nil) {
        self.id = id
        self.assetId = assetId
        self.assetLabel = assetLabel
        self.startOperationId = startOperationId
        self.startedAtMillis = startedAtMillis
        self.startMeterId = startMeterId
        self.missionId = missionId
        self.departureRunId = departureRunId
        self.lifecycle = nil
        self.finishOperationId = nil
        self.finishedAtMillis = nil
    }
}

/// Everything this device holds for one organization partition.
public struct EnterprisePartitionSnapshot: Codable, Equatable, Sendable {
    public static let currentVersion = 1

    public let schemaVersion: Int
    public let partition: EnterprisePartition
    public var membershipId: String
    public var enrollment: EnterpriseEnrollment?
    /// ACCESS_ENDED: no new command; held work is still offered to the server.
    public var locked: Bool
    public var queue: [EnterpriseQueueItem]
    public var attachments: [EnterpriseAttachmentItem]
    public var trips: [EnterpriseTripRecord]
    public var reference: EnterpriseReferenceCache?
    public var missionInbox: EnterpriseMissionInbox?
    public var returnChecklists: EnterpriseReturnChecklistBook?

    public init(partition: EnterprisePartition, membershipId: String) {
        self.schemaVersion = Self.currentVersion
        self.partition = partition
        self.membershipId = membershipId
        self.enrollment = nil
        self.locked = false
        self.queue = []
        self.attachments = []
        self.trips = []
        self.reference = nil
        self.missionInbox = nil
    }

    /// The trip in progress; a trip whose start the server refused is not in progress.
    public var openTrip: EnterpriseTripRecord? {
        return trips.last { trip in
            trip.isOpen && queue.first(where: { $0.id == trip.startOperationId })?.status != .rejected
        }
    }

    public var unsentCount: Int {
        return queue.filter { !$0.status.isFinal }.count + attachments.filter { $0.status == .pending || $0.status == .waitingOwner || $0.status == .exhausted }.count
    }

    /// Commands that count against the queue bound; dismissed fuelings stay only as the local fueling history.
    public var retainedCommandCount: Int {
        return queue.filter { $0.dismissedFromQueue != true }.count
    }
}

public enum EnterpriseQueueError: Error, Equatable, Sendable {
    case locked
    case full
    case duplicate
    case foreignPartition
}

/// Pure queue transitions: creation order, bounded retries with backoff and `Retry-After`,
/// final receipts and isolation by partition. Mirrors Android `EnterpriseOperationQueue`
/// except the explicit attempt bound required on iOS.
/// Zero postfix unwrap operators anywhere in this file.
public enum EnterpriseQueue {
    public static let maxItems = 500
    public static let maxAttempts = 25
    public static let batchLimit = 10
    public static let baseBackoffMillis: Int64 = 30_000
    /// Equal to the Android queue backoff cap (6 hours).
    public static let maxBackoffMillis: Int64 = 21_600_000
    /// Local final code of a trip command whose start the server refused; it is never sent.
    public static let startRejectedCode = "START_REJECTED"

    public static func enqueue(_ item: EnterpriseQueueItem, into snapshot: inout EnterprisePartitionSnapshot) throws {
        if snapshot.locked { throw EnterpriseQueueError.locked }
        // Fueling history is retained like Android (EDR-295); a dismissed fueling no longer fills the queue.
        if snapshot.retainedCommandCount >= maxItems { throw EnterpriseQueueError.full }
        if snapshot.queue.contains(where: { $0.id == item.id }) { throw EnterpriseQueueError.duplicate }
        snapshot.queue.append(item)
    }

    /// Due commands in creation order. A held trip chain does not prevent an unrelated delivery.
    public static func dueBatch(_ snapshot: EnterprisePartitionSnapshot, nowMillis: Int64, limit: Int = batchLimit) -> [EnterpriseQueueItem] {
        var batch: [EnterpriseQueueItem] = []
        for scheduled in commandSchedule(snapshot) where scheduled.at <= nowMillis {
            if batch.count >= limit { break }
            let item = scheduled.item
            if item.kind == .missionResponse {
                return batch.isEmpty ? [item] : batch
            }
            batch.append(item)
        }
        return batch
    }

    /// The command each trip command waits for: a lifecycle step or finish waits for the step before it,
    /// and a start waits for the departure checklist run it names (Android EDR-352).
    static func prerequisites(_ snapshot: EnterprisePartitionSnapshot) -> [String: String] {
        var prerequisite: [String: String] = [:]
        for trip in snapshot.trips {
            if let run = trip.departureRunId,
               let checklist = snapshot.queue.first(where: {
                   $0.kind == .checklist && $0.operation.entityType == "checklist_run" && $0.operation.entityId == run
               }) {
                prerequisite[trip.startOperationId] = checklist.id
            }
            var latest = trip.startOperationId
            for fact in trip.lifecycle ?? [] {
                prerequisite[fact.operationId] = latest
                latest = fact.operationId
            }
            if let finish = trip.finishOperationId { prerequisite[finish] = latest }
        }
        return prerequisite
    }

    private static func commandSchedule(_ snapshot: EnterprisePartitionSnapshot) -> [(item: EnterpriseQueueItem, at: Int64)] {
        let prerequisite = prerequisites(snapshot)
        let status = Dictionary(snapshot.queue.map { ($0.id, $0.status) }, uniquingKeysWith: { first, _ in first })
        return snapshot.queue.compactMap { item in
            guard item.status == .pending else { return nil }
            // A predecessor still held on this device (pending, even when due in the same pass, or
            // paused) holds its dependent. A final or dismissed predecessor releases it: the server
            // answers DEPENDENCY_PENDING or holds a stale revision for review, as on Android.
            if let prior = prerequisite[item.id].flatMap({ status[$0] }), !prior.isFinal { return nil }
            return (item, item.nextAttemptAtMillis)
        }
    }

    /// Applies the acknowledgements of one delivered batch.
    public static func apply(_ acknowledgements: [EnterpriseAcknowledgement], to snapshot: inout EnterprisePartitionSnapshot,
                             nowMillis: Int64, retryAfterMillis: Int64? = nil) {
        for ack in acknowledgements {
            guard let index = snapshot.queue.firstIndex(where: { $0.id == ack.operationId && $0.operation.payloadDigest == ack.payloadDigest }) else {
                continue
            }
            var item = snapshot.queue[index]
            switch ack.result {
            case .accepted:
                item.status = .accepted
                item.entityRevision = ack.entityRevision
                item.receivedAt = ack.receivedAt
                item.lastCode = nil
            case .acceptedForReview:
                item.status = .inReview
                item.receivedAt = ack.receivedAt
                item.lastCode = ack.code
            case .rejected:
                item.status = .rejected
                item.receivedAt = ack.receivedAt
                item.lastCode = ack.code
            case .retry:
                item = failed(item, code: ack.code, nowMillis: nowMillis, retryAfterMillis: retryAfterMillis)
            }
            snapshot.queue[index] = item
        }
        refuseCommandsOfRejectedStarts(in: &snapshot)
        releaseAttachments(in: &snapshot)
    }

    /// A trip whose start the server refused never exists there: its unsent lifecycle steps and finish
    /// become final locally with `START_REJECTED`, so they are never sent and can be dismissed.
    public static func refuseCommandsOfRejectedStarts(in snapshot: inout EnterprisePartitionSnapshot) {
        for trip in snapshot.trips where snapshot.queue.first(where: { $0.id == trip.startOperationId })?.status == .rejected {
            var dependents = Set((trip.lifecycle ?? []).map { $0.operationId })
            if let finish = trip.finishOperationId { dependents.insert(finish) }
            for index in snapshot.queue.indices where dependents.contains(snapshot.queue[index].id) {
                let status = snapshot.queue[index].status
                guard status == .pending || status == .exhausted else { continue }
                snapshot.queue[index].status = .rejected
                snapshot.queue[index].lastCode = startRejectedCode
                snapshot.queue[index].nextAttemptAtMillis = 0
            }
        }
    }

    /// A whole-delivery failure: every sent command keeps its place and backs off.
    public static func applyFailure(ids: [String], code: String, countsAttempt: Bool, to snapshot: inout EnterprisePartitionSnapshot,
                                    nowMillis: Int64, retryAfterMillis: Int64?) {
        for id in ids {
            guard let index = snapshot.queue.firstIndex(where: { $0.id == id }), snapshot.queue[index].status == .pending else { continue }
            if countsAttempt {
                snapshot.queue[index] = failed(snapshot.queue[index], code: code, nowMillis: nowMillis, retryAfterMillis: retryAfterMillis)
            } else {
                snapshot.queue[index].lastCode = code
                snapshot.queue[index].nextAttemptAtMillis = nowMillis + max(baseBackoffMillis, min(retryAfterMillis ?? 0, maxBackoffMillis))
            }
        }
    }

    static func failed(_ item: EnterpriseQueueItem, code: String?, nowMillis: Int64, retryAfterMillis: Int64?) -> EnterpriseQueueItem {
        var next = item
        next.attempts += 1
        next.lastCode = code
        if next.attempts >= maxAttempts {
            next.status = .exhausted
            next.nextAttemptAtMillis = 0
            return next
        }
        next.status = .pending
        next.nextAttemptAtMillis = nowMillis + backoffMillis(attempts: next.attempts, retryAfterMillis: retryAfterMillis)
        return next
    }

    /// Exponential from 30 seconds, never shorter than `Retry-After`, capped at 6 hours.
    public static func backoffMillis(attempts: Int, retryAfterMillis: Int64?) -> Int64 {
        let exponent = max(0, min(attempts - 1, 20))
        let exponential = min(maxBackoffMillis, baseBackoffMillis << Int64(exponent))
        let hinted = min(maxBackoffMillis, max(0, retryAfterMillis ?? 0))
        return max(exponential, hinted)
    }

    /// The person retries held work: attempts restart and the command is due now.
    public static func retryNow(id: String, in snapshot: inout EnterprisePartitionSnapshot) {
        if let index = snapshot.queue.firstIndex(where: { $0.id == id }), !snapshot.queue[index].status.isFinal {
            snapshot.queue[index].status = .pending
            snapshot.queue[index].attempts = 0
            snapshot.queue[index].nextAttemptAtMillis = 0
        }
        if let index = snapshot.attachments.firstIndex(where: { $0.id == id }), snapshot.attachments[index].status == .exhausted {
            snapshot.attachments[index].status = .pending
            snapshot.attachments[index].attempts = 0
            snapshot.attachments[index].nextAttemptAtMillis = 0
        }
    }

    /// Only a final server receipt can leave the visible list; unsent company work is never discarded here.
    public static func dismiss(id: String, in snapshot: inout EnterprisePartitionSnapshot) {
        if let index = snapshot.queue.firstIndex(where: { $0.id == id }), canDismiss(snapshot.queue[index], in: snapshot) {
            if snapshot.queue[index].kind == .fueling {
                snapshot.queue[index].dismissedFromQueue = true
            } else {
                if snapshot.missionInbox?.releaseDismissedAnswer(id) == false { return }
                snapshot.queue.remove(at: index)
            }
        }
        snapshot.attachments.removeAll { $0.id == id && ($0.status == .delivered || $0.status == .refused) }
    }

    /// A final receipt may leave the list once nothing local still reads it. Final lifecycle receipts no
    /// longer hold their dependents; a refused or held start stays while its trip is kept, the open trip's
    /// return checklist completion stays, and a mission answer stays while its inbox line still needs it.
    public static func canDismiss(_ item: EnterpriseQueueItem, in snapshot: EnterprisePartitionSnapshot) -> Bool {
        guard item.status.isFinal, item.dismissedFromQueue != true else { return false }
        if item.status != .accepted, item.kind == .tripStart,
           snapshot.trips.contains(where: { $0.startOperationId == item.id }) { return false }
        if let completion = snapshot.returnChecklists?.draft?.completion, completion.operationId == item.id,
           snapshot.openTrip?.id == completion.tripId { return false }
        if item.kind == .missionResponse, let inbox = snapshot.missionInbox, !inbox.canReleaseDismissedAnswer(item.id) {
            return false
        }
        return true
    }

    /// Receipts of a received owner become due; receipts of a rejected owner are refused locally.
    static func releaseAttachments(in snapshot: inout EnterprisePartitionSnapshot) {
        for index in snapshot.attachments.indices where snapshot.attachments[index].status == .waitingOwner {
            let ownerId = snapshot.attachments[index].ownerOperationId
            let owner = snapshot.queue.first(where: { $0.id == ownerId })
            if owner?.status.isReceived == true {
                snapshot.attachments[index].status = .pending
            } else if owner?.status == .rejected {
                snapshot.attachments[index].status = .refused
                snapshot.attachments[index].lastCode = "OWNER_REJECTED"
            }
        }
    }

    public static func dueAttachment(_ snapshot: EnterprisePartitionSnapshot, nowMillis: Int64) -> EnterpriseAttachmentItem? {
        return snapshot.attachments.first { $0.status == .pending && $0.nextAttemptAtMillis <= nowMillis }
    }

    public static func applyAttachment(id: String, storage: EnterpriseAttachmentStorage?, refusedCode: String?, failureCode: String?,
                                       to snapshot: inout EnterprisePartitionSnapshot, nowMillis: Int64, retryAfterMillis: Int64?) {
        guard let index = snapshot.attachments.firstIndex(where: { $0.id == id }) else { return }
        var item = snapshot.attachments[index]
        if let refusedCode {
            item.status = .refused
            item.lastCode = refusedCode
        } else if let storage, storage.holdsContent {
            item.status = .delivered
            item.lastCode = nil
        } else if let storage, storage != .uploading {
            item.status = .refused
            item.lastCode = storage.rawValue
        } else {
            item.attempts += 1
            item.lastCode = failureCode
            if item.attempts >= maxAttempts {
                item.status = .exhausted
            } else {
                item.nextAttemptAtMillis = nowMillis + backoffMillis(attempts: item.attempts, retryAfterMillis: retryAfterMillis)
            }
        }
        snapshot.attachments[index] = item
    }

    /// The next time any held work becomes due, if any.
    public static func nextWakeMillis(_ snapshot: EnterprisePartitionSnapshot) -> Int64? {
        let commands = commandSchedule(snapshot).map(\.at)
        let receipts = snapshot.attachments.filter { $0.status == .pending }.map { $0.nextAttemptAtMillis }
        return (commands + receipts).min()
    }
}

/// Backup-excluded, per-partition JSON files under Application Support, file-protected on iOS.
///
/// File names are opaque hashes of the partition. Personal data never enters
/// these files, and a file is only read back for its exact partition. Logout and
/// account deletion purge the whole directory; another account's files are erased
/// when a different account opens company mode.
public final class EnterprisePartitionStore: @unchecked Sendable {
    private let directory: URL
    private let lock = NSLock()

    public init(directory: URL? = nil) {
        self.directory = directory ?? Self.defaultDirectory()
    }

    public static func defaultDirectory() -> URL {
        let base = FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask).first
            ?? FileManager.default.temporaryDirectory
        return base.appendingPathComponent("enterprise-v1", isDirectory: true)
    }

    private func fileUrl(_ partition: EnterprisePartition) -> URL {
        return directory.appendingPathComponent(partition.storageKey + ".json")
    }

    private func receiptDirectory(_ partition: EnterprisePartition) -> URL {
        return directory.appendingPathComponent(partition.storageKey + "-receipts", isDirectory: true)
    }

    private func receiptUrl(_ partition: EnterprisePartition, documentId: String) -> URL {
        let name = EnterpriseCanonicalJson.sha256Hex(Data(documentId.utf8))
        return receiptDirectory(partition).appendingPathComponent(name + ".jpg")
    }

    public func load(_ partition: EnterprisePartition) -> EnterprisePartitionSnapshot? {
        lock.lock()
        defer { lock.unlock() }
        guard let data = try? Data(contentsOf: fileUrl(partition)),
              let snapshot = try? JSONDecoder().decode(EnterprisePartitionSnapshot.self, from: data),
              snapshot.schemaVersion == EnterprisePartitionSnapshot.currentVersion,
              snapshot.partition == partition else {
            return nil
        }
        return snapshot
    }

    @discardableResult
    public func save(_ snapshot: EnterprisePartitionSnapshot) -> Bool {
        lock.lock()
        defer { lock.unlock() }
        guard let data = try? JSONEncoder().encode(snapshot) else { return false }
        ensureDirectory(directory)
        return write(data, to: fileUrl(snapshot.partition))
    }

    public func saveReceipt(_ content: Data, partition: EnterprisePartition, documentId: String) -> Bool {
        lock.lock()
        defer { lock.unlock() }
        ensureDirectory(directory)
        ensureDirectory(receiptDirectory(partition))
        return write(content, to: receiptUrl(partition, documentId: documentId))
    }

    public func loadReceipt(partition: EnterprisePartition, documentId: String) -> Data? {
        lock.lock()
        defer { lock.unlock() }
        return try? Data(contentsOf: receiptUrl(partition, documentId: documentId))
    }

    public func deleteReceipt(partition: EnterprisePartition, documentId: String) {
        lock.lock()
        defer { lock.unlock() }
        try? FileManager.default.removeItem(at: receiptUrl(partition, documentId: documentId))
    }

    /// LOST_OR_COMPROMISED: this partition alone, records and receipts.
    public func erase(_ partition: EnterprisePartition) {
        lock.lock()
        defer { lock.unlock() }
        try? FileManager.default.removeItem(at: fileUrl(partition))
        try? FileManager.default.removeItem(at: receiptDirectory(partition))
    }

    /// Logout and account deletion: every company partition on this device.
    public func purgeAll() {
        lock.lock()
        defer { lock.unlock() }
        try? FileManager.default.removeItem(at: directory)
    }

    /// Opening company mode with another account erases what earlier accounts left on this device.
    public func purgeAccounts(except accountId: String) {
        lock.lock()
        defer { lock.unlock() }
        guard let names = try? FileManager.default.contentsOfDirectory(atPath: directory.path) else { return }
        for name in names where name.hasSuffix(".json") {
            let url = directory.appendingPathComponent(name)
            let decoded = (try? Data(contentsOf: url)).flatMap { try? JSONDecoder().decode(EnterprisePartitionSnapshot.self, from: $0) }
            if let decoded, decoded.partition.accountId == accountId {
                continue
            }
            try? FileManager.default.removeItem(at: url)
            let stem = String(name.dropLast(5))
            try? FileManager.default.removeItem(at: directory.appendingPathComponent(stem + "-receipts", isDirectory: true))
        }
    }

    private func ensureDirectory(_ url: URL) {
        try? FileManager.default.createDirectory(at: url, withIntermediateDirectories: true)
        markBackupExcluded(url)
    }

    @discardableResult
    private func write(_ data: Data, to url: URL) -> Bool {
        do {
            #if os(iOS)
            try data.write(to: url, options: [.atomic, .completeFileProtectionUntilFirstUserAuthentication])
            #else
            try data.write(to: url, options: .atomic)
            #endif
        } catch {
            return false
        }
        markBackupExcluded(url)
        return true
    }

    private func markBackupExcluded(_ url: URL) {
        var mutableUrl = url
        var values = URLResourceValues()
        values.isExcludedFromBackup = true
        try? mutableUrl.setResourceValues(values)
    }
}

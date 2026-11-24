import XCTest
@testable import AnpFuelCore

/// Protected queue transitions, bounded retries, receipt release and partition isolation on disk.
final class EnterpriseQueueTests: XCTestCase {
    private let accountA = "7b0e2a54-0000-4000-8000-00000000a001"
    private let accountB = "7b0e2a54-0000-4000-8000-00000000b001"
    private let orgOne = "7b0e2a54-0000-4000-8000-000000000101"
    private let orgTwo = "7b0e2a54-0000-4000-8000-000000000102"
    private let member = "7b0e2a54-0000-4000-8000-0000000000c9"
    private let enrollment = "7b0e2a54-0000-4000-8000-00000000012d"
    private var directory: URL?

    override func setUpWithError() throws {
        directory = FileManager.default.temporaryDirectory.appendingPathComponent("enterprise-tests-" + UUID().uuidString, isDirectory: true)
    }

    override func tearDownWithError() throws {
        if let directory { try? FileManager.default.removeItem(at: directory) }
    }

    private func partition(_ account: String, _ org: String) throws -> EnterprisePartition {
        return try XCTUnwrap(EnterprisePartition(accountId: account, origin: "https://abastevo.com.br", organizationId: org))
    }

    private func expense(_ partition: EnterprisePartition, at millis: Int64) throws -> EnterpriseQueueItem {
        let envelope = EnterpriseEnvelope(
            partition: partition, membershipId: member, enrollmentId: enrollment, operationId: EnterpriseValues.newId(),
            declared: EnterpriseTimeDeclaration(occurredAt: EnterpriseValues.formatInstant(epochMillis: millis),
                                                entryZone: "America/Sao_Paulo", deviceOffsetSeconds: -10_800)
        )
        let operation = try EnterpriseOperationBuilder.submitExpense(
            envelope: envelope, expenseId: EnterpriseValues.newId(), assetId: "7b0e2a54-0000-4000-8000-000000009721",
            category: "TOLL", paidCentavos: 1250, companyCentavos: 1250, payer: .company
        )
        return EnterpriseQueueItem(operation: operation, kind: .expense, summary: "Pedágio", createdAtMillis: millis)
    }

    private func ack(_ item: EnterpriseQueueItem, _ result: EnterpriseSyncResult, code: String? = nil) -> EnterpriseAcknowledgement {
        return EnterpriseAcknowledgement(
            operationId: item.id, payloadDigest: item.operation.payloadDigest, result: result, code: code,
            entityRevision: result == .accepted ? 1 : nil, receivedAt: result == .retry ? nil : "2026-11-15T12:00:01Z",
            reviewItemId: result == .acceptedForReview ? EnterpriseValues.newId() : nil, warnings: []
        )
    }

    func testDeliveryOrderAcknowledgementsAndBoundedRetries() throws {
        let home = try partition(accountA, orgOne)
        var snapshot = EnterprisePartitionSnapshot(partition: home, membershipId: member)
        let first = try expense(home, at: 1_000)
        let second = try expense(home, at: 2_000)
        let third = try expense(home, at: 3_000)
        for item in [first, second, third] {
            try EnterpriseQueue.enqueue(item, into: &snapshot)
        }
        XCTAssertThrowsError(try EnterpriseQueue.enqueue(first, into: &snapshot)) {
            XCTAssertEqual($0 as? EnterpriseQueueError, .duplicate)
        }
        XCTAssertEqual(EnterpriseQueue.dueBatch(snapshot, nowMillis: 10_000).map { $0.id }, [first.id, second.id, third.id])

        EnterpriseQueue.apply([ack(first, .accepted), ack(second, .rejected, code: "FORBIDDEN"), ack(third, .retry, code: "DEPENDENCY_PENDING")],
                              to: &snapshot, nowMillis: 10_000)
        XCTAssertEqual(snapshot.queue[0].status, .accepted)
        XCTAssertEqual(snapshot.queue[1].status, .rejected)
        XCTAssertEqual(snapshot.queue[1].lastCode, "FORBIDDEN")
        XCTAssertEqual(snapshot.queue[2].status, .pending)
        XCTAssertEqual(snapshot.queue[2].attempts, 1)
        XCTAssertEqual(snapshot.queue[2].nextAttemptAtMillis, 10_000 + EnterpriseQueue.baseBackoffMillis)
        XCTAssertTrue(EnterpriseQueue.dueBatch(snapshot, nowMillis: 10_001).isEmpty)

        // An acknowledgement for another digest never settles the command.
        var forged = ack(third, .accepted)
        forged = EnterpriseAcknowledgement(operationId: forged.operationId, payloadDigest: first.operation.payloadDigest, result: .accepted,
                                           code: nil, entityRevision: 1, receivedAt: forged.receivedAt, reviewItemId: nil, warnings: [])
        EnterpriseQueue.apply([forged], to: &snapshot, nowMillis: 20_000)
        XCTAssertEqual(snapshot.queue[2].status, .pending)

        for attempt in 2...EnterpriseQueue.maxAttempts {
            EnterpriseQueue.applyFailure(ids: [third.id], code: "UNAVAILABLE", countsAttempt: true, to: &snapshot,
                                         nowMillis: Int64(attempt) * 100_000_000, retryAfterMillis: nil)
        }
        XCTAssertEqual(snapshot.queue[2].status, .exhausted)
        XCTAssertTrue(EnterpriseQueue.dueBatch(snapshot, nowMillis: Int64.max / 2).isEmpty)

        EnterpriseQueue.dismiss(id: third.id, in: &snapshot)
        XCTAssertEqual(snapshot.queue.count, 3, "unsent company work is never discarded")
        EnterpriseQueue.retryNow(id: third.id, in: &snapshot)
        XCTAssertEqual(snapshot.queue[2].status, .pending)
        XCTAssertEqual(snapshot.queue[2].attempts, 0)
        EnterpriseQueue.dismiss(id: second.id, in: &snapshot)
        XCTAssertEqual(snapshot.queue.map { $0.id }, [first.id, third.id])
    }

    func testOfflineDoesNotSpendAttemptsAndRetryAfterIsHonoredWithinTheCap() throws {
        let home = try partition(accountA, orgOne)
        var snapshot = EnterprisePartitionSnapshot(partition: home, membershipId: member)
        let item = try expense(home, at: 1_000)
        try EnterpriseQueue.enqueue(item, into: &snapshot)
        EnterpriseQueue.applyFailure(ids: [item.id], code: "OFFLINE", countsAttempt: false, to: &snapshot, nowMillis: 5_000, retryAfterMillis: nil)
        XCTAssertEqual(snapshot.queue[0].attempts, 0)
        XCTAssertEqual(snapshot.queue[0].nextAttemptAtMillis, 5_000 + EnterpriseQueue.baseBackoffMillis)
        XCTAssertEqual(EnterpriseQueue.backoffMillis(attempts: 1, retryAfterMillis: 120_000), 120_000)
        XCTAssertEqual(EnterpriseQueue.backoffMillis(attempts: 3, retryAfterMillis: nil), 120_000)
        XCTAssertEqual(EnterpriseQueue.backoffMillis(attempts: 30, retryAfterMillis: nil), EnterpriseQueue.maxBackoffMillis)
        XCTAssertEqual(EnterpriseQueue.backoffMillis(attempts: 1, retryAfterMillis: Int64.max), EnterpriseQueue.maxBackoffMillis)
    }

    func testLockedPartitionAcceptsNoNewCommand() throws {
        let home = try partition(accountA, orgOne)
        var snapshot = EnterprisePartitionSnapshot(partition: home, membershipId: member)
        snapshot.locked = true
        XCTAssertThrowsError(try EnterpriseQueue.enqueue(try expense(home, at: 1_000), into: &snapshot)) {
            XCTAssertEqual($0 as? EnterpriseQueueError, .locked)
        }
    }

    func testTripFinishWaitsForItsStartAndReceiptsWaitForTheirExpense() throws {
        let home = try partition(accountA, orgOne)
        var snapshot = EnterprisePartitionSnapshot(partition: home, membershipId: member)
        let start = try expense(home, at: 1_000)
        let startItem = EnterpriseQueueItem(operation: start.operation, kind: .tripStart, summary: "Início", createdAtMillis: 1_000)
        let finish = try expense(home, at: 2_000)
        let finishItem = EnterpriseQueueItem(operation: finish.operation, kind: .tripFinish, summary: "Fim", createdAtMillis: 2_000)
        try EnterpriseQueue.enqueue(startItem, into: &snapshot)
        try EnterpriseQueue.enqueue(finishItem, into: &snapshot)
        var trip = EnterpriseTripRecord(id: EnterpriseValues.newId(), assetId: "7b0e2a54-0000-4000-8000-000000009721", assetLabel: "Van",
                                        startOperationId: startItem.id, startedAtMillis: 1_000, startMeterId: nil)
        trip.finishOperationId = finishItem.id
        snapshot.trips = [trip]
        EnterpriseQueue.applyFailure(ids: [startItem.id], code: "UNAVAILABLE", countsAttempt: true, to: &snapshot, nowMillis: 3_000, retryAfterMillis: nil)
        XCTAssertTrue(EnterpriseQueue.dueBatch(snapshot, nowMillis: 3_001).isEmpty, "the finish waits while its start backs off")

        var attachment = EnterpriseAttachmentItem(documentId: EnterpriseValues.newId(), declarationId: EnterpriseValues.newId(),
                                                  linkId: EnterpriseValues.newId(), sha256: String(repeating: "a", count: 64), bytes: 10,
                                                  targetKind: "EXPENSE", targetId: EnterpriseValues.newId(), ownerOperationId: startItem.id)
        snapshot.attachments = [attachment]
        EnterpriseQueue.apply([ack(startItem, .acceptedForReview, code: "ASSET_BUSY")], to: &snapshot, nowMillis: 40_000)
        XCTAssertEqual(snapshot.queue[0].status, .inReview)
        XCTAssertEqual(snapshot.attachments[0].status, .pending)
        XCTAssertEqual(EnterpriseQueue.dueAttachment(snapshot, nowMillis: 40_000)?.id, attachment.id)
        let uploadRefused = EnterpriseAttachmentWire.isFinalRefusal(status: 409, code: "CONFLICT", source: .capabilityUpload)
        EnterpriseQueue.applyAttachment(id: attachment.id, storage: nil, refusedCode: uploadRefused ? "CONFLICT" : nil,
                                        failureCode: "CONFLICT", to: &snapshot, nowMillis: 41_000, retryAfterMillis: 120_000)
        XCTAssertEqual(snapshot.attachments[0].status, .pending)
        XCTAssertNil(EnterpriseQueue.dueAttachment(snapshot, nowMillis: 160_999))
        let retry = try XCTUnwrap(EnterpriseQueue.dueAttachment(snapshot, nowMillis: 161_000))
        XCTAssertEqual(retry.declarationId, attachment.declarationId)
        XCTAssertEqual(retry.documentId, attachment.documentId)
        XCTAssertEqual(retry.linkId, attachment.linkId)
        XCTAssertEqual(retry.sha256, attachment.sha256)
        EnterpriseQueue.applyAttachment(id: attachment.id, storage: .stored, refusedCode: nil, failureCode: nil, to: &snapshot,
                                        nowMillis: 161_000, retryAfterMillis: nil)
        XCTAssertEqual(snapshot.attachments[0].status, .delivered)

        attachment = EnterpriseAttachmentItem(documentId: EnterpriseValues.newId(), declarationId: EnterpriseValues.newId(),
                                              linkId: EnterpriseValues.newId(), sha256: String(repeating: "b", count: 64), bytes: 10,
                                              targetKind: "EXPENSE", targetId: EnterpriseValues.newId(), ownerOperationId: finishItem.id)
        snapshot.attachments.append(attachment)
        EnterpriseQueue.apply([ack(finishItem, .rejected, code: "FORBIDDEN")], to: &snapshot, nowMillis: 50_000)
        XCTAssertEqual(snapshot.attachments[1].status, .refused)
        XCTAssertEqual(snapshot.attachments[1].lastCode, "OWNER_REJECTED")
    }

    func testStoreIsolatesPartitionsAndPurgesOnLogoutOrAccountChange() throws {
        let store = EnterprisePartitionStore(directory: try XCTUnwrap(directory))
        let a1 = try partition(accountA, orgOne)
        let a2 = try partition(accountA, orgTwo)
        let b1 = try partition(accountB, orgOne)
        var first = EnterprisePartitionSnapshot(partition: a1, membershipId: member)
        try EnterpriseQueue.enqueue(try expense(a1, at: 1_000), into: &first)
        store.save(first)
        store.save(EnterprisePartitionSnapshot(partition: a2, membershipId: member))
        store.save(EnterprisePartitionSnapshot(partition: b1, membershipId: member))
        XCTAssertTrue(store.saveReceipt(Data([1, 2, 3]), partition: a1, documentId: "doc-1"))

        XCTAssertEqual(store.load(a1)?.queue.count, 1)
        XCTAssertEqual(store.load(a2)?.queue.count, 0, "another organization of the same account holds its own work")
        XCTAssertEqual(store.load(b1)?.partition.accountId, accountB)
        XCTAssertNotEqual(a1.storageKey, b1.storageKey)
        XCTAssertFalse(a1.storageKey.contains(accountA))

        store.purgeAccounts(except: accountA)
        XCTAssertNil(store.load(b1))
        XCTAssertNotNil(store.load(a1))
        XCTAssertEqual(store.loadReceipt(partition: a1, documentId: "doc-1"), Data([1, 2, 3]))

        store.erase(a1)
        XCTAssertNil(store.load(a1))
        XCTAssertNil(store.loadReceipt(partition: a1, documentId: "doc-1"))
        XCTAssertNotNil(store.load(a2))

        store.purgeAll()
        XCTAssertNil(store.load(a2))
    }

    func testPartitionRefusesNonCanonicalValues() {
        XCTAssertNil(EnterprisePartition(accountId: accountA, origin: "http://abastevo.com.br", organizationId: orgOne))
        XCTAssertNil(EnterprisePartition(accountId: accountA.uppercased(), origin: "https://abastevo.com.br", organizationId: orgOne))
        XCTAssertNil(EnterprisePartition(accountId: accountA, origin: "https://abastevo.com.br", organizationId: EnterpriseValues.nilId))
    }
}

import Foundation
import XCTest
@testable import AnpFuelCore

final class EnterpriseTripLifecycleTests: XCTestCase {
    private let start = "7b0e2a54-0000-4000-8000-000000009601"
    private let tripId = "7b0e2a54-0000-4000-8000-000000009611"
    private let asset = "7b0e2a54-0000-4000-8000-000000009621"
    private let operation = "7b0e2a54-0000-4000-8000-000000009605"

    private func hex(_ value: String) throws -> Data {
        let characters = Array(value)
        guard characters.count.isMultiple(of: 2) else { throw EnterpriseOperationError.malformed }
        return try Data(stride(from: 0, to: characters.count, by: 2).map { offset in
            try XCTUnwrap(UInt8(String(characters[offset...offset + 1]), radix: 16))
        })
    }

    private func vectors() throws -> [[String]] {
        #if SWIFT_PACKAGE
        var url = URL(fileURLWithPath: #filePath)
        for _ in 0..<4 { url.deleteLastPathComponent() }
        url = url.appendingPathComponent("contracts/testdata/enterprise/trip/trip-operations-v3.psv")
        #else
        let url = try XCTUnwrap(Bundle(for: Self.self).url(forResource: "trip-operations-v3", withExtension: "psv"))
        #endif
        let text = try String(contentsOf: url, encoding: .utf8)
        return text.split(whereSeparator: \.isNewline).dropFirst().map {
            $0.split(separator: "|", omittingEmptySubsequences: false).map(String.init)
        }
    }

    private func decodeEnvelope(_ object: [String: Any]) throws -> EnterpriseEnvelope {
        let partition = try XCTUnwrap(EnterprisePartition(
            accountId: try XCTUnwrap(object["owner_account_id"] as? String),
            origin: try XCTUnwrap(object["api_origin"] as? String),
            organizationId: try XCTUnwrap(object["org_id"] as? String)
        ))
        let declaration = EnterpriseTimeDeclaration(
            occurredAt: try XCTUnwrap(object["occurred_at"] as? String),
            entryZone: try XCTUnwrap(object["entry_zone"] as? String),
            deviceOffsetSeconds: try XCTUnwrap((object["device_offset_seconds"] as? NSNumber)?.int64Value)
        )
        return EnterpriseEnvelope(partition: partition,
            membershipId: try XCTUnwrap(object["membership_id"] as? String),
            enrollmentId: try XCTUnwrap(object["enrollment_id"] as? String),
            operationId: try XCTUnwrap(object["operation_id"] as? String), declared: declaration)
    }

    func testTripLifecycleCanonicallyReplaysTheSharedProtocol() throws {
        var exercised = 0
        for row in try vectors() {
            guard row.count == 5,
                  let action = EnterpriseTripTransitionAction(rawValue: "trip." + row[1].lowercased()) else { continue }
            let data = try hex(row[4])
            let object = try XCTUnwrap(DeviceIdentityWire.object(data))
            let envelope = try decodeEnvelope(object)
            let entity = try XCTUnwrap(object["entity"] as? [String: Any])
            let id = try XCTUnwrap(entity["id"] as? String)
            let revision = try XCTUnwrap((entity["expected_revision"] as? NSNumber)?.int64Value)
            let payload = try XCTUnwrap(object["payload"] as? [String: Any])
            let reason = payload["reason"] as? String
            let dependency = (object["depends_on"] as? [String])?.first
            if row[3] == "OK" {
                let actual = try EnterpriseOperationBuilder.transitionTrip(envelope: envelope, tripId: id, action: action,
                        expectedRevision: revision, reason: reason, startOperationId: dependency)
                XCTAssertEqual(actual.canonical, data, row[0])
            } else {
                XCTAssertThrowsError(try EnterpriseOperationBuilder.transitionTrip(envelope: envelope, tripId: id, action: action,
                                     expectedRevision: revision, reason: reason, startOperationId: dependency), row[0])
            }
            exercised += 1
        }
        XCTAssertEqual(exercised, 17)
    }

    func testReturnRunReferenceMatchesTheSharedFinishWithoutRunClock() throws {
        let row = try XCTUnwrap(vectors().first { $0.first == "finish-absent-reading-with-return-run-without-dependency" })
        let bytes = try hex(row[4])
        let doc = try XCTUnwrap(DeviceIdentityWire.object(bytes))
        let envelope = try decodeEnvelope(doc)
        let built = try EnterpriseOperationBuilder.finishTrip(
            envelope: envelope, tripId: tripId, evidence: .missing(reason: "Sem leitura final"),
            startOperationId: nil, returnRunId: "7b0e2a54-0000-4000-8000-000000009662"
        )
        XCTAssertEqual(built.canonical, bytes)
        XCTAssertThrowsError(try EnterpriseOperationBuilder.finishTrip(
            envelope: envelope, tripId: tripId, evidence: .missing(reason: "Sem leitura final"),
            startOperationId: nil, returnRunId: "7B0E2A54-0000-4000-8000-000000009662"
        ))
    }

    func testLocalLifecycleRejectsReorderedOrConflictingDeclarations() throws {
        var record = EnterpriseTripRecord(id: tripId, assetId: asset, assetLabel: "Fixture",
                                          startOperationId: start, startedAtMillis: 1_000, startMeterId: nil)
        XCTAssertEqual(record.nextLifecycleRevision, 1)
        XCTAssertThrowsError(try record.recordTransition(.resume, operationId: operation, declaredAtMillis: 2_000))
        XCTAssertThrowsError(try record.recordTransition(.pause, operationId: operation, declaredAtMillis: 999))
        try record.recordTransition(.pause, operationId: operation, declaredAtMillis: 2_000)
        XCTAssertEqual(record.localState, .paused)
        XCTAssertTrue(record.isOpen)
        XCTAssertEqual(record.nextLifecycleRevision, 2)
        XCTAssertThrowsError(try record.recordTransition(.resume, operationId: operation, declaredAtMillis: 2_000))
        let resume = "7b0e2a54-0000-4000-8000-000000009606"
        let cancel = "7b0e2a54-0000-4000-8000-000000009607"
        XCTAssertThrowsError(try record.recordTransition(.resume, operationId: resume, declaredAtMillis: 1_999))
        try record.recordTransition(.resume, operationId: resume, declaredAtMillis: 3_000)
        try record.recordTransition(.cancel, operationId: cancel, declaredAtMillis: 4_000)
        XCTAssertEqual(record.localState, .cancelled)
        XCTAssertFalse(record.isOpen)
        XCTAssertThrowsError(try record.recordTransition(.resume, operationId: "7b0e2a54-0000-4000-8000-000000009608",
                                                         declaredAtMillis: 5_000))
        let restored = try JSONDecoder().decode(EnterpriseTripRecord.self, from: JSONEncoder().encode(record))
        XCTAssertEqual(restored, record)
    }

    private func queueItem(_ id: String, _ kind: EnterpriseCommandKind, _ type: String,
                           entityType: String = "trip", entityId: String? = nil) -> EnterpriseQueueItem {
        EnterpriseQueueItem(
            operation: EnterpriseSealedOperation(operationId: id, type: type, entityType: entityType, entityId: entityId ?? tripId,
                                                 payloadDigest: "fixture-" + id, canonical: Data()),
            kind: kind, summary: "Fixture", createdAtMillis: 0
        )
    }

    private func acknowledgement(_ item: EnterpriseQueueItem, _ result: EnterpriseSyncResult, code: String? = nil) -> EnterpriseAcknowledgement {
        EnterpriseAcknowledgement(
            operationId: item.id, payloadDigest: item.operation.payloadDigest, result: result, code: code,
            entityRevision: result == .accepted ? 1 : nil, receivedAt: result == .retry ? nil : "2026-11-15T12:00:01Z",
            reviewItemId: result == .acceptedForReview ? EnterpriseValues.newId() : nil, warnings: []
        )
    }

    private let resume = "7b0e2a54-0000-4000-8000-000000009606"

    private func pausedAndResumed() throws -> EnterprisePartitionSnapshot {
        let row = try XCTUnwrap(vectors().first { $0.first == "pause-without-reason" })
        let raw = try hex(row[4])
        let env = try decodeEnvelope(try XCTUnwrap(DeviceIdentityWire.object(raw)))
        var record = EnterpriseTripRecord(id: tripId, assetId: asset, assetLabel: "Fixture",
                                          startOperationId: start, startedAtMillis: 1000, startMeterId: nil)
        try record.recordTransition(.pause, operationId: operation, declaredAtMillis: 2000)
        try record.recordTransition(.resume, operationId: resume, declaredAtMillis: 3000)
        var snapshot = EnterprisePartitionSnapshot(partition: env.partition, membershipId: env.membershipId)
        snapshot.trips = [record]
        snapshot.queue = [
            queueItem(start, .tripStart, "trip.start"),
            queueItem(operation, .tripLifecycle, "trip.pause"),
            queueItem(resume, .tripLifecycle, "trip.resume"),
            queueItem(asset, .expense, "expense.submit", entityType: "expense", entityId: asset),
        ]
        return snapshot
    }

    func testTripChainWaitsOnlyWhileItsPredecessorIsHeldOnThisDevice() throws {
        var snapshot = try pausedAndResumed()
        snapshot.queue[1].nextAttemptAtMillis = 9000
        // A pending start holds its pause even when both would be due in the same pass.
        XCTAssertEqual(EnterpriseQueue.dueBatch(snapshot, nowMillis: 9000).map(\.id), [start, asset])
        snapshot.queue[0].status = .accepted
        snapshot.queue[3].status = .accepted
        XCTAssertEqual(EnterpriseQueue.nextWakeMillis(snapshot), 9000)
        XCTAssertEqual(EnterpriseQueue.dueBatch(snapshot, nowMillis: 9000).map(\.id), [operation])
        EnterpriseQueue.applyFailure(ids: [operation], code: "UNAVAILABLE", countsAttempt: true, to: &snapshot,
                                     nowMillis: 9000, retryAfterMillis: 60_000)
        XCTAssertEqual(EnterpriseQueue.nextWakeMillis(snapshot), 69_000)
        XCTAssertTrue(EnterpriseQueue.dueBatch(snapshot, nowMillis: 68_999).isEmpty, "the resume waits while its pause backs off")
        snapshot.queue[1].status = .exhausted
        XCTAssertTrue(EnterpriseQueue.dueBatch(snapshot, nowMillis: 70_000).isEmpty, "a paused predecessor still holds")
        XCTAssertNil(EnterpriseQueue.nextWakeMillis(snapshot))
        XCTAssertFalse(EnterpriseQueue.canDismiss(snapshot.queue[1], in: snapshot))

        // Any final receipt of the predecessor releases the dependent; the server decides it.
        for receipt in [EnterpriseQueueStatus.accepted, .inReview, .rejected] {
            snapshot.queue[1].status = receipt
            XCTAssertEqual(EnterpriseQueue.dueBatch(snapshot, nowMillis: 70_000).map(\.id), [resume], receipt.rawValue)
            XCTAssertEqual(EnterpriseQueue.nextWakeMillis(snapshot), 0)
        }
        snapshot.queue[1].status = .inReview
        XCTAssertTrue(EnterpriseQueue.canDismiss(snapshot.queue[1], in: snapshot), "a held lifecycle receipt no longer gates")
        EnterpriseQueue.dismiss(id: operation, in: &snapshot)
        XCTAssertNil(snapshot.queue.first { $0.id == operation })
        XCTAssertEqual(EnterpriseQueue.dueBatch(snapshot, nowMillis: 70_000).map(\.id), [resume], "an absent predecessor never holds")
    }

    func testHeldStartReleasesItsChainAndAReviewerApprovalCanStillReceiveIt() throws {
        var snapshot = try pausedAndResumed()
        let finish = "7b0e2a54-0000-4000-8000-000000009609"
        snapshot.trips[0].finishOperationId = finish
        snapshot.trips[0].finishedAtMillis = 4000
        snapshot.queue.append(queueItem(finish, .tripFinish, "trip.finish"))
        EnterpriseQueue.apply([acknowledgement(snapshot.queue[0], .acceptedForReview, code: "ASSET_BUSY")], to: &snapshot, nowMillis: 5000)
        XCTAssertEqual(snapshot.queue[0].status, .inReview)
        XCTAssertFalse(EnterpriseQueue.canDismiss(snapshot.queue[0], in: snapshot), "the held start stays with its trip")
        XCTAssertEqual(EnterpriseQueue.dueBatch(snapshot, nowMillis: 5000).map(\.id), [operation, asset])
        EnterpriseQueue.apply([acknowledgement(snapshot.queue[1], .retry, code: "DEPENDENCY_PENDING")], to: &snapshot, nowMillis: 5000)
        XCTAssertEqual(snapshot.queue[1].status, .pending)
        XCTAssertFalse(EnterpriseQueue.dueBatch(snapshot, nowMillis: 5000).map(\.id).contains(resume))
        let pause = snapshot.queue[1]
        EnterpriseQueue.apply([acknowledgement(pause, .accepted)], to: &snapshot, nowMillis: 40_000)
        XCTAssertEqual(EnterpriseQueue.dueBatch(snapshot, nowMillis: 40_000).map(\.id), [resume, asset])
        snapshot.queue[2].status = .accepted
        XCTAssertEqual(EnterpriseQueue.dueBatch(snapshot, nowMillis: 40_000).map(\.id), [asset, finish])
    }

    func testRejectedStartRefusesItsUnsentStepsLocallyAndLetsThemBeDismissed() throws {
        var snapshot = try pausedAndResumed()
        let finish = "7b0e2a54-0000-4000-8000-000000009609"
        snapshot.trips[0].finishOperationId = finish
        snapshot.trips[0].finishedAtMillis = 4000
        snapshot.queue.append(queueItem(finish, .tripFinish, "trip.finish"))
        snapshot.queue[2].status = .exhausted
        snapshot.queue[2].attempts = EnterpriseQueue.maxAttempts
        EnterpriseQueue.apply([acknowledgement(snapshot.queue[0], .rejected, code: "FORBIDDEN")], to: &snapshot, nowMillis: 5000)
        for id in [operation, resume, finish] {
            let item = try XCTUnwrap(snapshot.queue.first { $0.id == id })
            XCTAssertEqual(item.status, .rejected, id)
            XCTAssertEqual(item.lastCode, EnterpriseQueue.startRejectedCode, id)
            XCTAssertNil(item.receivedAt, "a local refusal is no server receipt")
            XCTAssertTrue(EnterpriseQueue.canDismiss(item, in: snapshot), id)
        }
        XCTAssertEqual(snapshot.queue[3].status, .pending, "unrelated work is untouched")
        XCTAssertEqual(EnterpriseQueue.dueBatch(snapshot, nowMillis: 5000).map(\.id), [asset])
        XCTAssertFalse(EnterpriseQueue.canDismiss(snapshot.queue[0], in: snapshot), "the refused start stays with its trip")
        XCTAssertEqual(snapshot.unsentCount, 1)

        // Removing the refused trip can no longer release an orphaned step to the server.
        snapshot.queue[1].status = .pending
        snapshot.queue[1].lastCode = nil
        EnterpriseQueue.refuseCommandsOfRejectedStarts(in: &snapshot)
        snapshot.trips.removeAll()
        XCTAssertEqual(snapshot.queue[1].status, .rejected)
        XCTAssertEqual(EnterpriseQueue.dueBatch(snapshot, nowMillis: 5000).map(\.id), [asset])
        XCTAssertTrue(EnterpriseQueue.canDismiss(snapshot.queue[0], in: snapshot))
    }

    func testStartWaitsForItsDepartureChecklistAndOldSnapshotsStillDecode() throws {
        var snapshot = try pausedAndResumed()
        let run = "7b0e2a54-0000-4000-8000-000000009631"
        let checklist = "7b0e2a54-0000-4000-8000-000000009632"
        let record = snapshot.trips[0]
        var departing = EnterpriseTripRecord(id: record.id, assetId: record.assetId, assetLabel: record.assetLabel,
                                             startOperationId: record.startOperationId, startedAtMillis: record.startedAtMillis,
                                             startMeterId: nil, departureRunId: run)
        departing.lifecycle = record.lifecycle
        snapshot.trips[0] = departing
        snapshot.queue.insert(queueItem(checklist, .checklist, "check.run.complete", entityType: "checklist_run", entityId: run), at: 0)
        XCTAssertEqual(EnterpriseQueue.dueBatch(snapshot, nowMillis: 1000).map(\.id), [checklist, asset],
                       "a start never travels with a departure run the server may not hold yet")
        EnterpriseQueue.apply([acknowledgement(snapshot.queue[0], .retry, code: "UNAVAILABLE")], to: &snapshot, nowMillis: 1000)
        XCTAssertFalse(EnterpriseQueue.dueBatch(snapshot, nowMillis: 1000).map(\.id).contains(start))
        EnterpriseQueue.apply([acknowledgement(snapshot.queue[0], .accepted)], to: &snapshot, nowMillis: 2000)
        XCTAssertEqual(EnterpriseQueue.dueBatch(snapshot, nowMillis: 2000).map(\.id), [start, asset])
        // Another run of the same asset does not hold this start.
        snapshot.queue[0] = queueItem(checklist, .checklist, "check.run.complete", entityType: "checklist_run",
                                      entityId: "7b0e2a54-0000-4000-8000-000000009633")
        XCTAssertEqual(EnterpriseQueue.dueBatch(snapshot, nowMillis: 2000).map(\.id), [checklist, start, asset])

        var stored = try XCTUnwrap(JSONSerialization.jsonObject(with: JSONEncoder().encode(snapshot.trips[0])) as? [String: Any])
        XCTAssertEqual(stored["departureRunId"] as? String, run)
        for key in ["departureRunId", "missionId", "lifecycle"] { stored.removeValue(forKey: key) }
        let legacy = try JSONDecoder().decode(EnterpriseTripRecord.self, from: JSONSerialization.data(withJSONObject: stored))
        XCTAssertNil(legacy.departureRunId)
        XCTAssertNil(legacy.missionId)
        XCTAssertEqual(legacy.localState, .running)
    }
}

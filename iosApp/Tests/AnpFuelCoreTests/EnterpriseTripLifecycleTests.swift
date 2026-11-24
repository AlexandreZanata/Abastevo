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

    func testTripChainWaitsThroughBackoffAndDoesNotRunPastReview() throws {
        let row = try XCTUnwrap(vectors().first { $0.first == "pause-without-reason" })
        let raw = try hex(row[4])
        let env = try decodeEnvelope(try XCTUnwrap(DeviceIdentityWire.object(raw)))
        var record = EnterpriseTripRecord(id: tripId, assetId: asset, assetLabel: "Fixture",
                                          startOperationId: start, startedAtMillis: 1000, startMeterId: nil)
        let resume = "7b0e2a54-0000-4000-8000-000000009606"
        try record.recordTransition(.pause, operationId: operation, declaredAtMillis: 2000)
        try record.recordTransition(.resume, operationId: resume, declaredAtMillis: 3000)
        func queueItem(_ id: String, _ kind: EnterpriseCommandKind, _ type: String) -> EnterpriseQueueItem {
            EnterpriseQueueItem(
                operation: EnterpriseSealedOperation(operationId: id, type: type, entityType: "trip", entityId: tripId,
                                                     payloadDigest: "fixture", canonical: Data()),
                kind: kind, summary: "Fixture", createdAtMillis: 0
            )
        }
        var snapshot = EnterprisePartitionSnapshot(partition: env.partition, membershipId: env.membershipId)
        snapshot.trips = [record]
        snapshot.queue = [
            queueItem(start, .tripStart, "trip.start"),
            queueItem(operation, .tripLifecycle, "trip.pause"),
            queueItem(resume, .tripLifecycle, "trip.resume"),
            queueItem(asset, .expense, "expense.submit"),
        ]
        snapshot.queue[1].nextAttemptAtMillis = 9000
        XCTAssertEqual(EnterpriseQueue.dueBatch(snapshot, nowMillis: 8000).map(\.id), [start, asset])
        snapshot.queue[0].status = .accepted
        snapshot.queue[3].status = .accepted
        XCTAssertEqual(EnterpriseQueue.nextWakeMillis(snapshot), 9000)
        XCTAssertEqual(EnterpriseQueue.dueBatch(snapshot, nowMillis: 9000).map(\.id), [operation, resume])
        snapshot.queue[1].status = .inReview
        XCTAssertFalse(EnterpriseQueue.canDismiss(snapshot.queue[1], in: snapshot))
        XCTAssertTrue(EnterpriseQueue.dueBatch(snapshot, nowMillis: 18_000).isEmpty)
        XCTAssertNil(EnterpriseQueue.nextWakeMillis(snapshot))
        EnterpriseQueue.dismiss(id: operation, in: &snapshot)
        XCTAssertNotNil(snapshot.queue.first { $0.id == operation })
    }
}

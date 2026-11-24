import Foundation
import XCTest
@testable import AnpFuelCore

final class EnterpriseReturnChecklistTests: XCTestCase {
    private let account = "7b0e2a54-0000-4000-8000-000000009901"
    private let organization = "7b0e2a54-0000-4000-8000-000000009902"
    private let member = "7b0e2a54-0000-4000-8000-000000009903"
    private let enrollment = "7b0e2a54-0000-4000-8000-000000009904"
    private let tripID = "7b0e2a54-0000-4000-8000-000000009905"
    private let asset = "7b0e2a54-0000-4000-8000-000000009721"
    private let template = "7b0e2a54-0000-4000-8000-000000009731"
    private let operation = "7b0e2a54-0000-4000-8000-000000009702"
    private let runID = "7b0e2a54-0000-4000-8000-000000009712"
    private let now: Int64 = 1_800_000_000_000

    private func corpus(_ name: String) throws -> [[String]] {
        #if SWIFT_PACKAGE
        var root = URL(fileURLWithPath: #filePath)
        for _ in 0..<4 { root.deleteLastPathComponent() }
        let url = root.appendingPathComponent("contracts/testdata/enterprise/checklist").appendingPathComponent(name).appendingPathExtension("psv")
        #else
        let url = try XCTUnwrap(Bundle(for: Self.self).url(forResource: name, withExtension: "psv"))
        #endif
        return try String(contentsOf: url, encoding: .utf8).split(separator: "\n").dropFirst().map {
            $0.split(separator: "|", omittingEmptySubsequences: false).map(String.init)
        }
    }

    private func document(required: Bool = true, evaluated: String = "2026-11-15T12:00:00Z") -> [String: Any] {
        var root: [String: Any] = [
            "asset_id": asset, "context": "RETURN", "required": required, "match": required ? "ASSET" : "NONE",
            "evaluated_at": evaluated, "data_mode": "SYNTHETIC",
        ]
        if required {
            root["template"] = ["template_id": template, "name": "Synthetic return", "version": 1,
                                "published_at": "2026-11-01T10:00:00Z", "items": [
                                    ["position": 1, "label": "Synthetic brakes", "critical": true, "optional": false, "photo_on_not_ok": false],
                                    ["position": 2, "label": "Synthetic body", "critical": false, "optional": true, "photo_on_not_ok": true],
                                ]]
            root["policy"] = ["revision": 3, "mode": "WARN", "validity_seconds": 3_600, "effective_at": "2026-11-01T10:00:00Z"]
        }
        return root
    }

    private func applicable() throws -> EnterpriseChecklistApplicable {
        try XCTUnwrap(EnterpriseCompanyWire.checklistApplicable(document(), assetId: asset, context: "RETURN"))
    }

    private func trip(id: String? = nil) -> EnterpriseTripRecord {
        EnterpriseTripRecord(id: id ?? tripID, assetId: asset, assetLabel: "Synthetic asset", startOperationId: operation,
                             startedAtMillis: now - 1, startMeterId: nil)
    }

    private func partition(organizationId: String? = nil) throws -> EnterprisePartition {
        try XCTUnwrap(EnterprisePartition(accountId: account, origin: "https://company.example.invalid",
                                           organizationId: organizationId ?? organization))
    }

    func testSharedCanonicalReturnRunBytesAndDeferredPhotoProtocol() throws {
        let cases = try corpus("run-v1")
        let row = try XCTUnwrap(cases.first { $0.first == "run-return-context" })
        XCTAssertEqual(Array(row.prefix(2)), ["run-return-context", "RUN"])
        XCTAssertEqual(row[3], "OK")
        let encoded = row[4]
        XCTAssertTrue(encoded.utf8.count.isMultiple(of: 2))
        let bytes = Array(encoded.utf8)
        let expected = try Data(stride(from: 0, to: bytes.count, by: 2).map { index in
            try XCTUnwrap(UInt8(String(decoding: bytes[index..<(index + 2)], as: UTF8.self), radix: 16))
        })
        let root = try XCTUnwrap(EnterpriseCanonicalJson.parse(expected).objectValue)
        let original = try XCTUnwrap(root["payload"]?.objectValue)
        let partition = try XCTUnwrap(EnterprisePartition(
            accountId: try XCTUnwrap(root["owner_account_id"]?.textValue),
            origin: try XCTUnwrap(root["api_origin"]?.textValue), organizationId: try XCTUnwrap(root["org_id"]?.textValue)
        ))
        let offset = try XCTUnwrap(root["device_offset_seconds"]?.integerLexeme)
        let time = EnterpriseTimeDeclaration(occurredAt: try XCTUnwrap(root["occurred_at"]?.textValue),
                                             entryZone: try XCTUnwrap(root["entry_zone"]?.textValue),
                                             deviceOffsetSeconds: try XCTUnwrap(Int64(offset)))
        let envelope = EnterpriseEnvelope(partition: partition, membershipId: try XCTUnwrap(root["membership_id"]?.textValue),
                                          enrollmentId: try XCTUnwrap(root["enrollment_id"]?.textValue), operationId: operation, declared: time)
        let answer = EnterpriseOperationBuilder.ChecklistResponse(answer: .ok)
        let actual = try EnterpriseOperationBuilder.completeChecklist(
            envelope: envelope, runId: runID, assetId: asset, context: "RETURN", templateId: template, templateVersion: 1,
            policyRevision: 1, startedAt: try XCTUnwrap(original["started_at"]?.textValue), responses: [answer, answer, answer],
            outcome: .completedOk, acknowledged: false
        )
        XCTAssertEqual(actual.canonical, expected)
        XCTAssertEqual(root["attachments"], .array([]))
        XCTAssertFalse(EnterpriseAttachmentWire.targetKinds.contains("CHECKLIST_RUN"))
        for item in try XCTUnwrap(original["answers"]?.arrayValue) {
            XCTAssertEqual(item.objectValue?["photo"], .bool(false))
        }
        let photoVector = try XCTUnwrap(corpus("checklist-v1").first { $0.first == "ev-photo-issue-without-photo" })
        XCTAssertEqual(photoVector[3], "error=PHOTO_REQUIRED")
        let items = [
            EnterpriseChecklistItem(position: 1, label: "Pneus", critical: true, optional: false, photoOnNotOk: true),
            EnterpriseChecklistItem(position: 2, label: "Freios", critical: true, optional: false, photoOnNotOk: false),
            EnterpriseChecklistItem(position: 3, label: "Estepe", critical: false, optional: true, photoOnNotOk: false),
            EnterpriseChecklistItem(position: 4, label: "Lataria", critical: false, optional: false, photoOnNotOk: true),
            EnterpriseChecklistItem(position: 5, label: "Buzina", critical: false, optional: false, photoOnNotOk: false),
        ]
        XCTAssertThrowsError(try EnterpriseChecklistRules.evaluate(items: items, policyMode: "BLOCK", answers: [.ok, .ok, .ok, .notOk, .ok])) {
            XCTAssertEqual($0 as? EnterpriseChecklistFailure, .photoRequired)
        }
    }

    func testStrictApplicabilityAndDraftAgeOwnerAndTemplatePin() throws {
        let shared = try applicable()
        XCTAssertEqual(shared.validitySeconds, 3_600)
        XCTAssertNil(EnterpriseCompanyWire.checklistApplicable(document(), assetId: template, context: "RETURN"))
        XCTAssertNil(EnterpriseCompanyWire.checklistApplicable(document(), assetId: asset, context: "DEPARTURE"))
        var malformed = document()
        malformed["data_mode"] = "UNKNOWN"
        XCTAssertNil(EnterpriseCompanyWire.checklistApplicable(malformed, assetId: asset, context: "RETURN"))
        malformed = document(required: false)
        malformed["template"] = [:]
        XCTAssertNil(EnterpriseCompanyWire.checklistApplicable(malformed, assetId: asset, context: "RETURN"))
        let knownNone = try XCTUnwrap(EnterpriseCompanyWire.checklistApplicable(document(required: false), assetId: asset, context: "RETURN"))
        XCTAssertFalse(knownNone.required)
        let currentTrip = trip()
        var book = EnterpriseReturnChecklistBook(membershipId: member, enrollmentId: enrollment)
        try book.remember(shared)
        var draft = try book.start(trip: currentTrip, nowMillis: now)
        XCTAssertTrue(draft.matches(currentTrip))
        XCTAssertFalse(draft.matches(trip(id: template)))
        XCTAssertFalse(book.matches(membershipId: member, enrollmentId: asset))
        try draft.update(position: 1, answer: .notOk, note: "Synthetic note", nowMillis: now)
        try draft.update(position: 2, answer: .notApplicable, note: "", nowMillis: now)
        XCTAssertThrowsError(try draft.evaluate(nowMillis: now)) {
            XCTAssertEqual($0 as? EnterpriseReturnChecklistError, .acknowledgementRequired)
        }
        try draft.acknowledge(true, nowMillis: now)
        XCTAssertEqual(try draft.evaluate(nowMillis: now).outcome, .completedWithIssues)
        try book.replace(draft, trip: currentTrip)
        try book.remember(knownNone)
        XCTAssertEqual(try book.start(trip: currentTrip, nowMillis: now + 3_600_000), draft)
        XCTAssertThrowsError(try draft.evaluate(nowMillis: now + 3_600_001)) {
            XCTAssertEqual($0 as? EnterpriseReturnChecklistError, .expired)
        }
        XCTAssertThrowsError(try book.start(trip: currentTrip, nowMillis: now + 3_600_001)) {
            XCTAssertEqual($0 as? EnterpriseReturnChecklistError, .notRequired)
        }
    }

    func testCompanyRunReadRejectsForeignBindingsAndUntrustedReceiptShape() throws {
        let response: [String: Any] = [
            "run_id": runID, "asset_id": asset, "template_id": template, "context": "RETURN", "template_version": 1,
            "policy_revision": 3, "server_policy_revision": 4, "revision": 1, "started_at": "2026-11-15T11:40:00Z",
            "completed_at": "2026-11-15T12:00:00Z", "received_at": "2026-11-15T12:01:00Z",
            "device_outcome": "COMPLETED_WITH_ISSUES", "recheck_outcome": "BLOCKING", "outcome": "BLOCKING",
            "outcome_differs": true, "review_required": true, "template_superseded": true, "acknowledged": true,
            "issues": [2], "critical_issues": [2], "data_mode": "SYNTHETIC",
        ]
        let valid = try XCTUnwrap(EnterpriseCompanyWire.checklistRun(response, assetId: asset, runId: runID))
        XCTAssertEqual(valid.outcome, .blocking)
        XCTAssertTrue(valid.outcomeDiffers)
        XCTAssertNil(EnterpriseCompanyWire.checklistRun(response, assetId: template, runId: runID))
        XCTAssertNil(EnterpriseCompanyWire.checklistRun(response, assetId: asset, runId: tripID))
        let invalidFields: [(String, Any)] = [("context", "DEPARTURE"), ("review_required", false), ("critical_issues", [1]),
                                              ("issues", [2, 2]), ("revision", 2), ("photo", true)]
        for (key, value) in invalidFields {
            var invalid = response
            invalid[key] = value
            XCTAssertNil(EnterpriseCompanyWire.checklistRun(invalid, assetId: asset, runId: runID), key)
        }
    }

    func testReenrolledDeviceStartsANewBookAndKeepsQueuedWork() throws {
        var held = EnterprisePartitionSnapshot(partition: try partition(), membershipId: member)
        var book = EnterpriseReturnChecklistBook(membershipId: member, enrollmentId: enrollment)
        try book.remember(applicable())
        _ = try book.start(trip: trip(), nowMillis: now)
        held.returnChecklists = book
        held.trips = [trip()]
        held.dropForeignBindings(membershipId: member, enrollmentId: enrollment, employeeId: nil)
        XCTAssertEqual(held.returnChecklists, book, "the same enrollment keeps its book")

        let reenrolled = "7b0e2a54-0000-4000-8000-000000009906"
        held.dropForeignBindings(membershipId: member, enrollmentId: reenrolled, employeeId: nil)
        XCTAssertNil(held.returnChecklists, "a stale book would refuse every later change")
        XCTAssertEqual(held.trips.count, 1)
        var fresh = held.returnChecklists ?? EnterpriseReturnChecklistBook(membershipId: member, enrollmentId: reenrolled)
        try fresh.remember(applicable())
        let restarted = try fresh.start(trip: trip(), nowMillis: now)
        XCTAssertEqual(restarted.tripId, tripID)

        held.returnChecklists = book
        held.dropForeignBindings(membershipId: "7b0e2a54-0000-4000-8000-000000009907", enrollmentId: enrollment, employeeId: nil)
        XCTAssertNil(held.returnChecklists, "another membership never reads this book")
    }

    func testProtectedSnapshotKeepsBoundRunAndQueueAcrossRestartAndPurge() throws {
        let root = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString, isDirectory: true)
        defer { try? FileManager.default.removeItem(at: root) }
        let store = EnterprisePartitionStore(directory: root)
        let scope = try partition()
        var held = EnterprisePartitionSnapshot(partition: scope, membershipId: member)
        let trip = trip()
        held.trips = [trip]
        var book = EnterpriseReturnChecklistBook(membershipId: member, enrollmentId: enrollment)
        try book.remember(applicable())
        var draft = try book.start(trip: trip, nowMillis: now)
        try draft.update(position: 1, answer: .ok, note: "", nowMillis: now)
        try draft.update(position: 2, answer: .ok, note: "", nowMillis: now)
        let envelope = EnterpriseEnvelope(partition: scope, membershipId: member, enrollmentId: enrollment,
                                          operationId: operation, declared: .at(epochMillis: now, zone: .gmt))
        let command = try draft.complete(envelope: envelope, nowMillis: now)
        try book.replace(draft, trip: trip)
        try EnterpriseQueue.enqueue(EnterpriseQueueItem(operation: command, kind: .checklist, summary: "Fixture", createdAtMillis: now), into: &held)
        held.returnChecklists = book
        held.queue[0].status = .accepted
        EnterpriseQueue.dismiss(id: command.operationId, in: &held)
        XCTAssertEqual(held.queue.first?.id, command.operationId)
        XCTAssertTrue(store.save(held))
        let reopened = EnterprisePartitionStore(directory: root)
        let recovered = try XCTUnwrap(reopened.load(scope))
        XCTAssertEqual(recovered.returnChecklists?.draft?.completion?.operationId, command.operationId)
        XCTAssertEqual(recovered.queue.first?.operation.canonical, command.canonical)
        XCTAssertNil(reopened.load(try partition(organizationId: template)))
        var old = try XCTUnwrap(JSONSerialization.jsonObject(with: JSONEncoder().encode(held)) as? [String: Any])
        old["returnChecklists"] = nil
        let earlier = try JSONDecoder().decode(EnterprisePartitionSnapshot.self, from: JSONSerialization.data(withJSONObject: old))
        XCTAssertNil(earlier.returnChecklists)
        reopened.purgeAll()
        XCTAssertNil(reopened.load(scope))
    }
}

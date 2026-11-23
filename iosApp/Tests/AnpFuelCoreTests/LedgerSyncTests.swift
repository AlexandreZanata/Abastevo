import Foundation
import XCTest
@testable import AnpFuelCore

/// Private ledger sync: wire shapes against `contracts/testdata/api/ledger-*.json` (values copied
/// verbatim), queue reconciliation, conflicts, capability lapse, tombstones, cursor paging and
/// account- and origin-scoped persistence.
final class LedgerSyncTests: XCTestCase {
    private let account = "a0000000-0000-4000-8000-000000000001"
    private let origin = "https://abastevo.com.br"
    private let vehicleId = "10000000-0000-4000-8000-000000000001"
    private let recordId = "77777777-7777-4777-8777-777777777777"
    /// 2026-03-01T12:00:00Z.
    private let instant: Int64 = 1_772_366_400_000
    private let fixtureSession = PortableAuth.Session(
        familyId: "11111111-1111-4111-8111-111111111111",
        accountId: "a0000000-0000-4000-8000-000000000001",
        accessToken: "synthetic",
        refreshToken: "refresh",
        accessExpiresAt: 2_000_000_000,
        absoluteExpiresAt: 2_100_000_000
    )

    private func scope() throws -> LedgerSyncScope {
        return try XCTUnwrap(LedgerSyncScope(accountId: account, origin: origin))
    }

    private func enabledState(_ datasets: [LedgerDataset] = LedgerDataset.allCases) throws -> LedgerSyncState {
        var state = LedgerSyncState(scope: try scope())
        state = LedgerSyncEngine.applyConsent(state, status: LedgerConsentStatus(revision: 1, datasets: datasets), enabled: true)
        return state
    }

    private func record(id: String? = nil, total: Int64 = 23_000, note: String = "", review: Bool = false) -> LedgerEntryRecord {
        return LedgerEntryRecord(
            id: id ?? recordId, vehicleId: vehicleId, kind: .refuel, occurredAtMillis: instant, timezone: "America/Sao_Paulo",
            fuel: .ethanol, quantityMilli: 40_000, totalCentavos: total, fill: .full, odometerMetres: 1_000_000, note: note,
            reviewRequired: review
        )
    }

    private func object(_ data: Data?) throws -> [String: Any] {
        let bytes = try XCTUnwrap(data)
        return try XCTUnwrap((try JSONSerialization.jsonObject(with: bytes)) as? [String: Any])
    }

    // MARK: Wire shapes (`contracts/testdata/api`)

    func testCommandBodyMatchesTheLedgerCommandsFixture() throws {
        let command = LedgerQueuedCommand(
            operationId: "66666666-6666-4666-8666-666666666666",
            dataset: .refuel,
            recordId: recordId,
            expectedRevision: 0,
            isDelete: false,
            payloadJson: try XCTUnwrap(LedgerWire.payloadJson(record())),
            createdAtMillis: 0
        )
        let data = try XCTUnwrap(LedgerSyncWire.commandBody(session: fixtureSession, command: command))
        let body = try object(data)
        XCTAssertEqual(body["family_id"] as? String, "11111111-1111-4111-8111-111111111111")
        XCTAssertEqual(body["access_token"] as? String, "synthetic")
        let commands = try XCTUnwrap(body["commands"] as? [[String: Any]])
        XCTAssertEqual(commands.count, 1)
        XCTAssertEqual(commands[0]["operation_id"] as? String, "66666666-6666-4666-8666-666666666666")
        XCTAssertEqual(commands[0]["dataset"] as? String, "refuel")
        XCTAssertEqual(commands[0]["id"] as? String, "77777777-7777-4777-8777-777777777777")
        XCTAssertEqual(commands[0]["expected_revision"] as? Int64, 0)
        XCTAssertEqual(commands[0]["delete"] as? Bool, false)
        XCTAssertEqual(commands[0]["void"] as? Bool, false)
        let payload = try XCTUnwrap(commands[0]["payload"] as? [String: Any])
        XCTAssertEqual(payload["quantity_milli"] as? Int64, 40_000)
        let text = String(decoding: data, as: UTF8.self)
        XCTAssertTrue(text.contains("\"payload\":" + command.payloadJson + "}"), "the payload travels byte for byte")

        let delete = LedgerQueuedCommand(
            operationId: "66666666-6666-4666-8666-666666666666", dataset: .expense, recordId: recordId,
            expectedRevision: 0, isDelete: true, payloadJson: "", createdAtMillis: 0
        )
        let deleteBody = try object(LedgerSyncWire.commandBody(session: fixtureSession, command: delete))
        let deleteCommand = try XCTUnwrap((deleteBody["commands"] as? [[String: Any]])?.first)
        XCTAssertEqual(deleteCommand["delete"] as? Bool, true)
        XCTAssertTrue(deleteCommand["payload"] is NSNull)

        let foreign = LedgerQueuedCommand(
            operationId: "66666666-6666-4666-8666-666666666666", dataset: .refuel, recordId: "88888888-8888-4888-8888-888888888888",
            expectedRevision: 0, isDelete: false, payloadJson: command.payloadJson, createdAtMillis: 0
        )
        XCTAssertNil(LedgerSyncWire.commandBody(session: fixtureSession, command: foreign), "payload id must match the command id")
    }

    func testPageConsentAndEraseBodiesMatchTheirFixtures() throws {
        let page = try object(LedgerSyncWire.recordsBody(session: fixtureSession, dataset: .refuel, vehicleId: nil, limit: 20, cursor: nil))
        XCTAssertEqual(page["family_id"] as? String, "11111111-1111-4111-8111-111111111111")
        XCTAssertEqual(page["access_token"] as? String, "synthetic")
        XCTAssertEqual(page["dataset"] as? String, "refuel")
        XCTAssertEqual(page["limit"] as? Int, 20)
        XCTAssertNil(page["cursor"])
        XCTAssertNil(LedgerSyncWire.recordsBody(session: fixtureSession, dataset: .refuel, vehicleId: nil, limit: 101, cursor: nil))
        XCTAssertNil(LedgerSyncWire.recordsBody(session: fixtureSession, dataset: .refuel, vehicleId: nil, limit: 20, cursor: "bad\ncursor"))
        let paged = try object(LedgerSyncWire.recordsBody(session: fixtureSession, dataset: .expense, vehicleId: vehicleId, limit: 100, cursor: "opaque.cursor"))
        XCTAssertEqual(paged["cursor"] as? String, "opaque.cursor")
        XCTAssertEqual(paged["vehicle_id"] as? String, vehicleId)

        let consent = try object(LedgerSyncWire.consentBody(session: fixtureSession, datasets: [.refuel], enabled: true))
        XCTAssertEqual(consent["datasets"] as? [String], ["refuel"])
        XCTAssertEqual(consent["enabled"] as? Bool, true)
        XCTAssertNil(LedgerSyncWire.consentBody(session: fixtureSession, datasets: [], enabled: true))
        XCTAssertNil(LedgerSyncWire.consentBody(session: fixtureSession, datasets: [.refuel, .refuel], enabled: true))

        let erase = try object(LedgerSyncWire.eraseBody(session: fixtureSession))
        XCTAssertEqual(Set(erase.keys), ["family_id", "access_token"])
    }

    func testReceiptConsentAndErasureDecoding() {
        let command = LedgerQueuedCommand(
            operationId: "66666666-6666-4666-8666-666666666666", dataset: .refuel, recordId: recordId,
            expectedRevision: 0, isDelete: false, payloadJson: "{}", createdAtMillis: 0
        )
        let valid = Data("""
        {"results":[{"operation_id":"66666666-6666-4666-8666-666666666666","dataset":"refuel","id":"\(recordId)","revision":3,"status":"accepted","replayed":true}]}
        """.utf8)
        let receipt = LedgerSyncWire.decodeReceipt(valid, command: command)
        XCTAssertEqual(receipt?.revision, 3)
        XCTAssertEqual(receipt?.replayed, true)
        XCTAssertNil(LedgerSyncWire.decodeReceipt(Data(String(decoding: valid, as: UTF8.self).replacingOccurrences(of: "\"revision\":3", with: "\"revision\":0").utf8), command: command))
        XCTAssertNil(LedgerSyncWire.decodeReceipt(Data(String(decoding: valid, as: UTF8.self).replacingOccurrences(of: "accepted", with: "pending").utf8), command: command))
        XCTAssertNil(LedgerSyncWire.decodeReceipt(Data(String(decoding: valid, as: UTF8.self).replacingOccurrences(of: "66666666-6666-4666-8666-666666666666", with: "55555555-5555-4555-8555-555555555555").utf8), command: command))
        XCTAssertNil(LedgerSyncWire.decodeReceipt(Data("{\"results\":[]}".utf8), command: command))

        XCTAssertEqual(LedgerSyncWire.decodeConsent(Data("{\"revision\":2,\"datasets\":[\"refuel\",\"charge\"]}".utf8)), LedgerConsentStatus(revision: 2, datasets: [.refuel, .charge]))
        XCTAssertNil(LedgerSyncWire.decodeConsent(Data("{\"revision\":2,\"datasets\":[\"professional\"]}".utf8)))
        XCTAssertTrue(LedgerSyncWire.decodeErase(Data("{\"status\":\"erased\"}".utf8)))
        XCTAssertFalse(LedgerSyncWire.decodeErase(Data("{\"status\":\"pending\"}".utf8)))
    }

    private func pageJson(rows: [String], next: String) -> Data {
        return Data("{\"items\":[\(rows.joined(separator: ","))],\"next_cursor\":\"\(next)\"}".utf8)
    }

    private func rowJson(id: String, revision: Int64, total: Int64 = 23_000, voided: Bool = false, dataset: String = "refuel", vehicle: String? = nil) -> String {
        let owner = vehicle ?? vehicleId
        return """
        {"dataset":"\(dataset)","id":"\(id)","vehicle_id":"\(owner)","occurred_at":"2026-03-01T12:00:00Z","revision":\(revision),"voided":\(voided),"payload":{"id":"\(id)","vehicle_id":"\(owner)","occurred_at":"2026-03-01T12:00:00Z","timezone":"America/Sao_Paulo","fuel":"ETHANOL","unit":"L","quantity_milli":40000,"total_centavos":\(total),"odometer_metres":1000000,"fill":"full"}}
        """
    }

    func testRecordPagesAreBoundedOwnedAndOpaque() throws {
        let page = try XCTUnwrap(LedgerSyncWire.decodeRecordsPage(pageJson(rows: [rowJson(id: recordId, revision: 2)], next: "c1"), dataset: .refuel, vehicleId: nil, limit: 20))
        XCTAssertEqual(page.items.count, 1)
        XCTAssertEqual(page.nextCursor, "c1")
        XCTAssertEqual(page.items[0].revision, 2)
        XCTAssertEqual(page.items[0].record?.totalCentavos, 23_000)
        XCTAssertEqual(page.items[0].canonicalPayload, LedgerWire.payloadJson(record()))

        let last = try XCTUnwrap(LedgerSyncWire.decodeRecordsPage(pageJson(rows: [], next: ""), dataset: .refuel, vehicleId: nil, limit: 20))
        XCTAssertNil(last.nextCursor, "an empty cursor ends the listing")
        XCTAssertNil(LedgerSyncWire.decodeRecordsPage(pageJson(rows: [rowJson(id: recordId, revision: 2, dataset: "expense")], next: ""), dataset: .refuel, vehicleId: nil, limit: 20))
        XCTAssertNil(LedgerSyncWire.decodeRecordsPage(pageJson(rows: [rowJson(id: recordId, revision: 2), rowJson(id: recordId, revision: 3)], next: ""), dataset: .refuel, vehicleId: nil, limit: 20))
        XCTAssertNil(LedgerSyncWire.decodeRecordsPage(pageJson(rows: [rowJson(id: recordId, revision: 2)], next: ""), dataset: .refuel, vehicleId: "20000000-0000-4000-8000-000000000002", limit: 20))
        XCTAssertNil(LedgerSyncWire.decodeRecordsPage(pageJson(rows: [rowJson(id: recordId, revision: 2), rowJson(id: "88888888-8888-4888-8888-888888888888", revision: 1)], next: ""), dataset: .refuel, vehicleId: nil, limit: 1))
        XCTAssertNil(LedgerSyncWire.decodeRecordsPage(pageJson(rows: [rowJson(id: recordId, revision: 0)], next: ""), dataset: .refuel, vehicleId: nil, limit: 20))

        let odd = rowJson(id: recordId, revision: 2).replacingOccurrences(of: "\"fill\":\"full\"", with: "\"fill\":\"half\"")
        let kept = try XCTUnwrap(LedgerSyncWire.decodeRecordsPage(pageJson(rows: [odd], next: ""), dataset: .refuel, vehicleId: nil, limit: 20))
        XCTAssertNil(kept.items[0].record, "an unreadable payload is reported, never invented")
    }

    // MARK: Queue and dispatch

    func testReconcileQueuesOnlyChangedConsentedSyncableRecords() throws {
        let disabled = LedgerSyncState(scope: try scope())
        XCTAssertTrue(LedgerSyncEngine.reconcile(disabled, records: [record()], nowMillis: 1).commands.isEmpty, "no upload without opt-in")

        var state = LedgerSyncEngine.reconcile(try enabledState(), records: [record()], nowMillis: 1)
        XCTAssertEqual(state.commands.count, 1)
        let first = try XCTUnwrap(state.commands.first)
        XCTAssertEqual(first.expectedRevision, 0)
        XCTAssertEqual(first.payloadJson, LedgerWire.payloadJson(record()))
        XCTAssertEqual(first.operationId, LedgerSyncEngine.operationId(scope: try scope(), dataset: .refuel, recordId: recordId, expectedRevision: 0, payloadJson: first.payloadJson))
        XCTAssertEqual(LedgerSyncEngine.reconcile(state, records: [record()], nowMillis: 2).commands, state.commands, "double saves converge")

        state = LedgerSyncEngine.applyReceipt(
            state, command: first,
            receipt: LedgerCommandReceipt(operationId: first.operationId, dataset: .refuel, recordId: recordId, revision: 1, replayed: false),
            nowMillis: 3
        )
        XCTAssertEqual(state.acked[first.key]?.revision, 1)
        XCTAssertTrue(LedgerSyncEngine.reconcile(state, records: [record()], nowMillis: 4).commands.isEmpty)
        XCTAssertEqual(LedgerSyncEngine.status(of: record(), in: state), .synced(revision: 1))

        let edited = LedgerSyncEngine.reconcile(state, records: [record(total: 24_000)], nowMillis: 5)
        XCTAssertEqual(edited.commands.count, 1)
        XCTAssertEqual(edited.commands.first?.expectedRevision, 1)
        XCTAssertNotEqual(edited.commands.first?.operationId, first.operationId)

        XCTAssertTrue(LedgerSyncEngine.reconcile(try enabledState([.expense]), records: [record()], nowMillis: 1).commands.isEmpty)
        XCTAssertTrue(LedgerSyncEngine.reconcile(try enabledState(), records: [record(review: true)], nowMillis: 1).commands.isEmpty)
        XCTAssertEqual(LedgerSyncEngine.status(of: record(review: true), in: state), .reviewRequired)
    }

    func testConflictKeepsBaseCurrentAndProposedUntilTheOwnerDecides() async throws {
        var state = LedgerSyncEngine.reconcile(try enabledState(), records: [record()], nowMillis: 1)
        let create = try XCTUnwrap(state.commands.first)
        state = LedgerSyncEngine.applyReceipt(
            state, command: create,
            receipt: LedgerCommandReceipt(operationId: create.operationId, dataset: .refuel, recordId: recordId, revision: 1, replayed: false),
            nowMillis: 2
        )
        let mine = record(total: 24_000, note: "minha")
        state = LedgerSyncEngine.reconcile(state, records: [mine], nowMillis: 3)

        let gateway = FakeLedgerGateway()
        gateway.submitErrors = [SignedTransportError.refused(status: 409, code: "ledger.conflict", retryAfterSeconds: nil)]
        gateway.pages = ["": LedgerRecordsPage(items: [serverRow(revision: 3, total: 25_000)], nextCursor: nil)]
        state = await LedgerSyncDispatcher.drain(state, records: [mine], gateway: gateway, nowMillis: 4)

        XCTAssertTrue(state.commands.isEmpty, "the stale command is parked, never overwritten by arrival order")
        let conflict = try XCTUnwrap(state.conflicts.first)
        XCTAssertEqual(conflict.expectedRevision, 1)
        XCTAssertEqual(conflict.basePayloadJson, LedgerWire.payloadJson(record()))
        XCTAssertEqual(conflict.proposedPayloadJson, LedgerWire.payloadJson(mine))
        XCTAssertEqual(conflict.current.state, .present)
        XCTAssertEqual(conflict.current.revision, 3)
        XCTAssertEqual(conflict.current.payloadJson, LedgerWire.payloadJson(record(total: 25_000)))
        XCTAssertEqual(gateway.listedVehicleIds, [vehicleId], "the lookup reads only the record's vehicle")
        XCTAssertEqual(LedgerSyncEngine.status(of: mine, in: state), .conflict)

        let editedAgain = record(total: 26_000, note: "minha")
        let held = LedgerSyncEngine.reconcile(state, records: [editedAgain], nowMillis: 5)
        XCTAssertTrue(held.commands.isEmpty)
        XCTAssertEqual(held.conflicts.first?.proposedPayloadJson, LedgerWire.payloadJson(editedAgain))

        guard case .requeued(let kept) = LedgerSyncEngine.resolve(state, conflictKey: conflict.key, choice: .keepMine, localRecord: mine, nowMillis: 6) else {
            return XCTFail("keep mine should requeue")
        }
        let rebased = LedgerSyncEngine.reconcile(kept, records: [mine], nowMillis: 7)
        XCTAssertTrue(rebased.conflicts.isEmpty)
        XCTAssertEqual(rebased.commands.first?.expectedRevision, 3, "resolution is a new command at the current revision")

        guard case .adoptServer(let adopted, let server) = LedgerSyncEngine.resolve(state, conflictKey: conflict.key, choice: .takeServer, localRecord: mine, nowMillis: 6) else {
            return XCTFail("take server should adopt")
        }
        XCTAssertEqual(server.totalCentavos, 25_000)
        XCTAssertTrue(adopted.commands.isEmpty)
        XCTAssertEqual(adopted.acked[conflict.key]?.revision, 3)

        let gone = LedgerSyncEngine.updateConflictCurrent(state, conflictKey: conflict.key, current: .missing)
        guard case .unavailable = LedgerSyncEngine.resolve(gone, conflictKey: conflict.key, choice: .keepMine, localRecord: mine, nowMillis: 6) else {
            return XCTFail("a deleted server record cannot be recreated under its identity")
        }
        guard case .removeLocal(let removed) = LedgerSyncEngine.resolve(gone, conflictKey: conflict.key, choice: .takeServer, localRecord: mine, nowMillis: 6) else {
            return XCTFail("deletion wins when the owner takes the server side")
        }
        XCTAssertTrue(removed.deletedKeys.contains(conflict.key))
        let unknown = LedgerSyncEngine.updateConflictCurrent(state, conflictKey: conflict.key, current: .unknown)
        guard case .unavailable = LedgerSyncEngine.resolve(unknown, conflictKey: conflict.key, choice: .takeServer, localRecord: mine, nowMillis: 6) else {
            return XCTFail("an unknown server side needs a refresh first")
        }
    }

    func testCapabilityLapsePausesPaidUploadsKeepsDataAndLetsDeletesThrough() async throws {
        let other = record(id: "88888888-8888-4888-8888-888888888888")
        var state = LedgerSyncEngine.reconcile(try enabledState(), records: [record(), other], nowMillis: 1)
        XCTAssertEqual(state.commands.count, 2)
        let gateway = FakeLedgerGateway()
        gateway.submitErrors = [SignedTransportError.refused(status: 403, code: "ledger.capability-required", retryAfterSeconds: nil)]
        state = await LedgerSyncDispatcher.drain(state, records: [record(), other], gateway: gateway, nowMillis: 2)
        XCTAssertTrue(state.paidUploadsPaused)
        XCTAssertEqual(state.pauseCode, "ledger.capability-required")
        XCTAssertEqual(state.commands.count, 2, "held work is never deleted on a lapse")
        XCTAssertEqual(gateway.submitted.count, 1, "no further paid upload after the 403")
        XCTAssertNil(LedgerSyncEngine.nextDispatchable(state, nowMillis: 3))
        XCTAssertEqual(LedgerSyncEngine.status(of: record(), in: state), .paused)

        let acked = LedgerSyncEngine.applyReceipt(
            LedgerSyncEngine.reconcile(try enabledState(), records: [other], nowMillis: 1),
            command: try XCTUnwrap(LedgerSyncEngine.reconcile(try enabledState(), records: [other], nowMillis: 1).commands.first),
            receipt: LedgerCommandReceipt(operationId: "x", dataset: .refuel, recordId: other.id, revision: 1, replayed: false),
            nowMillis: 2
        )
        var lapsed = acked
        lapsed.paidUploadsPaused = true
        lapsed = LedgerSyncEngine.markDeleted(lapsed, dataset: .refuel, recordId: other.id, nowMillis: 4)
        let delete = try XCTUnwrap(LedgerSyncEngine.nextDispatchable(lapsed, nowMillis: 5))
        XCTAssertTrue(delete.isDelete, "free deletes keep flowing after a lapse")
        XCTAssertEqual(delete.operationId, LedgerSyncEngine.deleteOperationId(scope: try scope(), dataset: .refuel, recordId: other.id))

        let resumed = LedgerSyncEngine.retryNow(state)
        XCTAssertFalse(resumed.paidUploadsPaused)
        XCTAssertNotNil(LedgerSyncEngine.nextDispatchable(resumed, nowMillis: 6))
    }

    func testDeletesAreTombstonedAndADeleteOfAnUnknownRecordConverges() async throws {
        // A create that may have reached the server (reply lost) is followed by a free delete.
        var state = LedgerSyncEngine.reconcile(try enabledState(), records: [record()], nowMillis: 1)
        state = LedgerSyncEngine.markDeleted(state, dataset: .refuel, recordId: recordId, nowMillis: 1)
        XCTAssertTrue(state.deletedKeys.contains(LedgerSyncKeys.key(.refuel, recordId)))
        XCTAssertEqual(state.commands.count, 1)
        XCTAssertEqual(state.commands.first?.isDelete, true)
        XCTAssertTrue(LedgerSyncEngine.reconcile(state, records: [record()], nowMillis: 2).commands.allSatisfy { $0.isDelete }, "a tombstoned record is never uploaded again")
        let gateway = FakeLedgerGateway()
        gateway.submitErrors = [SignedTransportError.refused(status: 409, code: "ledger.conflict", retryAfterSeconds: nil)]
        state = await LedgerSyncDispatcher.drain(state, records: [], gateway: gateway, nowMillis: 2)
        XCTAssertEqual(gateway.submitted.first?.isDelete, true)
        XCTAssertTrue(state.commands.isEmpty, "the server never held it; nothing remains to delete")
        XCTAssertTrue(state.conflicts.isEmpty)

        let guest = LedgerSyncEngine.markDeleted(LedgerSyncState(scope: try scope()), dataset: .refuel, recordId: recordId, nowMillis: 1)
        XCTAssertTrue(guest.commands.isEmpty, "a never-uploaded record needs no server delete")
    }

    func testFailuresRetryWithBackoffAndRefusalsPark() async throws {
        XCTAssertEqual(LedgerSyncEngine.classify(SignedTransportError.offline, nowMillis: 0), .retry(code: "ledger.offline", notBeforeMillis: nil))
        XCTAssertEqual(LedgerSyncEngine.classify(SignedTransportError.refused(status: 429, code: nil, retryAfterSeconds: 30), nowMillis: 1000), .retry(code: "http-429", notBeforeMillis: 31_000))
        XCTAssertEqual(LedgerSyncEngine.classify(SignedTransportError.refused(status: 503, code: "ledger.unavailable", retryAfterSeconds: nil), nowMillis: 0), .retry(code: "ledger.unavailable", notBeforeMillis: nil))
        XCTAssertEqual(LedgerSyncEngine.classify(SignedTransportError.refused(status: 401, code: "ledger.auth-required", retryAfterSeconds: nil), nowMillis: 0), .retry(code: "ledger.auth-required", notBeforeMillis: nil))
        XCTAssertEqual(LedgerSyncEngine.classify(SignedTransportError.refused(status: 400, code: "ledger.invalid", retryAfterSeconds: nil), nowMillis: 0), .refused(code: "ledger.invalid"))
        XCTAssertEqual(LedgerSyncEngine.classify(SignedTransportError.refused(status: 403, code: nil, retryAfterSeconds: nil), nowMillis: 0), .capabilityRequired(code: "ledger.capability-required"))
        XCTAssertEqual(LedgerSyncEngine.classify(SignedTransportError.refused(status: 409, code: nil, retryAfterSeconds: nil), nowMillis: 0), .conflict)
        XCTAssertEqual(LedgerSyncEngine.classify(SignedTransportError.sessionRequired, nowMillis: 0), .signInRequired)
        XCTAssertEqual(LedgerSyncEngine.classify(LedgerSyncError.ownerMismatch, nowMillis: 0), .signInRequired)
        XCTAssertEqual(LedgerSyncEngine.backoffMillis(attempts: 1), 1_000)
        XCTAssertEqual(LedgerSyncEngine.backoffMillis(attempts: 3), 4_000)
        XCTAssertEqual(LedgerSyncEngine.backoffMillis(attempts: 40), 3_600_000)

        let gateway = FakeLedgerGateway()
        gateway.submitErrors = [SignedTransportError.offline]
        var state = LedgerSyncEngine.reconcile(try enabledState(), records: [record()], nowMillis: 1)
        state = await LedgerSyncDispatcher.drain(state, records: [record()], gateway: gateway, nowMillis: 10_000)
        let retried = try XCTUnwrap(state.commands.first)
        XCTAssertEqual(retried.attempts, 1)
        XCTAssertEqual(retried.nextAttemptAtMillis, 11_000)
        XCTAssertEqual(retried.status, .pending)
        XCTAssertNil(LedgerSyncEngine.nextDispatchable(state, nowMillis: 10_500))

        gateway.submitErrors = [SignedTransportError.refused(status: 400, code: "ledger.invalid", retryAfterSeconds: nil)]
        state = await LedgerSyncDispatcher.drain(state, records: [record()], gateway: gateway, nowMillis: 20_000)
        XCTAssertEqual(state.commands.first?.status, .refused)
        XCTAssertEqual(LedgerSyncEngine.status(of: record(), in: state), .refused(code: "ledger.invalid"))
        XCTAssertEqual(LedgerSyncEngine.reconcile(state, records: [record()], nowMillis: 21_000).commands.first?.status, .refused, "a refusal is not retried silently")

        gateway.submitErrors = []
        let accepted = await LedgerSyncDispatcher.drain(LedgerSyncEngine.retryNow(state), records: [record()], gateway: gateway, nowMillis: 30_000)
        XCTAssertTrue(accepted.commands.isEmpty)
        XCTAssertEqual(accepted.acked[LedgerSyncKeys.key(.refuel, recordId)]?.revision, 1)
        XCTAssertEqual(gateway.submitted.last?.operationId, retried.operationId, "retries keep the operation id")
    }

    // MARK: Pull

    private func serverRow(id: String? = nil, revision: Int64, total: Int64 = 23_000, voided: Bool = false) -> LedgerServerRecord {
        let identifier = id ?? recordId
        return LedgerServerRecord(
            dataset: .refuel, id: identifier, vehicleId: vehicleId, occurredAtMillis: instant, revision: revision,
            voided: voided, record: record(id: identifier, total: total)
        )
    }

    func testPullPagesThroughOpaqueCursorsWithoutResurrectionOrOverwrite() async throws {
        let deletedId = "99999999-9999-4999-8999-999999999999"
        let heldId = "88888888-8888-4888-8888-888888888888"
        let cleanId = "66666666-6666-4666-8666-666666666666"
        let gateway = FakeLedgerGateway()
        gateway.pages = [
            "": LedgerRecordsPage(items: [serverRow(id: recordId, revision: 1), serverRow(id: cleanId, revision: 5, total: 30_000)], nextCursor: "c1"),
            "c1": LedgerRecordsPage(items: [serverRow(id: deletedId, revision: 2), serverRow(id: heldId, revision: 4), serverRow(id: "55555555-5555-4555-8555-555555555555", revision: 1, voided: true)], nextCursor: nil),
        ]
        var state = try enabledState()
        state = LedgerSyncEngine.markDeleted(state, dataset: .refuel, recordId: deletedId, nowMillis: 1)
        state.acked[LedgerSyncKeys.key(.refuel, cleanId)] = LedgerAck(revision: 2, payloadJson: try XCTUnwrap(LedgerWire.payloadJson(record(id: cleanId))), atMillis: 1)
        var local = [record(id: cleanId), record(id: heldId, total: 1)]
        state = LedgerSyncEngine.reconcile(state, records: local, nowMillis: 1)
        XCTAssertTrue(state.commands.contains { $0.recordId == heldId })

        var report = LedgerPullReport()
        var cursor: String?
        var pages = 0
        repeat {
            let page = try await gateway.listRecords(accountId: account, dataset: .refuel, vehicleId: nil, limit: 100, cursor: cursor)
            let merged = LedgerSyncEngine.mergePage(state, records: local, page: page.items, report: report, nowMillis: 2)
            state = merged.0
            local = merged.1
            report = merged.2
            cursor = page.nextCursor
            pages += 1
        } while cursor != nil && pages < 5

        XCTAssertEqual(gateway.cursorsSeen, [nil, "c1"], "the cursor is returned verbatim")
        XCTAssertEqual(report.pages, 2)
        XCTAssertEqual(report.added, 1)
        XCTAssertEqual(report.updated, 1)
        XCTAssertEqual(report.skippedDeleted, 1)
        XCTAssertEqual(report.skippedHeld, 1)
        XCTAssertEqual(report.skippedVoided, 1)
        XCTAssertFalse(local.contains { $0.id == deletedId }, "a local delete is never resurrected")
        XCTAssertEqual(local.first { $0.id == cleanId }?.totalCentavos, 30_000, "a clean record adopts the newer revision")
        XCTAssertEqual(local.first { $0.id == heldId }?.totalCentavos, 1, "pending local work is never overwritten")
        XCTAssertEqual(state.acked[LedgerSyncKeys.key(.refuel, recordId)]?.revision, 1)
        XCTAssertTrue(LedgerSyncEngine.reconcile(state, records: local, nowMillis: 3).commands.allSatisfy { $0.recordId != recordId })

        state.pullCursors["refuel"] = LedgerPullCursor(cursor: "c1", savedAtMillis: 1_000)
        XCTAssertEqual(LedgerSyncEngine.resumableCursor(state, dataset: .refuel, nowMillis: 1_000 + 60_000), "c1")
        XCTAssertNil(LedgerSyncEngine.resumableCursor(state, dataset: .refuel, nowMillis: 1_000 + 15 * 60_000))
    }

    func testErasureKeepsLocalRecordsOnlyOnThisDevice() throws {
        var state = LedgerSyncEngine.reconcile(try enabledState(), records: [record()], nowMillis: 1)
        state = LedgerSyncEngine.applyErasure(state, records: [record()])
        XCTAssertFalse(state.enabled)
        XCTAssertTrue(state.commands.isEmpty)
        XCTAssertEqual(LedgerSyncEngine.status(of: record(), in: state), .erasedFromCloud)
        let reenabled = LedgerSyncEngine.applyConsent(state, status: LedgerConsentStatus(revision: 3, datasets: LedgerDataset.allCases), enabled: true)
        XCTAssertTrue(LedgerSyncEngine.reconcile(reenabled, records: [record()], nowMillis: 2).commands.isEmpty, "erased records are not uploaded again")
        XCTAssertEqual(LedgerSyncEngine.reconcile(reenabled, records: [record(id: "88888888-8888-4888-8888-888888888888")], nowMillis: 2).commands.count, 1)
    }

    // MARK: Persistence scoped by account and origin

    func testStoreNeverMixesAccountsOrOrigins() throws {
        let directory = FileManager.default.temporaryDirectory.appendingPathComponent("ledger-sync-\(UUID().uuidString)", isDirectory: true)
        defer { try? FileManager.default.removeItem(at: directory) }
        let store = LedgerSyncStore(directory: directory)
        let mine = try XCTUnwrap(LedgerSyncScope(accountId: account, origin: origin))
        let mineElsewhere = try XCTUnwrap(LedgerSyncScope(accountId: account, origin: "http://127.0.0.1:18080"))
        let theirs = try XCTUnwrap(LedgerSyncScope(accountId: "b0000000-0000-4000-8000-000000000002", origin: origin))
        XCTAssertNil(LedgerSyncScope(accountId: nil, origin: origin))
        XCTAssertNil(LedgerSyncScope(accountId: account, origin: " "))

        var first = LedgerSyncEngine.reconcile(try enabledState(), records: [record()], nowMillis: 1)
        first.pullCursors["refuel"] = LedgerPullCursor(cursor: "c9", savedAtMillis: 1)
        store.save(first)
        var second = LedgerSyncState(scope: mineElsewhere)
        second.enabled = true
        store.save(second)
        store.save(LedgerSyncState(scope: theirs))

        XCTAssertEqual(store.load(scope: mine), first)
        XCTAssertEqual(store.load(scope: mineElsewhere).commands.count, 0)
        XCTAssertTrue(store.load(scope: mineElsewhere).enabled)
        XCTAssertTrue(store.load(scope: theirs).commands.isEmpty)
        XCTAssertNil(store.load(scope: theirs).pullCursors["refuel"])

        store.purge(accountId: account)
        XCTAssertEqual(store.load(scope: mine), LedgerSyncState(scope: mine))
        XCTAssertEqual(store.load(scope: mineElsewhere), LedgerSyncState(scope: mineElsewhere))
        XCTAssertEqual(store.load(scope: theirs).scope, theirs)

        let encoded = try JSONEncoder().encode(first)
        XCTAssertEqual(try JSONDecoder().decode(LedgerSyncState.self, from: encoded), first)
        let minimal = Data("{\"scope\":{\"accountId\":\"\(account)\",\"origin\":\"\(origin)\"}}".utf8)
        XCTAssertEqual(try JSONDecoder().decode(LedgerSyncState.self, from: minimal), LedgerSyncState(scope: mine))
    }
}

/// In-memory gateway: scripted submit failures, pages by cursor, and a record of what was sent.
final class FakeLedgerGateway: LedgerSyncGateway, @unchecked Sendable {
    private let lock = NSLock()
    private var _submitErrors: [Error] = []
    private var _pages: [String: LedgerRecordsPage] = [:]
    private var _submitted: [LedgerQueuedCommand] = []
    private var _cursorsSeen: [String?] = []
    private var _listedVehicleIds: [String?] = []
    private var revision: Int64 = 0

    var submitErrors: [Error] {
        get { lock.lock(); defer { lock.unlock() }; return _submitErrors }
        set { lock.lock(); _submitErrors = newValue; lock.unlock() }
    }

    var pages: [String: LedgerRecordsPage] {
        get { lock.lock(); defer { lock.unlock() }; return _pages }
        set { lock.lock(); _pages = newValue; lock.unlock() }
    }

    var submitted: [LedgerQueuedCommand] {
        lock.lock()
        defer { lock.unlock() }
        return _submitted
    }

    var cursorsSeen: [String?] {
        lock.lock()
        defer { lock.unlock() }
        return _cursorsSeen
    }

    var listedVehicleIds: [String?] {
        lock.lock()
        defer { lock.unlock() }
        return _listedVehicleIds
    }

    func submit(_ command: LedgerQueuedCommand, accountId: String) async throws -> LedgerCommandReceipt {
        let failure: Error? = lock.withLock {
            _submitted.append(command)
            return _submitErrors.isEmpty ? nil : _submitErrors.removeFirst()
        }
        if let failure { throw failure }
        let next: Int64 = lock.withLock {
            revision += 1
            return revision
        }
        return LedgerCommandReceipt(operationId: command.operationId, dataset: command.dataset, recordId: command.recordId, revision: next, replayed: false)
    }

    func listRecords(accountId: String, dataset: LedgerDataset, vehicleId: String?, limit: Int, cursor: String?) async throws -> LedgerRecordsPage {
        let page: LedgerRecordsPage? = lock.withLock {
            _cursorsSeen.append(cursor)
            _listedVehicleIds.append(vehicleId)
            return _pages[cursor ?? ""]
        }
        guard let page else { throw SignedTransportError.refused(status: 400, code: "ledger.invalid", retryAfterSeconds: nil) }
        return page
    }

    func setConsent(accountId: String, datasets: [LedgerDataset], enabled: Bool) async throws -> LedgerConsentStatus {
        return LedgerConsentStatus(revision: 1, datasets: enabled ? datasets : [])
    }

    func eraseAll(accountId: String) async throws {}
}

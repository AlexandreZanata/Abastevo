import Foundation
import XCTest
@testable import AnpFuelCore

final class DriverAccountingTests: XCTestCase {
    private let origin = "https://abastevo.com.br"
    private let account = "88888888-8888-4888-8888-888888888888"
    private let outsider = "77777777-7777-4777-8777-777777777777"
    private let recordId = "11111111-1111-4111-8111-111111111111"
    private let vehicleId = "22222222-2222-4222-8222-222222222222"

    func testWireFixturesAreReadVerbatimAndRefusalsAreStrict() throws {
        let receipt = try DriverAccountingWire.receipt(from: fixture("receipt-200"))
        XCTAssertEqual(receipt.id, recordId)
        XCTAssertEqual(receipt.revision, 1)
        XCTAssertEqual(receipt.epoch, 0)
        XCTAssertFalse(receipt.replayed)
        XCTAssertEqual(try DriverAccountingWire.errorCode(status: 401, from: fixture("auth-401")), "driver.auth-required")
        XCTAssertEqual(try DriverAccountingWire.errorCode(status: 409, from: fixture("conflict-409")), "driver.conflict")

        for invalid in [
            #"{"id":"11111111-1111-4111-8111-111111111111","revision":0,"epoch":0,"replayed":false}"#,
            #"{"id":"11111111-1111-4111-8111-111111111111","revision":1,"epoch":-1,"replayed":false}"#,
            #"{"id":"11111111-1111-4111-8111-111111111111","revision":1,"epoch":0,"replayed":1}"#,
            #"{"id":"unknown","revision":1,"epoch":0,"replayed":false}"#,
            #"{"id":"11111111-1111-4111-8111-111111111111","revision":"1","epoch":0}"#,
        ] {
            XCTAssertThrowsError(try DriverAccountingWire.receipt(from: Data(invalid.utf8)))
        }
        XCTAssertThrowsError(try DriverAccountingWire.errorCode(status: 200, from: fixture("auth-401")))
    }

    func testManualDriverMoneyReferenceVectors() throws {
        let file = try fixture("driver-manual-reference-v1", fileExtension: "psv", directory: "driver-suite")
        let rows = String(decoding: file, as: UTF8.self).split(separator: "\n").dropFirst()
        var covered: Set<String> = []
        for row in rows {
            let cells = row.split(separator: "|", omittingEmptySubsequences: false)
            XCTAssertEqual(cells.count, 4)
            guard cells.count == 4, cells[1] == "DETAILED" || cells[1] == "NET" else { continue }
            let args = pairs(cells[2])
            let expected = pairs(cells[3])
            func amount(_ key: String) throws -> Int64 {
                try XCTUnwrap(args[key].flatMap { Int64($0) })
            }
            let money: DriverEarningsMoney?
            if cells[1] == "NET" {
                money = try DriverEarningsMoney.inclusiveNet(amount("inclusive"))
                XCTAssertNil(money?.gross)
                XCTAssertNil(money?.fees)
                XCTAssertNil(money?.tips)
            } else {
                money = try DriverEarningsMoney.detailed(gross: amount("gross"), fees: amount("fees"), tips: amount("tips"),
                                      bonus: amount("bonus"), refund: amount("refund"))
            }
            let result = try XCTUnwrap(money?.earnedCentavos)
            XCTAssertEqual(String(result), expected["earned"])
            covered.insert(String(cells[0]))
        }
        XCTAssertEqual(covered, ["DRV-V06", "DRV-V07", "DRV-M07"])
        XCTAssertNil(DriverEarningsMoney.inclusiveNet(-1))
        XCTAssertNil(DriverEarningsMoney.detailed(gross: 1_000_000_000_000, fees: 0, tips: 1, bonus: 0, refund: 0))
    }

    func testClosedShiftCommandFreezesMoneyWindowAndIDWithoutDistanceOrCredentials() throws {
        let first = try command()
        let retry = try command()
        XCTAssertEqual(first.operationId, retry.operationId)
        XCTAssertEqual(first.canonicalPayload, retry.canonicalPayload)
        XCTAssertNotEqual(first.operationId, try command(net: 36_001).operationId)
        XCTAssertNotEqual(first.operationId, try command(epoch: 1).operationId)
        let bytes = try XCTUnwrap(first.canonicalPayload)
        let payload = try XCTUnwrap(JSONSerialization.jsonObject(with: bytes) as? [String: Any])
        let record = try XCTUnwrap(payload["record"] as? [String: Any])
        XCTAssertEqual(Set(payload.keys), ["operation_id", "expected_revision", "expected_epoch", "record"])
        XCTAssertEqual(Set(record.keys), ["id", "vehicle_id", "platform", "source_hash", "coverage", "currency",
                                        "from", "to", "timezone", "money"])
        XCTAssertEqual(record["coverage"] as? String, "SUMMARY")
        XCTAssertEqual(record["from"] as? String, "2026-11-24T10:00:00Z")
        XCTAssertEqual(record["to"] as? String, "2026-11-24T12:00:00Z")
        let money = try XCTUnwrap(record["money"] as? [String: Any])
        XCTAssertEqual(Set(money.keys), ["basis", "net"])
        XCTAssertEqual(money["net"] as? Int64, 36_000)
        let open = DriverShiftRecord(id: recordId, vehicleId: vehicleId, startedAtLabel: "2026-11-24T10:00:00Z",
                                     startOdometerKm: 0)
        XCTAssertNil(DriverAccountingCommand.earning(
            forCompletedShift: open, platform: .other, money: try XCTUnwrap(DriverEarningsMoney.inclusiveNet(10)), timezone: "UTC"
        ))
    }

    func testOutboxLeaseRestartRetryAfterConflictAndStaleResponses() throws {
        try directory { url in
            let outbox = DriverAccountingOutbox(directory: url)
            let owner = try scope()
            let other = try scope(account: outsider)
            let differentOrigin = try scope(origin: "https://other.example")
            let intent = try command()
            let queued = try outbox.enqueue(intent, in: owner, nowMillis: 1_000)
            XCTAssertEqual(try outbox.enqueue(intent, in: owner, nowMillis: 2_000), queued)
            XCTAssertThrowsError(try outbox.enqueue(command(net: 99), in: owner, nowMillis: 2_000))
            XCTAssertNil(try outbox.claim(in: other, nowMillis: 2_000))
            XCTAssertNil(try outbox.claim(in: differentOrigin, nowMillis: 2_000))
            let first = try XCTUnwrap(outbox.claim(in: owner, nowMillis: 2_000))
            let reopened = DriverAccountingOutbox(directory: url)
            XCTAssertNil(try reopened.claim(in: owner, nowMillis: 61_999))
            let second = try XCTUnwrap(reopened.claim(in: owner, nowMillis: 62_000))
            XCTAssertEqual(second.command.canonicalPayload, first.command.canonicalPayload)
            XCTAssertThrowsError(try outbox.settle(first, outcome: .interrupted, nowMillis: 62_000)) {
                XCTAssertEqual($0 as? DriverAccountingError, .staleClaim)
            }
            try reopened.settle(second, outcome: .retry(.rateLimited, retryAfterSeconds: 30), nowMillis: 62_100)
            XCTAssertNil(try reopened.claim(in: owner, nowMillis: 92_099))
            XCTAssertEqual(try reopened.nextEligibleMillis(in: owner), 92_100)
            let third = try XCTUnwrap(reopened.claim(in: owner, nowMillis: 92_100))
            try reopened.settle(third, outcome: .conflict(.conflict), nowMillis: 92_105)
            XCTAssertEqual(try reopened.entries(in: owner).first?.state, .conflict)
            XCTAssertNil(try reopened.claim(in: owner, nowMillis: 1_000_000))
            try reopened.erase(owner)
            XCTAssertThrowsError(try reopened.settle(third, outcome: .interrupted, nowMillis: 92_110))
            XCTAssertTrue(try reopened.entries(in: owner).isEmpty)
        }
    }

    func testRetryLimitAndExplicitWaitingResume() throws {
        try directory { url in
            let outbox = DriverAccountingOutbox(directory: url)
            let owner = try scope()
            _ = try outbox.enqueue(command(), in: owner, nowMillis: 0)
            let first = try XCTUnwrap(outbox.claim(in: owner, nowMillis: 0))
            try outbox.settle(first, outcome: .waiting(.capabilityRequired), nowMillis: 1)
            XCTAssertNil(try outbox.nextEligibleMillis(in: owner))
            XCTAssertNil(try outbox.claim(in: owner, nowMillis: 1_000))
            try outbox.resumeWaiting(in: owner, nowMillis: 5_000)
            var now: Int64 = 5_000
            for attempt in 1...DriverAccountingOutbox.maxAttempts {
                let claim = try XCTUnwrap(outbox.claim(in: owner, nowMillis: now))
                try outbox.settle(claim, outcome: .retry(.offline, retryAfterSeconds: nil), nowMillis: now)
                let row = try XCTUnwrap(outbox.entries(in: owner).first)
                XCTAssertEqual(row.attempts, attempt)
                if attempt < DriverAccountingOutbox.maxAttempts {
                    XCTAssertEqual(row.nextEligibleMillis, now + (5_000 << (attempt - 1)))
                    now = row.nextEligibleMillis
                } else {
                    XCTAssertEqual(row.state, .conflict)
                    XCTAssertEqual(row.issue, .retryLimit)
                }
            }
        }
    }

    func testHTTPRefusalCategoriesPreserveWaitRetryAndReconciliation() {
        let cases: [(Int, String?, Int64?, DriverOutboxOutcome)] = [
            (400, "driver.invalid", nil, .conflict(.invalid)),
            (401, "driver.auth-required", nil, .waiting(.signInRequired)),
            (403, "driver.capability-required", nil, .waiting(.capabilityRequired)),
            (404, "driver.disabled", nil, .waiting(.disabled)),
            (408, nil, nil, .retry(.timeout, retryAfterSeconds: nil)),
            (409, "driver.conflict", nil, .conflict(.conflict)),
            (429, nil, 32, .retry(.rateLimited, retryAfterSeconds: 32)),
            (503, "driver.unavailable", 5, .retry(.unavailable, retryAfterSeconds: 5)),
        ]
        for (status, code, retryAfter, outcome) in cases {
            let error = SignedTransportError.refused(status: status, code: code, retryAfterSeconds: retryAfter)
            XCTAssertEqual(DriverOutboxOutcome.forFailure(error), outcome)
        }
        XCTAssertEqual(DriverOutboxOutcome.forFailure(SignedTransportError.offline), .retry(.offline, retryAfterSeconds: nil))
        XCTAssertEqual(DriverOutboxOutcome.forFailure(CancellationError()), .interrupted)
    }

    func testSignedOwnerCommandUsesSameCanonicalPayloadAcrossRetriesAndRefusesForeignClaim() async throws {
        let url = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString, isDirectory: true)
        defer { try? FileManager.default.removeItem(at: url) }
        let store = DriverAccountingOutbox(directory: url)
        let owner = try scope()
        let command = try command()
        _ = try store.enqueue(command, in: owner, nowMillis: 0)
        let claim = try XCTUnwrap(store.claim(in: owner, nowMillis: 1))
        let session = PortableAuth.Session(
            familyId: "44444444-4444-4444-8444-444444444444", accountId: account,
            accessToken: "fixture-access", refreshToken: "fixture-refresh",
            accessExpiresAt: 2_000_000_000, absoluteExpiresAt: 2_100_000_000
        )
        let sessions = StaticSessions(session: session)
        let response = String(decoding: try fixture("receipt-200"), as: UTF8.self)
        let exchange = ScriptedExchange { request in
            FakeIdentityBackend.reply(request) { request in
                request.url?.path == DriverAccountingWire.commandsPath ? .init(200, response) : .init(404, "{}")
            }
        }
        let transport = SignedRequestTransport(
            origin: origin, signer: try IdentityVectors.signer(),
            identityStore: IosDeviceIdentityKeychainStore(useMemoryOnly: true), exchange: exchange,
            sessions: sessions, nowEpochSeconds: { 1_763_892_000 }
        )
        let client = CommunityWriteClient(transport: transport, exchange: exchange, sessions: sessions, photoBytes: { _ in nil })
        let active = try XCTUnwrap(AccountDataExportScope(
            origin: origin, ownerScope: XCTUnwrap(transport.ownerScope()), sessionFamilyId: session.familyId
        ))
        let first = try await client.submitDriverAccounting(claim, scope: active)
        let retry = try await client.submitDriverAccounting(claim, scope: active)
        XCTAssertEqual(first, retry)
        let posts = exchange.requests.filter { $0.url?.path == DriverAccountingWire.commandsPath }
        XCTAssertEqual(posts.count, 2)
        XCTAssertTrue(posts.allSatisfy { $0.value(forHTTPHeaderField: "Signature") != nil })
        XCTAssertEqual(posts.first?.httpBody, posts.last?.httpBody)
        let body = FakeIdentityBackend.json(try XCTUnwrap(posts.first))
        XCTAssertEqual(Set(body.keys), ["family_id", "access_token", "kind", "payload"])
        XCTAssertEqual(body["kind"] as? String, "EARNING_SAVE")
        try store.settle(claim, outcome: .synced(first), nowMillis: 2)
        XCTAssertEqual(try store.entries(in: owner).first?.state, .synced)
        XCTAssertEqual(try store.knownEpoch(in: owner), 0)

        let foreign = try scope(account: outsider)
        _ = try store.enqueue(command, in: foreign, nowMillis: 0)
        let foreignClaim = try XCTUnwrap(store.claim(in: foreign, nowMillis: 1))
        let count = exchange.requests.count
        do {
            _ = try await client.submitDriverAccounting(foreignClaim, scope: active)
            XCTFail("A foreign accounting claim cannot be sent by the current owner")
        } catch {
            XCTAssertEqual(error as? DriverAccountingError, .scopeChanged)
        }
        XCTAssertEqual(exchange.requests.count, count)
    }

    private func scope(account: String? = nil, origin: String? = nil) throws -> DriverOutboxScope {
        try XCTUnwrap(DriverOutboxScope(accountId: account ?? self.account, origin: origin ?? self.origin))
    }

    func testVehicleEnrollmentResumesOnlyThatVehiclesRefusedEntries() throws {
        try directory { url in
            let outbox = DriverAccountingOutbox(directory: url)
            let owner = try scope()
            let otherVehicle = "33333333-3333-4333-8333-333333333333"
            let refused = try command()
            let foreign = try command(record: "44444444-4444-4444-8444-444444444444", vehicle: otherVehicle)
            let signedOut = try command(net: 10, record: "55555555-5555-4555-8555-555555555555")
            for intent in [refused, foreign, signedOut] { _ = try outbox.enqueue(intent, in: owner, nowMillis: 0) }
            let outcomes: [String: DriverOutboxOutcome] = [
                refused.id: .waiting(.capabilityRequired), foreign.id: .waiting(.capabilityRequired),
                signedOut.id: .waiting(.signInRequired),
            ]
            for _ in 0..<3 {
                let claim = try XCTUnwrap(outbox.claim(in: owner, nowMillis: 1))
                try outbox.settle(claim, outcome: try XCTUnwrap(outcomes[claim.command.id]), nowMillis: 2)
            }
            XCTAssertNil(try outbox.claim(in: owner, nowMillis: 10))
            XCTAssertThrowsError(try outbox.resumeAfterVehicleEnrollment(vehicleId: "vehicle", in: owner, nowMillis: 10))
            XCTAssertEqual(try outbox.resumeAfterVehicleEnrollment(vehicleId: vehicleId, in: try scope(account: outsider), nowMillis: 10), 0)

            XCTAssertEqual(try outbox.resumeAfterVehicleEnrollment(vehicleId: vehicleId, in: owner, nowMillis: 10), 1)
            let retry = try XCTUnwrap(outbox.claim(in: owner, nowMillis: 10))
            XCTAssertEqual(retry.command.id, refused.id)
            XCTAssertEqual(retry.command.canonicalPayload, refused.canonicalPayload)
            XCTAssertNil(try outbox.claim(in: owner, nowMillis: 10))
            XCTAssertEqual(try outbox.resumeAfterVehicleEnrollment(vehicleId: vehicleId, in: owner, nowMillis: 11), 0)
        }
    }

    private func command(
        net: Int64 = 36_000, epoch: Int64 = 0, record: String? = nil, vehicle: String? = nil
    ) throws -> DriverAccountingCommand {
        let shift = DriverShiftRecord(
            id: record ?? recordId, vehicleId: vehicle ?? vehicleId,
            startedAtLabel: "2026-11-24T10:00:00Z", endedAtLabel: "2026-11-24T12:00:00Z",
            startOdometerKm: 1_000, endOdometerKm: 1_080, grossEarningsCentavos: 44_000,
            fuelCostCentavos: 8_000, activeMinutes: 100, isActive: false
        )
        return try XCTUnwrap(DriverAccountingCommand.earning(
            forCompletedShift: shift, platform: .other, money: XCTUnwrap(DriverEarningsMoney.inclusiveNet(net)),
            timezone: "America/Cuiaba", epoch: epoch
        ))
    }

    private func directory(_ use: (URL) throws -> Void) throws {
        let path = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString, isDirectory: true)
        defer { try? FileManager.default.removeItem(at: path) }
        try use(path)
    }

    private func pairs(_ cells: Substring) -> [String: String] {
        var fields: [String: String] = [:]
        for cell in cells.split(separator: ";") {
            let pair = cell.split(separator: "=", maxSplits: 1, omittingEmptySubsequences: false)
            if pair.count == 2 { fields[String(pair[0])] = String(pair[1]) }
        }
        return fields
    }

    private func fixture(_ name: String, fileExtension: String = "json", directory: String = "driver-suite/http") throws -> Data {
        #if SWIFT_PACKAGE
        let repo = (0..<4).reduce(URL(fileURLWithPath: #filePath)) { url, _ in url.deletingLastPathComponent() }
        let url = repo.appendingPathComponent("contracts/testdata/\(directory)/\(name).\(fileExtension)")
        #else
        let url = try XCTUnwrap(Bundle(for: Self.self).url(forResource: name, withExtension: fileExtension))
        #endif
        return try Data(contentsOf: url)
    }
}

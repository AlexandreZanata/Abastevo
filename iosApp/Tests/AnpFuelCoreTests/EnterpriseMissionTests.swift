import Foundation
import XCTest
@testable import AnpFuelCore

final class EnterpriseMissionTests: XCTestCase {
    private let prefix = "7b0e2a54-0000-4000-8000-"
    private let scopeOrigin = "https://api.example.invalid"
    private let clock = EnterpriseMissionClock(processID: UUID(), elapsedMillis: 100, wallMillis: 1000)

    private func id(_ suffix: String) -> String { prefix + "00000000" + suffix }

    private var employeeId: String { id("9631") }
    private var missionId: String { id("9701") }
    private var assetId: String { id("9621") }

    private func partition() throws -> EnterprisePartition {
        try XCTUnwrap(EnterprisePartition(accountId: id("9501"), origin: scopeOrigin, organizationId: id("9502")))
    }

    private func envelope(
        operation: String, at: String = "2026-11-15T12:00:00.123Z", zone: String = "America/Cuiaba", offset: Int64 = -14_400
    ) throws -> EnterpriseEnvelope {
        EnterpriseEnvelope(
            partition: try partition(), membershipId: id("9503"), enrollmentId: id("9504"), operationId: operation,
            declared: EnterpriseTimeDeclaration(occurredAt: at, entryZone: zone, deviceOffsetSeconds: offset)
        )
    }

    private func fixture() throws -> String {
        #if SWIFT_PACKAGE
        let repo = (0..<4).reduce(URL(fileURLWithPath: #filePath)) { url, _ in url.deletingLastPathComponent() }
        let url = repo.appendingPathComponent("contracts/testdata/enterprise/trip/trip-operations-v3.psv")
        #else
        let url = try XCTUnwrap(Bundle(for: Self.self).url(forResource: "trip-operations-v3", withExtension: "psv"))
        #endif
        return try String(contentsOf: url, encoding: .utf8)
    }

    private func bytes(_ hex: String) throws -> Data {
        let input = Array(hex.utf8)
        XCTAssertEqual(input.count % 2, 0)
        var output = Data()
        for index in stride(from: 0, to: input.count, by: 2) where index + 1 < input.count {
            let pair = String(decoding: input[index...index + 1], as: UTF8.self)
            output.append(try XCTUnwrap(UInt8(pair, radix: 16)))
        }
        return output
    }

    func testSignedAnswersAndMissionTripReplayUnchangedGoKotlinVectors() throws {
        let fixture = try self.fixture()
        var seen = Set<String>()
        for raw in fixture.split(separator: "\n").dropFirst() {
            let fields = raw.split(separator: "|", omittingEmptySubsequences: false).map(String.init)
            guard fields.count == 5, ["RESPOND", "START"].contains(fields[1]) else { continue }
            let name = fields[0]
            let fieldsByKey = Dictionary(uniqueKeysWithValues: fields[2].split(separator: ";").compactMap { pair -> (String, String)? in
                let kv = pair.split(separator: "=", maxSplits: 1, omittingEmptySubsequences: false)
                guard kv.count == 2 else { return nil }
                return (String(kv[0]), String(kv[1]))
            })
            if fields[1] == "RESPOND" {
                let operation = try XCTUnwrap(fieldsByKey["op"])
                let mission = try XCTUnwrap(fieldsByKey["mission"])
                let answer = fieldsByKey["response"].flatMap(EnterpriseMissionResponse.init(rawValue:))
                if fields[3] == "OK" {
                    let reason = try fieldsByKey["reason"].map { String(decoding: try bytes($0), as: UTF8.self) }
                    let result = try EnterpriseOperationBuilder.respondToMission(
                        envelope: envelope(operation: operation, at: fieldsByKey["at"] ?? "2026-11-15T12:00:00.123Z",
                                           zone: fieldsByKey["zone"] ?? "America/Cuiaba", offset: fieldsByKey["offset"].flatMap(Int64.init) ?? -14_400),
                        missionId: mission, revision: XCTUnwrap(fieldsByKey["revision"].flatMap(Int64.init)),
                        response: XCTUnwrap(answer),
                        reason: reason
                    )
                    XCTAssertEqual(result.canonical, try bytes(fields[4]), name)
                    seen.insert(name)
                } else if let answer, let revision = fieldsByKey["revision"].flatMap(Int64.init) {
                    let reason = try fieldsByKey["reason"].map { String(decoding: try bytes($0), as: UTF8.self) }
                    XCTAssertThrowsError(try EnterpriseOperationBuilder.respondToMission(
                        envelope: envelope(operation: operation), missionId: mission,
                        revision: revision, response: answer, reason: reason
                    ), name)
                }
            } else if name == "start-absent-reading-with-mission-reference" {
                let reason = String(decoding: try bytes(XCTUnwrap(fieldsByKey["reason"])), as: UTF8.self)
                let result = try EnterpriseOperationBuilder.startTrip(
                    envelope: envelope(operation: XCTUnwrap(fieldsByKey["op"])),
                    tripId: XCTUnwrap(fieldsByKey["trip"]), assetId: XCTUnwrap(fieldsByKey["asset"]),
                    driverEmployeeId: XCTUnwrap(fieldsByKey["driver"]), evidence: .missing(reason: reason),
                    departureRunId: nil, missionId: XCTUnwrap(fieldsByKey["mission"])
                )
                XCTAssertEqual(result.canonical, try bytes(fields[4]), name)
                seen.insert(name)
            }
        }
        XCTAssertEqual(seen, [
            "respond-accept", "respond-decline-with-reason", "respond-decline-reason-at-280-code-points",
            "respond-accept-utc-zone", "start-absent-reading-with-mission-reference",
        ])
    }

    func testWholeReadRefusesForeignMemberAndMalformedPlan() throws {
        let page = try EnterpriseMissionWire.page(body(items: [row()], cursor: "opaque.signature"), employeeId: employeeId)
        XCTAssertTrue(page.truncated)
        XCTAssertEqual(page.items.first?.assetId, assetId)
        XCTAssertEqual(page.items.first?.title, "Synthetic trip")
        XCTAssertThrowsError(try EnterpriseMissionWire.page(body(items: [row(), row()]), employeeId: employeeId))
        XCTAssertThrowsError(try EnterpriseMissionWire.page(body(items: [row(employee: id("9784"))]), employeeId: employeeId)) {
            XCTAssertEqual($0 as? EnterpriseMissionError, .foreignBinding)
        }
        var invalidStop = row()
        invalidStop["plan"] = [
            "revision": 2, "planned_at": "2026-11-19T12:00:00Z", "reason": "Synthetic delivery",
            "destination": ["kind": "TEXT", "text": "Depot"], "load": ["kind": "NONE"],
            "stops": [["sequence": 2, "kind": "TEXT", "text": "Gate"] as [String: Any]],
        ] as [String: Any]
        XCTAssertThrowsError(try EnterpriseMissionWire.page(body(items: [invalidStop]), employeeId: employeeId))
        invalidStop["plan"] = [
            "revision": 2, "planned_at": "2026-11-19T12:00:00Z", "reason": "Synthetic delivery",
            "destination": ["kind": "TEXT", "text": "Depot"], "load": ["kind": "PASSENGERS", "count": 4] as [String: Any],
            "stops": [["sequence": 1, "kind": "TEXT", "text": "Gate"] as [String: Any]],
        ] as [String: Any]
        let planned = try EnterpriseMissionWire.page(body(items: [invalidStop]), employeeId: employeeId)
        XCTAssertEqual(planned.items.first?.plan?.passengers, 4)
    }

    func testReceiptBeforeStartOwnAssetAndSingleDeliverySurviveProtectedRestart() throws {
        let directory = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
        defer { try? FileManager.default.removeItem(at: directory) }
        let store = EnterprisePartitionStore(directory: directory)
        let partition = try self.partition()
        let page = try EnterpriseMissionWire.page(body(items: [row()]), employeeId: employeeId)
        var inbox = try EnterpriseMissionInbox(membershipId: id("9503"), enrollmentId: id("9504"), employeeId: employeeId, page: page, received: clock)
        let answer = try EnterpriseOperationBuilder.respondToMission(
            envelope: envelope(operation: id("9606")), missionId: missionId, revision: 2, response: .accept, reason: nil
        )
        try inbox.addAnswer(missionId: missionId, operationId: answer.operationId, revision: 2, response: .accept)
        XCTAssertThrowsError(try inbox.addAnswer(missionId: missionId, operationId: id("9608"), revision: 2, response: .decline))

        let entry = EnterpriseQueueItem(operation: answer, kind: .missionResponse, summary: "Fixture", createdAtMillis: 200)
        let otherOperation = try EnterpriseOperationBuilder.startTrip(
            envelope: envelope(operation: id("9601")), tripId: id("9611"), assetId: assetId,
            driverEmployeeId: employeeId, evidence: .missing(reason: "Fixture"), departureRunId: nil
        )
        let other = EnterpriseQueueItem(operation: otherOperation, kind: .tripStart, summary: "Fixture", createdAtMillis: 100)
        var snapshot = EnterprisePartitionSnapshot(partition: partition, membershipId: id("9503"))
        try EnterpriseQueue.enqueue(other, into: &snapshot)
        try EnterpriseQueue.enqueue(entry, into: &snapshot)
        snapshot.missionInbox = inbox
        XCTAssertTrue(store.save(snapshot))
        let restarted = EnterprisePartitionStore(directory: directory)
        var loaded = try XCTUnwrap(restarted.load(partition))
        XCTAssertEqual(EnterpriseQueue.dueBatch(loaded, nowMillis: 200).map(\.id), [other.id])
        loaded.queue[0].status = .accepted
        XCTAssertEqual(EnterpriseQueue.dueBatch(loaded, nowMillis: 200).map(\.id), [answer.operationId])
        XCTAssertEqual(loaded.missionInbox?.rows(queue: loaded.queue, trips: []).first?.response, .saved)
        XCTAssertThrowsError(try inbox.readyTrip(missionId: missionId, assetId: assetId, queue: loaded.queue, trips: []))

        loaded.queue[1].status = .accepted
        loaded.queue[1].entityRevision = 3
        XCTAssertNoThrow(try inbox.readyTrip(missionId: missionId, assetId: assetId, queue: loaded.queue, trips: []))
        XCTAssertThrowsError(try inbox.readyTrip(missionId: missionId, assetId: id("9788"), queue: loaded.queue, trips: []))
        let started = EnterpriseTripRecord(
            id: id("9611"), assetId: assetId, assetLabel: "Fixture", startOperationId: other.id, startedAtMillis: 200,
            startMeterId: nil, missionId: missionId
        )
        XCTAssertThrowsError(try inbox.readyTrip(missionId: missionId, assetId: assetId, queue: loaded.queue, trips: [started]))
        EnterpriseQueue.dismiss(id: answer.operationId, in: &loaded)
        XCTAssertTrue(loaded.queue.contains(where: { $0.id == answer.operationId }), "The old list still needs this receipt")
        let reviewed = try EnterpriseMissionWire.page(
            body(items: [row(state: "ACCEPTED", revision: 3, answer: "ACCEPT")]), employeeId: employeeId
        )
        try loaded.missionInbox?.refresh(reviewed, received: clock)
        EnterpriseQueue.dismiss(id: answer.operationId, in: &loaded)
        XCTAssertFalse(loaded.queue.contains(where: { $0.id == answer.operationId }))
        let foreign = try XCTUnwrap(EnterprisePartition(accountId: id("9888"), origin: scopeOrigin, organizationId: partition.organizationId))
        XCTAssertNil(restarted.load(foreign))
        restarted.erase(partition)
        XCTAssertNil(store.load(partition))
    }

    func testOwnMissionListUsesTheProtectedCompanyChannelAndRefusesAnotherAccount() async throws {
        let partition = try self.partition()
        let account = partition.accountId
        let origin = partition.origin
        let employee = employeeId
        let path = "/v1/orgs/\(partition.organizationId)/missions/mine/list"
        let secret = String(repeating: "d", count: 64)
        let csrf = "abv_company_csrf_" + String(repeating: "e", count: 64)
        let cookie = "__Host-empresas-session=abv_company_\(secret)"
        let content = cookie + "; Path=/; Expires=Thu, 19 Nov 2026 20:00:00 GMT; HttpOnly; Secure; SameSite=Strict"
        let body = try JSONSerialization.data(withJSONObject: self.body(items: [row()]))
        let fixture = String(decoding: body, as: UTF8.self)
        let session = PortableAuth.Session(
            familyId: id("9511"), accountId: account, accessToken: "fixture-access-token", refreshToken: "fixture-refresh-token",
            accessExpiresAt: 2_000_000_000, absoluteExpiresAt: 2_100_000_000
        )
        let sessions = StaticSessions(session: session)
        let exchange = ScriptedExchange { request in
            switch request.url?.path {
            case EnterpriseCompanyWire.loginPath:
                return .init(200, #"""
                    {"account_id":"\#(account)","csrf_token":"\#(csrf)","authenticated_at":"2026-11-15T12:00:00Z","expires_at":"2026-11-15T20:00:00Z"}
                    """#, headers: ["Set-Cookie": content, "Content-Type": "application/json"])
            case path:
                guard request.value(forHTTPHeaderField: "X-CSRF") == csrf,
                      request.value(forHTTPHeaderField: "Origin") == origin,
                      request.value(forHTTPHeaderField: "Cookie") == cookie,
                      FakeIdentityBackend.json(request)["limit"] as? Int == 100 else { return .init(403, "{}") }
                return .init(200, fixture)
            default:
                return .init(404, "{}")
            }
        }
        let client = EnterpriseCompanyClient(origin: origin, exchange: exchange, sessions: sessions)
        let page = try await client.missions(partition: partition, employeeId: employee)
        XCTAssertEqual(page.items.map(\.id), [missionId])
        let read = try XCTUnwrap(exchange.requests.last)
        XCTAssertEqual(read.url?.path, path)
        XCTAssertEqual(read.httpMethod, "POST")
        XCTAssertNil(read.value(forHTTPHeaderField: "Signature"))
        XCTAssertNil(read.value(forHTTPHeaderField: "Authorization"))
        sessions.set(nil)
        let before = exchange.requests.count
        do {
            _ = try await client.missions(partition: partition, employeeId: employee)
            XCTFail("No account session may read another member's missions")
        } catch {
            XCTAssertEqual(error as? EnterpriseAccessError, .accessDenied)
        }
        XCTAssertEqual(exchange.requests.count, before)
    }

    func testReceiptClockUsesMonotonicTimeAndTreatsRollbackAsUnknown() {
        let later = EnterpriseMissionClock(processID: clock.processID, elapsedMillis: 125, wallMillis: 900)
        XCTAssertEqual(later.age(of: clock), 25)
        let restarted = EnterpriseMissionClock(processID: UUID(), elapsedMillis: 20, wallMillis: 900)
        XCTAssertNil(restarted.age(of: clock))
    }

    private func body(items: [[String: Any]], cursor: String = "") -> [String: Any] {
        ["items": items, "next_cursor": cursor, "data_mode": "SYNTHETIC"]
    }

    private func row(employee: String? = nil, state: String = "ASSIGNED", revision: Int64 = 2, answer: String? = nil) -> [String: Any] {
        var raw: [String: Any] = [
            "mission_id": missionId, "unit_id": id("9509"), "asset_id": assetId, "title": "Synthetic trip",
            "starts_at": "2026-11-20T11:00:00Z", "ends_at": "2026-11-20T15:00:00Z",
            "state": state, "revision": revision, "assignee_employee_id": employee ?? employeeId,
            "created_at": "2026-11-19T12:00:00Z", "updated_at": "2026-11-19T12:00:00Z",
        ]
        if let answer { raw["answer"] = ["response": answer] }
        return raw
    }
}

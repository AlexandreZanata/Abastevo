import CryptoKit
import Foundation
import XCTest
@testable import AnpFuelCore

final class QualityReportsTests: XCTestCase {
    private static let origin = "https://abastevo.com.br"
    private static let commandId = "22222222-2222-4222-8222-222222222222"
    private static let reportId = "33333333-3333-4333-8333-333333333333"
    private static let stationId = "44444444-4444-4444-8444-444444444444"
    private static let family = "11111111-1111-4111-8111-111111111111"
    private static let alternateFamily = "11111111-1111-4111-8111-111111111112"
    private static let account = "a0000000-0000-4000-8000-000000000001"
    private static let privateText = "Synthetic private quality experience"
    private let now = Date(timeIntervalSince1970: 1_800_000_000)

    private static func session(family: String = QualityReportsTests.family) -> PortableAuth.Session {
        PortableAuth.Session(familyId: family, accountId: account, accessToken: "fixture-access",
                             refreshToken: "fixture-refresh", accessExpiresAt: 2_000_000_000,
                             absoluteExpiresAt: 2_100_000_000)
    }

    private func command(action: QualityReportAction = .submit, text: String = QualityReportsTests.privateText) -> QualityReportCommand {
        switch action {
        case .submit, .edit:
            return QualityReportCommand(
                commandId: Self.commandId, reportId: Self.reportId, action: action, expectedRevision: action == .submit ? 0 : 2,
                kind: .experience, stationId: Self.stationId, fuel: FuelProduct.ethanol.wireCode, occurredOn: "2026-10-13",
                symptom: .starting, text: text
            )
        case .withdraw, .appeal:
            return QualityReportCommand(commandId: Self.commandId, reportId: Self.reportId, action: action,
                                        expectedRevision: 2, text: action == .appeal ? text : nil)
        }
    }

    private func fixture(_ name: String) throws -> [String: Any] {
        #if SWIFT_PACKAGE
        var source = URL(fileURLWithPath: #filePath)
        for _ in 0..<4 { source.deleteLastPathComponent() }
        let url = source.appendingPathComponent("contracts/testdata/api").appendingPathComponent(name).appendingPathExtension("json")
        #else
        let url = try XCTUnwrap(Bundle(for: Self.self).url(forResource: name, withExtension: "json"))
        #endif
        return try XCTUnwrap(JSONSerialization.jsonObject(with: Data(contentsOf: url)) as? [String: Any])
    }

    private func signedClient(
        _ sessions: StaticSessions = StaticSessions(session: QualityReportsTests.session()),
        respond: @escaping @Sendable (URLRequest) -> ScriptedExchange.Reply
    ) throws -> (CommunityWriteClient, ScriptedExchange, QualityReportScope) {
        let exchange = ScriptedExchange { request in FakeIdentityBackend.reply(request, fallback: respond) }
        let transport = SignedRequestTransport(
            origin: Self.origin, signer: try IdentityVectors.signer(),
            identityStore: IosDeviceIdentityKeychainStore(useMemoryOnly: true), exchange: exchange, sessions: sessions,
            nowEpochSeconds: { 1_763_892_000 }
        )
        let client = CommunityWriteClient(transport: transport, exchange: exchange, sessions: sessions, photoBytes: { _ in nil })
        let owner = try XCTUnwrap(transport.ownerScope())
        let scope = try XCTUnwrap(QualityReportScope(origin: Self.origin, ownerScope: owner, familyId: Self.family))
        return (client, exchange, scope)
    }

    func testSharedCommandsAcceptOnlyContractFieldsAndMutations() throws {
        let paths = [
            "quality-report-valid", "quality-report-appeal-valid", "quality-report-appeal-invalid-revision",
            "quality-report-appeal-invalid-target", "quality-report-invalid-checked", "quality-report-invalid-document",
            "quality-report-invalid-edit-revision", "quality-report-invalid-empty", "quality-report-invalid-owner",
            "quality-report-invalid-withdraw-payload",
        ]
        for name in paths {
            let doc = try fixture(name)
            var value = try XCTUnwrap(doc["value"] as? [String: Any])
            XCTAssertNotNil(value.removeValue(forKey: "family_id"))
            XCTAssertNotNil(value.removeValue(forKey: "access_token"))
            let data = try JSONSerialization.data(withJSONObject: value)
            if doc["valid"] as? Bool == true {
                let decoded = try QualityReportWire.decodeCommand(data, now: now)
                XCTAssertEqual(decoded.commandId, Self.commandId, name)
                XCTAssertEqual(decoded.reportId, Self.reportId, name)
                let serialized = try QualityReportWire.fields(decoded)
                for (key, field) in value {
                    XCTAssertEqual(serialized[key] as? NSObject, field as? NSObject, key)
                }
            } else {
                XCTAssertThrowsError(try QualityReportWire.decodeCommand(data, now: now), name)
            }
        }
        XCTAssertNoThrow(try command(text: String(repeating: "🚙", count: 1_000)).validate(now: now))
        XCTAssertThrowsError(try command(text: String(repeating: "🚙", count: 1_001)).validate(now: now))
        XCTAssertThrowsError(try command(text: "invalid\rbody").validate(now: now))
        XCTAssertThrowsError(try QualityReportCommand(
            reportId: Self.reportId, action: .submit, expectedRevision: 0, kind: .linkReview, stationId: Self.stationId,
            fuel: FuelProduct.ethanol.wireCode, occurredOn: "2026-10-13", symptom: .other, text: "Fixture"
        ).validate(now: now))
    }

    func testSignedCommandsReuseExactEnvelopeAndOwnerGetAndListStayPrivate() async throws {
        let (client, exchange, scope) = try signedClient { request in
            switch request.url?.lastPathComponent {
            case "commands":
                let body = FakeIdentityBackend.json(request)
                let revision = (body["expected_revision"] as? Int ?? 0) + 1
                let status = body["action"] as? String == "WITHDRAW" ? "WITHDRAWN" : "PENDING_REVIEW"
                return .init(200, "{\"report_id\":\"\(Self.reportId)\",\"revision\":\(revision),\"status\":\"\(status)\"}")
            case "get":
                return .init(200, #"{"id":"\#(Self.reportId)","revision":1,"kind":"EXPERIENCE","station_id":"\#(Self.stationId)","fuel":"ETHANOL","occurred_on":"2026-10-13","symptom":"STARTING","text":"\#(Self.privateText)","status":"REVIEWED","evidence_level":"USER_CLAIM","review_reason":"CLAIM_RECORDED"}"#)
            case "list": return .init(200, #"{"items":[]}"#)
            default: return .init(500, "{}")
            }
        }
        for action in [QualityReportAction.submit, .edit, .withdraw, .appeal] {
            let command = command(action: action)
            let first = try await client.executeQualityReport(command, scope: scope)
            let retry = try await client.executeQualityReport(command, scope: scope)
            XCTAssertEqual(first, retry)
            XCTAssertEqual(first.reportId, Self.reportId)
        }
        let row = try await client.qualityReport(id: Self.reportId, scope: scope)
        XCTAssertEqual(row.text, Self.privateText)
        XCTAssertEqual(row.evidenceLevel, "USER_CLAIM")
        let page = try await client.qualityReports(after: Self.reportId, scope: scope)
        XCTAssertTrue(page.items.isEmpty)
        let requests = exchange.requests.filter { $0.url?.path.hasPrefix("/v1/quality/reports/") == true }
        XCTAssertEqual(requests.count, 10)
        for (index, request) in requests.enumerated() {
            XCTAssertEqual(request.httpMethod, "POST")
            XCTAssertNil(request.url?.query)
            XCTAssertNotNil(request.value(forHTTPHeaderField: "Signature"))
            let fields = FakeIdentityBackend.json(request)
            XCTAssertEqual(fields["family_id"] as? String, Self.family)
            XCTAssertEqual(fields["access_token"] as? String, "fixture-access")
            for disallowed in ["account_id", "owner", "receipt_url", "latitude", "longitude", "refresh_token"] {
                XCTAssertNil(fields[disallowed])
            }
            if index < 8 {
                XCTAssertEqual(request.value(forHTTPHeaderField: "Idempotency-Key"), Self.commandId)
                if index.isMultiple(of: 2) { XCTAssertEqual(request.httpBody, requests[index + 1].httpBody) }
            }
        }
        XCTAssertEqual(FakeIdentityBackend.json(try XCTUnwrap(requests.last))["after_id"] as? String, Self.reportId)
    }

    func testRefusalPreservesServerOutcomeAndRetryAfter() async throws {
        let cases: [(Int, QualityReportError)] = [
            (400, .invalid), (401, .authRequired), (403, .authRequired), (404, .notFound), (409, .conflict),
            (429, .quota(retryAfterSeconds: 60)), (503, .unavailable(retryAfterSeconds: 60)),
        ]
        for (status, refusal) in cases {
            let (client, exchange, scope) = try signedClient { _ in
                .init(status, #"{"error":{"code":"quality.report-fixture-refusal"}}"#, headers: ["Retry-After": "60"])
            }
            do {
                _ = try await client.executeQualityReport(command(), scope: scope)
                XCTFail("Expected refusal \(status)")
            } catch {
                XCTAssertEqual(error as? QualityReportError, refusal)
            }
            XCTAssertEqual(exchange.requests.filter { $0.url?.path == "/v1/quality/reports/commands" }.count, 1)
        }
    }

    func testSameAccountNewFamilyDiscardsResponseAndCannotReuseOldScope() async throws {
        let sessions = StaticSessions(session: Self.session())
        let (client, exchange, scope) = try signedClient(sessions) { _ in
            sessions.set(Self.session(family: Self.alternateFamily))
            return .init(200, #"{"items":[]}"#)
        }
        for _ in 0..<2 {
            do {
                _ = try await client.qualityReports(scope: scope)
                XCTFail("Expected an old-session refusal")
            } catch {
                let refusal = try XCTUnwrap(error as? QualityReportError)
                XCTAssertTrue([.authRequired, .ownerChanged].contains(refusal))
            }
        }
        XCTAssertEqual(exchange.requests.filter { $0.url?.path == "/v1/quality/reports/list" }.count, 1)
    }

    func testJournalSealsCommandAndReopensWithSameLeaseAndBackoff() throws {
        let directory = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString, isDirectory: true)
        defer { try? FileManager.default.removeItem(at: directory) }
        let key = SymmetricKey(size: .bits256)
        let journal = QualityReportJournal(directory: directory, testKey: key)
        let scope = try XCTUnwrap(QualityReportScope(origin: Self.origin, ownerScope: "account:fixture:fp", familyId: Self.family))
        let otherFamily = try XCTUnwrap(QualityReportScope(origin: Self.origin, ownerScope: "account:fixture:fp", familyId: Self.alternateFamily))
        let otherOrigin = try XCTUnwrap(QualityReportScope(origin: "https://example.invalid", ownerScope: "account:fixture:fp", familyId: Self.family))
        let frozen = command()
        try journal.freeze(frozen, scope: scope, now: now)
        try journal.freeze(frozen, scope: scope, now: now)
        XCTAssertThrowsError(try journal.freeze(command(text: "Other request"), scope: scope, now: now))
        let ciphertext = try Data(contentsOf: directory.appendingPathComponent("pending-quality-reports").appendingPathComponent(scope.key))
        for secret in [Self.privateText, Self.family, Self.reportId, "fixture:fp"] {
            XCTAssertNil(ciphertext.range(of: Data(secret.utf8)))
        }
        let reopened = QualityReportJournal(directory: directory, testKey: key)
        XCTAssertEqual(try reopened.pending(scope: scope, now: now), frozen)
        XCTAssertNil(try reopened.pending(scope: otherFamily, now: now))
        XCTAssertNil(try reopened.pending(scope: otherOrigin, now: now))
        let initial = try XCTUnwrap(journal.claim(scope: scope, now: now))
        XCTAssertNil(try reopened.claim(scope: scope, now: now.addingTimeInterval(119)))
        let expiredLease = now.addingTimeInterval(121)
        let retried = try XCTUnwrap(reopened.claim(scope: scope, now: expiredLease))
        XCTAssertEqual(retried.command, initial.command)
        XCTAssertThrowsError(try journal.finish(initial, scope: scope, refusal: nil, now: expiredLease))
        try reopened.finish(retried, scope: scope, refusal: .quota(retryAfterSeconds: 90), now: expiredLease)
        XCTAssertNil(try reopened.claim(scope: scope, now: expiredLease.addingTimeInterval(89)))
        let third = try XCTUnwrap(reopened.claim(scope: scope, now: expiredLease.addingTimeInterval(91)))
        try reopened.finish(third, scope: scope, refusal: .conflict, now: expiredLease.addingTimeInterval(91))
        XCTAssertNil(try reopened.pending(scope: scope, now: expiredLease.addingTimeInterval(91)))
        try reopened.freeze(frozen, scope: scope, now: now)
        XCTAssertNil(try reopened.pending(scope: scope, now: now.addingTimeInterval(8 * 24 * 60 * 60)))
        try reopened.freeze(frozen, scope: scope, now: now)
        let beforePurge = try XCTUnwrap(reopened.claim(scope: scope, now: now))
        reopened.purge()
        try reopened.freeze(frozen, scope: scope, now: now)
        XCTAssertThrowsError(try reopened.finish(beforePurge, scope: scope, refusal: nil, now: now))
        XCTAssertEqual(try reopened.pending(scope: scope, now: now), frozen)
    }
}

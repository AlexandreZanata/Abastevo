import Foundation
import XCTest
@testable import AnpFuelCore

final class AdvancedAlertsTests: XCTestCase {
    private static let origin = "https://abastevo.com.br"
    private static let family = "11111111-1111-4111-8111-111111111111"
    private static let newFamily = "11111111-1111-4111-8111-111111111112"
    private static let account = "33333333-3333-4333-8333-333333333333"
    private static let operation = "66666666-6666-4666-8666-666666666666"
    private static let ruleID = "77777777-7777-4777-8777-777777777777"
    private static let stationID = "88888888-8888-4888-8888-888888888888"
    private static let firstInbox = "77777777-7777-4777-8777-777777777778"
    private static let secondInbox = "77777777-7777-4777-8777-777777777779"
    private let now = Date(timeIntervalSince1970: 1_800_000_000)

    private static func session(family: String = AdvancedAlertsTests.family) -> PortableAuth.Session {
        PortableAuth.Session(familyId: family, accountId: account, accessToken: "fixture-access",
                             refreshToken: "fixture-refresh", accessExpiresAt: 2_000_000_000,
                             absoluteExpiresAt: 2_100_000_000)
    }

    private func fixture(_ name: String) throws -> [String: Any] {
        #if SWIFT_PACKAGE
        var root = URL(fileURLWithPath: #filePath)
        for _ in 0..<4 { root.deleteLastPathComponent() }
        let url = root.appendingPathComponent("contracts/testdata/api").appendingPathComponent(name).appendingPathExtension("json")
        #else
        let url = try XCTUnwrap(Bundle(for: Self.self).url(forResource: name, withExtension: "json"))
        #endif
        return try XCTUnwrap(JSONSerialization.jsonObject(with: Data(contentsOf: url)) as? [String: Any])
    }

    private func rule(
        predicates: [AdvancedAlertPredicate]? = nil,
        target: AdvancedAlertTarget = AdvancedAlertTarget(stationIds: [AdvancedAlertsTests.stationID]),
        schedule: AdvancedAlertSchedule? = nil
    ) -> AdvancedAlertRule {
        AdvancedAlertRule(
            predicates: predicates ?? [AdvancedAlertPredicate(kind: .threshold, fuel: .ethanol, condition: .standard,
                                                               source: .community, thresholdMilli: 6_200)],
            op: .all, target: target,
            schedule: schedule ?? AdvancedAlertSchedule(days: [1], windowStart: "08:00", windowEnd: "20:00",
                                                        timezone: "America/Sao_Paulo", quietStart: "22:00",
                                                        quietEnd: "07:00", cooldownHours: 1)
        )
    }

    private func command() -> AdvancedAlertCommand {
        AdvancedAlertCommand(operationId: Self.operation, ruleId: Self.ruleID, expectedRevision: 0, payload: rule())
    }

    private func signedClient(
        _ sessions: StaticSessions = StaticSessions(session: AdvancedAlertsTests.session()),
        respond: @escaping @Sendable (URLRequest) -> ScriptedExchange.Reply
    ) throws -> (CommunityWriteClient, ScriptedExchange, AccountDataExportScope) {
        let exchange = ScriptedExchange { FakeIdentityBackend.reply($0, fallback: respond) }
        let transport = SignedRequestTransport(
            origin: Self.origin, signer: try IdentityVectors.signer(),
            identityStore: IosDeviceIdentityKeychainStore(useMemoryOnly: true), exchange: exchange, sessions: sessions,
            nowEpochSeconds: { 1_763_892_000 }
        )
        let client = CommunityWriteClient(transport: transport, exchange: exchange, sessions: sessions, photoBytes: { _ in nil })
        let owner = try XCTUnwrap(transport.ownerScope())
        let scope = try XCTUnwrap(AccountDataExportScope(origin: Self.origin, ownerScope: owner, sessionFamilyId: Self.family))
        return (client, exchange, scope)
    }

    func testSharedSchemasAndRuntimeRuleContract() throws {
        let valid = try fixture("alerts-commands-valid")
        let batch = try XCTUnwrap(valid["value"] as? [String: Any])
        let commands = try XCTUnwrap(batch["commands"] as? [[String: Any]])
        let envelope = try XCTUnwrap(commands.first)
        XCTAssertEqual(valid["valid"] as? Bool, true)
        XCTAssertEqual(batch["family_id"] as? String, Self.family)
        XCTAssertEqual(envelope["operation_id"] as? String, Self.operation)
        XCTAssertEqual(envelope["rule_id"] as? String, Self.ruleID)
        let schemaOnly = AdvancedAlertCommand(operationId: Self.operation, ruleId: Self.ruleID, expectedRevision: 0)
        XCTAssertThrowsError(try AdvancedAlertWire.commandBody(schemaOnly))
        let invalid = try fixture("alerts-commands-invalid")
        XCTAssertEqual(invalid["valid"] as? Bool, false)
        let empty = try XCTUnwrap((invalid["value"] as? [String: Any])?["commands"] as? [Any])
        XCTAssertTrue(empty.isEmpty)
        let inbox = try fixture("alerts-inbox-valid")
        let inboxFields = try XCTUnwrap(inbox["value"] as? [String: Any])
        XCTAssertEqual(inboxFields["limit"] as? Int, 20)
        XCTAssertEqual(inboxFields["family_id"] as? String, Self.family)
        let body = try AdvancedAlertWire.commandBody(command())
        let posted = try XCTUnwrap((body["commands"] as? [[String: Any]])?.first)
        let payload = try XCTUnwrap(posted["payload"] as? [String: Any])
        XCTAssertEqual(payload["operator"] as? String, "AND")
        let predicate = try XCTUnwrap((payload["predicates"] as? [[String: Any]])?.first)
        XCTAssertEqual(predicate["threshold_milli"] as? Int, 6_200)
        XCTAssertEqual(predicate["fuel"] as? String, "ETHANOL")
        XCTAssertEqual(predicate["unit"] as? String, "L")
        let schedule = try XCTUnwrap(payload["schedule"] as? [String: Any])
        XCTAssertEqual(schedule["window_start"] as? String, "08:00")
        XCTAssertEqual(schedule["quiet_end"] as? String, "07:00")
    }

    func testBoundedVocabularyRejectsWrongInputAndOmitsEmptyTogglePayload() throws {
        XCTAssertNoThrow(try command().validate())
        for candidate in [
            AdvancedAlertPredicate(kind: .threshold, fuel: .ethanol, condition: .standard, source: .official, thresholdMilli: 0),
            AdvancedAlertPredicate(kind: .threshold, fuel: .lpgP13, condition: .standard, source: .official, thresholdMilli: 10),
            AdvancedAlertPredicate(kind: .threshold, fuel: .ethanol, condition: .standard, source: .official, thresholdMilli: 10, dropMilli: 1),
            AdvancedAlertPredicate(kind: .change, fuel: .cng, condition: .other, source: .community, dropMilli: 0, baselineHours: 168),
            AdvancedAlertPredicate(kind: .change, fuel: .cng, condition: .other, source: .community, dropPercentBp: 10_001, baselineHours: 168),
        ] {
            let invalid = AdvancedAlertCommand(ruleId: Self.ruleID, expectedRevision: 0, payload: rule(predicates: [candidate]))
            XCTAssertThrowsError(try AdvancedAlertWire.commandBody(invalid))
        }
        for target in [AdvancedAlertTarget(stationIds: []), AdvancedAlertTarget(stationIds: [Self.stationID, Self.stationID]),
                       AdvancedAlertTarget(stationIds: [Self.stationID], city: "Fixture city"),
                       AdvancedAlertTarget(stationIds: ["00000000-0000-0000-0000-000000000000"])] {
            XCTAssertThrowsError(try rule(target: target).validate())
        }
        let city = rule(target: AdvancedAlertTarget(city: "Fixture city"))
        XCTAssertNoThrow(try city.validate())
        XCTAssertThrowsError(try AdvancedAlertCommand(ruleId: Self.ruleID, expectedRevision: 0, payload: city).validate())
        XCTAssertThrowsError(try rule(schedule: AdvancedAlertSchedule(days: [0, 0], windowStart: "24:00", windowEnd: "10:00",
                                                                       timezone: "Unknown/Zone", quietStart: "00:00",
                                                                       quietEnd: "00:00", cooldownHours: 169)).validate())
        let change = AdvancedAlertPredicate(kind: .change, fuel: .cng, condition: .other, source: .community,
                                            thresholdMilli: 0, dropPercentBp: 10_000, baselineHours: 168)
        let edit = AdvancedAlertCommand(ruleId: Self.ruleID, expectedRevision: 3, payload: rule(predicates: [change]))
        let body = try AdvancedAlertWire.commandBody(edit)
        let wire = try XCTUnwrap((body["commands"] as? [[String: Any]])?.first?["payload"] as? [String: Any])
        let predicate = try XCTUnwrap((wire["predicates"] as? [[String: Any]])?.first)
        XCTAssertNil(predicate["threshold_milli"])
        XCTAssertEqual(predicate["drop_percent_bp"] as? Int, 10_000)
        XCTAssertEqual(predicate["unit"] as? String, "M3")
        let toggle = AdvancedAlertCommand(ruleId: Self.ruleID, expectedRevision: 3, enabled: false)
        let toggleBody = try AdvancedAlertWire.commandBody(toggle)
        let toggled = try XCTUnwrap((toggleBody["commands"] as? [[String: Any]])?.first)
        XCTAssertEqual(toggled["enabled"] as? Bool, false)
        XCTAssertNil(toggled["payload"])
    }

    func testReadsRequireLiveSessionAndWritesUseSameSignedBytesAndOperationID() async throws {
        let (client, exchange, scope) = try signedClient { request in
            switch request.url?.lastPathComponent {
            case "rules": return .init(200, #"{"rules":[]}"#)
            case "inbox": return .init(200, #"{"items":[]}"#)
            case "commands": return .init(200, #"{"results":[{"operation_id":"\#(Self.operation)","rule_id":"\#(Self.ruleID)","revision":1,"status":"accepted","replayed":false}]}"#)
            case "erase": return .init(200, #"{"status":"erased"}"#)
            default: return .init(500, "{}")
            }
        }
        let rules = try await client.advancedAlertRules(scope: scope)
        XCTAssertTrue(rules.isEmpty)
        let inbox = try await client.advancedAlertInbox(limit: 20, scope: scope)
        XCTAssertTrue(inbox.items.isEmpty)
        XCTAssertFalse(inbox.limitReached)
        let saved = try await client.executeAdvancedAlert(command(), scope: scope)
        let retry = try await client.executeAdvancedAlert(command(), scope: scope)
        XCTAssertEqual(saved, retry)
        try await client.eraseAdvancedAlerts(operationId: Self.operation, scope: scope)
        let alerts = exchange.requests.filter { $0.url?.path.hasPrefix("/v1/alerts/") == true }
        XCTAssertEqual(alerts.compactMap { $0.url?.lastPathComponent }, ["rules", "inbox", "commands", "commands", "erase"])
        XCTAssertEqual(alerts.count, 5)
        for (index, request) in alerts.enumerated() {
            let body = FakeIdentityBackend.json(request)
            XCTAssertEqual(request.httpMethod, "POST")
            XCTAssertNil(request.url?.query)
            XCTAssertEqual(body["family_id"] as? String, Self.family)
            XCTAssertEqual(body["access_token"] as? String, "fixture-access")
            for key in ["account_id", "owner", "token", "latitude", "longitude", "refresh_token"] {
                XCTAssertNil(body[key])
            }
            if index < 2 {
                XCTAssertNil(request.value(forHTTPHeaderField: "Signature"))
                XCTAssertNil(request.value(forHTTPHeaderField: "Idempotency-Key"))
            } else {
                XCTAssertNotNil(request.value(forHTTPHeaderField: "Signature"))
                XCTAssertEqual(request.value(forHTTPHeaderField: "Idempotency-Key"), Self.operation)
            }
        }
        let commandBodies = alerts.filter { $0.url?.lastPathComponent == "commands" }.map(\.httpBody)
        XCTAssertEqual(commandBodies.first, commandBodies.last)
        XCTAssertFalse(exchange.requests.contains { $0.url?.path.contains("/push/") == true })
    }

    func testRefusalsAndSameAccountFamilySwitchCannotCrossUnsignedRead() async throws {
        let statuses: [(Int, AdvancedAlertError)] = [
            (400, .invalid), (401, .authRequired), (403, .capabilityRequired), (404, .notFound), (409, .conflict),
            (408, .unavailable(retryAfterSeconds: 25)), (429, .rateLimited(retryAfterSeconds: 25)),
            (503, .unavailable(retryAfterSeconds: 25)),
        ]
        for (status, refusal) in statuses {
            let (client, exchange, scope) = try signedClient { _ in
                .init(status, #"{"error":{"code":"alerts.fixture-refusal"}}"#, headers: ["Retry-After": "25"])
            }
            do {
                _ = try await client.executeAdvancedAlert(command(), scope: scope)
                XCTFail("Expected refusal \(status)")
            } catch {
                XCTAssertEqual(error as? AdvancedAlertError, refusal)
            }
            XCTAssertEqual(exchange.requests.filter { $0.url?.path == "/v1/alerts/commands" }.count, 1)
        }
        let sessions = StaticSessions(session: Self.session())
        let (client, exchange, scope) = try signedClient(sessions) { _ in
            sessions.set(Self.session(family: Self.newFamily))
            return .init(200, #"{"items":[]}"#)
        }
        for _ in 0..<2 {
            do {
                _ = try await client.advancedAlertInbox(scope: scope)
                XCTFail("Old family accepted")
            } catch {
                let refusal = try XCTUnwrap(error as? AdvancedAlertError)
                XCTAssertTrue([.authRequired, .ownerChanged].contains(refusal))
            }
        }
        XCTAssertEqual(exchange.requests.filter { $0.url?.path == "/v1/alerts/inbox" }.count, 1)
    }

    func testInboxKeepsUnknownAndSuppressesPreviouslySeenNoticesAcrossRestart() throws {
        let directory = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString, isDirectory: true)
        defer { try? FileManager.default.removeItem(at: directory) }
        let scope = try self.scope()
        let store = AdvancedAlertStore(directory: directory)
        try store.setNotificationsEnabled(true, scope: scope, now: now)
        let first = try inbox([(Self.firstInbox, "sent")])
        XCTAssertTrue(try store.notificationCandidates(for: first, scope: scope, now: now).isEmpty)
        let evaluated = try inbox([(Self.firstInbox, "sent"), (Self.secondInbox, "evaluated")])
        XCTAssertTrue(try store.notificationCandidates(for: evaluated, scope: scope, now: now).isEmpty)
        let dispatched = try inbox([(Self.firstInbox, "sent"), (Self.secondInbox, "dispatched")])
        let candidates = try store.notificationCandidates(for: dispatched, scope: scope, now: now)
        XCTAssertEqual(candidates.map(\.id), [Self.secondInbox])
        let stable = store.notificationID(Self.secondInbox, scope: scope)
        try store.recordNotification(Self.secondInbox, scope: scope, now: now)
        let reopened = AdvancedAlertStore(directory: directory)
        XCTAssertEqual(reopened.notificationID(Self.secondInbox, scope: scope), stable)
        XCTAssertTrue(try reopened.notificationCandidates(for: dispatched, scope: scope, now: now).isEmpty)
        let other = try self.scope(family: Self.newFamily)
        XCTAssertFalse(try reopened.notificationsEnabled(scope: other, now: now))
        XCTAssertNotEqual(reopened.notificationID(Self.secondInbox, scope: other), stable)
        let unknown = try inbox([(Self.firstInbox, "future-status")])
        XCTAssertEqual(unknown.items.first?.status, .unknown)
        XCTAssertFalse(try XCTUnwrap(unknown.items.first).canNotify)
        try reopened.purge(scope: scope)
        XCTAssertFalse(try reopened.notificationsEnabled(scope: scope, now: now))
    }

    func testCommandClaimRetryIsolationExpiryAndPurge() throws {
        let directory = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString, isDirectory: true)
        defer { try? FileManager.default.removeItem(at: directory) }
        let scope = try self.scope()
        let store = AdvancedAlertStore(directory: directory)
        let frozen = command()
        try store.freeze(frozen, scope: scope, now: now)
        try store.freeze(frozen, scope: scope, now: now)
        XCTAssertThrowsError(try store.freeze(AdvancedAlertCommand(ruleId: Self.ruleID, expectedRevision: 4, enabled: false),
                                              scope: scope, now: now))
        let reopened = AdvancedAlertStore(directory: directory)
        XCTAssertEqual(try reopened.pending(scope: scope, now: now), frozen)
        let differentOrigin = try self.scope(origin: "https://fixture.invalid")
        XCTAssertNil(try reopened.pending(scope: differentOrigin, now: now))
        let differentOwner = try self.scope(owner: "account:other:fp")
        XCTAssertNil(try reopened.pending(scope: differentOwner, now: now))
        let initial = try XCTUnwrap(store.claim(scope: scope, now: now))
        XCTAssertNil(try reopened.claim(scope: scope, now: now.addingTimeInterval(119)))
        XCTAssertThrowsError(try reopened.discardPending(scope: scope, now: now.addingTimeInterval(119)))
        let afterClaim = now.addingTimeInterval(121)
        let retry = try XCTUnwrap(reopened.claim(scope: scope, now: afterClaim))
        XCTAssertEqual(retry.command, initial.command)
        XCTAssertThrowsError(try store.finish(initial, scope: scope, refusal: nil, now: afterClaim))
        try reopened.finish(retry, scope: scope, refusal: .rateLimited(retryAfterSeconds: 80), now: afterClaim)
        XCTAssertNil(try reopened.claim(scope: scope, now: afterClaim.addingTimeInterval(79)))
        let next = try XCTUnwrap(reopened.claim(scope: scope, now: afterClaim.addingTimeInterval(81)))
        try reopened.finish(next, scope: scope, refusal: .conflict, now: afterClaim.addingTimeInterval(82))
        XCTAssertNil(try reopened.pending(scope: scope, now: afterClaim.addingTimeInterval(82)))
        try reopened.freeze(frozen, scope: scope, now: now)
        XCTAssertNil(try reopened.pending(scope: scope, now: now.addingTimeInterval(7 * 24 * 60 * 60)))
        try reopened.freeze(frozen, scope: scope, now: now)
        let claim = try XCTUnwrap(reopened.claim(scope: scope, now: now))
        reopened.purge()
        try reopened.freeze(frozen, scope: scope, now: now)
        XCTAssertThrowsError(try reopened.finish(claim, scope: scope, refusal: nil, now: now))
        XCTAssertEqual(try reopened.pending(scope: scope, now: now), frozen)
    }

    private func scope(origin: String = AdvancedAlertsTests.origin, owner: String = "account:fixture:fp",
                       family: String = AdvancedAlertsTests.family) throws -> AccountDataExportScope {
        try XCTUnwrap(AccountDataExportScope(origin: origin, ownerScope: owner, sessionFamilyId: family))
    }

    private func inbox(_ rows: [(String, String)]) throws -> AdvancedAlertInbox {
        let items = rows.map { inboxId, status -> [String: Any] in
            ["inbox_id": inboxId, "rule_id": Self.ruleID, "status": status, "payload": [
                "rule_id": Self.ruleID, "station_id": Self.stationID, "source": "official", "at": "2026-10-02T17:00:00Z",
                "milli": 6_200, "provider_message_id": "fixture-provider-value", "dedupe": "fixture-internal-value",
            ]]
        }
        return try AdvancedAlertWire.inbox(JSONSerialization.data(withJSONObject: ["items": items]), limit: 100)
    }
}

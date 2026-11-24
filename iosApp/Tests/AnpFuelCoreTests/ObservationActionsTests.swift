import XCTest
@testable import AnpFuelCore

final class ObservationActionsTests: XCTestCase {
    private let observationId = "d6c74c23-63db-4c24-a2e5-408cb23bad26"
    private let voteId = "d6c74c23-63db-4c24-a2e5-408cb23bad27"
    private let origin = "https://abastevo.com.br"

    private func fixture(_ name: String, group: String) throws -> Data {
        #if SWIFT_PACKAGE
        var root = URL(fileURLWithPath: #filePath)
        for _ in 0..<4 { root.deleteLastPathComponent() }
        let url = root.appendingPathComponent("contracts/testdata").appendingPathComponent(group)
            .appendingPathComponent(name).appendingPathExtension("json")
        #else
        let url = try XCTUnwrap(Bundle(for: ObservationActionsTests.self).url(forResource: name, withExtension: "json"))
        #endif
        return try Data(contentsOf: url)
    }

    private func object(_ data: Data) throws -> [String: Any] {
        try XCTUnwrap(JSONSerialization.jsonObject(with: data) as? [String: Any])
    }

    func testHelpfulnessWireReplaysSharedVectorsAndKeepsNoVoteShareAbsent() throws {
        for name in ["helpful-cast-valid", "helpful-cast-invalid-choice"] {
            let fixtureData = try fixture(name, group: "community-helpfulness")
            let root = try object(fixtureData)
            var value = try XCTUnwrap(root["value"] as? [String: Any])
            let target = try XCTUnwrap(value.removeValue(forKey: "observation_id") as? String)
            let sequence = try XCTUnwrap(value["decision_sequence"] as? Int64)
            let choiceWire = try XCTUnwrap(value["choice"] as? String)
            let operationId = try XCTUnwrap(value["client_submission_id"] as? String)
            let choice = CommunityHelpfulnessChoice(rawValue: choiceWire)
            XCTAssertEqual(choice != nil, root["valid"] as? Bool, name)
            if let choice {
                let reference = try XCTUnwrap(CommunityContributionReference(observationId: target, decisionSequence: sequence))
                let intent = CommunityObservationIntent(reference: reference, ownerScope: "account:test", origin: origin,
                                                        action: .helpfulness(choice), operationId: operationId)
                let expected = try JSONSerialization.data(withJSONObject: value, options: [.sortedKeys])
                XCTAssertEqual(try CommunityObservationWire.body(for: intent), expected, name)
            }
        }

        for name in ["helpful-tally-empty", "helpful-tally-with-votes"] {
            let fixtureData = try fixture(name, group: "community-helpfulness")
            let root = try object(fixtureData)
            let value = try XCTUnwrap(root["value"] as? [String: Any])
            let target = try XCTUnwrap(value["observation_id"] as? String)
            let sequence = try XCTUnwrap(value["decision_sequence"] as? Int64)
            let reference = try XCTUnwrap(CommunityContributionReference(observationId: target, decisionSequence: sequence))
            let data = try JSONSerialization.data(withJSONObject: value)
            let tally = try CommunityObservationWire.tally(data, for: reference, ownerRead: true)
            let helpful = try XCTUnwrap(value["helpful"] as? Int64)
            let notHelpful = try XCTUnwrap(value["not_helpful"] as? Int64)
            XCTAssertEqual(tally.helpful, helpful, name)
            XCTAssertEqual(tally.notHelpful, notHelpful, name)
            XCTAssertEqual(tally.total, helpful + notHelpful, name)
            XCTAssertEqual(tally.helpfulBasisPoints, value["helpful_basis_points"] as? Int64, name)
            XCTAssertEqual(tally.myChoice?.rawValue, value["my_choice"] as? String, name)
            if tally.myChoice == nil {
                XCTAssertNoThrow(try CommunityObservationWire.tally(data, for: reference, ownerRead: false))
            } else {
                XCTAssertThrowsError(try CommunityObservationWire.tally(data, for: reference, ownerRead: false))
            }
        }
    }

    func testOperationIdentityChangesOnlyWithIntentAndOwnerScope() throws {
        let reference = try XCTUnwrap(CommunityContributionReference(observationId: observationId, decisionSequence: 7))
        let ownedFirst = CommunityObservationIntent.prepare(
            reference: reference, ownerScope: "account:a", origin: origin, action: .confirm, retrying: nil
        )
        let retry = CommunityObservationIntent.prepare(
            reference: reference, ownerScope: "account:a", origin: origin, action: .confirm, retrying: ownedFirst
        )
        XCTAssertEqual(retry, ownedFirst)
        XCTAssertEqual(try CommunityObservationWire.body(for: retry), try CommunityObservationWire.body(for: ownedFirst))
        let changed = CommunityObservationIntent.prepare(
            reference: reference, ownerScope: "account:a", origin: origin,
            action: .dispute(.wrongStation), retrying: retry
        )
        XCTAssertNotEqual(changed.operationId, retry.operationId)
        let otherAccount = CommunityObservationIntent.prepare(
            reference: reference, ownerScope: "account:b", origin: origin, action: .confirm, retrying: retry
        )
        XCTAssertNotEqual(otherAccount.operationId, retry.operationId)
        let otherOrigin = CommunityObservationIntent.prepare(
            reference: reference, ownerScope: "account:a", origin: "https://other.example", action: .confirm, retrying: retry
        )
        XCTAssertNotEqual(otherOrigin.operationId, retry.operationId)
        XCTAssertNil(CommunityContributionReference(observationId: "station-name", decisionSequence: 1))
        XCTAssertNil(CommunityContributionReference(observationId: observationId, decisionSequence: 0))
    }

    func testRetryPolicyHonorsServerDelayAndRefusesDomainErrors() {
        let now = Date(timeIntervalSince1970: 1_000)
        for status in [408, 425, 429, 500, 503] {
            let error = SignedTransportError.refused(status: status, code: nil, retryAfterSeconds: 37)
            XCTAssertEqual(CommunityObservationRetry.notBefore(error, now: now), Date(timeIntervalSince1970: 1_037))
        }
        XCTAssertEqual(CommunityObservationRetry.notBefore(SignedTransportError.offline, now: now), now)
        for status in [400, 401, 403, 404, 409] {
            let error = SignedTransportError.refused(status: status, code: nil, retryAfterSeconds: nil)
            XCTAssertNil(CommunityObservationRetry.notBefore(error, now: now))
        }
    }

    func testTallyRefusesForeignRevisionAndInconsistentTotals() throws {
        let reference = try XCTUnwrap(CommunityContributionReference(observationId: observationId, decisionSequence: 7))
        let base: [String: Any] = ["observation_id": observationId, "decision_sequence": 7, "helpful": 2,
                                   "not_helpful": 1, "total": 3, "helpful_basis_points": 6_666]
        for (field, bad) in [("observation_id", voteId as Any), ("decision_sequence", 8),
                             ("helpful", -2), ("total", 2), ("helpful_basis_points", 10_000)] {
            var value = base
            value[field] = bad
            let body = try JSONSerialization.data(withJSONObject: value)
            XCTAssertThrowsError(try CommunityObservationWire.tally(body, for: reference, ownerRead: true), field)
        }
        let review = base.merging(["under_review": true]) { old, _ in old }
        let body = try JSONSerialization.data(withJSONObject: review)
        XCTAssertTrue(try CommunityObservationWire.tally(body, for: reference, ownerRead: false).underReview)
    }

    func testFeedPreservesServerConsensusFromSharedVectorsAndKeepsActionsOffStationIds() throws {
        for name in ["self-confirmation-excluded", "retry-no-amplification", "moderation-disputed"] {
            let root = try object(try fixture(name, group: "consensus"))
            let expected = try XCTUnwrap(root["expect"] as? [String: Any])
            let votes = try XCTUnwrap(root["votes"] as? [[String: Any]])
            let priceKey = try XCTUnwrap(root["price_key"] as? [String: Any])
            let station = try XCTUnwrap(priceKey["station_id"] as? String)
            let now = try XCTUnwrap(root["now"] as? String)
            let row: [String: Any]
            if expected["verdict"] as? String == "PRICE" {
                let representative = try XCTUnwrap(votes.first?["observation_id"] as? String)
                row = [
                    "station_id": station, "station_name": "Fixture station", "fuel_product": "GASOLINE_REGULAR",
                    "unit": "L", "currency": "BRL", "source": "COMMUNITY", "condition": "STANDARD",
                    "updated_at": now, "expires_at": "2026-10-01T12:00:00Z",
                    "amount_milli_brl": try XCTUnwrap(expected["amount_milli"]),
                    "confirmations": expected["confirmations"] ?? 0,
                    "contribution": ["observation_id": representative, "decision_sequence": 2],
                ]
            } else {
                row = [:]
            }
            let feed: [String: Any] = [
                "state": "MT", "municipality_code": "5103403", "fuel_product": "GASOLINE_REGULAR", "sort": "recent",
                "generated_at": now, "items": row.isEmpty ? [] : [row], "next_cursor": "opaque.fixture",
            ]
            let page = try AbastevoHttpClient.decodeCommunityFeedPage(
                JSONSerialization.data(withJSONObject: feed), state: .MT, municipalityCode: "5103403",
                fuel: .gasolineRegular, sort: "recent"
            )
            XCTAssertEqual(page.nextCursor, "opaque.fixture", name)
            if row.isEmpty {
                XCTAssertTrue(page.items.isEmpty, "disputed projections cannot create a current price")
            } else {
                let observation = try XCTUnwrap(page.items.first?.observation)
                XCTAssertEqual(observation.confirmations, expected["confirmations"] as? Int ?? 0, name)
                XCTAssertEqual(observation.amountMilli, expected["amount_milli"] as? Int64, name)
                XCTAssertEqual(observation.stationId, station, name)
                XCTAssertEqual(observation.contribution?.observationId, votes.first?["observation_id"] as? String, name)
                XCTAssertNotEqual(observation.contribution?.observationId, station, name)
            }
        }
    }

    func testSignedCallsCarryExactTargetBodyAndStableRetryKey() async throws {
        let observationId = self.observationId
        let voteId = self.voteId
        let reference = try XCTUnwrap(CommunityContributionReference(observationId: observationId, decisionSequence: 7))
        let exchange = ScriptedExchange { request in
            FakeIdentityBackend.reply(request) { request in
                if request.url?.path.hasSuffix("/confirmations") == true {
                    return ScriptedExchange.Reply(201,
                        "{\"confirmation_id\":\"\(voteId)\",\"observation_id\":\"\(observationId)\",\"received_at\":\"2026-09-30T12:00:00Z\"}")
                }
                if request.url?.path.hasSuffix("/disputes") == true {
                    return ScriptedExchange.Reply(201,
                        "{\"dispute_id\":\"\(voteId)\",\"target_observation_id\":\"\(observationId)\",\"state\":\"OPEN\",\"received_at\":\"2026-09-30T12:00:00Z\"}")
                }
                if request.url?.path.hasSuffix("/helpfulness/remove") == true {
                    return ScriptedExchange.Reply(200,
                        "{\"observation_id\":\"\(observationId)\",\"decision_sequence\":7,\"helpful\":0,\"not_helpful\":0,\"total\":0}")
                }
                if request.httpMethod == "GET" {
                    return ScriptedExchange.Reply(200,
                        "{\"observation_id\":\"\(observationId)\",\"decision_sequence\":7,\"helpful\":1,\"not_helpful\":0,\"total\":1,\"helpful_basis_points\":10000,\"my_choice\":\"HELPFUL\",\"own_contribution\":false}")
                }
                return ScriptedExchange.Reply(201,
                    "{\"observation_id\":\"\(observationId)\",\"decision_sequence\":7,\"helpful\":1,\"not_helpful\":0,\"total\":1,\"helpful_basis_points\":10000,\"my_choice\":\"HELPFUL\",\"vote_id\":\"\(voteId)\"}")
            }
        }
        let sessions = StaticSessions(session: PortableAuth.Session(
            familyId: "f0000000-0000-4000-8000-000000000001",
            accountId: "a0000000-0000-4000-8000-000000000001",
            accessToken: "access-token", refreshToken: "refresh-token", accessExpiresAt: 2_000_000_000,
            absoluteExpiresAt: 2_100_000_000
        ))
        let store = IosDeviceIdentityKeychainStore(useMemoryOnly: true)
        defer { store.clear() }
        let transport = SignedRequestTransport(
            origin: origin, signer: try IdentityVectors.signer(), identityStore: store, exchange: exchange,
            sessions: sessions, nowEpochSeconds: { 1_763_892_000 }
        )
        let client = CommunityWriteClient(transport: transport, exchange: exchange, sessions: sessions, photoBytes: { _ in nil })
        let ownerScope = try XCTUnwrap(transport.ownerScope())
        for action in [CommunityObservationAction.confirm, .dispute(.wrongProduct), .helpfulness(.helpful), .removeHelpfulness] {
            let intent = CommunityObservationIntent.prepare(
                reference: reference, ownerScope: ownerScope, origin: origin, action: action, retrying: nil
            )
            let first = try await client.submitObservationAction(intent)
            let retry = try await client.submitObservationAction(intent)
            XCTAssertEqual(first, retry)
            let requests = exchange.requests.filter { $0.url?.path == reference.path + "/" + action.pathComponent }
            XCTAssertEqual(requests.count, 2)
            for request in requests {
                XCTAssertNotNil(request.value(forHTTPHeaderField: "Signature"))
                XCTAssertEqual(request.value(forHTTPHeaderField: "Idempotency-Key"), intent.operationId)
                XCTAssertEqual(request.httpBody, try CommunityObservationWire.body(for: intent))
            }
        }
        let overlay = try await client.ownerHelpfulness(for: reference, ownerScope: ownerScope)
        XCTAssertEqual(overlay.myChoice, .helpful)
        XCTAssertEqual(overlay.ownContribution, false)
        XCTAssertTrue(exchange.requests.contains {
            $0.httpMethod == "GET" && $0.url?.query == "decision_sequence=7" &&
                $0.value(forHTTPHeaderField: "Signature") != nil
        })
        let before = exchange.requests.count
        sessions.set(nil)
        let oldIntent = CommunityObservationIntent.prepare(
            reference: reference, ownerScope: ownerScope, origin: origin, action: .confirm, retrying: nil
        )
        do {
            _ = try await client.submitObservationAction(oldIntent)
            XCTFail("A former account may not retry")
        } catch {
            XCTAssertEqual(error as? CommunityWriteError, .signInRequired)
        }
        XCTAssertEqual(exchange.requests.count, before)
    }
}

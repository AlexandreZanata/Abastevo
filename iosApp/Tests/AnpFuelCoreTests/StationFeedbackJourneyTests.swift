import Foundation
import XCTest
@testable import AnpFuelCore

final class StationFeedbackJourneyTests: XCTestCase {
    private let stationId = "d6c74c23-63db-4c24-a2e5-408cb23bad26"
    private let commentId = "bbd138ce-e442-4f57-a874-3c05d2e16f52"
    private let operationId = "c0000000-0000-4000-8000-000000000001"
    private let origin = "https://abastevo.com.br"
    private let fuel = FuelProduct.gasolinePremium
    private let session = PortableAuth.Session(
        familyId: "f0000000-0000-4000-8000-000000000001",
        accountId: "a0000000-0000-4000-8000-000000000001",
        accessToken: "fixture-access",
        refreshToken: "fixture-refresh",
        accessExpiresAt: 2_000_000_000,
        absoluteExpiresAt: 2_100_000_000
    )

    private func fixture(_ name: String, group: String = "feedback") throws -> [String: Any] {
        #if SWIFT_PACKAGE
        var url = URL(fileURLWithPath: #filePath)
        for _ in 0..<4 { url.deleteLastPathComponent() }
        url = url.appendingPathComponent("contracts/testdata").appendingPathComponent(group)
            .appendingPathComponent(name).appendingPathExtension("json")
        #else
        let url = try XCTUnwrap(Bundle(for: Self.self).url(forResource: name, withExtension: "json"))
        #endif
        return try XCTUnwrap(JSONSerialization.jsonObject(with: Data(contentsOf: url)) as? [String: Any])
    }

    private func value(_ name: String) throws -> [String: Any] {
        try XCTUnwrap(fixture(name, group: "api")["value"] as? [String: Any])
    }

    private func encode(_ object: [String: Any]) throws -> Data {
        try XCTUnwrap(DeviceIdentityWire.encode(object))
    }

    private func reply(_ object: [String: Any]) throws -> String {
        String(decoding: try encode(object), as: UTF8.self)
    }

    private func harness(
        _ sessions: StaticSessions, respond: @escaping @Sendable (URLRequest) -> ScriptedExchange.Reply
    ) throws -> (CommunityWriteClient, ScriptedExchange, String) {
        let exchange = ScriptedExchange(respond: respond)
        let transport = SignedRequestTransport(
            origin: origin, signer: try IdentityVectors.signer(),
            identityStore: IosDeviceIdentityKeychainStore(useMemoryOnly: true), exchange: exchange, sessions: sessions
        )
        let client = CommunityWriteClient(transport: transport, exchange: exchange, sessions: sessions, photoBytes: { _ in nil })
        return (client, exchange, try XCTUnwrap(transport.ownerScope()))
    }

    func testRatingsAcceptOnlyFrozenStarRangeAndUseServerMean() throws {
        for name in ["feedback-rating-valid", "feedback-rating-zero", "feedback-rating-six"] {
            let row = try fixture(name)
            let input = try XCTUnwrap(row["inputs"] as? [String: Any])
            let stars = try XCTUnwrap(input["stars"] as? Int)
            XCTAssertEqual(PortableFeedback.isValidRating(stars), row["verdict"] as? String == "accept", name)
        }
        let receipt = try StationFeedbackWire.decodeRating(
            try encode(["rating": value("feedback-rating-receipt-valid"), "stats": value("feedback-rating-stats-valid")]),
            stationId: stationId, fuel: fuel, stars: 5
        )
        XCTAssertEqual(receipt.rating.revision, 1)
        XCTAssertEqual(receipt.stats.meanTenths, 42)
        var empty = try value("feedback-rating-stats-valid")
        empty["count"] = 0
        empty["sum"] = 0
        empty["mean_milli"] = NSNull()
        let stats = try AbastevoHttpClient.publicDecoder().decode(StationRatingStats.self, from: encode(empty))
        XCTAssertTrue(stats.matches(stationId: stationId, fuel: fuel))
        XCTAssertNil(stats.meanTenths)
        empty["mean_milli"] = 0
        let fabricated = try AbastevoHttpClient.publicDecoder().decode(StationRatingStats.self, from: encode(empty))
        XCTAssertFalse(fabricated.matches(stationId: stationId, fuel: fuel))
    }

    func testCommentMutationNormalizesTextAndCarriesRevision() throws {
        let textCase = try XCTUnwrap(fixture("feedback-text-crlf")["inputs"] as? [String: Any])
        let text = try XCTUnwrap(textCase["text"] as? String)
        let revisionCase = try XCTUnwrap(fixture("feedback-revision-supersede")["inputs"] as? [String: Any])
        let previous = try XCTUnwrap(revisionCase["old"] as? [String: Any])
        let revision = try XCTUnwrap(previous["revision"] as? Int64)
        let body = try XCTUnwrap(StationFeedbackWire.commentBody(session: session, text: text, expectedRevision: revision))
        let payload = try XCTUnwrap(DeviceIdentityWire.object(body))
        XCTAssertEqual(payload["text"] as? String, textCase["normalized"] as? String)
        XCTAssertEqual(payload["expected_revision"] as? Int64, revision)
        XCTAssertEqual(Set(payload.keys), ["family_id", "access_token", "text", "expected_revision"])
        XCTAssertNil(StationFeedbackWire.commentBody(session: session, text: "   ", expectedRevision: revision))
        XCTAssertNil(StationFeedbackWire.commentBody(session: session, text: String(repeating: "🥾", count: 281)))
        XCTAssertNil(StationFeedbackWire.commentBody(session: session, text: "hello", expectedRevision: 0))
    }

    func testPageRetainsCursorAndRejectsForeignStationOrReply() throws {
        var comment = try value("feedback-comment-view-valid")
        let initial = try StationFeedbackWire.decodePage(
            try encode(["comments": [comment], "next_cursor": "opaque+/token="]),
            stationId: stationId, fuel: fuel, parentId: nil
        )
        XCTAssertEqual(initial.nextCursor, "opaque+/token=")
        XCTAssertEqual(initial.comments.first?.revision, 1)
        XCTAssertThrowsError(try StationFeedbackWire.decodePage(
            try encode(["comments": [comment]]), stationId: operationId, fuel: fuel, parentId: nil
        ))
        comment["parent_id"] = commentId
        comment["depth"] = 1
        comment["id"] = operationId
        let bytes = try encode(["comments": [comment]])
        XCTAssertThrowsError(try StationFeedbackWire.decodePage(bytes, stationId: stationId, fuel: fuel, parentId: nil))
        let replies = try StationFeedbackWire.decodePage(bytes, stationId: stationId, fuel: fuel, parentId: commentId)
        XCTAssertEqual(replies.comments.first?.parentId, commentId)
    }

    func testRatingAndRemovalUseSessionOnlyAndSameOperationOnRetry() async throws {
        let ratingReply = try reply(["rating": value("feedback-rating-receipt-valid"), "stats": value("feedback-rating-stats-valid")])
        let (client, exchange, scope) = try harness(StaticSessions(session: session)) { request in
            if request.url?.path.hasSuffix("/remove") == true { return .init(200, #"{"status":"deleted"}"#) }
            return .init(201, ratingReply)
        }
        for _ in 0..<2 {
            let result = try await client.rateStation(stationId: stationId, fuel: fuel, stars: 5, ownerScope: scope, operationId: operationId)
            XCTAssertEqual(result.rating.stars, 5)
        }
        try await client.removeStationRating(stationId: stationId, fuel: fuel, ownerScope: scope, operationId: operationId)
        XCTAssertEqual(exchange.requests.map { $0.url?.path }, [
            "/v1/feedback/ratings", "/v1/feedback/ratings", "/v1/feedback/ratings/remove",
        ])
        for request in exchange.requests {
            XCTAssertNil(request.value(forHTTPHeaderField: "Signature"))
            XCTAssertEqual(request.value(forHTTPHeaderField: "Idempotency-Key"), operationId)
            let payload = FakeIdentityBackend.json(request)
            XCTAssertEqual(payload["access_token"] as? String, session.accessToken)
            XCTAssertNil(payload["operation_id"])
        }
        XCTAssertEqual(exchange.requests[0].httpBody, exchange.requests[1].httpBody)
    }

    func testFeedbackRefusalsKeepStatusCodeAndRetryAfter() async throws {
        for (status, code) in [(400, "feedback.rating-out-of-range"), (401, "feedback.session-invalid"),
                               (403, "feedback.author-forbidden"), (404, "feedback.comment-not-found"),
                               (408, "feedback.unavailable"), (409, "feedback.stale-revision"),
                               (429, "feedback.quota-exceeded"), (503, "feedback.unavailable")] {
            let body = try reply(["error": ["code": code]])
            let (client, exchange, scope) = try harness(StaticSessions(session: session)) { _ in
                .init(status, body, headers: ["Retry-After": "30"])
            }
            do {
                try await client.removeStationComment(commentId: commentId, ownerScope: scope, operationId: operationId)
                XCTFail("Expected server refusal")
            } catch {
                XCTAssertEqual(error as? SignedTransportError,
                               .refused(status: status, code: code, retryAfterSeconds: 30))
            }
            XCTAssertEqual(exchange.requests.count, 1)
        }
    }

    func testOwnerChangeBeforeOrDuringSessionOnlyWriteDiscardsResponse() async throws {
        let sessions = StaticSessions(session: session)
        let (client, exchange, scope) = try harness(sessions) { _ in
            sessions.set(nil)
            return .init(200, #"{"status":"deleted"}"#)
        }
        for _ in 0..<2 {
            do {
                try await client.removeStationComment(commentId: commentId, ownerScope: scope, operationId: operationId)
                XCTFail("Old scope must not accept this result")
            } catch {
                XCTAssertEqual(error as? SignedTransportError, .sessionRequired)
            }
        }
        XCTAssertEqual(exchange.requests.count, 1)
    }

    func testFeedbackOwnershipSurvivesOutboxRestartAndIsPurged() throws {
        let directory = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString, isDirectory: true)
        defer { try? FileManager.default.removeItem(at: directory) }
        let store = CommunityOutboxStore(directory: directory)
        let scope = "account:fixture:fp:abcdef"
        let item = CommunityOutboxItem(
            id: operationId, kind: .stationComment, ownerScope: scope, origin: origin, createdAtMillis: 0,
            text: CommunityTextDraft(text: "fixture", stationId: stationId, product: fuel),
            status: .accepted, receiptId: commentId
        )
        store.save([item])
        let reopened = CommunityOutboxStore(directory: directory)
        let items = reopened.load()
        XCTAssertEqual(CommunityOutbox.ownedStationCommentIds(items, ownerScope: scope, origin: origin, stationId: stationId, fuel: fuel), [commentId])
        XCTAssertTrue(CommunityOutbox.ownedStationCommentIds(items, ownerScope: "account:other:fp:abcdef", origin: origin, stationId: stationId, fuel: fuel).isEmpty)
        XCTAssertTrue(CommunityOutbox.ownedStationCommentIds(items, ownerScope: scope, origin: "https://other.example", stationId: stationId, fuel: fuel).isEmpty)
        reopened.purge()
        XCTAssertTrue(store.load().isEmpty)
    }
}

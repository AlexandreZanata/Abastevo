import Foundation
import XCTest
@testable import AnpFuelCore

final class CommunitySafetyClientTests: XCTestCase {
    private static let itemId = "a1000000-0000-4000-8000-000000000001"
    private static let foreignId = "a1000000-0000-4000-8000-000000000002"
    private static let blockId = "b1000000-0000-4000-8000-000000000001"
    private static let reportId = "c1000000-0000-4000-8000-000000000001"
    private static let caseId = "c1000000-0000-4000-8000-000000000002"
    private static let operationId = "d1000000-0000-4000-8000-000000000001"

    private struct Harness {
        let client: CommunityWriteClient
        let exchange: ScriptedExchange
        let scope: String

        var calls: [URLRequest] {
            exchange.requests.filter {
                guard let path = $0.url?.path else { return false }
                return path.hasPrefix("/v1/community/") || path.hasSuffix("/report")
            }
        }
    }

    private struct DelayedHiddenExchange: SignedHttpExchange {
        let base: ScriptedExchange
        let delay: Duration

        func send(_ request: URLRequest) async throws -> (Data, HTTPURLResponse) {
            if request.url?.path == "/v1/community/blocks/hidden" {
                try await Task.sleep(for: delay)
            }
            return try await base.send(request)
        }
    }

    private static func session(_ id: String = "a0000000-0000-4000-8000-000000000001") -> PortableAuth.Session {
        PortableAuth.Session(
            familyId: "f0000000-0000-4000-8000-000000000001",
            accountId: id,
            accessToken: "fixture-safety-access",
            refreshToken: "fixture-safety-refresh",
            accessExpiresAt: 2_000_000_000,
            absoluteExpiresAt: 2_100_000_000
        )
    }

    private func harness(
        sessions: StaticSessions? = nil,
        origin: String = "https://abastevo.com.br",
        hiddenDelay: Duration? = nil,
        responding: @escaping @Sendable (URLRequest) -> ScriptedExchange.Reply
    ) throws -> Harness {
        let sessions = sessions ?? StaticSessions(session: Self.session())
        let exchange = ScriptedExchange { request in
            FakeIdentityBackend.reply(request) { responding($0) }
        }
        let network: any SignedHttpExchange
        if let hiddenDelay {
            network = DelayedHiddenExchange(base: exchange, delay: hiddenDelay)
        } else {
            network = exchange
        }
        let transport = SignedRequestTransport(
            origin: origin,
            signer: try IdentityVectors.signer(),
            identityStore: IosDeviceIdentityKeychainStore(useMemoryOnly: true),
            exchange: network,
            sessions: sessions,
            nowEpochSeconds: { 1_763_892_000 }
        )
        return Harness(
            client: CommunityWriteClient(transport: transport, exchange: network, sessions: sessions, photoBytes: { _ in nil }),
            exchange: exchange,
            scope: try XCTUnwrap(transport.ownerScope())
        )
    }

    func testReportDetailsAndReferenceBoundaries() {
        let canonical = CommunitySafetyReference(kind: .forumPost, id: "  \(Self.itemId.uppercased()) ")
        XCTAssertTrue(canonical.isWellFormed)
        XCTAssertEqual(canonical.id, Self.itemId)
        XCTAssertFalse(CommunitySafetyReference(kind: .forumPost, id: "../\(Self.itemId)").isWellFormed)
        XCTAssertNil(CommunitySafetyWire.normalizedUserDetail(" \r\n "))
        XCTAssertEqual(CommunitySafetyWire.normalizedUserDetail(" a\r\nb "), "a\nb")
        XCTAssertTrue(CommunitySafetyWire.userDetailFits(String(repeating: "🚲", count: 200)))
        XCTAssertFalse(CommunitySafetyWire.userDetailFits(String(repeating: "e\u{301}", count: 101)))
        XCTAssertTrue(CommunitySafetyWire.forumDetailFits(String(repeating: "🚲", count: 250)))
        XCTAssertFalse(CommunitySafetyWire.forumDetailFits(String(repeating: "🚲", count: 251)))
        XCTAssertEqual(CommunitySafetyWire.normalizedForumDetail(" a\r\nb "), "a\r\nb")
    }

    func testSignedUserReportReplayAndBlockLifecycle() async throws {
        let row = #"{"block_id":"\#(Self.blockId)","label":"Alias","source_kind":"FORUM_POST","created_at":"2025-11-23T10:00:00Z"}"#
        let item = #"{"content_kind":"FORUM_POST","content_id":"\#(Self.itemId)"}"#
        let foreign = #"{"content_kind":"FORUM_POST","content_id":"\#(Self.foreignId)"}"#
        let fixture = try harness { request in
            switch request.url?.path ?? "" {
            case "/v1/community/user-reports":
                return .init(200, #"{"receipt":{"report_id":"\#(Self.reportId)","reason":"SPAM","created_at":"2025-11-23T10:00:00Z","replayed":true}}"#)
            case "/v1/community/blocks":
                return .init(200, #"{"block":\#(row),"replayed":false}"#)
            case "/v1/community/blocks/list":
                return .init(200, #"{"items":[\#(row)]}"#)
            case "/v1/community/blocks/hidden":
                return .init(200, #"{"hidden":[\#(item),\#(foreign),{"content_kind":"UNKNOWN","content_id":"\#(Self.itemId)"}]}"#)
            case "/v1/community/blocks/\(Self.blockId)/remove":
                return .init(200, #"{"removed":true}"#)
            default:
                return .init(404, "{}")
            }
        }
        let reference = CommunitySafetyReference(kind: .forumPost, id: Self.itemId)
        let receipt = try await fixture.client.reportCommunityUser(reference, reason: .hate, detail: "  line\r\nnext ",
                                                                   operationId: Self.operationId, expectedScope: fixture.scope)
        XCTAssertEqual(receipt.reportId, Self.reportId)
        XCTAssertEqual(receipt.reason, .spam, "a repeat returns the original report reason")
        XCTAssertTrue(receipt.replayed)

        let block = try await fixture.client.blockCommunityUser(reference, operationId: Self.operationId, expectedScope: fixture.scope)
        let blocks = try await fixture.client.listCommunityBlocks(expectedScope: fixture.scope)
        let hidden = try await fixture.client.hiddenCommunityContent([reference, reference], expectedScope: fixture.scope)
        try await fixture.client.removeCommunityBlock(id: Self.blockId, operationId: Self.operationId, expectedScope: fixture.scope)
        XCTAssertEqual(block.user.label, "Alias")
        XCTAssertFalse(block.replayed)
        XCTAssertEqual(blocks, [block.user])
        XCTAssertEqual(hidden, [reference])

        let calls = fixture.calls
        XCTAssertEqual(calls.count, 5)
        for call in calls {
            XCTAssertEqual(call.value(forHTTPHeaderField: "Cache-Control"), "no-store")
            XCTAssertNotNil(call.value(forHTTPHeaderField: "Signature"))
            XCTAssertEqual(FakeIdentityBackend.json(call)["family_id"] as? String, Self.session().familyId)
            XCTAssertNil(FakeIdentityBackend.json(call)["account_id"])
            XCTAssertNil(FakeIdentityBackend.json(call)["operation_id"])
        }
        let posted = FakeIdentityBackend.json(try XCTUnwrap(calls.first))
        XCTAssertEqual(posted["detail"] as? String, "line\nnext")
        XCTAssertEqual(posted["reason"] as? String, "HATE")
        let list = try XCTUnwrap(calls.first(where: { $0.url?.path.hasSuffix("/list") == true }))
        XCTAssertEqual(Set(FakeIdentityBackend.json(list).keys), ["family_id", "access_token"])
        let lookup = try XCTUnwrap(calls.first(where: { $0.url?.path.hasSuffix("/hidden") == true }))
        XCTAssertEqual((FakeIdentityBackend.json(lookup)["items"] as? [[String: String]])?.count, 1)
    }

    func testContentReportsUseDistinctAuthoritiesAndReceipts() async throws {
        let fixture = try harness { request in
            let path = request.url?.path ?? ""
            if path.hasPrefix("/v1/feedback/") { return .init(202, #"{"status":"reported"}"#) }
            let kind = path.hasPrefix("/v1/forum/posts/") ? "FORUM_POST" : "FORUM_COMMENT"
            return .init(200, #"{"receipt":{"report_id":"\#(Self.reportId)","case_id":"\#(Self.caseId)","target_type":"\#(kind)","target_id":"\#(Self.itemId)","reason":"DEFAMATION","status":"OPEN","created_at":"2025-11-23T10:00:00Z","replayed":false}}"#)
        }
        let topic = CommunitySafetyReference(kind: .forumPost, id: Self.itemId)
        let reply = CommunitySafetyReference(kind: .forumComment, id: Self.itemId)
        let station = CommunitySafetyReference(kind: .stationComment, id: Self.itemId)
        let topicReceipt = try await fixture.client.reportForumContent(topic, reason: .defamation, detail: " commentary ",
                                                                      operationId: Self.operationId, expectedScope: fixture.scope)
        let replyReceipt = try await fixture.client.reportForumContent(reply, reason: .defamation, detail: nil,
                                                                      operationId: Self.operationId, expectedScope: fixture.scope)
        try await fixture.client.reportStationComment(station, reason: .offensive, operationId: Self.operationId, expectedScope: fixture.scope)

        XCTAssertEqual(topicReceipt.caseId, Self.caseId)
        XCTAssertEqual(replyReceipt.targetType, "FORUM_COMMENT")
        let calls = fixture.calls
        XCTAssertEqual(calls.map { $0.url?.path }, [
            "/v1/forum/posts/\(Self.itemId)/report",
            "/v1/forum/comments/\(Self.itemId)/report",
            "/v1/feedback/comments/\(Self.itemId)/report",
        ])
        XCTAssertEqual(calls.map { $0.value(forHTTPHeaderField: "Signature") != nil }, [true, true, false])
        let stationCall = try XCTUnwrap(calls.last)
        let stationBody = FakeIdentityBackend.json(stationCall)
        XCTAssertEqual(stationBody["reason"] as? String, "offensive")
        XCTAssertEqual(Set(stationBody.keys), ["reason", "family_id", "access_token"])
    }

    func testRefusalsStayTypedAndExplicitReattemptKeepsIdempotencyKey() async throws {
        let refusals: [(Int, String)] = [
            (400, "safety.bad-request"), (400, "safety.self-target"), (401, "safety.auth-required"),
            (404, "safety.not-found"), (408, "request.timeout"), (409, "safety.block-limit"),
            (429, "safety.rate-limited"), (503, "safety.unavailable"),
        ]
        for (status, code) in refusals {
            let fixture = try harness { _ in
                .init(status, #"{"error":{"code":"\#(code)"}}"#, headers: status == 429 ? ["Retry-After": "52"] : [:])
            }
            let reference = CommunitySafetyReference(kind: .forumPost, id: Self.itemId)
            for _ in 0..<2 {
                do {
                    _ = try await fixture.client.blockCommunityUser(reference, operationId: Self.operationId, expectedScope: fixture.scope)
                    XCTFail("The server refused \(code)")
                } catch {
                    XCTAssertEqual(CommunitySafetyError.from(error),
                                   .refused(status: status, code: code, retryAfterSeconds: status == 429 ? 52 : nil))
                }
            }
            XCTAssertEqual(fixture.calls.count, 2, "The route is attempted once per action")
            XCTAssertEqual(fixture.calls.map { $0.value(forHTTPHeaderField: "Idempotency-Key") },
                           [Self.operationId, Self.operationId])
            XCTAssertEqual(fixture.calls.first?.httpBody, fixture.calls.last?.httpBody)
        }
    }

    func testValidationAndHiddenBatchBoundFailBeforeNetwork() async throws {
        let fixture = try harness { _ in .init(500, "{}") }
        let references = (0...CommunitySafetyWire.maxHiddenBatch).map {
            let tail = String(repeating: "0", count: 12 - String($0).count) + String($0)
            return CommunitySafetyReference(kind: .stationComment, id: "a1000000-0000-4000-8000-\(tail)")
        }
        do {
            _ = try await fixture.client.hiddenCommunityContent(references, expectedScope: fixture.scope)
            XCTFail("A 101-item batch is invalid")
        } catch {
            XCTAssertEqual(CommunitySafetyError.from(error), .invalidTarget)
        }
        do {
            let reference = try XCTUnwrap(references.first)
            _ = try await fixture.client.reportCommunityUser(reference, reason: .other,
                detail: String(repeating: "🚲", count: 201), operationId: Self.operationId, expectedScope: fixture.scope)
            XCTFail("Oversized report detail is invalid")
        } catch {
            XCTAssertEqual(CommunitySafetyError.from(error), .invalidDetail)
        }
        XCTAssertTrue(fixture.exchange.requests.isEmpty)
    }

    func testHiddenFilteringBatchesKeepsLocalBlockAndFailsOpenPerRequest() async throws {
        let references = (0...CommunitySafetyWire.maxHiddenBatch + 1).map {
            let tail = String(repeating: "0", count: 12 - String($0).count) + String($0)
            return CommunitySafetyReference(kind: .stationComment, id: "a1000000-0000-4000-8000-\(tail)")
        }
        let known = try XCTUnwrap(references.first)
        let hiddenOnServer = references[1]
        let fixture = try harness { request in
            let items = FakeIdentityBackend.json(request)["items"] as? [[String: String]]
            if items?.count == CommunitySafetyWire.maxHiddenBatch {
                return .init(200, #"{"hidden":[{"content_kind":"STATION_COMMENT","content_id":"\#(hiddenOnServer.id)"}]}"#)
            }
            return .init(503, #"{"error":{"code":"safety.unavailable"}}"#)
        }
        let result = await fixture.client.resolveHiddenCommunityContent(
            references + [hiddenOnServer], immediatelyHidden: [known], knownBlockCount: 1, expectedScope: fixture.scope
        )
        XCTAssertEqual(result?.hidden, [known, hiddenOnServer])
        XCTAssertEqual(result?.blockCount, 1)
        let batchSizes = fixture.calls.compactMap { request in
            (FakeIdentityBackend.json(request)["items"] as? [[String: String]])?.count
        }
        XCTAssertEqual(batchSizes, [100, 1])
    }

    func testHiddenTimeoutKeepsLocalBlockAndLeavesOtherContentVisible() async throws {
        let fixture = try harness(hiddenDelay: .seconds(1)) { _ in
            .init(200, #"{"hidden":[{"content_kind":"FORUM_COMMENT","content_id":"\#(Self.foreignId)"}]}"#)
        }
        let known = CommunitySafetyReference(kind: .forumComment, id: Self.itemId)
        let wait = CommunitySafetyReference(kind: .forumComment, id: Self.foreignId)
        let result = await fixture.client.resolveHiddenCommunityContent(
            [known, wait], immediatelyHidden: [known], knownBlockCount: 1,
            expectedScope: fixture.scope, budget: .milliseconds(20)
        )
        XCTAssertEqual(result?.hidden, [known])
    }

    func testHiddenFilteringSkipsNetworkForKnownEmptyBlockList() async throws {
        let fixture = try harness { _ in .init(500, "{}") }
        let ref = CommunitySafetyReference(kind: .forumPost, id: Self.itemId)
        let result = await fixture.client.resolveHiddenCommunityContent(
            [ref], immediatelyHidden: [], knownBlockCount: 0, expectedScope: fixture.scope
        )
        XCTAssertEqual(result?.hidden, [])
        XCTAssertEqual(result?.blockCount, 0)
        XCTAssertTrue(fixture.exchange.requests.isEmpty)
    }

    func testWrongFeedbackStatusAndCrossTargetForumReceiptAreRefused() async throws {
        let fixture = try harness { request in
            if request.url?.path.hasPrefix("/v1/feedback/") == true { return .init(200, #"{"status":"reported"}"#) }
            return .init(200, #"{"receipt":{"report_id":"\#(Self.reportId)","case_id":"\#(Self.caseId)","target_type":"FORUM_COMMENT","target_id":"\#(Self.foreignId)","reason":"SPAM","status":"OPEN","created_at":"2025-11-23T10:00:00Z","replayed":false}}"#)
        }
        do {
            try await fixture.client.reportStationComment(CommunitySafetyReference(kind: .stationComment, id: Self.itemId),
                reason: .spam, operationId: Self.operationId, expectedScope: fixture.scope)
            XCTFail("Only the HTTP 202 acknowledgment counts")
        } catch {
            XCTAssertEqual(CommunitySafetyError.from(error), .invalidResponse)
        }
        do {
            _ = try await fixture.client.reportForumContent(CommunitySafetyReference(kind: .forumComment, id: Self.itemId),
                reason: .spam, detail: nil, operationId: Self.operationId, expectedScope: fixture.scope)
            XCTFail("A receipt for another target is not a confirmation")
        } catch {
            XCTAssertEqual(CommunitySafetyError.from(error), .invalidResponse)
        }
    }

    func testOwnerChangeBeforeResponseRefusesAndFreshOriginUsesServerList() async throws {
        let sessions = StaticSessions(session: Self.session())
        let old = try harness(sessions: sessions) { _ in
            sessions.set(Self.session("a0000000-0000-4000-8000-000000000002"))
            return .init(200, "{\"items\":[]}")
        }
        do {
            _ = try await old.client.listCommunityBlocks(expectedScope: old.scope)
            XCTFail("A private result must not survive an account change")
        } catch {
            XCTAssertEqual(old.calls.count, 1)
        }
        let fresh = try harness(sessions: sessions, origin: "https://teste.abastevo.com.br") { _ in .init(200, "{\"items\":[]}") }
        let freshBlocks = try await fresh.client.listCommunityBlocks(expectedScope: fresh.scope)
        XCTAssertTrue(freshBlocks.isEmpty)
        XCTAssertEqual(fresh.calls.first?.url?.host, "teste.abastevo.com.br")
    }
}

import Foundation
import XCTest
@testable import AnpFuelCore

final class ForumJourneysTests: XCTestCase {
    private let postId = "44444444-4444-4444-8444-444444444444"
    private let parentId = "33333333-3333-4333-8333-333333333333"
    private let replyId = "55555555-5555-4555-8555-555555555555"
    private let operationId = "22222222-2222-4222-8222-222222222222"
    private let session = PortableAuth.Session(
        familyId: "11111111-1111-4111-8111-111111111111",
        accountId: "a0000000-0000-4000-8000-000000000001",
        accessToken: "fixture-access-token",
        refreshToken: "fixture-refresh-token",
        accessExpiresAt: 2_000_000_000,
        absoluteExpiresAt: 2_100_000_000
    )

    private func fixture(_ name: String) throws -> Data {
        #if SWIFT_PACKAGE
        let repo = (0..<4).reduce(URL(fileURLWithPath: #filePath)) { url, _ in url.deletingLastPathComponent() }
        let url = repo.appendingPathComponent("contracts/testdata/api/\(name).json")
        #else
        let url = try XCTUnwrap(Bundle(for: Self.self).url(forResource: name, withExtension: "json"))
        #endif
        let data = try Data(contentsOf: url)
        let document = try XCTUnwrap(JSONSerialization.jsonObject(with: data) as? [String: Any])
        return try JSONSerialization.data(withJSONObject: XCTUnwrap(document["value"]), options: .sortedKeys)
    }

    private func object(_ data: Data?) throws -> NSDictionary {
        try XCTUnwrap(JSONSerialization.jsonObject(with: XCTUnwrap(data)) as? NSDictionary)
    }

    func testFeedAndDiscussionReadSharedVectorsWithoutAssumingOldCounts() throws {
        let feed = try ForumReadWire.feed(fixture("forum-feed-response-valid"), limit: 20)
        XCTAssertEqual(feed.snapshot, "rev-1")
        XCTAssertTrue(feed.items.isEmpty)
        XCTAssertNil(feed.nextCursor)

        let comments = try ForumReadWire.comments(
            fixture("forum-comments-response-valid"), postId: postId, parentId: nil, limit: 20
        )
        let parent = try XCTUnwrap(comments.items.first)
        XCTAssertEqual(parent.id, parentId)
        XCTAssertEqual(parent.depth, 0)
        XCTAssertEqual(parent.revision, 1)
        XCTAssertNil(parent.reactionCounts, "The old fixture has no aggregate. Keep it unknown.")

        let replies = try ForumReadWire.comments(
            fixture("forum-replies-response-valid"), postId: postId, parentId: parentId, limit: 20
        )
        XCTAssertEqual(replies.items.map(\.id), [replyId])
        XCTAssertEqual(replies.items.first?.depth, 1)
        XCTAssertEqual(replies.items.first?.parentId, parentId)
        XCTAssertThrowsError(try ForumReadWire.comments(
            fixture("forum-replies-response-valid"), postId: postId, parentId: nil, limit: 20
        ))
        XCTAssertThrowsError(try ForumReadWire.comments(
            fixture("forum-replies-response-valid"), postId: postId, parentId: replyId, limit: 20
        ))
        XCTAssertThrowsError(try ForumReadWire.comments(
            fixture("forum-replies-response-valid"), postId: replyId, parentId: parentId, limit: 20
        ))
    }

    func testRankedCommentPreviewIsBoundToItsParent() throws {
        let parents = try XCTUnwrap(try object(fixture("forum-comments-response-valid"))["items"] as? [[String: Any]])
        let replies = try XCTUnwrap(try object(fixture("forum-replies-response-valid"))["items"] as? [[String: Any]])
        var parent = try XCTUnwrap(parents.first)
        var reply = try XCTUnwrap(replies.first)
        parent["top_reply"] = reply
        let page = try JSONSerialization.data(withJSONObject: ["items": [parent]])
        let decoded = try ForumReadWire.comments(page, postId: postId, parentId: nil, limit: 20)
        XCTAssertEqual(decoded.items.first?.topReply?.comment.id, replyId)

        reply["parent_id"] = replyId
        parent["top_reply"] = reply
        let wrongParent = try JSONSerialization.data(withJSONObject: ["items": [parent]])
        XCTAssertThrowsError(try ForumReadWire.comments(wrongParent, postId: postId, parentId: nil, limit: 20))
    }

    func testSearchIsLimitedToLoadedPostsAndIgnoresCaseAndDiacritics() throws {
        let post = try AbastevoHttpClient.publicDecoder().decode(ForumPublicPost.self, from: fixture("forum-post-summary-valid"))
        XCTAssertEqual(ForumReadWire.filteredPosts([post], query: "  FORMAÇÃO  ").map(\.id), [post.id])
        XCTAssertEqual(ForumReadWire.filteredPosts([post], query: "TRIBUTÁRIA").map(\.id), [post.id])
        XCTAssertTrue(ForumReadWire.filteredPosts([post], query: "charging").isEmpty)
        XCTAssertTrue(ForumReadWire.filteredPosts([], query: post.title).isEmpty)
    }

    func testCursorChainKeepsOrderRejectsCyclesAndDropsStaleResults() throws {
        struct Row: Identifiable { let id: Int }
        var page = ForumPageAccumulator<Row>()
        try page.replace([Row(id: 1), Row(id: 1)], nextCursor: "cursor-a")
        try page.append([Row(id: 1), Row(id: 2)], after: "cursor-a", nextCursor: "cursor-b")
        XCTAssertEqual(page.items.map(\.id), [1, 2])
        XCTAssertThrowsError(try page.append([Row(id: 3)], after: "cursor-a", nextCursor: nil))
        XCTAssertThrowsError(try page.append([Row(id: 3)], after: "cursor-b", nextCursor: "cursor-a"))
        XCTAssertThrowsError(try page.append([Row(id: 3)], after: "cursor-b", nextCursor: "cursor-b"))
        XCTAssertEqual(page.items.map(\.id), [1, 2])
        try page.append([Row(id: 3)], after: "cursor-b", nextCursor: nil)
        XCTAssertEqual(page.items.map(\.id), [1, 2, 3])
        XCTAssertNil(page.nextCursor)

        let filters = ForumFeedFilters(category: "FUELS", uf: "SP", municipalityIbge: 3_550_308)
        let query = try filters.query(limit: 20, cursor: "opaque-cursor")
        XCTAssertEqual(query, [
            URLQueryItem(name: "limit", value: "20"), URLQueryItem(name: "cursor", value: "opaque-cursor"),
            URLQueryItem(name: "category", value: "FUELS"), URLQueryItem(name: "uf", value: "SP"),
            URLQueryItem(name: "municipality_ibge", value: "3550308"),
        ])
        XCTAssertThrowsError(try filters.query(limit: 51, cursor: nil))
        XCTAssertThrowsError(try filters.query(limit: 20, cursor: String(repeating: "z", count: 513)))
    }

    func testCommentEditAndRemoveHonorCompareAndSwapFixtures() throws {
        let expectedEdit = try object(fixture("forum-edit-comment-valid"))
        let revised = try XCTUnwrap(expectedEdit["body_text"] as? String)
        let edit = ForumCommand.editComment(commentId: parentId, revision: 1, text: "  \(revised)  ")
        XCTAssertEqual(edit.path, "/v1/forum/comments/\(parentId)/edit")
        XCTAssertEqual(try object(edit.body(session: session, operationId: operationId)), expectedEdit)

        let remove = ForumCommand.removeComment(commentId: parentId, revision: 1)
        XCTAssertEqual(remove.path, "/v1/forum/comments/\(parentId)/remove")
        XCTAssertEqual(try object(remove.body(session: session, operationId: operationId)),
                       try object(fixture("forum-remove-comment-valid")))

        let rejected = try object(fixture("forum-edit-comment-invalid"))
        let zero = try XCTUnwrap(rejected["expected_revision"] as? Int64)
        XCTAssertNil(ForumCommand.editComment(commentId: parentId, revision: zero, text: revised)
            .body(session: session, operationId: operationId))
        XCTAssertNil(ForumCommand.editComment(commentId: parentId, revision: 1, text: String(repeating: "💡", count: 281))
            .body(session: session, operationId: operationId))
        XCTAssertNil(ForumCommand.editComment(commentId: parentId, revision: 1, text: "\u{0}")
            .body(session: session, operationId: operationId))
        XCTAssertNil(remove.body(session: session, operationId: "invalid"))
    }

    func testReactionsToggleSwitchAndClampKnownCounts() throws {
        XCTAssertEqual(ForumReaction.none.toggled(with: .like), .like)
        XCTAssertEqual(ForumReaction.like.toggled(with: .like), .none)
        XCTAssertEqual(ForumReaction.like.toggled(with: .dislike), .dislike)
        let counts = try XCTUnwrap(ForumReactionCounts(likes: 0, dislikes: 2))
        XCTAssertEqual(counts.applying(from: .like, to: .dislike).likes, 0)
        XCTAssertEqual(counts.applying(from: .dislike, to: .like).dislikes, 1)
        XCTAssertEqual(counts.applying(from: .dislike, to: .dislike), counts)
        XCTAssertNil(ForumReactionCounts(likes: nil, dislikes: 0))
        XCTAssertNil(ForumReactionCounts(likes: 0, dislikes: -1))
    }

    func testUncertainWriteOutcomesKeepIdentityUntilRetryAndConflictIsTerminal() throws {
        let now = Date(timeIntervalSince1970: 1_000)
        let intent = ForumCommand.reaction(target: .post(postId), reaction: .like)
        var pending = ForumPendingCommand(intent: intent, operationId: operationId)
        let originalBody = try object(pending.intent.body(session: session, operationId: pending.operationId))
        XCTAssertTrue(pending.retainAfterFailure(SignedTransportError.offline, now: now))
        XCTAssertTrue(pending.retainAfterFailure(
            SignedTransportError.refused(status: 408, code: nil, retryAfterSeconds: nil), now: now
        ))
        XCTAssertTrue(pending.retainAfterFailure(
            SignedTransportError.refused(status: 429, code: "forum.rate-limited", retryAfterSeconds: 30), now: now
        ))
        XCTAssertFalse(pending.canRetry(at: now.addingTimeInterval(29)))
        XCTAssertTrue(pending.canRetry(at: now.addingTimeInterval(30)))
        XCTAssertTrue(pending.retainAfterFailure(
            SignedTransportError.refused(status: 503, code: "forum.service-unavailable", retryAfterSeconds: nil), now: now
        ))
        XCTAssertEqual(pending.operationId, operationId)
        XCTAssertEqual(try object(intent.body(session: session, operationId: pending.operationId)), originalBody)
        for status in [400, 401, 404, 409] {
            XCTAssertFalse(pending.retainAfterFailure(
                SignedTransportError.refused(status: status, code: nil, retryAfterSeconds: nil), now: now
            ))
        }
    }

    func testForumReactionAndOwnerReadsUseTheBoundSignedRoutes() async throws {
        let postId = self.postId
        let replyId = self.replyId
        let parentId = self.parentId
        let topicResponse = try fixture("forum-owner-topic-response-valid")
        let topic = try XCTUnwrap(CommunityWire.decodeForumReceipt(topicResponse, wrapper: "topic"))
        let topicBody = String(decoding: topicResponse, as: UTF8.self)
        let sessions = StaticSessions(session: session)
        let exchange = ScriptedExchange { request in
            FakeIdentityBackend.reply(request) { request in
                switch request.url?.path {
                case "/v1/forum/posts/\(postId)/reaction":
                    return .init(200, """
                        {"target_type":"POST","target_id":"\(postId)","reaction":"LIKE","like_count":3,"dislike_count":1,"replayed":false}
                        """)
                case "/v1/forum/owner/read":
                    if FakeIdentityBackend.json(request)["topic_id"] as? String == topic.id {
                        return .init(200, topicBody)
                    }
                    return .init(200, """
                        {"comment_ids":["\(parentId)"]}
                        """)
                case "/v1/forum/owner/reactions":
                    return .init(200, """
                        {"post_reaction":"LIKE","comments":[{"comment_id":"\(replyId)","reaction":"DISLIKE"}]}
                        """)
                default:
                    return .init(404, "{}")
                }
            }
        }
        let transport = SignedRequestTransport(
            origin: "https://abastevo.com.br", signer: try IdentityVectors.signer(),
            identityStore: IosDeviceIdentityKeychainStore(useMemoryOnly: true),
            exchange: exchange, sessions: sessions, nowEpochSeconds: { 1_763_892_000 }
        )
        let client = CommunityWriteClient(transport: transport, exchange: exchange, sessions: sessions, photoBytes: { _ in nil })
        let scope = try XCTUnwrap(transport.ownerScope())
        let result = try await client.submitForumCommand(
            ForumPendingCommand(intent: .reaction(target: .post(postId), reaction: .like), operationId: operationId),
            expectedScope: scope
        )
        guard case .reaction(let reaction) = result else { return XCTFail("Expected a reaction receipt") }
        XCTAssertEqual(reaction.counts?.likes, 3)
        XCTAssertEqual(reaction.reaction, .like)

        let comments = try await client.forumOwnCommentIds(postId: postId, expectedScope: scope)
        XCTAssertEqual(comments, [parentId])
        let ownedTopic = try await client.forumOwnTopic(topicId: topic.id, expectedScope: scope)
        XCTAssertEqual(ownedTopic, topic)
        let owner = try await client.forumOwnReactions(postId: postId, expectedScope: scope)
        XCTAssertEqual(owner.postReaction, .like)
        XCTAssertEqual(owner.comments.first?.commentId, replyId)

        let forumRequests = exchange.requests.filter { $0.url?.path.hasPrefix("/v1/forum/") == true }
        XCTAssertEqual(forumRequests.count, 4)
        XCTAssertTrue(forumRequests.allSatisfy { $0.value(forHTTPHeaderField: "Signature") != nil })
        let sentReaction = try XCTUnwrap(forumRequests.first)
        XCTAssertEqual(FakeIdentityBackend.json(sentReaction)["operation_id"] as? String, operationId)
        XCTAssertEqual(FakeIdentityBackend.json(sentReaction)["family_id"] as? String, session.familyId)
        XCTAssertEqual(FakeIdentityBackend.json(sentReaction)["reaction"] as? String, "LIKE")

        sessions.set(nil)
        let before = exchange.requests.count
        do {
            _ = try await client.forumOwnReactions(postId: postId, expectedScope: scope)
            XCTFail("Logged out scope must refuse")
        } catch {
            XCTAssertEqual(exchange.requests.count, before)
        }
    }

    func testReplyOutboxKeepsParentAndOperationAcrossRestartAndRefusesOtherScopes() async throws {
        let expected = try object(fixture("forum-create-comment-valid"))
        let text = try XCTUnwrap(expected["body_text"] as? String)
        let draft = CommunityTextDraft(text: text, postId: postId, parentId: parentId)
        var expectedReply = [String: Any]()
        for (key, value) in expected {
            if let key = key as? String { expectedReply[key] = value }
        }
        expectedReply["parent_id"] = parentId
        XCTAssertEqual(try object(CommunityWire.forumCommentBody(session: session, operationId: operationId, draft: draft)),
                       expectedReply as NSDictionary)

        let home = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
        defer { try? FileManager.default.removeItem(at: home) }
        let store = CommunityOutboxStore(directory: home)
        let scope = "account:\(session.accountId):fingerprint"
        let origin = "https://abastevo.com.br"
        let item = CommunityOutboxItem(id: operationId, kind: .forumComment, ownerScope: scope,
                                       origin: origin, createdAtMillis: 0, text: draft)
        let failed = CommunityOutbox.retryLater(item, code: "forum.rate-limited", nowMillis: 0, notBeforeMillis: 2_000)
        store.save([failed])
        let reused = try XCTUnwrap(CommunityOutboxStore(directory: home).load().first)
        XCTAssertEqual(reused.id, operationId)
        XCTAssertEqual(reused.text?.parentId, parentId)
        XCTAssertNil(CommunityOutbox.nextEligible([reused], nowMillis: 1_999))

        let gateway = FakeCommunityGateway()
        for context in [
            CommunityDispatchContext(nowMillis: 2_000, ownerScope: "other", origin: origin, hasAccount: true),
            CommunityDispatchContext(nowMillis: 2_000, ownerScope: scope, origin: "https://other.example", hasAccount: true),
        ] {
            let refused = await CommunityOutboxDispatcher.run(reused, context: context, gateway: gateway)
            XCTAssertEqual(refused.lastCode, CommunityOutbox.ownerChangedCode)
        }
        XCTAssertTrue(gateway.callLog.isEmpty)
        store.purge()
        XCTAssertTrue(CommunityOutboxStore(directory: home).load().isEmpty)
    }
}

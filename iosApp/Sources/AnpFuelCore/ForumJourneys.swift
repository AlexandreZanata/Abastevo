import Foundation

public enum ForumReplySort: String, CaseIterable, Identifiable, Sendable {
    case relevant, recent, liked

    public var id: String { rawValue }
}

public struct ForumFeedFilters: Equatable, Sendable {
    public var category: String?
    public var kind: ForumPublicPost.Kind?
    public var uf: String?
    public var municipalityIbge: Int?
    public var fuelProduct: FuelProduct?
    public var stationId: String?

    public init(
        category: String? = nil,
        kind: ForumPublicPost.Kind? = nil,
        uf: String? = nil,
        municipalityIbge: Int? = nil,
        fuelProduct: FuelProduct? = nil,
        stationId: String? = nil
    ) {
        self.category = category
        self.kind = kind
        self.uf = uf
        self.municipalityIbge = municipalityIbge
        self.fuelProduct = fuelProduct
        self.stationId = stationId
    }

    public func query(limit: Int, cursor: String?) throws -> [URLQueryItem] {
        var query = try ForumReadWire.pageQuery(limit: limit, cursor: cursor)
        if let category { query.append(URLQueryItem(name: "category", value: category)) }
        if let kind { query.append(URLQueryItem(name: "kind", value: kind.rawValue)) }
        if let uf { query.append(URLQueryItem(name: "uf", value: uf)) }
        if let municipalityIbge {
            query.append(URLQueryItem(name: "municipality_ibge", value: String(municipalityIbge)))
        }
        if let fuelProduct { query.append(URLQueryItem(name: "fuel_product", value: fuelProduct.wireCode)) }
        if let stationId {
            guard CommunityWire.isUuid(stationId) else { throw HttpClientError.invalidRequest }
            query.append(URLQueryItem(name: "station_id", value: stationId))
        }
        return query
    }
}

public struct ForumFeedPage: Decodable, Sendable {
    public let items: [ForumPublicPost]
    public let nextCursor: String?
    public let snapshot: String
    public let fetchedAt: String
}

public struct ForumCommentsPage: Decodable, Sendable {
    public let items: [ForumPublicComment]
    public let nextCursor: String?
}

public enum ForumReadWire {
    public static let pageSize = 20
    public static let maxPageSize = 50

    public static func pageQuery(limit: Int, cursor: String?) throws -> [URLQueryItem] {
        guard (1...maxPageSize).contains(limit), validCursor(cursor) else {
            throw HttpClientError.invalidRequest
        }
        var query = [URLQueryItem(name: "limit", value: String(limit))]
        if let cursor { query.append(URLQueryItem(name: "cursor", value: cursor)) }
        return query
    }

    public static func validCursor(_ cursor: String?) -> Bool {
        guard let cursor else { return true }
        return !cursor.isEmpty && cursor.utf8.count <= 512
    }

    public static func filteredPosts(_ posts: [ForumPublicPost], query: String) -> [ForumPublicPost] {
        let text = query.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !text.isEmpty else { return posts }
        return posts.filter { post in
            post.title.localizedStandardContains(text) || post.excerpt?.localizedStandardContains(text) == true
        }
    }

    static func feed(_ data: Data, limit: Int) throws -> ForumFeedPage {
        let page = try AbastevoHttpClient.publicDecoder().decode(ForumFeedPage.self, from: data)
        guard page.items.count <= limit, validCursor(page.nextCursor),
              !page.snapshot.isEmpty, page.snapshot.utf8.count <= 128,
              AbastevoHttpClient.readIsoDate(page.fetchedAt) != nil,
              page.items.allSatisfy({ CommunityWire.isUuid($0.id) }) else {
            throw HttpClientError.invalidJson
        }
        return page
    }

    static func comments(
        _ data: Data, postId: String, parentId: String?, limit: Int
    ) throws -> ForumCommentsPage {
        let page = try AbastevoHttpClient.publicDecoder().decode(ForumCommentsPage.self, from: data)
        guard page.items.count <= limit, validCursor(page.nextCursor),
              page.items.allSatisfy({ $0.belongsTo(postId: postId, parentId: parentId) }) else {
            throw HttpClientError.invalidJson
        }
        return page
    }
}

/// One cursor chain. Keep earlier server order, deduplicate IDs, and reject cursor cycles.
public struct ForumPageAccumulator<Item: Identifiable> {
    public private(set) var items: [Item] = []
    public private(set) var nextCursor: String?
    private var usedCursors: Set<String> = []

    public init() {}

    public mutating func replace(_ items: [Item], nextCursor: String?) throws {
        guard ForumReadWire.validCursor(nextCursor) else { throw HttpClientError.invalidJson }
        var seen = Set<Item.ID>()
        self.items = items.filter { seen.insert($0.id).inserted }
        self.nextCursor = nextCursor
        usedCursors.removeAll()
    }

    public mutating func append(_ items: [Item], after cursor: String, nextCursor: String?) throws {
        guard self.nextCursor == cursor else { throw CancellationError() }
        guard ForumReadWire.validCursor(nextCursor),
              nextCursor != cursor,
              nextCursor.map({ !usedCursors.contains($0) }) ?? true else {
            throw HttpClientError.invalidJson
        }
        var seen = Set(self.items.map(\.id))
        self.items += items.filter { seen.insert($0.id).inserted }
        usedCursors.insert(cursor)
        self.nextCursor = nextCursor
    }

    public mutating func remove(_ id: Item.ID) {
        items.removeAll { $0.id == id }
    }
}

public enum ForumReaction: String, Decodable, Equatable, Sendable {
    case like = "LIKE"
    case dislike = "DISLIKE"
    case none = "NONE"

    public func toggled(with tapped: Self) -> Self {
        self == tapped ? .none : tapped
    }
}

public enum ForumReactionTarget: Hashable, Sendable {
    case post(String)
    case comment(String)

    public var id: String {
        switch self {
        case .post(let id), .comment(let id): return id
        }
    }

    var segment: String {
        switch self {
        case .post: return "posts"
        case .comment: return "comments"
        }
    }

    var wireKind: String {
        switch self {
        case .post: return "POST"
        case .comment: return "COMMENT"
        }
    }
}

public struct ForumReactionCounts: Equatable, Sendable {
    public let likes: Int64
    public let dislikes: Int64

    public init?(likes: Int64?, dislikes: Int64?) {
        guard let likes, let dislikes, likes >= 0, dislikes >= 0 else { return nil }
        self.likes = likes
        self.dislikes = dislikes
    }

    public func applying(from: ForumReaction, to: ForumReaction) -> Self {
        guard from != to else { return self }
        var likes = self.likes
        var dislikes = self.dislikes
        if from == .like { likes = max(0, likes - 1) }
        if from == .dislike { dislikes = max(0, dislikes - 1) }
        if to == .like, likes < Int64.max { likes += 1 }
        if to == .dislike, dislikes < Int64.max { dislikes += 1 }
        return Self(likes: likes, dislikes: dislikes) ?? self
    }
}

public struct ForumReactionReceipt: Decodable, Sendable {
    public let targetType: String
    public let targetId: String
    public let reaction: ForumReaction
    public let likeCount: Int64
    public let dislikeCount: Int64
    public let replayed: Bool

    public var counts: ForumReactionCounts? { ForumReactionCounts(likes: likeCount, dislikes: dislikeCount) }
}

public struct ForumOwnReactions: Decodable, Sendable {
    public struct Comment: Decodable, Sendable {
        public let commentId: String
        public let reaction: ForumReaction
    }

    public let postReaction: ForumReaction
    public let comments: [Comment]
}

public enum ForumCommand: Equatable, Sendable {
    case editComment(commentId: String, revision: Int64, text: String)
    case removeComment(commentId: String, revision: Int64)
    case reaction(target: ForumReactionTarget, reaction: ForumReaction)

    var target: ForumReactionTarget {
        switch self {
        case .editComment(let id, _, _), .removeComment(let id, _): return .comment(id)
        case .reaction(let target, _): return target
        }
    }

    var path: String {
        let suffix: String
        switch self {
        case .editComment: suffix = "edit"
        case .removeComment: suffix = "remove"
        case .reaction: suffix = "reaction"
        }
        return "/v1/forum/" + target.segment + "/" + target.id + "/" + suffix
    }

    func body(session: PortableAuth.Session, operationId: String) -> Data? {
        guard CommunityWire.isUuid(target.id), CommunityWire.isUuid(operationId) else { return nil }
        var body: [String: Any] = [
            "family_id": session.familyId,
            "access_token": session.accessToken,
            "operation_id": operationId,
        ]
        switch self {
        case .editComment(_, let revision, let text):
            let normalized = PortableText.normalize(text)
            guard revision >= 1, PortableText.isValidComment(normalized),
                  !normalized.unicodeScalars.contains("\u{0}") else { return nil }
            body["expected_revision"] = NSNumber(value: revision)
            body["body_text"] = normalized
        case .removeComment(_, let revision):
            guard revision >= 1 else { return nil }
            body["expected_revision"] = NSNumber(value: revision)
        case .reaction(_, let reaction):
            body["reaction"] = reaction.rawValue
        }
        return DeviceIdentityWire.encode(body)
    }
}

public enum ForumCommandResult: Sendable {
    case comment(ForumWriteReceipt)
    case reaction(ForumReactionReceipt)
}

/// A screen-local operation ID survives uncertain results until the user retries or leaves the scope.
public struct ForumPendingCommand: Equatable, Sendable {
    public let intent: ForumCommand
    public let operationId: String
    public private(set) var retryNotBefore: Date?

    public init(intent: ForumCommand, operationId: String = UUID().uuidString.lowercased()) {
        self.intent = intent
        self.operationId = operationId
    }

    public func canRetry(at now: Date) -> Bool {
        retryNotBefore.map { $0 <= now } ?? true
    }

    /// Conflicts and permanent refusals are terminal. Uncertain outcomes retain the same operation ID.
    public mutating func retainAfterFailure(_ error: Error, now: Date) -> Bool {
        if let error = error as? SignedTransportError {
            switch error {
            case .refused(let status, _, let retryAfter)
                where status == 408 || status == 429 || status >= 500:
                if let retryAfter {
                    retryNotBefore = now.addingTimeInterval(TimeInterval(min(max(0, retryAfter), 86_400)))
                }
                return true
            case .offline, .oversizedResponse, .invalidResponse, .identityUnavailable, .signingUnavailable:
                return true
            default:
                return false
            }
        }
        if let error = error as? CommunityWriteError {
            return error == .invalidReceipt
        }
        return false
    }
}

extension CommunityWriteClient {
    public func submitForumCommand(
        _ pending: ForumPendingCommand, expectedScope: String
    ) async throws -> ForumCommandResult {
        let command = pending.intent
        let response = try await ownerPost(
            path: command.path, expectedScope: expectedScope, requiresDeviceProof: true
        ) { session in
            command.body(session: session, operationId: pending.operationId)
        }
        switch command {
        case .editComment(_, let revision, _), .removeComment(_, let revision):
            guard let receipt = CommunityWire.decodeForumReceipt(response.body, wrapper: "comment"),
                  receipt.id.caseInsensitiveCompare(command.target.id) == .orderedSame,
                  receipt.revision > revision else {
                throw CommunityWriteError.invalidReceipt
            }
            return .comment(receipt)
        case .reaction(let target, let reaction):
            guard let receipt = try? AbastevoHttpClient.publicDecoder().decode(ForumReactionReceipt.self, from: response.body),
                  receipt.targetId.caseInsensitiveCompare(target.id) == .orderedSame,
                  receipt.targetType == target.wireKind, receipt.reaction == reaction, receipt.counts != nil else {
                throw CommunityWriteError.invalidReceipt
            }
            return .reaction(receipt)
        }
    }

    public func forumOwnTopic(topicId: String, expectedScope: String) async throws -> ForumWriteReceipt {
        guard CommunityWire.isUuid(topicId) else { throw CommunityWriteError.invalid("forum.bad-request") }
        let response = try await ownerPost(
            path: "/v1/forum/owner/read", expectedScope: expectedScope, requiresDeviceProof: true
        ) { session in
            DeviceIdentityWire.encode([
                "family_id": session.familyId, "access_token": session.accessToken, "topic_id": topicId,
            ])
        }
        guard let receipt = CommunityWire.decodeForumReceipt(response.body, wrapper: "topic"),
              receipt.id.caseInsensitiveCompare(topicId) == .orderedSame else {
            throw CommunityWriteError.invalidReceipt
        }
        return receipt
    }

    public func forumOwnCommentIds(postId: String, expectedScope: String) async throws -> Set<String> {
        guard CommunityWire.isUuid(postId) else { throw CommunityWriteError.invalid("forum.bad-request") }
        let response = try await ownerPost(
            path: "/v1/forum/owner/read", expectedScope: expectedScope, requiresDeviceProof: true
        ) { session in
            Self.forumPostOwnerBody(session: session, postId: postId)
        }
        return try Self.decodeForumOwnerCommentIds(response.body, postId: postId)
    }

    static func decodeForumOwnerCommentIds(_ data: Data, postId: String) throws -> Set<String> {
        struct OwnerComments: Decodable {
            struct Comment: Decodable {
                let id: String
                let postId: String
            }
            let comments: [Comment]
        }
        guard let owned = try? AbastevoHttpClient.publicDecoder().decode(OwnerComments.self, from: data),
              owned.comments.count <= 100,
              owned.comments.allSatisfy({
                  CommunityWire.isUuid($0.id) && $0.postId.caseInsensitiveCompare(postId) == .orderedSame
              }) else {
            throw CommunityWriteError.invalidReceipt
        }
        let ids = Set(owned.comments.map { $0.id.lowercased() })
        guard ids.count == owned.comments.count else { throw CommunityWriteError.invalidReceipt }
        return ids
    }

    public func forumOwnReactions(postId: String, expectedScope: String) async throws -> ForumOwnReactions {
        guard CommunityWire.isUuid(postId) else { throw CommunityWriteError.invalid("forum.bad-request") }
        let response = try await ownerPost(
            path: "/v1/forum/owner/reactions", expectedScope: expectedScope, requiresDeviceProof: true
        ) { session in
            Self.forumPostOwnerBody(session: session, postId: postId)
        }
        let own = try AbastevoHttpClient.publicDecoder().decode(ForumOwnReactions.self, from: response.body)
        guard own.comments.count <= 1_000,
              own.comments.allSatisfy({ CommunityWire.isUuid($0.commentId) }),
              Set(own.comments.map { $0.commentId.lowercased() }).count == own.comments.count else {
            throw CommunityWriteError.invalidReceipt
        }
        return own
    }

    static func forumPostOwnerBody(session: PortableAuth.Session, postId: String) -> Data? {
        DeviceIdentityWire.encode([
            "family_id": session.familyId, "access_token": session.accessToken, "post_id": postId,
        ])
    }
}

import Foundation

public enum ForumReplySort: String, CaseIterable, Identifiable, Sendable {
    case relevant, recent, liked

    public var id: String { rawValue }
}

/// Forum subject fuels (backend `forum/domain/topics.go`, Android `SubmitForumTopicUseCase.VALID_FUELS`).
/// These are forum codes, not the price wire codes of `FuelProduct`.
public enum ForumFuelProduct: String, CaseIterable, Identifiable, Hashable, Sendable {
    case gasolineRegular = "GASOLINE_REGULAR"
    case gasolinePremium = "GASOLINE_PREMIUM"
    case ethanol = "ETHANOL"
    case dieselS10 = "DIESEL_S10"
    case dieselS500 = "DIESEL_S500"
    case glp = "GLP"
    case gnv = "GNV"

    public var id: String { rawValue }

    /// The matching price product, used only for its localized name.
    public var fuel: FuelProduct {
        switch self {
        case .gasolineRegular: return .gasolineRegular
        case .gasolinePremium: return .gasolinePremium
        case .ethanol: return .ethanol
        case .dieselS10: return .dieselS10
        case .dieselS500: return .dieselS500
        case .glp: return .lpgP13
        case .gnv: return .cng
        }
    }
}

public enum ForumFilterIssue: Error, Equatable, Sendable {
    case state
    case municipality
    case station
}

/// Region rules shared by feed filters and topic writes (backend `ValidateTopicRegion`, Android `validateRegion`).
public enum ForumRegionRules {
    public static let statePrefixes: [String: Int] = [
        "RO": 11, "AC": 12, "AM": 13, "RR": 14, "PA": 15, "AP": 16, "TO": 17,
        "MA": 21, "PI": 22, "CE": 23, "RN": 24, "PB": 25, "PE": 26, "AL": 27, "SE": 28, "BA": 29,
        "MG": 31, "ES": 32, "RJ": 33, "SP": 35,
        "PR": 41, "SC": 42, "RS": 43,
        "MS": 50, "MT": 51, "GO": 52, "DF": 53,
    ]

    public static var states: [String] { statePrefixes.keys.sorted() }

    public static func issue(uf: String?, municipalityIbge: Int?) -> ForumFilterIssue? {
        var prefix: Int?
        if let uf {
            guard let known = statePrefixes[uf] else { return .state }
            prefix = known
        }
        if let municipalityIbge {
            guard (1_000_000...9_999_999).contains(municipalityIbge) else { return .municipality }
            if let prefix, municipalityIbge / 100_000 != prefix { return .municipality }
        }
        return nil
    }

    /// Seven ASCII digits, or nil for an empty field.
    public static func municipality(_ text: String) throws -> Int? {
        let trimmed = text.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmed.isEmpty else { return nil }
        guard trimmed.utf8.count == 7, trimmed.utf8.allSatisfy({ (48...57).contains($0) }), let code = Int(trimmed) else {
            throw ForumFilterIssue.municipality
        }
        return code
    }
}

public struct ForumFeedFilters: Hashable, Sendable {
    public var category: String?
    public var kind: ForumPublicPost.Kind?
    public var uf: String?
    public var municipalityIbge: Int?
    public var fuelProduct: ForumFuelProduct?
    public var stationId: String?

    public init(
        category: String? = nil,
        kind: ForumPublicPost.Kind? = nil,
        uf: String? = nil,
        municipalityIbge: Int? = nil,
        fuelProduct: ForumFuelProduct? = nil,
        stationId: String? = nil
    ) {
        self.category = category
        self.kind = kind
        self.uf = uf
        self.municipalityIbge = municipalityIbge
        self.fuelProduct = fuelProduct
        self.stationId = stationId
    }

    /// Android `ForumFilterDialog.applyFilters`: optional seven-digit IBGE code inside the chosen state
    /// and an optional station UUID, sent in lowercase.
    public static func parse(
        category: String?, kind: ForumPublicPost.Kind?, uf: String?, municipality: String,
        fuelProduct: ForumFuelProduct?, station: String
    ) -> Result<ForumFeedFilters, ForumFilterIssue> {
        let code: Int?
        do {
            code = try ForumRegionRules.municipality(municipality)
        } catch {
            return .failure(.municipality)
        }
        let trimmedStation = station.trimmingCharacters(in: .whitespacesAndNewlines)
        let filters = ForumFeedFilters(
            category: category, kind: kind, uf: uf, municipalityIbge: code, fuelProduct: fuelProduct,
            stationId: trimmedStation.isEmpty ? nil : trimmedStation.lowercased()
        )
        if let issue = filters.issue { return .failure(issue) }
        return .success(filters)
    }

    /// Kind, region, fuel and station narrow the feed beyond its category.
    public var narrowsFeed: Bool {
        kind != nil || uf != nil || municipalityIbge != nil || fuelProduct != nil || stationId != nil
    }

    public var issue: ForumFilterIssue? {
        if let regionIssue = ForumRegionRules.issue(uf: uf, municipalityIbge: municipalityIbge) { return regionIssue }
        if let stationId, !CommunityWire.isUuid(stationId) { return .station }
        return nil
    }

    public func query(limit: Int, cursor: String?) throws -> [URLQueryItem] {
        guard issue == nil else { throw HttpClientError.invalidRequest }
        var query = try ForumReadWire.pageQuery(limit: limit, cursor: cursor)
        if let category { query.append(URLQueryItem(name: "category", value: category)) }
        if let kind { query.append(URLQueryItem(name: "kind", value: kind.rawValue)) }
        if let uf { query.append(URLQueryItem(name: "uf", value: uf)) }
        if let municipalityIbge {
            query.append(URLQueryItem(name: "municipality_ibge", value: String(municipalityIbge)))
        }
        if let fuelProduct { query.append(URLQueryItem(name: "fuel_product", value: fuelProduct.rawValue)) }
        if let stationId { query.append(URLQueryItem(name: "station_id", value: stationId.lowercased())) }
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

/// Editable topic fields (backend `EditTopicRequest`). The public alias is fixed at creation.
public struct ForumTopicDraft: Equatable, Sendable {
    public var category: String
    public var title: String
    public var bodyText: String
    public var regionUf: String?
    public var regionMunicipalityIbge: Int?
    public var fuelProduct: ForumFuelProduct?
    public var stationId: String?

    public init(
        category: String, title: String, bodyText: String, regionUf: String? = nil,
        regionMunicipalityIbge: Int? = nil, fuelProduct: ForumFuelProduct? = nil, stationId: String? = nil
    ) {
        self.category = category
        self.title = title
        self.bodyText = bodyText
        self.regionUf = regionUf
        self.regionMunicipalityIbge = regionMunicipalityIbge
        self.fuelProduct = fuelProduct
        self.stationId = stationId
    }

    public var isValid: Bool {
        !category.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
            && CommunityWire.isValidTopic(title: title, body: bodyText)
            && ForumRegionRules.issue(uf: regionUf, municipalityIbge: regionMunicipalityIbge) == nil
            && (stationId.map(CommunityWire.isUuid) ?? true)
    }
}

/// The owner's private projection of an authored topic (`/v1/forum/owner/read` with `topic_id`).
public struct ForumOwnerTopic: Equatable, Identifiable, Sendable {
    public let receipt: ForumWriteReceipt
    public let draft: ForumTopicDraft

    public var id: String { receipt.id }

    public init(receipt: ForumWriteReceipt, draft: ForumTopicDraft) {
        self.receipt = receipt
        self.draft = draft
    }
}

public enum ForumCommand: Equatable, Sendable {
    case editComment(commentId: String, revision: Int64, text: String)
    case removeComment(commentId: String, revision: Int64)
    case reaction(target: ForumReactionTarget, reaction: ForumReaction)
    case editTopic(topicId: String, revision: Int64, draft: ForumTopicDraft)
    case removeTopic(topicId: String, revision: Int64)

    var target: ForumReactionTarget {
        switch self {
        case .editComment(let id, _, _), .removeComment(let id, _): return .comment(id)
        case .editTopic(let id, _, _), .removeTopic(let id, _): return .post(id)
        case .reaction(let target, _): return target
        }
    }

    var path: String {
        switch self {
        case .editComment: return "/v1/forum/comments/" + target.id + "/edit"
        case .removeComment: return "/v1/forum/comments/" + target.id + "/remove"
        case .editTopic: return "/v1/forum/topics/" + target.id + "/edit"
        case .removeTopic: return "/v1/forum/topics/" + target.id + "/remove"
        case .reaction: return "/v1/forum/" + target.segment + "/" + target.id + "/reaction"
        }
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
        case .removeComment(_, let revision), .removeTopic(_, let revision):
            guard revision >= 1 else { return nil }
            body["expected_revision"] = NSNumber(value: revision)
        case .editTopic(_, let revision, let draft):
            guard revision >= 1, draft.isValid else { return nil }
            body["expected_revision"] = NSNumber(value: revision)
            body["category"] = draft.category.trimmingCharacters(in: .whitespacesAndNewlines)
            body["title"] = draft.title.trimmingCharacters(in: .whitespacesAndNewlines)
            body["body_text"] = draft.bodyText.trimmingCharacters(in: .whitespacesAndNewlines)
            if let uf = draft.regionUf { body["region_uf"] = uf }
            if let code = draft.regionMunicipalityIbge { body["region_municipality_ibge"] = NSNumber(value: code) }
            if let fuel = draft.fuelProduct { body["fuel_product"] = fuel.rawValue }
            if let station = draft.stationId { body["station_id"] = station.lowercased() }
        case .reaction(_, let reaction):
            body["reaction"] = reaction.rawValue
        }
        return DeviceIdentityWire.encode(body)
    }
}

public enum ForumCommandResult: Sendable {
    case comment(ForumWriteReceipt)
    case reaction(ForumReactionReceipt)
    case topic(ForumWriteReceipt)
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
        case .editTopic(_, let revision, _), .removeTopic(_, let revision):
            guard let receipt = CommunityWire.decodeForumReceipt(response.body, wrapper: "topic"),
                  receipt.id.caseInsensitiveCompare(command.target.id) == .orderedSame,
                  receipt.revision > revision else {
                throw CommunityWriteError.invalidReceipt
            }
            if case .removeTopic = command, receipt.publicationState != "REMOVED" {
                throw CommunityWriteError.invalidReceipt
            }
            return .topic(receipt)
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

    /// The editable fields of an owned topic. Android `ForumTopicFormViewModel.loadTopic`.
    public func forumOwnTopicDetail(topicId: String, expectedScope: String) async throws -> ForumOwnerTopic {
        guard CommunityWire.isUuid(topicId) else { throw CommunityWriteError.invalid("forum.bad-request") }
        let response = try await ownerPost(
            path: "/v1/forum/owner/read", expectedScope: expectedScope, requiresDeviceProof: true
        ) { session in
            DeviceIdentityWire.encode([
                "family_id": session.familyId, "access_token": session.accessToken, "topic_id": topicId,
            ])
        }
        return try Self.decodeForumOwnerTopic(response.body, topicId: topicId)
    }

    static func decodeForumOwnerTopic(_ data: Data, topicId: String) throws -> ForumOwnerTopic {
        struct Envelope: Decodable {
            struct Region: Decodable {
                let uf: String?
                let municipalityIbge: Int?
            }
            struct Topic: Decodable {
                let id: String
                let kind: String
                let category: String
                let title: String
                let bodyText: String
                let region: Region?
                let fuelProduct: String?
                let stationId: String?
            }
            let topic: Topic
        }
        guard let receipt = CommunityWire.decodeForumReceipt(data, wrapper: "topic"),
              receipt.id.caseInsensitiveCompare(topicId) == .orderedSame,
              let topic = try? AbastevoHttpClient.publicDecoder().decode(Envelope.self, from: data).topic,
              topic.kind == ForumPublicPost.Kind.topic.rawValue else {
            throw CommunityWriteError.invalidReceipt
        }
        let uf = topic.region?.uf.flatMap { $0.isEmpty ? nil : $0 }
        let fuel: ForumFuelProduct?
        if let raw = topic.fuelProduct, !raw.isEmpty {
            guard let known = ForumFuelProduct(rawValue: raw) else { throw CommunityWriteError.invalidReceipt }
            fuel = known
        } else {
            fuel = nil
        }
        let draft = ForumTopicDraft(
            category: topic.category, title: topic.title, bodyText: topic.bodyText, regionUf: uf,
            regionMunicipalityIbge: topic.region?.municipalityIbge, fuelProduct: fuel,
            stationId: topic.stationId.flatMap { $0.isEmpty ? nil : $0.lowercased() }
        )
        guard ForumRegionRules.issue(uf: draft.regionUf, municipalityIbge: draft.regionMunicipalityIbge) == nil,
              draft.stationId.map(CommunityWire.isUuid) ?? true else {
            throw CommunityWriteError.invalidReceipt
        }
        return ForumOwnerTopic(receipt: receipt, draft: draft)
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

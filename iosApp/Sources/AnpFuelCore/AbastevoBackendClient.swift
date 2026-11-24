import Foundation
#if canImport(FoundationNetworking)
import FoundationNetworking
#endif

private struct PublicItemsResponse<Item: Decodable>: Decodable {
    let items: [Item]
}

private struct ForumPublicDetailResponse: Decodable {
    let post: ForumPublicDetail
}

private struct PublicStationCommentsResponse: Decodable {
    let comments: [PublicStationComment]
}

public struct ForumPublicCategory: Decodable, Identifiable, Sendable {
    public var id: String { code }
    public let code: String
    public let name: String
}

public struct ForumPublicPost: Decodable, Identifiable, Sendable {
    public enum Kind: String, CaseIterable, Decodable, Equatable, Sendable {
        case editorial = "EDITORIAL"
        case news = "NEWS"
        case topic = "TOPIC"
        case bulletin = "BULLETIN"
    }

    public let id: String
    public let kind: Kind
    public let category: String
    public let title: String
    public let excerpt: String?
    public let publicAlias: String?
    public let firstPublishedAt: String
}

public struct ForumPublicDetail: Decodable, Sendable {
    public struct Source: Decodable, Sendable {
        public let name: String
        public let url: String
    }

    public let id: String
    public let bodyText: String?
    public let source: Source?
    public let commentsEnabled: Bool?
    public let likeCount: Int64?
    public let dislikeCount: Int64?

    public var reactionCounts: ForumReactionCounts? {
        ForumReactionCounts(likes: likeCount, dislikes: dislikeCount)
    }
}

public struct ForumPublicComment: Decodable, Identifiable, Sendable {
    public let id: String
    public let postId: String
    public let parentId: String?
    public let publicAlias: String
    public let bodyText: String
    public let createdAt: String
    public let depth: Int
    public let revision: Int64
    public let replyCount: Int
    public let likeCount: Int64?
    public let dislikeCount: Int64?
    public let topReply: ForumPublicReplyPreview?

    public var reactionCounts: ForumReactionCounts? {
        ForumReactionCounts(likes: likeCount, dislikes: dislikeCount)
    }

    func belongsTo(postId: String, parentId: String?) -> Bool {
        guard CommunityWire.isUuid(id),
              self.postId.caseInsensitiveCompare(postId) == .orderedSame,
              revision >= 1, replyCount >= 0 else {
            return false
        }
        if let parentId {
            return depth == 1 && self.parentId?.caseInsensitiveCompare(parentId) == .orderedSame
                && topReply == nil
        }
        guard depth == 0, self.parentId == nil else { return false }
        return topReply?.comment.belongsTo(postId: postId, parentId: id) ?? true
    }
}

/// Ranked pages can include one reply preview. Replies cannot themselves contain a preview.
public struct ForumPublicReplyPreview: Decodable, Sendable {
    public let id: String
    public let postId: String
    public let parentId: String
    public let publicAlias: String
    public let bodyText: String
    public let createdAt: String
    public let depth: Int
    public let revision: Int64
    public let replyCount: Int
    public let likeCount: Int64?
    public let dislikeCount: Int64?

    public var comment: ForumPublicComment {
        ForumPublicComment(
            id: id, postId: postId, parentId: parentId, publicAlias: publicAlias,
            bodyText: bodyText, createdAt: createdAt, depth: depth, revision: revision,
            replyCount: replyCount, likeCount: likeCount, dislikeCount: dislikeCount, topReply: nil
        )
    }
}

public struct PublicContributor: Decodable, Sendable {
    public let displayName: String
    public let contributorSince: String
    public let roles: [String]?
}

public struct PublicStationComment: Decodable, Identifiable, Sendable {
    public let id: String
    public let stationId: String
    public let product: String
    public let publicAlias: String
    public let text: String
    public let createdAt: String
    public let parentId: String?
    public let depth: Int?
    public let revision: Int64?
}

/// Explicit backend environment and URLSession HTTP client synchronized with `main`.
/// Zero postfix unwrap operators anywhere in this file.
public enum ApiEnvironment: String, CaseIterable, Codable, Sendable {
    case preview = "https://api.anpfuel.example.invalid"
    case staging = "https://abastevo.com.br"
    case localProof = "http://127.0.0.1:18080"

    public var origin: String { rawValue }

    public var localizedLabel: String {
        switch self {
        case .staging: return String(localized: "origin.official", bundle: CoreLocalization.bundle)
        case .preview: return String(localized: "origin.offline", bundle: CoreLocalization.bundle)
        case .localProof: return String(localized: "origin.local", bundle: CoreLocalization.bundle)
        }
    }

    public static func validateOrigin(_ value: String, allowLocalProof: Bool = false) -> Bool {
        let trimmed = value.trimmingCharacters(in: .whitespacesAndNewlines)
        if trimmed.isEmpty { return false }
        if allowLocalProof && trimmed == localProof.rawValue { return true }
        guard let url = URL(string: trimmed),
              let scheme = url.scheme?.lowercased(),
              scheme == "https",
              let host = url.host,
              !host.isEmpty,
              url.user == nil,
              url.password == nil,
              url.query == nil,
              url.fragment == nil else {
            return false
        }
        let path = url.path
        return path.isEmpty || path == "/"
    }

    public func join(_ path: String) -> URL? {
        guard path.hasPrefix("/") else { return nil }
        return URL(string: origin + path)
    }
}

public enum HttpClientError: Error, Equatable {
    case invalidRequest
    case unavailable
    case httpStatus(Int, String?)
    case oversizedResponse
    case invalidJson
}

public struct StationDirectoryPage: Sendable {
    public let stations: [StationRecord]
    public let nextCursor: String?
}

public struct OfficialStationPrice: Identifiable, Sendable {
    public var id: FuelProduct { product }
    public let product: FuelProduct
    public let amountMilli: Int64
    public let collectedOn: String
    public let surveyStart: String
    public let surveyEnd: String
}

/// Status and body of one account-bridge reply.
struct AuthHttpReply {
    let status: Int
    let body: Data
}

/// Synchronous exchange behind the account bridge; nil means no HTTP response arrived.
protocol AuthHttpExchange {
    func send(_ request: URLRequest) -> AuthHttpReply?
}

/// URLSession transport for public reads and the synchronous account bridge used by `AuthFlow`.
/// The bridge blocks its caller, so the shell runs it through `AuthBlockingWork`, never on the main actor.
/// Signed device requests live in `SignedRequestTransport`.
public final class AbastevoHttpClient: AuthAccountApi {
    static let communityFeedPath = "/v1/community/feed"
    private static let maxResponseBytes = 512 * 1024
    private static let requestTimeout: TimeInterval = 20.0

    public var environment: ApiEnvironment
    let session: URLSession
    /// Replaces URLSession for the account bridge in tests; nil sends over `session`.
    var authExchange: AuthHttpExchange?

    public init(environment: ApiEnvironment = .staging) {
        self.environment = environment
        let config = URLSessionConfiguration.ephemeral
        config.timeoutIntervalForRequest = Self.requestTimeout
        config.timeoutIntervalForResource = Self.requestTimeout
        config.httpCookieStorage = nil
        config.httpShouldSetCookies = false
        config.urlCache = nil
        self.session = URLSession(configuration: config)
    }

    // MARK: - AuthAccountApi (synchronous bridge over URLSession for AuthFlow)

    public func requestCode(
        email: String,
        purpose: PortableAuth.CodePurpose,
        locale: String
    ) -> AuthApiResult<EmptyAuthValue> {
        let res = postAuthSync(
            path: "/v1/accounts/email/codes",
            body: ["email": email, "purpose": purpose.wire, "locale": locale]
        )
        switch res {
        case .ok: return .ok(EmptyAuthValue())
        case .err(let verdict): return .err(verdict)
        }
    }

    public func signUp(name: String, email: String, code: String) -> AuthApiResult<AuthIssuedKey> {
        let res = postAuthSync(path: "/v1/accounts/keys/signup", body: ["name": name, "email": email, "code": code])
        return issuedKey(res, flag: "created")
    }

    public func recover(email: String, code: String) -> AuthApiResult<AuthIssuedKey> {
        let res = postAuthSync(path: "/v1/accounts/recovery", body: ["email": email, "code": code])
        return issuedKey(res, flag: "sessions_revoked")
    }

    public func profile(session: PortableAuth.Session) -> AuthApiResult<PortableAuth.Profile> {
        let res = postAuthSync(path: "/v1/accounts/profile", body: sessionProof(session))
        return profileResult(res, accountId: session.accountId)
    }

    public func updateName(session: PortableAuth.Session, name: String) -> AuthApiResult<PortableAuth.Profile> {
        var body = sessionProof(session)
        body["name"] = name
        let res = postAuthSync(path: "/v1/accounts/profile/name", body: body)
        return profileResult(res, accountId: session.accountId)
    }

    public func setRecoveryEmail(
        session: PortableAuth.Session,
        email: String,
        code: String
    ) -> AuthApiResult<PortableAuth.Profile> {
        var body = sessionProof(session)
        body["email"] = email
        body["code"] = code
        let res = postAuthSync(path: "/v1/accounts/recovery-email", body: body)
        return profileResult(res, accountId: session.accountId)
    }

    public func createKeyAccount(username: String) -> AuthApiResult<AuthKeyIssued> {
        let res = postAuthSync(path: "/v1/accounts/keys", body: ["username": username])
        switch res {
        case .err(let verdict): return .err(verdict)
        case .ok(let json):
            guard let name = json["username"] as? String,
                  let key = json["account_key"] as? String,
                  !name.isEmpty, !key.isEmpty else {
                return .err(PortableAuth.unavailable)
            }
            return .ok(AuthKeyIssued(username: name, accountKey: key))
        }
    }

    public func loginWithKey(accountKey: String) -> AuthApiResult<AuthKeyLogin> {
        let res = postAuthSync(path: "/v1/accounts/keys/login", body: ["account_key": accountKey])
        switch res {
        case .err(let verdict): return .err(verdict)
        case .ok(let json):
            guard let sessionDict = json["session"] as? [String: Any],
                  let parsed = parseSession(sessionDict),
                  let accountDict = json["account"] as? [String: Any] else {
                return .err(PortableAuth.unavailable)
            }
            let publicName = accountDict["public_name"] as? String ?? ""
            let username = publicName.isEmpty ? accountDict["public_alias"] as? String ?? "" : publicName
            guard !username.isEmpty else {
                return .err(PortableAuth.unavailable)
            }
            return .ok(AuthKeyLogin(session: parsed, username: username))
        }
    }

    public func refresh(familyId: String, refreshToken: String) -> AuthApiResult<PortableAuth.Session> {
        let res = postAuthSync(
            path: "/v1/accounts/sessions/refresh",
            body: ["family_id": familyId, "refresh_token": refreshToken]
        )
        switch res {
        case .err(let verdict): return .err(verdict)
        case .ok(let json):
            guard let sessionDict = json["session"] as? [String: Any],
                  let parsed = parseSession(sessionDict) else {
                return .err(PortableAuth.unavailable)
            }
            return .ok(parsed)
        }
    }

    public func revokeAll(familyId: String, accessToken: String) -> AuthApiResult<EmptyAuthValue> {
        let res = postAuthSync(
            path: "/v1/accounts/sessions/revoke",
            body: ["family_id": familyId, "access_token": accessToken]
        )
        switch res {
        case .ok: return .ok(EmptyAuthValue())
        case .err(let verdict): return .err(verdict)
        }
    }

    public func linkProvider(
        session: PortableAuth.Session,
        provider: String,
        idToken: String,
        nonce: String
    ) -> AuthApiResult<EmptyAuthValue> {
        let res = postAuthSync(
            path: "/v1/accounts/providers/link",
            body: [
                "family_id": session.familyId,
                "access_token": session.accessToken,
                "provider": provider,
                "id_token": idToken,
                "nonce": nonce,
            ]
        )
        switch res {
        case .ok: return .ok(EmptyAuthValue())
        case .err(let verdict): return .err(verdict)
        }
    }

    public func deleteAccount(session: PortableAuth.Session) -> AuthApiResult<EmptyAuthValue> {
        let res = postAuthSync(
            path: "/v1/accounts/deletion",
            body: [
                "family_id": session.familyId,
                "access_token": session.accessToken,
            ]
        )
        switch res {
        case .ok(let json):
            guard json["status"] as? String == "deleted",
                  json["erasure_requested"] as? Bool == true else {
                return .err(PortableAuth.unavailable)
            }
            return .ok(EmptyAuthValue())
        case .err(let verdict): return .err(verdict)
        }
    }

    public func loginWithProvider(
        provider: String,
        idToken: String,
        nonce: String,
        authorizationCode: String?
    ) -> AuthApiResult<AuthConsumeOk> {
        let res = postAuthSync(
            path: AccountProviderLogin.path,
            body: AccountProviderLogin.body(
                provider: provider,
                idToken: idToken,
                rawNonce: nonce,
                authorizationCode: authorizationCode
            )
        )
        switch res {
        case .err(let verdict): return .err(verdict)
        case .ok(let json):
            guard let sessionDict = json["session"] as? [String: Any],
                  let parsed = parseSession(sessionDict) else {
                return .err(PortableAuth.unavailable)
            }
            let created = json["created"] as? Bool ?? false
            return .ok(AuthConsumeOk(session: parsed, created: created))
        }
    }

    // MARK: - Public Reads & Community / Signed Writes (async)

    public func fetchStations(in municipality: Municipality, after cursor: String? = nil) async throws -> StationDirectoryPage {
        var query = [
            URLQueryItem(name: "state", value: municipality.state.rawValue),
            URLQueryItem(name: "municipality_code", value: municipality.ibgeCode),
            URLQueryItem(name: "limit", value: "50"),
        ]
        if let cursor {
            query.append(URLQueryItem(name: "cursor", value: cursor))
        }
        let data = try await publicRead(path: "/v1/stations", query: query)
        return try Self.decodeStations(data, in: municipality)
    }

    public func fetchOfficialStationPrices(stationId: String) async throws -> [OfficialStationPrice] {
        guard UUID(uuidString: stationId) != nil else { throw HttpClientError.invalidRequest }
        let data = try await publicRead(path: "/v1/stations/\(stationId)/prices")
        return try Self.decodeOfficialPrices(data, stationId: stationId)
    }

    public func fetchCommunityFeed(
        state: BrazilianState,
        municipalityCode: String,
        fuel: FuelProduct,
        sort: FeedSortOrder
    ) async -> [CommunityPriceObservation]? {
        let now = Int64(Date().timeIntervalSince1970)
        guard let items = try? await fetchCommunityFeedItems(
            state: state, municipalityCode: municipalityCode, fuel: fuel, sort: sort
        ) else { return nil }
        return items.filter { $0.isLive(nowEpochSeconds: now) }.map(\.observation)
    }

    public func fetchForumCategories() async throws -> [ForumPublicCategory] {
        let data = try await publicRead(path: "/v1/forum/categories")
        return try Self.decodePublicPage(data, as: ForumPublicCategory.self)
    }

    public func fetchForumPosts(category: String?) async throws -> [ForumPublicPost] {
        try await fetchForumFeed(filters: ForumFeedFilters(category: category)).items
    }

    public func fetchForumFeed(
        filters: ForumFeedFilters, limit: Int = ForumReadWire.pageSize, cursor: String? = nil
    ) async throws -> ForumFeedPage {
        let query = try filters.query(limit: limit, cursor: cursor)
        let data = try await publicRead(path: "/v1/forum/posts", query: query, preserveHttpErrors: true)
        return try ForumReadWire.feed(data, limit: limit)
    }

    public func fetchForumPost(id: String) async throws -> ForumPublicDetail {
        guard UUID(uuidString: id) != nil else { throw HttpClientError.invalidRequest }
        let data = try await publicRead(path: "/v1/forum/posts/\(id)", preserveHttpErrors: true)
        let post = try Self.publicDecoder().decode(ForumPublicDetailResponse.self, from: data).post
        guard post.id.caseInsensitiveCompare(id) == .orderedSame else { throw HttpClientError.invalidJson }
        return post
    }

    public func fetchForumComments(postId: String) async throws -> [ForumPublicComment] {
        try await fetchForumCommentsPage(postId: postId, sort: nil).items
    }

    public func fetchForumCommentsPage(
        postId: String, sort: ForumReplySort?, limit: Int = ForumReadWire.pageSize, cursor: String? = nil
    ) async throws -> ForumCommentsPage {
        guard UUID(uuidString: postId) != nil else { throw HttpClientError.invalidRequest }
        var query = try ForumReadWire.pageQuery(limit: limit, cursor: cursor)
        if let sort { query.append(URLQueryItem(name: "sort", value: sort.rawValue)) }
        let data = try await publicRead(
            path: "/v1/forum/posts/\(postId)/comments", query: query, preserveHttpErrors: true
        )
        return try ForumReadWire.comments(data, postId: postId, parentId: nil, limit: limit)
    }

    public func fetchForumReplies(
        postId: String, parentId: String, limit: Int = ForumReadWire.pageSize, cursor: String? = nil
    ) async throws -> ForumCommentsPage {
        guard UUID(uuidString: postId) != nil, UUID(uuidString: parentId) != nil else {
            throw HttpClientError.invalidRequest
        }
        let query = try ForumReadWire.pageQuery(limit: limit, cursor: cursor)
        let data = try await publicRead(
            path: "/v1/forum/comments/\(parentId)/replies", query: query, preserveHttpErrors: true
        )
        return try ForumReadWire.comments(data, postId: postId, parentId: parentId, limit: limit)
    }

    public func fetchContributors() async throws -> [PublicContributor] {
        let data = try await publicRead(path: "/v1/contributors")
        return try Self.decodePublicPage(data, as: PublicContributor.self)
            .filter { !$0.displayName.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty }
    }

    public func fetchStationComments(stationId: String, fuel: FuelProduct) async throws -> [PublicStationComment] {
        guard UUID(uuidString: stationId) != nil else { throw HttpClientError.invalidRequest }
        let query = [
            URLQueryItem(name: "station_id", value: stationId),
            URLQueryItem(name: "product", value: fuel.wireCode),
        ]
        let data = try await publicRead(path: "/v1/feedback/comments", query: query)
        return try Self.decodePublicStationComments(data, stationId: stationId, fuel: fuel)
    }

    static func decodePublicPage<Item: Decodable>(_ data: Data, as type: Item.Type) throws -> [Item] {
        return try publicDecoder().decode(PublicItemsResponse<Item>.self, from: data).items
    }

    static func decodePublicStationComments(_ data: Data, stationId: String, fuel: FuelProduct) throws -> [PublicStationComment] {
        let comments = try publicDecoder().decode(PublicStationCommentsResponse.self, from: data).comments
        guard comments.allSatisfy({ $0.stationId == stationId && $0.product == fuel.wireCode }) else {
            throw HttpClientError.invalidJson
        }
        return comments
    }

    func publicRead(
        path: String, query: [URLQueryItem] = [], maxBytes: Int? = nil, preserveHttpErrors: Bool = false
    ) async throws -> Data {
        guard let origin = environment.join(path),
              var components = URLComponents(url: origin, resolvingAgainstBaseURL: false) else {
            throw HttpClientError.invalidRequest
        }
        components.queryItems = query.isEmpty ? nil : query
        guard let url = components.url else { throw HttpClientError.invalidRequest }
        return try await performGet(url: url, maxBytes: maxBytes ?? Self.maxResponseBytes, preserveHttpErrors: preserveHttpErrors)
    }

    private func performGet(url: URL, maxBytes: Int, preserveHttpErrors: Bool) async throws -> Data {
        var request = URLRequest(url: url)
        request.httpMethod = "GET"
        request.setValue("application/json", forHTTPHeaderField: "Accept")
        let (data, response) = try await session.data(for: request)
        if data.count > maxBytes { throw HttpClientError.oversizedResponse }
        guard let http = response as? HTTPURLResponse else { throw HttpClientError.unavailable }
        guard (200..<300).contains(http.statusCode) else {
            if preserveHttpErrors { throw HttpClientError.httpStatus(http.statusCode, parseErrorCode(data)) }
            throw HttpClientError.unavailable
        }
        return data
    }

    private enum AuthHttpResult {
        case ok([String: Any])
        case err(String)
    }

    private final class AuthHttpResponse: @unchecked Sendable {
        private let lock = NSLock()
        private var reply: AuthHttpReply?

        func store(_ reply: AuthHttpReply) {
            lock.lock()
            defer { lock.unlock() }
            self.reply = reply
        }

        func load() -> AuthHttpReply? {
            lock.lock()
            defer { lock.unlock() }
            return reply
        }
    }

    private func postAuthSync(path: String, body: [String: Any]) -> AuthHttpResult {
        guard let url = environment.join(path),
              let bodyData = try? JSONSerialization.data(withJSONObject: body) else {
            return .err(PortableAuth.unavailable)
        }
        var request = URLRequest(url: url)
        request.httpMethod = "POST"
        request.httpBody = bodyData
        request.setValue("application/json", forHTTPHeaderField: "Accept")
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")

        let received: AuthHttpReply?
        if let exchange = authExchange {
            received = exchange.send(request)
        } else {
            received = sendOverSession(request)
        }
        guard let reply = received, reply.body.count <= Self.maxResponseBytes else {
            return .err(PortableAuth.unavailable)
        }
        if reply.status < 200 || reply.status >= 300 {
            let code = parseErrorCode(reply.body) ?? ""
            let verdict = code.hasPrefix("account.") ? String(code.dropFirst("account.".count)) : PortableAuth.unavailable
            return .err(verdict)
        }
        guard let json = try? JSONSerialization.jsonObject(with: reply.body) as? [String: Any] else {
            return .err(PortableAuth.unavailable)
        }
        return .ok(json)
    }

    private func sendOverSession(_ request: URLRequest) -> AuthHttpReply? {
        let semaphore = DispatchSemaphore(value: 0)
        let authResponse = AuthHttpResponse()

        let task = session.dataTask(with: request) { data, response, error in
            defer { semaphore.signal() }
            guard error == nil, let data = data, let http = response as? HTTPURLResponse else {
                return
            }
            authResponse.store(AuthHttpReply(status: http.statusCode, body: data))
        }
        task.resume()
        guard semaphore.wait(timeout: .now() + Self.requestTimeout + 1.0) == .success else {
            task.cancel()
            return nil
        }
        return authResponse.load()
    }

    private func sessionProof(_ session: PortableAuth.Session) -> [String: Any] {
        return ["family_id": session.familyId, "access_token": session.accessToken]
    }

    private func issuedKey(_ res: AuthHttpResult, flag: String) -> AuthApiResult<AuthIssuedKey> {
        switch res {
        case .err(let verdict):
            return .err(verdict)
        case .ok(let json):
            guard json[flag] as? Bool == true,
                  let sessionDict = json["session"] as? [String: Any],
                  let parsed = parseSession(sessionDict),
                  let account = json["account"] as? [String: Any],
                  account["account_id"] as? String == parsed.accountId,
                  let publicName = account["public_name"] as? String,
                  let key = json["account_key"] as? String,
                  !publicName.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty,
                  !key.isEmpty else {
                return .err(PortableAuth.unavailable)
            }
            return .ok(AuthIssuedKey(session: parsed, publicName: publicName, accountKey: key))
        }
    }

    private func profileResult(_ res: AuthHttpResult, accountId: String) -> AuthApiResult<PortableAuth.Profile> {
        switch res {
        case .err(let verdict):
            return .err(verdict)
        case .ok(let json):
            guard let profile = Self.parseProfile(json, accountId: accountId) else {
                return .err(PortableAuth.unavailable)
            }
            return .ok(profile)
        }
    }

    /// The reply must name the session owner; the recovery hint and the name may be null.
    static func parseProfile(_ json: [String: Any], accountId: String) -> PortableAuth.Profile? {
        guard json["account_id"] as? String == accountId,
              let publicName = json["public_name"] as? String,
              !publicName.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty,
              let complete = json["profile_complete"] as? Bool,
              let hasKey = json["has_account_key"] as? Bool,
              let name = nullableString(json["name"]),
              let hint = nullableString(json["recovery_email_hint"]) else {
            return nil
        }
        let providers: Set<String>
        if let list = json["providers"] as? [String] {
            providers = Set(list)
        } else if json["providers"] == nil || json["providers"] is NSNull {
            providers = []
        } else {
            return nil
        }
        return PortableAuth.Profile(
            accountId: accountId,
            publicName: publicName,
            name: name,
            profileComplete: complete,
            recoveryEmailHint: hint,
            hasAccountKey: hasKey,
            providers: providers
        )
    }

    /// `.some(nil)` for JSON null or an absent field, nil for any value that is not a string.
    private static func nullableString(_ value: Any?) -> String?? {
        guard let present = value, !(present is NSNull) else {
            return .some(nil)
        }
        guard let text = present as? String else {
            return nil
        }
        return .some(text)
    }

    private func parseErrorCode(_ data: Data) -> String? {
        guard let json = try? JSONSerialization.jsonObject(with: data) as? [String: Any],
              let errDict = json["error"] as? [String: Any],
              let code = errDict["code"] as? String else {
            return nil
        }
        return code
    }

    private func parseSession(_ dict: [String: Any]) -> PortableAuth.Session? {
        guard let familyId = dict["family_id"] as? String,
              let accountId = dict["account_id"] as? String,
              let accessToken = dict["access_token"] as? String,
              let refreshToken = dict["refresh_token"] as? String,
              let accessStr = dict["access_expires_at"] as? String,
              let absStr = dict["absolute_expires_at"] as? String,
              let accessDate = parseIsoDate(accessStr),
              let absDate = parseIsoDate(absStr) else {
            return nil
        }
        let accessExp = Int64(accessDate.timeIntervalSince1970)
        let absExp = Int64(absDate.timeIntervalSince1970)
        if familyId.isEmpty || accountId.isEmpty || accessToken.isEmpty || refreshToken.isEmpty || accessExp <= 0 || absExp <= accessExp {
            return nil
        }
        return PortableAuth.Session(
            familyId: familyId,
            accountId: accountId,
            accessToken: accessToken,
            refreshToken: refreshToken,
            accessExpiresAt: accessExp,
            absoluteExpiresAt: absExp
        )
    }

    private func parseIsoDate(_ raw: String) -> Date? {
        Self.readIsoDate(raw)
    }

    static func readIsoDate(_ raw: String) -> Date? {
        let formatter = ISO8601DateFormatter()
        formatter.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        if let d = formatter.date(from: raw) { return d }
        formatter.formatOptions = [.withInternetDateTime]
        return formatter.date(from: raw)
    }
}

extension AbastevoHttpClient {
    static func decodeStations(_ data: Data, in municipality: Municipality) throws -> StationDirectoryPage {
        let page = try publicDecoder().decode(StationPageWire.self, from: data)
        let stations = try page.items.map { row in
            guard UUID(uuidString: row.stationId) != nil, !row.displayName.isEmpty,
                  row.municipalityCode == municipality.ibgeCode, row.state == municipality.state.rawValue else {
                throw HttpClientError.invalidJson
            }
            let coordinates = row.coordinates.flatMap { point in
                (-90...90).contains(point.lat) && (-180...180).contains(point.lon) ? point : nil
            }
            let tier = CoordinatePinTier(rawValue: row.locationQuality)
            let pin = tier?.allowsMapPin == true ? coordinates : nil
            let address = [row.address?.street, row.address?.number].compactMap { $0 }.joined(separator: ", ")
            return StationRecord(
                id: row.stationId,
                cnpj: row.cnpjNormalized ?? "",
                name: row.displayName,
                brand: "",
                address: address,
                neighborhood: row.address?.district ?? "",
                municipalityCode: municipality.ibgeCode,
                latitude: pin?.lat ?? 0,
                longitude: pin?.lon ?? 0,
                pinTier: pin == nil ? .cityCentroid : tier ?? .cityCentroid,
                officialPricesMilli: [:],
                officialDateLabel: "",
                communityObservations: []
            )
        }
        return StationDirectoryPage(stations: stations, nextCursor: page.nextCursor)
    }

    static func decodeOfficialPrices(_ data: Data, stationId: String) throws -> [OfficialStationPrice] {
        let page = try publicDecoder().decode(PublicItems<PriceGroupWire>.self, from: data)
        return try page.items.compactMap { row in
            guard row.stationId == stationId else { throw HttpClientError.invalidJson }
            guard let official = row.official else { return nil }
            guard let product = FuelProduct.fromWireCode(row.fuelProduct), product.unitWire == row.unit,
                  row.condition.kind == "STANDARD", official.source == "ANP", official.currency == "BRL",
                  official.amountMilliBrl > 0,
                  official.surveyWeek.start <= official.collectedOn, official.collectedOn <= official.surveyWeek.end else {
                throw HttpClientError.invalidJson
            }
            return OfficialStationPrice(
                product: product,
                amountMilli: official.amountMilliBrl,
                collectedOn: official.collectedOn,
                surveyStart: official.surveyWeek.start,
                surveyEnd: official.surveyWeek.end
            )
        }
    }

    static func decodeCommunityFeed(
        _ data: Data,
        state: BrazilianState,
        municipalityCode: String,
        fuel: FuelProduct,
        now: Date = Date()
    ) throws -> [CommunityPriceObservation] {
        let nowSeconds = Int64(now.timeIntervalSince1970)
        return try decodeCommunityFeedItems(data, state: state, municipalityCode: municipalityCode, fuel: fuel)
            .filter { $0.isLive(nowEpochSeconds: nowSeconds) }
            .map(\.observation)
    }

    static func publicDecoder() -> JSONDecoder {
        let decoder = JSONDecoder()
        decoder.keyDecodingStrategy = .convertFromSnakeCase
        return decoder
    }
}

private struct PublicItems<Item: Decodable>: Decodable {
    let items: [Item]
}

private struct StationPageWire: Decodable {
    let items: [StationWire]
    let nextCursor: String?

    private enum CodingKeys: String, CodingKey {
        case items
        case nextCursor
    }

    init(from decoder: Decoder) throws {
        let values = try decoder.container(keyedBy: CodingKeys.self)
        // The Go directory marshals its empty slice as null.
        guard values.contains(.items) else { throw HttpClientError.invalidJson }
        items = try values.decodeIfPresent([StationWire].self, forKey: .items) ?? []
        nextCursor = try values.decodeIfPresent(String.self, forKey: .nextCursor)
    }
}

private struct StationWire: Decodable {
    struct Address: Decodable {
        let street: String?
        let number: String?
        let district: String?
    }

    struct Coordinates: Decodable {
        let lat: Double
        let lon: Double
    }

    let stationId: String
    let displayName: String
    let cnpjNormalized: String?
    let municipalityCode: String?
    let state: String?
    let address: Address?
    let coordinates: Coordinates?
    let locationQuality: String
}

private struct PriceGroupWire: Decodable {
    struct Condition: Decodable {
        let kind: String
    }

    struct Official: Decodable {
        struct SurveyWeek: Decodable {
            let start: String
            let end: String
        }

        let source: String
        let currency: String
        let amountMilliBrl: Int64
        let collectedOn: String
        let surveyWeek: SurveyWeek
    }

    let stationId: String
    let fuelProduct: String
    let unit: String
    let condition: Condition
    let official: Official?
}

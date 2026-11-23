import Foundation
#if canImport(FoundationNetworking)
import FoundationNetworking
#endif

/// Explicit backend environment and URLSession HTTP client synchronized with `main`.
/// Zero postfix unwrap operators anywhere in this file.
public enum ApiEnvironment: String, CaseIterable, Codable, Sendable {
    case preview = "https://api.anpfuel.example.invalid"
    case staging = "https://abastevo.com.br"
    case localProof = "http://127.0.0.1:18080"

    public var origin: String { rawValue }

    public var labelPtBr: String {
        switch self {
        case .staging: return "Oficial / Staging (abastevo.com.br)"
        case .preview: return "Offline / Preview (RFC 2606)"
        case .localProof: return "Local Proof (127.0.0.1:18080)"
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

/// Real URLSession transport for Abastevo backend APIs, including RFC 9421 / P10-T03 signed requests.
public final class AbastevoHttpClient: AuthAccountApi {
    private static let maxResponseBytes = 512 * 1024
    private static let requestTimeout: TimeInterval = 20.0

    public var environment: ApiEnvironment
    private let session: URLSession
    private let proofSigner: IosDeviceProofSigner
    private let lock = NSLock()
    private var registeredFingerprint: String?
    private var registeredContributorId: String?

    public init(
        environment: ApiEnvironment = .staging,
        proofSigner: IosDeviceProofSigner = IosDeviceProofSigner()
    ) {
        self.environment = environment
        self.proofSigner = proofSigner
        let config = URLSessionConfiguration.ephemeral
        config.timeoutIntervalForRequest = Self.requestTimeout
        config.timeoutIntervalForResource = Self.requestTimeout
        config.httpCookieStorage = nil
        config.httpShouldSetCookies = false
        config.urlCache = nil
        self.session = URLSession(configuration: config)
    }

    // MARK: - AuthAccountApi (synchronous bridge over URLSession for AuthFlow)

    public func requestCode(email: String) -> AuthApiResult<EmptyAuthValue> {
        let res = postAuthSync(path: "/v1/accounts/email/codes", body: ["email": email])
        switch res {
        case .ok: return .ok(EmptyAuthValue())
        case .err(let verdict): return .err(verdict)
        }
    }

    public func consumeCode(email: String, code: String) -> AuthApiResult<AuthConsumeOk> {
        let res = postAuthSync(path: "/v1/accounts/email/consume", body: ["email": email, "code": code])
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
                  let accountDict = json["account"] as? [String: Any],
                  let username = accountDict["public_alias"] as? String,
                  !username.isEmpty else {
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
        case .ok: return .ok(EmptyAuthValue())
        case .err(let verdict): return .err(verdict)
        }
    }

    // MARK: - Public Reads & Community / Signed Writes (async)

    public func fetchCommunityFeed(
        state: BrazilianState,
        municipalityCode: String,
        fuel: FuelProduct,
        sort: FeedSortOrder
    ) async -> [CommunityPriceObservation]? {
        let sortWire = sort == .cheapest ? "cheapest" : "recent"
        let path = "/v1/community/feed?state=\(state.rawValue)&municipality_code=\(municipalityCode)&fuel_product=\(fuel.wireCode)&sort=\(sortWire)&limit=20"
        guard let url = environment.join(path),
              let data = try? await performGet(url: url),
              let json = try? JSONSerialization.jsonObject(with: data) as? [String: Any],
              let items = json["items"] as? [[String: Any]] else {
            return nil
        }
        return items.compactMap { item -> CommunityPriceObservation? in
            guard let id = item["id"] as? String,
                  let stationId = item["station_id"] as? String,
                  let amount = (item["amount_milli_brl"] as? NSNumber)?.int64Value else {
                return nil
            }
            let stationName = item["station_name"] as? String ?? "Posto"
            let conditionRaw = item["condition_kind"] as? String ?? "STANDARD"
            let condition = PriceConditionKind(rawValue: conditionRaw) ?? .standard
            let confirmations = (item["confirmations"] as? NSNumber)?.intValue ?? 0
            let disputes = (item["disputes"] as? NSNumber)?.intValue ?? 0
            let helpful = (item["helpful_count"] as? NSNumber)?.intValue ?? 0
            let notHelpful = (item["not_helpful_count"] as? NSNumber)?.intValue ?? 0
            let observedAt = item["observed_at"] as? String ?? "Agora"
            let hasPhoto = item["has_photo_evidence"] as? Bool ?? false
            return CommunityPriceObservation(
                id: id,
                stationId: stationId,
                stationName: stationName,
                municipalityCode: municipalityCode,
                product: fuel,
                amountMilli: amount,
                condition: condition,
                confirmations: confirmations,
                disputes: disputes,
                helpfulCount: helpful,
                notHelpfulCount: notHelpful,
                observedAtLabel: observedAt,
                observedEpochSeconds: Int64(Date().timeIntervalSince1970),
                hasPhotoEvidence: hasPhoto
            )
        }
    }

    public func fetchForumCategories() async -> [ForumCategoryItem]? {
        guard let url = environment.join("/v1/forum/categories"),
              let data = try? await performGet(url: url),
              let json = try? JSONSerialization.jsonObject(with: data) as? [String: Any],
              let categories = json["categories"] as? [[String: Any]] else {
            return nil
        }
        return categories.compactMap { cat -> ForumCategoryItem? in
            guard let slug = cat["slug"] as? String,
                  let title = cat["title"] as? String else { return nil }
            let id = cat["id"] as? String ?? slug
            let count = (cat["topic_count"] as? NSNumber)?.intValue ?? 0
            return ForumCategoryItem(id: id, slug: slug, title: title, postCount: count)
        }
    }

    public func signedPost(
        path: String,
        idempotencyKey: String,
        payload: [String: Any]
    ) async -> Result<[String: Any], HttpClientError> {
        guard path.hasPrefix("/v1/"),
              let url = environment.join(path),
              let authority = url.host,
              let bodyData = try? JSONSerialization.data(withJSONObject: payload),
              let jwk = proofSigner.ensurePublicJwk() else {
            return .failure(.invalidRequest)
        }
        let authorityWithPort = url.port.map { "\(authority):\($0)" } ?? authority
        let challengeResult = await requestChallenge(fingerprint: jwk.fingerprint, purpose: PortableAnonymousProof.purposeSign)
        guard case .success(let nonce) = challengeResult else {
            return .failure(.unavailable)
        }
        let now = Int64(Date().timeIntervalSince1970)
        let digestHeader = IosDeviceProofSigner.sha512ContentDigest(bodyData)
        let lines = PortableAnonymousProof.buildBaseLines(
            method: "POST",
            authority: authorityWithPort,
            path: path,
            query: "",
            contentType: "application/json",
            contentDigest: digestHeader,
            created: now,
            expires: now + 120,
            keyId: jwk.fingerprint,
            nonce: nonce
        )
        guard let sig = proofSigner.signProofLines(lines) else {
            return .failure(.invalidRequest)
        }
        var request = URLRequest(url: url)
        request.httpMethod = "POST"
        request.httpBody = bodyData
        request.setValue("application/json", forHTTPHeaderField: "Accept")
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        request.setValue("no-store", forHTTPHeaderField: "Cache-Control")
        request.setValue(idempotencyKey, forHTTPHeaderField: "Idempotency-Key")
        request.setValue(sig, forHTTPHeaderField: "Signature")
        request.setValue(String(now), forHTTPHeaderField: "Signature-Created")
        request.setValue(String(now + 120), forHTTPHeaderField: "Signature-Expires")
        request.setValue(jwk.fingerprint, forHTTPHeaderField: "Signature-Keyid")
        request.setValue(nonce, forHTTPHeaderField: "Signature-Nonce")
        do {
            let (data, response) = try await session.data(for: request)
            if data.count > Self.maxResponseBytes { return .failure(.oversizedResponse) }
            guard let http = response as? HTTPURLResponse else { return .failure(.unavailable) }
            if http.statusCode < 200 || http.statusCode >= 300 {
                let code = parseErrorCode(data)
                return .failure(.httpStatus(http.statusCode, code))
            }
            guard let json = try JSONSerialization.jsonObject(with: data) as? [String: Any] else {
                return .failure(.invalidJson)
            }
            return .success(json)
        } catch {
            return .failure(.unavailable)
        }
    }

    public func syncTrainingConsentGrant(record: TrainingConsentRecord, commandId: String) async -> Bool {
        let payload: [String: Any] = [
            "terms_version": record.termsVersion,
            "terms_digest": record.termsDigest,
            "revision": record.revision,
        ]
        let res = await signedPost(path: "/v1/privacy/training-consent/grant", idempotencyKey: commandId, payload: payload)
        if case .success = res { return true }
        return false
    }

    public func syncTrainingConsentRevoke(revision: Int64, commandId: String) async -> Bool {
        let payload: [String: Any] = ["revision": revision]
        let res = await signedPost(path: "/v1/privacy/training-consent/revoke", idempotencyKey: commandId, payload: payload)
        if case .success = res { return true }
        return false
    }

    private func requestChallenge(fingerprint: String, purpose: String) async -> Result<String, HttpClientError> {
        guard let url = environment.join("/v1/identity/challenges"),
              let bodyData = try? JSONSerialization.data(withJSONObject: ["fingerprint": fingerprint, "purpose": purpose]) else {
            return .failure(.invalidRequest)
        }
        var request = URLRequest(url: url)
        request.httpMethod = "POST"
        request.httpBody = bodyData
        request.setValue("application/json", forHTTPHeaderField: "Accept")
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        do {
            let (data, response) = try await session.data(for: request)
            guard let http = response as? HTTPURLResponse, http.statusCode >= 200 && http.statusCode < 300,
                  let json = try JSONSerialization.jsonObject(with: data) as? [String: Any],
                  let nonce = json["nonce"] as? String,
                  PortableAnonymousProof.splitNonce(nonce) != nil else {
                return .failure(.unavailable)
            }
            return .success(nonce)
        } catch {
            return .failure(.unavailable)
        }
    }

    private func performGet(url: URL) async throws -> Data {
        var request = URLRequest(url: url)
        request.httpMethod = "GET"
        request.setValue("application/json", forHTTPHeaderField: "Accept")
        let (data, response) = try await session.data(for: request)
        if data.count > Self.maxResponseBytes { throw HttpClientError.oversizedResponse }
        guard let http = response as? HTTPURLResponse, http.statusCode >= 200 && http.statusCode < 300 else {
            throw HttpClientError.unavailable
        }
        return data
    }

    private func postAuthSync(path: String, body: [String: Any]) -> AuthApiResult<[String: Any]> {
        guard let url = environment.join(path),
              let bodyData = try? JSONSerialization.data(withJSONObject: body) else {
            return .err(PortableAuth.unavailable)
        }
        var request = URLRequest(url: url)
        request.httpMethod = "POST"
        request.httpBody = bodyData
        request.setValue("application/json", forHTTPHeaderField: "Accept")
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")

        let semaphore = DispatchSemaphore(value: 0)
        var capturedResult: AuthApiResult<[String: Any]> = .err(PortableAuth.unavailable)

        let task = session.dataTask(with: request) { [weak self] data, response, error in
            defer { semaphore.signal() }
            if error != nil {
                capturedResult = .err(PortableAuth.unavailable)
                return
            }
            guard let data = data, data.count <= Self.maxResponseBytes,
                  let http = response as? HTTPURLResponse else {
                capturedResult = .err(PortableAuth.unavailable)
                return
            }
            if http.statusCode < 200 || http.statusCode >= 300 {
                let code = self?.parseErrorCode(data) ?? ""
                let verdict = code.hasPrefix("account.") ? String(code.dropFirst("account.".count)) : PortableAuth.unavailable
                capturedResult = .err(verdict)
                return
            }
            guard let json = try? JSONSerialization.jsonObject(with: data) as? [String: Any] else {
                capturedResult = .err(PortableAuth.unavailable)
                return
            }
            capturedResult = .ok(json)
        }
        task.resume()
        _ = semaphore.wait(timeout: .now() + Self.requestTimeout + 1.0)
        return capturedResult
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
        let formatter = ISO8601DateFormatter()
        formatter.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        if let d = formatter.date(from: raw) { return d }
        formatter.formatOptions = [.withInternetDateTime]
        return formatter.date(from: raw)
    }
}

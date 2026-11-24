import Foundation
#if canImport(FoundationNetworking)
import FoundationNetworking
#endif

/// Request proof profile verified by `backend/internal/modules/identity/adapters/auth`.
///
/// The covered base holds `@method`, `@authority`, `@path` and `@query`,
/// then `content-type` and `content-digest` when the request carries a
/// body, then `created`, `expires`, `keyid` and `nonce`. The device signs
/// the SHA-512 digest of the newline-joined base with its P-256 key and
/// sends the raw r||s signature as unpadded base64url. The authority is the
/// canonical host of the selected origin. Every signed request spends one
/// fresh SIGN challenge nonce. Mirrors Android `PhotoProofTransport`.
/// Zero postfix unwrap operators anywhere in this file.
public enum SignedRequestProfile {
    public static let headerSignature = "Signature"
    public static let headerCreated = "Signature-Created"
    public static let headerExpires = "Signature-Expires"
    public static let headerKeyId = "Signature-Keyid"
    public static let headerNonce = "Signature-Nonce"
    public static let headerIdempotencyKey = "Idempotency-Key"
    public static let jsonContentType = "application/json"
    public static let proofLifetimeSeconds: Int64 = 120

    /// True when the exchange failed before any request bytes could reach the server:
    /// no network, no route, name or TLS failure. Timeouts and dropped connections are not proof.
    public static func provesRequestNotSent(_ error: Error) -> Bool {
        guard let failure = error as? URLError else { return false }
        switch failure.code {
        case .notConnectedToInternet, .cannotFindHost, .cannotConnectToHost, .dnsLookupFailed,
             .internationalRoamingOff, .dataNotAllowed, .callIsActive, .badURL, .unsupportedURL,
             .appTransportSecurityRequiresSecureConnection, .secureConnectionFailed,
             .serverCertificateUntrusted, .serverCertificateHasBadDate, .serverCertificateNotYetValid,
             .serverCertificateHasUnknownRoot, .clientCertificateRejected, .clientCertificateRequired:
            return true
        default:
            return false
        }
    }

    /// Host plus explicit port, matching Android `URI.rawAuthority`.
    public static func authority(origin: String) -> String? {
        let trimmed = origin.trimmingCharacters(in: .whitespacesAndNewlines)
        guard let url = URL(string: trimmed),
              let scheme = url.scheme?.lowercased(),
              scheme == "https" || scheme == "http",
              let host = url.host,
              !host.isEmpty,
              url.user == nil,
              url.password == nil else {
            return nil
        }
        if let port = url.port {
            return "\(host):\(port)"
        }
        return host
    }

    /// `"sha-512=:<standard base64>:"` over the exact transmitted bytes.
    public static func contentDigest(_ body: Data) -> String {
        return IosDeviceProofSigner.sha512ContentDigest(body)
    }

    /// Builds the covered base lines. An empty or absent body omits the content lines,
    /// matching the backend rule `r.ContentLength != 0`.
    public static func baseLines(
        method: String,
        authority: String,
        path: String,
        query: String,
        body: Data?,
        created: Int64,
        expires: Int64,
        keyId: String,
        nonce: String
    ) -> [String] {
        var covered: Data?
        if let body, !body.isEmpty {
            covered = body
        }
        return PortableAnonymousProof.buildBaseLines(
            method: method.uppercased(),
            authority: authority,
            path: path,
            query: query,
            contentType: covered == nil ? nil : jsonContentType,
            contentDigest: covered.map { Self.contentDigest($0) },
            created: created,
            expires: expires,
            keyId: keyId,
            nonce: nonce
        )
    }

    public static func headers(
        signature: String,
        created: Int64,
        expires: Int64,
        keyId: String,
        nonce: String
    ) -> [String: String] {
        return [
            headerSignature: signature,
            headerCreated: String(created),
            headerExpires: String(expires),
            headerKeyId: keyId,
            headerNonce: nonce,
        ]
    }

    /// A signed path starts with `/v1/`, carries no query or fragment and only visible ASCII.
    public static func isSignablePath(_ path: String) -> Bool {
        guard path.hasPrefix("/v1/"), !path.contains("?"), !path.contains("#") else {
            return false
        }
        return path.unicodeScalars.allSatisfy { $0.value > 0x20 && $0.value < 0x7F }
    }

    /// A signed query is empty or `name=value` pairs of unreserved characters, sent and signed verbatim.
    public static func isSignableQuery(_ query: String) -> Bool {
        if query.isEmpty { return true }
        if query.count > 2048 { return false }
        return query.unicodeScalars.allSatisfy { scalar in
            let value = scalar.value
            let isAlnum = (value >= 0x30 && value <= 0x39) || (value >= 0x41 && value <= 0x5A) || (value >= 0x61 && value <= 0x7A)
            return isAlnum || scalar == "=" || scalar == "&" || scalar == "-" || scalar == "_" || scalar == "." || scalar == "~" || scalar == "%"
        }
    }

    /// Stable backend error code from `{"error":{"code":...}}`; anything else is dropped.
    public static func errorCode(from data: Data) -> String? {
        guard data.count <= 65_536,
              let json = try? JSONSerialization.jsonObject(with: data) as? [String: Any],
              let error = json["error"] as? [String: Any],
              let code = error["code"] as? String,
              isStableCode(code) else {
            return nil
        }
        return code
    }

    public static func isStableCode(_ code: String) -> Bool {
        guard let first = code.unicodeScalars.first, first.value >= 0x61, first.value <= 0x7A, code.count <= 64 else {
            return false
        }
        return code.unicodeScalars.allSatisfy { scalar in
            let value = scalar.value
            return (value >= 0x61 && value <= 0x7A) || (value >= 0x30 && value <= 0x39) || scalar == "." || scalar == "-"
        }
    }

    /// `Retry-After` as delta seconds or an HTTP date; malformed values return nil.
    public static func retryAfterSeconds(_ value: String?, nowEpochSeconds: Int64) -> Int64? {
        guard let raw = value?.trimmingCharacters(in: .whitespaces), !raw.isEmpty, raw.count <= 128 else {
            return nil
        }
        if raw.unicodeScalars.allSatisfy({ $0.value >= 0x30 && $0.value <= 0x39 }) {
            return Int64(raw) ?? Int64.max
        }
        let formatter = DateFormatter()
        formatter.locale = Locale(identifier: "en_US_POSIX")
        formatter.timeZone = TimeZone(secondsFromGMT: 0)
        formatter.dateFormat = "EEE, dd MMM yyyy HH:mm:ss zzz"
        guard let date = formatter.date(from: raw) else { return nil }
        return max(0, Int64(date.timeIntervalSince1970) - nowEpochSeconds)
    }
}

/// Device key operations used by the signed transport. The private key never leaves the conformer.
public protocol DeviceProofSigning: AnyObject, Sendable {
    func ensurePublicJwk() -> IosDeviceProofSigner.PublicJwk?
    func signProofLines(_ lines: [String]) -> String?
    func signBindingStatement(_ statement: String) -> String?
}

extension IosDeviceProofSigner: DeviceProofSigning, @unchecked Sendable {}

/// Account session access for signed writes: a local read without network and a live session that may refresh.
public protocol SignedSessionSource: Sendable {
    func currentAccountId() -> String?
    func liveSession() async -> PortableAuth.Session?
}

/// One HTTP exchange. Implementations never follow redirects for signed requests.
public protocol SignedHttpExchange: Sendable {
    func send(_ request: URLRequest) async throws -> (Data, HTTPURLResponse)
}

public enum SignedTransportError: Error, Equatable, Sendable {
    case invalidRequest
    case originUnavailable
    case identityUnavailable
    case signingUnavailable
    case sessionRequired
    case offline
    case oversizedResponse
    case invalidResponse
    case refused(status: Int, code: String?, retryAfterSeconds: Int64?)
}

/// The request may have reached the server: the connection failed after it was handed to the network.
/// Only callers that cannot replay a create safely opt in; everyone else receives `.offline`.
public struct SignedDeliveryUncertain: Error, Equatable, Sendable {
    public init() {}
}

public struct SignedHttpResponse: Equatable, Sendable {
    public let status: Int
    public let body: Data

    public init(status: Int, body: Data) {
        self.status = status
        self.body = body
    }
}

/// Ephemeral URLSession exchange: no cookies, no cache and no redirects.
public final class UrlSessionSignedExchange: SignedHttpExchange, @unchecked Sendable {
    private let session: URLSession

    public init(timeout: TimeInterval = 20.0) {
        let config = URLSessionConfiguration.ephemeral
        config.timeoutIntervalForRequest = timeout
        config.timeoutIntervalForResource = timeout
        config.httpCookieStorage = nil
        config.httpShouldSetCookies = false
        config.urlCache = nil
        config.requestCachePolicy = .reloadIgnoringLocalCacheData
        self.session = URLSession(configuration: config)
    }

    public func send(_ request: URLRequest) async throws -> (Data, HTTPURLResponse) {
        let (data, response) = try await session.data(for: request, delegate: RedirectRefusal())
        guard let http = response as? HTTPURLResponse else {
            throw SignedTransportError.invalidResponse
        }
        return (data, http)
    }
}

private final class RedirectRefusal: NSObject, URLSessionTaskDelegate {
    func urlSession(
        _ session: URLSession,
        task: URLSessionTask,
        willPerformHTTPRedirection response: HTTPURLResponse,
        newRequest request: URLRequest,
        completionHandler: @escaping (URLRequest?) -> Void
    ) {
        completionHandler(nil)
    }
}

/// Registers the device contributor, binds it to a live account session and signs requests.
///
/// Registration runs once per process for the current key and origin (the
/// server converges a repeated registration of the same key), and a binding
/// runs once per process for each account, key and contributor triple, as
/// on Android. A 401 forgets both so the next request proves them again.
public actor SignedRequestTransport {
    public static let maxResponseBytes = 256 * 1024

    public nonisolated let origin: String
    private let authority: String?
    private let signer: DeviceProofSigning
    private let identityStore: DeviceIdentityStoring
    private let exchange: SignedHttpExchange
    private let sessions: SignedSessionSource
    private let now: @Sendable () -> Int64
    private var registered: DeviceIdentityRecord?
    private var boundScope: String?

    public init(
        origin: String,
        signer: DeviceProofSigning,
        identityStore: DeviceIdentityStoring,
        exchange: SignedHttpExchange,
        sessions: SignedSessionSource,
        nowEpochSeconds: @escaping @Sendable () -> Int64 = { Int64(Date().timeIntervalSince1970) }
    ) {
        var trimmed = origin.trimmingCharacters(in: .whitespacesAndNewlines)
        while trimmed.hasSuffix("/") {
            trimmed.removeLast()
        }
        self.origin = trimmed
        self.authority = SignedRequestProfile.authority(origin: trimmed)
        self.signer = signer
        self.identityStore = identityStore
        self.exchange = exchange
        self.sessions = sessions
        self.now = nowEpochSeconds
    }

    /// `account:<id>:<fingerprint>` when an account session exists locally, otherwise `anonymous:<fingerprint>`.
    public nonisolated func ownerScope() -> String? {
        guard let jwk = signer.ensurePublicJwk() else { return nil }
        if let accountId = sessions.currentAccountId() {
            return "account:\(accountId):\(jwk.fingerprint)"
        }
        return "anonymous:" + jwk.fingerprint
    }

    nonisolated func requireOwnerScope(_ expected: String?) throws {
        try Task.checkCancellation()
        guard let expected else { return }
        let accountPrefix = sessions.currentAccountId().map { "account:\($0):" } ?? "anonymous:"
        guard expected.hasPrefix(accountPrefix), ownerScope() == expected else {
            throw SignedTransportError.sessionRequired
        }
    }

    /// Ensures the contributor exists for this key and, with a live session, that it is bound to the account.
    public func ensureIdentity(expectedOwnerScope: String? = nil) async throws -> DeviceIdentityRecord {
        try requireOwnerScope(expectedOwnerScope)
        guard let authority else { throw SignedTransportError.originUnavailable }
        guard let jwk = signer.ensurePublicJwk(),
              PortableAnonymousProof.isFingerprintShape(jwk.fingerprint) else {
            throw SignedTransportError.identityUnavailable
        }
        let record: DeviceIdentityRecord
        if let current = registered, current.origin == origin, current.fingerprint == jwk.fingerprint {
            record = current
        } else {
            record = try await register(jwk: jwk, authority: authority, expectedOwnerScope: expectedOwnerScope)
            try requireOwnerScope(expectedOwnerScope)
            registered = record
            try? identityStore.save(record)
        }
        let session = await sessions.liveSession()
        try requireOwnerScope(expectedOwnerScope)
        if let session {
            if let expectedOwnerScope, !expectedOwnerScope.hasPrefix("account:\(session.accountId):") {
                throw SignedTransportError.sessionRequired
            }
            try await bind(record: record, session: session)
        }
        try requireOwnerScope(expectedOwnerScope)
        return record
    }

    /// The last registration stored for this origin and key, without network.
    public nonisolated func storedIdentity() -> DeviceIdentityRecord? {
        guard let record = identityStore.load(),
              record.origin == origin,
              let jwk = signer.ensurePublicJwk(),
              record.fingerprint == jwk.fingerprint else {
            return nil
        }
        return record
    }

    /// Drops the cached registration and binding; the next request proves both again.
    public func forgetProofs() {
        registered = nil
        boundScope = nil
    }

    /// Sends a request signed with a fresh SIGN challenge. The body is signed and sent byte for byte.
    public func signedRequest(
        method: String,
        path: String,
        query: String = "",
        body: Data?,
        idempotencyKey: String? = nil,
        expectedOwnerScope: String? = nil,
        expectedSessionFamilyId: String? = nil
    ) async throws -> SignedHttpResponse {
        let upperMethod = method.uppercased()
        guard upperMethod == "GET" || upperMethod == "POST",
              SignedRequestProfile.isSignablePath(path),
              SignedRequestProfile.isSignableQuery(query) else {
            throw SignedTransportError.invalidRequest
        }
        if let key = idempotencyKey, key.isEmpty || key.count > 128 {
            throw SignedTransportError.invalidRequest
        }
        guard let authority else { throw SignedTransportError.originUnavailable }
        try await requireSessionFamily(expectedSessionFamilyId, ownerScope: expectedOwnerScope)
        let record = try await ensureIdentity(expectedOwnerScope: expectedOwnerScope)
        let challenge = try await requestChallenge(fingerprint: record.fingerprint, purpose: PortableAnonymousProof.purposeSign)
        try await requireSessionFamily(expectedSessionFamilyId, ownerScope: expectedOwnerScope)
        try requireOwnerScope(expectedOwnerScope)
        let created = now()
        let expires = created + SignedRequestProfile.proofLifetimeSeconds
        let lines = SignedRequestProfile.baseLines(
            method: upperMethod,
            authority: authority,
            path: path,
            query: query,
            body: body,
            created: created,
            expires: expires,
            keyId: record.fingerprint,
            nonce: challenge.nonce
        )
        guard let signature = signer.signProofLines(lines) else {
            throw SignedTransportError.signingUnavailable
        }
        var headers = SignedRequestProfile.headers(
            signature: signature,
            created: created,
            expires: expires,
            keyId: record.fingerprint,
            nonce: challenge.nonce
        )
        if let key = idempotencyKey {
            headers[SignedRequestProfile.headerIdempotencyKey] = key
        }
        do {
            let response = try await send(method: upperMethod, path: path, query: query, body: body, headers: headers)
            try await requireSessionFamily(expectedSessionFamilyId, ownerScope: expectedOwnerScope)
            try requireOwnerScope(expectedOwnerScope)
            return response
        } catch let error as SignedTransportError {
            try await requireSessionFamily(expectedSessionFamilyId, ownerScope: expectedOwnerScope)
            try requireOwnerScope(expectedOwnerScope)
            if case .refused(let status, _, _) = error, status == 401 {
                forgetProofs()
            }
            throw error
        }
    }

    private func requireSessionFamily(_ expected: String?, ownerScope: String?) async throws {
        guard let expected else { return }
        guard let session = await sessions.liveSession(), session.familyId == expected else {
            throw SignedTransportError.sessionRequired
        }
        try requireOwnerScope(ownerScope)
    }

    /// Unsigned JSON POST for session-only routes (station comments and votes).
    /// `reportsUncertainDelivery` turns a connection failure that does not prove the request stayed
    /// local into `SignedDeliveryUncertain` instead of `.offline`.
    public func plainPost(
        path: String, body: Data, idempotencyKey: String? = nil, expectedOwnerScope: String? = nil,
        expectedSessionFamilyId: String? = nil, reportsUncertainDelivery: Bool = false
    ) async throws -> SignedHttpResponse {
        guard SignedRequestProfile.isSignablePath(path) else { throw SignedTransportError.invalidRequest }
        if let key = idempotencyKey, key.isEmpty || key.count > 128 {
            throw SignedTransportError.invalidRequest
        }
        try await requireSessionFamily(expectedSessionFamilyId, ownerScope: expectedOwnerScope)
        try requireOwnerScope(expectedOwnerScope)
        let headers = idempotencyKey.map { [SignedRequestProfile.headerIdempotencyKey: $0] } ?? [:]
        let response: SignedHttpResponse
        do {
            response = try await send(
                method: "POST", path: path, query: "", body: body, headers: headers,
                reportsUncertainDelivery: reportsUncertainDelivery
            )
        } catch {
            try await requireSessionFamily(expectedSessionFamilyId, ownerScope: expectedOwnerScope)
            throw error
        }
        try await requireSessionFamily(expectedSessionFamilyId, ownerScope: expectedOwnerScope)
        try requireOwnerScope(expectedOwnerScope)
        return response
    }

    // MARK: - Identity ceremonies

    private func requestChallenge(fingerprint: String, purpose: String) async throws -> IdentityChallenge {
        guard let body = DeviceIdentityWire.challengeBody(fingerprint: fingerprint, purpose: purpose) else {
            throw SignedTransportError.identityUnavailable
        }
        let response = try await send(method: "POST", path: DeviceIdentityWire.challengePath, query: "", body: body, headers: [:])
        guard let challenge = DeviceIdentityWire.decodeChallenge(response.body, fingerprint: fingerprint, purpose: purpose) else {
            throw SignedTransportError.invalidResponse
        }
        return challenge
    }

    private func register(
        jwk: IosDeviceProofSigner.PublicJwk, authority: String, expectedOwnerScope: String?
    ) async throws -> DeviceIdentityRecord {
        let challenge = try await requestChallenge(fingerprint: jwk.fingerprint, purpose: PortableAnonymousProof.purposeRegister)
        try requireOwnerScope(expectedOwnerScope)
        let created = now()
        let lines = SignedRequestProfile.baseLines(
            method: "POST",
            authority: authority,
            path: DeviceIdentityWire.registrationPath,
            query: "",
            body: nil,
            created: created,
            expires: created + SignedRequestProfile.proofLifetimeSeconds,
            keyId: jwk.fingerprint,
            nonce: challenge.nonce
        )
        guard let signature = signer.signProofLines(lines) else {
            throw SignedTransportError.signingUnavailable
        }
        guard let body = DeviceIdentityWire.registrationBody(
            jwkX: jwk.x,
            jwkY: jwk.y,
            challengeId: challenge.challengeId,
            lines: lines,
            signature: signature
        ) else {
            throw SignedTransportError.identityUnavailable
        }
        let response = try await send(method: "POST", path: DeviceIdentityWire.registrationPath, query: "", body: body, headers: [:])
        guard let registration = DeviceIdentityWire.decodeRegistration(response.body) else {
            throw SignedTransportError.invalidResponse
        }
        return DeviceIdentityRecord(
            origin: origin,
            fingerprint: jwk.fingerprint,
            contributorId: registration.contributorId,
            keyId: registration.keyId
        )
    }

    private func bind(record: DeviceIdentityRecord, session: PortableAuth.Session) async throws {
        let scope = session.accountId + "|" + record.fingerprint + "|" + record.contributorId
        if boundScope == scope { return }
        let expiry = now() + PortableAccountBinding.proofLifetimeSeconds
        let statement = PortableAccountBinding.statement(
            accountId: session.accountId,
            contributorId: record.contributorId,
            expiry: expiry
        )
        guard let signature = signer.signBindingStatement(statement),
              let proof = PortableAccountBinding.proof(fingerprint: record.fingerprint, expiry: expiry, signatureB64Url: signature) else {
            throw SignedTransportError.signingUnavailable
        }
        guard let body = DeviceIdentityWire.bindingBody(
            familyId: session.familyId,
            accessToken: session.accessToken,
            contributorId: record.contributorId,
            proof: proof
        ) else {
            throw SignedTransportError.sessionRequired
        }
        let response = try await send(method: "POST", path: DeviceIdentityWire.bindingPath, query: "", body: body, headers: [:])
        guard let binding = DeviceIdentityWire.decodeBinding(response.body),
              binding.contributorId == record.contributorId,
              binding.keyFingerprint == record.fingerprint else {
            throw SignedTransportError.invalidResponse
        }
        boundScope = scope
    }

    // MARK: - Exchange

    private func send(
        method: String,
        path: String,
        query: String,
        body: Data?,
        headers: [String: String],
        reportsUncertainDelivery: Bool = false
    ) async throws -> SignedHttpResponse {
        var text = origin + path
        if !query.isEmpty {
            text += "?" + query
        }
        guard let url = URL(string: text) else { throw SignedTransportError.invalidRequest }
        var request = URLRequest(url: url)
        request.httpMethod = method
        request.setValue("application/json", forHTTPHeaderField: "Accept")
        request.setValue("no-store", forHTTPHeaderField: "Cache-Control")
        if let body, !body.isEmpty {
            request.httpBody = body
            request.setValue(SignedRequestProfile.jsonContentType, forHTTPHeaderField: "Content-Type")
        }
        for (name, value) in headers {
            request.setValue(value, forHTTPHeaderField: name)
        }
        let result: (Data, HTTPURLResponse)
        do {
            result = try await exchange.send(request)
        } catch let error as SignedTransportError {
            throw error
        } catch {
            if reportsUncertainDelivery, !SignedRequestProfile.provesRequestNotSent(error) {
                throw SignedDeliveryUncertain()
            }
            throw SignedTransportError.offline
        }
        let (data, http) = result
        if data.count > Self.maxResponseBytes {
            throw SignedTransportError.oversizedResponse
        }
        guard http.statusCode >= 200 && http.statusCode < 300 else {
            throw SignedTransportError.refused(
                status: http.statusCode,
                code: SignedRequestProfile.errorCode(from: data) ?? SignedRequestProfile.enterpriseErrorCode(from: data),
                retryAfterSeconds: SignedRequestProfile.retryAfterSeconds(
                    http.value(forHTTPHeaderField: "Retry-After"),
                    nowEpochSeconds: now()
                )
            )
        }
        return SignedHttpResponse(status: http.statusCode, body: data)
    }
}

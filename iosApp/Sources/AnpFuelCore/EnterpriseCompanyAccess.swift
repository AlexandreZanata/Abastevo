import Foundation
#if canImport(FoundationNetworking)
import FoundationNetworking
#endif

/// Refusals of the company access and device planes, shown to the person as they are.
public enum EnterpriseAccessError: String, Error, Equatable, Sendable {
    case signInRequired = "SIGN_IN_REQUIRED"
    case freshAuthRequired = "FRESH_AUTH_REQUIRED"
    case invalidCode = "INVALID_CODE"
    case unavailable = "UNAVAILABLE"
    case conflict = "CONFLICT"
    case alreadyMember = "ALREADY_MEMBER"
    case managedRestricted = "MANAGED_RESTRICTED"
    case rateLimited = "RATE_LIMITED"
    case stale = "STALE"
    case accessDenied = "ACCESS_DENIED"
    case offline = "OFFLINE"
    case entitlementRequired = "ENTITLEMENT_REQUIRED"
    case membershipSuspended = "MEMBERSHIP_SUSPENDED"
    case organizationSuspended = "ORGANIZATION_SUSPENDED"
    case deviceRevoked = "DEVICE_REVOKED"
}

/// One organization the account may enter. A descriptor is a selector, never authority.
public struct EnterpriseContextSummary: Codable, Equatable, Sendable, Identifiable {
    public let organizationId: String
    public let membershipId: String
    public let name: String
    public let state: String?

    public var id: String { organizationId }

    public init(organizationId: String, membershipId: String, name: String, state: String?) {
        self.organizationId = organizationId
        self.membershipId = membershipId
        self.name = name
        self.state = state
    }
}

/// Current server membership metadata of the selected organization.
public struct EnterpriseContextDetail: Equatable, Sendable {
    public let summary: EnterpriseContextSummary
    /// The member's own employee record (B-BR-EMP-380); an older server omits it.
    public let employeeId: String?
    /// Display hints only; every command is still decided by the server.
    public let permissions: [String]
}

public struct EnterpriseInvitationGrant: Equatable, Hashable, Sendable {
    public static let roles: Set<String> = [
        "OWNER", "ADMIN", "FLEET_MANAGER", "SUPERVISOR", "FINANCE", "APPROVER", "DRIVER", "OPERATIONS_STAFF", "AUDITOR", "DATA_MANAGER",
    ]
    public let role: String
    public let scope: String
}

public struct EnterpriseInvitationPreview: Equatable, Sendable {
    public let organizationName: String
    public let legalEntityVerified: Bool
    public let grants: [EnterpriseInvitationGrant]
    public let confirmationRequired: Bool
    public let consentDigest: String
    public let noticeVersion: String
}

public struct EnterpriseInvitationAcknowledgement: Equatable, Sendable {
    public let organizationId: String
    public let invitationId: String
    public let membershipId: String
    public let state: String
    public let membershipState: String
    public let replayed: Bool

    public var isActive: Bool { state == "ACCEPTED" && membershipState == "ACTIVE" }
    public var awaitsConfirmation: Bool { state == "ACCEPTED" && membershipState == "INVITED" }
    public var isDeclined: Bool { state == "DECLINED" }
}

/// One ordered checklist question; its identity is its 1-based position.
public struct EnterpriseChecklistItem: Equatable, Sendable, Identifiable {
    public let position: Int
    public let label: String
    public let critical: Bool
    public let optional: Bool
    public let photoOnNotOk: Bool

    public var id: Int { position }
}

/// The EMP-CHECK-001 applicable read for one asset and moment.
public struct EnterpriseChecklistApplicable: Equatable, Sendable {
    public let assetId: String
    public let context: String
    public let required: Bool
    public let templateId: String?
    public let name: String?
    public let version: Int64?
    public let items: [EnterpriseChecklistItem]
    public let policyRevision: Int64?
    public let policyMode: String?
}

/// Pure wire rules of the company account cookie plane (Android `CompanyAccessClient`,
/// `EnterpriseContextHttpGateway`, `EnterpriseInvitationHttpGateway`, `EnterpriseChecklistHttpGateway`).
/// Zero postfix unwrap operators anywhere in this file.
public enum EnterpriseCompanyWire {
    public static let cookieName = "__Host-empresas-session"
    public static let loginPath = "/v1/org-access/session/login"
    public static let contextsListPath = "/v1/org-access/contexts/list"
    public static let contextsReadPath = "/v1/org-access/contexts/read"
    public static let invitationPrefix = "/v1/org-access/invitations/"
    public static let noticeVersion = "enterprise-invitation-v1"
    public static let noticeText = "The organization sees your employee record, accepted account alias and actions within this organization. It cannot see your personal credentials, personal vehicles, private history or other organizations."
    public static let maxResponseBytes = 64 * 1024

    /// Only the locator's optional UUID hyphens are normalized; the secret is opaque.
    public static func normalizeInvitationCode(_ raw: String) -> String {
        let value = raw.trimmingCharacters(in: .whitespacesAndNewlines)
        let parts = value.split(separator: ".", omittingEmptySubsequences: false).map(String.init)
        guard parts.count == 3, parts[0] == "AIN1", parts[1].count == 32, isLowerHex(parts[1]) else {
            return value
        }
        let id = Array(parts[1])
        let dashed = String(id[0..<8]) + "-" + String(id[8..<12]) + "-" + String(id[12..<16]) + "-" + String(id[16..<20]) + "-" + String(id[20..<32])
        return "AIN1." + dashed + "." + parts[2]
    }

    static func isLowerHex(_ value: String) -> Bool {
        return value.utf8.allSatisfy { ($0 >= 0x30 && $0 <= 0x39) || ($0 >= 0x61 && $0 <= 0x66) }
    }

    static func isSecret(_ value: String, prefix: String) -> Bool {
        guard value.hasPrefix(prefix) else { return false }
        let rest = String(value.dropFirst(prefix.count))
        return rest.utf8.count == 64 && isLowerHex(rest)
    }

    /// The single `__Host-` session cookie: exact value shape, `Path=/`, `Secure`, `HttpOnly`, `SameSite=Strict`, no `Domain`.
    public static func sessionCookie(fromSetCookie header: String?) -> String? {
        guard let header, header.components(separatedBy: cookieName + "=").count == 2, header.hasPrefix(cookieName + "=") else {
            return nil
        }
        let segments = header.split(separator: ";", omittingEmptySubsequences: false).map {
            $0.trimmingCharacters(in: .whitespaces)
        }
        guard let first = segments.first else { return nil }
        let value = String(first.dropFirst(cookieName.count + 1))
        guard isSecret(value, prefix: "abv_company_"), !value.hasPrefix("abv_company_csrf_") else { return nil }
        let attributes = segments.dropFirst().map { $0.lowercased() }
        guard attributes.contains("path=/"), attributes.contains("secure"), attributes.contains("httponly"),
              attributes.contains("samesite=strict"), !attributes.contains(where: { $0.hasPrefix("domain=") }),
              !attributes.contains(where: { $0 == "max-age=0" || $0.hasPrefix("max-age=-") }) else {
            return nil
        }
        return cookieName + "=" + value
    }

    /// The `POST /v1/org-access/session/login` answer bound to the proving account.
    public static func loginCsrf(_ json: [String: Any], accountId: String) -> String? {
        guard json["account_id"] as? String == accountId,
              let csrf = json["csrf_token"] as? String, isSecret(csrf, prefix: "abv_company_csrf_"),
              let authenticated = json["authenticated_at"] as? String, let expires = json["expires_at"] as? String,
              !authenticated.isEmpty, !expires.isEmpty else {
            return nil
        }
        return csrf
    }

    /// Android `CompanyAccessClient.error`; context reads deny generically on any 403/404.
    public static func error(status: Int, code: String?, path: String) -> EnterpriseAccessError {
        if path == contextsListPath || path == contextsReadPath, status == 403 || status == 404 {
            return .accessDenied
        }
        switch (status, code ?? "") {
        case (401, "FRESH_AUTH_REQUIRED"): return .freshAuthRequired
        case (401, _): return .signInRequired
        case (404, "TOKEN_INVALID"): return .invalidCode
        case (409, "ALREADY_MEMBER"): return .alreadyMember
        case (409, "CONFLICT"): return .conflict
        case (403, "MANAGED_ACCOUNT_RESTRICTED"): return .managedRestricted
        case (403, "ENTITLEMENT_REQUIRED"): return .entitlementRequired
        case (403, "MEMBERSHIP_SUSPENDED"), (403, "MEMBERSHIP_ENDED"): return .membershipSuspended
        case (403, "ORG_SUSPENDED"), (403, "ORG_ARCHIVED"): return .organizationSuspended
        case (403, "DEVICE_REVOKED"): return .deviceRevoked
        case (403, _), (404, _): return .accessDenied
        case (429, _): return .rateLimited
        default: return .unavailable
        }
    }

    static func errorCode(_ json: [String: Any]?) -> String? {
        guard let error = json?["error"] as? [String: Any], let code = error["code"] as? String,
              SignedRequestProfile.isEnterpriseCode(code) else {
            return nil
        }
        return code
    }

    static func bool(_ value: Any?) -> Bool? {
        guard let number = value as? NSNumber, CFGetTypeID(number) == CFBooleanGetTypeID() else { return nil }
        return number.boolValue
    }

    static func integer(_ value: Any?) -> Int64? {
        guard let number = value as? NSNumber, CFGetTypeID(number) != CFBooleanGetTypeID() else { return nil }
        let exact = number.int64Value
        return NSNumber(value: exact) == number ? exact : nil
    }

    static func summary(_ value: Any?) -> EnterpriseContextSummary? {
        guard let object = value as? [String: Any],
              let id = object["id"] as? String, EnterpriseValues.isId(id),
              let membership = object["membership_id"] as? String, EnterpriseValues.isId(membership),
              let name = object["name"] as? String, !name.trimmingCharacters(in: .whitespaces).isEmpty, name.count <= 200 else {
            return nil
        }
        return EnterpriseContextSummary(organizationId: id, membershipId: membership, name: name, state: object["state"] as? String)
    }

    /// `contexts/list`: bound to the account, strictly ascending organization ids after the cursor.
    public static func contextsPage(_ json: [String: Any], accountId: String, after: String?) -> (contexts: [EnterpriseContextSummary], next: String?)? {
        guard json["account_id"] as? String == accountId, let values = json["organizations"] as? [Any], values.count <= 100 else {
            return nil
        }
        var contexts: [EnterpriseContextSummary] = []
        for value in values {
            guard let parsed = Self.summary(value) else { return nil }
            if let last = contexts.last, last.organizationId >= parsed.organizationId { return nil }
            if let after, parsed.organizationId <= after { return nil }
            contexts.append(parsed)
        }
        let rawNext = json["next_after_org_id"] as? String ?? ""
        let next: String? = rawNext.isEmpty ? nil : rawNext
        if let next {
            guard EnterpriseValues.isId(next) else { return nil }
            if let after, next <= after { return nil }
        }
        return (contexts, next)
    }

    /// `contexts/read`: the same organization and membership that were selected.
    public static func contextDetail(_ json: [String: Any], accountId: String, selected: EnterpriseContextSummary) -> EnterpriseContextDetail? {
        guard json["account_id"] as? String == accountId, let parsed = Self.summary(json["organization"]),
              parsed.organizationId == selected.organizationId, parsed.membershipId == selected.membershipId else {
            return nil
        }
        var employee: String?
        if let raw = json["employee_id"], !(raw is NSNull) {
            guard let text = raw as? String, EnterpriseValues.isId(text) else { return nil }
            employee = text
        }
        let permissions = (json["permissions"] as? [Any] ?? []).compactMap { $0 as? String }
        return EnterpriseContextDetail(summary: parsed, employeeId: employee, permissions: permissions)
    }

    /// `invitations/preview` (synthetic data mode, the reviewed notice text, 1..10 distinct grants).
    public static func invitationPreview(_ json: [String: Any]) -> EnterpriseInvitationPreview? {
        guard json["data_mode"] as? String == "SYNTHETIC",
              let notice = json["notice"] as? [String: Any], notice["version"] as? String == noticeVersion,
              notice["text"] as? String == noticeText,
              let organization = json["organization"] as? [String: Any], let name = organization["name"] as? String,
              !name.trimmingCharacters(in: .whitespaces).isEmpty, name.count <= 200,
              let verified = bool(organization["legal_entity_verified"]),
              let rawGrants = json["grants"] as? [Any], rawGrants.count >= 1, rawGrants.count <= 10,
              let confirmation = bool(json["confirmation_required"]),
              let digest = json["consent_digest"] as? String, digest.utf8.count == 64, isLowerHex(digest) else {
            return nil
        }
        var grants: [EnterpriseInvitationGrant] = []
        for raw in rawGrants {
            guard let grant = raw as? [String: Any], Set(grant.keys) == ["role", "scope"],
                  let role = grant["role"] as? String, let scope = grant["scope"] as? String,
                  EnterpriseInvitationGrant.roles.contains(role), scope == "ORG" || scope == "SELF",
                  role != "OWNER" || scope == "ORG" else {
                return nil
            }
            let value = EnterpriseInvitationGrant(role: role, scope: scope)
            if grants.contains(value) { return nil }
            grants.append(value)
        }
        return EnterpriseInvitationPreview(
            organizationName: name,
            legalEntityVerified: verified,
            grants: grants,
            confirmationRequired: confirmation,
            consentDigest: digest,
            noticeVersion: noticeVersion
        )
    }

    /// `invitations/accept|decline`: the receipt names the invitation of the code that was sent.
    public static func invitationAcknowledgement(_ json: [String: Any], code: String, accepted: Bool) -> EnterpriseInvitationAcknowledgement? {
        guard json["data_mode"] as? String == "SYNTHETIC",
              let organization = json["organization_id"] as? String, EnterpriseValues.isId(organization),
              let invitation = json["invitation_id"] as? String, EnterpriseValues.isId(invitation),
              let membership = json["membership_id"] as? String, EnterpriseValues.isId(membership),
              let state = json["state"] as? String, let membershipState = json["membership_state"] as? String,
              let replayed = bool(json["replayed"]) else {
            return nil
        }
        let locator = normalizeInvitationCode(code).split(separator: ".", omittingEmptySubsequences: false).map(String.init)
        guard locator.count == 3, locator[0] == "AIN1", locator[1] == invitation else { return nil }
        if accepted {
            guard state == "ACCEPTED", membershipState == "ACTIVE" || membershipState == "INVITED",
                  let acceptedAt = json["accepted_at"] as? String, let deadline = json["confirmation_deadline"] as? String,
                  let acceptedMillis = EnterpriseValues.parseInstantMillis(acceptedAt),
                  let deadlineMillis = EnterpriseValues.parseInstantMillis(deadline),
                  deadlineMillis - acceptedMillis == 168 * 3600 * 1000 else {
                return nil
            }
        } else {
            guard state == "DECLINED", membershipState == "ENDED" else { return nil }
        }
        return EnterpriseInvitationAcknowledgement(
            organizationId: organization,
            invitationId: invitation,
            membershipId: membership,
            state: state,
            membershipState: membershipState,
            replayed: replayed
        )
    }

    /// `checklists/assets/{asset}/applicable`: bound to the asked asset and moment.
    public static func checklistApplicable(_ json: [String: Any], assetId: String, context: String) -> EnterpriseChecklistApplicable? {
        guard json["asset_id"] as? String == assetId, json["context"] as? String == context,
              json["data_mode"] as? String == "SYNTHETIC", let required = bool(json["required"]),
              let match = json["match"] as? String, ["ASSET", "CATEGORY", "ORGANIZATION", "NONE"].contains(match),
              required == (match != "NONE") else {
            return nil
        }
        guard required else {
            return EnterpriseChecklistApplicable(assetId: assetId, context: context, required: false, templateId: nil, name: nil,
                                                 version: nil, items: [], policyRevision: nil, policyMode: nil)
        }
        guard let template = json["template"] as? [String: Any], let policy = json["policy"] as? [String: Any],
              let templateId = template["template_id"] as? String, EnterpriseValues.isId(templateId),
              let name = template["name"] as? String, let version = integer(template["version"]), version >= 1,
              let rawItems = template["items"] as? [Any], rawItems.count >= 1, rawItems.count <= 40,
              let revision = integer(policy["revision"]), revision >= 1,
              let mode = policy["mode"] as? String, mode == "BLOCK" || mode == "WARN" else {
            return nil
        }
        var items: [EnterpriseChecklistItem] = []
        for (index, raw) in rawItems.enumerated() {
            guard let item = raw as? [String: Any], integer(item["position"]) == Int64(index + 1),
                  let label = item["label"] as? String, EnterpriseValues.isValidText(label, maxScalars: 120),
                  let critical = bool(item["critical"]), let optional = bool(item["optional"]),
                  let photo = bool(item["photo_on_not_ok"]), !(critical && optional) else {
                return nil
            }
            items.append(EnterpriseChecklistItem(position: index + 1, label: label, critical: critical, optional: optional, photoOnNotOk: photo))
        }
        return EnterpriseChecklistApplicable(assetId: assetId, context: context, required: true, templateId: templateId, name: name,
                                             version: version, items: items, policyRevision: revision, policyMode: mode)
    }
}

/// The company account cookie plane: memory-only channel, no cookie storage, no redirects.
///
/// `POST /v1/org-access/session/login` exchanges the live personal session for a
/// `__Host-empresas-session` cookie plus CSRF token. Every later call sends exactly
/// that cookie, `X-CSRF` and `Origin`. The channel is bound to the account and
/// session family; a different account or a 401 opens a new one. It never signs
/// or carries device proof and is never written to disk.
public actor EnterpriseCompanyClient {
    private struct Channel {
        let accountId: String
        let familyId: String
        let cookie: String
        let csrf: String
    }

    public nonisolated let origin: String
    private let exchange: SignedHttpExchange
    private let sessions: SignedSessionSource
    private var channel: Channel?

    public init(origin: String, exchange: SignedHttpExchange, sessions: SignedSessionSource) {
        var trimmed = origin.trimmingCharacters(in: .whitespacesAndNewlines)
        while trimmed.hasSuffix("/") {
            trimmed.removeLast()
        }
        self.origin = trimmed
        self.exchange = exchange
        self.sessions = sessions
    }

    /// Forgets the channel; the next call proves the personal session again.
    public func reset() {
        channel = nil
    }

    // MARK: - Context reads

    public func listContexts() async throws -> [EnterpriseContextSummary] {
        var all: [EnterpriseContextSummary] = []
        var after: String?
        for _ in 0..<10 {
            var body: [String: Any] = ["limit": 100]
            if let after { body["after_org_id"] = after }
            let (json, accountId) = try await post(EnterpriseCompanyWire.contextsListPath, body: body)
            guard let page = EnterpriseCompanyWire.contextsPage(json, accountId: accountId, after: after) else {
                throw EnterpriseAccessError.unavailable
            }
            all.append(contentsOf: page.contexts)
            guard let next = page.next else { break }
            after = next
        }
        return all
    }

    public func readContext(_ selected: EnterpriseContextSummary) async throws -> EnterpriseContextDetail {
        let body: [String: Any] = ["organization_id": selected.organizationId, "membership_id": selected.membershipId]
        let (json, accountId) = try await post(EnterpriseCompanyWire.contextsReadPath, body: body)
        guard json["account_id"] as? String == accountId else { throw EnterpriseAccessError.accessDenied }
        guard let detail = EnterpriseCompanyWire.contextDetail(json, accountId: accountId, selected: selected) else {
            throw EnterpriseAccessError.accessDenied
        }
        return detail
    }

    // MARK: - Invitations

    public func previewInvitation(code: String) async throws -> EnterpriseInvitationPreview {
        let normalized = EnterpriseCompanyWire.normalizeInvitationCode(code)
        guard !normalized.isEmpty, normalized.count <= 160 else { throw EnterpriseAccessError.invalidCode }
        let (json, _) = try await post(EnterpriseCompanyWire.invitationPrefix + "preview", body: ["code": normalized])
        guard let preview = EnterpriseCompanyWire.invitationPreview(json) else { throw EnterpriseAccessError.unavailable }
        return preview
    }

    /// One operation id per decision is the idempotency key; the same id is reused on retry.
    public func acceptInvitation(code: String, consentDigest: String, operationId: String) async throws -> EnterpriseInvitationAcknowledgement {
        let normalized = EnterpriseCompanyWire.normalizeInvitationCode(code)
        let body: [String: Any] = [
            "schema_version": 1, "operation_id": operationId, "code": normalized, "consent": true, "consent_digest": consentDigest,
        ]
        let (json, _) = try await post(EnterpriseCompanyWire.invitationPrefix + "accept", body: body)
        guard let ack = EnterpriseCompanyWire.invitationAcknowledgement(json, code: normalized, accepted: true) else {
            throw EnterpriseAccessError.unavailable
        }
        return ack
    }

    public func declineInvitation(code: String, operationId: String) async throws -> EnterpriseInvitationAcknowledgement {
        let normalized = EnterpriseCompanyWire.normalizeInvitationCode(code)
        let body: [String: Any] = ["schema_version": 1, "operation_id": operationId, "code": normalized]
        let (json, _) = try await post(EnterpriseCompanyWire.invitationPrefix + "decline", body: body)
        guard let ack = EnterpriseCompanyWire.invitationAcknowledgement(json, code: normalized, accepted: false) else {
            throw EnterpriseAccessError.unavailable
        }
        return ack
    }

    // MARK: - Checklists

    public func applicableChecklist(organizationId: String, assetId: String, context: String) async throws -> EnterpriseChecklistApplicable {
        guard EnterpriseValues.isId(organizationId), EnterpriseValues.isId(assetId) else { throw EnterpriseAccessError.unavailable }
        let path = "/v1/orgs/\(organizationId)/checklists/assets/\(assetId)/applicable"
        let (json, _) = try await post(path, body: ["context": context])
        guard let applicable = EnterpriseCompanyWire.checklistApplicable(json, assetId: assetId, context: context) else {
            throw EnterpriseAccessError.unavailable
        }
        return applicable
    }

    // MARK: - Channel

    /// Posts on the current channel, opening it first; a channel the server forgot is reopened once.
    private func post(_ path: String, body: [String: Any]) async throws -> ([String: Any], String) {
        guard let session = await sessions.liveSession() else {
            channel = nil
            throw EnterpriseAccessError.signInRequired
        }
        var current = try await ensureChannel(session: session)
        do {
            return (try await send(path, body: body, channel: current), current.accountId)
        } catch let error as EnterpriseAccessError where error == .signInRequired {
            channel = nil
            current = try await ensureChannel(session: session)
            return (try await send(path, body: body, channel: current), current.accountId)
        }
    }

    private func ensureChannel(session: PortableAuth.Session) async throws -> Channel {
        if let current = channel, current.accountId == session.accountId, current.familyId == session.familyId {
            return current
        }
        let retiring = channel.flatMap { $0.accountId == session.accountId ? $0.cookie : nil }
        channel = nil
        let body = try JSONSerialization.data(
            withJSONObject: ["family_id": session.familyId, "access_token": session.accessToken],
            options: [.sortedKeys]
        )
        let (data, http) = try await exchangeRaw(EnterpriseCompanyWire.loginPath, body: body, cookie: retiring, csrf: nil)
        let json = Self.object(data)
        guard http.statusCode == 200 else {
            throw EnterpriseCompanyWire.error(status: http.statusCode, code: EnterpriseCompanyWire.errorCode(json), path: EnterpriseCompanyWire.loginPath)
        }
        guard let json, let csrf = EnterpriseCompanyWire.loginCsrf(json, accountId: session.accountId),
              let cookie = EnterpriseCompanyWire.sessionCookie(fromSetCookie: http.value(forHTTPHeaderField: "Set-Cookie")) else {
            throw EnterpriseAccessError.unavailable
        }
        let opened = Channel(accountId: session.accountId, familyId: session.familyId, cookie: cookie, csrf: csrf)
        channel = opened
        return opened
    }

    private func send(_ path: String, body: [String: Any], channel current: Channel) async throws -> [String: Any] {
        let raw = try JSONSerialization.data(withJSONObject: body, options: [.sortedKeys])
        guard raw.count <= 16 * 1024 else { throw EnterpriseAccessError.unavailable }
        let (data, http) = try await exchangeRaw(path, body: raw, cookie: current.cookie, csrf: current.csrf)
        let json = Self.object(data)
        guard http.statusCode == 200 else {
            let error = EnterpriseCompanyWire.error(status: http.statusCode, code: EnterpriseCompanyWire.errorCode(json), path: path)
            if error == .signInRequired || error == .freshAuthRequired {
                channel = nil
            }
            throw error
        }
        // A data response never sets cookies; only login does.
        guard http.value(forHTTPHeaderField: "Set-Cookie") == nil, let json else { throw EnterpriseAccessError.unavailable }
        return json
    }

    private func exchangeRaw(_ path: String, body: Data, cookie: String?, csrf: String?) async throws -> (Data, HTTPURLResponse) {
        guard path.hasPrefix("/v1/"), !path.contains("?"), !path.contains("#"), !path.contains(".."),
              let url = URL(string: origin + path) else {
            throw EnterpriseAccessError.unavailable
        }
        var request = URLRequest(url: url)
        request.httpMethod = "POST"
        request.httpShouldHandleCookies = false
        request.httpBody = body
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        request.setValue("application/json", forHTTPHeaderField: "Accept")
        request.setValue("no-store", forHTTPHeaderField: "Cache-Control")
        request.setValue(origin, forHTTPHeaderField: "Origin")
        if let cookie { request.setValue(cookie, forHTTPHeaderField: "Cookie") }
        if let csrf { request.setValue(csrf, forHTTPHeaderField: "X-CSRF") }
        let result: (Data, HTTPURLResponse)
        do {
            result = try await exchange.send(request)
        } catch {
            throw EnterpriseAccessError.offline
        }
        if result.0.count > EnterpriseCompanyWire.maxResponseBytes { throw EnterpriseAccessError.unavailable }
        return result
    }

    private static func object(_ data: Data) -> [String: Any]? {
        guard !data.isEmpty else { return nil }
        return (try? JSONSerialization.jsonObject(with: data)) as? [String: Any]
    }
}

public extension SignedRequestProfile {
    /// Enterprise refusal codes are upper snake case (`orgapi.Code`), e.g. `ENTITLEMENT_REQUIRED`.
    static func isEnterpriseCode(_ code: String) -> Bool {
        let bytes = Array(code.utf8)
        guard let first = bytes.first, first >= 0x41, first <= 0x5A, bytes.count <= 64 else { return false }
        return bytes.allSatisfy { ($0 >= 0x41 && $0 <= 0x5A) || ($0 >= 0x30 && $0 <= 0x39) || $0 == 0x5F }
    }

    /// `{"error":{"code":"..."}}` with an enterprise code; anything else is dropped.
    static func enterpriseErrorCode(from data: Data) -> String? {
        guard data.count <= 65_536,
              let json = try? JSONSerialization.jsonObject(with: data) as? [String: Any],
              let error = json["error"] as? [String: Any],
              let code = error["code"] as? String,
              isEnterpriseCode(code) else {
            return nil
        }
        return code
    }
}

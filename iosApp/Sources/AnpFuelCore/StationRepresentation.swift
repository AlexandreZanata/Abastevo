import Foundation

public enum StationBusinessField: String, CaseIterable, Hashable, Identifiable, Sendable {
    case openingHours = "opening_hours"
    case services, phone, website, description
    public var id: String { rawValue }
}

public struct StationBusinessProfile: Equatable, Sendable {
    public let stationId: String
    public let displayName: String
    public let revision: Int?
    public let business: [StationBusinessField: String]
    public let operatorSource: String?
    public let hasBadge: Bool

    public var verifiedRepresentation: Bool {
        hasBadge && operatorSource.map { ["registry", "dou", "review"].contains($0) } == true
    }
}

public enum StationClaimRole: String, CaseIterable, Hashable, Identifiable, Sendable {
    case administrator, manager
    public var id: String { rawValue }

    public var permittedScopes: Set<String> {
        switch self {
        case .administrator: return ["profile.edit", "reply.official", "invite.propose", "revoke.request"]
        case .manager: return ["profile.edit", "reply.official"]
        }
    }
}

public enum StationClaimState: String, Decodable, Sendable {
    case draft
    case awaitingProof = "awaiting_proof"
    case checking
    case needsInformation = "needs_information"
    case inReview = "in_review"
    case approved, denied, cancelled, expired

    public var mayCancel: Bool {
        switch self {
        case .draft, .awaitingProof, .checking, .needsInformation, .inReview: return true
        case .approved, .denied, .cancelled, .expired: return false
        }
    }
}

public struct StationClaimSummary: Equatable, Sendable, Identifiable {
    public let id: String
    public let state: StationClaimState?
}

public struct StationRepresentationClaim: Equatable, Sendable, Identifiable {
    public let id: String
    public let stationId: String
    public let role: StationClaimRole
    public let scopes: Set<String>
    public let state: StationClaimState?
    public let version: Int?
    public let declarationState: String
    public let declarationId: String
    public let declaration: String
    public let expiresAt: Date

    public var canSupplyProof: Bool {
        guard declarationState == "active" else { return false }
        switch state {
        case .draft?, .awaitingProof?, .needsInformation?: return true
        default: return false
        }
    }

    public func canSupplyProof(at now: Date) -> Bool { canSupplyProof && expiresAt > now }
}

public struct StationClaimProofReceipt: Decodable, Equatable, Sendable {
    public let id: String
    public let status: String
    public let expiresAt: Date
}

public enum StationRepresentationError: Error, Equatable, Sendable {
    case invalidInput, invalidResponse, signInRequired, offline, ownerChanged, unavailable
    case refused(status: Int, code: String?, retryAfterSeconds: Int64?)

    public static func from(_ error: Error) -> StationRepresentationError {
        if let profile = error as? StationRepresentationError { return profile }
        if error is CancellationError { return .ownerChanged }
        if let transport = error as? SignedTransportError {
            switch transport {
            case .offline: return .offline
            case .sessionRequired: return .signInRequired
            case .refused(let status, let code, let delay):
                return .refused(status: status, code: code, retryAfterSeconds: delay)
            case .oversizedResponse, .invalidResponse: return .invalidResponse
            default: return .unavailable
            }
        }
        if (error as? CommunityWriteError) == .signInRequired { return .signInRequired }
        if error is URLError { return .offline }
        return .unavailable
    }

    public var integrationPending: Bool {
        if case .refused(status: 503, code: "profile.integration-pending", retryAfterSeconds: _) = self { return true }
        return false
    }

    public var proofStorageUnavailable: Bool {
        if case .refused(status: 503, code: "proof.storage-unavailable", retryAfterSeconds: _) = self { return true }
        return false
    }
}

public enum StationRepresentationRules {
    public static let claimBodyLimit = 64 * 1024
    public static let maxOwnedClaims = 20
    public static let maxProofBytes = 5 * 1_024 * 1_024

    private static let services: Set<String> = [
        "fuel", "convenience", "carwash", "tire-service", "oil-change",
        "restaurant", "atm", "restroom", "wifi", "parking",
    ]

    public static func validateBusinessFields(_ fields: [StationBusinessField: String]) -> Bool {
        guard !fields.isEmpty else { return false }
        return fields.allSatisfy { field, raw in
            let value = raw.trimmingCharacters(in: .whitespacesAndNewlines)
            guard !value.isEmpty else { return false }
            switch field {
            case .openingHours, .phone, .website: return value.utf8.count <= 256
            case .description: return value.unicodeScalars.count <= 280
            case .services:
                return value.split(separator: ",", omittingEmptySubsequences: false).allSatisfy {
                    services.contains($0.trimmingCharacters(in: .whitespacesAndNewlines))
                }
            }
        }
    }

    public static func isSignedPdfCandidate(_ data: Data) -> Bool {
        data.count <= maxProofBytes && data.starts(with: "%PDF-".utf8)
    }

    /// Bound reads regardless of a document provider’s reported size. Do not rewrite the PDF.
    public static func readPdf(from url: URL) throws -> Data {
        guard url.isFileURL else { throw StationRepresentationError.invalidInput }
        let file = try FileHandle(forReadingFrom: url)
        defer { try? file.close() }
        var body = Data()
        while body.count <= maxProofBytes {
            let remaining = maxProofBytes + 1 - body.count
            guard let chunk = try file.read(upToCount: min(8_192, remaining)), !chunk.isEmpty else { break }
            body.append(chunk)
        }
        guard isSignedPdfCandidate(body) else { throw StationRepresentationError.invalidInput }
        return body
    }

    public static func declarationData(_ declaration: String) throws -> Data {
        guard !declaration.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty, declaration.utf16.count <= 8_192 else {
            throw StationRepresentationError.invalidInput
        }
        return Data(declaration.utf8)
    }
}

enum StationRepresentationWire {
    struct Profile: Decodable {
        struct OperatorInfo: Decodable { let source: String? }
        let stationId: String
        let displayName: String
        let revision: Int?
        let business: [String: String]?
        let operatorInfo: OperatorInfo?
        let hasBadge: Bool?

        enum CodingKeys: String, CodingKey {
            case stationId, displayName, revision, business, hasBadge
            case operatorInfo = "operator"
        }
    }

    struct Claim: Decodable {
        let id: String
        let stationId: String
        let role: String
        let scopes: Set<String>
        let state: String
        let version: Int?
        let declarationState: String
        let declarationId: String
        let declaration: String
        let expiresAt: Date
    }

    struct List: Decodable {
        struct Item: Decodable { let id: String; let state: String }
        let items: [Item]
    }

    static func validatedId(_ raw: String) throws -> String {
        let id = raw.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
        guard CommunityWire.isUuid(id) else { throw StationRepresentationError.invalidInput }
        return id
    }

    static func decode<T: Decodable>(_ type: T.Type, data: Data) throws -> T {
        guard data.count <= StationRepresentationRules.claimBodyLimit else { throw StationRepresentationError.invalidResponse }
        let decoder = JSONDecoder()
        decoder.keyDecodingStrategy = .convertFromSnakeCase
        decoder.dateDecodingStrategy = .iso8601
        guard let doc = try? decoder.decode(type, from: data) else { throw StationRepresentationError.invalidResponse }
        return doc
    }

    static func profile(_ data: Data, stationId: String) throws -> StationBusinessProfile {
        let row = try decode(Profile.self, data: data)
        guard row.stationId == stationId else { throw StationRepresentationError.invalidResponse }
        let business = (row.business ?? [:]).reduce(into: [StationBusinessField: String]()) {
            if let field = StationBusinessField(rawValue: $1.key) { $0[field] = $1.value }
        }
        return StationBusinessProfile(stationId: stationId, displayName: row.displayName,
                                      revision: row.revision, business: business,
                                      operatorSource: row.operatorInfo?.source, hasBadge: row.hasBadge ?? false)
    }

    static func claim(_ response: SignedHttpResponse, expectedId: String? = nil, stationId: String? = nil) throws -> StationRepresentationClaim {
        let row = try decode(Claim.self, data: response.body)
        guard CommunityWire.isUuid(row.id), CommunityWire.isUuid(row.stationId),
              CommunityWire.isUuid(row.declarationId), let role = StationClaimRole(rawValue: row.role),
              (expectedId == nil || row.id == expectedId), (stationId == nil || row.stationId == stationId),
              role.permittedScopes.isSuperset(of: row.scopes),
              row.declaration.utf16.count <= 8_192 else {
            throw StationRepresentationError.invalidResponse
        }
        return StationRepresentationClaim(id: row.id, stationId: row.stationId, role: role,
            scopes: row.scopes, state: StationClaimState(rawValue: row.state), version: row.version,
            declarationState: row.declarationState, declarationId: row.declarationId,
            declaration: row.declaration, expiresAt: row.expiresAt)
    }
}

extension AbastevoHttpClient {
    public func fetchStationBusinessProfile(stationId: String) async throws -> StationBusinessProfile? {
        let id = try StationRepresentationWire.validatedId(stationId)
        do {
            let body = try await publicRead(path: "/v1/stations/\(id)/profile",
                                            maxBytes: StationRepresentationRules.claimBodyLimit, preserveHttpErrors: true)
            return try StationRepresentationWire.profile(body, stationId: id)
        } catch HttpClientError.httpStatus(404, _) {
            return nil
        }
    }
}

extension CommunityWriteClient {
    public func openStationClaim(stationId: String, role: StationClaimRole, scopes: Set<String>,
                                 clientKey: String, expectedScope: String) async throws -> StationRepresentationClaim {
        let id = try StationRepresentationWire.validatedId(stationId)
        guard !scopes.isEmpty, role.permittedScopes.isSuperset(of: scopes), CommunityWire.isUuid(clientKey) else {
            throw StationRepresentationError.invalidInput
        }
        let response = try await stationOwnerPost(
            "/v1/stations/\(id)/claims", scope: expectedScope,
            fields: ["role": role.rawValue, "scopes": scopes.sorted(), "client_key": clientKey],
            accepting: [200, 201]
        )
        return try StationRepresentationWire.claim(response, stationId: id)
    }

    public func ownedStationClaims(expectedScope: String) async throws -> [StationClaimSummary] {
        let response = try await stationOwnerPost("/v1/profile/claims/mine", scope: expectedScope)
        let rows = try StationRepresentationWire.decode(StationRepresentationWire.List.self, data: response.body).items
        guard rows.count <= StationRepresentationRules.maxOwnedClaims,
              rows.allSatisfy({ CommunityWire.isUuid($0.id) }),
              Set(rows.map(\.id)).count == rows.count else { throw StationRepresentationError.invalidResponse }
        return rows.map { StationClaimSummary(id: $0.id, state: StationClaimState(rawValue: $0.state)) }
    }

    public func stationClaim(id: String, expectedScope: String) async throws -> StationRepresentationClaim {
        let id = try StationRepresentationWire.validatedId(id)
        let response = try await stationOwnerPost("/v1/profile/claims/\(id)/status", scope: expectedScope)
        return try StationRepresentationWire.claim(response, expectedId: id)
    }

    public func stationClaims(stationId: String, expectedScope: String) async throws -> [StationRepresentationClaim] {
        let stationId = try StationRepresentationWire.validatedId(stationId)
        let owned = try await ownedStationClaims(expectedScope: expectedScope)
        var claims: [StationRepresentationClaim] = []
        for item in owned {
            try Task.checkCancellation()
            let claim = try await stationClaim(id: item.id, expectedScope: expectedScope)
            if claim.stationId == stationId { claims.append(claim) }
        }
        return claims
    }

    public func reissueStationClaim(id: String, operationId: String, expectedScope: String) async throws -> StationRepresentationClaim {
        let id = try StationRepresentationWire.validatedId(id)
        let response = try await stationOwnerPost("/v1/profile/claims/\(id)/reissue", scope: expectedScope, operationId: operationId)
        return try StationRepresentationWire.claim(response, expectedId: id)
    }

    public func cancelStationClaim(id: String, operationId: String, expectedScope: String) async throws {
        let id = try StationRepresentationWire.validatedId(id)
        let response = try await stationOwnerPost("/v1/profile/claims/\(id)/cancel", scope: expectedScope, operationId: operationId)
        struct Result: Decodable { let status: String }
        guard try StationRepresentationWire.decode(Result.self, data: response.body).status == "cancelled" else {
            throw StationRepresentationError.invalidResponse
        }
    }

    public func submitStationClaimProof(
        claim: StationRepresentationClaim, pdf: Data, operationId: String, expectedScope: String,
        now: @Sendable () -> Date = { Date() }
    ) async throws -> StationClaimProofReceipt {
        guard StationRepresentationRules.isSignedPdfCandidate(pdf) else { throw StationRepresentationError.invalidInput }
        let fresh = try await stationClaim(id: claim.id, expectedScope: expectedScope)
        guard fresh.declarationId == claim.declarationId, fresh.stationId == claim.stationId, fresh.canSupplyProof(at: now()) else {
            throw StationRepresentationError.invalidInput
        }
        let response = try await stationOwnerPost("/v1/profile/claims/\(fresh.id)/proof", scope: expectedScope, fields: [
            "declaration_id": fresh.declarationId,
            "filename": "signed.pdf",
            "content_type": "application/pdf",
            "kind": "scan",
            "content_base64": pdf.base64EncodedString(),
        ], operationId: operationId, accepting: [200, 201])
        let receipt = try StationRepresentationWire.decode(StationClaimProofReceipt.self, data: response.body)
        guard CommunityWire.isUuid(receipt.id), ["received", "checking", "valid", "indeterminate"].contains(receipt.status) else {
            throw StationRepresentationError.invalidResponse
        }
        return receipt
    }

    public func editStationBusinessProfile(
        stationId: String, revision: Int, fields: [StationBusinessField: String], operationId: String, expectedScope: String
    ) async throws -> Int {
        let id = try StationRepresentationWire.validatedId(stationId)
        guard revision > 0, StationRepresentationRules.validateBusinessFields(fields) else { throw StationRepresentationError.invalidInput }
        let wireFields = Dictionary(uniqueKeysWithValues: fields.map { ($0.key.rawValue, $0.value) })
        let response = try await stationOwnerPost("/v1/stations/\(id)/profile", scope: expectedScope,
            fields: ["expected_revision": revision, "fields": wireFields], operationId: operationId)
        struct Reply: Decodable { let revision: Int }
        let acceptedRevision = try StationRepresentationWire.decode(Reply.self, data: response.body).revision
        guard acceptedRevision > revision else { throw StationRepresentationError.invalidResponse }
        return acceptedRevision
    }

    public func replyAsStation(
        stationId: String, product: FuelProduct, text: String, operationId: String, expectedScope: String
    ) async throws -> String {
        let id = try StationRepresentationWire.validatedId(stationId)
        let message = PortableText.normalize(text)
        guard PortableText.isValidComment(message) else { throw StationRepresentationError.invalidInput }
        let response = try await stationOwnerPost("/v1/stations/\(id)/profile/replies", scope: expectedScope,
            fields: ["product": product.wireCode, "text": message], operationId: operationId, accepting: [201])
        struct Reply: Decodable { let commentId: String }
        let receipt = try StationRepresentationWire.decode(Reply.self, data: response.body)
        guard CommunityWire.isUuid(receipt.commentId) else { throw StationRepresentationError.invalidResponse }
        return receipt.commentId
    }

    private func stationOwnerPost(
        _ path: String, scope: String, fields: [String: Any] = [:], operationId: String? = nil, accepting statuses: Set<Int> = [200]
    ) async throws -> SignedHttpResponse {
        do {
            let reply = try await ownerPost(path: path, expectedScope: scope, requiresDeviceProof: false, idempotencyKey: operationId) { session in
                var body = fields
                body["family_id"] = session.familyId
                body["access_token"] = session.accessToken
                return try? JSONSerialization.data(withJSONObject: body, options: [.sortedKeys])
            }
            guard statuses.contains(reply.status) else { throw StationRepresentationError.invalidResponse }
            return reply
        } catch {
            throw StationRepresentationError.from(error)
        }
    }
}

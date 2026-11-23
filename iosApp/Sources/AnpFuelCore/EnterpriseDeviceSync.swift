import Foundation
#if canImport(FoundationNetworking)
import FoundationNetworking
#endif

/// The device enrollment of one membership; custody metadata, never business permission.
public struct EnterpriseEnrollment: Codable, Equatable, Sendable {
    public let membershipId: String
    public let enrollmentId: String
    public let revision: Int64

    public init(membershipId: String, enrollmentId: String, revision: Int64) {
        self.membershipId = membershipId
        self.enrollmentId = enrollmentId
        self.revision = revision
    }
}

/// The exact device contact verdicts (Android `EnterpriseContactReply`).
public enum EnterpriseContactVerdict: Equatable, Sendable {
    /// Existing-account custody for the approved 30-day offline window.
    case active
    /// LOST_OR_COMPROMISED: this partition's local data is erased.
    case erase
    /// ACCESS_ENDED: held work stays locked and no new command is accepted.
    case lock
}

public struct EnterpriseContactOutcome: Equatable, Sendable {
    public let enrollment: EnterpriseEnrollment
    public let verdict: EnterpriseContactVerdict
    public let serverTime: String
}

public enum EnterpriseSyncError: Error, Equatable, Sendable {
    case signInRequired
    case offline
    case unavailable
    case invalidResponse
    case refused(status: Int, code: String?, retryAfterSeconds: Int64?)

    init(_ error: Error) {
        if let known = error as? EnterpriseSyncError {
            self = known
            return
        }
        guard let transport = error as? SignedTransportError else {
            self = .invalidResponse
            return
        }
        switch transport {
        case .offline: self = .offline
        case .sessionRequired: self = .signInRequired
        case .oversizedResponse, .invalidResponse: self = .invalidResponse
        case .invalidRequest, .originUnavailable, .identityUnavailable, .signingUnavailable: self = .unavailable
        case .refused(let status, let code, let retryAfter): self = .refused(status: status, code: code, retryAfterSeconds: retryAfter)
        }
    }

    /// Android `EnterpriseDeviceContactClient.refusal`.
    public var accessError: EnterpriseAccessError {
        switch self {
        case .signInRequired: return .signInRequired
        case .offline: return .offline
        case .unavailable, .invalidResponse: return .unavailable
        case .refused(let status, let code, _):
            switch (status, code ?? "") {
            case (401, "FRESH_AUTH_REQUIRED"): return .freshAuthRequired
            case (401, _): return .signInRequired
            case (403, "MANAGED_ACCOUNT_RESTRICTED"): return .managedRestricted
            case (403, "ENTITLEMENT_REQUIRED"): return .entitlementRequired
            case (403, "MEMBERSHIP_SUSPENDED"), (403, "MEMBERSHIP_ENDED"): return .membershipSuspended
            case (403, "ORG_SUSPENDED"), (403, "ORG_ARCHIVED"): return .organizationSuspended
            case (403, "DEVICE_REVOKED"): return .deviceRevoked
            case (403, _), (404, _): return .accessDenied
            case (409, _): return .conflict
            case (429, _): return .rateLimited
            default: return .unavailable
            }
        }
    }
}

// MARK: - Reference feed values

public struct EnterpriseReferenceMeter: Codable, Equatable, Sendable, Identifiable {
    public let id: String
    public let kind: EnterpriseMeterKind
    public let modulus: Int64?
    public let segmentId: String
    public let installedAt: String
}

/// The minimised asset of the member's reference feed; a read copy decided again at intake.
public struct EnterpriseReferenceAsset: Codable, Equatable, Sendable, Identifiable {
    public let id: String
    public let revision: Int64
    public let kind: String
    public let usageBasis: String
    public let state: String
    public let name: String
    public let fleetNumber: String
    public let plate: String
    public let products: [String]
    public let meters: [EnterpriseReferenceMeter]
    public let assignmentKind: String
    public let assignmentValidTo: String?

    public var isActive: Bool { state == "ACTIVE" }

    public func assignmentEnded(nowMillis: Int64) -> Bool {
        guard let validTo = assignmentValidTo, let end = EnterpriseValues.parseInstantMillis(validTo) else { return false }
        return nowMillis >= end
    }
}

/// The local copy of one partition's feed (Android `ReferenceCache` v1 semantics).
public struct EnterpriseReferenceCache: Codable, Equatable, Sendable {
    public static let confirmationMillis: Int64 = 86_400_000

    public let membershipId: String
    public let enrollmentId: String
    public let revision: Int64
    public let cursor: String
    public let confirmedAt: String
    public let forbidden: Bool
    public let assets: [EnterpriseReferenceAsset]
    public let receivedAtMillis: Int64

    /// Cached reference data is not confirmed after 24 hours without contact (BR-OFFL-016).
    public func isConfirmed(nowMillis: Int64) -> Bool {
        let age = nowMillis - receivedAtMillis
        return age >= 0 && age < Self.confirmationMillis
    }
}

public struct EnterpriseReferenceChange: Equatable, Sendable {
    public let revision: Int64
    public let entityId: String
    public let isDelete: Bool
    public let asset: EnterpriseReferenceAsset?
}

public struct EnterpriseReferenceResponse: Equatable, Sendable {
    public let requestId: String
    public let membershipId: String
    public let enrollmentId: String
    public let isDelta: Bool
    public let reason: String?
    public let fromRevision: Int64?
    public let revision: Int64
    public let confirmedAt: String
    public let cursor: String
    public let changes: [EnterpriseReferenceChange]
}

/// Signed reference feed wire (`POST /v1/orgs/{org_id}/sync/reference`, contracts reference-feed-v1).
/// Zero postfix unwrap operators anywhere in this file.
public enum EnterpriseReferenceWire {
    public static let maxRequestBytes = 4096
    public static let maxChanges = 200
    static let resetReasons: Set<String> = ["NEW_CURSOR", "CURSOR_EXPIRED", "CURSOR_AHEAD", "FORBIDDEN"]

    public static func requestBody(requestId: String, familyId: String, accessToken: String, membershipId: String,
                                   enrollmentId: String, cursor: String?) throws -> Data {
        guard EnterpriseValues.isId(requestId), EnterpriseValues.isId(familyId), EnterpriseSyncWire.isAccessToken(accessToken),
              EnterpriseValues.isId(membershipId), EnterpriseValues.isId(enrollmentId) else {
            throw EnterpriseProtocolFailure.invalid
        }
        if let cursor, !isCursor(cursor) { throw EnterpriseProtocolFailure.invalid }
        let body = try EnterpriseCanonicalJson.encode(.object([
            "schema_version": .int(1),
            "request_id": .text(requestId),
            "family_id": .text(familyId),
            "access_token": .text(accessToken),
            "membership_id": .text(membershipId),
            "enrollment_id": .text(enrollmentId),
            "cursor": .optionalText(cursor),
        ]))
        if body.count > maxRequestBytes { throw EnterpriseProtocolFailure.limit }
        return body
    }

    static func isCursor(_ value: String) -> Bool {
        let bytes = Array(value.utf8)
        return !bytes.isEmpty && bytes.count <= 1024 && bytes.allSatisfy { $0 >= 0x21 && $0 <= 0x7E }
    }

    static func positive(_ value: EnterpriseJson?, minimum: Int64) throws -> Int64 {
        guard let lexeme = value?.integerLexeme, let number = Int64(lexeme), number >= minimum else {
            throw EnterpriseProtocolFailure.invalid
        }
        return number
    }

    static func text(_ fields: [String: EnterpriseJson], _ key: String) throws -> String {
        guard let value = fields[key]?.textValue else { throw EnterpriseProtocolFailure.invalid }
        return value
    }

    static func bounded(_ value: String, _ maximum: Int) throws -> String {
        if value.unicodeScalars.count > maximum { throw EnterpriseProtocolFailure.invalid }
        return value
    }

    static func instant(_ value: String) throws -> String {
        guard EnterpriseValues.isCanonicalInstant(value), value != "0001-01-01T00:00:00Z" else { throw EnterpriseProtocolFailure.invalid }
        return value
    }

    static func asset(_ value: EnterpriseJson) throws -> EnterpriseReferenceAsset {
        let keys: Set<String> = ["id", "revision", "kind", "usage_basis", "state", "name", "fleet_number", "plate", "catalog",
                                 "products", "meters", "assignment"]
        guard let fields = value.objectValue, Set(fields.keys) == keys, fields["catalog"]?.textValue == "fuel-products-v1",
              let products = fields["products"]?.arrayValue, products.count <= 64,
              let meters = fields["meters"]?.arrayValue, meters.count <= 16,
              let assignment = fields["assignment"]?.objectValue, Set(assignment.keys) == ["kind", "valid_to"] else {
            throw EnterpriseProtocolFailure.invalid
        }
        let id = try text(fields, "id")
        let kind = try text(fields, "kind")
        let usage = try text(fields, "usage_basis")
        let state = try text(fields, "state")
        let assignmentKind = try text(assignment, "kind")
        guard EnterpriseValues.isId(id), ["VEHICLE", "MACHINE", "IMPLEMENT"].contains(kind), ["DISTANCE", "HOURS"].contains(usage),
              ["DRAFT", "ACTIVE", "INACTIVE", "DISPOSED", "ARCHIVED"].contains(state),
              ["PRIMARY", "TEMPORARY", "ADDITIONAL"].contains(assignmentKind) else {
            throw EnterpriseProtocolFailure.invalid
        }
        var validTo: String?
        if let raw = assignment["valid_to"], !raw.isNull {
            guard let text = raw.textValue else { throw EnterpriseProtocolFailure.invalid }
            validTo = try instant(text)
        }
        let parsedProducts = try products.map { item -> String in
            guard let product = item.textValue else { throw EnterpriseProtocolFailure.invalid }
            return try bounded(product, 64)
        }
        let parsedMeters = try meters.map { item -> EnterpriseReferenceMeter in
            guard let meter = item.objectValue, Set(meter.keys) == ["id", "kind", "modulus", "segment_id", "installed_at"],
                  let rawKind = meter["kind"]?.textValue, let meterKind = EnterpriseMeterKind(rawValue: rawKind) else {
                throw EnterpriseProtocolFailure.invalid
            }
            let meterId = try text(meter, "id")
            let segment = try text(meter, "segment_id")
            guard EnterpriseValues.isId(meterId), EnterpriseValues.isId(segment) else { throw EnterpriseProtocolFailure.invalid }
            var modulus: Int64?
            if let raw = meter["modulus"], !raw.isNull {
                modulus = try positive(raw, minimum: 1)
            }
            return EnterpriseReferenceMeter(id: meterId, kind: meterKind, modulus: modulus, segmentId: segment,
                                            installedAt: try instant(try text(meter, "installed_at")))
        }
        return EnterpriseReferenceAsset(
            id: id,
            revision: try positive(fields["revision"], minimum: 1),
            kind: kind,
            usageBasis: usage,
            state: state,
            name: try bounded(try text(fields, "name"), 80),
            fleetNumber: try bounded(try text(fields, "fleet_number"), 40),
            plate: try bounded(try text(fields, "plate"), 40),
            products: parsedProducts,
            meters: parsedMeters,
            assignmentKind: assignmentKind,
            assignmentValidTo: validTo
        )
    }

    /// Structure first (anything the server cannot write is invalid), then correlation (binding).
    public static func parseResponse(_ data: Data, requestId: String, membershipId: String, enrollmentId: String,
                                     sentCursor: String?, heldRevision: Int64?) throws -> EnterpriseReferenceResponse {
        let root = try EnterpriseCanonicalJson.parse(data, maxBytes: SignedRequestTransport.maxResponseBytes)
        let keys: Set<String> = ["schema_version", "request_id", "membership_id", "enrollment_id", "mode", "reason",
                                 "from_revision", "revision", "confirmed_at", "cursor", "entity_types", "changes"]
        guard let fields = root.objectValue, Set(fields.keys) == keys, fields["schema_version"] == EnterpriseJson.int(1),
              let types = fields["entity_types"]?.arrayValue, types == [EnterpriseJson.text("asset")],
              let items = fields["changes"]?.arrayValue else {
            throw EnterpriseProtocolFailure.invalid
        }
        if items.count > maxChanges { throw EnterpriseProtocolFailure.limit }
        let responseRequest = try text(fields, "request_id")
        let responseMember = try text(fields, "membership_id")
        let responseEnrollment = try text(fields, "enrollment_id")
        let mode = try text(fields, "mode")
        guard EnterpriseValues.isId(responseRequest), EnterpriseValues.isId(responseMember), EnterpriseValues.isId(responseEnrollment),
              mode == "DELTA" || mode == "RESET" else {
            throw EnterpriseProtocolFailure.invalid
        }
        var reason: String?
        if let raw = fields["reason"], !raw.isNull {
            guard let value = raw.textValue, resetReasons.contains(value) else { throw EnterpriseProtocolFailure.invalid }
            reason = value
        }
        var from: Int64?
        if let raw = fields["from_revision"], !raw.isNull {
            from = try positive(raw, minimum: 0)
        }
        let revision = try positive(fields["revision"], minimum: 0)
        let confirmed = try instant(try text(fields, "confirmed_at"))
        let cursor = try text(fields, "cursor")
        guard isCursor(cursor) else { throw EnterpriseProtocolFailure.invalid }
        let changes = try items.map { item -> EnterpriseReferenceChange in
            guard let change = item.objectValue, Set(change.keys) == ["revision", "entity_type", "entity_id", "op", "asset"],
                  change["entity_type"]?.textValue == "asset", let op = change["op"]?.textValue, op == "UPSERT" || op == "DELETE" else {
                throw EnterpriseProtocolFailure.invalid
            }
            let entity = try text(change, "entity_id")
            guard EnterpriseValues.isId(entity), let rawAsset = change["asset"] else { throw EnterpriseProtocolFailure.invalid }
            var parsed: EnterpriseReferenceAsset?
            if !rawAsset.isNull {
                parsed = try asset(rawAsset)
            }
            return EnterpriseReferenceChange(revision: try positive(change["revision"], minimum: 1), entityId: entity,
                                             isDelete: op == "DELETE", asset: parsed)
        }
        let isDelta = mode == "DELTA"
        if isDelta {
            guard reason == nil, let start = from, start <= revision else { throw EnterpriseProtocolFailure.invalid }
        } else {
            guard let resetReason = reason, from == nil, resetReason != "FORBIDDEN" || changes.isEmpty else {
                throw EnterpriseProtocolFailure.invalid
            }
        }
        var prior = from ?? 0
        var entities = Set<String>()
        for change in changes {
            guard change.revision > prior, change.revision <= revision, entities.insert(change.entityId).inserted else {
                throw EnterpriseProtocolFailure.invalid
            }
            prior = change.revision
            if change.isDelete {
                if change.asset != nil || !isDelta { throw EnterpriseProtocolFailure.invalid }
            } else if change.asset?.id != change.entityId {
                throw EnterpriseProtocolFailure.invalid
            }
        }
        guard responseRequest == requestId, responseMember == membershipId, responseEnrollment == enrollmentId else {
            throw EnterpriseProtocolFailure.binding
        }
        if sentCursor == nil {
            guard heldRevision == nil, !isDelta, reason == "NEW_CURSOR" || reason == "FORBIDDEN" else {
                throw EnterpriseProtocolFailure.binding
            }
        } else {
            guard let held = heldRevision else { throw EnterpriseProtocolFailure.binding }
            if isDelta {
                if from != held { throw EnterpriseProtocolFailure.binding }
            } else if reason == "NEW_CURSOR" {
                throw EnterpriseProtocolFailure.binding
            }
        }
        return EnterpriseReferenceResponse(
            requestId: responseRequest, membershipId: responseMember, enrollmentId: responseEnrollment, isDelta: isDelta,
            reason: reason, fromRevision: from, revision: revision, confirmedAt: confirmed, cursor: cursor, changes: changes
        )
    }

    /// Applies one bound response whole, or throws and changes nothing.
    public static func apply(prior: EnterpriseReferenceCache?, response: EnterpriseReferenceResponse, receivedAtMillis: Int64) throws -> EnterpriseReferenceCache {
        var assets: [EnterpriseReferenceAsset]
        if response.isDelta {
            guard let prior, prior.membershipId == response.membershipId, prior.enrollmentId == response.enrollmentId,
                  prior.revision == response.fromRevision else {
                throw EnterpriseProtocolFailure.binding
            }
            var current: [String: EnterpriseReferenceAsset] = [:]
            for asset in prior.assets { current[asset.id] = asset }
            for change in response.changes {
                if change.isDelete {
                    current[change.entityId] = nil
                } else if let asset = change.asset {
                    current[change.entityId] = asset
                }
            }
            assets = Array(current.values)
        } else {
            assets = response.changes.compactMap { $0.asset }
        }
        if assets.count > maxChanges { throw EnterpriseProtocolFailure.limit }
        assets.sort { $0.id < $1.id }
        let forbidden = response.reason == "FORBIDDEN"
        return EnterpriseReferenceCache(
            membershipId: response.membershipId,
            enrollmentId: response.enrollmentId,
            revision: response.revision,
            cursor: response.cursor,
            confirmedAt: response.confirmedAt,
            forbidden: forbidden,
            assets: forbidden ? [] : assets,
            receivedAtMillis: receivedAtMillis
        )
    }
}

// MARK: - Device enrollment wire

public enum EnterpriseDeviceWire {
    public static let label = "iOS"

    static func object(_ data: Data) throws -> [String: EnterpriseJson] {
        guard let fields = try EnterpriseCanonicalJson.parse(data, maxDepth: 8).objectValue else {
            throw EnterpriseProtocolFailure.invalid
        }
        return fields
    }

    /// The enrolled device record of the selected membership.
    public static func device(_ fields: [String: EnterpriseJson], membershipId: String) throws -> EnterpriseEnrollment {
        let keys: Set<String> = ["id", "membership_id", "label", "state", "revision", "revocation_class", "enrolled_at",
                                 "last_sync_at", "revoked_at", "data_mode"]
        guard Set(fields.keys) == keys, fields["membership_id"]?.textValue == membershipId,
              fields["state"]?.textValue == "ENROLLED", fields["revocation_class"]?.textValue == "",
              fields["revoked_at"]?.isNull == true, fields["data_mode"]?.textValue == "SYNTHETIC",
              let id = fields["id"]?.textValue, EnterpriseValues.isId(id),
              let lexeme = fields["revision"]?.integerLexeme, let revision = Int64(lexeme), revision > 0 else {
            throw EnterpriseProtocolFailure.invalid
        }
        return EnterpriseEnrollment(membershipId: membershipId, enrollmentId: id, revision: revision)
    }

    public static func enrolled(_ fields: [String: EnterpriseJson], operationId: String, membershipId: String) throws -> EnterpriseEnrollment {
        guard Set(fields.keys) == ["device", "operation_id", "reason", "replayed", "data_mode"],
              fields["operation_id"]?.textValue == operationId, fields["reason"]?.textValue == "",
              fields["data_mode"]?.textValue == "SYNTHETIC", fields["replayed"]?.boolValue != nil,
              let device = fields["device"]?.objectValue else {
            throw EnterpriseProtocolFailure.invalid
        }
        return try self.device(device, membershipId: membershipId)
    }

    /// The contact verdict bound to the request, account, origin, organization, membership, enrollment and family.
    public static func contact(_ fields: [String: EnterpriseJson], requestId: String, partition: EnterprisePartition,
                               enrollment: EnterpriseEnrollment, familyId: String) throws -> EnterpriseContactOutcome {
        let common: Set<String> = ["schema_version", "request_id", "account_id", "api_origin", "org_id", "membership_id",
                                   "enrollment_id", "family_id", "enrollment_revision", "server_time", "verdict"]
        guard fields["schema_version"] == EnterpriseJson.int(1), fields["request_id"]?.textValue == requestId,
              fields["account_id"]?.textValue == partition.accountId, fields["api_origin"]?.textValue == partition.origin,
              fields["org_id"]?.textValue == partition.organizationId, fields["membership_id"]?.textValue == enrollment.membershipId,
              fields["enrollment_id"]?.textValue == enrollment.enrollmentId, fields["family_id"]?.textValue == familyId,
              let rawRevision = fields["enrollment_revision"]?.textValue, isRevisionText(rawRevision),
              let revision = Int64(rawRevision), revision >= enrollment.revision,
              let serverTime = fields["server_time"]?.textValue, EnterpriseValues.isCanonicalInstant(serverTime),
              let verdict = fields["verdict"]?.textValue else {
            throw EnterpriseProtocolFailure.invalid
        }
        let bound = EnterpriseEnrollment(membershipId: enrollment.membershipId, enrollmentId: enrollment.enrollmentId, revision: revision)
        let keys = Set(fields.keys)
        switch verdict {
        case "ACTIVE":
            guard keys == common.union(["access_model", "offline_validity_seconds", "managed_unlock_seconds"]),
                  fields["access_model"]?.textValue == "EXISTING_ACCOUNT",
                  fields["offline_validity_seconds"] == EnterpriseJson.int(2_592_000),
                  fields["managed_unlock_seconds"]?.isNull == true else {
                throw EnterpriseProtocolFailure.invalid
            }
            return EnterpriseContactOutcome(enrollment: bound, verdict: .active, serverTime: serverTime)
        case "ERASE":
            guard keys == common.union(["revocation_class"]), fields["revocation_class"]?.textValue == "LOST_OR_COMPROMISED" else {
                throw EnterpriseProtocolFailure.invalid
            }
            return EnterpriseContactOutcome(enrollment: bound, verdict: .erase, serverTime: serverTime)
        case "LOCK":
            guard keys == common.union(["revocation_class"]), fields["revocation_class"]?.textValue == "ACCESS_ENDED" else {
                throw EnterpriseProtocolFailure.invalid
            }
            return EnterpriseContactOutcome(enrollment: bound, verdict: .lock, serverTime: serverTime)
        default:
            throw EnterpriseProtocolFailure.invalid
        }
    }

    static func isRevisionText(_ value: String) -> Bool {
        let bytes = Array(value.utf8)
        guard let first = bytes.first, first >= 0x31, first <= 0x39, bytes.count <= 19 else { return false }
        return bytes.allSatisfy { $0 >= 0x30 && $0 <= 0x39 }
    }
}

// MARK: - Attachment session wire

public enum EnterpriseAttachmentStorage: String, Codable, Sendable {
    case uploading = "UPLOADING"
    case received = "RECEIVED"
    case stored = "STORED"
    case quarantined = "QUARANTINED"
    case failed = "FAILED"
    case replaced = "REPLACED"
    case deleted = "DELETED"

    /// The server holds the bytes; the local copy may go.
    public var holdsContent: Bool { self == .received || self == .stored }
}

public struct EnterpriseAttachmentSession: Equatable, Sendable {
    public let storage: EnterpriseAttachmentStorage
    public let storageRevision: Int64
    public let capability: String?
}

/// Signed attachment session (`POST /v1/orgs/{org_id}/sync/attachments`, contracts attachment-session-v1).
public enum EnterpriseAttachmentWire {
    public static let maxBytes = 5_242_880
    public static let mediaType = "image/jpeg"
    public static let capabilityHeader = "X-Abastevo-Upload-Capability"
    public static let targetKinds: Set<String> = ["EXPENSE", "FUELING", "METER_READING", "INCIDENT"]

    public struct Request: Equatable, Sendable {
        public let requestId: String
        public let familyId: String
        public let accessToken: String
        public let membershipId: String
        public let enrollmentId: String
        public let declarationId: String
        public let documentId: String
        public let linkId: String
        public let sha256: String
        public let bytes: Int64
        public let targetKind: String
        public let targetId: String
    }

    public static func requestBody(_ request: Request) throws -> Data {
        let ids = [request.requestId, request.familyId, request.membershipId, request.enrollmentId, request.declarationId,
                   request.documentId, request.linkId, request.targetId]
        guard ids.allSatisfy(EnterpriseValues.isId), Set([request.declarationId, request.documentId, request.linkId]).count == 3,
              EnterpriseSyncWire.isAccessToken(request.accessToken), request.sha256.utf8.count == 64,
              EnterpriseCompanyWire.isLowerHex(request.sha256), request.bytes >= 1, request.bytes <= Int64(maxBytes),
              targetKinds.contains(request.targetKind) else {
            throw EnterpriseProtocolFailure.invalid
        }
        let body = try EnterpriseCanonicalJson.encode(.object([
            "schema_version": .int(1),
            "request_id": .text(request.requestId),
            "family_id": .text(request.familyId),
            "access_token": .text(request.accessToken),
            "membership_id": .text(request.membershipId),
            "enrollment_id": .text(request.enrollmentId),
            "declaration_id": .text(request.declarationId),
            "document_id": .text(request.documentId),
            "kind": .text("RECEIPT"),
            "sha256": .text(request.sha256),
            "bytes": .int(request.bytes),
            "media_type": .text(mediaType),
            "link_id": .text(request.linkId),
            "target_kind": .text(request.targetKind),
            "target_id": .text(request.targetId),
        ]))
        if body.count > 4096 { throw EnterpriseProtocolFailure.limit }
        return body
    }

    public static func parseSession(_ data: Data, request: Request) throws -> EnterpriseAttachmentSession {
        if data.count > 8192 { throw EnterpriseProtocolFailure.limit }
        guard let fields = try EnterpriseCanonicalJson.parse(data, maxBytes: 8192).objectValue,
              Set(fields.keys) == ["schema_version", "request_id", "membership_id", "enrollment_id", "document_id", "storage",
                                   "storage_revision", "upload"],
              fields["schema_version"] == EnterpriseJson.int(1),
              let rawStorage = fields["storage"]?.textValue, let storage = EnterpriseAttachmentStorage(rawValue: rawStorage),
              let lexeme = fields["storage_revision"]?.integerLexeme, let revision = Int64(lexeme), revision >= 1,
              let upload = fields["upload"] else {
            throw EnterpriseProtocolFailure.invalid
        }
        var capability: String?
        if !upload.isNull {
            guard storage == .uploading, let grant = upload.objectValue, Set(grant.keys) == ["capability", "expires_at"],
                  let value = grant["capability"]?.textValue, !value.isEmpty, value.utf8.count <= 4096,
                  value.utf8.allSatisfy({ $0 >= 0x21 && $0 <= 0x7E }),
                  let expires = grant["expires_at"]?.textValue, EnterpriseValues.isCanonicalInstant(expires) else {
                throw EnterpriseProtocolFailure.invalid
            }
            capability = value
        }
        guard fields["request_id"]?.textValue == request.requestId, fields["membership_id"]?.textValue == request.membershipId,
              fields["enrollment_id"]?.textValue == request.enrollmentId, fields["document_id"]?.textValue == request.documentId else {
            throw EnterpriseProtocolFailure.binding
        }
        return EnterpriseAttachmentSession(storage: storage, storageRevision: revision, capability: capability)
    }

    /// The ingress answer: the same document and its storage state.
    public static func parseIngress(_ data: Data, documentId: String) throws -> EnterpriseAttachmentStorage {
        guard let fields = try EnterpriseCanonicalJson.parse(data, maxBytes: 16_384).objectValue,
              let document = fields["document"]?.objectValue, document["id"]?.textValue == documentId,
              let raw = document["statuses"]?.objectValue?["storage"]?.textValue,
              let storage = EnterpriseAttachmentStorage(rawValue: raw),
              let lexeme = document["revisions"]?.objectValue?["storage"]?.integerLexeme, let revision = Int64(lexeme), revision >= 1 else {
            throw EnterpriseProtocolFailure.invalid
        }
        return storage
    }

    /// Final, closed refusals of the session (B-BR-EMP-417); everything else is retried.
    public static func isFinalRefusal(status: Int, code: String?) -> Bool {
        switch (status, code ?? "") {
        case (403, "FORBIDDEN"), (409, "CONFLICT"), (409, "OPERATION_ID_REUSED"), (422, "VALIDATION_FAILED"): return true
        default: return false
        }
    }
}

// MARK: - Signed device plane client

/// Every company command goes through the device-signed transport; company cookies never carry device proof.
///
/// The account proof (`family_id`, `access_token`) travels inside the signed body,
/// as on Android. Responses are bound to the exact request before use.
public struct EnterpriseSyncClient: Sendable {
    public let transport: SignedRequestTransport
    let sessions: SignedSessionSource
    let uploads: SignedHttpExchange
    let newId: @Sendable () -> String

    public init(
        transport: SignedRequestTransport,
        sessions: SignedSessionSource,
        uploads: SignedHttpExchange,
        newId: @escaping @Sendable () -> String = { EnterpriseValues.newId() }
    ) {
        self.transport = transport
        self.sessions = sessions
        self.uploads = uploads
        self.newId = newId
    }

    public var origin: String { transport.origin }

    private func liveSession(for partition: EnterprisePartition) async throws -> PortableAuth.Session {
        guard partition.origin == origin else { throw EnterpriseSyncError.unavailable }
        guard let session = await sessions.liveSession(), session.accountId == partition.accountId,
              EnterpriseSyncWire.isAccessToken(session.accessToken) else {
            throw EnterpriseSyncError.signInRequired
        }
        return session
    }

    private func post(_ path: String, body: Data) async throws -> SignedHttpResponse {
        do {
            return try await transport.signedRequest(method: "POST", path: path, body: body)
        } catch {
            throw EnterpriseSyncError(error)
        }
    }

    private static func proofBody(_ session: PortableAuth.Session, extra: [String: EnterpriseJson]) throws -> Data {
        var fields = extra
        fields["family_id"] = .text(session.familyId)
        fields["access_token"] = .text(session.accessToken)
        return try EnterpriseCanonicalJson.encode(.object(fields), maxDepth: 4)
    }

    /// Discovers or bootstrap-enrolls this device for the membership, then asks the exact contact verdict.
    public func confirmDevice(partition: EnterprisePartition, membershipId: String, known: EnterpriseEnrollment?) async throws -> EnterpriseContactOutcome {
        guard EnterpriseValues.isId(membershipId) else { throw EnterpriseSyncError.unavailable }
        let session = try await liveSession(for: partition)
        let prefix = "/v1/orgs/\(partition.organizationId)/devices/"
        let enrollment: EnterpriseEnrollment
        if let known, known.membershipId == membershipId {
            enrollment = known
        } else {
            enrollment = try await discover(prefix: prefix, session: session, membershipId: membershipId)
        }
        let requestId = newId()
        let body = try Self.proofBody(session, extra: [
            "schema_version": .int(1),
            "request_id": .text(requestId),
            "membership_id": .text(membershipId),
            "enrollment_id": .text(enrollment.enrollmentId),
        ])
        let response = try await post(prefix + "contact", body: body)
        do {
            return try EnterpriseDeviceWire.contact(try EnterpriseDeviceWire.object(response.body), requestId: requestId,
                                                    partition: partition, enrollment: enrollment, familyId: session.familyId)
        } catch {
            throw EnterpriseSyncError.invalidResponse
        }
    }

    private func discover(prefix: String, session: PortableAuth.Session, membershipId: String) async throws -> EnterpriseEnrollment {
        let proof = try Self.proofBody(session, extra: [:])
        do {
            let checked = try await post(prefix + "check", body: proof)
            do {
                return try EnterpriseDeviceWire.device(try EnterpriseDeviceWire.object(checked.body), membershipId: membershipId)
            } catch {
                throw EnterpriseSyncError.invalidResponse
            }
        } catch EnterpriseSyncError.refused(let status, _, _) where status == 404 {
            // Absence authorizes only the source-owned bootstrap enrollment of this device.
        }
        let operationId = newId()
        let body = try Self.proofBody(session, extra: [
            "schema_version": .int(1),
            "operation_id": .text(operationId),
            "label": .text(EnterpriseDeviceWire.label),
        ])
        let enrolled = try await post(prefix + "enroll", body: body)
        do {
            return try EnterpriseDeviceWire.enrolled(try EnterpriseDeviceWire.object(enrolled.body), operationId: operationId,
                                                     membershipId: membershipId)
        } catch {
            throw EnterpriseSyncError.invalidResponse
        }
    }

    /// Pulls the member's reference feed and applies it to the prior cache.
    public func pullReference(partition: EnterprisePartition, enrollment: EnterpriseEnrollment, prior: EnterpriseReferenceCache?,
                              nowMillis: Int64) async throws -> EnterpriseReferenceCache {
        let session = try await liveSession(for: partition)
        let held = prior.flatMap { cache -> EnterpriseReferenceCache? in
            cache.membershipId == enrollment.membershipId && cache.enrollmentId == enrollment.enrollmentId ? cache : nil
        }
        let requestId = newId()
        let body = try EnterpriseReferenceWire.requestBody(
            requestId: requestId, familyId: session.familyId, accessToken: session.accessToken,
            membershipId: enrollment.membershipId, enrollmentId: enrollment.enrollmentId, cursor: held?.cursor
        )
        let response = try await post("/v1/orgs/\(partition.organizationId)/sync/reference", body: body)
        do {
            let parsed = try EnterpriseReferenceWire.parseResponse(
                response.body, requestId: requestId, membershipId: enrollment.membershipId, enrollmentId: enrollment.enrollmentId,
                sentCursor: held?.cursor, heldRevision: held?.revision
            )
            return try EnterpriseReferenceWire.apply(prior: held, response: parsed, receivedAtMillis: nowMillis)
        } catch {
            throw EnterpriseSyncError.invalidResponse
        }
    }

    /// Delivers one batch of the partition's own operations and returns the bound acknowledgements.
    public func deliver(partition: EnterprisePartition, operations: [EnterpriseSealedOperation]) async throws -> [EnterpriseAcknowledgement] {
        let session = try await liveSession(for: partition)
        let body: Data
        do {
            body = try EnterpriseSyncWire.batchBody(familyId: session.familyId, accessToken: session.accessToken, operations: operations)
        } catch {
            throw EnterpriseSyncError.unavailable
        }
        let response = try await post("/v1/orgs/\(partition.organizationId)/sync/operations", body: body)
        do {
            return try EnterpriseSyncWire.parseBatchResponse(response.body, expected: operations)
        } catch {
            throw EnterpriseSyncError.invalidResponse
        }
    }

    /// Opens the signed attachment session for one saved receipt.
    public func openAttachment(partition: EnterprisePartition, enrollment: EnterpriseEnrollment, declarationId: String,
                               documentId: String, linkId: String, sha256: String, bytes: Int64, targetKind: String,
                               targetId: String) async throws -> EnterpriseAttachmentSession {
        let session = try await liveSession(for: partition)
        let request = EnterpriseAttachmentWire.Request(
            requestId: newId(), familyId: session.familyId, accessToken: session.accessToken,
            membershipId: enrollment.membershipId, enrollmentId: enrollment.enrollmentId, declarationId: declarationId,
            documentId: documentId, linkId: linkId, sha256: sha256, bytes: bytes, targetKind: targetKind, targetId: targetId
        )
        let body: Data
        do {
            body = try EnterpriseAttachmentWire.requestBody(request)
        } catch {
            throw EnterpriseSyncError.unavailable
        }
        let response = try await post("/v1/orgs/\(partition.organizationId)/sync/attachments", body: body)
        do {
            return try EnterpriseAttachmentWire.parseSession(response.body, request: request)
        } catch {
            throw EnterpriseSyncError.invalidResponse
        }
    }

    /// The capability-bound content ingress: no cookie, signature or Origin; the capability is the only proof.
    public func upload(partition: EnterprisePartition, documentId: String, capability: String, content: Data) async throws -> EnterpriseAttachmentStorage {
        guard partition.origin == origin, EnterpriseValues.isId(documentId), !content.isEmpty,
              content.count <= EnterpriseAttachmentWire.maxBytes,
              let url = URL(string: origin + "/v1/orgs/\(partition.organizationId)/documents/\(documentId)/content/upload") else {
            throw EnterpriseSyncError.unavailable
        }
        var request = URLRequest(url: url)
        request.httpMethod = "POST"
        request.httpShouldHandleCookies = false
        request.httpBody = content
        request.setValue(EnterpriseAttachmentWire.mediaType, forHTTPHeaderField: "Content-Type")
        request.setValue("application/json", forHTTPHeaderField: "Accept")
        request.setValue(capability, forHTTPHeaderField: EnterpriseAttachmentWire.capabilityHeader)
        let result: (Data, HTTPURLResponse)
        do {
            result = try await uploads.send(request)
        } catch {
            throw EnterpriseSyncError.offline
        }
        let (data, http) = result
        guard http.statusCode == 200 else {
            throw EnterpriseSyncError.refused(
                status: http.statusCode,
                code: SignedRequestProfile.enterpriseErrorCode(from: data),
                retryAfterSeconds: SignedRequestProfile.retryAfterSeconds(
                    http.value(forHTTPHeaderField: "Retry-After"),
                    nowEpochSeconds: Int64(Date().timeIntervalSince1970)
                )
            )
        }
        do {
            return try EnterpriseAttachmentWire.parseIngress(data, documentId: documentId)
        } catch {
            throw EnterpriseSyncError.invalidResponse
        }
    }
}

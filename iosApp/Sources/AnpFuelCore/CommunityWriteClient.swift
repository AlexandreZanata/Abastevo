import Foundation
#if canImport(FoundationNetworking)
import FoundationNetworking
#endif

public struct ObservationReceipt: Equatable, Sendable {
    public let id: String
    public let validationState: String
    public let receivedAt: String
}

/// One owned observation from `GET /v1/observations/{id}` or `GET /v1/contributors/me/observations`.
public struct OwnObservation: Equatable, Identifiable, Sendable {
    public let id: String
    public let stationId: String
    public let fuelWire: String
    public let unit: String
    public let amountMilli: Int64
    public let conditionKind: String
    public let validationState: String
    public let receivedAt: String

    public var product: FuelProduct? { FuelProduct.fromWireCode(fuelWire) }
}

public struct OwnObservationsPage: Equatable, Sendable {
    public let items: [OwnObservation]
    public let nextCursor: String?
}

public struct ForumWriteReceipt: Equatable, Sendable {
    public let id: String
    public let revision: Int64
    public let publicationState: String
    public let visibility: String
    public let replayed: Bool
}

public struct StationCommentReceipt: Equatable, Sendable {
    public let id: String
    public let revision: Int64
}

public struct StationCommentTally: Equatable, Sendable {
    public let commentId: String
    public let revision: Int64
    public let valid: Int64
    public let invalid: Int64
}

/// Server authorization to take one evidence photo: the camera must fire before `cameraExpiresAtMillis`.
public struct PhotoCaptureGrant: Codable, Equatable, Sendable {
    public let captureId: String
    public let stationId: String
    public let issuedAtMillis: Int64
    public let cameraExpiresAtMillis: Int64
    public let expiresAtMillis: Int64

    public init(captureId: String, stationId: String, issuedAtMillis: Int64, cameraExpiresAtMillis: Int64, expiresAtMillis: Int64) {
        self.captureId = captureId
        self.stationId = stationId
        self.issuedAtMillis = issuedAtMillis
        self.cameraExpiresAtMillis = cameraExpiresAtMillis
        self.expiresAtMillis = expiresAtMillis
    }

    /// A photo taken at `capturedAtMillis` for `stationId` can become evidence under this grant.
    public func admits(stationId candidate: String, capturedAtMillis: Int64) -> Bool {
        return candidate.lowercased() == stationId.lowercased()
            && capturedAtMillis < cameraExpiresAtMillis
            && capturedAtMillis >= issuedAtMillis - CommunityWire.issuedAtLeewayMillis
    }
}

public struct UploadAuthorization: Equatable, Sendable {
    public let uploadId: String
    public let url: URL
    public let contentTypeHeader: String
    public let expiresAtMillis: Int64
    public let maxBytes: Int64
    public let sessionExpiresAtMillis: Int64
}

public struct UploadStatus: Equatable, Sendable {
    public let uploadId: String
    public let state: String
    public let evidenceId: String?
}

/// Wire shapes of the community, media, feedback and forum write routes, checked against `contracts/testdata/api`.
/// Zero postfix unwrap operators anywhere in this file.
public enum CommunityWire {
    public static let observationsPath = "/v1/observations"
    public static let ownObservationsPath = "/v1/contributors/me/observations"
    public static let photoCapturesPath = "/v1/photo-captures"
    public static let uploadsPath = "/v1/uploads"
    public static let stationCommentsPath = "/v1/feedback/comments"
    public static let forumTopicsPath = "/v1/forum/topics"
    public static let photoCapturePolicy = "photo-capture-community-v2"
    public static let issuedAtLeewayMillis: Int64 = 60_000
    public static let uploadUrlTtlMillis: Int64 = 300_000
    public static let maxTopicTitleScalars = 120
    public static let maxTopicBodyScalars = 4000
    public static let maxAmountMilli: Int64 = 1_000_000
    public static let voteValid = "VALID"
    public static let voteInvalid = "INVALID"

    /// Backend conditions: STANDARD, CASH, DEBIT, CREDIT, APP, LOYALTY and OTHER. Pix has no wire value yet.
    public static func conditionWire(_ condition: PriceConditionKind) -> String? {
        switch condition {
        case .standard: return "STANDARD"
        case .cash: return "CASH"
        case .debit: return "DEBIT"
        case .credit: return "CREDIT"
        case .app: return "APP"
        case .loyalty: return "LOYALTY"
        case .pix: return nil
        }
    }

    // MARK: Observations

    public static func observationBody(clientSubmissionId: String, price: CommunityPriceDraft) -> Data? {
        guard !clientSubmissionId.isEmpty, clientSubmissionId.count <= 128,
              isUuid(price.stationId),
              let condition = conditionWire(price.condition),
              price.amountMilli >= 1, price.amountMilli <= maxAmountMilli else {
            return nil
        }
        let money: [String: Any] = [
            "amount_milli_brl": NSNumber(value: price.amountMilli),
            "currency": "BRL",
            "unit": price.product.unitWire,
        ]
        var object: [String: Any] = [
            "client_submission_id": clientSubmissionId,
            "station_id": price.stationId,
            "fuel_product": price.product.wireCode,
            "price": money,
            "condition": ["kind": condition],
            "claimed_captured_at": formatRfc3339Millis(price.capturedAtMillis),
        ]
        if let photo = price.photo {
            guard price.condition == .standard,
                  let evidenceId = photo.evidenceId, isUuid(evidenceId),
                  isUuid(photo.captureId) else {
                return nil
            }
            object["evidence_id"] = evidenceId
            object["photo_capture_id"] = photo.captureId
            object["claimed_captured_at"] = formatRfc3339Millis(photo.capturedAtMillis)
        }
        return DeviceIdentityWire.encode(object)
    }

    /// Intake acknowledges only RECEIVED; anything else is not a receipt.
    public static func decodeObservationReceipt(_ data: Data) -> ObservationReceipt? {
        guard let json = DeviceIdentityWire.object(data),
              let id = json["id"] as? String, isUuid(id),
              let state = json["validation_state"] as? String, state == "RECEIVED" else {
            return nil
        }
        return ObservationReceipt(id: id, validationState: state, receivedAt: json["received_at"] as? String ?? "")
    }

    public static func decodeObservationStatus(_ data: Data) -> OwnObservation? {
        guard let json = DeviceIdentityWire.object(data),
              let row = json["observation"] as? [String: Any],
              let parsed = observation(from: row),
              let state = json["validation_state"] as? String,
              state == parsed.validationState else {
            return nil
        }
        return parsed
    }

    public static func decodeOwnObservations(_ data: Data) -> OwnObservationsPage? {
        guard let json = DeviceIdentityWire.object(data),
              let rows = json["items"] as? [[String: Any]],
              rows.count <= 100 else {
            return nil
        }
        var items: [OwnObservation] = []
        for row in rows {
            guard let item = observation(from: row) else { return nil }
            items.append(item)
        }
        return OwnObservationsPage(items: items, nextCursor: json["next_cursor"] as? String)
    }

    static func observation(from row: [String: Any]) -> OwnObservation? {
        let knownStates: Set<String> = ["RECEIVED", "VALIDATING", "VALIDATED", "REJECTED"]
        guard let id = row["id"] as? String, isUuid(id),
              let stationId = row["station_id"] as? String, isUuid(stationId),
              let fuel = row["fuel_product"] as? String, !fuel.isEmpty,
              let unit = row["unit"] as? String,
              let amount = row["amount_milli_brl"] as? Int64, amount > 0,
              let condition = row["condition"] as? [String: Any],
              let kind = condition["kind"] as? String,
              let state = row["validation_state"] as? String, knownStates.contains(state) else {
            return nil
        }
        return OwnObservation(
            id: id,
            stationId: stationId,
            fuelWire: fuel,
            unit: unit,
            amountMilli: amount,
            conditionKind: kind,
            validationState: state,
            receivedAt: row["received_at"] as? String ?? ""
        )
    }

    // MARK: Photo capture and media

    public static func photoCaptureBody(clientCaptureId: String, stationId: String) -> Data? {
        guard isUuid(clientCaptureId), isUuid(stationId) else { return nil }
        return DeviceIdentityWire.encode([
            "client_capture_id": clientCaptureId,
            "station_id": stationId.lowercased(),
        ])
    }

    public static func decodePhotoCaptureGrant(_ data: Data, stationId: String, nowMillis: Int64) -> PhotoCaptureGrant? {
        guard let json = DeviceIdentityWire.object(data),
              json["policy_version"] as? String == photoCapturePolicy,
              let captureId = json["capture_id"] as? String, isUuid(captureId),
              let station = json["station_id"] as? String,
              station.lowercased() == stationId.lowercased(),
              let issued = millis(json["issued_at"]),
              let camera = millis(json["camera_expires_at"]),
              let expires = millis(json["expires_at"]) else {
            return nil
        }
        let cameraWindow = camera - issued
        let window = expires - issued
        guard cameraWindow >= 1, cameraWindow <= 120_000,
              window >= 1, window <= 86_400_000,
              expires >= camera,
              issued - nowMillis <= issuedAtLeewayMillis,
              nowMillis < camera else {
            return nil
        }
        return PhotoCaptureGrant(
            captureId: captureId,
            stationId: station,
            issuedAtMillis: issued,
            cameraExpiresAtMillis: camera,
            expiresAtMillis: expires
        )
    }

    public static func uploadReserveBody(
        captureId: String,
        stationId: String,
        sizeBytes: Int,
        sha256Hex: String,
        capturedAtMillis: Int64
    ) -> Data? {
        guard isUuid(captureId), isUuid(stationId), sizeBytes > 0, sha256Hex.count == 64 else { return nil }
        return DeviceIdentityWire.encode([
            "client_submission_id": "photo:" + captureId,
            "content_type": PortablePhoto.wireMime,
            "size_bytes": NSNumber(value: sizeBytes),
            "sha256": sha256Hex,
            "photo_capture_id": captureId,
            "station_id": stationId,
            "captured_at": formatRfc3339Millis(capturedAtMillis),
        ])
    }

    /// Refuses any authorization that is not a short-lived HTTPS PUT with exactly one JPEG content-type header.
    public static func decodeUploadAuthorization(
        _ data: Data,
        nowMillis: Int64,
        sessionDeadlineMillis: Int64,
        byteCount: Int
    ) -> UploadAuthorization? {
        guard let json = DeviceIdentityWire.object(data),
              let uploadId = json["upload_id"] as? String, isUuid(uploadId),
              json["method"] as? String == "PUT",
              let urlText = json["url"] as? String, urlText.count <= 8192,
              let url = URL(string: urlText),
              url.scheme?.lowercased() == "https",
              let host = url.host, !host.isEmpty,
              url.user == nil, url.password == nil, url.fragment == nil,
              let query = url.query, !query.isEmpty,
              let expires = millis(json["expires_at"]),
              let session = millis(json["session_expires_at"]),
              let maxBytes = json["max_bytes"] as? Int64,
              let headers = json["required_headers"] as? [String: Any],
              headers.count == 1,
              let header = headers.first,
              header.key.lowercased() == "content-type",
              header.value as? String == PortablePhoto.wireMime else {
            return nil
        }
        guard expires > nowMillis,
              expires <= nowMillis + uploadUrlTtlMillis + issuedAtLeewayMillis,
              expires <= session,
              session > nowMillis,
              session <= sessionDeadlineMillis,
              maxBytes >= Int64(byteCount) else {
            return nil
        }
        return UploadAuthorization(
            uploadId: uploadId,
            url: url,
            contentTypeHeader: header.key,
            expiresAtMillis: expires,
            maxBytes: maxBytes,
            sessionExpiresAtMillis: session
        )
    }

    public static func decodeUploadStatus(_ data: Data) -> UploadStatus? {
        guard let json = DeviceIdentityWire.object(data),
              let uploadId = json["upload_id"] as? String, isUuid(uploadId),
              let state = json["state"] as? String, !state.isEmpty else {
            return nil
        }
        return UploadStatus(uploadId: uploadId, state: state, evidenceId: json["evidence_id"] as? String)
    }

    // MARK: Station comments (session-only routes)

    public static func stationCommentBody(session: PortableAuth.Session, stationId: String, productWire: String, text: String) -> Data? {
        let normalized = PortableText.normalize(text)
        guard isUuid(stationId), !productWire.isEmpty, PortableText.isValidComment(normalized) else { return nil }
        return DeviceIdentityWire.encode([
            "family_id": session.familyId,
            "access_token": session.accessToken,
            "station_id": stationId,
            "product": productWire,
            "text": normalized,
        ])
    }

    public static func decodeStationCommentReceipt(_ data: Data) -> StationCommentReceipt? {
        guard let json = DeviceIdentityWire.object(data),
              let comment = json["comment"] as? [String: Any],
              let id = comment["id"] as? String, !id.isEmpty,
              let revision = comment["revision"] as? Int64, revision >= 1 else {
            return nil
        }
        return StationCommentReceipt(id: id, revision: revision)
    }

    public static func voteBody(session: PortableAuth.Session, choice: String) -> Data? {
        guard choice == voteValid || choice == voteInvalid else { return nil }
        return DeviceIdentityWire.encode([
            "family_id": session.familyId,
            "access_token": session.accessToken,
            "choice": choice,
        ])
    }

    public static func decodeTally(_ data: Data) -> StationCommentTally? {
        guard let json = DeviceIdentityWire.object(data),
              let tally = json["tally"] as? [String: Any],
              let commentId = tally["comment_id"] as? String, !commentId.isEmpty,
              let revision = tally["revision"] as? Int64, revision >= 1,
              let valid = tally["valid"] as? Int64, valid >= 0,
              let invalid = tally["invalid"] as? Int64, invalid >= 0 else {
            return nil
        }
        return StationCommentTally(commentId: commentId, revision: revision, valid: valid, invalid: invalid)
    }

    // MARK: Forum (session plus signed device proof)

    public static func isValidTopic(title: String, body: String) -> Bool {
        let cleanTitle = title.trimmingCharacters(in: .whitespacesAndNewlines)
        let cleanBody = body.trimmingCharacters(in: .whitespacesAndNewlines)
        return !cleanTitle.isEmpty && cleanTitle.unicodeScalars.count <= maxTopicTitleScalars
            && !cleanBody.isEmpty && cleanBody.unicodeScalars.count <= maxTopicBodyScalars
            && !cleanTitle.unicodeScalars.contains("\u{0}") && !cleanBody.unicodeScalars.contains("\u{0}")
    }

    public static func forumTopicBody(session: PortableAuth.Session, operationId: String, draft: CommunityTextDraft) -> Data? {
        guard isUuid(operationId),
              let category = draft.categoryCode, !category.isEmpty,
              let title = draft.title,
              isValidTopic(title: title, body: draft.text) else {
            return nil
        }
        var object: [String: Any] = [
            "family_id": session.familyId,
            "access_token": session.accessToken,
            "operation_id": operationId,
            "category": category,
            "title": title.trimmingCharacters(in: .whitespacesAndNewlines),
            "body_text": draft.text.trimmingCharacters(in: .whitespacesAndNewlines),
        ]
        if let uf = draft.regionUf, uf.count == 2 {
            object["region_uf"] = uf
            if let code = draft.regionMunicipalityIbge, code >= 1_000_000, code <= 9_999_999 {
                object["region_municipality_ibge"] = NSNumber(value: code)
            }
        }
        return DeviceIdentityWire.encode(object)
    }

    public static func forumCommentBody(session: PortableAuth.Session, operationId: String, draft: CommunityTextDraft) -> Data? {
        let normalized = PortableText.normalize(draft.text)
        guard isUuid(operationId),
              let postId = draft.postId, isUuid(postId),
              PortableText.isValidComment(normalized),
              !normalized.unicodeScalars.contains("\u{0}") else {
            return nil
        }
        var object: [String: Any] = [
            "family_id": session.familyId,
            "access_token": session.accessToken,
            "operation_id": operationId,
            "body_text": normalized,
        ]
        if let parent = draft.parentId {
            guard isUuid(parent) else { return nil }
            object["parent_id"] = parent
        }
        return DeviceIdentityWire.encode(object)
    }

    /// Topic and comment receipts arrive at the top level or under `wrapper`, as Android `ForumMapper` accepts.
    public static func decodeForumReceipt(_ data: Data, wrapper: String) -> ForumWriteReceipt? {
        guard let json = DeviceIdentityWire.object(data) else { return nil }
        let row = json[wrapper] as? [String: Any] ?? json
        guard let id = row["id"] as? String, isUuid(id),
              let revision = row["revision"] as? Int64, revision >= 1,
              let state = row["publication_state"] as? String, !state.isEmpty else {
            return nil
        }
        return ForumWriteReceipt(
            id: id,
            revision: revision,
            publicationState: state,
            visibility: row["visibility"] as? String ?? "",
            replayed: row["replayed"] as? Bool ?? false
        )
    }

    // MARK: Values

    public static func isUuid(_ text: String) -> Bool {
        return text.count == 36 && UUID(uuidString: text) != nil
    }

    static func millis(_ value: Any?) -> Int64? {
        guard let text = value as? String else { return nil }
        return parseRfc3339Millis(text)
    }

    /// UTC RFC 3339 with milliseconds, identical for the upload reserve and the observation.
    public static func formatRfc3339Millis(_ millis: Int64) -> String {
        let seconds = millis >= 0 ? millis / 1000 : (millis - 999) / 1000
        let fraction = Int(millis - seconds * 1000)
        let formatter = ISO8601DateFormatter()
        formatter.formatOptions = [.withInternetDateTime]
        let base = formatter.string(from: Date(timeIntervalSince1970: TimeInterval(seconds)))
        guard base.hasSuffix("Z") else { return base }
        let digits = String(fraction)
        let padded = String(repeating: "0", count: max(0, 3 - digits.count)) + digits
        return String(base.dropLast()) + "." + padded + "Z"
    }

    /// Parses RFC 3339 with or without a fraction of any length (Go emits up to nine digits).
    public static func parseRfc3339Millis(_ text: String) -> Int64? {
        var base = text
        var fractionMillis: Int64 = 0
        if let dot = text.firstIndex(of: ".") {
            let afterDot = text[text.index(after: dot)...]
            let digits = afterDot.prefix(while: { $0 >= "0" && $0 <= "9" })
            guard !digits.isEmpty else { return nil }
            let zone = afterDot.dropFirst(digits.count)
            base = String(text[..<dot]) + String(zone)
            let firstThree = String(digits.prefix(3))
            let padded = firstThree + String(repeating: "0", count: 3 - firstThree.count)
            fractionMillis = Int64(padded) ?? 0
        }
        let formatter = ISO8601DateFormatter()
        formatter.formatOptions = [.withInternetDateTime]
        guard let date = formatter.date(from: base) else { return nil }
        return Int64(date.timeIntervalSince1970.rounded()) * 1000 + fractionMillis
    }
}

extension IosTransientPhotoCache: @unchecked Sendable {}

/// URLSession gateway for queued community writes and owner reads.
public final class CommunityWriteClient: CommunityWriteGateway, @unchecked Sendable {
    public let transport: SignedRequestTransport
    private let exchange: SignedHttpExchange
    private let sessions: SignedSessionSource
    private let photoBytes: @Sendable (String) -> Data?
    private let nowMillis: @Sendable () -> Int64

    public init(
        transport: SignedRequestTransport,
        exchange: SignedHttpExchange,
        sessions: SignedSessionSource,
        photoBytes: @escaping @Sendable (String) -> Data?,
        nowMillis: @escaping @Sendable () -> Int64 = { Int64(Date().timeIntervalSince1970 * 1000) }
    ) {
        self.transport = transport
        self.exchange = exchange
        self.sessions = sessions
        self.photoBytes = photoBytes
        self.nowMillis = nowMillis
    }

    func ownerSession(
        expectedScope: String, expectedFamilyId: String? = nil
    ) async throws -> PortableAuth.Session {
        guard expectedScope.hasPrefix("account:") else { throw CommunityWriteError.signInRequired }
        try transport.requireOwnerScope(expectedScope)
        guard let session = await sessions.liveSession(),
              expectedScope.hasPrefix("account:\(session.accountId):"),
              expectedFamilyId == nil || session.familyId == expectedFamilyId else {
            throw CommunityWriteError.signInRequired
        }
        try transport.requireOwnerScope(expectedScope)
        return session
    }

    func signedOwnerGet(
        path: String, expectedScope: String, query: String = "", expectedFamilyId: String? = nil
    ) async throws -> SignedHttpResponse {
        let session = try await ownerSession(expectedScope: expectedScope, expectedFamilyId: expectedFamilyId)
        return try await transport.signedRequest(
            method: "GET", path: path, query: query, body: nil, expectedOwnerScope: expectedScope,
            expectedSessionFamilyId: session.familyId
        )
    }

    func ownerPost(
        path: String,
        expectedScope: String,
        requiresDeviceProof: Bool,
        idempotencyKey: String? = nil,
        requiresSameSessionFamily: Bool = false,
        buildBody: (PortableAuth.Session) -> Data?
    ) async throws -> SignedHttpResponse {
        let session = try await ownerSession(expectedScope: expectedScope)
        guard let body = buildBody(session) else { throw CommunityWriteError.invalid("community.invalid-request") }
        if requiresDeviceProof {
            return try await transport.signedRequest(
                method: "POST", path: path, body: body, idempotencyKey: idempotencyKey, expectedOwnerScope: expectedScope,
                expectedSessionFamilyId: requiresSameSessionFamily ? session.familyId : nil
            )
        }
        return try await transport.plainPost(
            path: path, body: body, idempotencyKey: idempotencyKey, expectedOwnerScope: expectedScope,
            expectedSessionFamilyId: requiresSameSessionFamily ? session.familyId : nil
        )
    }

    // MARK: Price observations and evidence

    public func authorizePhotoCapture(stationId: String, clientCaptureId: String) async throws -> PhotoCaptureGrant {
        guard let body = CommunityWire.photoCaptureBody(clientCaptureId: clientCaptureId, stationId: stationId) else {
            throw CommunityWriteError.invalid("photo.capture-invalid")
        }
        let response = try await transport.signedRequest(
            method: "POST",
            path: CommunityWire.photoCapturesPath,
            body: body,
            idempotencyKey: clientCaptureId
        )
        guard let grant = CommunityWire.decodePhotoCaptureGrant(response.body, stationId: stationId, nowMillis: nowMillis()) else {
            throw CommunityWriteError.invalidReceipt
        }
        return grant
    }

    public func uploadEvidence(_ item: CommunityOutboxItem) async throws -> EvidenceProgress {
        guard let price = item.price, let photo = price.photo else {
            throw CommunityWriteError.invalid("contribution.invalid")
        }
        guard CommunityOutbox.isPhotoUsable(photo, nowMillis: nowMillis()) else {
            throw CommunityWriteError.photoExpired
        }
        if let saved = photo.uploadSessionId {
            guard CommunityWire.isUuid(saved) else { throw CommunityWriteError.invalidReceipt }
            let response = try await transport.signedRequest(method: "GET", path: CommunityWire.uploadsPath + "/" + saved, body: nil)
            guard let status = CommunityWire.decodeUploadStatus(response.body), status.uploadId == saved else {
                throw CommunityWriteError.invalidReceipt
            }
            if let progress = try Self.progress(from: status) {
                return progress
            }
        }
        guard let bytes = photoBytes(photo.photoId) else { throw CommunityWriteError.photoExpired }
        guard PortablePhoto.fitsWireCap(Int64(bytes.count)) else {
            throw CommunityWriteError.invalid("contribution.photo-size")
        }
        guard let reserveBody = CommunityWire.uploadReserveBody(
            captureId: photo.captureId,
            stationId: price.stationId,
            sizeBytes: bytes.count,
            sha256Hex: IosDeviceProofSigner.sha256Hex(bytes),
            capturedAtMillis: photo.capturedAtMillis
        ) else {
            throw CommunityWriteError.invalid("contribution.invalid")
        }
        let reserve = try await transport.signedRequest(
            method: "POST",
            path: CommunityWire.uploadsPath,
            body: reserveBody,
            idempotencyKey: "photo:" + photo.captureId
        )
        guard let authorization = CommunityWire.decodeUploadAuthorization(
            reserve.body,
            nowMillis: nowMillis(),
            sessionDeadlineMillis: photo.captureExpiresAtMillis,
            byteCount: bytes.count
        ) else {
            throw CommunityWriteError.invalidReceipt
        }
        try await put(bytes, authorization: authorization)
        let complete = try await transport.signedRequest(
            method: "POST",
            path: CommunityWire.uploadsPath + "/" + authorization.uploadId + "/complete",
            body: Data("{}".utf8),
            idempotencyKey: "complete:" + photo.captureId
        )
        guard let status = CommunityWire.decodeUploadStatus(complete.body), status.uploadId == authorization.uploadId else {
            throw CommunityWriteError.invalidReceipt
        }
        if let progress = try Self.progress(from: status) {
            return progress
        }
        return EvidenceProgress(uploadSessionId: authorization.uploadId, evidenceId: nil)
    }

    static func progress(from status: UploadStatus) throws -> EvidenceProgress? {
        switch status.state {
        case "READY":
            guard let evidence = status.evidenceId, CommunityWire.isUuid(evidence) else {
                throw CommunityWriteError.invalidReceipt
            }
            return EvidenceProgress(uploadSessionId: status.uploadId, evidenceId: evidence)
        case "ISSUED":
            return nil
        case "VERIFYING":
            return EvidenceProgress(uploadSessionId: status.uploadId, evidenceId: nil)
        case "EXPIRED":
            throw CommunityWriteError.photoExpired
        case "REJECTED":
            throw CommunityWriteError.photoRejected
        default:
            throw CommunityWriteError.invalidReceipt
        }
    }

    private func put(_ bytes: Data, authorization: UploadAuthorization) async throws {
        var request = URLRequest(url: authorization.url)
        request.httpMethod = "PUT"
        request.httpBody = bytes
        request.setValue(PortablePhoto.wireMime, forHTTPHeaderField: authorization.contentTypeHeader)
        let result: (Data, HTTPURLResponse)
        do {
            result = try await exchange.send(request)
        } catch {
            throw SignedTransportError.offline
        }
        let status = result.1.statusCode
        guard status >= 200 && status < 300 else {
            throw CommunityWriteError.mediaPending
        }
    }

    public func submitObservation(_ item: CommunityOutboxItem) async throws -> CommunityWriteReceipt {
        guard let price = item.price,
              let body = CommunityWire.observationBody(clientSubmissionId: item.id, price: price) else {
            throw CommunityWriteError.invalid("contribution.invalid")
        }
        let response = try await transport.signedRequest(
            method: "POST",
            path: CommunityWire.observationsPath,
            body: body,
            idempotencyKey: item.id
        )
        guard let receipt = CommunityWire.decodeObservationReceipt(response.body) else {
            throw CommunityWriteError.invalidReceipt
        }
        return CommunityWriteReceipt(id: receipt.id, state: receipt.validationState)
    }

    public func observationStatus(id: String) async throws -> OwnObservation {
        guard CommunityWire.isUuid(id) else { throw CommunityWriteError.invalid("contribution.missing-receipt") }
        let response = try await transport.signedRequest(method: "GET", path: CommunityWire.observationsPath + "/" + id, body: nil)
        guard let observation = CommunityWire.decodeObservationStatus(response.body), observation.id == id else {
            throw CommunityWriteError.invalidReceipt
        }
        return observation
    }

    public func ownObservations(limit: Int = 50) async throws -> OwnObservationsPage {
        let bounded = max(1, min(100, limit))
        let response = try await transport.signedRequest(
            method: "GET",
            path: CommunityWire.ownObservationsPath,
            query: "limit=\(bounded)",
            body: nil
        )
        guard let page = CommunityWire.decodeOwnObservations(response.body) else {
            throw CommunityWriteError.invalidReceipt
        }
        return page
    }

    // MARK: Station comments

    public func submitStationComment(_ item: CommunityOutboxItem) async throws -> CommunityWriteReceipt {
        guard let draft = item.text, let stationId = draft.stationId, let product = draft.product else {
            throw CommunityWriteError.invalid("feedback.invalid")
        }
        guard let session = await sessions.liveSession() else { throw CommunityWriteError.signInRequired }
        guard let body = CommunityWire.stationCommentBody(
            session: session,
            stationId: stationId,
            productWire: product.wireCode,
            text: draft.text
        ) else {
            throw CommunityWriteError.invalid("feedback.text-invalid")
        }
        let response = try await transport.plainPost(path: CommunityWire.stationCommentsPath, body: body)
        guard let receipt = CommunityWire.decodeStationCommentReceipt(response.body) else {
            throw CommunityWriteError.invalidReceipt
        }
        return CommunityWriteReceipt(id: receipt.id, state: "PUBLISHED")
    }

    public func voteStationComment(commentId: String, valid: Bool) async throws -> StationCommentTally {
        guard CommunityWire.isUuid(commentId) else { throw CommunityWriteError.invalid("feedback.comment-not-found") }
        guard let session = await sessions.liveSession() else { throw CommunityWriteError.signInRequired }
        guard let body = CommunityWire.voteBody(
            session: session,
            choice: valid ? CommunityWire.voteValid : CommunityWire.voteInvalid
        ) else {
            throw CommunityWriteError.invalid("feedback.vote-choice-invalid")
        }
        let response = try await transport.plainPost(
            path: CommunityWire.stationCommentsPath + "/" + commentId + "/votes",
            body: body
        )
        guard let tally = CommunityWire.decodeTally(response.body), tally.commentId == commentId else {
            throw CommunityWriteError.invalidReceipt
        }
        return tally
    }

    // MARK: Forum

    public func submitForumTopic(_ item: CommunityOutboxItem) async throws -> CommunityWriteReceipt {
        guard let draft = item.text else { throw CommunityWriteError.invalid("forum.invalid") }
        _ = try await transport.ensureIdentity()
        guard let session = await sessions.liveSession() else { throw CommunityWriteError.signInRequired }
        guard let body = CommunityWire.forumTopicBody(session: session, operationId: item.id, draft: draft) else {
            throw CommunityWriteError.invalid("forum.invalid")
        }
        let response = try await transport.signedRequest(method: "POST", path: CommunityWire.forumTopicsPath, body: body)
        guard let receipt = CommunityWire.decodeForumReceipt(response.body, wrapper: "topic") else {
            throw CommunityWriteError.invalidReceipt
        }
        return CommunityWriteReceipt(id: receipt.id, state: receipt.publicationState)
    }

    public func submitForumComment(_ item: CommunityOutboxItem) async throws -> CommunityWriteReceipt {
        guard let draft = item.text, let postId = draft.postId, CommunityWire.isUuid(postId) else {
            throw CommunityWriteError.invalid("forum.invalid")
        }
        _ = try await transport.ensureIdentity()
        guard let session = await sessions.liveSession() else { throw CommunityWriteError.signInRequired }
        guard let body = CommunityWire.forumCommentBody(session: session, operationId: item.id, draft: draft) else {
            throw CommunityWriteError.invalid("forum.invalid")
        }
        let response = try await transport.signedRequest(
            method: "POST",
            path: "/v1/forum/posts/" + postId + "/comments",
            body: body
        )
        guard let receipt = CommunityWire.decodeForumReceipt(response.body, wrapper: "comment") else {
            throw CommunityWriteError.invalidReceipt
        }
        return CommunityWriteReceipt(id: receipt.id, state: receipt.publicationState)
    }
}

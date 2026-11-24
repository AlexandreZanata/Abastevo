import Foundation

public struct StationFeedbackPage: Decodable, Sendable {
    public let comments: [PublicStationComment]
    public let nextCursor: String?
}

public struct StationRatingStats: Decodable, Equatable, Sendable {
    public let stationId: String
    public let product: String
    public let count: Int64
    public let sum: Int64
    public let meanMilli: Int64?

    public var meanTenths: Int64? { meanMilli.map { $0 / 100 } }

    func matches(stationId: String, fuel: FuelProduct) -> Bool {
        guard self.stationId.lowercased() == stationId.lowercased(),
              product == fuel.wireCode,
              count >= 0, count <= Int64.max / 5_000,
              sum >= count, sum <= count * 5 else {
            return false
        }
        if count == 0 { return meanMilli == nil }
        return meanMilli == sum * 1_000 / count
    }
}

public struct StationRatingReceipt: Decodable, Equatable, Sendable {
    public struct Rating: Decodable, Equatable, Sendable {
        public let stationId: String
        public let product: String
        public let stars: Int
        public let revision: Int
        public let created: Bool
    }

    public let rating: Rating
    public let stats: StationRatingStats
}

enum StationFeedbackWire {
    static let ratingsPath = "/v1/feedback/ratings"

    struct RatingSummary: Decodable {
        let stats: StationRatingStats
    }

    struct CommentReceipt: Decodable {
        let comment: PublicStationComment
    }

    struct StatusReceipt: Decodable {
        let status: String
    }

    static func body(session: PortableAuth.Session, fields: [String: Any]) -> Data? {
        let envelope: [String: Any] = [
            "family_id": session.familyId,
            "access_token": session.accessToken,
        ]
        return DeviceIdentityWire.encode(envelope.merging(fields) { original, _ in original })
    }

    static func commentBody(session: PortableAuth.Session, text: String, expectedRevision: Int64? = nil) -> Data? {
        let normalized = PortableText.normalize(text)
        guard PortableText.isValidComment(normalized) else { return nil }
        var fields: [String: Any] = ["text": normalized]
        if let expectedRevision {
            guard expectedRevision > 0 else { return nil }
            fields["expected_revision"] = expectedRevision
        }
        return body(session: session, fields: fields)
    }

    static func decodePage(_ data: Data, stationId: String, fuel: FuelProduct, parentId: String?) throws -> StationFeedbackPage {
        let page = try AbastevoHttpClient.publicDecoder().decode(StationFeedbackPage.self, from: data)
        guard page.comments.allSatisfy({ $0.matches(stationId: stationId, fuel: fuel, parentId: parentId) }),
              page.nextCursor?.isEmpty != true else {
            throw HttpClientError.invalidJson
        }
        return page
    }

    static func decodeRating(_ data: Data, stationId: String, fuel: FuelProduct, stars: Int) throws -> StationRatingReceipt {
        let receipt = try AbastevoHttpClient.publicDecoder().decode(StationRatingReceipt.self, from: data)
        guard receipt.rating.stationId.lowercased() == stationId.lowercased(),
              receipt.rating.product == fuel.wireCode, receipt.rating.stars == stars,
              receipt.rating.revision > 0, receipt.stats.matches(stationId: stationId, fuel: fuel) else {
            throw CommunityWriteError.invalidReceipt
        }
        return receipt
    }
}

extension PublicStationComment {
    func matches(stationId: String, fuel: FuelProduct, parentId: String?) -> Bool {
        CommunityWire.isUuid(id) && self.stationId.lowercased() == stationId.lowercased()
            && product == fuel.wireCode && self.parentId?.lowercased() == parentId?.lowercased()
            && (depth ?? 0) == (parentId == nil ? 0 : 1)
            && (revision ?? 1) > 0 && PortableText.isValidComment(text)
    }
}

extension AbastevoHttpClient {
    public func stationFeedbackPage(
        stationId: String, fuel: FuelProduct, parentId: String? = nil, cursor: String? = nil
    ) async throws -> StationFeedbackPage {
        guard CommunityWire.isUuid(stationId) else { throw HttpClientError.invalidRequest }
        var path = CommunityWire.stationCommentsPath
        var query = [URLQueryItem(name: "limit", value: "20")]
        if let parentId {
            guard CommunityWire.isUuid(parentId) else { throw HttpClientError.invalidRequest }
            path += "/" + parentId.lowercased() + "/replies"
        } else {
            query.append(URLQueryItem(name: "station_id", value: stationId.lowercased()))
            query.append(URLQueryItem(name: "product", value: fuel.wireCode))
        }
        if let cursor {
            query.append(URLQueryItem(name: "cursor", value: cursor))
        }
        let data = try await publicRead(path: path, query: query, preserveHttpErrors: true)
        return try StationFeedbackWire.decodePage(data, stationId: stationId, fuel: fuel, parentId: parentId)
    }

    public func stationRatingStats(stationId: String, fuel: FuelProduct) async throws -> StationRatingStats {
        guard CommunityWire.isUuid(stationId) else { throw HttpClientError.invalidRequest }
        let data = try await publicRead(
            path: StationFeedbackWire.ratingsPath + "/stats",
            query: [
                URLQueryItem(name: "station_id", value: stationId.lowercased()),
                URLQueryItem(name: "product", value: fuel.wireCode),
            ],
            preserveHttpErrors: true
        )
        let stats = try Self.publicDecoder().decode(StationFeedbackWire.RatingSummary.self, from: data).stats
        guard stats.matches(stationId: stationId, fuel: fuel) else { throw HttpClientError.invalidJson }
        return stats
    }
}

extension CommunityWriteClient {
    public func rateStation(
        stationId: String, fuel: FuelProduct, stars: Int, ownerScope: String, operationId: String
    ) async throws -> StationRatingReceipt {
        guard CommunityWire.isUuid(stationId), PortableFeedback.isValidRating(stars) else {
            throw CommunityWriteError.invalid("feedback.rating-out-of-range")
        }
        let response = try await ownerPost(
            path: StationFeedbackWire.ratingsPath, expectedScope: ownerScope,
            requiresDeviceProof: false, idempotencyKey: operationId
        ) { session in
            StationFeedbackWire.body(session: session, fields: [
                "station_id": stationId.lowercased(), "product": fuel.wireCode, "stars": stars,
            ])
        }
        return try StationFeedbackWire.decodeRating(response.body, stationId: stationId, fuel: fuel, stars: stars)
    }

    public func removeStationRating(
        stationId: String, fuel: FuelProduct, ownerScope: String, operationId: String
    ) async throws {
        guard CommunityWire.isUuid(stationId) else { throw CommunityWriteError.invalid("feedback.target-invalid") }
        let response = try await ownerPost(
            path: StationFeedbackWire.ratingsPath + "/remove", expectedScope: ownerScope,
            requiresDeviceProof: false, idempotencyKey: operationId
        ) { session in
            StationFeedbackWire.body(session: session, fields: [
                "station_id": stationId.lowercased(), "product": fuel.wireCode,
            ])
        }
        let status = try AbastevoHttpClient.publicDecoder().decode(StationFeedbackWire.StatusReceipt.self, from: response.body)
        guard status.status == "deleted" else { throw CommunityWriteError.invalidReceipt }
    }

    public func replyToStationComment(
        _ parent: PublicStationComment, fuel: FuelProduct, text: String, ownerScope: String, operationId: String
    ) async throws -> PublicStationComment {
        guard parent.matches(stationId: parent.stationId, fuel: fuel, parentId: nil) else {
            throw CommunityWriteError.invalid("feedback.parent-invalid")
        }
        let response = try await ownerPost(
            path: CommunityWire.stationCommentsPath + "/" + parent.id.lowercased() + "/replies",
            expectedScope: ownerScope, requiresDeviceProof: false, idempotencyKey: operationId
        ) { session in
            StationFeedbackWire.commentBody(session: session, text: text)
        }
        let comment = try AbastevoHttpClient.publicDecoder().decode(StationFeedbackWire.CommentReceipt.self, from: response.body).comment
        guard comment.matches(stationId: parent.stationId, fuel: fuel, parentId: parent.id),
              comment.revision == 1 else {
            throw CommunityWriteError.invalidReceipt
        }
        return comment
    }

    public func editStationComment(
        _ current: PublicStationComment, fuel: FuelProduct, text: String, ownerScope: String, operationId: String
    ) async throws -> PublicStationComment {
        guard let revision = current.revision, revision > 0, revision < Int64.max,
              current.matches(stationId: current.stationId, fuel: fuel, parentId: current.parentId) else {
            throw CommunityWriteError.invalid("feedback.stale-revision")
        }
        let response = try await ownerPost(
            path: CommunityWire.stationCommentsPath + "/" + current.id.lowercased() + "/edit",
            expectedScope: ownerScope, requiresDeviceProof: false, idempotencyKey: operationId
        ) { session in
            StationFeedbackWire.commentBody(session: session, text: text, expectedRevision: revision)
        }
        let edited = try AbastevoHttpClient.publicDecoder().decode(StationFeedbackWire.CommentReceipt.self, from: response.body).comment
        guard edited.matches(stationId: current.stationId, fuel: fuel, parentId: current.parentId),
              edited.id.lowercased() == current.id.lowercased(),
              edited.revision == revision || edited.revision == revision + 1,
              edited.text == PortableText.normalize(text) else {
            throw CommunityWriteError.invalidReceipt
        }
        return edited
    }

    public func removeStationComment(
        commentId: String, ownerScope: String, operationId: String
    ) async throws {
        guard CommunityWire.isUuid(commentId) else { throw CommunityWriteError.invalid("feedback.comment-not-found") }
        let response = try await ownerPost(
            path: CommunityWire.stationCommentsPath + "/" + commentId.lowercased() + "/remove",
            expectedScope: ownerScope, requiresDeviceProof: false, idempotencyKey: operationId
        ) { session in
            StationFeedbackWire.body(session: session, fields: [:])
        }
        let status = try AbastevoHttpClient.publicDecoder().decode(StationFeedbackWire.StatusReceipt.self, from: response.body)
        guard status.status == "deleted" else { throw CommunityWriteError.invalidReceipt }
    }
}

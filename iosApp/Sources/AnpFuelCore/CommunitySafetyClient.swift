import Foundation

public enum CommunitySafetyContentKind: String, Hashable, Sendable {
    case forumPost = "FORUM_POST"
    case forumComment = "FORUM_COMMENT"
    case stationComment = "STATION_COMMENT"
}

/// A public content reference names an author only on the server (B-BR-SAFE-02).
public struct CommunitySafetyReference: Hashable, Sendable {
    public let kind: CommunitySafetyContentKind
    public let id: String

    public init(kind: CommunitySafetyContentKind, id: String) {
        self.kind = kind
        self.id = CommunitySafetyWire.canonicalId(id)
    }

    public var isWellFormed: Bool { CommunityWire.isUuid(id) }

    fileprivate var fields: [String: String] {
        ["content_kind": kind.rawValue, "content_id": id]
    }
}

public enum CommunityUserReportReason: String, CaseIterable, Hashable, Sendable {
    case spam = "SPAM"
    case harassment = "HARASSMENT"
    case hate = "HATE"
    case impersonation = "IMPERSONATION"
    case illegal = "ILLEGAL"
    case other = "OTHER"
}

public enum CommunityForumReportReason: String, CaseIterable, Hashable, Sendable {
    case spam = "SPAM"
    case harassment = "HARASSMENT"
    case defamation = "DEFAMATION"
    case offTopic = "OFF_TOPIC"
    case illegal = "ILLEGAL"
    case other = "OTHER"
}

public enum CommunityStationReportReason: String, CaseIterable, Hashable, Sendable {
    case spam
    case offensive
    case misleading
    case inappropriate
}

public struct CommunityBlockedUser: Equatable, Sendable, Identifiable {
    public let id: String
    public let label: String
    public let sourceKind: CommunitySafetyContentKind?
    public let createdAt: String
}

public struct CommunityBlockReceipt: Equatable, Sendable {
    public let user: CommunityBlockedUser
    public let replayed: Bool
}

public struct CommunityUserReportReceipt: Equatable, Sendable {
    public let reportId: String
    public let reason: CommunityUserReportReason
    public let createdAt: String
    public let replayed: Bool
}

public struct CommunityForumReportReceipt: Decodable, Equatable, Sendable {
    public let reportId: String
    public let caseId: String
    public let targetType: String
    public let targetId: String
    public let reason: String
    public let status: String
    public let createdAt: String
    public let replayed: Bool
}

public struct CommunityHiddenContentResolution: Equatable, Sendable {
    public let hidden: Set<CommunitySafetyReference>
    public let blockCount: Int?
}

public enum CommunitySafetyError: Error, Equatable, Sendable {
    case invalidTarget
    case invalidDetail
    case invalidResponse
    case signInRequired
    case staleOwner
    case offline
    case refused(status: Int, code: String?, retryAfterSeconds: Int64?)

    public static func from(_ error: Error) -> CommunitySafetyError {
        if let safetyError = error as? CommunitySafetyError { return safetyError }
        if let transportError = error as? SignedTransportError {
            switch transportError {
            case .offline: return .offline
            case .sessionRequired: return .signInRequired
            case .refused(let status, let code, let delay):
                return .refused(status: status, code: code, retryAfterSeconds: delay)
            default: return .invalidResponse
            }
        }
        if let writeError = error as? CommunityWriteError, writeError == .signInRequired {
            return .signInRequired
        }
        return .invalidResponse
    }
}

public enum CommunitySafetyWire {
    public static let maxBlockCount = 500
    public static let maxHiddenBatch = 100
    public static let hiddenLookupBudget: Duration = .seconds(5)
    public static let maxUserDetailScalars = 200
    public static let maxForumDetailUnits = 500

    fileprivate static func canonicalId(_ id: String) -> String {
        id.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
    }

    public static func normalizedUserDetail(_ detail: String?) -> String? {
        guard let detail else { return nil }
        let normalized = PortableText.normalize(detail)
        return normalized.isEmpty ? nil : normalized
    }

    public static func normalizedForumDetail(_ detail: String?) -> String? {
        guard let detail else { return nil }
        let trimmed = detail.trimmingCharacters(in: .whitespacesAndNewlines)
        return trimmed.isEmpty ? nil : trimmed
    }

    public static func userDetailFits(_ detail: String?) -> Bool {
        (normalizedUserDetail(detail)?.unicodeScalars.count ?? 0) <= maxUserDetailScalars
    }

    public static func forumDetailFits(_ detail: String?) -> Bool {
        (normalizedForumDetail(detail)?.utf16.count ?? 0) <= maxForumDetailUnits
    }

    fileprivate static func body(session: PortableAuth.Session, fields: [String: Any]) -> Data? {
        var body = fields
        body["family_id"] = session.familyId
        body["access_token"] = session.accessToken
        return try? JSONSerialization.data(withJSONObject: body, options: [.sortedKeys])
    }

    fileprivate static func decode<T: Decodable>(_ type: T.Type, in response: SignedHttpResponse, expectedStatus: Int = 200) throws -> T {
        guard response.status == expectedStatus else { throw CommunitySafetyError.invalidResponse }
        let decoder = JSONDecoder()
        decoder.keyDecodingStrategy = .convertFromSnakeCase
        guard let decoded = try? decoder.decode(type, from: response.body) else {
            throw CommunitySafetyError.invalidResponse
        }
        return decoded
    }

    fileprivate static func requireId(_ id: String) throws -> String {
        let canonical = canonicalId(id)
        guard CommunityWire.isUuid(canonical) else { throw CommunitySafetyError.invalidTarget }
        return canonical
    }
}

private struct CommunityBlockWire: Decodable {
    let blockId: String
    let label: String?
    let sourceKind: String?
    let createdAt: String

    func validated() throws -> CommunityBlockedUser {
        let canonical = CommunitySafetyWire.canonicalId(blockId)
        guard CommunityWire.isUuid(canonical), !createdAt.isEmpty else {
            throw CommunitySafetyError.invalidResponse
        }
        return CommunityBlockedUser(
            id: canonical,
            label: label ?? "",
            sourceKind: sourceKind.flatMap(CommunitySafetyContentKind.init(rawValue:)),
            createdAt: createdAt
        )
    }
}

extension CommunityWriteClient {
    public func reportCommunityUser(
        _ reference: CommunitySafetyReference,
        reason: CommunityUserReportReason,
        detail: String?,
        operationId: String,
        expectedScope: String
    ) async throws -> CommunityUserReportReceipt {
        guard reference.isWellFormed else { throw CommunitySafetyError.invalidTarget }
        guard CommunitySafetyWire.userDetailFits(detail) else { throw CommunitySafetyError.invalidDetail }
        var fields = reference.fields
        fields["reason"] = reason.rawValue
        fields["detail"] = CommunitySafetyWire.normalizedUserDetail(detail)
        let response = try await safetyPost("/v1/community/user-reports", scope: expectedScope, fields: fields, operationId: operationId)
        struct Envelope: Decodable {
            struct Receipt: Decodable {
                let reportId: String
                let reason: String?
                let createdAt: String
                let replayed: Bool
            }
            let receipt: Receipt
        }
        let receipt = try CommunitySafetyWire.decode(Envelope.self, in: response).receipt
        guard CommunityWire.isUuid(receipt.reportId), !receipt.createdAt.isEmpty else {
            throw CommunitySafetyError.invalidResponse
        }
        return CommunityUserReportReceipt(
            reportId: receipt.reportId,
            reason: CommunityUserReportReason(rawValue: receipt.reason ?? "") ?? .other,
            createdAt: receipt.createdAt,
            replayed: receipt.replayed
        )
    }

    public func blockCommunityUser(
        _ reference: CommunitySafetyReference, operationId: String, expectedScope: String
    ) async throws -> CommunityBlockReceipt {
        guard reference.isWellFormed else { throw CommunitySafetyError.invalidTarget }
        let response = try await safetyPost("/v1/community/blocks", scope: expectedScope, fields: reference.fields, operationId: operationId)
        struct Envelope: Decodable {
            let block: CommunityBlockWire
            let replayed: Bool
        }
        let decoded = try CommunitySafetyWire.decode(Envelope.self, in: response)
        return try CommunityBlockReceipt(user: decoded.block.validated(), replayed: decoded.replayed)
    }

    public func listCommunityBlocks(expectedScope: String) async throws -> [CommunityBlockedUser] {
        let response = try await safetyPost("/v1/community/blocks/list", scope: expectedScope, fields: [:])
        struct Envelope: Decodable { let items: [CommunityBlockWire] }
        let rows = try CommunitySafetyWire.decode(Envelope.self, in: response).items
        guard rows.count <= CommunitySafetyWire.maxBlockCount else { throw CommunitySafetyError.invalidResponse }
        let users = try rows.map { try $0.validated() }
        guard Set(users.map(\.id)).count == users.count else { throw CommunitySafetyError.invalidResponse }
        return users
    }

    public func removeCommunityBlock(
        id: String, operationId: String, expectedScope: String
    ) async throws {
        let id = try CommunitySafetyWire.requireId(id)
        let response = try await safetyPost("/v1/community/blocks/\(id)/remove", scope: expectedScope, fields: [:], operationId: operationId)
        struct Envelope: Decodable { let removed: Bool }
        guard try CommunitySafetyWire.decode(Envelope.self, in: response).removed else {
            throw CommunitySafetyError.invalidResponse
        }
    }

    public func hiddenCommunityContent(
        _ references: [CommunitySafetyReference], expectedScope: String
    ) async throws -> Set<CommunitySafetyReference> {
        var asked: Set<CommunitySafetyReference> = []
        let unique = references.filter { asked.insert($0).inserted }
        guard !unique.isEmpty else { return [] }
        guard unique.count <= CommunitySafetyWire.maxHiddenBatch, unique.allSatisfy(\.isWellFormed) else {
            throw CommunitySafetyError.invalidTarget
        }
        let response = try await safetyPost("/v1/community/blocks/hidden", scope: expectedScope, fields: ["items": unique.map(\.fields)])
        struct Envelope: Decodable {
            struct Reference: Decodable {
                let contentKind: String?
                let contentId: String?
            }
            let hidden: [Reference]
        }
        let rows = try CommunitySafetyWire.decode(Envelope.self, in: response).hidden
        return Set(rows.compactMap { row in
            guard let kind = row.contentKind.flatMap(CommunitySafetyContentKind.init(rawValue:)),
                  let id = row.contentId else { return nil }
            let ref = CommunitySafetyReference(kind: kind, id: id)
            return asked.contains(ref) ? ref : nil
        })
    }

    /// B-BR-SAFE-06: query batches without letting a slow lookup hold up public browsing.
    /// Nil is cancellation, not a decision to show content.
    public func resolveHiddenCommunityContent(
        _ references: [CommunitySafetyReference],
        immediatelyHidden: Set<CommunitySafetyReference>,
        knownBlockCount: Int?,
        expectedScope: String,
        budget: Duration = CommunitySafetyWire.hiddenLookupBudget
    ) async -> CommunityHiddenContentResolution? {
        guard !Task.isCancelled else { return nil }
        var seen: Set<CommunitySafetyReference> = []
        let pending = references.filter {
            $0.isWellFormed && !immediatelyHidden.contains($0) && seen.insert($0).inserted
        }
        let immediate = immediatelyHidden.intersection(Set(references))
        guard !pending.isEmpty, knownBlockCount != 0 else {
            return CommunityHiddenContentResolution(hidden: immediate, blockCount: knownBlockCount)
        }

        let (stream, continuation) = AsyncStream<CommunityHiddenContentResolution>.makeStream(bufferingPolicy: .bufferingOldest(1))
        let lookup = Task {
            var count = knownBlockCount
            if count == nil {
                count = try? await listCommunityBlocks(expectedScope: expectedScope).count
            }
            var hidden = immediate
            if count != 0 {
                for start in stride(from: 0, to: pending.count, by: CommunitySafetyWire.maxHiddenBatch) {
                    guard !Task.isCancelled else { return }
                    let end = min(pending.count, start + CommunitySafetyWire.maxHiddenBatch)
                    do {
                        hidden.formUnion(try await hiddenCommunityContent(Array(pending[start..<end]), expectedScope: expectedScope))
                    } catch {
                        break
                    }
                }
            }
            continuation.yield(CommunityHiddenContentResolution(hidden: hidden, blockCount: count))
        }
        let timeout = Task {
            do {
                try await Task.sleep(for: budget)
                continuation.yield(CommunityHiddenContentResolution(hidden: immediate, blockCount: knownBlockCount))
            } catch {
                return
            }
        }
        defer {
            lookup.cancel()
            timeout.cancel()
            continuation.finish()
        }
        var iterator = stream.makeAsyncIterator()
        let result = await iterator.next()
        return Task.isCancelled ? nil : result
    }

    public func reportForumContent(
        _ reference: CommunitySafetyReference,
        reason: CommunityForumReportReason,
        detail: String?,
        operationId: String,
        expectedScope: String
    ) async throws -> CommunityForumReportReceipt {
        guard reference.isWellFormed, reference.kind != .stationComment else { throw CommunitySafetyError.invalidTarget }
        guard CommunitySafetyWire.forumDetailFits(detail) else { throw CommunitySafetyError.invalidDetail }
        let collection = reference.kind == .forumPost ? "posts" : "comments"
        var fields = ["reason": reason.rawValue]
        fields["detail"] = CommunitySafetyWire.normalizedForumDetail(detail)
        let response = try await safetyPost("/v1/forum/\(collection)/\(reference.id)/report", scope: expectedScope, fields: fields, operationId: operationId)
        struct Envelope: Decodable { let receipt: CommunityForumReportReceipt }
        let receipt = try CommunitySafetyWire.decode(Envelope.self, in: response).receipt
        guard CommunityWire.isUuid(receipt.reportId), CommunityWire.isUuid(receipt.caseId),
              !receipt.status.isEmpty, !receipt.createdAt.isEmpty,
              receipt.targetType == reference.kind.rawValue,
              receipt.targetId.lowercased() == reference.id else {
            throw CommunitySafetyError.invalidResponse
        }
        return receipt
    }

    public func reportStationComment(
        _ reference: CommunitySafetyReference,
        reason: CommunityStationReportReason,
        operationId: String,
        expectedScope: String
    ) async throws {
        guard reference.isWellFormed, reference.kind == .stationComment else { throw CommunitySafetyError.invalidTarget }
        let response = try await safetyPost("/v1/feedback/comments/\(reference.id)/report", scope: expectedScope,
                                            fields: ["reason": reason.rawValue], operationId: operationId, requiresDeviceProof: false)
        struct Envelope: Decodable { let status: String }
        guard try CommunitySafetyWire.decode(Envelope.self, in: response, expectedStatus: 202).status == "reported" else {
            throw CommunitySafetyError.invalidResponse
        }
    }

    private func safetyPost(
        _ path: String, scope: String, fields: [String: Any], operationId: String? = nil, requiresDeviceProof: Bool = true
    ) async throws -> SignedHttpResponse {
        do {
            return try await ownerPost(path: path, expectedScope: scope, requiresDeviceProof: requiresDeviceProof, idempotencyKey: operationId) {
                CommunitySafetyWire.body(session: $0, fields: fields)
            }
        } catch {
            throw CommunitySafetyError.from(error)
        }
    }
}

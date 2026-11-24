import Foundation

/// A public, opaque revision reference for one observation. Feed row IDs identify stations.
public struct CommunityContributionReference: Codable, Equatable, Hashable, Sendable {
    public let observationId: String
    public let decisionSequence: Int64

    public init?(observationId: String, decisionSequence: Int64) {
        guard CommunityWire.isUuid(observationId), decisionSequence > 0 else { return nil }
        self.observationId = observationId
        self.decisionSequence = decisionSequence
    }

    var isValid: Bool {
        CommunityWire.isUuid(observationId) && decisionSequence > 0
    }

    var path: String {
        CommunityWire.observationsPath + "/" + observationId
    }
}

public enum CommunityHelpfulnessChoice: String, Codable, CaseIterable, Sendable {
    case helpful = "HELPFUL"
    case notHelpful = "NOT_HELPFUL"
}

public enum CommunityDisputeReason: String, Codable, CaseIterable, Identifiable, Sendable {
    case priceChanged = "PRICE_CHANGED"
    case wrongStation = "WRONG_STATION"
    case wrongProduct = "WRONG_PRODUCT"
    case wrongCondition = "WRONG_CONDITION"
    case evidenceMismatch = "EVIDENCE_MISMATCH"
    case other = "OTHER"

    public var id: String { rawValue }
}

public enum CommunityObservationAction: Equatable, Sendable {
    case confirm
    case dispute(CommunityDisputeReason)
    case helpfulness(CommunityHelpfulnessChoice)
    case removeHelpfulness

    var pathComponent: String {
        switch self {
        case .confirm: return "confirmations"
        case .dispute: return "disputes"
        case .helpfulness: return "helpfulness"
        case .removeHelpfulness: return "helpfulness/remove"
        }
    }
}

public struct CommunityObservationIntent: Equatable, Sendable {
    public let reference: CommunityContributionReference
    public let ownerScope: String
    public let origin: String
    public let action: CommunityObservationAction
    public let operationId: String

    public init(
        reference: CommunityContributionReference,
        ownerScope: String,
        origin: String,
        action: CommunityObservationAction,
        operationId: String = UUID().uuidString.lowercased()
    ) {
        self.reference = reference
        self.ownerScope = ownerScope
        self.origin = origin
        self.action = action
        self.operationId = operationId
    }

    /// Keep the original key and content on a retry and start a new intent when either changes.
    public static func prepare(
        reference: CommunityContributionReference,
        ownerScope: String,
        origin: String,
        action: CommunityObservationAction,
        retrying previous: CommunityObservationIntent?
    ) -> CommunityObservationIntent {
        if let previous, previous.reference == reference, previous.ownerScope == ownerScope,
           previous.origin == origin, previous.action == action {
            return previous
        }
        return CommunityObservationIntent(reference: reference, ownerScope: ownerScope, origin: origin, action: action)
    }
}

public struct CommunityHelpfulnessTally: Equatable, Sendable {
    public let reference: CommunityContributionReference
    public let helpful: Int64
    public let notHelpful: Int64
    public let total: Int64
    public let helpfulBasisPoints: Int64?
    public let myChoice: CommunityHelpfulnessChoice?
    public let underReview: Bool
    public let ownContribution: Bool?
}

public enum CommunityObservationReceipt: Equatable, Sendable {
    case confirmation(id: String)
    case dispute(id: String)
    case helpfulness(CommunityHelpfulnessTally)
    case removedHelpfulness(CommunityHelpfulnessTally)
}

public enum CommunityObservationRetry {
    /// Foreground retries preserve intent. The signed client supplies a parsed server delay.
    public static func notBefore(_ error: Error, now: Date = Date()) -> Date? {
        guard let error = error as? SignedTransportError else { return nil }
        switch error {
        case .offline, .invalidResponse:
            return now
        case .refused(let status, _, let seconds) where status == 408 || status == 425 || status == 429 || status >= 500:
            return now.addingTimeInterval(TimeInterval(max(0, seconds ?? 0)))
        default:
            return nil
        }
    }
}

public enum CommunityObservationWire {
    public static func body(for intent: CommunityObservationIntent) throws -> Data {
        guard intent.reference.isValid, !intent.operationId.isEmpty, intent.operationId.utf8.count <= 128 else {
            throw SignedTransportError.invalidRequest
        }
        let body: [String: Any]
        switch intent.action {
        case .confirm:
            body = ["client_submission_id": intent.operationId]
        case .dispute(let reason):
            body = ["client_submission_id": intent.operationId, "reason": reason.rawValue]
        case .helpfulness(let choice):
            body = ["client_submission_id": intent.operationId, "decision_sequence": intent.reference.decisionSequence,
                    "choice": choice.rawValue]
        case .removeHelpfulness:
            body = ["decision_sequence": intent.reference.decisionSequence]
        }
        return try JSONSerialization.data(withJSONObject: body, options: [.sortedKeys])
    }

    private struct ReceiptWire: Decodable {
        let confirmationId: String?
        let disputeId: String?
        let observationId: String?
        let targetObservationId: String?
        let state: String?
        let receivedAt: String
    }

    private struct TallyWire: Decodable {
        let observationId: String
        let decisionSequence: Int64
        let helpful: Int64
        let notHelpful: Int64
        let total: Int64?
        let helpfulBasisPoints: Int64?
        let myChoice: CommunityHelpfulnessChoice?
        let underReview: Bool?
        let ownContribution: Bool?
        let voteId: String?
    }

    public static func tally(
        _ data: Data, for reference: CommunityContributionReference, ownerRead: Bool
    ) throws -> CommunityHelpfulnessTally {
        let wire = try AbastevoHttpClient.publicDecoder().decode(TallyWire.self, from: data)
        return try tally(from: wire, reference: reference, ownerRead: ownerRead)
    }

    private static func tally(
        from wire: TallyWire, reference: CommunityContributionReference, ownerRead: Bool
    ) throws -> CommunityHelpfulnessTally {
        let sum = wire.helpful.addingReportingOverflow(wire.notHelpful)
        guard reference.isValid, wire.observationId == reference.observationId,
              wire.decisionSequence == reference.decisionSequence, wire.helpful >= 0, wire.notHelpful >= 0,
              !sum.overflow, wire.total.map({ $0 == sum.partialValue }) ?? true,
              ownerRead || (wire.myChoice == nil && wire.ownContribution == nil) else {
            throw CommunityWriteError.invalidReceipt
        }
        let total = sum.partialValue
        let basisPoints: Int64?
        if total == 0 {
            basisPoints = nil
        } else {
            let scaled = UInt64(wire.helpful).multipliedFullWidth(by: 10_000)
            basisPoints = Int64(UInt64(total).dividingFullWidth(scaled).quotient)
        }
        guard wire.helpfulBasisPoints == basisPoints,
              wire.myChoice != .helpful || wire.helpful > 0,
              wire.myChoice != .notHelpful || wire.notHelpful > 0,
              wire.ownContribution != true || wire.myChoice == nil else {
            throw CommunityWriteError.invalidReceipt
        }
        return CommunityHelpfulnessTally(
            reference: reference, helpful: wire.helpful, notHelpful: wire.notHelpful, total: total,
            helpfulBasisPoints: wire.helpfulBasisPoints, myChoice: wire.myChoice,
            underReview: wire.underReview ?? false, ownContribution: wire.ownContribution
        )
    }

    public static func receipt(_ response: SignedHttpResponse, for intent: CommunityObservationIntent) throws -> CommunityObservationReceipt {
        switch intent.action {
        case .confirm, .dispute:
            guard response.status == 201 else { throw CommunityWriteError.invalidReceipt }
            let wire = try AbastevoHttpClient.publicDecoder().decode(ReceiptWire.self, from: response.body)
            guard AbastevoHttpClient.readIsoDate(wire.receivedAt) != nil else { throw CommunityWriteError.invalidReceipt }
            switch intent.action {
            case .confirm:
                guard wire.observationId == intent.reference.observationId, let id = wire.confirmationId,
                      CommunityWire.isUuid(id) else { throw CommunityWriteError.invalidReceipt }
                return .confirmation(id: id)
            case .dispute:
                guard wire.targetObservationId == intent.reference.observationId, let id = wire.disputeId,
                      CommunityWire.isUuid(id), wire.state == "OPEN" else { throw CommunityWriteError.invalidReceipt }
                return .dispute(id: id)
            default: throw CommunityWriteError.invalidReceipt
            }
        case .helpfulness(let choice):
            guard response.status == 201 else { throw CommunityWriteError.invalidReceipt }
            let wire = try AbastevoHttpClient.publicDecoder().decode(TallyWire.self, from: response.body)
            guard let voteId = wire.voteId, CommunityWire.isUuid(voteId), wire.myChoice == choice else {
                throw CommunityWriteError.invalidReceipt
            }
            return .helpfulness(try tally(from: wire, reference: intent.reference, ownerRead: true))
        case .removeHelpfulness:
            guard response.status == 200 else { throw CommunityWriteError.invalidReceipt }
            let result = try tally(response.body, for: intent.reference, ownerRead: true)
            guard result.myChoice == nil else { throw CommunityWriteError.invalidReceipt }
            return .removedHelpfulness(result)
        }
    }
}

public extension CommunityWriteClient {
    /// These routes authenticate by the existing contributor proof. The signed body never contains an account ID or token.
    func submitObservationAction(_ intent: CommunityObservationIntent) async throws -> CommunityObservationReceipt {
        guard intent.ownerScope.hasPrefix("account:"), transport.origin == intent.origin,
              transport.ownerScope() == intent.ownerScope else {
            throw CommunityWriteError.signInRequired
        }
        let response = try await transport.signedRequest(
            method: "POST",
            path: intent.reference.path + "/" + intent.action.pathComponent,
            body: try CommunityObservationWire.body(for: intent),
            idempotencyKey: intent.operationId,
            expectedOwnerScope: intent.ownerScope
        )
        return try CommunityObservationWire.receipt(response, for: intent)
    }

    func ownerHelpfulness(
        for reference: CommunityContributionReference, ownerScope: String
    ) async throws -> CommunityHelpfulnessTally {
        guard reference.isValid, ownerScope.hasPrefix("account:"), transport.ownerScope() == ownerScope else {
            throw CommunityWriteError.signInRequired
        }
        let response = try await transport.signedRequest(
            method: "GET",
            path: reference.path + "/helpfulness",
            query: "decision_sequence=\(reference.decisionSequence)",
            body: nil,
            expectedOwnerScope: ownerScope
        )
        guard response.status == 200 else { throw CommunityWriteError.invalidReceipt }
        let tally = try CommunityObservationWire.tally(response.body, for: reference, ownerRead: true)
        if tally.ownContribution != nil || tally.myChoice != nil { return tally }
        let ownsContribution: Bool
        do {
            let status = try await transport.signedRequest(
                method: "GET", path: reference.path, body: nil, expectedOwnerScope: ownerScope
            )
            guard CommunityWire.decodeObservationStatus(status.body)?.id == reference.observationId else {
                throw CommunityWriteError.invalidReceipt
            }
            ownsContribution = true
        } catch SignedTransportError.refused(let status, let code, _) where status == 404 && code == "community.not-found" {
            ownsContribution = false
        }
        return CommunityHelpfulnessTally(
            reference: reference, helpful: tally.helpful, notHelpful: tally.notHelpful, total: tally.total,
            helpfulBasisPoints: tally.helpfulBasisPoints, myChoice: tally.myChoice,
            underReview: tally.underReview, ownContribution: ownsContribution
        )
    }
}

public extension AbastevoHttpClient {
    func publicHelpfulness(for reference: CommunityContributionReference) async throws -> CommunityHelpfulnessTally {
        guard reference.isValid else { throw HttpClientError.invalidRequest }
        let data = try await publicRead(
            path: reference.path + "/helpfulness",
            query: [URLQueryItem(name: "decision_sequence", value: String(reference.decisionSequence))]
        )
        return try CommunityObservationWire.tally(data, for: reference, ownerRead: false)
    }
}

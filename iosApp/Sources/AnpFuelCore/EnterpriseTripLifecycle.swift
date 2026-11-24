import Foundation

public enum EnterpriseTripTransitionAction: String, Codable, CaseIterable, Sendable {
    case pause = "trip.pause"
    case resume = "trip.resume"
    case cancel = "trip.cancel"
}

public enum EnterpriseTripLocalState: Equatable, Sendable {
    case running
    case paused
    case cancelled
    case finished
}

/// One local lifecycle declaration. The corresponding queue item owns its server receipt.
public struct EnterpriseTripLifecycleFact: Codable, Equatable, Sendable {
    public let action: EnterpriseTripTransitionAction
    public let operationId: String
    public let declaredAtMillis: Int64
}

public extension EnterpriseTripRecord {
    var localState: EnterpriseTripLocalState {
        if finishOperationId != nil { return .finished }
        switch lifecycle?.last?.action {
        case .pause?: return .paused
        case .cancel?: return .cancelled
        case .resume?, nil: return .running
        }
    }

    var nextLifecycleRevision: Int64 { 1 + Int64(lifecycle?.count ?? 0) }

    func admits(_ action: EnterpriseTripTransitionAction) -> Bool {
        switch (localState, action) {
        case (.running, .pause), (.paused, .resume), (.running, .cancel), (.paused, .cancel):
            return true
        default:
            return false
        }
    }

    mutating func recordTransition(
        _ action: EnterpriseTripTransitionAction, operationId: String, declaredAtMillis: Int64
    ) throws {
        var facts = lifecycle ?? []
        guard admits(action), facts.count < EnterpriseQueue.maxItems,
              EnterpriseValues.isId(operationId), operationId != startOperationId,
              !facts.contains(where: { $0.operationId == operationId }),
              declaredAtMillis >= startedAtMillis, declaredAtMillis >= (facts.last?.declaredAtMillis ?? startedAtMillis) else {
            throw EnterpriseOperationError.malformed
        }
        facts.append(EnterpriseTripLifecycleFact(action: action, operationId: operationId, declaredAtMillis: declaredAtMillis))
        lifecycle = facts
    }
}

public extension EnterpriseOperationBuilder {
    static func transitionTrip(
        envelope: EnterpriseEnvelope, tripId: String, action: EnterpriseTripTransitionAction,
        expectedRevision: Int64, reason: String?, startOperationId: String?
    ) throws -> EnterpriseSealedOperation {
        guard (1...1_000_000).contains(expectedRevision), reason != nil || action != .cancel else {
            throw EnterpriseOperationError.malformed
        }
        if let reason, !EnterpriseValues.isValidText(reason, maxScalars: EnterpriseValues.maxReasonScalars) {
            throw EnterpriseOperationError.malformed
        }
        return try seal(
            envelope: envelope, type: action.rawValue, entityType: "trip", entityId: tripId,
            expectedRevision: expectedRevision, dependsOn: startOperationId.map { [$0] } ?? [],
            softReferences: [], payload: ["reason": reason.map(EnterpriseJson.text) ?? .null]
        )
    }
}

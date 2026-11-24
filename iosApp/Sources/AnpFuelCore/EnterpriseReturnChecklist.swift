import Foundation

public struct EnterpriseReturnChecklistCompletion: Codable, Equatable, Sendable {
    public let tripId: String
    public let runId: String
    public let operationId: String
    public let outcome: EnterpriseChecklistOutcome
    public let completedAtMillis: Int64
}

public enum EnterpriseReturnChecklistError: Error, Equatable, Sendable {
    case binding
    case unavailable
    case notRequired
    case expired
    case beforeStart
    case acknowledgementRequired
    case invalid
}

public struct EnterpriseReturnChecklistDraft: Codable, Equatable, Sendable, Identifiable {
    public let id: String
    public let tripId: String
    public let startedAtMillis: Int64
    public let applicable: EnterpriseChecklistApplicable
    public private(set) var answers: [EnterpriseChecklistAnswer?]
    public private(set) var notes: [String]
    public private(set) var acknowledged: Bool
    public private(set) var completion: EnterpriseReturnChecklistCompletion?

    public init(id: String = EnterpriseValues.newId(), trip: EnterpriseTripRecord,
                applicable: EnterpriseChecklistApplicable, nowMillis: Int64) throws {
        guard EnterpriseValues.isId(id), EnterpriseValues.isId(trip.id), trip.isOpen,
              applicable.context == "RETURN", applicable.assetId == trip.assetId else { throw EnterpriseReturnChecklistError.binding }
        guard applicable.required else { throw EnterpriseReturnChecklistError.notRequired }
        guard applicable.validitySeconds != nil, !applicable.items.isEmpty else { throw EnterpriseReturnChecklistError.invalid }
        guard nowMillis >= trip.startedAtMillis else { throw EnterpriseReturnChecklistError.beforeStart }
        self.id = id
        self.tripId = trip.id
        self.startedAtMillis = nowMillis
        self.applicable = applicable
        self.answers = Array(repeating: nil, count: applicable.items.count)
        self.notes = Array(repeating: "", count: applicable.items.count)
        self.acknowledged = false
    }

    public func matches(_ trip: EnterpriseTripRecord) -> Bool {
        trip.id == tripId && trip.assetId == applicable.assetId && trip.startedAtMillis <= startedAtMillis
    }

    public func isExpired(nowMillis: Int64) -> Bool {
        guard let seconds = applicable.validitySeconds, (3_600...259_200).contains(seconds) else { return true }
        let end = startedAtMillis.addingReportingOverflow(seconds * 1_000)
        return end.overflow || nowMillis > end.partialValue
    }

    public mutating func update(position: Int, answer: EnterpriseChecklistAnswer?, note: String, nowMillis: Int64) throws {
        try checkOpen(nowMillis: nowMillis)
        guard !answers.isEmpty, (1...answers.count).contains(position), notes.count == answers.count,
              EnterpriseValues.isValidText(note, maxScalars: 140, required: false) else { throw EnterpriseReturnChecklistError.invalid }
        if answer != answers[position - 1] { acknowledged = false }
        answers[position - 1] = answer
        notes[position - 1] = note
    }

    public mutating func acknowledge(_ value: Bool, nowMillis: Int64) throws {
        try checkOpen(nowMillis: nowMillis)
        acknowledged = value
    }

    public func evaluate(nowMillis: Int64) throws -> EnterpriseChecklistEvaluation {
        try checkOpen(nowMillis: nowMillis)
        guard let mode = applicable.policyMode else { throw EnterpriseReturnChecklistError.invalid }
        let evaluation = try EnterpriseChecklistRules.evaluate(items: applicable.items, policyMode: mode, answers: answers)
        if evaluation.acknowledgementRequired && !acknowledged { throw EnterpriseReturnChecklistError.acknowledgementRequired }
        return evaluation
    }

    /// Returns the authoritative queue bytes and completed draft together for one protected snapshot write.
    public mutating func complete(envelope: EnterpriseEnvelope, nowMillis: Int64) throws -> EnterpriseSealedOperation {
        let evaluation = try evaluate(nowMillis: nowMillis)
        guard let templateId = applicable.templateId, let templateVersion = applicable.version,
              let policy = applicable.policyRevision, notes.count == answers.count else { throw EnterpriseReturnChecklistError.invalid }
        let responses = try zip(answers, notes).map { answer, note -> EnterpriseOperationBuilder.ChecklistResponse in
            guard let answer else { throw EnterpriseChecklistFailure.incomplete }
            return EnterpriseOperationBuilder.ChecklistResponse(answer: answer, note: note)
        }
        let operation = try EnterpriseOperationBuilder.completeChecklist(
            envelope: envelope, runId: id, assetId: applicable.assetId, context: applicable.context,
            templateId: templateId, templateVersion: templateVersion, policyRevision: policy,
            startedAt: EnterpriseValues.formatInstant(epochMillis: startedAtMillis), responses: responses,
            outcome: evaluation.outcome, acknowledged: evaluation.acknowledgementRequired && acknowledged
        )
        completion = EnterpriseReturnChecklistCompletion(tripId: tripId, runId: id, operationId: operation.operationId,
                                                         outcome: evaluation.outcome, completedAtMillis: nowMillis)
        return operation
    }

    private func checkOpen(nowMillis: Int64) throws {
        guard completion == nil else { throw EnterpriseReturnChecklistError.invalid }
        guard nowMillis >= startedAtMillis else { throw EnterpriseReturnChecklistError.beforeStart }
        guard !isExpired(nowMillis: nowMillis) else { throw EnterpriseReturnChecklistError.expired }
    }
}

/// Existing company snapshot owns these rows. No separate file or photo is created for a checklist.
public struct EnterpriseReturnChecklistBook: Codable, Equatable, Sendable {
    public let membershipId: String
    public let enrollmentId: String
    public private(set) var applicables: [EnterpriseChecklistApplicable] = []
    public private(set) var draft: EnterpriseReturnChecklistDraft?
    public private(set) var serverRun: EnterpriseChecklistRunResult?

    public init(membershipId: String, enrollmentId: String) {
        self.membershipId = membershipId
        self.enrollmentId = enrollmentId
    }

    public func matches(membershipId: String, enrollmentId: String) -> Bool {
        self.membershipId == membershipId && self.enrollmentId == enrollmentId
    }

    public func cached(assetId: String) -> EnterpriseChecklistApplicable? {
        applicables.first { $0.assetId == assetId && $0.context == "RETURN" }
    }

    public mutating func remember(_ applicable: EnterpriseChecklistApplicable) throws {
        guard applicable.context == "RETURN", EnterpriseValues.isId(applicable.assetId) else { throw EnterpriseReturnChecklistError.binding }
        if let previous = cached(assetId: applicable.assetId), previous.evaluatedAtMillis > applicable.evaluatedAtMillis { return }
        applicables.removeAll { $0.assetId == applicable.assetId }
        applicables.append(applicable)
        applicables.sort { $0.evaluatedAtMillis < $1.evaluatedAtMillis }
        if applicables.count > 200 { applicables.removeFirst(applicables.count - 200) }
    }

    public mutating func start(trip: EnterpriseTripRecord, nowMillis: Int64, rejectedRunId: String? = nil) throws -> EnterpriseReturnChecklistDraft {
        if let draft, draft.matches(trip), draft.completion?.runId != rejectedRunId || rejectedRunId == nil {
            if draft.completion != nil || !draft.isExpired(nowMillis: nowMillis) { return draft }
        }
        guard let applicable = cached(assetId: trip.assetId) else { throw EnterpriseReturnChecklistError.unavailable }
        let next = try EnterpriseReturnChecklistDraft(trip: trip, applicable: applicable, nowMillis: nowMillis)
        draft = next
        serverRun = nil
        return next
    }

    public mutating func replace(_ next: EnterpriseReturnChecklistDraft, trip: EnterpriseTripRecord) throws {
        guard let draft, draft.id == next.id, draft.matches(trip), next.matches(trip),
              draft.applicable == next.applicable, draft.startedAtMillis == next.startedAtMillis,
              draft.completion == nil else { throw EnterpriseReturnChecklistError.binding }
        self.draft = next
    }

    public mutating func rememberRun(_ result: EnterpriseChecklistRunResult) throws {
        guard let draft, let local = draft.completion, let templateId = draft.applicable.templateId,
              result.runId == local.runId, result.assetId == draft.applicable.assetId, result.context == "RETURN",
              result.templateId == templateId, result.templateVersion == draft.applicable.version,
              result.policyRevision == draft.applicable.policyRevision, result.deviceOutcome == local.outcome else {
            throw EnterpriseReturnChecklistError.binding
        }
        if let previous = serverRun, previous.revision > result.revision { return }
        serverRun = result
    }
}

public struct EnterpriseChecklistRunResult: Codable, Equatable, Sendable {
    public let runId: String
    public let assetId: String
    public let templateId: String
    public let context: String
    public let templateVersion: Int64
    public let policyRevision: Int64
    public let serverPolicyRevision: Int64
    public let revision: Int64
    public let receivedAt: String
    public let deviceOutcome: EnterpriseChecklistOutcome
    public let recheckOutcome: EnterpriseChecklistOutcome
    public let outcome: EnterpriseChecklistOutcome
    public let outcomeDiffers: Bool
    public let templateSuperseded: Bool
    public let issues: [Int]
    public let criticalIssues: [Int]
}
